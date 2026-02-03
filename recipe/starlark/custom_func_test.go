// Copyright 2025 bazelbump authors
// SPDX-License-Identifier: Apache-2.0

package starlark

import (
	"testing"

	"go.starlark.net/starlark"
)

func TestCustomScanFunction(t *testing.T) {
	recipeCode := `
def my_scanner(ctx):
    return [
        {"file": "BUILD", "line": 1, "text": "cc_binary", "rule": "cc_binary"},
    ]

recipe(
    name = "test",
    version = "1.0.0",
    steps = [
        scan(
            id = "find_rules",
            name = "Find rules with custom scanner",
            impl = my_scanner,
        ),
    ],
)
`
	interp := NewInterpreter()
	recipe, err := interp.LoadString("test.star", recipeCode)
	if err != nil {
		t.Fatalf("LoadString failed: %v", err)
	}

	if len(recipe.Steps) != 1 {
		t.Fatalf("expected 1 step, got %d", len(recipe.Steps))
	}

	scanStep, ok := recipe.Steps[0].(*ScanStep)
	if !ok {
		t.Fatalf("expected ScanStep, got %T", recipe.Steps[0])
	}

	if scanStep.CustomImpl == nil {
		t.Error("expected CustomImpl to be set")
	}

	// Verify it's a callable
	if _, ok := scanStep.CustomImpl.(starlark.Callable); !ok {
		t.Errorf("expected CustomImpl to be Callable, got %T", scanStep.CustomImpl)
	}
}

func TestCustomTransformFunction(t *testing.T) {
	recipeCode := `
def dynamic_ops(ctx):
    ops = []
    if ctx.dry_run:
        ops.append(ensure_load("cc_test", "@rules_cc//cc:defs.bzl"))
    return ops

recipe(
    name = "test",
    version = "1.0.0",
    steps = [
        edit(
            id = "add_loads",
            name = "Add load statements",
            operations = dynamic_ops,
        ),
    ],
)
`
	interp := NewInterpreter()
	recipe, err := interp.LoadString("test.star", recipeCode)
	if err != nil {
		t.Fatalf("LoadString failed: %v", err)
	}

	if len(recipe.Steps) != 1 {
		t.Fatalf("expected 1 step, got %d", len(recipe.Steps))
	}

	editStep, ok := recipe.Steps[0].(*EditStep)
	if !ok {
		t.Fatalf("expected EditStep, got %T", recipe.Steps[0])
	}

	if editStep.CustomOperations == nil {
		t.Error("expected CustomOperations to be set")
	}

	// Verify it's a callable
	if _, ok := editStep.CustomOperations.(starlark.Callable); !ok {
		t.Errorf("expected CustomOperations to be Callable, got %T", editStep.CustomOperations)
	}
}

func TestScanRequiresImplOrPatterns(t *testing.T) {
	// Scan without impl or patterns should fail
	recipeCode := `
recipe(
    name = "test",
    version = "1.0.0",
    steps = [
        scan(
            id = "bad_scan",
            name = "Missing patterns and impl",
        ),
    ],
)
`
	interp := NewInterpreter()
	_, err := interp.LoadString("test.star", recipeCode)
	if err == nil {
		t.Error("expected error for scan without impl or patterns")
	}
}

func TestScanWithPatternsNoImpl(t *testing.T) {
	// Traditional scan with patterns should still work
	recipeCode := `
recipe(
    name = "test",
    version = "1.0.0",
    steps = [
        scan(
            id = "find_rules",
            name = "Find cc rules",
            files = ["**/BUILD"],
            patterns = ["cc_binary", "cc_library"],
        ),
    ],
)
`
	interp := NewInterpreter()
	recipe, err := interp.LoadString("test.star", recipeCode)
	if err != nil {
		t.Fatalf("LoadString failed: %v", err)
	}

	scanStep, ok := recipe.Steps[0].(*ScanStep)
	if !ok {
		t.Fatalf("expected ScanStep, got %T", recipe.Steps[0])
	}

	if scanStep.CustomImpl != nil {
		t.Error("expected CustomImpl to be nil for pattern-based scan")
	}

	if len(scanStep.FilePatterns) != 1 || scanStep.FilePatterns[0] != "**/BUILD" {
		t.Errorf("expected FilePatterns = [**/BUILD], got %v", scanStep.FilePatterns)
	}

	if len(scanStep.Patterns) != 2 {
		t.Errorf("expected 2 patterns, got %d", len(scanStep.Patterns))
	}
}

func TestEditWithStaticOperations(t *testing.T) {
	// Traditional edit with static operations should still work
	recipeCode := `
recipe(
    name = "test",
    version = "1.0.0",
    steps = [
        edit(
            id = "add_dep",
            name = "Add dependency",
            operations = [
                ensure_bazel_dep("rules_cc", "0.1.0"),
            ],
        ),
    ],
)
`
	interp := NewInterpreter()
	recipe, err := interp.LoadString("test.star", recipeCode)
	if err != nil {
		t.Fatalf("LoadString failed: %v", err)
	}

	editStep, ok := recipe.Steps[0].(*EditStep)
	if !ok {
		t.Fatalf("expected EditStep, got %T", recipe.Steps[0])
	}

	if editStep.CustomOperations != nil {
		t.Error("expected CustomOperations to be nil for static operations")
	}

	if len(editStep.Operations) != 1 {
		t.Errorf("expected 1 operation, got %d", len(editStep.Operations))
	}
}

func TestCtxObjectInCustomScan(t *testing.T) {
	// Verify ctx object is properly passed to custom scan function
	recipeCode := `
def check_ctx(ctx):
    # Access ctx properties to ensure they exist
    _ = ctx.project_path
    _ = ctx.dry_run
    _ = ctx.outputs
    return []

recipe(
    name = "test",
    version = "1.0.0",
    steps = [
        scan(
            id = "check",
            name = "Check ctx",
            impl = check_ctx,
        ),
    ],
)
`
	interp := NewInterpreter()
	recipe, err := interp.LoadString("test.star", recipeCode)
	if err != nil {
		t.Fatalf("LoadString failed: %v", err)
	}

	// The parsing should succeed - ctx object is created during execution
	if len(recipe.Steps) != 1 {
		t.Fatalf("expected 1 step, got %d", len(recipe.Steps))
	}
}

func TestCustomScanExecution(t *testing.T) {
	recipeCode := `
def my_scanner(ctx):
    return [
        {"file": "pkg/BUILD", "line": 10, "text": "cc_binary(", "rule": "cc_binary"},
        {"file": "pkg/BUILD", "line": 20, "text": "cc_library(", "rule": "cc_library"},
    ]

recipe(
    name = "test",
    version = "1.0.0",
    steps = [
        scan(
            id = "find_rules",
            name = "Find rules",
            impl = my_scanner,
        ),
    ],
)
`
	interp := NewInterpreter()
	recipe, err := interp.LoadString("test.star", recipeCode)
	if err != nil {
		t.Fatalf("LoadString failed: %v", err)
	}

	// Create executor and run
	exec := NewExecutor("/test/project", Options{DryRun: true})
	result, err := exec.Execute(recipe, Options{DryRun: true})
	if err != nil {
		t.Fatalf("Execute failed: %v", err)
	}

	if !result.Success {
		if result.Error != nil {
			t.Logf("Execution error: %v", result.Error)
		}
		for _, sr := range result.StepResults {
			if sr.Error != nil {
				t.Logf("Step %s failed: %v", sr.StepID, sr.Error)
			}
		}
		t.Errorf("expected execution to succeed, got %d steps run", result.StepsRun)
	}
}

func TestCustomEditExecution(t *testing.T) {
	recipeCode := `
def dynamic_ops(ctx):
    return [
        ensure_load("cc_binary", "@rules_cc//cc:defs.bzl"),
    ]

recipe(
    name = "test",
    version = "1.0.0",
    steps = [
        edit(
            id = "add_loads",
            name = "Add load statements",
            operations = dynamic_ops,
        ),
    ],
)
`
	interp := NewInterpreter()
	recipe, err := interp.LoadString("test.star", recipeCode)
	if err != nil {
		t.Fatalf("LoadString failed: %v", err)
	}

	exec := NewExecutor("/test/project", Options{DryRun: true})
	result, err := exec.Execute(recipe, Options{DryRun: true})
	if err != nil {
		t.Fatalf("Execute failed: %v", err)
	}

	if !result.Success {
		if result.Error != nil {
			t.Logf("Execution error: %v", result.Error)
		}
		for _, sr := range result.StepResults {
			if sr.Error != nil {
				t.Logf("Step %s failed: %v", sr.StepID, sr.Error)
			}
		}
		t.Errorf("expected execution to succeed")
	}
}

func TestCtxOutputsContainsPreviousStepResults(t *testing.T) {
	recipeCode := `
def scanner(ctx):
    return [
        {"file": "BUILD", "line": 1, "text": "cc_binary", "rule": "cc_binary"},
    ]

def transformer(ctx):
    # Access results from previous scan step
    scan_result = ctx.outputs["scan_step"]
    ops = []
    for match in scan_result:
        if match["rule"] == "cc_binary":
            ops.append(ensure_load("cc_binary", "@rules_cc//cc:defs.bzl"))
    return ops

recipe(
    name = "test",
    version = "1.0.0",
    steps = [
        scan(
            id = "scan_step",
            name = "Find rules",
            impl = scanner,
        ),
        edit(
            id = "edit_step",
            name = "Add loads based on scan",
            depends_on = ["scan_step"],
            operations = transformer,
        ),
    ],
)
`
	interp := NewInterpreter()
	recipe, err := interp.LoadString("test.star", recipeCode)
	if err != nil {
		t.Fatalf("LoadString failed: %v", err)
	}

	if len(recipe.Steps) != 2 {
		t.Fatalf("expected 2 steps, got %d", len(recipe.Steps))
	}

	exec := NewExecutor("/test/project", Options{DryRun: true})
	result, err := exec.Execute(recipe, Options{DryRun: true})
	if err != nil {
		t.Fatalf("Execute failed: %v", err)
	}

	if !result.Success {
		t.Errorf("expected execution to succeed")
	}
}

func TestMixedStaticAndCustomOperations(t *testing.T) {
	// Test that we can have both static operations and custom operations in different steps
	recipeCode := `
def custom_ops(ctx):
    return [
        ensure_load("cc_test", "@rules_cc//cc:defs.bzl"),
    ]

recipe(
    name = "test",
    version = "1.0.0",
    steps = [
        edit(
            id = "static_edit",
            name = "Static operations",
            operations = [
                ensure_bazel_dep("rules_cc", "0.1.0"),
            ],
        ),
        edit(
            id = "custom_edit",
            name = "Custom operations",
            depends_on = ["static_edit"],
            operations = custom_ops,
        ),
    ],
)
`
	interp := NewInterpreter()
	recipe, err := interp.LoadString("test.star", recipeCode)
	if err != nil {
		t.Fatalf("LoadString failed: %v", err)
	}

	if len(recipe.Steps) != 2 {
		t.Fatalf("expected 2 steps, got %d", len(recipe.Steps))
	}

	// First step should have static operations
	editStep1, ok := recipe.Steps[0].(*EditStep)
	if !ok {
		t.Fatalf("expected EditStep, got %T", recipe.Steps[0])
	}
	if len(editStep1.Operations) != 1 {
		t.Errorf("expected 1 static operation, got %d", len(editStep1.Operations))
	}
	if editStep1.CustomOperations != nil {
		t.Error("expected no custom operations in first step")
	}

	// Second step should have custom operations
	editStep2, ok := recipe.Steps[1].(*EditStep)
	if !ok {
		t.Fatalf("expected EditStep, got %T", recipe.Steps[1])
	}
	if editStep2.CustomOperations == nil {
		t.Error("expected custom operations in second step")
	}
}
