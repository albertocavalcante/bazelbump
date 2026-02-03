# bazelbump

A CLI tool to safely upgrade Bazel versions by detecting deprecated flags, removed features, and incompatible changes.

## Why bazelbump?

Upgrading Bazel is tedious. Breaking changes are scattered across release notes, GitHub issues, and source code. bazelbump consolidates this into a single command that tells you exactly what to fix.

## Features

- **Flag Analysis** - Detects removed, deprecated, and changed flags in your `.bazelrc` files
- **Version Diff** - Shows all flag changes between any two Bazel versions
- **Flag Explorer** - Lists flags by category (incompatible, experimental, deprecated)
- **Migration Recipes** - Automate complex upgrades with Starlark-based recipes
- **CI Ready** - JSON output and meaningful exit codes for automation

## Installation

### Quick Install (recommended)

```bash
git clone https://github.com/albertocavalcante/bazelbump.git
cd bazelbump
just setup  # Syncs flag data and installs to ~/.local/bin
```

### Alternative Install Methods

```bash
# Install to $GOPATH/bin (all platforms)
just install-gopath

# Install to /usr/local/bin (Unix only, system-wide)
just install-system

# Manual build
go build -o bazelbump ./cmd/bazelbump
```

**Install locations by platform:**
- **macOS/Linux**: `~/.local/bin/bazelbump`
- **Windows**: `%LOCALAPPDATA%\Programs\bazelbump\bazelbump.exe`

### Pre-built Binaries

Download from [GitHub Releases](https://github.com/albertocavalcante/bazelbump/releases).

## Quick Start

```bash
# Analyze your project for Bazel 9.0 upgrade
cd /path/to/your/project
bazelbump analyze --to 9.0.0

# Apply automatic fixes
bazelbump apply --to 9.0.0 --dry-run  # Preview
bazelbump apply --to 9.0.0            # Apply

# Run migration recipes
bazelbump recipe run --dry-run autoload-migration
```

Example output:

```
==> Analyzing upgrade from Bazel 8.0.0 to 9.0.0
    Project: /home/user/myproject
    Files scanned: 3

.bazelrc
  .bazelrc:12: removed --experimental_enable_bzlmod
    hint: Remove --experimental_enable_bzlmod; this experimental flag was removed in Bazel 9.0.0

[WARN] Found 1 issue(s): 1 removed
```

## Commands

| Command | Description |
|---------|-------------|
| `analyze --to VERSION` | Detect issues for upgrade |
| `apply --to VERSION` | Apply automatic fixes for detected issues |
| `diff --from VERSION --to VERSION` | Show flag changes between versions |
| `flags --version VERSION` | List flags with filters |
| `recipe list\|show\|run` | Manage and run migration recipes |
| `version` | Show bazelbump version |

See [Command Reference](docs/user/commands.md) for full documentation.

## Recipes

Recipes automate complex multi-step migrations. For example, migrating native rules to explicit load statements for Bazel 9.0+:

```bash
# Preview changes
bazelbump recipe run autoload-migration --dry-run

# Apply changes
bazelbump recipe run autoload-migration
```

Write custom recipes in Starlark:

```starlark
# my-recipe.star
recipe(
    name = "remove-deprecated-flags",
    version = "1.0.0",
    steps = [
        edit(
            id = "cleanup",
            operations = [
                remove_flag("experimental_enable_bzlmod"),
            ],
        ),
    ],
)
```

See [Recipes Documentation](docs/user/recipes.md) for more.

## Documentation

- [Getting Started](docs/user/getting-started.md) - Installation and basic usage
- [Command Reference](docs/user/commands.md) - All commands and options
- [Recipes](docs/user/recipes.md) - Using migration recipes
- [Authoring Recipes](docs/user/authoring-recipes.md) - Writing custom recipes
- [Troubleshooting](docs/user/troubleshooting.md) - Common issues and solutions

## Requirements

- Go 1.21+
- [just](https://github.com/casey/just) (for development commands)

## Development

```bash
just dev        # Format, lint, test
just build-cli  # Build binary to ./bin/
just test       # Run tests
just ci         # Full CI check
```

## How It Works

bazelbump maintains a database of Bazel flags across versions, synced from [registry.build](https://registry.build). When you run `analyze`, it:

1. Computes the diff between your source and target Bazel versions
2. Scans all `.bazelrc` files in your project
3. Reports any flags that are removed, deprecated, or have changed defaults

## License

Apache 2.0
