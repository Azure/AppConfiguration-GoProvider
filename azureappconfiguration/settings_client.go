// Copyright (c) Microsoft Corporation.
// Licensed under the MIT License.

package azureappconfiguration

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log"
	"net/http"
	"time"

	"github.com/Azure/AppConfiguration-GoProvider/azureappconfiguration/internal/tracing"
	"github.com/Azure/azure-sdk-for-go/sdk/azcore"
	"github.com/Azure/azure-sdk-for-go/sdk/azcore/policy"
	"github.com/Azure/azure-sdk-for-go/sdk/azcore/to"
	"github.com/Azure/azure-sdk-for-go/sdk/data/azappconfig/v2"
)

type settingWatcher struct {
	eTag                   *azcore.ETag
	lastServerResponseTime string
}

type settingsResponse struct {
	settings     []azappconfig.Setting
	watchedETags map[WatchedSetting]*azcore.ETag
	pageWatchers map[comparableSelector][]settingWatcher
}

type selectorSettingsClient struct {
	selectors      []Selector
	client         *azappconfig.Client
	tracingOptions tracing.Options
}

type watchedSettingClient struct {
	watchedSettings []WatchedSetting
	eTags           map[WatchedSetting]*azcore.ETag
	client          *azappconfig.Client
	tracingOptions  tracing.Options
}

type pageWatcherClient struct {
	pageWatchers   map[comparableSelector][]settingWatcher
	client         *azappconfig.Client
	tracingOptions tracing.Options
}

type settingsClient interface {
	getSettings(ctx context.Context) (*settingsResponse, error)
}

type eTagsClient interface {
	checkIfETagChanged(ctx context.Context) (bool, error)
}

// snapshotSettingsLoader is a function type that loads settings from a snapshot by name.
type snapshotSettingsLoader func(ctx context.Context, snapshotName string) ([]azappconfig.Setting, error)

type refreshClient struct {
	loader    settingsClient
	monitor   eTagsClient
	sentinels settingsClient
}

func (s *selectorSettingsClient) getSettings(ctx context.Context) (*settingsResponse, error) {
	if s.tracingOptions.Enabled {
		ctx = policy.WithHTTPHeader(ctx, tracing.CreateCorrelationContextHeader(ctx, s.tracingOptions))
	}

	// Capture the raw HTTP response
	var httpResponse *http.Response
	ctx = policy.WithCaptureResponse(ctx, &httpResponse)
	settings := make([]azappconfig.Setting, 0)
	pageWatchers := make(map[comparableSelector][]settingWatcher)
	for _, filter := range s.selectors {
		if filter.SnapshotName == "" {
			selector := azappconfig.SettingSelector{
				KeyFilter:   to.Ptr(filter.KeyFilter),
				LabelFilter: to.Ptr(filter.LabelFilter),
				TagsFilter:  filter.TagFilters,
				Fields:      azappconfig.AllSettingFields(),
			}

			pager := s.client.NewListSettingsPager(selector, nil)
			watchers := make([]settingWatcher, 0)
			for pager.More() {
				page, err := pager.NextPage(ctx)
				if err != nil {
					return nil, err
				} else if page.Settings != nil {
					settings = append(settings, page.Settings...)
					watchers = append(watchers, settingWatcher{
						eTag: page.ETag,
					})

					if s.tracingOptions.AfdUsed && httpResponse != nil {
						watchers[len(watchers)-1].lastServerResponseTime = httpResponse.Header.Get("X-Ms-Date")
					}
				}
			}

			pageWatchers[filter.comparableKey()] = watchers
		} else {
			snapshotSettings, err := loadSnapshotSettings(ctx, s.client, filter.SnapshotName)
			if err != nil {
				return nil, err
			}
			settings = append(settings, snapshotSettings...)
		}
	}

	return &settingsResponse{
		settings:     settings,
		pageWatchers: pageWatchers,
	}, nil
}

func (c *watchedSettingClient) getSettings(ctx context.Context) (*settingsResponse, error) {
	if c.tracingOptions.Enabled {
		ctx = policy.WithHTTPHeader(ctx, tracing.CreateCorrelationContextHeader(ctx, c.tracingOptions))
	}

	settings := make([]azappconfig.Setting, 0, len(c.watchedSettings))
	watchedETags := make(map[WatchedSetting]*azcore.ETag)
	for _, watchedSetting := range c.watchedSettings {
		response, err := c.client.GetSetting(ctx, watchedSetting.Key, &azappconfig.GetSettingOptions{Label: to.Ptr(watchedSetting.Label)})
		if err != nil {
			var respErr *azcore.ResponseError
			if errors.As(err, &respErr) && respErr.StatusCode == 404 {
				label := watchedSetting.Label
				if label == "" || label == "\x00" { // NUL is escaped to \x00 in golang
					label = "no"
				}
				// If the watched setting is not found, log and continue
				log.Printf("Watched key '%s' with %s label does not exist", watchedSetting.Key, label)
				continue
			}
			return nil, err
		}

		settings = append(settings, response.Setting)
		watchedETags[watchedSetting] = response.Setting.ETag
	}

	return &settingsResponse{
		settings:     settings,
		watchedETags: watchedETags,
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

func (c *pageWatcherClient) checkIfETagChanged(ctx context.Context) (bool, error) {
	// Capture the raw HTTP response
	var httpResponse *http.Response
	ctx = policy.WithCaptureResponse(ctx, &httpResponse)

	for selector, pageWatchers := range c.pageWatchers {
		s := azappconfig.SettingSelector{
			KeyFilter:   to.Ptr(selector.KeyFilter),
			LabelFilter: to.Ptr(selector.LabelFilter),
			Fields:      azappconfig.AllSettingFields(),
		}

		tagFilters := make([]string, 0)
		if selector.TagFilters != "" {
			json.Unmarshal([]byte(selector.TagFilters), &tagFilters)
			s.TagsFilter = tagFilters
		}

		conditions := make([]azcore.MatchConditions, 0)
		for _, watcher := range pageWatchers {
			conditions = append(conditions, azcore.MatchConditions{IfNoneMatch: watcher.eTag})
		}

		listOps := &azappconfig.ListSettingsOptions{}
		if !c.tracingOptions.AfdUsed {
			listOps.MatchConditions = conditions
		}

		pager := c.client.NewListSettingsPager(s, listOps)

		pageCount := 0
		for pager.More() {
			pageCount++
			page, err := pager.NextPage(ctx)
			if err != nil {
				return false, err
			}
			// ETag changed
			if page.ETag != nil {
				if !c.tracingOptions.AfdUsed {
					return true, nil
				}

				if httpResponse != nil {
					serverResponseTime, _ := time.Parse(time.RFC1123, httpResponse.Header.Get("X-Ms-Date"))
					lastResponseTime, _ := time.Parse(time.RFC1123, pageWatchers[pageCount-1].lastServerResponseTime)
					if lastResponseTime.Before(serverResponseTime) {
						return true, nil
					}
				}
			}
		}

		if pageCount != len(pageWatchers) {
			return true, nil
		}
	}

	return false, nil
}

func loadSnapshotSettings(ctx context.Context, client *azappconfig.Client, snapshotName string) ([]azappconfig.Setting, error) {
	settings := make([]azappconfig.Setting, 0)
	snapshot, err := client.GetSnapshot(ctx, snapshotName, nil)
	if err != nil {
		var respErr *azcore.ResponseError
		if errors.As(err, &respErr) && respErr.StatusCode == 404 {
			return settings, nil // treat non-existing snapshot as empty
		}
		return nil, err
	}

	if snapshot.CompositionType == nil || *snapshot.CompositionType != azappconfig.CompositionTypeKey {
		return nil, fmt.Errorf("composition type for the selected snapshot '%s' must be 'key'", snapshotName)
	}

	pager := client.NewListSettingsForSnapshotPager(snapshotName, nil)
	for pager.More() {
		page, err := pager.NextPage(ctx)
		if err != nil {
			return nil, err
		} else if page.Settings != nil {
			settings = append(settings, page.Settings...)
		}
	}

	return settings, nil
}
