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
	"maps"
	"os"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"

	"github.com/Azure/AppConfiguration-GoProvider/azureappconfiguration/internal/refresh"
	"github.com/Azure/AppConfiguration-GoProvider/azureappconfiguration/internal/tracing"
	"github.com/Azure/AppConfiguration-GoProvider/azureappconfiguration/internal/tree"
	"github.com/Azure/azure-sdk-for-go/sdk/azcore"
	"github.com/Azure/azure-sdk-for-go/sdk/data/azappconfig"
	decoder "github.com/go-viper/mapstructure/v2"
	"golang.org/x/sync/errgroup"
)

// An AzureAppConfiguration is a configuration provider that stores and manages settings sourced from Azure App Configuration.
type AzureAppConfiguration struct {
	// Settings loaded from Azure App Configuration
	keyValues    map[string]any
	featureFlags map[string]any

	// Settings configured from Options
	kvSelectors     []Selector
	ffEnabled       bool
	ffSelectors     []Selector
	trimPrefixes    []string
	watchedSettings []WatchedSetting

	// Settings used for refresh scenarios
	sentinelETags      map[WatchedSetting]*azcore.ETag
	watchAll           bool
	kvETags            map[Selector][]*azcore.ETag
	ffETags            map[Selector][]*azcore.ETag
	keyVaultRefs       map[string]string // unversioned Key Vault references
	kvRefreshTimer     refresh.Condition
	secretRefreshTimer refresh.Condition
	onRefreshSuccess   []func()
	tracingOptions     tracing.Options

	// Clients talking to Azure App Configuration/Azure Key Vault service
	clientManager *configurationClientManager
	resolver      *keyVaultReferenceResolver

	refreshInProgress atomic.Bool
}

// Load initializes a new AzureAppConfiguration instance and loads the configuration data from
// Azure App Configuration service.
//
// Parameters:
//   - ctx: The context for the operation.
//   - authentication: Authentication options for connecting to the Azure App Configuration service
//   - options: Configuration options to customize behavior, such as key filters and prefix trimming
//
// Returns:
//   - A configured AzureAppConfiguration instance that provides access to the loaded configuration data
//   - An error if the operation fails, such as authentication errors or connectivity issues
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
	azappcfg.featureFlags = make(map[string]any)
	azappcfg.kvSelectors = deduplicateSelectors(options.Selectors)
	azappcfg.ffEnabled = options.FeatureFlagOptions.Enabled

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
		azappcfg.kvETags = make(map[Selector][]*azcore.ETag)
		if len(options.RefreshOptions.WatchedSettings) == 0 {
			azappcfg.watchAll = true
		}
	}

	if options.KeyVaultOptions.RefreshOptions.Enabled {
		azappcfg.secretRefreshTimer = refresh.NewTimer(options.KeyVaultOptions.RefreshOptions.Interval)
		azappcfg.keyVaultRefs = make(map[string]string)
		azappcfg.tracingOptions.KeyVaultRefreshConfigured = true
	}

	if azappcfg.ffEnabled {
		azappcfg.ffSelectors = getFeatureFlagSelectors(deduplicateSelectors(options.FeatureFlagOptions.Selectors))
	}

	if err := azappcfg.load(ctx); err != nil {
		return nil, err
	}
	// Set the initial load finished flag
	azappcfg.tracingOptions.InitialLoadFinished = true

	return azappcfg, nil
}

// Unmarshal parses the configuration and stores the result in the value pointed to v. It builds a hierarchical configuration structure based on key separators.
// It supports converting values to appropriate target types.
//
// Fields in the target struct are matched with configuration keys using the field name by default.
// For custom field mapping, use json struct tags.
//
// Parameters:
//   - v: A pointer to the struct to populate with configuration values
//   - options: Optional parameters (e,g, separator) for controlling the unmarshalling behavior
//
// Returns:
//   - An error if unmarshalling fails due to type conversion issues or invalid configuration
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
//   - options: Optional parameters for controlling JSON construction, particularly the key separator
//
// Returns:
//   - A byte array containing the JSON representation of the configuration
//   - An error if JSON marshalling fails or if an invalid separator is specified
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
//   - Refresh has been configured with RefreshOptions when the client was created
//   - The configured refresh interval has elapsed since the last refresh
//   - No other refresh operation is currently in progress
//
// If the configuration has changed, any callback functions registered with OnRefreshSuccess will be executed.
//
// Parameters:
//   - ctx: The context for the operation.
//
// Returns:
//   - An error if refresh is not configured, or if the refresh operation fails
func (azappcfg *AzureAppConfiguration) Refresh(ctx context.Context) error {
	if azappcfg.kvRefreshTimer == nil && azappcfg.secretRefreshTimer == nil {
		return fmt.Errorf("refresh is not enabled for either key values or Key Vault secrets")
	}

	// Try to set refreshInProgress to true, returning false if it was already true
	if !azappcfg.refreshInProgress.CompareAndSwap(false, true) {
		return nil // Another refresh is already in progress
	}

	// Reset the flag when we're done
	defer azappcfg.refreshInProgress.Store(false)

	// Attempt to refresh and check if any values were actually updated
	keyValueRefreshed, err := azappcfg.refreshKeyValues(ctx, azappcfg.newKeyValueRefreshClient())
	if err != nil {
		return fmt.Errorf("failed to refresh configuration: %w", err)
	}

	// Attempt to reload Key Vault secrets and check if any values were actually updated
	// No need to reload Key Vault secrets if key values are refreshed
	secretRefreshed := false
	if !keyValueRefreshed {
		secretRefreshed, err = azappcfg.refreshKeyVaultSecrets(ctx)
		if err != nil {
			return fmt.Errorf("failed to reload Key Vault secrets: %w", err)
		}
	}

	// Only execute callbacks if actual changes were applied
	if keyValueRefreshed || secretRefreshed {
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
//   - callback: A function with no parameters that will be called after a successful refresh
func (azappcfg *AzureAppConfiguration) OnRefreshSuccess(callback func()) {
	if callback == nil {
		return
	}

	azappcfg.onRefreshSuccess = append(azappcfg.onRefreshSuccess, callback)
}

func (azappcfg *AzureAppConfiguration) load(ctx context.Context) error {
	eg, egCtx := errgroup.WithContext(ctx)
	eg.Go(func() error {
		keyValuesClient := &selectorSettingsClient{
			selectors:      azappcfg.kvSelectors,
			client:         azappcfg.clientManager.staticClient.client,
			tracingOptions: azappcfg.tracingOptions,
		}
		return azappcfg.loadKeyValues(egCtx, keyValuesClient)
	})

	if azappcfg.kvRefreshTimer != nil && len(azappcfg.watchedSettings) > 0 {
		eg.Go(func() error {
			watchedClient := &watchedSettingClient{
				watchedSettings: azappcfg.watchedSettings,
				client:          azappcfg.clientManager.staticClient.client,
				tracingOptions:  azappcfg.tracingOptions,
			}
			return azappcfg.loadWatchedSettings(egCtx, watchedClient)
		})
	}

	if azappcfg.ffEnabled {
		eg.Go(func() error {
			ffClient := &selectorSettingsClient{
				selectors:      azappcfg.ffSelectors,
				client:         azappcfg.clientManager.staticClient.client,
				tracingOptions: azappcfg.tracingOptions,
			}
			return azappcfg.loadFeatureFlags(egCtx, ffClient)
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

	// de-duplicate settings
	rawSettings := make(map[string]azappconfig.Setting, len(settingsResponse.settings))
	for _, setting := range settingsResponse.settings {
		if setting.Key == nil {
			continue
		}
		trimmedKey := azappcfg.trimPrefix(*setting.Key)
		if len(trimmedKey) == 0 {
			log.Printf("Key of the setting '%s' is trimmed to the empty string, just ignore it", *setting.Key)
			continue
		}
		rawSettings[trimmedKey] = setting
	}

	var useAIConfiguration, useAIChatCompletionConfiguration bool
	kvSettings := make(map[string]any, len(settingsResponse.settings))
	keyVaultRefs := make(map[string]string)
	for trimmedKey, setting := range rawSettings {
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

	secrets, err := azappcfg.loadKeyVaultSecrets(ctx, keyVaultRefs)
	if err != nil {
		return fmt.Errorf("failed to load Key Vault secrets: %w", err)
	}

	maps.Copy(kvSettings, secrets)
	azappcfg.keyValues = kvSettings
	azappcfg.keyVaultRefs = getUnversionedKeyVaultRefs(keyVaultRefs)
	azappcfg.kvETags = settingsResponse.pageETags

	return nil
}

func (azappcfg *AzureAppConfiguration) loadKeyVaultSecrets(ctx context.Context, keyVaultRefs map[string]string) (map[string]any, error) {
	secrets := make(map[string]any)
	if len(keyVaultRefs) == 0 {
		return secrets, nil
	}

	if azappcfg.resolver.credential == nil && azappcfg.resolver.secretResolver == nil {
		return secrets, fmt.Errorf("no Key Vault credential or SecretResolver was configured in KeyVaultOptions")
	}

	resolvedSecrets := sync.Map{}
	var eg errgroup.Group
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
		return secrets, fmt.Errorf("failed to resolve Key Vault references: %w", err)
	}

	resolvedSecrets.Range(func(key, value interface{}) bool {
		secrets[key.(string)] = value.(string)
		return true
	})

	return secrets, nil
}

func (azappcfg *AzureAppConfiguration) loadFeatureFlags(ctx context.Context, settingsClient settingsClient) error {
	settingsResponse, err := settingsClient.getSettings(ctx)
	if err != nil {
		return err
	}

	dedupFeatureFlags := make(map[string]any, len(settingsResponse.settings))
	for _, setting := range settingsResponse.settings {
		if setting.Key != nil {
			var v any
			if err := json.Unmarshal([]byte(*setting.Value), &v); err != nil {
				log.Printf("Invalid feature flag setting: key=%s, error=%s, just ignore", *setting.Key, err.Error())
				continue
			}
			dedupFeatureFlags[*setting.Key] = v
		}
	}

	featureFlags := make([]any, 0, len(dedupFeatureFlags))
	for _, v := range dedupFeatureFlags {
		featureFlags = append(featureFlags, v)
	}

	// "feature_management": {"feature_flags": [{...}, {...}]}
	ffSettings := map[string]any{
		featureManagementSectionKey: map[string]any{
			featureFlagSectionKey: featureFlags,
		},
	}

	azappcfg.ffETags = settingsResponse.pageETags
	azappcfg.featureFlags = ffSettings

	return nil
}

// refreshKeyValues checks if any watched settings have changed and reloads configuration if needed
// Returns true if configuration was actually refreshed, false otherwise
func (azappcfg *AzureAppConfiguration) refreshKeyValues(ctx context.Context, refreshClient refreshClient) (bool, error) {
	if azappcfg.kvRefreshTimer == nil ||
		!azappcfg.kvRefreshTimer.ShouldRefresh() {
		// Timer not expired, no need to refresh
		return false, nil
	}

	// Check if any ETags have changed
	eTagChanged, err := refreshClient.monitor.checkIfETagChanged(ctx)
	if err != nil {
		return false, fmt.Errorf("failed to check if key value settings have changed: %w", err)
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

func (azappcfg *AzureAppConfiguration) refreshKeyVaultSecrets(ctx context.Context) (bool, error) {
	if azappcfg.secretRefreshTimer == nil ||
		!azappcfg.secretRefreshTimer.ShouldRefresh() {
		// Timer not expired, no need to refresh
		return false, nil
	}

	if len(azappcfg.keyVaultRefs) == 0 {
		azappcfg.secretRefreshTimer.Reset()
		return false, nil
	}

	unversionedSecrets, err := azappcfg.loadKeyVaultSecrets(ctx, azappcfg.keyVaultRefs)
	if err != nil {
		return false, fmt.Errorf("failed to reload Key Vault secrets: %w", err)
	}

	// Check if any secrets have changed
	changed := false
	keyValues := make(map[string]any)
	maps.Copy(keyValues, azappcfg.keyValues)
	for key, newSecret := range unversionedSecrets {
		if oldSecret, exists := keyValues[key]; !exists || oldSecret != newSecret {
			changed = true
			keyValues[key] = newSecret
		}
	}

	// Reset the timer only after successful refresh
	azappcfg.keyValues = keyValues
	azappcfg.secretRefreshTimer.Reset()
	return changed, nil
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

func getFeatureFlagSelectors(selectors []Selector) []Selector {
	for i := range selectors {
		selectors[i].KeyFilter = featureFlagKeyPrefix + selectors[i].KeyFilter
	}

	return selectors
}

// constructHierarchicalMap converts a flat map with delimited keys to a hierarchical structure
func (azappcfg *AzureAppConfiguration) constructHierarchicalMap(separator string) map[string]any {
	tree := &tree.Tree{}
	for k, v := range azappcfg.keyValues {
		tree.Insert(strings.Split(k, separator), v)
	}

	constructedMap := tree.Build()
	if azappcfg.ffEnabled {
		maps.Copy(constructedMap, azappcfg.featureFlags)
	}

	return constructedMap
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
	var monitor eTagsClient
	if azappcfg.watchAll {
		monitor = &pageETagsClient{
			client:         azappcfg.clientManager.staticClient.client,
			tracingOptions: azappcfg.tracingOptions,
			pageETags:      azappcfg.kvETags,
		}
	} else {
		monitor = &watchedSettingClient{
			client:         azappcfg.clientManager.staticClient.client,
			tracingOptions: azappcfg.tracingOptions,
			eTags:          azappcfg.sentinelETags,
		}
	}

	return refreshClient{
		loader: &selectorSettingsClient{
			selectors:      azappcfg.kvSelectors,
			client:         azappcfg.clientManager.staticClient.client,
			tracingOptions: azappcfg.tracingOptions,
		},
		monitor: monitor,
		sentinels: &watchedSettingClient{
			watchedSettings: azappcfg.watchedSettings,
			client:          azappcfg.clientManager.staticClient.client,
			tracingOptions:  azappcfg.tracingOptions,
		},
	}
}
