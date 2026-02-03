// Copyright 2025 bazelbump authors
// SPDX-License-Identifier: Apache-2.0

package starlark

import (
	"os"
	"path/filepath"
	"testing"
)

func TestInterpreter_LoadString_SimpleRecipe(t *testing.T) {
	interp := NewInterpreter()

	content := `
recipe(
    name = "test-recipe",
    version = "1.0.0",
    description = "A test recipe",
    steps = [
        scan(
            id = "detect",
            name = "Detect rules",
            files = ["BUILD"],
            patterns = [r"cc_binary\s*\("],
        ),
        edit(
            id = "migrate",
            depends_on = ["detect"],
            operations = [
                ensure_bazel_dep("rules_cc", "0.1.1"),
            ],
        ),
    ],
)
`

	recipe, err := interp.LoadString("test.star", content)
	if err != nil {
		t.Fatalf("LoadString failed: %v", err)
	}

	if recipe.Name != "test-recipe" {
		t.Errorf("expected name 'test-recipe', got %q", recipe.Name)
	}
	if recipe.Version != "1.0.0" {
		t.Errorf("expected version '1.0.0', got %q", recipe.Version)
	}
	if len(recipe.Steps) != 2 {
		t.Errorf("expected 2 steps, got %d", len(recipe.Steps))
	}

	// Check first step is a scan
	scanStep, ok := recipe.Steps[0].(*ScanStep)
	if !ok {
		t.Fatalf("expected first step to be ScanStep, got %T", recipe.Steps[0])
	}
	if scanStep.ID != "detect" {
		t.Errorf("expected scan step ID 'detect', got %q", scanStep.ID)
	}

	// Check second step is an edit
	editStep, ok := recipe.Steps[1].(*EditStep)
	if !ok {
		t.Fatalf("expected second step to be EditStep, got %T", recipe.Steps[1])
	}
	if editStep.ID != "migrate" {
		t.Errorf("expected edit step ID 'migrate', got %q", editStep.ID)
	}
	if len(editStep.Operations) != 1 {
		t.Errorf("expected 1 operation, got %d", len(editStep.Operations))
	}
}

func TestInterpreter_LoadString_AllOperations(t *testing.T) {
	interp := NewInterpreter()

	content := `
recipe(
    name = "ops-test",
    version = "1.0.0",
    steps = [
        edit(
            id = "all-ops",
            operations = [
                ensure_load("cc_binary", "@rules_cc//cc:defs.bzl"),
                ensure_bazel_dep("rules_cc", "0.1.1"),
                update_bazel_dep("rules_cc", "0.2.0"),
                remove_bazel_dep("old_dep"),
                remove_flag("incompatible_foo"),
                replace_flag("--old_flag", "--new_flag"),
                ensure_flag("new_flag", value="true"),
                replace_text("old_text", "new_text", files=["*.txt"]),
            ],
        ),
    ],
)
`

	recipe, err := interp.LoadString("test.star", content)
	if err != nil {
		t.Fatalf("LoadString failed: %v", err)
	}

	editStep := recipe.Steps[0].(*EditStep)
	if len(editStep.Operations) != 8 {
		t.Errorf("expected 8 operations, got %d", len(editStep.Operations))
	}

	// Verify operation types
	expectedTypes := []string{
		"ensure_load",
		"ensure_bazel_dep",
		"update_bazel_dep",
		"remove_bazel_dep",
		"remove_flag",
		"replace_flag",
		"ensure_flag",
		"replace_text",
	}

	for i, expected := range expectedTypes {
		if editStep.Operations[i].GetType() != expected {
			t.Errorf("operation %d: expected type %q, got %q", i, expected, editStep.Operations[i].GetType())
		}
	}
}

func TestInterpreter_LoadString_VerifyStep(t *testing.T) {
	interp := NewInterpreter()

	content := `
recipe(
    name = "verify-test",
    version = "1.0.0",
    steps = [
        verify(
            id = "check",
            name = "Verify build",
            command = "bazel build //...",
            timeout = "10m",
            allow_failure = True,
        ),
    ],
)
`

	recipe, err := interp.LoadString("test.star", content)
	if err != nil {
		t.Fatalf("LoadString failed: %v", err)
	}

	verifyStep, ok := recipe.Steps[0].(*VerifyStep)
	if !ok {
		t.Fatalf("expected VerifyStep, got %T", recipe.Steps[0])
	}

	if verifyStep.Command != "bazel build //..." {
		t.Errorf("expected command 'bazel build //...', got %q", verifyStep.Command)
	}
	if !verifyStep.AllowFailure {
		t.Error("expected AllowFailure to be true")
	}
}

func TestInterpreter_LoadString_OnFailure(t *testing.T) {
	tests := []struct {
		onFailure string
		expected  FailureMode
	}{
		{"rollback", FailureModeRollback},
		{"stop", FailureModeStop},
		{"continue", FailureModeContinue},
	}

	for _, tc := range tests {
		t.Run(tc.onFailure, func(t *testing.T) {
			interp := NewInterpreter()

			content := `
recipe(
    name = "test",
    version = "1.0.0",
    steps = [
        edit(id = "dummy", operations = []),
    ],
    on_failure = "` + tc.onFailure + `",
)
`
			recipe, err := interp.LoadString("test.star", content)
			if err != nil {
				t.Fatalf("LoadString failed: %v", err)
			}

			if recipe.OnFailure != tc.expected {
				t.Errorf("expected OnFailure %v, got %v", tc.expected, recipe.OnFailure)
			}
		})
	}
}

func TestInterpreter_LoadString_Error_NoRecipe(t *testing.T) {
	interp := NewInterpreter()

	content := `
# No recipe defined
x = 1 + 1
`

	_, err := interp.LoadString("test.star", content)
	if err == nil {
		t.Fatal("expected error for missing recipe(), got nil")
	}
}

func TestInterpreter_LoadString_Error_DuplicateRecipe(t *testing.T) {
	interp := NewInterpreter()

	content := `
recipe(name = "first", version = "1.0.0", steps = [edit(id = "a", operations = [])])
recipe(name = "second", version = "1.0.0", steps = [edit(id = "b", operations = [])])
`

	_, err := interp.LoadString("test.star", content)
	if err == nil {
		t.Fatal("expected error for duplicate recipe(), got nil")
	}
}

func TestInterpreter_LoadString_Error_InvalidOnFailure(t *testing.T) {
	interp := NewInterpreter()

	content := `
recipe(
    name = "test",
    version = "1.0.0",
    steps = [edit(id = "a", operations = [])],
    on_failure = "invalid",
)
`

	_, err := interp.LoadString("test.star", content)
	if err == nil {
		t.Fatal("expected error for invalid on_failure, got nil")
	}
}

func TestInterpreter_LoadFile(t *testing.T) {
	// Create a temporary directory with a recipe file
	tmpDir, err := os.MkdirTemp("", "bazelbump-test-*")
	if err != nil {
		t.Fatal(err)
	}
	defer os.RemoveAll(tmpDir)

	recipePath := filepath.Join(tmpDir, "test.star")
	content := `
recipe(
    name = "file-test",
    version = "2.0.0",
    steps = [
        scan(id = "s", files = ["BUILD"], patterns = ["py_binary"]),
    ],
)
`
	if err := os.WriteFile(recipePath, []byte(content), 0644); err != nil {
		t.Fatal(err)
	}

	interp := NewInterpreter()
	recipe, err := interp.LoadFile(recipePath)
	if err != nil {
		t.Fatalf("LoadFile failed: %v", err)
	}

	if recipe.Name != "file-test" {
		t.Errorf("expected name 'file-test', got %q", recipe.Name)
	}
	if recipe.SourceFile != recipePath {
		t.Errorf("expected SourceFile %q, got %q", recipePath, recipe.SourceFile)
	}
}

func TestExecutor_Validate(t *testing.T) {
	executor := NewExecutor(".", Options{})

	recipe := &Recipe{
		Name:    "test",
		Version: "1.0.0",
		Steps: []Step{
			&ScanStep{
				baseStep:     baseStep{ID: "scan1"},
				FilePatterns: []string{"BUILD"},
				Patterns:     []string{`cc_binary\s*\(`},
			},
			&EditStep{
				baseStep:  baseStep{ID: "edit1", DependsOn: []string{"scan1"}},
				Operations: []Operation{
					&EnsureBazelDepOp{
						baseOperation: baseOperation{Type: "ensure_bazel_dep"},
						Name:          "rules_cc",
						Version:       "0.1.1",
					},
				},
			},
		},
	}

	errors := executor.validate(recipe)
	if len(errors) > 0 {
		t.Errorf("expected no validation errors, got: %v", errors)
	}
}

func TestExecutor_Validate_MissingDependency(t *testing.T) {
	executor := NewExecutor(".", Options{})

	recipe := &Recipe{
		Name:    "test",
		Version: "1.0.0",
		Steps: []Step{
			&EditStep{
				baseStep:   baseStep{ID: "edit1", DependsOn: []string{"nonexistent"}},
				Operations: []Operation{},
			},
		},
	}

	errors := executor.validate(recipe)
	if len(errors) == 0 {
		t.Error("expected validation errors for missing dependency")
	}
}

func TestExecutor_Validate_CyclicDependency(t *testing.T) {
	executor := NewExecutor(".", Options{})

	recipe := &Recipe{
		Name:    "test",
		Version: "1.0.0",
		Steps: []Step{
			&EditStep{
				baseStep:   baseStep{ID: "a", DependsOn: []string{"b"}},
				Operations: []Operation{},
			},
			&EditStep{
				baseStep:   baseStep{ID: "b", DependsOn: []string{"a"}},
				Operations: []Operation{},
			},
		},
	}

	errors := executor.validate(recipe)
	if len(errors) == 0 {
		t.Error("expected validation errors for cyclic dependency")
	}

	found := false
	for _, e := range errors {
		if contains(e, "circular") {
			found = true
			break
		}
	}
	if !found {
		t.Errorf("expected circular dependency error, got: %v", errors)
	}
}

func contains(s, substr string) bool {
	return len(s) >= len(substr) && (s == substr || len(s) > 0 && containsAt(s, substr, 0))
}

func containsAt(s, substr string, start int) bool {
	for i := start; i <= len(s)-len(substr); i++ {
		if s[i:i+len(substr)] == substr {
			return true
		}
	}
	return false
}
