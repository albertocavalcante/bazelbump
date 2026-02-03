# Getting Started

Get running with bazelbump in 5 minutes.

## What is bazelbump?

bazelbump detects breaking changes when upgrading Bazel versions. It scans your `.bazelrc` files and reports:

- **Removed flags** - Flags that no longer exist in the target version
- **Deprecated flags** - Flags marked for removal in future versions
- **Changed defaults** - Incompatible flags whose default values flipped

## Installation

### Quick Setup (recommended)

```bash
git clone https://github.com/albertocavalcante/bazelbump.git
cd bazelbump
just setup  # Downloads flag data and installs bazelbump
```

This:
1. Downloads the flag database from registry.build
2. Installs `bazelbump` to `~/.local/bin`

If `~/.local/bin` is not in your PATH, the installer will show you how to add it.

### Alternative Installation Methods

#### Install to $GOPATH/bin

If you prefer using Go's standard install location:

```bash
git clone https://github.com/albertocavalcante/bazelbump.git
cd bazelbump
just sync-flags
just install-gopath
```

Make sure `$GOPATH/bin` is in your PATH.

#### System-wide Install

```bash
git clone https://github.com/albertocavalcante/bazelbump.git
cd bazelbump
just sync-flags
just install-system  # Installs to /usr/local/bin (requires sudo)
```

#### Manual Build

```bash
git clone https://github.com/albertocavalcante/bazelbump.git
cd bazelbump
just sync-flags
go build -o bazelbump ./cmd/bazelbump
./bazelbump --help
```

### Pre-built Binaries

Download from [GitHub Releases](https://github.com/albertocavalcante/bazelbump/releases).

### Verify Installation

```bash
bazelbump version
# bazelbump dev (commit: unknown, built: unknown)
```

## Quick Start

### 1. Sync Flag Data

bazelbump needs flag data to analyze your project:

```bash
just sync-flags
```

This downloads flag information for Bazel versions 0.17.1 through 9.0.0+.

### 2. Analyze Your Project

Check what changes are needed for a Bazel upgrade:

```bash
cd /path/to/your/bazel/project

# Analyze upgrade to Bazel 9.0.0
bazelbump analyze --to 9.0.0
```

Output:

```
==> Analyzing upgrade from Bazel 8.0.0 to 9.0.0
    Project: /home/user/myproject
    Files scanned: 3

.bazelrc
  .bazelrc:12: removed --experimental_enable_bzlmod
    hint: Remove --experimental_enable_bzlmod; this experimental flag was removed in Bazel 9.0.0
  .bazelrc:15: removed --enable_workspace
    hint: Remove --enable_workspace; this flag was removed in Bazel 9.0.0

user.bazelrc
  user.bazelrc:8: changed --incompatible_disable_starlark_host_transitions
    hint: Default for --incompatible_disable_starlark_host_transitions changed from "false" to "true" in Bazel 9.0.0

[WARN] Found 3 issue(s): 2 removed, 1 changed defaults
```

### 3. Fix Issues

Address each issue based on the hints:

1. **Removed flags** - Delete the flag from your `.bazelrc`
2. **Deprecated flags** - Plan to remove before upgrading further
3. **Changed defaults** - Either remove the flag (accept new default) or set it explicitly

### 4. Verify

Run analyze again to confirm all issues are resolved:

```bash
bazelbump analyze --to 9.0.0
# [OK] No issues found!
```

## Common Workflows

### Compare Two Versions

See all flag changes between versions:

```bash
bazelbump diff --from 7.0.0 --to 8.0.0
```

### List Incompatible Flags

Review flags that may require code changes:

```bash
bazelbump flags --version 9.0.0 --incompatible
```

### CI Integration

Add to your CI pipeline to prevent flag regressions:

```yaml
# GitHub Actions
- name: Check Bazel compatibility
  run: bazelbump analyze --to 9.0.0
```

Exit code 1 indicates issues were found.

### JSON Output

For scripting and automation:

```bash
# Get JSON report
bazelbump analyze --to 9.0.0 --json > report.json

# Count issues
bazelbump analyze --to 9.0.0 --json | jq '.flag_issues | length'

# List removed flags
bazelbump diff --from 8.0.0 --to 9.0.0 --json | jq '.removed[].name'
```

## Next Steps

- [Command Reference](./commands.md) - Full documentation for all commands
- [Troubleshooting](./troubleshooting.md) - Solutions to common issues
