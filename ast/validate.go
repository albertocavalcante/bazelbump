// Copyright 2025 bazelbump authors
// SPDX-License-Identifier: Apache-2.0

package ast

import (
	"fmt"
	"strings"

	"github.com/albertocavalcante/starlark-syntax-go/bzl"
)

// ValidationLevel controls validation strictness.
type ValidationLevel int

const (
	// ValidationSyntax only checks that output is valid Starlark syntax.
	ValidationSyntax ValidationLevel = iota
	// ValidationSemantic also checks semantic correctness (valid load paths, etc.).
	ValidationSemantic
)

// ValidationError represents a validation failure.
type ValidationError struct {
	File    string
	Line    int
	Column  int
	Message string
}

// Error implements the error interface.
func (e *ValidationError) Error() string {
	if e.Line > 0 {
		return fmt.Sprintf("%s:%d:%d: %s", e.File, e.Line, e.Column, e.Message)
	}
	return fmt.Sprintf("%s: %s", e.File, e.Message)
}

// ValidationResult holds the results of validation.
type ValidationResult struct {
	Valid  bool
	Errors []*ValidationError
}

// Validator validates BUILD file transformations.
type Validator struct {
	Level ValidationLevel
}

// NewValidator creates a new validator with the specified level.
func NewValidator(level ValidationLevel) *Validator {
	return &Validator{Level: level}
}

// ValidateOutput validates that content is valid Starlark.
func (v *Validator) ValidateOutput(path string, content []byte) error {
	_, err := bzl.Parse(path, content)
	if err != nil {
		return &ValidationError{
			File:    path,
			Message: fmt.Sprintf("invalid Starlark syntax: %v", err),
		}
	}

	if v.Level >= ValidationSemantic {
		f, _ := bzl.Parse(path, content)
		return v.validateSemantics(path, f)
	}

	return nil
}

// ValidateTransformation validates before and after states.
// Ensures that a valid file remains valid after transformation.
func (v *Validator) ValidateTransformation(path string, before, after []byte) error {
	// Parse both
	_, errBefore := bzl.Parse(path, before)
	astAfter, errAfter := bzl.Parse(path, after)

	// If input was valid, output must be valid
	if errBefore == nil && errAfter != nil {
		return &ValidationError{
			File:    path,
			Message: fmt.Sprintf("transformation broke syntax: %v", errAfter),
		}
	}

	// Semantic validation
	if v.Level >= ValidationSemantic && astAfter != nil {
		return v.validateSemantics(path, astAfter)
	}

	return nil
}

// ValidateFile validates a File AST.
func (v *Validator) ValidateFile(f *File) error {
	// Re-parse the formatted output to ensure it's valid
	output := f.Format()
	_, err := bzl.Parse(f.Path(), output)
	if err != nil {
		return &ValidationError{
			File:    f.Path(),
			Message: fmt.Sprintf("formatted output is invalid: %v", err),
		}
	}

	if v.Level >= ValidationSemantic {
		return v.validateSemantics(f.Path(), f.File)
	}

	return nil
}

// validateSemantics performs semantic validation.
func (v *Validator) validateSemantics(path string, f *bzl.File) error {
	// Check load statements
	for _, stmt := range f.Stmt {
		if load, ok := stmt.(*bzl.LoadStmt); ok {
			if err := validateLoadModule(load.Module.Value); err != nil {
				start, _ := load.Span()
				return &ValidationError{
					File:    path,
					Line:    start.Line,
					Message: err.Error(),
				}
			}

			// Check for empty loads
			if len(load.From) == 0 {
				start, _ := load.Span()
				return &ValidationError{
					File:    path,
					Line:    start.Line,
					Message: "load statement has no symbols",
				}
			}

			// Check for duplicate symbols
			seen := make(map[string]bool)
			for _, to := range load.To {
				if seen[to.Name] {
					start, _ := load.Span()
					return &ValidationError{
						File:    path,
						Line:    start.Line,
						Message: fmt.Sprintf("duplicate symbol %q in load", to.Name),
					}
				}
				seen[to.Name] = true
			}
		}
	}

	return nil
}

// validateLoadModule checks that a load module path is valid.
func validateLoadModule(module string) error {
	if module == "" {
		return fmt.Errorf("empty load module path")
	}

	// Must have .bzl extension for .bzl files
	if !strings.HasSuffix(module, ".bzl") {
		return fmt.Errorf("load module %q should have .bzl extension", module)
	}

	// Check for valid prefixes
	if !strings.HasPrefix(module, "@") && !strings.HasPrefix(module, "//") && !strings.HasPrefix(module, ":") {
		return fmt.Errorf("load module %q should start with @, //, or :", module)
	}

	return nil
}

// ValidateAll validates multiple files and returns all errors.
func ValidateAll(files []*File, level ValidationLevel) *ValidationResult {
	v := NewValidator(level)
	result := &ValidationResult{Valid: true}

	for _, f := range files {
		if err := v.ValidateFile(f); err != nil {
			result.Valid = false
			if ve, ok := err.(*ValidationError); ok {
				result.Errors = append(result.Errors, ve)
			} else {
				result.Errors = append(result.Errors, &ValidationError{
					File:    f.Path(),
					Message: err.Error(),
				})
			}
		}
	}

	return result
}

// DryRunValidation performs validation without writing files.
// Returns nil if all transformations would produce valid output.
func DryRunValidation(changes []FileChange) error {
	v := NewValidator(ValidationSyntax)

	var errors []*ValidationError
	for _, change := range changes {
		if err := v.ValidateTransformation(change.Path, change.Before, change.After); err != nil {
			if ve, ok := err.(*ValidationError); ok {
				errors = append(errors, ve)
			} else {
				errors = append(errors, &ValidationError{
					File:    change.Path,
					Message: err.Error(),
				})
			}
		}
	}

	if len(errors) > 0 {
		var msg strings.Builder
		fmt.Fprintf(&msg, "dry-run validation found %d error(s):\n", len(errors))
		for _, e := range errors {
			msg.WriteString("  - ")
			msg.WriteString(e.Error())
			msg.WriteString("\n")
		}
		return fmt.Errorf("%s", msg.String())
	}

	return nil
}

// FileChange represents a pending file modification.
type FileChange struct {
	Path   string
	Before []byte
	After  []byte
}
