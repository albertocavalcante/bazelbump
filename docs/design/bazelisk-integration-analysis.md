# Bazelisk Integration Analysis

## Overview

This document compares bazelbump's bespoke bisect implementation with bazelisk's approach, and evaluates integration opportunities.

## Feature Comparison

### Bisect

| Aspect | Bazelisk | Bazelbump |
|--------|----------|-----------|
| **Granularity** | Git commits | Release versions |
| **Version Source** | GitHub API (commits) | Flag database (releases) |
| **Use Case** | Find exact commit that broke | Find release version that broke |
| **Speed** | Slower (downloads per commit) | Faster (fewer versions) |
| **Precision** | Very high (commit-level) | Good (release-level) |
| **Offline Support** | No (needs GitHub API) | Yes (embedded flag DB) |

**Bazelisk bisect:**
```bash
bazelisk --bisect=abc123..def456 build //...
```
- Bisects between **git commits**
- Fetches commit list from GitHub API
- Downloads Bazel binary at each commit point
- Finds exact commit that introduced breakage

**Bazelbump bisect:**
```bash
bazelbump bisect --from 7.0.0 --to 8.0.0
```
- Bisects between **release versions**
- Uses embedded flag database
- Works offline
- Reports flag changes between versions

### Incompatible Flag Testing

| Aspect | Bazelisk | Bazelbump |
|--------|----------|-----------|
| **Discovery** | `bazel help <cmd> --short` | Flag database |
| **Testing** | `--migrate` (tests each flag) | Manual / recipes |
| **Reporting** | Pass/fail per flag | Flag diff between versions |
| **Automation** | Yes (`--strict`, `--migrate`) | Partial (via recipes) |

**Bazelisk --migrate:**
```bash
bazelisk --migrate build //...
```
1. Discovers all `--incompatible_*` flags
2. Tests each flag individually
3. Reports which flags pass/fail

**Bazelbump flag analysis:**
```bash
bazelbump diff --from 7.0.0 --to 8.0.0
bazelbump analyze --to 8.0.0
```
- Shows flag changes between versions
- Identifies flags that need action
- Doesn't test flags automatically (yet)

---

## Pros and Cons

### Bazelisk Approach

**Pros:**
- ✅ Commit-level precision (find exact breaking change)
- ✅ `--migrate` automatically tests each flag
- ✅ `--strict` enables all incompatible flags at once
- ✅ Well-maintained by Bazel team
- ✅ Can be used as a library (exportable)

**Cons:**
- ❌ Requires GitHub API access (rate limited without token)
- ❌ Slower (downloads at each commit)
- ❌ No knowledge of flag semantics
- ❌ Doesn't suggest fixes
- ❌ No recipe/migration automation

### Bazelbump Approach

**Pros:**
- ✅ Works offline (embedded flag database)
- ✅ Faster (fewer versions to test)
- ✅ Knowledge of flag metadata (deprecated, experimental)
- ✅ Suggests next steps (analyze, diff)
- ✅ Integrates with recipe system
- ✅ Version-level is usually sufficient

**Cons:**
- ❌ Less precise (release-level, not commit-level)
- ❌ No automatic flag testing (yet)
- ❌ Separate tool from bazelisk

---

## Integration Opportunities

### Option 1: Use Bazelisk as Subprocess (Recommended for MVP)

**How:** Shell out to bazelisk for features we don't have.

```go
// bazelbump could add:
func runBazeliskMigrate(bazelVersion string, args []string) (*MigrateResult, error) {
    cmd := exec.Command("bazelisk", append([]string{"--migrate"}, args...)...)
    cmd.Env = append(os.Environ(), "USE_BAZEL_VERSION="+bazelVersion)
    output, err := cmd.CombinedOutput()
    return parseMigrateOutput(output), err
}
```

**Pros:**
- Simple to implement
- No dependency management
- Users already have bazelisk

**Cons:**
- Requires bazelisk installed
- Output parsing is fragile
- Less control over behavior

### Option 2: Import Bazelisk as Library

**How:** Import `github.com/bazelbuild/bazelisk/core` and use directly.

```go
import (
    "github.com/bazelbuild/bazelisk/core"
    "github.com/bazelbuild/bazelisk/repositories"
)

func runWithBazeliskCore(version string, args []string) (int, error) {
    gcs := &repositories.GCSRepo{}
    github := repositories.CreateGitHubRepo(token)
    repos := core.CreateRepositories(gcs, github, gcs, gcs, true)
    cfg := core.MakeDefaultConfig()

    return core.RunBazeliskWithArgsFuncAndConfigAndOutAndErr(
        func(v string) []string { return args },
        repos, cfg, os.Stdout, os.Stderr,
    )
}
```

**Pros:**
- Direct access to bazelisk internals
- Can reuse download/cache infrastructure
- More reliable than output parsing

**Cons:**
- Adds dependency
- API may change (not stable)
- Increases binary size

### Option 3: Hybrid Approach (Recommended Long-term)

**How:**
1. Use bazelbump's bisect for release-level (fast, offline)
2. When more precision needed, delegate to bazelisk commit-level
3. Add `--migrate` equivalent using bazelisk subprocess

```bash
# Quick release-level bisect (bazelbump)
bazelbump bisect --from 7.0.0 --to 8.0.0

# Found 7.3.0 is bad, want commit precision:
bazelbump bisect --from 7.2.0 --to 7.3.0 --commit-level
# Internally calls: bazelisk --bisect=7.2.0..7.3.0 build //...

# Test individual flags:
bazelbump migrate --version 8.0.0
# Internally calls: USE_BAZEL_VERSION=8.0.0 bazelisk --migrate build //...
```

---

## What Should Bazelisk Export?

Functions that would benefit from being exported as library APIs:

### Currently Usable (Public)

| Function | Package | Description |
|----------|---------|-------------|
| `RunBazeliskWithArgsFuncAndConfigAndOutAndErr` | core | Main execution entry |
| `GetBazelInstallation` | core | Download/locate Bazel |
| `GetBazelVersion` | core | Parse version from config |
| `CreateRepositories` | core | Create repo abstraction |
| `MakeDefaultConfig` | core | Create config |

### Should Be Exported (Currently Private)

| Function | Current Location | Why Export? |
|----------|------------------|-------------|
| `getIncompatibleFlags` | core.go:838 | Discover flags for any version |
| `getBazelCommitsBetween` | core.go:982 | Fetch commits for bisect |
| `testWithBazelAtCommit` | core.go:1130 | Test at specific commit |
| `migrate` | core.go:1147 | Run flag migration |
| `bisect` | core.go:1047 | Run bisect algorithm |

### Proposed Bazelisk Library API

```go
package bazelisk

// Version resolution
func ResolveVersion(spec string) (string, error)
func ListAvailableVersions() ([]string, error)

// Execution
func RunBazel(version string, args []string) (int, error)
func RunBazelWithOutput(version string, args []string) (int, string, string, error)

// Bisect
type BisectConfig struct {
    GoodVersion string
    BadVersion  string
    TestArgs    []string
    OnProgress  func(commit string, passed bool)
}
func Bisect(cfg BisectConfig) (*BisectResult, error)

// Flag analysis
func GetIncompatibleFlags(version, command string) ([]string, error)
func TestFlags(version string, flags []string, testArgs []string) (map[string]bool, error)
```

---

## Recommended Implementation Plan

### Phase 1: Subprocess Integration (Now)

Add new commands that delegate to bazelisk:

```bash
# Test flags at a version
bazelbump flags test --version 8.0.0
# Internally: USE_BAZEL_VERSION=8.0.0 bazelisk --migrate build //...

# Commit-level bisect
bazelbump bisect --from 7.0.0 --to 8.0.0 --commit-level
# Internally: bazelisk --bisect=7.0.0..8.0.0 build //...
```

### Phase 2: Enhanced Flag Testing

Add `migrate` subcommand to bazelbump:

```bash
bazelbump migrate --version 8.0.0 --test "bazel build //..."
```

Output:
```
Testing incompatible flags for Bazel 8.0.0...

Flags that PASS (safe to enable):
  ✅ --incompatible_disallow_empty_glob
  ✅ --incompatible_no_implicit_file_export

Flags that FAIL (need fixes):
  ❌ --incompatible_disable_starlark_host_transitions
     Affected: //src/lib:foo (needs explicit host transition)
  ❌ --incompatible_require_linker_input_cc_api
     Affected: //third_party:bar (uses deprecated API)

Suggested .bazelrc additions:
  # Enable passing flags
  build --incompatible_disallow_empty_glob
  build --incompatible_no_implicit_file_export

  # Disable failing flags (fix code first)
  build --noincompatible_disable_starlark_host_transitions
```

### Phase 3: Library Integration (If Needed)

If subprocess approach becomes limiting:
1. Open issue on bazelisk for public API
2. Contribute patches to export needed functions
3. Import as library in bazelbump

---

## Summary

| Approach | Effort | Value | Recommendation |
|----------|--------|-------|----------------|
| Keep bespoke bisect | Done | High | ✅ Keep for release-level |
| Add subprocess integration | Low | High | ✅ Do now |
| Import bazelisk as library | Medium | Medium | ⏸️ Wait for stable API |
| Contribute to bazelisk | High | High | 🔄 Long-term |

**Bottom line:** Our bespoke bisect is valuable for release-level analysis (fast, offline). We should **complement** it with bazelisk subprocess calls for commit-level precision and flag testing, rather than replace it.

---

*Last updated: 2025-02-05*
