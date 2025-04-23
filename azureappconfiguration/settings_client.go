// Copyright (c) Microsoft Corporation.
// Licensed under the MIT License.

package azureappconfiguration

import (
	"context"

	"github.com/Azure/AppConfiguration-GoProvider/azureappconfiguration/internal/tracing"
	"github.com/Azure/azure-sdk-for-go/sdk/azcore/policy"
	"github.com/Azure/azure-sdk-for-go/sdk/azcore/to"
	"github.com/Azure/azure-sdk-for-go/sdk/data/azappconfig"
)

type settingsResponse struct {
	settings []azappconfig.Setting
	// TODO: pageETags
}

type selectorSettingsClient struct {
	selectors      []Selector
	client         *azappconfig.Client
	tracingOptions tracing.Options
}

type settingsClient interface {
	getSettings(ctx context.Context) (*settingsResponse, error)
}

func (s *selectorSettingsClient) getSettings(ctx context.Context) (*settingsResponse, error) {
	if s.tracingOptions.Enabled {
		ctx = policy.WithHTTPHeader(ctx, tracing.CreateCorrelationContextHeader(ctx, s.tracingOptions))
	}

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

	return &settingsResponse{
		settings: settings,
	}, nil
}
