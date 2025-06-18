"""
Ogem Python SDK

Official Python client library for the Ogem AI proxy server.
Provides OpenAI-compatible API with advanced features like multi-tenancy,
intelligent caching, and enterprise security.

Example:
    ```python
    import ogem
    
    client = ogem.Client(
        base_url="http://localhost:8080",
        api_key="your-api-key",
        tenant_id="your-tenant-id"
    )
    
    response = client.chat.completions.create(
        model="gpt-4",
        messages=[
            {"role": "system", "content": "You are a helpful assistant."},
            {"role": "user", "content": "Hello, world!"}
        ]
    )
    
    print(response.choices[0].message.content)
    ```
"""

__version__ = "1.0.0"
__author__ = "Ogem Team"
__email__ = "support@ogem.ai"

from .client import Client
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
    ChatCompletionMessage,
    ChatCompletionMessageParam,
    Choice,
    ChoiceDelta,
    Embedding,
    EmbeddingCreateParams,
    Model,
    Usage,
    Function,
    FunctionCall,
    Tool,
    ToolCall,
    ResponseFormat
)

__all__ = [
    # Main client
    "Client",
    
    # Exceptions
    "OgemError",
    "APIError", 
    "AuthenticationError",
    "RateLimitError",
    "TenantError",
    "ValidationError",
    
    # Types
    "ChatCompletion",
    "ChatCompletionChunk", 
    "ChatCompletionMessage",
    "ChatCompletionMessageParam",
    "Choice",
    "ChoiceDelta",
    "Embedding",
    "EmbeddingCreateParams",
    "Model",
    "Usage",
    "Function",
    "FunctionCall",
    "Tool", 
    "ToolCall",
    "ResponseFormat"
]