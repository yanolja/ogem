package ogem

import (
	"fmt"
	"time"
)

// ProvidersStatus is a map of provider names to their status.
type ProvidersStatus map[string]*ProviderStatus

type ProviderStatus struct {
	// Base URL of the endpoint. E.g., "http://localhost:8080/v1"
	BaseUrl string `yaml:"base_url" json:"base_url"`

	// API protocol used by the endpoint. E.g., "openai"
	Protocol string `yaml:"protocol" json:"protocol"`

	// Environment variable name for the API key. E.g., "SELF_HOST_API_KEY"
	ApiKeyEnv string `yaml:"api_key_env" json:"api_key_env"`

	// Regions maps region names to their status.
	// The "default" region configures provider-wide settings.
	// E.g., Regions["us-central1"]
	Regions map[string]*RegionStatus `yaml:"regions" json:"regions"`
}

type RegionStatus struct {
	// Models supported by this region. Actual supported models are
	// a combination of this list and the default models of the provider.
	Models []*SupportedModel `yaml:"models" json:"models"`

	// Latency to this region.
	// Measured with minimal token completion and the fastest model.
	Latency time.Duration `json:"latency"`

	// Last time the region status was updated.
	LastChecked time.Time `json:"last_checked"`
}

type SupportedModel struct {
	// Model name. E.g., "gpt-4o"
	Name string `yaml:"name" json:"name"`

	// Model aliases. All names here and in `name` must refer to the same model.
	// E.g., {"gpt-4o-2024-05-13", "gpt-4o-2024-08-06"}
	OtherNames []string `yaml:"other_names" json:"other_names,omitempty"`

	// Rate key. Models sharing this key have the same rate limits.
	// E.g., "gpt-4o"
	RateKey string `yaml:"rate_key" json:"rate_key"`

	// Maximum tokens per minute (TPM).
	// Cannot send more than this number of tokens per minute for this model.
	MaxTokensPerMinute int `yaml:"tpm" json:"tpm,omitempty"`

	// Maximum requests per minute.
	// Cannot send more than this number of requests per minute for this model.
	MaxRequestsPerMinute int `yaml:"rpm" json:"rpm,omitempty"`
}

/**
 * Update modifies or creates a region status for a specific provider and region.
 * It applies the provided callback to update the region status. Note that the
 * model list in the region status is read-only and any changes will be ignored.
 *
 * @param provider - The provider name.
 * @param region - The region name.
 * @param callback - The callback function to be executed to update the region status.
 *   Should return an error to stop the update.
 * @returns An error if the provider or region is not found, or if the callback returns an error.
 */
func (providers ProvidersStatus) Update(provider, region string, callback func(*RegionStatus) error) error {
	providerStatus, ok := providers[provider]
	if !ok {
		return fmt.Errorf("provider %s not found", provider)
	}

	regionStatus, ok := providerStatus.Regions[region]
	if !ok {
		regionStatus = &RegionStatus{}
	}

	// Two reasons for copying the region status:
	// 1. Limits the fields that the callback can modify.
	// 2. Discards any changes made if the callback fails.
	statusCopy := &RegionStatus{
		Latency:     regionStatus.Latency,
		LastChecked: regionStatus.LastChecked,
	}
	copy(statusCopy.Models, regionStatus.Models)
	if err := callback(statusCopy); err != nil {
		return fmt.Errorf("error updating region status: %w", err)
	}

	statusCopy.Models = regionStatus.Models
	providerStatus.Regions[region] = statusCopy
	return nil
}

/**
 * Iterates over all models of all providers and regions.
 *
 * @param callback - The callback function to be executed for each model.
 *   Should return true to stop the iteration.
 *   Parameters are:
 *     provider - The provider name.
 *     providerStatus - The provider status. Read-only.
 *     region - The region name.
 *     regionStatus - The region status. Read-only.
 *     models - The list of models available in the region.
 * @returns true if the iteration was stopped by the callback, false otherwise.
 */
func (providers ProvidersStatus) ForEach(callback func(
	provider string,
	providerStatus ProviderStatus,
	region string,
	regionStatus RegionStatus,
	models []*SupportedModel,
) bool,
) bool {
	defaultModelsOf := func(providerStatus *ProviderStatus) []*SupportedModel {
		if defaultRegion, exists := providerStatus.Regions["default"]; exists {
			return defaultRegion.Models
		}
		return []*SupportedModel{}
	}

	combineModels := func(regionStatus *RegionStatus, defaultModels []*SupportedModel) []*SupportedModel {
		if regionStatus == nil || regionStatus.Models == nil {
			return defaultModels
		}
		return append(defaultModels, regionStatus.Models...)
	}

	for provider, providerStatus := range providers {
		defaultModels := defaultModelsOf(providerStatus)
		for region, regionStatus := range providerStatus.Regions {
			if region == "default" {
				continue
			}
			if regionStatus == nil {
				regionStatus = &RegionStatus{}
			}
			modelList := combineModels(regionStatus, defaultModels)
			stop := callback(provider, *providerStatus, region, *regionStatus, modelList)
			if stop {
				return true
			}
		}
	}
	return false
}
