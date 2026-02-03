// Copyright 2025 bazelbump authors
// SPDX-License-Identifier: Apache-2.0

// Package recipe provides a framework for multi-step, multi-file migrations.
//
// A Recipe is a coordinated set of transformations that migrate a codebase
// from one state to another. Unlike simple transforms (single-file, single-operation),
// recipes can span multiple files with proper ordering, validation, and rollback.
//
// Example:
//
//	engine := recipe.NewEngine()
//	result, err := engine.Run(ctx, "autoload-migration", recipe.Options{
//	    ProjectPath: ".",
//	    DryRun:      true,
//	})
package recipe

import (
	"fmt"
)

// Recipe represents a multi-step migration.
type Recipe struct {
	// Metadata
	Name        string `json:"name"`
	Version     string `json:"version"`
	Description string `json:"description"`

	// Applicable Bazel versions
	MinBazelVersion string `json:"min_bazel_version,omitempty"`
	MaxBazelVersion string `json:"max_bazel_version,omitempty"`

	// Steps to execute in order
	Steps []Step `json:"steps"`
}

// Step represents a single operation in a recipe.
type Step struct {
	ID          string   `json:"id"`
	Name        string   `json:"name"`
	Description string   `json:"description"`
	DependsOn   []string `json:"depends_on,omitempty"`

	// Exactly one of these should be set
	Scan   *ScanStep   `json:"scan,omitempty"`
	Edit   *EditStep   `json:"edit,omitempty"`
	Verify *VerifyStep `json:"verify,omitempty"`
}

// ScanStep detects patterns in the codebase.
type ScanStep struct {
	// File patterns to scan (glob)
	FilePatterns []string `json:"file_patterns"`

	// Patterns to detect (regex)
	Patterns []string `json:"patterns"`

	// Output key for detected items
	OutputKey string `json:"output_key"`
}

// EditStep modifies files.
type EditStep struct {
	// Operations to perform
	Operations []Operation `json:"operations"`
}

// Operation is a single edit operation.
type Operation struct {
	Type string `json:"type"` // ensure_load, ensure_bazel_dep, remove_flag, etc.

	// For ensure_load
	Rule     string `json:"rule,omitempty"`
	LoadFrom string `json:"load_from,omitempty"`

	// For ensure_bazel_dep
	DepName    string `json:"dep_name,omitempty"`
	DepVersion string `json:"dep_version,omitempty"`

	// For file edits
	File       string `json:"file,omitempty"`
	OldContent string `json:"old_content,omitempty"`
	NewContent string `json:"new_content,omitempty"`
}

// VerifyStep runs a command to verify correctness.
type VerifyStep struct {
	Command      string `json:"command"`
	AllowFailure bool   `json:"allow_failure"`
}

// Context holds state during recipe execution.
type Context struct {
	ProjectPath string
	DryRun      bool

	// Outputs from previous steps, keyed by step ID
	Outputs map[string]any

	// Changes made (for rollback)
	Changes []*FileChange
}

// FileChange records a change made to a file.
type FileChange struct {
	Path        string
	OldContent  []byte
	NewContent  []byte
	Description string
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
	Description string
	Changes     []*FileChange
	Output      any
	Error       error
}

// DetectedRule represents a native rule usage found during scanning.
type DetectedRule struct {
	File   string `json:"file"`
	Line   int    `json:"line"`
	Rule   string `json:"rule"`   // e.g., "sh_binary", "cc_library"
	Family string `json:"family"` // e.g., "shell", "cc", "java", "python"
}

// NativeRuleMapping maps native rules to their load statements and dependencies.
type NativeRuleMapping struct {
	Rule       string // e.g., "sh_binary"
	Family     string // e.g., "shell"
	LoadPath   string // e.g., "@rules_shell//shell:sh_binary.bzl"
	DepName    string // e.g., "rules_shell"
	DepVersion string // e.g., "0.4.1"
}

// NativeRuleMappings contains all known native rule migrations.
var NativeRuleMappings = []NativeRuleMapping{
	// Shell rules
	{Rule: "sh_binary", Family: "shell", LoadPath: "@rules_shell//shell:sh_binary.bzl", DepName: "rules_shell", DepVersion: "0.4.1"},
	{Rule: "sh_test", Family: "shell", LoadPath: "@rules_shell//shell:sh_test.bzl", DepName: "rules_shell", DepVersion: "0.4.1"},
	{Rule: "sh_library", Family: "shell", LoadPath: "@rules_shell//shell:sh_library.bzl", DepName: "rules_shell", DepVersion: "0.4.1"},

	// C++ rules
	{Rule: "cc_binary", Family: "cc", LoadPath: "@rules_cc//cc:defs.bzl", DepName: "rules_cc", DepVersion: "0.1.1"},
	{Rule: "cc_library", Family: "cc", LoadPath: "@rules_cc//cc:defs.bzl", DepName: "rules_cc", DepVersion: "0.1.1"},
	{Rule: "cc_test", Family: "cc", LoadPath: "@rules_cc//cc:defs.bzl", DepName: "rules_cc", DepVersion: "0.1.1"},
	{Rule: "cc_import", Family: "cc", LoadPath: "@rules_cc//cc:defs.bzl", DepName: "rules_cc", DepVersion: "0.1.1"},
	{Rule: "cc_shared_library", Family: "cc", LoadPath: "@rules_cc//cc:defs.bzl", DepName: "rules_cc", DepVersion: "0.1.1"},

	// Java rules
	{Rule: "java_binary", Family: "java", LoadPath: "@rules_java//java:defs.bzl", DepName: "rules_java", DepVersion: "8.6.3"},
	{Rule: "java_library", Family: "java", LoadPath: "@rules_java//java:defs.bzl", DepName: "rules_java", DepVersion: "8.6.3"},
	{Rule: "java_test", Family: "java", LoadPath: "@rules_java//java:defs.bzl", DepName: "rules_java", DepVersion: "8.6.3"},
	{Rule: "java_import", Family: "java", LoadPath: "@rules_java//java:defs.bzl", DepName: "rules_java", DepVersion: "8.6.3"},
	{Rule: "java_plugin", Family: "java", LoadPath: "@rules_java//java:defs.bzl", DepName: "rules_java", DepVersion: "8.6.3"},

	// Python rules
	{Rule: "py_binary", Family: "python", LoadPath: "@rules_python//python:defs.bzl", DepName: "rules_python", DepVersion: "1.4.1"},
	{Rule: "py_library", Family: "python", LoadPath: "@rules_python//python:defs.bzl", DepName: "rules_python", DepVersion: "1.4.1"},
	{Rule: "py_test", Family: "python", LoadPath: "@rules_python//python:defs.bzl", DepName: "rules_python", DepVersion: "1.4.1"},

	// Proto rules
	{Rule: "proto_library", Family: "proto", LoadPath: "@protobuf//bazel:proto_library.bzl", DepName: "protobuf", DepVersion: "29.3"},
	{Rule: "cc_proto_library", Family: "proto", LoadPath: "@protobuf//bazel:cc_proto_library.bzl", DepName: "protobuf", DepVersion: "29.3"},
	{Rule: "java_proto_library", Family: "proto", LoadPath: "@protobuf//bazel:java_proto_library.bzl", DepName: "protobuf", DepVersion: "29.3"},
	{Rule: "py_proto_library", Family: "proto", LoadPath: "@protobuf//bazel:py_proto_library.bzl", DepName: "protobuf", DepVersion: "29.3"},
}

// GetMapping returns the mapping for a native rule.
func GetMapping(rule string) (*NativeRuleMapping, bool) {
	for i := range NativeRuleMappings {
		if NativeRuleMappings[i].Rule == rule {
			return &NativeRuleMappings[i], true
		}
	}
	return nil, false
}

// GetMappingsByFamily returns all mappings for a rule family.
func GetMappingsByFamily(family string) []NativeRuleMapping {
	var result []NativeRuleMapping
	for _, m := range NativeRuleMappings {
		if m.Family == family {
			result = append(result, m)
		}
	}
	return result
}

// Validate checks if a recipe is well-formed.
func (r *Recipe) Validate() error {
	if r.Name == "" {
		return fmt.Errorf("recipe name is required")
	}
	if len(r.Steps) == 0 {
		return fmt.Errorf("recipe must have at least one step")
	}

	// Check for duplicate step IDs
	seen := make(map[string]bool)
	for _, step := range r.Steps {
		if step.ID == "" {
			return fmt.Errorf("step ID is required")
		}
		if seen[step.ID] {
			return fmt.Errorf("duplicate step ID: %s", step.ID)
		}
		seen[step.ID] = true
	}

	// Check that depends_on references exist
	for _, step := range r.Steps {
		for _, dep := range step.DependsOn {
			if !seen[dep] {
				return fmt.Errorf("step %q depends on unknown step %q", step.ID, dep)
			}
		}
	}

	return nil
}
