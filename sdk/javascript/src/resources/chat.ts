/**
 * Chat completion resource
 */

import type { 
  ChatCompletion,
  ChatCompletionChunk,
  ChatCompletionCreateParams,
  ChatCompletionCreateParamsStreaming,
  ChatCompletionCreateParamsNonStreaming
} from '../types';
import { ValidationError } from '../errors';
import type { Ogem } from '../client';

export class ChatCompletions {
  constructor(private client: Ogem) {}

  /**
   * Create a chat completion
   */
  create(params: ChatCompletionCreateParamsNonStreaming): Promise<ChatCompletion>;
  create(params: ChatCompletionCreateParamsStreaming): Promise<AsyncIterable<ChatCompletionChunk>>;
  create(params: ChatCompletionCreateParams): Promise<ChatCompletion | AsyncIterable<ChatCompletionChunk>>;
  create(
    params: ChatCompletionCreateParams
  ): Promise<ChatCompletion | AsyncIterable<ChatCompletionChunk>> {
    if (!params.model) {
      throw new ValidationError('model is required');
    }
    if (!params.messages || params.messages.length === 0) {
      throw new ValidationError('messages is required and cannot be empty');
    }

    if (params.stream) {
      return this.createStream(params);
    }

    return this.client.request<ChatCompletion>('/v1/chat/completions', {
      method: 'POST',
      body: params
    });
  }

  private async createStream(
    params: ChatCompletionCreateParamsStreaming
  ): Promise<AsyncIterable<ChatCompletionChunk>> {
    const response = await this.client.request<Response>('/v1/chat/completions', {
      method: 'POST',
      body: params,
      stream: true
    });

    return this.parseStream(response);
  }

  private async *parseStream(response: Response): AsyncIterable<ChatCompletionChunk> {
    if (!response.body) {
      throw new Error('No response body for streaming');
    }

    const reader = response.body.getReader();
    const decoder = new TextDecoder();

    try {
      while (true) {
        const { done, value } = await reader.read();
        if (done) break;

        const chunk = decoder.decode(value, { stream: true });
        const lines = chunk.split('\n');

        for (const line of lines) {
          const trimmed = line.trim();
          if (!trimmed || !trimmed.startsWith('data: ')) continue;

          const data = trimmed.slice(6); // Remove 'data: ' prefix
          if (data === '[DONE]') return;

          try {
            const parsed = JSON.parse(data) as ChatCompletionChunk;
            yield parsed;
          } catch (error) {
            // Log parsing errors but continue processing other chunks
            console.warn('Failed to parse stream chunk:', error);
          }
        }
      }
    } catch (error) {
      // Ensure reader is released even if an error occurs during processing
      throw error;
    } finally {
      reader.releaseLock();
    }
  }
}

export class Chat {
  readonly completions: ChatCompletions;

  constructor(client: Ogem) {
    this.completions = new ChatCompletions(client);
  }
}