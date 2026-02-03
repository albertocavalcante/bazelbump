# Milestone 2: Analysis Core

**Target:** Weeks 3-4
**Status:** Not Started
**Depends On:** M1 (Data Layer)

## Objective

Implement core analysis capabilities: .bazelrc parsing, flag analysis, and basic issue detection.

## Deliverables

### 1. .bazelrc Parser

```go
// internal/bazelrc/parser.go
type Parser struct {
    workspaceRoot string
}

type ParsedRC struct {
    Lines    []Line
    Imports  []Import
    Configs  map[string][]Line  // config name -> lines
}

type Line struct {
    LineNumber int
    Raw        string
    Command    string           // build, test, run, etc.
    Config     string           // e.g., "ci" for build:ci
    Flags      []Flag
    Comment    string
}

type Flag struct {
    Name     string
    Value    string
    Negated  bool    // --nofoo
    Position int     // position in line
}

func (p *Parser) Parse(path string) (*ParsedRC, error)
func (p *Parser) ParseContent(content string) (*ParsedRC, error)
```

**Features:**
- Parse commands (build, test, run, etc.)
- Parse configs (build:ci, test:unit, etc.)
- Handle imports and try-imports
- Resolve %workspace% paths
- Handle flag negation (--nofoo)
- Preserve comments and structure

### 2. Flag Analysis

```go
// internal/analysis/flags.go
type FlagAnalyzer struct {
    flagDB *datastore.FlagDatabase
}

type FlagIssue struct {
    Flag        string
    File        string
    Line        int
    IssueType   FlagIssueType
    Description string
    Migration   string
    Severity    Severity
}

func (a *FlagAnalyzer) Analyze(rc *ParsedRC, fromVersion, toVersion string) ([]FlagIssue, error)
```

**Detects:**
- Removed flags
- Renamed flags
- Changed defaults
- Deprecated flags
- Incompatible flags that flip

### 3. Starlark Scanner

```go
// internal/starlark/scanner.go
type Scanner struct {
    deprecations *StarlarkDeprecationDatabase
}

type StarlarkIssue struct {
    File        string
    Line        int
    API         string
    Replacement string
    Severity    Severity
    Context     string   // surrounding code
}

func (s *Scanner) ScanFile(path string, targetVersion string) ([]StarlarkIssue, error)
func (s *Scanner) ScanDirectory(dir string, targetVersion string) ([]StarlarkIssue, error)
```

**Detects:**
- `native.existing_rule()` usage
- `ctx.host_configuration` usage
- `cfg = "host"` usage
- `native.cc_library` etc. usage
- Other deprecated APIs

### 4. Analyze Command

```bash
$ bazelbump analyze --to 9.0.0

ANALYSIS REPORT: my-project
===========================

Current: Bazel 8.0.0
Target:  Bazel 9.0.0

FLAG ISSUES (5 found)
---------------------
  .bazelrc:12
    --experimental_enable_bzlmod
    Status: REMOVED in 9.0 (now always enabled)
    Action: Remove this flag

  .bazelrc:45
    --host_copt
    Status: RENAMED in 9.0
    Action: Use --copt instead

STARLARK ISSUES (3 found)
-------------------------
  tools/macros.bzl:45
    native.existing_rule()
    Status: REMOVED in 9.0
    Action: Use native.existing_rules().get("name")

  build/transitions.bzl:23
    cfg = "host"
    Status: REMOVED in 8.0
    Action: Use cfg = "exec"

SUMMARY
-------
  Blocking issues:  3
  Warnings:         5
  Upgrade ready:    NO

Run 'bazelbump apply --to 9.0.0' to fix automatically fixable issues.
```

### 5. Library API

```go
// pkg/libbazelbump/analyze.go
type Client struct {
    datastore *datastore.DataStore
}

type AnalyzeRequest struct {
    ProjectPath   string
    FromVersion   string
    ToVersion     string
    IncludeFlags  bool
    IncludeStarlark bool
}

type AnalyzeResponse struct {
    FromVersion   string
    ToVersion     string
    FlagIssues    []*FlagIssue
    StarlarkIssues []*StarlarkIssue
    Summary       *AnalysisSummary
}

func (c *Client) Analyze(ctx context.Context, req *AnalyzeRequest) (*AnalyzeResponse, error)
```

## Package Structure

```
internal/
├── bazelrc/
│   ├── parser.go
│   ├── parser_test.go
│   ├── types.go
│   └── testdata/
│       ├── simple.bazelrc
│       ├── with_imports.bazelrc
│       └── complex.bazelrc
│
├── starlark/
│   ├── scanner.go
│   ├── scanner_test.go
│   ├── patterns.go
│   └── testdata/
│       ├── deprecated_apis.bzl
│       └── clean.bzl
│
├── analysis/
│   ├── flags.go
│   ├── flags_test.go
│   ├── aggregator.go
│   └── report.go
│
pkg/libbazelbump/
├── client.go
├── analyze.go
└── analyze_test.go

cmd/bazelbump/
└── analyze.go
```

## Tasks

- [ ] Implement .bazelrc parser
- [ ] Add .bazelrc test cases
- [ ] Implement flag analyzer
- [ ] Implement Starlark scanner
- [ ] Add Starlark test cases
- [ ] Implement issue aggregation
- [ ] Implement report formatting (text, JSON)
- [ ] Implement `bazelbump analyze` command
- [ ] Implement library API
- [ ] Add integration tests
- [ ] Document analysis output format

## Acceptance Criteria

1. `.bazelrc` parser handles all common formats
2. Flag analysis correctly identifies deprecated/removed flags
3. Starlark scanner finds deprecated API usage
4. `bazelbump analyze` produces useful report
5. JSON output is machine-readable
6. Tests cover edge cases

## Test Data

Create test fixtures for:
- Simple .bazelrc (flags only)
- Complex .bazelrc (imports, configs, try-import)
- .bzl files with deprecated APIs
- .bzl files with no issues (clean)
- Real-world examples

## Notes

- Focus on accuracy over completeness
- Start with most common patterns
- Add patterns incrementally based on real usage
- Keep parser simple and maintainable
