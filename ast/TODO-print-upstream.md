# TODO: Contribute print.go to starlark-syntax-go

**Priority:** Medium
**Upstream Repo:** https://github.com/albertocavalcante/starlark-syntax-go
**Related:** format.go (local workaround)

---

## Background

The `starlark-syntax-go` package currently extracts only the **parser** from `bazelbuild/buildtools`:
- `ast.go` - AST node definitions ✅
- `lex.go` - Lexer ✅
- `parse.go` - Parser (yacc-generated) ✅
- `walk.go` - AST traversal ✅
- `quote.go` - String quoting ✅

**Missing:** `print.go` - The formatter that converts AST back to source code.

Without `print.go`, users cannot:
1. Format a modified AST back to valid Starlark
2. Round-trip parse → modify → print
3. Use the package for refactoring tools

---

## Current Workaround

We created a local `ast/format.go` that provides basic formatting:

```go
// ast/format.go
func Format(f *bzl.File) []byte { ... }
func FormatWithoutRewriting(f *bzl.File) []byte { ... }
```

This works but is incomplete compared to the full buildtools `print.go`.

### Missing Features in Our Formatter

1. **Intelligent line breaking** - buildtools has heuristics for when to use multi-line vs single-line
2. **Alignment** - buildtools aligns `=` in consecutive assignments
3. **Trailing comma handling** - buildtools adds/removes trailing commas appropriately
4. **String normalization** - buildtools normalizes quote styles
5. **Comment attachment on edits** - buildtools reassigns comments when nodes are moved

---

## What Needs to Be Done

### Step 1: Extract print.go from buildtools

The `print.go` file in `bazelbuild/buildtools/build/` contains:
- `Format(f *File) []byte` - Main formatting function
- `FormatWithoutRewriting(f *File) []byte` - Format without applying transformations
- `FormatString(f *File) string` - Convenience wrapper
- Internal formatting logic for each node type

**Files to extract:**
```
bazelbuild/buildtools/build/print.go → starlark-syntax-go/bzl/print.go
```

### Step 2: Update Import Paths

The `print.go` file likely imports from `build` package internally:
```go
// Before
package build

// After
package bzl

// May need to update internal references
```

### Step 3: Handle Dependencies

Check if `print.go` has any dependencies not already in starlark-syntax-go:
- `tables.go` - May have rewrite rules
- `rewrite.go` - Has transformation logic (may want to include some)

### Step 4: Update Vendoring Script

In `starlark-syntax-go`, update `.copybara/` or vendoring script to include:
```yaml
# .copybara/copy.bara.sky or equivalent
files_to_copy:
  - "build/print.go"  # ADD THIS
  - "build/ast.go"
  - "build/lex.go"
  - "build/parse.go"
  - "build/walk.go"
  - "build/quote.go"
```

### Step 5: Add Tests

Include print tests from buildtools:
```
build/print_test.go → bzl/print_test.go
```

---

## Upstream Contribution Checklist

- [ ] Fork starlark-syntax-go (or use maintainer access)
- [ ] Extract print.go from buildtools
- [ ] Update import paths to `bzl` package
- [ ] Verify no missing dependencies
- [ ] Run existing tests
- [ ] Add print-specific tests
- [ ] Update README with Format() documentation
- [ ] Submit PR
- [ ] After merge: Remove local ast/format.go
- [ ] Update bazelbump to use upstream Format()

---

## Alternative: Keep Local Formatter

If upstream contribution is delayed, we can:

1. **Keep ast/format.go** as a standalone formatter
2. **Improve incrementally** as needs arise
3. **Eventually replace** when upstream has Format()

### Pros of Local Formatter
- No external dependency on upstream timeline
- Can customize for bazelbump-specific needs
- Full control over output format

### Cons of Local Formatter
- Duplicated effort
- May diverge from buildifier style
- Missing advanced features

---

## References

- buildtools print.go: https://github.com/bazelbuild/buildtools/blob/master/build/print.go
- starlark-syntax-go: https://github.com/albertocavalcante/starlark-syntax-go
- Related spec: /docs/design/starlark-syntax-go-spec.md

---

## Notes on Comment Preservation

The current starlark-syntax-go parser may not fully preserve comments because:

1. **assignComments() logic** - buildtools has special logic in `lex.go` that assigns comments to AST nodes after parsing. This may not be fully included.

2. **Comment attachment rules:**
   - Suffix comments (same line) → attach to preceding expression
   - Line comments (whole line) → attach to following expression
   - File-level comments → attach to File.Comments.Before/After

3. **To investigate:**
   - Does starlark-syntax-go include `assignComments()` from lex.go?
   - Are comments being collected but not assigned?
   - Test with: `file.Comments.Before`, `stmt.Comment().Before`

If comment preservation is broken, this needs to be fixed in starlark-syntax-go as well.

---

## Contact

For questions about starlark-syntax-go:
- GitHub Issues: https://github.com/albertocavalcante/starlark-syntax-go/issues
- Maintainer: @albertocavalcante
