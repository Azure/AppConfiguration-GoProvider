// Copyright (c) Microsoft Corporation.
// Licensed under the MIT License.

package azureappconfiguration

import "time"

// Refresh interval constants
const (
	// minimalRefreshInterval is the minimum allowed refresh interval for key-value settings
	minimalRefreshInterval time.Duration = time.Second

	// keyVaultMinimalRefreshInterval is the minimum allowed refresh interval for Key Vault references
	keyVaultMinimalRefreshInterval time.Duration = time.Minute

	// defaultRefreshInterval is the default interval used when no interval is specified
	defaultRefreshInterval time.Duration = 30 * time.Second
)

// Feature flag constants
const (
	// featureFlagPrefixKey is the prefix used to identify feature flag settings
	featureFlagPrefixKey = ".appconfig.featureflag/"

	// featureFlagSectionKey is the section name used in the feature flag configuration
	featureFlagSectionKey = "feature_flags"

	// featureManagementSectionKey is the top-level section name for feature management
	featureManagementSectionKey = "feature_management"
)

// Configuration client constants
const (
	endpointKey                string        = "Endpoint"
	secretKey                  string        = "Secret"
	idKey                      string        = "Id"
	maxBackoffDuration         time.Duration = time.Minute * 10
	minBackoffDuration         time.Duration = time.Second * 30
	jitterRatio                float64       = 0.25
	safeShiftLimit             int           = 63
)

// General configuration constants
const (
	// nullLabel represents an empty label in the configuration
	nullLabel = "\x00"

	// wildCard is used in selectors to match any character sequence
	wildCard = "*"

	// defaultSeparator is the default character used to separate nested configuration paths
	defaultSeparator = "."

	secretReferenceContentType string        = "application/vnd.microsoft.appconfig.keyvaultref+json;charset=utf-8"
	featureFlagContentType     string        = "application/vnd.microsoft.appconfig.ff+json;charset=utf-8"
)
