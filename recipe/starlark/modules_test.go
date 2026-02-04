// Copyright 2025 bazelbump authors
// SPDX-License-Identifier: Apache-2.0

package starlark

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// TestVersionModuleAccess tests that the version module is accessible and works.
func TestVersionModuleAccess(t *testing.T) {
	tests := []struct {
		name        string
		script      string
		wantErr     bool
		errContains string
	}{
		{
			name: "version.parse",
			script: `
def check(ctx):
    v = version.parse("7.0.0")
    if v.major != 7:
        fail(msg="version.parse failed")
    return []

recipe(name="test", version="1.0.0", steps=[scan(id="s1", impl=check)])
`,
		},
		{
			name: "version.compare",
			script: `
def check(ctx):
    if version.compare("7.0.0", "8.0.0") != -1:
        fail(msg="version.compare failed")
    return []

recipe(name="test", version="1.0.0", steps=[scan(id="s1", impl=check)])
`,
		},
		{
			name: "version.matches",
			script: `
def check(ctx):
    if not version.matches("7.0.0", [">=6.0.0", "<=8.0.0"]):
        fail(msg="version.matches failed")
    return []

recipe(name="test", version="1.0.0", steps=[scan(id="s1", impl=check)])
`,
		},
		{
			name: "version.at_least",
			script: `
def check(ctx):
    if not version.at_least("8.0.0", "7.0.0"):
        fail(msg="version.at_least failed")
    return []

recipe(name="test", version="1.0.0", steps=[scan(id="s1", impl=check)])
`,
		},
		{
			name: "version.in_range",
			script: `
def check(ctx):
    if not version.in_range("7.5.0", "7.0.0", "8.0.0"):
        fail(msg="version.in_range failed")
    return []

recipe(name="test", version="1.0.0", steps=[scan(id="s1", impl=check)])
`,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			interp := NewInterpreter()
			_, err := interp.LoadString("test.star", tt.script)

			if tt.wantErr {
				if err == nil {
					t.Errorf("expected error containing %q, got nil", tt.errContains)
					return
				}
				if tt.errContains != "" && !strings.Contains(err.Error(), tt.errContains) {
					t.Errorf("error %q does not contain %q", err.Error(), tt.errContains)
				}
				return
			}

			if err != nil {
				t.Errorf("unexpected error: %v", err)
			}
		})
	}
}

// TestProjectModuleAccess tests that the project module is accessible.
func TestProjectModuleAccess(t *testing.T) {
	// Create a temp project directory with MODULE.bazel
	tmpDir := t.TempDir()
	modulePath := filepath.Join(tmpDir, "MODULE.bazel")
	err := os.WriteFile(modulePath, []byte(`
module(
    name = "test_project",
    version = "1.0.0",
    bazel_compatibility = [">=7.0.0"],
)

bazel_dep(name = "rules_cc", version = "0.1.0")
`), 0644)
	if err != nil {
		t.Fatalf("failed to create MODULE.bazel: %v", err)
	}

	// Create .bazelversion
	bazelVersionPath := filepath.Join(tmpDir, ".bazelversion")
	err = os.WriteFile(bazelVersionPath, []byte("8.0.0"), 0644)
	if err != nil {
		t.Fatalf("failed to create .bazelversion: %v", err)
	}

	tests := []struct {
		name        string
		script      string
		wantErr     bool
		errContains string
	}{
		{
			name: "project.context",
			script: `
def check(ctx):
    p = project.context()
    if not p.use_bzlmod:
        fail(msg="expected use_bzlmod=True")
    if p.module_name != "test_project":
        fail(msg="expected module_name=test_project, got " + p.module_name)
    return []

recipe(name="test", version="1.0.0", steps=[scan(id="s1", impl=check)])
`,
		},
		{
			name: "project.deps",
			script: `
def check(ctx):
    deps = project.deps()
    if len(deps) == 0:
        fail(msg="expected at least one dep")
    found = False
    for dep in deps:
        if dep["name"] == "rules_cc":
            found = True
            if dep["version"] != "0.1.0":
                fail(msg="expected version=0.1.0")
    if not found:
        fail(msg="rules_cc dep not found")
    return []

recipe(name="test", version="1.0.0", steps=[scan(id="s1", impl=check)])
`,
		},
		{
			name: "project.compatibility",
			script: `
def check(ctx):
    compat = project.compatibility()
    if len(compat) == 0:
        fail(msg="expected compatibility constraints")
    if ">=7.0.0" not in compat:
        fail(msg="expected >=7.0.0 constraint")
    return []

recipe(name="test", version="1.0.0", steps=[scan(id="s1", impl=check)])
`,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Clear the cache before each test
			ClearProjectContextCache()

			interp := NewInterpreter(WithProjectPath(tmpDir))
			_, err := interp.LoadString("test.star", tt.script)

			if tt.wantErr {
				if err == nil {
					t.Errorf("expected error containing %q, got nil", tt.errContains)
					return
				}
				if tt.errContains != "" && !strings.Contains(err.Error(), tt.errContains) {
					t.Errorf("error %q does not contain %q", err.Error(), tt.errContains)
				}
				return
			}

			if err != nil {
				t.Errorf("unexpected error: %v", err)
			}
		})
	}
}

// TestFsModuleAccess tests that the fs module is accessible.
func TestFsModuleAccess(t *testing.T) {
	// Create a temp project directory with test files
	tmpDir := t.TempDir()

	// Create test file
	testFilePath := filepath.Join(tmpDir, "test.txt")
	err := os.WriteFile(testFilePath, []byte("hello world"), 0644)
	if err != nil {
		t.Fatalf("failed to create test file: %v", err)
	}

	// Create BUILD file
	buildPath := filepath.Join(tmpDir, "BUILD")
	err = os.WriteFile(buildPath, []byte(`
load("@rules_cc//cc:defs.bzl", "cc_library")

cc_library(
    name = "mylib",
    srcs = ["mylib.cc"],
)
`), 0644)
	if err != nil {
		t.Fatalf("failed to create BUILD file: %v", err)
	}

	tests := []struct {
		name        string
		script      string
		wantErr     bool
		errContains string
	}{
		{
			name: "fs.read",
			script: `
def check(ctx):
    content = fs.read("test.txt")
    if content != "hello world":
        fail(msg="fs.read failed, got: " + content)
    return []

recipe(name="test", version="1.0.0", steps=[scan(id="s1", impl=check)])
`,
		},
		{
			name: "fs.read with default",
			script: `
def check(ctx):
    content = fs.read("nonexistent.txt", default="default value")
    if content != "default value":
        fail(msg="fs.read default failed")
    return []

recipe(name="test", version="1.0.0", steps=[scan(id="s1", impl=check)])
`,
		},
		{
			name: "fs.exists true",
			script: `
def check(ctx):
    if not fs.exists("test.txt"):
        fail(msg="fs.exists should return True")
    return []

recipe(name="test", version="1.0.0", steps=[scan(id="s1", impl=check)])
`,
		},
		{
			name: "fs.exists false",
			script: `
def check(ctx):
    if fs.exists("nonexistent.txt"):
        fail(msg="fs.exists should return False")
    return []

recipe(name="test", version="1.0.0", steps=[scan(id="s1", impl=check)])
`,
		},
		{
			name: "fs.glob",
			script: `
def check(ctx):
    files = fs.glob("*.txt")
    if len(files) == 0:
        fail(msg="fs.glob should find test.txt")
    return []

recipe(name="test", version="1.0.0", steps=[scan(id="s1", impl=check)])
`,
		},
		{
			name: "fs.list_dir",
			script: `
def check(ctx):
    entries = fs.list_dir(".")
    if "test.txt" not in entries:
        fail(msg="fs.list_dir should contain test.txt")
    return []

recipe(name="test", version="1.0.0", steps=[scan(id="s1", impl=check)])
`,
		},
		{
			name: "fs.parse_build",
			script: `
def check(ctx):
    bf = fs.parse_build("BUILD")
    rules = bf.rules()
    if len(rules) == 0:
        fail(msg="fs.parse_build should find rules")
    found = False
    for rule in rules:
        if rule.name == "mylib":
            found = True
    if not found:
        fail(msg="should find mylib rule")
    return []

recipe(name="test", version="1.0.0", steps=[scan(id="s1", impl=check)])
`,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			interp := NewInterpreter(WithProjectPath(tmpDir))
			_, err := interp.LoadString("test.star", tt.script)

			if tt.wantErr {
				if err == nil {
					t.Errorf("expected error containing %q, got nil", tt.errContains)
					return
				}
				if tt.errContains != "" && !strings.Contains(err.Error(), tt.errContains) {
					t.Errorf("error %q does not contain %q", err.Error(), tt.errContains)
				}
				return
			}

			if err != nil {
				t.Errorf("unexpected error: %v", err)
			}
		})
	}
}

// TestDeprecatedBuiltinsStillWork verifies that old deprecated builtins still work.
func TestDeprecatedBuiltinsStillWork(t *testing.T) {
	// Create a temp project directory
	tmpDir := t.TempDir()

	// Create test file
	testFilePath := filepath.Join(tmpDir, "test.txt")
	err := os.WriteFile(testFilePath, []byte("hello"), 0644)
	if err != nil {
		t.Fatalf("failed to create test file: %v", err)
	}

	// Create MODULE.bazel
	modulePath := filepath.Join(tmpDir, "MODULE.bazel")
	err = os.WriteFile(modulePath, []byte(`module(name = "test")`), 0644)
	if err != nil {
		t.Fatalf("failed to create MODULE.bazel: %v", err)
	}

	tests := []struct {
		name   string
		script string
	}{
		{
			name: "read_file deprecated",
			script: `
def check(ctx):
    content = read_file("test.txt")
    return []

recipe(name="test", version="1.0.0", steps=[scan(id="s1", impl=check)])
`,
		},
		{
			name: "file_exists deprecated",
			script: `
def check(ctx):
    result = file_exists("test.txt")
    return []

recipe(name="test", version="1.0.0", steps=[scan(id="s1", impl=check)])
`,
		},
		{
			name: "version_matches deprecated",
			script: `
def check(ctx):
    result = version_matches("7.0.0", [">=6.0.0"])
    return []

recipe(name="test", version="1.0.0", steps=[scan(id="s1", impl=check)])
`,
		},
		{
			name: "parse_version deprecated",
			script: `
def check(ctx):
    v = parse_version("7.0.0")
    return []

recipe(name="test", version="1.0.0", steps=[scan(id="s1", impl=check)])
`,
		},
		{
			name: "get_project_context deprecated",
			script: `
def check(ctx):
    p = get_project_context()
    return []

recipe(name="test", version="1.0.0", steps=[scan(id="s1", impl=check)])
`,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Clear cache between tests
			ClearProjectContextCache()

			interp := NewInterpreter(WithProjectPath(tmpDir))
			_, err := interp.LoadString("test.star", tt.script)
			if err != nil {
				t.Errorf("deprecated builtin should still work: %v", err)
			}
		})
	}
}

// TestFlagsModuleWithoutDB tests that flags module returns errors without DB.
func TestFlagsModuleWithoutDB(t *testing.T) {
	script := `
result = flags.exists("test_flag", "8.0.0")
recipe(name="test", version="1.0.0", steps=[scan(id="s1", files=["*"], patterns=["test"])])
`

	interp := NewInterpreter()
	_, err := interp.LoadString("test.star", script)
	if err == nil {
		t.Errorf("expected error when using flags module without DB")
	}
	if !strings.Contains(err.Error(), "flag database") {
		t.Errorf("error should mention flag database: %v", err)
	}
}

// TestModuleIntegration tests using multiple modules together.
func TestModuleIntegration(t *testing.T) {
	// Create a temp project directory
	tmpDir := t.TempDir()

	// Create MODULE.bazel
	modulePath := filepath.Join(tmpDir, "MODULE.bazel")
	err := os.WriteFile(modulePath, []byte(`
module(
    name = "integration_test",
    version = "1.0.0",
    bazel_compatibility = [">=7.0.0", "<=9.0.0"],
)
`), 0644)
	if err != nil {
		t.Fatalf("failed to create MODULE.bazel: %v", err)
	}

	// Create .bazelversion
	err = os.WriteFile(filepath.Join(tmpDir, ".bazelversion"), []byte("8.0.0"), 0644)
	if err != nil {
		t.Fatalf("failed to create .bazelversion: %v", err)
	}

	script := `
def check_all(ctx):
    # Use project module
    p = project.context()
    if not p.use_bzlmod:
        fail(msg="expected bzlmod")

    # Use version module with project context
    compat = project.compatibility()
    if not version.matches(p.bazel_version, compat):
        fail(msg="version should match compatibility")

    # Use fs module
    if not fs.exists("MODULE.bazel"):
        fail(msg="MODULE.bazel should exist")

    content = fs.read("MODULE.bazel")
    if "integration_test" not in content:
        fail(msg="module name should be in content")

    # Check version is at least 7.0.0
    if not version.at_least(p.bazel_version, "7.0.0"):
        fail(msg="should be at least 7.0.0")

    # Parse version
    v = version.parse(p.bazel_version)
    if v.major != 8:
        fail(msg="expected major version 8")

    return []

recipe(name="test", version="1.0.0", steps=[scan(id="s1", impl=check_all)])
`

	ClearProjectContextCache()
	interp := NewInterpreter(WithProjectPath(tmpDir))
	_, err = interp.LoadString("test.star", script)
	if err != nil {
		t.Errorf("integration test failed: %v", err)
	}
}
