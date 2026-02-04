// Copyright 2025 bazelbump authors
// SPDX-License-Identifier: Apache-2.0

package ast

import (
	"testing"
)

func TestValidateOutput_ValidStarlark(t *testing.T) {
	tests := []struct {
		name    string
		content string
		wantErr bool
	}{
		{
			name:    "valid load",
			content: `load("@rules_cc//cc:defs.bzl", "cc_library")`,
			wantErr: false,
		},
		{
			name: "valid multi-line load",
			content: `load(
    "@rules_cc//cc:defs.bzl",
    "cc_library",
    "cc_binary",
)`,
			wantErr: false,
		},
		{
			name:    "valid rule",
			content: `cc_library(name = "test", srcs = ["test.cc"])`,
			wantErr: false,
		},
		{
			name:    "empty file",
			content: "",
			wantErr: false,
		},
	}

	v := NewValidator(ValidationSyntax)

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := v.ValidateOutput("BUILD", []byte(tt.content))
			if (err != nil) != tt.wantErr {
				t.Errorf("ValidateOutput() error = %v, wantErr %v", err, tt.wantErr)
			}
		})
	}
}

func TestValidateTransformation(t *testing.T) {
	v := NewValidator(ValidationSyntax)

	tests := []struct {
		name    string
		before  string
		after   string
		wantErr bool
	}{
		{
			name:    "valid to valid",
			before:  `load("@rules_cc//cc:defs.bzl", "cc_library")`,
			after:   `load("@rules_cc//cc:defs.bzl", "cc_binary", "cc_library")`,
			wantErr: false,
		},
		{
			name:    "empty to load",
			before:  "",
			after:   `load("@rules_cc//cc:defs.bzl", "cc_library")`,
			wantErr: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := v.ValidateTransformation("BUILD", []byte(tt.before), []byte(tt.after))
			if (err != nil) != tt.wantErr {
				t.Errorf("ValidateTransformation() error = %v, wantErr %v", err, tt.wantErr)
			}
		})
	}
}

func TestValidateFile(t *testing.T) {
	v := NewValidator(ValidationSyntax)

	f, err := ParseFileContent("BUILD", []byte(`load("@rules_cc//cc:defs.bzl", "cc_library")`))
	if err != nil {
		t.Fatalf("ParseFileContent() error = %v", err)
	}

	err = v.ValidateFile(f)
	if err != nil {
		t.Errorf("ValidateFile() error = %v", err)
	}
}

func TestValidateSemantic_LoadModule(t *testing.T) {
	v := NewValidator(ValidationSemantic)

	tests := []struct {
		name    string
		content string
		wantErr bool
	}{
		{
			name:    "valid external load",
			content: `load("@rules_cc//cc:defs.bzl", "cc_library")`,
			wantErr: false,
		},
		{
			name:    "valid workspace load",
			content: `load("//bazel:defs.bzl", "custom_rule")`,
			wantErr: false,
		},
		{
			name:    "valid relative load",
			content: `load(":defs.bzl", "local_rule")`,
			wantErr: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := v.ValidateOutput("BUILD", []byte(tt.content))
			if (err != nil) != tt.wantErr {
				t.Errorf("ValidateOutput() error = %v, wantErr %v", err, tt.wantErr)
			}
		})
	}
}

func TestValidationError(t *testing.T) {
	err := &ValidationError{
		File:    "BUILD",
		Line:    10,
		Column:  5,
		Message: "test error",
	}

	errStr := err.Error()
	if errStr != "BUILD:10:5: test error" {
		t.Errorf("ValidationError.Error() = %q, want BUILD:10:5: test error", errStr)
	}

	// Without line number
	err2 := &ValidationError{
		File:    "BUILD",
		Message: "test error",
	}
	if err2.Error() != "BUILD: test error" {
		t.Errorf("ValidationError.Error() without line = %q", err2.Error())
	}
}

func TestDryRunValidation(t *testing.T) {
	changes := []FileChange{
		{
			Path:   "BUILD",
			Before: []byte(`load("@rules_cc//cc:defs.bzl", "cc_library")`),
			After:  []byte(`load("@rules_cc//cc:defs.bzl", "cc_binary", "cc_library")`),
		},
	}

	err := DryRunValidation(changes)
	if err != nil {
		t.Errorf("DryRunValidation() error = %v", err)
	}
}

func TestValidateAll(t *testing.T) {
	f1, _ := ParseFileContent("BUILD1", []byte(`load("@rules_cc//cc:defs.bzl", "cc_library")`))
	f2, _ := ParseFileContent("BUILD2", []byte(`load("@rules_go//go:def.bzl", "go_library")`))

	result := ValidateAll([]*File{f1, f2}, ValidationSyntax)
	if !result.Valid {
		t.Errorf("ValidateAll() should be valid, got errors: %v", result.Errors)
	}
	if len(result.Errors) != 0 {
		t.Errorf("ValidateAll() should have no errors, got %d", len(result.Errors))
	}
}
