// Copyright (c) Microsoft Corporation.
// Licensed under the MIT License.

package azureappconfiguration

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
