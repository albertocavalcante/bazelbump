# bazelbump Migration Strategy & Research

*Last updated: 2026-02-04*

This document consolidates research on Bazel migration challenges, bazelisk bisect integration, version compatibility, and agentic workflow patterns to inform bazelbump's roadmap.

---

## Table of Contents

1. [Bazelisk Bisect - Core Differentiator](#1-bazelisk-bisect---core-differentiator)
2. [Bazel Version Compatibility](#2-bazel-version-compatibility)
3. [Migration Gotchas & Breaking Changes](#3-migration-gotchas--breaking-changes)
4. [Recipe Prerequisites & Version Requirements](#4-recipe-prerequisites--version-requirements)
5. [Agentic Workflow Patterns](#5-agentic-workflow-patterns)
6. [Real-World Case Study: Envoy](#6-real-world-case-study-envoy)
7. [Implementation Roadmap](#7-implementation-roadmap)

---

## 1. Bazelisk Bisect - Core Differentiator

### Overview

Bazelisk's `--bisect` flag performs a binary search across Bazel versions to identify which commit introduced a build failure. This is a **powerful debugging tool** that bazelbump can leverage to automatically diagnose migration issues.

### Command Syntax

```bash
bazelisk --bisect=<GOOD>..<BAD> <command> [arguments]
```

The `--bisect` flag must be the **first argument** to bazelisk.

### Supported Version Formats

| Format | Example | Description |
|--------|---------|-------------|
| Release versions | `6.0.0..7.0.0` | Official Bazel releases |
| Release candidates | `6.1.0..release-6.2.0rc2` | RC versions |
| Git commit hashes | `abc123..def456` | Commits on main/release branches |
| HEAD reference | `6.0.0..HEAD` | Latest commit on main branch |
| Branch names | `6.4.0..release-7.0.0` | Release branch names |

### Reverse Bisection (Finding Fixes)

Prefix the range with `~` to find the first version that **fixes** a problem:

```bash
bazelisk --bisect=~6.0.0..HEAD test //foo:bar_test
```

### Environment Variables

| Variable | Purpose |
|----------|---------|
| `BAZELISK_CLEAN` | Run `clean --expunge` between each bisect iteration |
| `BAZELISK_SHUTDOWN` | Run `shutdown` between builds (releases JVM memory) |
| `BAZELISK_GITHUB_TOKEN` | Avoid GitHub API rate limits |

### How Bisect Works Internally

Based on bazelisk source code (`core/core.go`):

1. **Get Commit List**: Uses GitHub's compare API to fetch commits between versions
2. **Verify Endpoints**: Confirms the "good" version actually works
3. **Binary Search**: Tests midpoint commits, narrowing the range
4. **Report Result**: Outputs the first bad commit URL

```go
// Simplified algorithm
left, right := 0, len(commitList)
for left < right {
    mid := (left + right) / 2
    if testWithBazelAtCommit(commitList[mid]) == 0 {
        left = mid + 1  // Success: search later commits
    } else {
        right = mid     // Failure: search earlier commits
    }
}
fmt.Printf("first bad commit: %s\n", commitList[left])
```

### Proposed bazelbump Integration

#### 1. New CLI Command: `bazelbump bisect`

```bash
# Find which Bazel version broke the build
bazelbump bisect --from 7.0.0 --to 8.0.0 --command "build //..."

# Bisect with specific target
bazelbump bisect --from 7.0.0 --to 8.0.0 --target //src:main

# Integration with analyze
bazelbump analyze --to 9.0.0 --bisect-regressions
```

#### 2. New Starlark Builtin: `bisect_versions()`

```starlark
result = bisect_versions(
    good = "7.0.0",
    bad = "8.0.0",
    command = "build //...",
    clean = True,
)

# result.culprit_commit = "abc123..."
# result.commit_url = "https://github.com/bazelbuild/bazel/commit/abc123"
# result.iterations = 7
```

#### 3. Recipe Integration

```starlark
recipe(
    name = "diagnose-migration",
    description = "Find breaking commits for Bazel upgrade",
)

scan(
    id = "find-failures",
    impl = lambda ctx: [
        {"target": t, "good": "7.0.0", "bad": "8.0.0"}
        for t in failing_targets(ctx)
    ],
)

edit(
    id = "bisect-failures",
    operations = lambda ctx: [
        bisect_and_report(
            target=r["target"],
            good=r["good"],
            bad=r["bad"]
        )
        for r in ctx.outputs["find-failures"]
    ],
)
```

### Bisect Limitations

1. **Prebuilt Binary Availability**: Only commits with prebuilt binaries in GCS can be bisected
2. **Cross-Branch Issues**: Bisecting between major versions (6.x to 7.x) may fail at merge base
3. **GitHub API Rate Limits**: Use `BAZELISK_GITHUB_TOKEN` for heavy usage
4. **Flaky Tests**: Assumes deterministic pass/fail; flaky tests produce incorrect results
5. **No Skip Mechanism**: Unlike `git bisect`, no way to skip problematic commits
6. **Long Runs**: With hundreds of commits, bisecting can take hours

---

## 2. Bazel Version Compatibility

### Detecting Available Flags

Bazelisk can dynamically detect available incompatible flags:

```bash
# Test with all incompatible flags
bazelisk --strict build //...

# Test each flag individually and report failures
bazelisk --migrate build //...
```

**Programmatic detection:**
```go
// Parse output from: bazel help <cmd> --short
re := regexp.MustCompile(`(?m)^\s*--\[no\](incompatible_\w+)$`)
```

### Version Resolution Order

Bazelisk resolves the Bazel version in this order:

1. Environment variable `USE_BAZEL_VERSION`
2. `.bazeliskrc` file containing `USE_BAZEL_VERSION`
3. `.bazelversion` file (searches current + parent directories)
4. `USE_BAZEL_FALLBACK_VERSION` environment variable
5. Default: `latest`

### .bazelversion Formats

```
7.0.0           # Exact version
4.x             # Latest release from 4.x LTS series
4.*             # Latest release or candidate from 4.x series
latest          # Latest stable release
latest-1        # Previous stable release
last_green      # Latest passing commit on main
rolling         # Latest rolling release
```

### BCR Module Compatibility

The Bazel Central Registry (BCR) provides compatibility metadata:

**MODULE.bazel `bazel_compatibility` attribute:**
```python
module(
    name = "rules_java",
    version = "9.0.0",
    bazel_compatibility = [">=6.4.0"],  # Minimum Bazel version
    compatibility_level = 1,
)
```

**Constraint types:**
- `>=X.Y.Z` - Minimum Bazel version required
- `<=X.Y.Z` - Maximum Bazel version supported
- `-X.Y.Z` - Exclude specific buggy version

**Important:** `bazel_compatibility` does NOT affect dependency resolution; it's only for user warnings.

### Proposed bazelbump Integration

```starlark
# Check if all dependencies are compatible with target version
check_compatibility(
    target_version = "9.0.0",
    fail_on_incompatible = True,
)

# Get minimum required version for all deps
min_version = get_minimum_bazel_version()
```

---

## 3. Migration Gotchas & Breaking Changes

### Bazel 8 Breaking Changes

#### WORKSPACE Disabled by Default ([#23023](https://github.com/bazelbuild/bazel/issues/23023))

- `--enable_workspace` defaults to false
- WORKSPACE and WORKSPACE.bzlmod files are no longer read automatically
- **Migration**: Add `--enable_workspace=true` temporarily or migrate to bzlmod

#### Rules "Starlarkification" ([#23809](https://github.com/bazelbuild/bazel/issues/23809))

Native rules moved to external repositories:

| Rules | New Location |
|-------|--------------|
| Android rules | `rules_android` |
| C++ rules | `rules_cc` |
| Java rules | `rules_java` |
| Protobuf rules | `protobuf` repo |
| Python rules | `rules_python` |
| Shell rules | `rules_shell` |

**Bazel 8**: Auto-loaded via `--incompatible_autoload_externally`
**Bazel 9**: Explicit load statements required

#### Key Incompatible Flags Being Flipped

| Flag | Issue | Impact |
|------|-------|--------|
| `--incompatible_autoload_externally` | Default empty in Bazel 9 | Explicit loads required |
| `--legacy_external_runfiles` | [#23574](https://github.com/bazelbuild/bazel/issues/23574) | New runfiles layout |
| `--incompatible_sandbox_hermetic_tmp` | [#19915](https://github.com/bazelbuild/bazel/issues/19915) | Stricter sandboxing |
| `--incompatible_disable_starlark_host_transitions` | [#17032](https://github.com/bazelbuild/bazel/issues/17032) | `cfg = "host"` illegal |
| `--incompatible_auto_exec_groups` | [#17134](https://github.com/bazelbuild/bazel/issues/17134) | Toolchain per action |

### Bazel 9 Upcoming Changes

- **WORKSPACE Removal**: Complete removal of WORKSPACE functionality
- **Proto Toolchain Resolution**: `--incompatible_enable_proto_toolchain_resolution` defaults to true

### bzlmod vs WORKSPACE Migration

**Timeline:**
- WORKSPACE disabled in Bazel 8 (late 2024)
- WORKSPACE removed in Bazel 9 (late 2025)

**Key Differences:**

| Aspect | WORKSPACE | bzlmod |
|--------|-----------|--------|
| Dependency resolution | Macro-based, manual | MVS algorithm, automatic |
| Repository visibility | All visible to all | Strict visibility |
| Toolchain precedence | Order-dependent | Root module priority |

**Common Migration Issues:**

1. **Bind Rules Deprecation**: No bzlmod equivalent; use `alias` rules
2. **Duplication of Singletons**: Multiple copies of toolchain types ([rules_foreign_cc#1374](https://github.com/bazel-contrib/rules_foreign_cc/pull/1374))
3. **Toolchain Control**: Any `bazel_dep` can register toolchains ([#26503](https://github.com/bazelbuild/bazel/issues/26503))

### Aspect and Transition Changes

#### Host to Exec Configuration ([#17032](https://github.com/bazelbuild/bazel/issues/17032))

```starlark
# OLD (deprecated)
my_rule = rule(
    attrs = {"tool": attr.label(cfg = "host")},
)

# NEW (required)
my_rule = rule(
    attrs = {"tool": attr.label(cfg = "exec")},
)
```

#### Automatic Exec Groups ([#17134](https://github.com/bazelbuild/bazel/issues/17134))

Each toolchain type now has its own execution platform:

```starlark
# Must add toolchain parameter to actions
ctx.actions.run(
    toolchain = '@bazel_tools//tools/jdk:toolchain_type',
    executable = ctx.toolchains['@bazel_tools//tools/jdk:toolchain_type'].tool,
    ...
)
```

### Sibling Repository Layout ([#23576](https://github.com/bazelbuild/bazel/issues/23576))

**Before:**
```
$EXECROOT/<main repo>/external/<external repo>/<path>
$EXECROOT/<main repo>/bazel-out/<config>/bin/external/<external repo>/<path>
```

**After:**
```
$EXECROOT/<external repo>/<path>
$EXECROOT/<main repo>/bazel-out/<external repo>/<config>/bin/<path>
```

**Migration**: Use `file.path` in Starlark, `$(location <label>)` in genrules

---

## 4. Recipe Prerequisites & Version Requirements

### Version Requirements for Autoload Migration

When using `--incompatible_autoload_externally`:

| Dependency | Minimum Version |
|------------|-----------------|
| protobuf | >= 28.3 |
| rules_java | >= 8.5.1 |
| rules_cc | >= 0.0.17 |

### Recipe Prerequisite System

bazelbump should check prerequisites before running recipes:

```starlark
recipe(
    name = "autoload-migration",
    prerequisites = [
        bazel_version(">=8.0.0"),  # Requires Bazel 8+
        bazel_dep("rules_cc", ">=0.0.17"),
        flag_available("--incompatible_autoload_externally"),
    ],
)
```

### Proposed Prerequisite Checks

```starlark
# Check Bazel version
def bazel_version(constraint):
    """Returns prerequisite that checks Bazel version."""
    return {"type": "bazel_version", "constraint": constraint}

# Check module dependency version
def bazel_dep(name, constraint):
    """Returns prerequisite that checks bazel_dep version."""
    return {"type": "bazel_dep", "name": name, "constraint": constraint}

# Check flag availability
def flag_available(flag):
    """Returns prerequisite that checks if flag is available."""
    return {"type": "flag", "flag": flag}

# Check BCR compatibility
def bcr_compatible(name, version):
    """Returns prerequisite that checks BCR module compatibility."""
    return {"type": "bcr", "name": name, "version": version}
```

### Automatic Version Bumping

If a prerequisite fails, bazelbump could suggest or apply fixes:

```bash
$ bazelbump recipe run autoload-migration

Prerequisites check failed:
  [FAIL] Bazel version >= 8.0.0 (current: 7.7.1)
  [FAIL] rules_cc >= 0.0.17 (current: 0.0.15)

Suggested fixes:
  1. Update .bazelversion to 8.0.0
  2. Update bazel_dep(name = "rules_cc", version = "0.0.17")

Run with --auto-fix to apply these changes first.
```

---

## 5. Agentic Workflow Patterns

### Industry Best Practices

Based on research of OpenRewrite, Moderne, Sourcegraph, and jscodeshift:

#### 1. Hybrid AI + Deterministic Approach (Moderne)

- AI suggests changes but doesn't make them directly
- Deterministic recipes execute the actual transformations
- LLMs can invoke recipes as tool-function calls

#### 2. Three-Phase Execution (Already in bazelbump)

```
Phase 1: Interpretation
  - Parse recipe, build operation graph
  - No side effects, just planning

Phase 2: Validation
  - Check ALL operations valid
  - Fail fast with ALL errors, not just first

Phase 3: Execution
  - Apply changes atomically
  - Track for rollback
```

#### 3. Recipe Composition (OpenRewrite-style)

```starlark
recipe(
    name = "bazel-8-upgrade",
    recipes = [
        "remove-deprecated-flags",
        "autoload-migration",
        "bzlmod-enable",
    ],
)
```

#### 4. Human-in-the-Loop (HITL)

**Confidence-based routing:**
```starlark
edit(
    operations = [...],
    confidence = 0.8,  # Auto-apply if >= 0.8
    require_approval = ["MODULE.bazel"],  # Always require for these
)
```

**Risk-based categorization:**
- Low risk: Automatic execution
- Medium risk: Notify, proceed unless objection
- High risk: Require explicit approval

#### 5. Progressive Rollout

```starlark
recipe(
    rollout = "staged",  # or "canary", "all-at-once"
    batch_size = 10,     # Files per batch
    verify_after_batch = True,
)
```

### Checkpoint-Resume System

bazelbump already has this infrastructure:

```go
type checkpointState struct {
    StepIndex      int            // Current step (0-based)
    FilesProcessed int            // Files processed so far
    Outputs        map[string]any // Step outputs to preserve
    RecipeHash     string         // For cache invalidation
}
```

**Checkpoint types:**
- `CheckpointAuto` - Regular intervals
- `CheckpointManual` - User-triggered
- `CheckpointStepBoundary` - Between recipe steps

### Validation Strategies

1. **Pre-execution validation**: Check all operations before running
2. **Post-edit syntax validation**: Ensure output is valid Starlark
3. **Build verification**: Run `bazel build //...` after changes
4. **Test verification**: Run `bazel test //...` for semantic validation

```starlark
verify(
    command = "bazel build //...",
    on_failure = "rollback",  # or "pause", "continue"
    timeout = "10m",
)
```

---

## 6. Real-World Case Study: Envoy

### Current State

- **Bazel Version**: 7.7.1
- **bzlmod Status**: DISABLED (`common --noenable_bzlmod` in .bazelrc)
- **Repository Structure**: 3 nested modules (main, api, mobile)

### Migration Complexity Factors

1. **Custom Macros**: 20+ .bzl files with Envoy-specific rules
2. **Extension System**: Dynamic extension loading via `envoy_build_config`
3. **Triple Nesting**: api/, mobile/, mobile/envoy_build_config/ modules
4. **Non-BCR Dependencies**: 30+ custom repositories with patches
5. **LLVM Toolchain**: Pinned to 18.1.8 with custom sysroots
6. **Multi-Language**: Python, Go, Rust, C++

### Already Migrated

Envoy already migrated explicit loads in commit [`c7311de3d4`](https://github.com/envoyproxy/envoy/commit/c7311de3d4) (Dec 23, 2025):

> "bazel: Replace remaining native `cc_library` with rules_cc"

### Lessons Learned

1. **Macros don't need load statements**: `envoy_cc_library` wraps `cc_library` internally
2. **Word boundary regex is critical**: `\bcc_library\s*\(` correctly excludes `envoy_cc_library(`
3. **AST-based transformations**: Essential for handling multi-line load statements

---

## 7. Implementation Roadmap

### Phase 1: Bisect Integration (Q1 2026)

- [ ] Add `bazelbump bisect` CLI command
- [ ] Implement `bisect_versions()` Starlark builtin
- [ ] Integration with `analyze` for automatic regression detection
- [ ] Go package: `/bisect/bisect.go`

### Phase 2: Version Compatibility (Q1 2026)

- [ ] Parse `bazel_compatibility` from BCR modules
- [ ] Implement prerequisite checking system
- [ ] Add `--auto-fix` for prerequisite failures
- [ ] Version constraint validation in recipes

### Phase 3: Flag Database Enhancement (Q2 2026)

- [ ] Scrape GitHub release notes for flag changes
- [ ] Track flag availability per Bazel version
- [ ] Suggest flag removals/additions for target version
- [ ] Integration with bazelisk `--migrate` output

### Phase 4: Agentic Capabilities (Q2 2026)

- [ ] Confidence-based HITL for complex changes
- [ ] Progressive rollout with batch verification
- [ ] Recipe composition (recipes calling recipes)
- [ ] AI-assisted recipe generation (experimental)

### Phase 5: Production Readiness (Q3 2026)

- [ ] Test on 10+ open-source Bazel projects
- [ ] Snapshot & rollback CLI commands
- [ ] CI integration (GitHub Action)
- [ ] Recipe catalog with community contributions

---

## Sources

### Bazelisk Bisect
- [Bazelisk GitHub Repository](https://github.com/bazelbuild/bazelisk)
- [Troubleshooting Bazel with Git Bisect | BuildBuddy](https://www.buildbuddy.io/blog/bisect-bazel/)
- [Bazelisk Core Package](https://pkg.go.dev/github.com/bazelbuild/bazelisk/core)

### Bazel Migration
- [Bazel 8.0 Release Blog](https://blog.bazel.build/2024/12/09/bazel-8-release.html)
- [Bazel External Migration Guide](https://bazel.build/external/migration)
- [Bazel Backward Compatibility Policy](https://bazel.build/release/backward-compatibility)
- GitHub Issues: [#23023](https://github.com/bazelbuild/bazel/issues/23023), [#23809](https://github.com/bazelbuild/bazel/issues/23809), [#17032](https://github.com/bazelbuild/bazel/issues/17032), [#17134](https://github.com/bazelbuild/bazel/issues/17134)

### Agentic Workflows
- [Moderne Multi-repo AI Agent](https://www.moderne.ai/blog/introducing-moderne-multi-repo-ai-agent-for-transforming-code-at-scale)
- [OpenRewrite Documentation](https://docs.openrewrite.org/)
- [Facebook jscodeshift](https://github.com/facebook/jscodeshift)
- [Human-in-the-Loop Best Practices](https://www.permit.io/blog/human-in-the-loop-for-ai-agents-best-practices-frameworks-use-cases-and-demo)

### Real-World Examples
- Envoy bzlmod Migration Issue [#29849](https://github.com/envoyproxy/envoy/issues/29849)
- grpc-java bzlmod Issues [#11614](https://github.com/grpc/grpc-java/pull/11614)
- rules_cc Autoload Cycles [#27582](https://github.com/bazelbuild/bazel/issues/27582)

---

*This document is a living reference. Updated as new research and implementation progress.*
