# ADR-003: LLM Provider Abstraction with Bedrock Primary

**Status:** Accepted
**Date:** 2025-02-03
**Deciders:** Alberto Cavalcante

## Context

bazelbump's agent capabilities require LLM integration for:

1. **Error diagnosis** - Explain unknown build errors
2. **Patch generation** - Generate code fixes for deprecations
3. **Documentation** - Generate migration notes
4. **Suggestions** - Recommend fixes for complex scenarios

In our airgap-with-controlled-egress environment:
- AWS Bedrock is accessible (with proper IAM auth)
- Self-hosted models (Ollama, vLLM) are available as fallback
- Direct Anthropic API is NOT accessible

## Decision

1. **AWS Bedrock as primary LLM provider** (Claude models)
2. **Abstract LLM interface** to support multiple backends
3. **Ollama as fallback** for fully disconnected scenarios
4. **OpenAI-compatible API** support for flexibility

## Provider Hierarchy

```
┌─────────────────────────────────────────────────────────────────────────────┐
│                         LLM PROVIDER PRIORITY                                │
├─────────────────────────────────────────────────────────────────────────────┤
│                                                                              │
│   1. AWS Bedrock (Primary)                                                   │
│      └── Claude 3.5 Sonnet / Claude 3 Haiku                                 │
│      └── Auth: IAM Role, IRSA, Environment credentials                      │
│      └── Endpoint: VPC endpoint or public (with egress)                     │
│                                                                              │
│   2. OpenAI-Compatible (Secondary)                                           │
│      └── vLLM, LocalAI, LiteLLM proxy                                       │
│      └── Any model via OpenAI API format                                    │
│      └── Self-hosted within network                                          │
│                                                                              │
│   3. Ollama (Fallback)                                                       │
│      └── Qwen2.5-Coder, CodeLlama, Llama3                                   │
│      └── Runs locally or on internal server                                 │
│      └── No network required                                                 │
│                                                                              │
│   4. None (Degraded Mode)                                                    │
│      └── Deterministic transforms only                                       │
│      └── No LLM-assisted features                                           │
│      └── Still useful for basic upgrades                                    │
│                                                                              │
└─────────────────────────────────────────────────────────────────────────────┘
```

## Interface Design

```go
// pkg/llm/provider.go
package llm

import "context"

// Provider abstracts LLM backends
type Provider interface {
    // Complete generates a completion for the given prompt
    Complete(ctx context.Context, req *CompletionRequest) (*CompletionResponse, error)

    // Stream generates a streaming completion
    Stream(ctx context.Context, req *CompletionRequest) (<-chan StreamChunk, error)

    // Name returns the provider name for logging
    Name() string

    // Available checks if the provider is configured and reachable
    Available(ctx context.Context) bool
}

// CompletionRequest represents an LLM request
type CompletionRequest struct {
    Messages    []Message
    MaxTokens   int
    Temperature float64
    System      string
    // Provider-specific options via protobuf Any
    Options     map[string]any
}

// Message represents a conversation message
type Message struct {
    Role    Role   // User, Assistant, System
    Content string
}

type CompletionResponse struct {
    Content      string
    FinishReason string
    Usage        *Usage
}

type Usage struct {
    PromptTokens     int
    CompletionTokens int
    TotalTokens      int
}
```

## Implementation: Bedrock Provider

```go
// pkg/llm/bedrock/provider.go
package bedrock

import (
    "context"
    "github.com/aws/aws-sdk-go-v2/config"
    "github.com/aws/aws-sdk-go-v2/service/bedrockruntime"
)

type Provider struct {
    client  *bedrockruntime.Client
    model   string
    region  string
}

func New(ctx context.Context, opts ...Option) (*Provider, error) {
    cfg, err := config.LoadDefaultConfig(ctx)
    if err != nil {
        return nil, fmt.Errorf("load AWS config: %w", err)
    }

    p := &Provider{
        model:  "anthropic.claude-3-sonnet-20240229-v1:0",
        region: "us-east-1",
    }
    for _, opt := range opts {
        opt(p)
    }

    p.client = bedrockruntime.NewFromConfig(cfg, func(o *bedrockruntime.Options) {
        o.Region = p.region
    })

    return p, nil
}

func (p *Provider) Complete(ctx context.Context, req *llm.CompletionRequest) (*llm.CompletionResponse, error) {
    // Use Converse API for Claude
    input := &bedrockruntime.ConverseInput{
        ModelId: aws.String(p.model),
        Messages: convertMessages(req.Messages),
        System: []types.SystemContentBlock{
            &types.SystemContentBlockMemberText{Value: req.System},
        },
        InferenceConfig: &types.InferenceConfiguration{
            MaxTokens:   aws.Int32(int32(req.MaxTokens)),
            Temperature: aws.Float32(float32(req.Temperature)),
        },
    }

    output, err := p.client.Converse(ctx, input)
    if err != nil {
        return nil, fmt.Errorf("bedrock converse: %w", err)
    }

    return &llm.CompletionResponse{
        Content:      extractContent(output),
        FinishReason: string(output.StopReason),
        Usage:        convertUsage(output.Usage),
    }, nil
}
```

## Configuration

```yaml
# .bazelbump/config.yaml
llm:
  # Primary: AWS Bedrock
  provider: bedrock
  bedrock:
    region: us-east-1
    model: anthropic.claude-3-5-sonnet-20241022-v2:0
    # Auth via IAM role, IRSA, or env vars (AWS_ACCESS_KEY_ID, etc.)
    # Optional: VPC endpoint
    endpoint: vpce-xxx.bedrock-runtime.us-east-1.vpce.amazonaws.com

  # Fallback chain (tried in order if primary fails)
  fallback:
    - provider: ollama
      endpoint: http://localhost:11434
      model: qwen2.5-coder:14b

    - provider: none  # Degraded mode, deterministic only

  # Request settings
  max_tokens: 4096
  temperature: 0.1  # Low for code generation
  timeout: 60s
```

## Model Recommendations

| Use Case | Bedrock Model | Ollama Fallback |
|----------|---------------|-----------------|
| Error diagnosis | claude-3-5-sonnet | qwen2.5-coder:14b |
| Patch generation | claude-3-5-sonnet | qwen2.5-coder:32b |
| Quick suggestions | claude-3-haiku | qwen2.5-coder:7b |
| Documentation | claude-3-5-sonnet | llama3:8b |

## AWS Authentication

```bash
# Option 1: Environment variables
export AWS_ACCESS_KEY_ID=xxx
export AWS_SECRET_ACCESS_KEY=xxx
export AWS_REGION=us-east-1

# Option 2: IAM Role (EC2, ECS, Lambda)
# Automatic via instance metadata

# Option 3: IRSA (EKS)
# Automatic via service account

# Option 4: AWS Profile
export AWS_PROFILE=my-profile

# Option 5: Web Identity (OIDC)
export AWS_WEB_IDENTITY_TOKEN_FILE=/var/run/secrets/token
export AWS_ROLE_ARN=arn:aws:iam::123456789:role/my-role
```

## Consequences

### Positive

- Bedrock provides high-quality Claude models
- Works within enterprise AWS environments
- Fallback chain ensures graceful degradation
- Interface allows adding new providers easily

### Negative

- AWS dependency for primary LLM
- Bedrock pricing can be significant at scale
- Need to manage AWS credentials

### Mitigations

- Caching: Cache LLM responses for identical queries
- Batching: Combine multiple small requests
- Fallback: Use Ollama for development/testing
- Degraded mode: Core functionality works without LLM

## References

- AWS Bedrock Go SDK: https://docs.aws.amazon.com/sdk-for-go/v2/developer-guide/go_bedrock-runtime_code_examples.html
- Bedrock Claude: https://docs.anthropic.com/en/api/claude-on-amazon-bedrock
- Bedrock Converse API: https://docs.aws.amazon.com/bedrock/latest/userguide/bedrock-runtime_example_bedrock-runtime_Converse_AnthropicClaude_section.html
- Ollama: https://ollama.ai/
