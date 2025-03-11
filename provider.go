package azureappconfiguration

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"reflect"
	"regexp"
	"strings"
	"time"

	"github.com/Azure/azure-sdk-for-go/sdk/azcore/to"
	"github.com/Azure/azure-sdk-for-go/sdk/data/azappconfig"
)

// KeyVaultReferencePrefix is the prefix used in settings that reference Key Vault.
const KeyVaultReferencePrefix = "{{\"uri\":"

// FeatureFlagPrefix is the prefix for feature flag keys in App Configuration.
const FeatureFlagPrefix = ".appconfig.featureflag/"

// DefaultSeparator is the default separator used for flattened nested objects.
const DefaultSeparator = "."

// Load initializes and returns a new Azure App Configuration provider instance.
// It loads settings from the Azure App Configuration store according to the provided options.
func Load(ctx context.Context, authOptions AuthenticationOptions, options *Options) (*AzureAppConfiguration, error) {
	// Initialize the client
	client, err := createClient(authOptions)
	if err != nil {
		return nil, fmt.Errorf("failed to create App Configuration client: %w", err)
	}

	// Create a default empty options if none provided
	if options == nil {
		options = &Options{}
	}

	// Initialize the provider instance
	provider := &AzureAppConfiguration{
		client:          client,
		settings:        make(map[string]azappconfig.Setting),
		featureFlags:    make(map[string]FeatureFlag),
		options:         options,
		keyVaultOptions: options.KeyVaultOptions,
		keyVaultCache:   make(map[string]string),
		lastSyncTime:    time.Now(),
	}

	// Load the initial settings
	if err := provider.loadSettings(ctx); err != nil {
		return nil, fmt.Errorf("failed to load settings: %w", err)
	}

	return provider, nil
}

// createClient creates an azappconfig.Client based on the provided authentication options.
func createClient(authOptions AuthenticationOptions) (*azappconfig.Client, error) {
	// Validate authentication options
	if authOptions.ConnectionString == "" && (authOptions.Endpoint == "" || authOptions.Credential == nil) {
		return nil, errors.New("either ConnectionString or both Endpoint and Credential must be provided")
	}

	var client *azappconfig.Client
	var err error

	// Create client using connection string or AAD
	if authOptions.ConnectionString != "" {
		client, err = azappconfig.NewClientFromConnectionString(authOptions.ConnectionString, nil)
		if err != nil {
			return nil, fmt.Errorf("failed to create client from connection string: %w", err)
		}
	} else {
		client, err = azappconfig.NewClient(authOptions.Endpoint, authOptions.Credential, nil)
		if err != nil {
			return nil, fmt.Errorf("failed to create client with endpoint and credential: %w", err)
		}
	}

	return client, nil
}

// loadSettings loads settings from the Azure App Configuration store based on the configured options.
func (cfg *AzureAppConfiguration) loadSettings(ctx context.Context) error {
	cfg.mutex.Lock()
	defer cfg.mutex.Unlock()

	// Initialize or clear the settings map
	cfg.settings = make(map[string]azappconfig.Setting)

	// Load settings based on selectors
	selectors := cfg.getSelectors()

	for _, selector := range selectors {
		if err := cfg.loadSettingsWithSelector(ctx, selector); err != nil {
			return fmt.Errorf("failed to load settings with selector (key: %s, label: %s): %w",
				selector.KeyFilter, selector.LabelFilter, err)
		}
	}

	// Load feature flags if enabled
	if cfg.options.FeatureFlagOptions != nil && cfg.options.FeatureFlagOptions.Enabled {
		if err := cfg.loadFeatureFlags(ctx); err != nil {
			return fmt.Errorf("failed to load feature flags: %w", err)
		}
	}

	// Resolve Key Vault references if needed
	if cfg.keyVaultOptions != nil {
		if err := cfg.resolveKeyVaultReferences(ctx); err != nil {
			return fmt.Errorf("failed to resolve Key Vault references: %w", err)
		}
	}

	// Update last sync time
	cfg.lastSyncTime = time.Now()

	return nil
}

// getSelectors returns the selectors to use for loading settings.
// If no selectors are provided, a default selector for all keys with no label is used.
func (cfg *AzureAppConfiguration) getSelectors() []Selector {
	if cfg.options == nil || len(cfg.options.Selectors) == 0 {
		// Default selector: all keys, no label
		return []Selector{{KeyFilter: "*", LabelFilter: ""}}
	}
	return cfg.options.Selectors
}

// loadSettingsWithSelector loads settings that match the given selector.
func (cfg *AzureAppConfiguration) loadSettingsWithSelector(ctx context.Context, selector Selector) error {
	// Set default key filter if not provided
	keyFilter := selector.KeyFilter
	if keyFilter == "" {
		keyFilter = "*"
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
			return fmt.Errorf("failed to retrieve settings page: %w", err)
		}

		// Process settings from this page
		for _, setting := range page.Settings {
			// Skip feature flags if not specifically requested
			if strings.HasPrefix(*setting.Key, FeatureFlagPrefix) &&
				(cfg.options.FeatureFlagOptions == nil || !cfg.options.FeatureFlagOptions.Enabled) {
				continue
			}

			// Apply key prefix trimming if configured
			key := cfg.trimKeyPrefixes(*setting.Key)

			// Store the setting
			cfg.settings[key] = setting
		}
	}

	return nil
}

// trimKeyPrefixes removes configured prefixes from the key.
func (cfg *AzureAppConfiguration) trimKeyPrefixes(key string) string {
	if cfg.options == nil || len(cfg.options.TrimKeyPrefixes) == 0 {
		return key
	}

	result := key
	for _, prefix := range cfg.options.TrimKeyPrefixes {
		if strings.HasPrefix(result, prefix) {
			result = result[len(prefix):]
			break
		}
	}

	return result
}

// GetRawBytes returns the raw bytes (JSON encoding) of all settings.
// This can be used to integrate with third-party configuration packages like viper or koanf.
func (cfg *AzureAppConfiguration) GetRawBytes(options *ConstructOptions) []byte {
	cfg.mutex.RLock()
	defer cfg.mutex.RUnlock()

	// Create a map to hold the configuration
	configMap := make(map[string]interface{})

	// Get the separator to use
	separator := getSeparator(options)

	// Process each setting
	for key, setting := range cfg.settings {
		if setting.Value == nil {
			continue
		}

		// Handle the value based on its content type
		var value interface{}
		if setting.ContentType != nil && *setting.ContentType == "application/json" {
			// JSON value, parse it
			if err := json.Unmarshal([]byte(*setting.Value), &value); err != nil {
				// If parsing fails, use the raw string
				value = *setting.Value
			}
		} else {
			// Use string value as is
			value = *setting.Value
		}

		// Add to the map using hierarchical structure if separator is found
		addToMapWithSeparator(configMap, key, value, separator)
	}

	// Add feature flags if enabled
	if cfg.options.FeatureFlagOptions != nil && cfg.options.FeatureFlagOptions.Enabled {
		featureFlagsMap := make(map[string]bool)
		for name, flag := range cfg.featureFlags {
			featureFlagsMap[name] = flag.Enabled
		}
		configMap["FeatureManagement"] = featureFlagsMap
	}

	// Serialize to JSON
	jsonBytes, err := json.Marshal(configMap)
	if err != nil {
		// Return empty JSON object if marshaling fails
		return []byte("{}")
	}

	return jsonBytes
}

// Unmarshal unmarshals the configuration into the provided struct.
func (cfg *AzureAppConfiguration) Unmarshal(configStruct interface{}, options *ConstructOptions) error {
	// Check if the target is valid
	if configStruct == nil {
		return errors.New("configStruct cannot be nil")
	}

	// Check if the target is a pointer
	valuePtr := reflect.ValueOf(configStruct)
	if valuePtr.Kind() != reflect.Ptr || valuePtr.IsNil() {
		return errors.New("configStruct must be a non-nil pointer to a struct")
	}

	// Get the raw bytes and unmarshal them into the struct
	rawBytes := cfg.GetRawBytes(options)
	if err := json.Unmarshal(rawBytes, configStruct); err != nil {
		return fmt.Errorf("failed to unmarshal configuration: %w", err)
	}

	return nil
}

// addToMapWithSeparator adds a value to a map, respecting the hierarchical structure
// indicated by the separator in the key.
func addToMapWithSeparator(configMap map[string]interface{}, key string, value interface{}, separator string) {
	if separator == "" || !strings.Contains(key, separator) {
		// No hierarchy, add directly
		configMap[key] = value
		return
	}

	// Split the key by separator
	parts := strings.Split(key, separator)

	// Start with the root map
	current := configMap

	// Process all parts except the last one
	for i := 0; i < len(parts)-1; i++ {
		part := parts[i]
		if part == "" {
			continue
		}

		// Check if this level exists
		next, exists := current[part]
		if !exists {
			// Create a new map for this level
			newMap := make(map[string]interface{})
			current[part] = newMap
			current = newMap
		} else {
			// Check if the existing value is a map
			nextMap, ok := next.(map[string]interface{})
			if !ok {
				// It's not a map, make it a map now
				newMap := make(map[string]interface{})
				current[part] = newMap
				current = newMap
			} else {
				// Use the existing map
				current = nextMap
			}
		}
	}

	// Add the final value at the last part
	current[parts[len(parts)-1]] = value
}

// getSeparator returns the separator to use, defaulting to "." if not specified.
func getSeparator(options *ConstructOptions) string {
	if options == nil || options.Separator == "" {
		return DefaultSeparator
	}

	// Check if the separator is valid
	validSeparators := []string{".", ",", ";", "-", "_", "__", "/", ":"}
	for _, validSep := range validSeparators {
		if options.Separator == validSep {
			return options.Separator
		}
	}

	// Invalid separator, use default
	return DefaultSeparator
}

// isKeyVaultReference checks if a setting value is a Key Vault reference.
func isKeyVaultReference(value string) bool {
	return strings.HasPrefix(value, KeyVaultReferencePrefix)
}

// parseKeyVaultReference extracts the URI from a Key Vault reference.
func parseKeyVaultReference(reference string) (string, error) {
	// Simple regex to extract the URI from the Key Vault reference format
	re := regexp.MustCompile(`"uri":"([^"]+)"`)
	matches := re.FindStringSubmatch(reference)

	if len(matches) < 2 {
		return "", fmt.Errorf("invalid Key Vault reference format: %s", reference)
	}

	return matches[1], nil
}

// getRefreshIntervalWithDefault returns the refresh interval, using the default if not set.
func getRefreshIntervalWithDefault(interval time.Duration) time.Duration {
	if interval <= 0 {
		return DefaultRefreshInterval
	}
	return interval
}
