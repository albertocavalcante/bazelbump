// Copyright 2025 bazelbump authors
// SPDX-License-Identifier: Apache-2.0

// Package starlark provides a Starlark-based recipe DSL for bazelbump.
//
// This package implements a three-phase execution model:
//  1. Interpretation - Parse Starlark, build step graph, resolve load()
//  2. Validation - Check dependencies exist, validate patterns, verify file access
//  3. Execution - Run operations, track changes, support rollback
//
// Recipes are defined in .star files using a domain-specific API that mirrors
// Bazel's familiar Starlark syntax. See docs/design/starlark-recipes-spec.md
// for the full specification.
package starlark

import (
	"fmt"
	"time"
)

// Recipe represents a parsed Starlark recipe definition.
// A recipe is a coordinated set of steps that migrate a codebase
// from one state to another.
type Recipe struct {
	// Name is the unique identifier for this recipe.
	Name string

	// Version is the semantic version of this recipe (e.g., "1.0.0").
	Version string

	// Description provides human-readable information about what this recipe does.
	Description string

	// MinBazelVersion is the minimum Bazel version this recipe is compatible with.
	// Empty string means no minimum version requirement.
	MinBazelVersion string

	// MaxBazelVersion is the maximum Bazel version this recipe is compatible with.
	// Empty string means no maximum version requirement.
	MaxBazelVersion string

	// Steps is the ordered list of steps to execute.
	Steps []Step

	// OnFailure specifies the behavior when a step fails.
	// Valid values: "rollback" (default), "stop", "continue"
	OnFailure FailureMode

	// SourceFile is the path to the .star file this recipe was loaded from.
	// Empty for recipes created programmatically.
	SourceFile string
}

// FailureMode specifies how to handle step failures.
type FailureMode string

const (
	// FailureModeRollback reverts all changes and stops execution.
	FailureModeRollback FailureMode = "rollback"

	// FailureModeStop stops execution but keeps changes made so far.
	FailureModeStop FailureMode = "stop"

	// FailureModeContinue logs the error and continues with the next step.
	FailureModeContinue FailureMode = "continue"
)

// Validate checks that the recipe is well-formed.
func (r *Recipe) Validate() error {
	if r.Name == "" {
		return fmt.Errorf("recipe name is required")
	}
	if r.Version == "" {
		return fmt.Errorf("recipe version is required")
	}
	if len(r.Steps) == 0 {
		return fmt.Errorf("recipe must have at least one step")
	}

	// Check for duplicate step IDs
	seen := make(map[string]bool)
	for _, step := range r.Steps {
		id := step.GetID()
		if id == "" {
			return fmt.Errorf("step ID is required")
		}
		if seen[id] {
			return fmt.Errorf("duplicate step ID: %s", id)
		}
		seen[id] = true
	}

	// Check that depends_on references exist
	for _, step := range r.Steps {
		for _, dep := range step.GetDependsOn() {
			if !seen[dep] {
				return fmt.Errorf("step %q depends on unknown step %q", step.GetID(), dep)
			}
		}
	}

	return nil
}

// Step is the interface that all recipe steps must implement.
// There are three types of steps: ScanStep, EditStep, and VerifyStep.
type Step interface {
	// GetID returns the unique identifier for this step.
	GetID() string

	// GetName returns the display name for this step.
	GetName() string

	// GetDescription returns the description of what this step does.
	GetDescription() string

	// GetDependsOn returns the IDs of steps this step depends on.
	GetDependsOn() []string

	// stepMarker is a private method to prevent external implementations.
	stepMarker()
}

// baseStep contains fields common to all step types.
type baseStep struct {
	// ID is the unique identifier for this step within the recipe.
	ID string

	// Name is the human-readable display name for this step.
	Name string

	// Description explains what this step does.
	Description string

	// DependsOn lists step IDs that must complete before this step can run.
	// Dependencies can be explicitly specified or auto-inferred from future references.
	DependsOn []string
}

func (b *baseStep) GetID() string          { return b.ID }
func (b *baseStep) GetName() string        { return b.Name }
func (b *baseStep) GetDescription() string { return b.Description }
func (b *baseStep) GetDependsOn() []string { return b.DependsOn }
func (b *baseStep) stepMarker()            {}

// ScanStep detects patterns in the codebase.
// Scan results are stored and can be referenced by later steps using future references.
type ScanStep struct {
	baseStep

	// FilePatterns are glob patterns for files to scan (e.g., "**/BUILD", "*.bzl").
	FilePatterns []string

	// Patterns are regex patterns to detect within files.
	Patterns []string

	// CustomImpl holds a custom Starlark function for scanning.
	// When set, this function is called instead of the default pattern-based scanner.
	// The function receives (ctx) and returns a list of match dicts.
	CustomImpl any
}

// EditStep modifies files using a list of operations.
type EditStep struct {
	baseStep

	// Operations is the list of file modification operations to perform.
	Operations []Operation

	// CustomOperations holds a custom Starlark function for generating operations.
	// When set, this function is called with (ctx) and returns a list of operations.
	CustomOperations any
}

// VerifyStep runs a command to verify correctness after changes.
type VerifyStep struct {
	baseStep

	// Command is the shell command to execute.
	Command string

	// Timeout is the maximum duration to wait for the command.
	// Zero means use the default timeout.
	Timeout time.Duration

	// AllowFailure, when true, allows the step to fail without stopping the recipe.
	AllowFailure bool

	// AcceptableCodes are exit codes that are considered success.
	// If empty, only 0 is considered success.
	AcceptableCodes []int
}

// Operation is the interface that all edit operations must implement.
type Operation interface {
	// GetType returns the operation type (e.g., "ensure_load", "ensure_bazel_dep").
	GetType() string

	// operationMarker is a private method to prevent external implementations.
	operationMarker()
}

// baseOperation contains fields common to all operation types.
type baseOperation struct {
	// Type identifies the operation kind.
	Type string
}

func (b *baseOperation) GetType() string   { return b.Type }
func (b *baseOperation) operationMarker()  {}

// EnsureLoadOp adds a load statement to BUILD files if not already present.
type EnsureLoadOp struct {
	baseOperation

	// Rule is the rule name to load (e.g., "cc_binary").
	Rule string

	// LoadFrom is the load path (e.g., "@rules_cc//cc:defs.bzl").
	LoadFrom string

	// File is the specific file to modify. If empty, applies to all matching files.
	File string
}

// EnsureLoadForDetectedOp adds load statements for all rules found by a scan step.
type EnsureLoadForDetectedOp struct {
	baseOperation

	// ScanResultRef is a reference to the scan step that found the rules.
	ScanResultRef *ScanResult

	// Mapping overrides the default rule-to-load-path mapping.
	// If nil, uses the built-in mapping.
	Mapping map[string]string
}

// RemoveLoadOp removes a load statement from BUILD files.
type RemoveLoadOp struct {
	baseOperation

	// Rule is the rule name to remove from load statements.
	Rule string

	// File is the specific file to modify. If empty, applies to all matching files.
	File string
}

// EnsureBazelDepOp adds a bazel_dep to MODULE.bazel if not already present.
type EnsureBazelDepOp struct {
	baseOperation

	// Name is the dependency name (e.g., "rules_cc").
	Name string

	// Version is the dependency version (e.g., "0.1.1").
	Version string

	// DevDependency marks this as a dev-only dependency.
	DevDependency bool
}

// UpdateBazelDepOp updates the version of an existing bazel_dep.
type UpdateBazelDepOp struct {
	baseOperation

	// Name is the dependency name to update.
	Name string

	// Version is the new version to set.
	Version string
}

// RemoveBazelDepOp removes a bazel_dep from MODULE.bazel.
type RemoveBazelDepOp struct {
	baseOperation

	// Name is the dependency name to remove.
	Name string
}

// RemoveFlagOp removes a flag from .bazelrc.
type RemoveFlagOp struct {
	baseOperation

	// Flag is the flag name to remove (e.g., "incompatible_foo").
	Flag string

	// File is the bazelrc file to modify. Defaults to ".bazelrc".
	File string

	// Config limits removal to a specific config section.
	// If empty, removes from all sections.
	Config string
}

// ReplaceFlagOp replaces a flag in .bazelrc.
type ReplaceFlagOp struct {
	baseOperation

	// Old is the flag to replace.
	Old string

	// New is the replacement flag. If empty, the flag is removed.
	New string

	// File is the bazelrc file to modify. Defaults to ".bazelrc".
	File string
}

// EnsureFlagOp adds a flag to .bazelrc if not already present.
type EnsureFlagOp struct {
	baseOperation

	// Flag is the flag name to add.
	Flag string

	// Value is the flag value. If empty, the flag is a boolean flag.
	Value string

	// Config is the config section to add to. Defaults to "build".
	Config string

	// File is the bazelrc file to modify. Defaults to ".bazelrc".
	File string
}

// ReplaceTextOp performs text replacement in files.
type ReplaceTextOp struct {
	baseOperation

	// Old is the text to find.
	Old string

	// New is the replacement text.
	New string

	// FilePatterns are glob patterns for files to modify.
	FilePatterns []string

	// Regex, when true, treats Old as a regular expression.
	Regex bool
}

// CopyFileOp copies a file.
type CopyFileOp struct {
	baseOperation

	// Src is the source file path.
	Src string

	// Dst is the destination file path.
	Dst string
}

// DeleteFileOp deletes a file.
type DeleteFileOp struct {
	baseOperation

	// Path is the file path or glob pattern to delete.
	Path string
}

// ScanResult holds the results of a scan step.
// During interpretation, this is a "future reference" - a placeholder
// that will be resolved during execution.
type ScanResult struct {
	// StepID is the ID of the scan step that produced this result.
	StepID string

	// Resolved indicates whether this future has been resolved.
	// During interpretation, this is false. After execution, it's true.
	Resolved bool

	// Matches contains all pattern matches found.
	Matches []Match

	// Files is the list of files that contained matches.
	Files []string

	// Count is the total number of matches.
	Count int

	// Rules is the list of unique rule names found.
	Rules []string

	// Families is the list of rule families found (e.g., "cc", "python").
	Families []string
}

// Match represents a single pattern match in a file.
type Match struct {
	// File is the path to the file containing the match.
	File string

	// Line is the 1-based line number of the match.
	Line int

	// Column is the 1-based column number of the match start.
	Column int

	// Text is the matched text.
	Text string

	// Rule is the rule name if this match is a rule invocation.
	Rule string

	// Family is the rule family if this match is a rule invocation.
	Family string
}

// Context holds runtime state during recipe execution.
// The ctx object is available to dynamic operations in Starlark.
type Context struct {
	// ProjectPath is the root directory of the project being migrated.
	ProjectPath string

	// BazelVersion is the detected Bazel version. May be empty if not detected.
	BazelVersion string

	// DryRun is true when running in preview mode without making changes.
	DryRun bool

	// Verbose enables verbose logging output.
	Verbose bool

	// Outputs stores the outputs of completed steps, keyed by step ID.
	// Scan steps store ScanResult, edit steps store lists of changes.
	Outputs map[string]any

	// Changes records all file modifications made during execution.
	// Used for rollback support.
	Changes []*FileChange

	// Logger provides logging functions for recipe steps.
	Logger Logger
}

// FileChange records a modification to a file.
type FileChange struct {
	// Path is the absolute path to the modified file.
	Path string

	// OldContent is the original file content before modification.
	OldContent []byte

	// NewContent is the file content after modification.
	NewContent []byte

	// Description explains what change was made.
	Description string

	// Hunks contains line-level change information for diff display.
	// Populated during execution for detailed diff output.
	Hunks []Hunk
}

// Hunk represents a contiguous block of changes in a file.
type Hunk struct {
	// OldStart is the starting line number in the old content (1-based).
	OldStart int `json:"old_start"`

	// OldCount is the number of lines from old content in this hunk.
	OldCount int `json:"old_count"`

	// NewStart is the starting line number in the new content (1-based).
	NewStart int `json:"new_start"`

	// NewCount is the number of lines in new content for this hunk.
	NewCount int `json:"new_count"`

	// Lines contains the individual line changes in this hunk.
	Lines []LineDiff `json:"lines"`
}

// LineDiff represents a single line change within a hunk.
type LineDiff struct {
	// Type is "add", "remove", or "context".
	Type string `json:"type"`

	// LineNo is the line number (in old file for remove/context, new file for add).
	LineNo int `json:"line"`

	// Content is the line content (without newline).
	Content string `json:"content"`
}

// Logger provides logging functions for recipe execution.
type Logger interface {
	// Info logs an informational message.
	Info(format string, args ...any)

	// Warn logs a warning message.
	Warn(format string, args ...any)

	// Debug logs a debug message (only shown in verbose mode).
	Debug(format string, args ...any)
}

// RecipeError represents an error that occurred during recipe processing.
// It includes source location information when available.
type RecipeError struct {
	// File is the source file where the error occurred.
	File string

	// Line is the 1-based line number where the error occurred.
	Line int

	// Column is the 1-based column number where the error occurred.
	Column int

	// Message is the error message.
	Message string

	// Cause is the underlying error, if any.
	Cause error
}

// Error implements the error interface.
func (e *RecipeError) Error() string {
	if e.File != "" {
		if e.Line > 0 {
			if e.Column > 0 {
				return fmt.Sprintf("%s:%d:%d: %s", e.File, e.Line, e.Column, e.Message)
			}
			return fmt.Sprintf("%s:%d: %s", e.File, e.Line, e.Message)
		}
		return fmt.Sprintf("%s: %s", e.File, e.Message)
	}
	return e.Message
}

// Unwrap returns the underlying cause of this error.
func (e *RecipeError) Unwrap() error {
	return e.Cause
}

// NewRecipeError creates a new recipe error with source location.
func NewRecipeError(file string, line, column int, message string, cause error) *RecipeError {
	return &RecipeError{
		File:    file,
		Line:    line,
		Column:  column,
		Message: message,
		Cause:   cause,
	}
}

// NOTE: Result, StepResult, and ValidationError are defined in executor.go to avoid duplication
