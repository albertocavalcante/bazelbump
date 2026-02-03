// Copyright 2025 bazelbump authors
// SPDX-License-Identifier: Apache-2.0

// Package starlark provides a three-phase execution engine for Starlark recipes.
//
// The execution model follows the spec at docs/design/starlark-recipes-spec.md:
//
//	Phase 1: Interpretation
//	  - Parse Starlark file
//	  - Build step graph
//	  - Resolve load() statements
//	  - Return future references (not actual values)
//
//	Phase 2: Validation
//	  - Check all step dependencies exist
//	  - Validate file patterns are valid globs
//	  - Validate regex patterns compile
//	  - Verify referenced files exist (if possible)
//	  - Report ALL errors before proceeding
//
//	Phase 3: Execution
//	  - Execute steps in topological order
//	  - Resolve future references to actual values
//	  - Track changes for rollback
//	  - Support dry-run mode
package starlark

import (
	"bufio"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
)

// Executor runs Starlark recipes through the three-phase execution model.
type Executor struct {
	interpreter *Interpreter
	projectPath string
	dryRun      bool
	verbose     bool
	logger      Logger
}

// NewExecutor creates a new executor for Starlark recipes.
func NewExecutor(projectPath string, opts Options) *Executor {
	return &Executor{
		interpreter: NewInterpreter(),
		projectPath: projectPath,
		dryRun:      opts.DryRun,
		verbose:     opts.Verbose,
		logger:      opts.Logger,
	}
}

// Options configures recipe execution.
type Options struct {
	DryRun      bool   // Preview changes without applying
	Verbose     bool   // Verbose output
	StepID      string // Run only specific step (empty = all)
	NoVerify    bool   // Skip verification steps
	StopOnError bool   // Stop on first error (default: true for execution)
	Logger      Logger // Logger for output (optional)
}

// Result is the outcome of running a recipe.
type Result struct {
	Recipe      string
	Success     bool
	StepsRun    int
	StepResults []*StepResult
	Changes     []*FileChange
	Error       error
}

// StepResult is the outcome of a single step.
type StepResult struct {
	StepID      string
	StepName    string
	Success     bool
	Skipped     bool
	Description string
	Changes     []*FileChange
	Output      any
	Error       error
}

// Execute runs a recipe through all three phases.
func (e *Executor) Execute(recipe *Recipe, opts Options) (*Result, error) {
	result := &Result{
		Recipe:      recipe.Name,
		StepResults: make([]*StepResult, 0),
		Changes:     make([]*FileChange, 0),
	}

	// Phase 2: Validation (Phase 1 is done by Interpret)
	validationErrors := e.validate(recipe)
	if len(validationErrors) > 0 {
		result.Success = false
		result.Error = NewValidationError(validationErrors...)
		return result, nil
	}

	// Phase 3: Execution
	execResult, err := e.execute(recipe, opts)
	if err != nil {
		result.Success = false
		result.Error = err
		return result, nil
	}

	result.Success = execResult.Success
	result.StepsRun = execResult.StepsRun
	result.StepResults = execResult.StepResults
	result.Changes = execResult.Changes
	result.Error = execResult.Error

	return result, nil
}

// ExecuteFromFile parses and executes a recipe from a .star file.
func (e *Executor) ExecuteFromFile(path string, opts Options) (*Result, error) {
	// Phase 1: Interpretation
	recipe, err := e.Interpret(path)
	if err != nil {
		return nil, fmt.Errorf("interpretation failed: %w", err)
	}

	return e.Execute(recipe, opts)
}

// Interpret parses a Starlark file and builds the recipe (Phase 1).
func (e *Executor) Interpret(path string) (*Recipe, error) {
	return e.interpreter.LoadFile(path)
}

// validate checks a recipe for errors before execution (Phase 2).
// Returns ALL validation errors, not just the first one.
func (e *Executor) validate(recipe *Recipe) []string {
	var errors []string

	// Check recipe metadata
	if recipe.Name == "" {
		errors = append(errors, "recipe name is required")
	}
	if recipe.Version == "" {
		errors = append(errors, "recipe version is required")
	}
	if len(recipe.Steps) == 0 {
		errors = append(errors, "recipe must have at least one step")
	}

	// Build step index for dependency checking
	stepIndex := make(map[string]Step)
	for _, step := range recipe.Steps {
		id := step.GetID()
		if id == "" {
			errors = append(errors, "step has no ID")
			continue
		}
		if _, ok := stepIndex[id]; ok {
			errors = append(errors, fmt.Sprintf("duplicate step ID %q", id))
			continue
		}
		stepIndex[id] = step
	}

	// Validate each step
	for _, step := range recipe.Steps {
		stepErrors := e.validateStep(step, stepIndex)
		errors = append(errors, stepErrors...)
	}

	// Check for dependency cycles
	if cycleErr := e.detectCycles(recipe.Steps, stepIndex); cycleErr != "" {
		errors = append(errors, cycleErr)
	}

	return errors
}

// validateStep validates a single step.
func (e *Executor) validateStep(step Step, stepIndex map[string]Step) []string {
	var errors []string
	stepPrefix := fmt.Sprintf("step %q", step.GetID())

	// Check dependencies exist
	for _, dep := range step.GetDependsOn() {
		if _, ok := stepIndex[dep]; !ok {
			errors = append(errors, fmt.Sprintf("%s depends on unknown step %q", stepPrefix, dep))
		}
	}

	// Validate based on step type
	switch s := step.(type) {
	case *ScanStep:
		errors = append(errors, e.validateScanStep(stepPrefix, s)...)
	case *EditStep:
		errors = append(errors, e.validateEditStep(stepPrefix, s)...)
	case *VerifyStep:
		errors = append(errors, e.validateVerifyStep(stepPrefix, s)...)
	default:
		errors = append(errors, fmt.Sprintf("%s has unknown step type", stepPrefix))
	}

	return errors
}

// validateScanStep validates a scan step.
func (e *Executor) validateScanStep(stepPrefix string, scan *ScanStep) []string {
	var errors []string

	// Check file patterns
	if len(scan.FilePatterns) == 0 {
		errors = append(errors, fmt.Sprintf("%s: scan step requires at least one file pattern", stepPrefix))
	}
	for _, pattern := range scan.FilePatterns {
		if _, err := filepath.Match(pattern, "test"); err != nil {
			errors = append(errors, fmt.Sprintf("%s: invalid glob pattern %q: %v", stepPrefix, pattern, err))
		}
	}

	// Check regex patterns
	if len(scan.Patterns) == 0 {
		errors = append(errors, fmt.Sprintf("%s: scan step requires at least one regex pattern", stepPrefix))
	}
	for _, pattern := range scan.Patterns {
		if _, err := regexp.Compile(pattern); err != nil {
			errors = append(errors, fmt.Sprintf("%s: invalid regex pattern %q: %v", stepPrefix, pattern, err))
		}
	}

	return errors
}

// validateEditStep validates an edit step.
func (e *Executor) validateEditStep(stepPrefix string, edit *EditStep) []string {
	var errors []string

	if len(edit.Operations) == 0 {
		errors = append(errors, fmt.Sprintf("%s: edit step has no operations", stepPrefix))
	}

	for i, op := range edit.Operations {
		opPrefix := fmt.Sprintf("%s operation[%d]", stepPrefix, i)
		errors = append(errors, e.validateOperation(opPrefix, op)...)
	}

	return errors
}

// validateOperation validates a single operation.
func (e *Executor) validateOperation(opPrefix string, op Operation) []string {
	var errors []string

	opType := op.GetType()
	if opType == "" {
		errors = append(errors, fmt.Sprintf("%s: operation type is required", opPrefix))
		return errors
	}

	switch o := op.(type) {
	case *EnsureLoadOp:
		// Rule and LoadFrom are validated during execution
		_ = o // used

	case *EnsureBazelDepOp:
		if o.Name == "" {
			errors = append(errors, fmt.Sprintf("%s: ensure_bazel_dep requires name", opPrefix))
		}
		if o.Version == "" {
			errors = append(errors, fmt.Sprintf("%s: ensure_bazel_dep requires version", opPrefix))
		}

	case *RemoveFlagOp:
		if o.Flag == "" {
			errors = append(errors, fmt.Sprintf("%s: remove_flag requires flag", opPrefix))
		}

	case *ReplaceFlagOp:
		if o.Old == "" {
			errors = append(errors, fmt.Sprintf("%s: replace_flag requires old", opPrefix))
		}

	case *ReplaceTextOp:
		if o.Old == "" {
			errors = append(errors, fmt.Sprintf("%s: replace_text requires old", opPrefix))
		}
		if len(o.FilePatterns) == 0 {
			errors = append(errors, fmt.Sprintf("%s: replace_text requires files", opPrefix))
		}
		// Validate file patterns
		for _, pattern := range o.FilePatterns {
			if _, err := filepath.Match(pattern, "test"); err != nil {
				errors = append(errors, fmt.Sprintf("%s: invalid file pattern %q: %v", opPrefix, pattern, err))
			}
		}
		// Validate regex if specified
		if o.Regex {
			if _, err := regexp.Compile(o.Old); err != nil {
				errors = append(errors, fmt.Sprintf("%s: invalid regex %q: %v", opPrefix, o.Old, err))
			}
		}

	case *CopyFileOp:
		if o.Src == "" {
			errors = append(errors, fmt.Sprintf("%s: copy_file requires src", opPrefix))
		}
		if o.Dst == "" {
			errors = append(errors, fmt.Sprintf("%s: copy_file requires dst", opPrefix))
		}

	case *DeleteFileOp:
		if o.Path == "" {
			errors = append(errors, fmt.Sprintf("%s: delete_file requires path", opPrefix))
		}

	case *EnsureLoadForDetectedOp:
		// Requires a future reference, validated during execution

	case *UpdateBazelDepOp:
		if o.Name == "" {
			errors = append(errors, fmt.Sprintf("%s: update_bazel_dep requires name", opPrefix))
		}
		if o.Version == "" {
			errors = append(errors, fmt.Sprintf("%s: update_bazel_dep requires version", opPrefix))
		}

	case *RemoveBazelDepOp:
		if o.Name == "" {
			errors = append(errors, fmt.Sprintf("%s: remove_bazel_dep requires name", opPrefix))
		}

	case *RemoveLoadOp:
		if o.Rule == "" {
			errors = append(errors, fmt.Sprintf("%s: remove_load requires rule", opPrefix))
		}

	case *EnsureFlagOp:
		if o.Flag == "" {
			errors = append(errors, fmt.Sprintf("%s: ensure_flag requires flag", opPrefix))
		}

	default:
		errors = append(errors, fmt.Sprintf("%s: unknown operation type %q", opPrefix, opType))
	}

	return errors
}

// validateVerifyStep validates a verify step.
func (e *Executor) validateVerifyStep(stepPrefix string, verify *VerifyStep) []string {
	var errors []string

	if verify.Command == "" {
		errors = append(errors, fmt.Sprintf("%s: verify step requires a command", stepPrefix))
	}

	return errors
}

// detectCycles checks for circular dependencies in step graph.
func (e *Executor) detectCycles(steps []Step, stepIndex map[string]Step) string {
	// Build adjacency list
	graph := make(map[string][]string)
	for _, step := range steps {
		graph[step.GetID()] = step.GetDependsOn()
	}

	// Track visited and recursion stack
	visited := make(map[string]bool)
	recStack := make(map[string]bool)
	var cyclePath []string

	var hasCycle func(id string) bool
	hasCycle = func(id string) bool {
		visited[id] = true
		recStack[id] = true
		cyclePath = append(cyclePath, id)

		for _, dep := range graph[id] {
			if !visited[dep] {
				if hasCycle(dep) {
					return true
				}
			} else if recStack[dep] {
				// Found cycle - build the cycle path
				cyclePath = append(cyclePath, dep)
				return true
			}
		}

		recStack[id] = false
		cyclePath = cyclePath[:len(cyclePath)-1]
		return false
	}

	for _, step := range steps {
		id := step.GetID()
		if !visited[id] {
			cyclePath = nil
			if hasCycle(id) {
				return fmt.Sprintf("circular dependency detected: %s", strings.Join(cyclePath, " -> "))
			}
		}
	}

	return ""
}

// execute runs the recipe steps in topological order (Phase 3).
func (e *Executor) execute(recipe *Recipe, opts Options) (*Result, error) {
	ctx := &Context{
		ProjectPath: e.projectPath,
		DryRun:      e.dryRun || opts.DryRun,
		Verbose:     e.verbose || opts.Verbose,
		Outputs:     make(map[string]any),
		Changes:     make([]*FileChange, 0),
	}

	result := &Result{
		Recipe:      recipe.Name,
		StepResults: make([]*StepResult, 0),
		Changes:     make([]*FileChange, 0),
	}

	// Get topologically sorted steps
	sortedSteps, err := e.topologicalSort(recipe.Steps)
	if err != nil {
		return nil, fmt.Errorf("failed to sort steps: %w", err)
	}

	// Execute steps in order
	for _, step := range sortedSteps {
		// Skip if running specific step
		if opts.StepID != "" && step.GetID() != opts.StepID {
			continue
		}

		// Skip verify steps if requested
		if opts.NoVerify {
			if _, ok := step.(*VerifyStep); ok {
				continue
			}
		}

		stepResult := e.executeStep(ctx, step)
		result.StepResults = append(result.StepResults, stepResult)
		result.StepsRun++

		// Store output for dependent steps
		if stepResult.Output != nil {
			ctx.Outputs[step.GetID()] = stepResult.Output
		}

		// Collect changes
		result.Changes = append(result.Changes, stepResult.Changes...)
		ctx.Changes = append(ctx.Changes, stepResult.Changes...)

		// Handle failure
		if !stepResult.Success {
			result.Error = stepResult.Error
			result.Success = false

			// Rollback if configured
			if recipe.OnFailure == "rollback" && !ctx.DryRun {
				if rollbackErr := e.rollback(ctx); rollbackErr != nil {
					result.Error = fmt.Errorf("%w (rollback also failed: %v)", result.Error, rollbackErr)
				}
			}

			// Stop unless configured to continue
			if recipe.OnFailure != "continue" {
				return result, nil
			}
		}
	}

	result.Success = result.Error == nil
	return result, nil
}

// topologicalSort returns steps in dependency order.
func (e *Executor) topologicalSort(steps []Step) ([]Step, error) {
	// Build adjacency and in-degree maps
	inDegree := make(map[string]int)
	dependents := make(map[string][]string)
	stepMap := make(map[string]Step)

	for _, step := range steps {
		id := step.GetID()
		stepMap[id] = step
		if _, ok := inDegree[id]; !ok {
			inDegree[id] = 0
		}
		for _, dep := range step.GetDependsOn() {
			inDegree[id]++
			dependents[dep] = append(dependents[dep], id)
		}
	}

	// Collect steps with no dependencies
	var queue []string
	for id, degree := range inDegree {
		if degree == 0 {
			queue = append(queue, id)
		}
	}

	// Sort queue for deterministic order
	sort.Strings(queue)

	var sorted []Step
	for len(queue) > 0 {
		// Pop from queue
		id := queue[0]
		queue = queue[1:]

		step := stepMap[id]
		sorted = append(sorted, step)

		// Reduce in-degree of dependents
		deps := dependents[id]
		sort.Strings(deps) // Deterministic order
		for _, depID := range deps {
			inDegree[depID]--
			if inDegree[depID] == 0 {
				queue = append(queue, depID)
			}
		}
		sort.Strings(queue) // Keep sorted
	}

	// Check if all steps are included (no cycles)
	if len(sorted) != len(steps) {
		return nil, fmt.Errorf("cycle detected in step dependencies")
	}

	return sorted, nil
}

// executeStep runs a single step.
func (e *Executor) executeStep(ctx *Context, step Step) *StepResult {
	result := &StepResult{
		StepID:   step.GetID(),
		StepName: step.GetName(),
		Changes:  make([]*FileChange, 0),
	}

	var err error

	switch s := step.(type) {
	case *ScanStep:
		var scanResult *ScanResult
		scanResult, err = e.executeScan(ctx, s)
		if err == nil {
			result.Output = scanResult
			result.Description = fmt.Sprintf("Found %d matches in %d files", scanResult.Count, len(scanResult.Files))
		}

	case *EditStep:
		var changes []*FileChange
		changes, err = e.executeEdit(ctx, s)
		if err == nil {
			result.Changes = changes
			result.Description = fmt.Sprintf("Made %d changes", len(changes))
		}

	case *VerifyStep:
		err = e.executeVerify(ctx, s)
		if err == nil {
			result.Description = "Verification passed"
		}

	default:
		err = fmt.Errorf("step %q has unknown type", step.GetID())
	}

	if err != nil {
		result.Success = false
		result.Error = err
	} else {
		result.Success = true
	}

	return result
}

// executeScan runs a scan step and returns the results.
func (e *Executor) executeScan(ctx *Context, scan *ScanStep) (*ScanResult, error) {
	result := &ScanResult{
		Matches: make([]Match, 0),
		Files:   make([]string, 0),
		Rules:   make([]string, 0),
	}

	// Compile regex patterns
	var patterns []*regexp.Regexp
	for _, p := range scan.Patterns {
		re, err := regexp.Compile(p)
		if err != nil {
			return nil, fmt.Errorf("invalid pattern %q: %w", p, err)
		}
		patterns = append(patterns, re)
	}

	// Find matching files
	var matchingFiles []string
	err := filepath.Walk(ctx.ProjectPath, func(path string, info os.FileInfo, walkErr error) error {
		if walkErr != nil {
			return nil
		}
		if info.IsDir() {
			// Skip hidden directories (except project root)
			if strings.HasPrefix(info.Name(), ".") && path != ctx.ProjectPath {
				return filepath.SkipDir
			}
			return nil
		}

		// Check if file matches any pattern
		base := filepath.Base(path)
		for _, pattern := range scan.FilePatterns {
			matched, _ := filepath.Match(pattern, base)
			if matched {
				matchingFiles = append(matchingFiles, path)
				break
			}
		}
		return nil
	})
	if err != nil {
		return nil, err
	}

	// Scan each file for patterns
	filesWithMatches := make(map[string]bool)
	rulesFound := make(map[string]bool)
	familiesFound := make(map[string]bool)

	for _, file := range matchingFiles {
		matches, err := e.scanFile(file, patterns)
		if err != nil {
			return nil, fmt.Errorf("scanning %s: %w", file, err)
		}

		if len(matches) > 0 {
			filesWithMatches[file] = true
			result.Matches = append(result.Matches, matches...)

			for _, m := range matches {
				rulesFound[m.Rule] = true
				if m.Family != "" {
					familiesFound[m.Family] = true
				}
			}
		}
	}

	// Build result
	for f := range filesWithMatches {
		result.Files = append(result.Files, f)
	}
	sort.Strings(result.Files)

	for r := range rulesFound {
		result.Rules = append(result.Rules, r)
	}
	sort.Strings(result.Rules)

	for f := range familiesFound {
		result.Families = append(result.Families, f)
	}
	sort.Strings(result.Families)

	result.Count = len(result.Matches)

	return result, nil
}

// scanFile scans a single file for pattern matches.
func (e *Executor) scanFile(path string, patterns []*regexp.Regexp) ([]Match, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer f.Close()

	var matches []Match
	scanner := bufio.NewScanner(f)
	lineNum := 0

	// Track existing load statements to skip already-loaded rules
	loadedRules := make(map[string]bool)

	for scanner.Scan() {
		lineNum++
		line := scanner.Text()

		// Parse load statements
		if strings.Contains(line, "load(") {
			if idx := strings.Index(line, ","); idx != -1 {
				rest := line[idx+1:]
				re := regexp.MustCompile(`"(\w+)"`)
				symbolMatches := re.FindAllStringSubmatch(rest, -1)
				for _, m := range symbolMatches {
					loadedRules[m[1]] = true
				}
			}
			continue
		}

		// Check for pattern matches
		for _, pattern := range patterns {
			if matchStr := pattern.FindString(line); matchStr != "" {
				// Extract rule name
				ruleName := strings.TrimRight(matchStr, "( \t")

				// Skip if already loaded
				if loadedRules[ruleName] {
					continue
				}

				match := Match{
					File: path,
					Line: lineNum,
					Text: strings.TrimSpace(line),
					Rule: ruleName,
				}

				// Determine family from known mappings
				match.Family = e.getRuleFamily(ruleName)

				matches = append(matches, match)
			}
		}
	}

	return matches, scanner.Err()
}

// getRuleFamily returns the rule family for a known rule.
func (e *Executor) getRuleFamily(rule string) string {
	families := map[string]string{
		"sh_binary":          "shell",
		"sh_test":            "shell",
		"sh_library":         "shell",
		"cc_binary":          "cc",
		"cc_library":         "cc",
		"cc_test":            "cc",
		"cc_import":          "cc",
		"cc_shared_library":  "cc",
		"java_binary":        "java",
		"java_library":       "java",
		"java_test":          "java",
		"java_import":        "java",
		"java_plugin":        "java",
		"py_binary":          "python",
		"py_library":         "python",
		"py_test":            "python",
		"proto_library":      "proto",
		"cc_proto_library":   "proto",
		"java_proto_library": "proto",
		"py_proto_library":   "proto",
	}
	return families[rule]
}

// executeEdit runs an edit step and returns the file changes.
func (e *Executor) executeEdit(ctx *Context, edit *EditStep) ([]*FileChange, error) {
	var changes []*FileChange

	for _, op := range edit.Operations {
		opChanges, err := e.executeOperation(ctx, op)
		if err != nil {
			return changes, err
		}
		changes = append(changes, opChanges...)
	}

	return changes, nil
}

// executeOperation runs a single edit operation.
func (e *Executor) executeOperation(ctx *Context, op Operation) ([]*FileChange, error) {
	switch o := op.(type) {
	case *EnsureLoadOp:
		return e.opEnsureLoad(ctx, o)
	case *EnsureLoadForDetectedOp:
		return e.opEnsureLoadForDetected(ctx, o)
	case *RemoveLoadOp:
		return e.opRemoveLoad(ctx, o)
	case *EnsureBazelDepOp:
		return e.opEnsureBazelDep(ctx, o)
	case *UpdateBazelDepOp:
		return e.opUpdateBazelDep(ctx, o)
	case *RemoveBazelDepOp:
		return e.opRemoveBazelDep(ctx, o)
	case *RemoveFlagOp:
		return e.opRemoveFlag(ctx, o)
	case *ReplaceFlagOp:
		return e.opReplaceFlag(ctx, o)
	case *EnsureFlagOp:
		return e.opEnsureFlag(ctx, o)
	case *ReplaceTextOp:
		return e.opReplaceText(ctx, o)
	case *CopyFileOp:
		return e.opCopyFile(ctx, o)
	case *DeleteFileOp:
		return e.opDeleteFile(ctx, o)
	default:
		return nil, fmt.Errorf("unknown operation type: %s", op.GetType())
	}
}

// opEnsureLoad ensures a load statement exists for a rule.
func (e *Executor) opEnsureLoad(ctx *Context, op *EnsureLoadOp) ([]*FileChange, error) {
	if op.Rule == "" || op.LoadFrom == "" {
		return nil, nil // Nothing to do without specific rule
	}

	// Find BUILD files to update
	var files []string
	if op.File != "" {
		files = []string{filepath.Join(ctx.ProjectPath, op.File)}
	} else {
		// Find all BUILD files
		err := filepath.Walk(ctx.ProjectPath, func(path string, info os.FileInfo, walkErr error) error {
			if walkErr != nil || info.IsDir() {
				return nil
			}
			base := filepath.Base(path)
			if base == "BUILD" || base == "BUILD.bazel" {
				files = append(files, path)
			}
			return nil
		})
		if err != nil {
			return nil, err
		}
	}

	var changes []*FileChange
	for _, file := range files {
		change, err := e.ensureLoadInFile(ctx, file, op.Rule, op.LoadFrom)
		if err != nil {
			return changes, err
		}
		if change != nil {
			changes = append(changes, change)
		}
	}

	return changes, nil
}

// opEnsureLoadForDetected ensures loads for all detected rules.
func (e *Executor) opEnsureLoadForDetected(ctx *Context, op *EnsureLoadForDetectedOp) ([]*FileChange, error) {
	// Get scan results from context
	var scanResult *ScanResult
	for _, output := range ctx.Outputs {
		if sr, ok := output.(*ScanResult); ok {
			scanResult = sr
			break
		}
	}

	if scanResult == nil {
		return nil, nil // No scan results
	}

	// Group matches by file
	matchesByFile := make(map[string][]Match)
	for _, m := range scanResult.Matches {
		matchesByFile[m.File] = append(matchesByFile[m.File], m)
	}

	var changes []*FileChange
	for file, matches := range matchesByFile {
		change, err := e.ensureLoadsForMatches(ctx, file, matches)
		if err != nil {
			return changes, err
		}
		if change != nil {
			changes = append(changes, change)
		}
	}

	return changes, nil
}

// opRemoveLoad removes a load statement from files.
func (e *Executor) opRemoveLoad(ctx *Context, op *RemoveLoadOp) ([]*FileChange, error) {
	// Find BUILD files to update
	var files []string
	if op.File != "" {
		files = []string{filepath.Join(ctx.ProjectPath, op.File)}
	} else {
		err := filepath.Walk(ctx.ProjectPath, func(path string, info os.FileInfo, walkErr error) error {
			if walkErr != nil || info.IsDir() {
				return nil
			}
			base := filepath.Base(path)
			if base == "BUILD" || base == "BUILD.bazel" {
				files = append(files, path)
			}
			return nil
		})
		if err != nil {
			return nil, err
		}
	}

	var changes []*FileChange
	for _, file := range files {
		change, err := e.removeLoadFromFile(ctx, file, op.Rule)
		if err != nil {
			return changes, err
		}
		if change != nil {
			changes = append(changes, change)
		}
	}

	return changes, nil
}

// removeLoadFromFile removes a rule from load statements in a file.
func (e *Executor) removeLoadFromFile(ctx *Context, file, rule string) (*FileChange, error) {
	content, err := os.ReadFile(file)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, err
	}

	lines := strings.Split(string(content), "\n")
	var newLines []string
	changed := false

	for _, line := range lines {
		if strings.Contains(line, "load(") && strings.Contains(line, fmt.Sprintf(`"%s"`, rule)) {
			// Check if this is the only symbol in the load
			re := regexp.MustCompile(`"(\w+)"`)
			matches := re.FindAllStringSubmatch(line, -1)
			if len(matches) <= 2 { // Load path + one symbol
				// Remove the entire load statement
				changed = true
				continue
			}
			// Remove just this symbol from the load
			newLine := regexp.MustCompile(fmt.Sprintf(`,?\s*"%s"\s*,?`, regexp.QuoteMeta(rule))).ReplaceAllString(line, "")
			// Clean up double commas
			newLine = strings.ReplaceAll(newLine, ",,", ",")
			newLine = strings.ReplaceAll(newLine, "(,", "(")
			newLine = strings.ReplaceAll(newLine, ",)", ")")
			newLines = append(newLines, newLine)
			changed = true
		} else {
			newLines = append(newLines, line)
		}
	}

	if !changed {
		return nil, nil
	}

	newContent := []byte(strings.Join(newLines, "\n"))

	change := &FileChange{
		Path:        file,
		OldContent:  content,
		NewContent:  newContent,
		Description: fmt.Sprintf("Remove load statement for %s", rule),
	}

	if !ctx.DryRun {
		if err := os.WriteFile(file, newContent, 0644); err != nil {
			return nil, err
		}
	}

	return change, nil
}

// ensureLoadInFile adds a load statement to a file if needed.
func (e *Executor) ensureLoadInFile(ctx *Context, file, rule, loadFrom string) (*FileChange, error) {
	content, err := os.ReadFile(file)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, err
	}

	// Check if load already exists
	loadPattern := fmt.Sprintf(`load\s*\(\s*"%s"[^)]*"%s"`, regexp.QuoteMeta(loadFrom), regexp.QuoteMeta(rule))
	if matched, _ := regexp.MatchString(loadPattern, string(content)); matched {
		return nil, nil // Already has load
	}

	// Check if rule is actually used in this file
	rulePattern := fmt.Sprintf(`\b%s\s*\(`, regexp.QuoteMeta(rule))
	if matched, _ := regexp.MatchString(rulePattern, string(content)); !matched {
		return nil, nil // Rule not used
	}

	lines := strings.Split(string(content), "\n")
	insertIdx := e.findLoadInsertPoint(lines)

	// Create load statement
	loadStmt := fmt.Sprintf(`load("%s", "%s")`, loadFrom, rule)

	// Insert
	newLines := make([]string, 0, len(lines)+1)
	newLines = append(newLines, lines[:insertIdx]...)
	newLines = append(newLines, loadStmt)
	newLines = append(newLines, lines[insertIdx:]...)

	newContent := []byte(strings.Join(newLines, "\n"))

	change := &FileChange{
		Path:        file,
		OldContent:  content,
		NewContent:  newContent,
		Description: fmt.Sprintf("Add load statement for %s", rule),
	}

	if !ctx.DryRun {
		if err := os.WriteFile(file, newContent, 0644); err != nil {
			return nil, err
		}
	}

	return change, nil
}

// ensureLoadsForMatches adds load statements for all matches in a file.
func (e *Executor) ensureLoadsForMatches(ctx *Context, file string, matches []Match) (*FileChange, error) {
	content, err := os.ReadFile(file)
	if err != nil {
		return nil, err
	}

	// Determine needed loads
	neededLoads := make(map[string][]string) // loadPath -> rules
	for _, m := range matches {
		loadPath := e.getRuleLoadPath(m.Rule)
		if loadPath != "" {
			neededLoads[loadPath] = append(neededLoads[loadPath], m.Rule)
		}
	}

	if len(neededLoads) == 0 {
		return nil, nil
	}

	lines := strings.Split(string(content), "\n")
	insertIdx := e.findLoadInsertPoint(lines)

	// Generate load statements
	var newLoads []string
	for loadPath, rules := range neededLoads {
		// Check if load already exists for this path
		loadExists := false
		for _, line := range lines {
			if strings.Contains(line, loadPath) {
				loadExists = true
				break
			}
		}
		if loadExists {
			continue
		}

		// Deduplicate rules
		seen := make(map[string]bool)
		var unique []string
		for _, r := range rules {
			if !seen[r] {
				seen[r] = true
				unique = append(unique, r)
			}
		}
		sort.Strings(unique)

		// Create load statement
		quotedRules := make([]string, len(unique))
		for i, r := range unique {
			quotedRules[i] = fmt.Sprintf(`"%s"`, r)
		}
		load := fmt.Sprintf(`load("%s", %s)`, loadPath, strings.Join(quotedRules, ", "))
		newLoads = append(newLoads, load)
	}

	if len(newLoads) == 0 {
		return nil, nil
	}

	// Sort for deterministic output
	sort.Strings(newLoads)

	// Insert load statements
	newLines := make([]string, 0, len(lines)+len(newLoads))
	newLines = append(newLines, lines[:insertIdx]...)
	newLines = append(newLines, newLoads...)
	newLines = append(newLines, lines[insertIdx:]...)

	newContent := []byte(strings.Join(newLines, "\n"))

	change := &FileChange{
		Path:        file,
		OldContent:  content,
		NewContent:  newContent,
		Description: fmt.Sprintf("Add load statements for %d rules", len(matches)),
	}

	if !ctx.DryRun {
		if err := os.WriteFile(file, newContent, 0644); err != nil {
			return nil, err
		}
	}

	return change, nil
}

// findLoadInsertPoint finds where to insert new load statements.
func (e *Executor) findLoadInsertPoint(lines []string) int {
	insertIdx := 0
	for i, line := range lines {
		trimmed := strings.TrimSpace(line)
		if strings.HasPrefix(trimmed, "load(") {
			insertIdx = i + 1
		} else if trimmed != "" && !strings.HasPrefix(trimmed, "#") {
			if insertIdx == 0 {
				insertIdx = i
			}
			break
		}
	}
	return insertIdx
}

// getRuleLoadPath returns the load path for a known rule.
func (e *Executor) getRuleLoadPath(rule string) string {
	loadPaths := map[string]string{
		"sh_binary":          "@rules_shell//shell:sh_binary.bzl",
		"sh_test":            "@rules_shell//shell:sh_test.bzl",
		"sh_library":         "@rules_shell//shell:sh_library.bzl",
		"cc_binary":          "@rules_cc//cc:defs.bzl",
		"cc_library":         "@rules_cc//cc:defs.bzl",
		"cc_test":            "@rules_cc//cc:defs.bzl",
		"cc_import":          "@rules_cc//cc:defs.bzl",
		"cc_shared_library":  "@rules_cc//cc:defs.bzl",
		"java_binary":        "@rules_java//java:defs.bzl",
		"java_library":       "@rules_java//java:defs.bzl",
		"java_test":          "@rules_java//java:defs.bzl",
		"java_import":        "@rules_java//java:defs.bzl",
		"java_plugin":        "@rules_java//java:defs.bzl",
		"py_binary":          "@rules_python//python:defs.bzl",
		"py_library":         "@rules_python//python:defs.bzl",
		"py_test":            "@rules_python//python:defs.bzl",
		"proto_library":      "@protobuf//bazel:proto_library.bzl",
		"cc_proto_library":   "@protobuf//bazel:cc_proto_library.bzl",
		"java_proto_library": "@protobuf//bazel:java_proto_library.bzl",
		"py_proto_library":   "@protobuf//bazel:py_proto_library.bzl",
	}
	return loadPaths[rule]
}

// opEnsureBazelDep ensures a bazel_dep exists in MODULE.bazel.
func (e *Executor) opEnsureBazelDep(ctx *Context, op *EnsureBazelDepOp) ([]*FileChange, error) {
	modulePath := filepath.Join(ctx.ProjectPath, "MODULE.bazel")

	content, err := os.ReadFile(modulePath)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil // No MODULE.bazel, skip
		}
		return nil, err
	}

	// Check if dep already exists
	depPattern := fmt.Sprintf(`bazel_dep\s*\(\s*name\s*=\s*"%s"`, regexp.QuoteMeta(op.Name))
	if matched, _ := regexp.MatchString(depPattern, string(content)); matched {
		return nil, nil // Already has dep
	}

	// Find insertion point
	lines := strings.Split(string(content), "\n")
	insertIdx := len(lines)

	for i := len(lines) - 1; i >= 0; i-- {
		if strings.Contains(lines[i], "bazel_dep(") {
			insertIdx = i + 1
			break
		}
	}

	// Create new line
	newDep := fmt.Sprintf(`bazel_dep(name = "%s", version = "%s")`, op.Name, op.Version)

	// Insert
	newLines := make([]string, 0, len(lines)+1)
	newLines = append(newLines, lines[:insertIdx]...)
	newLines = append(newLines, newDep)
	newLines = append(newLines, lines[insertIdx:]...)

	newContent := []byte(strings.Join(newLines, "\n"))

	change := &FileChange{
		Path:        modulePath,
		OldContent:  content,
		NewContent:  newContent,
		Description: fmt.Sprintf("Add bazel_dep for %s", op.Name),
	}

	if !ctx.DryRun {
		if err := os.WriteFile(modulePath, newContent, 0644); err != nil {
			return nil, err
		}
	}

	return []*FileChange{change}, nil
}

// opUpdateBazelDep updates an existing bazel_dep in MODULE.bazel.
func (e *Executor) opUpdateBazelDep(ctx *Context, op *UpdateBazelDepOp) ([]*FileChange, error) {
	modulePath := filepath.Join(ctx.ProjectPath, "MODULE.bazel")

	content, err := os.ReadFile(modulePath)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil // No MODULE.bazel, skip
		}
		return nil, err
	}

	// Find and update the dep version
	pattern := fmt.Sprintf(`(bazel_dep\s*\(\s*name\s*=\s*"%s"\s*,\s*version\s*=\s*")[^"]*"`, regexp.QuoteMeta(op.Name))
	re := regexp.MustCompile(pattern)
	newContentStr := re.ReplaceAllString(string(content), fmt.Sprintf(`${1}%s"`, op.Version))

	if newContentStr == string(content) {
		return nil, nil // No change
	}

	change := &FileChange{
		Path:        modulePath,
		OldContent:  content,
		NewContent:  []byte(newContentStr),
		Description: fmt.Sprintf("Update bazel_dep %s to version %s", op.Name, op.Version),
	}

	if !ctx.DryRun {
		if err := os.WriteFile(modulePath, []byte(newContentStr), 0644); err != nil {
			return nil, err
		}
	}

	return []*FileChange{change}, nil
}

// opRemoveBazelDep removes a bazel_dep from MODULE.bazel.
func (e *Executor) opRemoveBazelDep(ctx *Context, op *RemoveBazelDepOp) ([]*FileChange, error) {
	modulePath := filepath.Join(ctx.ProjectPath, "MODULE.bazel")

	content, err := os.ReadFile(modulePath)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, err
	}

	lines := strings.Split(string(content), "\n")
	var newLines []string
	changed := false

	for _, line := range lines {
		if strings.Contains(line, "bazel_dep(") && strings.Contains(line, fmt.Sprintf(`name = "%s"`, op.Name)) {
			changed = true
			continue
		}
		newLines = append(newLines, line)
	}

	if !changed {
		return nil, nil
	}

	newContent := []byte(strings.Join(newLines, "\n"))

	change := &FileChange{
		Path:        modulePath,
		OldContent:  content,
		NewContent:  newContent,
		Description: fmt.Sprintf("Remove bazel_dep %s", op.Name),
	}

	if !ctx.DryRun {
		if err := os.WriteFile(modulePath, newContent, 0644); err != nil {
			return nil, err
		}
	}

	return []*FileChange{change}, nil
}

// opRemoveFlag removes a flag from .bazelrc.
func (e *Executor) opRemoveFlag(ctx *Context, op *RemoveFlagOp) ([]*FileChange, error) {
	file := op.File
	if file == "" {
		file = ".bazelrc"
	}
	filePath := filepath.Join(ctx.ProjectPath, file)

	content, err := os.ReadFile(filePath)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, err
	}

	lines := strings.Split(string(content), "\n")
	var newLines []string
	changed := false

	for _, line := range lines {
		// Check if config section matches if specified
		if op.Config != "" && strings.Contains(line, op.Flag) {
			if strings.HasPrefix(strings.TrimSpace(line), op.Config+":") {
				changed = true
				continue
			}
		} else if strings.Contains(line, op.Flag) {
			changed = true
			continue
		}
		newLines = append(newLines, line)
	}

	if !changed {
		return nil, nil
	}

	newContent := []byte(strings.Join(newLines, "\n"))

	change := &FileChange{
		Path:        filePath,
		OldContent:  content,
		NewContent:  newContent,
		Description: fmt.Sprintf("Remove flag %s", op.Flag),
	}

	if !ctx.DryRun {
		if err := os.WriteFile(filePath, newContent, 0644); err != nil {
			return nil, err
		}
	}

	return []*FileChange{change}, nil
}

// opReplaceFlag replaces a flag in .bazelrc.
func (e *Executor) opReplaceFlag(ctx *Context, op *ReplaceFlagOp) ([]*FileChange, error) {
	file := op.File
	if file == "" {
		file = ".bazelrc"
	}
	filePath := filepath.Join(ctx.ProjectPath, file)

	content, err := os.ReadFile(filePath)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, err
	}

	newContent := strings.ReplaceAll(string(content), op.Old, op.New)
	if newContent == string(content) {
		return nil, nil // No change
	}

	change := &FileChange{
		Path:        filePath,
		OldContent:  content,
		NewContent:  []byte(newContent),
		Description: fmt.Sprintf("Replace flag %s with %s", op.Old, op.New),
	}

	if !ctx.DryRun {
		if err := os.WriteFile(filePath, []byte(newContent), 0644); err != nil {
			return nil, err
		}
	}

	return []*FileChange{change}, nil
}

// opEnsureFlag ensures a flag exists in .bazelrc.
func (e *Executor) opEnsureFlag(ctx *Context, op *EnsureFlagOp) ([]*FileChange, error) {
	file := op.File
	if file == "" {
		file = ".bazelrc"
	}
	filePath := filepath.Join(ctx.ProjectPath, file)

	content, err := os.ReadFile(filePath)
	if err != nil {
		if os.IsNotExist(err) {
			content = []byte{}
		} else {
			return nil, err
		}
	}

	// Check if flag already exists
	if strings.Contains(string(content), op.Flag) {
		return nil, nil
	}

	// Build the flag line
	config := op.Config
	if config == "" {
		config = "build"
	}
	var flagLine string
	if op.Value != "" {
		flagLine = fmt.Sprintf("%s --%s=%s", config, op.Flag, op.Value)
	} else {
		flagLine = fmt.Sprintf("%s --%s", config, op.Flag)
	}

	// Append to file
	var newContent string
	if len(content) > 0 && !strings.HasSuffix(string(content), "\n") {
		newContent = string(content) + "\n" + flagLine + "\n"
	} else {
		newContent = string(content) + flagLine + "\n"
	}

	change := &FileChange{
		Path:        filePath,
		OldContent:  content,
		NewContent:  []byte(newContent),
		Description: fmt.Sprintf("Add flag %s to %s config", op.Flag, config),
	}

	if !ctx.DryRun {
		if err := os.WriteFile(filePath, []byte(newContent), 0644); err != nil {
			return nil, err
		}
	}

	return []*FileChange{change}, nil
}

// opReplaceText replaces text in files.
func (e *Executor) opReplaceText(ctx *Context, op *ReplaceTextOp) ([]*FileChange, error) {
	var files []string

	// Find files matching patterns
	for _, pattern := range op.FilePatterns {
		err := filepath.Walk(ctx.ProjectPath, func(path string, info os.FileInfo, walkErr error) error {
			if walkErr != nil || info.IsDir() {
				return nil
			}
			matched, _ := filepath.Match(pattern, filepath.Base(path))
			if matched {
				files = append(files, path)
			}
			return nil
		})
		if err != nil {
			return nil, err
		}
	}

	var changes []*FileChange
	for _, file := range files {
		change, err := e.replaceTextInFile(ctx, file, op)
		if err != nil {
			return changes, err
		}
		if change != nil {
			changes = append(changes, change)
		}
	}

	return changes, nil
}

// replaceTextInFile replaces text in a single file.
func (e *Executor) replaceTextInFile(ctx *Context, file string, op *ReplaceTextOp) (*FileChange, error) {
	content, err := os.ReadFile(file)
	if err != nil {
		return nil, err
	}

	var newContent string
	if op.Regex {
		re, err := regexp.Compile(op.Old)
		if err != nil {
			return nil, err
		}
		newContent = re.ReplaceAllString(string(content), op.New)
	} else {
		newContent = strings.ReplaceAll(string(content), op.Old, op.New)
	}

	if newContent == string(content) {
		return nil, nil
	}

	change := &FileChange{
		Path:        file,
		OldContent:  content,
		NewContent:  []byte(newContent),
		Description: fmt.Sprintf("Replace text in %s", filepath.Base(file)),
	}

	if !ctx.DryRun {
		if err := os.WriteFile(file, []byte(newContent), 0644); err != nil {
			return nil, err
		}
	}

	return change, nil
}

// opCopyFile copies a file.
func (e *Executor) opCopyFile(ctx *Context, op *CopyFileOp) ([]*FileChange, error) {
	src := filepath.Join(ctx.ProjectPath, op.Src)
	dst := filepath.Join(ctx.ProjectPath, op.Dst)

	content, err := os.ReadFile(src)
	if err != nil {
		return nil, fmt.Errorf("copy source %s: %w", op.Src, err)
	}

	change := &FileChange{
		Path:        dst,
		OldContent:  nil,
		NewContent:  content,
		Description: fmt.Sprintf("Copy %s to %s", op.Src, op.Dst),
	}

	if !ctx.DryRun {
		if err := os.WriteFile(dst, content, 0644); err != nil {
			return nil, err
		}
	}

	return []*FileChange{change}, nil
}

// opDeleteFile deletes a file.
func (e *Executor) opDeleteFile(ctx *Context, op *DeleteFileOp) ([]*FileChange, error) {
	var files []string

	// Path can be a glob pattern
	if strings.Contains(op.Path, "*") {
		matches, _ := filepath.Glob(filepath.Join(ctx.ProjectPath, op.Path))
		files = append(files, matches...)
	} else {
		files = []string{filepath.Join(ctx.ProjectPath, op.Path)}
	}

	var changes []*FileChange
	for _, file := range files {
		content, err := os.ReadFile(file)
		if err != nil {
			if os.IsNotExist(err) {
				continue
			}
			return changes, err
		}

		change := &FileChange{
			Path:        file,
			OldContent:  content,
			NewContent:  nil,
			Description: fmt.Sprintf("Delete %s", filepath.Base(file)),
		}

		if !ctx.DryRun {
			if err := os.Remove(file); err != nil {
				return changes, err
			}
		}

		changes = append(changes, change)
	}

	return changes, nil
}

// executeVerify runs a verification command.
func (e *Executor) executeVerify(ctx *Context, verify *VerifyStep) error {
	if ctx.DryRun {
		return nil // Skip in dry-run
	}

	// TODO: Implement actual command execution
	// For now, verification is skipped
	// This would use os/exec to run the command with timeout

	return nil
}

// rollback reverts all changes made during execution.
func (e *Executor) rollback(ctx *Context) error {
	// Rollback in reverse order
	for i := len(ctx.Changes) - 1; i >= 0; i-- {
		change := ctx.Changes[i]

		if change.OldContent == nil {
			// File was created, delete it
			if err := os.Remove(change.Path); err != nil && !os.IsNotExist(err) {
				return fmt.Errorf("rollback delete %s: %w", change.Path, err)
			}
		} else if change.NewContent == nil {
			// File was deleted, restore it
			if err := os.WriteFile(change.Path, change.OldContent, 0644); err != nil {
				return fmt.Errorf("rollback restore %s: %w", change.Path, err)
			}
		} else {
			// File was modified, restore original
			if err := os.WriteFile(change.Path, change.OldContent, 0644); err != nil {
				return fmt.Errorf("rollback revert %s: %w", change.Path, err)
			}
		}
	}

	return nil
}

// ValidationError contains all validation errors.
type ValidationError struct {
	Errors []string
}

func (e *ValidationError) Error() string {
	if len(e.Errors) == 1 {
		return e.Errors[0]
	}

	var sb strings.Builder
	sb.WriteString(fmt.Sprintf("validation failed with %d errors:\n", len(e.Errors)))
	for _, err := range e.Errors {
		sb.WriteString("  - ")
		sb.WriteString(err)
		sb.WriteString("\n")
	}
	return sb.String()
}

// NewValidationError creates a validation error from a list of error messages.
func NewValidationError(errors ...string) *ValidationError {
	return &ValidationError{Errors: errors}
}
