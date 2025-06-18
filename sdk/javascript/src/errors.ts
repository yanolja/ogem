/**
 * Error classes for the Ogem JavaScript SDK
 */

export class OgemError extends Error {
  constructor(message: string, public cause?: Error) {
    super(message);
    this.name = 'OgemError';
  }
}

export class APIError extends OgemError {
  constructor(
    message: string,
    public status?: number,
    public code?: string,
    public requestId?: string,
    cause?: Error
  ) {
    super(message, cause);
    this.name = 'APIError';
  }
}

export class AuthenticationError extends APIError {
  constructor(message: string = 'Authentication failed', requestId?: string, cause?: Error) {
    super(message, 401, 'authentication_error', requestId, cause);
    this.name = 'AuthenticationError';
  }
}

export class RateLimitError extends APIError {
  constructor(
    message: string = 'Rate limit exceeded',
    public retryAfter?: number,
    requestId?: string,
    cause?: Error
  ) {
    super(message, 429, 'rate_limit_exceeded', requestId, cause);
    this.name = 'RateLimitError';
  }
}

export class TenantError extends APIError {
  constructor(
    message: string,
    public tenantId?: string,
    requestId?: string,
    cause?: Error
  ) {
    super(message, 403, 'tenant_error', requestId, cause);
    this.name = 'TenantError';
  }
}

export class ValidationError extends OgemError {
  constructor(
    message: string,
    public fieldErrors?: Record<string, string[]>,
    cause?: Error
  ) {
    super(message, cause);
    this.name = 'ValidationError';
  }
}

export class ModelError extends APIError {
  constructor(
    message: string,
    public modelId?: string,
    requestId?: string,
    cause?: Error
  ) {
    super(message, 400, 'model_error', requestId, cause);
    this.name = 'ModelError';
  }
}

export class ProviderError extends APIError {
  constructor(
    message: string,
    public provider?: string,
    requestId?: string,
    cause?: Error
  ) {
    super(message, 502, 'provider_error', requestId, cause);
    this.name = 'ProviderError';
  }
}

export class StreamError extends OgemError {
  constructor(message: string, cause?: Error) {
    super(message, cause);
    this.name = 'StreamError';
  }
}

/**
 * Create an appropriate error from an HTTP response
 */
export function createErrorFromResponse(status: number, data: any): APIError {
  const message = data?.error?.message || data?.message || `HTTP ${status}`;
  const code = data?.error?.code || data?.code;
  const requestId = data?.request_id || data?.requestId;

  switch (status) {
    case 401:
      return new AuthenticationError(message, requestId);
    case 403:
      if (code === 'tenant_error' || message.toLowerCase().includes('tenant')) {
        return new TenantError(message, data?.tenant_id, requestId);
      }
      return new APIError(message, status, code, requestId);
    case 422:
      return new ValidationError(message, data?.field_errors);
    case 429:
      return new RateLimitError(message, data?.retry_after, requestId);
    case 400:
      if (code === 'model_error' || message.toLowerCase().includes('model')) {
        return new ModelError(message, data?.model_id, requestId);
      }
      return new ValidationError(message, data?.field_errors);
    case 502:
    case 503:
    case 504:
      return new ProviderError(message, data?.provider, requestId);
    default:
      return new APIError(message, status, code, requestId);
  }
}