/**
 * Main Ogem client class
 */

import fetch from 'cross-fetch';
import type { 
  OgemConfig, 
  RequestOptions,
  HealthResponse,
  StatsResponse,
  CacheStatsResponse,
  TenantUsageResponse
} from './types';
import { 
  OgemError, 
  APIError, 
  AuthenticationError, 
  RateLimitError, 
  TenantError,
  ValidationError,
  createErrorFromResponse
} from './errors';
import { Chat } from './resources/chat';
import { Embeddings } from './resources/embeddings';
import { Models } from './resources/models';

/**
 * Main Ogem client for making API requests.
 */
export class Ogem {
  readonly baseURL: string;
  readonly apiKey: string;
  private _tenantId?: string;
  readonly timeout: number;
  private _debug: boolean;

  // Resources
  readonly chat: Chat;
  readonly embeddings: Embeddings;
  readonly models: Models;

  /**
   * Create a new Ogem client.
   * 
   * @param config - Configuration options
   */
  constructor(config: OgemConfig) {
    if (!config.baseURL) {
      throw new ValidationError('baseURL is required');
    }
    if (!config.apiKey) {
      throw new ValidationError('apiKey is required');
    }

    this.baseURL = config.baseURL.replace(/\/$/, '');
    this.apiKey = config.apiKey;
    this._tenantId = config.tenantId;
    this.timeout = config.timeout ?? 30000;
    this._debug = config.debug ?? false;

    // Initialize resources
    this.chat = new Chat(this);
    this.embeddings = new Embeddings(this);
    this.models = new Models(this);
  }

  get tenantId(): string | undefined {
    return this._tenantId;
  }

  get debug(): boolean {
    return this._debug;
  }

  /**
   * Make an HTTP request to the Ogem API.
   */
  async request<T = any>(
    endpoint: string,
    options: RequestOptions = {}
  ): Promise<T> {
    const url = `${this.baseURL}${endpoint}`;
    const {
      method = 'GET',
      body,
      headers: customHeaders = {},
      signal,
      stream = false
    } = options;

    // Build headers
    const headers: Record<string, string> = {
      'Authorization': `Bearer ${this.apiKey}`,
      'User-Agent': `ogem-js/1.0.0`,
      ...customHeaders
    };

    if (this.tenantId) {
      headers['X-Tenant-ID'] = this.tenantId;
    }

    if (body && typeof body === 'object') {
      headers['Content-Type'] = 'application/json';
    }

    if (stream) {
      headers['Accept'] = 'text/event-stream';
      headers['Cache-Control'] = 'no-cache';
    }

    if (this.debug) {
      console.log(`DEBUG: ${method} ${url}`);
      if (body) {
        console.log('DEBUG: Request body:', JSON.stringify(body, null, 2));
      }
    }

    // Create AbortController for timeout
    const controller = new AbortController();
    const timeoutId = setTimeout(() => controller.abort(), this.timeout);
    
    // Use provided signal or our timeout signal
    const finalSignal = signal || controller.signal;

    try {
      const response = await fetch(url, {
        method,
        headers,
        body: body ? JSON.stringify(body) : undefined,
        signal: finalSignal
      });

      clearTimeout(timeoutId);

      if (this.debug) {
        console.log(`DEBUG: Response status: ${response.status}`);
      }

      if (!response.ok) {
        await this.handleErrorResponse(response);
      }

      if (stream) {
        return response as any;
      }

      const data = await response.json();
      return data;

    } catch (error) {
      clearTimeout(timeoutId);
      
      if (error instanceof OgemError) {
        throw error;
      }
      
      if (error instanceof Error) {
        if (error.name === 'AbortError') {
          throw new OgemError('Request timed out');
        }
        throw new OgemError(`Request failed: ${error.message}`);
      }
      
      throw new OgemError('Unknown error occurred');
    }
  }

  /**
   * Handle error responses from the API.
   */
  private async handleErrorResponse(response: Response): Promise<never> {
    let errorData: any;
    
    try {
      errorData = await response.json();
    } catch {
      errorData = { message: response.statusText || `HTTP ${response.status}` };
    }

    const error = createErrorFromResponse(response.status, errorData);
    throw error;
  }

  /**
   * Check the health status of the Ogem server.
   */
  async health(): Promise<HealthResponse> {
    return this.request<HealthResponse>('/health');
  }

  /**
   * Get server statistics (requires appropriate permissions).
   */
  async stats(): Promise<StatsResponse> {
    return this.request<StatsResponse>('/stats');
  }

  /**
   * Get cache statistics (requires appropriate permissions).
   */
  async cacheStats(): Promise<CacheStatsResponse> {
    return this.request<CacheStatsResponse>('/cache/stats');
  }

  /**
   * Get tenant usage metrics (requires appropriate permissions).
   */
  async tenantUsage(tenantId?: string): Promise<TenantUsageResponse> {
    const id = tenantId || this.tenantId;
    if (!id) {
      throw new ValidationError('tenantId is required');
    }
    return this.request<TenantUsageResponse>(`/tenants/${id}/usage`);
  }

  /**
   * Clear all cache entries (requires appropriate permissions).
   */
  async clearCache(): Promise<{ message: string; timestamp: string }> {
    return this.request('/cache/clear', { method: 'POST' });
  }

  /**
   * Clear cache for a specific tenant (requires appropriate permissions).
   */
  async clearTenantCache(tenantId?: string): Promise<{ message: string; tenant_id: string; timestamp: string }> {
    const id = tenantId || this.tenantId;
    if (!id) {
      throw new ValidationError('tenantId is required');
    }
    return this.request(`/cache/clear/tenant/${id}`, { method: 'POST' });
  }

  /**
   * Update the tenant ID for subsequent requests.
   */
  setTenantId(tenantId: string): void {
    this._tenantId = tenantId;
  }

  /**
   * Enable or disable debug logging.
   */
  setDebug(debug: boolean): void {
    this._debug = debug;
  }
}