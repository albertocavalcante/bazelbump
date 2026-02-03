# Research: Durable Execution Frameworks

**Date:** 2025-02-03
**Author:** Alberto Cavalcante

## Overview

This document compares durable execution frameworks for use in bazelbump's agent workflows.

## What is Durable Execution?

Durable execution ensures that long-running processes can survive failures by:
- Checkpointing state after each step
- Automatically resuming from last checkpoint on restart
- Providing exactly-once execution semantics

## Frameworks Evaluated

### 1. Temporal

**Website:** https://temporal.io/
**Go SDK:** https://github.com/temporalio/sdk-go
**License:** MIT

**Architecture:** External orchestration server

```
┌──────────┐     ┌──────────────┐     ┌──────────┐
│  Client  │────▶│   Temporal   │────▶│  Worker  │
│   App    │     │   Server     │     │ (code)   │
└──────────┘     │  (cluster)   │     └──────────┘
                 └──────────────┘
                       │
                ┌──────────────┐
                │  Cassandra/  │
                │  PostgreSQL  │
                └──────────────┘
```

**Pros:**
- Most mature (years of production use)
- Rich feature set (signals, queries, child workflows)
- Official OpenAI Agents SDK integration
- Excellent observability (Web UI)
- Large community

**Cons:**
- Complex deployment (multiple services)
- Requires rearchitecting code into workers/activities
- Heavy infrastructure footprint
- Steeper learning curve

**Code Example:**
```go
// Requires separate worker process
func Workflow(ctx workflow.Context, input string) (string, error) {
    var result string
    err := workflow.ExecuteActivity(ctx, MyActivity, input).Get(ctx, &result)
    return result, err
}
```

### 2. DBOS

**Website:** https://www.dbos.dev/
**Go SDK:** https://github.com/dbos-inc/dbos-transact-golang
**License:** MIT

**Architecture:** Embedded library + PostgreSQL

```
┌────────────────────────────────────┐
│         Your Application            │
│  ┌──────────────────────────────┐  │
│  │   dbos.RunWorkflow()         │  │     ┌──────────────┐
│  │   dbos.RunAsStep()           │──┼────▶│  PostgreSQL  │
│  │   (library calls)            │  │     └──────────────┘
│  └──────────────────────────────┘  │
└────────────────────────────────────┘
```

**Pros:**
- Simplest deployment (just Postgres)
- Minimal code changes (annotations)
- Go-native (uses context.Context)
- Active development (CockroachDB support Jan 2026)
- No separate services needed

**Cons:**
- Newer, smaller community
- PostgreSQL-only storage
- No built-in web UI
- Less feature-rich than Temporal

**Code Example:**
```go
// Same process, just annotated
func workflow(ctx dbos.DBOSContext, input string) (string, error) {
    return dbos.RunAsStep(ctx, func(ctx context.Context) (string, error) {
        return myFunction(ctx, input)
    })
}
```

### 3. Restate

**Website:** https://restate.dev/
**Go SDK:** https://github.com/restatedev/sdk-go
**License:** Business Source License 1.1

**Architecture:** Lightweight server + service handlers

```
┌──────────┐     ┌──────────────┐     ┌──────────┐
│  Client  │────▶│   Restate    │────▶│  Service │
│          │     │   Server     │     │ (code)   │
└──────────┘     │  (binary)    │     └──────────┘
                 └──────────────┘
                       │
                (embedded RocksDB)
```

**Pros:**
- Simpler than Temporal
- Single binary server
- No external database needed
- Good Go SDK
- Web UI included

**Cons:**
- BSL license (not fully open source)
- Requires separate server
- Smaller community than Temporal
- Newer in Go ecosystem

**Code Example:**
```go
func (s *Service) Handler(ctx restate.Context, input string) (string, error) {
    return restate.Run(ctx, func(ctx restate.RunContext) (string, error) {
        return myFunction(input)
    })
}
```

## Comparison Matrix

| Feature | Temporal | DBOS | Restate |
|---------|----------|------|---------|
| **Architecture** | External cluster | Embedded library | Lightweight server |
| **Storage** | Cassandra/Postgres | PostgreSQL | RocksDB |
| **Go SDK Maturity** | Excellent | Good | Good |
| **Setup Complexity** | High | **Low** | Medium |
| **Code Changes** | Major | **Minimal** | Moderate |
| **Self-hosted** | Yes (complex) | **Yes (simple)** | Yes |
| **License** | MIT | MIT | BSL 1.1 |
| **Web UI** | Yes | No | Yes |
| **Airgap Friendly** | Medium | **Best** | Good |
| **Community** | Large | Growing | Growing |
| **AI/LLM Integration** | Official OpenAI | Manual | Manual |

## Airgap Deployment Comparison

| Aspect | Temporal | DBOS | Restate |
|--------|----------|------|---------|
| **Services to deploy** | 3+ (server, db, workers) | 1 (Postgres) | 2 (server, app) |
| **Container images** | ~5 | 1 | 2 |
| **Storage requirements** | High (Cassandra) | Medium (Postgres) | Low (RocksDB) |
| **Complexity** | High | **Low** | Medium |

## Decision

**Selected: DBOS**

Rationale:
1. Best fit for airgap (only needs Postgres)
2. Minimal code changes required
3. Go-native design
4. Simple deployment model
5. Active development

We will implement a `WorkflowEngine` interface to allow future adaptation to Temporal or Restate if needed.

## References

- DBOS Blog: https://www.dbos.dev/blog/how-we-built-golang-native-durable-execution
- DBOS vs Temporal: https://www.dbos.dev/blog/durable-execution-coding-comparison
- Temporal Go SDK: https://docs.temporal.io/develop/go
- Restate Go SDK: https://docs.restate.dev/develop/go/overview
- Durable Execution Landscape: https://www.golem.cloud/post/the-emerging-landscape-of-durable-computing
