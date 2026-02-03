# ADR-002: Workflow Engine Strategy (SQLite Default, DBOS Optional)

**Status:** Accepted (Updated)
**Date:** 2025-02-03
**Updated:** 2025-02-03
**Deciders:** Architecture Review

## Context

bazelbump needs durable execution capabilities for:

1. **Long-running migrations** - Large monorepos may take hours to migrate
2. **Crash recovery** - Resume from last completed step after failures
3. **Human-in-loop** - Pause for approval before applying changes
4. **LLM reliability** - Retry and checkpoint around unreliable LLM calls

However, we also have strong constraints:
- **Airgap-first**: Must work without external services
- **Zero dependencies**: No Docker or Postgres requirement for basic usage
- **Single binary**: Simplest possible deployment

We evaluated multiple approaches:

| Framework | Architecture | Complexity | Airgap Fit | External Deps |
|-----------|--------------|------------|------------|---------------|
| SQLite simple | Embedded library | **Lowest** | **Best** | **None** |
| DBOS | Embedded library + Postgres | Low | Good | PostgreSQL |
| Temporal | External server cluster | High | Medium | Many |
| Restate | Lightweight server | Medium | Good | Restate server |

## Decision

**Primary (Default): SQLite-based simple workflow engine**
- Zero external dependencies
- Pure Go SQLite (modernc.org/sqlite)
- Local state persistence
- Sufficient for most use cases

**Optional (Opt-in): DBOS for enterprise/advanced use cases**
- Full durability guarantees
- Requires PostgreSQL
- Multi-machine coordination
- Long-running workflow support

Design with a `WorkflowEngine` interface to support both engines and future adapters.

## Rationale

### Why DBOS?

1. **Simplest airgap deployment**
   - Only requires PostgreSQL (common in enterprise)
   - No additional servers to deploy/manage
   - Single binary deployment

2. **Minimal code changes**
   - Library approach vs. architectural rearchitecture
   - Uses Go's native `context.Context`
   - Just annotate existing functions as workflows/steps

3. **Go-native design**
   - Built specifically for Go idioms
   - Generic type safety
   - Familiar patterns

4. **Active development**
   - CockroachDB support added January 2026
   - Regular releases and improvements
   - Responsive maintainers

### Code Example

```go
// With DBOS - minimal changes to existing code
func migrationWorkflow(ctx dbos.DBOSContext, request MigrationRequest) (*MigrationResult, error) {
    // Step 1: Analyze (checkpointed)
    analysis, err := dbos.RunAsStep(ctx, func(ctx context.Context) (*Analysis, error) {
        return analyzer.Analyze(ctx, request.ProjectPath, request.TargetVersion)
    })
    if err != nil {
        return nil, err
    }

    // Step 2: Generate patches with LLM (checkpointed, can retry)
    patches, err := dbos.RunAsStep(ctx, func(ctx context.Context) ([]*Patch, error) {
        return agent.GeneratePatches(ctx, analysis)
    })
    if err != nil {
        return nil, err
    }

    // Step 3: Wait for human approval (durable wait)
    approved, err := dbos.Recv[bool](ctx, "approval", 7*24*time.Hour)
    if err != nil || !approved {
        return nil, fmt.Errorf("migration not approved")
    }

    // Step 4: Apply changes (checkpointed)
    return dbos.RunAsStep(ctx, func(ctx context.Context) (*MigrationResult, error) {
        return transformer.Apply(ctx, patches)
    })
}
```

### Comparison with Temporal

| Aspect | DBOS | Temporal |
|--------|------|----------|
| Setup | `go get` + Postgres | Server cluster + workers |
| Code changes | Add annotations | Split into client/worker/activities |
| Deployment | Single binary | 3+ services |
| Dependencies | PostgreSQL | Cassandra/Postgres + Temporal server |
| Learning curve | Low | Medium-High |

## Interface Abstraction

To support future flexibility, we define:

```go
// pkg/workflow/engine.go
type Engine interface {
    // RunWorkflow starts a durable workflow
    RunWorkflow(ctx context.Context, workflow any, input any, opts ...Option) (Handle, error)

    // RunStep executes a checkpointed step within a workflow
    RunStep(ctx context.Context, step func(context.Context) (any, error)) (any, error)

    // Sleep pauses workflow durably
    Sleep(ctx context.Context, duration time.Duration) error

    // Send sends a message to a workflow
    Send(ctx context.Context, workflowID string, message any, topic string) error

    // Recv waits for a message (with timeout)
    Recv(ctx context.Context, topic string, timeout time.Duration) (any, error)
}

// Implementations (in order of preference)
type NoOpEngine struct { ... }      // Tier 0: Synchronous, no persistence
type SimpleEngine struct { ... }    // Tier 1: SQLite-based (DEFAULT)
type DBOSEngine struct { ... }      // Tier 2: Full durability (opt-in)
type TemporalEngine struct { ... }  // Future: For Temporal users
type RestateEngine struct { ... }   // Future: For Restate users
```

## Consequences

### Positive

- **Zero friction**: Download binary, run immediately
- **True airgap**: No external services required
- **Progressive complexity**: Start simple, opt-in to DBOS when needed
- **Interface abstraction**: Can swap engines without code changes
- Minimal learning curve for Go developers

### Negative

- SQLite engine has weaker durability guarantees than DBOS
- No multi-machine coordination in default mode
- DBOS (when used) requires Postgres

### Mitigations

- SQLite sufficient for 90% of use cases (single-machine migrations)
- Background execution via `--background` flag + status polling
- DBOS available for enterprise users who need full durability
- Observability: Add structured logging + metrics for all engines

## Related Decisions

- ADR-007: No-Daemon Architecture with SQLite Default

## References

- modernc.org/sqlite: https://pkg.go.dev/modernc.org/sqlite (Pure Go SQLite)
- DBOS Go SDK: https://github.com/dbos-inc/dbos-transact-golang
- DBOS Docs: https://docs.dbos.dev/golang/programming-guide
- DBOS vs Temporal: https://www.dbos.dev/blog/durable-execution-coding-comparison
- Microsoft durabletask-go: https://github.com/microsoft/durabletask-go (alternative reference)
