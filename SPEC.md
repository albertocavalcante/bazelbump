# bazelbump Specification

## Overview

**bazelbump** is a CLI tool to safely upgrade Bazel versions by analyzing breaking changes, migrating deprecated flags, and updating project configuration. It bridges the gap between Bazel's release notes and actionable migration steps.

### Why bazelbump?

Upgrading Bazel versions is tedious and error-prone:

1. **Scattered information**: Breaking changes are documented across GitHub issues, release notes, and source code
2. **Flag deprecation**: Bazel deprecates and removes flags without automated migration
3. **Incompatible changes**: `--incompatible_*` flags flip between versions, breaking builds silently
4. **Dependency compatibility**: Dependencies may not support your target Bazel version
5. **Manual process**: No single tool orchestrates the full upgrade workflow

bazelbump consolidates this into a single, automated workflow.

---

## CLI Interface

### Commands

```bash
# Analyze upgrade path (default command)
bazelbump analyze [--from VERSION] --to VERSION [--path DIR]

# Check current project compatibility
bazelbump check [--path DIR]

# Apply migrations automatically
bazelbump apply [--from VERSION] --to VERSION [--path DIR] [--dry-run]

# List incompatible flags between versions
bazelbump flags --from VERSION --to VERSION [--format json|table]

# Update MODULE.bazel compatibility constraints
bazelbump update-module --to VERSION [--path DIR]
```

### Global Flags

```bash
--path DIR          # Project root (default: current directory)
--verbose, -v       # Verbose output
--quiet, -q         # Suppress non-essential output
--format FORMAT     # Output format: text, json, yaml (default: text)
--color             # Force color output (default: auto)
--no-color          # Disable color output
```

### Examples

```bash
# Analyze upgrade from current version to 9.0.0
bazelbump analyze --to 9.0.0

# Analyze upgrade between specific versions
bazelbump analyze --from 7.0.0 --to 9.0.0

# Check compatibility of current project
bazelbump check

# Apply migrations with preview
bazelbump apply --to 9.0.0 --dry-run

# List all incompatible flag changes
bazelbump flags --from 8.0.0 --to 9.0.0 --format json

# Update MODULE.bazel bazel_compatibility
bazelbump update-module --to ">=9.0.0"
```

### Output Example

```
bazelbump analyze --from 8.0.0 --to 9.0.0

Analyzing upgrade: Bazel 8.0.0 -> 9.0.0
================================================================================

Current Version: 8.0.0 (from .bazelversion)
Target Version:  9.0.0

BREAKING CHANGES
----------------

[CRITICAL] Bzlmod now always enabled, WORKSPACE removed
  - WORKSPACE and WORKSPACE.bzlmod files are no longer supported
  - Migration: Convert to MODULE.bazel (see: bazelbump migrate-workspace)
  - Issue: https://github.com/bazelbuild/bazel/issues/18958

[HIGH] --incompatible_autoload_externally now defaults to ""
  - Language-specific rules must be loaded from their modules
  - Affected: rules_cc, rules_java, rules_python, etc.
  - Issue: https://github.com/bazelbuild/bazel/issues/23043

FLAG MIGRATIONS NEEDED
----------------------

.bazelrc:15 --experimental_enable_bzlmod
  -> Remove (bzlmod is now always enabled)

.bazelrc:23 --incompatible_strict_action_env=false
  -> Flag removed in 9.0.0, behavior is now default

user.bazelrc:8 --watchfs
  -> Remove (flag is no-op, will be removed)

INCOMPATIBLE FLAGS FLIPPED
--------------------------

Flag                                          8.0.0    9.0.0
--incompatible_autoload_externally            <set>    ""
--incompatible_enable_proto_toolchain_resol   false    true
--incompatible_disable_objc_library_trans     false    true

MODULE.bazel UPDATES
--------------------

Current bazel_compatibility: [">=7.0.0"]
Recommended: [">=9.0.0"]

Dependencies with compatibility issues:
  - rules_go@0.46.0: requires >=7.0.0 <=8.x (needs update)
  - gazelle@0.35.0: compatible

================================================================================
Summary: 2 critical, 3 high, 5 medium issues found
Run 'bazelbump apply --to 9.0.0 --dry-run' to preview migrations
```

---

## Core Features

### 1. Version Analysis

#### Detecting Current Version

bazelbump reads the current Bazel version from multiple sources (in priority order):

1. `.bazelversion` file (standard Bazelisk format)
2. `USE_BAZEL_VERSION` environment variable
3. `tools/bazel` wrapper script
4. Running `bazel --version` as fallback

**.bazelversion format:**
```
9.0.0
```

Or with Bazelisk directives:
```
9.0.0
# Comment lines are ignored
```

**Implementation:**
```go
// version/detect.go
func DetectCurrentVersion(projectRoot string) (string, Source, error)

type Source int
const (
    SourceBazelversion Source = iota
    SourceEnvVar
    SourceBazelWrapper
    SourceBazelBinary
)
```

#### Version Comparison

Use semantic versioning comparison following Bazel's own algorithm. Reference the go-bzlmod implementation:

**Reference:** `/Users/adsc/dev/ws/go-bzlmod/selection/version/version.go`

Key rules:
- Release versions: `MAJOR.MINOR.PATCH` compared numerically
- Prerelease versions sort BEFORE release versions (e.g., `9.0.0-rc1 < 9.0.0`)
- Empty version sorts LAST (special case for "any version")

```go
// Reuse go-bzlmod's version comparison
import "github.com/albertocavalcante/go-bzlmod/selection/version"

func CompareVersions(a, b string) int {
    return version.Compare(a, b)
}
```

#### LTS Version Awareness

Track Bazel's LTS release schedule:

| Version | Status       | Support Until |
|---------|--------------|---------------|
| Bazel 9 | Active       | Dec 2028      |
| Bazel 8 | Maintenance  | Dec 2027      |
| Bazel 7 | Maintenance  | Dec 2026      |
| Bazel 6 | Deprecated   | Dec 2025      |

**Reference:** https://bazel.build/release

---

### 2. Flag Migration

#### Parsing .bazelrc Files

.bazelrc uses a custom format (NOT Starlark). The parser must handle:

**Format Reference:** https://bazel.build/run/bazelrc

```
# Comment
command --flag=value
command --flag value
command:config --flag=value
startup --startup_flag
common --common_flag
import %workspace%/path/to/file.bazelrc
try-import %workspace%/optional.bazelrc
```

**Bazel Source Reference:**
- `RcChunkOfArgs.java`: https://github.com/bazelbuild/bazel/blob/master/src/main/java/com/google/devtools/build/lib/runtime/RcChunkOfArgs.java
- Handles import ordering and argument precedence

**Parser Implementation:**

```go
// bazelrc/parser.go
package bazelrc

type File struct {
    Path     string
    Lines    []Line
    Imports  []Import
}

type Line struct {
    Number    int
    Command   string      // "build", "test", "startup", "common", "always"
    Config    string      // Optional config name (after colon)
    Flags     []Flag
    Raw       string
}

type Flag struct {
    Name      string
    Value     string      // Empty for boolean flags
    Negated   bool        // --noflag form
    Position  Position
}

type Import struct {
    Path      string
    Required  bool        // import vs try-import
    Condition *VersionCondition  // try-import-if-bazel-version
}

func Parse(path string) (*File, error)
func ParseContent(filename string, content []byte) (*File, error)
```

#### Flag Deprecation Database

Build a database of deprecated flags by mining Bazel source code and release notes.

**Bazel Source References:**

1. **Option.java** - Defines the `@Option` annotation:
   - `deprecationWarning`: String message for deprecated flags
   - `oldName`: Previous name for renamed flags
   - `oldNameWarning`: Whether to warn on old name usage
   - Source: https://github.com/bazelbuild/bazel/blob/master/src/main/java/com/google/devtools/common/options/Option.java

2. **OptionsParserImpl.java** - Flag parsing and warning generation:
   - `maybeAddDeprecationWarning()`: Checks deprecation status
   - `maybeAddOldNameWarning()`: Generates migration messages
   - Source: https://github.com/bazelbuild/bazel/blob/master/src/main/java/com/google/devtools/common/options/OptionsParserImpl.java

**Database Schema:**

```go
// flags/database.go
type FlagChange struct {
    Flag            string
    DeprecatedIn    string          // Version where deprecated
    RemovedIn       string          // Version where removed (empty if still present)
    Replacement     string          // New flag name or empty
    Migration       MigrationAction
    Message         string          // Deprecation message
    GitHubIssue     string          // Associated issue URL
}

type MigrationAction int
const (
    ActionRemove MigrationAction = iota  // Just remove the flag
    ActionRename                          // Rename to new flag
    ActionReplace                         // Replace with different flag
    ActionUpdate                          // Update value
    ActionManual                          // Requires manual intervention
)
```

**Data Sources for Flag Changes:**

1. **Release Notes**: Parse GitHub releases for flag changes
   - URL pattern: `https://api.github.com/repos/bazelbuild/bazel/releases`
   - Look for: "deprecated", "removed", "--incompatible_", "RELNOTES"

2. **Bazel Source Scanning**: Extract `@Option` annotations
   - Search pattern: `@Option.*deprecationWarning|@Option.*oldName`
   - Key files:
     - `BuildRequestOptions.java`
     - `CommonCommandOptions.java`
     - `ExecutionOptions.java`

3. **GitHub Issues**: Track `incompatible-change` label
   - API: `gh search issues --repo bazelbuild/bazel --label "incompatible-change"`

---

### 3. Incompatible Changes Detection

#### Understanding --incompatible_* Flags

Bazel introduces breaking changes via `--incompatible_*` flags:

1. **Introduction**: Flag added with `default=false`, old behavior preserved
2. **Migration Window**: Users can test with `--incompatible_foo=true`
3. **Flip**: Default changes to `true` in a major release
4. **Removal**: Flag removed, new behavior is permanent

**Reference:** https://bazel.build/contribute/breaking-changes

#### Tracking Flag Flips

```go
// incompatible/tracker.go
type IncompatibleFlag struct {
    Name           string
    Description    string
    GitHubIssue    string
    IntroducedIn   string    // Version flag was added
    FlippedIn      string    // Version default became true
    RemovedIn      string    // Version flag was removed
    MigrationGuide string    // URL to migration docs
    Labels         []string  // GitHub labels (migration-ready, breaking-change-X.0)
}

// GetFlagsFlippedBetween returns flags that changed default between versions
func GetFlagsFlippedBetween(from, to string) []IncompatibleFlag
```

**Data Collection:**

1. **GitHub Issues API:**
```bash
gh search issues --repo bazelbuild/bazel --label "incompatible-change" --json number,title,labels,body
```

2. **Parse Issue Titles:**
   - Pattern: `--incompatible_<name>` or `incompatible_<name>`
   - Extract version from `breaking-change-X.0` labels

3. **Release Notes Mining:**
   - Search for `--incompatible_` in release bodies
   - Parse RELNOTES entries for flag flips

**Bazelisk Integration:**

bazelbump can leverage Bazelisk's `--migrate` for testing:

```bash
# Test all incompatible flags
BAZELISK_MIGRATE=1 bazel build //...

# Test specific flags
BAZELISK_INCOMPATIBLE_FLAGS=incompatible_foo,incompatible_bar bazel build //...
```

**Reference:** https://github.com/bazelbuild/bazelisk

---

### 4. MODULE.bazel Updates

#### Parsing MODULE.bazel

Use buildtools for Starlark parsing (same approach as go-bzlmod).

**Reference:** `/Users/adsc/dev/ws/go-bzlmod/third_party/buildtools/`

```go
// Vendor or import buildtools parser
import "github.com/bazelbuild/buildtools/build"

func ParseModuleFile(path string) (*build.File, error) {
    content, err := os.ReadFile(path)
    if err != nil {
        return nil, err
    }
    return build.ParseModule(path, content)
}
```

**Key functions to extract:**

```go
// module/parser.go
type ModuleInfo struct {
    Name               string
    Version            string
    BazelCompatibility []string
    Dependencies       []Dependency
}

type Dependency struct {
    Name       string
    Version    string
    RepoName   string
    DevDep     bool
}

func ParseModule(path string) (*ModuleInfo, error)
```

#### Updating bazel_compatibility

The `bazel_compatibility` attribute in `module()` specifies Bazel version constraints:

```starlark
module(
    name = "my_project",
    version = "1.0.0",
    bazel_compatibility = [">=9.0.0"],
)
```

**Constraint Format:**
- `>=X.Y.Z` - Minimum version
- `<=X.Y.Z` - Maximum version
- `>X.Y.Z`, `<X.Y.Z` - Exclusive bounds
- `-X.Y.Z` - Exclude specific version

**Reference:**
- go-bzlmod implementation: `/Users/adsc/dev/ws/go-bzlmod/bazel_compat.go`
- Bazel source: `BazelModuleResolutionFunction.java` lines 298-333

```go
// module/compatibility.go
func UpdateBazelCompatibility(path string, newConstraints []string) error {
    file, err := build.ParseModule(path, content)
    if err != nil {
        return err
    }

    // Find module() call and update bazel_compatibility attribute
    for _, stmt := range file.Stmt {
        if call, ok := stmt.(*build.CallExpr); ok {
            if ident, ok := call.X.(*build.Ident); ok && ident.Name == "module" {
                updateAttribute(call, "bazel_compatibility", newConstraints)
            }
        }
    }

    return build.Write(path, file)
}
```

#### Checking Dependency Compatibility

For each `bazel_dep`, check if the module supports the target Bazel version:

1. Query BCR (Bazel Central Registry) for module metadata
2. Parse `bazel_compatibility` from MODULE.bazel
3. Report incompatible dependencies

**BCR API:**
```
https://bcr.bazel.build/modules/{module_name}/{version}/MODULE.bazel
```

---

### 5. Migration Suggestions

#### WORKSPACE to Bzlmod Migration

Detect WORKSPACE-based projects and suggest bzlmod migration:

```go
// migration/workspace.go
type WorkspaceAnalysis struct {
    HasWorkspace     bool
    HasModuleBazel   bool
    ExternalRepos    []ExternalRepo
    MigrationHints   []MigrationHint
}

type ExternalRepo struct {
    Name     string
    Rule     string      // http_archive, git_repository, etc.
    URL      string
    BzlmodEquivalent string  // BCR module name if available
}

type MigrationHint struct {
    Severity    Severity
    Message     string
    Action      string
    Reference   string
}
```

**WORKSPACE patterns to detect:**

1. `http_archive` with BCR module available
2. `git_repository` that could use `git_override`
3. `local_repository` that could use `local_path_override`
4. `load("@bazel_tools//...")` statements (many now autoloaded)

#### Deprecated Rule Suggestions

Track deprecated rules and their replacements:

```go
// rules/deprecated.go
var DeprecatedRules = map[string]RuleReplacement{
    "native.cc_library": {
        Replacement: "cc_library from rules_cc",
        Since:       "8.0.0",
        LoadFrom:    "@rules_cc//cc:defs.bzl",
    },
    "native.java_library": {
        Replacement: "java_library from rules_java",
        Since:       "8.0.0",
        LoadFrom:    "@rules_java//java:defs.bzl",
    },
    // ... more rules
}
```

---

## Data Sources

### Primary Sources

1. **GitHub Releases API**
   - URL: `https://api.github.com/repos/bazelbuild/bazel/releases`
   - Content: Release notes, changelogs, breaking changes
   - Use: Version metadata, deprecation announcements

2. **GitHub Issues API**
   - Query: `label:incompatible-change`
   - Content: Migration guides, flag flip schedules
   - Use: Incompatible flag tracking

3. **Bazel Central Registry (BCR)**
   - URL: `https://bcr.bazel.build/`
   - Content: Module metadata, compatibility info
   - Use: Dependency version checking

4. **Bazel Source Code**
   - Repository: `bazelbuild/bazel`
   - Key files: Option definitions, deprecation annotations
   - Use: Authoritative flag metadata

### Data Caching

```go
// cache/cache.go
type Cache struct {
    Dir      string
    TTL      time.Duration
}

func (c *Cache) GetReleases() ([]Release, error)
func (c *Cache) GetIncompatibleFlags() ([]IncompatibleFlag, error)
func (c *Cache) GetModuleInfo(name, version string) (*ModuleInfo, error)

// Default cache location: ~/.cache/bazelbump/
// Default TTL: 24 hours for releases, 1 hour for BCR
```

### Offline Mode

bazelbump should work offline with cached data:

```bash
# Update cache explicitly
bazelbump cache update

# Work offline
bazelbump analyze --to 9.0.0 --offline
```

---

## Implementation Plan

### Package Structure

```
bazelbump/
├── cmd/
│   └── bazelbump/
│       └── main.go           # CLI entry point
├── internal/
│   ├── bazelrc/
│   │   ├── parser.go         # .bazelrc parser
│   │   ├── parser_test.go
│   │   └── types.go
│   ├── module/
│   │   ├── parser.go         # MODULE.bazel parser (uses buildtools)
│   │   ├── compatibility.go  # bazel_compatibility handling
│   │   └── update.go         # MODULE.bazel modifications
│   ├── version/
│   │   ├── detect.go         # Version detection
│   │   ├── compare.go        # Version comparison
│   │   └── lts.go            # LTS version tracking
│   ├── flags/
│   │   ├── database.go       # Flag deprecation database
│   │   ├── migration.go      # Flag migration logic
│   │   └── incompatible.go   # Incompatible flag tracking
│   ├── analysis/
│   │   ├── analyzer.go       # Main analysis orchestrator
│   │   ├── report.go         # Report generation
│   │   └── diff.go           # Version diff computation
│   ├── registry/
│   │   ├── bcr.go            # BCR client
│   │   └── cache.go          # Registry caching
│   └── migration/
│       ├── workspace.go      # WORKSPACE migration hints
│       └── rules.go          # Deprecated rules detection
├── third_party/
│   └── buildtools/           # Vendored Starlark parser
│       ├── build/
│       └── tables/
├── data/
│   ├── flags.json            # Embedded flag database
│   └── rules.json            # Embedded deprecated rules
├── go.mod
├── go.sum
└── SPEC.md
```

### Dependencies

```go
// go.mod
module github.com/albertocavalcante/bazelbump

go 1.22

require (
    github.com/spf13/cobra v1.8.0           // CLI framework
    github.com/fatih/color v1.16.0          // Colored output
    github.com/olekukonko/tablewriter v0.0.5 // Table formatting
)

// Vendored (no external dependency):
// - buildtools/build (Starlark parser)
```

### Development Phases

#### Phase 1: Foundation (Week 1-2)
- [ ] Project setup with Go modules
- [ ] Vendor buildtools parser (copy from go-bzlmod approach)
- [ ] Implement .bazelrc parser
- [ ] Implement version detection and comparison
- [ ] Basic CLI structure with cobra

#### Phase 2: Flag Analysis (Week 3-4)
- [ ] Build flag deprecation database from Bazel releases
- [ ] Implement GitHub API client for issues/releases
- [ ] Parse incompatible flag issues
- [ ] Create flag migration mapping

#### Phase 3: MODULE.bazel Support (Week 5-6)
- [ ] Implement MODULE.bazel parsing
- [ ] Add bazel_compatibility checking
- [ ] Implement BCR client for dependency checking
- [ ] Add MODULE.bazel update functionality

#### Phase 4: Analysis & Reporting (Week 7-8)
- [ ] Implement main analysis orchestrator
- [ ] Create report generator (text, JSON)
- [ ] Add WORKSPACE migration hints
- [ ] Implement dry-run apply functionality

#### Phase 5: Polish & Testing (Week 9-10)
- [ ] Comprehensive test suite
- [ ] Edge case handling (prereleases, LTS)
- [ ] Documentation and examples
- [ ] Performance optimization and caching

---

## Technical References

### Bazel Source Code

| Component | File | Purpose |
|-----------|------|---------|
| Option Annotation | [Option.java](https://github.com/bazelbuild/bazel/blob/master/src/main/java/com/google/devtools/common/options/Option.java) | Defines deprecationWarning, oldName fields |
| Options Parser | [OptionsParserImpl.java](https://github.com/bazelbuild/bazel/blob/master/src/main/java/com/google/devtools/common/options/OptionsParserImpl.java) | maybeAddDeprecationWarning(), flag parsing |
| RC File Parsing | [RcChunkOfArgs.java](https://github.com/bazelbuild/bazel/blob/master/src/main/java/com/google/devtools/build/lib/runtime/RcChunkOfArgs.java) | .bazelrc argument handling |
| Option Handler | [BlazeOptionHandler.java](https://github.com/bazelbuild/bazel/blob/master/src/main/java/com/google/devtools/build/lib/runtime/BlazeOptionHandler.java) | Flag expansion, config handling |
| Module Resolution | [BazelModuleResolutionFunction.java](https://github.com/bazelbuild/bazel/blob/master/src/main/java/com/google/devtools/build/lib/bazel/bzlmod/BazelModuleResolutionFunction.java) | bazel_compatibility checking (lines 298-333) |
| Build Options | [BuildRequestOptions.java](https://github.com/bazelbuild/bazel/blob/master/src/main/java/com/google/devtools/build/lib/buildtool/BuildRequestOptions.java) | @Option annotations with deprecation |

### go-bzlmod Reference Implementations

| Component | File | Purpose |
|-----------|------|---------|
| Starlark Parser | `/Users/adsc/dev/ws/go-bzlmod/ast/parser.go` | MODULE.bazel parsing approach |
| AST Types | `/Users/adsc/dev/ws/go-bzlmod/ast/types.go` | Statement type definitions |
| Version Comparison | `/Users/adsc/dev/ws/go-bzlmod/selection/version/version.go` | Bazel-compatible version comparison |
| Bazel Compatibility | `/Users/adsc/dev/ws/go-bzlmod/bazel_compat.go` | bazel_compatibility constraint checking |
| Buildtools Vendor | `/Users/adsc/dev/ws/go-bzlmod/third_party/buildtools/` | Vendored Starlark parser |

### External Documentation

| Resource | URL | Purpose |
|----------|-----|---------|
| Bazel Releases | https://bazel.build/release | LTS schedule, versioning |
| Breaking Changes Guide | https://bazel.build/contribute/breaking-changes | How incompatible changes work |
| .bazelrc Format | https://bazel.build/run/bazelrc | Configuration file syntax |
| Bazelisk Migrate | https://github.com/bazelbuild/bazelisk | --migrate flag testing |
| BCR | https://bcr.bazel.build/ | Module registry |

---

## Edge Cases

### Prerelease Versions
- RC versions: `9.0.0rc1`, `9.0.0-rc1`
- Rolling releases: `9.0.0-pre.20231115.1`
- Normalize for comparison but preserve for display

### LTS Version Jumps
- Support skipping major versions (7.x -> 9.x)
- Accumulate all breaking changes across skipped versions
- Warn about unsupported upgrade paths

### Mixed WORKSPACE/Bzlmod Projects
- Detect `WORKSPACE.bzlmod` files
- Handle `--enable_bzlmod` flag in .bazelrc
- Support gradual migration

### Platform-Specific Configs
- Parse `build:linux`, `build:macos`, `build:windows` sections
- Apply platform filtering for relevant migrations

### Custom RC Files
- Support `--bazelrc` flag analysis
- Handle `import` and `try-import` chains
- Resolve `%workspace%` paths

---

## Testing Strategy

### Unit Tests
- .bazelrc parser with various formats
- Version comparison edge cases
- MODULE.bazel parsing
- Flag database queries

### Integration Tests
- Real Bazel projects (create fixtures)
- BCR API mocking
- GitHub API mocking

### E2E Tests
- Full upgrade analysis on sample projects
- Apply migrations and verify with Bazel

### Test Fixtures
```
testdata/
├── bazelrc/
│   ├── simple.bazelrc
│   ├── with_imports.bazelrc
│   └── complex_configs.bazelrc
├── module/
│   ├── simple/MODULE.bazel
│   └── complex/MODULE.bazel
└── projects/
    ├── workspace_only/
    ├── bzlmod_only/
    └── mixed/
```
