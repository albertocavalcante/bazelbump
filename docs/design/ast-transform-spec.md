# AST-Aware Starlark Transformations Specification

**Status:** Draft v1.0
**Author:** bazelbump contributors
**Created:** 2026-02-04
**Related:** envoy-migration-analysis.md, starlark-recipes-spec.md

## Table of Contents

1. [Problem Statement](#1-problem-statement)
2. [Research Findings](#2-research-findings)
3. [Parser Selection](#3-parser-selection)
4. [Comment Preservation Strategy](#4-comment-preservation-strategy)
5. [Transformation Architecture](#5-transformation-architecture)
6. [Load Statement Operations](#6-load-statement-operations)
7. [Validation Pipeline](#7-validation-pipeline)
8. [Migration Path](#8-migration-path)
9. [Performance Considerations](#9-performance-considerations)
10. [Test Cases](#10-test-cases)
11. [Implementation Phases](#11-implementation-phases)

---

## 1. Problem Statement

### 1.1 Critical Bug: Text-Based Insertion Failure

The current `ensure_load` operation uses line-based text manipulation to insert load
statements. This approach fails catastrophically on multi-line load blocks.

**BROKEN OUTPUT (current behavior):**
```python
load(
load("@rules_cc//cc:defs.bzl", "cc_library")  # WRONG: inserted inside!
    "//bazel:envoy_build_system.bzl",
    "envoy_cc_library",
    "envoy_package",
)
```

**CORRECT OUTPUT (expected):**
```python
load("@rules_cc//cc:defs.bzl", "cc_library")  # CORRECT: before the block
load(
    "//bazel:envoy_build_system.bzl",
    "envoy_cc_library",
    "envoy_package",
)
```

### 1.2 Impact Analysis

Testing on Envoy (1,696 BUILD files, 463MB codebase):

| Metric | Value |
|--------|-------|
| Files modified | 893 |
| Files with broken syntax | ~855 (96%) |
| Files correctly modified | ~38 (4%) |
| Root cause | Multi-line load statements |

### 1.3 Why Text-Based Manipulation Fails

```
Line-based logic:
  1. Find line containing "load("  -> line 1
  2. Insert after line 1

Reality:
  Line 1: "load("
  Line 2: "    \"//bazel:envoy_build_system.bzl\","
  Line 3: "    \"envoy_cc_library\","
  Line 4: ")"

  Inserting after line 1 puts new content INSIDE the load() call.
```

### 1.4 Requirements

1. **AST-Aware Transformations** - Parse Starlark properly, not line-based manipulation
2. **Comment Preservation** - CRITICAL: Comments must survive transformations
3. **Syntax Validation** - Validate output is valid Starlark before writing
4. **Load Statement Placement** - Respect conventions (grouping, ordering)
5. **Dry-Run Validation** - Catch errors before applying changes
6. **Backward Compatibility** - Existing recipes must continue to work

---

## 2. Research Findings

### 2.1 go-bzlmod Analysis

The `go-bzlmod` library (at `/Users/adsc/dev/ws/go-bzlmod`) vendors buildifier's parser
in `third_party/buildtools/build/`. This is a mature, battle-tested parser specifically
designed for BUILD/Starlark files.

**Key Files Examined:**
- `syntax.go` - AST node definitions with comment support
- `lex.go` - Lexer with comment collection
- `print.go` - Printer with comment preservation
- `rewrite.go` - Transformation utilities
- `walk.go` - AST traversal helpers
- `rule.go` - Rule manipulation API

**Key Insight: Comment Architecture**

The buildifier parser attaches comments to AST nodes using a `Comments` struct:

```go
// From syntax.go
type Comments struct {
    Before []Comment // whole-line comments before this expression
    Suffix []Comment // end-of-line comments after this expression
    After  []Comment // For top-level expressions only
}

// Every Expr has comments attached
type Expr interface {
    Span() (start, end Position)
    Comment() *Comments
    Copy() Expr
}
```

This design ensures comments travel with their associated code during transformations.

### 2.2 Available Go Starlark Parsers

| Parser | Comment Support | BUILD-Aware | Transformation Support |
|--------|----------------|-------------|------------------------|
| `github.com/bazelbuild/buildtools/build` | Excellent | Yes | Excellent |
| `go.starlark.net/syntax` | Limited | No | Limited |
| `go.starlark.net/starlark` | None (interpreter) | No | None |

### 2.3 How Buildifier Handles Comments

From `lex.go`, the lexer collects comments separately and assigns them in
`assignComments()`:

```go
func (in *input) assignComments() {
    in.order(in.file)           // Build preorder/postorder lists
    in.assignSuffixComments()   // End-of-line comments -> preceding node
    in.assignLineComments()     // Whole-line comments -> following node
}
```

This two-pass approach ensures:
1. Suffix comments (same line) attach to the expression before them
2. Line comments (whole line) attach to the expression after them

### 2.4 How Buildifier Handles Load Statements

From `rewrite.go`, buildifier has several load-related transformations:

```go
var rewrites = []struct {
    name  string
    fn    func(*File, *Rewriter)
    scope FileType
}{
    // ...
    {"loadTop", moveLoadOnTop, scopeBoth},
    {"sameOriginLoad", compressSameOriginLoads, scopeBoth},
    {"sortLoadStatements", sortLoadStatements, scopeBoth},
    {"loadsort", sortAllLoadArgs, scopeBoth},
    // ...
}
```

The `LoadStmt` AST node:

```go
type LoadStmt struct {
    Comments
    Load         Position
    Module       *StringExpr
    From         []*Ident  // name defined in loading module
    To           []*Ident  // name in loaded module
    Rparen       End
    ForceCompact bool
}
```

### 2.5 How Gazelle Handles BUILD File Modifications

Gazelle uses the same buildtools parser and follows a pattern:
1. Parse file to AST
2. Modify AST (add/remove rules, loads, etc.)
3. Format and write back

Key functions from gazelle's approach:
- `rule.LoadInfo()` - Track load statement requirements
- `rule.SortLoads()` - Sort loads alphabetically
- `rule.MergeLoads()` - Combine duplicate load sources

---

## 3. Parser Selection

### 3.1 Recommendation: Use buildtools/build Parser

**Decision:** Use `github.com/bazelbuild/buildtools/build` (as vendored in go-bzlmod)

**Rationale:**

| Criterion | buildtools/build | go.starlark.net/syntax |
|-----------|------------------|------------------------|
| Comment preservation | Excellent | Poor (no attachment) |
| BUILD file semantics | Native support | Generic Starlark only |
| Load statement handling | LoadStmt type | Generic CallExpr |
| Rule manipulation API | Yes (rule.go) | No |
| Formatting/printing | Yes (print.go) | No |
| Used by | Buildifier, Gazelle | Starlark interpreter |
| Maturity | 8+ years | 5+ years |

### 3.2 Integration Approach

Since go-bzlmod already vendors buildtools, we can:

**Option A: Vendor buildtools directly**
```go
import "github.com/bazelbuild/buildtools/build"
```

**Option B: Use go-bzlmod's vendored copy**
```go
import "github.com/albertocavalcante/go-bzlmod/third_party/buildtools/build"
```

**Recommendation:** Option A (direct dependency) for clarity and maintainability.

### 3.3 Version Considerations

- Buildtools is actively maintained by the Bazel team
- Latest version supports all modern Starlark/BUILD syntax
- MODULE.bazel support included

---

## 4. Comment Preservation Strategy

### 4.1 Comment Types in Starlark

```python
# Block comment (before expression)
# Another line of block comment
load("@rules_cc//cc:defs.bzl", "cc_library")  # Suffix comment

cc_library(
    name = "foo",  # Inline attribute comment
    # Comment before deps
    deps = [
        "//lib:bar",  # Why this dep
    ],
)
# Comment after rule (After comment)
```

### 4.2 buildtools Comment Model

```
+------------------+
|    Expression    |
+------------------+
| Comments.Before  | <- Whole-line comments preceding
| Comments.Suffix  | <- Same-line comment after
| Comments.After   | <- Whole-line comments following (top-level only)
+------------------+
```

### 4.3 Preservation During Transformations

**Rule: Never discard comment nodes**

When modifying the AST:

```go
// BAD: Creates new node, loses comments
newLoad := &build.LoadStmt{
    Module: &build.StringExpr{Value: path},
    // Comments lost!
}

// GOOD: Preserve existing comments when modifying
if existingLoad != nil {
    existingLoad.From = append(existingLoad.From, newSymbol)
    existingLoad.To = append(existingLoad.To, newSymbol)
}

// GOOD: For new nodes, explicitly handle comment attachment
newLoad := &build.LoadStmt{
    Module: &build.StringExpr{Value: path},
}
// Attach comments from context if appropriate
```

### 4.4 Edge Cases

| Case | Handling |
|------|----------|
| Comment on deleted statement | Attach to next statement |
| Comment between merged loads | Keep all comments |
| Comment on moved statement | Comment moves with statement |
| Trailing file comments | Preserve in File.After |

### 4.5 Comment Preservation Test Cases

```go
func TestCommentPreservation(t *testing.T) {
    tests := []struct{
        name     string
        input    string
        expected string
    }{
        {
            name: "suffix_comment_preserved",
            input: `load("//foo:bar.bzl", "rule1")  # Important!`,
            // After adding rule2
            expected: `load("//foo:bar.bzl", "rule1", "rule2")  # Important!`,
        },
        {
            name: "block_comment_preserved",
            input: `# Load for special rules
load("//foo:bar.bzl", "rule1")`,
            expected: `# Load for special rules
load("//foo:bar.bzl", "rule1", "rule2")`,
        },
    }
}
```

---

## 5. Transformation Architecture

### 5.1 Package Structure

```
bazelbump/
├── transform/
│   ├── transform.go       # Existing text-based (deprecated)
│   └── transform_test.go
├── ast/                   # NEW: AST-aware transformations
│   ├── ast.go            # Core parsing utilities
│   ├── load.go           # Load statement operations
│   ├── rule.go           # Rule manipulation
│   ├── validate.go       # Syntax validation
│   ├── format.go         # Formatting preferences
│   └── ast_test.go
```

### 5.2 Core Interfaces

```go
// ast/ast.go

package ast

import (
    "github.com/bazelbuild/buildtools/build"
)

// File wraps a parsed BUILD/Starlark file with modification tracking.
type File struct {
    *build.File
    path     string
    modified bool
}

// ParseFile parses a BUILD or .bzl file.
func ParseFile(path string, data []byte) (*File, error) {
    f, err := build.Parse(path, data)
    if err != nil {
        return nil, fmt.Errorf("parse %s: %w", path, err)
    }
    return &File{File: f, path: path}, nil
}

// Format returns the formatted file content.
func (f *File) Format() []byte {
    return build.Format(f.File)
}

// FormatPreserving returns formatted content with minimal changes.
func (f *File) FormatPreserving() []byte {
    return build.FormatWithoutRewriting(f.File)
}

// IsModified returns true if the file has been changed.
func (f *File) IsModified() bool {
    return f.modified
}

// Transformer defines the interface for AST transformations.
type Transformer interface {
    // Transform modifies the file and returns true if changes were made.
    Transform(f *File) (bool, error)
}
```

### 5.3 High-Level Architecture Diagram

```
                                    ┌─────────────────┐
                                    │  Recipe Engine  │
                                    └────────┬────────┘
                                             │
                                             ▼
                    ┌────────────────────────────────────────────┐
                    │           Operation Dispatcher             │
                    │  (ensure_load, ensure_bazel_dep, etc.)     │
                    └────────────────────┬───────────────────────┘
                                         │
              ┌──────────────────────────┼──────────────────────────┐
              │                          │                          │
              ▼                          ▼                          ▼
     ┌────────────────┐        ┌────────────────┐        ┌────────────────┐
     │  BUILD Files   │        │ MODULE.bazel   │        │   .bazelrc     │
     │ (AST-based)    │        │ (AST-based)    │        │ (text-based)   │
     └───────┬────────┘        └───────┬────────┘        └───────┬────────┘
             │                         │                         │
             ▼                         ▼                         │
     ┌────────────────┐        ┌────────────────┐                │
     │ buildtools AST │        │ buildtools AST │                │
     │    Parser      │        │    Parser      │                │
     └───────┬────────┘        └───────┬────────┘                │
             │                         │                         │
             ▼                         ▼                         ▼
     ┌────────────────────────────────────────────────────────────────┐
     │                    Validation Pipeline                         │
     │  1. Parse result valid?                                        │
     │  2. Syntax check output                                        │
     │  3. Semantic validation (optional)                             │
     └────────────────────────────────────────────────────────────────┘
                                         │
                                         ▼
                              ┌─────────────────────┐
                              │   Write to Disk     │
                              │   (if not dry-run)  │
                              └─────────────────────┘
```

### 5.4 Operation Flow

```
┌─────────────────────────────────────────────────────────────────────┐
│                        ensure_load Operation                         │
├─────────────────────────────────────────────────────────────────────┤
│                                                                     │
│  1. Read file content                                               │
│        │                                                            │
│        ▼                                                            │
│  2. Parse to AST                                                    │
│     ┌─────────────────────────────────────────────────────────┐    │
│     │ build.Parse(filename, content)                          │    │
│     │   → *build.File with LoadStmt, CallExpr, Comments, etc. │    │
│     └─────────────────────────────────────────────────────────┘    │
│        │                                                            │
│        ▼                                                            │
│  3. Find existing loads                                             │
│     ┌─────────────────────────────────────────────────────────┐    │
│     │ for _, stmt := range file.Stmt {                        │    │
│     │     if load, ok := stmt.(*build.LoadStmt); ok {         │    │
│     │         // Check if module matches                       │    │
│     │     }                                                    │    │
│     │ }                                                        │    │
│     └─────────────────────────────────────────────────────────┘    │
│        │                                                            │
│        ▼                                                            │
│  4. Modify AST (one of):                                            │
│     a) Add symbol to existing LoadStmt                              │
│     b) Create new LoadStmt and insert at correct position           │
│        │                                                            │
│        ▼                                                            │
│  5. Format output                                                   │
│     ┌─────────────────────────────────────────────────────────┐    │
│     │ build.Format(file)                                      │    │
│     │   → Preserves comments, normalizes whitespace           │    │
│     └─────────────────────────────────────────────────────────┘    │
│        │                                                            │
│        ▼                                                            │
│  6. Validate output                                                 │
│     ┌─────────────────────────────────────────────────────────┐    │
│     │ build.Parse(filename, output)                           │    │
│     │   → Ensures valid Starlark syntax                       │    │
│     └─────────────────────────────────────────────────────────┘    │
│        │                                                            │
│        ▼                                                            │
│  7. Write file (if not dry-run)                                     │
│                                                                     │
└─────────────────────────────────────────────────────────────────────┘
```

---

## 6. Load Statement Operations

### 6.1 ensure_load Implementation

```go
// ast/load.go

package ast

import (
    "github.com/bazelbuild/buildtools/build"
)

// EnsureLoadOptions configures the ensure_load operation.
type EnsureLoadOptions struct {
    Module    string   // Load source (e.g., "@rules_cc//cc:defs.bzl")
    Symbols   []string // Symbols to load

    // Placement options
    Position  LoadPosition // Where to insert new loads
    GroupWith string       // Insert near loads from same repo/package
}

type LoadPosition int

const (
    LoadPositionTop     LoadPosition = iota // Before all statements
    LoadPositionWithExisting                // After last existing load
    LoadPositionAuto                        // Smart placement based on content
)

// EnsureLoad ensures symbols are loaded from a module.
// Returns true if the file was modified.
func EnsureLoad(f *File, opts EnsureLoadOptions) (bool, error) {
    // 1. Find existing load for this module
    existingLoad := findLoadByModule(f.File, opts.Module)

    if existingLoad != nil {
        // 2a. Add missing symbols to existing load
        return addSymbolsToLoad(existingLoad, opts.Symbols)
    }

    // 2b. Create new load statement
    newLoad := createLoadStmt(opts.Module, opts.Symbols)

    // 3. Find insertion position
    pos := findLoadInsertPosition(f.File, opts)

    // 4. Insert the new load
    insertStatement(f.File, pos, newLoad)
    f.modified = true

    return true, nil
}

// findLoadByModule finds a LoadStmt importing from the given module.
func findLoadByModule(f *build.File, module string) *build.LoadStmt {
    for _, stmt := range f.Stmt {
        if load, ok := stmt.(*build.LoadStmt); ok {
            if load.Module.Value == module {
                return load
            }
        }
    }
    return nil
}

// addSymbolsToLoad adds symbols to an existing LoadStmt.
// Returns true if any symbols were added.
func addSymbolsToLoad(load *build.LoadStmt, symbols []string) (bool, error) {
    added := false
    existing := make(map[string]bool)

    // Build set of already-loaded symbols
    for _, to := range load.To {
        existing[to.Name] = true
    }

    // Add missing symbols
    for _, sym := range symbols {
        if !existing[sym] {
            ident := &build.Ident{Name: sym}
            load.From = append(load.From, ident)
            load.To = append(load.To, ident)
            added = true
        }
    }

    if added {
        // Sort symbols for consistency
        build.SortLoadArgs(load)
    }

    return added, nil
}

// createLoadStmt creates a new LoadStmt.
func createLoadStmt(module string, symbols []string) *build.LoadStmt {
    load := &build.LoadStmt{
        Module: &build.StringExpr{Value: module},
    }

    for _, sym := range symbols {
        ident := &build.Ident{Name: sym}
        load.From = append(load.From, ident)
        load.To = append(load.To, ident)
    }

    // Sort for consistency
    build.SortLoadArgs(load)

    return load
}

// findLoadInsertPosition determines where to insert a new load.
func findLoadInsertPosition(f *build.File, opts EnsureLoadOptions) int {
    switch opts.Position {
    case LoadPositionTop:
        // After any leading comments/docstring
        return findFirstStatementPosition(f)

    case LoadPositionWithExisting:
        // After last existing load
        return findLastLoadPosition(f) + 1

    case LoadPositionAuto:
        // Smart placement: group with similar loads
        return findSmartInsertPosition(f, opts.Module)
    }

    return 0
}

// findFirstStatementPosition returns the index for inserting at "top".
// Accounts for leading comments and docstrings.
func findFirstStatementPosition(f *build.File) int {
    for i, stmt := range f.Stmt {
        // Skip comment blocks
        if _, ok := stmt.(*build.CommentBlock); ok {
            continue
        }
        // Skip docstrings (string at top of file)
        if str, ok := stmt.(*build.StringExpr); ok && i == 0 {
            _ = str
            continue
        }
        return i
    }
    return 0
}

// findLastLoadPosition returns the index of the last load statement.
// Returns -1 if no loads exist.
func findLastLoadPosition(f *build.File) int {
    lastIdx := -1
    for i, stmt := range f.Stmt {
        if _, ok := stmt.(*build.LoadStmt); ok {
            lastIdx = i
        }
    }
    return lastIdx
}

// findSmartInsertPosition finds the best position for a new load.
// Groups loads by: external (@), workspace-relative (//), relative
func findSmartInsertPosition(f *build.File, module string) int {
    isExternal := strings.HasPrefix(module, "@")
    isWorkspace := strings.HasPrefix(module, "//")

    // Find position among similar loads
    lastSimilar := -1
    firstDifferent := -1

    for i, stmt := range f.Stmt {
        load, ok := stmt.(*build.LoadStmt)
        if !ok {
            if firstDifferent == -1 && lastSimilar >= 0 {
                firstDifferent = i
            }
            continue
        }

        modIsExternal := strings.HasPrefix(load.Module.Value, "@")
        modIsWorkspace := strings.HasPrefix(load.Module.Value, "//")

        if isExternal == modIsExternal && isWorkspace == modIsWorkspace {
            lastSimilar = i
        }
    }

    if lastSimilar >= 0 {
        return lastSimilar + 1
    }

    // No similar loads, insert at end of load block
    return findLastLoadPosition(f) + 1
}

// insertStatement inserts a statement at the given position.
func insertStatement(f *build.File, pos int, stmt build.Expr) {
    f.Stmt = append(f.Stmt, nil) // Make room
    copy(f.Stmt[pos+1:], f.Stmt[pos:])
    f.Stmt[pos] = stmt
}
```

### 6.2 remove_load Implementation

```go
// RemoveLoadOptions configures the remove_load operation.
type RemoveLoadOptions struct {
    Module  string   // If set, only remove from this module
    Symbols []string // Symbols to remove
}

// RemoveLoad removes symbols from load statements.
func RemoveLoad(f *File, opts RemoveLoadOptions) (bool, error) {
    modified := false

    for i := 0; i < len(f.File.Stmt); i++ {
        load, ok := f.File.Stmt[i].(*build.LoadStmt)
        if !ok {
            continue
        }

        // Check module filter
        if opts.Module != "" && load.Module.Value != opts.Module {
            continue
        }

        // Remove specified symbols
        changed := removeSymbolsFromLoad(load, opts.Symbols)
        if changed {
            modified = true
        }

        // If load is now empty, remove it entirely
        if len(load.From) == 0 {
            // Preserve any comments by attaching to next statement
            if i+1 < len(f.File.Stmt) && len(load.Comment().After) > 0 {
                next := f.File.Stmt[i+1]
                next.Comment().Before = append(load.Comment().After, next.Comment().Before...)
            }
            f.File.Stmt = append(f.File.Stmt[:i], f.File.Stmt[i+1:]...)
            i-- // Adjust index after removal
        }
    }

    f.modified = f.modified || modified
    return modified, nil
}

func removeSymbolsFromLoad(load *build.LoadStmt, symbols []string) bool {
    toRemove := make(map[string]bool)
    for _, s := range symbols {
        toRemove[s] = true
    }

    newFrom := make([]*build.Ident, 0, len(load.From))
    newTo := make([]*build.Ident, 0, len(load.To))

    for i := range load.From {
        if !toRemove[load.To[i].Name] {
            newFrom = append(newFrom, load.From[i])
            newTo = append(newTo, load.To[i])
        }
    }

    if len(newFrom) == len(load.From) {
        return false // Nothing removed
    }

    load.From = newFrom
    load.To = newTo
    return true
}
```

### 6.3 merge_loads Implementation

```go
// MergeLoads combines load statements from the same module.
func MergeLoads(f *File) (bool, error) {
    // Group loads by module
    loadsByModule := make(map[string][]*loadInfo)

    type loadInfo struct {
        stmt  *build.LoadStmt
        index int
    }

    for i, stmt := range f.File.Stmt {
        if load, ok := stmt.(*build.LoadStmt); ok {
            module := load.Module.Value
            loadsByModule[module] = append(loadsByModule[module], &loadInfo{load, i})
        }
    }

    modified := false

    for _, loads := range loadsByModule {
        if len(loads) <= 1 {
            continue
        }

        // Merge all into first load
        first := loads[0].stmt
        for _, info := range loads[1:] {
            // Merge symbols
            for i := range info.stmt.From {
                first.From = append(first.From, info.stmt.From[i])
                first.To = append(first.To, info.stmt.To[i])
            }

            // Merge comments
            first.Comment().Before = append(first.Comment().Before, info.stmt.Comment().Before...)

            // Mark for removal
            f.File.Stmt[info.index] = nil
        }

        // Sort and deduplicate
        build.SortLoadArgs(first)
        modified = true
    }

    // Remove nil statements
    newStmt := make([]build.Expr, 0, len(f.File.Stmt))
    for _, stmt := range f.File.Stmt {
        if stmt != nil {
            newStmt = append(newStmt, stmt)
        }
    }
    f.File.Stmt = newStmt

    f.modified = f.modified || modified
    return modified, nil
}
```

### 6.4 Load Placement Logic

```
┌─────────────────────────────────────────────────────────────────────┐
│                    Load Statement Ordering                          │
├─────────────────────────────────────────────────────────────────────┤
│                                                                     │
│  Priority order (top to bottom):                                    │
│                                                                     │
│  1. External loads (@repo//)                                        │
│     - @rules_cc//cc:defs.bzl                                       │
│     - @rules_python//python:defs.bzl                               │
│                                                                     │
│  2. Workspace-absolute loads (//)                                   │
│     - //lib:common.bzl                                             │
│     - //bazel:envoy_build_system.bzl                               │
│                                                                     │
│  3. Relative loads                                                  │
│     - :common.bzl                                                  │
│     - ../shared:utils.bzl                                          │
│                                                                     │
│  Within each group: alphabetical by module path                     │
│                                                                     │
└─────────────────────────────────────────────────────────────────────┘
```

---

## 7. Validation Pipeline

### 7.1 Validation Stages

```
┌─────────────────────────────────────────────────────────────────────┐
│                      Validation Pipeline                            │
├─────────────────────────────────────────────────────────────────────┤
│                                                                     │
│  Stage 1: Parse Validation                                          │
│  ├─ Input parses as valid Starlark                                  │
│  └─ Collect parse errors with line numbers                          │
│                                                                     │
│  Stage 2: Transformation Validation                                 │
│  ├─ Transformation completed without errors                         │
│  └─ AST is internally consistent                                    │
│                                                                     │
│  Stage 3: Output Validation (CRITICAL)                              │
│  ├─ Formatted output parses as valid Starlark                       │
│  ├─ No syntax errors introduced                                     │
│  └─ Required elements still present                                 │
│                                                                     │
│  Stage 4: Semantic Validation (Optional)                            │
│  ├─ Load statements reference valid modules                         │
│  ├─ Rules have required attributes                                  │
│  └─ Dependencies are valid labels                                   │
│                                                                     │
└─────────────────────────────────────────────────────────────────────┘
```

### 7.2 Validation Implementation

```go
// ast/validate.go

package ast

import (
    "fmt"
    "github.com/bazelbuild/buildtools/build"
)

// ValidationLevel controls validation strictness.
type ValidationLevel int

const (
    ValidationSyntax   ValidationLevel = iota // Syntax only
    ValidationSemantic                        // Syntax + semantic checks
)

// ValidationError represents a validation failure.
type ValidationError struct {
    File    string
    Line    int
    Column  int
    Message string
}

func (e *ValidationError) Error() string {
    return fmt.Sprintf("%s:%d:%d: %s", e.File, e.Line, e.Column, e.Message)
}

// Validator validates BUILD file transformations.
type Validator struct {
    Level ValidationLevel
}

// ValidateOutput validates that output is valid Starlark.
func (v *Validator) ValidateOutput(path string, content []byte) error {
    _, err := build.Parse(path, content)
    if err != nil {
        // Convert to ValidationError with position info
        return &ValidationError{
            File:    path,
            Message: fmt.Sprintf("invalid Starlark syntax: %v", err),
        }
    }
    return nil
}

// ValidateTransformation validates before and after states.
func (v *Validator) ValidateTransformation(path string, before, after []byte) error {
    // Parse both
    _, errBefore := build.Parse(path, before)
    astAfter, errAfter := build.Parse(path, after)

    // If input was valid, output must be valid
    if errBefore == nil && errAfter != nil {
        return &ValidationError{
            File:    path,
            Message: fmt.Sprintf("transformation broke syntax: %v", errAfter),
        }
    }

    // Semantic validation
    if v.Level >= ValidationSemantic {
        return v.validateSemantics(path, astAfter)
    }

    return nil
}

// validateSemantics performs semantic validation.
func (v *Validator) validateSemantics(path string, f *build.File) error {
    // Check load statements reference valid-looking modules
    for _, stmt := range f.Stmt {
        if load, ok := stmt.(*build.LoadStmt); ok {
            if err := validateLoadModule(load.Module.Value); err != nil {
                start, _ := load.Span()
                return &ValidationError{
                    File:    path,
                    Line:    start.Line,
                    Message: err.Error(),
                }
            }
        }
    }
    return nil
}

func validateLoadModule(module string) error {
    // Basic validation: must have .bzl extension
    if !strings.HasSuffix(module, ".bzl") {
        return fmt.Errorf("load module %q should have .bzl extension", module)
    }
    return nil
}
```

### 7.3 Validation Integration Points

```go
// In recipe/engine.go or equivalent

func applyFileChange(ctx *Context, change *FileChange) error {
    validator := &ast.Validator{Level: ast.ValidationSyntax}

    // Validate transformation
    if err := validator.ValidateTransformation(
        change.Path,
        change.OldContent,
        change.NewContent,
    ); err != nil {
        return fmt.Errorf("validation failed: %w", err)
    }

    // Only write if validation passes
    if !ctx.DryRun {
        return os.WriteFile(change.Path, change.NewContent, 0644)
    }

    return nil
}
```

### 7.4 Dry-Run with Validation

```go
// Enhanced dry-run that validates would-be output
func dryRunWithValidation(ctx *Context, changes []*FileChange) error {
    validator := &ast.Validator{Level: ast.ValidationSyntax}

    var errors []error
    for _, change := range changes {
        if err := validator.ValidateOutput(change.Path, change.NewContent); err != nil {
            errors = append(errors, err)
        }
    }

    if len(errors) > 0 {
        return fmt.Errorf("dry-run validation found %d errors:\n%v", len(errors), errors)
    }

    return nil
}
```

---

## 8. Migration Path

### 8.1 Phased Rollout Strategy

```
Phase 1: Parallel Implementation (Week 1-2)
├─ Add ast/ package with new implementation
├─ Keep existing text-based code unchanged
├─ Add feature flag: BAZELBUMP_AST_TRANSFORMS=1
└─ Both paths available for testing

Phase 2: Integration Testing (Week 2-3)
├─ Run both implementations on test corpus
├─ Compare outputs for equivalence
├─ Fix any discrepancies
└─ Validate on real codebases (Envoy, etc.)

Phase 3: Gradual Migration (Week 3-4)
├─ Default new codebases to AST implementation
├─ Provide --legacy-transforms flag for old behavior
├─ Collect user feedback
└─ Update documentation

Phase 4: Deprecation (Week 4+)
├─ Log warning when using legacy transforms
├─ Plan removal timeline (2-3 releases)
├─ Remove legacy code
└─ Clean up feature flags
```

### 8.2 Feature Flag Implementation

```go
// pkg/config/features.go

package config

import "os"

// Features controls feature flags.
type Features struct {
    UseASTTransforms bool
}

// LoadFeatures loads feature flags from environment.
func LoadFeatures() Features {
    return Features{
        UseASTTransforms: os.Getenv("BAZELBUMP_AST_TRANSFORMS") == "1",
    }
}

// In operation dispatch:
func (e *Engine) ensureLoad(ctx *Context, op Operation) (*FileChange, error) {
    if ctx.Features.UseASTTransforms {
        return e.ensureLoadAST(ctx, op)
    }
    return e.ensureLoadLegacy(ctx, op)
}
```

### 8.3 Backward Compatibility

| Aspect | Compatibility Approach |
|--------|------------------------|
| Recipe DSL | Unchanged - same `ensure_load()` etc. |
| Output format | May differ in whitespace (buildifier normalized) |
| Comments | Better preserved with AST approach |
| Error messages | More precise line/column info |
| Performance | Similar or better |

### 8.4 Testing Strategy

```go
// Compare legacy and AST outputs
func TestTransformEquivalence(t *testing.T) {
    testCases := loadTestCorpus("testdata/build_files/")

    for _, tc := range testCases {
        // Run legacy transform
        legacyResult, err := legacyTransform(tc.input, tc.ops)
        require.NoError(t, err)

        // Run AST transform
        astResult, err := astTransform(tc.input, tc.ops)
        require.NoError(t, err)

        // Compare (normalize whitespace)
        assertEquivalent(t, legacyResult, astResult)
    }
}
```

---

## 9. Performance Considerations

### 9.1 Parsing Overhead

| Operation | Estimated Cost | Mitigation |
|-----------|----------------|------------|
| Parse file | ~1-5ms per file | Parse once, modify, format |
| Format file | ~0.5-2ms per file | Use FormatWithoutRewriting when possible |
| Validate output | ~0.5-1ms per file | Optional, can skip for trusted transforms |

### 9.2 Memory Usage

```
Per-file overhead:
├─ AST nodes: ~10-50KB for typical BUILD file
├─ Comment storage: ~1-5KB
└─ Total: ~15-60KB per file in memory

For 1,000 files processed:
├─ Serial processing: ~15-60KB (one file at a time)
└─ Parallel processing: ~15-60MB (all files in memory)
```

### 9.3 Caching Strategies

```go
// ast/cache.go

// FileCache caches parsed AST for repeated access.
type FileCache struct {
    mu    sync.RWMutex
    cache map[string]*cachedFile

    maxSize  int
    maxAge   time.Duration
}

type cachedFile struct {
    file       *File
    content    []byte // Original content for validation
    parsedAt   time.Time
    contentSum [32]byte // SHA256 for invalidation
}

// Get returns cached file if valid, nil otherwise.
func (c *FileCache) Get(path string) *File {
    c.mu.RLock()
    defer c.mu.RUnlock()

    entry, ok := c.cache[path]
    if !ok {
        return nil
    }

    // Check freshness
    if time.Since(entry.parsedAt) > c.maxAge {
        return nil
    }

    // Validate content hasn't changed
    current, err := os.ReadFile(path)
    if err != nil || sha256.Sum256(current) != entry.contentSum {
        return nil
    }

    return entry.file
}
```

### 9.4 Benchmark Targets

```go
// ast/benchmark_test.go

func BenchmarkParseFile(b *testing.B) {
    content, _ := os.ReadFile("testdata/large_build.txt")
    b.ResetTimer()

    for i := 0; i < b.N; i++ {
        ParseFile("BUILD", content)
    }
}

// Target: < 5ms for typical BUILD file
// Target: < 20ms for largest BUILD files

func BenchmarkEnsureLoad(b *testing.B) {
    f, _ := ParseFile("BUILD", content)
    b.ResetTimer()

    for i := 0; i < b.N; i++ {
        EnsureLoad(f.Copy(), EnsureLoadOptions{
            Module:  "@rules_cc//cc:defs.bzl",
            Symbols: []string{"cc_library"},
        })
    }
}

// Target: < 1ms per operation
```

---

## 10. Test Cases

### 10.1 Critical Bug Test Case

```go
func TestEnsureLoad_MultiLineExistingLoad(t *testing.T) {
    input := `load(
    "//bazel:envoy_build_system.bzl",
    "envoy_cc_library",
    "envoy_package",
)

envoy_cc_library(name = "test")
`

    expected := `load("@rules_cc//cc:defs.bzl", "cc_library")
load(
    "//bazel:envoy_build_system.bzl",
    "envoy_cc_library",
    "envoy_package",
)

envoy_cc_library(name = "test")
`

    f, err := ParseFile("BUILD", []byte(input))
    require.NoError(t, err)

    modified, err := EnsureLoad(f, EnsureLoadOptions{
        Module:   "@rules_cc//cc:defs.bzl",
        Symbols:  []string{"cc_library"},
        Position: LoadPositionTop,
    })

    require.NoError(t, err)
    require.True(t, modified)
    assert.Equal(t, expected, string(f.Format()))

    // Validate output is valid Starlark
    _, err = build.Parse("BUILD", f.Format())
    require.NoError(t, err, "output should be valid Starlark")
}
```

### 10.2 Comment Preservation Tests

```go
func TestEnsureLoad_PreservesComments(t *testing.T) {
    tests := []struct {
        name     string
        input    string
        expected string
    }{
        {
            name: "suffix_comment",
            input: `load("//foo:bar.bzl", "rule1")  # Important!
rule1(name = "test")`,
            expected: `load("@rules_cc//cc:defs.bzl", "cc_library")
load("//foo:bar.bzl", "rule1")  # Important!
rule1(name = "test")`,
        },
        {
            name: "block_comment_before_load",
            input: `# These are our custom rules
load("//foo:bar.bzl", "rule1")
rule1(name = "test")`,
            expected: `load("@rules_cc//cc:defs.bzl", "cc_library")
# These are our custom rules
load("//foo:bar.bzl", "rule1")
rule1(name = "test")`,
        },
        {
            name: "inline_comment_in_multiline_load",
            input: `load(
    "//foo:bar.bzl",
    "rule1",  # The main rule
    "rule2",
)`,
            expected: `load("@rules_cc//cc:defs.bzl", "cc_library")
load(
    "//foo:bar.bzl",
    "rule1",  # The main rule
    "rule2",
)`,
        },
    }

    for _, tc := range tests {
        t.Run(tc.name, func(t *testing.T) {
            f, err := ParseFile("BUILD", []byte(tc.input))
            require.NoError(t, err)

            _, err = EnsureLoad(f, EnsureLoadOptions{
                Module:   "@rules_cc//cc:defs.bzl",
                Symbols:  []string{"cc_library"},
                Position: LoadPositionTop,
            })
            require.NoError(t, err)

            assert.Equal(t, tc.expected, string(f.Format()))
        })
    }
}
```

### 10.3 Edge Case Tests

```go
func TestEnsureLoad_EdgeCases(t *testing.T) {
    tests := []struct {
        name     string
        input    string
        opts     EnsureLoadOptions
        expected string
    }{
        {
            name:  "empty_file",
            input: "",
            opts:  EnsureLoadOptions{Module: "@rules_cc//cc:defs.bzl", Symbols: []string{"cc_library"}},
            expected: `load("@rules_cc//cc:defs.bzl", "cc_library")
`,
        },
        {
            name:  "file_with_only_comments",
            input: "# This is a header comment\n",
            opts:  EnsureLoadOptions{Module: "@rules_cc//cc:defs.bzl", Symbols: []string{"cc_library"}},
            expected: `# This is a header comment
load("@rules_cc//cc:defs.bzl", "cc_library")
`,
        },
        {
            name: "symbol_already_loaded",
            input: `load("@rules_cc//cc:defs.bzl", "cc_library")
cc_library(name = "test")`,
            opts:  EnsureLoadOptions{Module: "@rules_cc//cc:defs.bzl", Symbols: []string{"cc_library"}},
            expected: `load("@rules_cc//cc:defs.bzl", "cc_library")
cc_library(name = "test")`, // Unchanged
        },
        {
            name: "add_to_existing_load",
            input: `load("@rules_cc//cc:defs.bzl", "cc_library")
cc_library(name = "test")`,
            opts:  EnsureLoadOptions{Module: "@rules_cc//cc:defs.bzl", Symbols: []string{"cc_binary"}},
            expected: `load("@rules_cc//cc:defs.bzl", "cc_binary", "cc_library")
cc_library(name = "test")`,
        },
        {
            name: "docstring_at_top",
            input: `"""Build file for foo."""
cc_library(name = "foo")`,
            opts:  EnsureLoadOptions{Module: "@rules_cc//cc:defs.bzl", Symbols: []string{"cc_library"}},
            expected: `"""Build file for foo."""

load("@rules_cc//cc:defs.bzl", "cc_library")

cc_library(name = "foo")`,
        },
    }

    for _, tc := range tests {
        t.Run(tc.name, func(t *testing.T) {
            f, err := ParseFile("BUILD", []byte(tc.input))
            require.NoError(t, err)

            _, err = EnsureLoad(f, tc.opts)
            require.NoError(t, err)

            assert.Equal(t, tc.expected, string(f.Format()))
        })
    }
}
```

### 10.4 Validation Tests

```go
func TestValidation_RejectsInvalidOutput(t *testing.T) {
    validator := &Validator{Level: ValidationSyntax}

    tests := []struct {
        name    string
        content string
        wantErr bool
    }{
        {
            name:    "valid_starlark",
            content: `load("@rules_cc//cc:defs.bzl", "cc_library")`,
            wantErr: false,
        },
        {
            name:    "missing_close_paren",
            content: `load("@rules_cc//cc:defs.bzl", "cc_library"`,
            wantErr: true,
        },
        {
            name:    "mismatched_quotes",
            content: `load("@rules_cc//cc:defs.bzl', "cc_library")`,
            wantErr: true,
        },
    }

    for _, tc := range tests {
        t.Run(tc.name, func(t *testing.T) {
            err := validator.ValidateOutput("BUILD", []byte(tc.content))
            if tc.wantErr {
                assert.Error(t, err)
            } else {
                assert.NoError(t, err)
            }
        })
    }
}
```

---

## 11. Implementation Phases

### Phase 1: Foundation (Week 1)

**Goal:** Core AST parsing and formatting infrastructure

**Deliverables:**
- [ ] Add buildtools dependency
- [ ] Create `ast/` package structure
- [ ] Implement `ParseFile()` and `Format()`
- [ ] Basic validation infrastructure
- [ ] Unit tests for parsing

**Files:**
- `ast/ast.go` - Core types and parsing
- `ast/ast_test.go` - Parsing tests

### Phase 2: Load Operations (Week 2)

**Goal:** AST-aware load statement operations

**Deliverables:**
- [ ] Implement `EnsureLoad()` with proper insertion
- [ ] Implement `RemoveLoad()`
- [ ] Implement `MergeLoads()`
- [ ] Comment preservation tests
- [ ] Multi-line load tests (the critical bug fix)

**Files:**
- `ast/load.go` - Load statement operations
- `ast/load_test.go` - Load operation tests

### Phase 3: Validation (Week 2-3)

**Goal:** Robust validation pipeline

**Deliverables:**
- [ ] Output syntax validation
- [ ] Transformation validation
- [ ] Integration with file write pipeline
- [ ] Dry-run validation

**Files:**
- `ast/validate.go` - Validation logic
- `ast/validate_test.go` - Validation tests

### Phase 4: Integration (Week 3)

**Goal:** Wire AST transforms into recipe engine

**Deliverables:**
- [ ] Feature flag for AST transforms
- [ ] Update `ensure_load` operation to use AST
- [ ] Update `ensure_bazel_dep` operation
- [ ] Integration tests with recipe engine

**Files:**
- `recipe/starlark/ops_build_ast.go` - New AST-based operations
- `recipe/engine.go` - Feature flag dispatch

### Phase 5: Testing & Validation (Week 3-4)

**Goal:** Comprehensive testing on real codebases

**Deliverables:**
- [ ] Test on Envoy codebase (regression test)
- [ ] Test on other large Bazel projects
- [ ] Performance benchmarks
- [ ] Documentation updates

### Phase 6: Rollout (Week 4+)

**Goal:** Production deployment

**Deliverables:**
- [ ] Make AST transforms the default
- [ ] Deprecation warnings for legacy code
- [ ] User communication
- [ ] Monitor for issues

---

## Appendix A: Trade-off Analysis

### A.1 Parser Choice

| Option | Pros | Cons | Decision |
|--------|------|------|----------|
| buildtools/build | Comment-aware, BUILD-native, mature | Larger dependency | **Selected** |
| go.starlark.net/syntax | Smaller, official Starlark | Poor comment handling | Rejected |
| Custom parser | Full control | Maintenance burden, bugs | Rejected |

### A.2 Validation Timing

| Option | Pros | Cons | Decision |
|--------|------|------|----------|
| After each transform | Catch errors early | Performance overhead | For debugging |
| Before write only | Faster execution | Delayed error detection | **Default** |
| Optional flag | Flexibility | Configuration complexity | Supported |

### A.3 Output Formatting

| Option | Pros | Cons | Decision |
|--------|------|------|----------|
| Full buildifier rewrite | Consistent style | May change unrelated code | Optional |
| Minimal formatting | Preserves style | Less consistent | **Default** |
| No formatting | Zero changes | May have odd whitespace | Not recommended |

---

## Appendix B: Error Message Examples

```
# Syntax error in output
error: transformation broke syntax in BUILD:15:3
  | 15 | load(@rules_cc//cc:defs.bzl", "cc_library")
  |      ^ missing quote

# Validation failure
error: ensure_load validation failed
  file: source/common/BUILD
  operation: ensure_load("cc_library", "@rules_cc//cc:defs.bzl")
  cause: output is not valid Starlark
  details: unexpected token "(" at line 5

# Comment loss warning (optional)
warning: transformation may have lost comments
  file: source/common/BUILD
  comments before: 5
  comments after: 4
  missing: "# Important note about deps"
```

---

## References

1. [Buildifier Source](https://github.com/bazelbuild/buildtools)
2. [Starlark Language Spec](https://github.com/bazelbuild/starlark/blob/master/spec.md)
3. [go.starlark.net Documentation](https://pkg.go.dev/go.starlark.net)
4. [Gazelle BUILD File Generation](https://github.com/bazelbuild/bazel-gazelle)
5. Envoy Migration Analysis - `docs/design/envoy-migration-analysis.md`
