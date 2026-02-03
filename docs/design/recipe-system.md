# Recipe System Design

## Overview

A **Recipe** is a multi-step, multi-file transformation that migrates a codebase from one state to another. Unlike simple transforms (which operate on single files), recipes coordinate complex changes across multiple files with proper ordering, validation, and rollback support.

## Inspiration

- **OpenRewrite**: Composable recipes with LST (Lossless Semantic Trees)
- **IDE Quick Fixes**: But more powerful - can span multiple files
- **Database migrations**: Ordered steps with rollback support

## Recipe Structure

```yaml
# recipes/autoload-migration.yaml
name: autoload-migration
version: 1.0.0
description: Migrate native rules to explicit load statements for Bazel 9.0
bazel_versions:
  from: ">=8.0.0"
  to: ">=9.0.0"

# What this recipe fixes
fixes:
  - flag: incompatible_autoload_externally
  - pattern: native rule usage without load statement

# Prerequisites - must pass before recipe runs
preconditions:
  - type: file_exists
    path: MODULE.bazel
  - type: bazel_version
    min: "8.0.0"

# Ordered steps
steps:
  - id: detect-native-rules
    name: Detect native rule usage
    type: scan
    description: Find all BUILD files using native rules without load statements
    scan:
      patterns:
        - "sh_binary\\s*\\("
        - "sh_test\\s*\\("
        - "cc_binary\\s*\\("
        - "py_binary\\s*\\("
      file_types: ["BUILD", "BUILD.bazel"]
    outputs:
      - detected_rules  # List of {file, rule, line}

  - id: add-module-deps
    name: Add bazel_dep entries to MODULE.bazel
    type: edit
    description: Add required rules_* dependencies
    depends_on: [detect-native-rules]
    edit:
      file: MODULE.bazel
      operations:
        - type: ensure_bazel_dep
          condition: "detected_rules contains sh_*"
          dep:
            name: rules_shell
            version: "0.4.1"
        - type: ensure_bazel_dep
          condition: "detected_rules contains cc_*"
          dep:
            name: rules_cc
            version: "0.1.1"
        - type: ensure_bazel_dep
          condition: "detected_rules contains py_*"
          dep:
            name: rules_python
            version: "1.4.1"

  - id: add-load-statements
    name: Add load statements to BUILD files
    type: edit
    description: Add load() for each native rule usage
    depends_on: [add-module-deps]
    edit:
      for_each: detected_rules
      operations:
        - type: ensure_load
          rule: sh_binary
          load: "@rules_shell//shell:sh_binary.bzl"
        - type: ensure_load
          rule: sh_test
          load: "@rules_shell//shell:sh_test.bzl"
        - type: ensure_load
          rule: cc_binary
          load: "@rules_cc//cc:defs.bzl"
        - type: ensure_load
          rule: py_binary
          load: "@rules_python//python:defs.bzl"

  - id: verify
    name: Verify build still works
    type: verify
    description: Run bazel build to ensure changes are correct
    depends_on: [add-load-statements]
    verify:
      command: bazel build //...
      allow_failure: false

# What to do if a step fails
on_failure:
  action: rollback
  message: "Recipe failed. All changes have been rolled back."
```

## Core Concepts

### 1. Recipe
Top-level container with metadata, preconditions, and ordered steps.

### 2. Step Types

| Type | Purpose |
|------|---------|
| `scan` | Detect patterns, collect information |
| `edit` | Modify files (BUILD, MODULE.bazel, .bazelrc) |
| `verify` | Run commands to verify correctness |
| `prompt` | Ask user for input/confirmation |

### 3. Operations

**For BUILD files:**
- `ensure_load` - Add load statement if not present
- `remove_load` - Remove load statement
- `replace_rule` - Replace rule invocation

**For MODULE.bazel:**
- `ensure_bazel_dep` - Add dependency if not present
- `update_bazel_dep` - Update version
- `remove_bazel_dep` - Remove dependency

**For .bazelrc:**
- `remove_flag` - Remove a flag
- `replace_flag` - Replace flag with another
- `add_flag` - Add a new flag

### 4. Dependency Graph

Steps declare dependencies via `depends_on`. The recipe engine:
1. Builds a DAG of steps
2. Topologically sorts for execution order
3. Passes outputs from earlier steps to later ones

### 5. Dry Run Mode

Every operation supports dry-run:
- Shows what would change without modifying files
- Generates a diff preview
- Validates preconditions

## Implementation Architecture

```
recipe/
├── recipe.go          # Recipe struct, parsing
├── engine.go          # Recipe execution engine
├── step.go            # Step interface and types
├── operations/
│   ├── scan.go        # Pattern scanning
│   ├── build.go       # BUILD file operations
│   ├── module.go      # MODULE.bazel operations (uses go-bzlmod)
│   └── bazelrc.go     # .bazelrc operations
└── builtin/
    ├── autoload.go    # Autoload migration recipe
    └── bzlmod.go      # WORKSPACE → bzlmod recipe
```

## API Design

```go
// Recipe represents a multi-step migration
type Recipe struct {
    Name          string
    Version       string
    Description   string
    BazelVersions VersionRange
    Preconditions []Precondition
    Steps         []Step
    OnFailure     FailureAction
}

// Step is a single operation in a recipe
type Step interface {
    ID() string
    Name() string
    DependsOn() []string
    Execute(ctx *Context) (*StepResult, error)
    DryRun(ctx *Context) (*StepPreview, error)
}

// Context passed between steps
type Context struct {
    ProjectPath string
    FromVersion string
    ToVersion   string
    DryRun      bool
    Outputs     map[string]any  // Step outputs
    Snapshot    *Snapshot       // For rollback
}

// Engine executes recipes
type Engine struct {
    recipes map[string]*Recipe
}

func (e *Engine) Run(ctx context.Context, recipeName string, opts Options) (*Result, error)
func (e *Engine) DryRun(ctx context.Context, recipeName string) (*Preview, error)
func (e *Engine) List() []RecipeSummary
```

## Built-in Recipes

### 1. `autoload-migration`
Migrate native rules to explicit load statements.

### 2. `workspace-to-bzlmod`
Migrate from WORKSPACE to MODULE.bazel (future).

### 3. `incompatible-flag-cleanup`
Remove flags that no longer exist.

## CLI Integration

```bash
# List available recipes
bazelbump recipe list

# Show recipe details
bazelbump recipe show autoload-migration

# Dry-run a recipe
bazelbump recipe run autoload-migration --dry-run

# Run a recipe
bazelbump recipe run autoload-migration

# Run specific step only
bazelbump recipe run autoload-migration --step add-load-statements
```

## Relationship to Existing Code

| Existing | Role in Recipe System |
|----------|----------------------|
| `transform/` | Low-level operations (used by recipe steps) |
| `analysis/` | Scanning/detection (used by scan steps) |
| `bazelrc/` | .bazelrc parsing/editing |
| `flagdb/` | Flag information for validation |
| `go-bzlmod` | MODULE.bazel parsing/editing |

## Next Steps

1. **Phase 1**: Implement recipe engine with scan/edit/verify steps
2. **Phase 2**: Implement `autoload-migration` as first recipe
3. **Phase 3**: Add MODULE.bazel operations using go-bzlmod
4. **Phase 4**: Add recipe sharing/distribution
