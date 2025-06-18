/**
 * Models resource
 */

import type { Model, ModelListResponse } from '../types';
import { ValidationError } from '../errors';
import type { Ogem } from '../client';

export class Models {
  constructor(private client: Ogem) {}

  /**
   * List all available models
   */
  async list(): Promise<ModelListResponse> {
    return this.client.request<ModelListResponse>('/v1/models');
  }

  /**
   * Retrieve information about a specific model
   */
  async retrieve(modelId: string): Promise<Model> {
    if (!modelId) {
      throw new ValidationError('modelId is required');
    }

    return this.client.request<Model>(`/v1/models/${modelId}`);
  }
}