# Milestone 4: Transform Engine

**Target:** Weeks 6-7
**Status:** Not Started
**Depends On:** M3 (Safety)

## Objective

Implement deterministic transformations and the apply workflow.

## Deliverables

### 1. Transform Primitives

```go
// internal/transform/types.go
type Transform interface {
    Type() TransformType
    File() string
    Apply(content []byte) ([]byte, error)
    Describe() string
}

type TransformType int
const (
    TransformReplace TransformType = iota
    TransformInsert
    TransformDelete
    TransformRegex
)

// Specific transforms
type ReplaceTransform struct {
    Search  string
    Replace string
    All     bool
}

type InsertTransform struct {
    Position InsertPosition  // Before, After, AtLine
    Anchor   string          // Line to anchor to
    Content  string
}

type DeleteTransform struct {
    Pattern string  // Line pattern to delete
}
```

### 2. Transform Engine

```go
// internal/transform/engine.go
type Engine struct {
    buildifier string  // path to buildifier
}

type ApplyResult struct {
    File     string
    Original []byte
    Modified []byte
    Diff     string
    Applied  []Transform
}

func (e *Engine) Apply(ctx context.Context, file string, transforms []Transform) (*ApplyResult, error)
func (e *Engine) Preview(ctx context.Context, file string, transforms []Transform) (*ApplyResult, error)
func (e *Engine) Format(ctx context.Context, file string) error
```

### 3. Flag Transforms

```go
// internal/transform/flags.go
func GenerateFlagTransforms(issues []*FlagIssue) []Transform {
    var transforms []Transform
    for _, issue := range issues {
        switch issue.IssueType {
        case FlagRemoved:
            transforms = append(transforms, &DeleteTransform{
                Pattern: issue.Flag,
            })
        case FlagRenamed:
            transforms = append(transforms, &ReplaceTransform{
                Search:  issue.Flag,
                Replace: issue.Replacement,
            })
        }
    }
    return transforms
}
```

### 4. Starlark Transforms

```go
// internal/transform/starlark.go
func GenerateStarlarkTransforms(issues []*StarlarkIssue) []Transform {
    var transforms []Transform
    for _, issue := range issues {
        switch issue.API {
        case "native.existing_rule":
            // Complex multi-line transform
            transforms = append(transforms, &StarlarkAPITransform{
                File:        issue.File,
                Line:        issue.Line,
                Pattern:     issue.Pattern,
                Replacement: issue.Replacement,
            })
        }
    }
    return transforms
}
```

### 5. Native Rule Load Statements

```go
// internal/transform/autoload.go
type LoadStatementGenerator struct {
    ruleMapping map[string]LoadInfo
}

type LoadInfo struct {
    Repository string
    File       string
    Symbols    []string
}

var DefaultRuleMapping = map[string]LoadInfo{
    "cc_library": {
        Repository: "@rules_cc",
        File:       "//cc:defs.bzl",
        Symbols:    []string{"cc_library"},
    },
    "cc_binary": {
        Repository: "@rules_cc",
        File:       "//cc:defs.bzl",
        Symbols:    []string{"cc_binary"},
    },
    // ... more rules
}

func (g *LoadStatementGenerator) Generate(file string, usedRules []string) []Transform
```

### 6. Apply Command

```bash
# Preview changes (dry-run)
$ bazelbump apply --to 9.0.0 --dry-run

PLANNED CHANGES
===============

.bazelrc (2 changes):
  - Line 12: Delete --experimental_enable_bzlmod
  - Line 45: Replace --host_copt with --copt

tools/macros.bzl (1 change):
  - Lines 45-46: Replace native.existing_rule() pattern

//src:BUILD (1 change):
  - Line 1: Add load("@rules_cc//cc:defs.bzl", "cc_library")

Total: 4 transforms in 3 files

Run without --dry-run to apply changes.

# Apply changes
$ bazelbump apply --to 9.0.0

Creating snapshot... snap_01HQ6Z
Applying 4 transforms to 3 files...

.bazelrc
  ✓ Deleted --experimental_enable_bzlmod
  ✓ Replaced --host_copt with --copt

tools/macros.bzl
  ✓ Updated native.existing_rule() usage

//src:BUILD
  ✓ Added load statement for cc_library

Formatting with buildifier...
  ✓ Formatted 2 files

Verifying build...
  ✓ bazel build //... succeeded

SUCCESS: Applied 4 transforms
Snapshot: snap_01HQ6Z (use 'bazelbump rollback' to revert)
```

### 7. Fix Subcommands

```bash
# Fix specific categories
$ bazelbump fix flags                    # Fix flag issues only
$ bazelbump fix imports                  # Add missing load() only
$ bazelbump fix starlark                 # Fix Starlark APIs only

# Fix with filters
$ bazelbump fix imports --rule cc_library
$ bazelbump fix flags --flag experimental_enable_bzlmod

# Preview
$ bazelbump fix imports --dry-run
```

## Package Structure

```
internal/
├── transform/
│   ├── engine.go
│   ├── engine_test.go
│   ├── types.go
│   ├── flags.go
│   ├── starlark.go
│   ├── autoload.go
│   ├── diff.go
│   └── buildifier.go
│
pkg/libbazelbump/
├── apply.go
├── apply_test.go
├── fix.go
└── fix_test.go

cmd/bazelbump/
├── apply.go
└── fix.go
```

## Tasks

- [ ] Define transform interface and types
- [ ] Implement replace transform
- [ ] Implement insert transform
- [ ] Implement delete transform
- [ ] Implement transform engine
- [ ] Implement flag transforms
- [ ] Implement Starlark transforms
- [ ] Implement load statement generator
- [ ] Integrate buildifier
- [ ] Implement diff generation
- [ ] Implement `bazelbump apply` command
- [ ] Implement `bazelbump fix` subcommands
- [ ] Add dry-run support
- [ ] Integrate with safety system
- [ ] Add verification step
- [ ] Add tests for all transforms

## Acceptance Criteria

1. All transforms are deterministic
2. Transforms preserve file structure where possible
3. Dry-run accurately previews changes
4. Buildifier integration works
5. Rollback works if apply fails mid-way
6. Verification catches broken builds

## Transform Examples

### Flag Removal
```diff
- build --experimental_enable_bzlmod
```

### Flag Rename
```diff
- build --host_copt=-O2
+ build --copt=-O2
```

### Starlark API Update
```diff
- rule = native.existing_rule(name)
- if rule:
+ rules = native.existing_rules()
+ rule = rules.get(name)
+ if rule is not None:
```

### Load Statement Addition
```diff
+ load("@rules_cc//cc:defs.bzl", "cc_library", "cc_binary")
+
  cc_library(
      name = "mylib",
```

## Notes

- Determinism is critical
- Preserve comments and formatting where possible
- Handle edge cases gracefully
- Consider multi-file transforms (e.g., adding to MODULE.bazel)
