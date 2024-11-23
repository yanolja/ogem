# Ogem

Ogem is a proxy server that provides unified access to various AI language models through a consistent OpenAI-compatible API interface. It supports multiple providers including OpenAI, Google's Gemini (both Studio and Vertex AI), and Anthropic's Claude models.

## Features

- OpenAI API-compatible interface
- Automatic fallback between different models
- Smart routing based on latency
- Rate limiting and quota management
- Response caching for deterministic requests
- Built-in batch processing support
- Regional endpoint selection
- Automatic model version mapping
- Function/tool calling support
- Support for multiple response formats

## Supported Providers

- OpenAI
  - Support for both standard and batch endpoints
- Google Gemini
  - Available through both Studio API and Vertex AI
- Anthropic Claude
  - Available through both direct API and Vertex AI

## Installation

```bash
go get github.com/yanolja/ogem
```

## Configuration

Create a `config.yaml` file with your settings:

```yaml
valkey_endpoint: "localhost:6379"  # Optional Redis-compatible endpoint for state management
api_key: "your-ogem-api-key"      # API key for accessing Ogem
google_cloud_project: "your-gcp-project-id"  # Required for Vertex AI
genai_studio_api_key: "your-studio-api-key"  # Required for Gemini Studio
openai_api_key: "your-openai-api-key"        # Required for OpenAI
claude_api_key: "your-claude-api-key"        # Required for Claude

# Configure retry intervals
retry_interval: "1m"
ping_interval: "1h"

# Configure providers and their rate limits
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

## Running Locally

```bash
export VALKEY_ENDPOINT=localhost:6379
export OPEN_GEMINI_API_KEY=your-ogem-api-key
export GOOGLE_CLOUD_PROJECT=your-gcp-project-id
export GENAI_STUDIO_API_KEY=your-studio-api-key
export OPENAI_API_KEY=your-openai-api-key
export CLAUDE_API_KEY=your-claude-api-key
export PORT=8080

go run cmd/main.go
```

## Docker

### Running with Docker

Pull and run the latest version:
```bash
docker pull ynext/ogem:latest
docker run -p 8080:8080 ynext/ogem:latest
```

Or use a specific version:
```bash
docker pull ynext/ogem:0.0.1
docker run -p 8080:8080 ynext/ogem:0.0.1
```

### Building Docker Image

The image supports both AMD64 (Intel/AMD) and ARM64 architectures.

Build locally:
```bash
# Build for your local architecture
docker build -t ogem:latest .

# Test locally
docker run -p 8080:8080 ogem:latest
```

Build and push multi-architecture images to Docker Hub:
```bash
# Set up buildx
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

## Usage

Send requests to the server using the OpenAI API format:

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

You can specify models in three formats:

1. Model name only (uses default routing):
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

### Fallback Support

You can specify multiple models for automatic fallback:

```json
{
  "model": "gpt-4,claude-3-opus,gemini-1.5-pro"
}
```

The system will try each model in order until it gets a successful response.

### Batch Processing

Append `-batch` to any model name to use batch processing:

```json
{
  "model": "gpt-4-batch"
}
```

Batch requests are automatically grouped and processed together for better efficiency.

## Environment Variables

- `VALKEY_ENDPOINT`: Redis-compatible endpoint for state management
- `OPEN_GEMINI_API_KEY`: API key for accessing Ogem
- `GOOGLE_CLOUD_PROJECT`: GCP project ID for Vertex AI
- `GENAI_STUDIO_API_KEY`: API key for Gemini Studio
- `OPENAI_API_KEY`: API key for OpenAI
- `CLAUDE_API_KEY`: API key for Claude
- `RETRY_INTERVAL`: Duration to wait before retrying when no endpoints are available
- `PING_INTERVAL`: Interval for checking endpoint health
- `PORT`: Server port (default: 8080)

## Error Handling

The server returns standard HTTP status codes:

- 400: Bad Request (invalid parameters)
- 401: Unauthorized (invalid API key)
- 429: Too Many Requests (rate limit exceeded)
- 500: Internal Server Error
- 503: Service Unavailable (no available endpoints)

## Development

Requirements:
- Go 1.22+
- Docker (optional, for containerization)
- Redis (optional, for distributed state management)

Building from source:

```bash
git clone https://github.com/yanolja/ogem.git
cd ogem
go build ./cmd/main.go
```

Running tests:

```bash
go test ./...
```

## License

This project is licensed under the terms of the Apache 2.0 license. See the [LICENSE](LICENSE) file for more details.

## Contributing

Before you submit any contributions, please make sure to review and agree to our [Contributor License Agreement](CLA.md).

## Code of Conduct

Please read our [Code of Conduct](CODE_OF_CONDUCT.md) before engaging with our community.
