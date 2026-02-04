// Copyright 2025 bazelbump authors
// SPDX-License-Identifier: Apache-2.0

package ast

import (
	"testing"

	"github.com/albertocavalcante/starlark-syntax-go/bzl"
)

func TestParseFile(t *testing.T) {
	tests := []struct {
		name       string
		content    string
		wantLoads  int
		wantRules  int
		wantErr    bool
	}{
		{
			name:      "empty file",
			content:   "",
			wantLoads: 0,
			wantRules: 0,
		},
		{
			name:      "single load",
			content:   `load("@rules_cc//cc:defs.bzl", "cc_library")`,
			wantLoads: 1,
			wantRules: 0,
		},
		{
			name: "load and rule",
			content: `load("@rules_cc//cc:defs.bzl", "cc_library")

cc_library(
    name = "foo",
)`,
			wantLoads: 1,
			wantRules: 1,
		},
		{
			name: "multi-line load",
			content: `load(
    "//bazel:envoy_build_system.bzl",
    "envoy_cc_library",
    "envoy_package",
)`,
			wantLoads: 1,
			wantRules: 0,
		},
		{
			name: "multiple loads",
			content: `load("@rules_cc//cc:defs.bzl", "cc_library")
load("//bazel:defs.bzl", "custom_rule")`,
			wantLoads: 2,
			wantRules: 0,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			f, err := ParseFileContent("BUILD", []byte(tt.content))
			if (err != nil) != tt.wantErr {
				t.Fatalf("ParseFileContent() error = %v, wantErr %v", err, tt.wantErr)
			}
			if tt.wantErr {
				return
			}

			loads := countLoads(f)
			if loads != tt.wantLoads {
				t.Errorf("got %d loads, want %d", loads, tt.wantLoads)
			}

			rules := countRules(f)
			if rules != tt.wantRules {
				t.Errorf("got %d rules, want %d", rules, tt.wantRules)
			}
		})
	}
}

func TestFormat(t *testing.T) {
	tests := []struct {
		name    string
		content string
	}{
		{
			name:    "simple load",
			content: `load("@rules_cc//cc:defs.bzl", "cc_library")`,
		},
		{
			name: "load with multiple symbols",
			content: `load("@rules_cc//cc:defs.bzl", "cc_binary", "cc_library", "cc_test")`,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			f, err := ParseFileContent("BUILD", []byte(tt.content))
			if err != nil {
				t.Fatalf("ParseFileContent() error = %v", err)
			}

			output := f.Format()
			if len(output) == 0 {
				t.Error("Format() returned empty output")
			}

			// Verify output can be re-parsed
			_, err = ParseFileContent("BUILD", output)
			if err != nil {
				t.Errorf("Format() output is not valid: %v", err)
			}
		})
	}
}

func TestIsModified(t *testing.T) {
	f, _ := ParseFileContent("BUILD", []byte(`load("@rules_cc//cc:defs.bzl", "cc_library")`))

	if f.IsModified() {
		t.Error("new file should not be modified")
	}

	f.SetModified(true)
	if !f.IsModified() {
		t.Error("file should be modified after SetModified(true)")
	}
}

// countLoads counts LoadStmt in a file.
func countLoads(f *File) int {
	count := 0
	for _, stmt := range f.Stmt {
		if _, ok := stmt.(*bzl.LoadStmt); ok {
			count++
		}
	}
	return count
}

// countRules counts non-load statements (rules) in a file.
func countRules(f *File) int {
	count := 0
	for _, stmt := range f.Stmt {
		if _, ok := stmt.(*bzl.LoadStmt); !ok {
			count++
		}
	}
	return count
}
