// Copyright 2025 bazelbump authors
// SPDX-License-Identifier: Apache-2.0

package starlark

import (
	"os"
	"path/filepath"
	"testing"
)

func TestParseBuildFileContent_Rules(t *testing.T) {
	content := `
load("@rules_cc//cc:defs.bzl", "cc_binary", "cc_library")

cc_binary(
    name = "main",
    srcs = ["main.cc"],
    deps = [":lib"],
)

cc_library(
    name = "lib",
    srcs = ["lib.cc"],
    hdrs = ["lib.h"],
)
`

	bf, err := ParseBuildFileContent("BUILD", content)
	if err != nil {
		t.Fatalf("ParseBuildFileContent failed: %v", err)
	}

	if len(bf.ParsedRules) != 2 {
		t.Fatalf("expected 2 rules, got %d", len(bf.ParsedRules))
	}

	// Check first rule
	if bf.ParsedRules[0].Kind != "cc_binary" {
		t.Errorf("expected first rule kind 'cc_binary', got %q", bf.ParsedRules[0].Kind)
	}
	if bf.ParsedRules[0].Name != "main" {
		t.Errorf("expected first rule name 'main', got %q", bf.ParsedRules[0].Name)
	}

	// Check second rule
	if bf.ParsedRules[1].Kind != "cc_library" {
		t.Errorf("expected second rule kind 'cc_library', got %q", bf.ParsedRules[1].Kind)
	}
	if bf.ParsedRules[1].Name != "lib" {
		t.Errorf("expected second rule name 'lib', got %q", bf.ParsedRules[1].Name)
	}
}

func TestParseBuildFileContent_Loads(t *testing.T) {
	content := `
load("@rules_cc//cc:defs.bzl", "cc_binary", "cc_library")
load("@rules_python//python:defs.bzl", "py_binary")

cc_binary(
    name = "main",
)
`

	bf, err := ParseBuildFileContent("BUILD", content)
	if err != nil {
		t.Fatalf("ParseBuildFileContent failed: %v", err)
	}

	if len(bf.ParsedLoads) != 2 {
		t.Fatalf("expected 2 load statements, got %d", len(bf.ParsedLoads))
	}

	// Check first load
	if bf.ParsedLoads[0].Path != "@rules_cc//cc:defs.bzl" {
		t.Errorf("expected first load path '@rules_cc//cc:defs.bzl', got %q", bf.ParsedLoads[0].Path)
	}
	if len(bf.ParsedLoads[0].Symbols) != 2 {
		t.Errorf("expected 2 symbols in first load, got %d", len(bf.ParsedLoads[0].Symbols))
	}

	// Check second load
	if bf.ParsedLoads[1].Path != "@rules_python//python:defs.bzl" {
		t.Errorf("expected second load path '@rules_python//python:defs.bzl', got %q", bf.ParsedLoads[1].Path)
	}
	if len(bf.ParsedLoads[1].Symbols) != 1 {
		t.Errorf("expected 1 symbol in second load, got %d", len(bf.ParsedLoads[1].Symbols))
	}
}

func TestParseBuildFileContent_RuleAttributes(t *testing.T) {
	content := `
cc_binary(
    name = "main",
    visibility = "//visibility:public",
)
`

	bf, err := ParseBuildFileContent("BUILD", content)
	if err != nil {
		t.Fatalf("ParseBuildFileContent failed: %v", err)
	}

	if len(bf.ParsedRules) != 1 {
		t.Fatalf("expected 1 rule, got %d", len(bf.ParsedRules))
	}

	rule := bf.ParsedRules[0]
	if rule.Attributes["name"] != "main" {
		t.Errorf("expected name attribute 'main', got %q", rule.Attributes["name"])
	}
	if rule.Attributes["visibility"] != "//visibility:public" {
		t.Errorf("expected visibility attribute '//visibility:public', got %q", rule.Attributes["visibility"])
	}
}

func TestParseBuildFileContent_LineNumbers(t *testing.T) {
	content := `load("@rules_cc//cc:defs.bzl", "cc_binary")

cc_binary(
    name = "main",
)
`

	bf, err := ParseBuildFileContent("BUILD", content)
	if err != nil {
		t.Fatalf("ParseBuildFileContent failed: %v", err)
	}

	if len(bf.ParsedLoads) != 1 {
		t.Fatalf("expected 1 load statement, got %d", len(bf.ParsedLoads))
	}
	if bf.ParsedLoads[0].Line != 1 {
		t.Errorf("expected load on line 1, got %d", bf.ParsedLoads[0].Line)
	}

	if len(bf.ParsedRules) != 1 {
		t.Fatalf("expected 1 rule, got %d", len(bf.ParsedRules))
	}
	if bf.ParsedRules[0].Line != 3 {
		t.Errorf("expected rule on line 3, got %d", bf.ParsedRules[0].Line)
	}
}

func TestMatchKind(t *testing.T) {
	tests := []struct {
		kind    string
		pattern string
		want    bool
	}{
		{"cc_binary", "cc_binary", true},
		{"cc_binary", "cc_library", false},
		{"cc_binary", "cc_*", true},
		{"cc_library", "cc_*", true},
		{"py_binary", "cc_*", false},
		{"cc_binary", "*", true},
		{"cc_binary", "*_binary", false}, // Only suffix wildcards supported
	}

	for _, tc := range tests {
		t.Run(tc.kind+"_"+tc.pattern, func(t *testing.T) {
			got := matchKind(tc.kind, tc.pattern)
			if got != tc.want {
				t.Errorf("matchKind(%q, %q) = %v, want %v", tc.kind, tc.pattern, got, tc.want)
			}
		})
	}
}

func TestParseBuildFile_Integration(t *testing.T) {
	// Create a temporary BUILD file
	tmpDir, err := os.MkdirTemp("", "bazelbump-buildfile-test-*")
	if err != nil {
		t.Fatal(err)
	}
	defer os.RemoveAll(tmpDir)

	buildContent := `
load("@rules_cc//cc:defs.bzl", "cc_binary", "cc_library", "cc_test")

cc_library(
    name = "lib",
    srcs = ["lib.cc"],
    hdrs = ["lib.h"],
)

cc_binary(
    name = "main",
    srcs = ["main.cc"],
    deps = [":lib"],
)

cc_test(
    name = "lib_test",
    srcs = ["lib_test.cc"],
    deps = [":lib"],
)
`

	buildPath := filepath.Join(tmpDir, "BUILD")
	if err := os.WriteFile(buildPath, []byte(buildContent), 0644); err != nil {
		t.Fatal(err)
	}

	bf, err := ParseBuildFile(buildPath)
	if err != nil {
		t.Fatalf("ParseBuildFile failed: %v", err)
	}

	// Verify load statement
	if len(bf.ParsedLoads) != 1 {
		t.Errorf("expected 1 load statement, got %d", len(bf.ParsedLoads))
	}

	// Verify rules
	if len(bf.ParsedRules) != 3 {
		t.Errorf("expected 3 rules, got %d", len(bf.ParsedRules))
	}
}

// TestParseBuildFile_Starlark tests the parse_build_file builtin from Starlark
func TestParseBuildFile_Starlark(t *testing.T) {
	// Create a temporary directory with a BUILD file
	tmpDir, err := os.MkdirTemp("", "bazelbump-starlark-test-*")
	if err != nil {
		t.Fatal(err)
	}
	defer os.RemoveAll(tmpDir)

	buildContent := `
load("@rules_cc//cc:defs.bzl", "cc_binary", "cc_library")

cc_library(
    name = "mylib",
    srcs = ["lib.cc"],
)

cc_binary(
    name = "myapp",
    srcs = ["main.cc"],
    deps = [":mylib"],
)

py_binary(
    name = "tool",
    srcs = ["tool.py"],
)
`

	buildPath := filepath.Join(tmpDir, "BUILD")
	if err := os.WriteFile(buildPath, []byte(buildContent), 0644); err != nil {
		t.Fatal(err)
	}

	// Create Starlark recipe that uses parse_build_file
	recipeContent := `
# Parse the BUILD file
build = parse_build_file("BUILD")

# Get all rules
all_rules = build.rules()

# Get only cc_* rules
cc_rules = build.rules(kind="cc_*")

# Get load statements
loads = build.loads()

# Access rule properties
first_rule = all_rules[0]
first_rule_name = first_rule.name
first_rule_kind = first_rule.kind

# Access load properties
first_load = loads[0]
load_path = first_load.path
load_symbols = first_load.symbols

recipe(
    name = "parse-test",
    version = "1.0.0",
    steps = [
        edit(id = "dummy", operations = []),
    ],
)
`

	recipePath := filepath.Join(tmpDir, "recipe.star")
	if err := os.WriteFile(recipePath, []byte(recipeContent), 0644); err != nil {
		t.Fatal(err)
	}

	// Create interpreter with project path set
	interp := NewInterpreter(WithProjectPath(tmpDir))
	_, err = interp.LoadFile(recipePath)
	if err != nil {
		t.Fatalf("LoadFile failed: %v", err)
	}
}

func TestParseBuildFile_RulesFiltering(t *testing.T) {
	// Create a temporary directory with a BUILD file
	tmpDir, err := os.MkdirTemp("", "bazelbump-filter-test-*")
	if err != nil {
		t.Fatal(err)
	}
	defer os.RemoveAll(tmpDir)

	buildContent := `
cc_library(name = "lib1")
cc_binary(name = "bin1")
cc_test(name = "test1")
py_binary(name = "pybin1")
py_library(name = "pylib1")
`

	buildPath := filepath.Join(tmpDir, "BUILD")
	if err := os.WriteFile(buildPath, []byte(buildContent), 0644); err != nil {
		t.Fatal(err)
	}

	// Test exact kind matching
	recipeContent := `
build = parse_build_file("BUILD")

# Filter by exact kind
cc_binary_rules = build.rules(kind="cc_binary")

# Filter by pattern
cc_rules = build.rules(kind="cc_*")
py_rules = build.rules(kind="py_*")

# All rules
all_rules = build.rules()

recipe(
    name = "filter-test",
    version = "1.0.0",
    steps = [
        edit(id = "dummy", operations = []),
    ],
)
`

	recipePath := filepath.Join(tmpDir, "recipe.star")
	if err := os.WriteFile(recipePath, []byte(recipeContent), 0644); err != nil {
		t.Fatal(err)
	}

	interp := NewInterpreter(WithProjectPath(tmpDir))
	_, err = interp.LoadFile(recipePath)
	if err != nil {
		t.Fatalf("LoadFile failed: %v", err)
	}
}

func TestParseBuildFile_RuleAttrMethod(t *testing.T) {
	// Create a temporary directory with a BUILD file
	tmpDir, err := os.MkdirTemp("", "bazelbump-attr-test-*")
	if err != nil {
		t.Fatal(err)
	}
	defer os.RemoveAll(tmpDir)

	buildContent := `
cc_binary(
    name = "myapp",
    visibility = "//visibility:public",
)
`

	buildPath := filepath.Join(tmpDir, "BUILD")
	if err := os.WriteFile(buildPath, []byte(buildContent), 0644); err != nil {
		t.Fatal(err)
	}

	// Test accessing attributes via attr() method
	recipeContent := `
build = parse_build_file("BUILD")
rules = build.rules()
rule = rules[0]

# Access name via property
name = rule.name

# Access via attr() method
visibility = rule.attr("visibility")
nonexistent = rule.attr("nonexistent")

recipe(
    name = "attr-test",
    version = "1.0.0",
    steps = [
        edit(id = "dummy", operations = []),
    ],
)
`

	recipePath := filepath.Join(tmpDir, "recipe.star")
	if err := os.WriteFile(recipePath, []byte(recipeContent), 0644); err != nil {
		t.Fatal(err)
	}

	interp := NewInterpreter(WithProjectPath(tmpDir))
	_, err = interp.LoadFile(recipePath)
	if err != nil {
		t.Fatalf("LoadFile failed: %v", err)
	}
}

func TestParseBuildFile_LoadSymbols(t *testing.T) {
	// Create a temporary directory with a BUILD file
	tmpDir, err := os.MkdirTemp("", "bazelbump-load-test-*")
	if err != nil {
		t.Fatal(err)
	}
	defer os.RemoveAll(tmpDir)

	buildContent := `
load("@rules_cc//cc:defs.bzl", "cc_binary", "cc_library")

cc_binary(name = "main")
`

	buildPath := filepath.Join(tmpDir, "BUILD")
	if err := os.WriteFile(buildPath, []byte(buildContent), 0644); err != nil {
		t.Fatal(err)
	}

	// Test accessing load symbols
	recipeContent := `
build = parse_build_file("BUILD")
loads = build.loads()
first_load = loads[0]

# Access load properties
path = first_load.path
symbols = first_load.symbols

recipe(
    name = "load-test",
    version = "1.0.0",
    steps = [
        edit(id = "dummy", operations = []),
    ],
)
`

	recipePath := filepath.Join(tmpDir, "recipe.star")
	if err := os.WriteFile(recipePath, []byte(recipeContent), 0644); err != nil {
		t.Fatal(err)
	}

	interp := NewInterpreter(WithProjectPath(tmpDir))
	_, err = interp.LoadFile(recipePath)
	if err != nil {
		t.Fatalf("LoadFile failed: %v", err)
	}
}

func TestParseBuildFile_EmptyFile(t *testing.T) {
	content := ""

	bf, err := ParseBuildFileContent("BUILD", content)
	if err != nil {
		t.Fatalf("ParseBuildFileContent failed: %v", err)
	}

	if len(bf.ParsedRules) != 0 {
		t.Errorf("expected 0 rules, got %d", len(bf.ParsedRules))
	}

	if len(bf.ParsedLoads) != 0 {
		t.Errorf("expected 0 loads, got %d", len(bf.ParsedLoads))
	}
}

func TestParseBuildFile_CommentsOnly(t *testing.T) {
	content := `
# This is a comment
# Another comment
`

	bf, err := ParseBuildFileContent("BUILD", content)
	if err != nil {
		t.Fatalf("ParseBuildFileContent failed: %v", err)
	}

	if len(bf.ParsedRules) != 0 {
		t.Errorf("expected 0 rules, got %d", len(bf.ParsedRules))
	}

	if len(bf.ParsedLoads) != 0 {
		t.Errorf("expected 0 loads, got %d", len(bf.ParsedLoads))
	}
}

func TestParseBuildFile_NestedParens(t *testing.T) {
	content := `
cc_binary(
    name = "main",
    deps = select({
        "//conditions:default": [":lib"],
        ":debug": [":debug_lib"],
    }),
)
`

	bf, err := ParseBuildFileContent("BUILD", content)
	if err != nil {
		t.Fatalf("ParseBuildFileContent failed: %v", err)
	}

	if len(bf.ParsedRules) != 1 {
		t.Fatalf("expected 1 rule, got %d", len(bf.ParsedRules))
	}

	if bf.ParsedRules[0].Name != "main" {
		t.Errorf("expected rule name 'main', got %q", bf.ParsedRules[0].Name)
	}
}

func TestParseBuildFile_MultipleLoadsOnSameLine(t *testing.T) {
	// This is unusual but should still parse correctly
	content := `load("@rules_cc//cc:defs.bzl", "cc_binary")
load("@rules_python//python:defs.bzl", "py_binary")

cc_binary(name = "main")
`

	bf, err := ParseBuildFileContent("BUILD", content)
	if err != nil {
		t.Fatalf("ParseBuildFileContent failed: %v", err)
	}

	if len(bf.ParsedLoads) != 2 {
		t.Errorf("expected 2 load statements, got %d", len(bf.ParsedLoads))
	}
}

func TestParseBuildFile_AliasedLoads(t *testing.T) {
	content := `
load("@rules_cc//cc:defs.bzl", cc_bin = "cc_binary", "cc_library")

cc_bin(name = "main")
`

	bf, err := ParseBuildFileContent("BUILD", content)
	if err != nil {
		t.Fatalf("ParseBuildFileContent failed: %v", err)
	}

	if len(bf.ParsedLoads) != 1 {
		t.Fatalf("expected 1 load statement, got %d", len(bf.ParsedLoads))
	}

	// Both the alias and the original name should be captured
	symbols := bf.ParsedLoads[0].Symbols
	if len(symbols) != 2 {
		t.Errorf("expected 2 symbols, got %d: %v", len(symbols), symbols)
	}
}

func TestParseBuildFile_VariousRuleTypes(t *testing.T) {
	content := `
genrule(
    name = "gen_file",
    outs = ["out.txt"],
    cmd = "echo hello > $@",
)

filegroup(
    name = "srcs",
    srcs = glob(["**/*.cc"]),
)

alias(
    name = "main_alias",
    actual = ":main",
)

config_setting(
    name = "debug",
    values = {"compilation_mode": "dbg"},
)
`

	bf, err := ParseBuildFileContent("BUILD", content)
	if err != nil {
		t.Fatalf("ParseBuildFileContent failed: %v", err)
	}

	// Should find genrule, filegroup, alias, and config_setting
	if len(bf.ParsedRules) != 4 {
		t.Errorf("expected 4 rules, got %d", len(bf.ParsedRules))
	}

	expectedKinds := map[string]bool{
		"genrule":        false,
		"filegroup":      false,
		"alias":          false,
		"config_setting": false,
	}

	for _, rule := range bf.ParsedRules {
		if _, ok := expectedKinds[rule.Kind]; ok {
			expectedKinds[rule.Kind] = true
		}
	}

	for kind, found := range expectedKinds {
		if !found {
			t.Errorf("expected to find rule kind %q", kind)
		}
	}
}
