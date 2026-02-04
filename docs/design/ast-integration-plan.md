# AST Integration Plan: Recipe Engine

**Status:** Ready for Implementation
**Created:** 2026-02-04
**Related:** ast-transform-spec.md, envoy-migration-analysis.md

## Overview

This plan describes how to wire the new `ast/` package into the recipe engine to replace text-based BUILD file manipulation with proper AST-aware transformations.

## Current State

### Text-Based Implementation (Problem)

Location: `recipe/starlark/executor.go`

```
opEnsureLoad() → ensureLoadInFile() → regex + string manipulation
opRemoveLoad() → removeLoadFromFile() → regex + string manipulation
```

**Known Issues:**
- Fails on multi-line load statements (96% of Envoy files broken)
- Loses comments during transformations
- Fragile regex patterns break on edge cases

### AST-Based Implementation (Solution)

Location: `ast/`

```
ast.EnsureLoad() → proper AST manipulation → formatted output
ast.RemoveLoad() → proper AST manipulation → formatted output
```

**Benefits:**
- Handles multi-line statements correctly
- Preserves comments
- Validates output syntax

---

## Integration Strategy

### Phase 1: Core Operations (High Priority)

Replace the internal implementation while keeping external API unchanged.

#### 1.1 Update `opEnsureLoad()` (executor.go:869-907)

**Before:**
```go
func (e *Executor) opEnsureLoad(ctx *Context, op Operation) ([]*FileChange, error) {
    // ... validation ...
    return e.ensureLoadsForMatches(ctx, matches, rule, loadFrom)
}
```

**After:**
```go
func (e *Executor) opEnsureLoad(ctx *Context, op Operation) ([]*FileChange, error) {
    // ... validation (unchanged) ...
    return e.ensureLoadsForMatchesAST(ctx, matches, rule, loadFrom)
}
```

#### 1.2 Create `ensureLoadsForMatchesAST()`

New function using AST:

```go
func (e *Executor) ensureLoadsForMatchesAST(ctx *Context, matches []string, rule, loadFrom string) ([]*FileChange, error) {
    var changes []*FileChange

    for _, file := range matches {
        // Parse with AST
        astFile, err := ast.ParseFile(file)
        if err != nil {
            return nil, fmt.Errorf("parse %s: %w", file, err)
        }

        // Read original for diff
        oldContent, _ := os.ReadFile(file)

        // Apply AST operation
        modified, err := ast.EnsureLoad(astFile, ast.EnsureLoadOptions{
            Module:   loadFrom,
            Symbols:  []string{rule},
            Position: ast.LoadPositionAuto,
        })
        if err != nil {
            return nil, fmt.Errorf("ensure_load in %s: %w", file, err)
        }

        if !modified {
            continue // Already has the load
        }

        // Format output
        newContent := astFile.Format()

        // Create change record
        changes = append(changes, &FileChange{
            Path:       file,
            OldContent: oldContent,
            NewContent: newContent,
            Operation:  "ensure_load",
        })

        // Write if not dry-run
        if !ctx.DryRun {
            if err := os.WriteFile(file, newContent, 0644); err != nil {
                return nil, fmt.Errorf("write %s: %w", file, err)
            }
        }
    }

    return changes, nil
}
```

#### 1.3 Update `opRemoveLoad()` (executor.go:945-978)

Similar pattern - create `removeLoadFromFileAST()` using `ast.RemoveLoad()`.

### Phase 2: Deprecation (Medium Priority)

#### 2.1 Mark Old Functions as Deprecated

```go
// Deprecated: Use ensureLoadsForMatchesAST instead.
// This text-based implementation fails on multi-line load statements.
func (e *Executor) ensureLoadsForMatches(...) { ... }

// Deprecated: Use removeLoadFromFileAST instead.
func (e *Executor) removeLoadFromFile(...) { ... }
```

#### 2.2 Remove After Validation

After successful testing on Envoy and other codebases:
- Delete `ensureLoadInFile()` (lines 1044-1097)
- Delete `removeLoadFromFile()` (lines 981-1041)
- Delete `findLoadInsertPoint()` (lines 1193-1207)

### Phase 3: Extended Operations (Lower Priority)

#### 3.1 Add New Starlark Builtins

Expose additional AST operations to recipe authors:

```python
# New builtins
merge_loads(path)      # Combine duplicate load sources
sort_loads(path)       # Sort loads: external, workspace, relative
validate_build(path)   # Check syntax validity
```

#### 3.2 Update `parse_build_file()` Builtin

Option A: Keep regex-based for backward compatibility
Option B: Upgrade to AST-based for better accuracy

Recommendation: Keep regex-based initially, add new `parse_build_file_ast()` builtin.

---

## Files to Modify

### High Priority

| File | Changes | Lines |
|------|---------|-------|
| `executor.go` | Add AST-based implementations | ~100 new |
| `executor.go` | Update opEnsureLoad/opRemoveLoad | ~20 modified |

### Medium Priority (After Validation)

| File | Changes | Lines |
|------|---------|-------|
| `executor.go` | Remove deprecated functions | ~200 deleted |
| `buildfile.go` | Consider deprecation | 450 lines |

### Low Priority

| File | Changes | Lines |
|------|---------|-------|
| `interpreter.go` | Add new builtins | ~50 new |
| `builtins_help.go` | Document new builtins | ~20 new |

---

## Testing Strategy

### Unit Tests

1. Add tests in `executor_test.go` for AST-based operations
2. Test multi-line load handling specifically
3. Test comment preservation

### Integration Tests

1. Run existing recipe tests (should pass unchanged)
2. Test on Envoy codebase (target: 0% broken files)

### Validation Checklist

- [ ] All existing recipe tests pass
- [ ] Multi-line load statements handled correctly
- [ ] Comments preserved in output
- [ ] Dry-run mode works
- [ ] Error messages are clear
- [ ] Performance acceptable (benchmark if needed)

---

## Rollout Plan

### Step 1: Implement (This PR)
- Add AST-based functions alongside existing
- Keep existing functions as fallback
- Add feature flag: `BAZELBUMP_USE_AST=1`

### Step 2: Validate
- Run on Envoy test copy
- Compare output with expected
- Fix any issues found

### Step 3: Default Enable
- Make AST the default
- Keep `BAZELBUMP_USE_AST=0` for legacy fallback

### Step 4: Cleanup
- Remove legacy text-based code
- Remove feature flag

---

## Risk Mitigation

| Risk | Mitigation |
|------|------------|
| AST output differs from original | Validate with diff, accept buildifier-style normalization |
| Performance regression | Benchmark on large codebases, optimize if needed |
| Edge cases not handled | Comprehensive test suite, gradual rollout |
| Recipe compatibility | Keep operation signatures unchanged |

---

## Success Criteria

1. **Envoy Migration**: 0% broken files (down from 96%)
2. **Backward Compatibility**: All existing recipes work unchanged
3. **Comment Preservation**: Comments survive transformations
4. **Code Quality**: ~200 lines of fragile regex code removed
