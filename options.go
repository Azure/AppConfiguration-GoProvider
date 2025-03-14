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
// If selectors are not provided, all key-values with no label are loaded.
type Options struct {
	// Trims the provided prefixes from the keys of all key-values retrieved from Azure App Configuration.
	TrimKeyPrefixes []string
	Selectors       []Selector
	RefreshOptions  KeyValueRefreshOptions
	KeyVaultOptions KeyVaultOptions
	ClientOptions   *azappconfig.ClientOptions
}

// AuthenticationOptions contains optional parameters to construct an Azure App Configuration client
// ConnectionString or endpoint with credential must be be provided
type AuthenticationOptions struct {
	Credential       azcore.TokenCredential
	Endpoint         string
	ConnectionString string
}

// Selector specifies what key-values to include in the configuration provider.
type Selector struct {
	KeyFilter   string
	LabelFilter string
}

// KeyValueRefreshOptions contains optional parameters to configure the behavior of key-value settings refresh
type KeyValueRefreshOptions struct {
	// WatchedSettings specifies the key-value settings to watch for changes
	// If not provided, all selected key-value settings will be watched
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

// SecretResolver is an interface to resolve secret from key vault reference
type SecretResolver interface {
	// keyVaultReference: "https://{keyVaultName}.vault.azure.net/secrets/{secretName}/{secretVersion}"
	ResolveSecret(ctx context.Context, keyVaultReference url.URL) (string, error)
}

// KeyVaultOptions contains optional parameters to configure the behavior of key vault reference resolution
type KeyVaultOptions struct {
	// Credential specifies the token credential used to authenticate to key vaults
	Credential azcore.TokenCredential

	// SecretResolver specifies the callback used to resolve key vault references
	SecretResolver SecretResolver
}

// ConstructionOptions contains optional parameters for Unmarshal and GetBytes methods
type ConstructionOptions struct {
	// Separator is used to unmarshal configuration when the keys themselves contain the separator
	// Supported values: '.', ',', ';', '-', '_', '__', '/', ':'.
	// If not provided, the default separator "." will be used
	Separator string
}
