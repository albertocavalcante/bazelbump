# Bazelbump Roadmap: Envoy Migration Support

## Goal

Enable reliable migration of large projects like Envoy through multiple Bazel versions (e.g., 7.x → 8.x → 9.x) with confidence.

## Current State Assessment (Updated 2025-02-05)

### What Works Well ✅
- **Pattern detection** - Found all 3,258 native rules in Envoy's 1,696 BUILD files
- **Recipe composition** - Starlark-based recipes with steps and operations
- **AST-aware transformations** - `ast` package handles multi-line loads correctly
- **Syntax validation** - `ast.ParseFileContent` validates output after modifications
- **Capability-based security** - `fs`, `bcr`, `exec` permissions
- **Flag database** - Version awareness from 0.17.1 to 9.0.0
- **Checkpoint/resume** - State management for long migrations
- **BCR queries** - `bcr.latest_stable()`, `bcr.versions()`, etc.
- **Exec module** - `exec.run()` for custom commands
- **Recipe testing** - `bazelbump recipe test` with skytest integration

### Remaining Gaps

| Gap | Impact | Priority |
|-----|--------|----------|
| No bisect functionality | Can't find breaking version | **P0** |
| No incremental version bumping | Must jump versions atomically | **P1** |
| Limited documentation | Hard to author complex recipes | **P1** |
| No staged rollout | All-or-nothing migrations | **P2** |

---

## ~~Phase 0: Fix Foundations~~ ✅ DONE

The AST-aware BUILD file transformations are implemented:
- `ast/ast.go` - File wrapper with modification tracking
- `ast/load.go` - `EnsureLoad`, `RemoveLoad`, `MergeLoads`, `SortLoads`
- `ast/format.go` - Output formatting
- Uses `starlark-syntax-go/bzl` for proper parsing
- All recipe operations use AST (see `recipe/starlark/executor.go`)
- Syntax validation after each modification

Tests confirm the Envoy multi-line bug is fixed:
```
=== RUN   TestEnvoyBugReproduction
--- PASS: TestEnvoyBugReproduction (0.00s)
```

---

## Phase 1: Bisect Command (P0)

### 2.1 Bisect Command

**Purpose**: Binary search to find which Bazel version breaks the build.

```
bazelbump bisect [flags]

Flags:
  --from VERSION       Known-good Bazel version
  --to VERSION         Known-bad Bazel version (or "latest")
  --test-cmd CMD       Command to test (default: "bazel build //...")
  --target TARGET      Build target (default: "//...")
  --auto               Run test automatically at each step
  --manual             Prompt user for good/bad verdict
  --max-steps N        Maximum bisect iterations
  --skip-prereleases   Skip prerelease versions
```

**Example workflow**:
```bash
# Find which version between 7.0.0 and 8.0.0 breaks the build
$ bazelbump bisect --from 7.0.0 --to 8.0.0 --auto

Bisecting: 12 versions between 7.0.0 and 8.0.0
Testing 7.3.0... PASS (exit 0)
Testing 7.5.0... FAIL (exit 1)
Testing 7.4.0... PASS (exit 0)
Testing 7.4.1... FAIL (exit 1)

First bad version: 7.4.1
Breaking changes in 7.4.1:
  - Flag --incompatible_foo removed
  - Native rule 'cc_library' requires explicit load
```

**Implementation**:
```go
// cmd/bazelbump/bisect.go
type BisectConfig struct {
    FromVersion    string
    ToVersion      string
    TestCommand    string
    Auto           bool
    SkipPrerelease bool
}

func runBisect(cfg BisectConfig) (*BisectResult, error) {
    versions := getVersionsBetween(cfg.FromVersion, cfg.ToVersion)
    return binarySearch(versions, cfg.TestCommand)
}
```

### 2.2 Version Bump Strategy

**Purpose**: Orchestrate migration through intermediate versions.

**New recipe builtins**:
```starlark
# Get migration path between versions
path = bazel.migration_path("7.0.0", "9.0.0")
# Returns: ["7.0.0", "7.1.0", "7.2.0", ..., "8.0.0", "8.1.0", ..., "9.0.0"]

# Get breaking changes for a version
changes = bazel.breaking_changes("8.0.0")
# Returns: struct with removed_flags, deprecated_flags, new_requirements

# Check if version requires specific migration
needs = bazel.requires_migration("8.0.0", "autoload")
# Returns: True if version requires explicit loads
```

**Bump recipe pattern**:
```starlark
# recipes/version-bump.star
def _bump_impl(ctx):
    current = project.context().bazel_version
    target = ctx.args.get("target", bazel.latest_lts())

    path = bazel.migration_path(current, target)

    for version in path:
        changes = bazel.breaking_changes(version)

        if changes.requires_autoload:
            # Apply autoload migration
            pass

        if changes.removed_flags:
            # Clean up flags
            pass

        # Test at this version
        result = exec.run("bazelisk test //...", env={"USE_BAZEL_VERSION": version})
        if not result.success:
            return fail("Build failed at version " + version)

        checkpoint("Migrated to " + version)

    return success("Migrated from " + current + " to " + target)
```

### 2.3 Staged Rollout Support

**Problem**: All-or-nothing migrations are risky for large codebases.

**Solution**: Directory-scoped recipe execution.

```starlark
# Apply to subset of directories
recipe(
    name = "staged-autoload",
    scope = ["src/core/**", "src/common/**"],  # Only these dirs
    exclude = ["src/core/legacy/**"],
    steps = [...],
)
```

**Or via CLI**:
```bash
# Migrate only specific directories
bazelbump recipe run autoload-migration --include "src/core/**" --dry-run

# Verify, then expand
bazelbump recipe run autoload-migration --include "src/**" --dry-run
```

---

## Phase 3: Documentation & Recipes (P2)

### 3.1 Documentation Overhaul

**Structure**:
```
docs/
├── user/
│   ├── getting-started.md      # Quick start (exists)
│   ├── commands.md             # CLI reference (exists)
│   ├── recipes.md              # Using recipes (exists)
│   ├── authoring-recipes.md    # Writing recipes (exists, needs expansion)
│   └── recipe-reference.md     # API reference (exists, needs completion)
├── guides/
│   ├── migrating-to-bzlmod.md          # WORKSPACE → MODULE.bazel
│   ├── migrating-native-rules.md       # Autoload migration
│   ├── migrating-major-versions.md     # 7.x → 8.x → 9.x strategy
│   ├── bisecting-failures.md           # Finding breaking versions
│   └── large-project-strategies.md     # Envoy-scale tips
├── cookbook/
│   ├── common-patterns.md              # Recipe patterns
│   ├── custom-validators.md            # Using exec for validation
│   └── ci-integration.md               # GitHub Actions, etc.
└── reference/
    ├── builtins-complete.md            # Every builtin with examples
    ├── flag-database.md                # How flags work
    └── capability-system.md            # Security model
```

### 3.2 Recipe Library

**Migration recipes needed**:

| Recipe | Purpose | Status |
|--------|---------|--------|
| `autoload-migration` | Add explicit loads for native rules | Exists (buggy) |
| `bzlmod-migration` | WORKSPACE → MODULE.bazel | Exists |
| `flag-cleanup` | Remove deprecated flags | Exists |
| `version-bump` | Orchestrated version migration | **NEW** |
| `cc-rules-migration` | Migrate to rules_cc | **NEW** |
| `python-rules-migration` | Migrate to rules_python | **NEW** |
| `java-rules-migration` | Migrate to rules_java | **NEW** |
| `proto-rules-migration` | Migrate to rules_proto | **NEW** |
| `test-rules-migration` | Migrate test rules | **NEW** |

**Validation recipes**:

| Recipe | Purpose | Status |
|--------|---------|--------|
| `validate-loads` | Check all loads are explicit | **NEW** |
| `validate-deps` | Check MODULE.bazel completeness | **NEW** |
| `validate-flags` | Check .bazelrc for deprecated flags | **NEW** |

### 3.3 Envoy-Specific Guide

Create `docs/guides/envoy-migration.md`:
- Envoy's specific challenges (size, complexity)
- Recommended migration order
- Known issues and workarounds
- Step-by-step walkthrough
- Troubleshooting common failures

---

## Phase 4: Advanced Features (P3)

### 4.1 Parallel Execution

For large repos, process files in parallel:
```go
// Process BUILD files concurrently
pool := NewWorkerPool(runtime.NumCPU())
for _, file := range buildFiles {
    pool.Submit(func() { processFile(file) })
}
pool.Wait()
```

### 4.2 Incremental Mode

Only process changed files since last run:
```bash
bazelbump recipe run autoload-migration --incremental
# Only processes files changed since last checkpoint
```

### 4.3 Remote Caching

Cache analysis results for large repos:
```bash
bazelbump analyze --to 9.0.0 --cache ~/.bazelbump/cache
# Subsequent runs reuse cached pattern matches
```

---

## Implementation Priority

### Sprint 1: Fix Load Statement Bug
1. [ ] Create `pkg/buildfile/` package
2. [ ] Integrate buildifier for AST operations
3. [ ] Update `ensure_load` operation
4. [ ] Add syntax validation
5. [ ] Test against Envoy subset

### Sprint 2: Bisect Command
1. [ ] Implement `bazelbump bisect` command
2. [ ] Add version enumeration from flag database
3. [ ] Integrate with bazelisk for version switching
4. [ ] Add `--auto` mode with test execution
5. [ ] Document bisect workflow

### Sprint 3: Version Bump Support
1. [ ] Add `bazel.migration_path()` builtin
2. [ ] Add `bazel.breaking_changes()` builtin
3. [ ] Create `version-bump` recipe template
4. [ ] Add staged rollout support (--include/--exclude)
5. [ ] Test end-to-end version migration

### Sprint 4: Documentation & Recipes
1. [ ] Complete API reference documentation
2. [ ] Write migration guides
3. [ ] Create recipe cookbook
4. [ ] Add Envoy-specific guide
5. [ ] Create rule-family migration recipes

---

## Success Criteria

For Envoy migration to be considered "supported":

1. **Correctness**: `ensure_load` works on 100% of BUILD files (not 4%)
2. **Discoverability**: `bazelbump bisect` finds breaking version in <10 steps
3. **Incrementality**: Can migrate 7→8→9 with checkpoints at each major version
4. **Reliability**: Syntax validation catches errors before commit
5. **Documentation**: Clear guide for Envoy-scale migrations

---

## Open Questions

1. **Buildifier vs go-starlark**: Which to use for AST? Buildifier is more mature but external dependency.

2. **Version source**: Use BCR, GitHub releases, or local flag database for version enumeration?

3. **Test isolation**: Should bisect use hermetic Bazel versions or rely on bazelisk?

4. **Checkpoint granularity**: Per-file, per-directory, or per-version checkpoints for staged rollout?

---

*Last updated: 2025-02-05*
