/**
 * Helper functions and constants for the Ogem JavaScript SDK
 */

import type { ChatCompletionMessageParam } from './types';

// Model constants
export const Models = {
  // OpenAI models
  GPT_4: 'gpt-4',
  GPT_4_TURBO: 'gpt-4-turbo',
  GPT_4_32K: 'gpt-4-32k',
  GPT_3_5_TURBO: 'gpt-3.5-turbo',
  GPT_3_5_TURBO_16K: 'gpt-3.5-turbo-16k',
  
  // Claude models
  CLAUDE_3_OPUS: 'claude-3-opus-20240229',
  CLAUDE_3_SONNET: 'claude-3-sonnet-20240229',
  CLAUDE_3_HAIKU: 'claude-3-haiku-20240307',
  CLAUDE_3_5_SONNET: 'claude-3-5-sonnet-20241022',
  
  // Gemini models
  GEMINI_PRO: 'gemini-pro',
  GEMINI_PRO_VISION: 'gemini-pro-vision',
  GEMINI_1_5_PRO: 'gemini-1.5-pro',
  GEMINI_1_5_FLASH: 'gemini-1.5-flash',
  
  // Embedding models
  TEXT_EMBEDDING_3_SMALL: 'text-embedding-3-small',
  TEXT_EMBEDDING_3_LARGE: 'text-embedding-3-large',
  TEXT_EMBEDDING_ADA_002: 'text-embedding-ada-002',
  
  // Other providers
  LLAMA_2_70B: 'llama-2-70b-chat',
  MIXTRAL_8X7B: 'mixtral-8x7b-instruct',
  CODESTRAL_LATEST: 'codestral-latest',
} as const;

// Role constants
export const Roles = {
  SYSTEM: 'system',
  USER: 'user',
  ASSISTANT: 'assistant',
  TOOL: 'tool',
  FUNCTION: 'function',
} as const;

// Finish reason constants
export const FinishReasons = {
  STOP: 'stop',
  LENGTH: 'length',
  FUNCTION_CALL: 'function_call',
  TOOL_CALLS: 'tool_calls',
  CONTENT_FILTER: 'content_filter',
} as const;

// Response format helpers
export const ResponseFormats = {
  TEXT: { type: 'text' as const },
  JSON: { type: 'json_object' as const },
} as const;

// Message creation helpers
export function createSystemMessage(content: string): ChatCompletionMessageParam {
  return {
    role: 'system',
    content,
  };
}

export function createUserMessage(content: string): ChatCompletionMessageParam {
  return {
    role: 'user',
    content,
  };
}

export function createAssistantMessage(content: string): ChatCompletionMessageParam {
  return {
    role: 'assistant',
    content,
  };
}

export function createToolMessage(
  content: string,
  toolCallId: string
): ChatCompletionMessageParam {
  return {
    role: 'tool',
    content,
    tool_call_id: toolCallId,
  };
}

export function createFunctionMessage(
  content: string,
  name: string
): ChatCompletionMessageParam {
  return {
    role: 'function',
    content,
    name,
  };
}

// Utility functions
export function isStreamingParams(params: any): boolean {
  return params?.stream === true;
}

export function validateModel(model: string): void {
  if (!model || typeof model !== 'string') {
    throw new Error('Model must be a non-empty string');
  }
}

export function validateMessages(messages: any[]): void {
  if (!Array.isArray(messages) || messages.length === 0) {
    throw new Error('Messages must be a non-empty array');
  }
  
  for (const [index, message] of messages.entries()) {
    if (!message || typeof message !== 'object') {
      throw new Error(`Message at index ${index} must be an object`);
    }
    
    if (!message.role || typeof message.role !== 'string') {
      throw new Error(`Message at index ${index} must have a valid role`);
    }
    
    const validRoles = ['system', 'user', 'assistant', 'tool', 'function'];
    if (!validRoles.includes(message.role)) {
      throw new Error(`Message at index ${index} has invalid role: ${message.role}`);
    }
  }
}

export function formatTokenUsage(usage: { prompt_tokens: number; completion_tokens?: number; total_tokens: number }): string {
  const prompt = usage.prompt_tokens;
  const completion = usage.completion_tokens ?? 0;
  const total = usage.total_tokens;
  
  return `${prompt} prompt + ${completion} completion = ${total} tokens`;
}

export function calculateCostEstimate(
  usage: { total_tokens: number },
  model: string
): number | null {
  // Rough cost estimates per 1K tokens (these are approximate)
  const costPer1K: Record<string, number> = {
    'gpt-4': 0.06,
    'gpt-4-turbo': 0.03,
    'gpt-3.5-turbo': 0.002,
    'claude-3-opus-20240229': 0.075,
    'claude-3-sonnet-20240229': 0.015,
    'claude-3-haiku-20240307': 0.0015,
    'text-embedding-3-small': 0.0001,
    'text-embedding-3-large': 0.00013,
  };
  
  const rate = costPer1K[model];
  if (!rate) return null;
  
  return (usage.total_tokens / 1000) * rate;
}

// Type guards
export function isChatCompletion(obj: any): obj is import('./types').ChatCompletion {
  return obj && obj.object === 'chat.completion' && Array.isArray(obj.choices);
}

export function isChatCompletionChunk(obj: any): obj is import('./types').ChatCompletionChunk {
  return obj && obj.object === 'chat.completion.chunk' && Array.isArray(obj.choices);
}

export function isEmbeddingResponse(obj: any): obj is import('./types').EmbeddingResponse {
  return obj && obj.object === 'list' && Array.isArray(obj.data) && obj.data.every((item: any) => item.object === 'embedding');
}

// Request ID utilities
export function generateRequestId(): string {
  return `req_${Date.now()}_${Math.random().toString(36).substring(2, 15)}`;
}

export function extractRequestId(headers: Headers): string | undefined {
  return headers.get('x-request-id') || headers.get('request-id') || undefined;
}