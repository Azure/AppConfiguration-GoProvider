package azureappconfiguration

import (
	"context"
	"fmt"
	"time"

	"github.com/Azure/azure-sdk-for-go/sdk/azcore/to"
	"github.com/Azure/azure-sdk-for-go/sdk/data/azappconfig"
)

// Refresh refreshes the configuration from the Azure App Configuration store.
// This can be triggered on demand or by a scheduled task.
func (cfg *AzureAppConfiguration) Refresh(ctx context.Context) error {
	// Skip if refresh is not enabled
	if cfg.options.RefreshOptions == nil || !cfg.options.RefreshOptions.Enabled {
		return nil
	}

	// Check if it's time to refresh based on the interval
	if !cfg.shouldRefresh() {
		return nil
	}

	// If watching specific settings, check if any have changed
	if len(cfg.options.RefreshOptions.WatchedSettings) > 0 {
		changed, err := cfg.haveWatchedSettingsChanged(ctx)
		if err != nil {
			return fmt.Errorf("failed to check watched settings: %w", err)
		}

		if !changed {
			// No change detected, update last sync time and return
			cfg.mutex.Lock()
			cfg.lastSyncTime = time.Now()
			cfg.mutex.Unlock()
			return nil
		}
	}

	// Reload all settings
	if err := cfg.loadSettings(ctx); err != nil {
		return fmt.Errorf("failed to refresh settings: %w", err)
	}

	// Call refresh callback if registered
	if cfg.refreshCallback != nil {
		cfg.refreshCallback()
	}

	return nil
}

// OnRefreshSuccess registers a callback function to be called when configurations change.
func (cfg *AzureAppConfiguration) OnRefreshSuccess(callback func()) {
	cfg.mutex.Lock()
	defer cfg.mutex.Unlock()

	cfg.refreshCallback = callback
}

// shouldRefresh checks if it's time to refresh the configuration.
func (cfg *AzureAppConfiguration) shouldRefresh() bool {
	cfg.mutex.RLock()
	defer cfg.mutex.RUnlock()

	if cfg.options.RefreshOptions == nil {
		return false
	}

	// Get refresh interval with default fallback
	interval := getRefreshIntervalWithDefault(cfg.options.RefreshOptions.Interval)

	// Check if enough time has passed since the last sync
	return time.Since(cfg.lastSyncTime) >= interval
}

// haveWatchedSettingsChanged checks if any of the watched settings have changed.
func (cfg *AzureAppConfiguration) haveWatchedSettingsChanged(ctx context.Context) (bool, error) {
	cfg.mutex.RLock()
	defer cfg.mutex.RUnlock()

	// Get the list of watched settings
	watchedSettings := cfg.options.RefreshOptions.WatchedSettings
	if len(watchedSettings) == 0 {
		return false, nil
	}

	// Check each watched setting
	for _, watched := range watchedSettings {
		// Get the current value from the App Configuration store
		options := &azappconfig.GetSettingOptions{
			Label: to.Ptr(watched.Label),
		}
		setting, err := cfg.client.GetSetting(ctx, watched.Key, options)
		if err != nil {
			return false, fmt.Errorf("failed to get watched setting %s/%s: %w", watched.Key, watched.Label, err)
		}

		// Compare ETag with the stored setting
		storedKey := watched.Key
		if setting.ETag != nil {
			// Check if this setting exists in our cached settings
			if stored, ok := cfg.settings[storedKey]; ok {
				// If ETags are different, a change has occurred
				if stored.ETag == nil || *stored.ETag != *setting.ETag {
					return true, nil
				}
			} else {
				// If we don't have this setting in cache, consider it a change
				return true, nil
			}
		}
	}

	return false, nil
}
