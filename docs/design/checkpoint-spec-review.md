# Executor-State Integration and Workflow Checkpoint Specification Review

**Date:** 2026-02-03
**Reviewer:** Claude Opus 4.5
**Status:** Comprehensive Review

---

## Executive Summary

The bazelbump workflow checkpoint specifications (`executor-state-integration-spec.md` and `workflow-checkpoint-spec.md`) present a well-designed system for progress tracking, checkpointing, and resumable execution. The architecture makes sensible choices (SQLite with WAL mode, pure-Go implementation, optional build tags) that align with the project's "airgap-first" philosophy.

However, after comparing with industry-standard solutions (Temporal, DBOS, Prefect, Airflow, Bazel, Git) and analyzing edge cases, I've identified several gaps, potential issues, and opportunities for improvement.

**Overall Assessment: 7.5/10** - Solid foundation with room for robustness improvements.

---

## 1. Critical Review: Gaps and Issues

### 1.1 Race Conditions and Concurrency

**Issue: File Lock Implementation**

The spec mentions file locking in Section 11 but uses OS-level `flock()`:

```go
err = syscall.Flock(int(l.file.Fd()), syscall.LOCK_EX|syscall.LOCK_NB)
```

**Problems:**
1. `flock()` is advisory on POSIX systems - other processes can ignore it
2. Doesn't work across NFS/network filesystems (common in CI)
3. Not portable to Windows (requires `LockFileEx`)

**Recommendation:** Use SQLite's built-in locking (`BEGIN EXCLUSIVE`) or implement cross-platform locking via `github.com/gofrs/flock` which handles platform differences.

---

**Issue: State Manager Mutex Granularity**

The `sqliteManager` uses a single `sync.RWMutex` for all operations:

```go
type sqliteManager struct {
    db   *sql.DB
    mu   sync.RWMutex  // Single lock for everything
    path string
}
```

**Problems:**
1. All writes block all reads across unrelated data (runs, checkpoints, file states)
2. SQLite already handles concurrency via WAL mode - the mutex is redundant
3. Could cause performance issues with frequent progress updates

**Recommendation:** Remove the mutex and rely on SQLite's concurrency control, or use finer-grained locks per table if needed.

---

### 1.2 Missing Error Recovery Scenarios

**Issue: Partial Checkpoint Writes**

The spec doesn't address what happens if a checkpoint write is interrupted mid-transaction:

```go
func (m *sqliteManager) CreateCheckpoint(runID string, opts CheckpointOptions) (*Checkpoint, error) {
    // What if process crashes here, after INSERT but before commit?
    _, err := m.db.Exec(`INSERT INTO checkpoints ...`)
}
```

**Recommendation:** Use explicit transactions with deferred commit and consider adding checkpoint validation on load:

```go
func (m *sqliteManager) CreateCheckpoint(...) (*Checkpoint, error) {
    tx, err := m.db.Begin()
    if err != nil {
        return nil, err
    }
    defer tx.Rollback() // No-op if committed

    // Create checkpoint
    // Update run progress atomically

    return cp, tx.Commit()
}
```

---

**Issue: Orphaned "Running" Runs**

If a process crashes hard (SIGKILL), runs remain in "running" status forever. The spec mentions detecting "incomplete runs" but doesn't specify remediation.

**Recommendation:** Add startup validation:

```go
func (m *sqliteManager) ValidateOnStartup() error {
    // Find runs stuck in "running" status for > 1 hour
    // Mark them as "interrupted" with error message
    _, err := m.db.Exec(`
        UPDATE runs
        SET status = 'interrupted',
            error_message = 'Process terminated unexpectedly'
        WHERE status = 'running'
        AND started_at < datetime('now', '-1 hour')
    `)
    return err
}
```

---

### 1.3 Edge Cases Not Handled

**Issue: File State Consistency**

The spec tracks file states but doesn't handle:

1. **Files moved/renamed mid-run** - Hash matches but path differs
2. **Symlink handling** - Should track the link or the target?
3. **Files created then deleted in same run** - State becomes stale

**Recommendation:** Add a `file_exists` column or verify file existence during resume.

---

**Issue: Checkpoint State Bloat**

The `ExecutionState` can grow unbounded:

```go
type ExecutionState struct {
    ScanResults       map[string]any `json:"scan_results,omitempty"`
    PendingOperations []any          `json:"pending_operations,omitempty"`
    ContextOutputs    map[string]any `json:"context_outputs,omitempty"`
}
```

For large projects (1,696 files), this could be several MB per checkpoint. With auto-checkpoints every 100 files, that's 17 checkpoints = potential memory/storage pressure.

**Recommendation:**
1. Limit checkpoint state size (warn if > 1MB)
2. Consider incremental state (store delta from previous checkpoint)
3. Add checkpoint cleanup (`DELETE FROM checkpoints WHERE run_id = ? ORDER BY created_at LIMIT -10 OFFSET 5`)

---

**Issue: No Idempotency Guarantees**

The spec doesn't ensure idempotent file processing. If a file is partially modified and the process crashes, resume will reprocess with corrupted input.

**Recommendation:** Apply a "two-phase commit" pattern for file modifications:
1. Write to `file.BUILD.bazelbump-tmp`
2. Mark file state as "staged"
3. Atomic rename to `file.BUILD`
4. Mark file state as "completed"

---

### 1.4 Performance Concerns

**Issue: Database Write Frequency**

Each file processing triggers multiple DB writes:
1. `SetFileState` (in_progress)
2. `SetFileState` (completed)
3. `LogAction` (file_modified)
4. Potentially `CreateCheckpoint`

For 893 files, that's 2,679+ individual writes.

**Recommendation:** Implement write batching:

```go
type batchedManager struct {
    Manager
    pendingFileStates []FileState
    pendingLogs       []AuditLogEntry
    flushThreshold    int // Flush every N operations
}

func (m *batchedManager) Flush() error {
    tx, _ := m.db.Begin()
    for _, fs := range m.pendingFileStates {
        // Bulk insert
    }
    m.pendingFileStates = nil
    return tx.Commit()
}
```

---

**Issue: Hash Computation During Processing**

Computing xxHash before AND after each file:

```go
// Compute file hash before processing
beforeHash := computeFileHash(file)
// ... process ...
afterHash := computeFileHash(file)
```

For 893 files at ~100KB each, that's ~170MB of I/O just for hashing.

**Recommendation:** Only compute after-hash for files that changed. Use mtime as initial filter:

```go
if stat.ModTime().After(beforeMtime) {
    afterHash = computeFileHash(file)
}
```

---

### 1.5 API Design Issues

**Issue: Inconsistent Error Handling**

Some methods return `nil` for "not found":
```go
if err == sql.ErrNoRows {
    return nil, nil  // Silent "not found"
}
```

Others might return an error. This is confusing for callers.

**Recommendation:** Use a consistent pattern - either sentinel errors or Option types:

```go
var ErrNotFound = errors.New("not found")

func (m *sqliteManager) GetRun(id string) (*Run, error) {
    // ...
    if err == sql.ErrNoRows {
        return nil, ErrNotFound
    }
}
```

---

**Issue: Resume Semantics Ambiguity**

The spec has `--resume` (from checkpoint) and `--restart` (from beginning) and `--since` (incremental from run). The interaction is unclear:

- What if `--resume` is passed but no checkpoint exists?
- What if the checkpoint's file list doesn't match current files?
- What if `--since` references a failed run?

**Recommendation:** Add a clear decision tree in the spec and explicit error messages.

---

## 2. Benchmark Against Industry Solutions

### 2.1 Temporal.io - Workflow Orchestration

| Aspect | Temporal | bazelbump |
|--------|----------|-----------|
| **State Persistence** | Event sourcing with full history replay | State snapshots in SQLite |
| **Checkpointing** | Automatic at every activity call | Manual + auto-interval |
| **Resume Semantics** | Deterministic replay from history | Resume from latest checkpoint |
| **Durability** | Distributed (Cassandra/MySQL) | Single-file SQLite |
| **Complexity** | High (requires cluster) | Low (embedded) |

**Key Insight from Temporal:** Their "durable execution" model captures every state transition automatically. This eliminates the manual checkpoint placement decisions in bazelbump.

**Recommendation:** Consider adding an "event log" table that records all state transitions. This enables:
- Post-hoc debugging
- Deterministic replay for testing
- Better audit trails

```sql
CREATE TABLE event_log (
    id INTEGER PRIMARY KEY,
    run_id TEXT NOT NULL,
    timestamp TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
    event_type TEXT NOT NULL,  -- 'file_start', 'file_complete', 'step_start', etc.
    event_data JSON
);
```

**Source:** [Temporal Workflow Documentation](https://docs.temporal.io/workflows)

---

### 2.2 DBOS - Transactional Workflows

| Aspect | DBOS | bazelbump |
|--------|------|-----------|
| **Foundation** | Postgres transactions | SQLite WAL |
| **Exactly-once** | Yes (via transaction piggyback) | No (at-least-once) |
| **Overhead** | 1 write/step + 2/workflow | 3+ writes/file |
| **Architecture** | Library-only | Library-only |

**Key Insight from DBOS:** They achieve exactly-once semantics by executing the step AND its checkpoint in a single transaction. If the step has side effects (like file modification), this isn't possible - but for pure database operations, it's elegant.

**Recommendation:** For the file_states table, use upsert pattern to make SetFileState idempotent:

```sql
INSERT INTO file_states (run_id, file_path, status, ...)
VALUES (?, ?, ?, ...)
ON CONFLICT(run_id, file_path) DO UPDATE SET
    status = excluded.status,
    ...
WHERE status != 'completed'  -- Don't overwrite completed
```

**Source:** [DBOS Architecture](https://docs.dbos.dev/architecture)

---

### 2.3 Prefect - Python Workflow Orchestration

| Aspect | Prefect | bazelbump |
|--------|---------|-----------|
| **State Types** | Rich (Running, Retrying, Paused, etc.) | Simple (pending/running/completed/failed) |
| **Result Caching** | First-class with `persist_result=True` | Manual via file hashes |
| **Deployment** | Requires Orion server | Embedded |
| **Cost Optimization** | 40-70% via caching | Similar potential |

**Key Insight from Prefect:** Their state machine distinguishes between "Running" and "Retrying" states. This helps users understand whether a task is fresh or recovering.

**Recommendation:** Add `retry_count` to the runs table and distinct states:

```sql
ALTER TABLE runs ADD COLUMN retry_count INTEGER DEFAULT 0;
ALTER TABLE runs ADD CONSTRAINT status_check CHECK(
    status IN ('pending', 'running', 'retrying', 'completed',
               'failed', 'interrupted', 'cancelled')
);
```

**Source:** [Prefect States Documentation](https://docs.prefect.io/v3/concepts/states)

---

### 2.4 Apache Airflow - DAG-based Workflows

| Aspect | Airflow | bazelbump |
|--------|---------|-----------|
| **Resume Unit** | Task-level | File-level |
| **Backfill** | Native with catchup | Manual via `--since` |
| **XCom** | For inter-task data | ContextOutputs |
| **State Persistence** | Metadata DB (Postgres) | SQLite |

**Key Insight from Airflow (AIP-30):** They identified that XCom records being cleared at task start prevents incremental job patterns. Our `ContextOutputs` faces the same issue - it's serialized per-checkpoint but not preserved across retries.

**Recommendation:** Store `ContextOutputs` at the run level, not just in checkpoints:

```sql
ALTER TABLE runs ADD COLUMN context_outputs TEXT;
```

Update after each step completes, not just at checkpoints.

**Source:** [AIP-30: State Persistence](https://cwiki.apache.org/confluence/display/AIRFLOW/AIP-30:+State+persistence)

---

### 2.5 Bazel - Build System Incremental Execution

| Aspect | Bazel | bazelbump |
|--------|-------|-----------|
| **Cache Key** | Content hash + deps hash | Content hash only |
| **Persistent Cache** | Custom binary format | SQLite |
| **Remote Cache** | gRPC protocol | Not supported |
| **Incremental Analysis** | Dependency graph | File-level only |

**Key Insight from Bazel:** Their action cache includes both input hashes AND action definition. This means changing the "recipe" invalidates the cache even if files haven't changed.

**Recommendation:** Add recipe version/hash to the cache key:

```go
type CacheKey struct {
    FilePath     string
    ContentHash  string
    RecipeHash   string  // Hash of the recipe definition
    RecipeVersion string
}
```

This prevents using stale state when the migration recipe is updated.

**Source:** [Bazel Persistent Action Cache](https://sluongng.hashnode.dev/bazel-caching-explained-pt-4-persistent-action-cache)

---

### 2.6 SQLite WAL - Storage Approach

| Aspect | Current Spec | Best Practices |
|--------|--------------|----------------|
| **Sync Mode** | Not specified | PRAGMA synchronous=FULL for durability |
| **WAL Checkpointing** | Automatic | Should configure max WAL size |
| **Busy Timeout** | 5000ms | Good |
| **Connection Pool** | MaxOpenConns=1 | Correct for single writer |

**Key Insights from SQLite WAL:**
1. With `synchronous=NORMAL` (the default), transactions can be lost on power failure
2. WAL files can grow unbounded without explicit checkpointing
3. WAL mode is not crash-safe across power loss without synchronous=FULL

**Recommendations:**

```go
// In New() function
db.Exec("PRAGMA synchronous=FULL")  // Durability over speed
db.Exec("PRAGMA wal_autocheckpoint=100")  // Checkpoint every 100 pages
db.Exec("PRAGMA journal_size_limit=5242880")  // Limit WAL to 5MB
```

**Source:** [SQLite WAL Mode](https://sqlite.org/wal.html)

---

### 2.7 Git - Content-Addressed Storage

| Aspect | Git | bazelbump |
|--------|-----|-----------|
| **Object Model** | Blob/Tree/Commit | Flat file states |
| **Deduplication** | Yes (SHA-1 content address) | No |
| **Compression** | Delta + zlib | None |
| **History** | Full graph | Flat list |

**Key Insight from Git:** Content-addressed storage enables deduplication and efficient diff computation. For large migrations with many similar files, this could significantly reduce storage.

**Recommendation for v2:** Consider storing file content hashes in a content-addressed store:

```
.bazelbump/
├── state.db
└── objects/
    ├── ab/
    │   └── cdef1234...  # File content by hash
    └── cd/
        └── ef5678...
```

This enables:
- Deduplication across runs
- Efficient undo without git
- Proof of what was changed

**Source:** [Git Objects](https://git-scm.com/book/en/v2/Git-Internals-Git-Objects)

---

## 3. Alternative Approaches to Consider

### 3.1 File-Based Checkpoints vs SQLite

**Current Approach:** SQLite for everything

**Alternative: Hybrid Approach**

```
.bazelbump/
├── state.db              # Only runs and audit log
├── checkpoints/
│   └── run-abc123/
│       ├── cp-001.json   # Checkpoint state
│       ├── cp-002.json
│       └── latest -> cp-002.json
└── file-states/
    └── run-abc123.json   # All file states for run
```

**Trade-offs:**

| Aspect | SQLite-Only | Hybrid |
|--------|-------------|--------|
| **Atomicity** | ACID transactions | File-level only |
| **Query Flexibility** | Full SQL | Must load + filter |
| **Backup** | Single file | Multiple files |
| **Debugging** | Requires SQLite tools | Human-readable |
| **Portability** | Better | Filesystem edge cases |

**Recommendation:** Keep SQLite for the core design but add JSON export for debugging:

```bash
bazelbump status --export-json run-abc123.json
```

---

### 3.2 Content-Addressed Storage

**Current:** Hash stored in metadata, file content managed separately

**Alternative:** Store snapshots as content-addressed blobs

```go
type ContentStore struct {
    path string // .bazelbump/objects/
}

func (s *ContentStore) Store(content []byte) (string, error) {
    hash := xxhash.Sum64(content)
    hashStr := fmt.Sprintf("%016x", hash)
    dir := filepath.Join(s.path, hashStr[:2])
    file := filepath.Join(dir, hashStr[2:])
    // Write if not exists
    return hashStr, nil
}
```

**Benefits:**
- Enables undo without git
- Deduplicates unchanged files across runs
- Provides proof of original content

**Drawbacks:**
- More complex implementation
- Requires garbage collection
- Storage overhead for full file copies

**Recommendation:** Implement as an optional enhancement for non-git repos using file tracker strategy.

---

### 3.3 Event Sourcing vs State Snapshots

**Current:** State snapshots (store current state periodically)

**Alternative:** Event sourcing (store all state changes)

**Analysis:**

For bazelbump's use case:
- **Write volume:** ~3 events per file = 2,679 events for Envoy
- **Read pattern:** Usually only need latest state
- **Replay need:** Rare (debugging, audit)

Event sourcing would add complexity without clear benefit. The current snapshot approach is appropriate.

**However:** Add an event log for audit/debugging purposes without full event sourcing semantics.

---

### 3.4 Distributed vs Local-Only State

**Current:** Local SQLite only

**Alternative:** Optional remote state backend

**Use Cases for Remote State:**
1. CI/CD where builds run on different machines
2. Team collaboration on large migrations
3. Centralized audit logging

**Recommendation (Future):** Define a `StateBackend` interface that allows plugging in remote storage:

```go
type StateBackend interface {
    GetRun(id string) (*Run, error)
    CreateRun(recipe string, opts RunOptions) (*Run, error)
    // ... etc
}

type LocalSQLiteBackend struct { ... }
type RemoteHTTPBackend struct { ... }  // Future
```

This is a v2 feature, not needed for initial implementation.

---

## 4. Prioritized Recommendations

### P0 - Critical (Before Implementation)

1. **Add SQLite durability settings**
   ```go
   db.Exec("PRAGMA synchronous=FULL")
   ```
   *Why:* Current spec may lose data on power failure.

2. **Fix orphaned run detection**
   - Add startup validation
   - Mark stuck "running" runs as "interrupted"
   *Why:* Silent data corruption otherwise.

3. **Implement cross-platform file locking**
   - Use `github.com/gofrs/flock` or SQLite-based locking
   *Why:* Current `flock()` approach doesn't work on Windows or NFS.

4. **Add atomic file updates**
   - Write to temp file, then rename
   *Why:* Prevents corrupted files on crash.

### P1 - Important (During Implementation)

5. **Batch database writes**
   - Buffer file states, flush every N files
   - Target: <50ms per batch, not per file
   *Why:* 3x performance improvement for large repos.

6. **Add checkpoint size limits**
   - Warn if state > 1MB
   - Auto-cleanup old checkpoints
   *Why:* Prevents unbounded storage growth.

7. **Consistent error handling**
   - Define `ErrNotFound` sentinel
   - Document expected errors for each method
   *Why:* API usability and correctness.

8. **Add recipe hash to cache key**
   - Invalidate state when recipe changes
   *Why:* Prevents subtle bugs when updating recipes.

### P2 - Nice to Have (Post-Implementation)

9. **Event log for audit/debugging**
   - Separate from checkpoints
   - Enables time-travel debugging
   *Why:* Better debugging, Temporal-inspired.

10. **JSON export for debugging**
    ```bash
    bazelbump checkpoint export cp-123 --format=json
    ```
    *Why:* Easier debugging without SQLite tools.

11. **Retry state tracking**
    - `retry_count` column
    - "retrying" state
    *Why:* Better user feedback, Prefect-inspired.

12. **Content-addressed file storage**
    - For non-git repos
    - Enables undo without git
    *Why:* Better undo experience for file tracker users.

---

## 5. Summary Comparison Matrix

| Feature | bazelbump (Current Spec) | Temporal | DBOS | Prefect | Airflow | Bazel |
|---------|--------------------------|----------|------|---------|---------|-------|
| Automatic Checkpointing | Interval-based | Every activity | Every step | Result caching | Task-level | Action cache |
| Exactly-Once | No | Yes (via replay) | Yes (via txn) | Yes (via cache) | No | Yes (cache key) |
| Distributed State | No | Yes | Yes | Yes (optional) | Yes | Yes (remote cache) |
| Event History | Partial (audit log) | Full | Full | Partial | Full | No |
| Resume Granularity | File-level | Activity-level | Step-level | Task-level | Task-level | Action-level |
| Storage Backend | SQLite | Cassandra/SQL | Postgres | Postgres/S3 | SQL | Custom binary |
| Complexity | Low | High | Medium | Medium | High | High |

---

## 6. Conclusion

The bazelbump checkpoint specification is a well-designed system that makes appropriate trade-offs for its use case (local-first, single-binary, airgap-friendly). The SQLite + WAL approach is sound.

**Key Strengths:**
- Clean interface design (`Manager`, `ProgressReporter`)
- Build tag support for minimal builds
- Integration with existing tracker system
- Reasonable default configurations

**Key Improvements Needed:**
- Durability settings (synchronous=FULL)
- Cross-platform file locking
- Atomic file updates
- Batched database writes
- Orphaned run handling

After implementing the P0 and P1 recommendations, the system will be production-ready for large-scale migrations like Envoy (1,696 BUILD files).

---

## References

- [Temporal Workflow Documentation](https://docs.temporal.io/workflows)
- [DBOS Architecture](https://docs.dbos.dev/architecture)
- [Prefect States Documentation](https://docs.prefect.io/v3/concepts/states)
- [AIP-30: State Persistence](https://cwiki.apache.org/confluence/display/AIRFLOW/AIP-30:+State+persistence)
- [SQLite WAL Mode](https://sqlite.org/wal.html)
- [Bazel Persistent Action Cache](https://sluongng.hashnode.dev/bazel-caching-explained-pt-4-persistent-action-cache)
- [Git Objects](https://git-scm.com/book/en/v2/Git-Internals-Git-Objects)
- [Event Sourcing Pattern](https://learn.microsoft.com/en-us/azure/architecture/patterns/event-sourcing)
- [Snapshots in Event Sourcing](https://www.kurrent.io/blog/snapshots-in-event-sourcing)
