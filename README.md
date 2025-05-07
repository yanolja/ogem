# Ogem

Ogem is a unified proxy server that provides access to multiple AI language models through an OpenAI-compatible API interface. It supports OpenAI, Google's Gemini (both Studio and Vertex AI), and Anthropic's Claude models.

## Features

- OpenAI API-compatible interface
- Support for multiple AI providers:
  - OpenAI (e.g., GPT-4, GPT-3.5)
  - Google Gemini (e.g., Gemini 1.5 Flash, Pro)
  - Anthropic Claude (e.g., Claude 3.5 Opus, Sonnet, Haiku)
- Smart routing based on latency
- Rate limiting and quota management
- Response caching for deterministic requests
- Batch processing support
- Regional endpoint selection
- Multi-provider fallback support

## Quick Start

### Using Docker

```bash
# Pull and run the latest version
docker pull ynext/ogem:latest
docker run -p 8080:8080 \
  -e OPEN_GEMINI_API_KEY="your-api-key" \
  -e CONFIG_SOURCE="path-or-url-to-config" \
  ynext/ogem:latest

# Or use a specific version
docker pull ynext/ogem:0.0.1
```

### Building from Source

```bash
go run cmd/main.go
```

## Configuration

Configuration can be provided through a local file or remote URL using the `CONFIG_SOURCE` environment variable.

Example config.yaml:
```yaml
# Amount of time to wait before retrying the request when there are no available endpoints due to rate limiting.
retry_interval: "1m"

# How frequently to check the health of the providers. If you don't want to check the health, set it to 0.
ping_interval: "1h"

# Slack webhook URL for receiving API schema change notifications
# Create a webhook URL at: https://api.slack.com/apps -> Your App -> Incoming Webhooks
slack_webhook_url: "https://hooks.slack.com/services/YOUR/WEBHOOK/URL"

providers:
  openai:
    regions:
      # For providers that does not support multiple regions, you can use the provider name as the region name.
      openai:
        models:
          - name: "gpt-4"
            rate_key: "gpt-4"
            rpm: 10_000
            tpm: 1_000_000
  vertex:
    regions:
      default:
        # Models listed under `default` serve as a template and will be automatically copied to all regions.
        # However, `default` itself is not a valid region - you must define at least one actual region
        # (like 'us-central1') for the provider to work.
        #
        # Example:
        # default:
        #   models: [model-a, model-b]  # These will be copied to all regions
        # us-central1:                  # This is a real region
        #   models: [model-c]           # Final models: model-a, model-b, model-c
        models:
          - name: "gemini-1.5-flash"
            rate_key: "gemini-1.5-flash"
            rpm: 200
            tpm: 4_000_000
      us-central1:
        models:
          - name: "gemini-1.5-pro"
            rate_key: "gemini-1.5-pro"
            rpm: 60
            tpm: 4_000_000
```

## Providers and Models

Ogem supports multiple AI providers through different integration methods:

- **openai**: Direct integration with OpenAI's API
  - Supports: GPT-4, GPT-3.5, and other OpenAI models
  - Requires: OPENAI_API_KEY

- **studio**: Google's Gemini API (via AI Studio)
  - Supports: Gemini 1.5 Pro, Gemini 1.5 Flash
  - Requires: GENAI_STUDIO_API_KEY

- **vertex**: Google Cloud's Vertex AI platform
  - Supports: Gemini models, custom/finetuned models
  - Requires: GOOGLE_CLOUD_PROJECT and GCP authentication

- **claude**: Direct integration with Anthropic's Claude API
  - Supports: Claude 3 Opus, Sonnet, Haiku
  - Requires: CLAUDE_API_KEY

- **vclaude**: Claude models via Vertex AI
  - Supports: Claude models deployed on GCP
  - Requires: GOOGLE_CLOUD_PROJECT and GCP authentication

- **custom**: Custom endpoint
  - Supports: Any API that is OpenAI-compatible
  - Requires: BASE_URL, PROTOCOL, API_KEY_ENV

### Using Custom Endpoint

For custom endpoints, you can specify the base URL, protocol, and API key environment variable.

```yaml
providers:
  custom:  # Choose any name for the custom provider
    base_url: https://api.example.com/v1
    protocol: openai  # Only openai protocol is supported for custom endpoints
    api_key_env: EXAMPLE_API_KEY
    regions:
      custom:  # This region name must match the provider name
        models:
          - name: some-model
            rate_key: some-model
            rpm: 10_000
            tpm: 30_000_000
  another:
    base_url: https://api.another.com/v1
    protocol: openai
    api_key_env: ANOTHER_API_KEY
    regions:
      another:
        models:
          - name: your-model-name
            rate_key: your-rate-key
```

For the API key, it is not allowed to specify any in the config.yaml file. Instead, you should set it as an environment variable and set the variable name in the `api_key_env` field.
Currently, only OpenAI protocol is supported for custom endpoints.

### Using Finetuned Models

For custom or finetuned models on Vertex AI, you can map the full endpoint path to a friendly name:

```yaml
models:
  - name: "projects/1234567890123/locations/us-central1/endpoints/45678901234567890123"
    other_names:
      - "finetuned-flash"    # This becomes the model name you use in API calls
    rate_key: "gemini-1.5-flash"
    rpm: 200    # Requests per minute limit
    tpm: 4_000_000    # Tokens per minute limit
```

You can then use `finetuned-flash` in your API calls instead of the full endpoint path.

## Rate Limiting and Quotas

Each model configuration includes rate limiting parameters:

- `rpm`: Requests Per Minute limit
- `tpm`: Tokens Per Minute limit (total tokens including both input and output)

Example configuration:
```yaml
models:
  - name: "gemini-1.5-pro"
    rate_key: "gemini-1.5-pro"
    rpm: 60      # Maximum 60 requests per minute
    tpm: 4000000 # Maximum 4 million tokens per minute
```

## State Management with Valkey (Redis-compatible)

Ogem can use Valkey for distributed state management, which is recommended for multi-instance deployments:

- **Purpose**: Manages rate limiting, quotas, and request caching across multiple Ogem instances
- **Configuration**: Set via `VALKEY_ENDPOINT` environment variable
- **Format**: `localhost:6379`
- **Optional**: If not configured, Ogem will use in-memory storage (suitable for single-instance deployments)

Example configuration:
```bash
export VALKEY_ENDPOINT="localhost:6379"
```

The reasons why we use Valkey instead of Redis are:
- Redis is not open source anymore so that it's not suitable for self-hosted deployments (https://github.com/redis/redis/pull/13157)
- Valkey is Redis-compatible so that you can migrate to Valkey easily

## Batch Processing

Batch processing is a cost-optimization feature that uses OpenAI's batch API to reduce costs. Here's how it works:

### How It Works
1. Add a batch model to the config (e.g., `gpt-4o@batch`).

```yaml
models:
  - name: "gpt-4o@batch"
    rate_key: "gpt-4o@batch"
    rpm: 10_000
    tpm: 30_000_000
```

2. When you send a request with a `@batch` suffix (e.g., `gpt-4o@batch`):
   - Your request joins a batch queue
   - Batches are processed every 10 seconds or when 50,000 requests accumulate
   - The request waits for the batch to complete

3. Response Behavior:
   - The request blocks until the batch is completed
   - If the batch completes within your request timeout: You get results
   - If timeout occurs: You can retry with the same request
   - Each identical request gets the same `request_id` internally, preventing duplicate processing

### Usage Example
```json
{
  "model": "gpt-4o@batch",
  "messages": [
    [{"role": "user", "content": "Hello! How can you help me today?"}]
  ]
}
```

### Benefits
- Up to 50% cost reduction using OpenAI's batch API pricing
- Automatic request batching and management
- Identical requests are deduplicated

### Limitations
- Currently only supported for OpenAI models
- Cannot expect different results for identical requests even if you set the temperature to 0

## Environment Variables

### Core Settings
- `CONFIG_SOURCE`: Path or URL to config file (default: "config.yaml")
- `CONFIG_TOKEN`: Bearer token for authenticated config URL (optional)
- `PORT`: Server port (default: 8080)

### API Keys
- `OPEN_GEMINI_API_KEY`: API key for accessing Ogem
- `OPENAI_API_KEY`: OpenAI API key
- `CLAUDE_API_KEY`: Anthropic Claude API key
- `GENAI_STUDIO_API_KEY`: Google Gemini Studio API key
- `GOOGLE_CLOUD_PROJECT`: GCP project ID for Vertex AI

### Performance Settings
- `VALKEY_ENDPOINT`: Redis-compatible endpoint for state management
- `RETRY_INTERVAL`: Wait duration before retrying failed requests
- `PING_INTERVAL`: Health check interval

## API Usage

Send requests using the OpenAI API format:

```bash
curl http://localhost:8080/v1/chat/completions \
  -H "Content-Type: application/json" \
  -H "Authorization: Bearer $OGEM_API_KEY" \
  -d '{
    "model": "gemini-1.5-flash",
    "messages": [
      {
        "role": "user",
        "content": "Hello! How can you help me today?"
      }
    ],
    "temperature": 0.7
  }'
```

### Model Selection

Three formats for model selection:

1. Simple model name:
```json
{
  "model": "gpt-4"
}
```

2. Provider and model:
```json
{
  "model": "vertex/gemini-1.5-pro"
}
```

3. Provider, region, and model:
```json
{
  "model": "vertex/us-central1/gemini-1.5-pro"
}
```

### Fallback Chain

Specify multiple models for automatic fallback:
```json
{
  "model": "gpt-4,claude-3-opus,gemini-1.5-pro"
}
```

### Batch Processing

Add `@batch` suffix for batch processing:
```json
{
  "model": "gpt-4@batch"
}
```
Currently, batch processing is only supported for OpenAI models.

## Docker Support

### Running with Docker

```bash
# Basic run
docker run -p 8080:8080 ynext/ogem:latest

# With configuration
docker run -p 8080:8080 \
  -e CONFIG_SOURCE="https://api.example.com/config.yaml" \
  -e CONFIG_TOKEN="your-token" \
  -e OPENAI_API_KEY="your-key" \
  ynext/ogem:latest
```

### Building Docker Image

The image supports both AMD64 (Intel/AMD) and ARM64 architectures.

```bash
# Setup buildx for multi-architecture support
docker buildx create --name mybuilder --use

# Build and push with version tag
docker buildx build \
  --platform linux/amd64,linux/arm64 \
  -t ynext/ogem:0.0.1 \
  -t ynext/ogem:latest \
  --push .

# Verify architectures
docker buildx imagetools inspect ynext/ogem:latest
```

## Error Handling

Standard HTTP status codes:
- 400: Bad Request
- 401: Unauthorized
- 429: Too Many Requests
- 500: Internal Server Error
- 503: Service Unavailable

## Development

Requirements:
- Go 1.22+
- Docker (optional)
- Valkey (Redis-compatible, optional)

```bash
# Clone repository
git clone https://github.com/yanolja/ogem.git
cd ogem

# Run tests
go test ./...

# Build binary
go build ./cmd/main.go
```

## License

This project is licensed under the terms of the Apache 2.0 license. See the [LICENSE](LICENSE) file for more details.

## Contributing

Before you submit any contributions, please make sure to review and agree to our [Contributor License Agreement](CLA.md).

## Code of Conduct

Please read our [Code of Conduct](CODE_OF_CONDUCT.md) before engaging with our community.
