# Milestone 0: Foundation

**Target:** Week 1
**Status:** Not Started

## Objective

Set up the project infrastructure, proto definitions, and build system.

## Deliverables

### 1. Project Structure

```
bazelbump/
├── proto/bazelbump/v1/          # Proto definitions
├── gen/go/bazelbump/v1/         # Generated Go code
├── pkg/                          # Public packages (stubs)
├── cmd/bazelbump/               # CLI entry point
├── internal/                     # Internal packages (stubs)
├── third_party/buildtools/      # Vendored buildtools
├── data/                         # Placeholder for embedded data
├── docs/                         # Documentation
├── buf.yaml                      # Buf configuration
├── buf.gen.yaml                  # Code generation config
├── go.mod                        # Go module
├── Makefile                      # Build targets
└── .github/workflows/           # CI/CD
```

### 2. Proto Definitions

Core protos to define:

```protobuf
// proto/bazelbump/v1/

// flag.proto - Flag deprecation types
message FlagChange { ... }
message FlagDatabase { ... }

// analysis.proto - Analysis request/response
message AnalyzeRequest { ... }
message AnalyzeResponse { ... }
message Issue { ... }

// transform.proto - Transformation types
message Transform { ... }
message Patch { ... }

// version.proto - Version handling
message BazelVersion { ... }
message VersionConstraint { ... }

// bazelrc.proto - .bazelrc representation
message BazelRC { ... }
message RCLine { ... }

// module.proto - MODULE.bazel types
message ModuleFile { ... }
message Dependency { ... }

// starlark.proto - Starlark deprecations
message StarlarkAPIChange { ... }
message StarlarkUsage { ... }

// snapshot.proto - Snapshot/rollback
message Snapshot { ... }
message SnapshotManifest { ... }

// preflight.proto - Pre-flight checks
message PreflightCheck { ... }
message PreflightResult { ... }

// score.proto - Readiness scoring
message ReadinessScore { ... }
message ScoreCategory { ... }

// workflow.proto - Workflow state
message WorkflowState { ... }
message WorkflowStep { ... }

// llm.proto - LLM request/response
message CompletionRequest { ... }
message CompletionResponse { ... }
```

### 3. Build System

**Makefile targets:**
```makefile
.PHONY: all generate build test lint

all: generate build

generate:
    buf generate

build: generate
    go build -o bin/bazelbump ./cmd/bazelbump

test: generate
    go test ./...

lint:
    buf lint
    golangci-lint run

clean:
    rm -rf bin/ gen/
```

**CI/CD (.github/workflows/ci.yml):**
- Run on push/PR
- buf lint
- buf generate
- go build
- go test
- golangci-lint

### 4. External Dependencies

**go.mod:**
```go
module github.com/albertocavalcante/bazelbump

go 1.23

require (
    // Core
    google.golang.org/protobuf v1.36.3
    github.com/spf13/cobra v1.10.0

    // External libraries (go-bzlmod, DBOS) - add when used
    // github.com/albertocavalcante/go-bzlmod v0.x.x
    // github.com/dbos-inc/dbos-transact-golang v0.x.x
)
```

**Vendored:**
- buildtools (from bazelbuild/buildtools)

### 5. CLI Skeleton

```go
// cmd/bazelbump/main.go
func main() {
    rootCmd := &cobra.Command{
        Use:   "bazelbump",
        Short: "Bazel version upgrade assistant",
    }

    rootCmd.AddCommand(
        newAnalyzeCmd(),
        newApplyCmd(),
        newDiagnoseCmd(),
        newDataCmd(),
        newVersionCmd(),
    )

    rootCmd.Execute()
}
```

## Tasks

- [ ] Create directory structure
- [ ] Initialize go.mod
- [ ] Set up buf.yaml and buf.gen.yaml
- [ ] Define all core proto files
- [ ] Generate Go code from protos
- [ ] Vendor buildtools
- [ ] Create Makefile
- [ ] Set up GitHub Actions CI
- [ ] Create CLI skeleton with cobra
- [ ] Add basic --version command
- [ ] Write README.md with setup instructions

## Acceptance Criteria

1. `make generate` successfully generates Go code from protos
2. `make build` produces a working binary
3. `bazelbump --version` works
4. `bazelbump --help` shows available commands
5. CI pipeline passes
6. All proto files compile without errors

## Dependencies

- None (this is the foundation)

## Notes

- Keep proto definitions minimal but complete
- Focus on structure, not implementation
- Ensure buf lint passes
- Add comments to all proto messages
