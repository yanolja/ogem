"""
Exception classes for the Ogem Python SDK.

This module defines all the custom exceptions that can be raised
by the SDK when interacting with the Ogem API.
"""

from typing import Optional, Any, Dict


class OgemError(Exception):
    """Base exception class for all Ogem SDK errors."""
    
    def __init__(self, message: str):
        self.message = message
        super().__init__(message)


class APIError(OgemError):
    """
    Exception raised when the API returns an error response.
    
    Attributes:
        message: The error message
        status_code: HTTP status code
        error_type: The type of error returned by the API
        error_code: Optional error code
        details: Additional error details
    """
    
    def __init__(
        self,
        message: str,
        status_code: Optional[int] = None,
        error_type: Optional[str] = None,
        error_code: Optional[str] = None,
        details: Optional[Dict[str, Any]] = None
    ):
        self.status_code = status_code
        self.error_type = error_type
        self.error_code = error_code
        self.details = details or {}
        
        # Create comprehensive error message
        full_message = message
        if status_code:
            full_message = f"[{status_code}] {full_message}"
        if error_type:
            full_message = f"{error_type}: {full_message}"
        if error_code:
            full_message = f"{full_message} (code: {error_code})"
        
        super().__init__(full_message)


class AuthenticationError(APIError):
    """
    Exception raised when authentication fails.
    
    This typically happens when:
    - The API key is invalid or missing
    - The API key has expired
    - The API key doesn't have the required permissions
    """
    
    def __init__(self, message: str = "Authentication failed"):
        super().__init__(
            message=message,
            status_code=401,
            error_type="authentication_error"
        )


class RateLimitError(APIError):
    """
    Exception raised when rate limits are exceeded.
    
    This happens when:
    - Too many requests are made in a short time period
    - Token limits are exceeded
    - Cost limits are exceeded
    - Concurrent request limits are exceeded
    
    Attributes:
        retry_after: Number of seconds to wait before retrying (if provided)
        limit_type: Type of limit that was exceeded
    """
    
    def __init__(
        self,
        message: str = "Rate limit exceeded",
        retry_after: Optional[int] = None,
        limit_type: Optional[str] = None
    ):
        super().__init__(
            message=message,
            status_code=429,
            error_type="rate_limit_error",
            details={
                "retry_after": retry_after,
                "limit_type": limit_type
            }
        )
        self.retry_after = retry_after
        self.limit_type = limit_type


class TenantError(APIError):
    """
    Exception raised when there are tenant-related errors.
    
    This happens when:
    - The tenant ID is invalid or not found
    - The tenant is suspended or deleted
    - The tenant has exceeded its limits
    - The user doesn't have access to the tenant
    """
    
    def __init__(self, message: str, tenant_id: Optional[str] = None):
        super().__init__(
            message=message,
            status_code=403,
            error_type="tenant_error",
            details={"tenant_id": tenant_id}
        )
        self.tenant_id = tenant_id


class ValidationError(APIError):
    """
    Exception raised when request validation fails.
    
    This happens when:
    - Required fields are missing
    - Field values are invalid
    - The request format is incorrect
    - Model or provider constraints are violated
    
    Attributes:
        field_errors: Dictionary of field-specific errors
    """
    
    def __init__(
        self,
        message: str = "Validation error",
        field_errors: Optional[Dict[str, Any]] = None
    ):
        super().__init__(
            message=message,
            status_code=422,
            error_type="validation_error",
            details={"field_errors": field_errors or {}}
        )
        self.field_errors = field_errors or {}


class ModelError(APIError):
    """
    Exception raised when there are model-related errors.
    
    This happens when:
    - The requested model is not available
    - The model doesn't support the requested feature
    - The model is temporarily unavailable
    - Model-specific limits are exceeded
    """
    
    def __init__(self, message: str, model_id: Optional[str] = None):
        super().__init__(
            message=message,
            error_type="model_error",
            details={"model_id": model_id}
        )
        self.model_id = model_id


class ProviderError(APIError):
    """
    Exception raised when there are provider-related errors.
    
    This happens when:
    - The AI provider is unavailable
    - Provider-specific errors occur
    - Provider rate limits are hit
    - Provider authentication fails
    """
    
    def __init__(self, message: str, provider: Optional[str] = None):
        super().__init__(
            message=message,
            error_type="provider_error",
            details={"provider": provider}
        )
        self.provider = provider


class CacheError(OgemError):
    """
    Exception raised when there are cache-related errors.
    
    This happens when:
    - Cache operations fail
    - Cache is unavailable
    - Cache configuration is invalid
    """
    
    def __init__(self, message: str):
        super().__init__(f"Cache error: {message}")


class StreamError(OgemError):
    """
    Exception raised when streaming operations fail.
    
    This happens when:
    - Stream connection is lost
    - Stream data is corrupted
    - Stream parsing fails
    """
    
    def __init__(self, message: str):
        super().__init__(f"Stream error: {message}")


class TimeoutError(OgemError):
    """
    Exception raised when requests timeout.
    
    This happens when:
    - Request takes longer than the configured timeout
    - Connection timeout occurs
    - Read timeout occurs
    """
    
    def __init__(self, message: str = "Request timed out"):
        super().__init__(message)


class ConnectionError(OgemError):
    """
    Exception raised when connection to the Ogem server fails.
    
    This happens when:
    - Cannot connect to the server
    - Network issues occur
    - DNS resolution fails
    """
    
    def __init__(self, message: str = "Failed to connect to Ogem server"):
        super().__init__(message)


class ConfigurationError(OgemError):
    """
    Exception raised when there are configuration errors.
    
    This happens when:
    - Required configuration is missing
    - Configuration values are invalid
    - Conflicting configuration options
    """
    
    def __init__(self, message: str):
        super().__init__(f"Configuration error: {message}")


# Exception mapping for HTTP status codes
STATUS_CODE_TO_EXCEPTION = {
    400: ValidationError,
    401: AuthenticationError,
    403: TenantError,  # May be overridden based on error type
    404: ModelError,   # Usually model not found
    422: ValidationError,
    429: RateLimitError,
    500: ProviderError,  # Usually provider issues
    502: ProviderError,
    503: ProviderError,
    504: TimeoutError,
}


def create_exception_from_response(
    status_code: int,
    error_data: Dict[str, Any],
    default_message: str = "API request failed"
) -> APIError:
    """
    Create an appropriate exception based on the error response.
    
    Args:
        status_code: HTTP status code
        error_data: Error data from the API response
        default_message: Default message if none provided
        
    Returns:
        Appropriate exception instance
    """
    message = error_data.get("message", default_message)
    error_type = error_data.get("type", "unknown_error")
    error_code = error_data.get("code")
    
    # Determine exception class based on error type or status code
    if "authentication" in error_type.lower():
        return AuthenticationError(message)
    elif "rate_limit" in error_type.lower() or "rate limit" in message.lower():
        retry_after = error_data.get("retry_after")
        limit_type = error_data.get("limit_type")
        return RateLimitError(message, retry_after=retry_after, limit_type=limit_type)
    elif "tenant" in error_type.lower():
        tenant_id = error_data.get("tenant_id")
        return TenantError(message, tenant_id=tenant_id)
    elif "validation" in error_type.lower():
        field_errors = error_data.get("field_errors")
        return ValidationError(message, field_errors=field_errors)
    elif "model" in error_type.lower():
        model_id = error_data.get("model_id")
        return ModelError(message, model_id=model_id)
    elif "provider" in error_type.lower():
        provider = error_data.get("provider")
        return ProviderError(message, provider=provider)
    else:
        # Fall back to status code mapping
        exception_class = STATUS_CODE_TO_EXCEPTION.get(status_code, APIError)
        return exception_class(
            message=message,
            status_code=status_code,
            error_type=error_type,
            error_code=error_code,
            details=error_data
        )


# Retry utilities
def is_retryable_error(error: Exception) -> bool:
    """
    Determine if an error is retryable.
    
    Args:
        error: The exception to check
        
    Returns:
        True if the error is retryable, False otherwise
    """
    if isinstance(error, RateLimitError):
        return True
    elif isinstance(error, APIError):
        # Retry on server errors
        return error.status_code is not None and error.status_code >= 500
    elif isinstance(error, (TimeoutError, ConnectionError)):
        return True
    
    return False


def get_retry_delay(error: Exception, attempt: int, base_delay: float = 1.0) -> float:
    """
    Calculate the delay before retrying a failed request.
    
    Args:
        error: The exception that occurred
        attempt: The current attempt number (1-based)
        base_delay: Base delay in seconds
        
    Returns:
        Delay in seconds before retry
    """
    if isinstance(error, RateLimitError) and error.retry_after:
        return float(error.retry_after)
    
    # Exponential backoff with jitter
    import random
    delay = base_delay * (2 ** (attempt - 1))
    jitter = random.uniform(0, 0.1) * delay
    return delay + jitter