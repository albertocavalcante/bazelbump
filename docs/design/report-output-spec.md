# Report Output System Specification

**Status:** Draft v1  
**Author:** bazelbump contributors  
**Created:** 2026-02-03  
**Updated:** 2026-02-03

## Summary

This specification defines a comprehensive report output system for bazelbump that provides structured, machine-readable output in multiple formats. The system enables CI/CD integrations, HTML reports for sharing, IDE integrations, terminal visualizations, and spreadsheet analysis.

## Motivation

### Current State

bazelbump currently outputs:
1. **Text output (default)**: Colorized diff-style output for terminal
2. **Basic JSON (`--json`)**: Minimal JSON serialization of results

The current JSON output lacks:
- Comprehensive metadata (timing, version, environment)
- Hierarchical organization (by directory, rule type, severity)
- Full diff information for file changes
- Statistics and aggregations
- Support for standard formats (SARIF, Markdown, CSV)

### Use Cases

| Consumer | Format | Use Case |
|----------|--------|----------|
| CI/CD Pipeline | JSON, SARIF | Automated checks, PR annotations |
| GitHub Code Scanning | SARIF | Native security tab integration |
| VS Code Extension | SARIF, JSON | Inline diagnostics |
| Team Lead | HTML | Share migration scope with stakeholders |
| Spreadsheet Analysis | CSV | Filter/sort issues, track progress |
| PR Description | Markdown | Summarize changes for reviewers |
| Large Projects (Envoy) | Streaming JSON | Memory-efficient processing |

---

## 1. Structured Report Data Model

### 1.1 Core Schema Overview

```
Report
├── metadata
│   ├── tool (name, version, commit)
│   ├── invocation (command, args, cwd, timestamp, duration)
│   └── project (path, bazel_version, from_version, to_version)
├── summary
│   ├── totals (files_scanned, files_modified, total_issues)
│   ├── by_severity (error, warning, info)
│   ├── by_type (removed, deprecated, changed_default)
│   └── by_directory (hierarchical counts)
├── results[]
│   ├── file
│   ├── issues[]
│   └── changes[]
└── rules[]  (for SARIF compatibility)
```

### 1.2 Example JSON Report

```json
{
  "$schema": "https://bazelbump.dev/schemas/report-v1.json",
  "version": "1.0.0",
  "metadata": {
    "tool": {
      "name": "bazelbump",
      "version": "0.2.0",
      "commit": "c662d88"
    },
    "invocation": {
      "command": "analyze",
      "arguments": ["--to", "9.0.0", "--path", "/home/user/envoy"],
      "working_directory": "/home/user/envoy",
      "timestamp": "2026-02-03T10:30:00Z",
      "duration_ms": 847
    },
    "project": {
      "path": "/home/user/envoy",
      "from_version": "8.0.0",
      "to_version": "9.0.0"
    }
  },
  "summary": {
    "totals": {
      "files_scanned": 1696,
      "files_with_issues": 893,
      "total_issues": 2847,
      "lines_added": 967,
      "lines_removed": 0
    },
    "by_severity": {
      "error": 156,
      "warning": 2691,
      "info": 0
    },
    "by_type": {
      "removed": 156,
      "deprecated": 47,
      "native_rule": 2644
    },
    "by_rule_family": {
      "cc": 1823,
      "python": 456,
      "proto": 365
    },
    "by_directory": [
      {
        "path": "source",
        "files": 423,
        "issues": 1456,
        "children": [
          { "path": "source/common", "files": 89, "issues": 234 },
          { "path": "source/extensions", "files": 312, "issues": 1189 }
        ]
      },
      {
        "path": "test",
        "files": 470,
        "issues": 1391
      }
    ]
  },
  "results": [
    {
      "path": "/home/user/envoy/.bazelrc",
      "relative_path": ".bazelrc",
      "issues": [
        {
          "id": "issue-001",
          "rule_id": "flag-removed",
          "type": "removed",
          "severity": "error",
          "message": "Flag --experimental_enable_bzlmod was removed in Bazel 9.0.0",
          "location": {
            "file": ".bazelrc",
            "line": 12,
            "column": 1,
            "snippet": "build --experimental_enable_bzlmod"
          },
          "suggestion": "Remove this flag; bzlmod is now the default",
          "fix_available": true
        }
      ]
    }
  ],
  "rules": [
    {
      "id": "flag-removed",
      "name": "Removed Flag",
      "short_description": "Flag does not exist in target Bazel version",
      "help_uri": "https://bazelbump.dev/rules/flag-removed",
      "default_severity": "error"
    }
  ]
}
```

---

## 2. Output Formats

### 2.1 Format Comparison

| Format | Machine Readable | Human Readable | Portable | Size | Use Case |
|--------|-----------------|----------------|----------|------|----------|
| JSON | Excellent | Poor | Excellent | Medium | CI/CD, APIs, tooling |
| SARIF | Excellent | Poor | Excellent | Large | GitHub, IDEs, security tools |
| HTML | N/A | Excellent | Excellent | Large | Reports, sharing |
| Markdown | Fair | Good | Good | Small | PRs, documentation |
| CSV | Good | Fair | Excellent | Small | Spreadsheets, data analysis |
| Text | Poor | Excellent | Good | Small | Terminal, logs |

### 2.2 JSON Format

**Purpose**: Foundation format for machine consumption and transformation.

**Variants**:
- `json` - Full report (default)
- `json-compact` - No indentation, minimal whitespace
- `json-lines` - Newline-delimited JSON (one result per line, for streaming)

**Example CLI**:
```bash
bazelbump analyze --to 9.0.0 --output json
bazelbump analyze --to 9.0.0 --output json-compact
bazelbump analyze --to 9.0.0 --output json-lines  # For streaming
```

**Pros**:
- Universal parsing support
- Lossless representation
- Easy to transform to other formats
- jq-friendly for filtering

**Cons**:
- Not human-readable without tooling
- Verbose for simple reports

### 2.3 SARIF Format

**Purpose**: [Static Analysis Results Interchange Format](https://docs.oasis-open.org/sarif/sarif/v2.1.0/sarif-v2.1.0.html) for GitHub Code Scanning and IDE integration.

**Example SARIF Output**:
```json
{
  "$schema": "https://json.schemastore.org/sarif-2.1.0.json",
  "version": "2.1.0",
  "runs": [
    {
      "tool": {
        "driver": {
          "name": "bazelbump",
          "version": "0.2.0",
          "informationUri": "https://github.com/albertocavalcante/bazelbump",
          "rules": [
            {
              "id": "flag-removed",
              "shortDescription": { "text": "Flag removed in target version" },
              "defaultConfiguration": { "level": "error" }
            }
          ]
        }
      },
      "results": [
        {
          "ruleId": "flag-removed",
          "level": "error",
          "message": { "text": "Flag --experimental_enable_bzlmod was removed" },
          "locations": [
            {
              "physicalLocation": {
                "artifactLocation": { "uri": ".bazelrc" },
                "region": { "startLine": 12, "startColumn": 1 }
              }
            }
          ]
        }
      ]
    }
  ]
}
```

**GitHub Integration**:
```yaml
# .github/workflows/check.yml
- name: Run bazelbump
  run: bazelbump analyze --to 9.0.0 --output sarif --report results.sarif

- name: Upload SARIF
  uses: github/codeql-action/upload-sarif@v3
  with:
    sarif_file: results.sarif
```

### 2.4 HTML Format

**Purpose**: Self-contained, shareable reports with visualizations.

**Features**:
- Single-file output (all CSS/JS inline)
- Interactive filtering and sorting
- Collapsible directory tree
- Syntax-highlighted code snippets
- Summary charts (by severity, type, directory)
- Dark/light mode support
- Print-friendly stylesheet

**Example Structure**:
```
┌─ Bazel Upgrade Report ─────────────────────────────────┐
│                                                         │
│  Project: envoy                                         │
│  Upgrade: 8.0.0 → 9.0.0                                │
│  Files: 1,696 scanned | 893 with issues                │
│                                                         │
│  ┌─ Summary ──────────┐  ┌─ By Type ─────────────────┐ │
│  │ 🔴 Errors:    156  │  │ Removed flags:       156  │ │
│  │ 🟡 Warnings: 2,691 │  │ Native rules:      2,644  │ │
│  │ ℹ️  Info:        0  │  │ Deprecated:           47  │ │
│  └────────────────────┘  └───────────────────────────┘ │
│                                                         │
│  ┌─ Directory Tree ─────────────────────────────────┐  │
│  │ ▼ source/ (1,456 issues)                         │  │
│  │   ▼ common/ (234)                                │  │
│  │   ▼ extensions/ (1,189)                          │  │
│  │ ▼ test/ (1,391 issues)                           │  │
│  └──────────────────────────────────────────────────┘  │
│                                                         │
│  [Filter: All] [Severity: All] [Export CSV]            │
│                                                         │
│  ┌─ Issues ─────────────────────────────────────────┐  │
│  │ .bazelrc:12  🔴 Flag --enable_bzlmod removed     │  │
│  │ source/BUILD:5  🟡 Native cc_library needs load  │  │
│  │ ...                                              │  │
│  └──────────────────────────────────────────────────┘  │
└─────────────────────────────────────────────────────────┘
```

### 2.5 Markdown Format

**Purpose**: PR descriptions, documentation, GitHub comments.

**Example Output**:
```markdown
# Bazel Upgrade Analysis

**Project**: envoy  
**Upgrade**: 8.0.0 → 9.0.0  
**Date**: 2026-02-03  

## Summary

| Metric | Count |
|--------|-------|
| Files Scanned | 1,696 |
| Files with Issues | 893 |
| Total Issues | 2,847 |

### By Severity

- 🔴 **Errors**: 156
- 🟡 **Warnings**: 2,691

<details>
<summary><code>.bazelrc</code> - 1 issue</summary>

| Line | Severity | Message |
|------|----------|---------|
| 12 | error | Flag `--experimental_enable_bzlmod` was removed |

</details>

---
*Generated by [bazelbump](https://github.com/albertocavalcante/bazelbump)*
```

### 2.6 CSV Format

**Purpose**: Spreadsheet analysis, data import.

**CSV Schema**:
```csv
file,line,column,severity,type,rule_id,message,suggestion,fix_available
.bazelrc,12,1,error,removed,flag-removed,"Flag --experimental_enable_bzlmod was removed","Remove this flag",true
source/common/BUILD,5,1,warning,native_rule,native-rule-usage,"Native rule cc_library used without load","Add load statement",true
```

---

## 3. CLI Interface Design

### 3.1 Output Flags

```bash
# Format selection
--output <format>   # json, json-compact, json-lines, sarif, html, markdown, csv, text
-o <format>         # Short form

# File output
--report <file>     # Write to file instead of stdout
-r <file>           # Short form

# Format-specific options
--output-options '{"indent":4,"theme":"dark"}'
```

### 3.2 Command Integration

```bash
# Analysis
bazelbump analyze --to 9.0.0 --output json
bazelbump analyze --to 9.0.0 --output sarif --report results.sarif
bazelbump analyze --to 9.0.0 --output html --report report.html

# Recipe run
bazelbump recipe run autoload-migration --dry-run --output json
bazelbump recipe run autoload-migration --output html --report changes.html

# Diff
bazelbump diff --from 8.0.0 --to 9.0.0 --output markdown
```

### 3.3 Streaming vs Buffered

| Scenario | Mode | Rationale |
|----------|------|-----------|
| < 100 files | Buffered | Build complete report |
| > 1000 files | Streaming | Memory efficiency |
| `json-lines` | Streaming | Always stream |
| `html` | Buffered | Needs complete data |

---

## 4. Implementation Architecture

### 4.1 Package Structure

```
pkg/report/
├── report.go           # Report type and builder
├── format.go           # Format interface and registry
├── writer.go           # Output writing utilities
├── formats/
│   ├── json.go         # JSON formatter
│   ├── sarif.go        # SARIF formatter  
│   ├── html.go         # HTML formatter
│   ├── markdown.go     # Markdown formatter
│   └── csv.go          # CSV formatter
├── templates/
│   ├── html/           # HTML templates (embedded)
│   └── markdown/       # Markdown templates
└── streaming/
    └── jsonlines.go    # JSON Lines streamer
```

### 4.2 Core Types

```go
// Report is the canonical internal representation
type Report struct {
    Metadata Metadata       `json:"metadata"`
    Summary  Summary        `json:"summary"`
    Results  []FileResult   `json:"results"`
    Rules    []Rule         `json:"rules,omitempty"`
}

// Format defines the interface for output formatters
type Format interface {
    Name() string
    ContentType() string
    FileExtension() string
    Format(report *Report, opts Options) ([]byte, error)
    SupportsStreaming() bool
}

// Registry manages available formats
var DefaultRegistry = &Registry{
    formats: map[string]Format{
        "json":      &JSONFormat{},
        "sarif":     &SARIFFormat{},
        "html":      &HTMLFormat{},
        "markdown":  &MarkdownFormat{},
        "csv":       &CSVFormat{},
    },
}
```

---

## 5. Performance Considerations

### 5.1 Memory Management

For large projects (1000+ files):

| Strategy | Implementation |
|----------|---------------|
| Streaming JSON | Write results as discovered |
| Lazy diff generation | Compute hunks only when needed |
| Summary-only mode | Skip detailed results |

### 5.2 Benchmarks

Target performance for Envoy-scale projects (1,696 files):

| Operation | Target |
|-----------|--------|
| Analysis + JSON | < 2s |
| Analysis + HTML | < 3s |
| Streaming JSON | < 1.5s |
| SARIF conversion | < 500ms |

---

## 6. Tradeoff Analysis

### 6.1 Rich JSON vs Flat JSON

**Decision**: Use **rich JSON** (nested structure) as canonical format.

**Rationale**: 
- Better type safety and extensibility
- Natural mapping to SARIF
- jq can still query nested structures

**Mitigation**: Provide jq examples:
```bash
# Get all errors
cat report.json | jq '.results[].issues[] | select(.severity == "error")'
```

### 6.2 Embedded HTML vs External Assets

**Decision**: Use **embedded HTML** (single file) by default.

**Rationale**:
- Excellent portability (share via email/Slack)
- Works offline
- No server needed

**Mitigation**: Add `--output-options '{"external_assets":true}'` for development.

### 6.3 SARIF Complexity

**Decision**: Implement **GitHub-compatible subset** of SARIF 2.1.0.

**Implemented**:
- `runs[].tool.driver.{name, version, rules}`
- `runs[].results[].{ruleId, level, message, locations, fixes}`
- `runs[].invocations`

**Not implemented** (yet):
- `taxonomies`, `graphs`, `threadFlowLocations`

### 6.4 Diff Granularity

**Decision**: Support **multiple granularities** based on format.

| Format | Granularity |
|--------|-------------|
| JSON | Hunk-level (full fidelity) |
| SARIF | Line-level (GitHub annotations) |
| HTML | Hunk-level (visual diff) |
| Markdown | File-level (readable PRs) |
| CSV | Issue-level (no diff) |

---

## 7. Examples

### 7.1 CI/CD Integration

```yaml
# .github/workflows/bazel-upgrade.yml
- name: Run analysis
  run: bazelbump analyze --to 9.0.0 --output sarif --report results.sarif

- name: Upload SARIF
  uses: github/codeql-action/upload-sarif@v3
  with:
    sarif_file: results.sarif

- name: PR Comment
  if: github.event_name == 'pull_request'
  run: |
    bazelbump analyze --to 9.0.0 --output markdown > comment.md
    gh pr comment ${{ github.event.pull_request.number }} --body-file comment.md
```

### 7.2 Spreadsheet Pipeline

```bash
# Weekly report
bazelbump analyze --to 9.0.0 --output csv --report issues-$(date +%Y%m%d).csv

# Summary by directory
bazelbump analyze --to 9.0.0 --output json | jq -r '
  .summary.by_directory[] | [.path, .issues] | @csv
' > by-directory.csv
```

### 7.3 Slack Notification

```bash
SUMMARY=$(bazelbump analyze --to 9.0.0 --output json | jq -r '
  "Files: \(.summary.totals.files_scanned) | Issues: \(.summary.totals.total_issues)"
')
curl -X POST --data "{\"text\":\"Bazel Status: $SUMMARY\"}" $SLACK_WEBHOOK
```

---

## 8. Migration Plan

| Phase | Scope | Timeline |
|-------|-------|----------|
| 1 | Core infrastructure + JSON | Week 1-2 |
| 2 | SARIF + Markdown | Week 3-4 |
| 3 | HTML + CSV | Week 5-6 |
| 4 | Streaming + Performance | Week 7-8 |

---

## References

- [SARIF 2.1.0 Specification](https://docs.oasis-open.org/sarif/sarif/v2.1.0/sarif-v2.1.0.html)
- [GitHub SARIF Support](https://docs.github.com/en/code-security/code-scanning/integrating-with-code-scanning/sarif-support-for-code-scanning)
- [ESLint Formatters](https://eslint.org/docs/latest/use/formatters/)
- [Terraform Plan JSON](https://developer.hashicorp.com/terraform/internals/json-format)
