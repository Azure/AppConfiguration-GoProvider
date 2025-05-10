// Package azureappconfiguration provides a Go provider for Azure App Configuration service.
//
// This package allows Go applications to easily access configuration settings and feature flags
// stored in Azure App Configuration. It supports authentication via connection string or Azure
// Active Directory, configuration filtering, dynamic refresh, Key Vault reference resolution, and
// feature flag management.
package azureappconfiguration

import (
	"context"
	"sync"
	"time"

	"github.com/Azure/azure-sdk-for-go/sdk/azcore"
	"github.com/Azure/azure-sdk-for-go/sdk/data/azappconfig"
	"github.com/Azure/azure-sdk-for-go/sdk/security/keyvault/azsecrets"
)

const (
	// DefaultRefreshInterval is the default interval for configuration refresh
	DefaultRefreshInterval = 30 * time.Second
)

// AzureAppConfiguration provides access to configuration settings stored in Azure App Configuration.
type AzureAppConfiguration struct {
	client          *azappconfig.Client
	settings        map[string]azappconfig.Setting
	featureFlags    map[string]FeatureFlag
	keyVaultOptions *KeyVaultOptions
	options         *Options
	refreshCallback func()
	lastSyncTime    time.Time
	mutex           sync.RWMutex
	keyVaultCache   map[string]string
	keyVaultCacheMu sync.RWMutex
}

// SecretResolver is an interface to resolve secrets from Key Vault references.
type SecretResolver interface {
	ResolveSecret(ctx context.Context, keyVaultReference string) (string, error)
}

// AuthenticationOptions configures authentication settings for Azure App Configuration.
type AuthenticationOptions struct {
	// Endpoint is the URL of the App Configuration store.
	Endpoint string

	// Credential is the credential used to authenticate with the App Configuration store.
	Credential azcore.TokenCredential

	// ConnectionString is the connection string for the App Configuration store.
	// If provided, Endpoint and Credential are ignored.
	ConnectionString string
}

// Options configures the behavior of the Azure App Configuration provider.
type Options struct {
	// Selectors is a list of key and label filters used to select configurations.
	// If not provided, all key-values with no label are loaded.
	Selectors []Selector

	// TrimKeyPrefixes is a list of prefixes to remove from keys.
	TrimKeyPrefixes []string

	// RefreshOptions configures automatic refresh of key-value settings.
	RefreshOptions *KeyValueRefreshOptions

	// KeyVaultOptions configures Key Vault reference resolution.
	KeyVaultOptions *KeyVaultOptions

	// FeatureFlagOptions configures feature flag support.
	FeatureFlagOptions *FeatureFlagOptions

	// ClientOptions configures the App Configuration client.
	ClientOptions *azcore.ClientOptions
}

// Selector defines filtering criteria for loading configurations.
type Selector struct {
	// KeyFilter filters keys to load. Default is "*" if not provided.
	KeyFilter string

	// LabelFilter filters labels to load.
	LabelFilter string
}

// KeyValueRefreshOptions configures automatic refresh of key-value settings.
type KeyValueRefreshOptions struct {
	// WatchedSettings is a list of settings to watch for changes.
	// If not provided, all selected settings will be watched.
	WatchedSettings []WatchedSetting

	// Interval is the frequency at which to check for changes.
	// Default is 30 seconds if not provided.
	Interval time.Duration

	// Enabled indicates whether automatic refresh is enabled.
	// Default is false if not provided.
	Enabled bool
}

// WatchedSetting identifies a specific key-value setting to watch for changes.
type WatchedSetting struct {
	// Key is the key of the setting to watch.
	Key string

	// Label is the label of the setting to watch.
	Label string
}

// RefreshOptions configures refresh settings.
type RefreshOptions struct {
	// Interval is the frequency at which to check for changes.
	Interval time.Duration

	// Enabled indicates whether automatic refresh is enabled.
	Enabled bool
}

// KeyVaultOptions configures Key Vault reference resolution.
type KeyVaultOptions struct {
	// Credential is the credential used to authenticate with Key Vault.
	Credential azcore.TokenCredential

	// SecretClients is a map of Key Vault URIs to Secret Clients.
	SecretClients map[string]*azsecrets.Client

	// SecretResolver is used to resolve secrets locally without connecting to Key Vault.
	SecretResolver SecretResolver

	// RefreshOptions configures automatic refresh of Key Vault references.
	RefreshOptions *RefreshOptions
}

// FeatureFlag represents a feature flag in Azure App Configuration.
type FeatureFlag struct {
	ID          string                 `json:"id"`
	Description string                 `json:"description"`
	Enabled     bool                   `json:"enabled"`
	Conditions  map[string]interface{} `json:"conditions"`
}

// FeatureFlagOptions configures feature flag support.
type FeatureFlagOptions struct {
	// Enabled indicates whether feature flag support is enabled.
	// Default is false if not provided.
	Enabled bool

	// Selectors is a list of key and label filters used to select feature flags.
	// If not provided, all feature flags with no label are loaded when Enabled is true.
	Selectors []Selector

	// RefreshOptions configures automatic refresh of feature flags.
	RefreshOptions *RefreshOptions
}

// ConstructOptions configures the construction of configuration values.
type ConstructOptions struct {
	// Separator is the character used to flatten nested objects in key names.
	// Default is "." if not provided.
	// Supported values: '.', ',', ';', '-', '_', '__', '/', ':'
	Separator string
}
