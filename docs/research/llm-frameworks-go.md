# Research: Go LLM/Agent Frameworks

**Date:** 2025-02-03
**Author:** Alberto Cavalcante

## Overview

This document surveys Go frameworks for LLM integration and agent development.

## Frameworks Evaluated

### 1. AWS SDK for Go v2 (Bedrock)

**Package:** `github.com/aws/aws-sdk-go-v2/service/bedrockruntime`
**Documentation:** https://docs.aws.amazon.com/sdk-for-go/v2/developer-guide/

**Description:** Official AWS SDK with native Bedrock support.

**Pros:**
- Official AWS support
- Native Go implementation
- Direct Claude access via Converse API
- IAM/IRSA authentication
- Production-grade

**Cons:**
- AWS-specific
- Requires AWS credentials
- Limited to Bedrock models

**Code Example:**
```go
import (
    "github.com/aws/aws-sdk-go-v2/config"
    "github.com/aws/aws-sdk-go-v2/service/bedrockruntime"
)

cfg, _ := config.LoadDefaultConfig(ctx)
client := bedrockruntime.NewFromConfig(cfg)

output, _ := client.Converse(ctx, &bedrockruntime.ConverseInput{
    ModelId: aws.String("anthropic.claude-3-sonnet-20240229-v1:0"),
    Messages: []types.Message{
        {Role: types.ConversationRoleUser, Content: []types.ContentBlock{...}},
    },
})
```

### 2. LangChainGo

**Repository:** https://github.com/tmc/langchaingo
**Documentation:** https://tmc.github.io/langchaingo/

**Description:** Go port of LangChain, provides abstractions for LLM applications.

**Pros:**
- 10+ provider integrations (OpenAI, Anthropic, Bedrock, Ollama)
- Chains, agents, tools abstractions
- Document loaders, vector stores
- Active community

**Cons:**
- Large dependency footprint
- May be over-abstracted for our needs
- Python-inspired API (not Go-idiomatic)

**Code Example:**
```go
import "github.com/tmc/langchaingo/llms/anthropic"

llm, _ := anthropic.New()
response, _ := llm.Call(ctx, "Explain this Bazel error...")
```

### 3. Google ADK for Go

**Blog:** https://developers.googleblog.com/announcing-the-agent-development-kit-for-go/
**Repository:** (Part of Google Cloud SDK)

**Description:** Google's Agent Development Kit for building AI agents.

**Pros:**
- Official Google support
- Designed for agents (not just completions)
- Tool use built-in
- Vertex AI integration

**Cons:**
- Google Cloud focused
- Newer, less proven
- May require GCP

### 4. Eino (ByteDance)

**Repository:** https://github.com/cloudwego/eino

**Description:** LLM application framework from ByteDance/CloudWeGo.

**Pros:**
- Comprehensive framework
- Multi-agent support
- Built by large-scale users

**Cons:**
- Less documentation in English
- Smaller Western community
- Newer project

### 5. Ollama Go Client

**Repository:** https://github.com/ollama/ollama (API)
**Documentation:** https://github.com/ollama/ollama/blob/main/docs/api.md

**Description:** Simple HTTP API for local Ollama server.

**Pros:**
- Simple REST API
- Works with any HTTP client
- Local/airgap friendly
- No vendor lock-in

**Cons:**
- Basic functionality only
- No advanced agent features
- Manual implementation needed

**Code Example:**
```go
// Simple HTTP client
resp, _ := http.Post("http://localhost:11434/api/generate",
    "application/json",
    strings.NewReader(`{"model":"codellama","prompt":"..."}`))
```

## Provider Comparison

| Provider | Bedrock | OpenAI | Ollama | vLLM |
|----------|---------|--------|--------|------|
| **Authentication** | IAM/IRSA | API Key | None | None/Custom |
| **Models** | Claude, Llama, etc | GPT-4, etc | Open models | Any HF model |
| **Airgap** | Via VPC endpoint | No | **Yes** | **Yes** |
| **Self-hosted** | No | No | **Yes** | **Yes** |
| **Go SDK** | Official | Official | HTTP API | HTTP API |
| **Cost** | Per token | Per token | Hardware only | Hardware only |

## Recommended Approach

### Primary: Direct AWS SDK for Bedrock

For our airgap-with-controlled-egress scenario, we use AWS SDK directly:

```go
// pkg/llm/bedrock/provider.go
type Provider struct {
    client *bedrockruntime.Client
    model  string
}

func (p *Provider) Complete(ctx context.Context, req *CompletionRequest) (*CompletionResponse, error) {
    return p.client.Converse(ctx, convertRequest(req))
}
```

### Secondary: OpenAI-Compatible API

For self-hosted models (Ollama, vLLM, LocalAI), use OpenAI-compatible API:

```go
// pkg/llm/openai/provider.go
type Provider struct {
    endpoint string  // http://localhost:11434/v1 for Ollama
    model    string
    apiKey   string  // Optional
}

func (p *Provider) Complete(ctx context.Context, req *CompletionRequest) (*CompletionResponse, error) {
    // POST to /v1/chat/completions
}
```

### Why Not LangChainGo?

1. **Over-abstraction** - We need simple completion, not chains
2. **Dependency weight** - Brings many transitive deps
3. **Control** - We want direct control over prompts/responses
4. **Simplicity** - Our use case is straightforward

## Interface Design

```go
// pkg/llm/provider.go
type Provider interface {
    Complete(ctx context.Context, req *CompletionRequest) (*CompletionResponse, error)
    Stream(ctx context.Context, req *CompletionRequest) (<-chan Chunk, error)
    Name() string
    Available(ctx context.Context) bool
}

// Implementations:
// - BedrockProvider (AWS)
// - OpenAIProvider (OpenAI-compatible: Ollama, vLLM, LocalAI)
// - NoOpProvider (returns errors, for degraded mode)
```

## Model Recommendations

| Use Case | Bedrock Model | Ollama Model |
|----------|---------------|--------------|
| Error diagnosis | claude-3-5-sonnet | qwen2.5-coder:14b |
| Code generation | claude-3-5-sonnet | qwen2.5-coder:32b |
| Quick tasks | claude-3-haiku | qwen2.5-coder:7b |
| Documentation | claude-3-5-sonnet | llama3:8b |

## References

- AWS Bedrock Go: https://docs.aws.amazon.com/code-library/latest/ug/go_2_bedrock-runtime_code_examples.html
- Bedrock Converse API: https://docs.aws.amazon.com/bedrock/latest/userguide/conversation-inference.html
- Ollama API: https://github.com/ollama/ollama/blob/main/docs/api.md
- OpenAI API Spec: https://platform.openai.com/docs/api-reference
- LangChainGo: https://github.com/tmc/langchaingo
