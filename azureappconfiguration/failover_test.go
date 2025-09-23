// Copyright (c) Microsoft Corporation.
// Licensed under the MIT License.

package azureappconfiguration

import (
	"context"
	"errors"
	"fmt"
	"net"
	"net/http"
	"strings"
	"testing"
	"time"

	"github.com/Azure/azure-sdk-for-go/sdk/azcore"
	"github.com/Azure/azure-sdk-for-go/sdk/data/azappconfig"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
)

// mockClientManager implements the clientManager interface for testing
type mockClientManager struct {
	mock.Mock
}

func (m *mockClientManager) getClients(ctx context.Context) ([]*configurationClientWrapper, error) {
	args := m.Called(ctx)
	return args.Get(0).([]*configurationClientWrapper), args.Error(1)
}

func (m *mockClientManager) refreshClients(ctx context.Context) {
	m.Called(ctx)
}

// Test executeFailoverPolicy with successful operation on first client
func TestExecuteFailoverPolicy_Success_FirstClient(t *testing.T) {
	mockClientManager := new(mockClientManager)

	// Create mock clients
	client1 := &azappconfig.Client{}
	client2 := &azappconfig.Client{}

	clientWrappers := []*configurationClientWrapper{
		{endpoint: "https://primary.azconfig.io", client: client1, failedAttempts: 0},
		{endpoint: "https://replica.azconfig.io", client: client2, failedAttempts: 0},
	}

	mockClientManager.On("getClients", mock.Anything).Return(clientWrappers, nil)

	azappcfg := &AzureAppConfiguration{
		clientManager: mockClientManager,
	}

	operationCallCount := 0
	operation := func(client *azappconfig.Client) error {
		operationCallCount++
		if client == client1 {
			return nil // Success on first client
		}
		return fmt.Errorf("should not reach second client")
	}

	err := azappcfg.executeFailoverPolicy(context.Background(), operation)

	assert.NoError(t, err)
	assert.Equal(t, 1, operationCallCount, "Operation should be called only once")
	assert.Equal(t, 0, clientWrappers[0].failedAttempts, "First client should have no failed attempts")
	mockClientManager.AssertExpectations(t)
}

// Test executeFailoverPolicy with failure on first client but success on second
func TestExecuteFailoverPolicy_FailoverToSecondClient(t *testing.T) {
	mockClientManager := new(mockClientManager)

	client1 := &azappconfig.Client{}
	client2 := &azappconfig.Client{}

	clientWrappers := []*configurationClientWrapper{
		{endpoint: "https://primary.azconfig.io", client: client1, failedAttempts: 0},
		{endpoint: "https://replica.azconfig.io", client: client2, failedAttempts: 0},
	}

	mockClientManager.On("getClients", mock.Anything).Return(clientWrappers, nil)

	azappcfg := &AzureAppConfiguration{
		clientManager: mockClientManager,
	}

	operationCallCount := 0
	operation := func(client *azappconfig.Client) error {
		operationCallCount++
		if client == client1 {
			// Simulate a failoverable error (network error)
			return &net.DNSError{Err: "no such host", Name: "primary.azconfig.io"}
		}
		if client == client2 {
			return nil // Success on second client
		}
		return fmt.Errorf("unexpected client")
	}

	err := azappcfg.executeFailoverPolicy(context.Background(), operation)

	assert.NoError(t, err)
	assert.Equal(t, 2, operationCallCount, "Operation should be called twice")
	assert.Equal(t, 1, clientWrappers[0].failedAttempts, "First client should have one failed attempt")
	assert.Equal(t, 0, clientWrappers[1].failedAttempts, "Second client should have no failed attempts")
	mockClientManager.AssertExpectations(t)
}

// Test executeFailoverPolicy with all clients failing with failoverable errors
func TestExecuteFailoverPolicy_AllClientsFail_FailoverableErrors(t *testing.T) {
	mockClientManager := new(mockClientManager)

	client1 := &azappconfig.Client{}
	client2 := &azappconfig.Client{}

	clientWrappers := []*configurationClientWrapper{
		{endpoint: "https://primary.azconfig.io", client: client1, failedAttempts: 0},
		{endpoint: "https://replica.azconfig.io", client: client2, failedAttempts: 0},
	}

	mockClientManager.On("getClients", mock.Anything).Return(clientWrappers, nil)
	mockClientManager.On("refreshClients", mock.Anything).Return()

	azappcfg := &AzureAppConfiguration{
		clientManager: mockClientManager,
	}

	operationCallCount := 0
	operation := func(client *azappconfig.Client) error {
		operationCallCount++
		if client == client1 {
			return &azcore.ResponseError{StatusCode: http.StatusServiceUnavailable}
		}
		if client == client2 {
			return &net.OpError{Op: "dial", Net: "tcp", Err: errors.New("connection refused")}
		}
		return fmt.Errorf("unexpected client")
	}

	err := azappcfg.executeFailoverPolicy(context.Background(), operation)

	assert.Error(t, err)
	assert.Contains(t, err.Error(), "failed to get settings from all clients")
	assert.Equal(t, 2, operationCallCount, "Operation should be called for both clients")
	assert.Equal(t, 1, clientWrappers[0].failedAttempts, "First client should have one failed attempt")
	assert.Equal(t, 1, clientWrappers[1].failedAttempts, "Second client should have one failed attempt")
	mockClientManager.AssertExpectations(t)
}

// Test executeFailoverPolicy with non-failoverable error
func TestExecuteFailoverPolicy_NonFailoverableError(t *testing.T) {
	mockClientManager := new(mockClientManager)

	client1 := &azappconfig.Client{}
	client2 := &azappconfig.Client{}

	clientWrappers := []*configurationClientWrapper{
		{endpoint: "https://primary.azconfig.io", client: client1, failedAttempts: 0},
		{endpoint: "https://replica.azconfig.io", client: client2, failedAttempts: 0},
	}

	mockClientManager.On("getClients", mock.Anything).Return(clientWrappers, nil)

	azappcfg := &AzureAppConfiguration{
		clientManager: mockClientManager,
	}

	operationCallCount := 0
	nonFailoverableError := &azcore.ResponseError{StatusCode: http.StatusBadRequest}
	operation := func(client *azappconfig.Client) error {
		operationCallCount++
		return nonFailoverableError
	}

	err := azappcfg.executeFailoverPolicy(context.Background(), operation)

	assert.Error(t, err)
	assert.Equal(t, nonFailoverableError, err)
	assert.Equal(t, 1, operationCallCount, "Operation should be called only once")
	assert.Equal(t, 0, clientWrappers[0].failedAttempts, "First client should have no failed attempts for non-failoverable error")
	mockClientManager.AssertExpectations(t)
}

// Test executeFailoverPolicy with no available clients
func TestExecuteFailoverPolicy_NoClientsAvailable(t *testing.T) {
	mockClientManager := new(mockClientManager)

	mockClientManager.On("getClients", mock.Anything).Return([]*configurationClientWrapper{}, nil)
	mockClientManager.On("refreshClients", mock.Anything).Return()

	azappcfg := &AzureAppConfiguration{
		clientManager: mockClientManager,
	}

	operationCallCount := 0
	operation := func(client *azappconfig.Client) error {
		operationCallCount++
		return nil
	}

	err := azappcfg.executeFailoverPolicy(context.Background(), operation)

	assert.Error(t, err)
	assert.Contains(t, err.Error(), "no client is available to connect to the target App Configuration store")
	assert.Equal(t, 0, operationCallCount, "Operation should not be called when no clients available")
	mockClientManager.AssertExpectations(t)
}

// Test executeFailoverPolicy with client manager error
func TestExecuteFailoverPolicy_ClientManagerError(t *testing.T) {
	mockClientManager := new(mockClientManager)

	clientManagerError := errors.New("failed to get clients")
	mockClientManager.On("getClients", mock.Anything).Return([]*configurationClientWrapper{}, clientManagerError)

	azappcfg := &AzureAppConfiguration{
		clientManager: mockClientManager,
	}

	operationCallCount := 0
	operation := func(client *azappconfig.Client) error {
		operationCallCount++
		return nil
	}

	err := azappcfg.executeFailoverPolicy(context.Background(), operation)

	assert.Error(t, err)
	assert.Equal(t, clientManagerError, err)
	assert.Equal(t, 0, operationCallCount, "Operation should not be called when client manager fails")
	mockClientManager.AssertExpectations(t)
}

// Test isFailoverable function with various error types
func TestIsFailoverable(t *testing.T) {
	tests := []struct {
		name     string
		err      error
		expected bool
	}{
		{
			name:     "nil error",
			err:      nil,
			expected: false,
		},
		{
			name:     "network DNS error",
			err:      &net.DNSError{Err: "no such host", Name: "test.com"},
			expected: true,
		},
		{
			name:     "network operation error",
			err:      &net.OpError{Op: "dial", Net: "tcp", Err: errors.New("connection refused")},
			expected: true,
		},
		{
			name:     "HTTP 429 Too Many Requests",
			err:      &azcore.ResponseError{StatusCode: http.StatusTooManyRequests},
			expected: true,
		},
		{
			name:     "HTTP 408 Request Timeout",
			err:      &azcore.ResponseError{StatusCode: http.StatusRequestTimeout},
			expected: true,
		},
		{
			name:     "HTTP 403 Forbidden",
			err:      &azcore.ResponseError{StatusCode: http.StatusForbidden},
			expected: true,
		},
		{
			name:     "HTTP 401 Unauthorized",
			err:      &azcore.ResponseError{StatusCode: http.StatusUnauthorized},
			expected: true,
		},
		{
			name:     "HTTP 502 Bad Gateway",
			err:      &azcore.ResponseError{StatusCode: http.StatusBadGateway},
			expected: true,
		},
		{
			name:     "HTTP 503 Service Unavailable",
			err:      &azcore.ResponseError{StatusCode: http.StatusServiceUnavailable},
			expected: true,
		},
		{
			name:     "HTTP 504 Gateway Timeout",
			err:      &azcore.ResponseError{StatusCode: http.StatusGatewayTimeout},
			expected: true,
		},
		{
			name:     "HTTP 500 Internal Server Error",
			err:      &azcore.ResponseError{StatusCode: http.StatusInternalServerError},
			expected: true,
		},
		{
			name:     "HTTP 599 (5xx range)",
			err:      &azcore.ResponseError{StatusCode: 599},
			expected: true,
		},
		{
			name:     "HTTP 400 Bad Request",
			err:      &azcore.ResponseError{StatusCode: http.StatusBadRequest},
			expected: false,
		},
		{
			name:     "HTTP 404 Not Found",
			err:      &azcore.ResponseError{StatusCode: http.StatusNotFound},
			expected: false,
		},
		{
			name:     "HTTP 200 OK",
			err:      &azcore.ResponseError{StatusCode: http.StatusOK},
			expected: false,
		},
		{
			name:     "generic error",
			err:      errors.New("generic error"),
			expected: false,
		},
		{
			name:     "wrapped network error",
			err:      fmt.Errorf("connection failed: %w", &net.DNSError{Err: "no such host"}),
			expected: true,
		},
		{
			name:     "wrapped response error",
			err:      fmt.Errorf("request failed: %w", &azcore.ResponseError{StatusCode: http.StatusServiceUnavailable}),
			expected: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := isFailoverable(tt.err)
			assert.Equal(t, tt.expected, result)
		})
	}
}

// Test executeFailoverPolicy with load balancing enabled - should rotate clients
func TestExecuteFailoverPolicy_LoadBalancing_RotateClients(t *testing.T) {
	mockClientManager := new(mockClientManager)

	client1 := &azappconfig.Client{}
	client2 := &azappconfig.Client{}
	client3 := &azappconfig.Client{}

	clientWrappers := []*configurationClientWrapper{
		{endpoint: "https://primary.azconfig.io", client: client1, failedAttempts: 0},
		{endpoint: "https://replica1.azconfig.io", client: client2, failedAttempts: 0},
		{endpoint: "https://replica2.azconfig.io", client: client3, failedAttempts: 0},
	}

	mockClientManager.On("getClients", mock.Anything).Return(clientWrappers, nil)

	azappcfg := &AzureAppConfiguration{
		clientManager:          mockClientManager,
		loadBalancingEnabled:   true,
		lastSuccessfulEndpoint: "https://primary.azconfig.io", // Last successful was the first client
	}

	var usedClient *azappconfig.Client
	operation := func(client *azappconfig.Client) error {
		usedClient = client
		return nil // Success
	}

	err := azappcfg.executeFailoverPolicy(context.Background(), operation)

	assert.NoError(t, err)
	// After rotation, the second client (replica1) should be used first
	assert.Equal(t, client2, usedClient, "Should use the next client after rotation")
	assert.Equal(t, "https://replica1.azconfig.io", azappcfg.lastSuccessfulEndpoint, "Should update last successful endpoint")
	mockClientManager.AssertExpectations(t)
}

// Test executeFailoverPolicy with load balancing enabled - last client was successful
func TestExecuteFailoverPolicy_LoadBalancing_LastClientSuccessful(t *testing.T) {
	mockClientManager := new(mockClientManager)

	client1 := &azappconfig.Client{}
	client2 := &azappconfig.Client{}
	client3 := &azappconfig.Client{}

	clientWrappers := []*configurationClientWrapper{
		{endpoint: "https://primary.azconfig.io", client: client1, failedAttempts: 0},
		{endpoint: "https://replica1.azconfig.io", client: client2, failedAttempts: 0},
		{endpoint: "https://replica2.azconfig.io", client: client3, failedAttempts: 0},
	}

	mockClientManager.On("getClients", mock.Anything).Return(clientWrappers, nil)

	azappcfg := &AzureAppConfiguration{
		clientManager:          mockClientManager,
		loadBalancingEnabled:   true,
		lastSuccessfulEndpoint: "https://replica2.azconfig.io", // Last successful was the last client
	}

	var usedClient *azappconfig.Client
	operation := func(client *azappconfig.Client) error {
		usedClient = client
		return nil // Success
	}

	err := azappcfg.executeFailoverPolicy(context.Background(), operation)

	assert.NoError(t, err)
	// After rotation, should wrap around to the first client
	assert.Equal(t, client1, usedClient, "Should wrap around to the first client")
	assert.Equal(t, "https://primary.azconfig.io", azappcfg.lastSuccessfulEndpoint, "Should update last successful endpoint")
	mockClientManager.AssertExpectations(t)
}

// Test executeFailoverPolicy with load balancing disabled - should not rotate
func TestExecuteFailoverPolicy_LoadBalancing_Disabled(t *testing.T) {
	mockClientManager := new(mockClientManager)

	client1 := &azappconfig.Client{}
	client2 := &azappconfig.Client{}

	clientWrappers := []*configurationClientWrapper{
		{endpoint: "https://primary.azconfig.io", client: client1, failedAttempts: 0},
		{endpoint: "https://replica.azconfig.io", client: client2, failedAttempts: 0},
	}

	mockClientManager.On("getClients", mock.Anything).Return(clientWrappers, nil)

	azappcfg := &AzureAppConfiguration{
		clientManager:          mockClientManager,
		loadBalancingEnabled:   false, // Load balancing disabled
		lastSuccessfulEndpoint: "https://primary.azconfig.io",
	}

	var usedClient *azappconfig.Client
	operation := func(client *azappconfig.Client) error {
		usedClient = client
		return nil // Success
	}

	err := azappcfg.executeFailoverPolicy(context.Background(), operation)

	assert.NoError(t, err)
	// Should use the first client (no rotation)
	assert.Equal(t, client1, usedClient, "Should use the first client when load balancing is disabled")
	// lastSuccessfulEndpoint should not be updated when load balancing is disabled
	assert.Equal(t, "https://primary.azconfig.io", azappcfg.lastSuccessfulEndpoint)
	mockClientManager.AssertExpectations(t)
}

// Test executeFailoverPolicy with load balancing - single client
func TestExecuteFailoverPolicy_LoadBalancing_SingleClient(t *testing.T) {
	mockClientManager := new(mockClientManager)

	client1 := &azappconfig.Client{}

	clientWrappers := []*configurationClientWrapper{
		{endpoint: "https://primary.azconfig.io", client: client1, failedAttempts: 0},
	}

	mockClientManager.On("getClients", mock.Anything).Return(clientWrappers, nil)

	azappcfg := &AzureAppConfiguration{
		clientManager:          mockClientManager,
		loadBalancingEnabled:   true,
		lastSuccessfulEndpoint: "https://primary.azconfig.io",
	}

	var usedClient *azappconfig.Client
	operation := func(client *azappconfig.Client) error {
		usedClient = client
		return nil // Success
	}

	err := azappcfg.executeFailoverPolicy(context.Background(), operation)

	assert.NoError(t, err)
	// Should use the only client available
	assert.Equal(t, client1, usedClient, "Should use the only available client")
	assert.Equal(t, "https://primary.azconfig.io", azappcfg.lastSuccessfulEndpoint, "Should update last successful endpoint")
	mockClientManager.AssertExpectations(t)
}

// Test rotateClientsToNextEndpoint function directly
func TestRotateClientsToNextEndpoint(t *testing.T) {
	client1 := &azappconfig.Client{}
	client2 := &azappconfig.Client{}
	client3 := &azappconfig.Client{}

	tests := []struct {
		name                   string
		clients                []*configurationClientWrapper
		lastSuccessfulEndpoint string
		expectedFirstClient    string
	}{
		{
			name: "rotate from first to second",
			clients: []*configurationClientWrapper{
				{endpoint: "https://primary.azconfig.io", client: client1},
				{endpoint: "https://replica1.azconfig.io", client: client2},
				{endpoint: "https://replica2.azconfig.io", client: client3},
			},
			lastSuccessfulEndpoint: "https://primary.azconfig.io",
			expectedFirstClient:    "https://replica1.azconfig.io",
		},
		{
			name: "rotate from middle to next",
			clients: []*configurationClientWrapper{
				{endpoint: "https://primary.azconfig.io", client: client1},
				{endpoint: "https://replica1.azconfig.io", client: client2},
				{endpoint: "https://replica2.azconfig.io", client: client3},
			},
			lastSuccessfulEndpoint: "https://replica1.azconfig.io",
			expectedFirstClient:    "https://replica2.azconfig.io",
		},
		{
			name: "rotate from last to first (wrap around)",
			clients: []*configurationClientWrapper{
				{endpoint: "https://primary.azconfig.io", client: client1},
				{endpoint: "https://replica1.azconfig.io", client: client2},
				{endpoint: "https://replica2.azconfig.io", client: client3},
			},
			lastSuccessfulEndpoint: "https://replica2.azconfig.io",
			expectedFirstClient:    "https://primary.azconfig.io",
		},
		{
			name: "single client - no rotation",
			clients: []*configurationClientWrapper{
				{endpoint: "https://primary.azconfig.io", client: client1},
			},
			lastSuccessfulEndpoint: "https://primary.azconfig.io",
			expectedFirstClient:    "https://primary.azconfig.io",
		},
		{
			name:                   "empty clients - no panic",
			clients:                []*configurationClientWrapper{},
			lastSuccessfulEndpoint: "https://primary.azconfig.io",
			expectedFirstClient:    "", // No clients, so no first client
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Make a copy to avoid modifying the original slice
			clientsCopy := make([]*configurationClientWrapper, len(tt.clients))
			copy(clientsCopy, tt.clients)

			rotateClientsToNextEndpoint(clientsCopy, tt.lastSuccessfulEndpoint)

			if len(clientsCopy) > 0 {
				assert.Equal(t, tt.expectedFirstClient, clientsCopy[0].endpoint, "First client after rotation should match expected")
			}
		})
	}
}

// Test rotateSliceInPlace function directly
func TestRotateSliceInPlace(t *testing.T) {
	tests := []struct {
		name     string
		input    []int
		k        int
		expected []int
	}{
		{
			name:     "rotate by 1",
			input:    []int{1, 2, 3, 4, 5},
			k:        1,
			expected: []int{2, 3, 4, 5, 1},
		},
		{
			name:     "rotate by 2",
			input:    []int{1, 2, 3, 4, 5},
			k:        2,
			expected: []int{3, 4, 5, 1, 2},
		},
		{
			name:     "rotate by length (no change)",
			input:    []int{1, 2, 3, 4, 5},
			k:        5,
			expected: []int{1, 2, 3, 4, 5},
		},
		{
			name:     "rotate by more than length",
			input:    []int{1, 2, 3, 4, 5},
			k:        7, // 7 % 5 = 2
			expected: []int{3, 4, 5, 1, 2},
		},
		{
			name:     "rotate by 0",
			input:    []int{1, 2, 3, 4, 5},
			k:        0,
			expected: []int{1, 2, 3, 4, 5},
		},
		{
			name:     "single element",
			input:    []int{1},
			k:        1,
			expected: []int{1},
		},
		{
			name:     "two elements rotate by 1",
			input:    []int{1, 2},
			k:        1,
			expected: []int{2, 1},
		},
		{
			name:     "empty slice",
			input:    []int{},
			k:        1,
			expected: []int{},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Make a copy to avoid modifying the original slice
			inputCopy := make([]int, len(tt.input))
			copy(inputCopy, tt.input)

			rotateSliceInPlace(inputCopy, tt.k)
			assert.Equal(t, tt.expected, inputCopy, "Rotated slice should match expected result")
		})
	}
}

// Test client wrapper backoff behavior
func TestClientWrapper_UpdateBackoffStatus(t *testing.T) {
	client := &configurationClientWrapper{
		endpoint:       "https://test.azconfig.io",
		client:         &azappconfig.Client{},
		failedAttempts: 0,
		backOffEndTime: time.Time{},
	}

	// Test successful operation resets backoff
	client.updateBackoffStatus(true)
	assert.Equal(t, 0, client.failedAttempts)
	assert.True(t, client.backOffEndTime.IsZero())

	// Test failed operation increments attempts and sets backoff
	client.updateBackoffStatus(false)
	assert.Equal(t, 1, client.failedAttempts)
	assert.False(t, client.backOffEndTime.IsZero())
	assert.True(t, client.backOffEndTime.After(time.Now()))

	// Test multiple failures increase backoff duration
	firstBackoffEnd := client.backOffEndTime
	time.Sleep(1 * time.Millisecond) // Ensure time progression
	client.updateBackoffStatus(false)
	assert.Equal(t, 2, client.failedAttempts)
	assert.True(t, client.backOffEndTime.After(firstBackoffEnd))
}

// Test client wrapper backoff duration calculation
func TestClientWrapper_GetBackoffDuration(t *testing.T) {
	client := &configurationClientWrapper{
		endpoint:       "https://test.azconfig.io",
		client:         &azappconfig.Client{},
		failedAttempts: 0,
	}

	// First failure should return minimum backoff duration
	client.failedAttempts = 1
	duration := calculateBackoffDuration(client.failedAttempts)
	assert.True(t, duration >= minBackoffDuration)
	assert.True(t, duration <= minBackoffDuration*2) // Account for jitter

	// Multiple failures should increase duration exponentially
	client.failedAttempts = 3
	duration3 := calculateBackoffDuration(client.failedAttempts)
	assert.True(t, duration3 > duration)

	// Very high failure count should be capped at max duration
	client.failedAttempts = 100
	durationMax := calculateBackoffDuration(client.failedAttempts)
	assert.True(t, durationMax <= maxBackoffDuration*2) // Account for jitter
}

// Test startupWithRetry with successful operation on first attempt
func TestStartupWithRetry_Success_FirstAttempt(t *testing.T) {
	mockClientManager := new(mockClientManager)

	azappcfg := &AzureAppConfiguration{
		clientManager: mockClientManager,
		keyValues:     make(map[string]any),
		featureFlags:  make(map[string]any),
		kvSelectors:   []Selector{{KeyFilter: "*", LabelFilter: "\x00"}},
	}

	// Create a successful operation that simply returns nil
	operation := func(ctx context.Context) error {
		return nil // Success on first attempt
	}

	ctx := context.Background()
	err := azappcfg.startupWithRetry(ctx, 10*time.Second, operation)

	assert.NoError(t, err)
}

// Test startupWithRetry with retry after retriable error
func TestStartupWithRetry_Success_AfterRetriableError(t *testing.T) {
	mockClientManager := new(mockClientManager)

	azappcfg := &AzureAppConfiguration{
		clientManager: mockClientManager,
		keyValues:     make(map[string]any),
		featureFlags:  make(map[string]any),
		kvSelectors:   []Selector{{KeyFilter: "*", LabelFilter: "\x00"}},
	}

	callCount := 0
	retriableError := &azcore.ResponseError{StatusCode: http.StatusServiceUnavailable}

	// Create an operation that fails first, then succeeds
	operation := func(ctx context.Context) error {
		callCount++
		if callCount == 1 {
			return retriableError // Fail on first attempt
		}
		return nil // Success on second attempt
	}

	ctx := context.Background()
	err := azappcfg.startupWithRetry(ctx, 10*time.Second, operation)

	assert.NoError(t, err)
	assert.Equal(t, 2, callCount, "Operation should be called twice")
}

// Test startupWithRetry with non-retriable error
func TestStartupWithRetry_NonRetriableError(t *testing.T) {
	mockClientManager := new(mockClientManager)

	azappcfg := &AzureAppConfiguration{
		clientManager: mockClientManager,
		keyValues:     make(map[string]any),
		featureFlags:  make(map[string]any),
		kvSelectors:   []Selector{{KeyFilter: "*", LabelFilter: "\x00"}},
	}

	callCount := 0
	nonRetriableError := &azcore.ResponseError{StatusCode: http.StatusBadRequest}

	// Create an operation that fails with non-retriable error
	operation := func(ctx context.Context) error {
		callCount++
		return nonRetriableError // Non-retriable error
	}

	ctx := context.Background()
	err := azappcfg.startupWithRetry(ctx, 10*time.Second, operation)

	assert.Error(t, err)
	assert.Contains(t, err.Error(), "load from Azure App Configuration failed with non-retriable error")
	assert.Equal(t, 1, callCount, "Operation should be called only once for non-retriable error")
}

// Test startupWithRetry with timeout
func TestStartupWithRetry_Timeout(t *testing.T) {
	mockClientManager := new(mockClientManager)

	azappcfg := &AzureAppConfiguration{
		clientManager: mockClientManager,
		keyValues:     make(map[string]any),
		featureFlags:  make(map[string]any),
		kvSelectors:   []Selector{{KeyFilter: "*", LabelFilter: "\x00"}},
	}

	callCount := 0
	retriableError := &azcore.ResponseError{StatusCode: http.StatusServiceUnavailable}

	// Create an operation that always fails with retriable error
	operation := func(ctx context.Context) error {
		callCount++
		return retriableError // Always fail
	}

	ctx := context.Background()
	// Use a very short timeout to trigger timeout quickly
	err := azappcfg.startupWithRetry(ctx, 100*time.Millisecond, operation)

	assert.Error(t, err)
	assert.True(t,
		err.Error() == "startup timeout reached after 100ms" ||
			err.Error() == fmt.Sprintf("load from Azure App Configuration failed after %d attempts within timeout 100ms: %v", callCount, retriableError),
		"Error should indicate timeout or max attempts reached: %v", err)
	assert.True(t, callCount >= 1, "Operation should be called at least once")
}

// Test startupWithRetry with context cancellation during backoff
func TestStartupWithRetry_ContextCancelledDuringBackoff(t *testing.T) {
	mockClientManager := new(mockClientManager)

	azappcfg := &AzureAppConfiguration{
		clientManager: mockClientManager,
		keyValues:     make(map[string]any),
		featureFlags:  make(map[string]any),
		kvSelectors:   []Selector{{KeyFilter: "*", LabelFilter: "\x00"}},
	}

	callCount := 0
	retriableError := &azcore.ResponseError{StatusCode: http.StatusServiceUnavailable}

	// Create an operation that fails with retriable error
	operation := func(ctx context.Context) error {
		callCount++
		return retriableError
	}

	ctx, cancel := context.WithCancel(context.Background())

	// Cancel the context after a short delay to simulate cancellation during backoff
	go func() {
		time.Sleep(50 * time.Millisecond)
		cancel()
	}()

	err := azappcfg.startupWithRetry(ctx, 10*time.Second, operation)

	assert.Error(t, err)
	assert.Contains(t, err.Error(), "load from Azure App Configuration timed out: context canceled")
}

// Test startupWithRetry with default timeout when zero timeout provided
func TestStartupWithRetry_DefaultTimeout(t *testing.T) {
	mockClientManager := new(mockClientManager)

	azappcfg := &AzureAppConfiguration{
		clientManager: mockClientManager,
		keyValues:     make(map[string]any),
		featureFlags:  make(map[string]any),
		kvSelectors:   []Selector{{KeyFilter: "*", LabelFilter: "\x00"}},
	}

	callCount := 0
	// Create an operation that succeeds
	operation := func(ctx context.Context) error {
		callCount++
		return nil
	}

	ctx := context.Background()
	// Pass zero timeout to test default timeout usage
	err := azappcfg.startupWithRetry(ctx, 0, operation)

	assert.NoError(t, err)
	assert.Equal(t, 1, callCount, "Operation should be called once")
}

// Test startupWithRetry with insufficient time remaining for retry
func TestStartupWithRetry_InsufficientTimeForRetry(t *testing.T) {
	mockClientManager := new(mockClientManager)

	azappcfg := &AzureAppConfiguration{
		clientManager: mockClientManager,
		keyValues:     make(map[string]any),
		featureFlags:  make(map[string]any),
		kvSelectors:   []Selector{{KeyFilter: "*", LabelFilter: "\x00"}},
	}

	callCount := 0
	retriableError := &azcore.ResponseError{StatusCode: http.StatusServiceUnavailable}

	// Create an operation that always fails
	operation := func(ctx context.Context) error {
		callCount++
		// Add some delay to consume time
		time.Sleep(50 * time.Millisecond)
		return retriableError
	}

	ctx := context.Background()
	// Use a short timeout that will be consumed by the first failure and not allow retry
	err := azappcfg.startupWithRetry(ctx, 80*time.Millisecond, operation)

	assert.Error(t, err)
	assert.True(t,
		err.Error() == "startup timeout reached after 80ms" ||
			strings.Contains(err.Error(), "load from Azure App Configuration failed after") && strings.Contains(err.Error(), "attempts within timeout"),
		"Error should indicate timeout or insufficient time: %v", err)
	assert.True(t, callCount >= 1, "Operation should be called at least once")
}
