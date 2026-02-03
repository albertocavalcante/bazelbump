# bazelbump Specification

> **Note:** This specification has been decomposed into detailed milestone documents.
> See [docs/milestones/README.md](./docs/milestones/README.md) for the implementation roadmap.

## Documentation Structure

```
docs/
├── milestones/              # Implementation phases
│   ├── README.md            # Milestone overview & timeline
│   ├── M0-foundation.md     # Project structure, protos
│   ├── M1-data-layer.md     # Flag DB, registry sync
│   ├── M2-analysis-core.md  # Flag & Starlark analysis
│   ├── M3-safety.md         # Snapshots, rollback, pre-flight
│   ├── M4-transform-engine.md # Deterministic transforms
│   ├── M5-llm-integration.md  # Bedrock, Ollama providers
│   ├── M6-agent-actions.md    # diagnose, patch, explain
│   ├── M7-workflow-integration.md # Durable workflows
│   └── M8-production-polish.md    # Error handling, docs
│
├── decisions/               # Architectural Decision Records
│   ├── 001-airgap-first.md
│   ├── 002-workflow-engine-dbos.md
│   ├── 003-llm-provider-abstraction.md
│   ├── 004-proto-first.md
│   ├── 005-data-sync-strategy.md
│   ├── 006-agent-scope-v1.md
│   └── 007-no-daemon-sqlite-default.md
│
├── research/                # Research notes
│   ├── durable-execution-comparison.md
│   ├── llm-frameworks-go.md
│   └── workflow-engines-sqlite.md
│
└── architecture/
    └── overview.md
```

---

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

## Feature Priority Matrix

Features are categorized by importance to guide development and set expectations:

```
┌─────────────────────────────────────────────────────────────────────────────┐
│                         FEATURE PRIORITY MATRIX                              │
├─────────────────────────────────────────────────────────────────────────────┤
│                                                                              │
│  🔴 PARAMOUNT (Core - Ship Blockers)                                        │
│  ├── Snapshot & Rollback           Safety net for failed upgrades           │
│  ├── Pre-flight Health Checks      Prevent doomed upgrades                  │
│  ├── Flag Migration Engine         Core value proposition                   │
│  ├── Dependency Compatibility      Know what breaks before upgrading        │
│  ├── Lockfile Migration            Critical for bzlmod users                │
│  ├── Starlark API Deprecations     Not just flags - APIs change too         │
│  └── Deterministic Transforms      Trust requires predictability            │
│                                                                              │
│  🟠 HIGH PRIORITY (Should Have - Major Value)                               │
│  ├── Upgrade Path Optimizer        Smart migration planning                 │
│  ├── Readiness Score               Clear upgrade status at a glance         │
│  ├── What-If Simulation            Safe exploration before changes          │
│  ├── Build Log Analysis            Leverage existing deprecation warnings   │
│  ├── Plugin System (Basic)         Foundation for extensibility             │
│  ├── CI/CD Integration             Most users need automation               │
│  ├── Monorepo Support              Many large codebases need this           │
│  ├── Cache Impact Analysis         Performance awareness                    │
│  ├── Cherry-Pick & Quick Fix       Run individual fixes without full upgrade│
│  ├── Prepare Mode                  Get ready for upgrade without switching  │
│  ├── Exclusions & Ignore System    Handle perpetually broken targets        │
│  └── Known Issues Tracking         Document acceptable failures             │
│                                                                              │
│  🟡 MEDIUM PRIORITY (Could Have - Nice Value)                               │
│  ├── Recipe Sharing System         Community knowledge base                 │
│  ├── Upgrade Documentation Gen     Help teams communicate changes           │
│  ├── Aspect-Based Deep Analysis    Richer semantic understanding            │
│  ├── GitHub Actions Integration    First-class CI support                   │
│  ├── Team-Based Migration          Per-team package ownership               │
│  └── Migration Progress Tracking   Status for long-running migrations       │
│                                                                              │
│  🟢 NICE TO HAVE (Want - Polish & Delight)                                  │
│  ├── Interactive TUI               Better UX for complex upgrades           │
│  ├── Multi-Repo Orchestration      Enterprise coordination                  │
│  ├── Slack/Discord Notifications   Convenience alerts                       │
│  ├── VS Code Extension             IDE integration                          │
│  └── Web Dashboard                 Visual upgrade tracking                  │
│                                                                              │
└─────────────────────────────────────────────────────────────────────────────┘
```

### Priority Definitions

| Priority | Meaning | Release Gate |
|----------|---------|--------------|
| 🔴 PARAMOUNT | Core functionality. Tool is incomplete without it. | Must have for v1.0 |
| 🟠 HIGH | Significant value-add. Most users will want it. | Should have for v1.0 |
| 🟡 MEDIUM | Good to have. Differentiator from manual process. | Target for v1.x |
| 🟢 NICE TO HAVE | Polish and delight. Not essential but appreciated. | Backlog / community |

---

## Design Inspirations

bazelbump draws deep inspiration from two masterfully designed systems: **Bazel** and **Git**.

### Learning from Bazel

```
┌─────────────────────────────────────────────────────────────────────────────┐
│                    BAZEL PHILOSOPHY → BAZELBUMP                              │
├─────────────────────────────────────────────────────────────────────────────┤
│                                                                              │
│  HERMETICITY                                                                 │
│  ├── Bazel: Builds are isolated, reproducible, no implicit deps             │
│  └── bazelbump: Analysis is hermetic                                        │
│      ├── All inputs explicit (version, project path, config)                │
│      ├── No implicit environment dependencies                               │
│      ├── Same inputs → same analysis results                                │
│      ├── Pinned flag database version                                       │
│      └── Vendored/versioned recipes                                         │
│                                                                              │
│  ANALYSIS vs EXECUTION                                                       │
│  ├── Bazel: Loading → Analysis → Execution phases                           │
│  └── bazelbump: Similar phased approach                                     │
│      ├── LOADING: Parse configs, discover files                             │
│      ├── ANALYSIS: Detect issues, plan transforms (READ-ONLY)               │
│      ├── EXECUTION: Apply changes, verify (WRITE)                           │
│      └── Phases are separate, cacheable, resumable                          │
│                                                                              │
│  AGGRESSIVE CACHING                                                          │
│  ├── Bazel: Action cache, repository cache, remote cache                    │
│  └── bazelbump: Multi-layer caching                                         │
│      ├── Analysis cache (per-file, content-addressed)                       │
│      ├── BCR query cache (TTL-based)                                        │
│      ├── GitHub API cache (ETags, conditional requests)                     │
│      ├── Flag database (embedded, versioned)                                │
│      └── Incremental analysis (only changed files)                          │
│                                                                              │
│  QUERY SYSTEM                                                                │
│  ├── Bazel: `bazel query`, `bazel cquery`, powerful filtering               │
│  └── bazelbump: Query upgrade space                                         │
│      ├── `bazelbump query "severity:critical"`                              │
│      ├── `bazelbump query "category:flag AND file://src/..."`               │
│      └── Filter, sort, aggregate issues                                     │
│                                                                              │
│  CONFIGURATION LAYERING                                                      │
│  ├── Bazel: system rc → workspace rc → user rc → command line               │
│  └── bazelbump: Same precedence model                                       │
│      ├── Built-in defaults (lowest)                                         │
│      ├── User config (~/.config/bazelbump/config.yaml)                      │
│      ├── Project config (.bazelbump/config.yaml)                            │
│      ├── Environment variables (BAZELBUMP_*)                                │
│      └── Command-line flags (highest)                                       │
│                                                                              │
└─────────────────────────────────────────────────────────────────────────────┘
```

### Learning from Git

```
┌─────────────────────────────────────────────────────────────────────────────┐
│                      GIT PHILOSOPHY → BAZELBUMP                              │
├─────────────────────────────────────────────────────────────────────────────┤
│                                                                              │
│  PORCELAIN vs PLUMBING                                                       │
│  ├── Git: High-level (porcelain) wraps low-level (plumbing)                 │
│  └── bazelbump: Same separation                                             │
│                                                                              │
│      PORCELAIN (User-facing, opinionated, friendly):                        │
│      ├── bazelbump apply        ← "just upgrade me"                         │
│      ├── bazelbump fix imports  ← "fix this one thing"                      │
│      ├── bazelbump rollback     ← "undo!"                                   │
│      └── bazelbump prepare      ← "get ready for upgrade"                   │
│                                                                              │
│      PLUMBING (Low-level, composable, scriptable):                          │
│      ├── bazelbump analyze-file ← analyze single file                       │
│      ├── bazelbump transform    ← apply single transform                    │
│      ├── bazelbump snapshot-*   ← snapshot primitives                       │
│      ├── bazelbump cache-*      ← cache primitives                          │
│      └── bazelbump db-*         ← flag database primitives                  │
│                                                                              │
│  CONTENT-ADDRESSABLE STORAGE                                                 │
│  ├── Git: Objects identified by SHA, immutable                              │
│  └── bazelbump: Snapshots are content-addressed                             │
│      ├── Snapshot ID = hash of contents                                     │
│      ├── Deduplication across snapshots                                     │
│      └── Verifiable integrity                                               │
│                                                                              │
│  REFS AND HISTORY                                                            │
│  ├── Git: branches, tags, HEAD, reflog                                      │
│  └── bazelbump: Upgrade history                                             │
│      ├── Named snapshots (tags)                                             │
│      ├── Upgrade history log                                                │
│      ├── "HEAD" = current state                                             │
│      └── Can traverse history                                               │
│                                                                              │
│  CONSISTENT UX PATTERNS                                                      │
│  ├── Git: -v, -q, --dry-run, -f, consistent across commands                 │
│  └── bazelbump: Same consistency                                            │
│      ├── --dry-run everywhere                                               │
│      ├── --verbose / --quiet everywhere                                     │
│      ├── --format for all output                                            │
│      ├── Meaningful exit codes                                              │
│      └── Predictable flag names                                             │
│                                                                              │
└─────────────────────────────────────────────────────────────────────────────┘
```

### Hermeticity in Practice

```go
// pkg/libbazelbump/hermetic.go

// AnalysisContext contains ALL inputs for hermetic analysis.
// Same context = same results, always.
type AnalysisContext struct {
    // Version inputs
    FromVersion     string
    ToVersion       string

    // Data inputs (pinned versions)
    FlagDBVersion   string    // e.g., "2024.01.15"
    RecipeVersion   string    // Recipe bundle version

    // Project inputs (content-addressed)
    ProjectHash     string    // Hash of all analyzed files

    // Configuration (explicit)
    Config          *Config   // Fully resolved config

    // NO implicit inputs:
    // - No reading from environment (except explicit BAZELBUMP_* vars)
    // - No network calls during analysis (use cached data)
    // - No current time dependencies
}

// AnalysisResult is deterministic given the same AnalysisContext
type AnalysisResult struct {
    Context     *AnalysisContext  // Record inputs for reproducibility
    CacheKey    string            // Derived from context
    // ... results
}
```

---

## Foundational Libraries

bazelbump is built on a foundation of **reusable, focused libraries**. Some exist (reuse them), some must be created (build them first).

### Dependency Architecture

```
┌─────────────────────────────────────────────────────────────────────────────┐
│                         LIBRARY ARCHITECTURE                                 │
├─────────────────────────────────────────────────────────────────────────────┤
│                                                                              │
│                              bazelbump                                       │
│                                  │                                           │
│           ┌──────────────────────┼──────────────────────┐                   │
│           │                      │                      │                   │
│           ▼                      ▼                      ▼                   │
│    ┌─────────────┐      ┌──────────────┐      ┌──────────────┐             │
│    │  go-bzlmod  │      │  go-bazelrc  │      │    go-bcr    │             │
│    │  (EXISTS)   │      │  (CREATE)    │      │   (CREATE)   │             │
│    └─────────────┘      └──────────────┘      └──────────────┘             │
│           │                      │                      │                   │
│           │              ┌───────┴───────┐              │                   │
│           │              │               │              │                   │
│           ▼              ▼               ▼              ▼                   │
│    ┌─────────────┐ ┌──────────┐ ┌─────────────┐ ┌─────────────┐           │
│    │ buildtools  │ │ go-bep   │ │ go-bazelflags│ │   go-starlark│          │
│    │ (VENDOR)    │ │ (CREATE) │ │  (CREATE)    │ │   (EXISTS)   │          │
│    └─────────────┘ └──────────┘ └─────────────┘ └─────────────┘           │
│                                                                              │
│  Legend:                                                                     │
│    EXISTS  = Use as dependency                                              │
│    CREATE  = Build as separate library first                                │
│    VENDOR  = Vendor into bazelbump                                          │
│                                                                              │
└─────────────────────────────────────────────────────────────────────────────┘
```

### Libraries to REUSE (Already Exist)

#### 1. go-bzlmod (github.com/albertocavalcante/go-bzlmod)

**Use for:**
- MODULE.bazel parsing
- Version comparison (Bazel's semver algorithm)
- bazel_compatibility constraint checking
- Module resolution logic

```go
import (
    "github.com/albertocavalcante/go-bzlmod/ast"
    "github.com/albertocavalcante/go-bzlmod/selection/version"
    "github.com/albertocavalcante/go-bzlmod"
)

// Reuse version comparison
func CompareVersions(a, b string) int {
    return version.Compare(a, b)
}

// Reuse MODULE.bazel parsing
func ParseModule(path string) (*ast.Module, error) {
    return ast.ParseModuleFile(path)
}

// Reuse bazel_compatibility checking
func CheckCompatibility(constraints []string, version string) (bool, error) {
    return gobzlmod.CheckBazelCompatibility(constraints, version)
}
```

#### 2. buildtools (github.com/bazelbuild/buildtools)

**Vendor into project** (same approach as go-bzlmod):
- Starlark parsing (BUILD, .bzl files)
- AST manipulation
- Formatting (buildifier integration)

#### 3. go-starlark (go.starlark.net)

**Use for:**
- Starlark execution (if needed for recipe evaluation)
- Syntax validation

### Libraries to CREATE (Build First)

These are **generally useful** libraries that should exist independently of bazelbump.

#### 1. go-bazelrc — .bazelrc Parser

**Why separate?** Useful for: bazelisk, IDE plugins, CI tools, buildifier, any tool that reads Bazel config.

```
github.com/bazel-contrib/go-bazelrc
├── parser.go          # Parse .bazelrc files
├── types.go           # Line, Flag, Import types
├── resolver.go        # Resolve imports, %workspace%
├── writer.go          # Write/modify .bazelrc
└── testdata/          # Test fixtures
```

```go
// go-bazelrc API
package bazelrc

type File struct {
    Path    string
    Lines   []Line
    Imports []Import
}

type Line struct {
    Number  int
    Command string   // "build", "test", "common", etc.
    Config  string   // Optional config name
    Flags   []Flag
    Raw     string
}

type Flag struct {
    Name    string
    Value   string
    Negated bool     // --nofoo form
}

func Parse(path string) (*File, error)
func ParseWithResolver(path string, resolver ImportResolver) (*File, error)
func (f *File) Write(path string) error
func (f *File) GetFlag(command, config, flagName string) (*Flag, bool)
func (f *File) SetFlag(command, config string, flag Flag) error
func (f *File) RemoveFlag(command, config, flagName string) bool
```

#### 2. go-bcr — Bazel Central Registry Client

**Why separate?** Useful for: dependency scanners, security tools, IDE plugins, documentation generators.

```
github.com/bazel-contrib/go-bcr
├── client.go          # BCR HTTP client
├── types.go           # Module, Version, Metadata types
├── cache.go           # Response caching
├── mirror.go          # Mirror support
└── registry.go        # Registry interface (for custom registries)
```

```go
// go-bcr API
package bcr

type Client struct {
    BaseURL string
    Cache   Cache
}

type Module struct {
    Name     string
    Versions []string
    Metadata ModuleMetadata
}

type ModuleVersion struct {
    Name               string
    Version            string
    BazelCompatibility []string
    Dependencies       []Dependency
    ModuleFile         []byte  // Raw MODULE.bazel content
}

func NewClient(opts ...Option) *Client
func (c *Client) GetModule(name string) (*Module, error)
func (c *Client) GetModuleVersion(name, version string) (*ModuleVersion, error)
func (c *Client) ListModules() ([]string, error)
func (c *Client) Search(query string) ([]Module, error)
```

#### 3. go-bep — Build Event Protocol Parser

**Why separate?** Useful for: build dashboards, CI integration, performance analysis, test result aggregation.

```
github.com/bazel-contrib/go-bep
├── parser.go          # Parse BEP JSON/proto stream
├── types.go           # Event types (from Bazel protos)
├── stream.go          # Streaming parser
├── analysis.go        # Extract useful information
└── proto/             # Vendored Bazel BEP protos
```

```go
// go-bep API
package bep

type Parser struct{}

type BuildEvent struct {
    ID      EventID
    Payload interface{}  // Type-specific payload
}

type BuildSummary struct {
    Success        bool
    TargetsBuilt   int
    TargetsFailed  int
    ActionsCreated int
    ActionsRun     int
    Warnings       []Warning
    Errors         []Error
}

func Parse(path string) ([]BuildEvent, error)
func ParseStream(r io.Reader) <-chan BuildEvent
func Summarize(events []BuildEvent) *BuildSummary
func ExtractWarnings(events []BuildEvent) []Warning
```

#### 4. go-bazelflags — Bazel Flag Database

**Why separate?** Useful for: documentation, migration tools, IDE autocompletion, CLI helpers.

```
github.com/bazel-contrib/go-bazelflags
├── database.go        # Flag database operations
├── types.go           # Flag, Deprecation types
├── embed.go           # Embedded flag data
├── generate/          # Generation from Bazel source
│   ├── parser.go      # Parse @Option annotations
│   └── main.go        # Generator CLI
└── data/              # Generated flag data
    ├── 7.0.0.json
    ├── 8.0.0.json
    └── 9.0.0.json
```

```go
// go-bazelflags API
package bazelflags

type Flag struct {
    Name             string
    Description      string
    Type             FlagType
    Default          string
    DeprecatedIn     string
    RemovedIn        string
    Replacement      string
    DeprecationMsg   string
    Categories       []string
    AffectsActionEnv bool  // For cache impact analysis
}

type Database struct {
    Flags   map[string]Flag
    Version string
}

func LoadEmbedded() *Database
func LoadForVersion(version string) (*Database, error)
func (db *Database) GetFlag(name string) (*Flag, bool)
func (db *Database) GetDeprecatedBetween(from, to string) []Flag
func (db *Database) GetRemovedBetween(from, to string) []Flag
func (db *Database) GetFlagsByCategory(category string) []Flag
```

### Library Development Order

Build foundational libraries **before** bazelbump to ensure clean separation:

```
PHASE 0: Foundational Libraries
================================

Week 1-2: go-bazelrc
  - .bazelrc parsing is foundational
  - Used heavily by bazelbump
  - Immediately useful to community

Week 3-4: go-bcr
  - BCR client needed for dependency checking
  - Useful standalone for other tools

Week 5-6: go-bazelflags
  - Flag database generation
  - Core to bazelbump's value proposition

Week 7-8: go-bep (can be parallel with bazelbump)
  - Build log analysis
  - Lower priority, can develop alongside

PHASE 1+: bazelbump (uses above libraries)
```

### Why This Separation Matters

| Benefit | Explanation |
|---------|-------------|
| **Reusability** | Other tools benefit from these libraries |
| **Testability** | Smaller, focused libraries are easier to test |
| **Maintainability** | Clear boundaries, single responsibility |
| **Community** | Lower barrier to contribute to focused libs |
| **Quality** | Library can mature independently |
| **Flexibility** | bazelbump can swap implementations |

---

## Architecture Principles

bazelbump follows four core architectural principles:

### 1. Proto-First

All data types are defined as Protocol Buffers first, providing:

- **Language-agnostic contracts** — Types can be used across Go, Python, Rust, etc.
- **Serialization for free** — Binary, JSON, and text formats
- **Schema evolution** — Backward-compatible changes via proto3 semantics
- **API contracts** — Clear interface definitions for library consumers

**Vendor canonical protos when available:**

```
proto/
├── vendor/                          # Vendored from upstream (automated sync)
│   └── bazel/                       # From bazelbuild/bazel
│       ├── src/main/protobuf/
│       │   ├── build.proto          # BUILD file representation
│       │   ├── analysis.proto       # Analysis results
│       │   └── command_line.proto   # Command line structure
│       └── MODULE.bazel.lock.proto  # Lockfile schema (if exists)
└── bazelbump/                       # Our custom protos
    └── v1/
        ├── flag.proto               # Flag deprecation/migration
        ├── version.proto            # Version info and constraints
        ├── analysis.proto           # Upgrade analysis results
        ├── bazelrc.proto            # Parsed .bazelrc representation
        └── migration.proto          # Migration rules and hints
```

**Vendor automation** (similar to go-bzlmod's buildtools approach):

```bash
# tools/sync-protos.sh
# Fetches canonical protos from Bazel repo at pinned commit
BAZEL_COMMIT="abc123"
curl -L "https://github.com/bazelbuild/bazel/archive/${BAZEL_COMMIT}.tar.gz" | \
  tar -xz --strip-components=1 -C proto/vendor/bazel \
  "bazel-${BAZEL_COMMIT}/src/main/protobuf/"
```

**Custom proto example:**

```protobuf
// proto/bazelbump/v1/flag.proto
syntax = "proto3";
package bazelbump.v1;

option go_package = "github.com/albertocavalcante/bazelbump/proto/bazelbump/v1;bazelbumpv1";

// FlagChange represents a flag deprecation or removal between Bazel versions.
message FlagChange {
  string name = 1;                    // Flag name (e.g., "--incompatible_foo")
  string deprecated_in = 2;           // Version where deprecated
  string removed_in = 3;              // Version where removed (empty if still present)
  string replacement = 4;             // Replacement flag or empty
  MigrationAction action = 5;
  string message = 6;                 // Human-readable deprecation message
  string github_issue = 7;            // Associated issue URL
}

enum MigrationAction {
  MIGRATION_ACTION_UNSPECIFIED = 0;
  MIGRATION_ACTION_REMOVE = 1;        // Just remove the flag
  MIGRATION_ACTION_RENAME = 2;        // Rename to replacement
  MIGRATION_ACTION_REPLACE = 3;       // Replace with different flag
  MIGRATION_ACTION_UPDATE_VALUE = 4;  // Update the value
  MIGRATION_ACTION_MANUAL = 5;        // Requires manual intervention
}

// FlagDatabase contains all known flag changes across Bazel versions.
message FlagDatabase {
  repeated FlagChange changes = 1;
  string last_updated = 2;            // ISO 8601 timestamp
  string bazel_commit = 3;            // Bazel commit this was generated from
}
```

### 2. Lib-First

The library (`libbazelbump`) provides all functionality. The CLI is a thin wrapper.

**Benefits:**

- **Embeddable** — Use in CI systems, IDEs, other tools
- **Testable** — Unit test the library directly
- **Composable** — Mix and match functionality
- **Multiple frontends** — CLI, TUI, web UI, language bindings

**Library API design:**

```go
// pkg/libbazelbump/libbazelbump.go
package libbazelbump

import (
    pb "github.com/albertocavalcante/bazelbump/proto/bazelbump/v1"
)

// Client is the main entry point for bazelbump functionality.
type Client struct {
    config *Config
    cache  Cache
}

// NewClient creates a new bazelbump client.
func NewClient(opts ...Option) (*Client, error)

// Analyze performs upgrade analysis between two Bazel versions.
func (c *Client) Analyze(ctx context.Context, req *pb.AnalyzeRequest) (*pb.AnalyzeResponse, error)

// Check validates current project compatibility.
func (c *Client) Check(ctx context.Context, req *pb.CheckRequest) (*pb.CheckResponse, error)

// Apply executes migrations (with dry-run support).
func (c *Client) Apply(ctx context.Context, req *pb.ApplyRequest) (*pb.ApplyResponse, error)

// GetFlagChanges returns flag changes between versions.
func (c *Client) GetFlagChanges(ctx context.Context, from, to string) ([]*pb.FlagChange, error)
```

**CLI as thin wrapper:**

```go
// cmd/bazelbump/main.go
func main() {
    client, _ := libbazelbump.NewClient()

    // CLI just parses args and calls library
    resp, err := client.Analyze(ctx, &pb.AnalyzeRequest{
        FromVersion: fromFlag,
        ToVersion:   toFlag,
        ProjectPath: pathFlag,
    })

    // Format and print response
    formatter.Print(resp, outputFormat)
}
```

### 3. Vendor-First

External dependencies are vendored with automation for reproducibility.

**What to vendor:**

| Dependency | Source | Automation |
|------------|--------|------------|
| buildtools | `bazelbuild/buildtools` | `tools/sync-buildtools.sh` |
| Bazel protos | `bazelbuild/bazel/src/main/protobuf` | `tools/sync-protos.sh` |
| Flag database | Generated from Bazel releases | `tools/generate-flags.sh` |

**Vendor structure:**

```
third_party/
├── buildtools/              # Starlark parser (from go-bzlmod pattern)
│   ├── build/
│   ├── tables/
│   └── README.md            # Version info, sync instructions
└── sync.sh                  # Master sync script

proto/vendor/
├── bazel/                   # Canonical Bazel protos
│   └── src/main/protobuf/
└── README.md                # Commit hash, sync date
```

**Sync automation:**

```makefile
# Makefile
.PHONY: sync-deps
sync-deps: sync-buildtools sync-protos

sync-buildtools:
    ./tools/sync-buildtools.sh $(BUILDTOOLS_VERSION)

sync-protos:
    ./tools/sync-protos.sh $(BAZEL_VERSION)

generate: sync-deps
    buf generate proto/
```

---

## Safety & Recovery [🔴 PARAMOUNT]

**Philosophy:** Upgrades can fail. When they do, recovery must be instant and painless.

### Snapshot System

Every destructive operation creates a snapshot that can be restored:

```protobuf
// proto/bazelbump/v1/snapshot.proto
syntax = "proto3";
package bazelbump.v1;

// Snapshot captures project state before modifications.
message Snapshot {
  string id = 1;                      // Unique identifier (ulid or uuid)
  string created_at = 2;              // ISO 8601 timestamp
  string bazel_version_before = 3;    // Bazel version at snapshot time
  string bazel_version_target = 4;    // Target version of the upgrade
  string git_commit = 5;              // Git HEAD at snapshot (if in repo)
  string git_branch = 6;              // Git branch name
  bool git_dirty = 7;                 // Whether working tree was dirty
  repeated FileSnapshot files = 8;    // All files that will be modified
  string description = 9;             // Human-readable description
  SnapshotStatus status = 10;
}

message FileSnapshot {
  string path = 1;                    // Relative path from project root
  bytes content = 2;                  // Original file content
  string sha256 = 3;                  // Content hash for verification
  int64 mode = 4;                     // File permissions
}

enum SnapshotStatus {
  SNAPSHOT_STATUS_UNSPECIFIED = 0;
  SNAPSHOT_STATUS_ACTIVE = 1;         // Can be restored
  SNAPSHOT_STATUS_RESTORED = 2;       // Was restored
  SNAPSHOT_STATUS_SUPERSEDED = 3;     // Newer snapshot exists
  SNAPSHOT_STATUS_EXPIRED = 4;        // Past retention period
}

// SnapshotStore manages snapshot lifecycle.
message SnapshotStore {
  repeated Snapshot snapshots = 1;
  int32 max_snapshots = 2;            // Retention limit (default: 10)
  int32 retention_days = 3;           // Auto-expire after N days
}
```

**Implementation:**

```go
// internal/snapshot/snapshot.go
package snapshot

type Manager struct {
    storeDir string  // ~/.local/share/bazelbump/snapshots/
    maxSnapshots int
}

// Create captures current state before modifications
func (m *Manager) Create(ctx context.Context, req *CreateRequest) (*Snapshot, error) {
    snap := &Snapshot{
        ID:        ulid.Make().String(),
        CreatedAt: time.Now().Format(time.RFC3339),
        Files:     make([]*FileSnapshot, 0),
    }

    // Capture git state if available
    if gitInfo, err := m.captureGitState(req.ProjectPath); err == nil {
        snap.GitCommit = gitInfo.Commit
        snap.GitBranch = gitInfo.Branch
        snap.GitDirty = gitInfo.Dirty
    }

    // Snapshot each file that will be modified
    for _, path := range req.FilesToModify {
        content, err := os.ReadFile(path)
        if err != nil {
            continue // File might not exist yet (that's ok)
        }
        snap.Files = append(snap.Files, &FileSnapshot{
            Path:    path,
            Content: content,
            SHA256:  sha256sum(content),
        })
    }

    return m.save(snap)
}

// Restore reverts project to snapshot state
func (m *Manager) Restore(ctx context.Context, snapshotID string) error {
    snap, err := m.load(snapshotID)
    if err != nil {
        return fmt.Errorf("snapshot not found: %s", snapshotID)
    }

    // Verify files haven't been modified by something else
    for _, f := range snap.Files {
        current, _ := os.ReadFile(f.Path)
        if sha256sum(current) != f.SHA256 && !req.Force {
            return fmt.Errorf("file %s modified since snapshot, use --force to override", f.Path)
        }
    }

    // Restore all files
    for _, f := range snap.Files {
        if err := os.WriteFile(f.Path, f.Content, os.FileMode(f.Mode)); err != nil {
            return err
        }
    }

    snap.Status = SnapshotStatusRestored
    return m.save(snap)
}

// List shows available snapshots
func (m *Manager) List(ctx context.Context) ([]*Snapshot, error)

// Prune removes old snapshots based on retention policy
func (m *Manager) Prune(ctx context.Context) (int, error)
```

**CLI Usage:**

```bash
# Apply creates snapshot automatically
bazelbump apply --to 9.0.0
# Output: Snapshot created: 01HQ3KXYZ... (10 files captured)

# List snapshots
bazelbump snapshot list
# Output:
# ID                    CREATED              TARGET    FILES  STATUS
# 01HQ3KXYZ...          2024-01-15 10:30    9.0.0     10     active
# 01HQ2ABCD...          2024-01-14 15:45    8.1.0     8      active

# Restore to previous state
bazelbump rollback 01HQ3KXYZ
# Output: Restored 10 files to pre-9.0.0 state

# Or restore the most recent snapshot
bazelbump rollback --latest

# Disable snapshot (not recommended)
bazelbump apply --to 9.0.0 --no-snapshot
```

### Pre-flight Health Checks

Before attempting any upgrade, validate the project is in a good state:

```go
// internal/preflight/checks.go
package preflight

type Check interface {
    Name() string
    Description() string
    Severity() Severity       // BLOCKER, WARNING, INFO
    Run(ctx context.Context, project *Project) *Result
}

type Severity int
const (
    SeverityInfo Severity = iota
    SeverityWarning
    SeverityBlocker
)

type Result struct {
    Passed      bool
    Message     string
    Remediation string   // How to fix if failed
    Details     string   // Additional context
}

// Built-in checks
var DefaultChecks = []Check{
    &GitCleanCheck{},           // Working tree should be clean
    &CurrentBuildCheck{},       // Current version should build
    &TestsPassCheck{},          // Tests should pass (optional)
    &BazelrcValidCheck{},       // .bazelrc should parse
    &ModuleLockCheck{},         // MODULE.bazel.lock should be current
    &DiskSpaceCheck{},          // Enough space for operations
    &NetworkCheck{},            // Can reach BCR, GitHub (if needed)
    &DeprecationWarningsCheck{}, // Check for existing warnings
}

// GitCleanCheck ensures no uncommitted changes
type GitCleanCheck struct{}

func (c *GitCleanCheck) Name() string { return "git-clean" }
func (c *GitCleanCheck) Severity() Severity { return SeverityWarning }

func (c *GitCleanCheck) Run(ctx context.Context, p *Project) *Result {
    cmd := exec.CommandContext(ctx, "git", "status", "--porcelain")
    cmd.Dir = p.Root
    out, err := cmd.Output()
    if err != nil {
        return &Result{Passed: true, Message: "Not a git repository (skipped)"}
    }
    if len(out) > 0 {
        return &Result{
            Passed:      false,
            Message:     "Working tree has uncommitted changes",
            Remediation: "Commit or stash changes before upgrading",
            Details:     string(out),
        }
    }
    return &Result{Passed: true, Message: "Working tree is clean"}
}

// CurrentBuildCheck verifies the project builds with current Bazel
type CurrentBuildCheck struct {
    Targets []string  // Default: //...
    Timeout time.Duration
}

func (c *CurrentBuildCheck) Run(ctx context.Context, p *Project) *Result {
    // Run: bazel build //... with timeout
    // Parse exit code and output
}

// DeprecationWarningsCheck scans for existing deprecation warnings
type DeprecationWarningsCheck struct{}

func (c *DeprecationWarningsCheck) Run(ctx context.Context, p *Project) *Result {
    // Run: bazel build //... --output_filter=DONT_MATCH_ANYTHING 2>&1 | grep -i deprecat
    // Count and report warnings
}
```

**CLI Output:**

```
bazelbump preflight

PRE-FLIGHT CHECKS
=================

Checking project readiness for upgrade...

  [✓] git-clean              Working tree is clean
  [✓] bazelrc-valid          .bazelrc parses successfully
  [✓] module-lock-current    MODULE.bazel.lock is up to date
  [✓] disk-space             12.4 GB available (need ~500 MB)
  [✓] network                BCR and GitHub reachable
  [⚠] current-build          Build passes with 3 deprecation warnings
      └── Warnings found:
          - --experimental_enable_bzlmod is deprecated (use bzlmod by default)
          - native.cc_library is deprecated (load from @rules_cc)
          - ctx.host_configuration is deprecated (use ctx.configuration)
  [✓] tests-pass             All tests pass (skipped: --skip-tests)

━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━

Summary: 6 passed, 1 warning, 0 blockers

⚠ Existing deprecation warnings should be addressed before upgrading.
  Run 'bazelbump analyze --from-warnings' to see migration suggestions.

Ready to proceed with upgrade analysis.
```

**Blocker Example:**

```
bazelbump preflight

PRE-FLIGHT CHECKS
=================

  [✓] git-clean              Working tree is clean
  [✗] current-build          Build fails with current Bazel version
      └── Error: //src/lib:mylib does not build
          Fix the current build before attempting an upgrade.
  [—] tests-pass             Skipped (build failed)

━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━

Summary: 1 passed, 0 warnings, 1 BLOCKER

✗ Cannot proceed: project must build successfully before upgrade.
```

**Integration with Apply:**

```go
// pkg/libbazelbump/apply.go

func (c *Client) Apply(ctx context.Context, req *ApplyRequest) (*ApplyResponse, error) {
    // 1. Run preflight checks (unless --skip-preflight)
    if !req.SkipPreflight {
        preflightResult := c.preflight.Run(ctx, req.Project)
        if preflightResult.HasBlockers() {
            return nil, &PreflightFailedError{Results: preflightResult}
        }
        if preflightResult.HasWarnings() && !req.Force {
            return nil, &PreflightWarningsError{
                Results: preflightResult,
                Message: "Use --force to proceed with warnings",
            }
        }
    }

    // 2. Create snapshot (unless --no-snapshot)
    if !req.NoSnapshot {
        snap, err := c.snapshots.Create(ctx, &CreateRequest{
            ProjectPath:    req.Project.Root,
            FilesToModify:  c.predictModifiedFiles(req),
            Description:    fmt.Sprintf("Upgrade to Bazel %s", req.ToVersion),
        })
        if err != nil {
            return nil, fmt.Errorf("failed to create snapshot: %w", err)
        }
        // Store snapshot ID in response for reference
    }

    // 3. Proceed with upgrade...
}
```

---

## Transformation Philosophy

### Inspiration: Modern Code Transformation Tools

bazelbump draws inspiration from tools that perform reliable, large-scale code transformations:

| Tool | Approach | Key Insight |
|------|----------|-------------|
| [**Sourcegraph Batch Changes**](https://docs.sourcegraph.com/batch_changes) | Structural search + comby | Scale transformations across thousands of repos |
| [**OpenRewrite**](https://docs.openrewrite.org/) | Recipe-based CST transforms | Preserve formatting via Lossless Semantic Trees (LST) |
| [**Comby**](https://comby.dev/) | Structural pattern matching | Language-agnostic, whitespace-preserving |
| [**IntelliJ Platform**](https://plugins.jetbrains.com/docs/intellij/psi.html) | PSI (Program Structure Interface) | Rich semantic understanding + intentions |
| [**LSP Code Actions**](https://microsoft.github.io/language-server-protocol/) | textDocument/codeAction | Standardized automated fixes |
| [**Buildifier**](https://github.com/bazelbuild/buildtools/tree/master/buildifier) | Bazel-specific AST | Load sorting, formatting, linting |
| [**Gazelle**](https://github.com/bazelbuild/bazel-gazelle) | Heuristic BUILD generation | Infer structure from source files |

### Core Principle: Deterministic First, Heuristics Second

```
┌─────────────────────────────────────────────────────────────────┐
│                    TRANSFORMATION HIERARCHY                      │
├─────────────────────────────────────────────────────────────────┤
│  TIER 1: Deterministic (CST-based)                              │
│  ├── Exact pattern match → exact replacement                    │
│  ├── Preserves whitespace, comments, formatting                 │
│  ├── Verifiable: input X always produces output Y               │
│  └── Examples: flag rename, load() insertion, version bump      │
├─────────────────────────────────────────────────────────────────┤
│  TIER 2: Heuristic (Pattern-based)                              │
│  ├── Best-effort when deterministic isn't possible              │
│  ├── Clearly marked as suggestions, not guarantees              │
│  ├── User confirmation required before applying                 │
│  └── Examples: native rule detection, dependency inference      │
└─────────────────────────────────────────────────────────────────┘
```

**Why this matters:**

1. **Trust** — Users must trust automated changes. Deterministic transforms build confidence.
2. **Reviewability** — Changes should be explainable: "replaced X with Y because Z"
3. **Rollback** — If something breaks, understanding exactly what changed is critical.
4. **CI/CD** — Deterministic transforms can run in CI without human review.

### Concrete Example: `--incompatible_autoload_externally` Migration

From [Bazel 9.0.0 release notes](https://github.com/bazelbuild/bazel/releases/tag/9.0.0):

> The flag `--incompatible_autoload_externally` now defaults to the empty string (#23043), meaning that all language-specific rules will now need to be loaded from their respective modules (rules_cc, rules_java, etc). This marks the conclusion of the Starlarkification effort.

**Before (Bazel 8.x):** Native rules auto-loaded

```starlark
# BUILD - just works
cc_library(
    name = "mylib",
    srcs = ["mylib.cc"],
)
```

**After (Bazel 9.0):** Explicit loads required

```starlark
# BUILD - must add load()
load("@rules_cc//cc:defs.bzl", "cc_library")

cc_library(
    name = "mylib",
    srcs = ["mylib.cc"],
)
```

#### Tier 1: Deterministic Transform

```go
// internal/transforms/autoload.go

// NativeRuleMapping maps native rules to their new load locations.
// Source: https://github.com/bazelbuild/bazel/issues/23043
var NativeRuleMapping = map[string]LoadStatement{
    "cc_library":    {Label: "@rules_cc//cc:defs.bzl", Symbols: []string{"cc_library"}},
    "cc_binary":     {Label: "@rules_cc//cc:defs.bzl", Symbols: []string{"cc_binary"}},
    "cc_test":       {Label: "@rules_cc//cc:defs.bzl", Symbols: []string{"cc_test"}},
    "java_library":  {Label: "@rules_java//java:defs.bzl", Symbols: []string{"java_library"}},
    "java_binary":   {Label: "@rules_java//java:defs.bzl", Symbols: []string{"java_binary"}},
    "py_library":    {Label: "@rules_python//python:defs.bzl", Symbols: []string{"py_library"}},
    "py_binary":     {Label: "@rules_python//python:defs.bzl", Symbols: []string{"py_binary"}},
    // ... complete mapping from Bazel source
}

// TransformAutoload adds required load() statements to a BUILD file.
func TransformAutoload(file *build.File) (*build.File, []Change) {
    // 1. Parse BUILD file with buildtools (CST - preserves formatting)
    // 2. Find all rule calls in the file
    // 3. Match against NativeRuleMapping
    // 4. Generate load() statements for missing rules
    // 5. Use buildtools to insert loads (handles sorting, deduplication)
    // 6. Return modified file + list of changes for reporting
}
```

**Integration with Buildifier:**

Rather than reinventing load() sorting, delegate to buildifier:

```go
// After inserting loads, run buildifier for canonical formatting
import "github.com/bazelbuild/buildtools/build"

func FormatWithBuildifier(file *build.File) *build.File {
    // buildtools handles:
    // - Load statement sorting (by label, then by symbol)
    // - Deduplication of symbols
    // - Correct positioning (loads at top, after comments)
    return build.Format(file)
}
```

#### Tier 2: Heuristic Assist

When CST parsing isn't available or practical:

```go
// Heuristic: Quick scan for native rule usage
// Used for: Initial project assessment, CI checks, large monorepos

func QuickScanForNativeRules(content []byte) []string {
    var detected []string
    patterns := []string{
        `\bcc_library\s*\(`,
        `\bcc_binary\s*\(`,
        `\bjava_library\s*\(`,
        // ...
    }
    for _, pattern := range patterns {
        if regexp.MustCompile(pattern).Match(content) {
            detected = append(detected, extractRuleName(pattern))
        }
    }
    return detected
}
```

**When to use heuristics:**

| Scenario | Approach | Rationale |
|----------|----------|-----------|
| Single BUILD file | Deterministic (CST) | Accuracy matters, fast enough |
| 10,000 BUILD files | Heuristic scan first | Find candidates, then CST for actual changes |
| Malformed files | Heuristic | CST parsing fails, still provide guidance |
| Quick CI check | Heuristic | "You might need to migrate X" warning |

### Transformation Pipeline

```
┌──────────────┐     ┌──────────────┐     ┌──────────────┐     ┌──────────────┐
│   ANALYZE    │ ──▶ │    PLAN      │ ──▶ │   PREVIEW    │ ──▶ │    APPLY     │
│              │     │              │     │              │     │              │
│ - Parse files│     │ - Match rules│     │ - Show diff  │     │ - Write files│
│ - Detect ver │     │ - Gen changes│     │ - User review│     │ - Run buildf │
│ - Find usage │     │ - Sort by pri│     │ - Confirm    │     │ - Verify     │
└──────────────┘     └──────────────┘     └──────────────┘     └──────────────┘
       │                    │                    │                    │
       ▼                    ▼                    ▼                    ▼
   Heuristics OK      Deterministic         Human-in-loop      Deterministic
```

### Output: Explainable Changes

Every transformation must be explainable:

```
CHANGE: Add load statement to //src/lib:BUILD

  Reason: cc_library requires explicit load in Bazel 9.0+
  Reference: https://github.com/bazelbuild/bazel/issues/23043

  @@ -1,3 +1,5 @@
  +load("@rules_cc//cc:defs.bzl", "cc_library")
  +
   cc_library(
       name = "mylib",
       srcs = ["mylib.cc"],

  Confidence: HIGH (deterministic CST transform)
  Reversible: YES (remove load statement)
```

---

## Rules Ecosystem & Agentic Architecture

### The Rules Problem

Bazel's power comes from its **plugin-first architecture** — rules like `rules_go`, `rules_python`, `rules_java` extend Bazel's capabilities. But this creates upgrade blockers:

```
┌─────────────────────────────────────────────────────────────────┐
│  YOU WANT TO UPGRADE: Bazel 8.0 → 9.0                          │
├─────────────────────────────────────────────────────────────────┤
│  BLOCKERS:                                                      │
│  ├── rules_go@0.46.0 — max bazel_compatibility: <=8.x          │
│  ├── rules_python@0.31.0 — uses deprecated native.py_library   │
│  └── internal_rules (proprietary) — unknown compatibility       │
└─────────────────────────────────────────────────────────────────┘
```

**This is not solvable by deterministic transforms alone.**

### Expanding Scope: CLI → Agent

bazelbump must evolve from a CLI tool to an **agentic upgrade assistant**:

```
┌─────────────────────────────────────────────────────────────────┐
│                    BAZELBUMP ARCHITECTURE                        │
├─────────────────────────────────────────────────────────────────┤
│                                                                  │
│  ┌──────────────┐  ┌──────────────┐  ┌──────────────┐          │
│  │   CLI MODE   │  │  AGENT MODE  │  │ WORKFLOW MODE│          │
│  │              │  │              │  │              │          │
│  │ Quick fixes  │  │ Complex ops  │  │ Long-running │          │
│  │ Local only   │  │ LLM-powered  │  │ Multi-step   │          │
│  │ Deterministic│  │ Heuristic+AI │  │ Orchestrated │          │
│  └──────┬───────┘  └──────┬───────┘  └──────┬───────┘          │
│         │                 │                 │                   │
│         ▼                 ▼                 ▼                   │
│  ┌─────────────────────────────────────────────────────────┐   │
│  │              CORE LIBRARY (libbazelbump)                 │   │
│  │  ┌─────────┐ ┌─────────┐ ┌─────────┐ ┌─────────┐        │   │
│  │  │ Parser  │ │Analyzer │ │Transform│ │ Registry│        │   │
│  │  └─────────┘ └─────────┘ └─────────┘ └─────────┘        │   │
│  └─────────────────────────────────────────────────────────┘   │
│                                                                  │
└─────────────────────────────────────────────────────────────────┘
```

### Agent Capabilities

#### 1. Patch Generation (LLM-Powered)

When a ruleset isn't compatible and no fix exists:

```go
// agent/patch.go

type PatchRequest struct {
    Ruleset       string   // e.g., "rules_go"
    CurrentVersion string
    TargetBazel   string
    ErrorLog      string   // Build failure output
    SourceFiles   []File   // Relevant source from the ruleset
}

type PatchResponse struct {
    Patch       string       // Generated diff
    Confidence  float64      // 0.0-1.0
    Explanation string       // Why this patch should work
    TestPlan    string       // How to verify the patch
    Caveats     []string     // Known limitations
}

// GeneratePatch uses LLM to create a compatibility patch
func (a *Agent) GeneratePatch(ctx context.Context, req *PatchRequest) (*PatchResponse, error) {
    // 1. Gather context: ruleset source, Bazel changes, error logs
    // 2. Build prompt with relevant code snippets
    // 3. Request patch from LLM
    // 4. Validate patch syntax (deterministic check)
    // 5. Optionally: test patch in sandbox
    // 6. Return with confidence score
}
```

**Key insight:** The agent doesn't replace deterministic transforms — it handles the **long tail** where recipes don't exist.

#### 2. Bug Report Drafting

When upgrade is blocked and no workaround exists:

```go
// agent/issues.go

type IssueContext struct {
    Ruleset       string
    RulesetRepo   string       // e.g., "bazelbuild/rules_go"
    IsOSS         bool
    BazelVersion  string
    ErrorDetails  string
    SimilarIssues []GitHubIssue  // Found during research
}

type DraftIssue struct {
    Title       string
    Body        string
    Labels      []string
    References  []string   // Links to related issues, docs
    Confidence  float64
}

// DraftBugReport generates an issue for ruleset maintainers
func (a *Agent) DraftBugReport(ctx context.Context, c *IssueContext) (*DraftIssue, error) {
    // 1. Search existing issues (deterministic)
    // 2. If duplicate found, suggest commenting instead
    // 3. Analyze error and Bazel changes
    // 4. Generate structured bug report
    // 5. Include minimal reproduction steps
    // 6. Reference relevant Bazel changes
}
```

**Output example:**

```markdown
## Title: [Bazel 9.0] Compatibility issue with native rule autoloading

## Environment
- rules_go version: 0.46.0
- Bazel version: 9.0.0 (upgrading from 8.0.0)
- OS: Linux

## Problem
After upgrading to Bazel 9.0, builds fail with:
```
ERROR: no such target '@rules_go//go:def.bzl': target 'def.bzl' not declared
```

## Root Cause Analysis
Bazel 9.0 changed `--incompatible_autoload_externally` default (bazelbuild/bazel#23043).
rules_go@0.46.0 still relies on native rule autoloading.

## Suggested Fix
[Agent-generated patch or suggestion]

## Related Issues
- bazelbuild/bazel#23043 (autoload change)
- bazelbuild/rules_go#XXXX (similar report)
```

#### 3. Documentation Mining

**Source code is the best documentation.** When official docs are lacking:

```go
// agent/docs.go

type DocQuery struct {
    Topic       string   // e.g., "bazel_compatibility constraint format"
    Sources     []DocSource
}

type DocSource int
const (
    DocSourceCode      DocSource = iota  // Always check first
    DocSourceOfficial                     // bazel.build, rules docs
    DocSourceReleaseNotes                 // GitHub releases
    DocSourceIssues                       // GitHub issues/discussions
    DocSourceCommunity                    // Stack Overflow, Slack
)

// Research finds authoritative information about a topic
func (a *Agent) Research(ctx context.Context, q *DocQuery) (*DocResult, error) {
    // Priority order:
    // 1. Source code (grep for implementations, comments)
    // 2. Official docs (if exist and current)
    // 3. Release notes
    // 4. GitHub issues (often contain explanations)
    // 5. Community (last resort, verify against source)
}
```

**Source code mining example:**

```go
// Finding bazel_compatibility constraint format
// Instead of searching docs, search Bazel source:

// BazelModuleResolutionFunction.java:298-333
// Shows exact parsing logic for constraints

// ModuleFileGlobals.java:65-66
// Shows constraint syntax: >=, <=, >, <, -
```

#### 4. Documentation Gap Reporting

When docs are missing or outdated, the agent can draft improvements:

```go
// agent/docs_improve.go

type DocGap struct {
    Topic        string
    CurrentDoc   string   // URL to current doc (if exists)
    SourceTruth  string   // What source code says
    Discrepancy  string   // What's missing or wrong
    SuggestedFix string   // Proposed documentation update
}

// DetectDocGaps finds documentation issues during research
func (a *Agent) DetectDocGaps(ctx context.Context) ([]DocGap, error)

// DraftDocPR creates a pull request to improve docs
func (a *Agent) DraftDocPR(ctx context.Context, gap DocGap) (*DraftPR, error)
```

### OSS vs Proprietary Handling

```
┌─────────────────────────────────────────────────────────────────┐
│                    RULESET TYPE DETECTION                        │
├─────────────────────────────────────────────────────────────────┤
│                                                                  │
│  OSS Ruleset (e.g., rules_go, rules_python)                     │
│  ├── Check BCR for latest version                               │
│  ├── Search GitHub releases for Bazel 9 support                 │
│  ├── Search issues for known blockers                           │
│  ├── If fix exists: suggest version upgrade                     │
│  ├── If PR pending: show status, suggest testing                │
│  ├── If no fix: attempt patch generation                        │
│  └── If patch fails: draft issue for maintainers                │
│                                                                  │
│  Proprietary Ruleset (internal, vendored)                       │
│  ├── Cannot search external sources                             │
│  ├── Analyze source if available locally                        │
│  ├── Generate compatibility report                              │
│  ├── Attempt patch generation if source available               │
│  └── Flag as manual intervention required                       │
│                                                                  │
└─────────────────────────────────────────────────────────────────┘
```

### Workflow Orchestration with Temporal

Complex upgrades require **durable, resumable workflows**. [Temporal](https://temporal.io) provides:

- **Durability** — Workflows survive process restarts
- **Visibility** — Track progress of long-running upgrades
- **Retry logic** — Handle transient failures (API rate limits, etc.)
- **Human-in-loop** — Pause for approval at critical steps

```go
// workflows/upgrade.go

// UpgradeWorkflow orchestrates a full Bazel version upgrade
func UpgradeWorkflow(ctx workflow.Context, req *UpgradeRequest) (*UpgradeResult, error) {

    // Phase 1: Analysis (deterministic)
    analysis := workflow.ExecuteActivity(ctx, AnalyzeProject, req.ProjectPath)

    // Phase 2: Identify blockers (deterministic)
    blockers := workflow.ExecuteActivity(ctx, FindBlockers, analysis, req.TargetVersion)

    // Phase 3: Resolve blockers (mixed deterministic + agentic)
    var resolutions []Resolution
    for _, blocker := range blockers {
        switch blocker.Type {
        case BlockerFlagDeprecated:
            // Deterministic: apply known transform
            res := workflow.ExecuteActivity(ctx, ApplyFlagMigration, blocker)
            resolutions = append(resolutions, res)

        case BlockerRulesetIncompatible:
            // Agentic: research and suggest fix
            res := workflow.ExecuteActivity(ctx, ResolveRulesetBlocker, blocker)

            if res.RequiresHumanReview {
                // Pause workflow, wait for approval
                workflow.GetSignalChannel(ctx, "approval").Receive(ctx, &approval)
                if !approval.Approved {
                    return nil, errors.New("upgrade cancelled by user")
                }
            }
            resolutions = append(resolutions, res)

        case BlockerUnknown:
            // Agentic: attempt LLM analysis
            res := workflow.ExecuteActivity(ctx, AgentAnalyzeBlocker, blocker)
            resolutions = append(resolutions, res)
        }
    }

    // Phase 4: Generate upgrade report
    report := workflow.ExecuteActivity(ctx, GenerateReport, analysis, resolutions)

    // Phase 5: Apply changes (with human approval for agentic changes)
    if req.AutoApply {
        for _, res := range resolutions {
            if res.Confidence >= req.MinConfidence {
                workflow.ExecuteActivity(ctx, ApplyResolution, res)
            }
        }
    }

    return &UpgradeResult{
        Report:      report,
        Resolutions: resolutions,
        Status:      computeStatus(resolutions),
    }, nil
}
```

**Workflow visualization:**

```
┌────────────────────────────────────────────────────────────────────────┐
│  UPGRADE WORKFLOW: my-project (Bazel 8.0 → 9.0)                        │
├────────────────────────────────────────────────────────────────────────┤
│                                                                         │
│  [✓] Phase 1: Analysis                                    2 min        │
│      └── Found 3 blockers                                              │
│                                                                         │
│  [✓] Phase 2: Flag migrations                             30 sec       │
│      └── Applied 5 flag changes to .bazelrc                            │
│                                                                         │
│  [●] Phase 3: Ruleset compatibility                       WAITING      │
│      ├── [✓] rules_go: upgraded to 0.47.0                              │
│      ├── [✓] rules_python: applied autoload patch                      │
│      └── [⏸] internal_rules: awaiting human review                     │
│          │                                                              │
│          │   Agent generated patch with 0.72 confidence                │
│          │   [View Patch] [Approve] [Reject] [Request Changes]         │
│          │                                                              │
│  [ ] Phase 4: Generate report                                          │
│  [ ] Phase 5: Apply & verify                                           │
│                                                                         │
└────────────────────────────────────────────────────────────────────────┘
```

### Recipe vs Agent Decision Tree

```
┌─────────────────────────────────────────────────────────────────┐
│            WHEN TO USE RECIPE vs AGENT                          │
├─────────────────────────────────────────────────────────────────┤
│                                                                  │
│  Is there a known, deterministic transform?                     │
│  ├── YES → Use RECIPE (OpenRewrite-style)                       │
│  │         Examples:                                            │
│  │         - Flag rename: --foo → --bar                         │
│  │         - Load addition: known rule → known load             │
│  │         - Version bump: known safe upgrade                   │
│  │                                                              │
│  └── NO → Does it require external research?                    │
│           ├── NO → Use HEURISTIC (pattern matching)             │
│           │        Examples:                                    │
│           │        - Detect native rule usage                   │
│           │        - Infer module dependencies                  │
│           │                                                     │
│           └── YES → Use AGENT (LLM-powered)                     │
│                     Examples:                                   │
│                     - Generate compatibility patch              │
│                     - Draft bug report                          │
│                     - Mine source for documentation             │
│                     - Analyze unknown error                     │
│                                                                  │
└─────────────────────────────────────────────────────────────────┘
```

### Data Sources Priority

```go
// research/sources.go

// SourcePriority defines the order of trust for information
var SourcePriority = []Source{
    // Tier 1: Ground truth
    {Name: "Bazel Source Code", Trust: 1.0, Type: SourceCode},
    {Name: "Rules Source Code", Trust: 1.0, Type: SourceCode},

    // Tier 2: Official documentation
    {Name: "bazel.build docs", Trust: 0.9, Type: OfficialDocs},
    {Name: "Rules README/docs", Trust: 0.9, Type: OfficialDocs},

    // Tier 3: Release artifacts
    {Name: "GitHub Releases", Trust: 0.85, Type: ReleaseNotes},
    {Name: "CHANGELOG files", Trust: 0.85, Type: ReleaseNotes},

    // Tier 4: Community knowledge
    {Name: "GitHub Issues", Trust: 0.7, Type: Community},
    {Name: "GitHub Discussions", Trust: 0.7, Type: Community},
    {Name: "Stack Overflow", Trust: 0.5, Type: Community},
    {Name: "Bazel Slack", Trust: 0.4, Type: Community},
}

// Always verify community knowledge against source code
func (r *Researcher) VerifyAgainstSource(claim Claim) (verified bool, evidence string)
```

---

## Upgrade Intelligence [🟠 HIGH PRIORITY]

Beyond basic analysis, bazelbump provides intelligent upgrade planning.

### Upgrade Path Optimizer

Not all upgrade paths are equal. Jumping from 7.x to 9.x directly might be harder than going 7.x → 8.x → 9.x:

```protobuf
// proto/bazelbump/v1/path.proto
syntax = "proto3";
package bazelbump.v1;

message UpgradePath {
  repeated UpgradeStep steps = 1;
  PathRisk overall_risk = 2;
  string recommendation = 3;          // Why this path is suggested
}

message UpgradeStep {
  string from_version = 1;
  string to_version = 2;
  int32 breaking_changes = 3;
  int32 flag_changes = 4;
  int32 dependency_updates = 5;
  PathRisk risk = 6;
  string notes = 7;
}

enum PathRisk {
  PATH_RISK_UNSPECIFIED = 0;
  PATH_RISK_LOW = 1;                  // < 10 changes, all automated
  PATH_RISK_MEDIUM = 2;               // 10-30 changes, some manual
  PATH_RISK_HIGH = 3;                 // > 30 changes or major blockers
}

message PathAnalysisRequest {
  string from_version = 1;
  string to_version = 2;
  string project_path = 3;
  bool include_rc_versions = 4;       // Include release candidates
}

message PathAnalysisResponse {
  repeated UpgradePath paths = 1;     // Multiple options
  UpgradePath recommended = 2;        // Best path
}
```

**Implementation:**

```go
// internal/pathfinder/pathfinder.go
package pathfinder

type Pathfinder struct {
    releases   *releases.Client
    analyzer   *analysis.Analyzer
}

// FindPaths discovers possible upgrade routes
func (p *Pathfinder) FindPaths(ctx context.Context, req *PathAnalysisRequest) (*PathAnalysisResponse, error) {
    // Get all LTS versions between from and to
    versions := p.releases.GetLTSVersionsBetween(req.FromVersion, req.ToVersion)

    // Build possible paths
    paths := []UpgradePath{
        p.directPath(req.FromVersion, req.ToVersion),      // Direct jump
        p.stagedPath(req.FromVersion, versions, req.ToVersion), // Via LTS versions
    }

    // If target is new, consider RC path for early testing
    if req.IncludeRCVersions {
        if rc := p.releases.GetLatestRC(req.ToVersion); rc != "" {
            paths = append(paths, p.rcPath(req.FromVersion, rc, req.ToVersion))
        }
    }

    // Analyze each path
    for i := range paths {
        p.analyzePath(ctx, &paths[i], req.ProjectPath)
    }

    // Sort by risk and recommend
    sort.Slice(paths, func(i, j int) bool {
        return paths[i].OverallRisk < paths[j].OverallRisk
    })

    return &PathAnalysisResponse{
        Paths:       paths,
        Recommended: &paths[0],
    }, nil
}

func (p *Pathfinder) directPath(from, to string) UpgradePath {
    return UpgradePath{
        Steps: []UpgradeStep{{FromVersion: from, ToVersion: to}},
    }
}

func (p *Pathfinder) stagedPath(from string, intermediates []string, to string) UpgradePath {
    steps := make([]UpgradeStep, 0, len(intermediates)+1)
    current := from
    for _, v := range intermediates {
        steps = append(steps, UpgradeStep{FromVersion: current, ToVersion: v})
        current = v
    }
    steps = append(steps, UpgradeStep{FromVersion: current, ToVersion: to})
    return UpgradePath{Steps: steps}
}
```

**CLI Output:**

```
bazelbump path --from 7.0.0 --to 9.0.0

UPGRADE PATH ANALYSIS
=====================

Analyzing paths from Bazel 7.0.0 to 9.0.0...

┌─────────────────────────────────────────────────────────────────────────────┐
│  OPTION A: Direct Jump (7.0.0 → 9.0.0)                                      │
├─────────────────────────────────────────────────────────────────────────────┤
│  Risk: HIGH                                                                  │
│                                                                              │
│  Changes to address:                                                         │
│    • 47 breaking changes                                                     │
│    • 12 incompatible flag flips                                             │
│    • 8 dependency updates required                                          │
│                                                                              │
│  Considerations:                                                             │
│    ⚠ Large change set makes debugging failures difficult                    │
│    ⚠ WORKSPACE → Bzlmod migration required                                  │
│    ✓ Fastest if everything works                                            │
└─────────────────────────────────────────────────────────────────────────────┘

┌─────────────────────────────────────────────────────────────────────────────┐
│  OPTION B: Staged via LTS (7.0.0 → 8.0.0 → 9.0.0)            [RECOMMENDED]  │
├─────────────────────────────────────────────────────────────────────────────┤
│  Risk: MEDIUM                                                                │
│                                                                              │
│  Step 1: 7.0.0 → 8.0.0                                                      │
│    • 23 changes (all automated)                                             │
│    • Can verify builds work before proceeding                               │
│                                                                              │
│  Step 2: 8.0.0 → 9.0.0                                                      │
│    • 24 changes (3 require manual review)                                   │
│    • WORKSPACE removal (if still present)                                   │
│                                                                              │
│  Considerations:                                                             │
│    ✓ Incremental validation at each step                                    │
│    ✓ Easier to identify which change caused issues                          │
│    ✓ Can pause and ship at 8.x if needed                                    │
│    ⚠ Takes longer overall                                                   │
└─────────────────────────────────────────────────────────────────────────────┘

┌─────────────────────────────────────────────────────────────────────────────┐
│  OPTION C: Early Testing (7.0.0 → 9.0.0-rc1 → 9.0.0)                        │
├─────────────────────────────────────────────────────────────────────────────┤
│  Risk: MEDIUM-HIGH                                                          │
│                                                                              │
│  Step 1: 7.0.0 → 9.0.0-rc1                                                  │
│    • Test against release candidate                                         │
│    • Find issues before 9.0.0 ships                                         │
│                                                                              │
│  Step 2: 9.0.0-rc1 → 9.0.0                                                  │
│    • Typically minimal changes                                              │
│                                                                              │
│  Considerations:                                                             │
│    ✓ Opportunity to report/fix issues upstream                              │
│    ✓ Be ready on day 1 of 9.0.0 release                                     │
│    ⚠ RC may have bugs not in final release                                  │
└─────────────────────────────────────────────────────────────────────────────┘

Recommendation: OPTION B (Staged via LTS)
  Reason: Safest path with incremental validation checkpoints.

Run 'bazelbump analyze --from 7.0.0 --to 8.0.0' to start Step 1.
```

### Readiness Score

Quantify how ready a project is for upgrade:

```protobuf
// proto/bazelbump/v1/score.proto
syntax = "proto3";
package bazelbump.v1;

message ReadinessScore {
  int32 overall = 1;                  // 0-100
  repeated CategoryScore categories = 2;
  repeated Blocker blockers = 3;
  string effort_estimate = 4;         // "LOW", "MEDIUM", "HIGH"
  string summary = 5;
}

message CategoryScore {
  string name = 1;
  int32 score = 2;                    // 0-100
  int32 weight = 3;                   // Importance weight
  repeated string issues = 4;         // What's dragging the score down
}

message Blocker {
  string description = 1;
  BlockerSeverity severity = 2;
  string remediation = 3;
}

enum BlockerSeverity {
  BLOCKER_SEVERITY_UNSPECIFIED = 0;
  BLOCKER_SEVERITY_HARD = 1;          // Cannot proceed without fixing
  BLOCKER_SEVERITY_SOFT = 2;          // Can proceed but will have issues
}
```

**CLI Output:**

```
bazelbump score --to 9.0.0

UPGRADE READINESS SCORE
=======================

Project: my-project
Target:  Bazel 9.0.0

Overall Score: 72/100  ████████████████████████████░░░░░░░░░░

Category Breakdown:

  Flag Compatibility         85/100  █████████████████████████████████░░░░
    ✓ 12/14 flags are compatible
    ⚠ 2 flags need migration

  Dependency Compatibility   60/100  ████████████████████████░░░░░░░░░░░░░
    ✓ 8/12 dependencies compatible
    ✗ rules_go needs update (0.46.0 → 0.50.0+)
    ✗ rules_python needs update (0.31.0 → 0.35.0+)
    ⚠ 2 dependencies untested with Bazel 9

  Starlark API Usage         80/100  ████████████████████████████████░░░░░
    ✓ No deprecated ctx.* APIs
    ⚠ 3 uses of native.existing_rule() (deprecated)

  Build Configuration        65/100  ██████████████████████████░░░░░░░░░░░
    ✓ MODULE.bazel present
    ⚠ WORKSPACE still exists (needs removal for 9.0)
    ⚠ bazel_compatibility not set

Blockers:

  [HARD] rules_go@0.46.0 incompatible with Bazel 9.0
         Fix: Update to rules_go@0.50.0 or later
         Issue: https://github.com/bazelbuild/rules_go/issues/...

  [SOFT] 3 uses of deprecated native.existing_rule()
         Fix: Migrate to native.existing_rules()
         Files: //tools:macros.bzl, //build:defs.bzl

━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━

Estimated Effort: MEDIUM
  • Automated migrations: ~80%
  • Manual interventions: 5 items
  • Recommended approach: Fix blockers first, then run 'bazelbump apply'

Run 'bazelbump apply --to 9.0.0 --dry-run' to preview all changes.
```

### What-If Simulation

Explore upgrade impact without modifying anything:

```go
// pkg/libbazelbump/simulate.go
package libbazelbump

// Simulate performs a complete upgrade analysis and generates
// the exact changes that would be made, without applying them.
func (c *Client) Simulate(ctx context.Context, req *SimulateRequest) (*SimulateResponse, error) {
    // 1. Run full analysis
    analysis, err := c.Analyze(ctx, &AnalyzeRequest{
        FromVersion: req.FromVersion,
        ToVersion:   req.ToVersion,
        ProjectPath: req.ProjectPath,
    })
    if err != nil {
        return nil, err
    }

    // 2. Generate all transforms (but don't apply)
    transforms := make([]*Transform, 0)
    for _, issue := range analysis.Issues {
        if t := c.transformer.GenerateTransform(issue); t != nil {
            transforms = append(transforms, t)
        }
    }

    // 3. Generate diffs for each transform
    diffs := make([]*FileDiff, 0)
    for _, t := range transforms {
        diff, err := c.transformer.GenerateDiff(t)
        if err != nil {
            continue
        }
        diffs = append(diffs, diff)
    }

    // 4. Identify potential issues
    issues := c.predictor.PredictIssues(transforms)

    // 5. Estimate impact
    impact := c.estimator.EstimateImpact(analysis, transforms)

    return &SimulateResponse{
        Analysis:    analysis,
        Transforms:  transforms,
        Diffs:       diffs,
        PotentialIssues: issues,
        Impact:      impact,
    }, nil
}
```

**CLI Output:**

```
bazelbump simulate --to 9.0.0

UPGRADE SIMULATION
==================

Simulating upgrade to Bazel 9.0.0... (no files will be modified)

FILES THAT WOULD BE MODIFIED (23 total):

  .bazelrc
    - Remove: --experimental_enable_bzlmod (line 15)
    - Remove: --incompatible_strict_action_env=false (line 23)
    - Update: --incompatible_autoload_externally="" (line 31)

  MODULE.bazel
    - Update: bazel_compatibility = [">=9.0.0"]
    - Update: bazel_dep(name = "rules_go", version = "0.50.0")
    - Update: bazel_dep(name = "rules_python", version = "0.35.0")

  src/lib/BUILD (and 18 similar files)
    + Add: load("@rules_cc//cc:defs.bzl", "cc_library", "cc_binary")

  tools/macros.bzl
    - Replace: native.existing_rule() → native.existing_rules()

COMMANDS THAT WOULD RUN:

  1. bazel mod tidy              # Update lockfile
  2. bazel shutdown              # Clear server state
  3. bazel build //...           # Verify build works

POTENTIAL ISSUES DETECTED:

  ⚠ rules_go update may change go_repository behavior
    Impact: External Go dependencies might resolve differently
    Mitigation: Run 'go mod tidy' and verify go.sum

  ⚠ Cache invalidation expected
    Impact: ~15,000 actions will rebuild (full rebuild)
    Reason: --incompatible_strict_action_env flip changes action environment

IMPACT SUMMARY:

  Files modified:        23
  Lines changed:         ~150
  Dependencies updated:  2
  Cache impact:          FULL REBUILD
  Confidence:            HIGH (all changes are deterministic)

━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━

[Export Full Report]  [Show All Diffs]  [Proceed with Apply]
```

**Export Options:**

```bash
# Export simulation as different formats
bazelbump simulate --to 9.0.0 --format json > simulation.json
bazelbump simulate --to 9.0.0 --format markdown > UPGRADE_PLAN.md
bazelbump simulate --to 9.0.0 --format patch > upgrade.patch
```

---

## Execution Model

### Phased Execution (Like Bazel)

```
┌─────────────────────────────────────────────────────────────────────────────┐
│                         EXECUTION PHASES                                     │
├─────────────────────────────────────────────────────────────────────────────┤
│                                                                              │
│  ┌─────────────┐    ┌─────────────┐    ┌─────────────┐    ┌─────────────┐  │
│  │   LOADING   │ ─▶ │  ANALYSIS   │ ─▶ │  PLANNING   │ ─▶ │  EXECUTION  │  │
│  └─────────────┘    └─────────────┘    └─────────────┘    └─────────────┘  │
│                                                                              │
│  LOADING PHASE                                                               │
│  ├── Read configuration (layered precedence)                                │
│  ├── Discover project files (.bazelrc, MODULE.bazel, BUILD, .bzl)          │
│  ├── Parse file contents (AST)                                              │
│  ├── Load flag database (embedded, versioned)                               │
│  └── Load recipes (if applicable)                                           │
│  Output: ProjectGraph (immutable snapshot of inputs)                        │
│  Cacheable: YES (by content hash)                                           │
│                                                                              │
│  ANALYSIS PHASE (READ-ONLY)                                                  │
│  ├── Detect current Bazel version                                           │
│  ├── Compare against target version                                         │
│  ├── Identify flag deprecations/removals                                    │
│  ├── Identify Starlark API deprecations                                     │
│  ├── Check dependency compatibility                                         │
│  ├── Scan for native rule usage                                             │
│  └── Aggregate issues by severity/category                                  │
│  Output: AnalysisResult (list of issues, no side effects)                   │
│  Cacheable: YES (by ProjectGraph hash + target version)                     │
│                                                                              │
│  PLANNING PHASE                                                              │
│  ├── Generate transforms for each issue                                     │
│  ├── Order transforms (dependencies)                                        │
│  ├── Check for conflicts                                                    │
│  ├── Generate diffs/previews                                                │
│  └── Calculate confidence scores                                            │
│  Output: ExecutionPlan (ordered transforms with previews)                   │
│  Cacheable: YES (by AnalysisResult hash)                                    │
│                                                                              │
│  EXECUTION PHASE (WRITE)                                                     │
│  ├── Create snapshot (safety)                                               │
│  ├── Apply transforms                                                       │
│  ├── Run formatters (buildifier)                                            │
│  ├── Verify (optional build check)                                          │
│  └── Report results                                                         │
│  Output: ExecutionResult (changes made, verification status)                │
│  Cacheable: NO (has side effects)                                           │
│                                                                              │
└─────────────────────────────────────────────────────────────────────────────┘
```

### Caching Architecture

```go
// internal/cache/cache.go
package cache

// Cache provides multi-layer caching for hermetic, fast analysis
type Cache struct {
    // Layer 1: In-memory (process lifetime)
    memory *MemoryCache

    // Layer 2: On-disk (persistent, content-addressed)
    disk *DiskCache

    // Layer 3: Remote (optional, shared across machines)
    remote RemoteCache
}

// ContentAddressable stores items by content hash
type ContentAddressable interface {
    // Key is SHA256 of content
    Get(key string) ([]byte, bool)
    Put(key string, value []byte) error
    Has(key string) bool
}

// CacheKey for analysis results
type AnalysisCacheKey struct {
    ProjectHash   string   // Hash of all input files
    TargetVersion string   // Target Bazel version
    FlagDBVersion string   // Flag database version
    ConfigHash    string   // Configuration hash
}

func (k *AnalysisCacheKey) String() string {
    return sha256(fmt.Sprintf("%s:%s:%s:%s",
        k.ProjectHash, k.TargetVersion, k.FlagDBVersion, k.ConfigHash))
}
```

**Cache Invalidation:**

```go
// Invalidation rules (like Bazel's action cache)
type InvalidationRule int
const (
    // Content changed
    InvalidateOnContentChange InvalidationRule = iota

    // Config changed
    InvalidateOnConfigChange

    // Time-based (for external data)
    InvalidateOnTTL
)

// Per-cache-type TTLs
var CacheTTLs = map[string]time.Duration{
    "bcr":          1 * time.Hour,    // BCR responses
    "github":       1 * time.Hour,    // GitHub API
    "analysis":     0,                // Never expires (content-addressed)
    "flagdb":       0,                // Never expires (versioned)
}
```

### Query Language

```bash
# Query issues by various criteria (like bazel query)
bazelbump query "severity:critical"
bazelbump query "category:flag"
bazelbump query "file://src/lib/..."
bazelbump query "rule:cc_library"
bazelbump query "severity:high AND category:starlark"
bazelbump query "NOT file://experimental/..."

# Output formats
bazelbump query "severity:critical" --output=table
bazelbump query "severity:critical" --output=json
bazelbump query "severity:critical" --output=paths  # Just file paths

# Count
bazelbump query "category:flag" --count

# With analysis cache
bazelbump query "severity:critical" --cache  # Use cached analysis
```

**Query Grammar:**

```
query       := expression
expression  := term (('AND' | 'OR') term)*
term        := 'NOT'? factor
factor      := '(' expression ')' | predicate
predicate   := key ':' value | key '://' pattern

# Keys
severity    := 'critical' | 'high' | 'medium' | 'low' | 'info'
category    := 'flag' | 'starlark' | 'dependency' | 'config' | 'rule'
file        := glob pattern (e.g., file://src/...)
rule        := rule name (e.g., rule:cc_library)
transform   := 'deterministic' | 'heuristic' | 'manual'
```

---

## CLI Interface

### Porcelain vs Plumbing

Following Git's successful pattern, bazelbump has two command layers:

```
┌─────────────────────────────────────────────────────────────────────────────┐
│                      PORCELAIN vs PLUMBING                                   │
├─────────────────────────────────────────────────────────────────────────────┤
│                                                                              │
│  PORCELAIN (User-facing, opinionated, great UX)                             │
│  ─────────────────────────────────────────────                              │
│  For: Humans, common workflows, getting things done                         │
│                                                                              │
│  bazelbump check          "Is my project ready for upgrade?"                │
│  bazelbump apply          "Upgrade me to version X"                         │
│  bazelbump rollback       "Undo the last upgrade"                           │
│  bazelbump fix            "Fix this specific issue"                         │
│  bazelbump prepare        "Get ready for future upgrade"                    │
│  bazelbump status         "Where am I in the migration?"                    │
│                                                                              │
│  PLUMBING (Low-level, composable, scriptable)                               │
│  ────────────────────────────────────────────                               │
│  For: Scripts, automation, power users, building tools on top              │
│                                                                              │
│  # Analysis primitives                                                       │
│  bazelbump analyze-file   Analyze a single file                             │
│  bazelbump analyze-flags  Analyze flags only                                │
│  bazelbump analyze-deps   Analyze dependencies only                         │
│                                                                              │
│  # Transform primitives                                                      │
│  bazelbump transform      Apply a single transform                          │
│  bazelbump diff           Show diff for a transform                         │
│                                                                              │
│  # Snapshot primitives                                                       │
│  bazelbump snapshot-create   Create snapshot                                │
│  bazelbump snapshot-restore  Restore snapshot                               │
│  bazelbump snapshot-list     List snapshots                                 │
│  bazelbump snapshot-gc       Garbage collect                                │
│                                                                              │
│  # Cache primitives                                                          │
│  bazelbump cache-get      Get cached analysis                               │
│  bazelbump cache-put      Store analysis in cache                           │
│  bazelbump cache-invalidate  Invalidate cache entries                       │
│  bazelbump cache-stats    Cache statistics                                  │
│                                                                              │
│  # Database primitives                                                       │
│  bazelbump db-flag        Query flag database                               │
│  bazelbump db-starlark    Query Starlark deprecation database               │
│  bazelbump db-version     Show database versions                            │
│                                                                              │
│  # Recipe primitives                                                         │
│  bazelbump recipe-parse   Parse and validate recipe                         │
│  bazelbump recipe-expand  Expand recipe to transforms                       │
│                                                                              │
└─────────────────────────────────────────────────────────────────────────────┘
```

**Composition Examples:**

```bash
# Power user: Custom analysis pipeline
bazelbump analyze-flags --to 9.0.0 --format json | \
  jq '.issues[] | select(.severity == "critical")' | \
  bazelbump transform --stdin

# CI: Cache-aware incremental analysis
CACHE_KEY=$(bazelbump cache-key --to 9.0.0)
if ! bazelbump cache-get "$CACHE_KEY" > analysis.json; then
  bazelbump analyze --to 9.0.0 --format json > analysis.json
  bazelbump cache-put "$CACHE_KEY" < analysis.json
fi

# Automation: Apply specific transforms
bazelbump analyze --to 9.0.0 --format json | \
  jq '.transforms[] | select(.category == "imports")' | \
  bazelbump transform --stdin --dry-run

# Debugging: See exactly what's in the flag database
bazelbump db-flag --name experimental_enable_bzlmod
bazelbump db-flag --deprecated-in 8.0.0 --removed-in 9.0.0 --format table
```

### Commands Overview

```
┌─────────────────────────────────────────────────────────────────────────────┐
│                           BAZELBUMP COMMANDS                                 │
├─────────────────────────────────────────────────────────────────────────────┤
│                                                                              │
│  CORE COMMANDS (🔴 Paramount)                                               │
│  ├── analyze       Full upgrade analysis between versions                   │
│  ├── check         Quick compatibility check for current project            │
│  ├── apply         Apply migrations (creates snapshot automatically)        │
│  ├── preflight     Run health checks before upgrade                         │
│  ├── rollback      Restore from snapshot if upgrade fails                   │
│  └── simulate      What-if analysis without modifying files                 │
│                                                                              │
│  INTELLIGENCE COMMANDS (🟠 High Priority)                                   │
│  ├── path          Find optimal upgrade path between versions               │
│  ├── score         Calculate upgrade readiness score                        │
│  └── flags         List flag changes between versions                       │
│                                                                              │
│  MANAGEMENT COMMANDS                                                         │
│  ├── snapshot      Manage upgrade snapshots                                 │
│  ├── cache         Manage data cache                                        │
│  ├── lockfile      Check/migrate MODULE.bazel.lock                          │
│  └── update-module Update MODULE.bazel compatibility                        │
│                                                                              │
│  EXTENSIBILITY COMMANDS (🟠 High Priority)                                  │
│  ├── plugin        Manage plugins                                           │
│  └── recipe        Manage migration recipes                                 │
│                                                                              │
└─────────────────────────────────────────────────────────────────────────────┘
```

### Core Commands

```bash
# Analyze upgrade path (default command)
bazelbump analyze [--from VERSION] --to VERSION [--path DIR]
    --include-starlark          # Also scan for Starlark API deprecations
    --include-cache-impact      # Analyze cache invalidation
    --from-build-log FILE       # Analyze from existing build output
    --from-bep FILE             # Analyze from Build Event Protocol file

# Check current project compatibility
bazelbump check [--path DIR]
    --ci                        # CI-friendly output (exit codes, formats)
    --format FORMAT             # Output: text, json, sarif, junit, github-annotations

# Apply migrations automatically
bazelbump apply [--from VERSION] --to VERSION [--path DIR]
    --dry-run                   # Preview changes without applying
    --no-snapshot               # Skip snapshot creation (not recommended)
    --skip-preflight            # Skip health checks (not recommended)
    --force                     # Proceed despite warnings
    --packages PATTERN          # Only apply to specific packages (monorepo)
    --incremental               # Apply in batches (monorepo)

# Pre-flight health checks
bazelbump preflight [--path DIR]
    --skip-build                # Skip build verification
    --skip-tests                # Skip test verification

# Restore from snapshot
bazelbump rollback [SNAPSHOT_ID]
    --latest                    # Rollback to most recent snapshot
    --force                     # Override modified file check
    --list                      # Alias for 'snapshot list'

# What-if simulation
bazelbump simulate [--from VERSION] --to VERSION [--path DIR]
    --format FORMAT             # Output: text, json, markdown, patch
    --output FILE               # Write to file instead of stdout
```

### Intelligence Commands

```bash
# Find optimal upgrade path
bazelbump path --from VERSION --to VERSION
    --include-rc                # Include release candidates as options

# Calculate readiness score
bazelbump score [--to VERSION] [--path DIR]
    --format FORMAT             # Output: text, json
    --categories                # Show detailed category breakdown

# List flag changes between versions
bazelbump flags --from VERSION --to VERSION
    --format FORMAT             # Output: text, json, table
    --deprecated-only           # Only show deprecated flags
    --removed-only              # Only show removed flags
```

### Management Commands

```bash
# Snapshot management
bazelbump snapshot list
bazelbump snapshot show SNAPSHOT_ID
bazelbump snapshot delete SNAPSHOT_ID
bazelbump snapshot prune [--keep N]   # Keep last N snapshots

# Cache management
bazelbump cache update              # Refresh cached data
bazelbump cache clear               # Clear all cached data
bazelbump cache info                # Show cache status

# Lockfile management
bazelbump lockfile check [--to VERSION]
bazelbump lockfile migrate [--to VERSION]

# MODULE.bazel updates
bazelbump update-module --to VERSION [--path DIR]
```

### Quick Fix Commands [🟠 HIGH PRIORITY]

```bash
# Targeted fixes without full upgrade
bazelbump fix imports                           # Add missing load() statements
bazelbump fix imports --rule cc_library         # Only for specific rule
bazelbump fix imports --package //src/lib/...   # Only in specific package

bazelbump fix flags                             # Migrate deprecated flags
bazelbump fix flags --flag experimental_enable_bzlmod  # Specific flag only

bazelbump fix starlark                          # Update deprecated Starlark APIs
bazelbump fix starlark --api native.existing_rule     # Specific API only

bazelbump fix dependency rules_go --version 0.50.0    # Update single dependency

bazelbump fix --list                            # Show available fix types
```

### Prepare Mode Commands [🟠 HIGH PRIORITY]

```bash
# Prepare for upgrade without changing .bazelversion
bazelbump prepare --for 9.0.0
bazelbump prepare --for 9.0.0 --only imports    # Only add load() statements
bazelbump prepare --for 9.0.0 --only flags      # Only migrate flags
bazelbump prepare --for 9.0.0 --test            # Test with target version
```

### Extensibility Commands

```bash
# Plugin management
bazelbump plugin list
bazelbump plugin install PATH_OR_URL
bazelbump plugin enable NAME
bazelbump plugin disable NAME
bazelbump plugin info NAME
bazelbump plugin remove NAME

# Recipe management
bazelbump recipe search QUERY
bazelbump recipe show RECIPE_NAME
bazelbump recipe steps RECIPE_NAME              # Show recipe steps
bazelbump recipe apply RECIPE_NAME [--dry-run]
bazelbump recipe run RECIPE_NAME --step N       # Run specific step only
bazelbump recipe run RECIPE_NAME --steps 1,2,3  # Run specific steps
bazelbump recipe run RECIPE_NAME --from N       # Start from step N
bazelbump recipe run RECIPE_NAME --until N      # Stop before step N
bazelbump recipe run RECIPE_NAME --skip 2,3     # Skip specific steps
bazelbump recipe init NAME                      # Create new recipe template
```

### Monorepo & Exclusion Commands [🟠 HIGH PRIORITY]

```bash
# Known issues management
bazelbump known-issues list
bazelbump known-issues add TARGET --reason "text" [--jira TICKET]
bazelbump known-issues remove TARGET
bazelbump known-issues prune                    # Remove expired issues

# Migration status (for large migrations)
bazelbump status                                # Show overall progress
bazelbump status --by-team                      # Progress by team
bazelbump status mark-done PACKAGE              # Mark manually completed

# Verification with exclusions
bazelbump verify                                # Verify build works
bazelbump verify --allow-failures               # Report but don't fail
bazelbump verify --only-new-failures            # Ignore pre-existing failures
```

### Global Flags

```bash
--path DIR          # Project root (default: current directory)
--verbose, -v       # Verbose output
--quiet, -q         # Suppress non-essential output
--format FORMAT     # Output format: text, json, yaml (default: text)
--color             # Force color output (default: auto)
--no-color          # Disable color output
--offline           # Work offline with cached data only
--config FILE       # Custom config file (default: .bazelbump/config.yaml)

# Exclusion flags (available on most commands)
--exclude PATTERN           # Exclude packages (repeatable)
--exclude-targets TARGETS   # Exclude specific targets
--exclude-from FILE         # Read exclusions from file
--only PATTERN              # Include only these packages (inverse of exclude)
--respect-ignore            # Use .bazelbumpignore (default: true)
--no-ignore                 # Don't use .bazelbumpignore
```

### Examples

```bash
# === Basic Workflow ===

# 1. Check current status
bazelbump check

# 2. See upgrade readiness
bazelbump score --to 9.0.0

# 3. Analyze what needs to change
bazelbump analyze --to 9.0.0

# 4. Preview changes without applying
bazelbump simulate --to 9.0.0

# 5. Apply migrations (creates snapshot automatically)
bazelbump apply --to 9.0.0

# 6. If something breaks, rollback
bazelbump rollback --latest


# === Advanced Usage ===

# Find best upgrade path for big version jump
bazelbump path --from 7.0.0 --to 9.0.0

# Analyze including Starlark API changes
bazelbump analyze --to 9.0.0 --include-starlark

# Analyze from existing build output
bazel build //... 2>&1 | tee build.log
bazelbump analyze --from-build-log build.log

# CI-friendly check
bazelbump check --ci --format sarif > results.sarif

# Monorepo: upgrade specific package
bazelbump apply --to 9.0.0 --packages //src/lib/...

# Monorepo: incremental upgrade
bazelbump apply --to 9.0.0 --incremental --batch-size 50

# Export simulation as upgrade plan
bazelbump simulate --to 9.0.0 --format markdown > UPGRADE_PLAN.md


# === Plugin & Recipe Usage ===

# Install company-specific plugin
bazelbump plugin install ./tools/bazelbump-mycompany

# Search for migration recipes
bazelbump recipe search rules_go

# Apply a community recipe
bazelbump recipe apply rules_go_0.46_to_0.50 --dry-run


# === Cherry-Pick & Incremental Preparation ===

# Just add missing imports (don't do full upgrade)
bazelbump fix imports

# Just add imports for cc_library in specific package
bazelbump fix imports --rule cc_library --package //src/core/...

# Just migrate one specific flag
bazelbump fix flags --flag experimental_enable_bzlmod

# Prepare for 9.0 but stay on 8.x
bazelbump prepare --for 9.0.0

# Run specific recipe steps only
bazelbump recipe steps rules_go_0.46_to_0.50      # See steps
bazelbump recipe run rules_go_0.46_to_0.50 --step 1  # Run step 1 only


# === Monorepo & Exclusions ===

# Analyze excluding specific packages
bazelbump analyze --to 9.0.0 --exclude //experimental/...

# Analyze only your team's packages
bazelbump analyze --to 9.0.0 --only //services/backend/...

# Use exclusion file
bazelbump analyze --to 9.0.0 --exclude-from .bazelbumpignore

# Manage known broken targets
bazelbump known-issues add //legacy:broken --reason "Deprecated" --jira PLAT-123
bazelbump known-issues list

# Verify build (ignoring known issues)
bazelbump verify --allow-failures

# Track migration progress in large repo
bazelbump status
bazelbump status --by-team


# === Snapshot Management ===

# List all snapshots
bazelbump snapshot list

# Rollback to specific snapshot
bazelbump rollback 01HQ3KXYZ

# Clean up old snapshots
bazelbump snapshot prune --keep 5
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

## Deep Analysis [🔴 PARAMOUNT + 🟠 HIGH]

Beyond flags, bazelbump provides comprehensive analysis of all upgrade impacts.

### Starlark API Deprecation Tracking [🔴 PARAMOUNT]

Flags aren't the only thing that changes—Starlark APIs evolve too:

```protobuf
// proto/bazelbump/v1/starlark.proto
syntax = "proto3";
package bazelbump.v1;

message StarlarkAPIChange {
  string api = 1;                     // e.g., "ctx.host_configuration"
  string deprecated_in = 2;
  string removed_in = 3;
  string replacement = 4;
  string pattern = 5;                 // Regex to detect usage
  string migration_example = 6;
}

message StarlarkUsage {
  string file = 1;
  int32 line = 2;
  string api = 3;
  string context = 4;                 // Surrounding code
  StarlarkAPIChange change = 5;
}
```

**Built-in API Deprecation Database:**

```go
// internal/starlark/deprecations.go
package starlark

// APIDeprecations tracks deprecated Starlark APIs across Bazel versions
var APIDeprecations = []APIChange{
    // Context APIs
    {
        API:          "ctx.host_configuration",
        DeprecatedIn: "7.0.0",
        RemovedIn:    "8.0.0",
        Replacement:  "ctx.configuration",
        Pattern:      `ctx\.host_configuration`,
    },
    {
        API:          "ctx.host_fragments",
        DeprecatedIn: "7.0.0",
        RemovedIn:    "8.0.0",
        Replacement:  "ctx.fragments",
        Pattern:      `ctx\.host_fragments`,
    },

    // Native rule APIs
    {
        API:          "native.existing_rule",
        DeprecatedIn: "7.0.0",
        RemovedIn:    "9.0.0",
        Replacement:  "native.existing_rules()",
        Pattern:      `native\.existing_rule\s*\(`,
        MigrationExample: `
# Before
rule = native.existing_rule("target_name")

# After
rules = native.existing_rules()
rule = rules.get("target_name")
`,
    },
    {
        API:          "native.cc_library",
        DeprecatedIn: "8.0.0",
        RemovedIn:    "9.0.0",
        Replacement:  "load from @rules_cc",
        Pattern:      `native\.cc_library`,
    },

    // Transition APIs
    {
        API:          "cfg = \"host\"",
        DeprecatedIn: "7.0.0",
        RemovedIn:    "8.0.0",
        Replacement:  "cfg = \"exec\"",
        Pattern:      `cfg\s*=\s*["']host["']`,
    },

    // Aspect APIs
    {
        API:          "aspect.make_variables",
        DeprecatedIn: "6.0.0",
        RemovedIn:    "7.0.0",
        Replacement:  "Use TemplateVariableInfo provider",
        Pattern:      `make_variables\s*=`,
    },

    // ... many more from Bazel source
}

// ScanFile finds deprecated API usage in a .bzl or BUILD file
func ScanFile(path string, content []byte, targetVersion string) []StarlarkUsage {
    var usages []StarlarkUsage

    for _, dep := range APIDeprecations {
        // Skip if not relevant to target version
        if !dep.AffectsVersion(targetVersion) {
            continue
        }

        re := regexp.MustCompile(dep.Pattern)
        matches := re.FindAllIndex(content, -1)

        for _, match := range matches {
            line := countLines(content[:match[0]])
            usages = append(usages, StarlarkUsage{
                File:    path,
                Line:    line,
                API:     dep.API,
                Context: extractContext(content, match[0], match[1]),
                Change:  &dep,
            })
        }
    }

    return usages
}
```

**CLI Output:**

```
bazelbump analyze --to 9.0.0 --include-starlark

STARLARK API DEPRECATIONS
=========================

Found 7 deprecated API usages in 4 files:

  tools/macros.bzl:45
    │ API: native.existing_rule()
    │ Status: Removed in Bazel 9.0
    │ Fix: Use native.existing_rules().get("name") instead
    │
    │   44│ def check_target(name):
    │   45│     rule = native.existing_rule(name)  # ← DEPRECATED
    │   46│     if rule:

  tools/macros.bzl:78
    │ API: native.existing_rule()
    │ (same as above)

  build/transitions.bzl:23
    │ API: cfg = "host"
    │ Status: Removed in Bazel 8.0
    │ Fix: Use cfg = "exec" instead
    │
    │   22│ my_rule = rule(
    │   23│     cfg = "host",  # ← DEPRECATED
    │   24│     ...

  src/rules/my_rule.bzl:156
    │ API: ctx.host_configuration
    │ Status: Removed in Bazel 8.0
    │ Fix: Use ctx.configuration instead
```

### Build Log Analysis [🟠 HIGH PRIORITY]

Parse existing build output to detect deprecation warnings:

```go
// internal/buildlog/analyzer.go
package buildlog

import (
    "bufio"
    "regexp"
)

type Warning struct {
    Type        WarningType
    Message     string
    Flag        string           // For flag warnings
    File        string           // For file-specific warnings
    Line        int
    Severity    Severity
    Actionable  bool             // Can bazelbump fix this?
    Suggestion  string
}

type WarningType int
const (
    WarningTypeFlag WarningType = iota
    WarningTypeAPI
    WarningTypeRule
    WarningTypeDependency
    WarningTypeOther
)

var warningPatterns = []*regexp.Regexp{
    // Flag deprecation warnings
    regexp.MustCompile(`WARNING: Option '([^']+)' is deprecated`),
    regexp.MustCompile(`WARNING: --([a-z_]+) is deprecated`),

    // Starlark API warnings
    regexp.MustCompile(`WARNING: ([^:]+):(\d+):\d+: ([^.]+) is deprecated`),

    // Rule warnings
    regexp.MustCompile(`WARNING: Use of native\.(\w+) is deprecated`),

    // Incompatible flag warnings
    regexp.MustCompile(`WARNING: --incompatible_(\w+) will be enabled by default in Bazel (\d+\.\d+)`),
}

// AnalyzeBuildLog extracts actionable warnings from build output
func AnalyzeBuildLog(log io.Reader) ([]Warning, error) {
    var warnings []Warning
    scanner := bufio.NewScanner(log)

    for scanner.Scan() {
        line := scanner.Text()
        for _, pattern := range warningPatterns {
            if matches := pattern.FindStringSubmatch(line); matches != nil {
                warning := parseWarning(pattern, matches)
                warnings = append(warnings, warning)
            }
        }
    }

    return deduplicate(warnings), nil
}

// AnalyzeBEP parses Build Event Protocol for richer data
func AnalyzeBEP(bepPath string) (*BuildAnalysis, error) {
    // BEP provides structured build events including:
    // - Action execution details
    // - Configuration info
    // - Diagnostic messages
    // Parse and extract relevant warnings/errors
}
```

**CLI Integration:**

```bash
# Analyze from build log file
bazelbump analyze --from-build-log ./build.log

# Analyze from BEP (Build Event Protocol)
bazelbump analyze --from-bep ./bep.json

# Pipe directly from bazel
bazel build //... 2>&1 | bazelbump analyze --from-stdin

# Or generate BEP and analyze
bazel build //... --build_event_json_file=bep.json
bazelbump analyze --from-bep bep.json
```

### Cache Impact Analysis [🟠 HIGH PRIORITY]

Warn about changes that will invalidate the action cache:

```protobuf
// proto/bazelbump/v1/cache.proto
syntax = "proto3";
package bazelbump.v1;

message CacheImpact {
  CacheImpactLevel level = 1;
  int32 estimated_actions_invalidated = 2;
  repeated CacheInvalidationReason reasons = 3;
  string estimated_rebuild_time = 4;   // Based on historical data
  repeated string recommendations = 5;
}

enum CacheImpactLevel {
  CACHE_IMPACT_UNSPECIFIED = 0;
  CACHE_IMPACT_NONE = 1;              // No cache impact
  CACHE_IMPACT_PARTIAL = 2;           // Some actions rebuild
  CACHE_IMPACT_FULL = 3;              // Full rebuild expected
}

message CacheInvalidationReason {
  string change = 1;                  // What's changing
  string impact = 2;                  // What it affects
  int32 actions_affected = 3;
}
```

**Implementation:**

```go
// internal/cache/impact.go
package cache

// Known cache-invalidating changes
var cacheInvalidators = []CacheInvalidator{
    {
        Flag:   "--incompatible_strict_action_env",
        Impact: CacheImpactFull,
        Reason: "Changes action environment, invalidating all actions",
    },
    {
        Flag:   "--incompatible_enable_cc_toolchain_resolution",
        Impact: CacheImpactPartial,
        Reason: "Changes C++ toolchain resolution, invalidating C++ actions",
    },
    {
        Pattern: "rules_go version change",
        Impact:  CacheImpactPartial,
        Reason:  "Go toolchain change invalidates Go compilation actions",
    },
    // ... more patterns
}

func AnalyzeCacheImpact(changes []Change, project *Project) *CacheImpact {
    impact := &CacheImpact{Level: CacheImpactNone}

    for _, change := range changes {
        for _, inv := range cacheInvalidators {
            if inv.Matches(change) {
                impact.Reasons = append(impact.Reasons, &CacheInvalidationReason{
                    Change:          change.Description,
                    Impact:          inv.Reason,
                    ActionsAffected: estimateActions(inv, project),
                })
                if inv.Impact > impact.Level {
                    impact.Level = inv.Impact
                }
            }
        }
    }

    // Estimate rebuild time based on historical data
    impact.EstimatedRebuildTime = estimateRebuildTime(impact, project)

    // Add recommendations
    if impact.Level == CacheImpactFull {
        impact.Recommendations = append(impact.Recommendations,
            "Schedule upgrade during low-traffic period",
            "Consider warming cache after upgrade with 'bazel build //...'",
            "If using remote cache, coordinate with team",
        )
    }

    return impact
}
```

**CLI Output:**

```
CACHE IMPACT ANALYSIS
=====================

The following changes will affect your action cache:

  FULL REBUILD EXPECTED
  └── --incompatible_strict_action_env flip
      Changes action environment for all actions
      Affected: ~15,000 actions

  PARTIAL REBUILD
  └── rules_go update (0.46.0 → 0.50.0)
      Go toolchain change
      Affected: ~3,000 Go compilation actions

  NO IMPACT
  └── Flag removals (--experimental_enable_bzlmod)
      These flags are no-ops and don't affect cache

Summary:
  Cache impact level:      FULL REBUILD
  Estimated actions:       ~18,000 will rebuild
  Estimated rebuild time:  ~45 minutes (based on build history)

Recommendations:
  • Schedule upgrade during low-traffic period
  • If using remote cache, coordinate with team
  • First build after upgrade will be slow
```

### Lockfile Migration [🔴 PARAMOUNT]

Handle MODULE.bazel.lock format changes between Bazel versions:

```go
// internal/lockfile/migrate.go
package lockfile

// Lockfile format versions by Bazel version
var lockfileFormats = map[string]int{
    "7.0.0": 3,
    "7.1.0": 4,
    "8.0.0": 5,
    "9.0.0": 6,  // Hypothetical
}

type LockfileMigrator struct{}

// CheckCompatibility verifies lockfile works with target Bazel
func (m *LockfileMigrator) CheckCompatibility(lockPath, targetVersion string) (*LockfileStatus, error) {
    lock, err := m.parse(lockPath)
    if err != nil {
        return nil, err
    }

    targetFormat := lockfileFormats[majorVersion(targetVersion)]

    return &LockfileStatus{
        CurrentFormat: lock.LockfileVersion,
        TargetFormat:  targetFormat,
        Compatible:    lock.LockfileVersion <= targetFormat,
        Action:        m.recommendAction(lock.LockfileVersion, targetFormat),
    }, nil
}

// Migrate updates lockfile for new Bazel version
func (m *LockfileMigrator) Migrate(lockPath, targetVersion string) error {
    // Most lockfile migrations are handled by Bazel itself
    // Run: bazel mod deps --lockfile_mode=update
    return nil
}

type LockfileStatus struct {
    CurrentFormat int
    TargetFormat  int
    Compatible    bool
    Action        string  // "none", "regenerate", "manual"
}
```

**CLI Output:**

```
bazelbump lockfile check --to 9.0.0

LOCKFILE COMPATIBILITY
======================

  Current lockfile version: 5 (Bazel 8.x format)
  Target lockfile version:  6 (Bazel 9.x format)

  Status: MIGRATION REQUIRED

  The lockfile format changed in Bazel 9.0. After upgrading:
    1. Run: bazel mod deps --lockfile_mode=update
    2. Commit the updated MODULE.bazel.lock

  Alternatively, bazelbump can handle this automatically:
    bazelbump apply --to 9.0.0  # Updates lockfile as part of upgrade
```

---

## Extensibility & Plugins [🟠 HIGH PRIORITY]

bazelbump is designed for extensibility without becoming a mess.

### Plugin Architecture

**Design Principles:**

1. **Minimal Interface** — Plugins implement a small, stable interface
2. **Isolation** — Plugins can't break core functionality
3. **Discoverability** — Easy to find and install community plugins
4. **Versioning** — Plugins declare compatible bazelbump versions

```protobuf
// proto/bazelbump/v1/plugin.proto
syntax = "proto3";
package bazelbump.v1;

// PluginManifest describes a bazelbump plugin
message PluginManifest {
  string name = 1;                    // e.g., "mycompany-rules"
  string version = 2;                 // semver
  string description = 3;
  string author = 4;
  string homepage = 5;
  repeated string bazelbump_versions = 6;  // Compatible versions: [">=1.0.0", "<2.0.0"]
  repeated PluginCapability capabilities = 7;
}

enum PluginCapability {
  PLUGIN_CAPABILITY_UNSPECIFIED = 0;
  PLUGIN_CAPABILITY_ANALYZER = 1;     // Can analyze projects
  PLUGIN_CAPABILITY_TRANSFORMER = 2;  // Can generate transforms
  PLUGIN_CAPABILITY_CHECKER = 3;      // Can add preflight checks
  PLUGIN_CAPABILITY_DATASOURCE = 4;   // Can provide additional data
}
```

**Plugin Interface (Go):**

```go
// pkg/plugin/plugin.go
package plugin

import (
    "context"
    pb "github.com/albertocavalcante/bazelbump/proto/bazelbump/v1"
)

// Plugin is the interface all plugins must implement
type Plugin interface {
    // Manifest returns plugin metadata
    Manifest() *pb.PluginManifest

    // Initialize is called once when plugin is loaded
    Initialize(ctx context.Context, config *Config) error

    // Shutdown is called when plugin is unloaded
    Shutdown(ctx context.Context) error
}

// Analyzer plugins can detect additional issues
type Analyzer interface {
    Plugin
    // Analyze examines the project and returns issues
    Analyze(ctx context.Context, project *Project, targetVersion string) ([]*pb.Issue, error)
}

// Transformer plugins can generate fixes
type Transformer interface {
    Plugin
    // CanTransform checks if this plugin handles the issue
    CanTransform(issue *pb.Issue) bool
    // Transform generates a fix for the issue
    Transform(ctx context.Context, issue *pb.Issue, file *File) (*pb.Transform, error)
}

// Checker plugins add preflight checks
type Checker interface {
    Plugin
    // Checks returns additional preflight checks
    Checks() []Check
}

// DataSource plugins provide additional data
type DataSource interface {
    Plugin
    // FlagChanges returns additional flag deprecation data
    FlagChanges(from, to string) ([]*pb.FlagChange, error)
    // APIChanges returns additional API deprecation data
    APIChanges(from, to string) ([]*pb.StarlarkAPIChange, error)
}
```

**Example Plugin Implementation:**

```go
// plugins/mycompany-rules/plugin.go
package main

import (
    "context"
    "github.com/albertocavalcante/bazelbump/pkg/plugin"
    pb "github.com/albertocavalcante/bazelbump/proto/bazelbump/v1"
)

type MyCompanyPlugin struct{}

func (p *MyCompanyPlugin) Manifest() *pb.PluginManifest {
    return &pb.PluginManifest{
        Name:        "mycompany-rules",
        Version:     "1.0.0",
        Description: "Support for MyCompany internal Bazel rules",
        Author:      "MyCompany Platform Team",
        BazelbumpVersions: []string{">=1.0.0"},
        Capabilities: []pb.PluginCapability{
            pb.PluginCapabilityAnalyzer,
            pb.PluginCapabilityTransformer,
        },
    }
}

func (p *MyCompanyPlugin) Initialize(ctx context.Context, cfg *plugin.Config) error {
    return nil
}

func (p *MyCompanyPlugin) Shutdown(ctx context.Context) error {
    return nil
}

// Analyzer implementation
func (p *MyCompanyPlugin) Analyze(ctx context.Context, proj *plugin.Project, targetVersion string) ([]*pb.Issue, error) {
    var issues []*pb.Issue

    // Check for MyCompany-specific rule usage that changed
    files, _ := proj.FindFiles("**/BUILD", "**/BUILD.bazel")
    for _, f := range files {
        content, _ := f.Read()
        if containsDeprecatedMyCompanyRule(content, targetVersion) {
            issues = append(issues, &pb.Issue{
                Severity:    pb.SeverityHigh,
                Category:    "mycompany-rules",
                Description: "my_company_rule() deprecated in Bazel " + targetVersion,
                File:        f.Path,
                Remediation: "Use my_company_rule_v2() instead",
            })
        }
    }

    return issues, nil
}

// Transformer implementation
func (p *MyCompanyPlugin) CanTransform(issue *pb.Issue) bool {
    return issue.Category == "mycompany-rules"
}

func (p *MyCompanyPlugin) Transform(ctx context.Context, issue *pb.Issue, file *plugin.File) (*pb.Transform, error) {
    // Generate transform to update rule usage
    return &pb.Transform{
        File:    issue.File,
        Type:    pb.TransformTypeReplace,
        Search:  "my_company_rule(",
        Replace: "my_company_rule_v2(",
    }, nil
}

// Export as plugin
var Plugin MyCompanyPlugin
```

**Plugin Configuration:**

```yaml
# .bazelbump/config.yaml
plugins:
  # Local plugin (for internal/proprietary rules)
  - path: ./tools/bazelbump-plugins/mycompany-rules
    enabled: true
    config:
      rule_prefix: "mycompany_"

  # Remote plugin (from Git)
  - git: https://github.com/myorg/bazelbump-mycompany
    version: v1.2.0
    enabled: true

  # Published plugin (from registry - future)
  - name: bazelbump-plugin-kubernetes
    version: ">=1.0.0"
```

**Plugin CLI:**

```bash
# List installed plugins
bazelbump plugin list

# Install a plugin
bazelbump plugin install ./path/to/plugin
bazelbump plugin install https://github.com/org/plugin

# Enable/disable plugins
bazelbump plugin enable mycompany-rules
bazelbump plugin disable mycompany-rules

# Plugin info
bazelbump plugin info mycompany-rules
```

### Recipe System [🟡 MEDIUM PRIORITY]

Shareable, versioned migration recipes:

```yaml
# recipes/rules_go_0.46_to_0.50.yaml
apiVersion: bazelbump.dev/v1
kind: MigrationRecipe
metadata:
  name: rules_go_0.46_to_0.50
  version: 1.0.0
  author: community
  verified: true
  rating: 4.8
  downloads: 12500

spec:
  # When this recipe applies
  applicable:
    dependency: rules_go
    fromVersion: ">=0.46.0 <0.50.0"
    toVersion: ">=0.50.0"
    bazelVersion: ">=8.0.0"

  # Transformations to apply
  transforms:
    - name: Update MODULE.bazel
      type: module-dep-update
      moduleName: rules_go
      newVersion: "0.50.0"

    - name: Update load statements
      type: starlark-replace
      files: ["**/WORKSPACE", "**/WORKSPACE.bazel"]
      search: 'load("@io_bazel_rules_go//go:deps.bzl"'
      replace: 'load("@rules_go//go:deps.bzl"'

    - name: Update go_repository calls
      type: starlark-replace
      files: ["**/deps.bzl", "**/repositories.bzl"]
      search: "@io_bazel_rules_go//go:deps.bzl"
      replace: "@rules_go//go:deps.bzl"

  # Verification
  verify:
    - command: "bazel build //..."
      description: "Verify build succeeds"

  # Rollback hints
  rollback:
    instructions: |
      1. Revert MODULE.bazel changes
      2. Run 'bazel mod tidy'
      3. Verify build with original rules_go version

  # Known issues
  caveats:
    - "go_repository behavior changed - external deps may resolve differently"
    - "Run 'go mod tidy' after upgrade"
```

**Recipe CLI:**

```bash
# Search for recipes
bazelbump recipe search rules_go
bazelbump recipe search --bazel-version 9.0.0

# Show recipe details
bazelbump recipe show rules_go_0.46_to_0.50

# Apply a recipe
bazelbump recipe apply rules_go_0.46_to_0.50

# Preview recipe changes
bazelbump recipe apply rules_go_0.46_to_0.50 --dry-run

# Create a new recipe
bazelbump recipe init my-internal-rules

# Share a recipe (future: recipe registry)
bazelbump recipe publish my-recipe.yaml
```

### Standalone & Cherry-Pick Execution [🟠 HIGH PRIORITY]

Real-world upgrades are often incremental. Users need to prepare gradually:

```
┌─────────────────────────────────────────────────────────────────────────────┐
│  CHERRY-PICK PHILOSOPHY                                                      │
├─────────────────────────────────────────────────────────────────────────────┤
│                                                                              │
│  Full Upgrade:     analyze → plan → apply ALL changes                       │
│                                                                              │
│  Cherry-Pick:      Pick specific fixes, apply incrementally                 │
│                    ├── "Just add missing cc_library imports"                │
│                    ├── "Just flip --incompatible_foo flag"                  │
│                    ├── "Just update rules_go version"                       │
│                    └── "Prepare for 9.0, but stay on 8.x for now"          │
│                                                                              │
│  Why?                                                                        │
│  ├── Reduce risk: smaller changes = easier debugging                        │
│  ├── Team coordination: different teams own different packages             │
│  ├── Gradual migration: spread work over multiple sprints                  │
│  └── Testing: validate each change independently                           │
│                                                                              │
└─────────────────────────────────────────────────────────────────────────────┘
```

**Quick Fix Commands:**

```bash
# === Quick Fixes (single-purpose commands) ===

# Add missing load() statements for native rules
bazelbump fix imports
bazelbump fix imports --rule cc_library        # Only cc_library
bazelbump fix imports --package //src/lib/...  # Only in specific package

# Remove/migrate deprecated flags
bazelbump fix flags
bazelbump fix flags --flag experimental_enable_bzlmod  # Specific flag

# Update Starlark API usage
bazelbump fix starlark
bazelbump fix starlark --api native.existing_rule  # Specific API

# Update a single dependency
bazelbump fix dependency rules_go --version 0.50.0

# List available quick fixes
bazelbump fix --list
```

**Recipe Step Execution:**

```bash
# Show recipe steps
bazelbump recipe steps rules_go_0.46_to_0.50

# Output:
# RECIPE: rules_go_0.46_to_0.50
#
# Steps:
#   1. update-module-dep     Update MODULE.bazel dependency
#   2. update-load-paths     Update load() statement paths
#   3. update-go-repository  Update go_repository calls
#
# Run specific step: bazelbump recipe run rules_go_0.46_to_0.50 --step 1

# Run a specific step only
bazelbump recipe run rules_go_0.46_to_0.50 --step 1
bazelbump recipe run rules_go_0.46_to_0.50 --step update-module-dep  # By name

# Run steps 1 and 2 only
bazelbump recipe run rules_go_0.46_to_0.50 --steps 1,2

# Run steps up to (but not including) step 3
bazelbump recipe run rules_go_0.46_to_0.50 --until 3

# Run steps starting from step 2
bazelbump recipe run rules_go_0.46_to_0.50 --from 2

# Skip specific steps
bazelbump recipe run rules_go_0.46_to_0.50 --skip 2,3
```

**Prepare Mode (Incremental Readiness):**

```bash
# Prepare for upgrade without changing .bazelversion
bazelbump prepare --for 9.0.0

# This does everything EXCEPT:
# - Updating .bazelversion
# - Updating bazel_compatibility in MODULE.bazel
#
# Useful for: "I want to be ready for 9.0, but ship on 8.x for now"

# Prepare specific aspects only
bazelbump prepare --for 9.0.0 --only imports      # Just add load() statements
bazelbump prepare --for 9.0.0 --only flags        # Just migrate flags
bazelbump prepare --for 9.0.0 --only starlark     # Just fix Starlark APIs
bazelbump prepare --for 9.0.0 --only dependencies # Just update deps

# Prepare and test with target version (without committing)
bazelbump prepare --for 9.0.0 --test
# This:
# 1. Applies changes
# 2. Temporarily sets USE_BAZEL_VERSION=9.0.0
# 3. Runs bazel build //...
# 4. Reports results
# 5. Does NOT modify .bazelversion
```

**Implementation:**

```go
// pkg/libbazelbump/fix.go
package libbazelbump

// FixRequest specifies a targeted fix operation
type FixRequest struct {
    Type        FixType           // What to fix
    Filter      *FixFilter        // Narrow scope
    DryRun      bool
    ProjectPath string
}

type FixType int
const (
    FixTypeImports FixType = iota    // Add missing load() statements
    FixTypeFlags                      // Migrate deprecated flags
    FixTypeStarlark                   // Update Starlark APIs
    FixTypeDependency                 // Update a dependency
)

type FixFilter struct {
    Rules       []string          // Only these rules (e.g., "cc_library")
    Flags       []string          // Only these flags
    APIs        []string          // Only these APIs
    Packages    []string          // Only these packages
    Files       []string          // Only these files
}

// Fix applies a targeted fix
func (c *Client) Fix(ctx context.Context, req *FixRequest) (*FixResponse, error) {
    // 1. Analyze only the relevant subset
    // 2. Generate transforms for that subset
    // 3. Apply (or preview if dry-run)
}
```

```go
// pkg/libbazelbump/recipe_runner.go
package libbazelbump

// RecipeRunRequest allows fine-grained recipe execution
type RecipeRunRequest struct {
    RecipeName  string
    StepFilter  *StepFilter       // Which steps to run
    DryRun      bool
    ProjectPath string
}

type StepFilter struct {
    Steps       []int             // Run these step numbers
    StepNames   []string          // Run these step names
    From        int               // Start from step N
    Until       int               // Stop before step N
    Skip        []int             // Skip these steps
}

// RunRecipe executes a recipe with optional step filtering
func (c *Client) RunRecipe(ctx context.Context, req *RecipeRunRequest) (*RecipeRunResponse, error)
```

---

## CI/CD Integration [🟠 HIGH PRIORITY]

First-class support for continuous integration.

### Output Formats

```bash
# Standard formats for CI systems
bazelbump check --format sarif > results.sarif    # GitHub Security tab
bazelbump check --format junit > results.xml      # JUnit test reporters
bazelbump check --format json > results.json      # Machine readable
bazelbump check --format github-annotations       # GitHub Actions annotations
```

### Exit Codes

```go
// cmd/bazelbump/exitcodes.go
const (
    ExitSuccess           = 0   // No issues found
    ExitIssuesFound       = 1   // Issues found but not blockers
    ExitBlockersFound     = 2   // Blocking issues found
    ExitError             = 3   // Tool error (not project issues)
    ExitPreflightFailed   = 4   // Preflight checks failed
)
```

### GitHub Actions

```yaml
# .github/workflows/bazel-upgrade-check.yml
name: Bazel Upgrade Check

on:
  schedule:
    - cron: '0 0 * * 1'  # Weekly on Monday
  push:
    paths:
      - '.bazelversion'
      - 'MODULE.bazel'
      - '.bazelrc'

jobs:
  check-upgrade:
    runs-on: ubuntu-latest
    steps:
      - uses: actions/checkout@v4

      - name: Setup bazelbump
        uses: bazelbump/setup-action@v1

      - name: Check for latest Bazel
        id: check
        run: |
          bazelbump check --format json > check-results.json
          echo "score=$(jq .readinessScore check-results.json)" >> $GITHUB_OUTPUT

      - name: Post results
        if: always()
        uses: bazelbump/report-action@v1
        with:
          results-file: check-results.json
          comment-on-pr: true

      - name: Fail if blockers
        run: |
          if [ $(jq '.blockers | length' check-results.json) -gt 0 ]; then
            echo "::error::Blocking issues found for Bazel upgrade"
            exit 2
          fi
```

### Pre-commit Hook

```yaml
# .pre-commit-config.yaml
repos:
  - repo: https://github.com/albertocavalcante/bazelbump
    rev: v1.0.0
    hooks:
      - id: bazelbump-check
        name: Check Bazel compatibility
        entry: bazelbump check --ci
        language: golang
        files: '(\.bazelrc|MODULE\.bazel|\.bazelversion)$'
```

---

## Monorepo Support [🟠 HIGH PRIORITY]

Handle large monorepos efficiently, including the reality that not everything is always green.

### The Monorepo Reality

```
┌─────────────────────────────────────────────────────────────────────────────┐
│  MONOREPO REALITY CHECK                                                      │
├─────────────────────────────────────────────────────────────────────────────┤
│                                                                              │
│  Assumption: "All targets are green before upgrade"                         │
│  Reality:    "LOL no"                                                        │
│                                                                              │
│  In large monorepos:                                                         │
│  ├── Some targets are perpetually broken (abandoned, low priority)          │
│  ├── Some targets are owned by other teams (not your problem)              │
│  ├── Some targets are platform-specific (won't build on your machine)      │
│  ├── Some targets have flaky tests (don't block on them)                   │
│  ├── Some targets are in migration (intentionally broken)                  │
│  └── Some targets require special setup (credentials, hardware)            │
│                                                                              │
│  bazelbump must work WITH this reality, not against it.                     │
│                                                                              │
└─────────────────────────────────────────────────────────────────────────────┘
```

### Exclusion & Ignore System

**`.bazelbumpignore` File:**

```gitignore
# .bazelbumpignore
# Patterns follow .gitignore syntax

# Exclude entire directories
experimental/
legacy/
third_party/vendored/

# Exclude specific packages
//src/deprecated/...
//test/flaky/...

# Exclude by pattern
**/testdata/
**/*_benchmark.go

# Exclude specific targets (not just packages)
//src/service:broken_test
//src/lib:platform_specific

# Exclude with comments explaining why
//infra/k8s/...  # Owned by platform team, they'll handle separately
//mobile/ios/... # Requires macOS, CI runs on Linux
```

**CLI Exclusion Flags:**

```bash
# Exclude packages from analysis
bazelbump analyze --to 9.0.0 --exclude //experimental/...
bazelbump analyze --to 9.0.0 --exclude //legacy/...,//deprecated/...

# Exclude from apply
bazelbump apply --to 9.0.0 --exclude //third_party/...

# Include only specific packages (inverse)
bazelbump analyze --to 9.0.0 --only //src/core/...

# Use exclusion file
bazelbump analyze --to 9.0.0 --exclude-from .bazelbumpignore
bazelbump analyze --to 9.0.0 --exclude-from team-a-exclusions.txt

# Combine multiple exclusion sources
bazelbump analyze --to 9.0.0 \
  --exclude-from .bazelbumpignore \
  --exclude //temp/... \
  --exclude-targets //src:flaky_test
```

**Configuration-Based Exclusions:**

```yaml
# .bazelbump/config.yaml
exclusions:
  # Always exclude these (persistent)
  packages:
    - "//experimental/..."
    - "//legacy/..."
    - "//third_party/vendored/..."

  # Exclude specific targets
  targets:
    - "//src/service:broken_test"
    - "//src/lib:platform_specific"

  # Exclude by pattern
  patterns:
    - "**/testdata/**"
    - "**/*_benchmark*"

  # Exclude files from transformation (but still analyze)
  files:
    - "generated/**"
    - "*.gen.go"

  # Reasons (for documentation/auditing)
  reasons:
    "//experimental/...": "Experimental code, not production"
    "//mobile/ios/...": "Requires macOS build environment"
```

### Known Issues & Acceptable Failures

For preflight checks and verification, some failures are expected:

```yaml
# .bazelbump/known-issues.yaml
known_issues:
  # Targets that are known to be broken (don't fail preflight)
  - target: "//src/legacy:old_service"
    issue: "Deprecated, scheduled for removal in Q2"
    jira: "PLAT-1234"
    expires: "2024-06-01"  # Re-evaluate after this date

  - target: "//test/flaky:integration_test"
    issue: "Flaky test, tracked separately"
    jira: "TEST-567"
    severity: "warning"  # Report but don't block

  # Packages with known compatibility issues
  - package: "//third_party/old_lib/..."
    issue: "Vendored library, manual upgrade required"
    owner: "platform-team"
    action: "manual"  # Skip automated transforms

  # Patterns
  - pattern: "//experimental/**"
    issue: "Experimental code, best-effort only"
    severity: "info"
```

**CLI Integration:**

```bash
# Preflight respects known issues
bazelbump preflight
# Output:
# [✓] git-clean
# [✓] current-build (3 known failures ignored)
#     └── Ignored: //src/legacy:old_service (PLAT-1234)
#     └── Ignored: //test/flaky:integration_test (TEST-567)
#     └── Ignored: //experimental/foo:bar (experimental)
# [✓] Ready to proceed

# Show what's being ignored
bazelbump preflight --show-ignored

# Fail on known issues anyway (strict mode)
bazelbump preflight --strict

# Add a new known issue interactively
bazelbump known-issues add //src:broken_target --reason "WIP migration" --jira PLAT-999

# List known issues
bazelbump known-issues list

# Clean up expired known issues
bazelbump known-issues prune
```

**Verification with Exclusions:**

```bash
# After apply, verify build (respecting exclusions)
bazelbump verify
# Runs: bazel build //... --deleted_packages=<excluded>

# Verify specific packages only
bazelbump verify --packages //src/core/...

# Verify with custom target pattern
bazelbump verify --targets "//... except //experimental/..."

# Continue despite failures (report but don't fail)
bazelbump verify --allow-failures

# Fail only if NEW failures appear (not pre-existing)
bazelbump verify --only-new-failures
```

### Parallel Analysis

```bash
# Analyze specific packages only
bazelbump analyze --to 9.0.0 --packages //src/backend/...

# Parallel file scanning
bazelbump analyze --to 9.0.0 --parallel 8

# Memory-efficient streaming for huge repos
bazelbump analyze --to 9.0.0 --stream
```

### Incremental Migration

For massive codebases, migrate incrementally:

```bash
# Phase 1: Migrate one subtree
bazelbump apply --to 9.0.0 --package //src/lib --incremental

# Phase 2: Verify it works
bazel test //src/lib/...

# Phase 3: Continue with next subtree
bazelbump apply --to 9.0.0 --package //src/app --incremental

# Or let bazelbump determine order
bazelbump apply --to 9.0.0 --incremental --batch-size 100
```

### Package Dependency Awareness

```go
// internal/monorepo/ordering.go
package monorepo

// DetermineUpgradeOrder returns packages in dependency order
// (dependencies first, then dependents)
func DetermineUpgradeOrder(project *Project, packages []string) ([][]string, error) {
    // Use bazel query to build dependency graph
    // bazel query 'deps(//src/...)' --output=graph

    // Return batches that can be upgraded in parallel
    // e.g., [[//lib/a, //lib/b], [//app/x depends on lib/a]]
}
```

### Team-Based Migration

For organizations where different teams own different packages:

```yaml
# .bazelbump/teams.yaml
teams:
  platform:
    packages:
      - "//infra/..."
      - "//lib/core/..."
    owners: ["platform-team@company.com"]
    slack: "#platform-eng"

  backend:
    packages:
      - "//services/..."
      - "//api/..."
    owners: ["backend-team@company.com"]
    slack: "#backend-eng"

  mobile:
    packages:
      - "//mobile/..."
    owners: ["mobile-team@company.com"]
    slack: "#mobile-eng"
    # Mobile team will handle their own upgrade
    exclude_from_global: true
```

```bash
# Analyze/apply for your team only
bazelbump analyze --to 9.0.0 --team platform
bazelbump apply --to 9.0.0 --team backend

# Generate per-team reports
bazelbump analyze --to 9.0.0 --by-team --output reports/

# Notify teams of their required actions
bazelbump notify --teams --message "Bazel 9.0 upgrade starting next sprint"
```

### Progress Tracking for Large Migrations

```bash
# Track migration progress across the monorepo
bazelbump status

# Output:
# MIGRATION STATUS: Bazel 8.0.0 → 9.0.0
# =====================================
#
# Overall Progress: 67% (2,847 / 4,250 packages)
#
# By Team:
#   platform    ████████████████████ 100%  (450/450)
#   backend     ████████████████░░░░  80%  (1,200/1,500)
#   frontend    ████████████░░░░░░░░  60%  (900/1,500)
#   mobile      ░░░░░░░░░░░░░░░░░░░░   0%  (0/800) [excluded]
#
# Blockers:
#   3 packages blocked on rules_go update
#   12 packages have manual intervention required
#
# Next: bazelbump apply --to 9.0.0 --team frontend --continue

# Resume from where you left off
bazelbump apply --to 9.0.0 --continue

# Mark package as manually completed
bazelbump status mark-done //custom/package/...
```

**Implementation:**

```go
// internal/monorepo/progress.go
package monorepo

type MigrationProgress struct {
    TargetVersion   string
    StartedAt       time.Time
    Packages        map[string]PackageStatus  // package -> status
    Teams           map[string]TeamProgress
}

type PackageStatus int
const (
    StatusPending PackageStatus = iota
    StatusInProgress
    StatusCompleted
    StatusFailed
    StatusExcluded
    StatusManual  // Requires manual intervention
)

// Track progress persistently
func (m *MigrationProgress) Save(path string) error
func LoadProgress(path string) (*MigrationProgress, error)
```

---

## Nice to Have Features [🟢]

These features add polish but aren't essential for v1.0.

### Interactive TUI [🟢 NICE TO HAVE]

A terminal user interface for guided upgrades:

```
┌─ BAZELBUMP INTERACTIVE UPGRADE ──────────────────────────────────────────┐
│                                                                           │
│  Upgrading: my-project                                                   │
│  From: Bazel 8.0.0 → To: Bazel 9.0.0                                    │
│                                                                           │
│  ┌─ Progress ──────────────────────────────────────────────────────────┐ │
│  │ [✓] Pre-flight checks                                    [2/2]      │ │
│  │ [✓] Flag migrations                                      [5/5]      │ │
│  │ [●] Starlark API updates                                 [2/7]      │ │
│  │ [ ] Dependency updates                                   [0/3]      │ │
│  │ [ ] Verification                                         [0/1]      │ │
│  └─────────────────────────────────────────────────────────────────────┘ │
│                                                                           │
│  ┌─ Current: Starlark API Updates ─────────────────────────────────────┐ │
│  │                                                                      │ │
│  │  File: tools/macros.bzl:45                                          │ │
│  │                                                                      │ │
│  │  Issue: native.existing_rule() is deprecated                        │ │
│  │                                                                      │ │
│  │  44│ def check_target(name):                                        │ │
│  │  45│ -   rule = native.existing_rule(name)                          │ │
│  │  45│ +   rule = native.existing_rules().get(name)                   │ │
│  │  46│     if rule:                                                   │ │
│  │                                                                      │ │
│  │  [A]pply  [S]kip  [V]iew context  [?] Help                          │ │
│  └──────────────────────────────────────────────────────────────────────┘ │
│                                                                           │
│  [←Prev] [Next→] [Apply All] [Export Report] [Quit]                      │
└───────────────────────────────────────────────────────────────────────────┘
```

### Notifications [🟢 NICE TO HAVE]

```yaml
# .bazelbump/config.yaml
notifications:
  # Slack notifications
  slack:
    webhook_url: ${SLACK_WEBHOOK}
    channel: "#bazel-upgrades"
    events:
      - new_bazel_release
      - dependency_update_available
      - upgrade_completed

  # Email notifications
  email:
    smtp_server: smtp.company.com
    recipients:
      - platform-team@company.com
    events:
      - blocking_issue_detected
```

### Multi-Repo Orchestration [🟢 NICE TO HAVE]

For organizations with multiple Bazel repositories:

```yaml
# bazelbump-org.yaml
organization:
  name: mycompany
  repos:
    - path: ~/repos/common-libs
      priority: 1              # Upgrade first (other repos depend on it)

    - path: ~/repos/backend
      priority: 2
      depends_on: [common-libs]

    - path: ~/repos/frontend
      priority: 2
      depends_on: [common-libs]

    - path: ~/repos/mobile
      priority: 3
      depends_on: [backend]

strategy: dependency-order      # or "parallel", "manual"

notifications:
  on_complete: slack://#platform
```

```bash
bazelbump org upgrade --config bazelbump-org.yaml --to 9.0.0
```

### VS Code Extension [🟢 NICE TO HAVE]

Inline upgrade hints in the editor:

- Highlight deprecated flags in `.bazelrc`
- Show API deprecation warnings in `.bzl` files
- Quick-fix actions for common migrations
- Upgrade readiness indicator in status bar

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

Following the proto-first, lib-first, vendor-first architecture:

```
bazelbump/
├── proto/                           # PROTO-FIRST: All types defined here
│   ├── bazelbump/v1/                # Our custom protos
│   │   ├── flag.proto               # Flag deprecation/migration types
│   │   ├── version.proto            # Version info, constraints
│   │   ├── analysis.proto           # Upgrade analysis request/response
│   │   ├── bazelrc.proto            # Parsed .bazelrc representation
│   │   ├── module.proto             # MODULE.bazel types
│   │   ├── migration.proto          # Migration rules and hints
│   │   ├── snapshot.proto           # 🆕 Snapshot/rollback types
│   │   ├── preflight.proto          # 🆕 Pre-flight check types
│   │   ├── score.proto              # 🆕 Readiness score types
│   │   ├── path.proto               # 🆕 Upgrade path types
│   │   ├── starlark.proto           # 🆕 Starlark API deprecation types
│   │   ├── cache_impact.proto       # 🆕 Cache impact analysis types
│   │   ├── plugin.proto             # 🆕 Plugin manifest types
│   │   └── recipe.proto             # 🆕 Migration recipe types
│   ├── vendor/                      # Vendored canonical protos
│   │   └── bazel/
│   │       └── src/main/protobuf/
│   └── buf.yaml                     # Buf configuration
│
├── pkg/                             # LIB-FIRST: Public library API
│   ├── libbazelbump/
│   │   ├── client.go                # Main Client type
│   │   ├── analyze.go               # Analyze() implementation
│   │   ├── check.go                 # Check() implementation
│   │   ├── apply.go                 # Apply() implementation
│   │   ├── simulate.go              # 🆕 Simulate() implementation
│   │   ├── preflight.go             # 🆕 Preflight() implementation
│   │   ├── score.go                 # 🆕 Score() implementation
│   │   ├── path.go                  # 🆕 Path() implementation
│   │   ├── fix.go                   # 🆕 Fix() implementation (quick fixes)
│   │   ├── prepare.go               # 🆕 Prepare() implementation
│   │   ├── verify.go                # 🆕 Verify() implementation
│   │   ├── recipe_runner.go         # 🆕 RunRecipe() with step filtering
│   │   ├── flags.go                 # Flag operations
│   │   ├── options.go               # Functional options
│   │   └── libbazelbump_test.go
│   │
│   └── plugin/                      # 🆕 Public plugin API
│       ├── plugin.go                # Plugin interface definitions
│       ├── loader.go                # Plugin loading/discovery
│       ├── project.go               # Project abstraction for plugins
│       └── types.go                 # Types available to plugins
│
├── cmd/                             # CLI: Thin wrapper over library
│   └── bazelbump/
│       ├── main.go                  # Entry point
│       ├── analyze.go               # analyze command
│       ├── check.go                 # check command
│       ├── apply.go                 # apply command
│       ├── simulate.go              # 🆕 simulate command
│       ├── preflight.go             # 🆕 preflight command
│       ├── rollback.go              # 🆕 rollback command
│       ├── score.go                 # 🆕 score command
│       ├── path.go                  # 🆕 path command
│       ├── flags.go                 # flags command
│       ├── fix.go                   # 🆕 fix subcommands (quick fixes)
│       ├── prepare.go               # 🆕 prepare command
│       ├── verify.go                # 🆕 verify command
│       ├── status.go                # 🆕 status command (migration progress)
│       ├── snapshot.go              # 🆕 snapshot subcommands
│       ├── plugin.go                # 🆕 plugin subcommands
│       ├── recipe.go                # 🆕 recipe subcommands
│       ├── known_issues.go          # 🆕 known-issues subcommands
│       ├── cache.go                 # 🆕 cache subcommands
│       ├── lockfile.go              # 🆕 lockfile subcommands
│       └── format.go                # Output formatting (text, json, sarif, etc.)
│
├── internal/                        # Private implementation details
│   ├── bazelrc/
│   │   ├── parser.go                # .bazelrc parser
│   │   └── parser_test.go
│   │
│   ├── starlark/
│   │   ├── module.go                # MODULE.bazel parsing (uses buildtools)
│   │   ├── workspace.go             # WORKSPACE parsing
│   │   └── deprecations.go          # 🆕 Starlark API deprecation scanner
│   │
│   ├── version/
│   │   ├── detect.go                # Version detection
│   │   ├── compare.go               # Version comparison
│   │   └── lts.go                   # LTS version tracking
│   │
│   ├── flagdb/
│   │   ├── database.go              # Flag database operations
│   │   ├── generate.go              # Database generation from Bazel
│   │   └── embed.go                 # Embedded database
│   │
│   ├── registry/
│   │   ├── bcr.go                   # BCR client
│   │   └── github.go                # GitHub API client
│   │
│   ├── cache/
│   │   └── cache.go                 # Caching layer
│   │
│   ├── snapshot/                    # 🆕 Snapshot management
│   │   ├── manager.go               # Snapshot CRUD operations
│   │   ├── storage.go               # Snapshot storage backend
│   │   └── restore.go               # Restore logic
│   │
│   ├── preflight/                   # 🆕 Pre-flight checks
│   │   ├── runner.go                # Check execution
│   │   ├── checks.go                # Built-in check implementations
│   │   └── git.go                   # Git-related checks
│   │
│   ├── pathfinder/                  # 🆕 Upgrade path optimization
│   │   ├── pathfinder.go            # Path discovery
│   │   └── risk.go                  # Risk assessment
│   │
│   ├── scorer/                      # 🆕 Readiness scoring
│   │   ├── scorer.go                # Score calculation
│   │   └── categories.go            # Score categories
│   │
│   ├── buildlog/                    # 🆕 Build log analysis
│   │   ├── analyzer.go              # Warning extraction
│   │   └── bep.go                   # Build Event Protocol parsing
│   │
│   ├── cacheimpact/                 # 🆕 Cache impact analysis
│   │   ├── analyzer.go              # Impact detection
│   │   └── estimator.go             # Rebuild time estimation
│   │
│   ├── lockfile/                    # 🆕 Lockfile handling
│   │   ├── parser.go                # Lockfile parsing
│   │   └── migrate.go               # Version migration
│   │
│   ├── plugin/                      # 🆕 Plugin runtime
│   │   ├── loader.go                # Dynamic loading
│   │   ├── registry.go              # Plugin registry
│   │   └── sandbox.go               # Plugin isolation
│   │
│   ├── recipe/                      # 🆕 Recipe system
│   │   ├── parser.go                # Recipe YAML parsing
│   │   ├── executor.go              # Recipe execution
│   │   └── registry.go              # Recipe discovery
│   │
│   ├── transform/                   # File transformations
│   │   ├── engine.go                # Transform orchestration
│   │   ├── diff.go                  # Diff generation
│   │   └── autoload.go              # Native rule autoload transform
│   │
│   ├── monorepo/                    # 🆕 Monorepo support
│   │   ├── detector.go              # Monorepo detection
│   │   ├── ordering.go              # Package upgrade ordering
│   │   ├── batch.go                 # Batch processing
│   │   ├── progress.go              # 🆕 Migration progress tracking
│   │   └── teams.go                 # 🆕 Team-based ownership
│   │
│   ├── exclusion/                   # 🆕 Exclusion system
│   │   ├── matcher.go               # Pattern matching for exclusions
│   │   ├── ignore.go                # .bazelbumpignore parser
│   │   └── filter.go                # Apply exclusions to file/target lists
│   │
│   ├── knownissues/                 # 🆕 Known issues tracking
│   │   ├── store.go                 # Known issues storage
│   │   ├── matcher.go               # Match failures to known issues
│   │   └── expiry.go                # Handle expired issues
│   │
│   ├── quickfix/                    # 🆕 Quick fix implementations
│   │   ├── imports.go               # Add missing load() statements
│   │   ├── flags.go                 # Migrate specific flags
│   │   ├── starlark.go              # Fix specific Starlark APIs
│   │   └── dependency.go            # Update specific dependency
│   │
│   ├── prepare/                     # 🆕 Prepare mode
│   │   ├── prepare.go               # Prepare without version switch
│   │   └── test.go                  # Test with target version
│   │
│   └── output/                      # Output formatting
│       ├── text.go                  # Human-readable output
│       ├── json.go                  # JSON output
│       ├── sarif.go                 # 🆕 SARIF format (GitHub Security)
│       ├── junit.go                 # 🆕 JUnit XML format
│       └── github.go                # 🆕 GitHub annotations
│
├── third_party/                     # VENDOR-FIRST: Vendored dependencies
│   ├── buildtools/                  # Starlark parser
│   │   ├── build/
│   │   ├── tables/
│   │   └── README.md                # Version, sync info
│   └── sync.sh                      # Vendor sync script
│
├── tools/                           # Development/sync tooling
│   ├── sync-buildtools.sh           # Sync buildtools from upstream
│   ├── sync-protos.sh               # Sync Bazel protos
│   └── generate-flags.sh            # Generate flag database
│
├── gen/                             # Generated code (from protos)
│   └── go/
│       └── bazelbump/v1/
│           ├── flag.pb.go
│           ├── version.pb.go
│           ├── snapshot.pb.go       # 🆕
│           ├── preflight.pb.go      # 🆕
│           ├── plugin.pb.go         # 🆕
│           └── ...
│
├── data/                            # Embedded data files
│   ├── flags.pb                     # Flag database (protobuf binary)
│   └── starlark_deprecations.pb     # 🆕 Starlark API deprecations
│
├── recipes/                         # 🆕 Built-in migration recipes
│   ├── rules_go_0.46_to_0.50.yaml
│   ├── rules_python_0.31_to_0.35.yaml
│   └── workspace_to_bzlmod.yaml
│
├── .bazelbump/                      # 🆕 Default config location
│   └── config.example.yaml
│
├── buf.gen.yaml                     # Buf code generation config
├── buf.work.yaml                    # Buf workspace config
├── Makefile                         # Build, generate, sync targets
├── go.mod
├── go.sum
└── SPEC.md
```

### Build Targets

```makefile
# Makefile
.PHONY: all generate build test sync

all: generate build

# Generate Go code from protos
generate:
	buf generate

# Build CLI
build: generate
	go build -o bin/bazelbump ./cmd/bazelbump

# Run tests
test: generate
	go test ./...

# Sync all vendored dependencies
sync: sync-buildtools sync-protos

sync-buildtools:
	./tools/sync-buildtools.sh v0.1.0

sync-protos:
	./tools/sync-protos.sh 7.4.0

# Generate flag database from Bazel releases
generate-flagdb:
	./tools/generate-flags.sh > data/flags.pb
```

### Dependencies

```go
// go.mod
module github.com/albertocavalcante/bazelbump

go 1.22

require (
    // === Foundational Libraries (Our Ecosystem) ===

    // MODULE.bazel parsing, version comparison, compatibility checking
    github.com/albertocavalcante/go-bzlmod v0.x.x

    // .bazelrc parsing (CREATE THIS FIRST)
    github.com/bazel-contrib/go-bazelrc v0.x.x

    // Bazel Central Registry client (CREATE THIS)
    github.com/bazel-contrib/go-bcr v0.x.x

    // Bazel flag database (CREATE THIS)
    github.com/bazel-contrib/go-bazelflags v0.x.x

    // Build Event Protocol parser (CREATE THIS, lower priority)
    github.com/bazel-contrib/go-bep v0.x.x


    // === Core Infrastructure ===

    // Proto runtime
    google.golang.org/protobuf v1.32.0

    // CLI framework
    github.com/spf13/cobra v1.8.0
    github.com/spf13/viper v1.18.0

    // Output formatting
    github.com/fatih/color v1.16.0
    github.com/olekukonko/tablewriter v0.0.5

    // Content-addressable storage
    github.com/oklog/ulid/v2 v2.1.0         // Unique IDs

    // Config/Data formats
    gopkg.in/yaml.v3 v3.0.1


    // === Utilities ===

    // Diff generation
    github.com/sergi/go-diff v1.3.1

    // Glob patterns
    github.com/bmatcuk/doublestar/v4 v4.6.1


    // === Optional (Build Tags) ===

    // Interactive TUI
    // +build tui
    // github.com/charmbracelet/bubbletea v0.25.0
    // github.com/charmbracelet/lipgloss v0.9.1
)

// Vendored (third_party/):
// - buildtools/build (Starlark parser) - same approach as go-bzlmod
```

**Dependency Philosophy:**

```
┌─────────────────────────────────────────────────────────────────────────────┐
│                      DEPENDENCY PHILOSOPHY                                   │
├─────────────────────────────────────────────────────────────────────────────┤
│                                                                              │
│  RULE 1: Build Foundational Libraries First                                 │
│  ├── If functionality is reusable → make it a separate library             │
│  ├── If library doesn't exist → create it before bazelbump                 │
│  └── If library exists → use it as dependency (not vendor)                 │
│                                                                              │
│  RULE 2: Vendor Sparingly                                                    │
│  ├── Only vendor when: no Go module, needs patches, tightly coupled        │
│  ├── buildtools → vendor (follows go-bzlmod pattern)                       │
│  └── Everything else → proper Go dependency                                │
│                                                                              │
│  RULE 3: Minimize External Dependencies                                      │
│  ├── Prefer stdlib when reasonable                                          │
│  ├── Each dependency is a maintenance burden                                │
│  └── Our foundational libs have minimal deps themselves                    │
│                                                                              │
│  RULE 4: Optional Features Behind Build Tags                                │
│  ├── TUI → +build tui                                                       │
│  ├── Remote cache → +build remote                                           │
│  └── Core functionality works without optional deps                        │
│                                                                              │
└─────────────────────────────────────────────────────────────────────────────┘
```

| Category | Policy |
|----------|--------|
| Our ecosystem libs | Always use as dependency, contribute upstream |
| Stdlib | Prefer when reasonable |
| Well-known deps | Accept (cobra, viper, protobuf) |
| Optional features | Build tags |
| Bazel internals | Vendor (buildtools) |

```yaml
# buf.gen.yaml
version: v1
plugins:
  - plugin: buf.build/protocolbuffers/go
    out: gen/go
    opt: paths=source_relative
```

### Development Phases

Development follows the **"Build Foundations First"** philosophy. Foundational libraries are created as independent, reusable projects before bazelbump itself.

```
┌─────────────────────────────────────────────────────────────────────────────┐
│                      DEVELOPMENT STRATEGY                                    │
├─────────────────────────────────────────────────────────────────────────────┤
│                                                                              │
│  PHASE 0: FOUNDATIONAL LIBRARIES                                            │
│  ──────────────────────────────────                                         │
│  Build reusable libraries FIRST. These benefit the entire Bazel ecosystem. │
│  Each library is its own repo, its own release cycle, its own community.   │
│                                                                              │
│  PHASE 1+: BAZELBUMP                                                         │
│  ──────────────────────                                                      │
│  bazelbump consumes the foundational libraries as dependencies.             │
│  Focuses on upgrade logic, not reinventing parsing/querying.                │
│                                                                              │
└─────────────────────────────────────────────────────────────────────────────┘
```

---

#### PHASE 0: Foundational Libraries (Weeks 1-8)

**These are separate repositories, separate projects.**

##### 0.1: go-bazelrc (Weeks 1-2)

**Repository:** `github.com/bazel-contrib/go-bazelrc`

```
go-bazelrc/
├── parser.go           # Parse .bazelrc files
├── types.go            # Line, Flag, Import, Command types
├── resolver.go         # Resolve imports, %workspace%, etc.
├── writer.go           # Modify and write .bazelrc
├── parser_test.go
└── testdata/
    ├── simple.bazelrc
    ├── with_imports.bazelrc
    └── complex.bazelrc
```

- [ ] Core parsing (commands, flags, configs)
- [ ] Import resolution (%workspace%, relative paths)
- [ ] Flag negation (--nofoo)
- [ ] Writer (modify existing files preserving structure)
- [ ] Comprehensive tests
- [ ] Documentation and examples
- [ ] Release v0.1.0

##### 0.2: go-bcr (Weeks 3-4)

**Repository:** `github.com/bazel-contrib/go-bcr`

```
go-bcr/
├── client.go           # HTTP client for BCR
├── types.go            # Module, Version, Metadata
├── cache.go            # Response caching (ETags, TTL)
├── registry.go         # Registry interface (for custom registries)
├── client_test.go
└── testdata/
    └── fixtures/
```

- [ ] List modules, get module info
- [ ] Get MODULE.bazel for specific version
- [ ] Search functionality
- [ ] Caching layer (ETags, conditional requests)
- [ ] Custom registry support (for enterprise)
- [ ] Tests with mocked responses
- [ ] Release v0.1.0

##### 0.3: go-bazelflags (Weeks 5-6)

**Repository:** `github.com/bazel-contrib/go-bazelflags`

```
go-bazelflags/
├── database.go         # Flag database operations
├── types.go            # Flag, Deprecation types
├── embed.go            # Embedded flag data
├── query.go            # Query flags by criteria
├── generate/           # Generation from Bazel source
│   ├── parser.go       # Parse @Option annotations from Java
│   └── cmd/            # Generator CLI
├── data/               # Generated per-version data
│   ├── flags_7.0.0.json
│   ├── flags_8.0.0.json
│   └── flags_9.0.0.json
└── database_test.go
```

- [ ] Database schema design
- [ ] Generator from Bazel source code
- [ ] Query API (by version, by deprecation status, etc.)
- [ ] Embedded data for common versions
- [ ] Fetch/update mechanism for new versions
- [ ] Release v0.1.0

##### 0.4: go-bep (Weeks 7-8, parallel with Phase 1)

**Repository:** `github.com/bazel-contrib/go-bep`

```
go-bep/
├── parser.go           # Parse BEP JSON/proto
├── types.go            # Event types
├── stream.go           # Streaming parser
├── summary.go          # Build summary extraction
├── warnings.go         # Warning extraction
└── proto/              # Vendored BEP protos
```

- [ ] Parse BEP JSON format
- [ ] Streaming parser for large files
- [ ] Extract build summary
- [ ] Extract warnings and errors
- [ ] Release v0.1.0

---

#### PHASE 1: bazelbump Core (Weeks 9-14) [🔴 PARAMOUNT]

**Now we build bazelbump, consuming the foundational libraries.**

##### 1.1: Project Setup & Integration (Week 9)

- [ ] Project setup with Go modules
- [ ] Integrate go-bzlmod as dependency
- [ ] Integrate go-bazelrc as dependency
- [ ] Integrate go-bcr as dependency
- [ ] Integrate go-bazelflags as dependency
- [ ] Vendor buildtools (Starlark parser)
- [ ] Proto definitions for bazelbump-specific types
- [ ] Basic CLI structure (cobra)

##### 1.2: Core Safety System (Week 10) [🔴 PARAMOUNT]

- [ ] **Snapshot system** — Content-addressed storage
- [ ] **Pre-flight checks** — Extensible check framework
- [ ] Snapshot CLI (create, restore, list, gc)
- [ ] Integration with apply command

##### 1.3: Analysis Engine (Weeks 11-12) [🔴 PARAMOUNT]

- [ ] **Phased execution** — Loading, Analysis, Planning, Execution
- [ ] **Caching layer** — Content-addressed, multi-layer
- [ ] Flag analysis (using go-bazelflags)
- [ ] Starlark API deprecation scanner
- [ ] Dependency compatibility checking (using go-bcr)
- [ ] Native rule usage detection
- [ ] Issue aggregation and reporting

##### 1.4: Transform Engine (Weeks 13-14) [🔴 PARAMOUNT]

- [ ] Transform primitives (replace, insert, delete)
- [ ] Deterministic transforms
- [ ] Diff generation
- [ ] Buildifier integration
- [ ] Apply workflow with verification

---

#### PHASE 2: Intelligence & UX (Weeks 15-18) [🟠 HIGH PRIORITY]

##### 2.1: Upgrade Intelligence (Weeks 15-16)

- [ ] **Upgrade path optimizer** — Find best route
- [ ] **Readiness score** — Quantify upgrade readiness
- [ ] **What-if simulation** — Preview without changes
- [ ] **Cache impact analysis** — Warn about rebuilds
- [ ] **Query system** — Filter and explore issues

##### 2.2: Cherry-Pick & Monorepo (Weeks 17-18)

- [ ] **Quick fix commands** — Single-purpose fixes
- [ ] **Prepare mode** — Ready without switching version
- [ ] **Exclusion system** — .bazelbumpignore, known issues
- [ ] **Monorepo support** — Packages, teams, incremental
- [ ] **Progress tracking** — Long-running migration status

---

#### PHASE 3: Extensibility & Integration (Weeks 19-22) [🟠 HIGH PRIORITY]

##### 3.1: Plugin System (Weeks 19-20)

- [ ] Plugin interface (Analyzer, Transformer, Checker)
- [ ] Plugin loading and discovery
- [ ] Plugin isolation
- [ ] Built-in plugins for common cases

##### 3.2: Recipe System (Week 21)

- [ ] Recipe YAML schema
- [ ] Recipe parser and validator
- [ ] Step-by-step execution
- [ ] Built-in recipes for major migrations

##### 3.3: CI/CD Integration (Week 22)

- [ ] Output formats (SARIF, JUnit, GitHub annotations)
- [ ] Exit codes
- [ ] GitHub Actions example
- [ ] Pre-commit hooks

---

#### PHASE 4: Polish & v1.0 (Weeks 23-24)

- [ ] Comprehensive test suite
- [ ] Edge case handling
- [ ] Documentation
- [ ] Performance optimization
- [ ] Error messages
- [ ] Release v1.0.0

---

#### FUTURE: Post v1.0 [🟡 MEDIUM / 🟢 NICE TO HAVE]

- [ ] Interactive TUI mode
- [ ] Multi-repo orchestration
- [ ] Notifications (Slack, etc.)
- [ ] VS Code extension
- [ ] Recipe marketplace
- [ ] Web dashboard
- [ ] Remote cache support

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

## Configuration

bazelbump is configured via `.bazelbump/config.yaml` in the project root:

```yaml
# .bazelbump/config.yaml

# Project settings
project:
  name: "my-project"                    # Optional, defaults to directory name
  root: "."                             # Project root (default: config file location)

# Safety settings [🔴 PARAMOUNT]
safety:
  snapshots:
    enabled: true                       # Create snapshots before apply (default: true)
    max_count: 10                       # Keep last N snapshots
    retention_days: 30                  # Auto-expire after N days
    location: "~/.local/share/bazelbump/snapshots"  # Snapshot storage

  preflight:
    enabled: true                       # Run preflight checks (default: true)
    fail_on_warnings: false             # Treat warnings as blockers
    skip_checks:                        # Checks to skip
      - "tests-pass"                    # Skip test verification
    custom_checks:                      # Add project-specific checks
      - name: "custom-lint"
        command: "make lint"
        severity: "warning"

# Analysis settings
analysis:
  include_starlark: true                # Scan for Starlark API deprecations
  include_cache_impact: true            # Analyze cache invalidation
  parallel: 4                           # Parallel file processing

# Transform settings
transforms:
  dry_run_by_default: false             # Require explicit --apply
  format_after_edit: true               # Run buildifier after edits
  preserve_comments: true               # Don't remove comments

# Plugin settings [🟠 HIGH PRIORITY]
plugins:
  # Local plugin
  - path: ./tools/bazelbump-mycompany
    enabled: true
    config:
      custom_key: "value"

  # Git plugin
  - git: https://github.com/myorg/bazelbump-plugin
    version: v1.0.0
    enabled: true

# Output settings
output:
  format: "text"                        # Default: text, json, yaml
  color: "auto"                         # auto, always, never
  verbose: false

# CI/CD settings [🟠 HIGH PRIORITY]
ci:
  exit_code_on_issues: true             # Exit non-zero if issues found
  annotations: true                     # Emit GitHub annotations
  sarif_output: "./results.sarif"       # Auto-generate SARIF

# Exclusions [🟠 HIGH PRIORITY]
exclusions:
  # Packages to always exclude
  packages:
    - "//experimental/..."
    - "//legacy/..."
    - "//third_party/vendored/..."

  # Specific targets to exclude
  targets:
    - "//src/service:broken_test"
    - "//test/flaky:integration_test"

  # File patterns to exclude from transformation
  patterns:
    - "**/testdata/**"
    - "**/*.gen.go"
    - "generated/**"

  # Load exclusions from file (like .gitignore)
  from_file: ".bazelbumpignore"

  # Documented reasons (for auditing)
  reasons:
    "//experimental/...": "Not production code"
    "//mobile/ios/...": "Requires macOS build environment"

# Known issues (acceptable failures) [🟠 HIGH PRIORITY]
known_issues:
  # File containing known issues
  file: ".bazelbump/known-issues.yaml"

  # Or inline
  inline:
    - target: "//legacy:old_service"
      reason: "Deprecated, removal scheduled"
      ticket: "PLAT-1234"
      expires: "2024-06-01"

    - target: "//test/flaky:integration_test"
      reason: "Flaky test, tracked separately"
      ticket: "TEST-567"
      severity: "warning"  # warning = report but don't block

# Monorepo settings [🟠 HIGH PRIORITY]
monorepo:
  enabled: false                        # Auto-detected if not set
  packages:                             # Package groups for staged upgrade
    - name: "core-libs"
      patterns: ["//lib/...", "//common/..."]
      priority: 1
    - name: "services"
      patterns: ["//services/..."]
      priority: 2
      depends_on: ["core-libs"]
  batch_size: 50                        # Files per batch for incremental

  # Team ownership (for team-based migrations)
  teams:
    - name: "platform"
      packages: ["//infra/...", "//lib/core/..."]
      owners: ["platform@company.com"]
    - name: "backend"
      packages: ["//services/..."]
      owners: ["backend@company.com"]
      slack: "#backend-eng"

# Notifications [🟢 NICE TO HAVE]
notifications:
  slack:
    webhook_url: "${SLACK_WEBHOOK}"
    channel: "#bazel-upgrades"
    events: ["upgrade_completed", "blocking_issue"]

# Cache settings
cache:
  enabled: true
  location: "~/.cache/bazelbump"
  ttl:
    releases: "24h"
    bcr: "1h"
    github: "1h"
```

**Environment Variable Substitution:**

Config values can reference environment variables:

```yaml
plugins:
  - git: https://${GITHUB_TOKEN}@github.com/myorg/private-plugin
```

**Config Precedence:**

1. Command-line flags (highest)
2. Environment variables (`BAZELBUMP_*`)
3. `.bazelbump/config.yaml` in project
4. `~/.config/bazelbump/config.yaml` (user defaults)
5. Built-in defaults (lowest)

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

### Snapshot Edge Cases [🔴 PARAMOUNT]
- Files created during upgrade (not in snapshot) — track as "new files"
- Files deleted during upgrade — restore them on rollback
- Concurrent modifications — detect and warn
- Large files — consider excluding binaries, use gitignore patterns
- Symlinks — preserve symlink vs target content

### Plugin Edge Cases [🟠 HIGH PRIORITY]
- Plugin version incompatibility — clear error message
- Plugin crashes — isolate, don't crash bazelbump
- Plugin conflicts — deterministic ordering, allow disabling
- Slow plugins — timeout with warning

### Monorepo Edge Cases [🟠 HIGH PRIORITY]
- Circular dependencies between packages — detect and error
- Partial upgrade state — track progress, allow resume
- Different packages need different Bazel versions — not supported, warn
- Exclusion patterns overlap — more specific pattern wins
- Team ownership conflicts — warn, require explicit resolution
- Package moved during migration — detect via bazel query, update tracking

### Cherry-Pick & Prepare Mode Edge Cases [🟠 HIGH PRIORITY]
- Partial fix creates inconsistent state — warn, suggest completing
- Recipe step depends on previous step — validate dependencies before skip
- `prepare` mode with `--test` fails — clearly separate from actual upgrade
- Fix applied twice — idempotent, no-op if already applied
- Fix conflicts with manual edit — detect, require resolution

### Known Issues Edge Cases [🟠 HIGH PRIORITY]
- Known issue expires — remind user, require explicit renewal
- Known issue target deleted — auto-remove from list
- New failure matches known issue pattern — check before flagging as new
- Known issue severity escalation — track if issue gets worse

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
├── snapshots/                         # 🆕
│   ├── valid_snapshot.pb
│   └── corrupted_snapshot.pb
├── plugins/                           # 🆕
│   ├── valid_plugin/
│   └── incompatible_plugin/
├── recipes/                           # 🆕
│   ├── valid_recipe.yaml
│   └── malformed_recipe.yaml
└── projects/
    ├── workspace_only/
    ├── bzlmod_only/
    ├── mixed/
    ├── monorepo/                      # 🆕
    └── with_deprecations/             # 🆕
```

---

## Design Philosophy

### Why These Design Decisions?

**1. Safety First (Snapshots, Preflight)**

Upgrades are scary. Users need confidence that they can undo mistakes:

- **Snapshots are automatic** — Users shouldn't have to remember to backup
- **Preflight catches doomed upgrades** — Better to fail fast than waste time
- **Rollback is one command** — Recovery should be trivial

**2. Deterministic Before Heuristic**

Trust is built through predictability:

```
User: "What will this tool do?"
Bad:  "It will try to fix things"
Good: "It will make exactly these 5 changes to these 3 files"
```

**3. Extensibility Without Complexity**

The plugin system follows these principles:

| Principle | Rationale |
|-----------|-----------|
| **Small interface** | Easy to implement, stable over time |
| **Isolation** | Plugin bugs don't crash the tool |
| **Discoverability** | Easy to find and install plugins |
| **No dependency hell** | Plugins are self-contained |

**4. Library First, CLI Second**

All functionality lives in `libbazelbump`. Benefits:

- **Testable** — Unit test the library, not CLI parsing
- **Embeddable** — Use in CI, IDEs, other tools
- **Consistent** — CLI and library have identical behavior

**5. Proto First**

Types are defined in protobuf before Go:

- **Serialization** — Easy to cache, store, transmit
- **Evolution** — Add fields without breaking compatibility
- **Documentation** — Protos are self-documenting contracts

### Extensibility API Stability

The plugin API follows semantic versioning independently:

```go
// pkg/plugin/version.go
const (
    PluginAPIVersion = "1.0.0"  // Increment on breaking changes
)

// Plugins declare required API version
type PluginManifest struct {
    RequiredAPIVersion string  // e.g., ">=1.0.0 <2.0.0"
}
```

**Stability Guarantees:**

| Component | Stability |
|-----------|-----------|
| `pkg/plugin.Plugin` interface | Stable (v1.0+) |
| `pkg/plugin.Analyzer` interface | Stable (v1.0+) |
| `pkg/plugin.Transformer` interface | Stable (v1.0+) |
| `pkg/plugin/types.go` | Stable (v1.0+) |
| `internal/*` | No guarantees |
| Proto messages | Backward compatible additions only |

### What NOT to Add

To keep bazelbump focused, these are explicitly out of scope:

| Feature | Why Not |
|---------|---------|
| Build execution | Use Bazel directly |
| Dependency resolution | Use `bazel mod` |
| Remote cache management | Use Bazel's `--remote_cache` |
| IDE integration (core) | Provide library, IDEs integrate |
| Arbitrary code generation | Use Gazelle |

### Future Extensibility Considerations

For features not in v1.0, the architecture supports:

**Recipe Marketplace (Future)**
```go
// Future: Recipe registry protocol
type RecipeRegistry interface {
    Search(query string) ([]RecipeMetadata, error)
    Fetch(name, version string) (*Recipe, error)
    Publish(recipe *Recipe, auth AuthToken) error
}
```

**Web Dashboard (Future)**
```go
// Future: HTTP API for web UI
type APIServer struct {
    lib *libbazelbump.Client
}

func (s *APIServer) Analyze(w http.ResponseWriter, r *http.Request)
func (s *APIServer) Apply(w http.ResponseWriter, r *http.Request)
func (s *APIServer) Snapshots(w http.ResponseWriter, r *http.Request)
```

**Language Bindings (Future)**

The proto-first approach enables bindings:

```python
# Future: Python bindings
from bazelbump import Client, AnalyzeRequest

client = Client()
response = client.analyze(AnalyzeRequest(
    from_version="8.0.0",
    to_version="9.0.0",
))
```

---

## Third-Party Dependencies

### Complete Dependency List

All dependencies with their current versions (as of February 2025):

```
┌─────────────────────────────────────────────────────────────────────────────┐
│                      DEPENDENCY INVENTORY                                    │
├─────────────────────────────────────────────────────────────────────────────┤
│                                                                              │
│  🔵 OUR ECOSYSTEM (To Create/Use)                                           │
│  ════════════════════════════════                                           │
│  github.com/albertocavalcante/go-bzlmod     EXISTS    Use as dependency    │
│  github.com/bazel-contrib/go-bazelrc        CREATE    .bazelrc parsing      │
│  github.com/bazel-contrib/go-bcr            CREATE    BCR client            │
│  github.com/bazel-contrib/go-bazelflags     CREATE    Flag database         │
│  github.com/bazel-contrib/go-bep            CREATE    BEP parsing           │
│                                                                              │
│  🟢 CORE INFRASTRUCTURE                                                      │
│  ════════════════════════                                                    │
│  google.golang.org/protobuf         v1.36.x    Proto runtime               │
│  github.com/spf13/cobra             v1.10.0    CLI framework               │
│  github.com/spf13/viper             v1.19.x    Configuration               │
│  gopkg.in/yaml.v3                   v3.0.1     YAML parsing                │
│                                                                              │
│  🟡 OUTPUT & FORMATTING                                                      │
│  ══════════════════════                                                      │
│  github.com/fatih/color             v1.18.0    Colored output              │
│  github.com/olekukonko/tablewriter  v1.1.3     Table formatting            │
│  github.com/sergi/go-diff           v1.3.1     Diff generation             │
│                                                                              │
│  🟠 UTILITIES                                                                │
│  ════════════                                                                │
│  github.com/oklog/ulid/v2           v2.1.0     Unique IDs (snapshots)      │
│  github.com/bmatcuk/doublestar/v4   v4.7.1     Glob patterns               │
│                                                                              │
│  🔴 VENDORED                                                                 │
│  ═══════════                                                                 │
│  github.com/bazelbuild/buildtools   v7.3.1     Starlark parser (vendor)    │
│                                                                              │
│  ⚪ TESTING                                                                   │
│  ═════════                                                                   │
│  github.com/stretchr/testify        v1.11.1    Assertions & mocks          │
│                                                                              │
│  🟣 OPTIONAL (Build Tags)                                                    │
│  ════════════════════════                                                    │
│  github.com/charmbracelet/bubbletea v1.2.4     TUI framework (+build tui)  │
│  github.com/charmbracelet/lipgloss  v1.0.0     TUI styling (+build tui)    │
│                                                                              │
└─────────────────────────────────────────────────────────────────────────────┘
```

### Detailed Dependency Reference

#### Core Infrastructure

| Package | Version | License | Purpose | Links |
|---------|---------|---------|---------|-------|
| [google.golang.org/protobuf](https://pkg.go.dev/google.golang.org/protobuf) | v1.36.x | BSD-3 | Protocol Buffers runtime for generated code | [GitHub](https://github.com/protocolbuffers/protobuf-go) |
| [github.com/spf13/cobra](https://pkg.go.dev/github.com/spf13/cobra) | v1.10.0 | Apache-2.0 | CLI framework (used by kubectl, hugo, gh) | [GitHub](https://github.com/spf13/cobra) |
| [github.com/spf13/viper](https://pkg.go.dev/github.com/spf13/viper) | v1.19.x | MIT | Configuration management with layering | [GitHub](https://github.com/spf13/viper) |
| [gopkg.in/yaml.v3](https://pkg.go.dev/gopkg.in/yaml.v3) | v3.0.1 | MIT/Apache-2.0 | YAML parsing for config and recipes | [GitHub](https://github.com/go-yaml/yaml) |

#### Output & Formatting

| Package | Version | License | Purpose | Links |
|---------|---------|---------|---------|-------|
| [github.com/fatih/color](https://pkg.go.dev/github.com/fatih/color) | v1.18.0 | MIT | ANSI color output for terminals | [GitHub](https://github.com/fatih/color) |
| [github.com/olekukonko/tablewriter](https://pkg.go.dev/github.com/olekukonko/tablewriter) | v1.1.3 | MIT | ASCII/Unicode table rendering | [GitHub](https://github.com/olekukonko/tablewriter) |
| [github.com/sergi/go-diff](https://pkg.go.dev/github.com/sergi/go-diff) | v1.3.1 | MIT | Diff, match, patch text (unified diffs) | [GitHub](https://github.com/sergi/go-diff) |

#### Utilities

| Package | Version | License | Purpose | Links |
|---------|---------|---------|---------|-------|
| [github.com/oklog/ulid/v2](https://pkg.go.dev/github.com/oklog/ulid/v2) | v2.1.0 | Apache-2.0 | Lexicographically sortable unique IDs | [GitHub](https://github.com/oklog/ulid) |
| [github.com/bmatcuk/doublestar/v4](https://pkg.go.dev/github.com/bmatcuk/doublestar/v4) | v4.7.1 | MIT | Glob patterns with ** support | [GitHub](https://github.com/bmatcuk/doublestar) |

#### Vendored

| Package | Version | License | Purpose | Links |
|---------|---------|---------|---------|-------|
| [github.com/bazelbuild/buildtools](https://pkg.go.dev/github.com/bazelbuild/buildtools) | v7.3.1 | Apache-2.0 | Starlark/BUILD file parsing & formatting | [GitHub](https://github.com/bazelbuild/buildtools) |

**Why vendor buildtools?**
- Tightly coupled to our Starlark manipulation
- May need patches for our use cases
- Follows go-bzlmod's approach
- Reduces external dependency chain

#### Testing

| Package | Version | License | Purpose | Links |
|---------|---------|---------|---------|-------|
| [github.com/stretchr/testify](https://pkg.go.dev/github.com/stretchr/testify) | v1.11.1 | MIT | Assertions, mocks, test suites | [GitHub](https://github.com/stretchr/testify) |

#### Optional (TUI - Behind Build Tag)

| Package | Version | License | Purpose | Links |
|---------|---------|---------|---------|-------|
| [github.com/charmbracelet/bubbletea](https://pkg.go.dev/github.com/charmbracelet/bubbletea) | v1.2.4 | MIT | Terminal UI framework (Elm-inspired) | [GitHub](https://github.com/charmbracelet/bubbletea) |
| [github.com/charmbracelet/lipgloss](https://pkg.go.dev/github.com/charmbracelet/lipgloss) | v1.0.0 | MIT | Terminal styling and layout | [GitHub](https://github.com/charmbracelet/lipgloss) |

### go.mod Template

```go
// go.mod
module github.com/albertocavalcante/bazelbump

go 1.23

require (
    // === Our Ecosystem ===
    github.com/albertocavalcante/go-bzlmod v0.x.x
    // github.com/bazel-contrib/go-bazelrc v0.x.x   // CREATE FIRST
    // github.com/bazel-contrib/go-bcr v0.x.x       // CREATE FIRST
    // github.com/bazel-contrib/go-bazelflags v0.x.x // CREATE FIRST

    // === Core Infrastructure ===
    google.golang.org/protobuf v1.36.3
    github.com/spf13/cobra v1.10.0
    github.com/spf13/viper v1.19.0
    gopkg.in/yaml.v3 v3.0.1

    // === Output & Formatting ===
    github.com/fatih/color v1.18.0
    github.com/olekukonko/tablewriter v1.1.3
    github.com/sergi/go-diff v1.3.1

    // === Utilities ===
    github.com/oklog/ulid/v2 v2.1.0
    github.com/bmatcuk/doublestar/v4 v4.7.1
)

require (
    // === Testing (test only) ===
    github.com/stretchr/testify v1.11.1
)

// === Optional: TUI (+build tui) ===
// github.com/charmbracelet/bubbletea v1.2.4
// github.com/charmbracelet/lipgloss v1.0.0
```

### Dependency Selection Criteria

| Criteria | Requirement |
|----------|-------------|
| **Maintenance** | Active development, recent releases |
| **License** | Permissive (MIT, Apache-2.0, BSD) |
| **Stability** | Stable API, follows semver |
| **Popularity** | Widely used, battle-tested |
| **Dependencies** | Minimal transitive dependencies |
| **Go Version** | Compatible with Go 1.21+ |

### Dependency Update Policy

```yaml
# .github/dependabot.yml
version: 2
updates:
  - package-ecosystem: "gomod"
    directory: "/"
    schedule:
      interval: "weekly"
    groups:
      production-dependencies:
        patterns:
          - "github.com/spf13/*"
          - "google.golang.org/*"
      development-dependencies:
        patterns:
          - "github.com/stretchr/*"
    ignore:
      # Don't auto-update major versions
      - dependency-name: "*"
        update-types: ["version-update:semver-major"]
```

### Security Considerations

- All dependencies are scanned via `govulncheck`
- Dependabot alerts enabled
- License compliance checked via `go-licenses`
- Vendored code (buildtools) pinned to specific commits

```bash
# Security scanning
govulncheck ./...

# License check
go-licenses check ./...

# Dependency audit
go mod verify
```

---

## Summary

### Core Philosophies

```
┌─────────────────────────────────────────────────────────────────────────────┐
│                         BAZELBUMP PHILOSOPHIES                               │
├─────────────────────────────────────────────────────────────────────────────┤
│                                                                              │
│  FROM BAZEL:                                                                 │
│  ├── Hermetic        Same inputs → same outputs, always                     │
│  ├── Phased          Loading → Analysis → Planning → Execution              │
│  ├── Cacheable       Aggressive caching at every layer                      │
│  ├── Queryable       Powerful filtering and exploration                     │
│  └── Configurable    Layered config with clear precedence                   │
│                                                                              │
│  FROM GIT:                                                                   │
│  ├── Porcelain/Plumbing  High-level UX + low-level composability           │
│  ├── Content-Addressed   Snapshots identified by content hash              │
│  ├── History             Track upgrade history, support rollback            │
│  └── Consistent UX       Same flags work everywhere                         │
│                                                                              │
│  OUR OWN:                                                                    │
│  ├── Safety First        Snapshots + preflight by default                   │
│  ├── Build Foundations   Create reusable libraries first                    │
│  ├── Deterministic First Heuristics only when necessary                     │
│  ├── Cherry-Pick Ready   Partial, incremental migrations                    │
│  └── Monorepo Reality    Handle broken targets, team ownership              │
│                                                                              │
└─────────────────────────────────────────────────────────────────────────────┘
```

### Architecture Summary

```
┌─────────────────────────────────────────────────────────────────────────────┐
│                                                                              │
│                              bazelbump                                       │
│                     (Upgrade logic, UX, orchestration)                      │
│                                                                              │
│  ┌─────────────────────────────────────────────────────────────────────┐   │
│  │                    Foundational Libraries                            │   │
│  │  ┌───────────┐ ┌───────────┐ ┌─────────────┐ ┌───────────┐         │   │
│  │  │ go-bzlmod │ │go-bazelrc │ │go-bazelflags│ │  go-bcr   │         │   │
│  │  │ (EXISTS)  │ │ (CREATE)  │ │  (CREATE)   │ │ (CREATE)  │         │   │
│  │  └───────────┘ └───────────┘ └─────────────┘ └───────────┘         │   │
│  │       │             │              │              │                 │   │
│  │       ▼             ▼              ▼              ▼                 │   │
│  │  ┌─────────────────────────────────────────────────────────────┐   │   │
│  │  │                      buildtools (VENDOR)                     │   │   │
│  │  │                    (Starlark parsing/formatting)             │   │   │
│  │  └─────────────────────────────────────────────────────────────┘   │   │
│  └─────────────────────────────────────────────────────────────────────┘   │
│                                                                              │
└─────────────────────────────────────────────────────────────────────────────┘
```

### Design Principles

| Principle | Implementation |
|-----------|----------------|
| **Safe** | Snapshots + preflight = confident upgrades |
| **Hermetic** | Explicit inputs, reproducible analysis |
| **Smart** | Path optimization + readiness scores |
| **Extensible** | Plugins + recipes = community contributions |
| **Composable** | Porcelain + plumbing = scripts and power users |
| **Incremental** | Cherry-pick + prepare + exclusions |
| **Realistic** | Known issues + team ownership = monorepo reality |
| **Reusable** | Foundational libraries benefit ecosystem |

### Priority Matrix

| Priority | Features | Target |
|----------|----------|--------|
| **PHASE 0** | Foundational libraries (go-bazelrc, go-bcr, go-bazelflags) | Before v1.0 |
| 🔴 **PARAMOUNT** | Snapshots, Preflight, Phased Analysis, Transforms | v1.0 |
| 🟠 **HIGH** | Path Optimizer, Cherry-Pick, Exclusions, Plugins, CI/CD | v1.0 |
| 🟡 **MEDIUM** | Recipes, Team-Based Migration, Documentation Gen | v1.x |
| 🟢 **NICE TO HAVE** | TUI, Notifications, Multi-Repo, Web Dashboard | Backlog |

### Key Differentiators

```
What makes bazelbump special?

1. SAFETY BUILT-IN
   Not an afterthought. Snapshots and preflight are DEFAULT.
   Users can't accidentally destroy their project.

2. HERMETIC ANALYSIS
   Unlike ad-hoc scripts, bazelbump is reproducible.
   Same inputs = same results. Cache everything.

3. PHASED LIKE BAZEL
   Clear separation: Loading → Analysis → Planning → Execution.
   Each phase is cacheable, debuggable, resumable.

4. PORCELAIN + PLUMBING
   Great UX for humans. Composable primitives for scripts.
   Power users can build custom workflows.

5. MONOREPO REALITY
   We don't pretend all targets are green.
   Exclusions, known issues, team ownership, partial migration.

6. ECOSYSTEM CONTRIBUTION
   We build foundational libraries that benefit everyone.
   go-bazelrc, go-bcr, go-bazelflags are useful beyond bazelbump.

7. CHERRY-PICK READY
   Don't have to do everything at once.
   Fix imports today, flags next week, upgrade next month.
```
