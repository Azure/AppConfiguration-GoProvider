package azureappconfiguration

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	"github.com/Azure/azure-sdk-for-go/sdk/azcore/to"
	"github.com/Azure/azure-sdk-for-go/sdk/data/azappconfig"
)

// loadFeatureFlags loads feature flags from the Azure App Configuration store.
func (cfg *AzureAppConfiguration) loadFeatureFlags(ctx context.Context) error {
	// Skip if feature flags are not enabled
	if cfg.options.FeatureFlagOptions == nil || !cfg.options.FeatureFlagOptions.Enabled {
		return nil
	}

	// Initialize the feature flags map
	cfg.featureFlags = make(map[string]FeatureFlag)

	// Use feature flag specific selectors if provided, otherwise use default
	var selectors []Selector
	if len(cfg.options.FeatureFlagOptions.Selectors) > 0 {
		selectors = cfg.options.FeatureFlagOptions.Selectors
	} else {
		// Default: load all feature flags with no label
		selectors = []Selector{{
			KeyFilter:   fmt.Sprintf("%s*", FeatureFlagPrefix),
			LabelFilter: "",
		}}
	}

	// Load feature flags for each selector
	for _, selector := range selectors {
		if err := cfg.loadFeatureFlagsWithSelector(ctx, selector); err != nil {
			return fmt.Errorf("failed to load feature flags with selector (key: %s, label: %s): %w",
				selector.KeyFilter, selector.LabelFilter, err)
		}
	}

	return nil
}

// loadFeatureFlagsWithSelector loads feature flags that match the given selector.
func (cfg *AzureAppConfiguration) loadFeatureFlagsWithSelector(ctx context.Context, selector Selector) error {
	// Ensure the key filter targets feature flags
	keyFilter := selector.KeyFilter
	if keyFilter == "" || keyFilter == "*" {
		keyFilter = fmt.Sprintf("%s*", FeatureFlagPrefix)
	} else if !strings.Contains(keyFilter, FeatureFlagPrefix) {
		// If the key filter doesn't include the feature flag prefix, add it
		// Only do this if it's not a complex filter pattern
		if !strings.ContainsAny(keyFilter, "*,") {
			keyFilter = fmt.Sprintf("%s%s", FeatureFlagPrefix, keyFilter)
		}
	}

	// Create list options for the query

	settingSelector := azappconfig.SettingSelector{
		KeyFilter:   &keyFilter,
		LabelFilter: to.Ptr(selector.LabelFilter),
	}
	// Query settings from the store
	pager := cfg.client.NewListSettingsPager(settingSelector, nil)

	for pager.More() {
		page, err := pager.NextPage(ctx)
		if err != nil {
			return fmt.Errorf("failed to retrieve feature flags page: %w", err)
		}

		// Process feature flags from this page
		for _, setting := range page.Settings {
			if setting.Key == nil || setting.Value == nil {
				continue
			}

			// Skip settings that don't have the feature flag prefix
			if !strings.HasPrefix(*setting.Key, FeatureFlagPrefix) {
				continue
			}

			// Parse the feature flag
			var featureFlag FeatureFlag
			if err := json.Unmarshal([]byte(*setting.Value), &featureFlag); err != nil {
				return fmt.Errorf("failed to parse feature flag %s: %w", *setting.Key, err)
			}

			// Extract feature name from the key
			featureName := strings.TrimPrefix(*setting.Key, FeatureFlagPrefix)

			// Store the feature flag
			cfg.featureFlags[featureName] = featureFlag
		}
	}

	return nil
}

// IsFeatureEnabled checks if a feature flag is enabled.
func (cfg *AzureAppConfiguration) IsFeatureEnabled(featureName string) bool {
	cfg.mutex.RLock()
	defer cfg.mutex.RUnlock()

	// Check if feature flags are enabled
	if cfg.options.FeatureFlagOptions == nil || !cfg.options.FeatureFlagOptions.Enabled {
		return false
	}

	// Check if the feature flag exists
	featureFlag, found := cfg.featureFlags[featureName]
	if !found {
		return false
	}

	// Return the enabled status
	return featureFlag.Enabled
}

// GetFeatureFlag returns a specific feature flag.
func (cfg *AzureAppConfiguration) GetFeatureFlag(featureName string) (*FeatureFlag, bool) {
	cfg.mutex.RLock()
	defer cfg.mutex.RUnlock()

	// Check if feature flags are enabled
	if cfg.options.FeatureFlagOptions == nil || !cfg.options.FeatureFlagOptions.Enabled {
		return nil, false
	}

	// Check if the feature flag exists
	featureFlag, found := cfg.featureFlags[featureName]
	if !found {
		return nil, false
	}

	// Return a copy of the feature flag
	return &featureFlag, true
}

// GetAllFeatureFlags returns all loaded feature flags.
func (cfg *AzureAppConfiguration) GetAllFeatureFlags() map[string]FeatureFlag {
	cfg.mutex.RLock()
	defer cfg.mutex.RUnlock()

	// Check if feature flags are enabled
	if cfg.options.FeatureFlagOptions == nil || !cfg.options.FeatureFlagOptions.Enabled {
		return make(map[string]FeatureFlag)
	}

	// Create a copy of the feature flags map
	result := make(map[string]FeatureFlag, len(cfg.featureFlags))
	for name, flag := range cfg.featureFlags {
		result[name] = flag
	}

	return result
}
