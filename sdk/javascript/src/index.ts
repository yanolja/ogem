/**
 * Ogem JavaScript/TypeScript SDK
 * 
 * Official client library for the Ogem AI proxy server.
 * Provides OpenAI-compatible API with advanced features like multi-tenancy,
 * intelligent caching, and enterprise security.
 * 
 * @example
 * ```typescript
 * import { Ogem } from '@ogem/sdk';
 * 
 * const client = new Ogem({
 *   baseURL: 'http://localhost:8080',
 *   apiKey: 'your-api-key',
 *   tenantId: 'your-tenant-id'
 * });
 * 
 * const response = await client.chat.completions.create({
 *   model: 'gpt-4',
 *   messages: [
 *     { role: 'system', content: 'You are a helpful assistant.' },
 *     { role: 'user', content: 'Hello, world!' }
 *   ]
 * });
 * 
 * console.log(response.choices[0].message.content);
 * ```
 */

export { Ogem } from './client';
export { 
  OgemError, 
  APIError, 
  AuthenticationError, 
  RateLimitError, 
  TenantError, 
  ValidationError,
  ModelError,
  ProviderError,
  StreamError
} from './errors';

export type {
  // Core types
  OgemConfig,
  
  // Chat types
  ChatCompletion,
  ChatCompletionChunk,
  ChatCompletionMessage,
  ChatCompletionMessageParam,
  ChatCompletionCreateParams,
  ChatCompletionCreateParamsStreaming,
  ChatCompletionCreateParamsNonStreaming,
  Choice,
  ChoiceDelta,
  
  // Function and tool types
  Function,
  FunctionCall,
  Tool,
  ToolCall,
  ToolChoice,
  
  // Embedding types
  Embedding,
  EmbeddingCreateParams,
  EmbeddingResponse,
  
  // Model types
  Model,
  ModelListResponse,
  
  // Utility types
  Usage,
  ResponseFormat,
  LogProbs,
  
  // Monitoring types
  HealthResponse,
  StatsResponse,
  CacheStatsResponse,
  TenantUsageResponse,
  
  // Request options
  RequestOptions,
  StreamOptions
} from './types';

export {
  // Model constants
  Models,
  
  // Helper functions
  createUserMessage,
  createSystemMessage,
  createAssistantMessage,
  createToolMessage,
  createFunctionMessage,
  
  // Response format helpers
  ResponseFormats,
  
  // Role constants
  Roles,
  
  // Finish reason constants
  FinishReasons
} from './helpers';

// Default export
export default Ogem;