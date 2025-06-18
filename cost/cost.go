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
	// Latest GPT-4o models
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
	
	// o1 reasoning models
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
	
	// Latest Claude models
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
	
	// Latest Gemini models
	"gemini-pro": {
		InputTokenPrice:  0.5,
		OutputTokenPrice: 1.5,
	},
	"gemini-1.5-pro": {
		InputTokenPrice:  1.25,
		OutputTokenPrice: 5.0,
	},
	"gemini-1.5-pro-002": {
		InputTokenPrice:  1.25,
		OutputTokenPrice: 5.0,
	},
	"gemini-1.5-flash": {
		InputTokenPrice:  0.075,
		OutputTokenPrice: 0.3,
	},
	"gemini-1.5-flash-002": {
		InputTokenPrice:  0.075,
		OutputTokenPrice: 0.3,
	},
	"gemini-2.0-flash": {
		InputTokenPrice:  0.075,
		OutputTokenPrice: 0.3,
	},
	"gemini-2.0-flash-exp": {
		InputTokenPrice:  0.0, // Experimental pricing
		OutputTokenPrice: 0.0,
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
	
	// OpenAI o1 reasoning models
	if strings.Contains(lower, "o1-preview") {
		return "o1-preview"
	}
	if strings.Contains(lower, "o1-mini") {
		return "o1-mini"
	}
	
	// OpenAI GPT-4o models
	if strings.Contains(lower, "gpt-4o-realtime") {
		return "gpt-4o-realtime"
	}
	if strings.Contains(lower, "gpt-4o-mini") {
		return "gpt-4o-mini"
	}
	if strings.Contains(lower, "gpt-4o") {
		return "gpt-4o"
	}
	
	// OpenAI GPT-4 models
	if strings.Contains(lower, "gpt-4-turbo-2024-04-09") {
		return "gpt-4-turbo-2024-04-09"
	}
	if strings.Contains(lower, "gpt-4-turbo") || strings.Contains(lower, "gpt-4-1106") || strings.Contains(lower, "gpt-4-0125") {
		return "gpt-4-turbo"
	}
	if strings.Contains(lower, "gpt-4") {
		return "gpt-4"
	}
	
	// OpenAI GPT-3.5 models
	if strings.Contains(lower, "gpt-3.5-turbo-0125") {
		return "gpt-3.5-turbo-0125"
	}
	if strings.Contains(lower, "gpt-3.5-turbo") {
		return "gpt-3.5-turbo"
	}
	
	// Anthropic Claude models (latest versions)
	if strings.Contains(lower, "claude-3-5-haiku-20241022") {
		return "claude-3-5-haiku-20241022"
	}
	if strings.Contains(lower, "claude-3-5-haiku") {
		return "claude-3-5-haiku"
	}
	if strings.Contains(lower, "claude-3-5-sonnet-20241022") {
		return "claude-3-5-sonnet-20241022"
	}
	if strings.Contains(lower, "claude-3-5-sonnet") {
		return "claude-3-5-sonnet"
	}
	if strings.Contains(lower, "claude-3-opus") {
		return "claude-3-opus"
	}
	if strings.Contains(lower, "claude-3-sonnet") {
		return "claude-3-sonnet"
	}
	if strings.Contains(lower, "claude-3-haiku") {
		return "claude-3-haiku"
	}
	
	// Google Gemini models (latest versions)
	if strings.Contains(lower, "gemini-2.0-flash-exp") {
		return "gemini-2.0-flash-exp"
	}
	if strings.Contains(lower, "gemini-2.0-flash") {
		return "gemini-2.0-flash"
	}
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
		return "gemini-pro"
	}
	if strings.Contains(lower, "gemini") {
		return "gemini-pro"
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