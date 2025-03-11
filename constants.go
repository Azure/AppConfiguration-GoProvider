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

// Configuration client constants
const (
	endpointKey string = "Endpoint"
	secretKey   string = "Secret"
	idKey       string = "Id"
)

// General configuration constants
const (
	nullLabel                         = "\x00"
	wildCard                          = "*"
	defaultSeparator                  = "."
	secretReferenceContentType string = "application/vnd.microsoft.appconfig.keyvaultref+json;charset=utf-8"
	featureFlagContentType     string = "application/vnd.microsoft.appconfig.ff+json;charset=utf-8"
)
