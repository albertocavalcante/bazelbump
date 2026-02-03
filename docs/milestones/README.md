# bazelbump Milestones

## Overview

bazelbump development is organized into 8 milestones spanning approximately 14 weeks.

```
Week:  1   2   3   4   5   6   7   8   9  10  11  12  13  14
       ├───┴───┤   │   │   ├───┴───┤   ├───┴───┤   ├───┴───┤
       │  M0   │   │   │   │  M4   │   │  M6   │   │  M7   │
       │       ├───┴───┤   │       ├───┴───┤   │   │       │
       │       │  M1   │   │       │  M5   │   │   │       │
       │       │       ├───┤       │       │   ├───┴───────┤
       │       │       │M2 │       │       │   │    M8     │
       │       │       │   ├───┤   │       │   │           │
       │       │       │   │M3 │   │       │   │           │
       └───────┴───────┴───┴───┴───┴───────┴───┴───────────┘
```

## Milestone Summary

| ID | Name | Weeks | Status | Key Deliverable |
|----|------|-------|--------|-----------------|
| M0 | [Foundation](./M0-foundation.md) | 1-2 | Not Started | Project structure, proto definitions |
| M1 | [Data Layer](./M1-data-layer.md) | 2-4 | Not Started | Flag DB, registry sync, datastore |
| M2 | [Analysis Core](./M2-analysis-core.md) | 4-5 | Not Started | Flag analyzer, Starlark scanner |
| M3 | [Safety System](./M3-safety.md) | 5 | Not Started | Snapshots, rollback, pre-flight |
| M4 | [Transform Engine](./M4-transform-engine.md) | 6-7 | Not Started | Transforms, apply workflow |
| M5 | [LLM Integration](./M5-llm-integration.md) | 8-9 | Not Started | Bedrock provider, fallback chain |
| M6 | [Agent Actions](./M6-agent-actions.md) | 10-11 | Not Started | diagnose, patch, explain, document |
| M7 | [Workflow Integration](./M7-workflow-integration.md) | 12-13 | Not Started | Durable workflows, background execution |
| M8 | Production Polish | 14 | Not Started | Error handling, docs, testing |

## Architecture Principles

From [ADRs](../decisions/):

1. **Airgap-First** (ADR-001): Works without network, sync data when connected
2. **No Daemon Required** (ADR-007): CLI-first, SQLite default, no external services
3. **Proto-First** (ADR-004): All types in Protocol Buffers from day 0
4. **LLM Provider Abstraction** (ADR-003): Bedrock primary, Ollama fallback

## Dependency Graph

```
M0 (Foundation)
 │
 ├──▶ M1 (Data Layer)
 │     │
 │     └──▶ M2 (Analysis Core)
 │           │
 │           └──▶ M3 (Safety System)
 │                 │
 │                 └──▶ M4 (Transform Engine)
 │                       │
 │                       ├──▶ M5 (LLM Integration)
 │                       │     │
 │                       │     └──▶ M6 (Agent Actions)
 │                       │           │
 │                       │           └──▶ M7 (Workflow Integration)
 │                       │                 │
 │                       │                 └──▶ M8 (Production Polish)
 │                       │
 │                       └──▶ (M4 can be used standalone for deterministic transforms)
```

## v1.0 MVP Scope

**Must Have:**
- M0-M4: Core transformation without LLM
- M5-M6: LLM-assisted analysis and patching
- M7 (partial): SQLite workflow engine

**Nice to Have:**
- M7 (full): DBOS integration
- M8: Full production polish

## Key Design Decisions

### Storage Architecture

```
~/.local/share/bazelbump/
├── workflows.db          # SQLite: workflow state
├── flags.db             # SQLite: cached flag data
├── snapshots/           # File snapshots for rollback
└── cache/               # Temporary cache
```

### Workflow Engine Tiers

| Tier | Engine | Dependencies | Use Case |
|------|--------|--------------|----------|
| 0 | NoOp | None | Quick commands (analyze) |
| 1 | Simple | SQLite | Most migrations (default) |
| 2 | DBOS | PostgreSQL | Enterprise, long-running |

### LLM Provider Chain

```
1. Bedrock (primary)     - AWS auth required
2. Ollama (fallback)     - Local, offline
3. NoOp (degraded)       - Deterministic-only mode
```

## Getting Started

Once development begins:

```bash
# Clone and setup
git clone https://github.com/yourorg/bazelbump
cd bazelbump
go mod download

# Generate proto
make proto

# Build
make build

# Run
./bin/bazelbump analyze --to 9.0.0
```

## Related Documentation

- [Architecture Overview](../architecture/overview.md)
- [Architectural Decisions](../decisions/)
- [Research Notes](../research/)
