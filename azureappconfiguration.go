// Copyright (c) Microsoft Corporation.
// Licensed under the MIT License.

package azureappconfiguration

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"regexp"
	"strings"
	"sync"

	"github.com/Azure/AppConfiguration-GoProvider/azureappconfiguration/internal/tree"
	decoder "github.com/go-viper/mapstructure/v2"
	"golang.org/x/sync/errgroup"
)

// AzureAppConfiguration is a configuration provider that retrieves and manages
// settings from Azure App Configuration service
type AzureAppConfiguration struct {
	keyValues    map[string]any
	kvSelectors  []Selector
	trimPrefixes []string

	clientManager *configurationClientManager
	resolver      *keyVaultReferenceResolver
}

// Load initializes and returns an AzureAppConfiguration instance populated with configuration data
// retrieved from Azure App Configuration service.
//
// Parameters:
//   - ctx: Context for controlling request lifetime and cancellation.
//   - authentication: AuthenticationOptions containing credentials or connection details required to access Azure App Configuration.
//   - options: Optional configuration settings to customize the loading behavior. If nil, default settings are applied.
//
// Returns:
//   - A pointer to an AzureAppConfiguration instance containing loaded configuration data.
//   - An error if authentication fails, client initialization encounters issues, or configuration loading is unsuccessful.
//
// Example usage:
//
//	authOptions := azureappconfiguration.AuthenticationOptions{
//	    ConnectionString: os.Getenv("AZURE_APPCONFIG_CONNECTION_STRING"),
//	}
//
//	options := &azureappconfiguration.Options{
//	    Selectors: []azureappconfiguration.Selector{
//	        {
//	            KeyFilter: "AppName:*",  // Load only keys starting with "AppName:"
//	        },
//	    },
//	    TrimKeyPrefixes: []string{"AppName:"},  // Remove "AppName:" prefix from keys
//	}
//
//	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
//	defer cancel()
//
//	provider, err := azureappconfiguration.Load(ctx, authOptions, options)
//	if err != nil {
//	    log.Fatalf("Failed to load configuration: %v", err)
//	}
func Load(ctx context.Context, authentication AuthenticationOptions, options *Options) (*AzureAppConfiguration, error) {
	if err := verifyAuthenticationOptions(authentication); err != nil {
		return nil, err
	}

	if options == nil {
		options = &Options{}
	}

	clientManager, err := newConfigurationClientManager(authentication, options.ClientOptions)
	if err != nil {
		return nil, err
	}

	azappcfg := new(AzureAppConfiguration)
	azappcfg.keyValues = make(map[string]any)
	azappcfg.kvSelectors = deduplicateSelectors(options.Selectors)
	azappcfg.trimPrefixes = options.TrimKeyPrefixes
	azappcfg.clientManager = clientManager
	azappcfg.resolver = &keyVaultReferenceResolver{
		clients:        sync.Map{},
		secretResolver: options.KeyVaultOptions.SecretResolver,
		credential:     options.KeyVaultOptions.Credential,
	}

	if err := azappcfg.load(ctx); err != nil {
		return nil, err
	}

	return azappcfg, nil
}

// Unmarshal converts the configuration data from Azure App Configuration into a struct or other Go type.
// It uses the key-value pairs from the configuration, constructs a hierarchical structure based on the
// separator in the keys, and then decodes this structure into the provided target.
//
// Parameters:
//   - v: A pointer to the target variable where configuration will be unmarshaled.
//        This must be a pointer to a struct or a map. When using structs, the method
//        supports struct fields with `json` tags to control field mapping.
//   - options: Optional ConstructionOptions to customize unmarshaling behavior, such as 
//        specifying a custom separator for hierarchical keys. If nil, default options are used.
//
// Returns:
//   - An error if the configuration cannot be unmarshaled into the target type.
//
// Example usage:
//
//	type Config struct {
//	    Database struct {
//	        Host     string `json:"host"`
//	        Port     int    `json:"port"`
//	        Username string `json:"username"`
//	        Password string `json:"password"`
//	    } `json:"database"`
//	    Timeout int    `json:"timeout"`
//	    Debug   bool   `json:"debug"`
//	}
//
//	var config Config
//	err := provider.Unmarshal(&config, &azureappconfiguration.ConstructionOptions{
//	    Separator: ":",  // Use ":" as the separator in hierarchical keys
//	})
//	if err != nil {
//	    log.Fatalf("Failed to unmarshal configuration: %v", err)
//	}
//
//	fmt.Printf("Database host: %s, port: %d\n", config.Database.Host, config.Database.Port)
func (azappcfg *AzureAppConfiguration) Unmarshal(v any, options *ConstructionOptions) error {
	if options == nil || options.Separator == "" {
		options = &ConstructionOptions{
			Separator: defaultSeparator,
		}
	} else {
		err := verifySeparator(options.Separator)
		if err != nil {
			return err
		}
	}

	config := &decoder.DecoderConfig{
		Result:           v,
		WeaklyTypedInput: true,
		TagName:          "json",
		DecodeHook: decoder.ComposeDecodeHookFunc(
			decoder.StringToTimeDurationHookFunc(),
			decoder.StringToSliceHookFunc(","),
		),
	}

	decoder, err := decoder.NewDecoder(config)
	if err != nil {
		return err
	}

	return decoder.Decode(azappcfg.constructHierarchicalMap(options.Separator))
}

// GetBytes serializes the configuration data from Azure App Configuration into a JSON byte array.
// This method is particularly useful for developers migrating from other configuration systems to 
// Azure App Configuration, as it allows them to feed the configuration data into existing 
// solutions with minimal code changes.
//
// Parameters:
//   - options: Optional ConstructionOptions to customize serialization behavior, such as
//        specifying a custom separator for hierarchical keys. If nil, default options are used.
//
// Returns:
//   - A JSON byte array representing the hierarchical configuration structure.
//   - An error if the configuration cannot be marshaled into JSON.
//
// Example usage:
//
//	// Get configuration as JSON bytes
//	jsonBytes, err := provider.GetBytes(&azureappconfiguration.ConstructionOptions{
//	    Separator: ":",  // Use ":" as the separator in hierarchical keys
//	})
//	if err != nil {
//	    log.Fatalf("Failed to get configuration as bytes: %v", err)
//	}
//
//	// Use with existing configuration systems
//	// Example with Viper:
//	v := viper.New()
//	if err := v.ReadConfig(bytes.NewBuffer(jsonBytes)); err != nil {
//	    log.Fatalf("Failed to read configuration: %v", err)
//	}
//
//	// Example with encoding/json:
//	var config map[string]interface{}
//	if err := json.Unmarshal(jsonBytes, &config); err != nil {
//	    log.Fatalf("Failed to parse configuration: %v", err)
//	}
func (azappcfg *AzureAppConfiguration) GetBytes(options *ConstructionOptions) ([]byte, error) {
	if options == nil || options.Separator == "" {
		options = &ConstructionOptions{
			Separator: defaultSeparator,
		}
	} else {
		err := verifySeparator(options.Separator)
		if err != nil {
			return nil, err
		}
	}

	return json.Marshal(azappcfg.constructHierarchicalMap(options.Separator))
}

func (azappcfg *AzureAppConfiguration) load(ctx context.Context) error {
	keyValuesClient := &selectorSettingsClient{
		selectors: azappcfg.kvSelectors,
		client:    azappcfg.clientManager.staticClient.client,
	}

	return azappcfg.loadKeyValues(ctx, keyValuesClient)
}

func (azappcfg *AzureAppConfiguration) loadKeyValues(ctx context.Context, settingsClient settingsClient) error {
	settingsResponse, err := settingsClient.getSettings(ctx)
	if err != nil {
		return err
	}

	kvSettings := make(map[string]any, len(settingsResponse.settings))
	keyVaultRefs := make(map[string]string)
	for _, setting := range settingsResponse.settings {
		if setting.Key == nil {
			continue
		}
		trimmedKey := azappcfg.trimPrefix(*setting.Key)
		if len(trimmedKey) == 0 {
			log.Printf("Key of the setting '%s' is trimmed to the empty string, just ignore it", *setting.Key)
			continue
		}

		if setting.ContentType == nil || setting.Value == nil {
			kvSettings[trimmedKey] = setting.Value
			continue
		}

		switch *setting.ContentType {
		case featureFlagContentType:
			continue // ignore feature flag while getting key value settings
		case secretReferenceContentType:
			keyVaultRefs[trimmedKey] = *setting.Value
		default:
			if isJsonContentType(setting.ContentType) {
				var v any
				if err := json.Unmarshal([]byte(*setting.Value), &v); err != nil {
					log.Printf("Failed to unmarshal JSON value: key=%s, error=%s", *setting.Key, err.Error())
					continue
				}
				kvSettings[trimmedKey] = v
			} else {
				kvSettings[trimmedKey] = setting.Value
			}
		}
	}

	var eg errgroup.Group
	resolvedSecrets := sync.Map{}
	if len(keyVaultRefs) > 0 {
		if azappcfg.resolver.credential == nil && azappcfg.resolver.secretResolver == nil {
			return fmt.Errorf("no Key Vault credential or SecretResolver configured")
		}

		for key, kvRef := range keyVaultRefs {
			key, kvRef := key, kvRef
			eg.Go(func() error {
				resolvedSecret, err := azappcfg.resolver.resolveSecret(ctx, kvRef)
				if err != nil {
					return fmt.Errorf("fail to resolve the Key Vault reference '%s': %s", key, err.Error())
				}
				resolvedSecrets.Store(key, resolvedSecret)
				return nil
			})
		}

		if err := eg.Wait(); err != nil {
			return err
		}
	}

	resolvedSecrets.Range(func(key, value interface{}) bool {
		kvSettings[key.(string)] = value.(string)
		return true
	})

	azappcfg.keyValues = kvSettings

	return nil
}

func (azappcfg *AzureAppConfiguration) trimPrefix(key string) string {
	result := key
	for _, prefix := range azappcfg.trimPrefixes {
		if strings.HasPrefix(result, prefix) {
			result = result[len(prefix):]
			break
		}
	}

	return result
}

func isJsonContentType(contentType *string) bool {
	if contentType == nil {
		return false
	}
	contentTypeStr := strings.ToLower(strings.Trim(*contentType, " "))
	matched, _ := regexp.MatchString("^application\\/(?:[^\\/]+\\+)?json(;.*)?$", contentTypeStr)
	return matched
}

func deduplicateSelectors(selectors []Selector) []Selector {
	// If no selectors provided, return the default selector
	if len(selectors) == 0 {
		return []Selector{
			{
				KeyFilter:   wildCard,
				LabelFilter: defaultLabel,
			},
		}
	}

	// Create a map to track unique selectors
	seen := make(map[Selector]struct{})
	var result []Selector

	// Process the selectors in reverse order to maintain the behavior
	// where later duplicates take precedence over earlier ones
	for i := len(selectors) - 1; i >= 0; i-- {
		// Normalize empty label filter
		if selectors[i].LabelFilter == "" {
			selectors[i].LabelFilter = defaultLabel
		}

		// Check if we've seen this selector before
		if _, exists := seen[selectors[i]]; !exists {
			seen[selectors[i]] = struct{}{}
			result = append(result, selectors[i])
		}
	}

	// Reverse the result to maintain the original order
	reverse(result)
	return result
}

// constructHierarchicalMap converts a flat map with delimited keys to a hierarchical structure
func (azappcfg *AzureAppConfiguration) constructHierarchicalMap(separator string) map[string]any {
	tree := &tree.Tree{}
	for k, v := range azappcfg.keyValues {
		tree.Insert(strings.Split(k, separator), v)
	}

	return tree.Build()
}
