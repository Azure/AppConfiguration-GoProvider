// Copyright (c) Microsoft Corporation.
// Licensed under the MIT License.

package azureappconfiguration

import "time"

// Configuration client constants
const (
	endpointKey string = "Endpoint"
	secretKey   string = "Secret"
	idKey       string = "Id"
)

// General configuration constants
const (
	defaultLabel                       = "\x00"
	wildCard                           = "*"
	defaultSeparator                   = "."
	secretReferenceContentType  string = "application/vnd.microsoft.appconfig.keyvaultref+json;charset=utf-8"
	featureFlagContentType      string = "application/vnd.microsoft.appconfig.ff+json;charset=utf-8"
	featureFlagKeyPrefix        string = ".appconfig.featureflag/"
	featureManagementSectionKey string = "feature_management"
	featureFlagSectionKey       string = "feature_flags"
)

// Feature flag constants
const (
	enabledKey              string = "enabled"
	telemetryKey            string = "telemetry"
	metadataKey             string = "metadata"
	nameKey                 string = "name"
	eTagKey                 string = "ETag"
	featureFlagReferenceKey string = "FeatureFlagReference"
	allocationKeyName       string = "allocation"
	defaultWhenEnabledKey   string = "default_when_enabled"
	percentileKeyName       string = "percentile"
	fromKeyName             string = "from"
	toKeyName               string = "to"
	seedKeyName             string = "seed"
	variantKeyName          string = "variant"
	variantsKeyName         string = "variants"
	configurationValueKey   string = "configuration_value"
	allocationIdKeyName     string = "AllocationId"
	conditionsKeyName       string = "conditions"
	clientFiltersKeyName    string = "client_filters"
)

// Refresh interval constants
const (
	// minimalRefreshInterval is the minimum allowed refresh interval for key-value settings
	minimalRefreshInterval time.Duration = time.Second
	// minimalKeyVaultRefreshInterval is the minimum allowed refresh interval for Key Vault references
	minimalKeyVaultRefreshInterval time.Duration = 1 * time.Minute
)

// Failover constants
const (
	tcpKey                                 string        = "tcp"
	originKey                              string        = "origin"
	altKey                                 string        = "alt"
	azConfigDomainLabel                   string        = ".azconfig."
	appConfigDomainLabel                  string        = ".appconfig."
	fallbackClientRefreshExpireInterval   time.Duration = time.Hour
	minimalClientRefreshInterval          time.Duration = time.Second * 30
	maxBackoffDuration                    time.Duration = time.Minute * 10
	minBackoffDuration                    time.Duration = time.Second * 30
	failoverTimeout                       time.Duration = time.Second * 5
	jitterRatio                           float64       = 0.25
	safeShiftLimit                        int           = 63
)
