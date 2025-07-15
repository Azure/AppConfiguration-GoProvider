// Copyright (c) Microsoft Corporation.
// Licensed under the MIT License.

package azureappconfiguration

import (
	"context"
	"errors"
	"fmt"
	"net"
	"net/http"
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
	duration := client.getBackoffDuration()
	assert.True(t, duration >= minBackoffDuration)
	assert.True(t, duration <= minBackoffDuration*2) // Account for jitter

	// Multiple failures should increase duration exponentially
	client.failedAttempts = 3
	duration3 := client.getBackoffDuration()
	assert.True(t, duration3 > duration)

	// Very high failure count should be capped at max duration
	client.failedAttempts = 100
	durationMax := client.getBackoffDuration()
	assert.True(t, durationMax <= maxBackoffDuration*2) // Account for jitter
}
