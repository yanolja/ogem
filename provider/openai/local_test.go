package openai

import (
	"context"
	"fmt"
	"os"
	"strings"
	"testing"
	"time"

	"flag"

	"github.com/joho/godotenv"
	ogem "github.com/yanolja/ogem/sdk/go"
)

// Dynamic Provider/Region Model Testing
// Usage:
//   go test -v -run TestChatCompletion_CombinedContext ./provider/openai/ -provider_region=openai/openai
//
// - The -provider_region flag should be in the format <provider>/<region>, matching the structure in config.yaml.
// - The test will automatically load all model names for that provider/region and run the tests against them.
// - If the flag is omitted, a default provider/region will be used.

var providerRegionFlag = flag.String("provider_region", "openai/openai", "Provider and region in the format <provider>/<region> (e.g., openai/openai)")

// Helper to get API key and base URL from env
func getTestClient(t *testing.T) *ogem.Client {
	if err := godotenv.Load(); err != nil {
		fmt.Println("Failed to load env file:", err)
	}
	apiKey := os.Getenv("OGEM_API_KEY")
	baseURL := os.Getenv("OGEM_BASE_URL")

	if apiKey == "" || baseURL == "" {
		t.Fatal("OGEM_API_KEY and OGEM_BASE_URL must be set for integration tests")
	}
	client, err := ogem.NewClient(ogem.Config{
		BaseURL: baseURL,
		APIKey:  apiKey,
	})
	if err != nil {
		t.Fatalf("Failed to create client: %v", err)
	}
	return client
}

var gptModels = []string{
	ogem.ModelGPT4o,
	ogem.ModelGPT4oMini,
	ogem.ModelGPT41,
	ogem.ModelGPT41Mini,
	ogem.ModelGPT41Nano,
	ogem.ModelO4Mini,
	ogem.ModelO3,
	ogem.ModelO3Mini,
	ogem.ModelO1,
	ogem.ModelO1Mini,
	ogem.ModelGPT4,
	ogem.ModelGPT35Turbo,
	ogem.ModelGPT4Turbo,
	ogem.ModelGPT4TurboPreview,
}

var geminiModels = []string{
	ogem.ModelGemini25Pro,
	ogem.ModelGemini25Flash,
	ogem.ModelGemini25FlashLite,
}

var claudeModels = []string{
	ogem.ModelClaudeOpus4,
	ogem.ModelClaudeSonnet4,
	ogem.ModelClaude37Sonnet,
	ogem.ModelClaude35Sonnet,
	ogem.ModelClaude35Haiku,
	ogem.ModelClaude3Haiku,
}

// Models that do NOT support function calling (legacy functions parameter)
var functionNotSupportedModels = []string{
	ogem.ModelO1Mini,
	ogem.ModelO1,
	ogem.ModelO3Mini,
	ogem.ModelO3,
	ogem.ModelO4Mini,
}

// Models that do NOT support tool calling (new tools parameter)
var toolNotSupportedModels = []string{
	ogem.ModelO1Mini,
}

var modelRegistry = map[string]map[string][]string{
	"openai": {
		"openai": gptModels,
	},
	"claude": {
		"claude": claudeModels,
	},
	"gemini": {
		"gemini": geminiModels,
	},
}

func getTestModelsFromConfig(t *testing.T) []string {
	flag.Parse()
	providerRegion := *providerRegionFlag
	if providerRegion == "" {
		providerRegion = "openai/openai"
	}

	parts := strings.SplitN(providerRegion, "/", 2)
	if len(parts) != 2 {
		t.Fatalf("Invalid provider_region format: %s. Expected <provider>/<region>", providerRegion)
	}
	provider, region := parts[0], parts[1]

	regions, ok := modelRegistry[provider]
	if !ok {
		t.Fatalf("Unknown provider: %s. Supported: %v", provider, keys(modelRegistry))
	}
	models, ok := regions[region]
	if !ok {
		t.Fatalf("Unknown region: %s for provider %s. Supported: %v", region, provider, keys(regions))
	}
	if len(models) == 0 {
		t.Fatalf("No models defined for provider %s region %s", provider, region)
	}
	return models
}

// Helper to get map keys as []string for error messages
func keys(m interface{}) []string {
	switch mm := m.(type) {
	case map[string][]string:
		var out []string
		for k := range mm {
			out = append(out, k)
		}
		return out
	case map[string]map[string][]string:
		var out []string
		for k := range mm {
			out = append(out, k)
		}
		return out
	default:
		return nil
	}
}

// Helper function to check if a model does NOT support functions
func doesNotSupportFunctions(model string) bool {
	for _, notSupportedModel := range functionNotSupportedModels {
		if notSupportedModel == model {
			return true
		}
	}
	return false
}

// Helper function to check if a model does NOT support tools
func doesNotSupportTools(model string) bool {
	for _, notSupportedModel := range toolNotSupportedModels {
		if notSupportedModel == model {
			return true
		}
	}
	return false
}

func TestChatCompletion_UserRole(t *testing.T) {
	testModels := getTestModelsFromConfig(t)
	client := getTestClient(t)

	// Set timeout for the entire test function
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Minute)
	defer cancel()

	t.Logf("Starting UserRole test with %d models", len(testModels))

	for i, model := range testModels {
		t.Logf("Testing model %d/%d: %s", i+1, len(testModels), model)

		func() {
			// Create a subtest that runs sequentially
			t.Run(model, func(t *testing.T) {
				// Use the parent context with timeout
				req := ogem.NewChatCompletionRequest(model, []ogem.Message{
					ogem.NewUserMessage("Tell me about the brightest star in our night sky in less than 100 words."),
				})
				resp, err := client.ChatCompletion(ctx, req)
				if err != nil {
					t.Fatalf("API error: %v", err)
				}
				if len(resp.Choices) == 0 {
					t.Fatal("No choices returned")
				}

				t.Logf("✓ %s completed successfully", model)
			})
		}()

		// Small delay between tests to avoid overwhelming the API
		time.Sleep(1 * time.Second)
	}

	t.Logf("UserRole test completed for all %d models", len(testModels))
}

func TestChatCompletion_AssistantRole(t *testing.T) {
	testModels := getTestModelsFromConfig(t)
	client := getTestClient(t)

	// Set timeout for the entire test function
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Minute)
	defer cancel()

	t.Logf("Starting AssistantRole test with %d models", len(testModels))

	for i, model := range testModels {
		t.Logf("Testing model %d/%d: %s", i+1, len(testModels), model)

		func() {
			// Create a subtest that runs sequentially
			t.Run(model, func(t *testing.T) {
				// Use the parent context with timeout
				req := ogem.NewChatCompletionRequest(model, []ogem.Message{
					ogem.NewAssistantMessage("Sirius is the brightest star visible from Earth, located in the constellation Canis Major."),
					ogem.NewUserMessage("What makes Sirius so bright? Explain in less than 100 words."),
				})
				resp, err := client.ChatCompletion(ctx, req)
				if err != nil {
					t.Fatalf("API error: %v", err)
				}
				if len(resp.Choices) == 0 {
					t.Fatal("No choices returned")
				}
				t.Logf("✓ %s completed successfully", model)
			})
		}()

		// Small delay between tests to avoid overwhelming the API
		time.Sleep(1 * time.Second)
	}

	t.Logf("AssistantRole test completed for all %d models", len(testModels))
}

func TestChatCompletion_FunctionRole(t *testing.T) {
	testModels := getTestModelsFromConfig(t)
	client := getTestClient(t)

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Minute)
	defer cancel()

	t.Logf("Starting FunctionRole test with %d models", len(testModels))

	for i, model := range testModels {
		t.Logf("Testing model %d/%d: %s", i+1, len(testModels), model)

		func() {
			t.Run(model, func(t *testing.T) {
				if doesNotSupportFunctions(model) {
					t.Skipf("Model %s does not support function calling", model)
					return
				}

				function := ogem.Function{
					Name:        "get_star_distance",
					Description: "Returns the distance of a star from Earth in light years.",
					Parameters: map[string]interface{}{
						"type": "object",
						"properties": map[string]interface{}{
							"star_name": map[string]interface{}{
								"type":        "string",
								"description": "Name of the star",
							},
						},
						"required": []string{"star_name"},
					},
				}
				messages := []ogem.Message{
					{
						Role:    ogem.RoleUser,
						Content: "How far is Sirius from Earth?",
					},
					{
						Role:    ogem.RoleFunction,
						Name:    "get_star_distance",
						Content: `{"star_name": "Sirius", "distance_ly": 8.6}`,
					},
				}
				req := ogem.NewChatCompletionRequest(model, messages)
				req.Functions = []ogem.Function{function}
				resp, err := client.ChatCompletion(ctx, req)
				if err != nil {
					t.Fatalf("API error: %v", err)
				}
				if len(resp.Choices) == 0 {
					t.Fatal("No choices returned")
				}
				msg := resp.Choices[0].Message
				if msg.FunctionCall == nil && msg.Role != ogem.RoleAssistant {
					t.Errorf("Expected function_call or assistant role, got: %+v", msg)
				}
				t.Logf("✓ %s completed successfully", model)
			})
		}()

		time.Sleep(1 * time.Second)
	}

	t.Logf("FunctionRole test completed for all %d models", len(testModels))
}

func TestChatCompletion_ToolRole(t *testing.T) {
	testModels := getTestModelsFromConfig(t)
	client := getTestClient(t)

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Minute)
	defer cancel()

	t.Logf("Starting ToolRole test with %d models", len(testModels))

	for i, model := range testModels {
		t.Logf("Testing model %d/%d: %s", i+1, len(testModels), model)

		func() {
			t.Run(model, func(t *testing.T) {
				if doesNotSupportTools(model) {
					t.Skipf("Model %s does not support tool calling", model)
					return
				}

				tool := ogem.Tool{
					Type: "function",
					Function: ogem.Function{
						Name:        "get_planet_info",
						Description: "Returns information about a planet in our solar system.",
						Parameters: map[string]interface{}{
							"type": "object",
							"properties": map[string]interface{}{
								"planet_name": map[string]interface{}{
									"type":        "string",
									"description": "Name of the planet",
								},
							},
							"required": []string{"planet_name"},
						},
					},
				}
				messages := []ogem.Message{
					{
						Role:    ogem.RoleUser,
						Content: "Tell me about Mars.",
					},
					{
						Role:       ogem.RoleTool,
						ToolCallID: "call_get_planet_info_123",
						Content:    `{"planet_name": "Mars", "info": "Mars is the fourth planet from the Sun."}`,
					},
				}
				req := ogem.NewChatCompletionRequest(model, messages)
				req.Tools = []ogem.Tool{tool}
				resp, err := client.ChatCompletion(ctx, req)
				if err != nil {
					t.Fatalf("API error: %v", err)
				}
				if len(resp.Choices) == 0 {
					t.Fatal("No choices returned")
				}
				msg := resp.Choices[0].Message
				if len(msg.ToolCalls) == 0 && msg.Role != ogem.RoleAssistant {
					t.Errorf("Expected tool_calls or assistant role, got: %+v", msg)
				}
				t.Logf("✓ %s completed successfully", model)
			})
		}()

		time.Sleep(1 * time.Second)
	}

	t.Logf("ToolRole test completed for all %d models", len(testModels))
}

func TestChatCompletion_CombinedContext(t *testing.T) {
	testModels := getTestModelsFromConfig(t)
	client := getTestClient(t)

	// Set timeout for the entire test function
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Minute)
	defer cancel()

	t.Logf("Starting ToolsAndFunctionsSequential test with %d models", len(testModels))

	for i, model := range testModels {
		t.Logf("Testing model %d/%d: %s", i+1, len(testModels), model)

		func() {
			// Create a subtest that runs sequentially
			t.Run(model, func(t *testing.T) {
				// Use the parent context with timeout

				// Test 1: Tools-based conversation
				t.Logf("Testing %s with tools...", model)
				if !doesNotSupportTools(model) {
					// Define tools for the test
					tools := []ogem.Tool{
						{
							Type: "function",
							Function: ogem.Function{
								Name:        "calculate_factorial",
								Description: "Calculate the factorial of a positive integer",
								Parameters: map[string]interface{}{
									"type": "object",
									"properties": map[string]interface{}{
										"number": map[string]interface{}{
											"type":        "integer",
											"description": "The positive integer to calculate factorial for",
										},
									},
									"required": []string{"number"},
								},
							},
						},
					}

					// Build conversation messages for tools
					toolMsgs := []ogem.Message{
						ogem.NewUserMessage("Calculate the factorial of 7"),
					}

					// Add assistant message with tool call
					toolMsgs = append(toolMsgs, ogem.Message{
						Role:    ogem.RoleAssistant,
						Content: nil,
						ToolCalls: []ogem.ToolCall{
							{
								ID:   "call_factorial_123",
								Type: "function",
								Function: ogem.FunctionCall{
									Name:      "calculate_factorial",
									Arguments: `{"number": 7}`,
								},
							},
						},
					})

					// Add tool response
					toolMsgs = append(toolMsgs, ogem.Message{
						Role:       ogem.RoleTool,
						ToolCallID: "call_factorial_123",
						Content:    `{"result": 5040, "calculation": "7! = 7 × 6 × 5 × 4 × 3 × 2 × 1"}`,
					})

					// Add final user question
					toolMsgs = append(toolMsgs, ogem.NewUserMessage("Now calculate the factorial of 10"))

					// Create request with tools
					toolReq := ogem.NewChatCompletionRequest(model, toolMsgs)
					toolReq.Tools = tools
					toolReq.ToolChoice = "auto"

					toolResp, err := client.ChatCompletion(ctx, toolReq)
					if err != nil {
						t.Fatalf("Tools API error: %v", err)
					}
					if len(toolResp.Choices) == 0 {
						t.Fatal("No choices returned from tools request")
					}

					t.Logf("✓ %s tools test completed successfully", model)
				} else {
					t.Logf("Skipping tools test for %s (not supported)", model)
				}

				// Small delay between tools and functions tests
				time.Sleep(2 * time.Second)

				// Test 2: Functions-based conversation
				t.Logf("Testing %s with functions...", model)
				if !doesNotSupportFunctions(model) {
					// Define functions for the test
					functions := []ogem.Function{
						{
							Name:        "calculate_compound_interest",
							Description: "Calculate compound interest for an investment",
							Parameters: map[string]interface{}{
								"type": "object",
								"properties": map[string]interface{}{
									"principal": map[string]interface{}{
										"type":        "number",
										"description": "Initial investment amount",
									},
									"rate": map[string]interface{}{
										"type":        "number",
										"description": "Annual interest rate as a decimal",
									},
									"time": map[string]interface{}{
										"type":        "number",
										"description": "Investment period in years",
									},
									"frequency": map[string]interface{}{
										"type":        "integer",
										"description": "Number of times interest is compounded per year",
									},
								},
								"required": []string{"principal", "rate", "time", "frequency"},
							},
						},
					}

					// Build conversation messages for functions
					funcMsgs := []ogem.Message{
						ogem.NewUserMessage("Calculate the compound interest for $10,000 invested at 5% annual rate for 3 years, compounded monthly"),
					}

					// Add assistant message with function call
					funcMsgs = append(funcMsgs, ogem.Message{
						Role:    ogem.RoleAssistant,
						Content: nil,
						FunctionCall: &ogem.FunctionCall{
							Name:      "calculate_compound_interest",
							Arguments: `{"principal": 10000, "rate": 0.05, "time": 3, "frequency": 12}`,
						},
					})

					// Add function response
					funcMsgs = append(funcMsgs, ogem.Message{
						Role:    ogem.RoleFunction,
						Name:    "calculate_compound_interest",
						Content: `{"final_amount": 11614.72, "interest_earned": 1614.72, "effective_rate": 0.0512}`,
					})

					// // Add final user question
					// funcMsgs = append(funcMsgs, ogem.NewUserMessage("What's the effective annual rate?"))

					// Create request with functions
					funcReq := ogem.NewChatCompletionRequest(model, funcMsgs)
					funcReq.Functions = functions

					funcResp, err := client.ChatCompletion(ctx, funcReq)
					if err != nil {
						t.Fatalf("Functions API error: %v", err)
					}
					if len(funcResp.Choices) == 0 {
						t.Fatal("No choices returned from functions request")
					}

					t.Logf("✓ %s functions test completed successfully", model)
				} else {
					t.Logf("Skipping functions test for %s (not supported)", model)
				}

				t.Logf("✓ %s completed both tools and functions tests", model)
			})
		}()

		// Small delay between tests to avoid overwhelming the API
		time.Sleep(1 * time.Second)
	}

	t.Logf("ToolsAndFunctionsSequential test completed for all %d models", len(testModels))
}
