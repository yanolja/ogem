/**
 * Embeddings resource
 */

import type { EmbeddingCreateParams, EmbeddingResponse } from '../types';
import { ValidationError } from '../errors';
import type { Ogem } from '../client';

export class Embeddings {
  constructor(private client: Ogem) {}

  /**
   * Create embeddings for the given input
   */
  async create(params: EmbeddingCreateParams): Promise<EmbeddingResponse> {
    if (!params.model) {
      throw new ValidationError('model is required');
    }
    if (!params.input) {
      throw new ValidationError('input is required');
    }

    return this.client.request<EmbeddingResponse>('/v1/embeddings', {
      method: 'POST',
      body: params
    });
  }
}