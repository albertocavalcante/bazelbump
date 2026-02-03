# bazelbump Handoff

**Date:** 2025-02-03
**Status:** MVP functional, ready for CLI integration and polish

## What's Built

### Core Packages

| Package | Status | Description |
|---------|--------|-------------|
| `flagdb/` | ✅ Complete | Load, query, diff Bazel flags across 93 versions |
| `bazelrc/` | ✅ Complete | Parse .bazelrc files with config/inheritance support |
| `analysis/` | ✅ Complete | Detect flag issues (removed, deprecated, changed) |
| `transform/` | ✅ Complete | Generate and apply patches for flag issues |
| `recipe/` | ✅ Complete | Multi-step, multi-file migration framework |
| `internal/version/` | ✅ Complete | Bazel version parsing and comparison |

### CLI (`cmd/bazelbump/`)

| Command | Status | Description |
|---------|--------|-------------|
| `analyze` | ✅ Working | Detect issues for version upgrade |
| `diff` | ✅ Working | Show flag changes between versions |
| `flags` | ✅ Working | List flags with filters |
| `version` | ✅ Working | Show version info |
| `recipe` | ❌ Not wired | Recipe commands not in CLI yet |
| `apply` | ❌ Not implemented | Apply transforms to fix issues |

### Tooling

| Tool | Status | Description |
|------|--------|-------------|
| `tools/sync-flags/` | ✅ Working | Sync flag data from registry.build |
| `tools/lint/` | ✅ Configured | golangci-lint, staticcheck |
| `justfile` | ✅ Complete | build, test, install, dev, ci, release |

### Documentation

| Doc | Status | Location |
|-----|--------|----------|
| User docs | ✅ Complete | `docs/user/` (getting-started, commands, troubleshooting) |
| ADRs | ✅ Complete | `docs/decisions/` (7 ADRs) |
| Milestones | ✅ Complete | `docs/milestones/` (M0-M8) |
| Recipe design | ✅ Complete | `docs/design/recipe-system.md` |
| README | ✅ Complete | Project overview |

## What Needs to Be Done

### High Priority (Complete the MVP)

#### 1. Wire Recipe Commands to CLI

Add recipe subcommands to `cmd/bazelbump/main.go`:

```bash
bazelbump recipe list                           # List available recipes
bazelbump recipe show <name>                    # Show recipe details
bazelbump recipe run <name> [--dry-run]         # Run a recipe
bazelbump recipe run <name> --step <step-id>    # Run specific step
```

**Files to modify:**
- `cmd/bazelbump/main.go` - Add recipe command handling

**Reference:**
- `recipe/engine.go` - Engine.List(), Engine.Get(), Engine.Run()

#### 2. Add `apply` Command

Apply transforms from analysis to fix issues:

```bash
bazelbump apply --to 9.0.0 [--dry-run]
```

**Implementation:**
1. Run analysis to get issues
2. Generate patches using `transform.GeneratePatches()`
3. Apply patches using `transform.ApplyPatches()`

**Files to modify:**
- `cmd/bazelbump/main.go` - Add apply command

#### 3. Test on Real Projects

Test bazelbump on actual Bazel projects:
- A project with native rules (sh_binary, cc_library, etc.)
- A project upgrading from Bazel 8 to 9
- Edge cases: multiple .bazelrc files, configs, etc.

### Medium Priority (Polish)

#### 4. Improve Load Statement Deduplication

Current `recipe/engine.go` adds load statements but doesn't:
- Merge with existing loads from the same path
- Handle multiple rules from same repository efficiently

Example improvement:
```starlark
# Current (adds separate loads):
load("@rules_cc//cc:defs.bzl", "cc_binary")
load("@rules_cc//cc:defs.bzl", "cc_library")

# Better (merged):
load("@rules_cc//cc:defs.bzl", "cc_binary", "cc_library")
```

#### 5. Add Buildifier Integration

Use buildifier for:
- Formatting after edits
- Detecting native rule usage (buildifier has `--lint=warn --warnings=native-*`)
- Canonical load statement ordering

#### 6. Snapshot/Rollback System

Implement safety features from M3 milestone:
- Create snapshots before applying changes
- Rollback if something goes wrong
- Store in `~/.local/share/bazelbump/snapshots/`

#### 7. Better Error Messages

Add context to errors:
- Which file failed
- Which line/rule caused the issue
- Suggested fixes

### Lower Priority (Future)

#### 8. Starlark API Analysis

Detect deprecated Starlark APIs (not just flags):
- `native.existing_rule()` → `native.existing_rules().get()`
- `cfg = "host"` → `cfg = "exec"`
- Struct providers → proper provider definitions

#### 9. LLM Integration

Add LLM-assisted features:
- `bazelbump explain <flag>` - Explain what a flag does
- `bazelbump diagnose` - Diagnose build errors
- Complex migration suggestions

#### 10. WORKSPACE to bzlmod Recipe

Create recipe for migrating from WORKSPACE to MODULE.bazel:
- Convert `http_archive` to `bazel_dep`
- Handle overrides
- Migrate toolchains

## Key Files to Understand

### Entry Points

```
cmd/bazelbump/main.go      # CLI entry point
recipe/engine.go           # Recipe execution
analysis/analysis.go       # Issue detection
transform/transform.go     # Patch generation
```

### Data Flow

```
1. User runs: bazelbump analyze --to 9.0.0

2. flagdb.Load() loads flag data from data/flags/

3. analysis.Analyzer scans .bazelrc files:
   - Parses with bazelrc.ParseFile()
   - Compares flags with flagdb.Diff()
   - Returns []FlagIssue

4. (Future) transform.GeneratePatches() creates patches

5. (Future) transform.ApplyPatches() modifies files
```

### Recipe Flow

```
1. User runs: bazelbump recipe run autoload-migration

2. recipe.Engine.Run() executes steps:
   Step 1: Scan BUILD files for native rules
   Step 2: Add bazel_dep to MODULE.bazel
   Step 3: Add load() to BUILD files

3. Each step can be dry-run or applied
```

## Testing

```bash
# Run all tests
just test

# Run specific package tests
go test -v ./recipe/...
go test -v ./analysis/...

# Build and test CLI
just build-cli
./bin/bazelbump analyze --to 9.0.0
./bin/bazelbump diff --from 8.0.0 --to 9.0.0
```

## Dependencies

**Runtime:** None (zero dependencies in go.mod)

**Data:** Flag data in `data/flags/` (sync with `just sync-flags`)

**Dev tools:** In `tools/lint/go.mod` (golangci-lint, staticcheck)

## Architecture Decisions

Key ADRs to understand:
- **ADR-001**: Airgap-first (no external services required)
- **ADR-007**: No daemon, SQLite default
- **ADR-004**: Plain Go structs (not protobuf) for simplicity

## Contact

Repository: https://github.com/albertocavalcante/bazelbump
