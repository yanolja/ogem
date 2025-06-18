"""
Resource classes for the Ogem Python SDK.

This module contains resource classes that handle specific API endpoints
and provide a structured interface for different API operations.
"""

import json
from typing import Dict, List, Optional, Union, Any, Iterator

from .types import (
    ChatCompletion,
    ChatCompletionChunk,
    ChatCompletionMessageParam,
    EmbeddingResponse,
    Model,
    ModelList,
    ChatCompletionRequest,
    EmbeddingRequest
)
from .exceptions import StreamError, ValidationError


class Chat:
    """
    Resource class for chat completion operations.
    
    Handles all chat-related API endpoints including regular completions
    and streaming completions.
    """
    
    def __init__(self, client):
        self._client = client
        self.completions = ChatCompletions(client)


class ChatCompletions:
    """Handle chat completion requests."""
    
    def __init__(self, client):
        self._client = client
    
    def create(
        self,
        *,
        model: str,
        messages: List[ChatCompletionMessageParam],
        max_tokens: Optional[int] = None,
        temperature: Optional[float] = None,
        top_p: Optional[float] = None,
        n: Optional[int] = None,
        stream: Optional[bool] = None,
        stop: Optional[Union[str, List[str]]] = None,
        presence_penalty: Optional[float] = None,
        frequency_penalty: Optional[float] = None,
        logit_bias: Optional[Dict[str, int]] = None,
        user: Optional[str] = None,
        functions: Optional[List[Dict[str, Any]]] = None,
        function_call: Optional[Union[str, Dict[str, Any]]] = None,
        tools: Optional[List[Dict[str, Any]]] = None,
        tool_choice: Optional[Union[str, Dict[str, Any]]] = None,
        response_format: Optional[Dict[str, Any]] = None,
        seed: Optional[int] = None,
        logprobs: Optional[bool] = None,
        top_logprobs: Optional[int] = None,
        **kwargs
    ) -> Union[ChatCompletion, Iterator[ChatCompletionChunk]]:
        """
        Create a chat completion.
        
        Args:
            model: The model to use for completion
            messages: List of messages in the conversation
            max_tokens: Maximum number of tokens to generate
            temperature: Sampling temperature (0-2)
            top_p: Nucleus sampling parameter
            n: Number of completions to generate
            stream: Whether to stream the response
            stop: Stop sequences
            presence_penalty: Presence penalty (-2 to 2)
            frequency_penalty: Frequency penalty (-2 to 2)
            logit_bias: Logit bias for specific tokens
            user: User identifier for tracking
            functions: Available functions (deprecated, use tools)
            function_call: Function calling behavior (deprecated, use tool_choice)
            tools: Available tools for the model
            tool_choice: Tool choice behavior
            response_format: Response format specification
            seed: Seed for deterministic outputs
            logprobs: Whether to return log probabilities
            top_logprobs: Number of top log probabilities to return
            **kwargs: Additional parameters
            
        Returns:
            ChatCompletion or Iterator[ChatCompletionChunk] if streaming
        """
        # Build request data
        request_data = {
            "model": model,
            "messages": messages
        }
        
        # Add optional parameters
        if max_tokens is not None:
            request_data["max_tokens"] = max_tokens
        if temperature is not None:
            request_data["temperature"] = temperature
        if top_p is not None:
            request_data["top_p"] = top_p
        if n is not None:
            request_data["n"] = n
        if stream is not None:
            request_data["stream"] = stream
        if stop is not None:
            request_data["stop"] = stop if isinstance(stop, list) else [stop]
        if presence_penalty is not None:
            request_data["presence_penalty"] = presence_penalty
        if frequency_penalty is not None:
            request_data["frequency_penalty"] = frequency_penalty
        if logit_bias is not None:
            request_data["logit_bias"] = logit_bias
        if user is not None:
            request_data["user"] = user
        if functions is not None:
            request_data["functions"] = functions
        if function_call is not None:
            request_data["function_call"] = function_call
        if tools is not None:
            request_data["tools"] = tools
        if tool_choice is not None:
            request_data["tool_choice"] = tool_choice
        if response_format is not None:
            request_data["response_format"] = response_format
        if seed is not None:
            request_data["seed"] = seed
        if logprobs is not None:
            request_data["logprobs"] = logprobs
        if top_logprobs is not None:
            request_data["top_logprobs"] = top_logprobs
        
        # Add any additional kwargs
        request_data.update(kwargs)
        
        # Validate required fields
        if not model:
            raise ValidationError("model is required")
        if not messages:
            raise ValidationError("messages is required")
        
        # Handle streaming vs non-streaming
        if request_data.get("stream", False):
            return self._create_stream(request_data)
        else:
            response = self._client._make_request(
                "POST",
                "/v1/chat/completions",
                json_data=request_data
            )
            return self._parse_completion_response(response)
    
    def _create_stream(self, request_data: Dict[str, Any]) -> Iterator[ChatCompletionChunk]:
        """Create a streaming chat completion."""
        stream = self._client._make_request(
            "POST",
            "/v1/chat/completions",
            json_data=request_data,
            stream=True
        )
        
        for line in stream:
            if line.strip() == "[DONE]":
                break
            
            try:
                chunk_data = json.loads(line)
                yield self._parse_chunk_response(chunk_data)
            except json.JSONDecodeError as e:
                raise StreamError(f"Failed to parse stream chunk: {e}")
    
    def _parse_completion_response(self, data: Dict[str, Any]) -> ChatCompletion:
        """Parse a non-streaming completion response."""
        return ChatCompletion(**data)
    
    def _parse_chunk_response(self, data: Dict[str, Any]) -> ChatCompletionChunk:
        """Parse a streaming chunk response."""
        return ChatCompletionChunk(**data)


class Embeddings:
    """
    Resource class for embedding operations.
    
    Handles creating embeddings from text inputs.
    """
    
    def __init__(self, client):
        self._client = client
    
    def create(
        self,
        *,
        model: str,
        input: Union[str, List[str], List[int], List[List[int]]],
        encoding_format: Optional[str] = None,
        dimensions: Optional[int] = None,
        user: Optional[str] = None,
        **kwargs
    ) -> EmbeddingResponse:
        """
        Create embeddings for the given input.
        
        Args:
            model: The embedding model to use
            input: Input text(s) to embed
            encoding_format: Encoding format (float, base64)
            dimensions: Number of dimensions (for supported models)
            user: User identifier for tracking
            **kwargs: Additional parameters
            
        Returns:
            EmbeddingResponse containing the embeddings
        """
        # Build request data
        request_data = {
            "model": model,
            "input": input
        }
        
        # Add optional parameters
        if encoding_format is not None:
            request_data["encoding_format"] = encoding_format
        if dimensions is not None:
            request_data["dimensions"] = dimensions
        if user is not None:
            request_data["user"] = user
        
        # Add any additional kwargs
        request_data.update(kwargs)
        
        # Validate required fields
        if not model:
            raise ValidationError("model is required")
        if not input:
            raise ValidationError("input is required")
        
        response = self._client._make_request(
            "POST",
            "/v1/embeddings",
            json_data=request_data
        )
        
        return EmbeddingResponse(**response)


class Models:
    """
    Resource class for model operations.
    
    Handles listing available models and retrieving model information.
    """
    
    def __init__(self, client):
        self._client = client
    
    def list(self) -> ModelList:
        """
        List all available models.
        
        Returns:
            ModelList containing all available models
        """
        response = self._client._make_request("GET", "/v1/models")
        return ModelList(**response)
    
    def retrieve(self, model_id: str) -> Model:
        """
        Retrieve information about a specific model.
        
        Args:
            model_id: The ID of the model to retrieve
            
        Returns:
            Model information
        """
        if not model_id:
            raise ValidationError("model_id is required")
        
        response = self._client._make_request("GET", f"/v1/models/{model_id}")
        return Model(**response)


class Tenants:
    """
    Resource class for tenant operations.
    
    Handles tenant-specific operations like usage tracking and management.
    """
    
    def __init__(self, client):
        self._client = client
    
    def usage(self, tenant_id: Optional[str] = None) -> Dict[str, Any]:
        """
        Get usage metrics for a tenant.
        
        Args:
            tenant_id: Tenant ID (uses client's tenant_id if not provided)
            
        Returns:
            Tenant usage metrics
        """
        if not tenant_id:
            tenant_id = self._client.tenant_id
        if not tenant_id:
            raise ValidationError("tenant_id is required")
        
        return self._client._make_request("GET", f"/tenants/{tenant_id}/usage")
    
    def limits(self, tenant_id: Optional[str] = None) -> Dict[str, Any]:
        """
        Get limits for a tenant.
        
        Args:
            tenant_id: Tenant ID (uses client's tenant_id if not provided)
            
        Returns:
            Tenant limits
        """
        if not tenant_id:
            tenant_id = self._client.tenant_id
        if not tenant_id:
            raise ValidationError("tenant_id is required")
        
        return self._client._make_request("GET", f"/tenants/{tenant_id}/limits")
    
    def settings(self, tenant_id: Optional[str] = None) -> Dict[str, Any]:
        """
        Get settings for a tenant.
        
        Args:
            tenant_id: Tenant ID (uses client's tenant_id if not provided)
            
        Returns:
            Tenant settings
        """
        if not tenant_id:
            tenant_id = self._client.tenant_id
        if not tenant_id:
            raise ValidationError("tenant_id is required")
        
        return self._client._make_request("GET", f"/tenants/{tenant_id}/settings")


class Cache:
    """
    Resource class for cache operations.
    
    Handles cache management and statistics.
    """
    
    def __init__(self, client):
        self._client = client
    
    def stats(self, tenant_id: Optional[str] = None) -> Dict[str, Any]:
        """
        Get cache statistics.
        
        Args:
            tenant_id: Optional tenant ID for tenant-specific stats
            
        Returns:
            Cache statistics
        """
        if tenant_id:
            return self._client._make_request("GET", f"/cache/stats/tenant/{tenant_id}")
        else:
            return self._client._make_request("GET", "/cache/stats")
    
    def clear(self, tenant_id: Optional[str] = None) -> Dict[str, Any]:
        """
        Clear cache entries.
        
        Args:
            tenant_id: Optional tenant ID to clear only tenant-specific entries
            
        Returns:
            Success message
        """
        if tenant_id:
            return self._client._make_request("POST", f"/cache/clear/tenant/{tenant_id}")
        else:
            return self._client._make_request("POST", "/cache/clear")
    
    def entries(
        self,
        tenant_id: Optional[str] = None,
        model: Optional[str] = None,
        limit: int = 50,
        offset: int = 0
    ) -> Dict[str, Any]:
        """
        List cache entries.
        
        Args:
            tenant_id: Filter by tenant ID
            model: Filter by model
            limit: Maximum number of entries to return
            offset: Number of entries to skip
            
        Returns:
            List of cache entries with pagination info
        """
        params = {"limit": limit, "offset": offset}
        if tenant_id:
            params["tenant_id"] = tenant_id
        if model:
            params["model"] = model
        
        return self._client._make_request(
            "GET",
            "/cache/entries",
            params=params
        )
    
    def analysis(self) -> Dict[str, Any]:
        """
        Get cache analysis and insights.
        
        Returns:
            Cache analysis data
        """
        return self._client._make_request("GET", "/cache/analysis")
    
    def adaptive_state(self) -> Dict[str, Any]:
        """
        Get adaptive caching state.
        
        Returns:
            Adaptive caching state information
        """
        return self._client._make_request("GET", "/cache/adaptive/state")
    
    def set_strategy(self, strategy: str, reason: Optional[str] = None) -> Dict[str, Any]:
        """
        Set the adaptive caching strategy.
        
        Args:
            strategy: Caching strategy to use
            reason: Optional reason for the change
            
        Returns:
            Success message with strategy change info
        """
        request_data = {"strategy": strategy}
        if reason:
            request_data["reason"] = reason
        
        return self._client._make_request(
            "POST",
            "/cache/adaptive/strategy",
            json_data=request_data
        )


class Monitoring:
    """
    Resource class for monitoring and observability.
    
    Handles health checks, statistics, and system monitoring.
    """
    
    def __init__(self, client):
        self._client = client
    
    def health(self) -> Dict[str, Any]:
        """
        Get server health status.
        
        Returns:
            Health status information
        """
        return self._client._make_request("GET", "/health")
    
    def stats(self) -> Dict[str, Any]:
        """
        Get server statistics.
        
        Returns:
            Server statistics
        """
        return self._client._make_request("GET", "/stats")
    
    def metrics(self) -> Dict[str, Any]:
        """
        Get detailed metrics.
        
        Returns:
            Detailed metrics data
        """
        return self._client._make_request("GET", "/metrics")


# Helper functions for common operations
def create_chat_request(
    model: str,
    messages: List[ChatCompletionMessageParam],
    **kwargs
) -> ChatCompletionRequest:
    """
    Create a chat completion request with fluent interface.
    
    Args:
        model: The model to use
        messages: List of messages
        **kwargs: Additional parameters
        
    Returns:
        ChatCompletionRequest builder instance
    """
    request = ChatCompletionRequest(model, messages)
    
    # Apply any provided parameters
    for key, value in kwargs.items():
        if hasattr(request, key):
            getattr(request, key)(value)
    
    return request


def create_embedding_request(
    model: str,
    input_data: Union[str, List[str]],
    **kwargs
) -> EmbeddingRequest:
    """
    Create an embedding request with fluent interface.
    
    Args:
        model: The embedding model to use
        input_data: Input text(s) to embed
        **kwargs: Additional parameters
        
    Returns:
        EmbeddingRequest builder instance
    """
    request = EmbeddingRequest(model, input_data)
    
    # Apply any provided parameters
    for key, value in kwargs.items():
        if hasattr(request, key):
            getattr(request, key)(value)
    
    return request