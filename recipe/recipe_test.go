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
