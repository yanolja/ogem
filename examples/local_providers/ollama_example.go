package main

import (
	"context"
	"fmt"
	"log"
	"time"

	"github.com/yanolja/ogem/providers/local"
	"github.com/yanolja/ogem/types"
)

func main() {
	// Create Ollama provider configuration
	config := &local.OllamaConfig{
		BaseURL:    "http://localhost:11434",
		Timeout:    30 * time.Second,
		MaxRetries: 3,
		Models: []string{
			"llama2",
			"codellama",
			"mistral",
		},
	}

	// Create Ollama provider
	provider := local.NewOllamaProvider(config)

	ctx := context.Background()

	// Check health
	fmt.Println("=== Ollama Health Check ===")
	if err := provider.Health(ctx); err != nil {
		log.Fatalf("Ollama health check failed: %v", err)
	}
	fmt.Println("✓ Ollama is healthy")

	// Get available models
	fmt.Println("\n=== Available Models ===")
	models, err := provider.GetModels(ctx)
	if err != nil {
		log.Fatalf("Failed to get models: %v", err)
	}

	for _, model := range models {
		fmt.Printf("- %s (owned by %s)\n", model.ID, model.OwnedBy)
	}

	if len(models) == 0 {
		fmt.Println("No models found. Please install models using 'ollama pull <model>'")
		return
	}

	// Use the first available model for examples
	modelID := models[0].ID
	fmt.Printf("\nUsing model: %s\n", modelID)

	// Example 1: Basic chat completion
	fmt.Println("\n=== Basic Chat Completion ===")
	chatReq := &types.ChatCompletionRequest{
		Model: modelID,
		Messages: []types.Message{
			{
				Role:    "system",
				Content: "You are a helpful assistant.",
			},
			{
				Role:    "user",
				Content: "What is the capital of France?",
			},
		},
		MaxTokens:   &[]int{100}[0],
		Temperature: &[]float64{0.7}[0],
	}

	response, err := provider.ChatCompletion(ctx, chatReq)
	if err != nil {
		log.Fatalf("Chat completion failed: %v", err)
	}

	fmt.Printf("Response: %s\n", response.Choices[0].Message.Content)
	fmt.Printf("Tokens used: %d\n", response.Usage.TotalTokens)

	// Example 2: Streaming chat completion
	fmt.Println("\n=== Streaming Chat Completion ===")
	streamReq := &types.ChatCompletionRequest{
		Model: modelID,
		Messages: []types.Message{
			{
				Role:    "user",
				Content: "Write a short poem about coding.",
			},
		},
		Stream:      true,
		MaxTokens:   &[]int{200}[0],
		Temperature: &[]float64{0.8}[0],
	}

	streamChan, err := provider.ChatCompletionStream(ctx, streamReq)
	if err != nil {
		log.Fatalf("Streaming failed: %v", err)
	}

	fmt.Print("Assistant: ")
	fullResponse := ""
	
	for streamResp := range streamChan {
		if streamResp.Error != nil {
			log.Fatalf("Stream error: %v", streamResp.Error)
		}

		if len(streamResp.Response.Choices) > 0 && streamResp.Response.Choices[0].Delta != nil {
			content := streamResp.Response.Choices[0].Delta.Content
			fmt.Print(content)
			fullResponse += content
		}
	}

	fmt.Printf("\n\nFull response length: %d characters\n", len(fullResponse))

	// Example 3: Embeddings (if supported)
	fmt.Println("\n=== Embeddings ===")
	embeddingReq := &types.EmbeddingRequest{
		Model: modelID,
		Input: []string{
			"Hello, world!",
			"How are you today?",
		},
	}

	embeddingResp, err := provider.CreateEmbedding(ctx, embeddingReq)
	if err != nil {
		fmt.Printf("Embeddings failed (may not be supported): %v\n", err)
	} else {
		fmt.Printf("Generated %d embeddings\n", len(embeddingResp.Data))
		for i, embedding := range embeddingResp.Data {
			fmt.Printf("Embedding %d: %d dimensions\n", i+1, len(embedding.Embedding))
		}
	}

	// Example 4: Multiple conversation turns
	fmt.Println("\n=== Multi-turn Conversation ===")
	messages := []types.Message{
		{
			Role:    "system",
			Content: "You are a helpful coding assistant. Keep responses concise.",
		},
	}

	questions := []string{
		"How do I create a slice in Go?",
		"How do I append to it?",
		"What about removing elements?",
	}

	for i, question := range questions {
		fmt.Printf("\nQ%d: %s\n", i+1, question)
		
		// Add user message
		messages = append(messages, types.Message{
			Role:    "user",
			Content: question,
		})

		// Get response
		convReq := &types.ChatCompletionRequest{
			Model:       modelID,
			Messages:    messages,
			MaxTokens:   &[]int{150}[0],
			Temperature: &[]float64{0.3}[0],
		}

		convResp, err := provider.ChatCompletion(ctx, convReq)
		if err != nil {
			log.Printf("Conversation failed: %v", err)
			continue
		}

		assistantMessage := convResp.Choices[0].Message.Content
		fmt.Printf("A%d: %s\n", i+1, assistantMessage)

		// Add assistant response to conversation
		messages = append(messages, types.Message{
			Role:    "assistant",
			Content: assistantMessage,
		})
	}

	fmt.Println("\n=== Ollama Example Complete ===")
}