// Copyright 2025 bazelbump authors
// SPDX-License-Identifier: Apache-2.0

package starlark

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestGetProjectContext(t *testing.T) {
	// Clear cache before test
	ClearProjectContextCache()
	defer ClearProjectContextCache()

	// Create temp directory with project files
	tempDir := t.TempDir()

	// Create MODULE.bazel
	moduleContent := `
module(
    name = "test_project",
    version = "1.0.0",
    bazel_compatibility = [">=7.0.0"],
)

bazel_dep(name = "rules_cc", version = "0.0.17")
`
	if err := os.WriteFile(filepath.Join(tempDir, "MODULE.bazel"), []byte(moduleContent), 0o644); err != nil {
		t.Fatalf("failed to create MODULE.bazel: %v", err)
	}

	// Create .bazelversion
	if err := os.WriteFile(filepath.Join(tempDir, ".bazelversion"), []byte("7.5.0\n"), 0o644); err != nil {
		t.Fatalf("failed to create .bazelversion: %v", err)
	}

	tests := []struct {
		name        string
		script      string
		wantErr     bool
		errContains string
	}{
		{
			name: "get project context basic",
			script: `
ctx = get_project_context()
# Just verify we can access attributes without error
root = ctx.root
bazel_version = ctx.bazel_version
use_bzlmod = ctx.use_bzlmod
use_workspace = ctx.use_workspace
module_name = ctx.module_name
module_version = ctx.module_version
recipe(name="test", version="1.0.0", steps=[
    scan(id="s1", files=["*"], patterns=["test"]),
])
`,
		},
		{
			name: "verify bzlmod detection with custom impl",
			script: `
def check_bzlmod(ctx):
    project_ctx = get_project_context()
    if not project_ctx.use_bzlmod:
        fail(msg="Expected use_bzlmod to be True")
    return []

recipe(name="test", version="1.0.0", steps=[
    scan(id="s1", impl=check_bzlmod),
])
`,
		},
		{
			name: "verify module name with custom impl",
			script: `
def check_module_name(ctx):
    project_ctx = get_project_context()
    if project_ctx.module_name != "test_project":
        fail(msg="Expected module_name to be 'test_project', got " + project_ctx.module_name)
    return []

recipe(name="test", version="1.0.0", steps=[
    scan(id="s1", impl=check_module_name),
])
`,
		},
		{
			name: "verify bazel version with custom impl",
			script: `
def check_bazel_version(ctx):
    project_ctx = get_project_context()
    if project_ctx.bazel_version != "7.5.0":
        fail(msg="Expected bazel_version to be '7.5.0', got " + project_ctx.bazel_version)
    return []

recipe(name="test", version="1.0.0", steps=[
    scan(id="s1", impl=check_bazel_version),
])
`,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Clear cache for each test
			ClearProjectContextCache()

			interp := NewInterpreter(WithProjectPath(tempDir))
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

func TestGetProjectContextNoProjectPath(t *testing.T) {
	ClearProjectContextCache()
	defer ClearProjectContextCache()

	script := `
ctx = get_project_context()
recipe(name="test", version="1.0.0", steps=[
    scan(id="s1", files=["*"], patterns=["test"]),
])
`
	interp := NewInterpreter() // No project path
	_, err := interp.LoadString("test.star", script)

	if err == nil {
		t.Error("expected error for missing project path, got nil")
		return
	}
	if !strings.Contains(err.Error(), "project path") {
		t.Errorf("error %q should mention project path", err.Error())
	}
}

func TestGetBazelDeps(t *testing.T) {
	ClearProjectContextCache()
	defer ClearProjectContextCache()

	tempDir := t.TempDir()

	moduleContent := `
module(name = "test")

bazel_dep(name = "rules_cc", version = "0.0.17")
bazel_dep(name = "rules_go", version = "0.50.0", repo_name = "io_bazel_rules_go")
bazel_dep(name = "gazelle", version = "0.39.0", dev_dependency = True)
`
	if err := os.WriteFile(filepath.Join(tempDir, "MODULE.bazel"), []byte(moduleContent), 0o644); err != nil {
		t.Fatalf("failed to create MODULE.bazel: %v", err)
	}

	tests := []struct {
		name        string
		script      string
		wantErr     bool
		errContains string
	}{
		{
			name: "get bazel deps list",
			script: `
def check_deps(ctx):
    deps = get_bazel_deps()
    if len(deps) != 3:
        fail(msg="Expected 3 deps, got " + str(len(deps)))
    return []

recipe(name="test", version="1.0.0", steps=[
    scan(id="s1", impl=check_deps),
])
`,
		},
		{
			name: "access dep fields",
			script: `
def check_rules_cc(ctx):
    deps = get_bazel_deps()
    found_rules_cc = False
    for dep in deps:
        if dep["name"] == "rules_cc":
            found_rules_cc = True
            if dep["version"] != "0.0.17":
                fail(msg="rules_cc version mismatch")
    if not found_rules_cc:
        fail(msg="rules_cc not found")
    return []

recipe(name="test", version="1.0.0", steps=[
    scan(id="s1", impl=check_rules_cc),
])
`,
		},
		{
			name: "check repo_name",
			script: `
def check_repo_name(ctx):
    deps = get_bazel_deps()
    for dep in deps:
        if dep["name"] == "rules_go":
            if dep["repo_name"] != "io_bazel_rules_go":
                fail(msg="rules_go repo_name mismatch: " + dep["repo_name"])
    return []

recipe(name="test", version="1.0.0", steps=[
    scan(id="s1", impl=check_repo_name),
])
`,
		},
		{
			name: "check dev_dependency",
			script: `
def check_dev_dep(ctx):
    deps = get_bazel_deps()
    for dep in deps:
        if dep["name"] == "gazelle":
            if not dep["dev_dependency"]:
                fail(msg="gazelle should be dev_dependency")
        if dep["name"] == "rules_cc":
            if dep["dev_dependency"]:
                fail(msg="rules_cc should not be dev_dependency")
    return []

recipe(name="test", version="1.0.0", steps=[
    scan(id="s1", impl=check_dev_dep),
])
`,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Clear cache for each test
			ClearProjectContextCache()

			interp := NewInterpreter(WithProjectPath(tempDir))
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

func TestGetBazelCompatibility(t *testing.T) {
	ClearProjectContextCache()
	defer ClearProjectContextCache()

	tempDir := t.TempDir()

	moduleContent := `
module(
    name = "test",
    bazel_compatibility = [">=6.4.0", "<=9.0.0", "-7.1.0"],
)
`
	if err := os.WriteFile(filepath.Join(tempDir, "MODULE.bazel"), []byte(moduleContent), 0o644); err != nil {
		t.Fatalf("failed to create MODULE.bazel: %v", err)
	}

	tests := []struct {
		name        string
		script      string
		wantErr     bool
		errContains string
	}{
		{
			name: "get compatibility list",
			script: `
def check_compat(ctx):
    compat = get_bazel_compatibility()
    if len(compat) != 3:
        fail(msg="Expected 3 constraints, got " + str(len(compat)))
    return []

recipe(name="test", version="1.0.0", steps=[
    scan(id="s1", impl=check_compat),
])
`,
		},
		{
			name: "check constraint values",
			script: `
def check_values(ctx):
    compat = get_bazel_compatibility()
    if compat[0] != ">=6.4.0":
        fail(msg="First constraint mismatch: " + compat[0])
    if compat[1] != "<=9.0.0":
        fail(msg="Second constraint mismatch: " + compat[1])
    if compat[2] != "-7.1.0":
        fail(msg="Third constraint mismatch: " + compat[2])
    return []

recipe(name="test", version="1.0.0", steps=[
    scan(id="s1", impl=check_values),
])
`,
		},
		{
			name: "use with version_matches",
			script: `
def check_matches(ctx):
    compat = get_bazel_compatibility()
    if not version_matches("8.0.0", compat):
        fail(msg="8.0.0 should match constraints")
    if version_matches("7.1.0", compat):
        fail(msg="7.1.0 should be excluded")
    return []

recipe(name="test", version="1.0.0", steps=[
    scan(id="s1", impl=check_matches),
])
`,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ClearProjectContextCache()

			interp := NewInterpreter(WithProjectPath(tempDir))
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

func TestGetBazelCompatibilityEmpty(t *testing.T) {
	ClearProjectContextCache()
	defer ClearProjectContextCache()

	tempDir := t.TempDir()

	// Module without bazel_compatibility
	moduleContent := `module(name = "test")`
	if err := os.WriteFile(filepath.Join(tempDir, "MODULE.bazel"), []byte(moduleContent), 0o644); err != nil {
		t.Fatalf("failed to create MODULE.bazel: %v", err)
	}

	script := `
def check_empty(ctx):
    compat = get_bazel_compatibility()
    if len(compat) != 0:
        fail(msg="Expected empty list, got " + str(len(compat)))
    return []

recipe(name="test", version="1.0.0", steps=[
    scan(id="s1", impl=check_empty),
])
`
	interp := NewInterpreter(WithProjectPath(tempDir))
	_, err := interp.LoadString("test.star", script)
	if err != nil {
		t.Errorf("unexpected error: %v", err)
	}
}
