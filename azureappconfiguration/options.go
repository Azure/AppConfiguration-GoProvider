// Copyright (c) Microsoft Corporation.
// Licensed under the MIT License.

package azureappconfiguration

import (
	"context"
	"net/url"
	"time"

	"github.com/Azure/azure-sdk-for-go/sdk/azcore"
	"github.com/Azure/azure-sdk-for-go/sdk/data/azappconfig"
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

	// ClientOptions provides options for configuring the underlying Azure App Configuration client.
	ClientOptions *azappconfig.ClientOptions
}

// AuthenticationOptions contains parameters for authenticating with the Azure App Configuration service.
// Either a connection string or an endpoint with credential must be provided.
type AuthenticationOptions struct {
	// Credential is a token credential for Azure EntraID Authenticaiton.
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
	// - ctx: The context for the operation
	// - keyVaultReference: A URL in the format "https://{keyVaultName}.vault.azure.net/secrets/{secretName}/{secretVersion}"
	//
	// Returns:
	// - The resolved secret value as a string
	// - An error if the secret could not be resolved
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
}

// ConstructionOptions contains parameters for parsing keys with hierarchical structure.
type ConstructionOptions struct {
	// Separator specifies the character used to determine hierarchy in configuration keys
	// when mapping to nested struct fields during unmarshaling operations.
	// Supported values: '.', ',', ';', '-', '_', '__', '/', ':'.
	// If not provided, the default separator "." will be used.
	Separator string
}
