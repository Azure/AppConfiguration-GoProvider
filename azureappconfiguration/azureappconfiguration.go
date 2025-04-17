// Copyright (c) Microsoft Corporation.
// Licensed under the MIT License.

// Package azureappconfiguration provides a client for Azure App Configuration, enabling Go applications
// to manage application settings with Microsoft's Azure App Configuration service.
//
// The azureappconfiguration package allows loading configuration data from Azure App Configuration in a structured way,
// with support for automatic Key Vault reference resolution, hierarchical configuration construction,
// and strongly typed configuration binding.
//
// For more information about Azure App Configuration, see:
// https://learn.microsoft.com/en-us/azure/azure-app-configuration/
package azureappconfiguration

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"os"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"

	"github.com/Azure/AppConfiguration-GoProvider/azureappconfiguration/internal/tracing"
	"github.com/Azure/AppConfiguration-GoProvider/azureappconfiguration/internal/refresh"
	"github.com/Azure/AppConfiguration-GoProvider/azureappconfiguration/internal/tree"
	"github.com/Azure/azure-sdk-for-go/sdk/azcore"
	decoder "github.com/go-viper/mapstructure/v2"
	"golang.org/x/sync/errgroup"
)

// An AzureAppConfiguration is a configuration provider that stores and manages settings sourced from Azure App Configuration.
type AzureAppConfiguration struct {
	keyValues       map[string]any
	kvSelectors     []Selector
	trimPrefixes    []string
	watchedSettings []WatchedSetting

	sentinelETags    map[WatchedSetting]*azcore.ETag
	kvRefreshTimer   refresh.Condition
	onRefreshSuccess []func()
	tracingOptions tracing.Options

	clientManager      *configurationClientManager
	resolver           *keyVaultReferenceResolver
	newKvRefreshClient func() refreshClient

	refreshInProgress atomic.Bool
}

// Load initializes a new AzureAppConfiguration instance and loads the configuration data from
// Azure App Configuration service.
//
// Parameters:
// - ctx: The context for the operation.
// - authentication: Authentication options for connecting to the Azure App Configuration service
// - options: Configuration options to customize behavior, such as key filters and prefix trimming
//
// Returns:
// - A configured AzureAppConfiguration instance that provides access to the loaded configuration data
// - An error if the operation fails, such as authentication errors or connectivity issues
func Load(ctx context.Context, authentication AuthenticationOptions, options *Options) (*AzureAppConfiguration, error) {
	if err := verifyAuthenticationOptions(authentication); err != nil {
		return nil, err
	}

	if err := verifyOptions(options); err != nil {
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
	azappcfg.tracingOptions = configureTracingOptions(options)
	azappcfg.keyValues = make(map[string]any)
	azappcfg.kvSelectors = deduplicateSelectors(options.Selectors)
	azappcfg.trimPrefixes = options.TrimKeyPrefixes
	azappcfg.clientManager = clientManager
	azappcfg.resolver = &keyVaultReferenceResolver{
		clients:        sync.Map{},
		secretResolver: options.KeyVaultOptions.SecretResolver,
		credential:     options.KeyVaultOptions.Credential,
	}

	if options.RefreshOptions.Enabled {
		azappcfg.kvRefreshTimer = refresh.NewTimer(options.RefreshOptions.Interval)
		azappcfg.watchedSettings = normalizedWatchedSettings(options.RefreshOptions.WatchedSettings)
		azappcfg.sentinelETags = make(map[WatchedSetting]*azcore.ETag)
		azappcfg.newKvRefreshClient = azappcfg.newKeyValueRefreshClient
	}

	if err := azappcfg.load(ctx); err != nil {
		return nil, err
	}

	return azappcfg, nil
}

// Unmarshal parses the configuration and stores the result in the value pointed to v. It builds a hierarchical configuration structure based on key separators.
// It supports converting values to appropriate target types.
//
// Fields in the target struct are matched with configuration keys using the field name by default.
// For custom field mapping, use json struct tags.
//
// Parameters:
// - v: A pointer to the struct to populate with configuration values
// - options: Optional parameters (e,g, separator) for controlling the unmarshalling behavior
//
// Returns:
// - An error if unmarshalling fails due to type conversion issues or invalid configuration
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

// GetBytes returns the configuration as a JSON byte array with hierarchical structure.
// This method is particularly useful for integrating with "encoding/json" package or third-party configuration packages like Viper or Koanf.
//
// Parameters:
// - options: Optional parameters for controlling JSON construction, particularly the key separator
//
// Returns:
// - A byte array containing the JSON representation of the configuration
// - An error if JSON marshalling fails or if an invalid separator is specified
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

// Refresh manually triggers a refresh of the configuration from Azure App Configuration.
// It checks if any watched settings have changed, and if so, reloads all configuration data.
//
// The refresh only occurs if:
// - Refresh has been configured with RefreshOptions when the client was created
// - The configured refresh interval has elapsed since the last refresh
// - No other refresh operation is currently in progress
//
// If the configuration has changed, any callback functions registered with OnRefreshSuccess will be executed.
//
// Parameters:
// - ctx: The context for the operation.
//
// Returns:
// - An error if refresh is not configured, or if the refresh operation fails
func (azappcfg *AzureAppConfiguration) Refresh(ctx context.Context) error {
	if azappcfg.kvRefreshTimer == nil {
		return fmt.Errorf("refresh is not configured")
	}

	// Try to set refreshInProgress to true, returning false if it was already true
	if !azappcfg.refreshInProgress.CompareAndSwap(false, true) {
		return nil // Another refresh is already in progress
	}

	// Reset the flag when we're done
	defer azappcfg.refreshInProgress.Store(false)

	// Check if it's time to perform a refresh based on the timer interval
	if !azappcfg.kvRefreshTimer.ShouldRefresh() {
		return nil
	}

	// Attempt to refresh and check if any values were actually updated
	refreshed, err := azappcfg.refreshKeyValues(ctx, azappcfg.newKvRefreshClient())
	if err != nil {
		return fmt.Errorf("failed to refresh configuration: %w", err)
	}

	// Only execute callbacks if actual changes were applied
	if refreshed {
		for _, callback := range azappcfg.onRefreshSuccess {
			if callback != nil {
				callback()
			}
		}
	}

	return nil
}

// OnRefreshSuccess registers a callback function that will be executed whenever the configuration
// is successfully refreshed and actual changes were detected.
//
// Multiple callback functions can be registered, and they will be executed in the order they were added.
// Callbacks are only executed when configuration values actually change. They run synchronously
// in the thread that initiated the refresh.
//
// Parameters:
// - callback: A function with no parameters that will be called after a successful refresh
func (azappcfg *AzureAppConfiguration) OnRefreshSuccess(callback func()) {
	azappcfg.onRefreshSuccess = append(azappcfg.onRefreshSuccess, callback)
}

func (azappcfg *AzureAppConfiguration) load(ctx context.Context) error {
	eg, egCtx := errgroup.WithContext(ctx)
	eg.Go(func() error {
		keyValuesClient := &selectorSettingsClient{
			selectors: azappcfg.kvSelectors,
			client:    azappcfg.clientManager.staticClient.client,
			tracingOptions: azappcfg.tracingOptions,
		}
		return azappcfg.loadKeyValues(egCtx, keyValuesClient)
	})

	if azappcfg.kvRefreshTimer != nil && len(azappcfg.watchedSettings) > 0 {
		eg.Go(func() error {
			watchedClient := &watchedSettingClient{
				watchedSettings: azappcfg.watchedSettings,
				client:          azappcfg.clientManager.staticClient.client,
				tracingOptions: azappcfg.tracingOptions,
			}
			return azappcfg.loadWatchedSettings(egCtx, watchedClient)
		})
	}

	return eg.Wait()
}

func (azappcfg *AzureAppConfiguration) loadWatchedSettings(ctx context.Context, settingsClient settingsClient) error {
	settingsResponse, err := settingsClient.getSettings(ctx)
	if err != nil {
		return err
	}

	// Store ETags for all watched settings
	if settingsResponse != nil && settingsResponse.watchedETags != nil {
		azappcfg.sentinelETags = settingsResponse.watchedETags
	}

	return nil
}

func (azappcfg *AzureAppConfiguration) loadKeyValues(ctx context.Context, settingsClient settingsClient) error {
	settingsResponse, err := settingsClient.getSettings(ctx)
	if err != nil {
		return err
	}

	var useAIConfiguration, useAIChatCompletionConfiguration bool
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

		switch strings.TrimSpace(strings.ToLower(*setting.ContentType)) {
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
				if isAIConfigurationContentType(setting.ContentType) {
					useAIConfiguration = true
				}
				if isAIChatCompletionContentType(setting.ContentType) {
					useAIChatCompletionConfiguration = true
				}
			} else {
				kvSettings[trimmedKey] = setting.Value
			}
		}
	}

	azappcfg.tracingOptions.UseAIConfiguration = useAIConfiguration
	azappcfg.tracingOptions.UseAIChatCompletionConfiguration = useAIChatCompletionConfiguration

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

// refreshKeyValues checks if any watched settings have changed and reloads configuration if needed
// Returns true if configuration was actually refreshed, false otherwise
func (azappcfg *AzureAppConfiguration) refreshKeyValues(ctx context.Context, refreshClient refreshClient) (bool, error) {
	// Check if any ETags have changed
	eTagChanged, err := refreshClient.monitor.checkIfETagChanged(ctx)
	if err != nil {
		return false, fmt.Errorf("failed to check if watched settings have changed: %w", err)
	}

	if !eTagChanged {
		// No changes detected, reset timer and return
		azappcfg.kvRefreshTimer.Reset()
		return false, nil
	}

	// Use an errgroup to reload key values and watched settings concurrently
	eg, egCtx := errgroup.WithContext(ctx)

	// Reload key values in one goroutine
	eg.Go(func() error {
		settingsClient := refreshClient.loader
		return azappcfg.loadKeyValues(egCtx, settingsClient)
	})

	if len(azappcfg.watchedSettings) > 0 {
		eg.Go(func() error {
			watchedClient := refreshClient.sentinels
			return azappcfg.loadWatchedSettings(egCtx, watchedClient)
		})
	}

	// Wait for all reloads to complete
	if err := eg.Wait(); err != nil {
		// Don't reset the timer if reload failed
		return false, fmt.Errorf("failed to reload configuration: %w", err)
	}

	// Reset the timer only after successful refresh
	azappcfg.kvRefreshTimer.Reset()
	return true, nil
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

func configureTracingOptions(options *Options) tracing.Options {
	tracingOption := tracing.Options{
		Enabled: true,
	}

	if value, exist := os.LookupEnv(tracing.EnvVarTracingDisabled); exist {
		tracingDisabled, _ := strconv.ParseBool(value)
		if tracingDisabled {
			tracingOption.Enabled = false
			return tracingOption
		}
	}

	tracingOption.Host = tracing.GetHostType()

	if !(options.KeyVaultOptions.SecretResolver == nil && options.KeyVaultOptions.Credential == nil) {
		tracingOption.KeyVaultConfigured = true
	}

	return tracingOption
}

func normalizedWatchedSettings(s []WatchedSetting) []WatchedSetting {
	result := make([]WatchedSetting, len(s))
	for i, setting := range s {
		// Make a copy of the setting
		normalizedSetting := setting
		if normalizedSetting.Label == "" {
			normalizedSetting.Label = defaultLabel
		}

		result[i] = normalizedSetting
	}

	return result
}

func (azappcfg *AzureAppConfiguration) newKeyValueRefreshClient() refreshClient {
	return refreshClient{
		loader: &selectorSettingsClient{
			selectors: azappcfg.kvSelectors,
			client:    azappcfg.clientManager.staticClient.client,
		},
		monitor: &watchedSettingClient{
			eTags:  azappcfg.sentinelETags,
			client: azappcfg.clientManager.staticClient.client,
		},
		sentinels: &watchedSettingClient{
			watchedSettings: azappcfg.watchedSettings,
			client:          azappcfg.clientManager.staticClient.client,
		},
	 }
}
