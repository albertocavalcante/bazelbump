# bazelbump Handoff

**Date:** 2026-02-03
**Status:** Phase 1 complete, entering Phase 2 (production readiness)

## What's Built

### Core Packages

| Package | Status | Description |
|---------|--------|-------------|
| `flagdb/` | ✅ Complete | Load, query, diff Bazel flags across 93 versions |
| `bazelrc/` | ✅ Complete | Parse .bazelrc files with config/inheritance support |
| `analysis/` | ✅ Complete | Detect flag issues (removed, deprecated, changed) |
| `transform/` | ✅ Complete | Generate and apply patches for flag issues |
| `recipe/` | ✅ Complete | Multi-step, multi-file migration framework |
| `recipe/starlark/` | ✅ Complete | Starlark recipe DSL with three-phase execution |
| `internal/version/` | ✅ Complete | Bazel version parsing and comparison |

### CLI (`cmd/bazelbump/`)

| Command | Status | Description |
|---------|--------|-------------|
| `analyze` | ✅ Working | Detect issues for version upgrade |
| `apply` | ✅ Working | Apply transforms to fix issues |
| `diff` | ✅ Working | Show flag changes between versions |
| `flags` | ✅ Working | List flags with filters |
| `recipe list` | ✅ Working | List available recipes |
| `recipe show` | ✅ Working | Show recipe details |
| `recipe run` | ✅ Working | Run a recipe (supports .star files) |
| `version` | ✅ Working | Show version info |

### Starlark Recipe System

| Component | Status | Description |
|-----------|--------|-------------|
| Interpreter | ✅ Complete | Parse and execute .star recipe files |
| Three-phase execution | ✅ Complete | Interpret → Validate → Execute |
| Recipe discovery | ✅ Complete | Project, user, builtin sources |
| Standard library | ✅ Complete | `@stdlib//` with rule patterns |
| Operations | ✅ Complete | ensure_load, remove_flag, ensure_bazel_dep, run_command, etc. |
| Load system | ✅ Complete | Relative, absolute, stdlib imports |

### Tooling

| Tool | Status | Description |
|------|--------|-------------|
| `tools/sync-flags/` | ✅ Working | Sync flag data from registry.build |
| `tools/lint/` | ✅ Configured | golangci-lint, staticcheck |
| `justfile` | ✅ Complete | build, test, install (cross-platform), dev, ci, release |

### Documentation

| Doc | Status | Location |
|-----|--------|----------|
| User docs | ✅ Complete | `docs/user/` (getting-started, commands, recipes, authoring) |
| Roadmap | ✅ Complete | `docs/ROADMAP.md` |
| Design philosophy | ✅ Complete | `docs/design/standing-on-shoulders.md` |
| Starlark spec | ✅ Complete | `docs/design/starlark-recipes-spec.md` |
| Research | ✅ Complete | `docs/research/` (starlark-tools, extensibility) |
| ADRs | ✅ Complete | `docs/decisions/` (7 ADRs) |
| README | ✅ Complete | Project overview |

---

## What's Next

**See [ROADMAP.md](docs/ROADMAP.md) for the full development plan.**

### Phase 1: 100% Starlark Authoring ✅ COMPLETE

Users can now write fully custom recipes:

- **File I/O Builtins** - `read_file()`, `glob()`, `list_dir()`, `file_exists()`
- **Regex Module** - starlib's `re` module embedded
- **BUILD File Parsing** - `parse_build_file()` for structured access
- **Custom Functions** - User-defined scanners and transforms with `impl=` and `operations=`
- **run_command** - Execute shell commands (formatters, linters, external tools)
- **Authoring UX** - `recipe validate`, `recipe builtins`, `recipe init`

### Phase 2: Production Readiness (Current Focus)

- Real-world testing on OSS projects
- Built-in recipes (autoload, bzlmod migration)
- **Formatter recipes added**: `format-buildifier.star`, `format-skyfmt.star`, `format-compare.star`
- Snapshot & rollback

### Phase 3: Ecosystem Integration

- GitHub Actions
- IDE integration (VS Code)
- Recipe catalog

---

## Key Files to Understand

### Entry Points

```
cmd/bazelbump/main.go           # CLI entry point
recipe/starlark/interpreter.go  # Starlark DSL interpreter
recipe/starlark/executor.go     # Three-phase execution engine
recipe/starlark/discovery.go    # Recipe discovery
analysis/analysis.go            # Issue detection
transform/transform.go          # Patch generation
```

### Data Flow: analyze

```
1. User runs: bazelbump analyze --to 9.0.0

2. flagdb.Load() loads flag data from data/flags/

3. analysis.Analyzer scans .bazelrc files:
   - Parses with bazelrc.ParseFile()
   - Compares flags with flagdb.Diff()
   - Returns []FlagIssue

4. Results displayed or output as JSON
```

### Data Flow: recipe run

```
1. User runs: bazelbump recipe run autoload-migration.star

2. Recipe discovery finds the recipe file

3. Starlark interpreter parses and executes:
   Phase 1 (Interpret): Build operation graph
   Phase 2 (Validate): Check all operations valid
   Phase 3 (Execute): Apply changes or dry-run

4. Results displayed with changes made
```

---

## Testing

```bash
# Run all tests
just test

# Run specific package tests
go test -v ./recipe/starlark/...
go test -v ./analysis/...

# Build and test CLI
just install
bazelbump analyze --to 9.0.0
bazelbump recipe list
bazelbump recipe run examples/recipes/cleanup-flags.star --dry-run
```

---

## Installation

```bash
# Quick setup (recommended)
git clone https://github.com/albertocavalcante/bazelbump.git
cd bazelbump
just setup  # Syncs flags and installs to ~/.local/bin

# Other options
just install-gopath   # Install to $GOPATH/bin
just install-system   # Install to /usr/local/bin (sudo)
just install ~/bin    # Custom directory
```

---

## Dependencies

**Runtime:** `go.starlark.net` (Starlark interpreter)

**Data:** Flag data in `data/flags/` (sync with `just sync-flags`)

**Dev tools:** In `tools/lint/go.mod` (golangci-lint, staticcheck)

---

## Architecture Decisions

Key ADRs to understand:
- **ADR-001**: Airgap-first (no external services required)
- **ADR-007**: No daemon, SQLite default
- **ADR-004**: Plain Go structs (not protobuf) for simplicity

Key design docs:
- **Standing on Shoulders**: How we learned from Copybara, Orion, OpenRewrite, etc.
- **Starlark Recipes Spec**: Three-phase execution model

---

## Contact

Repository: https://github.com/albertocavalcante/bazelbump
