#!/usr/bin/env python3
"""
Basic usage examples for the Ogem Python SDK.

This script demonstrates common use cases and patterns for using
the Ogem AI proxy server through the Python SDK.
"""

import asyncio
import os
import time
from typing import List

import ogem
from ogem.types import Models, create_user_message, create_system_message


def main():
    """Run all examples."""
    print("=== Ogem Python SDK Examples ===\n")
    
    # Example 1: Basic Chat Completion
    basic_chat_example()
    
    # Example 2: Multi-turn Conversation
    conversation_example()
    
    # Example 3: Function Calling
    function_calling_example()
    
    # Example 4: Streaming
    streaming_example()
    
    # Example 5: Embeddings
    embeddings_example()
    
    # Example 6: Multi-tenant Usage
    multi_tenant_example()
    
    # Example 7: Monitoring and Stats
    monitoring_example()
    
    # Example 8: Error Handling
    error_handling_example()
    
    # Example 9: Async Usage
    print("Running async example...")
    asyncio.run(async_example())
    
    print("\n=== All examples completed ===")


def basic_chat_example():
    """Basic chat completion example."""
    print("1. Basic Chat Completion")
    print("-" * 40)
    
    # Create client
    client = ogem.Client(
        base_url="http://localhost:8080",
        api_key="your-api-key",
        debug=True
    )
    
    try:
        # Simple chat completion
        response = client.chat.completions.create(
            model=Models.GPT_3_5_TURBO,
            messages=[
                create_system_message("You are a helpful assistant."),
                create_user_message("What is the capital of France?")
            ],
            max_tokens=100,
            temperature=0.7
        )
        
        print(f"Response: {response.choices[0].message.content}")
        print(f"Tokens used: {response.usage.total_tokens}")
        print(f"Model: {response.model}")
        
    except ogem.APIError as e:
        print(f"API Error: {e}")
    except Exception as e:
        print(f"Error: {e}")
    
    print()


def conversation_example():
    """Multi-turn conversation example."""
    print("2. Multi-turn Conversation")
    print("-" * 40)
    
    client = ogem.Client(
        base_url="http://localhost:8080",
        api_key="your-api-key"
    )
    
    # Start conversation
    messages = [
        create_system_message("You are a coding assistant. Keep responses concise.")
    ]
    
    questions = [
        "How do I create a list in Python?",
        "How do I add items to it?",
        "What about removing items?"
    ]
    
    try:
        for i, question in enumerate(questions, 1):
            print(f"Q{i}: {question}")
            
            # Add user message
            messages.append(create_user_message(question))
            
            # Get response
            response = client.chat.completions.create(
                model=Models.GPT_4,
                messages=messages,
                max_tokens=200,
                temperature=0.3
            )
            
            assistant_message = response.choices[0].message.content
            print(f"A{i}: {assistant_message}\n")
            
            # Add assistant response to conversation
            messages.append({
                "role": "assistant",
                "content": assistant_message
            })
            
    except ogem.APIError as e:
        print(f"API Error: {e}")
    
    print()


def function_calling_example():
    """Function calling example."""
    print("3. Function Calling")
    print("-" * 40)
    
    client = ogem.Client(
        base_url="http://localhost:8080",
        api_key="your-api-key"
    )
    
    # Define a weather function
    tools = [
        {
            "type": "function",
            "function": {
                "name": "get_weather",
                "description": "Get current weather for a location",
                "parameters": {
                    "type": "object",
                    "properties": {
                        "location": {
                            "type": "string",
                            "description": "City and state, e.g. San Francisco, CA"
                        },
                        "unit": {
                            "type": "string",
                            "enum": ["celsius", "fahrenheit"],
                            "description": "Temperature unit"
                        }
                    },
                    "required": ["location"]
                }
            }
        }
    ]
    
    try:
        response = client.chat.completions.create(
            model=Models.GPT_4,
            messages=[
                create_user_message("What's the weather like in New York?")
            ],
            tools=tools,
            tool_choice="auto"
        )
        
        choice = response.choices[0]
        if choice.message.tool_calls:
            tool_call = choice.message.tool_calls[0]
            print(f"Function called: {tool_call.function.name}")
            print(f"Arguments: {tool_call.function.arguments}")
        else:
            print(f"Response: {choice.message.content}")
            
    except ogem.APIError as e:
        print(f"API Error: {e}")
    
    print()


def streaming_example():
    """Streaming chat completion example."""
    print("4. Streaming Chat Completion")
    print("-" * 40)
    
    client = ogem.Client(
        base_url="http://localhost:8080",
        api_key="your-api-key"
    )
    
    try:
        print("Assistant: ", end="", flush=True)
        
        stream = client.chat.completions.create(
            model=Models.GPT_3_5_TURBO,
            messages=[
                create_system_message("You are a helpful assistant."),
                create_user_message("Write a short poem about coding.")
            ],
            max_tokens=200,
            temperature=0.8,
            stream=True
        )
        
        full_response = ""
        for chunk in stream:
            if chunk.choices and chunk.choices[0].delta.content:
                content = chunk.choices[0].delta.content
                print(content, end="", flush=True)
                full_response += content
        
        print(f"\n\nFull response length: {len(full_response)} characters")
        
    except ogem.APIError as e:
        print(f"API Error: {e}")
    except ogem.StreamError as e:
        print(f"Stream Error: {e}")
    
    print()


def embeddings_example():
    """Embeddings example."""
    print("5. Embeddings")
    print("-" * 40)
    
    client = ogem.Client(
        base_url="http://localhost:8080",
        api_key="your-api-key"
    )
    
    texts = [
        "The quick brown fox jumps over the lazy dog",
        "Machine learning is transforming technology",
        "Python is a popular programming language"
    ]
    
    try:
        response = client.embeddings.create(
            model=Models.TEXT_EMBEDDING_3_SMALL,
            input=texts
        )
        
        print(f"Generated {len(response.data)} embeddings")
        
        for i, embedding in enumerate(response.data):
            print(f"Text {i+1}: {len(embedding.embedding)} dimensions")
            print(f"  First 5 values: {embedding.embedding[:5]}")
        
        print(f"Total tokens: {response.usage.total_tokens}")
        
    except ogem.APIError as e:
        print(f"API Error: {e}")
    
    print()


def multi_tenant_example():
    """Multi-tenant usage example."""
    print("6. Multi-tenant Usage")
    print("-" * 40)
    
    # Client for tenant A
    client_a = ogem.Client(
        base_url="http://localhost:8080",
        api_key="your-api-key",
        tenant_id="tenant-a"
    )
    
    # Client for tenant B
    client_b = ogem.Client(
        base_url="http://localhost:8080",
        api_key="your-api-key", 
        tenant_id="tenant-b"
    )
    
    try:
        # Make requests from different tenants
        response_a = client_a.chat.completions.create(
            model=Models.GPT_3_5_TURBO,
            messages=[create_user_message("Hello from tenant A!")],
            max_tokens=50
        )
        
        response_b = client_b.chat.completions.create(
            model=Models.GPT_3_5_TURBO,
            messages=[create_user_message("Hello from tenant B!")],
            max_tokens=50
        )
        
        print(f"Tenant A response: {response_a.choices[0].message.content}")
        print(f"Tenant B response: {response_b.choices[0].message.content}")
        
        # Get usage stats for each tenant
        try:
            usage_a = client_a.tenant_usage()
            print(f"\nTenant A usage:")
            print(f"  Requests today: {usage_a['requests_this_day']}")
            print(f"  Cost today: ${usage_a['cost_this_day']:.4f}")
        except ogem.APIError as e:
            print(f"Could not get tenant A usage: {e}")
        
        try:
            usage_b = client_b.tenant_usage()
            print(f"\nTenant B usage:")
            print(f"  Requests today: {usage_b['requests_this_day']}")
            print(f"  Cost today: ${usage_b['cost_this_day']:.4f}")
        except ogem.APIError as e:
            print(f"Could not get tenant B usage: {e}")
            
    except ogem.APIError as e:
        print(f"API Error: {e}")
    
    print()


def monitoring_example():
    """Monitoring and stats example."""
    print("7. Monitoring and Stats")
    print("-" * 40)
    
    client = ogem.Client(
        base_url="http://localhost:8080",
        api_key="your-api-key"
    )
    
    try:
        # Health check
        health = client.health()
        print(f"Server status: {health['status']}")
        print(f"Version: {health['version']}")
        print(f"Uptime: {health['uptime']}")
        
        # Server stats
        try:
            stats = client.stats()
            print(f"\nServer Statistics:")
            print(f"  Total requests: {stats['requests']['total']}")
            print(f"  Success rate: {stats['requests']['success_rate']:.2%}")
            print(f"  Average latency: {stats['performance']['average_latency']}")
            print(f"  Throughput: {stats['performance']['throughput_rpm']:.1f} RPM")
        except ogem.APIError as e:
            print(f"Could not get server stats: {e}")
        
        # Cache stats
        try:
            cache_stats = client.cache_stats()
            print(f"\nCache Statistics:")
            print(f"  Hit rate: {cache_stats['hit_rate']:.2%}")
            print(f"  Total entries: {cache_stats['total_entries']}")
            print(f"  Memory usage: {cache_stats['memory_usage_mb']:.1f} MB")
        except ogem.APIError as e:
            print(f"Could not get cache stats: {e}")
        
        # List available models
        try:
            models = client.models.list()
            print(f"\nAvailable models: {len(models.data)}")
            for model in models.data[:5]:  # Show first 5
                print(f"  - {model.id} (by {model.owned_by})")
        except ogem.APIError as e:
            print(f"Could not list models: {e}")
            
    except ogem.APIError as e:
        print(f"Health check failed: {e}")
    
    print()


def error_handling_example():
    """Error handling example."""
    print("8. Error Handling")
    print("-" * 40)
    
    client = ogem.Client(
        base_url="http://localhost:8080",
        api_key="your-api-key"
    )
    
    # Example 1: Invalid model
    try:
        client.chat.completions.create(
            model="invalid-model",
            messages=[create_user_message("Hello")]
        )
    except ogem.ModelError as e:
        print(f"Model error: {e}")
    except ogem.APIError as e:
        print(f"API error: {e}")
    
    # Example 2: Rate limiting with retry
    max_retries = 3
    for attempt in range(max_retries):
        try:
            response = client.chat.completions.create(
                model=Models.GPT_3_5_TURBO,
                messages=[create_user_message("Test message")],
                max_tokens=10
            )
            print("Request succeeded!")
            break
            
        except ogem.RateLimitError as e:
            if attempt < max_retries - 1:
                wait_time = e.retry_after or (2 ** attempt)
                print(f"Rate limited, waiting {wait_time}s before retry...")
                time.sleep(wait_time)
            else:
                print(f"Rate limited after {max_retries} attempts: {e}")
                
        except ogem.APIError as e:
            print(f"API error: {e}")
            break
    
    # Example 3: Validation error
    try:
        client.chat.completions.create(
            model="",  # Empty model
            messages=[]  # Empty messages
        )
    except ogem.ValidationError as e:
        print(f"Validation error: {e}")
        if hasattr(e, 'field_errors'):
            print(f"Field errors: {e.field_errors}")
    
    print()


async def async_example():
    """Async client example."""
    print("9. Async Usage")
    print("-" * 40)
    
    async with ogem.AsyncClient(
        base_url="http://localhost:8080",
        api_key="your-api-key"
    ) as client:
        try:
            # Multiple concurrent requests
            tasks = []
            
            for i in range(3):
                task = client.chat.completions.create(
                    model=Models.GPT_3_5_TURBO,
                    messages=[create_user_message(f"Count to {i+3}")],
                    max_tokens=20
                )
                tasks.append(task)
            
            # Wait for all requests to complete
            responses = await asyncio.gather(*tasks, return_exceptions=True)
            
            for i, response in enumerate(responses):
                if isinstance(response, Exception):
                    print(f"Request {i+1} failed: {response}")
                else:
                    print(f"Request {i+1}: {response.choices[0].message.content}")
            
            # Health check
            health = await client.health()
            print(f"\nAsync health check: {health['status']}")
            
        except ogem.APIError as e:
            print(f"Async API error: {e}")
    
    print()


def advanced_usage_example():
    """Advanced usage patterns."""
    print("Advanced Usage Patterns")
    print("-" * 40)
    
    client = ogem.Client(
        base_url="http://localhost:8080",
        api_key="your-api-key",
        timeout=60.0,
        debug=True
    )
    
    # Using the fluent request builder
    try:
        from ogem.types import ChatCompletionRequest
        
        request = ChatCompletionRequest(
            model=Models.GPT_4,
            messages=[
                create_system_message("You are an expert programmer."),
                create_user_message("Explain async/await in Python")
            ]
        ).max_tokens(500).temperature(0.3).top_p(0.9).seed(42)
        
        response = client.chat.completions.create(**request.build())
        print(f"Response: {response.choices[0].message.content[:100]}...")
        
    except ogem.APIError as e:
        print(f"API error: {e}")
    
    # Context manager usage
    try:
        with ogem.Client(
            base_url="http://localhost:8080",
            api_key="your-api-key"
        ) as temp_client:
            response = temp_client.chat.completions.create(
                model=Models.GPT_3_5_TURBO,
                messages=[create_user_message("What is 2+2?")],
                max_tokens=20
            )
            print(f"Quick calculation: {response.choices[0].message.content}")
    except ogem.APIError as e:
        print(f"Context manager error: {e}")
    
    print()


if __name__ == "__main__":
    # Set up environment (you can also use environment variables)
    # os.environ["OGEM_BASE_URL"] = "http://localhost:8080"
    # os.environ["OGEM_API_KEY"] = "your-api-key"
    
    main()