// Copyright (c) Microsoft Corporation.
// Licensed under the MIT License.

package azureappconfiguration

import (
	"context"
	"fmt"
	"testing"
	"time"

	"github.com/Azure/AppConfiguration-GoProvider/azureappconfiguration/internal/refreshtimer"
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

func TestRefresh_AlreadyInProgress(t *testing.T) {
	// Setup a provider with refresh already in progress
	azappcfg := &AzureAppConfiguration{
		kvRefreshTimer:    &mockRefreshCondition{},
		refreshInProgress: true,
	}

	// Attempt to refresh
	err := azappcfg.Refresh(context.Background())

	// Verify no error and that we returned early
	assert.NoError(t, err)
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

func TestRefresh_NoChanges(t *testing.T) {
	// Setup mock clients
	mockTimer := &mockRefreshCondition{shouldRefresh: true}
	mockEtags := &mockETagsClient{changed: false}

	// Setup a provider
	azappcfg := &AzureAppConfiguration{
		kvRefreshTimer:         mockTimer,
		watchedSettingsMonitor: mockEtags,
	}

	// Attempt to refresh
	err := azappcfg.Refresh(context.Background())

	// Verify no error and that refresh was attempted but no changes were detected
	assert.NoError(t, err)
	assert.Equal(t, 1, mockEtags.checkCallCount)
	assert.True(t, mockTimer.resetCalled, "Timer should be reset even when no changes detected")
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

func TestRefresh_ErrorDuringETagCheck(t *testing.T) {
	// Setup mocks
	mockTimer := &mockRefreshCondition{shouldRefresh: true}
	mockEtags := &mockETagsClient{
		err: fmt.Errorf("etag check failed"),
	}

	// Setup provider
	azappcfg := &AzureAppConfiguration{
		kvRefreshTimer:         mockTimer,
		watchedSettingsMonitor: mockEtags,
	}

	// Attempt to refresh
	err := azappcfg.Refresh(context.Background())

	// Verify error and that timer was not reset
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "etag check failed")
	assert.False(t, mockTimer.resetCalled, "Timer should not be reset on error")
}

// Additional test to verify real RefreshTimer behavior
func TestRealRefreshTimer(t *testing.T) {
	// Create a real refresh timer with a short interval
	timer := refreshtimer.New(100 * time.Millisecond)

	// Initially it should not be time to refresh
	assert.False(t, timer.ShouldRefresh(), "New timer should not immediately indicate refresh needed")

	// After the interval passes, it should indicate time to refresh
	time.Sleep(110 * time.Millisecond)
	assert.True(t, timer.ShouldRefresh(), "Timer should indicate refresh needed after interval")

	// After reset, it should not be time to refresh again
	timer.Reset()
	assert.False(t, timer.ShouldRefresh(), "Timer should not indicate refresh needed right after reset")
}
