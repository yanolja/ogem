# Ogem JavaScript SDK

Official JavaScript/TypeScript client library for the Ogem AI proxy server. Provides OpenAI-compatible API with advanced features like multi-tenancy, intelligent caching, and enterprise security.

## Features

- 🔄 **OpenAI API Compatibility** - Drop-in replacement for OpenAI client
- 🏢 **Multi-tenancy** - Isolated environments for different organizations
- 🚀 **Intelligent Caching** - Semantic similarity and adaptive learning
- 🔒 **Enterprise Security** - PII masking, rate limiting, audit logging
- 📊 **Advanced Monitoring** - Real-time metrics and analytics
- 🌊 **Streaming Support** - Real-time token streaming
- 🔧 **Function Calling** - Tool and function calling capabilities
- 📝 **Full TypeScript** - Complete type definitions

## Installation

```bash
npm install @ogem/sdk
# or
yarn add @ogem/sdk
# or
pnpm add @ogem/sdk
```

## Quick Start

```typescript
import { Ogem, Models, createSystemMessage, createUserMessage } from '@ogem/sdk';

const client = new Ogem({
  baseURL: 'http://localhost:8080',
  apiKey: 'your-api-key',
  tenantId: 'your-tenant-id' // optional
});

// Chat completion
const response = await client.chat.completions.create({
  model: Models.GPT_4,
  messages: [
    createSystemMessage('You are a helpful assistant.'),
    createUserMessage('Hello, world!')
  ],
  max_tokens: 100
});

console.log(response.choices[0].message.content);
```

## Configuration

### Basic Configuration

```typescript
const client = new Ogem({
  baseURL: 'http://localhost:8080',    // Required: Ogem server URL
  apiKey: 'your-api-key',              // Required: Your API key
  tenantId: 'your-tenant-id',          // Optional: Tenant ID for multi-tenancy
  timeout: 30000,                      // Optional: Request timeout in ms (default: 30000)
  debug: false                         // Optional: Enable debug logging (default: false)
});
```

### Environment Variables

You can also configure the client using environment variables:

```bash
export OGEM_BASE_URL=http://localhost:8080
export OGEM_API_KEY=your-api-key
export OGEM_TENANT_ID=your-tenant-id
```

## API Reference

### Chat Completions

#### Basic Chat Completion

```typescript
const response = await client.chat.completions.create({
  model: Models.GPT_4,
  messages: [
    { role: 'system', content: 'You are a helpful assistant.' },
    { role: 'user', content: 'What is TypeScript?' }
  ],
  max_tokens: 150,
  temperature: 0.7
});
```

#### Streaming Chat Completion

```typescript
const stream = await client.chat.completions.create({
  model: Models.GPT_4,
  messages: [
    { role: 'user', content: 'Write a short story.' }
  ],
  stream: true
});

for await (const chunk of stream) {
  const content = chunk.choices[0]?.delta?.content;
  if (content) {
    process.stdout.write(content);
  }
}
```

#### Function Calling

```typescript
const tools = [
  {
    type: 'function',
    function: {
      name: 'get_weather',
      description: 'Get current weather for a location',
      parameters: {
        type: 'object',
        properties: {
          location: { type: 'string', description: 'City name' }
        },
        required: ['location']
      }
    }
  }
];

const response = await client.chat.completions.create({
  model: Models.GPT_4,
  messages: [{ role: 'user', content: 'What\'s the weather in Paris?' }],
  tools,
  tool_choice: 'auto'
});

if (response.choices[0].message.tool_calls) {
  const toolCall = response.choices[0].message.tool_calls[0];
  console.log(`Called: ${toolCall.function.name}`);
  console.log(`Arguments: ${toolCall.function.arguments}`);
}
```

### Embeddings

```typescript
const response = await client.embeddings.create({
  model: Models.TEXT_EMBEDDING_3_SMALL,
  input: ['Hello world', 'How are you?']
});

console.log(response.data[0].embedding); // Array of numbers
```

### Models

```typescript
// List all available models
const models = await client.models.list();
console.log(models.data);

// Get specific model info
const model = await client.models.retrieve('gpt-4');
console.log(model);
```

## Multi-tenancy

Ogem supports multi-tenancy for isolated environments:

```typescript
// Client for tenant A
const clientA = new Ogem({
  baseURL: 'http://localhost:8080',
  apiKey: 'your-api-key',
  tenantId: 'tenant-a'
});

// Client for tenant B
const clientB = new Ogem({
  baseURL: 'http://localhost:8080',
  apiKey: 'your-api-key',
  tenantId: 'tenant-b'
});

// Get tenant usage
const usage = await clientA.tenantUsage();
console.log(usage);
```

## Monitoring and Observability

### Health Check

```typescript
const health = await client.health();
console.log(health.status); // "healthy"
```

### Server Statistics

```typescript
const stats = await client.stats();
console.log(stats.requests.total);
console.log(stats.performance.average_latency);
```

### Cache Statistics

```typescript
const cacheStats = await client.cacheStats();
console.log(cacheStats.hit_rate);
console.log(cacheStats.total_entries);
```

## Error Handling

The SDK provides specific error types for different scenarios:

```typescript
import { 
  OgemError, 
  APIError, 
  AuthenticationError, 
  RateLimitError, 
  ValidationError 
} from '@ogem/sdk';

try {
  const response = await client.chat.completions.create({
    model: 'invalid-model',
    messages: [{ role: 'user', content: 'Hello' }]
  });
} catch (error) {
  if (error instanceof AuthenticationError) {
    console.error('Authentication failed:', error.message);
  } else if (error instanceof RateLimitError) {
    console.error('Rate limited:', error.message);
    console.log('Retry after:', error.retryAfter);
  } else if (error instanceof ValidationError) {
    console.error('Validation error:', error.message);
    console.log('Field errors:', error.fieldErrors);
  } else if (error instanceof APIError) {
    console.error('API error:', error.message, error.status);
  }
}
```

## Helper Functions

The SDK includes helpful utility functions:

```typescript
import { 
  createSystemMessage,
  createUserMessage,
  createAssistantMessage,
  Models,
  ResponseFormats
} from '@ogem/sdk';

// Message creation helpers
const messages = [
  createSystemMessage('You are a helpful assistant.'),
  createUserMessage('Hello!'),
  createAssistantMessage('Hi there!')
];

// Response format helpers
const response = await client.chat.completions.create({
  model: Models.GPT_4,
  messages,
  response_format: ResponseFormats.JSON
});
```

## TypeScript Support

The SDK is written in TypeScript and provides comprehensive type definitions:

```typescript
import type { 
  ChatCompletion,
  ChatCompletionCreateParams,
  EmbeddingResponse,
  Model
} from '@ogem/sdk';

// Full type safety
const params: ChatCompletionCreateParams = {
  model: 'gpt-4',
  messages: [{ role: 'user', content: 'Hello' }]
};

const response: ChatCompletion = await client.chat.completions.create(params);
```

## Examples

Check out the [examples](./examples/) directory for more usage patterns:

- [Basic Usage](./examples/basic.ts) - Simple chat completions and health checks
- [Streaming](./examples/streaming.ts) - Real-time streaming responses  
- [Function Calling](./examples/functions.ts) - Tool and function calling

## Building and Development

```bash
# Install dependencies
npm install

# Build the package
npm run build

# Run type checking
npm run typecheck

# Run linting
npm run lint

# Run examples
npm run example:basic
npm run example:streaming
npm run example:functions
```

## Contributing

1. Fork the repository
2. Create a feature branch
3. Make your changes
4. Add tests for new functionality
5. Ensure all tests pass
6. Submit a pull request

## License

Apache-2.0 License - see [LICENSE](../../LICENSE) file for details.

## Support

- 📖 [Documentation](https://github.com/yanolja/ogem#readme)
- 🐛 [Issue Tracker](https://github.com/yanolja/ogem/issues)
- 💬 [Discussions](https://github.com/yanolja/ogem/discussions)

## Related

- [Ogem Python SDK](../python/) - Python client library
- [Ogem Go SDK](../go/) - Go client library
- [Ogem Server](../../) - Main Ogem proxy server