// Copyright (c) Microsoft Corporation.
// Licensed under the MIT License.

package azureappconfiguration

import (
	"context"

	"github.com/Azure/azure-sdk-for-go/sdk/azcore"
	"github.com/Azure/azure-sdk-for-go/sdk/azcore/runtime"
	"github.com/Azure/azure-sdk-for-go/sdk/data/azappconfig/v2"
)

// appConfigClient abstracts the Azure App Configuration operations used by the provider.
type appConfigClient interface {
	// Key-value operations.
	NewListSettingsPager(selector azappconfig.SettingSelector, options *azappconfig.ListSettingsOptions) *runtime.Pager[azappconfig.ListSettingsPageResponse]
	GetSetting(ctx context.Context, key string, options *azappconfig.GetSettingOptions) (azappconfig.GetSettingResponse, error)
	GetSnapshot(ctx context.Context, snapshotName string, options *azappconfig.GetSnapshotOptions) (azappconfig.GetSnapshotResponse, error)
	NewListSettingsForSnapshotPager(snapshotName string, options *azappconfig.ListSettingsForSnapshotOptions) *runtime.Pager[azappconfig.ListSettingsForSnapshotResponse]

	// Enhanced feature flag operations.
	NewListFeatureFlagsPager(selector azappconfig.FeatureFlagSelector, options *azappconfig.ListFeatureFlagsOptions) *runtime.Pager[azappconfig.ListFeatureFlagsPageResponse]
}

// appConfigurationClient is the default appConfigClient implementation.
// The feature flag client is derived from the configuration client so both share the same pipeline and sync-token cache.
type appConfigurationClient struct {
	configurationClient *azappconfig.Client
	featureFlagClient   *azappconfig.FeatureFlagClient
}

func newAppConfigurationClient(endpoint string, credential azcore.TokenCredential, options *azappconfig.ClientOptions) (appConfigClient, error) {
	configurationClient, err := azappconfig.NewClient(endpoint, credential, options)
	if err != nil {
		return nil, err
	}

	return &appConfigurationClient{
		configurationClient: configurationClient,
		featureFlagClient:   configurationClient.NewFeatureFlagClient(),
	}, nil
}

func newAppConfigurationClientFromConnectionString(connectionString string, options *azappconfig.ClientOptions) (appConfigClient, error) {
	configurationClient, err := azappconfig.NewClientFromConnectionString(connectionString, options)
	if err != nil {
		return nil, err
	}

	return &appConfigurationClient{
		configurationClient: configurationClient,
		featureFlagClient:   configurationClient.NewFeatureFlagClient(),
	}, nil
}

func (c *appConfigurationClient) NewListSettingsPager(selector azappconfig.SettingSelector, options *azappconfig.ListSettingsOptions) *runtime.Pager[azappconfig.ListSettingsPageResponse] {
	return c.configurationClient.NewListSettingsPager(selector, options)
}

func (c *appConfigurationClient) GetSetting(ctx context.Context, key string, options *azappconfig.GetSettingOptions) (azappconfig.GetSettingResponse, error) {
	return c.configurationClient.GetSetting(ctx, key, options)
}

func (c *appConfigurationClient) GetSnapshot(ctx context.Context, snapshotName string, options *azappconfig.GetSnapshotOptions) (azappconfig.GetSnapshotResponse, error) {
	return c.configurationClient.GetSnapshot(ctx, snapshotName, options)
}

func (c *appConfigurationClient) NewListSettingsForSnapshotPager(snapshotName string, options *azappconfig.ListSettingsForSnapshotOptions) *runtime.Pager[azappconfig.ListSettingsForSnapshotResponse] {
	return c.configurationClient.NewListSettingsForSnapshotPager(snapshotName, options)
}

func (c *appConfigurationClient) NewListFeatureFlagsPager(selector azappconfig.FeatureFlagSelector, options *azappconfig.ListFeatureFlagsOptions) *runtime.Pager[azappconfig.ListFeatureFlagsPageResponse] {
	return c.featureFlagClient.NewListFeatureFlagsPager(selector, options)
}
