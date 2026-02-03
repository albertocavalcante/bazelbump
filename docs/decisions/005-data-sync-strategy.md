# ADR-005: Data Sync Strategy

**Status:** Accepted
**Date:** 2025-02-03
**Deciders:** Alberto Cavalcante

## Context

bazelbump requires several data sources that are typically fetched from the internet:

1. **Flag database** - Bazel flags across all versions
2. **BCR module metadata** - Module compatibility information
3. **Starlark deprecations** - API changes and migrations
4. **Migration recipes** - Community-contributed migration patterns

In our airgap-first architecture, these must be:
- Pre-synced from a connected environment
- Bundled into the binary or data files
- Updateable without rebuilding

## Decision

**Implement a two-phase data strategy:**

1. **Sync tooling** - `bazelbump data sync` fetches from sources
2. **Embedded data** - Compile-time embedded for core data
3. **External data** - Runtime-loadable for updates

## Data Sources

```
┌─────────────────────────────────────────────────────────────────────────────┐
│                           DATA SOURCES                                       │
├─────────────────────────────────────────────────────────────────────────────┤
│                                                                              │
│  SOURCE                        │  DATA                 │  FORMAT             │
│  ═════════════════════════════│══════════════════════│════════════════════ │
│                                │                       │                     │
│  registry.build                │  Flag database        │  JSON → Protobuf   │
│  /flag/bazel/{version}/*.json  │  (95 versions)        │  ~50MB embedded    │
│                                │                       │                     │
│  bcr.bazel.build               │  Module metadata      │  JSON → Protobuf   │
│  /modules/{name}/{ver}/...     │  (2600+ modules)      │  ~100MB embedded   │
│                                │                       │                     │
│  Bazel source code             │  Starlark API         │  Manual + Regex    │
│  (deprecation patterns)        │  deprecations         │  ~1MB embedded     │
│                                │                       │                     │
│  Community + Internal          │  Migration recipes    │  YAML files        │
│                                │                       │  ~500KB            │
│                                │                       │                     │
│  GitHub releases               │  Bazel release notes  │  Markdown → Proto  │
│  bazelbuild/bazel              │                       │  ~10MB             │
│                                │                       │                     │
└─────────────────────────────────────────────────────────────────────────────┘
```

## Sync Command Design

```bash
# Sync all data (on connected machine)
bazelbump data sync

# Sync specific data types
bazelbump data sync --flags           # Just flags
bazelbump data sync --bcr             # Just BCR metadata
bazelbump data sync --recipes         # Just recipes

# Sync specific Bazel versions
bazelbump data sync --bazel-versions 8.0.0,9.0.0

# Output to specific directory
bazelbump data sync --output ./data-bundle/

# Show what would be synced
bazelbump data sync --dry-run

# Sync from local registry.build clone
bazelbump data sync --flags-source /path/to/registry.build
```

## Data Structure

```
data/
├── flags/
│   ├── manifest.json              # Metadata about flag data
│   ├── flags.pb                   # All flags (protobuf binary)
│   └── by-version/                # Optional per-version files
│       ├── 8.0.0.pb
│       └── 9.0.0.pb
│
├── bcr/
│   ├── manifest.json
│   ├── modules.pb                 # All module metadata
│   └── by-module/                 # Optional per-module files
│       ├── rules_go.pb
│       └── rules_python.pb
│
├── starlark/
│   ├── manifest.json
│   └── deprecations.pb            # API deprecations
│
├── recipes/
│   ├── manifest.json
│   ├── rules_go_0.46_to_0.50.yaml
│   ├── workspace_to_bzlmod.yaml
│   └── ...
│
└── releases/
    ├── manifest.json
    └── release_notes.pb           # Parsed release notes
```

## Manifest Format

```json
{
  "version": "1.0.0",
  "generated_at": "2025-02-03T10:00:00Z",
  "source": "registry.build",
  "bazel_versions": ["7.0.0", "7.1.0", "8.0.0", "9.0.0"],
  "checksum": "sha256:abc123...",
  "stats": {
    "total_flags": 764,
    "total_changes": 1250
  }
}
```

## Embedding vs External Loading

```go
// Embedded at compile time (for releases)
//go:embed data/flags/flags.pb
var embeddedFlagsData []byte

// External loading (for updates without rebuild)
func LoadFlagDatabase(path string) (*FlagDatabase, error) {
    // 1. Try external path if provided
    if path != "" {
        return loadFromFile(path)
    }

    // 2. Try XDG data directory
    xdgPath := filepath.Join(xdg.DataHome, "bazelbump", "flags.pb")
    if exists(xdgPath) {
        return loadFromFile(xdgPath)
    }

    // 3. Fall back to embedded
    return loadFromBytes(embeddedFlagsData)
}
```

## Sync Implementation

```go
// internal/datasync/sync.go
package datasync

type Syncer struct {
    flagsSource    string  // registry.build path or URL
    bcrSource      string  // BCR API URL
    outputDir      string
    bazelVersions  []string
}

func (s *Syncer) SyncFlags(ctx context.Context) error {
    // 1. For each Bazel version
    for _, version := range s.bazelVersions {
        // 2. Fetch flag data from registry.build
        flags, err := s.fetchFlags(ctx, version)
        if err != nil {
            return fmt.Errorf("fetch flags for %s: %w", version, err)
        }

        // 3. Convert to protobuf
        pb := convertToProto(flags)

        // 4. Write to output
        if err := s.writeProto(pb, version); err != nil {
            return err
        }
    }

    // 5. Generate manifest
    return s.writeManifest()
}

func (s *Syncer) fetchFlags(ctx context.Context, version string) ([]Flag, error) {
    // Fetch from registry.build
    // /flag/bazel/{version}/all.json
    url := fmt.Sprintf("%s/flag/bazel/%s/all.json", s.flagsSource, version)
    // ... HTTP fetch and parse JSON
}
```

## registry.build Integration

We leverage the existing registry.build data at `/Users/adsc/dev/temp/registry.build`:

```go
// internal/datasync/registrybuild.go
package datasync

// RegistryBuildSource reads from a local registry.build clone
type RegistryBuildSource struct {
    basePath string  // e.g., /Users/adsc/dev/temp/registry.build
}

func (r *RegistryBuildSource) GetFlags(version string) ([]Flag, error) {
    path := filepath.Join(r.basePath, "flag", "bazel", version, "all.json")
    data, err := os.ReadFile(path)
    if err != nil {
        return nil, err
    }

    var flags []Flag
    if err := json.Unmarshal(data, &flags); err != nil {
        return nil, err
    }
    return flags, nil
}

func (r *RegistryBuildSource) ListVersions() ([]string, error) {
    entries, err := os.ReadDir(filepath.Join(r.basePath, "flag", "bazel"))
    // ... return version directories
}
```

## Data Commands

```bash
# Show data info
bazelbump data info
# Output:
# DATA SOURCES
# ============
# Flags:     embedded (v1.0.0, 2025-02-01, 95 versions)
# BCR:       external (~/.local/share/bazelbump/bcr.pb, v1.0.0)
# Recipes:   embedded (15 recipes)
# Starlark:  embedded (v1.0.0)

# Import synced data
bazelbump data import ./data-bundle/

# Export current data (for sharing)
bazelbump data export ./export/

# Validate data integrity
bazelbump data validate

# Show flag diff between versions
bazelbump data diff flags 8.0.0 9.0.0
```

## Consequences

### Positive

- Works fully offline with embedded data
- Can update data without rebuilding binary
- Leverages existing registry.build infrastructure
- Clear separation of sync and usage

### Negative

- Binary size increases with embedded data
- Need to maintain sync tooling
- Data can become stale

### Mitigations

- Compress embedded data
- Version manifests to detect staleness
- Provide easy sync commands
- Document update procedures

## References

- registry.build: /Users/adsc/dev/temp/registry.build
- Go embed: https://pkg.go.dev/embed
- XDG Base Directory: https://specifications.freedesktop.org/basedir-spec/basedir-spec-latest.html
