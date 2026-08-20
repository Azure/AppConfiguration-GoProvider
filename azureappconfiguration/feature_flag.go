// Copyright (c) Microsoft Corporation.
// Licensed under the MIT License.

package azureappconfiguration

import (
	"encoding/json"
	"fmt"
	"log"
	"strings"

	"github.com/Azure/azure-sdk-for-go/sdk/azcore"
	"github.com/Azure/azure-sdk-for-go/sdk/data/azappconfig/v2"
)

// processFeatureFlags merges feature flags with enhanced feature flags into the Microsoft Feature Management schema.
// When a feature flag id is present in both sources, the enhanced feature flag supersedes the classic one.
func (azappcfg *AzureAppConfiguration) processFeatureFlags(ffSettings []azappconfig.Setting, enhancedFlags []azappconfig.FeatureFlag) []any {
	clientEndpoint := ""
	if manager, ok := azappcfg.clientManager.(*appConfigClientManager); ok {
		clientEndpoint = manager.endpoint
	}

	merged := make([]map[string]any, 0, len(ffSettings)+len(enhancedFlags))
	for _, setting := range ffSettings {
		if setting.ContentType == nil || *setting.ContentType != featureFlagContentType {
			continue
		}
		if setting.Key == nil || setting.Value == nil {
			continue
		}

		var ff map[string]any
		if err := json.Unmarshal([]byte(*setting.Value), &ff); err != nil {
			log.Printf("Invalid feature flag setting: key=%s, error=%s, ignoring", *setting.Key, err.Error())
			continue
		}

		azappcfg.updateFeatureFlagTracing(ff)
		populateTelemetryMetadata(ff, setting.ETag, generateFeatureFlagReference(clientEndpoint, keyValueResourceType, *setting.Key, setting.Label))
		merged = append(merged, ff)
	}

	for _, flag := range enhancedFlags {
		if flag.Name == nil {
			continue
		}

		ff := convertToMicrosoftSchema(flag)
		azappcfg.updateFeatureFlagTracing(ff)
		populateTelemetryMetadata(ff, flag.ETag, generateFeatureFlagReference(clientEndpoint, featureFlagResourceType, *flag.Name, flag.Label))
		merged = append(merged, ff)
	}

	return deduplicateFeatureFlags(merged)
}

func deduplicateFeatureFlags(featureFlags []map[string]any) []any {
	seen := make(map[string]bool, len(featureFlags))
	deduplicated := make([]any, 0, len(featureFlags))

	for i := len(featureFlags) - 1; i >= 0; i-- {
		if id, ok := featureFlags[i][featureFlagIdKey].(string); ok && id != "" {
			if seen[id] {
				continue
			}
			seen[id] = true
		}
		deduplicated = append(deduplicated, featureFlags[i])
	}

	for i, j := 0, len(deduplicated)-1; i < j; i, j = i+1, j-1 {
		deduplicated[i], deduplicated[j] = deduplicated[j], deduplicated[i]
	}

	return deduplicated
}

func generateFeatureFlagReference(endpoint string, resourceType string, name string, label *string) string {
	reference := fmt.Sprintf("%s/%s/%s", endpoint, resourceType, name)
	if label != nil && strings.TrimSpace(*label) != "" {
		reference += fmt.Sprintf("?label=%s", *label)
	}

	return reference
}

func populateTelemetryMetadata(featureFlag map[string]any, eTag *azcore.ETag, reference string) {
	telemetry, ok := featureFlag[telemetryKey].(map[string]any)
	if !ok {
		return
	}

	enabled, ok := telemetry[enabledKey].(bool)
	if !ok || !enabled {
		return
	}

	metadata, ok := telemetry[metadataKey].(map[string]any)
	if !ok || metadata == nil {
		metadata = make(map[string]any)
	}

	if eTag != nil {
		metadata[eTagKey] = string(*eTag)
	}
	metadata[featureFlagReferenceKey] = reference
	telemetry[metadataKey] = metadata
}

func convertToMicrosoftSchema(flag azappconfig.FeatureFlag) map[string]any {
	result := make(map[string]any)

	if flag.Name != nil {
		result[featureFlagIdKey] = *flag.Name
	}

	if flag.Enabled != nil {
		result[enabledKey] = *flag.Enabled
	} else {
		result[enabledKey] = false
	}

	if flag.Description != nil {
		result[descriptionKey] = *flag.Description
	}

	// conditions: Filters -> client_filters, RequirementType -> requirement_type
	conditions := make(map[string]any)
	clientFilters := make([]any, 0)
	if flag.Conditions != nil {
		for _, filter := range flag.Conditions.Filters {
			clientFilter := make(map[string]any)
			if filter.Name != nil {
				clientFilter[nameKey] = *filter.Name
			}
			if filter.Parameters != nil {
				parameters := make(map[string]any, len(filter.Parameters))
				for key, value := range filter.Parameters {
					parameters[key] = parseFeatureFlagParameterValue(value)
				}
				clientFilter[parametersKey] = parameters
			}
			clientFilters = append(clientFilters, clientFilter)
		}
	}
	conditions[clientFiltersKeyName] = clientFilters
	if flag.Conditions != nil && flag.Conditions.RequirementType != nil {
		conditions[requirementTypeKey] = string(*flag.Conditions.RequirementType)
	}
	result[conditionsKeyName] = conditions

	// variants: Value -> configuration_value, StatusOverride -> status_override
	if flag.Variants != nil {
		variants := make([]any, 0, len(flag.Variants))
		for _, variant := range flag.Variants {
			variantMap := make(map[string]any)
			if variant.Name != nil {
				variantMap[nameKey] = *variant.Name
			}
			if variant.Value != nil {
				configurationValue, err := parseFeatureFlagVariantValue(variant.Value, variant.ContentType)
				if err != nil && flag.Name != nil {
					log.Printf("Invalid enhanced feature flag: name=%s, error=%s, using variant value as string", *flag.Name, err.Error())
				}
				variantMap[configurationValueKey] = configurationValue
			}
			if variant.StatusOverride != nil {
				variantMap[statusOverrideKey] = string(*variant.StatusOverride)
			}
			variants = append(variants, variantMap)
		}
		result[variantsKeyName] = variants
	}

	// allocation: camelCase -> snake_case
	if flag.Allocation != nil {
		allocation := make(map[string]any)
		source := flag.Allocation
		if source.DefaultWhenDisabled != nil {
			allocation[defaultWhenDisabledKey] = *source.DefaultWhenDisabled
		}
		if source.DefaultWhenEnabled != nil {
			allocation[defaultWhenEnabledKey] = *source.DefaultWhenEnabled
		}
		if source.Percentile != nil {
			percentiles := make([]any, 0, len(source.Percentile))
			for _, percentile := range source.Percentile {
				percentileMap := make(map[string]any)
				if percentile.Variant != nil {
					percentileMap[variantKeyName] = *percentile.Variant
				}
				if percentile.From != nil {
					percentileMap[fromKeyName] = *percentile.From
				}
				if percentile.To != nil {
					percentileMap[toKeyName] = *percentile.To
				}
				percentiles = append(percentiles, percentileMap)
			}
			allocation[percentileKeyName] = percentiles
		}
		if source.Group != nil {
			groups := make([]any, 0, len(source.Group))
			for _, group := range source.Group {
				groupMap := make(map[string]any)
				if group.Variant != nil {
					groupMap[variantKeyName] = *group.Variant
				}
				if group.Groups != nil {
					groupMap[groupsKey] = toInterfaceSlice(group.Groups)
				}
				groups = append(groups, groupMap)
			}
			allocation[groupKey] = groups
		}
		if source.User != nil {
			users := make([]any, 0, len(source.User))
			for _, user := range source.User {
				userMap := make(map[string]any)
				if user.Variant != nil {
					userMap[variantKeyName] = *user.Variant
				}
				if user.Users != nil {
					userMap[usersKey] = toInterfaceSlice(user.Users)
				}
				users = append(users, userMap)
			}
			allocation[userKey] = users
		}
		if source.Seed != nil {
			allocation[seedKeyName] = *source.Seed
		}
		result[allocationKeyName] = allocation
	}

	// telemetry: metadata is (re)populated later by populateTelemetryMetadata with ETag/FeatureFlagReference
	if flag.Telemetry != nil {
		telemetry := make(map[string]any)
		if flag.Telemetry.Enabled != nil {
			telemetry[enabledKey] = *flag.Telemetry.Enabled
		} else {
			telemetry[enabledKey] = false
		}
		if flag.Telemetry.Metadata != nil {
			metadata := make(map[string]any, len(flag.Telemetry.Metadata))
			for key, value := range flag.Telemetry.Metadata {
				if value != nil {
					metadata[key] = *value
				}
			}
			telemetry[metadataKey] = metadata
		}
		result[telemetryKey] = telemetry
	}

	return result
}

func parseFeatureFlagParameterValue(raw *string) any {
	if raw == nil {
		return nil
	}

	trimmed := strings.TrimLeft(*raw, " \t\r\n")
	if len(trimmed) == 0 || (trimmed[0] != '{' && trimmed[0] != '[') {
		return *raw
	}

	var parsed any
	if err := json.Unmarshal([]byte(*raw), &parsed); err == nil {
		return parsed
	}

	return *raw
}

func parseFeatureFlagVariantValue(raw *string, contentType *string) (any, error) {
	if raw == nil {
		return nil, nil
	}

	if !isJsonContentType(contentType) {
		return *raw, nil
	}

	var parsed any
	if err := json.Unmarshal([]byte(*raw), &parsed); err != nil {
		return *raw, err
	}

	return parsed, nil
}

// toInterfaceSlice converts a slice of strings into a slice of any for inclusion in the
// generic map that is marshaled into the feature management schema.
func toInterfaceSlice(values []string) []any {
	result := make([]any, len(values))
	for i, value := range values {
		result[i] = value
	}

	return result
}

// equalETagSlices reports whether two ordered slices of page ETags are equivalent.
func equalETagSlices(a []*azcore.ETag, b []*azcore.ETag) bool {
	if len(a) != len(b) {
		return false
	}

	for i := range a {
		if a[i] == nil || b[i] == nil {
			if a[i] != b[i] {
				return false
			}
			continue
		}
		if *a[i] != *b[i] {
			return false
		}
	}

	return true
}
