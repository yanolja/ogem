package main

import (
	"context"
	"fmt"
	"log"
	"time"

	ogem "github.com/yanolja/ogem/sdk/go"
)

func main() {
	// Example 1: Basic Chat Completion
	basicChatExample()

	// Example 2: Multi-turn Conversation
	conversationExample()

	// Example 3: Function Calling
	functionCallingExample()

	// Example 4: Embeddings
	embeddingsExample()

	// Example 5: Multi-tenant Usage
	multiTenantExample()

	// Example 6: Health Check and Stats
	monitoringExample()
}

func basicChatExample() {
	fmt.Println("=== Basic Chat Completion Example ===")

	// Create client
	client, err := ogem.NewClient(ogem.Config{
		BaseURL: "http://localhost:8080",
		APIKey:  "your-api-key",
		Debug:   true,
	})
	if err != nil {
		log.Fatal("Failed to create client:", err)
	}

	// Create a simple chat request
	request := ogem.NewChatCompletionRequest(
		"gpt-3.5-turbo",
		[]ogem.Message{
			ogem.NewSystemMessage("You are a helpful assistant."),
			ogem.NewUserMessage("What is the capital of France?"),
		},
	).WithMaxTokens(100).WithTemperature(0.7)

	// Make the request
	ctx := context.Background()
	response, err := client.ChatCompletion(ctx, request)
	if err != nil {
		log.Printf("Chat completion failed: %v", err)
		return
	}

	// Print the response
	if len(response.Choices) > 0 {
		fmt.Printf("Response: %s\n", response.Choices[0].Message.Content)
		fmt.Printf("Tokens used: %d\n", response.Usage.TotalTokens)
	}

	fmt.Println()
}

func conversationExample() {
	fmt.Println("=== Multi-turn Conversation Example ===")

	client, err := ogem.NewClient(ogem.Config{
		BaseURL: "http://localhost:8080",
		APIKey:  "your-api-key",
	})
	if err != nil {
		log.Fatal("Failed to create client:", err)
	}

	// Start with system message
	messages := []ogem.Message{
		ogem.NewSystemMessage("You are a friendly coding assistant."),
	}

	// Simulate a conversation
	userQueries := []string{
		"How do I create a HTTP server in Go?",
		"Can you show me an example with error handling?",
		"What about adding middleware?",
	}

	ctx := context.Background()

	for i, query := range userQueries {
		fmt.Printf("Turn %d:\n", i+1)
		fmt.Printf("User: %s\n", query)

		// Add user message
		messages = append(messages, ogem.NewUserMessage(query))

		// Create request
		request := ogem.NewChatCompletionRequest("gpt-4", messages).
			WithMaxTokens(500).
			WithTemperature(0.3)

		// Get response
		response, err := client.ChatCompletion(ctx, request)
		if err != nil {
			log.Printf("Chat completion failed: %v", err)
			continue
		}

		if len(response.Choices) > 0 {
			assistantMessage := response.Choices[0].Message.Content.(string)
			fmt.Printf("Assistant: %s\n\n", assistantMessage)

			// Add assistant response to conversation history
			messages = append(messages, ogem.NewAssistantMessage(assistantMessage))
		}
	}
}

func functionCallingExample() {
	fmt.Println("=== Function Calling Example ===")

	client, err := ogem.NewClient(ogem.Config{
		BaseURL: "http://localhost:8080",
		APIKey:  "your-api-key",
	})
	if err != nil {
		log.Fatal("Failed to create client:", err)
	}

	// Define a weather function
	weatherFunction := ogem.Function{
		Name:        "get_weather",
		Description: "Get the current weather for a location",
		Parameters: map[string]interface{}{
			"type": "object",
			"properties": map[string]interface{}{
				"location": map[string]interface{}{
					"type":        "string",
					"description": "The city and state, e.g. San Francisco, CA",
				},
				"unit": map[string]interface{}{
					"type":        "string",
					"enum":        []string{"celsius", "fahrenheit"},
					"description": "The temperature unit",
				},
			},
			"required": []string{"location"},
		},
	}

	// Create request with function
	request := ogem.NewChatCompletionRequest(
		"gpt-4",
		[]ogem.Message{
			ogem.NewUserMessage("What's the weather like in New York?"),
		},
	).WithTools([]ogem.Tool{
		{Type: "function", Function: weatherFunction},
	})

	ctx := context.Background()
	response, err := client.ChatCompletion(ctx, request)
	if err != nil {
		log.Printf("Function calling failed: %v", err)
		return
	}

	if len(response.Choices) > 0 {
		choice := response.Choices[0]
		if len(choice.Message.ToolCalls) > 0 {
			toolCall := choice.Message.ToolCalls[0]
			fmt.Printf("Function called: %s\n", toolCall.Function.Name)
			fmt.Printf("Arguments: %s\n", toolCall.Function.Arguments)
		} else {
			fmt.Printf("Response: %s\n", choice.Message.Content)
		}
	}

	fmt.Println()
}

func embeddingsExample() {
	fmt.Println("=== Embeddings Example ===")

	client, err := ogem.NewClient(ogem.Config{
		BaseURL: "http://localhost:8080",
		APIKey:  "your-api-key",
	})
	if err != nil {
		log.Fatal("Failed to create client:", err)
	}

	// Create embeddings request
	request := ogem.NewEmbeddingsRequest(
		"text-embedding-3-small",
		[]string{
			"The quick brown fox jumps over the lazy dog",
			"Machine learning is a subset of artificial intelligence",
			"Go is a programming language developed by Google",
		},
	)

	ctx := context.Background()
	response, err := client.Embeddings(ctx, request)
	if err != nil {
		log.Printf("Embeddings failed: %v", err)
		return
	}

	fmt.Printf("Generated %d embeddings\n", len(response.Data))
	for i, embedding := range response.Data {
		fmt.Printf("Embedding %d: %d dimensions (first 5: %.4f, %.4f, %.4f, %.4f, %.4f...)\n",
			i, len(embedding.Embedding),
			embedding.Embedding[0], embedding.Embedding[1], embedding.Embedding[2],
			embedding.Embedding[3], embedding.Embedding[4])
	}
	fmt.Printf("Total tokens used: %d\n", response.Usage.TotalTokens)

	fmt.Println()
}

func multiTenantExample() {
	fmt.Println("=== Multi-tenant Usage Example ===")

	// Create client for tenant A
	clientA, err := ogem.NewClient(ogem.Config{
		BaseURL:  "http://localhost:8080",
		APIKey:   "your-api-key",
		TenantID: "tenant-a",
	})
	if err != nil {
		log.Fatal("Failed to create client A:", err)
	}

	// Create client for tenant B
	clientB, err := ogem.NewClient(ogem.Config{
		BaseURL:  "http://localhost:8080",
		APIKey:   "your-api-key",
		TenantID: "tenant-b",
	})
	if err != nil {
		log.Fatal("Failed to create client B:", err)
	}

	ctx := context.Background()

	// Make requests from different tenants
	requestA := ogem.NewChatCompletionRequest(
		"gpt-3.5-turbo",
		[]ogem.Message{ogem.NewUserMessage("Hello from tenant A")},
	)

	requestB := ogem.NewChatCompletionRequest(
		"gpt-3.5-turbo",
		[]ogem.Message{ogem.NewUserMessage("Hello from tenant B")},
	)

	// Execute requests
	responseA, err := clientA.ChatCompletion(ctx, requestA)
	if err != nil {
		log.Printf("Tenant A request failed: %v", err)
	} else {
		fmt.Printf("Tenant A response: %s\n", responseA.Choices[0].Message.Content)
	}

	responseB, err := clientB.ChatCompletion(ctx, requestB)
	if err != nil {
		log.Printf("Tenant B request failed: %v", err)
	} else {
		fmt.Printf("Tenant B response: %s\n", responseB.Choices[0].Message.Content)
	}

	// Get tenant usage stats
	usageA, err := clientA.TenantUsage(ctx, "tenant-a")
	if err != nil {
		log.Printf("Failed to get tenant A usage: %v", err)
	} else {
		fmt.Printf("Tenant A usage - Requests today: %d, Cost today: $%.4f\n",
			usageA.RequestsThisDay, usageA.CostThisDay)
	}

	usageB, err := clientB.TenantUsage(ctx, "tenant-b")
	if err != nil {
		log.Printf("Failed to get tenant B usage: %v", err)
	} else {
		fmt.Printf("Tenant B usage - Requests today: %d, Cost today: $%.4f\n",
			usageB.RequestsThisDay, usageB.CostThisDay)
	}

	fmt.Println()
}

func monitoringExample() {
	fmt.Println("=== Health Check and Stats Example ===")

	client, err := ogem.NewClient(ogem.Config{
		BaseURL: "http://localhost:8080",
		APIKey:  "your-api-key",
	})
	if err != nil {
		log.Fatal("Failed to create client:", err)
	}

	ctx := context.Background()

	// Health check
	health, err := client.Health(ctx)
	if err != nil {
		log.Printf("Health check failed: %v", err)
	} else {
		fmt.Printf("Server status: %s\n", health.Status)
		fmt.Printf("Version: %s\n", health.Version)
		fmt.Printf("Uptime: %s\n", health.Uptime)
	}

	// Server stats
	stats, err := client.Stats(ctx)
	if err != nil {
		log.Printf("Stats request failed: %v", err)
	} else {
		fmt.Printf("Total requests: %d\n", stats.Requests.Total)
		fmt.Printf("Success rate: %.2f%%\n", stats.Requests.SuccessRate*100)
		fmt.Printf("Average latency: %s\n", stats.Performance.AverageLatency)
		fmt.Printf("Throughput: %.2f RPM\n", stats.Performance.ThroughputRPM)
	}

	// Cache stats
	cacheStats, err := client.CacheStats(ctx)
	if err != nil {
		log.Printf("Cache stats failed: %v", err)
	} else {
		fmt.Printf("Cache hit rate: %.2f%%\n", cacheStats.HitRate*100)
		fmt.Printf("Total cache entries: %d\n", cacheStats.TotalEntries)
		fmt.Printf("Cache memory usage: %.2f MB\n", cacheStats.MemoryUsageMB)
	}

	// List available models
	models, err := client.Models(ctx)
	if err != nil {
		log.Printf("Models request failed: %v", err)
	} else {
		fmt.Printf("Available models: %d\n", len(models.Data))
		for _, model := range models.Data[:5] { // Show first 5
			fmt.Printf("  - %s (owned by %s)\n", model.ID, model.OwnedBy)
		}
	}

	fmt.Println()
}

// Advanced example with error handling and retries
func advancedExample() {
	fmt.Println("=== Advanced Usage with Error Handling ===")

	client, err := ogem.NewClient(ogem.Config{
		BaseURL: "http://localhost:8080",
		APIKey:  "your-api-key",
		Timeout: 30 * time.Second,
		Debug:   true,
	})
	if err != nil {
		log.Fatal("Failed to create client:", err)
	}

	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()

	request := ogem.NewChatCompletionRequest(
		"gpt-4",
		[]ogem.Message{
			ogem.NewSystemMessage("You are a helpful assistant."),
			ogem.NewUserMessage("Explain quantum computing in simple terms."),
		},
	).WithMaxTokens(500).WithTemperature(0.3)

	// Retry logic
	maxRetries := 3
	for attempt := 1; attempt <= maxRetries; attempt++ {
		response, err := client.ChatCompletion(ctx, request)
		if err != nil {
			if apiErr, ok := err.(*ogem.APIError); ok {
				switch apiErr.StatusCode {
				case 429: // Rate limit
					fmt.Printf("Rate limited, retrying in %d seconds...\n", attempt*2)
					time.Sleep(time.Duration(attempt*2) * time.Second)
					continue
				case 500, 502, 503, 504: // Server errors
					if attempt < maxRetries {
						fmt.Printf("Server error, retrying... (attempt %d/%d)\n", attempt, maxRetries)
						time.Sleep(time.Duration(attempt) * time.Second)
						continue
					}
				default:
					log.Printf("API error: %v", apiErr)
					return
				}
			} else {
				log.Printf("Request failed: %v", err)
				return
			}
		} else {
			// Success
			if len(response.Choices) > 0 {
				fmt.Printf("Response: %s\n", response.Choices[0].Message.Content)
				fmt.Printf("Model: %s\n", response.Model)
				fmt.Printf("Tokens: %d\n", response.Usage.TotalTokens)
			}
			break
		}
	}

	fmt.Println()
}
