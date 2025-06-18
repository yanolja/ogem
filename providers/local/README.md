# Local AI Providers

This package provides support for local AI providers that run on your own infrastructure, offering privacy, control, and cost benefits compared to cloud-based APIs.

## Supported Providers

### 🦙 Ollama
- **Description**: Easy-to-use local LLM runner
- **Default Port**: 11434
- **Models**: Llama 2, Code Llama, Mistral, Mixtral, and many more
- **Features**: 
  - Simple model management with `ollama pull`
  - Efficient inference optimized for consumer hardware
  - Support for chat completions and embeddings
  - Automatic model quantization

### 🚀 vLLM
- **Description**: High-throughput, memory-efficient inference engine
- **Default Port**: 8000
- **Models**: Any HuggingFace Transformers model
- **Features**:
  - OpenAI-compatible API
  - PagedAttention for efficient memory usage
  - Continuous batching for high throughput
  - Support for popular models like Llama, Mistral, etc.

### 🎨 LM Studio
- **Description**: User-friendly desktop application for running LLMs
- **Default Port**: 1234
- **Models**: Wide variety of GGUF models
- **Features**:
  - Graphical interface for model management
  - OpenAI-compatible API server
  - Easy model discovery and download
  - Cross-platform support (Windows, macOS, Linux)

## Quick Start

### Installation and Setup

#### Ollama
```bash
# Install Ollama
curl -fsSL https://ollama.ai/install.sh | sh

# Pull a model
ollama pull llama2

# Start the server (usually runs automatically)
ollama serve
```

#### vLLM
```bash
# Install vLLM
pip install vllm

# Start vLLM server with a model
python -m vllm.entrypoints.openai.api_server \
  --model meta-llama/Llama-2-7b-chat-hf \
  --port 8000
```

#### LM Studio
1. Download LM Studio from https://lmstudio.ai/
2. Install and launch the application
3. Download a model from the built-in model browser
4. Start the local server from the "Local Server" tab

### Basic Usage

```go
package main

import (
    "context"
    "fmt"
    "log"
    
    "github.com/yanolja/ogem/providers/local"
    "github.com/yanolja/ogem/types"
)

func main() {
    // Configure local providers
    config := &local.LocalConfig{
        AutoDiscovery: true,
        Ollama: &local.OllamaConfig{
            BaseURL: "http://localhost:11434",
        },
        VLLM: &local.VLLMConfig{
            BaseURL: "http://localhost:8000",
        },
        LMStudio: &local.LMStudioConfig{
            BaseURL: "http://localhost:1234",
        },
    }
    
    // Create manager
    manager := local.NewLocalProviderManager(config)
    
    // Get available models
    models, err := manager.GetAllModels(context.Background())
    if err != nil {
        log.Fatal(err)
    }
    
    fmt.Printf("Available models: %+v\n", models)
    
    // Use a specific provider
    provider, err := manager.GetProvider("ollama")
    if err != nil {
        log.Fatal(err)
    }
    
    // Chat completion
    response, err := provider.ChatCompletion(context.Background(), &types.ChatCompletionRequest{
        Model: "llama2",
        Messages: []types.Message{
            {Role: "user", Content: "Hello!"},
        },
    })
    
    fmt.Printf("Response: %s\n", response.Choices[0].Message.Content)
}
```

## Configuration

### Environment Variables

```bash
# Ollama
export OLLAMA_HOST=http://localhost:11434

# vLLM
export VLLM_HOST=http://localhost:8000
export VLLM_API_KEY=your-api-key  # Optional

# LM Studio
export LMSTUDIO_HOST=http://localhost:1234
```

### Configuration File

```yaml
# config/local_providers.yaml
local_providers:
  auto_discovery: true
  health_check_interval: 30s
  
  ollama:
    base_url: "http://localhost:11434"
    timeout: 30s
    models:
      - "llama2"
      - "codellama"
      - "mistral"
  
  vllm:
    base_url: "http://localhost:8000"
    api_key: ""
    timeout: 60s
    models:
      - "meta-llama/Llama-2-7b-chat-hf"
  
  lmstudio:
    base_url: "http://localhost:1234"
    timeout: 60s
```

## API Endpoints

When integrated with Ogem, local providers expose these endpoints:

### Provider Management
- `GET /local/providers` - List all providers
- `GET /local/providers/{name}` - Get provider details
- `GET /local/providers/{name}/health` - Check provider health
- `GET /local/providers/{name}/models` - Get provider models

### Model Operations
- `GET /local/models` - Get all models from all providers
- `GET /local/status` - Get overall status

### Chat and Embeddings
- `POST /local/chat/completions/{provider}` - Chat with specific provider
- `POST /local/embeddings/{provider}` - Create embeddings

### Monitoring
- `GET /local/health` - Health check for all providers
- `POST /local/discover` - Trigger provider auto-discovery

## Features

### Auto-Discovery
Automatically detects running local providers on standard ports:
- Ollama: `http://localhost:11434`
- vLLM: `http://localhost:8000`
- LM Studio: `http://localhost:1234`

### Health Monitoring
- Continuous health checks
- Automatic failover
- Provider resurrection detection

### Model Routing
Intelligent routing to the best provider for each model:
- Pattern matching for model names
- Provider capability detection
- Fallback strategies

### Streaming Support
Full support for streaming responses from all providers that support it.

### Error Handling
Comprehensive error handling with specific error types:
- Provider unavailable
- Model not found
- Timeout errors
- Authentication errors

## Performance Optimization

### Connection Management
- Connection pooling for each provider
- Keep-alive connections
- Configurable timeouts

### Caching
- Model information caching
- Response caching (when enabled)
- Adaptive cache strategies

### Load Balancing
- Round-robin between multiple instances
- Health-based routing
- Circuit breaker patterns

## Security Considerations

### Network Security
- Local network only by default
- Configurable allowed hosts
- Optional API key authentication

### Request Validation
- Input sanitization
- Size limits on requests/responses
- Rate limiting

### Privacy
- All data stays local
- No external API calls required
- Full control over model and data

## Troubleshooting

### Common Issues

#### Provider Not Found
```
Error: Provider ollama not found
```
**Solution**: Ensure the provider is running and accessible:
```bash
curl http://localhost:11434/api/tags  # Ollama
curl http://localhost:8000/v1/models  # vLLM
curl http://localhost:1234/v1/models  # LM Studio
```

#### No Models Available
```
Error: no models loaded
```
**Solutions**:
- **Ollama**: `ollama pull llama2`
- **vLLM**: Ensure model is downloaded and specified in startup
- **LM Studio**: Download and load a model through the GUI

#### Connection Timeout
```
Error: context deadline exceeded
```
**Solutions**:
- Increase timeout in configuration
- Check provider resource usage
- Verify network connectivity

#### Health Check Failed
```
Error: ollama health check failed
```
**Solutions**:
- Restart the provider service
- Check provider logs
- Verify configuration

### Debugging

Enable debug mode for detailed logging:
```go
config := &local.LocalConfig{
    // ... other config
}

// Enable debug logging in your application
```

Or via environment:
```bash
export OGEM_DEBUG=true
export OGEM_LOG_LEVEL=debug
```

## Examples

See the [examples](../../examples/local_providers/) directory for:
- [Ollama Example](../../examples/local_providers/ollama_example.go)
- [Local Manager Example](../../examples/local_providers/local_manager_example.go)

## Best Practices

### Model Selection
- Use Ollama for quick setup and experimentation
- Use vLLM for high-throughput production workloads
- Use LM Studio for desktop applications and testing

### Resource Management
- Monitor GPU memory usage
- Configure appropriate model quantization
- Use model warming for production

### Deployment
- Use container orchestration for vLLM
- Consider model caching strategies
- Implement proper monitoring and alerting

## Integration with Ogem

Local providers integrate seamlessly with Ogem's core features:

### Multi-tenancy
Each tenant can have different local provider preferences and access controls.

### Caching
Local provider responses can be cached using Ogem's intelligent caching system.

### Monitoring
Full integration with Ogem's monitoring and analytics platform.

### Security
Local providers respect all Ogem security policies including PII masking and rate limiting.

## Contributing

To add support for a new local provider:

1. Implement the `providers.Provider` interface
2. Add configuration structures
3. Update the manager to include the new provider
4. Add tests and examples
5. Update documentation

See existing providers as examples for implementation patterns.