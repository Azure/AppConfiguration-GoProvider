// Copyright (c) Microsoft Corporation.
// Licensed under the MIT License.

package azureappconfiguration

import (
	"context"
	"errors"
	"net/http"
	"testing"
	"testing/synctest"
	"time"

	"github.com/Azure/azure-sdk-for-go/sdk/azcore"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"
)

func TestStartupWithRetry_MinimumFailureDelay(t *testing.T) {
	failure := &azcore.ResponseError{StatusCode: http.StatusBadRequest}
	tests := []struct {
		name              string
		operationDuration time.Duration
		operationError    error
		timeout           time.Duration
		expectedDuration  time.Duration
	}{
		{name: "immediate success"},
		{name: "slow success", operationDuration: 2 * time.Second, expectedDuration: 2 * time.Second},
		{name: "immediate failure with default timeout", operationError: failure, expectedDuration: 5 * time.Second},
		{name: "partially elapsed minimum", operationDuration: 2 * time.Second, operationError: failure, expectedDuration: 5 * time.Second},
		{name: "exactly elapsed minimum", operationDuration: 5 * time.Second, operationError: failure, expectedDuration: 5 * time.Second},
		{name: "already elapsed minimum", operationDuration: 7 * time.Second, operationError: failure, expectedDuration: 7 * time.Second},
		{name: "unrecognized failure", operationError: errors.New("secret resolution failed"), expectedDuration: 5 * time.Second},
		{
			name:             "insufficient time for retry",
			operationError:   &azcore.ResponseError{StatusCode: http.StatusServiceUnavailable},
			timeout:          3 * time.Second,
			expectedDuration: 3 * time.Second,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			synctest.Test(t, func(t *testing.T) {
				azappcfg := &AzureAppConfiguration{}
				attempts := 0
				start := time.Now()
				err := azappcfg.startupWithRetry(context.Background(), tt.timeout, func(context.Context) error {
					attempts++
					time.Sleep(tt.operationDuration)
					return tt.operationError
				})

				if tt.operationError == nil {
					require.NoError(t, err)
				} else {
					require.ErrorIs(t, err, tt.operationError)
				}
				assert.Equal(t, tt.expectedDuration, time.Since(start))
				assert.Equal(t, 1, attempts)
			})
		})
	}
}

func TestStartupWithRetry_MinimumFailureDelayAfterRetry(t *testing.T) {
	failure := &azcore.ResponseError{StatusCode: http.StatusBadRequest}
	tests := []struct {
		name       string
		finalError error
	}{
		{name: "success"},
		{name: "failure", finalError: failure},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			synctest.Test(t, func(t *testing.T) {
				azappcfg := &AzureAppConfiguration{}
				var attemptTimes []time.Duration
				start := time.Now()
				err := azappcfg.startupWithRetry(context.Background(), 20*time.Second, func(context.Context) error {
					attemptTimes = append(attemptTimes, time.Since(start))
					if len(attemptTimes) == 1 {
						return &azcore.ResponseError{StatusCode: http.StatusServiceUnavailable}
					}
					return tt.finalError
				})

				if tt.finalError == nil {
					require.NoError(t, err)
				} else {
					require.ErrorIs(t, err, tt.finalError)
				}
				assert.Equal(t, []time.Duration{0, 5 * time.Second}, attemptTimes)
				assert.Equal(t, 5*time.Second, time.Since(start))
			})
		})
	}
}

func TestStartupWithRetry_MinimumFailureDelayContext(t *testing.T) {
	tests := []struct {
		name             string
		startupTimeout   time.Duration
		callerTimeout    time.Duration
		cancelAfter      time.Duration
		cancelBefore     bool
		expectedDuration time.Duration
	}{
		{name: "already canceled", startupTimeout: 10 * time.Second, cancelBefore: true},
		{name: "canceled during delay", startupTimeout: 10 * time.Second, cancelAfter: 2 * time.Second, expectedDuration: 2 * time.Second},
		{name: "caller deadline", startupTimeout: 10 * time.Second, callerTimeout: 2 * time.Second, expectedDuration: 2 * time.Second},
		{name: "startup deadline", startupTimeout: 2 * time.Second, expectedDuration: 2 * time.Second},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			synctest.Test(t, func(t *testing.T) {
				ctx, cancel := context.WithCancel(context.Background())
				defer cancel()
				if tt.callerTimeout > 0 {
					var deadlineCancel context.CancelFunc
					ctx, deadlineCancel = context.WithTimeout(ctx, tt.callerTimeout)
					defer deadlineCancel()
				}
				if tt.cancelBefore {
					cancel()
				} else if tt.cancelAfter > 0 {
					timer := time.AfterFunc(tt.cancelAfter, cancel)
					defer timer.Stop()
				}

				azappcfg := &AzureAppConfiguration{}
				failure := &azcore.ResponseError{StatusCode: http.StatusBadRequest}
				attempts := 0
				start := time.Now()
				err := azappcfg.startupWithRetry(ctx, tt.startupTimeout, func(context.Context) error {
					attempts++
					return failure
				})

				require.ErrorIs(t, err, failure)
				assert.Equal(t, tt.expectedDuration, time.Since(start))
				assert.Equal(t, 1, attempts)
			})
		})
	}
}

func TestLoad_PreflightFailuresReturnImmediately(t *testing.T) {
	tests := []struct {
		name           string
		authentication AuthenticationOptions
		options        *Options
		errorMessage   string
	}{
		{
			name:         "authentication validation",
			errorMessage: "either connection string or endpoint and credential must be provided",
		},
		{
			name:           "options validation",
			authentication: AuthenticationOptions{ConnectionString: "invalid"},
			options:        &Options{Selectors: []Selector{{KeyFilter: "*", LabelFilter: "*"}}},
			errorMessage:   "label filter cannot contain",
		},
		{
			name:           "client construction",
			authentication: AuthenticationOptions{ConnectionString: "invalid"},
			errorMessage:   "failed to initialize configuration client",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			synctest.Test(t, func(t *testing.T) {
				start := time.Now()
				azappcfg, err := Load(context.Background(), tt.authentication, tt.options)

				require.ErrorContains(t, err, tt.errorMessage)
				assert.Nil(t, azappcfg)
				assert.Zero(t, time.Since(start))
			})
		})
	}
}

func TestRefresh_FailureDoesNotWaitForStartupDelay(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		failure := errors.New("failed to get clients")
		manager := new(mockClientManager)
		manager.On("getClients", mock.Anything).Return([]*configurationClientWrapper(nil), failure).Once()
		azappcfg := &AzureAppConfiguration{
			clientManager:  manager,
			kvRefreshTimer: &mockRefreshCondition{shouldRefresh: true},
		}

		start := time.Now()
		err := azappcfg.Refresh(context.Background())

		require.ErrorIs(t, err, failure)
		assert.Zero(t, time.Since(start))
		manager.AssertExpectations(t)
	})
}
