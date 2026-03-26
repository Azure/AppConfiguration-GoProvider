// Copyright (c) Microsoft Corporation.
// Licensed under the MIT License.

package azureappconfiguration

import (
	"context"
	"testing"

	"github.com/Azure/azure-sdk-for-go/sdk/azcore"
	"github.com/Azure/azure-sdk-for-go/sdk/azcore/to"
	"github.com/Azure/azure-sdk-for-go/sdk/data/azappconfig/v2"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
)

func TestLoadKeyValues_WithSnapshot_Success(t *testing.T) {
	// Create mock client
	mockClient := &mockSettingsClient{}

	// Create string variables for the test values
	appName := "MyApp"
	appVersion := "1.0.0"
	dbHost := "localhost"

	// Mock settings response for snapshot
	settings := []azappconfig.Setting{
		{
			Key:   to.Ptr("app:name"),
			Value: &appName,
			ETag:  to.Ptr(azcore.ETag("test-etag-1")),
		},
		{
			Key:   to.Ptr("app:version"),
			Value: &appVersion,
			ETag:  to.Ptr(azcore.ETag("test-etag-2")),
		},
		{
			Key:   to.Ptr("database:host"),
			Value: &dbHost,
			ETag:  to.Ptr(azcore.ETag("test-etag-3")),
		},
	}

	mockClient.On("getSettings", mock.Anything).Return(&settingsResponse{
		settings: settings,
	}, nil)

	// Create app configuration with snapshot selector
	azappcfg := &AzureAppConfiguration{
		keyValues: make(map[string]any),
		kvSelectors: []Selector{
			{SnapshotName: "test-snapshot"},
		},
	}

	// Load key values
	err := azappcfg.loadKeyValues(context.Background(), mockClient)

	// Verify results
	assert.NoError(t, err)
	assert.Equal(t, &appName, azappcfg.keyValues["app:name"])
	assert.Equal(t, &appVersion, azappcfg.keyValues["app:version"])
	assert.Equal(t, &dbHost, azappcfg.keyValues["database:host"])

	// Verify that mock was called
	mockClient.AssertExpectations(t)
}

func TestLoadKeyValues_WithSnapshot_MixedWithRegularSelectors(t *testing.T) {
	// Create mock client that will be called twice (once for snapshot, once for regular selector)
	mockClient := &mockSettingsClient{}

	// Create string variables for the test values
	value1 := "value1"
	value2 := "value2"

	// First call for snapshot
	snapshotSettings := []azappconfig.Setting{
		{
			Key:   toPtr("snapshot:key1"),
			Value: &value1,
			ETag:  to.Ptr(azcore.ETag("test-etag-1")),
		},
	}

	// Second call for regular selector
	regularSettings := []azappconfig.Setting{
		{
			Key:   toPtr("regular:key2"),
			Value: &value2,
			ETag:  to.Ptr(azcore.ETag("test-etag-2")),
		},
	}

	// Set up sequential mock calls
	mockClient.On("getSettings", mock.Anything).Return(&settingsResponse{
		settings: append(snapshotSettings, regularSettings...),
	}, nil).Once()

	// Create app configuration with mixed selectors
	azappcfg := &AzureAppConfiguration{
		keyValues: make(map[string]any),
		kvSelectors: []Selector{
			{SnapshotName: "test-snapshot"},
			{KeyFilter: "regular*", LabelFilter: "prod"},
		},
	}

	// Load key values
	err := azappcfg.loadKeyValues(context.Background(), mockClient)

	// Verify results
	assert.NoError(t, err)
	assert.Equal(t, &value1, azappcfg.keyValues["snapshot:key1"])
	assert.Equal(t, &value2, azappcfg.keyValues["regular:key2"])

	// Verify that mock was called
	mockClient.AssertExpectations(t)
}

func TestLoadFeatureFlags_WithSnapshot(t *testing.T) {
	// Create mock client
	mockClient := &mockSettingsClient{}

	// Mock feature flag from snapshot - create as string variable
	featureFlagJson := `{
		"id": "SnapshotFeature",
		"description": "Feature from snapshot",
		"enabled": true,
		"conditions": {
			"client_filters": []
		}
	}`

	settings := []azappconfig.Setting{
		{
			Key:         toPtr(".appconfig.featureflag/SnapshotFeature"),
			Value:       &featureFlagJson,
			ContentType: toPtr("application/vnd.microsoft.appconfig.ff+json;charset=utf-8"),
			ETag:        to.Ptr(azcore.ETag("test-etag-1")),
		},
	}

	mockClient.On("getSettings", mock.Anything).Return(&settingsResponse{
		settings: settings,
	}, nil)

	// Create app configuration with feature flags enabled and snapshot selector
	azappcfg := &AzureAppConfiguration{
		featureFlags: make(map[string]any),
		ffEnabled:    true,
		ffSelectors: []Selector{
			{SnapshotName: "feature-snapshot"},
		},
	}

	// Load feature flags
	err := azappcfg.loadFeatureFlags(context.Background(), mockClient)

	// Verify results
	assert.NoError(t, err)

	// Verify feature management structure is created correctly
	featureManagement, exists := azappcfg.featureFlags["feature_management"]
	assert.True(t, exists)

	featureManagementMap, ok := featureManagement.(map[string]any)
	assert.True(t, ok)

	// Verify feature_flags array exists
	featureFlagsArray, exists := featureManagementMap["feature_flags"]
	assert.True(t, exists)

	// Verify we have 1 feature flag
	flags, ok := featureFlagsArray.([]any)
	assert.True(t, ok)
	assert.Len(t, flags, 1)

	// Verify the feature flag is properly unmarshaled
	flag, ok := flags[0].(map[string]any)
	assert.True(t, ok)
	assert.Equal(t, "SnapshotFeature", flag["id"])
	assert.Equal(t, "Feature from snapshot", flag["description"])
	assert.Equal(t, true, flag["enabled"])

	// Verify that mock was called
	mockClient.AssertExpectations(t)
}

func TestLoadSnapshot_MixedContent_OnlyKeyValuesWithoutFeatureFlagsEnabled(t *testing.T) {
	// Create mock client
	mockClient := &mockSettingsClient{}

	// Create string variables for the test values
	appName := "MyApp"
	appVersion := "1.0.0"
	dbHost := "localhost"
	featureFlagValue := `{"id": "MyFeature", "enabled": true, "conditions": {"client_filters": []}}`

	// Mock snapshot that contains both key values and feature flags
	settings := []azappconfig.Setting{
		// Regular key values
		{
			Key:   toPtr("app:name"),
			Value: &appName,
			ETag:  to.Ptr(azcore.ETag("test-etag-1")),
		},
		{
			Key:   toPtr("app:version"),
			Value: &appVersion,
			ETag:  to.Ptr(azcore.ETag("test-etag-2")),
		},
		// Feature flag (should be filtered out when feature flags are not enabled)
		{
			Key:         toPtr(".appconfig.featureflag/MyFeature"),
			Value:       &featureFlagValue,
			ContentType: toPtr("application/vnd.microsoft.appconfig.ff+json;charset=utf-8"),
			ETag:        to.Ptr(azcore.ETag("test-etag-3")),
		},
		// Another regular key value
		{
			Key:   toPtr("database:host"),
			Value: &dbHost,
			ETag:  to.Ptr(azcore.ETag("test-etag-4")),
		},
	}

	mockClient.On("getSettings", mock.Anything).Return(&settingsResponse{
		settings: settings,
	}, nil)

	// Create app configuration with snapshot selector but WITHOUT feature flags enabled
	azappcfg := &AzureAppConfiguration{
		keyValues:    make(map[string]any),
		featureFlags: make(map[string]any),
		kvSelectors: []Selector{
			{SnapshotName: "mixed-snapshot"},
		},
		ffEnabled: false, // Feature flags are NOT enabled
	}

	// Load key values
	err := azappcfg.loadKeyValues(context.Background(), mockClient)

	// Verify results
	assert.NoError(t, err)

	// Verify that only key values are loaded, not feature flags
	assert.Equal(t, &appName, azappcfg.keyValues["app:name"])
	assert.Equal(t, &appVersion, azappcfg.keyValues["app:version"])
	assert.Equal(t, &dbHost, azappcfg.keyValues["database:host"])

	// Verify that feature flag key is NOT loaded as a regular key value
	assert.NotContains(t, azappcfg.keyValues, ".appconfig.featureflag/MyFeature")

	// Verify that feature flags map remains empty since feature flags are not enabled
	assert.Empty(t, azappcfg.featureFlags)

	// Verify that mock was called
	mockClient.AssertExpectations(t)
}

func TestLoadSnapshot_MixedContent_FeatureFlagsEnabledWithDifferentSelectors(t *testing.T) {
	// Create two mock clients - one for key values, one for feature flags
	kvMockClient := &mockSettingsClient{}
	ffMockClient := &mockSettingsClient{}

	// Create string variables for test values
	appName := "MyApp"
	appVersion := "1.0.0"
	featureFlagValue := `{"id": "FeatureFromDifferentSnapshot", "enabled": true, "conditions": {"client_filters": []}}`

	// Mock key values from snapshot (excluding feature flags)
	kvSettings := []azappconfig.Setting{
		{
			Key:   toPtr("app:name"),
			Value: &appName,
			ETag:  to.Ptr(azcore.ETag("test-etag-1")),
		},
		{
			Key:   toPtr("app:version"),
			Value: &appVersion,
			ETag:  to.Ptr(azcore.ETag("test-etag-2")),
		},
	}

	// Mock feature flags from a different snapshot
	ffSettings := []azappconfig.Setting{
		{
			Key:         toPtr(".appconfig.featureflag/FeatureFromDifferentSnapshot"),
			Value:       &featureFlagValue,
			ContentType: toPtr("application/vnd.microsoft.appconfig.ff+json;charset=utf-8"),
			ETag:        to.Ptr(azcore.ETag("test-etag-3")),
		},
	}

	kvMockClient.On("getSettings", mock.Anything).Return(&settingsResponse{
		settings: kvSettings,
	}, nil)

	ffMockClient.On("getSettings", mock.Anything).Return(&settingsResponse{
		settings: ffSettings,
	}, nil)

	// Create app configuration with different snapshot selectors for key values and feature flags
	azappcfg := &AzureAppConfiguration{
		keyValues:    make(map[string]any),
		featureFlags: make(map[string]any),
		kvSelectors: []Selector{
			{SnapshotName: "keyvalue-snapshot"},
		},
		ffEnabled: true,
		ffSelectors: []Selector{
			{SnapshotName: "featureflag-snapshot"},
		},
	}

	// Load key values and feature flags separately
	err := azappcfg.loadKeyValues(context.Background(), kvMockClient)
	assert.NoError(t, err)

	err = azappcfg.loadFeatureFlags(context.Background(), ffMockClient)
	assert.NoError(t, err)

	// Verify results
	// Key values should be loaded from keyvalue-snapshot
	assert.Equal(t, &appName, azappcfg.keyValues["app:name"])
	assert.Equal(t, &appVersion, azappcfg.keyValues["app:version"])

	// Feature flags should be loaded from featureflag-snapshot
	featureManagement, exists := azappcfg.featureFlags["feature_management"]
	assert.True(t, exists)

	featureManagementMap, ok := featureManagement.(map[string]any)
	assert.True(t, ok)

	featureFlagsArray, exists := featureManagementMap["feature_flags"]
	assert.True(t, exists)

	flags, ok := featureFlagsArray.([]any)
	assert.True(t, ok)
	assert.Len(t, flags, 1)

	flag, ok := flags[0].(map[string]any)
	assert.True(t, ok)
	assert.Equal(t, "FeatureFromDifferentSnapshot", flag["id"])

	// Verify that both mocks were called
	kvMockClient.AssertExpectations(t)
	ffMockClient.AssertExpectations(t)
}

func TestLoadSnapshot_MixedContent_KeyValueSelectorsIgnoreFeatureFlags(t *testing.T) {
	// Create mock client
	mockClient := &mockSettingsClient{}

	// Create string variables for test values
	appName := "MyApp"
	configTimeout := "30"
	dbPort := "5432"
	feature1Value := `{"id": "Feature1", "enabled": true, "conditions": {"client_filters": []}}`
	feature2Value := `{"id": "Feature2", "enabled": false, "conditions": {"client_filters": []}}`

	// Mock snapshot containing mixed content
	settings := []azappconfig.Setting{
		// Regular key values
		{
			Key:   toPtr("app:name"),
			Value: &appName,
			ETag:  to.Ptr(azcore.ETag("test-etag-1")),
		},
		{
			Key:   toPtr("config:timeout"),
			Value: &configTimeout,
			ETag:  to.Ptr(azcore.ETag("test-etag-2")),
		},
		// Feature flags that should be ignored by key value loading
		{
			Key:         toPtr(".appconfig.featureflag/Feature1"),
			Value:       &feature1Value,
			ContentType: toPtr("application/vnd.microsoft.appconfig.ff+json;charset=utf-8"),
			ETag:        to.Ptr(azcore.ETag("test-etag-3")),
		},
		{
			Key:         toPtr(".appconfig.featureflag/Feature2"),
			Value:       &feature2Value,
			ContentType: toPtr("application/vnd.microsoft.appconfig.ff+json;charset=utf-8"),
			ETag:        to.Ptr(azcore.ETag("test-etag-4")),
		},
		// Another regular key value
		{
			Key:   toPtr("database:port"),
			Value: &dbPort,
			ETag:  to.Ptr(azcore.ETag("test-etag-5")),
		},
	}

	mockClient.On("getSettings", mock.Anything).Return(&settingsResponse{
		settings: settings,
	}, nil)

	// Create app configuration with snapshot selector for key values only
	azappcfg := &AzureAppConfiguration{
		keyValues:    make(map[string]any),
		featureFlags: make(map[string]any),
		kvSelectors: []Selector{
			{SnapshotName: "mixed-content-snapshot"},
		},
		ffEnabled: false, // Feature flags are disabled
	}

	// Load key values
	err := azappcfg.loadKeyValues(context.Background(), mockClient)

	// Verify results
	assert.NoError(t, err)

	// Verify that only non-feature-flag key values are loaded
	assert.Equal(t, &appName, azappcfg.keyValues["app:name"])
	assert.Equal(t, &configTimeout, azappcfg.keyValues["config:timeout"])
	assert.Equal(t, &dbPort, azappcfg.keyValues["database:port"])

	// Verify that feature flag keys are NOT loaded as regular key values
	assert.NotContains(t, azappcfg.keyValues, ".appconfig.featureflag/Feature1")
	assert.NotContains(t, azappcfg.keyValues, ".appconfig.featureflag/Feature2")

	// Verify that feature flags map remains empty
	assert.Empty(t, azappcfg.featureFlags)

	// Verify the total number of loaded key values (should be 3, not 5)
	assert.Len(t, azappcfg.keyValues, 3)

	// Verify that mock was called
	mockClient.AssertExpectations(t)
}

// --- Snapshot Reference Tests ---

func TestParseSnapshotReference(t *testing.T) {
	t.Run("valid reference", func(t *testing.T) {
		name, err := parseSnapshotReference(`{"snapshot_name":"my-snapshot"}`)
		assert.NoError(t, err)
		assert.Equal(t, "my-snapshot", name)
	})

	t.Run("empty snapshot name", func(t *testing.T) {
		_, err := parseSnapshotReference(`{"snapshot_name":""}`)
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "snapshot_name is empty")
	})

	t.Run("invalid JSON", func(t *testing.T) {
		_, err := parseSnapshotReference(`invalid json`)
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "failed to parse snapshot reference")
	})

	t.Run("missing snapshot_name field", func(t *testing.T) {
		_, err := parseSnapshotReference(`{"other_field":"value"}`)
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "snapshot_name is empty")
	})
}

func TestLoadSettingsFromSnapshotRefs_ResolvesReference(t *testing.T) {
	settingValue := "value from snapshot"
	mockLoader := snapshotSettingsLoader(func(ctx context.Context, snapshotName string) ([]azappconfig.Setting, error) {
		assert.Equal(t, "my-snapshot", snapshotName)
		return []azappconfig.Setting{
			{
				Key:   toPtr("key1"),
				Value: &settingValue,
			},
		}, nil
	})

	azappcfg := &AzureAppConfiguration{}
	snapshotRefs := map[string]string{
		"ref1": `{"snapshot_name":"my-snapshot"}`,
	}
	kvSettings := make(map[string]any)
	keyVaultRefs := make(map[string]string)

	err := azappcfg.loadSettingsFromSnapshotRefs(context.Background(), mockLoader, snapshotRefs, kvSettings, keyVaultRefs)
	assert.NoError(t, err)
	assert.Equal(t, &settingValue, kvSettings["key1"])
}

func TestLoadSettingsFromSnapshotRefs_OverridesExistingKeys(t *testing.T) {
	snapshotValue := "value from snapshot"
	mockLoader := snapshotSettingsLoader(func(ctx context.Context, snapshotName string) ([]azappconfig.Setting, error) {
		return []azappconfig.Setting{
			{
				Key:   toPtr("key1"),
				Value: &snapshotValue,
			},
		}, nil
	})

	azappcfg := &AzureAppConfiguration{}
	snapshotRefs := map[string]string{
		"ref1": `{"snapshot_name":"my-snapshot"}`,
	}
	originalValue := "original value"
	kvSettings := map[string]any{
		"key1": &originalValue,
	}
	keyVaultRefs := make(map[string]string)

	err := azappcfg.loadSettingsFromSnapshotRefs(context.Background(), mockLoader, snapshotRefs, kvSettings, keyVaultRefs)
	assert.NoError(t, err)
	assert.Equal(t, &snapshotValue, kvSettings["key1"])
}

func TestLoadSettingsFromSnapshotRefs_IgnoresFeatureFlags(t *testing.T) {
	ffValue := `{"id":"Feature1","enabled":true}`
	regularValue := "regular value"
	mockLoader := snapshotSettingsLoader(func(ctx context.Context, snapshotName string) ([]azappconfig.Setting, error) {
		return []azappconfig.Setting{
			{
				Key:         toPtr(".appconfig.featureflag/Feature1"),
				Value:       &ffValue,
				ContentType: toPtr(featureFlagContentType),
			},
			{
				Key:   toPtr("regular-key"),
				Value: &regularValue,
			},
		}, nil
	})

	azappcfg := &AzureAppConfiguration{}
	snapshotRefs := map[string]string{
		"ref1": `{"snapshot_name":"my-snapshot"}`,
	}
	kvSettings := make(map[string]any)
	keyVaultRefs := make(map[string]string)

	err := azappcfg.loadSettingsFromSnapshotRefs(context.Background(), mockLoader, snapshotRefs, kvSettings, keyVaultRefs)
	assert.NoError(t, err)
	assert.NotContains(t, kvSettings, ".appconfig.featureflag/Feature1")
	assert.Equal(t, &regularValue, kvSettings["regular-key"])
}

func TestLoadSettingsFromSnapshotRefs_CollectsKeyVaultRefs(t *testing.T) {
	kvRefValue := `{"uri":"https://myvault.vault.azure.net/secrets/mysecret"}`
	mockLoader := snapshotSettingsLoader(func(ctx context.Context, snapshotName string) ([]azappconfig.Setting, error) {
		return []azappconfig.Setting{
			{
				Key:         toPtr("secret-key"),
				Value:       &kvRefValue,
				ContentType: toPtr(secretReferenceContentType),
			},
		}, nil
	})

	azappcfg := &AzureAppConfiguration{}
	snapshotRefs := map[string]string{
		"ref1": `{"snapshot_name":"my-snapshot"}`,
	}
	kvSettings := make(map[string]any)
	keyVaultRefs := make(map[string]string)

	err := azappcfg.loadSettingsFromSnapshotRefs(context.Background(), mockLoader, snapshotRefs, kvSettings, keyVaultRefs)
	assert.NoError(t, err)
	assert.Equal(t, kvRefValue, keyVaultRefs["secret-key"])
	assert.NotContains(t, kvSettings, "secret-key")
}

func TestLoadSettingsFromSnapshotRefs_NonExistentSnapshot(t *testing.T) {
	mockLoader := snapshotSettingsLoader(func(ctx context.Context, snapshotName string) ([]azappconfig.Setting, error) {
		return []azappconfig.Setting{}, nil // 404 returns empty, aligned with JS behavior
	})

	azappcfg := &AzureAppConfiguration{}
	snapshotRefs := map[string]string{
		"ref1": `{"snapshot_name":"non-existent"}`,
	}
	kvSettings := make(map[string]any)
	keyVaultRefs := make(map[string]string)

	err := azappcfg.loadSettingsFromSnapshotRefs(context.Background(), mockLoader, snapshotRefs, kvSettings, keyVaultRefs)
	assert.NoError(t, err)
	assert.Empty(t, kvSettings)
}

func TestLoadSettingsFromSnapshotRefs_InvalidFormat(t *testing.T) {
	mockLoader := snapshotSettingsLoader(func(ctx context.Context, snapshotName string) ([]azappconfig.Setting, error) {
		t.Fatal("loader should not be called for invalid format")
		return nil, nil
	})

	azappcfg := &AzureAppConfiguration{}
	snapshotRefs := map[string]string{
		"ref1": `invalid json`,
	}
	kvSettings := make(map[string]any)
	keyVaultRefs := make(map[string]string)

	err := azappcfg.loadSettingsFromSnapshotRefs(context.Background(), mockLoader, snapshotRefs, kvSettings, keyVaultRefs)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "invalid format for Snapshot reference setting")
}

func TestLoadSettingsFromSnapshotRefs_HandlesNilContentType(t *testing.T) {
	regularValue := "plain value"
	mockLoader := snapshotSettingsLoader(func(ctx context.Context, snapshotName string) ([]azappconfig.Setting, error) {
		return []azappconfig.Setting{
			{
				Key:   toPtr("plain-key"),
				Value: &regularValue,
				// ContentType is nil
			},
		}, nil
	})

	azappcfg := &AzureAppConfiguration{}
	snapshotRefs := map[string]string{
		"ref1": `{"snapshot_name":"my-snapshot"}`,
	}
	kvSettings := make(map[string]any)
	keyVaultRefs := make(map[string]string)

	err := azappcfg.loadSettingsFromSnapshotRefs(context.Background(), mockLoader, snapshotRefs, kvSettings, keyVaultRefs)
	assert.NoError(t, err)
	assert.Equal(t, &regularValue, kvSettings["plain-key"])
}

func TestLoadSettingsFromSnapshotRefs_AppliesTrimPrefix(t *testing.T) {
	settingValue := "value1"
	mockLoader := snapshotSettingsLoader(func(ctx context.Context, snapshotName string) ([]azappconfig.Setting, error) {
		return []azappconfig.Setting{
			{
				Key:   toPtr("app:name"),
				Value: &settingValue,
			},
		}, nil
	})

	azappcfg := &AzureAppConfiguration{
		trimPrefixes: []string{"app:"},
	}
	snapshotRefs := map[string]string{
		"ref1": `{"snapshot_name":"my-snapshot"}`,
	}
	kvSettings := make(map[string]any)
	keyVaultRefs := make(map[string]string)

	err := azappcfg.loadSettingsFromSnapshotRefs(context.Background(), mockLoader, snapshotRefs, kvSettings, keyVaultRefs)
	assert.NoError(t, err)
	// Should be stored with trimmed key
	assert.Equal(t, &settingValue, kvSettings["name"])
	assert.NotContains(t, kvSettings, "app:name")
}

func TestLoadSettingsFromSnapshotRefs_HandlesJsonContentType(t *testing.T) {
	jsonValue := `{"nested":"value"}`
	mockLoader := snapshotSettingsLoader(func(ctx context.Context, snapshotName string) ([]azappconfig.Setting, error) {
		return []azappconfig.Setting{
			{
				Key:         toPtr("json-key"),
				Value:       &jsonValue,
				ContentType: toPtr("application/json"),
			},
		}, nil
	})

	azappcfg := &AzureAppConfiguration{}
	snapshotRefs := map[string]string{
		"ref1": `{"snapshot_name":"my-snapshot"}`,
	}
	kvSettings := make(map[string]any)
	keyVaultRefs := make(map[string]string)

	err := azappcfg.loadSettingsFromSnapshotRefs(context.Background(), mockLoader, snapshotRefs, kvSettings, keyVaultRefs)
	assert.NoError(t, err)
	assert.Equal(t, map[string]any{"nested": "value"}, kvSettings["json-key"])
}
