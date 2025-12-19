// Copyright (c) Microsoft Corporation.
// Licensed under the MIT License.

package azureappconfiguration

import (
	"context"
	"encoding/json"
	"net/url"
	"sort"
	"time"

	"github.com/Azure/azure-sdk-for-go/sdk/azcore"
	"github.com/Azure/azure-sdk-for-go/sdk/data/azappconfig/v2"
)

// Options contains optional parameters to configure the behavior of an Azure App Configuration provider.
// It provides control over which key-values to fetch, how to trim key prefixes, and how to handle Key Vault references.
type Options struct {
	// TrimKeyPrefixes specifies a list of prefixes to trim from the keys of all key-values
	// retrieved from Azure App Configuration, making them more suitable for binding to structured types.
	TrimKeyPrefixes []string

	// Selectors defines what key-values to load from Azure App Configuration
	// Each selector combines a key filter and label filter
	// If selectors are not provided, all key-values with no label are loaded by default.
	Selectors []Selector

	// RefreshOptions contains optional parameters to configure the behavior of key-value settings refresh
	RefreshOptions KeyValueRefreshOptions

	// KeyVaultOptions configures how Key Vault references are resolved.
	KeyVaultOptions KeyVaultOptions

	// FeatureFlagOptions contains optional parameters for Azure App Configuration feature flags.
	FeatureFlagOptions FeatureFlagOptions

	// ClientOptions provides options for configuring the underlying Azure App Configuration client.
	ClientOptions *azappconfig.ClientOptions

	// ReplicaDiscoveryEnabled specifies whether to enable replica discovery for the Azure App Configuration service.
	// It defaults to true, which allows the provider to discover and use replicas for improved availability.
	ReplicaDiscoveryEnabled *bool

	// LoadBalancingEnabled specifies whether to enable load balancing across multiple replicas of the Azure App Configuration service.
	// It defaults to false.
	LoadBalancingEnabled bool

	// StartupOptions is used when initially loading data into the configuration provider.
	StartupOptions StartupOptions
}

// AuthenticationOptions contains parameters for authenticating with the Azure App Configuration service.
// Either a connection string or an endpoint with credential must be provided.
type AuthenticationOptions struct {
	// Credential is a token credential for Azure EntraID Authentication.
	// Required when Endpoint is provided.
	Credential azcore.TokenCredential

	// Endpoint is the URL of the Azure App Configuration service.
	// Required when using token-based authentication with Credential.
	Endpoint string

	// ConnectionString is the connection string for the Azure App Configuration service.
	ConnectionString string
}

// Selector specifies what key-values to load from Azure App Configuration.
type Selector struct {
	// KeyFilter specifies which keys to retrieve from Azure App Configuration.
	// It can include wildcards, e.g. "app*" will match all keys starting with "app".
	KeyFilter string

	// LabelFilter specifies which labels to retrieve from Azure App Configuration.
	// Empty string or omitted value will use the default no-label filter.
	// Note: Wildcards are not supported in label filters.
	LabelFilter string

	// Snapshot is a set of key-values selected from the App Configuration store based on the composition type and filters.
	// Once created, it is stored as an immutable entity that can be referenced by name.
	// SnapshotName specifies the name of the snapshot to retrieve.
	// If SnapshotName is used in a selector, no key and label filter should be used for it. Otherwise, an error will be returned.
	SnapshotName string

	// TagFilters specifies which tags to retrieve from Azure App Configuration.
	// Each tag filter must follow the format "tagName=tagValue". Only those key-values will be loaded whose tags match all the tags provided here.
	// Up to 5 tag filters can be provided. If no tag filters are provided, key-values will not be filtered based on tags.
	TagFilters []string
}

// comparableKey returns a comparable representation of the Selector that can be used as a map key.
// This method creates a deterministic string representation by sorting the TagFilter slice.
func (s Selector) comparableKey() comparableSelector {
	cs := comparableSelector{
		KeyFilter:    s.KeyFilter,
		LabelFilter:  s.LabelFilter,
		SnapshotName: s.SnapshotName,
	}

	if len(s.TagFilters) > 0 {
		// Deduplicate TagFilter
		unique := make(map[string]struct{}, len(s.TagFilters))
		var tagFilter []string
		for _, tag := range s.TagFilters {
			if _, exists := unique[tag]; !exists {
				unique[tag] = struct{}{}
				tagFilter = append(tagFilter, tag)
			}
		}
		sort.Strings(tagFilter)

		// Use JSON encoding for robust serialization that handles all special characters
		tagFilterJSON, _ := json.Marshal(tagFilter) // Marshal of []string should never fail
		cs.TagFilters = string(tagFilterJSON)
	}

	return cs
}

// comparableSelector is a comparable version of Selector that can be used as a map key.
// It represents the same selector information but with TagFilter as a sorted, JSON-encoded string.
type comparableSelector struct {
	KeyFilter    string
	LabelFilter  string
	SnapshotName string
	TagFilters   string // Sorted, JSON-encoded representation of the original TagFilter slice
}

// KeyValueRefreshOptions contains optional parameters to configure the behavior of key-value settings refresh
type KeyValueRefreshOptions struct {
	// WatchedSettings specifies the key-value settings to watch for changes
	WatchedSettings []WatchedSetting

	// Interval specifies the minimum time interval between consecutive refresh operations for the watched settings
	// Must be greater than 1 second. If not provided, the default interval 30 seconds will be used
	Interval time.Duration

	// Enabled specifies whether the provider should automatically refresh when the configuration is changed.
	Enabled bool
}

// WatchedSetting specifies the key and label of a key-value setting to watch for changes
type WatchedSetting struct {
	Key   string
	Label string
}

// SecretResolver is an interface to resolve secrets from Key Vault references.
// Implement this interface to provide custom secret resolution logic.
type SecretResolver interface {
	// ResolveSecret resolves a Key Vault reference URL to the actual secret value.
	//
	// Parameters:
	//   - ctx: The context for the operation
	//   - keyVaultReference: A URL in the format "https://{keyVaultName}.vault.azure.net/secrets/{secretName}/{secretVersion}"
	//
	// Returns:
	//   - The resolved secret value as a string
	//   - An error if the secret could not be resolved
	ResolveSecret(ctx context.Context, keyVaultReference url.URL) (string, error)
}

// KeyVaultOptions contains parameters to configure the build-in Key Vault reference resolution.
// These options determine how the provider will authenticate with and retrieve
type KeyVaultOptions struct {
	// Credential specifies the token credential used to authenticate to Azure Key Vault services.
	// This is required for Key Vault reference resolution unless a custom SecretResolver is provided.
	Credential azcore.TokenCredential

	// SecretResolver specifies a custom implementation for resolving Key Vault references.
	// When provided, this takes precedence over using the default resolver with Credential.
	SecretResolver SecretResolver

	// RefreshOptions specifies the behavior of Key Vault secrets refresh.
	// Sets the refresh interval for periodically reloading secrets from Key Vault, must be greater than 1 minute.
	RefreshOptions RefreshOptions
}

// FeatureFlagOptions contains optional parameters for Azure App Configuration feature flags that will be parsed and transformed into feature management configuration.
type FeatureFlagOptions struct {
	// Enabled specifies whether feature flags will be loaded from Azure App Configuration.
	Enabled bool

	// If no selectors are provided, all feature flags with no label will be loaded when enabled feature flags.
	Selectors []Selector

	// RefreshOptions specifies the behavior of feature flags refresh.
	// Refresh interval must be greater than 1 second. If not provided, the default interval 30 seconds will be used
	// All loaded feature flags will be automatically watched when feature flags refresh is enabled.
	RefreshOptions RefreshOptions
}

// RefreshOptions contains optional parameters to configure the behavior of refresh
type RefreshOptions struct {
	// Interval specifies the minimum time interval between consecutive refresh operations
	Interval time.Duration

	// Enabled specifies whether the provider should automatically refresh when data is changed.
	Enabled bool
}

// ConstructionOptions contains parameters for parsing keys with hierarchical structure.
type ConstructionOptions struct {
	// Separator specifies the character used to determine hierarchy in configuration keys
	// when mapping to nested struct fields during unmarshaling operations.
	// Supported values: '.', ',', ';', '-', '_', '__', '/', ':'.
	// If not provided, the default separator "." will be used.
	Separator string
}

// StartupOptions is used when initially loading data into the configuration provider.
type StartupOptions struct {
	// Timeout specifies the amount of time allowed to load data from Azure App Configuration on startup.
	Timeout time.Duration
}
