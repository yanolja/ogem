# Ogem

Ogem is a unified proxy server that provides access to multiple AI language models through an OpenAI-compatible API interface. It supports OpenAI, Google's Gemini (both Studio and Vertex AI), and Anthropic's Claude models.

## Features

- OpenAI API-compatible interface
- Support for multiple AI providers:
  - OpenAI (e.g., GPT-4, GPT-3.5)
  - Google Gemini (e.g., 1.5 Flash, Pro)
  - Anthropic Claude (e.g., 3.5 Opus, Sonnet, Haiku)
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
retry_interval: "1m"
ping_interval: "1h"
providers:
  openai:
    regions:
      openai:
        models:
          - name: "gpt-4"
            rate_key: "gpt-4"
            rpm: 10000
            tpm: 1000000
  vertex:
    regions:
      us-central1:
        models:
          - name: "gemini-1.5-pro"
            rate_key: "gemini-1.5-pro"
            rpm: 60
            tpm: 4000000
```

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

Add `-batch` suffix for batch processing:
```json
{
  "model": "gpt-4-batch"
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
- Redis (optional)

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
