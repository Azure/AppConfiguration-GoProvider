// Copyright (c) Microsoft Corporation.
// Licensed under the MIT License.

package azureappconfiguration

import (
	"fmt"
	"strings"

	"github.com/Azure/azure-sdk-for-go/sdk/azcore"
	"github.com/Azure/azure-sdk-for-go/sdk/azcore/policy"
	"github.com/Azure/azure-sdk-for-go/sdk/data/azappconfig"
)

// configurationClientManager handles creation and management of app configuration clients
type configurationClientManager struct {
	clientOptions *azappconfig.ClientOptions
	staticClient  *configurationClientWrapper
	endpoint      string
	credential    azcore.TokenCredential
	secret        string
	id            string
}

// configurationClientWrapper wraps an Azure App Configuration client with additional metadata
type configurationClientWrapper struct {
	endpoint string
	client   *azappconfig.Client
}

// newConfigurationClientManager creates a new configuration client manager
func newConfigurationClientManager(authOptions AuthenticationOptions, clientOptions *azappconfig.ClientOptions) (*configurationClientManager, error) {
	manager := &configurationClientManager{
		clientOptions: setTelemetry(clientOptions),
	}

	// Create client based on authentication options
	if err := manager.initializeClient(authOptions); err != nil {
		return nil, fmt.Errorf("failed to initialize configuration client: %w", err)
	}

	return manager, nil
}

// initializeClient sets up the Azure App Configuration client based on the provided authentication options
func (manager *configurationClientManager) initializeClient(authOptions AuthenticationOptions) error {
	var err error
	var staticClient *azappconfig.Client

	if authOptions.ConnectionString != "" {
		// Initialize using connection string
		connectionString := authOptions.ConnectionString

		if manager.endpoint, err = parseConnectionString(connectionString, endpointKey); err != nil {
			return err
		}

		if manager.secret, err = parseConnectionString(connectionString, secretKey); err != nil {
			return err
		}

		if manager.id, err = parseConnectionString(connectionString, idKey); err != nil {
			return err
		}

		if staticClient, err = azappconfig.NewClientFromConnectionString(connectionString, manager.clientOptions); err != nil {
			return err
		}
	} else {
		// Initialize using explicit endpoint and credential
		if staticClient, err = azappconfig.NewClient(authOptions.Endpoint, authOptions.Credential, manager.clientOptions); err != nil {
			return err
		}
		manager.endpoint = authOptions.Endpoint
		manager.credential = authOptions.Credential
	}

	// Initialize the static client wrapper
	manager.staticClient = &configurationClientWrapper{
		endpoint: manager.endpoint,
		client:   staticClient,
	}

	return nil
}

// parseConnectionString extracts a named value from a connection string
func parseConnectionString(connectionString string, token string) (string, error) {
	if connectionString == "" {
		return "", fmt.Errorf("connectionString cannot be empty")
	}

	parseToken := token + "="
	startIndex := strings.Index(connectionString, parseToken)
	if startIndex < 0 {
		return "", fmt.Errorf("missing %s in connection string", token)
	}

	// Move past the token=
	startIndex += len(parseToken)

	// Find the end of this value (either ; or end of string)
	endIndex := strings.Index(connectionString[startIndex:], ";")
	if endIndex < 0 {
		// No semicolon found, use the rest of the string
		return connectionString[startIndex:], nil
	}

	// Adjust endIndex to be relative to the original string
	endIndex += startIndex

	return connectionString[startIndex:endIndex], nil
}

func setTelemetry(options *azappconfig.ClientOptions) *azappconfig.ClientOptions {
	if options == nil {
		options = &azappconfig.ClientOptions{}
	}

	if options.Telemetry.Disabled == false && options.Telemetry.ApplicationID == "" {
		options.Telemetry = policy.TelemetryOptions{
			ApplicationID: fmt.Sprintf("%s/%s", moduleName, moduleVersion),
		}
	}

	return options
}
