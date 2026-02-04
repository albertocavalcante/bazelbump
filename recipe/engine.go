// Copyright 2025 bazelbump authors
// SPDX-License-Identifier: Apache-2.0

package recipe

import (
	"bufio"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"

	"github.com/albertocavalcante/bazelbump/ast"
)

// Engine executes recipes.
type Engine struct {
	recipes map[string]*Recipe
}

// NewEngine creates a new recipe engine with built-in recipes.
func NewEngine() *Engine {
	e := &Engine{
		recipes: make(map[string]*Recipe),
	}
	e.registerBuiltinRecipes()
	return e
}

// registerBuiltinRecipes adds all built-in recipes.
func (e *Engine) registerBuiltinRecipes() {
	e.Register(AutoloadMigrationRecipe())
}

// Register adds a recipe to the engine.
func (e *Engine) Register(r *Recipe) error {
	if err := r.Validate(); err != nil {
		return fmt.Errorf("invalid recipe %q: %w", r.Name, err)
	}
	e.recipes[r.Name] = r
	return nil
}

// List returns summaries of all available recipes.
func (e *Engine) List() []RecipeSummary {
	var result []RecipeSummary
	for _, r := range e.recipes {
		result = append(result, RecipeSummary{
			Name:        r.Name,
			Version:     r.Version,
			Description: r.Description,
			StepCount:   len(r.Steps),
		})
	}
	return result
}

// RecipeSummary provides overview info about a recipe.
type RecipeSummary struct {
	Name        string
	Version     string
	Description string
	StepCount   int
}

// Get returns a recipe by name.
func (e *Engine) Get(name string) (*Recipe, bool) {
	r, ok := e.recipes[name]
	return r, ok
}

// Options configures recipe execution.
type Options struct {
	ProjectPath string
	DryRun      bool
	StepID      string // Run only this step (empty = all)
	Verbose     bool
}

// Run executes a recipe.
func (e *Engine) Run(name string, opts Options) (*Result, error) {
	recipe, ok := e.recipes[name]
	if !ok {
		return nil, fmt.Errorf("unknown recipe: %s", name)
	}

	if opts.ProjectPath == "" {
		opts.ProjectPath = "."
	}

	ctx := &Context{
		ProjectPath: opts.ProjectPath,
		DryRun:      opts.DryRun,
		Outputs:     make(map[string]any),
	}

	result := &Result{
		Recipe: name,
	}

	// Execute steps in order
	for _, step := range recipe.Steps {
		// Skip if running specific step
		if opts.StepID != "" && step.ID != opts.StepID {
			continue
		}

		stepResult, err := e.runStep(ctx, &step)
		result.StepResults = append(result.StepResults, stepResult)
		result.StepsRun++

		if err != nil {
			result.Error = err
			result.Success = false
			return result, nil
		}

		// Store output for dependent steps
		if stepResult.Output != nil {
			ctx.Outputs[step.ID] = stepResult.Output
		}

		// Collect changes
		result.Changes = append(result.Changes, stepResult.Changes...)
	}

	result.Success = true
	return result, nil
}

// runStep executes a single step.
func (e *Engine) runStep(ctx *Context, step *Step) (*StepResult, error) {
	result := &StepResult{
		StepID:   step.ID,
		StepName: step.Name,
	}

	var err error

	switch {
	case step.Scan != nil:
		result.Output, result.Changes, err = e.runScanStep(ctx, step.Scan)
	case step.Edit != nil:
		result.Changes, err = e.runEditStep(ctx, step.Edit)
	case step.Verify != nil:
		err = e.runVerifyStep(ctx, step.Verify)
	default:
		err = fmt.Errorf("step %q has no operation", step.ID)
	}

	if err != nil {
		result.Success = false
		result.Error = err
		return result, err
	}

	result.Success = true
	result.Description = fmt.Sprintf("%s completed", step.Name)
	return result, nil
}

// runScanStep detects patterns in the codebase.
func (e *Engine) runScanStep(ctx *Context, scan *ScanStep) (any, []*FileChange, error) {
	var detected []DetectedRule

	// Compile patterns
	var patterns []*regexp.Regexp
	for _, p := range scan.Patterns {
		re, err := regexp.Compile(p)
		if err != nil {
			return nil, nil, fmt.Errorf("invalid pattern %q: %w", p, err)
		}
		patterns = append(patterns, re)
	}

	// Find files matching patterns
	var allMatches []string

	// Search recursively for BUILD files
	err := filepath.Walk(ctx.ProjectPath, func(path string, info os.FileInfo, walkErr error) error {
		if walkErr != nil {
			return nil
		}
		if info.IsDir() {
			// Skip hidden directories
			if strings.HasPrefix(info.Name(), ".") && info.Name() != "." {
				return filepath.SkipDir
			}
			return nil
		}
		base := filepath.Base(path)
		for _, pattern := range scan.FilePatterns {
			matched, _ := filepath.Match(pattern, base)
			if matched {
				allMatches = append(allMatches, path)
				break
			}
		}
		return nil
	})
	if err != nil {
		return nil, nil, err
	}

	// Remove duplicates
	seen := make(map[string]bool)
	var uniqueMatches []string
	for _, m := range allMatches {
		if !seen[m] {
			seen[m] = true
			uniqueMatches = append(uniqueMatches, m)
		}
	}

	// Scan each file
	for _, path := range uniqueMatches {
		rules, err := scanFileForRules(path, patterns)
		if err != nil {
			return nil, nil, fmt.Errorf("scan %s: %w", path, err)
		}
		detected = append(detected, rules...)
	}

	return detected, nil, nil
}

// scanFileForRules scans a file for native rule usage.
func scanFileForRules(path string, patterns []*regexp.Regexp) ([]DetectedRule, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer f.Close()

	var rules []DetectedRule
	scanner := bufio.NewScanner(f)
	lineNum := 0

	// Track existing load statements
	loadedRules := make(map[string]bool)

	for scanner.Scan() {
		lineNum++
		line := scanner.Text()

		// Check for existing load statements
		if strings.Contains(line, "load(") {
			// Extract loaded symbols
			// Simple pattern: load("...", "symbol1", "symbol2")
			if idx := strings.Index(line, ","); idx != -1 {
				rest := line[idx+1:]
				// Extract quoted strings
				re := regexp.MustCompile(`"(\w+)"`)
				matches := re.FindAllStringSubmatch(rest, -1)
				for _, m := range matches {
					loadedRules[m[1]] = true
				}
			}
			continue
		}

		// Check for rule usage
		for _, pattern := range patterns {
			if match := pattern.FindString(line); match != "" {
				// Extract rule name (remove trailing paren/whitespace)
				ruleName := strings.TrimRight(match, "( \t")

				// Skip if already loaded
				if loadedRules[ruleName] {
					continue
				}

				// Get mapping to determine family
				mapping, ok := GetMapping(ruleName)
				if !ok {
					continue
				}

				rules = append(rules, DetectedRule{
					File:   path,
					Line:   lineNum,
					Rule:   ruleName,
					Family: mapping.Family,
				})
			}
		}
	}

	return rules, scanner.Err()
}

// runEditStep modifies files.
func (e *Engine) runEditStep(ctx *Context, edit *EditStep) ([]*FileChange, error) {
	var changes []*FileChange

	// Get detected rules from previous scan step
	detected, ok := ctx.Outputs["detect-native-rules"].([]DetectedRule)
	if !ok {
		detected = []DetectedRule{}
	}

	// Group rules by file
	rulesByFile := make(map[string][]DetectedRule)
	for _, d := range detected {
		rulesByFile[d.File] = append(rulesByFile[d.File], d)
	}

	// Determine which dependencies are needed
	neededDeps := make(map[string]NativeRuleMapping)
	for _, d := range detected {
		if m, ok := GetMapping(d.Rule); ok {
			neededDeps[m.DepName] = *m
		}
	}

	// Process operations
	for _, op := range edit.Operations {
		switch op.Type {
		case "ensure_bazel_dep":
			change, err := e.ensureBazelDep(ctx, op.DepName, op.DepVersion, neededDeps)
			if err != nil {
				return changes, err
			}
			if change != nil {
				changes = append(changes, change)
			}

		case "ensure_load":
			// Add load statements to files that need them
			for file, rules := range rulesByFile {
				change, err := e.ensureLoadStatements(ctx, file, rules)
				if err != nil {
					return changes, err
				}
				if change != nil {
					changes = append(changes, change)
				}
			}
		}
	}

	return changes, nil
}

// ensureBazelDep adds a bazel_dep to MODULE.bazel if needed.
func (e *Engine) ensureBazelDep(ctx *Context, depName, depVersion string, neededDeps map[string]NativeRuleMapping) (*FileChange, error) {
	modulePath := filepath.Join(ctx.ProjectPath, "MODULE.bazel")

	content, err := os.ReadFile(modulePath)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil // No MODULE.bazel, skip
		}
		return nil, err
	}

	// Check if dep already exists
	depPattern := fmt.Sprintf(`bazel_dep\s*\(\s*name\s*=\s*"%s"`, depName)
	if matched, _ := regexp.MatchString(depPattern, string(content)); matched {
		return nil, nil // Already has dep
	}

	// Check if this dep is needed
	mapping, needed := neededDeps[depName]
	if !needed {
		return nil, nil
	}

	// Find insertion point (after last bazel_dep or at end)
	lines := strings.Split(string(content), "\n")
	insertIdx := len(lines)

	for i := len(lines) - 1; i >= 0; i-- {
		if strings.Contains(lines[i], "bazel_dep(") {
			insertIdx = i + 1
			break
		}
	}

	// Create new line
	newDep := fmt.Sprintf(`bazel_dep(name = "%s", version = "%s")`, mapping.DepName, mapping.DepVersion)

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
		Description: fmt.Sprintf("Add bazel_dep for %s", depName),
	}

	if !ctx.DryRun {
		if err := os.WriteFile(modulePath, newContent, 0644); err != nil {
			return nil, err
		}
	}

	return change, nil
}

// ensureLoadStatements adds load statements for native rules.
func (e *Engine) ensureLoadStatements(ctx *Context, file string, rules []DetectedRule) (*FileChange, error) {
	content, err := os.ReadFile(file)
	if err != nil {
		return nil, err
	}

	// Determine which loads are needed
	neededLoads := make(map[string][]string) // loadPath -> rules
	for _, r := range rules {
		if m, ok := GetMapping(r.Rule); ok {
			neededLoads[m.LoadPath] = append(neededLoads[m.LoadPath], r.Rule)
		}
	}

	if len(neededLoads) == 0 {
		return nil, nil
	}

	// Parse with AST for correct multi-line load handling
	f, err := ast.ParseFileContent(file, content)
	if err != nil {
		// If AST parsing fails, skip this file
		return nil, nil
	}

	// Apply AST operations for each needed load
	anyModified := false
	for loadPath, ruleNames := range neededLoads {
		// Deduplicate rules
		seen := make(map[string]bool)
		var unique []string
		for _, r := range ruleNames {
			if !seen[r] {
				seen[r] = true
				unique = append(unique, r)
			}
		}
		sort.Strings(unique)

		modified, err := ast.EnsureLoad(f, ast.EnsureLoadOptions{
			Module:   loadPath,
			Symbols:  unique,
			Position: ast.LoadPositionTop,
		})
		if err != nil {
			return nil, fmt.Errorf("ensure_load in %s: %w", file, err)
		}
		if modified {
			anyModified = true
		}
	}

	if !anyModified {
		return nil, nil
	}

	// Format output
	newContent := f.Format()

	// Validate output is valid Starlark (re-parse to check syntax)
	if _, err := ast.ParseFileContent(file, newContent); err != nil {
		return nil, fmt.Errorf("invalid output for %s: %w", file, err)
	}

	change := &FileChange{
		Path:        file,
		OldContent:  content,
		NewContent:  newContent,
		Description: fmt.Sprintf("Add load statements for %d rules", len(rules)),
	}

	if !ctx.DryRun {
		if err := os.WriteFile(file, newContent, 0644); err != nil {
			return nil, err
		}
	}

	return change, nil
}

// runVerifyStep runs a verification command.
func (e *Engine) runVerifyStep(ctx *Context, verify *VerifyStep) error {
	if ctx.DryRun {
		return nil // Skip verification in dry-run
	}

	// TODO: Actually run the command
	// For now, skip verification
	return nil
}
