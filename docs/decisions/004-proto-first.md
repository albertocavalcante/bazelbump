# ADR-004: Proto-First Type Definitions

**Status:** Accepted
**Date:** 2025-02-03
**Deciders:** Alberto Cavalcante

## Context

bazelbump needs well-defined types for:

1. **Analysis results** - Flags, deprecations, issues
2. **Migration plans** - Transforms, patches, steps
3. **Configuration** - Settings, profiles, recipes
4. **Data exchange** - Cache, snapshots, sync artifacts

We need to choose between:
- Plain Go structs with JSON tags
- Protocol Buffers (protobuf)
- Other serialization formats

## Decision

**All core types will be defined in Protocol Buffers first, then generated to Go.**

This is the "proto-first" approach used successfully in go-bzlmod.

## Rationale

### Why Protocol Buffers?

1. **Schema as documentation**
   - Proto files are self-documenting contracts
   - Clear field types, optionality, defaults
   - Comments become documentation

2. **Serialization flexibility**
   - Binary format for efficiency (embedded data)
   - JSON for human readability (configs)
   - Both from same definition

3. **Forward compatibility**
   - Add fields without breaking existing data
   - Field numbers provide stable identity
   - Unknown fields are preserved

4. **Code generation**
   - Consistent Go structs from protos
   - Validation can be generated
   - Less boilerplate, fewer typos

5. **Cross-language potential**
   - Future: Python bindings
   - Future: TypeScript SDK
   - Same protos generate all

### Proto Organization

```
proto/
├── bazelbump/v1/                    # Our domain types
│   ├── flag.proto                   # Flag deprecation types
│   ├── version.proto                # Version info, constraints
│   ├── analysis.proto               # Analysis request/response
│   ├── transform.proto              # Transform definitions
│   ├── bazelrc.proto                # .bazelrc representation
│   ├── module.proto                 # MODULE.bazel types
│   ├── starlark.proto               # Starlark API deprecations
│   ├── snapshot.proto               # Snapshot/rollback types
│   ├── preflight.proto              # Pre-flight check types
│   ├── score.proto                  # Readiness score types
│   ├── path.proto                   # Upgrade path types
│   ├── cache.proto                  # Cache impact analysis
│   ├── workflow.proto               # Workflow state types
│   ├── llm.proto                    # LLM request/response
│   ├── plugin.proto                 # Plugin manifest
│   └── recipe.proto                 # Migration recipe types
│
├── vendor/                          # Vendored external protos
│   └── bazel/                       # Bazel's own protos (if needed)
│
├── buf.yaml                         # Buf module config
└── buf.gen.yaml                     # Code generation config
```

### Example Proto

```protobuf
// proto/bazelbump/v1/flag.proto
syntax = "proto3";
package bazelbump.v1;

option go_package = "github.com/user/bazelbump/gen/go/bazelbump/v1;bazelbumpv1";

import "google/protobuf/timestamp.proto";

// FlagChange represents a flag that changed between Bazel versions
message FlagChange {
  // The flag name (e.g., "--incompatible_enable_bzlmod")
  string name = 1;

  // Type of change
  FlagChangeType type = 2;

  // Bazel version where this change occurs
  string version = 3;

  // Human-readable description
  string description = 4;

  // Migration guidance
  string migration = 5;

  // Links to relevant documentation or issues
  repeated string references = 6;

  // Old default value (if changed)
  optional string old_default = 7;

  // New default value (if changed)
  optional string new_default = 8;

  // Replacement flag (if renamed/replaced)
  optional string replacement = 9;
}

enum FlagChangeType {
  FLAG_CHANGE_TYPE_UNSPECIFIED = 0;
  FLAG_CHANGE_TYPE_REMOVED = 1;        // Flag removed entirely
  FLAG_CHANGE_TYPE_RENAMED = 2;        // Flag renamed
  FLAG_CHANGE_TYPE_DEFAULT_CHANGED = 3; // Default value changed
  FLAG_CHANGE_TYPE_DEPRECATED = 4;      // Deprecated but still works
  FLAG_CHANGE_TYPE_BEHAVIOR_CHANGED = 5; // Semantics changed
}

// FlagDatabase contains all known flag changes
message FlagDatabase {
  // Version of this database
  string version = 1;

  // When this database was generated
  google.protobuf.Timestamp generated_at = 2;

  // All flag changes
  repeated FlagChange changes = 3;

  // Source of the data
  string source = 4;
}
```

### Buf Configuration

```yaml
# buf.yaml
version: v2
name: buf.build/bazelbump/bazelbump
deps:
  - buf.build/googleapis/googleapis
lint:
  use:
    - DEFAULT
breaking:
  use:
    - FILE
```

```yaml
# buf.gen.yaml
version: v2
plugins:
  - remote: buf.build/protocolbuffers/go
    out: gen/go
    opt:
      - paths=source_relative
```

### Build Integration

```makefile
# Makefile
.PHONY: generate proto

# Generate all code from protos
generate: proto
	@echo "Generating Go code from protos..."
	buf generate

# Lint protos
proto-lint:
	buf lint

# Check for breaking changes
proto-breaking:
	buf breaking --against '.git#branch=main'

# Format protos
proto-format:
	buf format -w
```

## Consequences

### Positive

- Single source of truth for types
- Self-documenting schemas
- Efficient binary serialization
- Language-agnostic definitions
- Forward compatible data formats

### Negative

- Learning curve for protobuf
- Additional build step (buf generate)
- Slightly more verbose than raw Go structs
- Need to keep protos and usage in sync

### Mitigations

- Use buf for modern protobuf tooling
- Generate code in CI to catch drift
- Provide good examples and documentation

## Implementation

### Directory Structure

```
bazelbump/
├── proto/
│   └── bazelbump/v1/
│       └── *.proto
├── gen/
│   └── go/
│       └── bazelbump/v1/
│           └── *.pb.go
├── buf.yaml
├── buf.gen.yaml
└── buf.lock
```

### Usage in Go

```go
package main

import (
    pb "github.com/user/bazelbump/gen/go/bazelbump/v1"
    "google.golang.org/protobuf/encoding/protojson"
    "google.golang.org/protobuf/proto"
)

func main() {
    flag := &pb.FlagChange{
        Name:        "--incompatible_enable_bzlmod",
        Type:        pb.FlagChangeType_FLAG_CHANGE_TYPE_REMOVED,
        Version:     "9.0.0",
        Description: "Bzlmod is now always enabled",
    }

    // Binary serialization (for embedding)
    data, _ := proto.Marshal(flag)

    // JSON serialization (for configs)
    json, _ := protojson.Marshal(flag)
}
```

## References

- Buf: https://buf.build/
- Protocol Buffers: https://protobuf.dev/
- go-bzlmod proto approach: /Users/adsc/dev/ws/go-bzlmod
