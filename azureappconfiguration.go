// Copyright (c) Microsoft Corporation.
// Licensed under the MIT License.

package azureappconfiguration

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log"
	"net"
	"net/http"
	"regexp"
	"strings"

	"github.com/Azure/azure-sdk-for-go/sdk/azcore"
	"golang.org/x/sync/errgroup"
)

type AzureAppConfiguration struct {
	keyValues        map[string]any
	keyValueETags    map[Selector][]*azcore.ETag
	kvSelectors  []Selector
	trimPrefixes []string
	onRefreshSuccess func()

	clientManager  *configurationClientManager
	settingsClient // used for testing
}

func Load(ctx context.Context, authOptions AuthenticationOptions, cfgOptions *Options) (*AzureAppConfiguration, error) {
	if err := verifyAuthenticationOptions(authOptions); err != nil {
		return nil, err
	}

	options := cfgOptions
	if options == nil {
		options = &Options{}
	}

	clientManager, err := newConfigurationClientManager(ctx, authOptions, options.ClientOptions)
	if err != nil {
		return nil, err
	}

	azappcfg := new(AzureAppConfiguration)
	azappcfg.keyValues = make(map[string]any)
	azappcfg.keyValueETags = make(map[Selector][]*azcore.ETag)
	azappcfg.kvSelectors = getValidKeyValuesSelectors(options.Selectors)
	azappcfg.trimPrefixes = options.TrimKeyPrefixes
	azappcfg.clientManager = clientManager

	if err := azappcfg.load(ctx); err != nil {
		return nil, err
	}

	return azappcfg, nil
}

func (azappcfg *AzureAppConfiguration) Refresh(ctx context.Context) error {
	// Todo - implement refresh
	return nil
}

func (azappcfg *AzureAppConfiguration) OnRefreshSuccess(callback func()) {
	azappcfg.onRefreshSuccess = callback
}

func (azappcfg *AzureAppConfiguration) Unmarshal(configStruct any, options ConstructionOptions) error {
	// Todo - implement Unmarshal

	return nil
}

func (azappcfg *AzureAppConfiguration) GetBytes(options ConstructionOptions) []byte {
	// Todo - implement GetBytes

	return nil
}

func (azappcfg *AzureAppConfiguration) load(ctx context.Context) error {
	type loadTask func(_ context.Context) error
	tasks := []loadTask{
		azappcfg.loadKeyValues,
	}

	eg, egCtx := errgroup.WithContext(ctx)
	for _, task := range tasks {
		task := task
		eg.Go(func() error {
			return task(egCtx)
		})
	}

	if err := eg.Wait(); err != nil {
		return err
	}

	return nil
}

func (azappcfg *AzureAppConfiguration) loadKeyValues(ctx context.Context) error {
	settingsClient := azappcfg.settingsClient
	if settingsClient == nil {
		settingsClient = &selectorSettingsClient{
			selectors: azappcfg.kvSelectors,
		}
	}

	settingsResponse, err := azappcfg.executeFailoverPolicy(ctx, settingsClient)
	if err != nil {
		return err
	}

	kvSettings := make(map[string]any, len(settingsResponse.settings))
	for _, setting := range settingsResponse.settings {
		if setting.Key == nil {
			continue
		}
		trimmedKey := trimPrefix(*setting.Key, azappcfg.trimPrefixes)
		if len(trimmedKey) == 0 {
			log.Printf("Key of the setting '%s' is trimmed to the empty string, just ignore it", *setting.Key)
			continue
		}

		if setting.ContentType == nil || setting.Value == nil {
			kvSettings[trimmedKey] = setting.Value
			continue
		}

		switch *setting.ContentType {
		case FeatureFlagContentType:
			continue // ignore feature flag while getting key value settings
		case SecretReferenceContentType:
			continue // Todo - implement secret reference
		default:
			kvSettings[trimmedKey] = *setting.Value
			if isJsonContentType(setting.ContentType) {
				var v any
				if err := json.Unmarshal([]byte(*setting.Value), &v); err != nil {
					log.Printf("Failed to unmarshal JSON value: key=%s, error=%s", *setting.Key, err.Error())
					continue
				}
				kvSettings[trimmedKey] = v
			}
		}
	}

	azappcfg.keyValueETags = settingsResponse.eTags
	azappcfg.keyValues = kvSettings

	return nil
}

func (azappcfg *AzureAppConfiguration) executeFailoverPolicy(ctx context.Context, settingsClient settingsClient) (*settingsResponse, error) {
	clients := azappcfg.clientManager.getClients(ctx)
	for _, clientWrapper := range clients {
		settingsResponse, err := settingsClient.getSettings(ctx, clientWrapper.Client)
		successful := true
		if err != nil {
			successful = false
			updateClientBackoffStatus(clientWrapper, successful)
			if isFailoverable(err) {
				continue
			}
			return nil, err
		}

		updateClientBackoffStatus(clientWrapper, successful)
		return settingsResponse, nil
	}

	return nil, fmt.Errorf("All app configuration clients failed to get settings.")
}

func getValidKeyValuesSelectors(selectors []Selector) []Selector {
	return deduplicateSelectors(selectors)
}

func getValidFeatureFlagSelectors(selectors []Selector) []Selector {
	s := deduplicateSelectors(selectors)
	for _, selector := range s {
		selector.KeyFilter = FeatureFlagPrefixKey + selector.KeyFilter
	}

	return s
}

func trimPrefix(key string, prefixToTrim []string) string {
	if len(prefixToTrim) > 0 {
		for _, v := range prefixToTrim {
			if strings.HasPrefix(key, v) {
				return strings.TrimPrefix(key, v)
			}
		}
	}

	return key
}

func isJsonContentType(contentType *string) bool {
	if contentType == nil {
		return false
	}
	contentTypeStr := strings.ToLower(strings.Trim(*contentType, " "))
	matched, _ := regexp.MatchString("^application\\/(?:[^\\/]+\\+)?json(;.*)?$", contentTypeStr)
	return matched
}

func isFailoverable(err error) bool {
	if err == nil {
		return false
	}

	if _, ok := err.(net.Error); ok {
		return true
	}

	var respErr *azcore.ResponseError
	if errors.As(err, &respErr) &&
		(respErr.StatusCode == http.StatusTooManyRequests ||
			respErr.StatusCode == http.StatusRequestTimeout ||
			respErr.StatusCode >= http.StatusInternalServerError) {
		return true
	}

	return false
}

func deduplicateSelectors(selectors []Selector) []Selector {
	var result []Selector
	findDuplicate := false

	if len(selectors) > 0 {
		// Deduplicate the filters in a way that in honor of what user tell us
		// If user populate the selectors with  `{KeyFilter: "one*", LabelFilter: "prod"}, {KeyFilter: "two*", LabelFilter: "dev"}, {KeyFilter: "one*", LabelFilter: "prod"}`
		// We deduplicate it into `{KeyFilter: "two*", LabelFilter: "dev"}, {KeyFilter: "one*", LabelFilter: "prod"}`
		// not `{KeyFilter: "one*", LabelFilter: "prod"}, {KeyFilter: "two*", LabelFilter: "dev"}`
		for i := len(selectors) - 1; i >= 0; i-- {
			findDuplicate = false
			// Normalize the empty label filter to NullLabel
			if selectors[i].LabelFilter == "" {
				selectors[i].LabelFilter = NullLabel
			}

			for j := 0; j < len(result); j++ {
				if result[j].KeyFilter == selectors[i].KeyFilter &&
					result[j].LabelFilter == selectors[i].LabelFilter {
					findDuplicate = true
					break
				}
			}
			if !findDuplicate {
				result = append(result, selectors[i])
			}
		}
		reverse(result)
	} else {
		// If no selectors provided, we will load all key values with no label
		result = append(result, Selector{
			KeyFilter:   WildCard,
			LabelFilter: NullLabel,
		})
	}

	return result
}
