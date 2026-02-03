# bazelbump

A CLI tool to safely upgrade Bazel versions by detecting deprecated flags, removed features, and incompatible changes.

## Why bazelbump?

Upgrading Bazel is tedious. Breaking changes are scattered across release notes, GitHub issues, and source code. bazelbump consolidates this into a single command that tells you exactly what to fix.

## Features

- **Flag Analysis** - Detects removed, deprecated, and changed flags in your `.bazelrc` files
- **Version Diff** - Shows all flag changes between any two Bazel versions
- **Flag Explorer** - Lists flags by category (incompatible, experimental, deprecated)
- **CI Ready** - JSON output and meaningful exit codes for automation

## Quick Start

```bash
# Install
git clone https://github.com/albertocavalcante/bazelbump.git
cd bazelbump
just sync-flags  # Download flag database
just install     # Install to $GOPATH/bin

# Analyze your project
cd /path/to/your/project
bazelbump analyze --to 9.0.0
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
| `diff --from VERSION --to VERSION` | Show flag changes between versions |
| `flags --version VERSION` | List flags with filters |
| `version` | Show bazelbump version |

See [Command Reference](docs/user/commands.md) for full documentation.

## Documentation

- [Getting Started](docs/user/getting-started.md) - Installation and basic usage
- [Command Reference](docs/user/commands.md) - All commands and options
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
