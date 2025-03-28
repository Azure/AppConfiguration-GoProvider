// Copyright (c) Microsoft Corporation.
// Licensed under the MIT License.

package azureappconfiguration

import (
	"context"
	"errors"
	"log"

	"github.com/Azure/AppConfiguration-GoProvider/azureappconfiguration/internal/tracing"
	"github.com/Azure/azure-sdk-for-go/sdk/azcore/policy"
	"github.com/Azure/azure-sdk-for-go/sdk/azcore"
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

type watchedSettingClient struct {
	watchedSetting WatchedSetting
	eTags          map[WatchedSetting]*azcore.ETag
	client         *azappconfig.Client
	tracingOptions tracing.Options
}

type settingsClient interface {
	getSettings(ctx context.Context) (*settingsResponse, error)
}

type eTagsClient interface {
	checkIfETagChanged(ctx context.Context) (bool, error)
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

func (c *watchedSettingClient) getSettings(ctx context.Context) (*settingsResponse, error) {
	if c.tracingOptions.Enabled {
		ctx = policy.WithHTTPHeader(ctx, tracing.CreateCorrelationContextHeader(ctx, c.tracingOptions))
	}

	response, err := c.client.GetSetting(ctx, c.watchedSetting.Key, &azappconfig.GetSettingOptions{Label: to.Ptr(c.watchedSetting.Label)})
	if err != nil {
		var respErr *azcore.ResponseError
		if errors.As(err, &respErr) && respErr.StatusCode == 404 {
			label := c.watchedSetting.Label
			if label == "" || label == "\x00" { // NUL is escaped to \x00 in golang
				label = "no"
			}
			// If the watched setting is not found, not return error
			log.Printf("Watched key '%s' with %s label does not exist", c.watchedSetting.Key, label)
			return nil, nil
		}

		return nil, err
	}

	return &settingsResponse{
		settings: []azappconfig.Setting{response.Setting},
	}, nil
}

func (c *watchedSettingClient) checkIfETagChanged(ctx context.Context) (bool, error) {
	if c.tracingOptions.Enabled {
		ctx = policy.WithHTTPHeader(ctx, tracing.CreateCorrelationContextHeader(ctx, c.tracingOptions))
	}

	for watchedSetting, ETag := range c.eTags {
		_, err := c.client.GetSetting(ctx, watchedSetting.Key, &azappconfig.GetSettingOptions{Label: to.Ptr(watchedSetting.Label), OnlyIfChanged: ETag})
		if err != nil {
			var respErr *azcore.ResponseError
			if errors.As(err, &respErr) && (respErr.StatusCode == 404 || respErr.StatusCode == 304) {
				continue
			}

			return false, err
		}

		return true, nil
	}

	return false, nil
}
