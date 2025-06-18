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
	fmt.Println("=== Local Provider Manager Example ===")

	// Create configuration for all local providers
	config := &local.LocalConfig{
		AutoDiscovery:       true,
		HealthCheckInterval: 30 * time.Second,
		DefaultTimeout:      30 * time.Second,
		
		// Ollama configuration
		Ollama: &local.OllamaConfig{
			BaseURL: "http://localhost:11434",
			Timeout: 30 * time.Second,
		},
		
		// vLLM configuration
		VLLM: &local.VLLMConfig{
			BaseURL: "http://localhost:8000",
			Timeout: 60 * time.Second,
		},
		
		// LM Studio configuration
		LMStudio: &local.LMStudioConfig{
			BaseURL: "http://localhost:1234",
			Timeout: 60 * time.Second,
		},
	}

	// Create local provider manager
	manager := local.NewLocalProviderManager(config)

	ctx := context.Background()

	// Get overall status
	fmt.Println("\n=== Local Provider Status ===")
	status := manager.GetStatus(ctx)
	fmt.Printf("Total providers: %d\n", status.TotalProviders)
	fmt.Printf("Healthy providers: %d\n", status.HealthyProviders)

	for name, providerStatus := range status.Providers {
		fmt.Printf("\n%s:\n", name)
		fmt.Printf("  Healthy: %t\n", providerStatus.Healthy)
		if providerStatus.Error != "" {
			fmt.Printf("  Error: %s\n", providerStatus.Error)
		}
		fmt.Printf("  Models: %d\n", providerStatus.ModelCount)
	}

	// List all available providers
	fmt.Println("\n=== Available Providers ===")
	providers := manager.GetProviders()
	for name, provider := range providers {
		fmt.Printf("- %s (%s)\n", name, provider.GetName())
	}

	if len(providers) == 0 {
		fmt.Println("No local providers available. Make sure Ollama, vLLM, or LM Studio are running.")
		return
	}

	// Get all models from all providers
	fmt.Println("\n=== All Available Models ===")
	allModels, err := manager.GetAllModels(ctx)
	if err != nil {
		log.Printf("Failed to get all models: %v", err)
	} else {
		for providerName, models := range allModels {
			fmt.Printf("\n%s models:\n", providerName)
			for _, model := range models {
				fmt.Printf("  - %s\n", model.ID)
			}
		}
	}

	// Find a model to use for examples
	var testModel string
	var testProvider string
	
	for providerName, models := range allModels {
		if len(models) > 0 {
			testModel = models[0].ID
			testProvider = providerName
			break
		}
	}

	if testModel == "" {
		fmt.Println("No models available for testing")
		return
	}

	fmt.Printf("\nUsing model: %s from provider: %s\n", testModel, testProvider)

	// Example 1: Get provider for specific model
	fmt.Println("\n=== Provider Selection ===")
	provider, err := manager.GetProviderForModel(ctx, testModel)
	if err != nil {
		log.Printf("Failed to get provider for model: %v", err)
	} else {
		fmt.Printf("Provider for model '%s': %s\n", testModel, provider.GetName())
	}

	// Example 2: Test chat completion through manager
	fmt.Println("\n=== Chat Completion via Manager ===")
	if provider != nil {
		chatReq := &types.ChatCompletionRequest{
			Model: testModel,
			Messages: []types.Message{
				{
					Role:    "user",
					Content: "Hello! Can you tell me about yourself?",
				},
			},
			MaxTokens:   &[]int{100}[0],
			Temperature: &[]float64{0.7}[0],
		}

		response, err := provider.ChatCompletion(ctx, chatReq)
		if err != nil {
			log.Printf("Chat completion failed: %v", err)
		} else {
			fmt.Printf("Response: %s\n", response.Choices[0].Message.Content)
		}
	}

	// Example 3: Health monitoring
	fmt.Println("\n=== Health Monitoring ===")
	healthResults := manager.HealthCheck(ctx)
	
	fmt.Println("Provider health status:")
	for name, err := range healthResults {
		if err == nil {
			fmt.Printf("  ✓ %s: healthy\n", name)
		} else {
			fmt.Printf("  ✗ %s: %v\n", name, err)
		}
	}

	// Example 4: Provider comparison
	fmt.Println("\n=== Provider Comparison ===")
	testPrompt := "What is 2 + 2?"
	
	for providerName, providerModels := range allModels {
		if len(providerModels) == 0 {
			continue
		}

		provider, err := manager.GetProvider(providerName)
		if err != nil {
			continue
		}

		fmt.Printf("\nTesting %s with model %s:\n", providerName, providerModels[0].ID)
		
		start := time.Now()
		chatReq := &types.ChatCompletionRequest{
			Model: providerModels[0].ID,
			Messages: []types.Message{
				{
					Role:    "user",
					Content: testPrompt,
				},
			},
			MaxTokens:   &[]int{50}[0],
			Temperature: &[]float64{0.1}[0],
		}

		response, err := provider.ChatCompletion(ctx, chatReq)
		duration := time.Since(start)

		if err != nil {
			fmt.Printf("  Error: %v\n", err)
		} else {
			fmt.Printf("  Response: %s\n", response.Choices[0].Message.Content)
			fmt.Printf("  Duration: %v\n", duration)
			if response.Usage != nil {
				fmt.Printf("  Tokens: %d\n", response.Usage.TotalTokens)
			}
		}
	}

	// Example 5: Start health monitoring (would run in background)
	fmt.Println("\n=== Starting Health Monitoring ===")
	fmt.Println("Health monitoring would run in background...")
	
	// In a real application, you would start this in a goroutine:
	// go manager.StartHealthMonitoring(context.Background())

	// Example 6: Dynamic provider management
	fmt.Println("\n=== Dynamic Provider Management ===")
	initialCount := len(manager.GetProviders())
	fmt.Printf("Initial provider count: %d\n", initialCount)

	// You could add custom providers here:
	// customProvider := &MyCustomProvider{}
	// manager.AddProvider("custom", customProvider)

	fmt.Println("\n=== Local Provider Manager Example Complete ===")
}