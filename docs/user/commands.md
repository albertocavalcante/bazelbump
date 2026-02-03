# Command Reference

Complete documentation for all bazelbump commands.

## Overview

```
bazelbump <command> [flags]

Commands:
  analyze   Analyze a project for Bazel upgrade issues
  apply     Apply transforms to fix detected flag issues
  diff      Show flag changes between Bazel versions
  flags     List flags for a Bazel version
  recipe    Run migration recipes
  version   Show bazelbump version
```

## analyze

Analyze a project for Bazel upgrade compatibility issues. Scans `.bazelrc` files and reports flags that are removed, deprecated, or have changed defaults.

```bash
bazelbump analyze --to <version> [flags]
```

### Flags

| Flag | Required | Default | Description |
|------|----------|---------|-------------|
| `--to` | Yes | - | Target Bazel version |
| `--from` | No | auto | Source Bazel version |
| `--path` | No | `.` | Project directory to analyze |
| `--json` | No | `false` | Output as JSON |

### Examples

```bash
# Analyze upgrade to Bazel 9.0.0
bazelbump analyze --to 9.0.0

# Explicit source version
bazelbump analyze --from 7.0.0 --to 9.0.0

# Analyze a specific directory
bazelbump analyze --to 9.0.0 --path ~/projects/myapp

# JSON output for CI/scripting
bazelbump analyze --to 9.0.0 --json
```

### Exit Codes

| Code | Meaning |
|------|---------|
| 0 | No issues found |
| 1 | Issues found |

### JSON Output

```json
{
  "project_path": "/path/to/project",
  "from_version": "8.0.0",
  "to_version": "9.0.0",
  "files_scanned": 3,
  "flag_issues": [
    {
      "flag_name": "experimental_enable_bzlmod",
      "issue_type": "removed",
      "file": ".bazelrc",
      "line": 12,
      "command": "build",
      "config": "",
      "raw_flag": "--experimental_enable_bzlmod",
      "suggestion": "Remove --experimental_enable_bzlmod..."
    }
  ]
}
```

Issue types: `removed`, `deprecated`, `default_changed`

---

## apply

Apply automatic transforms to fix detected flag issues. This runs analysis first, then generates and applies patches.

```bash
bazelbump apply --to <version> [flags]
```

### Flags

| Flag | Required | Default | Description |
|------|----------|---------|-------------|
| `--to` | Yes | - | Target Bazel version |
| `--from` | No | auto | Source Bazel version |
| `--path` | No | `.` | Project directory |
| `--dry-run` | No | `false` | Preview changes without applying |

### Examples

```bash
# Preview what would change
bazelbump apply --to 9.0.0 --dry-run

# Apply fixes
bazelbump apply --to 9.0.0

# Specific versions
bazelbump apply --from 7.0.0 --to 9.0.0
```

---

## diff

Show flag changes between two Bazel versions. Useful for understanding what changed before upgrading.

```bash
bazelbump diff --from <version> --to <version> [flags]
```

### Flags

| Flag | Required | Default | Description |
|------|----------|---------|-------------|
| `--from` | Yes | - | Source Bazel version |
| `--to` | Yes | - | Target Bazel version |
| `--json` | No | `false` | Output as JSON |

### Examples

```bash
# Show changes from 8.0.0 to 9.0.0
bazelbump diff --from 8.0.0 --to 9.0.0

# JSON output
bazelbump diff --from 8.0.0 --to 9.0.0 --json

# Count removed flags
bazelbump diff --from 8.0.0 --to 9.0.0 --json | jq '.removed | length'
```

### JSON Output

```json
{
  "from_version": "8.0.0",
  "to_version": "9.0.0",
  "added": [
    {
      "name": "new_flag",
      "type": "boolean",
      "default": "false",
      "description": "Description of the flag"
    }
  ],
  "removed": [
    {
      "name": "old_flag",
      "type": "boolean",
      "default": "true",
      "description": "Description of the flag"
    }
  ],
  "changed": [
    {
      "name": "incompatible_flag",
      "change_type": "default_changed",
      "old_default": "false",
      "new_default": "true"
    }
  ]
}
```

---

## flags

List Bazel flags for a specific version. Supports filtering by flag category.

```bash
bazelbump flags --version <version> [flags]
```

### Flags

| Flag | Required | Default | Description |
|------|----------|---------|-------------|
| `--version` | Yes | - | Bazel version |
| `--deprecated` | No | `false` | Show only deprecated flags |
| `--incompatible` | No | `false` | Show only `--incompatible_*` flags |
| `--experimental` | No | `false` | Show only `--experimental_*` flags |
| `--json` | No | `false` | Output as JSON |

Filters are mutually exclusive. If none specified, all flags are shown.

### Examples

```bash
# List all flags for Bazel 9.0.0
bazelbump flags --version 9.0.0

# List incompatible flags (most likely to cause issues)
bazelbump flags --version 9.0.0 --incompatible

# List deprecated flags as JSON
bazelbump flags --version 8.0.0 --deprecated --json

# Count experimental flags
bazelbump flags --version 9.0.0 --experimental --json | jq 'length'
```

### JSON Output

```json
[
  {
    "name": "keep_going",
    "type": "boolean",
    "default": "false",
    "description": "Continue as much as possible after an error."
  }
]
```

---

## recipe

Run migration recipes. Recipes are multi-step Starlark scripts that automate complex migrations.

```bash
bazelbump recipe <subcommand> [flags]
```

### Subcommands

#### recipe list

List all available recipes.

```bash
bazelbump recipe list [--json]
```

**Flags:**

| Flag | Description |
|------|-------------|
| `--json` | Output as JSON |

**Example:**

```bash
bazelbump recipe list
```

#### recipe show

Show details about a specific recipe.

```bash
bazelbump recipe show <name> [--json]
```

**Flags:**

| Flag | Description |
|------|-------------|
| `--json` | Output as JSON |

**Example:**

```bash
bazelbump recipe show autoload-migration
```

#### recipe run

Execute a recipe.

```bash
bazelbump recipe run <name> [flags]
bazelbump recipe run ./path/to/recipe.star [flags]
```

**Flags:**

| Flag | Default | Description |
|------|---------|-------------|
| `--dry-run` | `false` | Preview changes without applying |
| `--path` | `.` | Project directory |
| `--step` | - | Run only a specific step by ID |
| `--verbose` | `false` | Show detailed output |
| `--json` | `false` | Output as JSON |

**Examples:**

```bash
# Preview changes
bazelbump recipe run autoload-migration --dry-run

# Apply changes
bazelbump recipe run autoload-migration

# Run against specific directory
bazelbump recipe run autoload-migration --path ~/myproject

# Run a custom recipe file
bazelbump recipe run ./my-recipe.star --dry-run

# Run only one step
bazelbump recipe run autoload-migration --step detect-native-rules
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

See [Recipes](recipes.md) for detailed documentation.

---

## version

Show bazelbump version information.

```bash
bazelbump version
```

### Output

```
bazelbump v0.1.0
  commit: abc1234
  built:  2025-01-15T10:00:00Z
  go:     go1.23.0
```

---

## Environment Variables

| Variable | Description |
|----------|-------------|
| `BAZELBUMP_DATA_DIR` | Override flag data directory location |
| `NO_COLOR` | Disable colored output (any value) |

## Data Directory

bazelbump looks for flag data in these locations (in order):

1. `$BAZELBUMP_DATA_DIR` (if set)
2. `./data/flags/` (relative to current directory)
3. `<executable-dir>/data/flags/` (relative to bazelbump binary)

Sync flag data with:

```bash
just sync-flags
```

## Global Behavior

### Color Output

- Colors are enabled when stdout is a terminal
- Colors are disabled when piping to files or other commands
- Set `NO_COLOR` to disable colors explicitly

### Error Handling

- Invalid arguments print help and exit with code 1
- Missing flag data prints a clear error message
- Unknown Bazel versions are reported with available versions listed
