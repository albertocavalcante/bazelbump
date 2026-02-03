# Milestone 3: Safety System

**Target:** Week 5
**Status:** Not Started
**Depends On:** M2 (Analysis Core)

## Objective

Implement snapshot/rollback system and pre-flight checks to ensure safe upgrades.

## Deliverables

### 1. Snapshot System

```bash
# Create snapshot before changes
$ bazelbump snapshot create
Snapshot created: snap_01HQ5X...
Files: 45 | Size: 2.3MB | Location: ~/.local/share/bazelbump/snapshots/

# List snapshots
$ bazelbump snapshot list
ID              CREATED              FILES    SIZE     DESCRIPTION
snap_01HQ5X     2025-02-03 10:00    45       2.3MB    Before upgrade to 9.0.0
snap_01HQ4Y     2025-02-02 15:30    42       2.1MB    Before upgrade to 8.5.0

# Restore snapshot
$ bazelbump snapshot restore snap_01HQ5X
Restored 45 files from snapshot snap_01HQ5X

# Delete old snapshots
$ bazelbump snapshot gc --keep 5
Deleted 3 old snapshots, kept 5 most recent
```

### 2. Snapshot Storage

```go
// internal/snapshot/manager.go
type Manager struct {
    storageDir string
    maxCount   int
}

type Snapshot struct {
    ID          string
    CreatedAt   time.Time
    Description string
    Files       []FileEntry
    Checksum    string
}

type FileEntry struct {
    Path     string
    Checksum string
    Mode     os.FileMode
    Content  []byte  // or reference to content-addressed storage
}

func (m *Manager) Create(ctx context.Context, paths []string, desc string) (*Snapshot, error)
func (m *Manager) Restore(ctx context.Context, id string) error
func (m *Manager) List(ctx context.Context) ([]*Snapshot, error)
func (m *Manager) Delete(ctx context.Context, id string) error
func (m *Manager) GC(ctx context.Context, keep int) (int, error)
```

**Storage format:**
```
~/.local/share/bazelbump/snapshots/
├── snap_01HQ5X/
│   ├── manifest.json
│   ├── files/
│   │   ├── .bazelrc
│   │   ├── MODULE.bazel
│   │   └── tools/
│   │       └── macros.bzl
│   └── checksum
└── snap_01HQ4Y/
    └── ...
```

### 3. Pre-flight Checks

```bash
$ bazelbump preflight --to 9.0.0

PRE-FLIGHT CHECKS
=================

[✓] git-clean
    Working directory is clean

[✓] bazel-version
    Current version: 8.0.0

[✓] current-build
    bazel build //... succeeded

[✓] disk-space
    Available: 50GB (required: 500MB)

[!] uncommitted-bazelrc
    WARNING: .bazelrc has uncommitted changes
    Recommendation: Commit or stash before upgrade

[✓] snapshot-space
    Snapshot storage available

RESULT: READY (1 warning)

Proceed with upgrade? [y/N]
```

### 4. Pre-flight Check Framework

```go
// internal/preflight/runner.go
type Runner struct {
    checks []Check
}

type Check interface {
    Name() string
    Description() string
    Run(ctx context.Context, project *Project) (*CheckResult, error)
}

type CheckResult struct {
    Status      Status  // Pass, Warn, Fail
    Message     string
    Details     string
    Remediation string
}

// Built-in checks
type GitCleanCheck struct{}
type BazelVersionCheck struct{}
type CurrentBuildCheck struct{}
type DiskSpaceCheck struct{}
type SnapshotSpaceCheck struct{}
```

### 5. Integration with Apply

```go
// pkg/libbazelbump/apply.go
func (c *Client) Apply(ctx context.Context, req *ApplyRequest) (*ApplyResponse, error) {
    // 1. Run pre-flight checks (unless --skip-preflight)
    if !req.SkipPreflight {
        results, err := c.preflight.Run(ctx, req.Project)
        if err != nil {
            return nil, err
        }
        if hasBlockers(results) {
            return nil, &PreflightFailedError{Results: results}
        }
    }

    // 2. Create snapshot (unless --no-snapshot)
    if !req.NoSnapshot {
        snap, err := c.snapshots.Create(ctx, affectedFiles, "Before upgrade")
        if err != nil {
            return nil, fmt.Errorf("create snapshot: %w", err)
        }
        defer func() {
            if err != nil {
                c.snapshots.Restore(ctx, snap.ID)
            }
        }()
    }

    // 3. Apply transformations
    // ...

    // 4. Verify build (unless --skip-verify)
    // ...
}
```

### 6. Rollback Command

```bash
# Rollback to last snapshot
$ bazelbump rollback
Rolled back to snapshot snap_01HQ5X (created 2025-02-03 10:00)
Restored 45 files

# Rollback to specific snapshot
$ bazelbump rollback snap_01HQ4Y

# Rollback with verification
$ bazelbump rollback --verify
Rolled back to snapshot snap_01HQ5X
Running verification build...
Build successful!
```

## Package Structure

```
internal/
├── snapshot/
│   ├── manager.go
│   ├── manager_test.go
│   ├── storage.go
│   └── types.go
│
├── preflight/
│   ├── runner.go
│   ├── runner_test.go
│   ├── checks/
│   │   ├── git.go
│   │   ├── bazel.go
│   │   ├── disk.go
│   │   └── build.go
│   └── types.go

cmd/bazelbump/
├── snapshot.go
├── preflight.go
└── rollback.go
```

## Tasks

- [ ] Design snapshot storage format
- [ ] Implement snapshot create/restore/delete
- [ ] Implement snapshot list with filtering
- [ ] Implement garbage collection
- [ ] Define pre-flight check interface
- [ ] Implement git-clean check
- [ ] Implement bazel-version check
- [ ] Implement current-build check
- [ ] Implement disk-space check
- [ ] Implement `bazelbump snapshot` commands
- [ ] Implement `bazelbump preflight` command
- [ ] Implement `bazelbump rollback` command
- [ ] Integrate safety into apply workflow
- [ ] Add tests
- [ ] Document safety features

## Acceptance Criteria

1. Snapshots correctly capture all affected files
2. Restore fully reverts changes
3. Pre-flight checks catch common issues
4. Rollback works reliably
5. GC correctly removes old snapshots
6. Safety features can be bypassed when needed (--skip-*)

## Configuration

```yaml
# .bazelbump/config.yaml
safety:
  snapshots:
    enabled: true
    max_count: 10
    retention_days: 30
    location: ~/.local/share/bazelbump/snapshots

  preflight:
    enabled: true
    fail_on_warnings: false
    skip_checks:
      - tests-pass  # Skip test verification
    custom_checks:
      - name: custom-lint
        command: make lint
        severity: warning
```

## Notes

- Snapshots should be efficient (consider compression)
- Handle large files gracefully (maybe exclude binaries)
- Pre-flight checks should be fast
- Allow customization of checks per project
