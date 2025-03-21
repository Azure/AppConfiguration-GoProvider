// Copyright (c) Microsoft Corporation.
// Licensed under the MIT License.

package azureappconfiguration

import (
	"context"

	"github.com/Azure/azure-sdk-for-go/sdk/azcore/to"
	"github.com/Azure/azure-sdk-for-go/sdk/data/azappconfig"
)

type selectorSettingsClient struct {
	selectors []Selector
	client    *azappconfig.Client
}

type settingsClient interface {
	getSettings(ctx context.Context) ([]azappconfig.Setting, error)
}

func (s *selectorSettingsClient) getSettings(ctx context.Context) ([]azappconfig.Setting, error) {
	settings := make([]azappconfig.Setting, 0)
	for _, filter := range s.selectors {
		selector := azappconfig.SettingSelector{
			KeyFilter:   to.Ptr(filter.KeyFilter),
			LabelFilter: to.Ptr(filter.LabelFilter),
			Fields:      azappconfig.AllSettingFields(),
		}

		pager := s.client.NewListSettingsPager(selector, nil)
		for pager.More() {
			page, err := pager.NextPage(ctx)
			if err != nil {
				return nil, err
			} else if page.Settings != nil {
				settings = append(settings, page.Settings...)
			}
		}
	}

	return settings, nil
}
