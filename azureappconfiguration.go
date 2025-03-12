// Copyright (c) Microsoft Corporation.
// Licensed under the MIT License.

package azureappconfiguration

import (
	"context"
	"encoding/json"
	"log"
	"regexp"
	"strings"

	"github.com/Azure/azure-sdk-for-go/sdk/azcore"
	"golang.org/x/sync/errgroup"
)

type AzureAppConfiguration struct {
	keyValues     map[string]any
	keyValueETags map[Selector][]*azcore.ETag
	kvSelectors   []Selector
	trimPrefixes  []string

	clientManager *configurationClientManager
}

func Load(ctx context.Context, authentication AuthenticationOptions, options *Options) (*AzureAppConfiguration, error) {
	if err := verifyAuthenticationOptions(authentication); err != nil {
		return nil, err
	}

	if options == nil {
		options = &Options{}
	}

	clientManager, err := newConfigurationClientManager(authentication, options.ClientOptions)
	if err != nil {
		return nil, err
	}

	azappcfg := new(AzureAppConfiguration)
	azappcfg.keyValues = make(map[string]any)
	azappcfg.keyValueETags = make(map[Selector][]*azcore.ETag)
	azappcfg.kvSelectors = deduplicateSelectors(options.Selectors)
	azappcfg.trimPrefixes = options.TrimKeyPrefixes
	azappcfg.clientManager = clientManager

	if err := azappcfg.load(ctx); err != nil {
		return nil, err
	}

	return azappcfg, nil
}

func (azappcfg *AzureAppConfiguration) load(ctx context.Context) error {
	keyValuesClient := &selectorSettingsClient{
		selectors: azappcfg.kvSelectors,
		client:    azappcfg.clientManager.staticClient.client,
	}

	eg, egCtx := errgroup.WithContext(ctx)
	eg.Go(func() error {
		return azappcfg.loadKeyValues(egCtx, keyValuesClient)
	})

	return eg.Wait()
}

func (azappcfg *AzureAppConfiguration) loadKeyValues(ctx context.Context, settingsClient settingsClient) error {
	settingsResponse, err := settingsClient.getSettings(ctx)
	if err != nil {
		return err
	}

	kvSettings := make(map[string]any, len(settingsResponse.settings))
	for _, setting := range settingsResponse.settings {
		if setting.Key == nil {
			continue
		}
		trimmedKey := azappcfg.trimPrefix(*setting.Key)
		if len(trimmedKey) == 0 {
			log.Printf("Key of the setting '%s' is trimmed to the empty string, just ignore it", *setting.Key)
			continue
		}

		if setting.ContentType == nil || setting.Value == nil {
			kvSettings[trimmedKey] = setting.Value
			continue
		}

		switch *setting.ContentType {
		case featureFlagContentType:
			continue // ignore feature flag while getting key value settings
		case secretReferenceContentType:
			continue // Todo - implement secret reference
		default:
			if isJsonContentType(setting.ContentType) {
				var v any
				if err := json.Unmarshal([]byte(*setting.Value), &v); err != nil {
					log.Printf("Failed to unmarshal JSON value: key=%s, error=%s", *setting.Key, err.Error())
					continue
				}
				kvSettings[trimmedKey] = v
			} else {
				kvSettings[trimmedKey] = *setting.Value
			}
		}
	}

	azappcfg.keyValueETags = settingsResponse.eTags
	azappcfg.keyValues = kvSettings

	return nil
}

func (azappcfg *AzureAppConfiguration) trimPrefix(key string) string {
	result := key
	for _, prefix := range azappcfg.trimPrefixes {
		if strings.HasPrefix(result, prefix) {
			result = result[len(prefix):]
			break
		}
	}

	return result
}

func isJsonContentType(contentType *string) bool {
	if contentType == nil {
		return false
	}
	contentTypeStr := strings.ToLower(strings.Trim(*contentType, " "))
	matched, _ := regexp.MatchString("^application\\/(?:[^\\/]+\\+)?json(;.*)?$", contentTypeStr)
	return matched
}

func deduplicateSelectors(selectors []Selector) []Selector {
	// If no selectors provided, return the default selector
	if len(selectors) == 0 {
		return []Selector{
			{
				KeyFilter:   wildCard,
				LabelFilter: defaultLabel,
			},
		}
	}

	// Create a map to track unique selectors
	seen := make(map[Selector]struct{})
	var result []Selector

	// Process the selectors in reverse order to maintain the behavior
	// where later duplicates take precedence over earlier ones
	for i := len(selectors) - 1; i >= 0; i-- {
		// Normalize empty label filter
		if selectors[i].LabelFilter == "" {
			selectors[i].LabelFilter = defaultLabel
		}

		// Check if we've seen this selector before
		if _, exists := seen[selectors[i]]; !exists {
			seen[selectors[i]] = struct{}{}
			result = append(result, selectors[i])
		}
	}

	// Reverse the result to maintain the original order
	reverse(result)
	return result
}
