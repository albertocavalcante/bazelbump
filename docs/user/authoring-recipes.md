# Authoring Recipes

This guide explains how to write custom bazelbump recipes using Starlark.

## Getting Started

### Your First Recipe

Create a file with the `.star` extension:

```starlark
# remove-flag.star
recipe(
    name = "remove-flag",
    version = "1.0.0",
    description = "Remove a deprecated flag from .bazelrc",
    steps = [
        edit(
            id = "remove",
            name = "Remove deprecated flag",
            operations = [
                remove_flag("experimental_enable_bzlmod"),
            ],
        ),
    ],
)
```

Run it:

```bash
bazelbump recipe run ./remove-flag.star --dry-run
```

### Recipe Structure

Every recipe file must call `recipe()` exactly once. The recipe defines:

1. **Metadata** - Name, version, description, Bazel version constraints
2. **Steps** - Ordered list of scan, edit, or verify steps
3. **Failure handling** - What to do when a step fails

```starlark
recipe(
    name = "my-migration",           # Required: unique identifier
    version = "1.0.0",               # Required: semver version
    description = "What it does",    # Optional: human-readable description
    min_bazel_version = "8.0.0",     # Optional: minimum Bazel version
    max_bazel_version = "10.0.0",    # Optional: maximum Bazel version
    steps = [...],                   # Required: list of steps
    on_failure = "rollback",         # Optional: "rollback", "stop", or "continue"
)
```

## Step Types

### scan() - Detect Patterns

Scans files for regex patterns. Returns a future reference that can be used in later steps.

```starlark
detected = scan(
    id = "find-rules",                         # Required: unique step ID
    name = "Find native rules",                # Optional: display name
    files = ["BUILD", "BUILD.bazel"],          # Required: file name patterns
    patterns = [r"cc_binary\s*\("],            # Required: regex patterns
)
```

The scan result is a **future reference** - it doesn't contain actual values during recipe parsing, but is resolved during execution. You can use it in later steps:

```starlark
edit(
    id = "add-loads",
    operations = [
        ensure_load_for_detected(detected),  # Uses the scan result
    ],
)
```

### edit() - Modify Files

Makes changes to files using operations.

```starlark
edit(
    id = "update-files",                # Required: unique step ID
    name = "Update BUILD files",        # Optional: display name
    depends_on = ["find-rules"],        # Optional: step IDs that must run first
    operations = [                      # Required: list of operations
        ensure_load("cc_binary", "@rules_cc//cc:defs.bzl"),
        ensure_bazel_dep("rules_cc", "0.1.1"),
    ],
)
```

### verify() - Run Commands

Executes a shell command to verify changes worked.

```starlark
verify(
    id = "check-build",                 # Required: unique step ID
    name = "Verify build",              # Optional: display name
    depends_on = ["update-files"],      # Optional: step IDs that must run first
    command = "bazel build //...",      # Required: shell command
    timeout = "10m",                    # Optional: timeout (default: 5m)
    allow_failure = True,               # Optional: continue if command fails
)
```

## Operations Reference

Operations are the building blocks of edit steps. They follow naming conventions:

- `ensure_*` - Add if not present (idempotent)
- `remove_*` - Delete if present
- `replace_*` - Find and replace
- `update_*` - Modify existing

### BUILD File Operations

#### ensure_load()

Add a load statement to BUILD files if not already present.

```starlark
ensure_load(
    rule = "cc_binary",                          # Rule name to load
    load_from = "@rules_cc//cc:defs.bzl",        # Source file
    file = "src/BUILD",                          # Optional: specific file
)
```

Only adds the load if the rule is actually used in the file.

#### ensure_load_for_detected()

Add load statements for all rules found by a scan step.

```starlark
detected = scan(
    id = "find",
    files = ["BUILD", "BUILD.bazel"],
    patterns = [r"cc_(binary|library)\s*\("],
)

edit(
    id = "add-loads",
    operations = [
        ensure_load_for_detected(detected),      # Adds loads for all matches
    ],
)
```

#### remove_load()

Remove a rule from load statements.

```starlark
remove_load(
    rule = "deprecated_rule",            # Rule to remove
    file = "BUILD",                      # Optional: specific file
)
```

### MODULE.bazel Operations

#### ensure_bazel_dep()

Add a bazel_dep if not already present.

```starlark
ensure_bazel_dep(
    name = "rules_cc",                   # Dependency name
    version = "0.1.1",                   # Version
    dev_dependency = False,              # Optional: mark as dev dependency
)
```

#### update_bazel_dep()

Update the version of an existing bazel_dep.

```starlark
update_bazel_dep(
    name = "rules_cc",                   # Dependency name
    version = "0.2.0",                   # New version
)
```

#### remove_bazel_dep()

Remove a bazel_dep.

```starlark
remove_bazel_dep(
    name = "old_dependency",             # Dependency to remove
)
```

### .bazelrc Operations

#### remove_flag()

Remove a flag from .bazelrc.

```starlark
remove_flag(
    flag = "experimental_enable_bzlmod", # Flag name (without --)
    file = ".bazelrc",                   # Optional: file (default: .bazelrc)
    config = "build",                    # Optional: specific config section
)
```

#### replace_flag()

Replace a flag with another.

```starlark
replace_flag(
    old = "--incompatible_old_flag",     # Flag to replace
    new = "--incompatible_new_flag",     # Replacement (empty string to just remove)
    file = ".bazelrc",                   # Optional: file
)
```

#### ensure_flag()

Add a flag if not already present.

```starlark
ensure_flag(
    flag = "keep_going",                 # Flag name
    value = "true",                      # Optional: flag value
    config = "build",                    # Optional: config section (default: build)
    file = ".bazelrc",                   # Optional: file
)
```

### Generic Operations

#### replace_text()

Replace text in files.

```starlark
replace_text(
    old = "old_text",                    # Text to find
    new = "new_text",                    # Replacement text
    files = ["*.bzl", "BUILD"],          # File patterns
    regex = False,                       # Optional: treat old as regex
)
```

With regex:

```starlark
replace_text(
    old = r"version\s*=\s*\"[^\"]+\"",   # Regex pattern
    new = "version = \"2.0.0\"",         # Replacement
    files = ["MODULE.bazel"],
    regex = True,
)
```

#### copy_file()

Copy a file.

```starlark
copy_file(
    src = "templates/BUILD.template",    # Source path
    dst = "new_package/BUILD",           # Destination path
)
```

#### delete_file()

Delete a file or files matching a pattern.

```starlark
delete_file(
    path = "old_config.bzl",             # File path or glob pattern
)
```

## Error Handling

### fail()

Stop execution immediately and trigger rollback.

```starlark
if not module_bazel_exists:
    fail("MODULE.bazel not found - this recipe requires bzlmod")
```

### warn()

Log a warning but continue execution.

```starlark
warn("Flag already removed, skipping")
```

### skip()

Skip the current step and continue with the recipe.

```starlark
if no_rules_found:
    skip("No native rules detected, nothing to migrate")
```

## Step Dependencies

Steps can declare dependencies with `depends_on`. Dependencies are automatically inferred when you pass a scan result to an operation.

### Explicit Dependencies

```starlark
scan(id = "detect", ...),
edit(id = "fix", depends_on = ["detect"], ...),
verify(id = "check", depends_on = ["fix"], ...),
```

### Implicit Dependencies (Future References)

When you use a scan result, the dependency is automatic:

```starlark
detected = scan(id = "detect", ...)

edit(
    id = "fix",
    # No need for depends_on - detected creates an implicit dependency
    operations = [
        ensure_load_for_detected(detected),
    ],
)
```

## Using load() for Composition

Split complex recipes into reusable modules.

### Creating a Library

```starlark
# lib/cc_helpers.star

def cc_migration_ops(version = "0.1.1"):
    """Returns operations for CC rule migration."""
    return [
        ensure_bazel_dep("rules_cc", version),
        ensure_load("cc_binary", "@rules_cc//cc:defs.bzl"),
        ensure_load("cc_library", "@rules_cc//cc:defs.bzl"),
        ensure_load("cc_test", "@rules_cc//cc:defs.bzl"),
    ]
```

### Using the Library

```starlark
# recipes/migrate-cc.star

load("./lib/cc_helpers.star", "cc_migration_ops")

recipe(
    name = "migrate-cc",
    version = "1.0.0",
    steps = [
        edit(
            id = "migrate",
            operations = cc_migration_ops(version = "0.2.0"),
        ),
    ],
)
```

### Load Path Resolution

| Prefix | Resolution |
|--------|------------|
| `./path` | Relative to current file |
| `../path` | Relative to parent directory |
| `//path` | Relative to recipe search roots |
| `@stdlib//path` | Built-in standard library (future) |

## Testing Recipes

### Dry-Run Testing

Always test with `--dry-run` first:

```bash
bazelbump recipe run ./my-recipe.star --dry-run
```

This shows exactly what would change without modifying files.

### Step-by-Step Testing

Run individual steps to debug issues:

```bash
# Just run the scan step
bazelbump recipe run ./my-recipe.star --step detect-rules --dry-run

# Then the edit step
bazelbump recipe run ./my-recipe.star --step add-loads --dry-run
```

### Verbose Output

Get detailed output for debugging:

```bash
bazelbump recipe run ./my-recipe.star --verbose --dry-run
```

### Test Against a Copy

For destructive testing, work on a copy:

```bash
cp -r myproject myproject-test
bazelbump recipe run ./my-recipe.star --path myproject-test
```

## Best Practices

### 1. Make Operations Idempotent

Use `ensure_*` operations that check before modifying:

```starlark
# Good: won't duplicate if already present
ensure_bazel_dep("rules_cc", "0.1.1")
ensure_load("cc_binary", "@rules_cc//cc:defs.bzl")

# Avoid: might create duplicates
# (there's no raw "add" operation for this reason)
```

### 2. Use Descriptive Step Names

```starlark
# Good
edit(
    id = "add-cc-loads",
    name = "Add load statements for CC rules",
    ...
)

# Avoid
edit(id = "step2", ...)
```

### 3. Add Verification Steps

```starlark
verify(
    id = "check",
    name = "Verify build still works",
    command = "bazel build //...",
    allow_failure = True,  # Don't rollback on build failure
)
```

### 4. Handle Missing Files Gracefully

Operations like `ensure_bazel_dep` skip gracefully if MODULE.bazel doesn't exist. For critical files, check explicitly:

```starlark
scan(
    id = "check-module",
    files = ["MODULE.bazel"],
    patterns = [r"module\s*\("],
)
```

### 5. Version Your Recipes

Use semantic versioning:

```starlark
recipe(
    name = "my-migration",
    version = "1.2.0",  # Major.Minor.Patch
    ...
)
```

### 6. Document with Comments

```starlark
# This recipe migrates from WORKSPACE to bzlmod.
#
# Prerequisites:
#   - Bazel 8.0+
#   - Existing WORKSPACE file
#
# What it does:
#   1. Creates MODULE.bazel with module() call
#   2. Migrates http_archive to bazel_dep where possible
#   3. Updates .bazelrc to enable bzlmod

recipe(
    name = "workspace-to-bzlmod",
    ...
)
```

## Common Patterns

### Pattern: Conditional Operations

```starlark
detected = scan(
    id = "find-cc",
    files = ["BUILD", "BUILD.bazel"],
    patterns = [r"cc_(binary|library|test)\s*\("],
)

edit(
    id = "add-cc-dep",
    operations = [
        # Only adds if cc rules were detected (implicit dependency)
        ensure_bazel_dep("rules_cc", "0.1.1"),
        ensure_load_for_detected(detected),
    ],
)
```

### Pattern: Multi-Language Migration

```starlark
# Detect all rule types
cc_rules = scan(id = "cc", files = ["BUILD", "BUILD.bazel"],
                patterns = [r"cc_(binary|library|test)\s*\("])
py_rules = scan(id = "py", files = ["BUILD", "BUILD.bazel"],
                patterns = [r"py_(binary|library|test)\s*\("])

# Add deps based on what's used
edit(
    id = "deps",
    operations = [
        ensure_bazel_dep("rules_cc", "0.1.1"),
        ensure_bazel_dep("rules_python", "1.4.1"),
    ],
)

# Add loads for each
edit(id = "cc-loads", operations = [ensure_load_for_detected(cc_rules)])
edit(id = "py-loads", operations = [ensure_load_for_detected(py_rules)])
```

### Pattern: Flag Cleanup

```starlark
recipe(
    name = "cleanup-bazelrc",
    version = "1.0.0",
    description = "Remove deprecated flags from .bazelrc",
    steps = [
        edit(
            id = "remove-deprecated",
            name = "Remove deprecated flags",
            operations = [
                remove_flag("experimental_enable_bzlmod"),
                remove_flag("incompatible_load_python_rules_from_bzl"),
                replace_flag(
                    old = "--incompatible_disable_depset_items=false",
                    new = "",  # Just remove it
                ),
            ],
        ),
    ],
)
```

## Troubleshooting

### "recipe() must be called exactly once"

Ensure your file has exactly one `recipe()` call at the top level.

### "step depends on unknown step"

Check that `depends_on` references valid step IDs:

```starlark
scan(id = "detect", ...)  # ID is "detect"
edit(id = "fix", depends_on = ["detect"], ...)  # Must match exactly
```

### "circular dependency detected"

Check for cycles in `depends_on`:

```starlark
# This creates a cycle!
edit(id = "a", depends_on = ["b"], ...)
edit(id = "b", depends_on = ["a"], ...)
```

### Operations not making changes

- Check that file patterns match (`BUILD` vs `BUILD.bazel`)
- Verify the rule/flag is actually present
- Use `--verbose` to see what's happening
- Check that patterns use raw strings: `r"pattern\s*\("`

## See Also

- [Recipes Overview](recipes.md) - Using recipes
- [Recipe Reference](recipe-reference.md) - Complete API reference
- [Starlark Recipe Spec](../design/starlark-recipes-spec.md) - Design specification
