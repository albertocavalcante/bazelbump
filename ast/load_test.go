// Copyright 2025 bazelbump authors
// SPDX-License-Identifier: Apache-2.0

package ast

import (
	"strings"
	"testing"
)

// TestEnsureLoad_MultiLineExistingLoad is the CRITICAL test case.
// This is the bug that broke 96% of Envoy BUILD files.
// The text-based implementation inserted new loads INSIDE multi-line load statements.
func TestEnsureLoad_MultiLineExistingLoad(t *testing.T) {
	input := `load(
    "//bazel:envoy_build_system.bzl",
    "envoy_cc_library",
    "envoy_package",
)

envoy_cc_library(name = "test")
`

	f, err := ParseFileContent("BUILD", []byte(input))
	if err != nil {
		t.Fatalf("ParseFileContent() error = %v", err)
	}

	modified, err := EnsureLoad(f, EnsureLoadOptions{
		Module:   "@rules_cc//cc:defs.bzl",
		Symbols:  []string{"cc_library"},
		Position: LoadPositionTop,
	})
	if err != nil {
		t.Fatalf("EnsureLoad() error = %v", err)
	}
	if !modified {
		t.Error("EnsureLoad() should have modified the file")
	}

	output := string(f.Format())

	// The new load MUST appear BEFORE the multi-line load, not inside it
	ccLoadIdx := strings.Index(output, `load("@rules_cc`)
	envoyLoadIdx := strings.Index(output, `"//bazel:envoy_build_system.bzl"`)

	if ccLoadIdx < 0 {
		t.Errorf("output missing @rules_cc load:\n%s", output)
	}
	if envoyLoadIdx < 0 {
		t.Errorf("output missing envoy load:\n%s", output)
	}
	if ccLoadIdx >= envoyLoadIdx {
		t.Errorf("new load should appear BEFORE existing multi-line load, got:\n%s", output)
	}

	// Verify output is valid Starlark (can be re-parsed)
	_, err = ParseFileContent("BUILD", []byte(output))
	if err != nil {
		t.Errorf("output is not valid Starlark: %v\nOutput:\n%s", err, output)
	}
}

func TestEnsureLoad_AddToExisting(t *testing.T) {
	input := `load("@rules_cc//cc:defs.bzl", "cc_library")

cc_library(name = "test")
`

	f, err := ParseFileContent("BUILD", []byte(input))
	if err != nil {
		t.Fatalf("ParseFileContent() error = %v", err)
	}

	modified, err := EnsureLoad(f, EnsureLoadOptions{
		Module:  "@rules_cc//cc:defs.bzl",
		Symbols: []string{"cc_binary"},
	})
	if err != nil {
		t.Fatalf("EnsureLoad() error = %v", err)
	}
	if !modified {
		t.Error("EnsureLoad() should have modified the file")
	}

	output := string(f.Format())

	// Should have both cc_library and cc_binary
	if !strings.Contains(output, "cc_binary") {
		t.Errorf("output should contain cc_binary:\n%s", output)
	}
	if !strings.Contains(output, "cc_library") {
		t.Errorf("output should contain cc_library:\n%s", output)
	}

	// Should be a single load statement
	loadCount := strings.Count(output, "load(")
	if loadCount != 1 {
		t.Errorf("expected 1 load statement, got %d:\n%s", loadCount, output)
	}
}

func TestEnsureLoad_AlreadyLoaded(t *testing.T) {
	input := `load("@rules_cc//cc:defs.bzl", "cc_library")

cc_library(name = "test")
`

	f, err := ParseFileContent("BUILD", []byte(input))
	if err != nil {
		t.Fatalf("ParseFileContent() error = %v", err)
	}

	modified, err := EnsureLoad(f, EnsureLoadOptions{
		Module:  "@rules_cc//cc:defs.bzl",
		Symbols: []string{"cc_library"},
	})
	if err != nil {
		t.Fatalf("EnsureLoad() error = %v", err)
	}
	if modified {
		t.Error("EnsureLoad() should NOT have modified - symbol already loaded")
	}
}

func TestEnsureLoad_EmptyFile(t *testing.T) {
	f, err := ParseFileContent("BUILD", []byte(""))
	if err != nil {
		t.Fatalf("ParseFileContent() error = %v", err)
	}

	modified, err := EnsureLoad(f, EnsureLoadOptions{
		Module:  "@rules_cc//cc:defs.bzl",
		Symbols: []string{"cc_library"},
	})
	if err != nil {
		t.Fatalf("EnsureLoad() error = %v", err)
	}
	if !modified {
		t.Error("EnsureLoad() should have modified empty file")
	}

	output := string(f.Format())
	if !strings.Contains(output, `load("@rules_cc//cc:defs.bzl"`) {
		t.Errorf("output should contain load statement:\n%s", output)
	}
}

func TestEnsureLoad_Position(t *testing.T) {
	tests := []struct {
		name     string
		input    string
		position LoadPosition
		module   string
		wantFirst bool // Should new load be first?
	}{
		{
			name: "top position",
			input: `load("//bazel:defs.bzl", "custom_rule")

custom_rule(name = "test")
`,
			position:  LoadPositionTop,
			module:    "@rules_cc//cc:defs.bzl",
			wantFirst: true,
		},
		{
			name: "with existing position",
			input: `load("@rules_go//go:def.bzl", "go_library")

go_library(name = "test")
`,
			position:  LoadPositionWithExisting,
			module:    "@rules_cc//cc:defs.bzl",
			wantFirst: false, // Goes after existing load
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			f, err := ParseFileContent("BUILD", []byte(tt.input))
			if err != nil {
				t.Fatalf("ParseFileContent() error = %v", err)
			}

			_, err = EnsureLoad(f, EnsureLoadOptions{
				Module:   tt.module,
				Symbols:  []string{"cc_library"},
				Position: tt.position,
			})
			if err != nil {
				t.Fatalf("EnsureLoad() error = %v", err)
			}

			output := string(f.Format())
			newLoadIdx := strings.Index(output, tt.module)

			// Find any other load
			otherLoadIdx := -1
			for _, load := range []string{"@rules_go", "//bazel:defs.bzl"} {
				idx := strings.Index(output, load)
				if idx >= 0 {
					otherLoadIdx = idx
					break
				}
			}

			if tt.wantFirst && otherLoadIdx >= 0 && newLoadIdx > otherLoadIdx {
				t.Errorf("new load should be first, got:\n%s", output)
			}
			if !tt.wantFirst && otherLoadIdx >= 0 && newLoadIdx < otherLoadIdx {
				t.Errorf("new load should be after existing, got:\n%s", output)
			}
		})
	}
}

func TestRemoveLoad(t *testing.T) {
	input := `load("@rules_cc//cc:defs.bzl", "cc_binary", "cc_library", "cc_test")

cc_library(name = "test")
`

	f, err := ParseFileContent("BUILD", []byte(input))
	if err != nil {
		t.Fatalf("ParseFileContent() error = %v", err)
	}

	modified, err := RemoveLoad(f, RemoveLoadOptions{
		Module:  "@rules_cc//cc:defs.bzl",
		Symbols: []string{"cc_test"},
	})
	if err != nil {
		t.Fatalf("RemoveLoad() error = %v", err)
	}
	if !modified {
		t.Error("RemoveLoad() should have modified the file")
	}

	output := string(f.Format())

	if strings.Contains(output, "cc_test") {
		t.Errorf("output should not contain cc_test:\n%s", output)
	}
	if !strings.Contains(output, "cc_library") {
		t.Errorf("output should still contain cc_library:\n%s", output)
	}
	if !strings.Contains(output, "cc_binary") {
		t.Errorf("output should still contain cc_binary:\n%s", output)
	}
}

func TestRemoveLoad_EntireStatement(t *testing.T) {
	input := `load("@rules_cc//cc:defs.bzl", "cc_library")

cc_library(name = "test")
`

	f, err := ParseFileContent("BUILD", []byte(input))
	if err != nil {
		t.Fatalf("ParseFileContent() error = %v", err)
	}

	_, err = RemoveLoad(f, RemoveLoadOptions{
		Module:  "@rules_cc//cc:defs.bzl",
		Symbols: []string{"cc_library"},
	})
	if err != nil {
		t.Fatalf("RemoveLoad() error = %v", err)
	}

	output := string(f.Format())

	// The entire load statement should be removed
	if strings.Contains(output, "load(") {
		t.Errorf("load statement should be removed entirely:\n%s", output)
	}
}

func TestMergeLoads(t *testing.T) {
	input := `load("@rules_cc//cc:defs.bzl", "cc_library")
load("@rules_cc//cc:defs.bzl", "cc_binary")
load("@rules_go//go:def.bzl", "go_library")

cc_library(name = "test")
`

	f, err := ParseFileContent("BUILD", []byte(input))
	if err != nil {
		t.Fatalf("ParseFileContent() error = %v", err)
	}

	modified, err := MergeLoads(f)
	if err != nil {
		t.Fatalf("MergeLoads() error = %v", err)
	}
	if !modified {
		t.Error("MergeLoads() should have modified the file")
	}

	output := string(f.Format())

	// Should have merged the two @rules_cc loads
	ccLoadCount := strings.Count(output, "@rules_cc//cc:defs.bzl")
	if ccLoadCount != 1 {
		t.Errorf("expected 1 @rules_cc load after merge, got %d:\n%s", ccLoadCount, output)
	}

	// Should have both symbols
	if !strings.Contains(output, "cc_library") {
		t.Errorf("output should contain cc_library:\n%s", output)
	}
	if !strings.Contains(output, "cc_binary") {
		t.Errorf("output should contain cc_binary:\n%s", output)
	}

	// @rules_go load should be unchanged
	if !strings.Contains(output, "@rules_go//go:def.bzl") {
		t.Errorf("output should contain @rules_go load:\n%s", output)
	}
}

func TestSortLoads(t *testing.T) {
	input := `load("//local:defs.bzl", "local_rule")
load("@rules_cc//cc:defs.bzl", "cc_library")
load(":relative.bzl", "relative_rule")
load("@rules_go//go:def.bzl", "go_library")

cc_library(name = "test")
`

	f, err := ParseFileContent("BUILD", []byte(input))
	if err != nil {
		t.Fatalf("ParseFileContent() error = %v", err)
	}

	modified, err := SortLoads(f)
	if err != nil {
		t.Fatalf("SortLoads() error = %v", err)
	}
	if !modified {
		t.Error("SortLoads() should have modified the file")
	}

	output := string(f.Format())

	// External loads (@) should come first
	ccIdx := strings.Index(output, "@rules_cc")
	goIdx := strings.Index(output, "@rules_go")
	localIdx := strings.Index(output, "//local")

	if ccIdx < 0 || goIdx < 0 || localIdx < 0 {
		t.Fatalf("missing expected loads in output:\n%s", output)
	}

	// Both external loads should come before workspace load
	if ccIdx > localIdx || goIdx > localIdx {
		t.Errorf("external loads should come before workspace loads:\n%s", output)
	}
}

func TestEnsureLoad_CommentPreservation(t *testing.T) {
	// NOTE: Comment preservation depends on starlark-syntax-go including
	// the assignComments() logic from buildtools. Currently, leading file
	// comments may not be attached to statements.
	// This test verifies inline suffix comments are preserved.
	tests := []struct {
		name          string
		input         string
		wantComments  []string // Comments that should be in output
	}{
		{
			name: "suffix comment on load",
			input: `load("//bazel:defs.bzl", "custom_rule")  # important

custom_rule(name = "test")
`,
			wantComments: []string{"# important"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			f, err := ParseFileContent("BUILD", []byte(tt.input))
			if err != nil {
				t.Fatalf("ParseFileContent() error = %v", err)
			}

			_, err = EnsureLoad(f, EnsureLoadOptions{
				Module:   "@rules_cc//cc:defs.bzl",
				Symbols:  []string{"cc_library"},
				Position: LoadPositionTop,
			})
			if err != nil {
				t.Fatalf("EnsureLoad() error = %v", err)
			}

			output := string(f.Format())

			for _, comment := range tt.wantComments {
				if !strings.Contains(output, comment) {
					t.Errorf("output should preserve comment %q:\n%s", comment, output)
				}
			}
		})
	}
}

// TestEnvoyBugReproduction reproduces the exact bug found in Envoy migration.
func TestEnvoyBugReproduction(t *testing.T) {
	// This is the exact pattern from Envoy BUILD files that broke
	input := `load(
    "//bazel:envoy_build_system.bzl",
    "envoy_cc_library",
    "envoy_package",
)

licenses(["notice"])  # Apache 2

envoy_package()

envoy_cc_library(
    name = "filter_test",
    srcs = ["filter_test.cc"],
)
`

	f, err := ParseFileContent("BUILD", []byte(input))
	if err != nil {
		t.Fatalf("ParseFileContent() error = %v", err)
	}

	_, err = EnsureLoad(f, EnsureLoadOptions{
		Module:   "@rules_cc//cc:defs.bzl",
		Symbols:  []string{"cc_test"},
		Position: LoadPositionTop,
	})
	if err != nil {
		t.Fatalf("EnsureLoad() error = %v", err)
	}

	output := string(f.Format())

	// THE CRITICAL CHECK: The output must be valid Starlark
	_, err = ParseFileContent("BUILD", []byte(output))
	if err != nil {
		t.Fatalf("OUTPUT IS INVALID STARLARK - BUG NOT FIXED!\nError: %v\nOutput:\n%s", err, output)
	}

	// The new load should NOT appear inside the multi-line load
	// Check that we don't have "load(\nload(" which indicates insertion inside
	if strings.Contains(output, "load(\nload(") {
		t.Errorf("BUG: new load inserted INSIDE multi-line load:\n%s", output)
	}

	// The new @rules_cc load should appear as its own statement
	if !strings.Contains(output, `load("@rules_cc//cc:defs.bzl"`) {
		t.Errorf("missing new load statement:\n%s", output)
	}

	t.Logf("Output (valid):\n%s", output)
}
