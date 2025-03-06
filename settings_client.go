// Copyright (c) Microsoft Corporation.
// Licensed under the MIT License.

package azureappconfiguration

import (
	"context"

	"github.com/Azure/azure-sdk-for-go/sdk/azcore"
	"github.com/Azure/azure-sdk-for-go/sdk/azcore/to"
	"github.com/Azure/azure-sdk-for-go/sdk/data/azappconfig"
)

type settingsResponse struct {
	settings []azappconfig.Setting
	eTags    map[Selector][]*azcore.ETag
}

type selectorSettingsClient struct {
	selectors []Selector
}

type settingsClient interface {
	getSettings(ctx context.Context, client *azappconfig.Client) (*settingsResponse, error)
}

func (s *selectorSettingsClient) getSettings(ctx context.Context, client *azappconfig.Client) (*settingsResponse, error) {
	settings := make([]azappconfig.Setting, 0)
	pageETags := make(map[Selector][]*azcore.ETag)

	for _, filter := range s.selectors {
		selector := azappconfig.SettingSelector{
			KeyFilter:   to.Ptr(filter.KeyFilter),
			LabelFilter: to.Ptr(filter.LabelFilter),
			Fields:      azappconfig.AllSettingFields(),
		}
		pager := client.NewListSettingsPager(selector, nil)
		latestEtags := make([]*azcore.ETag, 0)

		for pager.More() {
			page, err := pager.NextPage(ctx)
			if err != nil {
				return nil, err
			} else if page.Settings != nil {
				settings = append(settings, page.Settings...)
				latestEtags = append(latestEtags, page.ETag)
			}
		}
		// update the etags for the filter
		pageETags[filter] = latestEtags
	}

	return &settingsResponse{
		settings: settings,
		eTags:    pageETags,
	}, nil
}