# Bisect v2: Fine-Grained Version Bisect

## Problem Statement

Current bisect only works with release versions from our flag database. Users need:
1. **Release-level** bisect (7.0.0 → 8.0.0) - fast, offline
2. **RC-level** bisect (7.0.0-rc1 → 7.0.0) - find regression in RC cycle
3. **Commit-level** bisect (abc123 → def456) - find exact breaking commit

Additionally, users should know **explicitly** whether a feature uses:
- Native bazelbump engine (offline, fast)
- Delegated to bazelisk (online, more precise)

## Proposed Architecture

### Version Granularity Levels

```
Level 0: Major     (7.x.x → 8.x.x)     ~10 versions    Native
Level 1: Minor     (7.0.x → 7.1.x)     ~5 versions     Native
Level 2: Patch     (7.0.0 → 7.0.1)     ~3 versions     Native
Level 3: RC        (7.0.0-rc1 → rc2)   ~5 versions     Native (need RC data)
Level 4: Commit    (abc123 → def456)   ~100s commits   Delegate to bazelisk
```

### Data Sources

| Granularity | Source | Engine | Offline? |
|-------------|--------|--------|----------|
| Release | Flag database | Native | Yes |
| RC | GitHub Releases API | Native | No |
| Commit | GitHub Compare API | Bazelisk | No |

### CLI Design

```bash
# Auto-detect granularity from version format
bazelbump bisect --from 7.0.0 --to 8.0.0           # Release
bazelbump bisect --from 7.0.0-rc1 --to 7.0.0       # RC
bazelbump bisect --from abc123 --to def456         # Commit

# Explicit granularity
bazelbump bisect --from 7.0.0 --to 8.0.0 --granularity=release
bazelbump bisect --from 7.0.0 --to 8.0.0 --granularity=rc
bazelbump bisect --from 7.0.0 --to 8.0.0 --granularity=commit

# Show what engine will be used (dry-run)
bazelbump bisect --from 7.0.0 --to 8.0.0 --explain
```

### Output with Engine Transparency

```
$ bazelbump bisect --from 7.0.0 --to 8.0.0

Engine: native (flag database)
Granularity: release
Versions to test: 15

Testing 7.3.0... PASS (12s)
Testing 7.5.0... FAIL (8s)
...

First bad version: 7.4.1
Last good version: 7.4.0

Flag changes (from flag database):
  - --incompatible_foo: false → true

Suggested next steps:
  # For more precision, bisect at commit level:
  bazelbump bisect --from 7.4.0 --to 7.4.1 --granularity=commit
```

```
$ bazelbump bisect --from 7.4.0 --to 7.4.1 --granularity=commit

Engine: bazelisk (delegated)
Granularity: commit
Note: Requires GitHub API access

Fetching commits between 7.4.0 and 7.4.1...
Found 47 commits to test.

Testing abc123... PASS
Testing def456... FAIL
...

First bad commit: ghi789
Commit message: "Enable --incompatible_foo by default"
Author: bazel-team
Date: 2024-01-15

View: https://github.com/bazelbuild/bazel/commit/ghi789
```

## Implementation

### Version Detection

```go
type VersionSpec struct {
    Raw         string
    Type        VersionType  // Release, RC, Commit
    Major       int
    Minor       int
    Patch       int
    RC          int          // 0 if not RC
    CommitSHA   string       // empty if not commit
}

type VersionType int
const (
    VersionTypeRelease VersionType = iota
    VersionTypeRC
    VersionTypeCommit
)

func ParseVersionSpec(s string) (*VersionSpec, error) {
    // Detect type from format:
    // - "7.0.0" → Release
    // - "7.0.0-rc1" or "7.0.0rc1" → RC
    // - 7+ hex chars → Commit
    // - "7.0.0-pre.20240115.1" → Rolling (treat as commit)
}
```

### Engine Selection

```go
type BisectEngine interface {
    Name() string                           // "native" or "bazelisk"
    GetVersions(from, to string) ([]string, error)
    TestVersion(version string, cmd []string) (*TestResult, error)
    GetChanges(from, to string) (*VersionChanges, error)
}

type NativeEngine struct {
    flagDB *flagdb.DB
    // For RC: GitHub releases cache
}

type BazeliskEngine struct {
    // Delegates to bazelisk CLI or library
}

func SelectEngine(granularity Granularity) BisectEngine {
    switch granularity {
    case GranularityRelease, GranularityRC:
        return &NativeEngine{}
    case GranularityCommit:
        return &BazeliskEngine{}
    }
}
```

### RC Version Support

Need to fetch RC versions from GitHub:

```go
func (e *NativeEngine) GetRCVersions(from, to string) ([]string, error) {
    // GET https://api.github.com/repos/bazelbuild/bazel/releases
    // Filter: tag_name matches "7.0.0-rc*" pattern
    // Sort by RC number
}
```

### Commit-Level Delegation

```go
func (e *BazeliskEngine) Bisect(from, to string, testCmd []string) (*BisectResult, error) {
    // Option 1: Shell out to bazelisk
    args := []string{
        fmt.Sprintf("--bisect=%s..%s", from, to),
    }
    args = append(args, testCmd...)
    return runBazeliskAndParse(args)

    // Option 2: Import bazelisk as library (future)
}
```

## Flag Knowledge Improvements

### Current Schema

```json
{
  "name": "incompatible_foo",
  "type": "boolean",
  "default": "false",
  "description": "...",
  "tags": ["incompatible_change"]
}
```

### Proposed Enhanced Schema

```json
{
  "name": "incompatible_foo",
  "type": "boolean",
  "default": "false",
  "description": "...",
  "tags": ["incompatible_change"],

  // NEW: Lifecycle tracking
  "lifecycle": {
    "added_version": "7.0.0",
    "default_changed": [
      {"version": "7.4.0", "from": "false", "to": "true"}
    ],
    "removed_version": "9.0.0",
    "deprecation_version": "8.0.0"
  },

  // NEW: Migration guidance
  "migration": {
    "difficulty": "medium",
    "affected_patterns": ["glob()", "native rules"],
    "fix_recipe": "autoload-migration",
    "documentation": "https://bazel.build/..."
  },

  // NEW: Relationship to other flags
  "related_flags": ["incompatible_bar"],
  "superseded_by": "incompatible_baz",

  // NEW: Common breakage info
  "breakage": {
    "frequency": "high",
    "common_errors": [
      "target '//foo:bar' not visible",
      "no such package"
    ]
  }
}
```

### Data Sources for Enhanced Flags

1. **Bazel release notes** - Parse for flag changes
2. **GitHub issues** - Common breakage patterns
3. **Bazel documentation** - Migration guides
4. **Community feedback** - What breaks most often

### Sync Tool Enhancement

```bash
# Current
tools/sync-flags --version 9.0.0

# Enhanced
tools/sync-flags --version 9.0.0 \
  --include-rcs \
  --include-lifecycle \
  --fetch-migration-docs
```

## Migration Path

### Phase 1: Explicit Engine Display
- Show "Engine: native" or "Engine: bazelisk (delegated)"
- Add `--explain` flag to show what will happen

### Phase 2: RC Support
- Fetch RC versions from GitHub Releases API
- Add to native engine
- Cache locally

### Phase 3: Commit-Level Delegation
- Detect commit SHA format
- Delegate to bazelisk transparently
- Parse and format output consistently

### Phase 4: Enhanced Flag Knowledge
- Extend flag schema
- Build sync tool enhancements
- Add migration guidance to output

## Example Workflow

```bash
# 1. Fast release-level bisect (offline, native)
$ bazelbump bisect --from 7.0.0 --to 9.0.0
Engine: native (flag database)
First bad version: 8.0.0

# 2. Narrow down with RC-level (online, native)
$ bazelbump bisect --from 7.4.0 --to 8.0.0 --granularity=rc
Engine: native (GitHub releases)
First bad version: 8.0.0-rc2

# 3. Find exact commit (online, bazelisk)
$ bazelbump bisect --from 8.0.0-rc1 --to 8.0.0-rc2 --granularity=commit
Engine: bazelisk (delegated)
First bad commit: abc123def

# 4. Get detailed flag analysis
$ bazelbump analyze --to 8.0.0 --verbose
Flag: --incompatible_disallow_empty_glob
  Added: 7.0.0
  Default changed: 8.0.0-rc2 (false → true)
  Difficulty: easy
  Fix: Update glob() calls to handle empty case
  Recipe: bazelbump recipe run glob-fix
```

## Open Questions

1. Should we cache GitHub API responses for RC versions?
2. How to handle rate limiting for commit-level bisect?
3. Should enhanced flag data be crowdsourced or curated?
4. How to keep lifecycle data in sync with Bazel releases?
