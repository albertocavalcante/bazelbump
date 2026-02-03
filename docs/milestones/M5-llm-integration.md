# Milestone 5: LLM Integration

**Target:** Weeks 8-9
**Status:** Not Started
**Depends On:** M4 (Transform Engine)

## Objective

Implement LLM provider abstraction with Bedrock as primary and Ollama as fallback.

## Deliverables

### 1. LLM Provider Interface

```go
// pkg/llm/provider.go
package llm

import "context"

type Provider interface {
    // Complete generates a completion
    Complete(ctx context.Context, req *CompletionRequest) (*CompletionResponse, error)

    // Stream generates a streaming completion
    Stream(ctx context.Context, req *CompletionRequest) (<-chan StreamChunk, error)

    // Name returns provider name
    Name() string

    // Available checks if provider is reachable
    Available(ctx context.Context) bool
}

type CompletionRequest struct {
    Messages    []Message
    System      string
    MaxTokens   int
    Temperature float64
    StopWords   []string
}

type Message struct {
    Role    Role
    Content string
}

type Role string
const (
    RoleUser      Role = "user"
    RoleAssistant Role = "assistant"
    RoleSystem    Role = "system"
)

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

type StreamChunk struct {
    Content string
    Done    bool
    Error   error
}
```

### 2. Bedrock Provider

```go
// pkg/llm/bedrock/provider.go
package bedrock

import (
    "github.com/aws/aws-sdk-go-v2/config"
    "github.com/aws/aws-sdk-go-v2/service/bedrockruntime"
)

type Provider struct {
    client *bedrockruntime.Client
    model  string
    region string
}

type Options struct {
    Region string
    Model  string
}

func New(ctx context.Context, opts Options) (*Provider, error) {
    cfg, err := config.LoadDefaultConfig(ctx, config.WithRegion(opts.Region))
    if err != nil {
        return nil, fmt.Errorf("load AWS config: %w", err)
    }

    return &Provider{
        client: bedrockruntime.NewFromConfig(cfg),
        model:  opts.Model,
        region: opts.Region,
    }, nil
}

func (p *Provider) Complete(ctx context.Context, req *llm.CompletionRequest) (*llm.CompletionResponse, error) {
    input := &bedrockruntime.ConverseInput{
        ModelId:  aws.String(p.model),
        Messages: p.convertMessages(req.Messages),
        System:   p.convertSystem(req.System),
        InferenceConfig: &types.InferenceConfiguration{
            MaxTokens:   aws.Int32(int32(req.MaxTokens)),
            Temperature: aws.Float32(float32(req.Temperature)),
        },
    }

    output, err := p.client.Converse(ctx, input)
    if err != nil {
        return nil, fmt.Errorf("bedrock converse: %w", err)
    }

    return p.convertResponse(output), nil
}
```

### 3. OpenAI-Compatible Provider (Ollama, vLLM)

```go
// pkg/llm/openai/provider.go
package openai

import (
    "net/http"
)

type Provider struct {
    endpoint string
    model    string
    apiKey   string
    client   *http.Client
}

type Options struct {
    Endpoint string  // e.g., http://localhost:11434/v1
    Model    string  // e.g., qwen2.5-coder:14b
    APIKey   string  // optional for Ollama
}

func New(opts Options) *Provider {
    return &Provider{
        endpoint: opts.Endpoint,
        model:    opts.Model,
        apiKey:   opts.APIKey,
        client:   &http.Client{Timeout: 120 * time.Second},
    }
}

func (p *Provider) Complete(ctx context.Context, req *llm.CompletionRequest) (*llm.CompletionResponse, error) {
    // POST to /v1/chat/completions
    body := openaiRequest{
        Model:       p.model,
        Messages:    p.convertMessages(req.Messages),
        MaxTokens:   req.MaxTokens,
        Temperature: req.Temperature,
    }
    // ...
}
```

### 4. Provider Chain (Fallback)

```go
// pkg/llm/chain.go
type Chain struct {
    providers []Provider
}

func NewChain(providers ...Provider) *Chain {
    return &Chain{providers: providers}
}

func (c *Chain) Complete(ctx context.Context, req *CompletionRequest) (*CompletionResponse, error) {
    var lastErr error
    for _, p := range c.providers {
        if !p.Available(ctx) {
            continue
        }
        resp, err := p.Complete(ctx, req)
        if err == nil {
            return resp, nil
        }
        lastErr = err
        log.Warn("provider failed, trying next", "provider", p.Name(), "error", err)
    }
    return nil, fmt.Errorf("all providers failed: %w", lastErr)
}
```

### 5. NoOp Provider (Degraded Mode)

```go
// pkg/llm/noop/provider.go
type Provider struct{}

func (p *Provider) Complete(ctx context.Context, req *CompletionRequest) (*CompletionResponse, error) {
    return nil, ErrLLMUnavailable
}

func (p *Provider) Available(ctx context.Context) bool {
    return false
}

var ErrLLMUnavailable = errors.New("LLM provider not available")
```

### 6. Configuration

```yaml
# .bazelbump/config.yaml
llm:
  # Primary provider
  provider: bedrock
  bedrock:
    region: us-east-1
    model: anthropic.claude-3-5-sonnet-20241022-v2:0
    # endpoint: vpce-xxx.bedrock.us-east-1.vpce.amazonaws.com  # Optional VPC endpoint

  # Fallback chain
  fallback:
    - provider: openai
      endpoint: http://localhost:11434/v1
      model: qwen2.5-coder:14b

    - provider: none

  # Request defaults
  defaults:
    max_tokens: 4096
    temperature: 0.1
    timeout: 60s
```

### 7. Provider Factory

```go
// pkg/llm/factory.go
func NewFromConfig(ctx context.Context, cfg *config.LLMConfig) (Provider, error) {
    var providers []Provider

    // Primary
    primary, err := createProvider(ctx, cfg.Provider, cfg)
    if err != nil {
        return nil, err
    }
    providers = append(providers, primary)

    // Fallbacks
    for _, fb := range cfg.Fallback {
        p, err := createProvider(ctx, fb.Provider, &fb)
        if err != nil {
            log.Warn("failed to create fallback provider", "provider", fb.Provider, "error", err)
            continue
        }
        providers = append(providers, p)
    }

    return NewChain(providers...), nil
}
```

## Package Structure

```
pkg/
├── llm/
│   ├── provider.go        # Interface
│   ├── chain.go           # Fallback chain
│   ├── factory.go         # Provider factory
│   ├── bedrock/
│   │   ├── provider.go
│   │   └── provider_test.go
│   ├── openai/
│   │   ├── provider.go
│   │   └── provider_test.go
│   └── noop/
│       └── provider.go
```

## Tasks

- [ ] Define provider interface
- [ ] Implement Bedrock provider
- [ ] Implement OpenAI-compatible provider
- [ ] Implement NoOp provider
- [ ] Implement fallback chain
- [ ] Implement provider factory
- [ ] Add configuration support
- [ ] Add streaming support
- [ ] Add retry logic with backoff
- [ ] Add usage tracking/logging
- [ ] Test with real Bedrock
- [ ] Test with Ollama
- [ ] Document authentication setup

## Acceptance Criteria

1. Bedrock provider works with IAM auth
2. Ollama provider works locally
3. Fallback chain tries providers in order
4. Graceful degradation when all providers fail
5. Streaming works for long responses
6. Errors are informative

## AWS Authentication

Supported methods (in order):
1. Environment variables (`AWS_ACCESS_KEY_ID`, `AWS_SECRET_ACCESS_KEY`)
2. IAM instance role (EC2, ECS)
3. IRSA (EKS)
4. Web Identity (OIDC)
5. AWS Profile (`AWS_PROFILE`)

## Testing

```bash
# Test Bedrock
AWS_PROFILE=my-profile go test ./pkg/llm/bedrock/...

# Test Ollama (requires running Ollama)
ollama serve &
go test ./pkg/llm/openai/...
```

## Notes

- Keep provider implementations simple
- Don't over-abstract (we only need completion)
- Handle rate limits gracefully
- Log usage for cost tracking
- Consider caching for identical requests
