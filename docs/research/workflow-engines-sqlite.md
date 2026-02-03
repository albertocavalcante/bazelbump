# Workflow Engines with SQLite Support

**Date:** 2025-02-03
**Purpose:** Research lightweight workflow/durable execution engines that work without PostgreSQL

## Requirements

1. **Zero external dependencies** - No Docker, no Postgres
2. **Pure Go** - Ideally CGO-free for easy cross-compilation
3. **Durable state** - Survive process restarts
4. **Checkpointing** - Resume from last completed step
5. **Simple API** - Minimal learning curve

## Evaluated Options

### 1. Microsoft durabletask-go

**Repository:** https://github.com/microsoft/durabletask-go

**Description:** Lightweight, embeddable engine for writing durable, fault-tolerant business logic (orchestrations). Developed by Microsoft Azure.

**SQLite Support:** Yes, built-in

```go
import (
    "github.com/microsoft/durabletask-go/backend"
    "github.com/microsoft/durabletask-go/backend/sqlite"
)

// Create SQLite backend
be := sqlite.NewSqliteBackend(sqlite.NewSqliteOptions("./workflows.db"))

// Create task hub worker
worker := backend.NewTaskHubWorker(be)

// Register orchestrations and activities
worker.AddOrchestratorN("migration", migrationOrchestration)
worker.AddActivityN("analyze", analyzeActivity)

// Start worker
worker.Start(ctx)
```

**Pros:**
- Microsoft-backed, production-ready
- SQLite backend included
- Similar model to Temporal/Durable Functions
- Well-documented
- Supports orchestration patterns (fan-out/fan-in, sub-orchestrations)

**Cons:**
- More complex API than DBOS
- Requires separating orchestrations from activities
- Smaller community than Temporal

**Verdict:** Strong candidate for SQLite-based workflow engine

---

### 2. Custom SQLite Implementation

**Approach:** Build minimal workflow engine using pure Go SQLite

```go
// Simple schema
CREATE TABLE workflows (
    id TEXT PRIMARY KEY,
    name TEXT NOT NULL,
    status TEXT NOT NULL,  -- pending, running, completed, failed
    input BLOB,
    output BLOB,
    created_at TIMESTAMP,
    updated_at TIMESTAMP
);

CREATE TABLE steps (
    id TEXT PRIMARY KEY,
    workflow_id TEXT REFERENCES workflows(id),
    name TEXT NOT NULL,
    status TEXT NOT NULL,
    result BLOB,
    error TEXT,
    started_at TIMESTAMP,
    completed_at TIMESTAMP
);
```

```go
// pkg/workflow/simple/engine.go
type Engine struct {
    db *sql.DB
}

func (e *Engine) RunStep(ctx context.Context, name string, fn StepFunc) (any, error) {
    // Check if step already completed
    var result []byte
    err := e.db.QueryRowContext(ctx,
        "SELECT result FROM steps WHERE workflow_id = ? AND name = ?",
        workflowID(ctx), name,
    ).Scan(&result)

    if err == nil {
        // Already completed, return cached result
        return unmarshal(result)
    }

    // Execute step
    out, err := fn(ctx)
    if err != nil {
        e.db.ExecContext(ctx,
            "INSERT INTO steps (id, workflow_id, name, status, error) VALUES (?, ?, ?, 'failed', ?)",
            newID(), workflowID(ctx), name, err.Error(),
        )
        return nil, err
    }

    // Checkpoint result
    e.db.ExecContext(ctx,
        "INSERT INTO steps (id, workflow_id, name, status, result) VALUES (?, ?, ?, 'completed', ?)",
        newID(), workflowID(ctx), name, marshal(out),
    )
    return out, nil
}
```

**Pros:**
- Complete control over implementation
- Minimal dependencies
- Tailored to our exact needs
- Educational value

**Cons:**
- More work to implement correctly
- Edge cases (timeouts, retries, etc.)
- Testing burden

**Verdict:** Selected for v1.0 - simple enough for our needs

---

### 3. Watermill

**Repository:** https://github.com/ThreeDotsLabs/watermill

**Description:** Message-based processing library with multiple backends including SQLite.

```go
import (
    "github.com/ThreeDotsLabs/watermill-sql/pkg/sql"
)

// SQLite pub/sub
publisher, _ := sql.NewPublisher(db, sql.PublisherConfig{
    SchemaAdapter:        sql.DefaultSQLiteSchema{},
    AutoInitializeSchema: true,
})

subscriber, _ := sql.NewSubscriber(db, sql.SubscriberConfig{
    SchemaAdapter:    sql.DefaultSQLiteSchema{},
    OffsetsAdapter:   sql.DefaultSQLiteOffsetsAdapter{},
})
```

**Pros:**
- Mature, well-tested
- Multiple backends (Kafka, Redis, SQL)
- Good for event-driven architectures

**Cons:**
- Message-oriented, not workflow-oriented
- Would need to build workflow semantics on top
- Overkill for our use case

**Verdict:** Not suitable - too message-focused

---

### 4. Embedded PostgreSQL

**Repository:** https://github.com/fergusstrange/embedded-postgres

**Description:** Runs PostgreSQL binaries embedded in Go application.

```go
postgres := embeddedpostgres.NewDatabase(
    embeddedpostgres.DefaultConfig().
        DataPath("/tmp/bazelbump-pg").
        Port(15432),
)
postgres.Start()
defer postgres.Stop()

// Now connect to postgres://localhost:15432/postgres
```

**Important Notes:**
- Not truly embedded - downloads PostgreSQL binaries (~50MB) on first run
- Binaries cached locally
- Works offline after first download
- Cross-platform (Linux, macOS, Windows)

**Pros:**
- Full PostgreSQL compatibility
- Can use DBOS with embedded Postgres
- No user-visible Postgres installation

**Cons:**
- 50MB binary download on first run
- Not truly single-binary
- Complexity of managing Postgres lifecycle
- Resource usage (memory, disk)

**Verdict:** Useful for DBOS opt-in mode, not for default

---

## SQLite Libraries for Go

### modernc.org/sqlite (Selected)

**Pure Go, CGO-free**

```go
import (
    "database/sql"
    _ "modernc.org/sqlite"
)

db, _ := sql.Open("sqlite", "file:workflows.db?_journal_mode=WAL")
```

**Pros:**
- No CGO required
- Cross-compiles easily
- Single binary deployment
- WAL mode for better concurrency
- Active maintenance

**Cons:**
- Slower than CGO version (~30%)
- Larger binary size

**Selected for:** Default SQLite engine

---

### mattn/go-sqlite3

**CGO-based, higher performance**

```go
import (
    "database/sql"
    _ "github.com/mattn/go-sqlite3"
)
```

**Pros:**
- Better performance
- Most widely used

**Cons:**
- Requires CGO
- Cross-compilation harder
- Needs C compiler

**Not selected:** CGO requirement conflicts with single-binary goal

---

## Recommendation

### Default (v1.0): Custom SQLite Implementation

Use a simple, custom workflow engine built on modernc.org/sqlite:

1. **Minimal complexity** - Just what we need
2. **Zero dependencies** - Pure Go
3. **Single binary** - Easy distribution
4. **Sufficient durability** - Survives process restart

### Optional (v1.x): DBOS with Embedded Postgres

For users needing full durability:

1. **Opt-in configuration** - `workflow.engine: dbos`
2. **Embedded Postgres option** - No user-visible installation
3. **Full DBOS guarantees** - True exactly-once semantics

### Implementation Plan

```go
// pkg/workflow/factory.go
func New(cfg *Config) (Engine, error) {
    switch cfg.Engine {
    case "noop", "":
        return noop.New(), nil
    case "simple":
        return simple.New(cfg.Simple.Database)  // SQLite
    case "dbos":
        if cfg.DBOS.Embedded {
            startEmbeddedPostgres(cfg.DBOS.DataDir)
        }
        return dbos.New(cfg.DBOS.DatabaseURL)
    default:
        return nil, fmt.Errorf("unknown engine: %s", cfg.Engine)
    }
}
```

## References

- Microsoft durabletask-go: https://github.com/microsoft/durabletask-go
- Watermill: https://github.com/ThreeDotsLabs/watermill
- embedded-postgres: https://github.com/fergusstrange/embedded-postgres
- modernc.org/sqlite: https://pkg.go.dev/modernc.org/sqlite
- SQLite WAL mode: https://www.sqlite.org/wal.html
