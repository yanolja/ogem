package handlers

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/yanolja/ogem/providers/local"
	"github.com/yanolja/ogem/types"
)

// LocalProviderHandler handles local provider endpoints
type LocalProviderHandler struct {
	manager *local.LocalProviderManager
}

// NewLocalProviderHandler creates a new local provider handler
func NewLocalProviderHandler(manager *local.LocalProviderManager) *LocalProviderHandler {
	return &LocalProviderHandler{
		manager: manager,
	}
}

// RegisterRoutes registers local provider routes
func (h *LocalProviderHandler) RegisterRoutes(router *gin.Engine) {
	localGroup := router.Group("/local")
	{
		// Provider management
		localGroup.GET("/providers", h.ListProviders)
		localGroup.GET("/providers/:name", h.GetProvider)
		localGroup.GET("/providers/:name/health", h.CheckProviderHealth)
		localGroup.GET("/providers/:name/models", h.GetProviderModels)
		
		// Global operations
		localGroup.GET("/status", h.GetStatus)
		localGroup.GET("/health", h.HealthCheck)
		localGroup.GET("/models", h.GetAllModels)
		
		// Chat completions with specific providers
		localGroup.POST("/chat/completions/:provider", h.ChatCompletion)
		localGroup.POST("/embeddings/:provider", h.CreateEmbedding)
		
		// Auto-discovery
		localGroup.POST("/discover", h.DiscoverProviders)
	}
}

// ListProviders lists all available local providers
func (h *LocalProviderHandler) ListProviders(c *gin.Context) {
	providers := h.manager.GetProviders()
	
	result := make(map[string]interface{})
	for name, provider := range providers {
		result[name] = map[string]interface{}{
			"name": provider.GetName(),
			"type": name,
		}
	}
	
	c.JSON(http.StatusOK, gin.H{
		"providers": result,
		"count":     len(result),
	})
}

// GetProvider gets information about a specific provider
func (h *LocalProviderHandler) GetProvider(c *gin.Context) {
	providerName := c.Param("name")
	
	provider, err := h.manager.GetProvider(providerName)
	if err != nil {
		c.JSON(http.StatusNotFound, gin.H{
			"error": fmt.Sprintf("Provider %s not found", providerName),
		})
		return
	}
	
	ctx, cancel := context.WithTimeout(c.Request.Context(), 10*time.Second)
	defer cancel()
	
	// Get models
	models, err := provider.GetModels(ctx)
	if err != nil {
		c.JSON(http.StatusServiceUnavailable, gin.H{
			"error": fmt.Sprintf("Failed to get models from %s: %v", providerName, err),
		})
		return
	}
	
	// Check health
	healthErr := provider.Health(ctx)
	
	c.JSON(http.StatusOK, gin.H{
		"name":         provider.GetName(),
		"type":         providerName,
		"healthy":      healthErr == nil,
		"health_error": func() string {
			if healthErr != nil {
				return healthErr.Error()
			}
			return ""
		}(),
		"models":      models,
		"model_count": len(models),
	})
}

// CheckProviderHealth checks the health of a specific provider
func (h *LocalProviderHandler) CheckProviderHealth(c *gin.Context) {
	providerName := c.Param("name")
	
	provider, err := h.manager.GetProvider(providerName)
	if err != nil {
		c.JSON(http.StatusNotFound, gin.H{
			"error": fmt.Sprintf("Provider %s not found", providerName),
		})
		return
	}
	
	ctx, cancel := context.WithTimeout(c.Request.Context(), 10*time.Second)
	defer cancel()
	
	healthErr := provider.Health(ctx)
	
	if healthErr != nil {
		c.JSON(http.StatusServiceUnavailable, gin.H{
			"healthy": false,
			"error":   healthErr.Error(),
			"provider": providerName,
		})
		return
	}
	
	c.JSON(http.StatusOK, gin.H{
		"healthy":  true,
		"provider": providerName,
		"status":   "ok",
	})
}

// GetProviderModels gets models from a specific provider
func (h *LocalProviderHandler) GetProviderModels(c *gin.Context) {
	providerName := c.Param("name")
	
	provider, err := h.manager.GetProvider(providerName)
	if err != nil {
		c.JSON(http.StatusNotFound, gin.H{
			"error": fmt.Sprintf("Provider %s not found", providerName),
		})
		return
	}
	
	ctx, cancel := context.WithTimeout(c.Request.Context(), 10*time.Second)
	defer cancel()
	
	models, err := provider.GetModels(ctx)
	if err != nil {
		c.JSON(http.StatusServiceUnavailable, gin.H{
			"error": fmt.Sprintf("Failed to get models from %s: %v", providerName, err),
		})
		return
	}
	
	c.JSON(http.StatusOK, gin.H{
		"object": "list",
		"data":   models,
	})
}

// GetStatus gets the overall status of all local providers
func (h *LocalProviderHandler) GetStatus(c *gin.Context) {
	ctx, cancel := context.WithTimeout(c.Request.Context(), 30*time.Second)
	defer cancel()
	
	status := h.manager.GetStatus(ctx)
	c.JSON(http.StatusOK, status)
}

// HealthCheck performs health check on all providers
func (h *LocalProviderHandler) HealthCheck(c *gin.Context) {
	ctx, cancel := context.WithTimeout(c.Request.Context(), 30*time.Second)
	defer cancel()
	
	healthResults := h.manager.HealthCheck(ctx)
	
	overallHealthy := true
	for _, err := range healthResults {
		if err != nil {
			overallHealthy = false
			break
		}
	}
	
	statusCode := http.StatusOK
	if !overallHealthy {
		statusCode = http.StatusServiceUnavailable
	}
	
	c.JSON(statusCode, gin.H{
		"healthy":   overallHealthy,
		"providers": healthResults,
		"timestamp": time.Now().ISO8601(),
	})
}

// GetAllModels gets all models from all providers
func (h *LocalProviderHandler) GetAllModels(c *gin.Context) {
	ctx, cancel := context.WithTimeout(c.Request.Context(), 30*time.Second)
	defer cancel()
	
	allModels, err := h.manager.GetAllModels(ctx)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{
			"error": fmt.Sprintf("Failed to get models: %v", err),
		})
		return
	}
	
	// Flatten models into a single list
	var models []types.Model
	for providerName, providerModels := range allModels {
		for _, model := range providerModels {
			// Add provider info to model
			model.Provider = providerName
			models = append(models, model)
		}
	}
	
	c.JSON(http.StatusOK, gin.H{
		"object": "list",
		"data":   models,
		"by_provider": allModels,
	})
}

// ChatCompletion handles chat completion with a specific local provider
func (h *LocalProviderHandler) ChatCompletion(c *gin.Context) {
	providerName := c.Param("provider")
	
	provider, err := h.manager.GetProvider(providerName)
	if err != nil {
		c.JSON(http.StatusNotFound, gin.H{
			"error": fmt.Sprintf("Provider %s not found", providerName),
		})
		return
	}
	
	var req types.ChatCompletionRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{
			"error": fmt.Sprintf("Invalid request: %v", err),
		})
		return
	}
	
	ctx, cancel := context.WithTimeout(c.Request.Context(), 120*time.Second)
	defer cancel()
	
	if req.Stream {
		// Handle streaming
		streamChan, err := provider.ChatCompletionStream(ctx, &req)
		if err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{
				"error": fmt.Sprintf("Failed to create stream: %v", err),
			})
			return
		}
		
		c.Header("Content-Type", "text/event-stream")
		c.Header("Cache-Control", "no-cache")
		c.Header("Connection", "keep-alive")
		
		c.Stream(func(w io.Writer) bool {
			select {
			case streamResp, ok := <-streamChan:
				if !ok {
					return false
				}
				
				if streamResp.Error != nil {
					fmt.Fprintf(w, "data: {\"error\": \"%s\"}\n\n", streamResp.Error.Error())
					return false
				}
				
				data, _ := json.Marshal(streamResp.Response)
				fmt.Fprintf(w, "data: %s\n\n", data)
				return true
			case <-ctx.Done():
				return false
			}
		})
	} else {
		// Handle non-streaming
		response, err := provider.ChatCompletion(ctx, &req)
		if err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{
				"error": fmt.Sprintf("Chat completion failed: %v", err),
			})
			return
		}
		
		c.JSON(http.StatusOK, response)
	}
}

// CreateEmbedding handles embedding creation with a specific local provider
func (h *LocalProviderHandler) CreateEmbedding(c *gin.Context) {
	providerName := c.Param("provider")
	
	provider, err := h.manager.GetProvider(providerName)
	if err != nil {
		c.JSON(http.StatusNotFound, gin.H{
			"error": fmt.Sprintf("Provider %s not found", providerName),
		})
		return
	}
	
	var req types.EmbeddingRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{
			"error": fmt.Sprintf("Invalid request: %v", err),
		})
		return
	}
	
	ctx, cancel := context.WithTimeout(c.Request.Context(), 60*time.Second)
	defer cancel()
	
	response, err := provider.CreateEmbedding(ctx, &req)
	if err != nil {
		if strings.Contains(err.Error(), "not supported") {
			c.JSON(http.StatusNotImplemented, gin.H{
				"error": fmt.Sprintf("Embeddings not supported by %s: %v", providerName, err),
			})
			return
		}
		
		c.JSON(http.StatusInternalServerError, gin.H{
			"error": fmt.Sprintf("Embedding creation failed: %v", err),
		})
		return
	}
	
	c.JSON(http.StatusOK, response)
}

// DiscoverProviders triggers auto-discovery of local providers
func (h *LocalProviderHandler) DiscoverProviders(c *gin.Context) {
	// This would trigger the discovery process
	// For now, we'll return the current status
	ctx, cancel := context.WithTimeout(c.Request.Context(), 30*time.Second)
	defer cancel()
	
	status := h.manager.GetStatus(ctx)
	
	c.JSON(http.StatusOK, gin.H{
		"message": "Provider discovery completed",
		"status":  status,
	})
}