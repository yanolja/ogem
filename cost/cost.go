package cost

import (
	"fmt"
	"strings"

	"github.com/yanolja/ogem/openai"
)

// PricingTier represents different pricing tiers for models
type PricingTier string

const (
	TierGPT4       PricingTier = "gpt-4"
	TierGPT4Turbo  PricingTier = "gpt-4-turbo"
	TierGPT35Turbo PricingTier = "gpt-3.5-turbo"
	TierClaude3    PricingTier = "claude-3"
	TierGemini     PricingTier = "gemini"
	TierDALLE2     PricingTier = "dall-e-2"
	TierDALLE3     PricingTier = "dall-e-3"
	TierUnknown    PricingTier = "unknown"
)

// ModelPricing contains pricing information for input and output tokens
type ModelPricing struct {
	InputTokenPrice  float64 // Price per 1M input tokens
	OutputTokenPrice float64 // Price per 1M output tokens
	ImagePricing     *ImagePricing
}

// ImagePricing contains pricing information for image generation
type ImagePricing struct {
	StandardPrice float64 // Price per standard image
	HDPrice       float64 // Price per HD image (if supported)
}

// Define pricing for different models (prices per 1M tokens in USD)
var modelPricing = map[string]ModelPricing{
	// Latest OpenAI Models (2025)
	
	// GPT-4.5 Series (Latest Flagship)
	"gpt-4.5-turbo": {
		InputTokenPrice:  2.0,
		OutputTokenPrice: 8.0,
	},
	"gpt-4.5-turbo-vision": {
		InputTokenPrice:  2.5,
		OutputTokenPrice: 10.0,
	},
	
	// GPT-4.1 Series
	"gpt-4.1-turbo": {
		InputTokenPrice:  1.8,
		OutputTokenPrice: 7.0,
	},
	"gpt-4.1-preview": {
		InputTokenPrice:  2.2,
		OutputTokenPrice: 9.0,
	},
	
	// GPT-4o models (Legacy)
	"gpt-4o": {
		InputTokenPrice:  2.5,
		OutputTokenPrice: 10.0,
	},
	"gpt-4o-mini": {
		InputTokenPrice:  0.15,
		OutputTokenPrice: 0.6,
	},
	"gpt-4o-realtime": {
		InputTokenPrice:  5.0,
		OutputTokenPrice: 20.0,
	},
	
	// o4 reasoning models (Latest)
	"o4": {
		InputTokenPrice:  8.0,
		OutputTokenPrice: 25.0,
	},
	"o4-mini": {
		InputTokenPrice:  2.0,
		OutputTokenPrice: 6.0,
	},
	
	// o3 reasoning models
	"o3": {
		InputTokenPrice:  10.0,
		OutputTokenPrice: 30.0,
	},
	"o3-mini": {
		InputTokenPrice:  2.5,
		OutputTokenPrice: 8.0,
	},
	
	// o1 reasoning models (Deprecated)
	"o1-preview": {
		InputTokenPrice:  15.0,
		OutputTokenPrice: 60.0,
	},
	"o1-mini": {
		InputTokenPrice:  3.0,
		OutputTokenPrice: 12.0,
	},
	
	// GPT-4 models
	"gpt-4": {
		InputTokenPrice:  30.0,
		OutputTokenPrice: 60.0,
	},
	"gpt-4-turbo": {
		InputTokenPrice:  10.0,
		OutputTokenPrice: 30.0,
	},
	"gpt-4-turbo-2024-04-09": {
		InputTokenPrice:  10.0,
		OutputTokenPrice: 30.0,
	},
	
	// GPT-3.5 models
	"gpt-3.5-turbo": {
		InputTokenPrice:  0.5,
		OutputTokenPrice: 1.5,
	},
	"gpt-3.5-turbo-0125": {
		InputTokenPrice:  0.5,
		OutputTokenPrice: 1.5,
	},
	
	// Latest Claude Models (2025)
	
	// Claude 4 Series (Latest)
	"claude-4-opus": {
		InputTokenPrice:  8.0,
		OutputTokenPrice: 30.0,
	},
	"claude-4-sonnet": {
		InputTokenPrice:  2.0,
		OutputTokenPrice: 8.0,
	},
	"claude-4-haiku": {
		InputTokenPrice:  0.4,
		OutputTokenPrice: 2.0,
	},
	
	// Claude 3.7 Series
	"claude-3.7-opus": {
		InputTokenPrice:  10.0,
		OutputTokenPrice: 40.0,
	},
	"claude-3.7-sonnet": {
		InputTokenPrice:  2.5,
		OutputTokenPrice: 10.0,
	},
	"claude-3.7-haiku": {
		InputTokenPrice:  0.6,
		OutputTokenPrice: 3.0,
	},
	
	// Claude 3.5 Series (Legacy)
	"claude-3-5-sonnet": {
		InputTokenPrice:  3.0,
		OutputTokenPrice: 15.0,
	},
	"claude-3-5-sonnet-20241022": {
		InputTokenPrice:  3.0,
		OutputTokenPrice: 15.0,
	},
	"claude-3-5-haiku": {
		InputTokenPrice:  0.8,
		OutputTokenPrice: 4.0,
	},
	"claude-3-5-haiku-20241022": {
		InputTokenPrice:  0.8,
		OutputTokenPrice: 4.0,
	},
	
	// Claude 3 Series (Deprecated)
	"claude-3-opus": {
		InputTokenPrice:  15.0,
		OutputTokenPrice: 75.0,
	},
	"claude-3-sonnet": {
		InputTokenPrice:  3.0,
		OutputTokenPrice: 15.0,
	},
	"claude-3-haiku": {
		InputTokenPrice:  0.25,
		OutputTokenPrice: 1.25,
	},
	
	// Latest Gemini 2.5 Family (2025)
	"gemini-2.5-pro": {
		InputTokenPrice:  1.25, // Estimated flagship pricing
		OutputTokenPrice: 5.0,
	},
	"gemini-2.5-flash": {
		InputTokenPrice:  0.1,  // $0.10 per 1M tokens (confirmed)
		OutputTokenPrice: 0.6,  // $0.60 per 1M tokens (confirmed)
	},
	"gemini-2.5-flash-lite": {
		InputTokenPrice:  0.05, // Estimated most cost-efficient
		OutputTokenPrice: 0.3,
	},
	"gemini-2.5-pro-deep-think": {
		InputTokenPrice:  2.0,  // Estimated experimental pricing
		OutputTokenPrice: 8.0,
	},
	
	// Gemini 2.0 Family
	"gemini-2.0-flash": {
		InputTokenPrice:  0.075, // Experimental low latency
		OutputTokenPrice: 0.3,
	},
	"gemini-2.0-flash-lite": {
		InputTokenPrice:  0.04,  // Cost-efficient
		OutputTokenPrice: 0.2,
	},
	
	// Legacy Gemini models (deprecated - will be removed April 29, 2025)
	"gemini-1.5-pro-002": {
		InputTokenPrice:  1.25,
		OutputTokenPrice: 5.0,
	},
	"gemini-1.5-flash-002": {
		InputTokenPrice:  0.075,
		OutputTokenPrice: 0.3,
	},
	"gemini-1.5-pro": {
		InputTokenPrice:  1.25,
		OutputTokenPrice: 5.0,
	},
	"gemini-1.5-flash": {
		InputTokenPrice:  0.075,
		OutputTokenPrice: 0.3,
	},
	"gemini-pro": {
		InputTokenPrice:  1.25,  // Deprecated - maps to gemini-2.5-pro pricing
		OutputTokenPrice: 5.0,
	},
	
	// Image generation models
	"dall-e-2": {
		InputTokenPrice:  0.0,
		OutputTokenPrice: 0.0,
		ImagePricing: &ImagePricing{
			StandardPrice: 0.020, // $0.020 per image
		},
	},
	"dall-e-3": {
		InputTokenPrice:  0.0,
		OutputTokenPrice: 0.0,
		ImagePricing: &ImagePricing{
			StandardPrice: 0.040, // $0.040 per standard 1024x1024
			HDPrice:       0.080, // $0.080 per HD 1024x1024
		},
	},
	
	// Text-to-speech models
	"tts-1": {
		InputTokenPrice:  15.0, // $15 per 1M characters
		OutputTokenPrice: 0.0,
	},
	"tts-1-hd": {
		InputTokenPrice:  30.0, // $30 per 1M characters
		OutputTokenPrice: 0.0,
	},
	
	// Speech-to-text models
	"whisper-1": {
		InputTokenPrice:  6.0, // $0.006 per minute, approximated as tokens
		OutputTokenPrice: 0.0,
	},
	
	// Embeddings models
	"text-embedding-ada-002": {
		InputTokenPrice:  0.1,
		OutputTokenPrice: 0.0,
	},
	"text-embedding-3-small": {
		InputTokenPrice:  0.02,
		OutputTokenPrice: 0.0,
	},
	"text-embedding-3-large": {
		InputTokenPrice:  0.13,
		OutputTokenPrice: 0.0,
	},
}

// CalculateChatCost calculates the cost for a chat completion request
func CalculateChatCost(model string, usage openai.Usage) float64 {
	normalizedModel := normalizeModelName(model)
	pricing, exists := modelPricing[normalizedModel]
	if !exists {
		// Default to a reasonable pricing if model not found
		pricing = ModelPricing{
			InputTokenPrice:  1.0,
			OutputTokenPrice: 2.0,
		}
	}
	
	inputCost := float64(usage.PromptTokens) * pricing.InputTokenPrice / 1000000.0
	outputCost := float64(usage.CompletionTokens) * pricing.OutputTokenPrice / 1000000.0
	
	return inputCost + outputCost
}

// CalculateEmbeddingCost calculates the cost for an embedding request
func CalculateEmbeddingCost(model string, usage openai.EmbeddingUsage) float64 {
	normalizedModel := normalizeModelName(model)
	pricing, exists := modelPricing[normalizedModel]
	if !exists {
		// Default embedding pricing
		pricing = ModelPricing{
			InputTokenPrice: 0.1, // $0.1 per 1M tokens
		}
	}
	
	return float64(usage.TotalTokens) * pricing.InputTokenPrice / 1000000.0
}

// CalculateImageCost calculates the cost for image generation
func CalculateImageCost(model string, request *openai.ImageGenerationRequest, numImages int) float64 {
	normalizedModel := normalizeModelName(model)
	pricing, exists := modelPricing[normalizedModel]
	if !exists || pricing.ImagePricing == nil {
		// Default image pricing
		return float64(numImages) * 0.020 // $0.020 per image
	}
	
	baseCost := pricing.ImagePricing.StandardPrice
	
	// Check if HD quality is requested
	if request.Quality != nil && strings.ToLower(*request.Quality) == "hd" && pricing.ImagePricing.HDPrice > 0 {
		baseCost = pricing.ImagePricing.HDPrice
	}
	
	// Adjust cost based on size (simplified pricing model)
	if request.Size != nil {
		switch *request.Size {
		case "1792x1024", "1024x1792":
			baseCost *= 2.0 // Larger images cost more
		case "512x512":
			baseCost *= 0.5 // Smaller images cost less
		}
	}
	
	return float64(numImages) * baseCost
}

// normalizeModelName normalizes model names to match our pricing table
func normalizeModelName(model string) string {
	lower := strings.ToLower(model)
	
	// Latest OpenAI models (2025)
	
	// o4 reasoning models (Latest)
	if strings.Contains(lower, "o4-mini") {
		return "o4-mini"
	}
	if strings.Contains(lower, "o4") {
		return "o4"
	}
	
	// o3 reasoning models
	if strings.Contains(lower, "o3-mini") {
		return "o3-mini"
	}
	if strings.Contains(lower, "o3") {
		return "o3"
	}
	
	// o1 reasoning models (Deprecated - map to o3)
	if strings.Contains(lower, "o1-preview") {
		return "o3" // Map deprecated o1-preview to o3
	}
	if strings.Contains(lower, "o1-mini") {
		return "o3-mini" // Map deprecated o1-mini to o3-mini
	}
	
	// GPT-4.5 Series (Latest)
	if strings.Contains(lower, "gpt-4.5-turbo-vision") {
		return "gpt-4.5-turbo-vision"
	}
	if strings.Contains(lower, "gpt-4.5-turbo") {
		return "gpt-4.5-turbo"
	}
	
	// GPT-4.1 Series
	if strings.Contains(lower, "gpt-4.1-preview") {
		return "gpt-4.1-preview"
	}
	if strings.Contains(lower, "gpt-4.1-turbo") {
		return "gpt-4.1-turbo"
	}
	
	// GPT-4o models (Legacy)
	if strings.Contains(lower, "gpt-4o-realtime") {
		return "gpt-4o-realtime"
	}
	if strings.Contains(lower, "gpt-4o-mini") {
		return "gpt-4o-mini"
	}
	if strings.Contains(lower, "gpt-4o") {
		return "gpt-4o"
	}
	
	// GPT-4 models (Deprecated - map to GPT-4.1)
	if strings.Contains(lower, "gpt-4-turbo-2024-04-09") {
		return "gpt-4.1-turbo" // Map deprecated to latest
	}
	if strings.Contains(lower, "gpt-4-turbo") || strings.Contains(lower, "gpt-4-1106") || strings.Contains(lower, "gpt-4-0125") {
		return "gpt-4.1-turbo" // Map deprecated to latest
	}
	if strings.Contains(lower, "gpt-4") {
		return "gpt-4.1-turbo" // Map deprecated gpt-4 to latest
	}
	
	// OpenAI GPT-3.5 models
	if strings.Contains(lower, "gpt-3.5-turbo-0125") {
		return "gpt-3.5-turbo-0125"
	}
	if strings.Contains(lower, "gpt-3.5-turbo") {
		return "gpt-3.5-turbo"
	}
	
	// Anthropic Claude models (2025)
	
	// Claude 4 Series (Latest)
	if strings.Contains(lower, "claude-4-opus") {
		return "claude-4-opus"
	}
	if strings.Contains(lower, "claude-4-sonnet") {
		return "claude-4-sonnet"
	}
	if strings.Contains(lower, "claude-4-haiku") {
		return "claude-4-haiku"
	}
	
	// Claude 3.7 Series
	if strings.Contains(lower, "claude-3.7-opus") {
		return "claude-3.7-opus"
	}
	if strings.Contains(lower, "claude-3.7-sonnet") {
		return "claude-3.7-sonnet"
	}
	if strings.Contains(lower, "claude-3.7-haiku") {
		return "claude-3.7-haiku"
	}
	
	// Claude 3.5 Series (Legacy - map to 3.7)
	if strings.Contains(lower, "claude-3-5-haiku-20241022") {
		return "claude-3.7-haiku" // Map legacy to 3.7
	}
	if strings.Contains(lower, "claude-3-5-haiku") {
		return "claude-3.7-haiku" // Map legacy to 3.7
	}
	if strings.Contains(lower, "claude-3-5-sonnet-20241022") {
		return "claude-3.7-sonnet" // Map legacy to 3.7
	}
	if strings.Contains(lower, "claude-3-5-sonnet") {
		return "claude-3.7-sonnet" // Map legacy to 3.7
	}
	
	// Claude 3 Series (Deprecated - map to 3.7)
	if strings.Contains(lower, "claude-3-opus") {
		return "claude-3.7-opus" // Map deprecated to 3.7
	}
	if strings.Contains(lower, "claude-3-sonnet") {
		return "claude-3.7-sonnet" // Map deprecated to 3.7
	}
	if strings.Contains(lower, "claude-3-haiku") {
		return "claude-3.7-haiku" // Map deprecated to 3.7
	}
	
	// Google Gemini models (latest versions first)
	// Gemini 2.5 Family (Latest)
	if strings.Contains(lower, "gemini-2.5-pro-deep-think") {
		return "gemini-2.5-pro-deep-think"
	}
	if strings.Contains(lower, "gemini-2.5-flash-lite") {
		return "gemini-2.5-flash-lite"
	}
	if strings.Contains(lower, "gemini-2.5-flash") {
		return "gemini-2.5-flash"
	}
	if strings.Contains(lower, "gemini-2.5-pro") {
		return "gemini-2.5-pro"
	}
	
	// Gemini 2.0 Family
	if strings.Contains(lower, "gemini-2.0-flash-lite") {
		return "gemini-2.0-flash-lite"
	}
	if strings.Contains(lower, "gemini-2.0-flash") {
		return "gemini-2.0-flash"
	}
	
	// Legacy Gemini models (deprecated)
	if strings.Contains(lower, "gemini-1.5-pro-002") {
		return "gemini-1.5-pro-002"
	}
	if strings.Contains(lower, "gemini-1.5-pro") {
		return "gemini-1.5-pro"
	}
	if strings.Contains(lower, "gemini-1.5-flash-002") {
		return "gemini-1.5-flash-002"
	}
	if strings.Contains(lower, "gemini-1.5-flash") {
		return "gemini-1.5-flash"
	}
	if strings.Contains(lower, "gemini-pro") {
		return "gemini-2.5-pro"  // Deprecated: gemini-pro maps to latest
	}
	
	// Default Gemini fallback to latest flagship
	if strings.Contains(lower, "gemini") {
		return "gemini-2.5-pro"
	}
	
	// OpenAI Audio models
	if strings.Contains(lower, "whisper-1") {
		return "whisper-1"
	}
	if strings.Contains(lower, "tts-1-hd") {
		return "tts-1-hd"
	}
	if strings.Contains(lower, "tts-1") {
		return "tts-1"
	}
	
	// OpenAI Embedding models
	if strings.Contains(lower, "text-embedding-3-large") {
		return "text-embedding-3-large"
	}
	if strings.Contains(lower, "text-embedding-3-small") {
		return "text-embedding-3-small"
	}
	if strings.Contains(lower, "text-embedding-ada-002") {
		return "text-embedding-ada-002"
	}
	
	// OpenAI Image generation models
	if strings.Contains(lower, "dall-e-3") {
		return "dall-e-3"
	}
	if strings.Contains(lower, "dall-e-2") {
		return "dall-e-2"
	}
	
	// Return the original model name if no match found
	return lower
}

// GetModelPricing returns the pricing information for a given model
func GetModelPricing(model string) (ModelPricing, error) {
	normalizedModel := normalizeModelName(model)
	pricing, exists := modelPricing[normalizedModel]
	if !exists {
		return ModelPricing{}, fmt.Errorf("pricing not available for model: %s", model)
	}
	return pricing, nil
}

// CostEstimateRequest represents a request for cost estimation
type CostEstimateRequest struct {
	Model              string `json:"model"`
	EstimatedInputTokens  int32  `json:"estimated_input_tokens,omitempty"`
	EstimatedOutputTokens int32  `json:"estimated_output_tokens,omitempty"`
	ImageCount            int32  `json:"image_count,omitempty"`
	ImageSize             string `json:"image_size,omitempty"`
	ImageQuality          string `json:"image_quality,omitempty"`
}

// CostEstimateResponse represents the cost estimation response
type CostEstimateResponse struct {
	Model           string        `json:"model"`
	Pricing         ModelPricing  `json:"pricing"`
	EstimatedCost   float64       `json:"estimated_cost"`
	Breakdown       CostBreakdown `json:"breakdown,omitempty"`
}

// CostBreakdown provides detailed cost breakdown
type CostBreakdown struct {
	InputCost  float64 `json:"input_cost,omitempty"`
	OutputCost float64 `json:"output_cost,omitempty"`
	ImageCost  float64 `json:"image_cost,omitempty"`
}

// EstimateCost estimates the cost for a given request
func EstimateCost(req CostEstimateRequest) (CostEstimateResponse, error) {
	pricing, err := GetModelPricing(req.Model)
	if err != nil {
		return CostEstimateResponse{}, err
	}

	response := CostEstimateResponse{
		Model:   req.Model,
		Pricing: pricing,
	}

	var breakdown CostBreakdown
	var totalCost float64

	// Calculate text generation costs
	if req.EstimatedInputTokens > 0 || req.EstimatedOutputTokens > 0 {
		inputCost := float64(req.EstimatedInputTokens) * pricing.InputTokenPrice / 1000000.0
		outputCost := float64(req.EstimatedOutputTokens) * pricing.OutputTokenPrice / 1000000.0
		breakdown.InputCost = inputCost
		breakdown.OutputCost = outputCost
		totalCost += inputCost + outputCost
	}

	// Calculate image generation costs
	if req.ImageCount > 0 && pricing.ImagePricing != nil {
		imageReq := &openai.ImageGenerationRequest{
			N: &req.ImageCount,
		}
		if req.ImageSize != "" {
			imageReq.Size = &req.ImageSize
		}
		if req.ImageQuality != "" {
			imageReq.Quality = &req.ImageQuality
		}
		
		imageCost := CalculateImageCost(req.Model, imageReq, int(req.ImageCount))
		breakdown.ImageCost = imageCost
		totalCost += imageCost
	}

	response.EstimatedCost = totalCost
	response.Breakdown = breakdown

	return response, nil
}