# bazelbump Architecture Overview

**Version:** 1.0 (Draft)
**Date:** 2025-02-03

## High-Level Architecture

```
┌─────────────────────────────────────────────────────────────────────────────┐
│                          BAZELBUMP ARCHITECTURE                              │
├─────────────────────────────────────────────────────────────────────────────┤
│                                                                              │
│                           ┌─────────────────────┐                           │
│                           │     bazelbump CLI   │                           │
│                           │  (cmd/bazelbump)    │                           │
│                           └──────────┬──────────┘                           │
│                                      │                                       │
│              ┌───────────────────────┼───────────────────────┐              │
│              │                       │                       │              │
│              ▼                       ▼                       ▼              │
│    ┌─────────────────┐    ┌─────────────────┐    ┌─────────────────┐       │
│    │   libbazelbump  │    │   Agent Engine  │    │ Workflow Engine │       │
│    │ (pkg/libbazelbump)   │ (pkg/agent)     │    │ (pkg/workflow)  │       │
│    │                 │    │                 │    │                 │       │
│    │  • Analyze      │◀──▶│  • Diagnose     │◀──▶│  • DBOS         │       │
│    │  • Transform    │    │  • Patch        │    │  • (Temporal)   │       │
│    │  • Apply        │    │  • Explain      │    │  • (Restate)    │       │
│    │  • Score        │    │  • Document     │    │  • NoOp         │       │
│    └────────┬────────┘    └────────┬────────┘    └────────┬────────┘       │
│             │                      │                      │                 │
│             │                      ▼                      │                 │
│             │             ┌─────────────────┐             │                 │
│             │             │   LLM Provider  │             │                 │
│             │             │   (pkg/llm)     │             │                 │
│             │             └────────┬────────┘             │                 │
│             │                      │                      │                 │
│             │        ┌─────────────┼─────────────┐        │                 │
│             │        │             │             │        │                 │
│             │        ▼             ▼             ▼        │                 │
│             │   ┌────────┐   ┌────────┐   ┌────────┐     │                 │
│             │   │Bedrock │   │OpenAI- │   │  NoOp  │     │                 │
│             │   │        │   │compat  │   │        │     │                 │
│             │   └────────┘   └────────┘   └────────┘     │                 │
│             │                                             │                 │
│             ▼                                             ▼                 │
│    ┌─────────────────────────────────────────────────────────────┐         │
│    │                      INTERNAL PACKAGES                       │         │
│    │                                                              │         │
│    │  ┌──────────┐ ┌──────────┐ ┌──────────┐ ┌──────────┐       │         │
│    │  │ bazelrc  │ │ starlark │ │  flagdb  │ │transform │       │         │
│    │  │ parser   │ │ scanner  │ │          │ │  engine  │       │         │
│    │  └──────────┘ └──────────┘ └──────────┘ └──────────┘       │         │
│    │                                                              │         │
│    │  ┌──────────┐ ┌──────────┐ ┌──────────┐ ┌──────────┐       │         │
│    │  │ snapshot │ │preflight │ │  scorer  │ │ datasync │       │         │
│    │  └──────────┘ └──────────┘ └──────────┘ └──────────┘       │         │
│    │                                                              │         │
│    └─────────────────────────────────────────────────────────────┘         │
│                                                                              │
│    ┌─────────────────────────────────────────────────────────────┐         │
│    │                      EXTERNAL DEPS                           │         │
│    │                                                              │         │
│    │  ┌──────────────┐    ┌──────────────┐    ┌──────────────┐  │         │
│    │  │  go-bzlmod   │    │  buildtools  │    │     DBOS     │  │         │
│    │  │ (Go module)  │    │  (vendored)  │    │  (Go module) │  │         │
│    │  └──────────────┘    └──────────────┘    └──────────────┘  │         │
│    │                                                              │         │
│    └─────────────────────────────────────────────────────────────┘         │
│                                                                              │
│    ┌─────────────────────────────────────────────────────────────┐         │
│    │                      DATA LAYER                              │         │
│    │                                                              │         │
│    │  ┌──────────┐ ┌──────────┐ ┌──────────┐ ┌──────────┐       │         │
│    │  │ Embedded │ │ External │ │PostgreSQL│ │  Cache   │       │         │
│    │  │  (flags, │ │  (sync'd │ │  (DBOS)  │ │ (local)  │       │         │
│    │  │  bcr,..) │ │  data)   │ │          │ │          │       │         │
│    │  └──────────┘ └──────────┘ └──────────┘ └──────────┘       │         │
│    │                                                              │         │
│    └─────────────────────────────────────────────────────────────┘         │
│                                                                              │
└─────────────────────────────────────────────────────────────────────────────┘
```

## Package Structure

```
bazelbump/
├── proto/                           # PROTO-FIRST: All types defined here
│   └── bazelbump/v1/
│       ├── flag.proto
│       ├── analysis.proto
│       ├── transform.proto
│       ├── workflow.proto
│       ├── llm.proto
│       └── ...
│
├── gen/                             # Generated code
│   └── go/bazelbump/v1/
│       └── *.pb.go
│
├── pkg/                             # Public API
│   ├── libbazelbump/                # Main library
│   │   ├── client.go
│   │   ├── analyze.go
│   │   ├── apply.go
│   │   └── ...
│   │
│   ├── llm/                         # LLM provider interface
│   │   ├── provider.go
│   │   ├── bedrock/
│   │   └── openai/
│   │
│   ├── workflow/                    # Workflow engine interface
│   │   ├── engine.go
│   │   ├── dbos/
│   │   └── noop/
│   │
│   └── agent/                       # Agent capabilities
│       ├── diagnose.go
│       ├── patch.go
│       └── document.go
│
├── cmd/bazelbump/                   # CLI
│   ├── main.go
│   ├── analyze.go
│   ├── apply.go
│   ├── diagnose.go
│   └── ...
│
├── internal/                        # Private implementation
│   ├── bazelrc/                     # .bazelrc parser
│   ├── starlark/                    # Starlark scanner
│   ├── flagdb/                      # Flag database
│   ├── transform/                   # Transform engine
│   ├── snapshot/                    # Snapshot system
│   ├── preflight/                   # Pre-flight checks
│   ├── scorer/                      # Readiness scoring
│   └── datasync/                    # Data sync tooling
│
├── third_party/                     # Vendored deps
│   └── buildtools/
│
├── data/                            # Embedded data
│   ├── flags.pb
│   ├── bcr.pb
│   └── starlark.pb
│
├── docs/                            # Documentation
│   ├── decisions/                   # ADRs
│   ├── architecture/
│   ├── research/
│   └── milestones/
│
└── buf.yaml, buf.gen.yaml           # Protobuf tooling
```

## Key Design Principles

### 1. Airgap-First

All functionality works without network access:
- Data is embedded or pre-synced
- LLM is optional (graceful degradation)
- No runtime external API calls

### 2. Proto-First

Types defined in protobuf:
- Single source of truth
- Efficient serialization
- Forward compatibility

### 3. Library-First

All functionality in `pkg/libbazelbump`:
- CLI is thin wrapper
- Embeddable in other tools
- Testable without CLI

### 4. Interface-Based

Key components are interfaces:
- `llm.Provider` for LLM backends
- `workflow.Engine` for durable execution
- Easy to mock, easy to extend

## Data Flow

### Analysis Flow

```
User Request
    │
    ▼
┌─────────────────┐
│ bazelbump CLI   │
│ analyze --to X  │
└────────┬────────┘
         │
         ▼
┌─────────────────┐     ┌─────────────────┐
│ libbazelbump    │────▶│ Embedded Data   │
│ Analyze()       │     │ (flags, bcr)    │
└────────┬────────┘     └─────────────────┘
         │
    ┌────┴────┐
    │         │
    ▼         ▼
┌───────┐ ┌───────┐
│bazelrc│ │MODULE │
│parser │ │parser │
└───┬───┘ └───┬───┘
    │         │
    └────┬────┘
         │
         ▼
┌─────────────────┐
│ Issue Detection │
│ - flags         │
│ - starlark      │
│ - dependencies  │
└────────┬────────┘
         │
         ▼
┌─────────────────┐
│ Analysis Result │
│ (protobuf)      │
└─────────────────┘
```

### Agent Flow (with LLM)

```
Error Log
    │
    ▼
┌─────────────────┐
│ bazelbump CLI   │
│ diagnose        │
└────────┬────────┘
         │
         ▼
┌─────────────────┐
│ Agent Engine    │
│ Diagnose()      │
└────────┬────────┘
         │
    ┌────┴────┐
    │         │
    ▼         ▼
┌───────┐ ┌───────┐
│Pattern│ │  LLM  │
│Match  │ │Provider│
└───┬───┘ └───┬───┘
    │         │
    │    ┌────┴────┐
    │    │         │
    │    ▼         ▼
    │ ┌───────┐ ┌───────┐
    │ │Bedrock│ │Ollama │
    │ └───┬───┘ └───┬───┘
    │     │         │
    └─────┴────┬────┘
               │
               ▼
       ┌─────────────────┐
       │ Diagnosis Result│
       │ - root cause    │
       │ - suggested fix │
       │ - confidence    │
       └─────────────────┘
```

### Workflow Flow (with DBOS)

```
Migration Request
    │
    ▼
┌─────────────────┐
│ bazelbump CLI   │
│ migrate --to X  │
└────────┬────────┘
         │
         ▼
┌─────────────────┐     ┌─────────────────┐
│ Workflow Engine │────▶│   PostgreSQL    │
│ (DBOS)          │     │   (checkpoints) │
└────────┬────────┘     └─────────────────┘
         │
    ┌────┴────────────────────┐
    │                         │
    ▼                         │
┌───────────┐                 │
│ Step 1:   │                 │
│ Analyze   │ ◀── checkpoint  │
└─────┬─────┘                 │
      │                       │
      ▼                       │
┌───────────┐                 │
│ Step 2:   │                 │
│ Plan      │ ◀── checkpoint  │
└─────┬─────┘                 │
      │                       │
      ▼                       │
┌───────────┐                 │
│ Step 3:   │                 │
│ Approval  │ ◀── durable wait│
└─────┬─────┘                 │
      │ (human approves)      │
      ▼                       │
┌───────────┐                 │
│ Step 4:   │                 │
│ Apply     │ ◀── checkpoint  │
└─────┬─────┘                 │
      │                       │
      ▼                       │
┌─────────────────┐           │
│ Migration Done  │           │
└─────────────────┘           │
                              │
 If crash occurs anywhere ────┘
 → Resume from last checkpoint
```

## Configuration

```yaml
# .bazelbump/config.yaml
project:
  name: my-project

# LLM Configuration
llm:
  provider: bedrock
  bedrock:
    region: us-east-1
    model: anthropic.claude-3-5-sonnet-20241022-v2:0
  fallback:
    - provider: ollama
      endpoint: http://localhost:11434
      model: qwen2.5-coder:14b

# Workflow Configuration
workflow:
  engine: dbos
  dbos:
    database_url: postgres://localhost:5432/bazelbump

# Data Configuration
data:
  flags: embedded          # or path to external file
  bcr: embedded
  recipes: embedded

# Safety Configuration
safety:
  snapshots:
    enabled: true
    max_count: 10
  preflight:
    enabled: true
```

## External Dependencies

| Dependency | Purpose | Bundled |
|------------|---------|---------|
| go-bzlmod | MODULE.bazel parsing | Go module |
| buildtools | Starlark parsing | Vendored |
| DBOS | Durable workflows | Go module |
| AWS SDK v2 | Bedrock access | Go module |
| cobra | CLI framework | Go module |
| protobuf | Serialization | Go module |

## Next Steps

See [Milestones](../milestones/) for implementation plan.
