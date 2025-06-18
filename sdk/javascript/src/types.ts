/**
 * Type definitions for the Ogem JavaScript SDK
 */

export interface OgemConfig {
  baseURL: string;
  apiKey: string;
  tenantId?: string;
  timeout?: number;
  debug?: boolean;
}

export interface RequestOptions {
  method?: string;
  body?: any;
  headers?: Record<string, string>;
  signal?: AbortSignal;
  stream?: boolean;
}

export interface StreamOptions {
  signal?: AbortSignal;
}

// OpenAI-compatible types
export interface ChatCompletionMessage {
  role: 'system' | 'user' | 'assistant' | 'tool' | 'function';
  content?: string | null;
  name?: string;
  tool_calls?: ToolCall[];
  tool_call_id?: string;
  function_call?: FunctionCall;
}

export interface ChatCompletionMessageParam {
  role: 'system' | 'user' | 'assistant' | 'tool' | 'function';
  content?: string | null;
  name?: string;
  tool_calls?: ToolCall[];
  tool_call_id?: string;
  function_call?: FunctionCall;
}

export interface ToolCall {
  id: string;
  type: 'function';
  function: {
    name: string;
    arguments: string;
  };
}

export interface FunctionCall {
  name: string;
  arguments: string;
}

export interface Function {
  name: string;
  description?: string;
  parameters?: Record<string, any>;
}

export interface Tool {
  type: 'function';
  function: Function;
}

export type ToolChoice = 'none' | 'auto' | 'required' | { type: 'function'; function: { name: string } };

export interface Usage {
  prompt_tokens: number;
  completion_tokens?: number;
  total_tokens: number;
}

export interface Choice {
  index: number;
  message: ChatCompletionMessage;
  logprobs?: LogProbs | null;
  finish_reason: 'stop' | 'length' | 'function_call' | 'tool_calls' | 'content_filter' | null;
}

export interface ChoiceDelta {
  role?: 'system' | 'user' | 'assistant' | 'tool' | 'function';
  content?: string | null;
  function_call?: Partial<FunctionCall>;
  tool_calls?: Partial<ToolCall>[];
}

export interface LogProbs {
  tokens?: string[];
  token_logprobs?: number[];
  top_logprobs?: Record<string, number>[];
  text_offset?: number[];
}

export interface ResponseFormat {
  type: 'text' | 'json_object';
}

export interface ChatCompletion {
  id: string;
  object: 'chat.completion';
  created: number;
  model: string;
  choices: Choice[];
  usage?: Usage;
  system_fingerprint?: string;
}

export interface ChatCompletionChunk {
  id: string;
  object: 'chat.completion.chunk';
  created: number;
  model: string;
  choices: Array<{
    index: number;
    delta: ChoiceDelta;
    logprobs?: LogProbs | null;
    finish_reason: 'stop' | 'length' | 'function_call' | 'tool_calls' | 'content_filter' | null;
  }>;
  usage?: Usage;
  system_fingerprint?: string;
}

// Chat completion parameters
export interface ChatCompletionCreateParamsBase {
  model: string;
  messages: ChatCompletionMessageParam[];
  max_tokens?: number | null;
  temperature?: number | null;
  top_p?: number | null;
  n?: number | null;
  stop?: string | string[] | null;
  presence_penalty?: number | null;
  frequency_penalty?: number | null;
  logit_bias?: Record<string, number> | null;
  user?: string;
  functions?: Function[];
  function_call?: 'none' | 'auto' | { name: string };
  tools?: Tool[];
  tool_choice?: ToolChoice;
  response_format?: ResponseFormat;
  seed?: number | null;
  logprobs?: boolean | null;
  top_logprobs?: number | null;
}

export interface ChatCompletionCreateParamsNonStreaming extends ChatCompletionCreateParamsBase {
  stream?: false | null;
}

export interface ChatCompletionCreateParamsStreaming extends ChatCompletionCreateParamsBase {
  stream: true;
}

export type ChatCompletionCreateParams =
  | ChatCompletionCreateParamsNonStreaming
  | ChatCompletionCreateParamsStreaming;

// Embedding types
export interface Embedding {
  object: 'embedding';
  index: number;
  embedding: number[];
}

export interface EmbeddingResponse {
  object: 'list';
  data: Embedding[];
  model: string;
  usage: {
    prompt_tokens: number;
    total_tokens: number;
  };
}

export interface EmbeddingCreateParams {
  model: string;
  input: string | string[] | number[] | number[][];
  encoding_format?: 'float' | 'base64';
  dimensions?: number;
  user?: string;
}

// Model types
export interface Model {
  id: string;
  object: 'model';
  created: number;
  owned_by: string;
  permission?: any[];
  root?: string;
  parent?: string | null;
}

export interface ModelListResponse {
  object: 'list';
  data: Model[];
}

// Monitoring types
export interface HealthResponse {
  status: string;
  version: string;
  uptime: string;
  timestamp: string;
  checks?: Record<string, any>;
}

export interface StatsResponse {
  requests: {
    total: number;
    success: number;
    errors: number;
    success_rate: number;
  };
  performance: {
    average_latency: string;
    p95_latency: string;
    p99_latency: string;
    throughput_rpm: number;
  };
  providers: Record<string, {
    requests: number;
    errors: number;
    average_latency: string;
  }>;
  cache?: {
    hits: number;
    misses: number;
    hit_rate: number;
  };
  timestamp: string;
}

export interface CacheStatsResponse {
  hit_rate: number;
  total_entries: number;
  memory_usage_mb: number;
  strategies: Record<string, {
    entries: number;
    hit_rate: number;
    memory_mb: number;
  }>;
  adaptive_state?: {
    current_strategy: string;
    learning_enabled: boolean;
    confidence_score: number;
  };
  timestamp: string;
}

export interface TenantUsageResponse {
  tenant_id: string;
  requests_this_hour: number;
  requests_this_day: number;
  requests_this_month: number;
  cost_this_hour: number;
  cost_this_day: number;
  cost_this_month: number;
  tokens_this_hour: number;
  tokens_this_day: number;
  tokens_this_month: number;
  limits: {
    requests_per_hour?: number;
    requests_per_day?: number;
    requests_per_month?: number;
    cost_per_hour?: number;
    cost_per_day?: number;
    cost_per_month?: number;
  };
  timestamp: string;
}