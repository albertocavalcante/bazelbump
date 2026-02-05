// Copyright 2025 bazelbump authors
// SPDX-License-Identifier: Apache-2.0

package recipe

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestEngine_List(t *testing.T) {
	engine := NewEngine()
	recipes := engine.List()

	if len(recipes) == 0 {
		t.Error("expected at least one built-in recipe")
	}

	// Check for autoload-migration
	found := false
	for _, r := range recipes {
		if r.Name == "autoload-migration" {
			found = true
			break
		}
	}
	if !found {
		t.Error("expected autoload-migration recipe")
	}
}

func TestEngine_Get(t *testing.T) {
	engine := NewEngine()

	recipe, ok := engine.Get("autoload-migration")
	if !ok {
		t.Fatal("expected to find autoload-migration recipe")
	}

	if recipe.Name != "autoload-migration" {
		t.Errorf("expected name autoload-migration, got %s", recipe.Name)
	}

	if len(recipe.Steps) != 3 {
		t.Errorf("expected 3 steps, got %d", len(recipe.Steps))
	}
}

func TestAutoloadMigration_DryRun(t *testing.T) {
	// Create temp directory with test files
	tmpDir := t.TempDir()

	// Create MODULE.bazel
	moduleContent := `module(name = "test_project")
bazel_dep(name = "rules_go", version = "0.50.0")
`
	if err := os.WriteFile(filepath.Join(tmpDir, "MODULE.bazel"), []byte(moduleContent), 0644); err != nil {
		t.Fatal(err)
	}

	// Create BUILD file with native rule
	buildContent := `# Test BUILD file
package(default_visibility = ["//visibility:public"])

sh_binary(
    name = "hello",
    srcs = ["hello.sh"],
)

cc_library(
    name = "mylib",
    srcs = ["lib.cc"],
)
`
	if err := os.WriteFile(filepath.Join(tmpDir, "BUILD.bazel"), []byte(buildContent), 0644); err != nil {
		t.Fatal(err)
	}

	// Run recipe in dry-run mode
	engine := NewEngine()
	result, err := engine.Run("autoload-migration", Options{
		ProjectPath: tmpDir,
		DryRun:      true,
	})

	if err != nil {
		t.Fatalf("recipe failed: %v", err)
	}

	if !result.Success {
		t.Errorf("expected success, got failure: %v", result.Error)
	}

	if result.StepsRun != 3 {
		t.Errorf("expected 3 steps run, got %d", result.StepsRun)
	}

	// Check that changes were detected
	if len(result.Changes) == 0 {
		t.Log("No changes detected - this may be expected if files already have loads")
	}

	// In dry-run, files should NOT be modified
	content, _ := os.ReadFile(filepath.Join(tmpDir, "BUILD.bazel"))
	if strings.Contains(string(content), "load(") {
		t.Error("dry-run modified the file")
	}
}

func TestAutoloadMigration_Apply(t *testing.T) {
	// Create temp directory with test files
	tmpDir := t.TempDir()

	// Create MODULE.bazel
	moduleContent := `module(name = "test_project")
`
	if err := os.WriteFile(filepath.Join(tmpDir, "MODULE.bazel"), []byte(moduleContent), 0644); err != nil {
		t.Fatal(err)
	}

	// Create BUILD file with native rule (no existing loads)
	buildContent := `package(default_visibility = ["//visibility:public"])

sh_binary(
    name = "hello",
    srcs = ["hello.sh"],
)
`
	if err := os.WriteFile(filepath.Join(tmpDir, "BUILD.bazel"), []byte(buildContent), 0644); err != nil {
		t.Fatal(err)
	}

	// Run recipe (not dry-run)
	engine := NewEngine()
	result, err := engine.Run("autoload-migration", Options{
		ProjectPath: tmpDir,
		DryRun:      false,
	})

	if err != nil {
		t.Fatalf("recipe failed: %v", err)
	}

	if !result.Success {
		t.Errorf("expected success, got failure: %v", result.Error)
	}

	// Check that BUILD file was modified
	content, _ := os.ReadFile(filepath.Join(tmpDir, "BUILD.bazel"))
	if !strings.Contains(string(content), `load("@rules_shell//shell:sh_binary.bzl"`) {
		t.Error("expected load statement for sh_binary")
		t.Logf("Content:\n%s", content)
	}

	// Check that MODULE.bazel was modified
	moduleContent2, _ := os.ReadFile(filepath.Join(tmpDir, "MODULE.bazel"))
	if !strings.Contains(string(moduleContent2), `bazel_dep(name = "rules_shell"`) {
		t.Error("expected bazel_dep for rules_shell")
		t.Logf("MODULE.bazel:\n%s", moduleContent2)
	}
}

func TestGetMapping(t *testing.T) {
	tests := []struct {
		rule   string
		expect string
	}{
		{"sh_binary", "rules_shell"},
		{"cc_library", "rules_cc"},
		{"java_test", "rules_java"},
		{"py_binary", "rules_python"},
		{"proto_library", "protobuf"},
		{"unknown_rule", ""},
	}

	for _, tt := range tests {
		t.Run(tt.rule, func(t *testing.T) {
			m, ok := GetMapping(tt.rule)
			if tt.expect == "" {
				if ok {
					t.Errorf("expected no mapping for %s", tt.rule)
				}
			} else {
				if !ok {
					t.Errorf("expected mapping for %s", tt.rule)
				} else if m.DepName != tt.expect {
					t.Errorf("expected dep %s, got %s", tt.expect, m.DepName)
				}
			}
		})
	}
}

func TestRecipe_Validate(t *testing.T) {
	tests := []struct {
		name    string
		recipe  Recipe
		wantErr bool
	}{
		{
			name:    "empty name",
			recipe:  Recipe{},
			wantErr: true,
		},
		{
			name:    "no steps",
			recipe:  Recipe{Name: "test"},
			wantErr: true,
		},
		{
			name: "valid",
			recipe: Recipe{
				Name:  "test",
				Steps: []Step{{ID: "step1", Name: "Step 1"}},
			},
			wantErr: false,
		},
		{
			name: "duplicate step ID",
			recipe: Recipe{
				Name: "test",
				Steps: []Step{
					{ID: "step1", Name: "Step 1"},
					{ID: "step1", Name: "Step 1 again"},
				},
			},
			wantErr: true,
		},
		{
			name: "unknown dependency",
			recipe: Recipe{
				Name: "test",
				Steps: []Step{
					{ID: "step1", Name: "Step 1", DependsOn: []string{"nonexistent"}},
				},
			},
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := tt.recipe.Validate()
			if (err != nil) != tt.wantErr {
				t.Errorf("Validate() error = %v, wantErr %v", err, tt.wantErr)
			}
		})
	}
}

// TestAutoloadMigration_MacrosNotDetected verifies that macro wrappers like envoy_cc_library
// are NOT detected as native rules requiring explicit loads.
// Reference: Envoy uses macros defined in //bazel:envoy_build_system.bzl
// See: https://github.com/envoyproxy/envoy/blob/main/source/common/buffer/BUILD
func TestAutoloadMigration_MacrosNotDetected(t *testing.T) {
	tmpDir := t.TempDir()

	// Create MODULE.bazel
	moduleContent := `module(name = "test_project")
`
	if err := os.WriteFile(filepath.Join(tmpDir, "MODULE.bazel"), []byte(moduleContent), 0644); err != nil {
		t.Fatal(err)
	}

	// Create BUILD file using MACROS (like Envoy does), not native rules
	// The pattern \bcc_library\s*\( should NOT match envoy_cc_library(
	buildContent := `load(
    "//bazel:envoy_build_system.bzl",
    "envoy_cc_library",
    "envoy_cc_test",
    "envoy_package",
)

licenses(["notice"])  # Apache 2

envoy_package()

# These are MACROS, not native rules - should NOT trigger load additions
envoy_cc_library(
    name = "buffer_lib",
    srcs = ["buffer.cc"],
    hdrs = ["buffer.h"],
)

envoy_cc_test(
    name = "buffer_test",
    srcs = ["buffer_test.cc"],
)
`
	buildPath := filepath.Join(tmpDir, "BUILD")
	if err := os.WriteFile(buildPath, []byte(buildContent), 0644); err != nil {
		t.Fatal(err)
	}

	// Run recipe
	engine := NewEngine()
	result, err := engine.Run("autoload-migration", Options{
		ProjectPath: tmpDir,
		DryRun:      false,
	})

	if err != nil {
		t.Fatalf("recipe failed: %v", err)
	}

	if !result.Success {
		t.Errorf("expected success, got failure: %v", result.Error)
	}

	// CRITICAL: No changes should be made - macros don't need load statements
	if len(result.Changes) > 0 {
		t.Errorf("expected no changes for macro-only BUILD file, got %d changes", len(result.Changes))
		for _, c := range result.Changes {
			t.Logf("  Change: %s - %s", c.Path, c.Description)
		}
	}

	// Verify BUILD file was NOT modified
	content, _ := os.ReadFile(buildPath)
	if strings.Contains(string(content), `@rules_cc`) {
		t.Error("BUILD file was incorrectly modified - added rules_cc load for macro")
		t.Logf("Content:\n%s", content)
	}
}

// TestAutoloadMigration_AlreadyLoaded verifies that rules with existing load statements
// are not detected again.
// Reference: Envoy's test/fuzz/BUILD already has explicit loads
// See: https://github.com/envoyproxy/envoy/blob/c7311de3d4/test/fuzz/BUILD
func TestAutoloadMigration_AlreadyLoaded(t *testing.T) {
	tmpDir := t.TempDir()

	// Create MODULE.bazel with existing dep
	moduleContent := `module(name = "test_project")
bazel_dep(name = "rules_cc", version = "0.1.0")
`
	if err := os.WriteFile(filepath.Join(tmpDir, "MODULE.bazel"), []byte(moduleContent), 0644); err != nil {
		t.Fatal(err)
	}

	// Create BUILD file with EXISTING load statement for cc_library
	buildContent := `load("@rules_cc//cc:defs.bzl", "cc_library")

package(default_visibility = ["//visibility:public"])

cc_library(
    name = "mylib",
    srcs = ["lib.cc"],
)
`
	buildPath := filepath.Join(tmpDir, "BUILD")
	if err := os.WriteFile(buildPath, []byte(buildContent), 0644); err != nil {
		t.Fatal(err)
	}

	// Run recipe
	engine := NewEngine()
	result, err := engine.Run("autoload-migration", Options{
		ProjectPath: tmpDir,
		DryRun:      false,
	})

	if err != nil {
		t.Fatalf("recipe failed: %v", err)
	}

	if !result.Success {
		t.Errorf("expected success, got failure: %v", result.Error)
	}

	// No changes should be needed - rule is already loaded
	buildChanges := 0
	for _, c := range result.Changes {
		if strings.Contains(c.Path, "BUILD") {
			buildChanges++
		}
	}
	if buildChanges > 0 {
		t.Errorf("expected no BUILD changes for already-loaded rule, got %d", buildChanges)
	}
}

// TestAutoloadMigration_MixedMacrosAndNative verifies correct behavior when a file
// has both macros (no action needed) and native rules (need loads).
func TestAutoloadMigration_MixedMacrosAndNative(t *testing.T) {
	tmpDir := t.TempDir()

	// Create MODULE.bazel
	moduleContent := `module(name = "test_project")
`
	if err := os.WriteFile(filepath.Join(tmpDir, "MODULE.bazel"), []byte(moduleContent), 0644); err != nil {
		t.Fatal(err)
	}

	// Create BUILD file with BOTH macros AND native rules
	buildContent := `load(
    "//bazel:custom.bzl",
    "custom_cc_library",
)

package(default_visibility = ["//visibility:public"])

# This is a MACRO - should NOT trigger load
custom_cc_library(
    name = "custom_lib",
    srcs = ["custom.cc"],
)

# This is a NATIVE rule - SHOULD trigger load
cc_library(
    name = "native_lib",
    srcs = ["native.cc"],
)
`
	buildPath := filepath.Join(tmpDir, "BUILD")
	if err := os.WriteFile(buildPath, []byte(buildContent), 0644); err != nil {
		t.Fatal(err)
	}

	// Run recipe
	engine := NewEngine()
	result, err := engine.Run("autoload-migration", Options{
		ProjectPath: tmpDir,
		DryRun:      false,
	})

	if err != nil {
		t.Fatalf("recipe failed: %v", err)
	}

	if !result.Success {
		t.Errorf("expected success, got failure: %v", result.Error)
	}

	// Check that BUILD file was modified for the native rule only
	content, _ := os.ReadFile(buildPath)
	if !strings.Contains(string(content), `load("@rules_cc//cc:defs.bzl"`) {
		t.Error("expected load statement for native cc_library")
		t.Logf("Content:\n%s", content)
	}

	// Verify the custom macro load is still there
	if !strings.Contains(string(content), `"//bazel:custom.bzl"`) {
		t.Error("original custom load was removed")
		t.Logf("Content:\n%s", content)
	}
}

// TestAutoloadMigration_WordBoundary verifies that word boundary matching works correctly.
// The pattern \bcc_library\s*\( should match "cc_library(" but NOT "my_cc_library(" or "cc_library_wrapper(".
func TestAutoloadMigration_WordBoundary(t *testing.T) {
	tmpDir := t.TempDir()

	// Create MODULE.bazel
	moduleContent := `module(name = "test_project")
`
	if err := os.WriteFile(filepath.Join(tmpDir, "MODULE.bazel"), []byte(moduleContent), 0644); err != nil {
		t.Fatal(err)
	}

	// Test various patterns that should NOT match
	buildContent := `load("//bazel:macros.bzl", "my_cc_library", "cc_library_wrapper", "super_cc_library_extra")

package(default_visibility = ["//visibility:public"])

# Prefix: should NOT match \bcc_library
my_cc_library(
    name = "prefixed",
    srcs = ["a.cc"],
)

# Suffix: should NOT match \bcc_library
cc_library_wrapper(
    name = "suffixed",
    srcs = ["b.cc"],
)

# Both: should NOT match
super_cc_library_extra(
    name = "both",
    srcs = ["c.cc"],
)
`
	buildPath := filepath.Join(tmpDir, "BUILD")
	if err := os.WriteFile(buildPath, []byte(buildContent), 0644); err != nil {
		t.Fatal(err)
	}

	// Run recipe
	engine := NewEngine()
	result, err := engine.Run("autoload-migration", Options{
		ProjectPath: tmpDir,
		DryRun:      false,
	})

	if err != nil {
		t.Fatalf("recipe failed: %v", err)
	}

	// No changes should be made - none of these are actual native rules
	if len(result.Changes) > 0 {
		t.Errorf("expected no changes for non-native rules, got %d", len(result.Changes))
		for _, c := range result.Changes {
			t.Logf("  Change: %s - %s", c.Path, c.Description)
		}
	}
}

func TestEngine_RunVerifyStep(t *testing.T) {
	// Test verification step execution
	tmpDir := t.TempDir()

	// Create a simple script that succeeds
	scriptPath := filepath.Join(tmpDir, "check.sh")
	if err := os.WriteFile(scriptPath, []byte("#!/bin/sh\nexit 0"), 0755); err != nil {
		t.Fatal(err)
	}

	engine := NewEngine()

	// Register a test recipe with a verify step
	testRecipe := &Recipe{
		Name:    "test-verify",
		Version: "1.0.0",
		Steps: []Step{
			{
				ID:   "verify",
				Name: "Verify Build",
				Verify: &VerifyStep{
					Command: filepath.Join(tmpDir, "check.sh"),
				},
			},
		},
	}
	if err := engine.Register(testRecipe); err != nil {
		t.Fatal(err)
	}

	// Run the recipe
	result, err := engine.Run("test-verify", Options{
		ProjectPath: tmpDir,
		DryRun:      false,
	})

	if err != nil {
		t.Fatalf("recipe failed: %v", err)
	}

	if !result.Success {
		t.Errorf("expected success, got failure: %v", result.Error)
	}
}

func TestEngine_VerifyStep_Failure(t *testing.T) {
	tmpDir := t.TempDir()

	// Create a script that fails
	scriptPath := filepath.Join(tmpDir, "fail.sh")
	if err := os.WriteFile(scriptPath, []byte("#!/bin/sh\nexit 1"), 0755); err != nil {
		t.Fatal(err)
	}

	engine := NewEngine()

	// Register a test recipe with a failing verify step
	testRecipe := &Recipe{
		Name:    "test-verify-fail",
		Version: "1.0.0",
		Steps: []Step{
			{
				ID:   "verify",
				Name: "Verify Build",
				Verify: &VerifyStep{
					Command: filepath.Join(tmpDir, "fail.sh"),
				},
			},
		},
	}
	if err := engine.Register(testRecipe); err != nil {
		t.Fatal(err)
	}

	// Run the recipe
	result, err := engine.Run("test-verify-fail", Options{
		ProjectPath: tmpDir,
		DryRun:      false,
	})

	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// Recipe should complete but report failure
	if result.Success {
		t.Error("expected failure, got success")
	}

	if result.Error == nil {
		t.Error("expected error in result")
	}
}

func TestEngine_VerifyStep_AllowFailure(t *testing.T) {
	tmpDir := t.TempDir()

	// Create a script that fails
	scriptPath := filepath.Join(tmpDir, "fail.sh")
	if err := os.WriteFile(scriptPath, []byte("#!/bin/sh\nexit 1"), 0755); err != nil {
		t.Fatal(err)
	}

	engine := NewEngine()

	// Register a test recipe with a failing verify step that allows failure
	testRecipe := &Recipe{
		Name:    "test-verify-allow-fail",
		Version: "1.0.0",
		Steps: []Step{
			{
				ID:   "verify",
				Name: "Verify Build",
				Verify: &VerifyStep{
					Command:      filepath.Join(tmpDir, "fail.sh"),
					AllowFailure: true,
				},
			},
		},
	}
	if err := engine.Register(testRecipe); err != nil {
		t.Fatal(err)
	}

	// Run the recipe
	result, err := engine.Run("test-verify-allow-fail", Options{
		ProjectPath: tmpDir,
		DryRun:      false,
	})

	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// Recipe should succeed since failure is allowed
	if !result.Success {
		t.Errorf("expected success (allow_failure=true), got failure: %v", result.Error)
	}
}

func TestEngine_VerifyStep_DryRun(t *testing.T) {
	tmpDir := t.TempDir()

	// Create a script that creates a marker file
	scriptPath := filepath.Join(tmpDir, "create_marker.sh")
	markerPath := filepath.Join(tmpDir, "marker")
	if err := os.WriteFile(scriptPath, []byte("#!/bin/sh\ntouch "+markerPath), 0755); err != nil {
		t.Fatal(err)
	}

	engine := NewEngine()

	// Register a test recipe
	testRecipe := &Recipe{
		Name:    "test-verify-dryrun",
		Version: "1.0.0",
		Steps: []Step{
			{
				ID:   "verify",
				Name: "Verify Build",
				Verify: &VerifyStep{
					Command: filepath.Join(tmpDir, "create_marker.sh"),
				},
			},
		},
	}
	if err := engine.Register(testRecipe); err != nil {
		t.Fatal(err)
	}

	// Run in dry-run mode
	result, err := engine.Run("test-verify-dryrun", Options{
		ProjectPath: tmpDir,
		DryRun:      true,
	})

	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if !result.Success {
		t.Errorf("expected success, got failure: %v", result.Error)
	}

	// Marker file should NOT exist (dry-run should skip verification)
	if _, err := os.Stat(markerPath); err == nil {
		t.Error("marker file exists - dry-run should not execute verification command")
	}
}

func TestAutoloadMigration_MultiLineLoad(t *testing.T) {
	// Test that AST-based ensure_load correctly handles multi-line load statements
	// This was a critical bug that broke 96% of Envoy BUILD files with text-based approach
	// Reference: https://github.com/envoyproxy/envoy uses multi-line loads extensively
	tmpDir := t.TempDir()

	// Create MODULE.bazel
	moduleContent := `module(name = "test_project")
`
	if err := os.WriteFile(filepath.Join(tmpDir, "MODULE.bazel"), []byte(moduleContent), 0644); err != nil {
		t.Fatal(err)
	}

	// Create BUILD file with multi-line load (the problematic pattern)
	buildContent := `load(
    "//bazel:custom.bzl",
    "custom_rule",
)

package(default_visibility = ["//visibility:public"])

# This cc_library needs a load statement added
cc_library(
    name = "test_lib",
    srcs = ["test.cc"],
)
`
	buildPath := filepath.Join(tmpDir, "BUILD")
	if err := os.WriteFile(buildPath, []byte(buildContent), 0644); err != nil {
		t.Fatal(err)
	}

	// Run recipe
	engine := NewEngine()
	result, err := engine.Run("autoload-migration", Options{
		ProjectPath: tmpDir,
		DryRun:      false,
	})

	if err != nil {
		t.Fatalf("recipe failed: %v", err)
	}

	if !result.Success {
		t.Errorf("expected success, got failure: %v", result.Error)
	}

	// Check that BUILD file was modified with load statement
	content, err := os.ReadFile(buildPath)
	if err != nil {
		t.Fatal(err)
	}

	// The new load should be added WITHOUT corrupting the existing multi-line load
	if !strings.Contains(string(content), `load("@rules_cc//cc:defs.bzl"`) {
		t.Error("expected load statement for cc_library to be added")
		t.Logf("Content:\n%s", content)
	}

	// CRITICAL: The output must be valid Starlark (not corrupted)
	// Text-based approach would insert the new load INSIDE the multi-line load
	if strings.Contains(string(content), "load(\nload(") || strings.Contains(string(content), "load(\n    load(") {
		t.Error("BUG: new load was inserted inside existing multi-line load (corrupted output)")
		t.Logf("Content:\n%s", content)
	}

	// Verify original multi-line load is preserved
	if !strings.Contains(string(content), "//bazel:custom.bzl") {
		t.Error("original load was corrupted")
		t.Logf("Content:\n%s", content)
	}
}
