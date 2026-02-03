# Recipes

Recipes are multi-step migration scripts that automate complex Bazel upgrade tasks. They scan your codebase, detect patterns, make targeted edits, and optionally verify the changes.

## Why Use Recipes?

Upgrading Bazel often requires coordinated changes across multiple files:

- Adding `load()` statements to BUILD files when native rules are deprecated
- Adding `bazel_dep()` entries to MODULE.bazel
- Removing deprecated flags from .bazelrc
- Updating rule syntax or attributes

Doing this manually is tedious and error-prone. Recipes automate these multi-step migrations with:

- **Detection** - Find all files that need changes
- **Idempotent edits** - Safe to run multiple times
- **Dry-run mode** - Preview changes before applying
- **Rollback** - Revert all changes if something fails
- **Composition** - Build complex migrations from simple parts

## Quick Start

```bash
# List available recipes
bazelbump recipe list

# Preview what a recipe would change
bazelbump recipe run autoload-migration --dry-run

# Apply the recipe
bazelbump recipe run autoload-migration

# Run a custom recipe file
bazelbump recipe run ./my-recipe.star --dry-run
```

## Recipe Commands

### `bazelbump recipe list`

List all available recipes, including built-in and custom recipes discovered from project and user directories.

```bash
bazelbump recipe list [--json] [--path <dir>]
```

Example output:

```
==> Available recipes:

  NAME                 SOURCE   VERSION   DESCRIPTION
  -------------------  -------  --------  -----------
  my-custom-recipe     project  1.0.0     Project-specific migration
  autoload-migration   builtin  1.0.0     Migrate native rules to explicit load statements

Total: 2 recipe(s)

Recipe search paths:
  - /path/to/project/.bazelbump/recipes
  - /path/to/project/recipes
  - ~/.config/bazelbump/recipes
```

The `SOURCE` column shows where each recipe was discovered:
- `project` - Found in the project's `.bazelbump/recipes/` or `recipes/` directory
- `user` - Found in the user's `~/.config/bazelbump/recipes/` directory
- `builtin` - Embedded in the bazelbump binary

### `bazelbump recipe show`

Show detailed information about a recipe, including its steps.

```bash
bazelbump recipe show <name>
bazelbump recipe show ./path/to/recipe.star
```

Example:

```bash
bazelbump recipe show autoload-migration
```

Output:

```
==> Recipe: autoload-migration

  Version:     1.0.0
  Description: Migrate native rules to explicit load statements for Bazel 9.0+
  Bazel:       >= 8.0.0

Steps: (4 total)

  1. Detect native rule usage (scan)
     ID: detect-native-rules
     Scan BUILD files for cc_*, java_*, py_*, sh_* rules

  2. Add bazel_dep entries to MODULE.bazel (edit)
     ID: add-module-deps
     Depends on: detect-native-rules

  3. Add load statements for native rules (edit)
     ID: add-load-statements
     Depends on: add-module-deps

  4. Verify build works (verify)
     ID: verify-build
     Depends on: add-load-statements
```

### `bazelbump recipe run`

Execute a recipe against your project.

```bash
bazelbump recipe run <name> [flags]
bazelbump recipe run ./path/to/recipe.star [flags]
```

**Flags:**

| Flag | Description |
|------|-------------|
| `--dry-run` | Preview changes without applying them |
| `--path <dir>` | Project directory (default: current directory) |
| `--step <id>` | Run only a specific step by ID |
| `--verbose` | Show detailed output |
| `--json` | Output results as JSON |

**Examples:**

```bash
# Dry-run to see what would change
bazelbump recipe run autoload-migration --dry-run

# Run against a specific directory
bazelbump recipe run autoload-migration --path ~/myproject

# Run only the detection step
bazelbump recipe run autoload-migration --step detect-native-rules

# Run a custom recipe file
bazelbump recipe run ./recipes/custom-migration.star
```

## Built-in Recipes

### autoload-migration

Migrates native Bazel rules to explicit `load()` statements, required for Bazel 9.0+ where autoload is disabled by default.

**What it does:**

1. Scans BUILD files for native rule usage (cc_*, java_*, py_*, sh_*, proto_*)
2. Adds required `bazel_dep()` entries to MODULE.bazel
3. Adds `load()` statements to BUILD files that use native rules
4. Optionally verifies the build still works

**Usage:**

```bash
# Preview changes
bazelbump recipe run autoload-migration --dry-run

# Apply changes
bazelbump recipe run autoload-migration
```

## Recipe Discovery

Recipes are discovered from multiple locations in priority order:

| Priority | Location | Description |
|----------|----------|-------------|
| 1 | Explicit path | `bazelbump recipe run ./my-recipe.star` |
| 2 | Project recipes | `.bazelbump/recipes/*.star` |
| 3 | Project recipes (alt) | `recipes/*.star` |
| 4 | User recipes | `~/.config/bazelbump/recipes/*.star` |
| 5 | Built-in recipes | Embedded in bazelbump binary |

**Name resolution:**

- If you specify a `.star` extension or a path, bazelbump loads that file directly
- Otherwise, bazelbump searches for a recipe by name in the discovery locations
- Later sources don't override earlier ones with the same name

### Setting Up Project Recipes

Create project-specific recipes in your repository:

```bash
mkdir -p .bazelbump/recipes
```

Add your `.star` recipe files to this directory. They will be discovered automatically when running `bazelbump recipe list` or `bazelbump recipe run`.

### Setting Up User Recipes

Create user-level recipes that are available across all projects:

```bash
# Linux/macOS (uses XDG_CONFIG_HOME or ~/.config)
mkdir -p ~/.config/bazelbump/recipes

# Or set XDG_CONFIG_HOME to customize the location
export XDG_CONFIG_HOME=/custom/config
mkdir -p $XDG_CONFIG_HOME/bazelbump/recipes
```

User recipes are useful for personal automation scripts or organization-wide standards.

### Priority and Overriding

When multiple recipes have the same name:

1. **Project recipes win** - A recipe in `.bazelbump/recipes/` overrides one in `recipes/`
2. **Project beats user** - A project recipe overrides a user recipe with the same name
3. **User beats built-in** - A user recipe overrides a built-in recipe

This allows you to customize or replace built-in recipes without modifying the bazelbump binary.

## Recipe Output

### Text Output (default)

```
==> Running recipe: autoload-migration (dry-run)

  [OK] Detect native rule usage
      Found 12 matches in 5 files
  [OK] Add bazel_dep entries to MODULE.bazel
      Made 4 changes
  [OK] Add load statements for native rules
      Made 8 changes
  [SKIP] Verify build works
      Skipped in dry-run mode

Changes:
  [would change] MODULE.bazel
      Add bazel_dep for rules_cc
  [would change] src/BUILD
      Add load statement for cc_binary
  [would change] lib/BUILD
      Add load statement for cc_library

[OK] Dry run complete. 3 file(s) would be modified.
    Run without --dry-run to apply changes.
```

### JSON Output

```json
{
  "recipe": "autoload-migration",
  "success": true,
  "steps_run": 4,
  "step_results": [
    {
      "step_id": "detect-native-rules",
      "step_name": "Detect native rule usage",
      "success": true,
      "description": "Found 12 matches in 5 files"
    }
  ],
  "changes": [
    {
      "path": "MODULE.bazel",
      "description": "Add bazel_dep for rules_cc"
    }
  ]
}
```

## Error Handling

### Failure Modes

Recipes can specify how to handle failures:

- **rollback** (default) - Revert all changes and stop
- **stop** - Stop execution but keep changes made so far
- **continue** - Log the error and continue with remaining steps

### Dry-Run Mode

Always run with `--dry-run` first to preview changes:

```bash
bazelbump recipe run my-recipe --dry-run
```

This shows exactly what would be modified without making any changes.

## Writing Custom Recipes

Recipes are written in [Starlark](https://github.com/bazelbuild/starlark), the same language used for BUILD files. See [Authoring Recipes](authoring-recipes.md) for a complete guide.

Quick example:

```starlark
# my-recipe.star
recipe(
    name = "remove-deprecated-flag",
    version = "1.0.0",
    description = "Remove deprecated experimental_enable_bzlmod flag",
    steps = [
        edit(
            id = "remove-flag",
            name = "Remove deprecated flag",
            operations = [
                remove_flag("experimental_enable_bzlmod"),
            ],
        ),
    ],
)
```

## See Also

- [Authoring Recipes](authoring-recipes.md) - How to write custom recipes
- [Recipe Reference](recipe-reference.md) - Complete API reference
- [Starlark Recipe Spec](../design/starlark-recipes-spec.md) - Design specification
