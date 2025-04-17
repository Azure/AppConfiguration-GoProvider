// Copyright (c) Microsoft Corporation.
// Licensed under the MIT License.

package azureappconfiguration

import (
	"context"
	"fmt"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/Azure/AppConfiguration-GoProvider/azureappconfiguration/internal/refresh"
	"github.com/Azure/azure-sdk-for-go/sdk/azcore"
	"github.com/Azure/azure-sdk-for-go/sdk/data/azappconfig"
	"github.com/stretchr/testify/assert"
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
	assert.Contains(t, err.Error(), "refresh is not configured")
}

func TestRefresh_NotTimeToRefresh(t *testing.T) {
	// Setup a provider with a timer that indicates it's not time to refresh
	mockTimer := &mockRefreshCondition{shouldRefresh: false}
	azappcfg := &AzureAppConfiguration{
		kvRefreshTimer: mockTimer,
	}

	// Attempt to refresh
	err := azappcfg.Refresh(context.Background())

	// Verify no error and that we returned early
	assert.NoError(t, err)
	// Timer should not be reset if we're not refreshing
	assert.False(t, mockTimer.resetCalled)
}

func TestRefreshEnabled_EmptyWatchedSettings(t *testing.T) {
	// Test verifying validation when refresh is enabled but no watched settings
	options := &Options{
		RefreshOptions: KeyValueRefreshOptions{
			Enabled:         true, // Enabled but without watched settings
			WatchedSettings: []WatchedSetting{},
		},
	}

	// Verify error
	err := verifyOptions(options)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "watched settings cannot be empty")
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
	mockTimer := &mockRefreshCondition{}
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
	mockTimer := &mockRefreshCondition{}
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
	mockTimer := &mockRefreshCondition{}
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
	mockTimer := &mockRefreshCondition{}
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
	mockTimer := &mockRefreshCondition{}
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

// TestRefresh_ConcurrentCalls tests calling Refresh concurrently from multiple goroutines
func TestRefresh_ConcurrentCalls(t *testing.T) {
	// Skip in short mode as race detector makes it slower
	if testing.Short() {
		t.Skip("Skipping concurrent refresh test in short mode")
	}

	// Setup mock components
	mockTimer := refresh.NewTimer(100 * time.Millisecond)
	time.Sleep(100 * time.Millisecond) // Ensure timer is set to refresh
	mockMonitor := &mockETagsClient{changed: true}
	mockLoader := &mockKvRefreshClient{}
	mockSentinels := &mockKvRefreshClient{}

	// Track actual refresh operations
	refreshCount := int32(0)

	// Create a provider with the components needed for refresh
	azappcfg := &AzureAppConfiguration{
		kvRefreshTimer:  mockTimer,
		watchedSettings: []WatchedSetting{{Key: "test", Label: "test"}},
		sentinelETags:   make(map[WatchedSetting]*azcore.ETag),
		onRefreshSuccess: []func(){
			func() {
				// Count each successful refresh
				atomic.AddInt32(&refreshCount, 1)
			},
		},
	}

	// Override the newKvRefreshClient method to return our mocks
	originalNewMethod := azappcfg.newKvRefreshClient
	azappcfg.newKvRefreshClient = func() refreshClient {
		return refreshClient{
			loader:    mockLoader,
			monitor:   mockMonitor,
			sentinels: mockSentinels,
		}
	}
	defer func() {
		// Restore original method after test
		if originalNewMethod != nil {
			azappcfg.newKvRefreshClient = originalNewMethod
		}
	}()

	// Number of concurrent goroutines to launch
	const concurrentCalls = 10

	// Use a wait group to ensure all goroutines complete
	var wg sync.WaitGroup
	wg.Add(concurrentCalls)

	// Launch multiple goroutines to call Refresh concurrently
	for i := 0; i < concurrentCalls; i++ {
		go func(idx int) {
			defer wg.Done()

			// Call Refresh with a small delay between calls to increase chance of concurrency
			time.Sleep(time.Millisecond * time.Duration(idx))
			err := azappcfg.Refresh(context.Background())

			// Each call should succeed without error
			assert.NoError(t, err, "Refresh call %d should not return error", idx)
		}(i)
	}

	// Wait for all goroutines to complete
	wg.Wait()

	// Only one refresh operation should actually complete successfully
	// Since refreshInProgress prevents multiple refreshes
	assert.Equal(t, int32(1), refreshCount, "Only one refresh operation should have executed")
}

// TestRefresh_SequentialCalls tests multiple sequential calls to Refresh
func TestRefresh_SequentialCalls(t *testing.T) {
	// Setup mock components
	mockTimer := &mockRefreshCondition{shouldRefresh: true}
	mockMonitor := &mockETagsClient{changed: true}
	mockLoader := &mockKvRefreshClient{}
	mockSentinels := &mockKvRefreshClient{}

	// Track actual refresh operations
	refreshCount := int32(0)

	// Create a provider with the components needed for refresh
	azappcfg := &AzureAppConfiguration{
		kvRefreshTimer:  mockTimer,
		watchedSettings: []WatchedSetting{{Key: "test", Label: "test"}},
		sentinelETags:   make(map[WatchedSetting]*azcore.ETag),
		onRefreshSuccess: []func(){
			func() {
				// Count each successful refresh
				atomic.AddInt32(&refreshCount, 1)
			},
		},
	}

	// Override the newKvRefreshClient method to return our mocks
	azappcfg.newKvRefreshClient = func() refreshClient {
		return refreshClient{
			loader:    mockLoader,
			monitor:   mockMonitor,
			sentinels: mockSentinels,
		}
	}

	// First call should perform a refresh
	err1 := azappcfg.Refresh(context.Background())
	assert.NoError(t, err1)
	assert.Equal(t, int32(1), refreshCount)

	// Reset the refreshInProgress flag to simulate completion
	azappcfg.refreshInProgress.Store(false)

	// Second call should also perform a refresh
	err2 := azappcfg.Refresh(context.Background())
	assert.NoError(t, err2)
	assert.Equal(t, int32(2), refreshCount)
}
