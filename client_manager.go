// Copyright (c) Microsoft Corporation.
// Licensed under the MIT License.

package azureappconfiguration

import (
	"fmt"
	"math"
	"math/rand/v2"
	"strings"
	"time"

	"github.com/Azure/azure-sdk-for-go/sdk/azcore"
	"github.com/Azure/azure-sdk-for-go/sdk/data/azappconfig"
)

// Configuration client constants
const (
	EndpointKey                string        = "Endpoint"
	SecretKey                  string        = "Secret"
	IdKey                      string        = "Id"
	MaxBackoffDuration         time.Duration = time.Minute * 10
	MinBackoffDuration         time.Duration = time.Second * 30
	JitterRatio                float64       = 0.25
	SafeShiftLimit             int           = 63
	SecretReferenceContentType string        = "application/vnd.microsoft.appconfig.keyvaultref+json;charset=utf-8"
	FeatureFlagContentType     string        = "application/vnd.microsoft.appconfig.ff+json;charset=utf-8"
)

// configurationClientManager handles creation and management of app configuration clients
type configurationClientManager struct {
	clientOptions *azappconfig.ClientOptions
	staticClient *configurationClientWrapper
	endpoint      string
	credential    azcore.TokenCredential
	secret        string
	id            string
}

// configurationClientWrapper wraps an Azure App Configuration client with additional metadata
type configurationClientWrapper struct {
	Endpoint       string
	Client         *azappconfig.Client
	BackOffEndTime time.Time
	FailedAttempts int
}

// newConfigurationClientManager creates a new configuration client manager
func newConfigurationClientManager(authOptions AuthenticationOptions, clientOptions *azappconfig.ClientOptions) (*configurationClientManager, error) {
	manager := &configurationClientManager{
		clientOptions: clientOptions,
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

		if manager.endpoint, err = parseConnectionString(connectionString, EndpointKey); err != nil {
			return err
		}

		if manager.secret, err = parseConnectionString(connectionString, SecretKey); err != nil {
			return err
		}

		if manager.id, err = parseConnectionString(connectionString, IdKey); err != nil {
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
		Endpoint: manager.endpoint,
		Client:   staticClient,
	}

	return nil
}

// getClients returns the available configuration clients
func (manager *configurationClientManager) getClients() []*configurationClientWrapper {
	return []*configurationClientWrapper{manager.staticClient}
}

// calculateBackoffDuration calculates the exponential backoff duration with jitter
func calculateBackoffDuration(failedAttempts int) time.Duration {
	if failedAttempts <= 1 {
		return MinBackoffDuration
	}

	// Calculate exponential backoff with safety limits
	minDurationMs := float64(MinBackoffDuration.Milliseconds())
	calculatedMilliseconds := math.Max(1, minDurationMs) *
		math.Pow(2, math.Min(float64(failedAttempts-1), float64(SafeShiftLimit)))

	maxDurationMs := float64(MaxBackoffDuration.Milliseconds())
	if calculatedMilliseconds > maxDurationMs || calculatedMilliseconds <= 0 {
		calculatedMilliseconds = maxDurationMs
	}

	calculatedDuration := time.Duration(calculatedMilliseconds) * time.Millisecond
	return addJitter(calculatedDuration)
}

// addJitter adds random jitter to the duration to avoid thundering herd problem
func addJitter(duration time.Duration) time.Duration {
	// Calculate the amount of jitter to add to the duration
	jitter := float64(duration) * JitterRatio

	// Generate a random number between -jitter and +jitter
	randomJitter := rand.Float64()*(2*jitter) - jitter

	// Apply the random jitter to the original duration
	return duration + time.Duration(randomJitter)
}

// updateBackoffStatus updates the client's backoff status based on success/failure
func (clientWrapper *configurationClientWrapper) updateBackoffStatus(successful bool) {
	if successful {
		clientWrapper.BackOffEndTime = time.Time{}
		clientWrapper.FailedAttempts = 0
	} else {
		clientWrapper.FailedAttempts++
		clientWrapper.BackOffEndTime = time.Now().Add(calculateBackoffDuration(clientWrapper.FailedAttempts))
	}
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