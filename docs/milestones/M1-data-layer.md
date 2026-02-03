# Milestone 1: Data Layer

**Target:** Week 2
**Status:** Not Started
**Depends On:** M0 (Foundation)

## Objective

Implement data sync tooling and embedded data system using registry.build as primary source.

## Deliverables

### 1. Data Sync Command

```bash
# Sync all data
bazelbump data sync --output ./data/

# Sync from local registry.build clone
bazelbump data sync --flags-source /path/to/registry.build

# Sync specific versions
bazelbump data sync --bazel-versions 8.0.0,9.0.0

# Dry run
bazelbump data sync --dry-run
```

### 2. Data Info Command

```bash
$ bazelbump data info

DATA SOURCES
============
Flags:     embedded v1.0.0 (2025-02-03, 95 versions, 764 flags)
BCR:       external ~/.local/share/bazelbump/bcr.pb (2025-02-01)
Starlark:  embedded v1.0.0 (47 deprecations)
Recipes:   embedded (15 recipes)

Data location: /usr/local/share/bazelbump/
```

### 3. Flag Database

**Proto schema:**
```protobuf
message Flag {
  string name = 1;
  string type = 2;           // boolean, string, path, etc.
  string default_value = 3;
  string description = 4;
  repeated string tags = 5;
  repeated string commands = 6;  // build, test, run, etc.
}

message FlagVersion {
  string bazel_version = 1;
  repeated Flag flags = 2;
}

message FlagDatabase {
  string version = 1;
  google.protobuf.Timestamp generated_at = 2;
  repeated FlagVersion versions = 3;
}
```

**Sync from registry.build:**
```go
// internal/datasync/flags.go
func SyncFlags(ctx context.Context, source, output string) error {
    // 1. Read /flag/bazel/{version}/all.json for each version
    // 2. Convert to FlagDatabase proto
    // 3. Write to output path as protobuf binary
}
```

### 4. Starlark Deprecations

**Proto schema:**
```protobuf
message StarlarkAPIChange {
  string api = 1;                    // e.g., "native.existing_rule"
  string deprecated_in = 2;          // e.g., "7.0.0"
  string removed_in = 3;             // e.g., "9.0.0"
  string replacement = 4;            // e.g., "native.existing_rules().get()"
  string pattern = 5;                // Regex to detect usage
  string migration_example = 6;
}

message StarlarkDeprecationDatabase {
  string version = 1;
  repeated StarlarkAPIChange changes = 2;
}
```

### 5. Data Loading

```go
// internal/datastore/loader.go
type DataStore struct {
    flags    *FlagDatabase
    starlark *StarlarkDeprecationDatabase
    bcr      *BCRDatabase
}

func NewDataStore(opts ...Option) (*DataStore, error) {
    // Priority:
    // 1. Explicit path from config
    // 2. XDG data directory (~/.local/share/bazelbump/)
    // 3. Embedded data
}

//go:embed data/flags.pb
var embeddedFlags []byte

func (d *DataStore) loadFlags() error {
    // Try external, fall back to embedded
}
```

### 6. Data Import/Export

```bash
# Import synced data
bazelbump data import ./data-bundle/

# Export for transfer
bazelbump data export --output ./export/
```

## Package Structure

```
internal/
├── datasync/
│   ├── sync.go           # Main sync orchestration
│   ├── flags.go          # Flag sync from registry.build
│   ├── starlark.go       # Starlark deprecation data
│   └── bcr.go            # BCR metadata sync
│
├── datastore/
│   ├── loader.go         # Data loading logic
│   ├── flags.go          # Flag database operations
│   ├── starlark.go       # Starlark operations
│   └── embed.go          # Embedded data handling
│
cmd/bazelbump/
└── data.go               # data subcommands
```

## Tasks

- [ ] Define data-related proto schemas
- [ ] Implement registry.build parser for flags
- [ ] Implement flag database sync
- [ ] Create Starlark deprecation database (manual + patterns)
- [ ] Implement data loading with fallback chain
- [ ] Implement `bazelbump data sync` command
- [ ] Implement `bazelbump data info` command
- [ ] Implement `bazelbump data import` command
- [ ] Implement `bazelbump data export` command
- [ ] Create initial embedded data files
- [ ] Add tests for data loading
- [ ] Document data sync workflow

## Acceptance Criteria

1. `bazelbump data sync` successfully syncs from registry.build
2. `bazelbump data info` shows data status
3. Embedded data loads correctly when external not available
4. External data takes precedence over embedded
5. Data format is versioned and validated
6. Sync produces deterministic output

## Data Sources

| Data | Source | Format |
|------|--------|--------|
| Flags | /Users/adsc/dev/temp/registry.build/flag/bazel/ | JSON → Proto |
| Starlark | Manual + Bazel source patterns | YAML → Proto |
| BCR | bcr.bazel.build (future) | JSON → Proto |

## Notes

- Start with flag database (most critical)
- Starlark deprecations can be manually curated initially
- BCR sync can be deferred to M4
- Keep embedded data size reasonable (~50MB target)
