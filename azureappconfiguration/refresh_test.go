// Copyright (c) Microsoft Corporation.
// Licensed under the MIT License.

package azureappconfiguration

import (
	"context"
	"encoding/json"
	"fmt"
	"net/url"
	"sync"
	"testing"
	"time"

	"github.com/Azure/AppConfiguration-GoProvider/azureappconfiguration/internal/refresh"
	"github.com/Azure/azure-sdk-for-go/sdk/azcore"
	"github.com/Azure/azure-sdk-for-go/sdk/data/azappconfig"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"
)

// mockETagsClient implements the eTagsClient interface for testing
type mockETagsClient struct {
	changed        bool
	checkCallCount int
	err            error
}

func (m *mockETagsClient) checkIfETagChanged(ctx context.Context) (bool, error) {
	m.checkCallCount++
	if m.err != nil {
		return false, m.err
	}
	return m.changed, nil
}

// mockRefreshCondition implements the refreshtimer.RefreshCondition interface for testing
type mockRefreshCondition struct {
	shouldRefresh bool
	resetCalled   bool
}

func (m *mockRefreshCondition) ShouldRefresh() bool {
	return m.shouldRefresh
}

func (m *mockRefreshCondition) Reset() {
	m.resetCalled = true
}

func TestRefresh_NotConfigured(t *testing.T) {
	// Setup a provider with no refresh configuration
	azappcfg := &AzureAppConfiguration{}

	// Attempt to refresh
	err := azappcfg.Refresh(context.Background())

	// Verify that an error is returned
	require.Error(t, err)
	assert.Contains(t, err.Error(), "refresh is not enabled for either key values or Key Vault secrets")
}

func TestRefreshEnabled_IntervalTooShort(t *testing.T) {
	// Test verifying validation when refresh interval is too short
	options := &Options{
		RefreshOptions: KeyValueRefreshOptions{
			Enabled:  true,
			Interval: 500 * time.Millisecond, // Too short, should be at least minimalRefreshInterval
			WatchedSettings: []WatchedSetting{
				{Key: "test-key", Label: "test-label"},
			},
		},
	}

	// Verify error
	err := verifyOptions(options)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "key value refresh interval cannot be less than")
}

func TestRefreshEnabled_EmptyWatchedSettingKey(t *testing.T) {
	// Test verifying validation when a watched setting has an empty key
	options := &Options{
		RefreshOptions: KeyValueRefreshOptions{
			Enabled: true,
			WatchedSettings: []WatchedSetting{
				{Key: "", Label: "test-label"}, // Empty key should be rejected
			},
		},
	}

	// Verify error
	err := verifyOptions(options)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "watched setting key cannot be empty")
}

func TestRefreshEnabled_InvalidWatchedSettingKey(t *testing.T) {
	// Test verifying validation when watched setting keys contain invalid chars
	options := &Options{
		RefreshOptions: KeyValueRefreshOptions{
			Enabled: true,
			WatchedSettings: []WatchedSetting{
				{Key: "test*key", Label: "test-label"}, // Key contains wildcard, not allowed
			},
		},
	}

	// Verify error
	err := verifyOptions(options)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "watched setting key cannot contain")
}

func TestRefreshEnabled_InvalidWatchedSettingLabel(t *testing.T) {
	// Test verifying validation when watched setting labels contain invalid chars
	options := &Options{
		RefreshOptions: KeyValueRefreshOptions{
			Enabled: true,
			WatchedSettings: []WatchedSetting{
				{Key: "test-key", Label: "test*label"}, // Label contains wildcard, not allowed
			},
		},
	}

	// Verify error
	err := verifyOptions(options)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "watched setting label cannot contain")
}

func TestRefreshEnabled_ValidSettings(t *testing.T) {
	// Test verifying valid refresh options pass validation
	options := &Options{
		RefreshOptions: KeyValueRefreshOptions{
			Enabled:  true,
			Interval: 5 * time.Second, // Valid interval
			WatchedSettings: []WatchedSetting{
				{Key: "test-key-1", Label: "test-label-1"},
				{Key: "test-key-2", Label: ""}, // Empty label should be normalized later
			},
		},
	}

	// Verify no error
	err := verifyOptions(options)
	assert.NoError(t, err)
}

func TestNormalizedWatchedSettings(t *testing.T) {
	// Test the normalizedWatchedSettings function
	settings := []WatchedSetting{
		{Key: "key1", Label: "label1"},
		{Key: "key2", Label: ""}, // Empty label should be set to defaultLabel
	}

	normalized := normalizedWatchedSettings(settings)

	// Verify results
	assert.Len(t, normalized, 2)
	assert.Equal(t, "key1", normalized[0].Key)
	assert.Equal(t, "label1", normalized[0].Label)
	assert.Equal(t, "key2", normalized[1].Key)
	assert.Equal(t, defaultLabel, normalized[1].Label)
}

// Additional test to verify real RefreshTimer behavior
func TestRealRefreshTimer(t *testing.T) {
	// Create a real refresh timer with a short interval
	timer := refresh.NewTimer(100 * time.Millisecond)

	// Initially it should not be time to refresh
	assert.False(t, timer.ShouldRefresh(), "New timer should not immediately indicate refresh needed")

	// After the interval passes, it should indicate time to refresh
	time.Sleep(110 * time.Millisecond)
	assert.True(t, timer.ShouldRefresh(), "Timer should indicate refresh needed after interval")

	// After reset, it should not be time to refresh again
	timer.Reset()
	assert.False(t, timer.ShouldRefresh(), "Timer should not indicate refresh needed right after reset")
}

// mockKvRefreshClient implements the settingsClient interface for testing
type mockKvRefreshClient struct {
	settings     []azappconfig.Setting
	watchedETags map[WatchedSetting]*azcore.ETag
	getCallCount int
	err          error
}

func (m *mockKvRefreshClient) getSettings(ctx context.Context) (*settingsResponse, error) {
	m.getCallCount++
	if m.err != nil {
		return nil, m.err
	}
	return &settingsResponse{
		settings:     m.settings,
		watchedETags: m.watchedETags,
	}, nil
}

// TestRefreshKeyValues_NoChanges tests when no ETags change is detected
func TestRefreshKeyValues_NoChanges(t *testing.T) {
	// Setup mocks
	mockTimer := &mockRefreshCondition{shouldRefresh: true}
	mockMonitor := &mockETagsClient{changed: false}
	mockLoader := &mockKvRefreshClient{}
	mockSentinels := &mockKvRefreshClient{}

	mockClient := refreshClient{
		loader:    mockLoader,
		monitor:   mockMonitor,
		sentinels: mockSentinels,
	}

	// Setup provider
	azappcfg := &AzureAppConfiguration{
		kvRefreshTimer: mockTimer,
	}

	// Call refreshKeyValues
	refreshed, err := azappcfg.refreshKeyValues(context.Background(), mockClient)

	// Verify results
	assert.NoError(t, err)
	assert.False(t, refreshed, "Should return false when no changes detected")
	assert.Equal(t, 1, mockMonitor.checkCallCount, "Monitor should be called exactly once")
	assert.Equal(t, 0, mockLoader.getCallCount, "Loader should not be called when no changes")
	assert.Equal(t, 0, mockSentinels.getCallCount, "Sentinels should not be called when no changes")
	assert.True(t, mockTimer.resetCalled, "Timer should be reset even when no changes")
}

// TestRefreshKeyValues_ChangesDetected tests when ETags changed and reload succeeds
func TestRefreshKeyValues_ChangesDetected(t *testing.T) {
	// Setup mocks for successful refresh
	mockTimer := &mockRefreshCondition{shouldRefresh: true}
	mockMonitor := &mockETagsClient{changed: true}
	mockLoader := &mockKvRefreshClient{}
	mockSentinels := &mockKvRefreshClient{}

	mockClient := refreshClient{
		loader:    mockLoader,
		monitor:   mockMonitor,
		sentinels: mockSentinels,
	}

	// Setup provider with watchedSettings
	azappcfg := &AzureAppConfiguration{
		kvRefreshTimer:  mockTimer,
		watchedSettings: []WatchedSetting{{Key: "test", Label: "test"}},
	}

	// Call refreshKeyValues
	refreshed, err := azappcfg.refreshKeyValues(context.Background(), mockClient)

	// Verify results
	assert.NoError(t, err)
	assert.True(t, refreshed, "Should return true when changes detected and applied")
	assert.Equal(t, 1, mockMonitor.checkCallCount, "Monitor should be called exactly once")
	assert.Equal(t, 1, mockLoader.getCallCount, "Loader should be called when changes detected")
	assert.Equal(t, 1, mockSentinels.getCallCount, "Sentinels should be called when changes detected")
	assert.True(t, mockTimer.resetCalled, "Timer should be reset after successful refresh")
}

// TestRefreshKeyValues_LoaderError tests when loader client returns an error
func TestRefreshKeyValues_LoaderError(t *testing.T) {
	// Setup mocks with loader error
	mockTimer := &mockRefreshCondition{shouldRefresh: true}
	mockMonitor := &mockETagsClient{changed: true}
	mockLoader := &mockKvRefreshClient{err: fmt.Errorf("loader error")}
	mockSentinels := &mockKvRefreshClient{}

	mockClient := refreshClient{
		loader:    mockLoader,
		monitor:   mockMonitor,
		sentinels: mockSentinels,
	}

	// Setup provider
	azappcfg := &AzureAppConfiguration{
		kvRefreshTimer: mockTimer,
	}

	// Call refreshKeyValues
	refreshed, err := azappcfg.refreshKeyValues(context.Background(), mockClient)

	// Verify results
	assert.Error(t, err)
	assert.False(t, refreshed, "Should return false when error occurs")
	assert.Contains(t, err.Error(), "loader error")
	assert.Equal(t, 1, mockMonitor.checkCallCount, "Monitor should be called exactly once")
	assert.Equal(t, 1, mockLoader.getCallCount, "Loader should be called when changes detected")
	assert.False(t, mockTimer.resetCalled, "Timer should not be reset when error occurs")
}

// TestRefreshKeyValues_SentinelError tests when sentinel client returns an error
func TestRefreshKeyValues_SentinelError(t *testing.T) {
	// Setup mocks with sentinel error
	mockTimer := &mockRefreshCondition{shouldRefresh: true}
	mockMonitor := &mockETagsClient{changed: true}
	mockLoader := &mockKvRefreshClient{}
	mockSentinels := &mockKvRefreshClient{err: fmt.Errorf("sentinel error")}

	mockClient := refreshClient{
		loader:    mockLoader,
		monitor:   mockMonitor,
		sentinels: mockSentinels,
	}

	// Setup provider with watchedSettings to ensure sentinels are used
	azappcfg := &AzureAppConfiguration{
		kvRefreshTimer:  mockTimer,
		watchedSettings: []WatchedSetting{{Key: "test", Label: "test"}},
	}

	// Call refreshKeyValues
	refreshed, err := azappcfg.refreshKeyValues(context.Background(), mockClient)

	// Verify results
	assert.Error(t, err)
	assert.False(t, refreshed, "Should return false when error occurs")
	assert.Contains(t, err.Error(), "sentinel error")
	assert.Equal(t, 1, mockMonitor.checkCallCount, "Monitor should be called exactly once")
	assert.Equal(t, 1, mockLoader.getCallCount, "Loader should be called when changes detected")
	assert.Equal(t, 1, mockSentinels.getCallCount, "Sentinels should be called when changes detected")
	assert.False(t, mockTimer.resetCalled, "Timer should not be reset when error occurs")
}

// TestRefreshKeyValues_MonitorError tests when monitor client returns an error
func TestRefreshKeyValues_MonitorError(t *testing.T) {
	// Setup mocks with monitor error
	mockTimer := &mockRefreshCondition{shouldRefresh: true}
	mockMonitor := &mockETagsClient{err: fmt.Errorf("monitor error")}
	mockLoader := &mockKvRefreshClient{}
	mockSentinels := &mockKvRefreshClient{}

	mockClient := refreshClient{
		loader:    mockLoader,
		monitor:   mockMonitor,
		sentinels: mockSentinels,
	}

	// Setup provider
	azappcfg := &AzureAppConfiguration{
		kvRefreshTimer: mockTimer,
	}

	// Call refreshKeyValues
	refreshed, err := azappcfg.refreshKeyValues(context.Background(), mockClient)

	// Verify results
	assert.Error(t, err)
	assert.False(t, refreshed, "Should return false when error occurs")
	assert.Contains(t, err.Error(), "monitor error")
	assert.Equal(t, 1, mockMonitor.checkCallCount, "Monitor should be called exactly once")
	assert.Equal(t, 0, mockLoader.getCallCount, "Loader should not be called when monitor fails")
	assert.Equal(t, 0, mockSentinels.getCallCount, "Sentinels should not be called when monitor fails")
	assert.False(t, mockTimer.resetCalled, "Timer should not be reset when error occurs")
}

// TestRefresh_AlreadyInProgress tests the new atomic implementation of refresh status checking
func TestRefresh_AlreadyInProgress(t *testing.T) {
	// Setup a provider with refresh already in progress
	azappcfg := &AzureAppConfiguration{
		kvRefreshTimer: &mockRefreshCondition{},
	}

	// Manually set the refresh in progress flag
	azappcfg.refreshInProgress.Store(true)

	// Attempt to refresh
	err := azappcfg.Refresh(context.Background())

	// Verify no error and that we returned early
	assert.NoError(t, err)
}

func TestRefreshKeyVaultSecrets_WithMockResolver_Scenarios(t *testing.T) {
	// resolutionInstruction defines how a specific Key Vault URI should be resolved by the mock.
	type resolutionInstruction struct {
		Value string
		Err   error
	}

	tests := []struct {
		name        string
		description string // Optional: for more clarity

		// Initial state for AzureAppConfiguration
		initialTimer        refresh.Condition
		initialKeyVaultRefs map[string]string // map[appConfigKey]jsonURIString -> e.g., {"secretAppKey": `{"uri":"https://mykv.vault.azure.net/secrets/mysecret"}`}
		initialKeyValues    map[string]any    // map[appConfigKey]currentValue

		// Configuration for the mockSecretResolver
		// map[actualURIString]resolutionInstruction -> e.g., {"https://mykv.vault.azure.net/secrets/mysecret": {Value: "resolvedValue", Err: nil}}
		secretResolutionConfig map[string]resolutionInstruction

		// Expected outcomes
		expectedChanged        bool
		expectedErrSubstring   string // Substring of the error expected from refreshKeyVaultSecrets
		expectedTimerReset     bool
		expectedFinalKeyValues map[string]any
	}{
		{
			name:                   "Timer is nil",
			initialTimer:           nil,
			initialKeyVaultRefs:    map[string]string{"appSecret1": `{"uri":"https://kv.com/s/s1/"}`},
			initialKeyValues:       map[string]any{"appSecret1": "oldVal1"},
			expectedChanged:        false,
			expectedTimerReset:     false,
			expectedFinalKeyValues: map[string]any{"appSecret1": "oldVal1"},
		},
		{
			name:                   "Timer not expired",
			initialTimer:           &mockRefreshCondition{shouldRefresh: false},
			initialKeyVaultRefs:    map[string]string{"appSecret1": `{"uri":"https://kv.com/s/s1/"}`},
			initialKeyValues:       map[string]any{"appSecret1": "oldVal1"},
			expectedChanged:        false,
			expectedTimerReset:     false,
			expectedFinalKeyValues: map[string]any{"appSecret1": "oldVal1"},
		},
		{
			name:                   "No keyVaultRefs, timer ready",
			initialTimer:           &mockRefreshCondition{shouldRefresh: true},
			initialKeyVaultRefs:    map[string]string{},
			initialKeyValues:       map[string]any{"appKey": "appVal"},
			expectedChanged:        false,
			expectedTimerReset:     true,
			expectedFinalKeyValues: map[string]any{"appKey": "appVal"},
		},
		{
			name:                "Secrets not changed, timer ready",
			initialTimer:        &mockRefreshCondition{shouldRefresh: true},
			initialKeyVaultRefs: map[string]string{"appSecret1": `{"uri":"https://myvault.vault.azure.net/secrets/s1"}`},
			initialKeyValues:    map[string]any{"appSecret1": "currentVal", "appKey": "appVal"},
			secretResolutionConfig: map[string]resolutionInstruction{
				"https://myvault.vault.azure.net/secrets/s1": {Value: "currentVal"},
			},
			expectedChanged:        false,
			expectedTimerReset:     true,
			expectedFinalKeyValues: map[string]any{"appSecret1": "currentVal", "appKey": "appVal"},
		},
		{
			name:                "Secrets changed - existing secret updated, timer ready",
			initialTimer:        &mockRefreshCondition{shouldRefresh: true},
			initialKeyVaultRefs: map[string]string{"appSecret1": `{"uri":"https://myvault.vault.azure.net/secrets/s1"}`},
			initialKeyValues:    map[string]any{"appSecret1": "oldVal1", "appKey": "appVal"},
			secretResolutionConfig: map[string]resolutionInstruction{
				"https://myvault.vault.azure.net/secrets/s1": {Value: "newVal1"},
			},
			expectedChanged:    true,
			expectedTimerReset: true,
			expectedFinalKeyValues: map[string]any{
				"appSecret1": "newVal1",
				"appKey":     "appVal",
			},
		},
		{
			name:                "Secrets changed - mix of updated, unchanged, timer ready",
			initialTimer:        &mockRefreshCondition{shouldRefresh: true},
			initialKeyVaultRefs: map[string]string{"s1": `{"uri":"https://myvault.vault.azure.net/secrets/s1"}`, "s3": `{"uri":"https://myvault.vault.azure.net/secrets/s3"}`},
			initialKeyValues:    map[string]any{"s1": "oldVal1", "s3": "val3Unchanged", "appKey": "appVal"},
			secretResolutionConfig: map[string]resolutionInstruction{
				"https://myvault.vault.azure.net/secrets/s1": {Value: "newVal1"},
				"https://myvault.vault.azure.net/secrets/s3": {Value: "val3Unchanged"},
			},
			expectedChanged:    true,
			expectedTimerReset: true,
			expectedFinalKeyValues: map[string]any{
				"s1":     "newVal1",
				"s3":     "val3Unchanged",
				"appKey": "appVal",
			},
		},
	}

	ctx := context.Background()
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			// Setup
			currentKeyValues := make(map[string]any)
			if tc.initialKeyValues != nil {
				for k, v := range tc.initialKeyValues {
					currentKeyValues[k] = v
				}
			}

			mockResolver := new(mockSecretResolver)
			azappcfg := &AzureAppConfiguration{
				secretRefreshTimer: tc.initialTimer,
				keyVaultRefs:       tc.initialKeyVaultRefs,
				keyValues:          currentKeyValues,
				resolver: &keyVaultReferenceResolver{
					clients:        sync.Map{},
					secretResolver: mockResolver,
				},
			}

			if tc.initialKeyVaultRefs != nil && tc.secretResolutionConfig != nil {
				for _, jsonRefString := range tc.initialKeyVaultRefs {
					var kvRefInternal struct { // Re-declare locally or use the actual keyVaultReference type if accessible
						URI string `json:"uri"`
					}
					err := json.Unmarshal([]byte(jsonRefString), &kvRefInternal)
					if err != nil {
						continue
					}
					actualURIString := kvRefInternal.URI
					if actualURIString == "" {
						continue
					}

					if instruction, ok := tc.secretResolutionConfig[actualURIString]; ok {
						parsedURL, parseErr := url.Parse(actualURIString)
						require.NoError(t, parseErr, "Test setup: Failed to parse URI for mock expectation: %s", actualURIString)
						mockResolver.On("ResolveSecret", ctx, *parsedURL).Return(instruction.Value, instruction.Err).Once()
					}
				}
			}

			// Execute
			changed, err := azappcfg.refreshKeyVaultSecrets(context.Background())

			// Assert Error
			if tc.expectedErrSubstring != "" {
				require.Error(t, err, "Expected an error but got nil")
				assert.Contains(t, err.Error(), tc.expectedErrSubstring, "Error message mismatch")
			} else {
				require.NoError(t, err, "Expected no error but got: %v", err)
			}

			// Assert Changed Flag
			assert.Equal(t, tc.expectedChanged, changed, "Changed flag mismatch")

			// Assert Timer Reset
			if mockTimer, ok := tc.initialTimer.(*mockRefreshCondition); ok {
				assert.Equal(t, tc.expectedTimerReset, mockTimer.resetCalled, "Timer reset state mismatch")
			} else if tc.initialTimer == nil {
				assert.False(t, tc.expectedTimerReset, "Timer was nil, reset should not be expected")
			}

			// Assert Final KeyValues
			assert.Equal(t, tc.expectedFinalKeyValues, azappcfg.keyValues, "Final keyValues mismatch")

			// Verify mock expectations
			mockResolver.AssertExpectations(t)
		})
	}
}

func TestRefresh_SettingsUpdated_WatchAll(t *testing.T) {
	// Create initial cached values
	initialKeyValues := map[string]any{
		"setting1": "initial-value1",
		"setting2": "initial-value2",
		"setting3": "value-unchanged",
	}

	// Set up mock etags client that will detect changes
	mockETags := &mockETagsClient{
		changed: true, // Simulate that etags have changed
	}

	// Set up mock settings client that will return updated values
	mockSettings := new(mockSettingsClient)
	updatedValue1 := "updated-value1"
	updatedValue2 := "new-value"
	mockResponse := &settingsResponse{
		settings: []azappconfig.Setting{
			{Key: toPtr("setting1"), Value: &updatedValue1, ContentType: toPtr("")},
			{Key: toPtr("setting3"), Value: toPtr("value-unchanged"), ContentType: toPtr("")},
			{Key: toPtr("setting4"), Value: &updatedValue2, ContentType: toPtr("")}, // New setting
			// Note: setting2 is missing - will be removed
		},
	}
	mockSettings.On("getSettings", mock.Anything).Return(mockResponse, nil)

	// Create refresh client wrapping the mocks
	mockRefreshClient := refreshClient{
		monitor: mockETags,
		loader:  mockSettings,
	}

	// Set up AzureAppConfiguration with initial values and refresh capabilities
	azappcfg := &AzureAppConfiguration{
		keyValues:      make(map[string]any),
		kvRefreshTimer: &mockRefreshCondition{shouldRefresh: true},
		watchAll:       true, // Enable watching all settings
	}

	// Copy initial values
	for k, v := range initialKeyValues {
		azappcfg.keyValues[k] = v
	}

	// Call Refresh
	changed, err := azappcfg.refreshKeyValues(context.Background(), mockRefreshClient)

	// Verify results
	require.NoError(t, err)
	assert.True(t, changed, "Expected cache to be updated")

	// Verify cache was updated correctly
	assert.Equal(t, "updated-value1", *azappcfg.keyValues["setting1"].(*string), "Setting1 should be updated")
	assert.Equal(t, "value-unchanged", *azappcfg.keyValues["setting3"].(*string), "Setting3 should remain unchanged")
	assert.Equal(t, "new-value", *azappcfg.keyValues["setting4"].(*string), "Setting4 should be added")

	// Verify setting2 was removed
	_, exists := azappcfg.keyValues["setting2"]
	assert.False(t, exists, "Setting2 should be removed")

	// Verify mocks were called as expected
	mockSettings.AssertExpectations(t)
	assert.Equal(t, 1, mockETags.checkCallCount, "ETag check should be called once")
}

// TestRefreshKeyValues_NoChanges tests when no ETags change is detected
func TestRefreshKeyValues_NoChanges_WatchAll(t *testing.T) {
	// Setup mocks
	mockTimer := &mockRefreshCondition{shouldRefresh: true}
	mockMonitor := &mockETagsClient{changed: false}
	mockLoader := &mockKvRefreshClient{}

	mockClient := refreshClient{
		loader:  mockLoader,
		monitor: mockMonitor,
	}

	// Setup provider
	azappcfg := &AzureAppConfiguration{
		kvRefreshTimer: mockTimer,
		watchAll:       true,
	}

	// Call refreshKeyValues
	refreshed, err := azappcfg.refreshKeyValues(context.Background(), mockClient)

	// Verify results
	assert.NoError(t, err)
	assert.False(t, refreshed, "Should return false when no changes detected")
	assert.Equal(t, 1, mockMonitor.checkCallCount, "Monitor should be called exactly once")
	assert.Equal(t, 0, mockLoader.getCallCount, "Loader should not be called when no changes")
	assert.True(t, mockTimer.resetCalled, "Timer should be reset even when no changes")
}
