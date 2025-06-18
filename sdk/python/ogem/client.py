"""
Ogem Python SDK Client

Main client class for interacting with the Ogem AI proxy server.
"""

import json
import time
from typing import Dict, List, Optional, Union, Any, Iterator
from urllib.parse import urljoin

import httpx

from .exceptions import (
    OgemError,
    APIError,
    AuthenticationError,
    RateLimitError,
    TenantError,
    ValidationError
)
from .types import (
    ChatCompletion,
    ChatCompletionChunk,
    ChatCompletionMessageParam,
    EmbeddingCreateParams,
    Model,
    ResponseFormat
)
from .resources import Chat, Embeddings, Models


class Client:
    """
    Main Ogem client for making API requests.
    
    Args:
        base_url: The base URL of the Ogem server
        api_key: API key for authentication
        tenant_id: Optional tenant ID for multi-tenant environments
        timeout: Request timeout in seconds (default: 30)
        max_retries: Maximum number of retries for failed requests (default: 3)
        debug: Enable debug logging (default: False)
    
    Example:
        ```python
        client = ogem.Client(
            base_url="http://localhost:8080",
            api_key="your-api-key",
            tenant_id="your-tenant-id"
        )
        ```
    """
    
    def __init__(
        self,
        *,
        base_url: str,
        api_key: str,
        tenant_id: Optional[str] = None,
        timeout: float = 30.0,
        max_retries: int = 3,
        debug: bool = False,
        **kwargs
    ):
        if not base_url:
            raise ValueError("base_url is required")
        if not api_key:
            raise ValueError("api_key is required")
        
        self.base_url = base_url.rstrip("/")
        self.api_key = api_key
        self.tenant_id = tenant_id
        self.debug = debug
        
        # Create HTTP client
        self._client = httpx.Client(
            timeout=timeout,
            **kwargs
        )
        
        # Initialize resources
        self.chat = Chat(self)
        self.embeddings = Embeddings(self)
        self.models = Models(self)
    
    def _get_headers(self) -> Dict[str, str]:
        """Get default headers for requests."""
        headers = {
            "Authorization": f"Bearer {self.api_key}",
            "Content-Type": "application/json",
            "User-Agent": f"ogem-python/{self.__module__.split('.')[0]}",
        }
        
        if self.tenant_id:
            headers["X-Tenant-ID"] = self.tenant_id
        
        return headers
    
    def _make_request(
        self,
        method: str,
        endpoint: str,
        *,
        json_data: Optional[Dict[str, Any]] = None,
        params: Optional[Dict[str, Any]] = None,
        stream: bool = False
    ) -> Union[Dict[str, Any], Iterator[str]]:
        """
        Make an HTTP request to the Ogem API.
        
        Args:
            method: HTTP method (GET, POST, etc.)
            endpoint: API endpoint (relative to base_url)
            json_data: JSON data to send in request body
            params: Query parameters
            stream: Whether to stream the response
            
        Returns:
            Response data or stream iterator
        """
        url = urljoin(self.base_url, endpoint.lstrip("/"))
        headers = self._get_headers()
        
        if stream:
            headers["Accept"] = "text/event-stream"
            headers["Cache-Control"] = "no-cache"
        
        if self.debug:
            print(f"DEBUG: {method} {url}")
            if json_data:
                print(f"DEBUG: Request body: {json.dumps(json_data, indent=2)}")
        
        try:
            if stream:
                response = self._client.stream(
                    method=method,
                    url=url,
                    headers=headers,
                    json=json_data,
                    params=params
                )
                return self._handle_stream_response(response)
            else:
                response = self._client.request(
                    method=method,
                    url=url,
                    headers=headers,
                    json=json_data,
                    params=params
                )
                return self._handle_response(response)
                
        except httpx.TimeoutException:
            raise OgemError("Request timed out")
        except httpx.ConnectError:
            raise OgemError("Failed to connect to Ogem server")
        except Exception as e:
            raise OgemError(f"Request failed: {str(e)}")
    
    def _handle_response(self, response: httpx.Response) -> Dict[str, Any]:
        """Handle non-streaming HTTP response."""
        if self.debug:
            print(f"DEBUG: Response status: {response.status_code}")
        
        if response.status_code == 200:
            return response.json()
        elif response.status_code == 401:
            raise AuthenticationError("Invalid API key")
        elif response.status_code == 403:
            error_data = self._parse_error_response(response)
            if "tenant" in error_data.get("type", "").lower():
                raise TenantError(error_data.get("message", "Tenant access denied"))
            raise APIError(
                message=error_data.get("message", "Access denied"),
                status_code=response.status_code,
                error_type=error_data.get("type")
            )
        elif response.status_code == 422:
            error_data = self._parse_error_response(response)
            raise ValidationError(error_data.get("message", "Validation error"))
        elif response.status_code == 429:
            error_data = self._parse_error_response(response)
            raise RateLimitError(error_data.get("message", "Rate limit exceeded"))
        else:
            error_data = self._parse_error_response(response)
            raise APIError(
                message=error_data.get("message", f"HTTP {response.status_code}"),
                status_code=response.status_code,
                error_type=error_data.get("type")
            )
    
    def _handle_stream_response(self, response_stream) -> Iterator[str]:
        """Handle streaming HTTP response."""
        try:
            response = response_stream.__enter__()
            
            if response.status_code != 200:
                self._handle_response(response)
            
            for line in response.iter_lines():
                if line.startswith("data: "):
                    data = line[6:]  # Remove "data: " prefix
                    if data.strip() == "[DONE]":
                        break
                    yield data
                    
        except Exception as e:
            raise OgemError(f"Stream error: {str(e)}")
        finally:
            response_stream.__exit__(None, None, None)
    
    def _parse_error_response(self, response: httpx.Response) -> Dict[str, Any]:
        """Parse error response from API."""
        try:
            error_data = response.json()
            if "error" in error_data:
                return error_data["error"]
            return error_data
        except:
            return {"message": response.text or f"HTTP {response.status_code}"}
    
    def health(self) -> Dict[str, Any]:
        """
        Check the health status of the Ogem server.
        
        Returns:
            Health status information
        """
        return self._make_request("GET", "/health")
    
    def stats(self) -> Dict[str, Any]:
        """
        Get server statistics (requires appropriate permissions).
        
        Returns:
            Server statistics
        """
        return self._make_request("GET", "/stats")
    
    def cache_stats(self) -> Dict[str, Any]:
        """
        Get cache statistics (requires appropriate permissions).
        
        Returns:
            Cache statistics
        """
        return self._make_request("GET", "/cache/stats")
    
    def tenant_usage(self, tenant_id: Optional[str] = None) -> Dict[str, Any]:
        """
        Get tenant usage metrics (requires appropriate permissions).
        
        Args:
            tenant_id: Tenant ID (uses client's tenant_id if not provided)
            
        Returns:
            Tenant usage metrics
        """
        if not tenant_id:
            tenant_id = self.tenant_id
        if not tenant_id:
            raise ValueError("tenant_id is required")
        
        return self._make_request("GET", f"/tenants/{tenant_id}/usage")
    
    def clear_cache(self) -> Dict[str, Any]:
        """
        Clear all cache entries (requires appropriate permissions).
        
        Returns:
            Success message
        """
        return self._make_request("POST", "/cache/clear")
    
    def clear_tenant_cache(self, tenant_id: Optional[str] = None) -> Dict[str, Any]:
        """
        Clear cache for a specific tenant (requires appropriate permissions).
        
        Args:
            tenant_id: Tenant ID (uses client's tenant_id if not provided)
            
        Returns:
            Success message
        """
        if not tenant_id:
            tenant_id = self.tenant_id
        if not tenant_id:
            raise ValueError("tenant_id is required")
        
        return self._make_request("POST", f"/cache/clear/tenant/{tenant_id}")
    
    def set_tenant_id(self, tenant_id: str) -> None:
        """
        Update the tenant ID for subsequent requests.
        
        Args:
            tenant_id: New tenant ID
        """
        self.tenant_id = tenant_id
    
    def set_debug(self, debug: bool) -> None:
        """
        Enable or disable debug logging.
        
        Args:
            debug: Whether to enable debug logging
        """
        self.debug = debug
    
    def close(self) -> None:
        """Close the HTTP client."""
        self._client.close()
    
    def __enter__(self):
        return self
    
    def __exit__(self, exc_type, exc_val, exc_tb):
        self.close()


class AsyncClient:
    """
    Async version of the Ogem client.
    
    Similar to the sync client but uses httpx.AsyncClient for async operations.
    """
    
    def __init__(
        self,
        *,
        base_url: str,
        api_key: str,
        tenant_id: Optional[str] = None,
        timeout: float = 30.0,
        max_retries: int = 3,
        debug: bool = False,
        **kwargs
    ):
        if not base_url:
            raise ValueError("base_url is required")
        if not api_key:
            raise ValueError("api_key is required")
        
        self.base_url = base_url.rstrip("/")
        self.api_key = api_key
        self.tenant_id = tenant_id
        self.debug = debug
        
        # Create async HTTP client
        self._client = httpx.AsyncClient(
            timeout=timeout,
            **kwargs
        )
    
    def _get_headers(self) -> Dict[str, str]:
        """Get default headers for requests."""
        headers = {
            "Authorization": f"Bearer {self.api_key}",
            "Content-Type": "application/json",
            "User-Agent": f"ogem-python-async/{self.__module__.split('.')[0]}",
        }
        
        if self.tenant_id:
            headers["X-Tenant-ID"] = self.tenant_id
        
        return headers
    
    async def _make_request(
        self,
        method: str,
        endpoint: str,
        *,
        json_data: Optional[Dict[str, Any]] = None,
        params: Optional[Dict[str, Any]] = None
    ) -> Dict[str, Any]:
        """Make an async HTTP request to the Ogem API."""
        url = urljoin(self.base_url, endpoint.lstrip("/"))
        headers = self._get_headers()
        
        if self.debug:
            print(f"DEBUG: {method} {url}")
            if json_data:
                print(f"DEBUG: Request body: {json.dumps(json_data, indent=2)}")
        
        try:
            response = await self._client.request(
                method=method,
                url=url,
                headers=headers,
                json=json_data,
                params=params
            )
            return self._handle_response(response)
                
        except httpx.TimeoutException:
            raise OgemError("Request timed out")
        except httpx.ConnectError:
            raise OgemError("Failed to connect to Ogem server")
        except Exception as e:
            raise OgemError(f"Request failed: {str(e)}")
    
    def _handle_response(self, response: httpx.Response) -> Dict[str, Any]:
        """Handle HTTP response (same as sync client)."""
        if self.debug:
            print(f"DEBUG: Response status: {response.status_code}")
        
        if response.status_code == 200:
            return response.json()
        elif response.status_code == 401:
            raise AuthenticationError("Invalid API key")
        elif response.status_code == 403:
            error_data = self._parse_error_response(response)
            if "tenant" in error_data.get("type", "").lower():
                raise TenantError(error_data.get("message", "Tenant access denied"))
            raise APIError(
                message=error_data.get("message", "Access denied"),
                status_code=response.status_code,
                error_type=error_data.get("type")
            )
        elif response.status_code == 422:
            error_data = self._parse_error_response(response)
            raise ValidationError(error_data.get("message", "Validation error"))
        elif response.status_code == 429:
            error_data = self._parse_error_response(response)
            raise RateLimitError(error_data.get("message", "Rate limit exceeded"))
        else:
            error_data = self._parse_error_response(response)
            raise APIError(
                message=error_data.get("message", f"HTTP {response.status_code}"),
                status_code=response.status_code,
                error_type=error_data.get("type")
            )
    
    def _parse_error_response(self, response: httpx.Response) -> Dict[str, Any]:
        """Parse error response from API."""
        try:
            error_data = response.json()
            if "error" in error_data:
                return error_data["error"]
            return error_data
        except:
            return {"message": response.text or f"HTTP {response.status_code}"}
    
    async def health(self) -> Dict[str, Any]:
        """Check the health status of the Ogem server."""
        return await self._make_request("GET", "/health")
    
    async def stats(self) -> Dict[str, Any]:
        """Get server statistics."""
        return await self._make_request("GET", "/stats")
    
    async def cache_stats(self) -> Dict[str, Any]:
        """Get cache statistics."""
        return await self._make_request("GET", "/cache/stats")
    
    async def tenant_usage(self, tenant_id: Optional[str] = None) -> Dict[str, Any]:
        """Get tenant usage metrics."""
        if not tenant_id:
            tenant_id = self.tenant_id
        if not tenant_id:
            raise ValueError("tenant_id is required")
        
        return await self._make_request("GET", f"/tenants/{tenant_id}/usage")
    
    def set_tenant_id(self, tenant_id: str) -> None:
        """Update the tenant ID for subsequent requests."""
        self.tenant_id = tenant_id
    
    def set_debug(self, debug: bool) -> None:
        """Enable or disable debug logging."""
        self.debug = debug
    
    async def close(self) -> None:
        """Close the async HTTP client."""
        await self._client.aclose()
    
    async def __aenter__(self):
        return self
    
    async def __aexit__(self, exc_type, exc_val, exc_tb):
        await self.close()