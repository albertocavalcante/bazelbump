# Envoy Migration Analysis - Bugs and Findings

**Date:** 2026-02-04
**Test Subject:** Envoy proxy (1,696 BUILD files, 463MB)
**Recipe:** autoload-migration v2.0.0

## Executive Summary

The autoload-migration recipe successfully **detected** all native rules and **identified** the correct changes needed. However, a critical bug in the load statement insertion logic caused **~96% of modified BUILD files to have broken syntax**.

| Metric | Value |
|--------|-------|
| Files scanned | 1,696 |
| Native rules detected | 3,258 |
| Files modified | 893 |
| Files with broken syntax | ~855 (96%) |
| Files correctly modified | ~38 (4%) |
| MODULE.bazel changes | ✅ Correct |

## What Worked Well

### 1. Detection Engine ✅
- Correctly identified all 3,258 native rule usages
- Pattern matching worked across all rule families (cc, java, proto, shell)
- Scan completed in under 1 second for 1,696 files

### 2. Dependency Resolution ✅
- Correctly determined which `bazel_dep` entries were needed
- MODULE.bazel changes were syntactically correct:
```python
bazel_dep(name = "rules_cc", version = "0.1.1")
bazel_dep(name = "rules_java", version = "8.6.3")
bazel_dep(name = "protobuf", version = "29.3")
```

### 3. State Management ✅
- Run tracking worked correctly
- Checkpoints were created at step boundaries
- History command showed accurate information
- Resume functionality worked (though there was nothing to resume after interruption)

### 4. Performance ✅
- 893 files modified in ~1.1 seconds
- Checkpointing overhead was minimal

## Critical Bug: Load Statement Insertion

### The Problem

When inserting `load()` statements into BUILD files, the recipe inserts them **inside** existing multi-line `load()` blocks instead of **before** them.

### Example of Broken Output

**Before (valid):**
```python
load(
    "//bazel:envoy_build_system.bzl",
    "envoy_cc_library",
    "envoy_package",
)
```

**After (BROKEN):**
```python
load(
load("@rules_cc//cc:defs.bzl", "cc_library")
    "//bazel:envoy_build_system.bzl",
    "envoy_cc_library",
    "envoy_package",
)
```

The `load("@rules_cc...")` line was inserted AFTER the opening `load(` but BEFORE its arguments, creating invalid Starlark syntax.

### Expected Correct Output

```python
load("@rules_cc//cc:defs.bzl", "cc_library")
load(
    "//bazel:envoy_build_system.bzl",
    "envoy_cc_library",
    "envoy_package",
)
```

### Root Cause Analysis

The `ensure_load` operation uses line-based insertion with `position="top"` or similar logic. The bug is in how it identifies where to insert:

1. **Current behavior:** Finds line 1 (or first load statement), inserts after it
2. **Problem:** Line 1 might be `load(` which is the START of a multi-line statement
3. **Result:** New load is inserted inside the multi-line statement

### Affected Files

- **855 out of 893** modified files have multi-line load statements
- These files follow Envoy's style guide which uses multi-line loads for readability
- Only ~38 files with single-line loads at the top were correctly modified

## Other Observations

### 1. Load Statement Ordering
The recipe doesn't consider existing load statement ordering conventions. Envoy groups loads by source:
```python
# Envoy internal loads
load("//bazel:envoy_build_system.bzl", ...)
load("//bazel:envoy_internal.bzl", ...)

# External loads should probably go elsewhere
```

### 2. No Syntax Validation
The recipe doesn't validate that the output is syntactically valid Starlark. A post-modification syntax check would have caught this bug.

### 3. Idempotency Concern
If the recipe is run again on the broken files, it may add duplicate loads or fail to parse the files entirely.

## Foundational Work Needed

### Priority 1: Fix Multi-line Load Detection (Critical)

**Problem:** The insertion logic doesn't understand multi-line statements.

**Solution Options:**

| Option | Complexity | Correctness |
|--------|------------|-------------|
| A. Parse with Starlark AST | High | Best |
| B. Regex to detect multi-line loads | Medium | Good |
| C. Insert at line 0 always | Low | Fragile |

**Recommended:** Option A - Use proper Starlark parsing

```go
// Pseudo-code for proper insertion
func insertLoadStatement(content []byte, newLoad string) []byte {
    // Parse the file as Starlark AST
    ast, err := starlark.Parse(content)

    // Find the first non-load statement
    insertPos := findFirstNonLoadPosition(ast)

    // Insert before that position
    return insertAt(content, insertPos, newLoad)
}
```

**Foundational work required:**
- Add Starlark AST parsing capability to the transform package
- Modify `ensure_load` operation to use AST-aware insertion
- Handle edge cases: files with no loads, files starting with comments, etc.

### Priority 2: Add Syntax Validation (High)

**Problem:** No validation that output is valid Starlark.

**Solution:** Add a validation step after file modification.

```go
func validateStarlark(path string, content []byte) error {
    _, err := syntax.Parse(path, content, 0)
    return err
}
```

**Foundational work required:**
- Import `go.starlark.net/syntax` for parsing
- Add validation hook in the file modification pipeline
- Decide behavior on validation failure (rollback? warn?)

### Priority 3: Improve Load Statement Placement (Medium)

**Problem:** New loads are inserted without considering style/grouping.

**Solution:** Analyze existing load patterns and insert appropriately.

**Options:**
1. Insert after last existing external load (if any)
2. Insert at the beginning, before all other loads
3. Make it configurable per-recipe

### Priority 4: Add Dry-Run Syntax Check (Medium)

**Problem:** Dry-run shows changes but doesn't validate they're correct.

**Solution:** Parse the "would-be" output during dry-run to catch errors before applying.

## Recommended Fix Implementation Order

1. **Immediate (blocks usage):** Fix multi-line load detection
   - This is blocking - the recipe produces broken output
   - Estimate: Requires Starlark AST integration

2. **Short-term (prevents regressions):** Add syntax validation
   - Catches this class of bug automatically
   - Estimate: Small change, starlark parser already available

3. **Medium-term (improves quality):** Better load placement
   - Improves output quality but not correctness
   - Estimate: Medium effort

4. **Nice-to-have:** Dry-run validation
   - Defense in depth
   - Estimate: Small change

## Test Cases to Add

```go
func TestEnsureLoad_MultiLineExistingLoad(t *testing.T) {
    input := `load(
    "//foo:bar.bzl",
    "some_rule",
)

some_rule(name = "test")
`
    expected := `load("@rules_cc//cc:defs.bzl", "cc_library")
load(
    "//foo:bar.bzl",
    "some_rule",
)

some_rule(name = "test")
`
    // Test that new load is inserted BEFORE the multi-line load
}

func TestEnsureLoad_PreservesExistingLoads(t *testing.T) {
    // Ensure we don't duplicate existing loads
}

func TestEnsureLoad_ValidStarlarkOutput(t *testing.T) {
    // Ensure output parses as valid Starlark
}
```

## Conclusion

The recipe's **analysis capabilities are solid** - it correctly identified what needs to change. The **execution has a critical bug** in how changes are applied to files with multi-line load statements.

This is a good example of why:
1. Large-scale migrations need thorough testing on real codebases
2. Syntax validation should be built into the modification pipeline
3. AST-aware transformations are more reliable than text manipulation

The fix requires foundational work in the transform layer to handle Starlark syntax properly, but once fixed, the recipe will be production-ready for large codebases like Envoy.
