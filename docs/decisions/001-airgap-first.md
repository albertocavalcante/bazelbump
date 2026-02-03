# ADR-001: Airgap-First Architecture

**Status:** Accepted
**Date:** 2025-02-03
**Deciders:** Alberto Cavalcante

## Context

bazelbump is a tool for managing Bazel version upgrades in enterprise environments. Many enterprise users operate in restricted network environments where:

1. Direct internet access is blocked or heavily restricted
2. Only approved services (like AWS Bedrock) are accessible via controlled egress
3. External registries (BCR, GitHub) are not directly reachable
4. All dependencies must be bundled or pre-synced

## Decision

**bazelbump will be designed with an "airgap-first" mindset.**

This means:

1. **All data required for analysis must be embeddable/bundleable**
   - Flag database (from registry.build)
   - BCR module metadata
   - Starlark deprecation patterns
   - Migration recipes

2. **No runtime network calls to external services**
   - BCR queries use embedded data
   - GitHub API calls use embedded data
   - Version information uses embedded data

3. **Controlled egress is supported**
   - AWS Bedrock for LLM (with proper IAM auth)
   - Other approved LLM endpoints
   - Self-hosted services (Ollama, vLLM)

4. **Sync tooling for connected environments**
   - `bazelbump data sync` updates embedded data
   - Runs on a connected machine
   - Produces artifacts that can be transferred to airgap environment

## Network Access Model

```
┌─────────────────────────────────────────────────────────────────────────────┐
│                    AIRGAP WITH CONTROLLED EGRESS                             │
├─────────────────────────────────────────────────────────────────────────────┤
│                                                                              │
│   BLOCKED (No Access)                    ALLOWED (Controlled Egress)        │
│   ═══════════════════                    ══════════════════════════         │
│                                                                              │
│   ✗ api.github.com                       ✓ AWS Bedrock (Claude)             │
│   ✗ bcr.bazel.build                      ✓ Self-hosted Ollama               │
│   ✗ registry.build                       ✓ Self-hosted vLLM                 │
│   ✗ releases.bazel.build                 ✓ Internal PostgreSQL              │
│   ✗ npm/pip/cargo registries             ✓ Internal artifact storage        │
│                                                                              │
│   Solution: SYNC + EMBED                 Solution: DIRECT ACCESS            │
│   • Pre-sync data on connected machine   • Authenticate via IAM/IRSA        │
│   • Bundle into binary or data files     • Use standard SDK auth            │
│   • Transfer via approved channels       • No special handling needed       │
│                                                                              │
└─────────────────────────────────────────────────────────────────────────────┘
```

## Consequences

### Positive

- Works in highly restricted enterprise environments
- No surprise network calls that could fail or be blocked
- Predictable behavior regardless of network conditions
- Security teams can audit all external communications
- Data freshness is explicit and controlled

### Negative

- Embedded data can become stale
- Binary size increases with embedded data
- Requires sync process for updates
- Two-step workflow for updates (sync → transfer → use)

### Neutral

- Need to maintain sync tooling
- Need to version embedded data
- Need clear documentation on data freshness

## Implementation

### Data Embedding

```go
// Embedded at compile time
//go:embed data/flags.pb
var flagsData []byte

//go:embed data/bcr-modules.pb
var bcrData []byte

//go:embed data/starlark-deprecations.pb
var starlarkData []byte
```

### Sync Command

```bash
# On connected machine
bazelbump data sync --output ./data-bundle/
bazelbump data sync --flags --bcr --recipes

# Transfer data-bundle/ to airgap environment
# Then update bazelbump's data directory
bazelbump data import ./data-bundle/
```

### LLM Configuration

```yaml
# Airgap with Bedrock access
llm:
  provider: bedrock
  region: us-east-1
  model: anthropic.claude-3-sonnet-20240229-v1:0
  # Auth via IAM role, IRSA, or environment credentials

# Fully disconnected (self-hosted)
llm:
  provider: ollama
  endpoint: http://localhost:11434
  model: qwen2.5-coder:14b
```

## References

- AWS Bedrock in VPC: https://docs.aws.amazon.com/bedrock/latest/userguide/vpc-interface-endpoints.html
- Go embed directive: https://pkg.go.dev/embed
