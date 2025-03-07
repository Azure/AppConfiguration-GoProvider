// Copyright (c) Microsoft Corporation.
// Licensed under the MIT License.

package azureappconfiguration

import "time"

// Refresh interval constants
const (
	// MinimalRefreshInterval is the minimum allowed refresh interval for key-value settings
	MinimalRefreshInterval time.Duration = time.Second

	// KeyVaultMinimalRefreshInterval is the minimum allowed refresh interval for Key Vault references
	KeyVaultMinimalRefreshInterval time.Duration = time.Minute

	// DefaultRefreshInterval is the default interval used when no interval is specified
	DefaultRefreshInterval time.Duration = 30 * time.Second
)

// Feature flag constants
const (
	// FeatureFlagPrefixKey is the prefix used to identify feature flag settings
	FeatureFlagPrefixKey = ".appconfig.featureflag/"

	// FeatureFlagSectionKey is the section name used in the feature flag configuration
	FeatureFlagSectionKey = "feature_flags"

	// FeatureManagementSectionKey is the top-level section name for feature management
	FeatureManagementSectionKey = "feature_management"
)

// General configuration constants
const (
	// NullLabel represents an empty label in the configuration
	NullLabel = "\x00"

	// WildCard is used in selectors to match any character sequence
	WildCard = "*"

	// DefaultSeparator is the default character used to separate nested configuration paths
	DefaultSeparator = "."
)
