package azureappconfiguration

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"reflect"
	"regexp"
	"strconv"
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

			// Check if this is a Key Vault reference by content type
			isKeyVaultRef := setting.ContentType != nil && *setting.ContentType == KeyVaultContentType

			// Also check by value prefix as a fallback
			// if !isKeyVaultRef && setting.Value != nil && isKeyVaultReference(*setting.Value) {
			// 	isKeyVaultRef = true
			// }

			// Resolve Key Vault references immediately if needed
			if isKeyVaultRef && cfg.keyVaultOptions != nil && setting.Value != nil {
				uri, err := parseKeyVaultReference(*setting.Value)
				if err != nil {
					return fmt.Errorf("failed to parse Key Vault reference for key %s: %w", key, err)
				}
				secret, err := cfg.getKeyVaultSecret(ctx, uri)
				if err != nil {
					return fmt.Errorf("failed to retrieve Key Vault secret for key %s: %w", key, err)
				}
				// Update the setting with the resolved secret
				settingCopy := setting
				settingCopy.Value = &secret
				cfg.settings[key] = settingCopy
			}
		}
	}

	return nil
}

// getKeyVaultSecret retrieves a secret from Key Vault using the provided URI
func (cfg *AzureAppConfiguration) getKeyVaultSecret(ctx context.Context, uri string) (string, error) {
	// Skip if KeyVaultOptions is not configured
	if cfg.keyVaultOptions == nil {
		return "", errors.New("Key Vault options not configured")
	}

	// Check if this reference is already cached
	cfg.keyVaultCacheMu.RLock()
	cachedSecret, found := cfg.keyVaultCache[uri]
	cfg.keyVaultCacheMu.RUnlock()

	if found {
		// Use the cached value
		return cachedSecret, nil
	}

	// Resolve the secret using the appropriate method
	resolvedSecret, err := cfg.resolveSecret(ctx, uri)
	if err != nil {
		return "", err
	}

	// Cache the resolved secret
	cfg.keyVaultCacheMu.Lock()
	cfg.keyVaultCache[uri] = resolvedSecret
	cfg.keyVaultCacheMu.Unlock()

	return resolvedSecret, nil
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

	// Get the raw JSON data
	rawBytes := cfg.GetRawBytes(options)

	// Unmarshal into a map first for preprocessing
	var configMap map[string]interface{}
	if err := json.Unmarshal(rawBytes, &configMap); err != nil {
		return fmt.Errorf("failed to unmarshal configuration: %w", err)
	}

	// Get the target type information to guide the conversion
	targetType := reflect.TypeOf(configStruct).Elem()

	// Process the map with type information to guide conversions
	processedMap := preprocessConfigMapWithTypeInfo(configMap, targetType)

	// Convert the processed map back to JSON
	processedBytes, err := json.Marshal(processedMap)
	if err != nil {
		return fmt.Errorf("failed to marshal processed configuration: %w", err)
	}

	// Now unmarshal the processed JSON into the struct
	decoder := json.NewDecoder(strings.NewReader(string(processedBytes)))
	decoder.DisallowUnknownFields() // Optional: fails if there are fields in the JSON that don't exist in the struct

	if err := decoder.Decode(configStruct); err != nil {
		return fmt.Errorf("failed to unmarshal configuration into struct: %w", err)
	}

	return nil
}

// preprocessConfigMapWithTypeInfo processes a configuration map while considering the target struct type.
// It uses type information to determine whether to convert string values or not.
func preprocessConfigMapWithTypeInfo(configMap map[string]interface{}, targetType reflect.Type) map[string]interface{} {
	if targetType.Kind() != reflect.Struct {
		// If not targeting a struct, use the standard preprocessing
		return preprocessConfigMap(configMap)
	}

	result := make(map[string]interface{})

	for key, value := range configMap {
		// Try to find the corresponding field in the target struct
		var field reflect.StructField
		var found bool

		// Handle potential JSON tag mappings
		for i := 0; i < targetType.NumField(); i++ {
			f := targetType.Field(i)

			// Check for JSON tag that matches the key
			tag := f.Tag.Get("json")
			tagName := strings.Split(tag, ",")[0]

			if tagName == key {
				field = f
				found = true
				break
			} else if f.Name == key {
				// Also check by field name
				field = f
				found = true
				break
			}
		}

		// Process the value based on its type and the target field
		switch v := value.(type) {
		case string:
			// If we found a matching field, consider its type
			if found {
				fieldKind := field.Type.Kind()

				// If the target is a string, keep as string
				if fieldKind == reflect.String {
					result[key] = v
				} else {
					// Try to convert to the appropriate type
					result[key] = convertStringToAppropriateType(v)
				}
			} else {
				// If no matching field, apply general conversion
				result[key] = convertStringToAppropriateType(v)
			}

		case map[string]interface{}:
			// Handle nested structs
			var nestedType reflect.Type
			if found && field.Type.Kind() == reflect.Struct {
				nestedType = field.Type
			}

			if nestedType != nil {
				result[key] = preprocessConfigMapWithTypeInfo(v, nestedType)
			} else {
				result[key] = preprocessConfigMap(v)
			}

		case []interface{}:
			// Handle arrays/slices
			var elemType reflect.Type
			if found && field.Type.Kind() == reflect.Slice {
				elemType = field.Type.Elem()
			}

			if elemType != nil {
				result[key] = preprocessArrayWithTypeInfo(v, elemType)
			} else {
				result[key] = preprocessArray(v)
			}

		default:
			// For non-string values, check if they need conversion to string
			if found && field.Type.Kind() == reflect.String {
				// Convert to string if the target is a string
				result[key] = fmt.Sprintf("%v", v)
			} else {
				result[key] = v
			}
		}
	}

	return result
}

// preprocessArrayWithTypeInfo processes array elements with type information
func preprocessArrayWithTypeInfo(arr []interface{}, elemType reflect.Type) []interface{} {
	result := make([]interface{}, len(arr))

	for i, item := range arr {
		switch v := item.(type) {
		case string:
			if elemType.Kind() == reflect.String {
				// Keep as string if target type is string
				result[i] = v
			} else {
				// Try to convert
				result[i] = convertStringToAppropriateType(v)
			}

		case map[string]interface{}:
			if elemType.Kind() == reflect.Struct {
				result[i] = preprocessConfigMapWithTypeInfo(v, elemType)
			} else {
				result[i] = preprocessConfigMap(v)
			}

		case []interface{}:
			if elemType.Kind() == reflect.Slice {
				result[i] = preprocessArrayWithTypeInfo(v, elemType.Elem())
			} else {
				result[i] = preprocessArray(v)
			}

		default:
			// For non-string values, check if they need conversion to string
			if elemType.Kind() == reflect.String {
				// Convert to string if the target is a string
				result[i] = fmt.Sprintf("%v", v)
			} else {
				result[i] = v
			}
		}
	}

	return result
}

// preprocessConfigMap recursively processes a configuration map to convert string values
// to appropriate types based on common patterns (numbers, booleans, etc.)
func preprocessConfigMap(configMap map[string]interface{}) map[string]interface{} {
	result := make(map[string]interface{})

	for key, value := range configMap {
		switch v := value.(type) {
		case string:
			// Try to convert the string to an appropriate type
			result[key] = convertStringToAppropriateType(v)
		case map[string]interface{}:
			// Recursively process nested maps
			result[key] = preprocessConfigMap(v)
		case []interface{}:
			// Process array elements
			result[key] = preprocessArray(v)
		default:
			// Keep other types as is
			result[key] = v
		}
	}

	return result
}

// preprocessArray processes array elements for type conversion
func preprocessArray(arr []interface{}) []interface{} {
	result := make([]interface{}, len(arr))

	for i, item := range arr {
		switch v := item.(type) {
		case string:
			// Try to convert the string to an appropriate type
			result[i] = convertStringToAppropriateType(v)
		case map[string]interface{}:
			// Recursively process nested maps
			result[i] = preprocessConfigMap(v)
		case []interface{}:
			// Recursively process nested arrays
			result[i] = preprocessArray(v)
		default:
			// Keep other types as is
			result[i] = v
		}
	}

	return result
}

// convertStringToAppropriateType tries to convert a string value to a more appropriate type
// based on its content (integer, float, boolean, etc.)
func convertStringToAppropriateType(s string) interface{} {
	// Check for boolean values
	switch strings.ToLower(s) {
	case "true", "yes", "on":
		return true
	case "false", "no", "off":
		return false
	}

	// Check for integer values
	if i, err := parseInt(s); err == nil {
		return i
	}

	// Check for floating-point values
	if f, err := parseFloat(s); err == nil {
		return f
	}

	// Check for JSON objects or arrays
	if (strings.HasPrefix(s, "{") && strings.HasSuffix(s, "}")) ||
		(strings.HasPrefix(s, "[") && strings.HasSuffix(s, "]")) {
		var result interface{}
		if err := json.Unmarshal([]byte(s), &result); err == nil {
			return result
		}
	}

	// If no conversion applies, return the original string
	return s
}

// parseInt attempts to convert a string to an integer
func parseInt(s string) (int, error) {
	// Try to parse as integer
	i, err := parseInt64(s)
	if err != nil {
		return 0, err
	}

	// Check if the value fits in an int
	if i > int64(^uint(0)>>1) || i < -int64(^uint(0)>>1)-1 {
		return 0, fmt.Errorf("integer value out of range: %s", s)
	}

	return int(i), nil
}

// parseInt64 attempts to convert a string to an int64
func parseInt64(s string) (int64, error) {
	// Remove any commas that might be used for formatting
	s = strings.Replace(s, ",", "", -1)

	// Try to parse as integer
	return strconv.ParseInt(s, 10, 64)
}

// parseFloat attempts to convert a string to a float64
func parseFloat(s string) (float64, error) {
	// Remove any commas that might be used for formatting
	s = strings.Replace(s, ",", "", -1)

	// Try to parse as float
	return strconv.ParseFloat(s, 64)
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
