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
	defaultLabel                      = "\x00"
	wildCard                          = "*"
	defaultSeparator                  = "."
	secretReferenceContentType string = "application/vnd.microsoft.appconfig.keyvaultref+json;charset=utf-8"
	featureFlagContentType     string = "application/vnd.microsoft.appconfig.ff+json;charset=utf-8"
)

// Refresh interval constants
const (
	// minimalRefreshInterval is the minimum allowed refresh interval for key-value settings
	minimalRefreshInterval       time.Duration = time.Second
	minimalSecretRefreshInterval time.Duration = 1 * time.Minute
)
