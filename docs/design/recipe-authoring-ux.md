# Recipe Authoring UX Improvement Plan

*Objective: Make writing bazelbump recipes as pleasant as writing BUILD files*

---

## Current Pain Points

1. **Opaque errors** - Starlark errors don't always point to the right place
2. **No discoverability** - Users don't know what builtins are available
3. **No scaffolding** - Users start from scratch every time
4. **Limited examples** - Not enough real-world recipes to learn from
5. **No testing story** - Hard to test recipes before running on real code
6. **Verbose output** - Hard to see what actually changed
7. **No IDE support** - No syntax highlighting or auto-completion

---

## Improvement Plan

### Phase A: Better Errors & Validation (High Impact, Quick Wins)

#### A1. Recipe Validation Command
```bash
bazelbump recipe validate my-recipe.star

# Output:
# ✓ Syntax valid
# ✓ All required fields present
# ✓ Step dependencies form valid DAG
# ✓ All referenced builtins exist
# ⚠ Warning: step "fix-flags" has no depends_on but uses ctx.outputs
#
# Recipe is valid!
```

#### A2. Enhanced Error Messages
```
# Before:
Error: starlark: invalid syntax

# After:
my-recipe.star:15:3: error: missing required parameter 'id' in scan()

    14 |     steps = [
  > 15 |         scan(patterns = ["**/*.go"]),
       |         ^~~~
    16 |     ],

hint: Every step needs a unique 'id' for dependency tracking.
example: scan(id = "find-go-files", patterns = ["**/*.go"])
```

#### A3. Builtin Documentation in Errors
```
# When user calls a builtin wrong:
my-recipe.star:8:5: error: ensure_load() missing required argument 'load_from'

ensure_load(rule, load_from, file=None)
  Ensures a load statement exists for a rule.

  Args:
    rule: The rule name to load (e.g., "cc_binary")
    load_from: The .bzl file path (e.g., "@rules_cc//cc:defs.bzl")
    file: Optional BUILD file path (default: all BUILD files)

  Example:
    ensure_load("cc_binary", "@rules_cc//cc:defs.bzl")
```

### Phase B: Discoverability & Documentation

#### B1. Interactive Builtin Explorer
```bash
bazelbump recipe builtins

# Output:
# Recipe Builtins
# ===============
#
# Step Types:
#   scan(id, patterns, files, impl)     Scan files for patterns
#   edit(id, operations, depends_on)    Apply transformations
#   verify(id, command, depends_on)     Run verification command
#
# Operations:
#   ensure_load(rule, load_from)        Add load() statement
#   ensure_bazel_dep(name, version)     Add bazel_dep() to MODULE.bazel
#   remove_flag(flag)                   Remove flag from .bazelrc
#   replace_text(old, new, files)       Replace text in files
#
# File I/O:
#   read_file(path, default)            Read file contents
#   file_exists(path)                   Check if file exists
#   glob(pattern, exclude)              Find files by pattern
#   list_dir(path, recursive)           List directory contents
#   parse_build_file(path)              Parse BUILD file structure
#
# Use 'bazelbump recipe builtins <name>' for detailed help.

bazelbump recipe builtins ensure_load

# Output:
# ensure_load(rule, load_from, file=None)
# ========================================
# Ensures a load statement exists for a Bazel rule.
#
# If the load already exists, this is a no-op (idempotent).
# If there's an existing load from the same path, the symbol is added to it.
#
# Arguments:
#   rule (str): The rule/symbol name to load
#   load_from (str): The .bzl file to load from
#   file (str, optional): Specific BUILD file, or all BUILD files if None
#
# Example:
#   ensure_load("cc_binary", "@rules_cc//cc:defs.bzl")
#   ensure_load("py_test", "@rules_python//python:defs.bzl", "tests/BUILD")
#
# See also: ensure_bazel_dep, parse_build_file
```

#### B2. Recipe Cheatsheet
Create a quick-reference document users can print/bookmark:
- All builtins with signatures
- Common patterns
- Example snippets

#### B3. Contextual Help in Recipes
```starlark
# Users can add this to see available context:
def my_scanner(ctx):
    print(dir(ctx))  # Shows: ['project_path', 'dry_run', 'outputs']
    print(help(ctx)) # Shows detailed help
```

### Phase C: Scaffolding & Templates

#### C1. Recipe Init Command
```bash
bazelbump recipe init my-recipe

# Interactive prompts:
# ? Recipe name: my-recipe
# ? Description: Remove deprecated flags for Bazel 9.0
# ? Recipe type:
#   > Flag cleanup (remove deprecated/removed flags)
#     Load migration (add load() for native rules)
#     Custom (blank template)
#
# Created: my-recipe.star
#
# Next steps:
#   1. Edit my-recipe.star
#   2. Test with: bazelbump recipe run my-recipe.star --dry-run
#   3. Apply with: bazelbump recipe run my-recipe.star
```

#### C2. Recipe Templates
```bash
bazelbump recipe init --template flag-cleanup my-recipe
bazelbump recipe init --template load-migration my-recipe
bazelbump recipe init --template custom-scanner my-recipe
```

Templates provide starting points:
- `flag-cleanup` - Remove flags template
- `load-migration` - Add load() statements
- `bazel-dep` - Add bazel_dep() entries
- `custom-scanner` - Custom scan + transform

#### C3. Generated Boilerplate
```starlark
# Generated my-recipe.star
"""
My Recipe - Remove deprecated flags for Bazel 9.0

This recipe:
1. Scans .bazelrc files for deprecated flags
2. Removes them

Usage:
  bazelbump recipe run my-recipe.star --dry-run
  bazelbump recipe run my-recipe.star
"""

recipe(
    name = "my-recipe",
    version = "1.0.0",
    description = "Remove deprecated flags for Bazel 9.0",
    steps = [
        # TODO: Add your steps here
        #
        # Example scan step:
        # scan(
        #     id = "find-flags",
        #     patterns = ["*.bazelrc"],
        # ),
        #
        # Example edit step:
        # edit(
        #     id = "remove-flags",
        #     operations = [
        #         remove_flag("experimental_enable_bzlmod"),
        #     ],
        # ),
    ],
)
```

### Phase D: Testing & Debugging

#### D1. Recipe Test Command
```bash
bazelbump recipe test my-recipe.star

# Runs recipe against test fixtures:
#
# Testing my-recipe.star...
#
# Test: basic_removal
#   Input:  testdata/basic/.bazelrc
#   Expect: testdata/basic/.bazelrc.expected
#   Result: ✓ PASS
#
# Test: multiple_flags
#   Input:  testdata/multiple/.bazelrc
#   Expect: testdata/multiple/.bazelrc.expected
#   Result: ✓ PASS
#
# 2/2 tests passed
```

#### D2. Test Fixtures Convention
```
my-recipe.star
my-recipe_test/
  basic/
    .bazelrc           # Input
    .bazelrc.expected  # Expected output
  multiple_flags/
    .bazelrc
    .bazelrc.expected
```

#### D3. Verbose/Debug Mode
```bash
bazelbump recipe run my-recipe.star --verbose

# Output:
# [interpret] Loading my-recipe.star
# [interpret] Found recipe: my-recipe v1.0.0
# [interpret] Steps: find-flags, remove-flags
# [validate] Checking step dependencies...
# [validate] Resolving file patterns...
# [validate]   *.bazelrc -> [.bazelrc, user.bazelrc]
# [execute] Running step: find-flags
# [execute]   Scanning 2 files...
# [execute]   Found 3 matches
# [execute] Running step: remove-flags
# [execute]   Applying 3 operations...
# [execute]   Modified: .bazelrc (2 changes)
# [execute]   Modified: user.bazelrc (1 change)
# [done] Recipe completed: 2 files modified
```

#### D4. Step-by-Step Execution
```bash
# Run just the scan step to see what it finds
bazelbump recipe run my-recipe.star --step find-flags

# Output shows scan results without applying changes
```

### Phase E: Better Output & Feedback

#### E1. Diff-Style Output
```bash
bazelbump recipe run my-recipe.star --dry-run

# Output:
# ==> Running recipe: my-recipe (dry-run)
#
# .bazelrc
# --------
# @@ -12,2 +12,0 @@
# - build --experimental_enable_bzlmod
# - build --incompatible_enable_cc_toolchain_resolution
#
# user.bazelrc
# ------------
# @@ -5,1 +5,0 @@
# - build --experimental_worker_multiplex
#
# Summary: 3 lines removed from 2 files
# Run without --dry-run to apply changes.
```

#### E2. JSON Output for Tooling
```bash
bazelbump recipe run my-recipe.star --dry-run --json

{
  "recipe": "my-recipe",
  "version": "1.0.0",
  "dry_run": true,
  "changes": [
    {
      "file": ".bazelrc",
      "hunks": [
        {"start": 12, "removed": ["build --experimental_enable_bzlmod"]}
      ]
    }
  ],
  "summary": {
    "files_modified": 2,
    "lines_removed": 3,
    "lines_added": 0
  }
}
```

#### E3. Progress Indicators
```bash
bazelbump recipe run autoload-migration

# Output with progress:
# ==> Running recipe: autoload-migration
#
# [1/4] Scanning BUILD files...
#       Found 47 files with native rules
# [2/4] Adding bazel_dep entries...
#       Added rules_cc, rules_java, rules_python
# [3/4] Adding load statements...
#       ████████████████████░░░░ 42/47 files
# [4/4] Verifying build...
#       Running: bazel build //...
#
# ✓ Recipe completed successfully
#   Modified 47 BUILD files
#   Modified MODULE.bazel
```

### Phase F: IDE Support (Future)

#### F1. JSON Schema for Recipes
Generate JSON schema that IDEs can use for validation.

#### F2. Language Server
Basic LSP server for:
- Syntax highlighting
- Auto-completion of builtins
- Hover documentation
- Error diagnostics

#### F3. VS Code Extension
- Syntax highlighting for .star files
- Recipe snippets
- Run recipe from editor

---

## Implementation Priority

| Phase | Effort | Impact | Priority |
|-------|--------|--------|----------|
| A: Better Errors | Medium | High | 🔴 P0 |
| B: Discoverability | Low | High | 🔴 P0 |
| C: Scaffolding | Low | Medium | 🟡 P1 |
| D: Testing | Medium | High | 🟡 P1 |
| E: Better Output | Low | Medium | 🟡 P1 |
| F: IDE Support | High | Medium | 🟢 P2 |

---

## Quick Wins to Implement First

1. **`bazelbump recipe validate`** - Validate without running
2. **`bazelbump recipe builtins`** - List available builtins
3. **`bazelbump recipe init`** - Scaffold new recipes
4. **Diff-style output** - Show changes as diffs
5. **Better error messages** - Include hints and examples

---

## Success Metrics

- Time to write first working recipe: < 10 minutes
- Error messages lead to fix: > 90% of the time
- Users can discover builtins without docs: Yes
- Recipes are testable: Yes
