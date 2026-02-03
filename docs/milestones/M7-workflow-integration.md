# Milestone 7: Workflow Integration

**Target:** Weeks 12-13
**Status:** Not Started
**Depends On:** M6 (Agent Actions)

## Objective

Implement durable workflow support for long-running migrations with optional DBOS integration.

## Deliverables

### 1. Workflow Engine Interface

```go
// pkg/workflow/engine.go
type Engine interface {
    // RunWorkflow starts a durable workflow
    RunWorkflow(ctx context.Context, name string, fn WorkflowFunc, input any) (Handle, error)

    // RunStep executes a checkpointed step
    RunStep(ctx context.Context, name string, fn StepFunc) (any, error)

    // Sleep pauses durably
    Sleep(ctx context.Context, duration time.Duration) error

    // WaitForApproval waits for human approval
    WaitForApproval(ctx context.Context, prompt string, timeout time.Duration) (bool, error)

    // SendNotification sends progress update
    SendNotification(ctx context.Context, msg string) error
}

type Handle interface {
    ID() string
    Wait(ctx context.Context) (any, error)
    Status() WorkflowStatus
    Cancel() error
}
```

### 2. Simple Engine (SQLite, No External Deps)

```go
// pkg/workflow/simple/engine.go
package simple

import (
    "database/sql"
    _ "modernc.org/sqlite"
)

type Engine struct {
    db *sql.DB
}

func New(dbPath string) (*Engine, error) {
    db, err := sql.Open("sqlite", dbPath)
    if err != nil {
        return nil, err
    }
    if err := migrate(db); err != nil {
        return nil, err
    }
    return &Engine{db: db}, nil
}

func (e *Engine) RunWorkflow(ctx context.Context, name string, fn WorkflowFunc, input any) (Handle, error) {
    // 1. Create workflow record in SQLite
    // 2. Execute workflow function
    // 3. Checkpoint completed steps
    // 4. Return handle for status/wait
}

func (e *Engine) RunStep(ctx context.Context, name string, fn StepFunc) (any, error) {
    // 1. Check if step already completed (in SQLite)
    // 2. If yes, return cached result
    // 3. If no, execute and checkpoint
}
```

### 3. DBOS Engine (Optional, Full Durability)

```go
// pkg/workflow/dbos/engine.go
package dbosengine

import (
    "github.com/dbos-inc/dbos-transact-golang/dbos"
)

type Engine struct {
    ctx dbos.DBOSContext
}

func New(ctx context.Context, cfg *Config) (*Engine, error) {
    dbosCtx, err := dbos.NewDBOSContext(ctx, dbos.Config{
        DatabaseURL: cfg.DatabaseURL,
        AppName:     "bazelbump",
    })
    if err != nil {
        return nil, err
    }
    return &Engine{ctx: dbosCtx}, nil
}

func (e *Engine) RunWorkflow(ctx context.Context, name string, fn WorkflowFunc, input any) (Handle, error) {
    // Delegate to DBOS
}
```

### 4. No-Op Engine (CLI Mode)

```go
// pkg/workflow/noop/engine.go
type Engine struct{}

func (e *Engine) RunWorkflow(ctx context.Context, name string, fn WorkflowFunc, input any) (Handle, error) {
    // Just run synchronously, no durability
    result, err := fn(ctx, input)
    return &immediateHandle{result: result, err: err}, nil
}

func (e *Engine) RunStep(ctx context.Context, name string, fn StepFunc) (any, error) {
    // Just run, no checkpoint
    return fn(ctx)
}
```

### 5. Migration Workflow

```go
// pkg/libbazelbump/migrate.go
func (c *Client) Migrate(ctx context.Context, req *MigrateRequest) (*MigrateResponse, error) {
    return c.workflow.RunWorkflow(ctx, "migrate", func(ctx context.Context, input any) (any, error) {
        req := input.(*MigrateRequest)

        // Step 1: Analyze
        analysis, err := c.workflow.RunStep(ctx, "analyze", func(ctx context.Context) (any, error) {
            return c.Analyze(ctx, &AnalyzeRequest{
                ProjectPath: req.ProjectPath,
                ToVersion:   req.ToVersion,
            })
        })
        if err != nil {
            return nil, err
        }

        // Step 2: Generate plan
        plan, err := c.workflow.RunStep(ctx, "plan", func(ctx context.Context) (any, error) {
            return c.GeneratePlan(ctx, analysis.(*AnalyzeResponse))
        })
        if err != nil {
            return nil, err
        }

        // Step 3: Wait for approval (if interactive)
        if req.RequireApproval {
            approved, err := c.workflow.WaitForApproval(ctx,
                fmt.Sprintf("Apply %d changes?", len(plan.Transforms)),
                7*24*time.Hour)
            if err != nil || !approved {
                return nil, fmt.Errorf("migration not approved")
            }
        }

        // Step 4: Apply
        result, err := c.workflow.RunStep(ctx, "apply", func(ctx context.Context) (any, error) {
            return c.Apply(ctx, &ApplyRequest{
                Transforms: plan.Transforms,
            })
        })

        return result, err
    }, req)
}
```

### 6. Workflow Commands

```bash
# Start long-running migration
$ bazelbump migrate --to 9.0.0 --background
Migration started: wf_01HQ7A
Use 'bazelbump workflow status wf_01HQ7A' to check progress

# Check status
$ bazelbump workflow status wf_01HQ7A
Workflow: wf_01HQ7A
Status: WAITING_APPROVAL
Current Step: approval (3/5)
Elapsed: 2h 15m

Waiting for approval to apply 45 transforms.
Run 'bazelbump workflow approve wf_01HQ7A' to continue.

# Approve
$ bazelbump workflow approve wf_01HQ7A
Approved. Migration continuing...

# List workflows
$ bazelbump workflow list
ID          STATUS              STARTED              STEP
wf_01HQ7A   RUNNING             2025-02-03 10:00    apply (4/5)
wf_01HQ6Z   COMPLETED           2025-02-02 15:30    done
wf_01HQ5Y   FAILED              2025-02-01 09:00    analyze (1/5)

# Cancel
$ bazelbump workflow cancel wf_01HQ7A

# Resume failed
$ bazelbump workflow resume wf_01HQ5Y
```

## Package Structure

```
pkg/
├── workflow/
│   ├── engine.go           # Interface
│   ├── factory.go          # Create engine from config
│   ├── simple/
│   │   ├── engine.go       # SQLite-based
│   │   ├── schema.sql
│   │   └── engine_test.go
│   ├── dbos/
│   │   ├── engine.go       # DBOS wrapper
│   │   └── engine_test.go
│   └── noop/
│       └── engine.go       # No durability

cmd/bazelbump/
├── migrate.go
└── workflow.go
```

## Tasks

- [ ] Define workflow engine interface
- [ ] Implement simple SQLite engine
- [ ] Implement DBOS engine wrapper
- [ ] Implement no-op engine
- [ ] Implement engine factory
- [ ] Implement migration workflow
- [ ] Implement `bazelbump migrate` command
- [ ] Implement `bazelbump workflow` subcommands
- [ ] Add background execution support
- [ ] Add approval workflow
- [ ] Add tests for each engine
- [ ] Document engine selection

## Acceptance Criteria

1. Simple engine works with zero external deps
2. DBOS engine provides full durability (when enabled)
3. Workflows survive process restart (with SQLite or DBOS)
4. Approval workflow pauses correctly
5. Background mode works
6. Resume from failure works

## Configuration

```yaml
# .bazelbump/config.yaml
workflow:
  # Engine selection
  engine: simple           # simple (SQLite), dbos, or none

  # Simple engine config
  simple:
    database: ~/.local/share/bazelbump/workflows.db

  # DBOS config (if engine: dbos)
  dbos:
    database_url: postgres://localhost:5432/bazelbump
    # Or use embedded postgres:
    # embedded: true
```

## Notes

- Simple engine is default (no external deps)
- DBOS is opt-in for users who need it
- Consider embedded-postgres for "batteries included" DBOS
- Keep workflow interface minimal
