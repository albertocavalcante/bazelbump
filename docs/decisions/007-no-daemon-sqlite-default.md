# ADR-007: No-Daemon Architecture with SQLite Default

**Status:** Accepted
**Date:** 2025-02-03
**Deciders:** Architecture Review

## Context

We need to decide on the deployment architecture for bazelbump:
1. **Daemon/Client model**: Long-running background service handling requests
2. **Control plane model**: Centralized server managing distributed workers
3. **CLI-first model**: Direct execution with optional persistence

The user has expressed strong preferences:
- Airgap-first (no external services required)
- No Docker or Postgres requirement for basic usage
- OSS-only dependencies
- Single-binary deployment ideal

## Decision

**Primary: CLI-first with SQLite persistence (no daemon required)**

bazelbump will operate as a standalone CLI tool by default:
- All operations execute directly in the CLI process
- State persisted to local SQLite database
- No background services required
- Single binary distribution

**Optional: DBOS mode for enterprise/advanced use cases**

For users who need full durability and workflow orchestration:
- Opt-in via configuration
- Requires PostgreSQL (can use embedded-postgres for dev)
- Provides resumable long-running workflows
- Enables multi-machine coordination

## Architecture Comparison

### Option A: Daemon/Client (Rejected for Default)

```
┌─────────────────┐     ┌─────────────────────────────┐
│  bazelbump CLI  │────▶│  bazelbump daemon (gRPC)    │
│   (client)      │     │  - State management         │
└─────────────────┘     │  - Background tasks         │
                        │  - LLM connection pool      │
                        └─────────────────────────────┘
```

**Pros:**
- Shared connection pool to LLM providers
- Background task execution
- Persistent state across CLI invocations

**Cons:**
- Requires starting/managing daemon process
- More complex deployment
- State management complexity
- Not airgap-friendly (daemon needs network)

### Option B: Control Plane (Rejected)

```
┌─────────────────┐     ┌─────────────────────────────┐
│  bazelbump CLI  │────▶│  Control Plane Server       │
│                 │     │  - Postgres                 │
└─────────────────┘     │  - Task queue               │
                        │  - Multi-tenant             │
                        └─────────────────────────────┘
```

**Pros:**
- Centralized management
- Multi-project coordination
- Team collaboration features

**Cons:**
- Requires infrastructure
- Not suitable for airgap
- Overkill for single-developer use case
- Operational complexity

### Option C: CLI-First with SQLite (Selected)

```
┌───────────────────────────────────────────────────────┐
│                    bazelbump CLI                       │
│  ┌─────────────┐  ┌─────────────┐  ┌───────────────┐  │
│  │  Commands   │  │  Workflow   │  │  LLM Provider │  │
│  │  analyze    │  │  Engine     │  │  (on-demand)  │  │
│  │  apply      │  │  (SQLite)   │  │               │  │
│  │  migrate    │  │             │  │               │  │
│  └─────────────┘  └─────────────┘  └───────────────┘  │
│                          │                             │
│                    ┌─────▼─────┐                       │
│                    │  SQLite   │                       │
│                    │  ~/.local/│                       │
│                    │  share/   │                       │
│                    │  bazelbump│                       │
│                    └───────────┘                       │
└───────────────────────────────────────────────────────┘
```

**Pros:**
- Zero external dependencies
- Single binary distribution
- Works offline/airgap
- Simple mental model
- Fast startup (no daemon connection)

**Cons:**
- No shared LLM connection pool (acceptable)
- Long-running tasks tied to terminal (solved with nohup/background)
- No built-in multi-machine coordination (rare use case)

## Implementation

### Storage Tiers

```go
// pkg/workflow/engine.go
type Engine interface {
    RunWorkflow(ctx context.Context, name string, fn WorkflowFunc, input any) (Handle, error)
    RunStep(ctx context.Context, name string, fn StepFunc) (any, error)
    Sleep(ctx context.Context, duration time.Duration) error
    WaitForApproval(ctx context.Context, prompt string, timeout time.Duration) (bool, error)
}

// Tier 1: No persistence (default for quick commands)
// pkg/workflow/noop/engine.go
type NoOpEngine struct{}

// Tier 2: SQLite persistence (default for workflows)
// pkg/workflow/simple/engine.go
type SimpleEngine struct {
    db *sql.DB  // SQLite via modernc.org/sqlite (pure Go)
}

// Tier 3: Full durability (opt-in, requires Postgres)
// pkg/workflow/dbos/engine.go
type DBOSEngine struct {
    ctx dbos.DBOSContext
}
```

### SQLite Library Selection

**Selected: modernc.org/sqlite**
- Pure Go implementation (CGO-free)
- Cross-platform without C compiler
- Single binary deployment
- Active maintenance

**Alternative considered: mattn/go-sqlite3**
- Requires CGO
- Better performance
- More complex cross-compilation

### State Location

```
~/.local/share/bazelbump/
├── workflows.db          # SQLite: workflow state
├── snapshots/            # File snapshots for rollback
│   └── snap_01HQ5X/
├── cache/                # Cached analysis results
│   ├── flags.db
│   └── registry/
└── config/               # User configuration
```

### Configuration

```yaml
# .bazelbump/config.yaml
workflow:
  # Engine selection: noop | simple | dbos
  engine: simple  # Default: SQLite-based

  # Simple engine (SQLite)
  simple:
    database: ~/.local/share/bazelbump/workflows.db

  # DBOS engine (opt-in for full durability)
  dbos:
    database_url: postgres://localhost:5432/bazelbump
    # Or use embedded postgres for local dev:
    # embedded: true
```

### Background Execution (Without Daemon)

For long-running migrations without a daemon:

```bash
# Option 1: Shell background
$ bazelbump migrate --to 9.0.0 &
[1] 12345
$ bazelbump workflow status wf_01HQ7A

# Option 2: nohup for disconnect-safe
$ nohup bazelbump migrate --to 9.0.0 > migrate.log 2>&1 &

# Option 3: Built-in background mode
$ bazelbump migrate --to 9.0.0 --background
Migration started: wf_01HQ7A
Use 'bazelbump workflow status wf_01HQ7A' to check progress

# Resume after crash/restart (SQLite persists state)
$ bazelbump workflow resume wf_01HQ7A
Resuming from step 3/5: apply
```

### Embedded Postgres (Optional Future Enhancement)

For users who want DBOS durability without installing Postgres:

```go
// pkg/workflow/dbos/embedded.go
import "github.com/fergusstrange/embedded-postgres"

func StartEmbeddedPostgres(dataDir string) (*embeddedpostgres.EmbeddedPostgres, error) {
    postgres := embeddedpostgres.NewDatabase(
        embeddedpostgres.DefaultConfig().
            DataPath(dataDir).
            Port(15432),
    )
    return postgres, postgres.Start()
}
```

Note: embedded-postgres downloads PostgreSQL binaries on first run (~50MB), so not truly embedded but useful for dev/testing.

## Consequences

### Positive

1. **Zero-friction installation**: Download binary, run immediately
2. **Airgap compatible**: No network required for core functionality
3. **Simple operations**: No services to manage
4. **Portable state**: Copy SQLite file to move state
5. **Debuggable**: Standard SQLite, can inspect with any tool

### Negative

1. **No real-time coordination**: Can't coordinate multiple machines (rare need)
2. **Manual background handling**: User manages long-running processes
3. **Single-machine state**: Workflow state local to machine

### Mitigations

1. For multi-machine: Document manual coordination patterns
2. For background: Provide `--background` flag with status polling
3. For shared state: DBOS mode available for teams needing it

## Migration Path

1. **v1.0**: SQLite-only, simple engine
2. **v1.x**: Add DBOS engine as opt-in
3. **Future**: Consider daemon mode if user demand exists

## Related Decisions

- ADR-001: Airgap-First Architecture
- ADR-002: DBOS as Optional Workflow Engine (updated to "optional")
- M7: Workflow Integration milestone

## References

- [modernc.org/sqlite](https://pkg.go.dev/modernc.org/sqlite) - Pure Go SQLite
- [Microsoft durabletask-go](https://github.com/microsoft/durabletask-go) - SQLite-based durable tasks
- [embedded-postgres](https://github.com/fergusstrange/embedded-postgres) - Embedded PostgreSQL for Go
- [XDG Base Directory](https://specifications.freedesktop.org/basedir-spec/basedir-spec-latest.html) - State location spec
