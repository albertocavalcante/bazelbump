// Copyright 2025 bazelbump authors
// SPDX-License-Identifier: Apache-2.0

package starlark

import (
	"embed"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/qri-io/starlib/re"
	"go.starlark.net/starlark"
	"go.starlark.net/starlarkstruct"
)

// Embed the standard library files.
// These files are available via @stdlib// loads in recipes.
//
//go:embed stdlib/*.star stdlib/migrations/*.star
var stdlibFS embed.FS

// Interpreter executes Starlark recipe files and produces Recipe objects.
// It implements the interpretation phase of the three-phase execution model.
type Interpreter struct {
	// thread is the Starlark execution thread.
	thread *starlark.Thread

	// globals contains the predeclared functions available to recipes.
	globals starlark.StringDict

	// loadCache caches loaded modules to avoid re-parsing.
	loadCache map[string]*loadCacheEntry

	// loadStack tracks the current load chain for cycle detection.
	loadStack []string

	// recipeSearchPaths are directories to search for recipes with // prefix.
	recipeSearchPaths []string

	// currentFile tracks the file being parsed for error messages.
	currentFile string

	// recipe is the recipe being built during interpretation.
	recipe *Recipe

	// projectPath is the root directory of the project being analyzed.
	// Used for resolving relative paths in file operations.
	projectPath string
}

// loadCacheEntry holds a cached loaded module.
type loadCacheEntry struct {
	globals starlark.StringDict
	err     error
}

// InterpreterOption configures an Interpreter.
type InterpreterOption func(*Interpreter)

// WithSearchPaths sets the search paths for // prefix loads.
func WithSearchPaths(paths ...string) InterpreterOption {
	return func(i *Interpreter) {
		i.recipeSearchPaths = paths
	}
}

// WithProjectPath sets the project path for resolving relative file paths.
// This is used by file I/O builtins like parse_build_file.
func WithProjectPath(path string) InterpreterOption {
	return func(i *Interpreter) {
		i.projectPath = path
	}
}

// NewInterpreter creates a new Starlark interpreter with the recipe DSL builtins.
func NewInterpreter(opts ...InterpreterOption) *Interpreter {
	interp := &Interpreter{
		loadCache: make(map[string]*loadCacheEntry),
	}

	// Apply options
	for _, opt := range opts {
		opt(interp)
	}

	// Create the thread
	interp.thread = &starlark.Thread{
		Name: "bazelbump-recipe",
		Load: interp.load,
		Print: func(_ *starlark.Thread, msg string) {
			fmt.Println(msg)
		},
	}

	// Set resource limits for sandboxed execution
	// See spec section 14.2: Security Model - Resource Limits
	interp.thread.SetMaxExecutionSteps(100_000)

	// Initialize builtins
	interp.globals = interp.makeBuiltins()

	return interp
}

// makeBuiltins creates the predeclared functions for the recipe DSL.
func (i *Interpreter) makeBuiltins() starlark.StringDict {
	builtins := starlark.StringDict{
		// Core recipe function
		"recipe": starlark.NewBuiltin("recipe", i.builtinRecipe),

		// Step functions
		"scan":   starlark.NewBuiltin("scan", i.builtinScan),
		"edit":   starlark.NewBuiltin("edit", i.builtinEdit),
		"verify": starlark.NewBuiltin("verify", i.builtinVerify),

		// Operation functions - BUILD file operations
		"ensure_load":              starlark.NewBuiltin("ensure_load", i.builtinEnsureLoad),
		"ensure_load_for_detected": starlark.NewBuiltin("ensure_load_for_detected", i.builtinEnsureLoadForDetected),
		"remove_load":              starlark.NewBuiltin("remove_load", i.builtinRemoveLoad),

		// Operation functions - MODULE.bazel operations
		"ensure_bazel_dep": starlark.NewBuiltin("ensure_bazel_dep", i.builtinEnsureBazelDep),
		"update_bazel_dep": starlark.NewBuiltin("update_bazel_dep", i.builtinUpdateBazelDep),
		"remove_bazel_dep": starlark.NewBuiltin("remove_bazel_dep", i.builtinRemoveBazelDep),

		// Operation functions - .bazelrc operations
		"remove_flag":  starlark.NewBuiltin("remove_flag", i.builtinRemoveFlag),
		"replace_flag": starlark.NewBuiltin("replace_flag", i.builtinReplaceFlag),
		"ensure_flag":  starlark.NewBuiltin("ensure_flag", i.builtinEnsureFlag),

		// Operation functions - Generic operations
		"replace_text": starlark.NewBuiltin("replace_text", i.builtinReplaceText),
		"copy_file":    starlark.NewBuiltin("copy_file", i.builtinCopyFile),
		"delete_file":  starlark.NewBuiltin("delete_file", i.builtinDeleteFile),
		"run_command":  starlark.NewBuiltin("run_command", i.builtinRunCommand),

		// Error handling functions
		"fail": starlark.NewBuiltin("fail", i.builtinFail),
		"warn": starlark.NewBuiltin("warn", i.builtinWarn),
		"skip": starlark.NewBuiltin("skip", i.builtinSkip),

		// Struct constructor for custom types
		"struct": starlark.NewBuiltin("struct", starlarkstruct.Make),

		// BUILD file parsing builtins
		"parse_build_file": starlark.NewBuiltin("parse_build_file", builtinParseBuildFile(i.projectPath)),

		// File I/O builtins (sandboxed to project directory)
		"read_file":   starlark.NewBuiltin("read_file", i.builtinReadFile),
		"file_exists": starlark.NewBuiltin("file_exists", i.builtinFileExists),
		"list_dir":    starlark.NewBuiltin("list_dir", i.builtinListDir),
		"glob":        starlark.NewBuiltin("glob", i.builtinGlob),
	}

	// Add the starlib regex module as a predeclared global.
	// This provides re.match(), re.findall(), re.sub(), re.split(), etc.
	reModule, err := re.LoadModule()
	if err == nil {
		for name, value := range reModule {
			builtins[name] = value
		}
	}

	return builtins
}

// LoadFile parses a .star file and returns the defined recipe.
func (i *Interpreter) LoadFile(path string) (*Recipe, error) {
	// Read the file
	content, err := os.ReadFile(path)
	if err != nil {
		return nil, NewRecipeError(path, 0, 0, fmt.Sprintf("failed to read file: %v", err), err)
	}

	// Store the file path for error messages and relative loads
	absPath, err := filepath.Abs(path)
	if err != nil {
		absPath = path
	}
	i.currentFile = absPath

	return i.LoadString(absPath, string(content))
}

// LoadString parses Starlark content and returns the defined recipe.
// This is useful for testing where the recipe content is inline.
func (i *Interpreter) LoadString(name, content string) (*Recipe, error) {
	// Reset state for a new interpretation
	i.recipe = nil
	i.currentFile = name

	// Parse and execute the Starlark code
	_, err := starlark.ExecFile(i.thread, name, content, i.globals)
	if err != nil {
		return nil, i.wrapStarlarkError(err)
	}

	// Check that recipe() was called exactly once
	if i.recipe == nil {
		return nil, NewRecipeError(name, 0, 0, "no recipe defined: recipe() must be called exactly once", nil)
	}

	// Store the source file
	i.recipe.SourceFile = name

	// Validate the recipe
	if err := i.recipe.Validate(); err != nil {
		return nil, NewRecipeError(name, 0, 0, fmt.Sprintf("invalid recipe: %v", err), err)
	}

	return i.recipe, nil
}

// wrapStarlarkError converts a Starlark error to a RecipeError with location info.
func (i *Interpreter) wrapStarlarkError(err error) error {
	if err == nil {
		return nil
	}

	// Try to extract position information from the error
	if evalErr, ok := err.(*starlark.EvalError); ok {
		// Get the first frame of the backtrace for location
		if len(evalErr.CallStack) > 0 {
			pos := evalErr.CallStack[0].Pos
			return NewRecipeError(pos.Filename(), int(pos.Line), int(pos.Col), evalErr.Msg, err)
		}
		return NewRecipeError(i.currentFile, 0, 0, evalErr.Msg, err)
	}

	// For other errors, try to extract position info from the string representation
	// Starlark syntax errors typically include file:line:col in the message

	// Fall back to wrapping the error message
	return NewRecipeError(i.currentFile, 0, 0, err.Error(), err)
}

// load implements the Starlark load() function for importing other modules.
func (i *Interpreter) load(thread *starlark.Thread, module string) (starlark.StringDict, error) {
	// Check for cycles
	for _, loaded := range i.loadStack {
		if loaded == module {
			cycle := append(i.loadStack, module)
			return nil, fmt.Errorf("circular load detected: %s", strings.Join(cycle, " -> "))
		}
	}

	// Check cache
	if entry, ok := i.loadCache[module]; ok {
		return entry.globals, entry.err
	}

	var content []byte
	var modulePath string
	var err error

	// Handle @stdlib// loads specially - read from embedded filesystem
	if strings.HasPrefix(module, "@stdlib//") {
		content, modulePath, err = i.loadStdlib(module)
		if err != nil {
			return nil, err
		}
	} else {
		// Resolve the module path
		modulePath, err = i.resolveLoadPath(module)
		if err != nil {
			return nil, err
		}

		// Read the module content from filesystem
		content, err = os.ReadFile(modulePath)
		if err != nil {
			return nil, fmt.Errorf("failed to load %q: %v", module, err)
		}
	}

	// Push onto load stack for cycle detection
	i.loadStack = append(i.loadStack, module)
	defer func() {
		i.loadStack = i.loadStack[:len(i.loadStack)-1]
	}()

	// Execute the module
	globals, err := starlark.ExecFile(thread, modulePath, content, i.globals)

	// Cache the result
	i.loadCache[module] = &loadCacheEntry{globals: globals, err: err}

	return globals, err
}

// loadStdlib loads a module from the embedded standard library.
// The module path should be in the form "@stdlib//path/to/file.star".
func (i *Interpreter) loadStdlib(module string) ([]byte, string, error) {
	// Convert @stdlib//path to stdlib/path for embedded filesystem
	path := strings.TrimPrefix(module, "@stdlib//")
	embedPath := "stdlib/" + path

	content, err := stdlibFS.ReadFile(embedPath)
	if err != nil {
		return nil, "", fmt.Errorf("stdlib module %q not found: %v", module, err)
	}

	// Return the module path as the @stdlib// path for error messages
	return content, module, nil
}

// resolveLoadPath resolves a module path to an absolute file path.
// Load path formats:
//   - ./path - Relative to current file's directory
//   - ../path - Relative to current file's directory
//   - //path - Relative to recipe search roots
//   - @stdlib//path - Built-in standard library (handled by loadStdlib)
//   - ext://name - Official extension repository (not yet implemented)
//
// Note: @stdlib// paths are handled specially in the load() function
// and should not reach this function.
func (i *Interpreter) resolveLoadPath(module string) (string, error) {
	switch {
	case strings.HasPrefix(module, "./") || strings.HasPrefix(module, "../"):
		// Relative to current file
		if i.currentFile == "" {
			return "", fmt.Errorf("cannot use relative load path without a current file context")
		}
		dir := filepath.Dir(i.currentFile)
		return filepath.Join(dir, module), nil

	case strings.HasPrefix(module, "//"):
		// Relative to search roots
		path := strings.TrimPrefix(module, "//")
		for _, root := range i.recipeSearchPaths {
			candidate := filepath.Join(root, path)
			if _, err := os.Stat(candidate); err == nil {
				return candidate, nil
			}
		}
		return "", fmt.Errorf("module %q not found in search paths", module)

	case strings.HasPrefix(module, "@stdlib//"):
		// This should be handled in load() before reaching here
		// If we get here, something is wrong
		return "", fmt.Errorf("internal error: @stdlib// path should be handled by loadStdlib")

	case strings.HasPrefix(module, "ext://"):
		// Extensions - not yet implemented
		return "", fmt.Errorf("extension loads (ext://) are not yet implemented")

	default:
		return "", fmt.Errorf("invalid load path %q: must start with './', '../', '//', '@stdlib//', or 'ext://'", module)
	}
}

// builtinRecipe implements the recipe() builtin function.
func (i *Interpreter) builtinRecipe(thread *starlark.Thread, fn *starlark.Builtin, args starlark.Tuple, kwargs []starlark.Tuple) (starlark.Value, error) {
	// Check that recipe() hasn't been called already
	if i.recipe != nil {
		return nil, fmt.Errorf("recipe() can only be called once per file")
	}

	var (
		name            string
		version         string
		description     string
		minBazelVersion string
		maxBazelVersion string
		steps           *starlark.List
		onFailure       string = "rollback"
	)

	if err := starlark.UnpackArgs("recipe", args, kwargs,
		"name", &name,
		"version", &version,
		"description?", &description,
		"min_bazel_version?", &minBazelVersion,
		"max_bazel_version?", &maxBazelVersion,
		"steps", &steps,
		"on_failure?", &onFailure,
	); err != nil {
		return nil, err
	}

	// Validate on_failure
	var failureMode FailureMode
	switch onFailure {
	case "rollback":
		failureMode = FailureModeRollback
	case "stop":
		failureMode = FailureModeStop
	case "continue":
		failureMode = FailureModeContinue
	default:
		return nil, fmt.Errorf("invalid on_failure value %q: must be 'rollback', 'stop', or 'continue'", onFailure)
	}

	// Convert steps
	var recipeSteps []Step
	iter := steps.Iterate()
	defer iter.Done()
	var stepVal starlark.Value
	for iter.Next(&stepVal) {
		// Handle scan results (which are also steps)
		if scanResult, ok := stepVal.(*scanResultValue); ok {
			recipeSteps = append(recipeSteps, scanResult.step)
			continue
		}
		// Handle regular steps (edit, verify)
		step, ok := stepVal.(stepValue)
		if !ok {
			return nil, fmt.Errorf("steps must be created with scan(), edit(), or verify(), got %s", stepVal.Type())
		}
		recipeSteps = append(recipeSteps, step.step)
	}

	i.recipe = &Recipe{
		Name:            name,
		Version:         version,
		Description:     description,
		MinBazelVersion: minBazelVersion,
		MaxBazelVersion: maxBazelVersion,
		Steps:           recipeSteps,
		OnFailure:       failureMode,
	}

	return starlark.None, nil
}

// stepValue wraps a Step for use as a Starlark value.
type stepValue struct {
	step Step
}

func (s stepValue) String() string        { return fmt.Sprintf("<step %s>", s.step.GetID()) }
func (s stepValue) Type() string          { return "step" }
func (s stepValue) Freeze()               {}
func (s stepValue) Truth() starlark.Bool  { return true }
func (s stepValue) Hash() (uint32, error) { return 0, fmt.Errorf("unhashable type: step") }

// builtinScan implements the scan() builtin function.
func (i *Interpreter) builtinScan(thread *starlark.Thread, fn *starlark.Builtin, args starlark.Tuple, kwargs []starlark.Tuple) (starlark.Value, error) {
	var (
		id          string
		name        string
		description string
		files       *starlark.List
		patterns    *starlark.List
		impl        starlark.Value
	)

	if err := starlark.UnpackArgs("scan", args, kwargs,
		"id", &id,
		"name?", &name,
		"description?", &description,
		"files?", &files,
		"patterns?", &patterns,
		"impl?", &impl,
	); err != nil {
		return nil, err
	}

	// Check that either impl or (files + patterns) is provided
	hasImpl := impl != nil && impl != starlark.None
	hasPatterns := files != nil && patterns != nil

	if !hasImpl && !hasPatterns {
		return nil, fmt.Errorf("scan() requires either 'impl' or both 'files' and 'patterns'")
	}

	var filePatterns []string
	var patternStrs []string

	if hasPatterns {
		var err error
		filePatterns, err = stringListFromStarlark(files, "files")
		if err != nil {
			return nil, err
		}

		patternStrs, err = stringListFromStarlark(patterns, "patterns")
		if err != nil {
			return nil, err
		}
	}

	step := &ScanStep{
		baseStep: baseStep{
			ID:          id,
			Name:        name,
			Description: description,
		},
		FilePatterns: filePatterns,
		Patterns:     patternStrs,
	}

	// Store custom impl if provided
	if hasImpl {
		callable, ok := impl.(starlark.Callable)
		if !ok {
			return nil, fmt.Errorf("impl must be a callable function, got %s", impl.Type())
		}
		step.CustomImpl = callable
	}

	// Return a future reference that can be used in later steps
	return &scanResultValue{
		stepID: id,
		step:   step,
	}, nil
}

// scanResultValue represents a future reference to scan results.
// It implements starlark.Value and can be passed to operations.
type scanResultValue struct {
	stepID string
	step   *ScanStep
}

func (s *scanResultValue) String() string        { return fmt.Sprintf("<scan_result %s>", s.stepID) }
func (s *scanResultValue) Type() string          { return "scan_result" }
func (s *scanResultValue) Freeze()               {}
func (s *scanResultValue) Truth() starlark.Bool  { return true }
func (s *scanResultValue) Hash() (uint32, error) { return 0, fmt.Errorf("unhashable type: scan_result") }

// Attr implements starlark.HasAttrs for accessing future result attributes.
func (s *scanResultValue) Attr(name string) (starlark.Value, error) {
	switch name {
	case "matches", "files", "count", "rules", "families":
		// Return a placeholder that will be resolved during execution
		return &futureAttr{scanResult: s, attr: name}, nil
	default:
		return nil, starlark.NoSuchAttrError(fmt.Sprintf("scan_result has no attribute %q", name))
	}
}

// AttrNames returns the available attributes.
func (s *scanResultValue) AttrNames() []string {
	return []string{"matches", "files", "count", "rules", "families"}
}

// futureAttr represents an attribute access on a future reference.
type futureAttr struct {
	scanResult *scanResultValue
	attr       string
}

func (f *futureAttr) String() string        { return fmt.Sprintf("<future %s.%s>", f.scanResult.stepID, f.attr) }
func (f *futureAttr) Type() string          { return "future_attr" }
func (f *futureAttr) Freeze()               {}
func (f *futureAttr) Truth() starlark.Bool  { return true }
func (f *futureAttr) Hash() (uint32, error) { return 0, fmt.Errorf("unhashable type: future_attr") }

// The scanResultValue also implements stepValue interface indirectly.
// We need to make scan results usable in the steps list.
var _ starlark.Value = (*scanResultValue)(nil)

// builtinEdit implements the edit() builtin function.
func (i *Interpreter) builtinEdit(thread *starlark.Thread, fn *starlark.Builtin, args starlark.Tuple, kwargs []starlark.Tuple) (starlark.Value, error) {
	var (
		id          string
		name        string
		description string
		dependsOn   *starlark.List
		operations  starlark.Value
	)

	if err := starlark.UnpackArgs("edit", args, kwargs,
		"id", &id,
		"name?", &name,
		"description?", &description,
		"depends_on?", &dependsOn,
		"operations", &operations,
	); err != nil {
		return nil, err
	}

	var deps []string
	if dependsOn != nil {
		var err error
		deps, err = stringListFromStarlark(dependsOn, "depends_on")
		if err != nil {
			return nil, err
		}
	}

	step := &EditStep{
		baseStep: baseStep{
			ID:          id,
			Name:        name,
			Description: description,
			DependsOn:   deps,
		},
	}

	// Check if operations is a callable (custom transform function)
	if callable, ok := operations.(starlark.Callable); ok {
		step.CustomOperations = callable
	} else if opList, ok := operations.(*starlark.List); ok {
		// Convert static operations list
		var ops []Operation
		iter := opList.Iterate()
		defer iter.Done()
		var opVal starlark.Value
		for iter.Next(&opVal) {
			// Skip None values (for conditional operations)
			if opVal == starlark.None {
				continue
			}
			op, ok := opVal.(operationValue)
			if !ok {
				return nil, fmt.Errorf("operations must be created with ensure_load(), ensure_bazel_dep(), etc., got %s", opVal.Type())
			}
			ops = append(ops, op.op)
		}
		step.Operations = ops
	} else {
		return nil, fmt.Errorf("operations must be a list or a callable function, got %s", operations.Type())
	}

	return stepValue{step: step}, nil
}

// builtinVerify implements the verify() builtin function.
func (i *Interpreter) builtinVerify(thread *starlark.Thread, fn *starlark.Builtin, args starlark.Tuple, kwargs []starlark.Tuple) (starlark.Value, error) {
	var (
		id           string
		name         string
		description  string
		dependsOn    *starlark.List
		command      string
		timeout      string = "5m"
		allowFailure bool
	)

	if err := starlark.UnpackArgs("verify", args, kwargs,
		"id", &id,
		"name?", &name,
		"description?", &description,
		"depends_on?", &dependsOn,
		"command", &command,
		"timeout?", &timeout,
		"allow_failure?", &allowFailure,
	); err != nil {
		return nil, err
	}

	var deps []string
	if dependsOn != nil {
		var err error
		deps, err = stringListFromStarlark(dependsOn, "depends_on")
		if err != nil {
			return nil, err
		}
	}

	// Parse timeout
	dur, err := time.ParseDuration(timeout)
	if err != nil {
		return nil, fmt.Errorf("invalid timeout %q: %v", timeout, err)
	}

	step := &VerifyStep{
		baseStep: baseStep{
			ID:          id,
			Name:        name,
			Description: description,
			DependsOn:   deps,
		},
		Command:      command,
		Timeout:      dur,
		AllowFailure: allowFailure,
	}

	return stepValue{step: step}, nil
}

// operationValue wraps an Operation for use as a Starlark value.
type operationValue struct {
	op Operation
}

func (o operationValue) String() string        { return fmt.Sprintf("<operation %s>", o.op.GetType()) }
func (o operationValue) Type() string          { return "operation" }
func (o operationValue) Freeze()               {}
func (o operationValue) Truth() starlark.Bool  { return true }
func (o operationValue) Hash() (uint32, error) { return 0, fmt.Errorf("unhashable type: operation") }

// builtinEnsureLoad implements the ensure_load() builtin.
func (i *Interpreter) builtinEnsureLoad(thread *starlark.Thread, fn *starlark.Builtin, args starlark.Tuple, kwargs []starlark.Tuple) (starlark.Value, error) {
	var (
		rule     string
		loadFrom string
		file     string
	)

	if err := starlark.UnpackArgs("ensure_load", args, kwargs,
		"rule", &rule,
		"load_from", &loadFrom,
		"file?", &file,
	); err != nil {
		return nil, err
	}

	op := &EnsureLoadOp{
		baseOperation: baseOperation{Type: "ensure_load"},
		Rule:          rule,
		LoadFrom:      loadFrom,
		File:          file,
	}

	return operationValue{op: op}, nil
}

// builtinEnsureLoadForDetected implements the ensure_load_for_detected() builtin.
func (i *Interpreter) builtinEnsureLoadForDetected(thread *starlark.Thread, fn *starlark.Builtin, args starlark.Tuple, kwargs []starlark.Tuple) (starlark.Value, error) {
	var (
		detected starlark.Value
		mapping  starlark.Value
	)

	if err := starlark.UnpackArgs("ensure_load_for_detected", args, kwargs,
		"detected", &detected,
		"mapping?", &mapping,
	); err != nil {
		return nil, err
	}

	scanResult, ok := detected.(*scanResultValue)
	if !ok {
		return nil, fmt.Errorf("detected must be a scan result, got %s", detected.Type())
	}

	// Convert mapping if provided
	var mappingMap map[string]string
	if mapping != nil && mapping != starlark.None {
		dict, ok := mapping.(*starlark.Dict)
		if !ok {
			return nil, fmt.Errorf("mapping must be a dict, got %s", mapping.Type())
		}
		mappingMap = make(map[string]string)
		for _, item := range dict.Items() {
			key, ok := starlark.AsString(item[0])
			if !ok {
				return nil, fmt.Errorf("mapping keys must be strings")
			}
			val, ok := starlark.AsString(item[1])
			if !ok {
				return nil, fmt.Errorf("mapping values must be strings")
			}
			mappingMap[key] = val
		}
	}

	op := &EnsureLoadForDetectedOp{
		baseOperation: baseOperation{Type: "ensure_load_for_detected"},
		ScanResultRef: &ScanResult{StepID: scanResult.stepID},
		Mapping:       mappingMap,
	}

	return operationValue{op: op}, nil
}

// builtinRemoveLoad implements the remove_load() builtin.
func (i *Interpreter) builtinRemoveLoad(thread *starlark.Thread, fn *starlark.Builtin, args starlark.Tuple, kwargs []starlark.Tuple) (starlark.Value, error) {
	var (
		rule string
		file string
	)

	if err := starlark.UnpackArgs("remove_load", args, kwargs,
		"rule", &rule,
		"file?", &file,
	); err != nil {
		return nil, err
	}

	op := &RemoveLoadOp{
		baseOperation: baseOperation{Type: "remove_load"},
		Rule:          rule,
		File:          file,
	}

	return operationValue{op: op}, nil
}

// builtinEnsureBazelDep implements the ensure_bazel_dep() builtin.
func (i *Interpreter) builtinEnsureBazelDep(thread *starlark.Thread, fn *starlark.Builtin, args starlark.Tuple, kwargs []starlark.Tuple) (starlark.Value, error) {
	var (
		name          string
		version       string
		devDependency bool
	)

	if err := starlark.UnpackArgs("ensure_bazel_dep", args, kwargs,
		"name", &name,
		"version", &version,
		"dev_dependency?", &devDependency,
	); err != nil {
		return nil, err
	}

	op := &EnsureBazelDepOp{
		baseOperation: baseOperation{Type: "ensure_bazel_dep"},
		Name:          name,
		Version:       version,
		DevDependency: devDependency,
	}

	return operationValue{op: op}, nil
}

// builtinUpdateBazelDep implements the update_bazel_dep() builtin.
func (i *Interpreter) builtinUpdateBazelDep(thread *starlark.Thread, fn *starlark.Builtin, args starlark.Tuple, kwargs []starlark.Tuple) (starlark.Value, error) {
	var (
		name    string
		version string
	)

	if err := starlark.UnpackArgs("update_bazel_dep", args, kwargs,
		"name", &name,
		"version", &version,
	); err != nil {
		return nil, err
	}

	op := &UpdateBazelDepOp{
		baseOperation: baseOperation{Type: "update_bazel_dep"},
		Name:          name,
		Version:       version,
	}

	return operationValue{op: op}, nil
}

// builtinRemoveBazelDep implements the remove_bazel_dep() builtin.
func (i *Interpreter) builtinRemoveBazelDep(thread *starlark.Thread, fn *starlark.Builtin, args starlark.Tuple, kwargs []starlark.Tuple) (starlark.Value, error) {
	var name string

	if err := starlark.UnpackArgs("remove_bazel_dep", args, kwargs,
		"name", &name,
	); err != nil {
		return nil, err
	}

	op := &RemoveBazelDepOp{
		baseOperation: baseOperation{Type: "remove_bazel_dep"},
		Name:          name,
	}

	return operationValue{op: op}, nil
}

// builtinRemoveFlag implements the remove_flag() builtin.
func (i *Interpreter) builtinRemoveFlag(thread *starlark.Thread, fn *starlark.Builtin, args starlark.Tuple, kwargs []starlark.Tuple) (starlark.Value, error) {
	var (
		flag   string
		file   string = ".bazelrc"
		config string
	)

	if err := starlark.UnpackArgs("remove_flag", args, kwargs,
		"flag", &flag,
		"file?", &file,
		"config?", &config,
	); err != nil {
		return nil, err
	}

	op := &RemoveFlagOp{
		baseOperation: baseOperation{Type: "remove_flag"},
		Flag:          flag,
		File:          file,
		Config:        config,
	}

	return operationValue{op: op}, nil
}

// builtinReplaceFlag implements the replace_flag() builtin.
func (i *Interpreter) builtinReplaceFlag(thread *starlark.Thread, fn *starlark.Builtin, args starlark.Tuple, kwargs []starlark.Tuple) (starlark.Value, error) {
	var (
		old  string
		new  string
		file string = ".bazelrc"
	)

	if err := starlark.UnpackArgs("replace_flag", args, kwargs,
		"old", &old,
		"new", &new,
		"file?", &file,
	); err != nil {
		return nil, err
	}

	op := &ReplaceFlagOp{
		baseOperation: baseOperation{Type: "replace_flag"},
		Old:           old,
		New:           new,
		File:          file,
	}

	return operationValue{op: op}, nil
}

// builtinEnsureFlag implements the ensure_flag() builtin.
func (i *Interpreter) builtinEnsureFlag(thread *starlark.Thread, fn *starlark.Builtin, args starlark.Tuple, kwargs []starlark.Tuple) (starlark.Value, error) {
	var (
		flag   string
		value  string
		config string = "build"
		file   string = ".bazelrc"
	)

	if err := starlark.UnpackArgs("ensure_flag", args, kwargs,
		"flag", &flag,
		"value?", &value,
		"config?", &config,
		"file?", &file,
	); err != nil {
		return nil, err
	}

	op := &EnsureFlagOp{
		baseOperation: baseOperation{Type: "ensure_flag"},
		Flag:          flag,
		Value:         value,
		Config:        config,
		File:          file,
	}

	return operationValue{op: op}, nil
}

// builtinReplaceText implements the replace_text() builtin.
func (i *Interpreter) builtinReplaceText(thread *starlark.Thread, fn *starlark.Builtin, args starlark.Tuple, kwargs []starlark.Tuple) (starlark.Value, error) {
	var (
		old   string
		new   string
		files *starlark.List
		regex bool
	)

	if err := starlark.UnpackArgs("replace_text", args, kwargs,
		"old", &old,
		"new", &new,
		"files", &files,
		"regex?", &regex,
	); err != nil {
		return nil, err
	}

	filePatterns, err := stringListFromStarlark(files, "files")
	if err != nil {
		return nil, err
	}

	op := &ReplaceTextOp{
		baseOperation: baseOperation{Type: "replace_text"},
		Old:           old,
		New:           new,
		FilePatterns:  filePatterns,
		Regex:         regex,
	}

	return operationValue{op: op}, nil
}

// builtinCopyFile implements the copy_file() builtin.
func (i *Interpreter) builtinCopyFile(thread *starlark.Thread, fn *starlark.Builtin, args starlark.Tuple, kwargs []starlark.Tuple) (starlark.Value, error) {
	var (
		src string
		dst string
	)

	if err := starlark.UnpackArgs("copy_file", args, kwargs,
		"src", &src,
		"dst", &dst,
	); err != nil {
		return nil, err
	}

	op := &CopyFileOp{
		baseOperation: baseOperation{Type: "copy_file"},
		Src:           src,
		Dst:           dst,
	}

	return operationValue{op: op}, nil
}

// builtinDeleteFile implements the delete_file() builtin.
func (i *Interpreter) builtinDeleteFile(thread *starlark.Thread, fn *starlark.Builtin, args starlark.Tuple, kwargs []starlark.Tuple) (starlark.Value, error) {
	var path string

	if err := starlark.UnpackArgs("delete_file", args, kwargs,
		"path", &path,
	); err != nil {
		return nil, err
	}

	op := &DeleteFileOp{
		baseOperation: baseOperation{Type: "delete_file"},
		Path:          path,
	}

	return operationValue{op: op}, nil
}

// builtinRunCommand implements the run_command() builtin for executing shell commands.
func (i *Interpreter) builtinRunCommand(thread *starlark.Thread, fn *starlark.Builtin, args starlark.Tuple, kwargs []starlark.Tuple) (starlark.Value, error) {
	var (
		command      string
		description  string
		timeout      string
		allowFailure bool
		echo         bool
		capture      bool
		envDict      *starlark.Dict
		workingDir   string
		shell        string
	)

	if err := starlark.UnpackArgs("run_command", args, kwargs,
		"command", &command,
		"description?", &description,
		"timeout?", &timeout,
		"allow_failure?", &allowFailure,
		"echo?", &echo,
		"capture?", &capture,
		"env?", &envDict,
		"working_dir?", &workingDir,
		"shell?", &shell,
	); err != nil {
		return nil, err
	}

	// Default timeout
	if timeout == "" {
		timeout = "5m"
	}

	// Default shell
	if shell == "" {
		shell = "sh"
	}

	// Convert env dict to map
	var env map[string]string
	if envDict != nil {
		env = make(map[string]string)
		for _, item := range envDict.Items() {
			key, ok := starlark.AsString(item[0])
			if !ok {
				return nil, fmt.Errorf("env key must be string, got %s", item[0].Type())
			}
			val, ok := starlark.AsString(item[1])
			if !ok {
				return nil, fmt.Errorf("env value must be string, got %s", item[1].Type())
			}
			env[key] = val
		}
	}

	op := &RunCommandOp{
		baseOperation: baseOperation{Type: "run_command"},
		Command:       command,
		Description:   description,
		Timeout:       timeout,
		AllowFailure:  allowFailure,
		Echo:          echo,
		Capture:       capture,
		Env:           env,
		WorkingDir:    workingDir,
		Shell:         shell,
	}

	return operationValue{op: op}, nil
}

// builtinFail implements the fail() builtin for hard errors.
func (i *Interpreter) builtinFail(thread *starlark.Thread, fn *starlark.Builtin, args starlark.Tuple, kwargs []starlark.Tuple) (starlark.Value, error) {
	var msg string
	if err := starlark.UnpackArgs("fail", args, kwargs, "msg", &msg); err != nil {
		return nil, err
	}
	return nil, fmt.Errorf("fail: %s", msg)
}

// builtinWarn implements the warn() builtin for warnings.
func (i *Interpreter) builtinWarn(thread *starlark.Thread, fn *starlark.Builtin, args starlark.Tuple, kwargs []starlark.Tuple) (starlark.Value, error) {
	var msg string
	if err := starlark.UnpackArgs("warn", args, kwargs, "msg", &msg); err != nil {
		return nil, err
	}
	// Print warning but continue
	fmt.Printf("warning: %s\n", msg)
	return starlark.None, nil
}

// builtinSkip implements the skip() builtin for skipping steps.
func (i *Interpreter) builtinSkip(thread *starlark.Thread, fn *starlark.Builtin, args starlark.Tuple, kwargs []starlark.Tuple) (starlark.Value, error) {
	var msg string
	if err := starlark.UnpackArgs("skip", args, kwargs, "msg", &msg); err != nil {
		return nil, err
	}
	// Return a skip marker that the executor will recognize
	return &skipValue{msg: msg}, nil
}

// skipValue represents a skip() call result.
type skipValue struct {
	msg string
}

func (s *skipValue) String() string        { return fmt.Sprintf("skip(%q)", s.msg) }
func (s *skipValue) Type() string          { return "skip" }
func (s *skipValue) Freeze()               {}
func (s *skipValue) Truth() starlark.Bool  { return true }
func (s *skipValue) Hash() (uint32, error) { return 0, fmt.Errorf("unhashable type: skip") }

// stringListFromStarlark converts a Starlark list to a Go string slice.
func stringListFromStarlark(list *starlark.List, paramName string) ([]string, error) {
	if list == nil {
		return nil, nil
	}

	result := make([]string, 0, list.Len())
	iter := list.Iterate()
	defer iter.Done()

	var val starlark.Value
	for iter.Next(&val) {
		s, ok := starlark.AsString(val)
		if !ok {
			return nil, fmt.Errorf("%s must be a list of strings, got %s", paramName, val.Type())
		}
		result = append(result, s)
	}

	return result, nil
}

// stepFromScanResult creates a Step from a scan result value.
// This allows scan results to be used directly in the steps list.
func (s *scanResultValue) asStep() Step {
	return s.step
}

// Ensure scanResultValue can be used as a step in the steps list.
// We do this by having it also implement stepValue behavior.

// Modify the recipe() builtin to handle scan results specially.
// This is handled in builtinRecipe by checking if the value is a scanResultValue.
