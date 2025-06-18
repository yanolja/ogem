"""
Type definitions for the Ogem Python SDK.

This module contains all the data classes and type definitions used
throughout the SDK for API requests and responses.
"""

from typing import Any, Dict, List, Optional, Union, Literal
from dataclasses import dataclass
from datetime import datetime


# Message types
@dataclass
class ChatCompletionMessage:
    """A message in a chat completion."""
    role: Literal["system", "user", "assistant", "tool", "function"]
    content: Optional[Union[str, List[Dict[str, Any]]]] = None
    name: Optional[str] = None
    function_call: Optional["FunctionCall"] = None
    tool_calls: Optional[List["ToolCall"]] = None
    tool_call_id: Optional[str] = None


# Message parameters for requests
ChatCompletionMessageParam = Dict[str, Any]


@dataclass
class FunctionCall:
    """A function call in a message."""
    name: str
    arguments: str


@dataclass
class ToolCall:
    """A tool call in a message."""
    id: str
    type: str
    function: FunctionCall


@dataclass
class Function:
    """A function definition."""
    name: str
    description: Optional[str] = None
    parameters: Optional[Dict[str, Any]] = None


@dataclass
class Tool:
    """A tool definition."""
    type: str
    function: Function


@dataclass
class ResponseFormat:
    """Response format specification."""
    type: Literal["text", "json_object"]


# Choice and completion types
@dataclass
class Choice:
    """A choice in a chat completion response."""
    index: int
    message: ChatCompletionMessage
    finish_reason: Optional[str]
    logprobs: Optional[Dict[str, Any]] = None


@dataclass
class ChoiceDelta:
    """A delta choice in a streaming chat completion."""
    index: int
    delta: ChatCompletionMessage
    finish_reason: Optional[str]
    logprobs: Optional[Dict[str, Any]] = None


@dataclass
class Usage:
    """Token usage information."""
    prompt_tokens: int
    completion_tokens: int
    total_tokens: int


@dataclass
class ChatCompletion:
    """A chat completion response."""
    id: str
    object: str
    created: int
    model: str
    choices: List[Choice]
    usage: Usage
    system_fingerprint: Optional[str] = None


@dataclass
class ChatCompletionChunk:
    """A chunk in a streaming chat completion."""
    id: str
    object: str
    created: int
    model: str
    choices: List[ChoiceDelta]
    system_fingerprint: Optional[str] = None


# Embedding types
@dataclass
class Embedding:
    """An embedding object."""
    object: str
    embedding: List[float]
    index: int


@dataclass
class EmbeddingCreateParams:
    """Parameters for creating embeddings."""
    model: str
    input: Union[str, List[str], List[int], List[List[int]]]
    encoding_format: Optional[str] = None
    dimensions: Optional[int] = None
    user: Optional[str] = None


@dataclass
class EmbeddingResponse:
    """Response from embeddings API."""
    object: str
    data: List[Embedding]
    model: str
    usage: Usage


# Model types
@dataclass
class Model:
    """A model object."""
    id: str
    object: str
    created: int
    owned_by: str
    permission: Optional[List[Dict[str, Any]]] = None


@dataclass
class ModelList:
    """List of models."""
    object: str
    data: List[Model]


# Health and stats types
@dataclass
class HealthStatus:
    """Health status response."""
    status: str
    version: str
    uptime: str
    timestamp: str
    services: Optional[Dict[str, Any]] = None


@dataclass
class RequestStats:
    """Request statistics."""
    total: int
    successful: int
    failed: int
    success_rate: float


@dataclass
class PerformanceStats:
    """Performance statistics."""
    average_latency: str
    throughput_rpm: float
    error_rate: float


@dataclass
class ServerStats:
    """Server statistics."""
    requests: RequestStats
    performance: PerformanceStats
    providers: Dict[str, Any]
    cache: Optional[Dict[str, Any]] = None
    tenants: Optional[Dict[str, Any]] = None
    generated_at: Optional[str] = None


@dataclass
class CacheStats:
    """Cache statistics."""
    hits: int
    misses: int
    hit_rate: float
    total_entries: int
    memory_usage_mb: float
    exact_hits: int
    semantic_hits: int
    token_hits: int
    tenant_stats: Optional[Dict[str, Any]] = None
    last_updated: Optional[str] = None


@dataclass
class TenantUsage:
    """Tenant usage metrics."""
    tenant_id: str
    requests_this_hour: int
    requests_this_day: int
    requests_this_month: int
    tokens_this_hour: int
    tokens_this_day: int
    tokens_this_month: int
    cost_this_hour: float
    cost_this_day: float
    cost_this_month: float
    storage_used_gb: int
    files_count: int
    active_users: int
    teams_count: int
    projects_count: int
    last_updated: str


# Constants for model names
class Models:
    """Common model identifiers."""
    
    # OpenAI models
    GPT_4_TURBO = "gpt-4-turbo-preview"
    GPT_4 = "gpt-4"
    GPT_4_32K = "gpt-4-32k"
    GPT_3_5_TURBO = "gpt-3.5-turbo"
    GPT_3_5_TURBO_16K = "gpt-3.5-turbo-16k"
    
    # Anthropic models
    CLAUDE_3_OPUS = "claude-3-opus-20240229"
    CLAUDE_3_SONNET = "claude-3-sonnet-20240229"
    CLAUDE_3_HAIKU = "claude-3-haiku-20240307"
    CLAUDE_2_1 = "claude-2.1"
    CLAUDE_2 = "claude-2"
    CLAUDE_INSTANT = "claude-instant-1.2"
    
    # Google models
    GEMINI_PRO = "gemini-pro"
    GEMINI_PRO_VISION = "gemini-pro-vision"
    
    # Embedding models
    TEXT_EMBEDDING_ADA_002 = "text-embedding-ada-002"
    TEXT_EMBEDDING_3_SMALL = "text-embedding-3-small"
    TEXT_EMBEDDING_3_LARGE = "text-embedding-3-large"


class Roles:
    """Message role constants."""
    SYSTEM = "system"
    USER = "user"
    ASSISTANT = "assistant"
    TOOL = "tool"
    FUNCTION = "function"


class ResponseFormats:
    """Response format constants."""
    TEXT = "text"
    JSON_OBJECT = "json_object"


class FinishReasons:
    """Finish reason constants."""
    STOP = "stop"
    LENGTH = "length"
    FUNCTION_CALL = "function_call"
    TOOL_CALLS = "tool_calls"
    CONTENT_FILTER = "content_filter"


# Helper functions for creating common objects
def create_user_message(content: str, name: Optional[str] = None) -> ChatCompletionMessageParam:
    """Create a user message."""
    message = {
        "role": Roles.USER,
        "content": content
    }
    if name:
        message["name"] = name
    return message


def create_system_message(content: str) -> ChatCompletionMessageParam:
    """Create a system message."""
    return {
        "role": Roles.SYSTEM,
        "content": content
    }


def create_assistant_message(
    content: Optional[str] = None,
    function_call: Optional[Dict[str, Any]] = None,
    tool_calls: Optional[List[Dict[str, Any]]] = None
) -> ChatCompletionMessageParam:
    """Create an assistant message."""
    message = {"role": Roles.ASSISTANT}
    
    if content:
        message["content"] = content
    if function_call:
        message["function_call"] = function_call
    if tool_calls:
        message["tool_calls"] = tool_calls
    
    return message


def create_tool_message(content: str, tool_call_id: str) -> ChatCompletionMessageParam:
    """Create a tool response message."""
    return {
        "role": Roles.TOOL,
        "content": content,
        "tool_call_id": tool_call_id
    }


def create_function_message(content: str, name: str) -> ChatCompletionMessageParam:
    """Create a function response message."""
    return {
        "role": Roles.FUNCTION,
        "content": content,
        "name": name
    }


def create_multimodal_message(
    text: str,
    image_urls: List[str],
    image_detail: str = "auto"
) -> ChatCompletionMessageParam:
    """Create a multimodal message with text and images."""
    content = [{"type": "text", "text": text}]
    
    for url in image_urls:
        content.append({
            "type": "image_url",
            "image_url": {
                "url": url,
                "detail": image_detail
            }
        })
    
    return {
        "role": Roles.USER,
        "content": content
    }


# Utility classes for request building
class ChatCompletionRequest:
    """Builder class for chat completion requests."""
    
    def __init__(self, model: str, messages: List[ChatCompletionMessageParam]):
        self.data = {
            "model": model,
            "messages": messages
        }
    
    def max_tokens(self, value: int) -> "ChatCompletionRequest":
        """Set max tokens."""
        self.data["max_tokens"] = value
        return self
    
    def temperature(self, value: float) -> "ChatCompletionRequest":
        """Set temperature."""
        self.data["temperature"] = value
        return self
    
    def top_p(self, value: float) -> "ChatCompletionRequest":
        """Set top_p."""
        self.data["top_p"] = value
        return self
    
    def stream(self, value: bool = True) -> "ChatCompletionRequest":
        """Enable/disable streaming."""
        self.data["stream"] = value
        return self
    
    def stop(self, value: Union[str, List[str]]) -> "ChatCompletionRequest":
        """Set stop sequences."""
        self.data["stop"] = value if isinstance(value, list) else [value]
        return self
    
    def presence_penalty(self, value: float) -> "ChatCompletionRequest":
        """Set presence penalty."""
        self.data["presence_penalty"] = value
        return self
    
    def frequency_penalty(self, value: float) -> "ChatCompletionRequest":
        """Set frequency penalty."""
        self.data["frequency_penalty"] = value
        return self
    
    def user(self, value: str) -> "ChatCompletionRequest":
        """Set user identifier."""
        self.data["user"] = value
        return self
    
    def tools(self, value: List[Dict[str, Any]]) -> "ChatCompletionRequest":
        """Set tools."""
        self.data["tools"] = value
        return self
    
    def tool_choice(self, value: Union[str, Dict[str, Any]]) -> "ChatCompletionRequest":
        """Set tool choice."""
        self.data["tool_choice"] = value
        return self
    
    def response_format(self, value: Dict[str, Any]) -> "ChatCompletionRequest":
        """Set response format."""
        self.data["response_format"] = value
        return self
    
    def seed(self, value: int) -> "ChatCompletionRequest":
        """Set seed for deterministic outputs."""
        self.data["seed"] = value
        return self
    
    def logprobs(self, value: bool = True) -> "ChatCompletionRequest":
        """Enable log probabilities."""
        self.data["logprobs"] = value
        return self
    
    def top_logprobs(self, value: int) -> "ChatCompletionRequest":
        """Set top logprobs count."""
        self.data["top_logprobs"] = value
        return self
    
    def build(self) -> Dict[str, Any]:
        """Build the final request dictionary."""
        return self.data.copy()


class EmbeddingRequest:
    """Builder class for embedding requests."""
    
    def __init__(self, model: str, input_data: Union[str, List[str]]):
        self.data = {
            "model": model,
            "input": input_data
        }
    
    def encoding_format(self, value: str) -> "EmbeddingRequest":
        """Set encoding format."""
        self.data["encoding_format"] = value
        return self
    
    def dimensions(self, value: int) -> "EmbeddingRequest":
        """Set dimensions (for models that support it)."""
        self.data["dimensions"] = value
        return self
    
    def user(self, value: str) -> "EmbeddingRequest":
        """Set user identifier."""
        self.data["user"] = value
        return self
    
    def build(self) -> Dict[str, Any]:
        """Build the final request dictionary."""
        return self.data.copy()