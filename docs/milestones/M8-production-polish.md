# Milestone 8: Production Polish

**Target:** Week 14
**Status:** Not Started
**Depends On:** M7 (Workflow Integration)

## Objective

Harden the tool for production use: error handling, documentation, testing, and release packaging.

## Deliverables

### 1. Error Handling & Messaging

```go
// pkg/errors/errors.go
package errors

import "fmt"

type Category int

const (
    CategoryUser Category = iota    // User error (fixable by user)
    CategoryConfig                  // Configuration error
    CategoryNetwork                 // Network/connectivity issue
    CategoryInternal               // Bug in bazelbump
)

type Error struct {
    Category    Category
    Code        string
    Message     string
    Details     string
    Remediation string
    Wrapped     error
}

func (e *Error) Error() string {
    return fmt.Sprintf("[%s] %s", e.Code, e.Message)
}

// User-friendly error display
func (e *Error) UserMessage() string {
    msg := e.Message
    if e.Details != "" {
        msg += "\n\nDetails: " + e.Details
    }
    if e.Remediation != "" {
        msg += "\n\nTo fix: " + e.Remediation
    }
    return msg
}
```

**Common errors:**

```go
var (
    ErrNoModuleFile = &Error{
        Category:    CategoryUser,
        Code:        "NO_MODULE_BAZEL",
        Message:     "No MODULE.bazel found",
        Remediation: "Run 'bazelbump init' to create one, or run from a Bazel workspace",
    }

    ErrBazelNotFound = &Error{
        Category:    CategoryUser,
        Code:        "BAZEL_NOT_FOUND",
        Message:     "Bazel not found in PATH",
        Remediation: "Install Bazel: https://bazel.build/install",
    }

    ErrLLMUnavailable = &Error{
        Category:    CategoryNetwork,
        Code:        "LLM_UNAVAILABLE",
        Message:     "LLM provider not reachable",
        Remediation: "Check network connection or configure local Ollama",
    }
)
```

### 2. Logging & Observability

```go
// pkg/observability/logging.go
package observability

import (
    "log/slog"
    "os"
)

func SetupLogging(level string, format string) *slog.Logger {
    var handler slog.Handler

    opts := &slog.HandlerOptions{
        Level: parseLevel(level),
    }

    switch format {
    case "json":
        handler = slog.NewJSONHandler(os.Stderr, opts)
    default:
        handler = slog.NewTextHandler(os.Stderr, opts)
    }

    logger := slog.New(handler)
    slog.SetDefault(logger)
    return logger
}
```

**Structured logging:**

```bash
# Normal output (stderr)
$ bazelbump analyze --to 9.0.0
Analyzing for Bazel 9.0.0...
Found 12 flag issues, 3 Starlark issues

# Debug mode
$ bazelbump analyze --to 9.0.0 -v
2025-02-03T10:00:00Z INF scanning .bazelrc files
2025-02-03T10:00:01Z INF found flag issue flag=--experimental_enable_bzlmod action=remove
2025-02-03T10:00:02Z INF scanning starlark files count=45
2025-02-03T10:00:05Z INF analysis complete flags=12 starlark=3 duration=5.2s

# JSON output for tooling
$ bazelbump analyze --to 9.0.0 --output json 2>/dev/null
{"version":"9.0.0","flags":12,"starlark":3,...}
```

### 3. Test Coverage

**Unit tests:**
```
pkg/
├── analyzer/
│   ├── flags_test.go        # Flag analysis tests
│   └── starlark_test.go     # Starlark analysis tests
├── transform/
│   ├── engine_test.go       # Transform engine tests
│   └── transforms_test.go   # Individual transform tests
├── workflow/
│   └── simple/
│       └── engine_test.go   # SQLite workflow tests
└── llm/
    └── chain_test.go        # Provider chain tests
```

**Integration tests:**
```go
// tests/integration/migrate_test.go
func TestMigration_8_to_9(t *testing.T) {
    // Setup test project with Bazel 8 patterns
    project := setupTestProject(t, "testdata/bazel8-project")

    // Run migration
    client := libbazelbump.New(libbazelbump.Options{
        Workflow: workflow.NewNoOp(),
        LLM:      llm.NewNoOp(),
    })

    resp, err := client.Apply(context.Background(), &libbazelbump.ApplyRequest{
        ProjectPath: project,
        ToVersion:   "9.0.0",
    })
    require.NoError(t, err)
    assert.Equal(t, 12, resp.TransformsApplied)

    // Verify build works
    cmd := exec.Command("bazel", "build", "//...")
    cmd.Dir = project
    assert.NoError(t, cmd.Run())
}
```

**Golden tests:**
```
testdata/
├── bazel8-project/
│   ├── .bazelrc
│   ├── MODULE.bazel
│   └── BUILD
├── expected/
│   ├── .bazelrc.golden
│   ├── MODULE.bazel.golden
│   └── BUILD.golden
```

### 4. Documentation

**User documentation:**
```
docs/
├── getting-started.md       # Quick start guide
├── installation.md          # Install methods
├── configuration.md         # Config file reference
├── commands/
│   ├── analyze.md
│   ├── apply.md
│   ├── diagnose.md
│   └── workflow.md
├── migration-guides/
│   ├── 7-to-8.md
│   └── 8-to-9.md
└── troubleshooting.md
```

**CLI help:**
```bash
$ bazelbump --help
bazelbump - Bazel version upgrade assistant

USAGE:
    bazelbump <command> [options]

COMMANDS:
    analyze     Analyze project for upgrade issues
    apply       Apply transformations
    diagnose    Diagnose build errors (LLM-assisted)
    explain     Explain flags or APIs
    snapshot    Manage snapshots
    workflow    Manage workflows
    init        Initialize bazelbump in project
    version     Show version

GLOBAL OPTIONS:
    -v, --verbose    Verbose output
    --config FILE    Config file (default: .bazelbump/config.yaml)
    --no-color       Disable colored output

Run 'bazelbump <command> --help' for command-specific help.
```

### 5. Release Packaging

**Makefile:**
```makefile
VERSION := $(shell git describe --tags --always --dirty)
LDFLAGS := -ldflags "-X main.version=$(VERSION)"

.PHONY: build release

build:
	go build $(LDFLAGS) -o bin/bazelbump ./cmd/bazelbump

release:
	# Linux
	GOOS=linux GOARCH=amd64 go build $(LDFLAGS) -o dist/bazelbump-linux-amd64
	GOOS=linux GOARCH=arm64 go build $(LDFLAGS) -o dist/bazelbump-linux-arm64

	# macOS
	GOOS=darwin GOARCH=amd64 go build $(LDFLAGS) -o dist/bazelbump-darwin-amd64
	GOOS=darwin GOARCH=arm64 go build $(LDFLAGS) -o dist/bazelbump-darwin-arm64

	# Windows
	GOOS=windows GOARCH=amd64 go build $(LDFLAGS) -o dist/bazelbump-windows-amd64.exe

	# Checksums
	cd dist && sha256sum * > checksums.txt
```

**GitHub Actions:**
```yaml
# .github/workflows/release.yml
name: Release
on:
  push:
    tags: ['v*']

jobs:
  release:
    runs-on: ubuntu-latest
    steps:
      - uses: actions/checkout@v4
      - uses: actions/setup-go@v5
        with:
          go-version: '1.22'

      - name: Build
        run: make release

      - name: Create Release
        uses: softprops/action-gh-release@v1
        with:
          files: dist/*
```

### 6. Configuration Validation

```go
// pkg/config/validate.go
func Validate(cfg *Config) error {
    var errs []error

    // LLM config
    if cfg.LLM.Provider == "bedrock" {
        if cfg.LLM.Bedrock.Region == "" {
            errs = append(errs, errors.New("bedrock.region required"))
        }
    }

    // Workflow config
    if cfg.Workflow.Engine == "dbos" {
        if cfg.Workflow.DBOS.DatabaseURL == "" && !cfg.Workflow.DBOS.Embedded {
            errs = append(errs, errors.New("dbos.database_url required (or set embedded: true)"))
        }
    }

    if len(errs) > 0 {
        return &ConfigValidationError{Errors: errs}
    }
    return nil
}
```

### 7. Telemetry (Opt-in)

```go
// pkg/telemetry/telemetry.go
package telemetry

type Event struct {
    Name       string
    Properties map[string]any
    Timestamp  time.Time
}

type Client struct {
    enabled  bool
    endpoint string
    buffer   []Event
}

func (c *Client) Track(name string, props map[string]any) {
    if !c.enabled {
        return
    }
    // Buffer and batch send
}

// Tracked events (anonymous, no PII):
// - command_executed: {command, duration, success}
// - analysis_completed: {flags_found, starlark_found, duration}
// - transform_applied: {count, duration}
// - error_occurred: {error_code}
```

**Privacy-first:**
```yaml
# .bazelbump/config.yaml
telemetry:
  enabled: false  # Default off
  # When enabled, sends anonymous usage statistics
  # No file paths, no code, no PII
```

## Package Structure

```
pkg/
├── errors/
│   ├── errors.go
│   └── catalog.go
├── observability/
│   ├── logging.go
│   └── metrics.go
├── config/
│   └── validate.go
└── telemetry/
    └── telemetry.go

docs/
├── getting-started.md
├── installation.md
├── configuration.md
├── commands/
└── migration-guides/

tests/
├── integration/
└── e2e/
```

## Tasks

- [ ] Define error catalog with codes
- [ ] Implement structured logging
- [ ] Add --verbose flag globally
- [ ] Write unit tests (target: 80% coverage)
- [ ] Write integration tests for migrations
- [ ] Create golden test fixtures
- [ ] Write getting-started guide
- [ ] Write command reference docs
- [ ] Write migration guides (7→8, 8→9)
- [ ] Create troubleshooting guide
- [ ] Setup release automation
- [ ] Add version command
- [ ] Validate config on load
- [ ] Implement opt-in telemetry
- [ ] Create homebrew formula
- [ ] Test on Linux, macOS, Windows

## Acceptance Criteria

1. All errors have user-friendly messages with remediation
2. Verbose mode shows detailed progress
3. Test coverage >= 80%
4. Documentation covers all commands
5. Single binary works on Linux/macOS/Windows
6. Release process automated
7. Config validation catches common mistakes

## Configuration Reference

```yaml
# .bazelbump/config.yaml - Full reference

# Workflow engine
workflow:
  engine: simple  # noop, simple, dbos
  simple:
    database: ~/.local/share/bazelbump/workflows.db
  dbos:
    database_url: postgres://localhost/bazelbump
    embedded: false

# LLM provider
llm:
  provider: bedrock  # bedrock, openai, none
  bedrock:
    region: us-east-1
    model: anthropic.claude-3-5-sonnet-20241022-v2:0
  fallback:
    - provider: openai
      endpoint: http://localhost:11434/v1
      model: qwen2.5-coder:14b
  defaults:
    max_tokens: 4096
    temperature: 0.1
    timeout: 60s

# Safety features
safety:
  snapshots:
    enabled: true
    max_count: 10
    location: ~/.local/share/bazelbump/snapshots
  preflight:
    enabled: true
    fail_on_warnings: false

# Output
output:
  format: text  # text, json
  color: auto   # auto, always, never

# Telemetry (anonymous, opt-in)
telemetry:
  enabled: false
```

## Notes

- Error messages are critical for adoption
- Documentation should include real examples
- Test on actual Bazel projects (not just synthetic)
- Consider shell completion scripts
