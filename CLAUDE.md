# CLAUDE.md

This file provides guidance to Claude Code (claude.ai/code) when working with code in this repository.

## Project Overview

Ogem is a production-ready AI proxy server that provides unified access to multiple AI language models through an OpenAI-compatible API. It acts as an intelligent gateway between clients and various AI providers (OpenAI, Claude, Gemini, etc.) with enterprise features including routing, caching, multi-tenancy, rate limiting, and security.

## Development Commands

### Building and Running
```bash
# Build the binary
go build -o ogem cmd/main.go

# Run locally (uses config.yaml by default)
go run cmd/main.go

# Run with custom config
go run cmd/main.go -config path/to/config.yaml

# Run with Docker
docker build -t ogem:latest .
docker run -p 8080:8080 -e OPEN_GEMINI_API_KEY="your-key" ogem:latest
```

### Testing
```bash
# Run all tests
go test ./...

# Run tests with race detection
go test -race ./...

# Run tests for specific package
go test ./cache
go test ./routing
go test ./provider

# Run comprehensive test coverage analysis
./scripts/test-coverage.sh

# Run specific test categories
./scripts/test-coverage.sh unit          # Unit tests only
./scripts/test-coverage.sh integration   # Integration tests only
./scripts/test-coverage.sh e2e          # End-to-end tests only

# Run single test
go test ./cache -run TestCacheHit
```

### Linting
```bash
# Requires golangci-lint to be installed
golangci-lint run

# Auto-fix issues where possible
golangci-lint run --fix
```

## Architecture Overview

### Request Flow

1. **Entry Point** (`server/server.go`): HTTP server receives OpenAI-compatible requests
2. **Authentication** (`auth/`): Validates API keys (master keys, virtual keys, JWT, OAuth2)
3. **Multi-Tenancy** (`tenancy/`): Identifies tenant and applies tenant-specific quotas/limits
4. **Security** (`security/`): Rate limiting, PII masking, audit logging
5. **Cache Layer** (`cache/`): Checks for cached responses (exact, semantic, token-based)
6. **Router** (`routing/`): Selects optimal AI provider endpoint based on latency, cost, load
7. **Provider** (`provider/`): Transforms request to provider-specific format and executes
8. **Response**: Flows back through cache, monitoring, cost tracking

### Core Packages

**provider/**: Abstract interface `AiEndpoint` with implementations for each AI provider
- `openai/`: Direct OpenAI API integration
- `claude/`: Anthropic Claude API integration
- `vertex/`: Google Vertex AI (GCP-hosted Gemini models)
- `studio/`: Google AI Studio (Gemini API)
- `vclaude/`: Claude models via Vertex AI
- Each provider implements the same interface but handles provider-specific protocols

**routing/**: Intelligent request routing with multiple strategies
- `StrategyLatency`: Routes to fastest endpoint (default)
- `StrategyCost`: Routes to cheapest endpoint
- `StrategyRoundRobin`: Even distribution
- `StrategyLeastConnections`: Routes to endpoint with fewest active requests
- `StrategyPerformanceBased`: Weighted combination of cost, latency, success rate, load
- `StrategyAdaptive`: Dynamically switches strategies based on conditions
- Includes circuit breaker pattern to handle failing endpoints

**cache/**: Multi-strategy caching system
- `StrategyExact`: Caches exact request matches
- `StrategySemantic`: Semantic similarity matching
- `StrategyToken`: Token-level similarity with fuzzy matching
- `StrategyHybrid`: Combines multiple strategies
- `StrategyAdaptive`: Dynamically adjusts based on hit rates
- Supports memory and Redis backends

**auth/**: Authentication system with multiple methods
- Master API key authentication (OPEN_GEMINI_API_KEY)
- Virtual keys with granular permissions and spend limits
- JWT authentication
- OAuth2 integration
- Priority: virtual keys checked first, then master key

**tenancy/**: Multi-tenant isolation and resource management
- Hierarchical tenant structure (enterprise → team → user)
- Per-tenant quotas, rate limits, cost budgets
- Usage tracking and billing integration
- Tenant-specific configuration overrides

**security/**: Security features
- Rate limiting (per tenant, per key)
- PII masking in logs and responses
- Audit logging of all requests
- Security manager coordinates all security policies

**monitoring/**: Observability and metrics
- Prometheus metrics export
- OpenTelemetry integration
- Datadog integration
- Custom metrics backend support

**state/**: Distributed state management
- Memory-based state (single instance)
- Valkey/Redis state (distributed deployment)
- Used for rate limiting, quotas, request tracking

**cost/**: Cost calculation and tracking
- Per-model pricing (input tokens, output tokens, reasoning tokens)
- Real-time cost tracking per request
- Budget enforcement
- Pricing updated for 2025 models

### Configuration System

Configuration is loaded from YAML files with environment variable overrides:
- `CONFIG_SOURCE`: Path or URL to config file (default: config.yaml)
- `CONFIG_TOKEN`: Bearer token for authenticated remote configs
- Provider credentials via environment variables (OPENAI_API_KEY, CLAUDE_API_KEY, etc.)
- `VALKEY_ENDPOINT`: Redis-compatible endpoint for distributed state

Configuration supports:
- Per-provider, per-region, per-model rate limits (RPM, TPM)
- Model aliases (e.g., "finetuned-flash" → "projects/.../endpoints/...")
- Default region configuration that propagates to all regions
- Custom endpoints with OpenAI-compatible protocol

### Provider Architecture

Each provider implementation follows the `AiEndpoint` interface but handles different:
- Authentication mechanisms (API keys, GCP credentials, etc.)
- Request/response formats
- Model naming conventions
- Error handling patterns

Providers are grouped by region for multi-region support. The router selects optimal provider/region combination based on configured strategy.

### State Management

Rate limiting and quota tracking requires shared state across instances:
- **Single instance**: Uses in-memory state (no external dependencies)
- **Multi-instance**: Uses Valkey (Redis-compatible) for distributed state
- State includes: token counters, request counters, cache entries, session data

### Batch Processing

OpenAI batch API integration for 50% cost reduction:
- Add `@batch` suffix to model name (e.g., "gpt-4o@batch")
- Requests are queued and submitted in batches
- Batches processed every 10 seconds or at 50,000 requests
- Identical requests deduplicated by request_id
- Response waits for batch completion (may timeout and retry)

## Comment Standards

**Remember: Comments must explain WHY, not WHAT**

- Comments should explain reasons, rationale, or provide examples
- Never describe what code does - if it can be inferred by LLMs/coding assistants, don't comment it
- Focus on business constraints using "need BUT constraint" pattern
- Don't prefix with subjects ("CallSomething does..." → "Does...")
- Only comment things that cannot be inferred from code itself
- Explain the reasoning behind design decisions, not the implementation

## Examples of Good Comments
```go
// Rate limiting disabled during batch processing because upstream API
// has separate quota pools for batch vs real-time requests
client.DisableRateLimit()

// Virtual keys validated before master key to prioritize user permissions
// over administrative access patterns
if virtualKey != nil { ... }
```

## Examples of Bad Comments
```go
// Set headers for SSE streaming
httpResponse.Header().Set("Content-Type", "text/event-stream")

// Create new virtual key
key := &VirtualKey{...}
```

## Key Design Patterns

### Rate Limiting Strategy
Rate limits are applied at multiple levels with priority order:
1. Virtual key limits (most specific)
2. Tenant limits (organization-level)
3. Model limits (provider quota)
4. Global limits (system capacity)

Rate limiting tracks both RPM (requests per minute) and TPM (tokens per minute). TPM includes both input and output tokens.

### Error Handling and Fallback
When a provider request fails:
1. Router marks endpoint as unhealthy (circuit breaker)
2. If fallback models specified in request (comma-separated), tries next model
3. If all models fail, returns error to client
4. Circuit breaker automatically re-enables endpoints after timeout

### Request Deduplication
For batch processing, identical requests receive the same `request_id` to prevent duplicate processing. Request signature is computed from:
- Model name
- Messages content
- Temperature and other generation parameters
- Excluding: stream flag, user ID, custom metadata

### Cost Tracking
Cost is calculated per request based on:
- Model-specific pricing (input/output/reasoning tokens)
- Actual token usage from provider response
- Tracked at tenant and virtual key level
- Can enforce budget limits (soft warnings + hard limits)

## Important Constraints

### Provider SDK Compatibility
- OpenAI: Uses official SDK, fully supported
- Claude: Fixed compatibility with Anthropic SDK 1.4.0 (see provider/claude/claude.go)
- Vertex AI: Fixed compatibility with Google genai SDK (see provider/vertex/vertex.go)
- Legacy SDKs for other providers may need updates

### Model Naming
Legacy model names are automatically mapped to current versions:
- "gpt-4o" → "gpt-4.5-turbo"
- "claude-3.5-sonnet" → "claude-4-sonnet"
- "gemini-1.5-pro" → "gemini-2.5-pro"

Pricing reflects the actual model used, not the requested alias.

### Valkey vs Redis
The project uses Valkey (open-source Redis fork) because Redis changed to a non-open-source license. Valkey is protocol-compatible with Redis, so existing Redis clients work without modification.

### Default Region Behavior
The "default" region in configuration is a template that copies models to all actual regions. It is not itself a valid region for routing - at least one real region (e.g., "us-central1") must be defined.

## Testing Guidelines

- Unit tests should mock external dependencies (provider APIs, Redis)
- Integration tests require API keys set as environment variables
- E2E tests validate complete request flows
- Use `testing/testutils` for common test utilities
- Mock implementations available in `state/memory_test.go`, `auth/memory.go`
- Test coverage target: 80%+
