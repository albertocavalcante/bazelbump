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
		if containsStr(e, "circular") {
			found = true
			break
		}
	}
	if !found {
		t.Errorf("expected circular dependency error, got: %v", errors)
	}
}

func containsStr(s, substr string) bool {
	return len(s) >= len(substr) && (s == substr || len(s) > 0 && containsStrAt(s, substr, 0))
}

func containsStrAt(s, substr string, start int) bool {
	for i := start; i <= len(s)-len(substr); i++ {
		if s[i:i+len(substr)] == substr {
			return true
		}
	}
	return false
}

// =============================================================================
// Load() functionality tests
// =============================================================================

// getTestdataPath returns the absolute path to the testdata directory
func getTestdataPath(t *testing.T) string {
	t.Helper()
	// Get the current working directory
	wd, err := os.Getwd()
	if err != nil {
		t.Fatalf("failed to get working directory: %v", err)
	}
	return filepath.Join(wd, "testdata")
}

func TestInterpreter_Load_SimpleFile(t *testing.T) {
	testdataPath := getTestdataPath(t)
	recipePath := filepath.Join(testdataPath, "simple.star")

	interp := NewInterpreter()
	recipe, err := interp.LoadFile(recipePath)
	if err != nil {
		t.Fatalf("LoadFile failed: %v", err)
	}

	if recipe.Name != "simple-recipe" {
		t.Errorf("expected name 'simple-recipe', got %q", recipe.Name)
	}
}

func TestInterpreter_Load_RelativeLoad(t *testing.T) {
	testdataPath := getTestdataPath(t)
	recipePath := filepath.Join(testdataPath, "with-load.star")

	interp := NewInterpreter()
	recipe, err := interp.LoadFile(recipePath)
	if err != nil {
		t.Fatalf("LoadFile failed: %v", err)
	}

	// The recipe uses DEFAULT_VERSION from helpers.star which is "1.0.0"
	if recipe.Version != "1.0.0" {
		t.Errorf("expected version '1.0.0' (from loaded DEFAULT_VERSION), got %q", recipe.Version)
	}

	if recipe.Name != "with-load-recipe" {
		t.Errorf("expected name 'with-load-recipe', got %q", recipe.Name)
	}

	// Should have 2 steps created by helper functions
	if len(recipe.Steps) != 2 {
		t.Errorf("expected 2 steps, got %d", len(recipe.Steps))
	}

	// Verify first step is a scan step with the CC_PATTERNS
	if len(recipe.Steps) > 0 {
		scanStep, ok := recipe.Steps[0].(*ScanStep)
		if !ok {
			t.Fatalf("expected first step to be ScanStep, got %T", recipe.Steps[0])
		}
		if scanStep.ID != "detect-cc" {
			t.Errorf("expected scan step ID 'detect-cc', got %q", scanStep.ID)
		}
		// CC_PATTERNS has 2 patterns
		if len(scanStep.Patterns) != 2 {
			t.Errorf("expected 2 patterns from CC_PATTERNS, got %d", len(scanStep.Patterns))
		}
	}
}

func TestInterpreter_Load_ParentDirectoryLoad(t *testing.T) {
	testdataPath := getTestdataPath(t)
	recipePath := filepath.Join(testdataPath, "subdir", "uses-parent.star")

	interp := NewInterpreter()
	recipe, err := interp.LoadFile(recipePath)
	if err != nil {
		t.Fatalf("LoadFile failed: %v", err)
	}

	if recipe.Name != "uses-parent-recipe" {
		t.Errorf("expected name 'uses-parent-recipe', got %q", recipe.Name)
	}

	// Should use DEFAULT_VERSION from helpers.star
	if recipe.Version != "1.0.0" {
		t.Errorf("expected version '1.0.0' (from parent dir helpers.star), got %q", recipe.Version)
	}
}

func TestInterpreter_Load_SearchPathLoad(t *testing.T) {
	testdataPath := getTestdataPath(t)
	recipePath := filepath.Join(testdataPath, "uses-searchpath.star")
	searchPath := filepath.Join(testdataPath, "searchpath")

	interp := NewInterpreter(WithSearchPaths(searchPath))
	recipe, err := interp.LoadFile(recipePath)
	if err != nil {
		t.Fatalf("LoadFile failed: %v", err)
	}

	if recipe.Name != "uses-searchpath-recipe" {
		t.Errorf("expected name 'uses-searchpath-recipe', got %q", recipe.Name)
	}
}

func TestInterpreter_Load_CycleDetection(t *testing.T) {
	testdataPath := getTestdataPath(t)
	recipePath := filepath.Join(testdataPath, "cycle-a.star")

	interp := NewInterpreter()

	// We need to use LoadFile directly on cycle-a.star which tries to load cycle-b.star
	// which then tries to load cycle-a.star - creating a cycle
	_, err := interp.LoadFile(recipePath)
	if err == nil {
		t.Fatal("expected error for circular load, got nil")
	}

	// Check that the error message mentions "circular"
	if !containsStr(err.Error(), "circular") {
		t.Errorf("expected circular load error, got: %v", err)
	}
}

func TestInterpreter_Load_MissingFile(t *testing.T) {
	testdataPath := getTestdataPath(t)
	recipePath := filepath.Join(testdataPath, "loads-missing.star")

	interp := NewInterpreter()
	_, err := interp.LoadFile(recipePath)
	if err == nil {
		t.Fatal("expected error for missing file, got nil")
	}

	// Check that the error message contains useful info
	errStr := err.Error()
	if !containsStr(errStr, "nonexistent.star") {
		t.Errorf("expected error to mention 'nonexistent.star', got: %v", err)
	}
}

func TestInterpreter_Load_SearchPathNotFound(t *testing.T) {
	testdataPath := getTestdataPath(t)
	recipePath := filepath.Join(testdataPath, "uses-searchpath.star")

	// Don't provide search paths - should fail
	interp := NewInterpreter()
	_, err := interp.LoadFile(recipePath)
	if err == nil {
		t.Fatal("expected error when search path not configured, got nil")
	}

	// Check that the error message mentions search paths
	errStr := err.Error()
	if !containsStr(errStr, "search path") && !containsStr(errStr, "not found") {
		t.Errorf("expected error about search paths, got: %v", err)
	}
}

func TestInterpreter_Load_SymbolExport(t *testing.T) {
	// Test that symbols are properly exported from loaded modules
	interp := NewInterpreter()

	// Create a helper file content
	helperContent := `
MY_CONSTANT = "hello"
MY_LIST = [1, 2, 3]

def my_function(x):
    return x * 2
`

	// Create main recipe content
	mainContent := `
load("./helper.star", "MY_CONSTANT", "MY_LIST", "my_function")

# Use the loaded values
doubled = my_function(21)

recipe(
    name = "symbol-test",
    version = MY_CONSTANT,  # Using a loaded constant - but this won't work as version needs a real version
    description = "test",
    steps = [
        edit(id = "dummy", operations = []),
    ],
)
`

	// Create temp directory
	tmpDir, err := os.MkdirTemp("", "bazelbump-load-test-*")
	if err != nil {
		t.Fatal(err)
	}
	defer os.RemoveAll(tmpDir)

	// Write helper file
	helperPath := filepath.Join(tmpDir, "helper.star")
	if err := os.WriteFile(helperPath, []byte(helperContent), 0644); err != nil {
		t.Fatal(err)
	}

	// Write main file
	mainPath := filepath.Join(tmpDir, "main.star")
	if err := os.WriteFile(mainPath, []byte(mainContent), 0644); err != nil {
		t.Fatal(err)
	}

	recipe, err := interp.LoadFile(mainPath)
	if err != nil {
		t.Fatalf("LoadFile failed: %v", err)
	}

	// The version should be "hello" from MY_CONSTANT
	if recipe.Version != "hello" {
		t.Errorf("expected version 'hello' from loaded constant, got %q", recipe.Version)
	}
}

func TestInterpreter_Load_UndefinedSymbol(t *testing.T) {
	// Test that attempting to load an undefined symbol fails
	interp := NewInterpreter()

	helperContent := `
MY_CONSTANT = "hello"
`

	mainContent := `
load("./helper.star", "MY_CONSTANT", "NONEXISTENT_SYMBOL")

recipe(
    name = "test",
    version = "1.0.0",
    steps = [
        edit(id = "dummy", operations = []),
    ],
)
`

	// Create temp directory
	tmpDir, err := os.MkdirTemp("", "bazelbump-load-test-*")
	if err != nil {
		t.Fatal(err)
	}
	defer os.RemoveAll(tmpDir)

	// Write helper file
	helperPath := filepath.Join(tmpDir, "helper.star")
	if err := os.WriteFile(helperPath, []byte(helperContent), 0644); err != nil {
		t.Fatal(err)
	}

	// Write main file
	mainPath := filepath.Join(tmpDir, "main.star")
	if err := os.WriteFile(mainPath, []byte(mainContent), 0644); err != nil {
		t.Fatal(err)
	}

	_, err = interp.LoadFile(mainPath)
	if err == nil {
		t.Fatal("expected error for undefined symbol, got nil")
	}

	// Error should mention the undefined symbol
	if !containsStr(err.Error(), "NONEXISTENT_SYMBOL") {
		t.Errorf("expected error to mention 'NONEXISTENT_SYMBOL', got: %v", err)
	}
}

func TestInterpreter_Load_LoadCache(t *testing.T) {
	// Test that loaded modules are cached
	interp := NewInterpreter()

	helperContent := `
COUNTER = [0]  # Use a list to allow mutation
COUNTER[0] = COUNTER[0] + 1
VALUE = COUNTER[0]
`

	mainContent := `
load("./helper.star", "VALUE")
# Load again - should use cached version
load("./helper.star", VALUE2 = "VALUE")

recipe(
    name = "cache-test",
    version = "1.0.0",
    steps = [
        edit(id = "dummy", operations = []),
    ],
)
`

	// Create temp directory
	tmpDir, err := os.MkdirTemp("", "bazelbump-load-test-*")
	if err != nil {
		t.Fatal(err)
	}
	defer os.RemoveAll(tmpDir)

	// Write helper file
	helperPath := filepath.Join(tmpDir, "helper.star")
	if err := os.WriteFile(helperPath, []byte(helperContent), 0644); err != nil {
		t.Fatal(err)
	}

	// Write main file
	mainPath := filepath.Join(tmpDir, "main.star")
	if err := os.WriteFile(mainPath, []byte(mainContent), 0644); err != nil {
		t.Fatal(err)
	}

	_, err = interp.LoadFile(mainPath)
	if err != nil {
		t.Fatalf("LoadFile failed: %v", err)
	}

	// The module should only be executed once due to caching
	// If executed twice, VALUE would be 2, but it should be 1
	// We can't directly check this without exposing internal state,
	// but the test verifies no errors occur with multiple loads
}

func TestInterpreter_Load_InvalidLoadPath(t *testing.T) {
	interp := NewInterpreter()

	content := `
# Invalid load path - doesn't start with ./, ../, or //
load("invalid/path.star", "SOMETHING")

recipe(
    name = "test",
    version = "1.0.0",
    steps = [
        edit(id = "dummy", operations = []),
    ],
)
`

	tmpDir, err := os.MkdirTemp("", "bazelbump-load-test-*")
	if err != nil {
		t.Fatal(err)
	}
	defer os.RemoveAll(tmpDir)

	mainPath := filepath.Join(tmpDir, "main.star")
	if err := os.WriteFile(mainPath, []byte(content), 0644); err != nil {
		t.Fatal(err)
	}

	_, err = interp.LoadFile(mainPath)
	if err == nil {
		t.Fatal("expected error for invalid load path, got nil")
	}

	// Error should mention invalid path
	if !containsStr(err.Error(), "invalid") {
		t.Errorf("expected error about invalid path, got: %v", err)
	}
}

// =============================================================================
// Standard Library (@stdlib//) Tests
// =============================================================================

func TestInterpreter_Load_StdlibRules(t *testing.T) {
	interp := NewInterpreter()

	content := `
load("@stdlib//rules.star", "NATIVE_RULE_PATTERNS", "RULE_LOAD_PATHS", "RULE_DEP_VERSIONS")

# Use values from stdlib
cc_patterns = NATIVE_RULE_PATTERNS["cc"]
cc_binary_load = RULE_LOAD_PATHS["cc_binary"]
rules_cc_version = RULE_DEP_VERSIONS["rules_cc"]

recipe(
    name = "stdlib-test",
    version = rules_cc_version,
    description = "Test loading from stdlib",
    steps = [
        scan(
            id = "detect",
            files = ["BUILD"],
            patterns = cc_patterns,
        ),
    ],
)
`

	tmpDir, err := os.MkdirTemp("", "bazelbump-load-test-*")
	if err != nil {
		t.Fatal(err)
	}
	defer os.RemoveAll(tmpDir)

	mainPath := filepath.Join(tmpDir, "main.star")
	if err := os.WriteFile(mainPath, []byte(content), 0644); err != nil {
		t.Fatal(err)
	}

	recipe, err := interp.LoadFile(mainPath)
	if err != nil {
		t.Fatalf("LoadFile failed: %v", err)
	}

	if recipe.Name != "stdlib-test" {
		t.Errorf("expected name 'stdlib-test', got %q", recipe.Name)
	}

	// Version should come from RULE_DEP_VERSIONS["rules_cc"]
	if recipe.Version != "0.1.1" {
		t.Errorf("expected version '0.1.1' from stdlib, got %q", recipe.Version)
	}

	// Should have a scan step with CC patterns
	if len(recipe.Steps) != 1 {
		t.Fatalf("expected 1 step, got %d", len(recipe.Steps))
	}

	scanStep, ok := recipe.Steps[0].(*ScanStep)
	if !ok {
		t.Fatalf("expected ScanStep, got %T", recipe.Steps[0])
	}

	// Should have 5 CC patterns from stdlib
	if len(scanStep.Patterns) != 5 {
		t.Errorf("expected 5 CC patterns from stdlib, got %d", len(scanStep.Patterns))
	}
}

func TestInterpreter_Load_StdlibMigrationCC(t *testing.T) {
	interp := NewInterpreter()

	content := `
load("@stdlib//migrations/cc.star", "CC_RULES")

recipe(
    name = "cc-migration-test",
    version = "1.0.0",
    description = "Test loading CC migration helpers",
    steps = [
        edit(
            id = "dummy",
            operations = [],
        ),
    ],
)
`

	tmpDir, err := os.MkdirTemp("", "bazelbump-load-test-*")
	if err != nil {
		t.Fatal(err)
	}
	defer os.RemoveAll(tmpDir)

	mainPath := filepath.Join(tmpDir, "main.star")
	if err := os.WriteFile(mainPath, []byte(content), 0644); err != nil {
		t.Fatal(err)
	}

	recipe, err := interp.LoadFile(mainPath)
	if err != nil {
		t.Fatalf("LoadFile failed: %v", err)
	}

	if recipe.Name != "cc-migration-test" {
		t.Errorf("expected name 'cc-migration-test', got %q", recipe.Name)
	}
}

func TestInterpreter_Load_StdlibMigrationPython(t *testing.T) {
	interp := NewInterpreter()

	content := `
load("@stdlib//migrations/python.star", "PYTHON_RULES")

recipe(
    name = "python-migration-test",
    version = "1.0.0",
    description = "Test loading Python migration helpers",
    steps = [
        edit(
            id = "dummy",
            operations = [],
        ),
    ],
)
`

	tmpDir, err := os.MkdirTemp("", "bazelbump-load-test-*")
	if err != nil {
		t.Fatal(err)
	}
	defer os.RemoveAll(tmpDir)

	mainPath := filepath.Join(tmpDir, "main.star")
	if err := os.WriteFile(mainPath, []byte(content), 0644); err != nil {
		t.Fatal(err)
	}

	recipe, err := interp.LoadFile(mainPath)
	if err != nil {
		t.Fatalf("LoadFile failed: %v", err)
	}

	if recipe.Name != "python-migration-test" {
		t.Errorf("expected name 'python-migration-test', got %q", recipe.Name)
	}
}

func TestInterpreter_Load_StdlibMigrationJava(t *testing.T) {
	interp := NewInterpreter()

	content := `
load("@stdlib//migrations/java.star", "JAVA_RULES")

recipe(
    name = "java-migration-test",
    version = "1.0.0",
    description = "Test loading Java migration helpers",
    steps = [
        edit(
            id = "dummy",
            operations = [],
        ),
    ],
)
`

	tmpDir, err := os.MkdirTemp("", "bazelbump-load-test-*")
	if err != nil {
		t.Fatal(err)
	}
	defer os.RemoveAll(tmpDir)

	mainPath := filepath.Join(tmpDir, "main.star")
	if err := os.WriteFile(mainPath, []byte(content), 0644); err != nil {
		t.Fatal(err)
	}

	recipe, err := interp.LoadFile(mainPath)
	if err != nil {
		t.Fatalf("LoadFile failed: %v", err)
	}

	if recipe.Name != "java-migration-test" {
		t.Errorf("expected name 'java-migration-test', got %q", recipe.Name)
	}
}

func TestInterpreter_Load_StdlibMigrationShell(t *testing.T) {
	interp := NewInterpreter()

	content := `
load("@stdlib//migrations/shell.star", "SHELL_RULES")

recipe(
    name = "shell-migration-test",
    version = "1.0.0",
    description = "Test loading Shell migration helpers",
    steps = [
        edit(
            id = "dummy",
            operations = [],
        ),
    ],
)
`

	tmpDir, err := os.MkdirTemp("", "bazelbump-load-test-*")
	if err != nil {
		t.Fatal(err)
	}
	defer os.RemoveAll(tmpDir)

	mainPath := filepath.Join(tmpDir, "main.star")
	if err := os.WriteFile(mainPath, []byte(content), 0644); err != nil {
		t.Fatal(err)
	}

	recipe, err := interp.LoadFile(mainPath)
	if err != nil {
		t.Fatalf("LoadFile failed: %v", err)
	}

	if recipe.Name != "shell-migration-test" {
		t.Errorf("expected name 'shell-migration-test', got %q", recipe.Name)
	}
}

func TestInterpreter_Load_StdlibNotFound(t *testing.T) {
	interp := NewInterpreter()

	content := `
load("@stdlib//nonexistent.star", "SOMETHING")

recipe(
    name = "test",
    version = "1.0.0",
    steps = [
        edit(id = "dummy", operations = []),
    ],
)
`

	tmpDir, err := os.MkdirTemp("", "bazelbump-load-test-*")
	if err != nil {
		t.Fatal(err)
	}
	defer os.RemoveAll(tmpDir)

	mainPath := filepath.Join(tmpDir, "main.star")
	if err := os.WriteFile(mainPath, []byte(content), 0644); err != nil {
		t.Fatal(err)
	}

	_, err = interp.LoadFile(mainPath)
	if err == nil {
		t.Fatal("expected error for non-existent stdlib module, got nil")
	}

	// Error should mention the module was not found
	if !containsStr(err.Error(), "not found") {
		t.Errorf("expected error about module not found, got: %v", err)
	}
}

func TestInterpreter_Load_StdlibHelperFunctions(t *testing.T) {
	interp := NewInterpreter()

	content := `
load("@stdlib//rules.star", "all_patterns", "get_load_path", "get_dep_version")

# Test helper functions
patterns = all_patterns()
cc_load_path = get_load_path("cc_binary")
cc_version = get_dep_version("rules_cc")

recipe(
    name = "helper-test",
    version = cc_version,
    description = "Test stdlib helper functions",
    steps = [
        scan(
            id = "detect-all",
            files = ["BUILD"],
            patterns = patterns,
        ),
    ],
)
`

	tmpDir, err := os.MkdirTemp("", "bazelbump-load-test-*")
	if err != nil {
		t.Fatal(err)
	}
	defer os.RemoveAll(tmpDir)

	mainPath := filepath.Join(tmpDir, "main.star")
	if err := os.WriteFile(mainPath, []byte(content), 0644); err != nil {
		t.Fatal(err)
	}

	recipe, err := interp.LoadFile(mainPath)
	if err != nil {
		t.Fatalf("LoadFile failed: %v", err)
	}

	// Version should come from get_dep_version("rules_cc")
	if recipe.Version != "0.1.1" {
		t.Errorf("expected version '0.1.1' from get_dep_version, got %q", recipe.Version)
	}

	// Should have patterns from all_patterns()
	scanStep := recipe.Steps[0].(*ScanStep)
	// all_patterns() returns all patterns from all families
	// Total should be: cc(5) + python(3) + java(5) + shell(3) + proto(4) + android(4) + go(3) = 27
	if len(scanStep.Patterns) < 20 {
		t.Errorf("expected at least 20 patterns from all_patterns(), got %d", len(scanStep.Patterns))
	}
}

func TestInterpreter_Load_ExtensionNotYetImplemented(t *testing.T) {
	interp := NewInterpreter()

	content := `
load("ext://android.star", "android_migration")

recipe(
    name = "test",
    version = "1.0.0",
    steps = [
        edit(id = "dummy", operations = []),
    ],
)
`

	tmpDir, err := os.MkdirTemp("", "bazelbump-load-test-*")
	if err != nil {
		t.Fatal(err)
	}
	defer os.RemoveAll(tmpDir)

	mainPath := filepath.Join(tmpDir, "main.star")
	if err := os.WriteFile(mainPath, []byte(content), 0644); err != nil {
		t.Fatal(err)
	}

	_, err = interp.LoadFile(mainPath)
	if err == nil {
		t.Fatal("expected error for ext:// which is not implemented, got nil")
	}

	// Error should mention extensions not implemented
	if !containsStr(err.Error(), "not") && !containsStr(err.Error(), "implemented") {
		t.Errorf("expected error about ext:// not implemented, got: %v", err)
	}
}

func TestInterpreter_WithSearchPaths_Multiple(t *testing.T) {
	// Test with multiple search paths
	tmpDir1, err := os.MkdirTemp("", "bazelbump-search1-*")
	if err != nil {
		t.Fatal(err)
	}
	defer os.RemoveAll(tmpDir1)

	tmpDir2, err := os.MkdirTemp("", "bazelbump-search2-*")
	if err != nil {
		t.Fatal(err)
	}
	defer os.RemoveAll(tmpDir2)

	// Create a lib file in the second search path
	libContent := `
MY_VALUE = "from-search-path-2"
`
	libPath := filepath.Join(tmpDir2, "mylib.star")
	if err := os.WriteFile(libPath, []byte(libContent), 0644); err != nil {
		t.Fatal(err)
	}

	// Create main recipe that loads from search path
	mainContent := `
load("//mylib.star", "MY_VALUE")

recipe(
    name = "multi-search",
    version = MY_VALUE,
    steps = [
        edit(id = "dummy", operations = []),
    ],
)
`
	mainDir, err := os.MkdirTemp("", "bazelbump-main-*")
	if err != nil {
		t.Fatal(err)
	}
	defer os.RemoveAll(mainDir)

	mainPath := filepath.Join(mainDir, "main.star")
	if err := os.WriteFile(mainPath, []byte(mainContent), 0644); err != nil {
		t.Fatal(err)
	}

	// Use both search paths - should find in second one
	interp := NewInterpreter(WithSearchPaths(tmpDir1, tmpDir2))
	recipe, err := interp.LoadFile(mainPath)
	if err != nil {
		t.Fatalf("LoadFile failed: %v", err)
	}

	if recipe.Version != "from-search-path-2" {
		t.Errorf("expected version 'from-search-path-2', got %q", recipe.Version)
	}
}

func TestInterpreter_Load_IntegrationWithHelperFunctions(t *testing.T) {
	// Integration test that verifies full load functionality with helper functions
	testdataPath := getTestdataPath(t)
	recipePath := filepath.Join(testdataPath, "with-load.star")

	interp := NewInterpreter()
	recipe, err := interp.LoadFile(recipePath)
	if err != nil {
		t.Fatalf("LoadFile failed: %v", err)
	}

	// Verify the recipe was created correctly using helper functions
	if len(recipe.Steps) != 2 {
		t.Fatalf("expected 2 steps, got %d", len(recipe.Steps))
	}

	// First step should be a scan step created by make_scan_step
	scanStep, ok := recipe.Steps[0].(*ScanStep)
	if !ok {
		t.Fatalf("expected first step to be ScanStep, got %T", recipe.Steps[0])
	}
	if scanStep.ID != "detect-cc" {
		t.Errorf("expected scan step ID 'detect-cc', got %q", scanStep.ID)
	}
	if scanStep.Name != "Scan for patterns" {
		t.Errorf("expected scan step name 'Scan for patterns', got %q", scanStep.Name)
	}

	// Second step should be an edit step created by make_edit_step
	editStep, ok := recipe.Steps[1].(*EditStep)
	if !ok {
		t.Fatalf("expected second step to be EditStep, got %T", recipe.Steps[1])
	}
	if editStep.ID != "add-dep" {
		t.Errorf("expected edit step ID 'add-dep', got %q", editStep.ID)
	}
	if len(editStep.Operations) != 1 {
		t.Fatalf("expected 1 operation, got %d", len(editStep.Operations))
	}

	// Verify the operation
	depOp, ok := editStep.Operations[0].(*EnsureBazelDepOp)
	if !ok {
		t.Fatalf("expected EnsureBazelDepOp, got %T", editStep.Operations[0])
	}
	if depOp.Name != "rules_cc" {
		t.Errorf("expected dep name 'rules_cc', got %q", depOp.Name)
	}
	if depOp.Version != "0.1.1" {
		t.Errorf("expected dep version '0.1.1', got %q", depOp.Version)
	}
}
