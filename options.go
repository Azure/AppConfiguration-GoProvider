// Copyright (c) Microsoft Corporation.
// Licensed under the MIT License.

package azureappconfiguration

import (
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
	FeatureFlagOptions FeatureFlagOptions
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

// equal compares two Selectors for equality
func (s Selector) equal(other Selector) bool {
	return s.KeyFilter == other.KeyFilter && s.LabelFilter == other.LabelFilter
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

// FeatureFlagOptions contains optional parameters for Azure App Configuration feature flags that will be parsed and transformed into feature management configuration.
type FeatureFlagOptions struct {
	// Enabled specifies whether feature flags will be loaded from Azure App Configuration.
	Enabled        bool

	// If no selectors are provided, all feature flags with no label will be loaded when enabled feature flags.
	Selectors      []Selector

	// RefreshOptions specifies the behavior of feature flags refresh.
	// Refresh interval must be greater than 1 second. If not provided, the default interval 30 seconds will be used
	// All loaded feature flags will be automatically watched when feature flags refresh is enabled.
	RefreshOptions RefreshOptions
}

// RefreshOptions contains optional parameters to configure the behavior of refresh
type RefreshOptions struct {
	Interval time.Duration
	Enabled  bool
}
