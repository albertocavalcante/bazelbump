# Vendoring Plan

## Philosophy

bazelbump follows the same zero-dependency philosophy as go-bzlmod:
- **No direct dependencies** in go.mod
- **Vendor essential code** with automated tooling
- **Sync external data** (flags, registry) with reproducible tools
- **Isolate dev tools** in `tools/*/go.mod` with tool directives

## What We Vendor/Sync

### 1. Flag Data from registry.build

**Source:** `/Users/adsc/dev/temp/registry.build/flag/bazel/`
**Destination:** `data/flags/`
**Tool:** `tools/sync-flags/`

This contains flag information for 65+ Bazel versions (0.17.1 to 9.0.0).

**Format:**
```
data/flags/
├── VERSION.json           # Sync metadata
├── versions.json          # List of available versions
└── {version}/
    └── all.json           # All flags for this version
```

**Sync command:**
```bash
just sync-flags                    # Sync all versions
just sync-flags-version 9.0.0      # Sync specific version
```

### 2. Bazel Protos (Optional)

**Source:** `github.com/bazelbuild/bazel` (src/main/protobuf/)
**Destination:** `third_party/bazel/proto/`
**Tool:** `tools/vendor-protos/`

Only if we decide to use Bazel's official proto definitions:
- `bazel_flags.proto` - Flag structure
- `option_filters.proto` - Effect/metadata tags

**Decision:** TBD - may define our own simpler types instead.

### 3. buildtools Parser (Future)

**Source:** `github.com/bazelbuild/buildtools`
**Destination:** `third_party/buildtools/`
**Tool:** `tools/vendor-buildtools/`

For Starlark/BUILD file parsing. Follow go-bzlmod's pattern.

## Vendoring Metadata

Each vendored/synced package includes:

**VERSION.json:**
```json
{
  "source": "registry.build or github URL",
  "ref": "commit hash or version",
  "synced_at": "ISO timestamp",
  "tool": "tools/sync-flags",
  "packages": ["list", "of", "packages"]
}
```

## Tool Isolation

Dev tools live in separate go.mod files:

```
tools/
├── lint/
│   └── go.mod           # golangci-lint, staticcheck, goimports
├── sync-flags/
│   ├── go.mod           # Dependencies for sync tool
│   └── main.go
├── vendor-protos/
│   ├── go.mod
│   └── main.go
└── proto-gen/
    ├── go.mod           # protoc-gen-go, etc.
    └── main.go
```

This keeps the main module dependency-free while allowing tools to use whatever they need.

## Updating Vendored Code

1. **Flags:** Run `just sync-flags` periodically or when new Bazel version released
2. **Protos:** Run `just vendor-protos-tag v9.0.0` when targeting new Bazel
3. **buildtools:** Run `just vendor-buildtools` when parser updates needed

All updates should be committed with clear messages referencing the source version.
