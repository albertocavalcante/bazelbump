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

## Architecture Principles

bazelbump follows three core architectural principles:

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
│   │   └── migration.proto          # Migration rules and hints
│   ├── vendor/                      # Vendored canonical protos
│   │   └── bazel/
│   │       └── src/main/protobuf/
│   └── buf.yaml                     # Buf configuration
│
├── pkg/                             # LIB-FIRST: Public library API
│   └── libbazelbump/
│       ├── client.go                # Main Client type
│       ├── analyze.go               # Analyze() implementation
│       ├── check.go                 # Check() implementation
│       ├── apply.go                 # Apply() implementation
│       ├── flags.go                 # Flag operations
│       ├── options.go               # Functional options
│       └── libbazelbump_test.go
│
├── cmd/                             # CLI: Thin wrapper over library
│   └── bazelbump/
│       ├── main.go                  # Entry point
│       ├── analyze.go               # analyze command
│       ├── check.go                 # check command
│       ├── apply.go                 # apply command
│       ├── flags.go                 # flags command
│       └── format.go                # Output formatting
│
├── internal/                        # Private implementation details
│   ├── bazelrc/
│   │   ├── parser.go                # .bazelrc parser
│   │   └── parser_test.go
│   ├── starlark/
│   │   ├── module.go                # MODULE.bazel parsing (uses buildtools)
│   │   └── workspace.go             # WORKSPACE parsing
│   ├── version/
│   │   ├── detect.go                # Version detection
│   │   ├── compare.go               # Version comparison
│   │   └── lts.go                   # LTS version tracking
│   ├── flagdb/
│   │   ├── database.go              # Flag database operations
│   │   ├── generate.go              # Database generation from Bazel
│   │   └── embed.go                 # Embedded database
│   ├── registry/
│   │   ├── bcr.go                   # BCR client
│   │   └── github.go                # GitHub API client
│   └── cache/
│       └── cache.go                 # Caching layer
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
│           └── ...
│
├── data/                            # Embedded data files
│   └── flags.pb                     # Flag database (protobuf binary)
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
    // Proto runtime
    google.golang.org/protobuf v1.32.0      // Proto runtime (required for generated code)

    // CLI layer only
    github.com/spf13/cobra v1.8.0           // CLI framework
    github.com/fatih/color v1.16.0          // Colored output
    github.com/olekukonko/tablewriter v0.0.5 // Table formatting
)

// Vendored (no external dependency):
// - buildtools/build (Starlark parser)
// - bazel protos (canonical types)
```

```yaml
# buf.gen.yaml
version: v1
plugins:
  - plugin: buf.build/protocolbuffers/go
    out: gen/go
    opt: paths=source_relative
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
