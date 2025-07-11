// Copyright (c) Microsoft Corporation.
// Licensed under the MIT License.

package azureappconfiguration

import (
	"context"
	"fmt"
	"log"
	"math"
	"math/rand"
	"net"
	"net/url"
	"strconv"
	"strings"
	"time"

	"github.com/Azure/azure-sdk-for-go/sdk/azcore"
	"github.com/Azure/azure-sdk-for-go/sdk/azcore/policy"
	"github.com/Azure/azure-sdk-for-go/sdk/data/azappconfig"
)

// configurationClientManager handles creation and management of app configuration clients
type configurationClientManager struct {
	replicaDiscoveryEnabled bool
	clientOptions *azappconfig.ClientOptions
	staticClient  *configurationClientWrapper
	dynamicClients []*configurationClientWrapper
	endpoint      string
	validDomain   string
	credential    azcore.TokenCredential
	secret        string
	id            string
	lastFallbackClientAttempt time.Time
	lastFallbackClientRefresh time.Time
}

// configurationClientWrapper wraps an Azure App Configuration client with additional metadata
type configurationClientWrapper struct {
	endpoint string
	client   *azappconfig.Client
	backOffEndTime time.Time
	failedAttempts int
}

// newConfigurationClientManager creates a new configuration client manager
func newConfigurationClientManager(authOptions AuthenticationOptions, options *Options) (*configurationClientManager, error) {
	manager := &configurationClientManager{
		clientOptions: setTelemetry(options.ClientOptions),
	}

	if options.ReplicaDiscoveryEnabled == nil || *options.ReplicaDiscoveryEnabled {
		manager.replicaDiscoveryEnabled = true
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

	manager.validDomain = getValidDomain(manager.endpoint)
	manager.staticClient = &configurationClientWrapper{
		endpoint: manager.endpoint,
		client:   staticClient,
	}

	return nil
}

func (manager *configurationClientManager) getClients(ctx context.Context) ([]*configurationClientWrapper, error) {
	currentTime := time.Now()
	clients := make([]*configurationClientWrapper, 0, 1+len(manager.dynamicClients))

	// Add the static client if it is not in backoff
	if currentTime.After(manager.staticClient.backOffEndTime) {
		clients = append(clients, manager.staticClient)
	}

	if !manager.replicaDiscoveryEnabled {
		return clients, nil
	}

	if currentTime.After(manager.lastFallbackClientAttempt.Add(minimalClientRefreshInterval)) &&
		(manager.dynamicClients == nil ||
			currentTime.After(manager.lastFallbackClientRefresh.Add(fallbackClientRefreshExpireInterval))) {
		manager.lastFallbackClientAttempt = currentTime
		url, _ := url.Parse(manager.endpoint)
		manager.discoverFallbackClients(ctx, url.Host)
	}

	for _, clientWrapper := range manager.dynamicClients {
		if currentTime.After(clientWrapper.backOffEndTime) {
			clients = append(clients, clientWrapper)
		}
	}

	return clients, nil
}

func (manager *configurationClientManager) refreshClients(ctx context.Context) {
	currentTime := time.Now()
	if manager.replicaDiscoveryEnabled &&
		currentTime.After(manager.lastFallbackClientAttempt.Add(minimalClientRefreshInterval)) {
		manager.lastFallbackClientAttempt = currentTime
		url, _ := url.Parse(manager.endpoint)
		manager.discoverFallbackClients(ctx, url.Host)
	}
}

func (manager *configurationClientManager) discoverFallbackClients(ctx context.Context, host string) {
	srvTargetHosts, err := querySrvTargetHost(ctx, host)
	if err != nil {
		log.Printf("failed to discover fallback clients for %s: %v", host, err)
		return
	}
	
	manager.processSrvTargetHosts(srvTargetHosts)
}

func (manager *configurationClientManager) processSrvTargetHosts(srvTargetHosts []string) {
	// Shuffle the list of SRV target hosts for load balancing
	rand.Shuffle(len(srvTargetHosts), func(i, j int) {
		srvTargetHosts[i], srvTargetHosts[j] = srvTargetHosts[j], srvTargetHosts[i]
	})

	newDynamicClients := make([]*configurationClientWrapper, 0, len(srvTargetHosts))
	for _, host := range srvTargetHosts {
		if isValidEndpoint(host, manager.validDomain) {
			targetEndpoint := "https://" + host
			if strings.EqualFold(targetEndpoint, manager.endpoint) {
				continue // Skip primary endpoint
			}
			
			client, err := manager.newConfigurationClient(targetEndpoint)
			if err != nil {
				log.Printf("failed to create client for replica %s: %v", targetEndpoint, err)
				continue // Continue with other replicas instead of returning
			}
			
			newDynamicClients = append(newDynamicClients, &configurationClientWrapper{
				endpoint: targetEndpoint,
				client:   client,
			})
		}
	}

	manager.dynamicClients = newDynamicClients
	manager.lastFallbackClientRefresh = time.Now()
}

func querySrvTargetHost(ctx context.Context, host string) ([]string, error) {
	results := make([]string, 0)

	_, originRecords, err := net.DefaultResolver.LookupSRV(ctx, originKey, tcpKey, host)
	if err != nil {
		// If the host does not have SRV records => no replicas
		if dnsErr, ok := err.(*net.DNSError); ok && dnsErr.IsNotFound {
			return results, nil
		} else {
			return results, err
		}
	}

	if len(originRecords) == 0 {
		return results, nil
	}

	originHost := strings.TrimSuffix(originRecords[0].Target, ".")
	results = append(results, originHost)
	index := 0
	for {
		currentAlt := altKey + strconv.Itoa(index)
		_, altRecords, err := net.DefaultResolver.LookupSRV(ctx, currentAlt, tcpKey, originHost)
		if err != nil {
			// If the host does not have SRV records => no more replicas
			if dnsErr, ok := err.(*net.DNSError); ok && dnsErr.IsNotFound {
				break
			} else {
				return results, err
			}
		}

		for _, record := range altRecords {
			altHost := strings.TrimSuffix(record.Target, ".")
			if altHost != "" {
				results = append(results, altHost)
			}
		}
		index = index + 1
	}

	return results, nil
}

func (manager *configurationClientManager) newConfigurationClient(endpoint string) (*azappconfig.Client, error) {
	if manager.credential != nil {
		return azappconfig.NewClient(endpoint, manager.credential, manager.clientOptions)
	}

	connectionStr := buildConnectionString(endpoint, manager.secret, manager.id)
	if connectionStr == "" {
		return nil, fmt.Errorf("failed to build connection string for fallback client")
	}

	return azappconfig.NewClientFromConnectionString(connectionStr, manager.clientOptions)
}

func buildConnectionString(endpoint string, secret string, id string) string {
	if secret == "" || id == "" {
		return ""
	}

	return fmt.Sprintf("%s=%s;%s=%s;%s=%s",
		endpointKey, endpoint,
		idKey, id,
		secretKey, secret)
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

	if !options.Telemetry.Disabled && options.Telemetry.ApplicationID == "" {
		options.Telemetry = policy.TelemetryOptions{
			ApplicationID: fmt.Sprintf("%s/%s", moduleName, moduleVersion),
		}
	}

	return options
}

func getValidDomain(endpoint string) string {
	url, _ := url.Parse(endpoint)
	TrustedDomainLabels := []string{azConfigDomainLabel, appConfigDomainLabel}
	for _, label := range TrustedDomainLabels {
		index := strings.LastIndex(strings.ToLower(url.Host), strings.ToLower(label))
		if index != -1 {
			return url.Host[index:]
		}
	}

	return ""
}

func isValidEndpoint(host string, validDomain string) bool {
	if validDomain == "" {
		return false
	}

	return strings.HasSuffix(strings.ToLower(host), strings.ToLower(validDomain))
}

func (client *configurationClientWrapper) updateBackoffStatus(success bool) {
	if success {
		client.failedAttempts = 0
		client.backOffEndTime = time.Time{} 
	} else {
		client.failedAttempts++
		client.backOffEndTime = time.Now().Add(client.getBackoffDuration())
	}
}

func (client *configurationClientWrapper) getBackoffDuration() time.Duration {
	if client.failedAttempts <= 1 {
		return minBackoffDuration
	}

	// Cap the exponent to prevent overflow
	exponent := math.Min(float64(client.failedAttempts-1), float64(safeShiftLimit))
	calculatedMilliseconds := float64(minBackoffDuration.Milliseconds()) * math.Pow(2, exponent)
	
	if calculatedMilliseconds > float64(maxBackoffDuration.Milliseconds()) || calculatedMilliseconds <= 0 {
		calculatedMilliseconds = float64(maxBackoffDuration.Milliseconds())
	}

	calculatedDuration := time.Duration(calculatedMilliseconds) * time.Millisecond
	return jitter(calculatedDuration)
}

func jitter(duration time.Duration) time.Duration {
	// Calculate the amount of jitter to add to the duration
	jitter := float64(duration) * jitterRatio

	// Generate a random number between -jitter and +jitter
	randomJitter := rand.Float64()*(2*jitter) - jitter

	// Apply the random jitter to the original duration
	return duration + time.Duration(randomJitter)
}