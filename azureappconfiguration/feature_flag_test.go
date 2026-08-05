// Copyright (c) Microsoft Corporation.
// Licensed under the MIT License.

package azureappconfiguration

import (
	"context"
	"encoding/json"
	"fmt"
	"testing"

	"github.com/Azure/azure-sdk-for-go/sdk/azcore"
	"github.com/Azure/azure-sdk-for-go/sdk/azcore/to"
	"github.com/Azure/azure-sdk-for-go/sdk/data/azappconfig/v2"
	"github.com/stretchr/testify/assert"
)

// enhancedFeatureFlag builds a minimal enhanced feature flag returned by the dedicated endpoint.
func enhancedFeatureFlag(name string, enabled bool) azappconfig.FeatureFlag {
	etag := azcore.ETag("etag-" + name)
	return azappconfig.FeatureFlag{
		Name:       to.Ptr(name),
		Enabled:    to.Ptr(enabled),
		ETag:       &etag,
		Conditions: &azappconfig.FeatureFlagConditions{Filters: []azappconfig.FeatureFlagFilter{}},
	}
}

// classicFFSetting builds a classic feature flag key-value setting.
func classicFFSetting(id string, enabled bool) azappconfig.Setting {
	value := fmt.Sprintf(`{"id":%q,"enabled":%t,"conditions":{"client_filters":[]}}`, id, enabled)
	etag := azcore.ETag("etag-" + id)
	return azappconfig.Setting{
		Key:         to.Ptr(featureFlagKeyPrefix + id),
		Value:       to.Ptr(value),
		ContentType: to.Ptr(featureFlagContentType),
		ETag:        &etag,
	}
}

func TestConvertFeatureFlagToMicrosoftSchema(t *testing.T) {
	filterName := "Microsoft.TimeWindow"
	startParam := "Mon, 01 Jan 2024 00:00:00 GMT"
	requirementType := azappconfig.RequirementTypeAll
	statusOverride := azappconfig.StatusOverrideDisabled
	defaultVariant := "Off"
	percentileVariant := "On"
	from, to2 := 0.0, 50.0

	featureFlag := azappconfig.FeatureFlag{
		Name:    to.Ptr("Variant"),
		Enabled: to.Ptr(true),
		Conditions: &azappconfig.FeatureFlagConditions{
			RequirementType: &requirementType,
			Filters: []azappconfig.FeatureFlagFilter{
				{Name: &filterName, Parameters: map[string]*string{"Start": &startParam}},
			},
		},
		Variants: []azappconfig.FeatureFlagVariantDefinition{
			{Name: to.Ptr("Off"), Value: to.Ptr("false"), StatusOverride: &statusOverride},
			{Name: to.Ptr("On"), Value: to.Ptr("true")},
		},
		Allocation: &azappconfig.FeatureFlagAllocation{
			DefaultWhenEnabled:  &defaultVariant,
			DefaultWhenDisabled: &defaultVariant,
			Percentile:          []azappconfig.PercentileAllocation{{Variant: &percentileVariant, From: &from, To: &to2}},
			Seed:                to.Ptr("seed-value"),
		},
		Telemetry: &azappconfig.FeatureFlagTelemetryConfiguration{Enabled: to.Ptr(true)},
	}

	actual, err := json.Marshal(convertToMicrosoftSchema(featureFlag))
	assert.NoError(t, err)

	expected := `{"allocation":{"default_when_disabled":"Off","default_when_enabled":"Off","percentile":[{"from":0,"to":50,"variant":"On"}],"seed":"seed-value"},"conditions":{"client_filters":[{"name":"Microsoft.TimeWindow","parameters":{"Start":"Mon, 01 Jan 2024 00:00:00 GMT"}}],"requirement_type":"All"},"enabled":true,"id":"Variant","telemetry":{"enabled":true},"variants":[{"configuration_value":false,"name":"Off","status_override":"Disabled"},{"configuration_value":true,"name":"On"}]}`
	assert.Equal(t, expected, string(actual))
}

func TestProcessFeatureFlags_EnhancedSupersedesClassic(t *testing.T) {
	azappcfg := &AzureAppConfiguration{
		clientManager: &appConfigClientManager{endpoint: "https://fake.azconfig.io"},
	}

	classic := []azappconfig.Setting{
		classicFFSetting("Beta", false),
		classicFFSetting("Gamma", false),
	}
	enhanced := []azappconfig.FeatureFlag{
		enhancedFeatureFlag("Beta", true),
		enhancedFeatureFlag("Delta", true),
	}

	merged := azappcfg.processFeatureFlags(classic, enhanced)
	assert.Len(t, merged, 3)

	byID := map[string]map[string]any{}
	for _, f := range merged {
		m := f.(map[string]any)
		byID[m[featureFlagIdKey].(string)] = m
	}

	assert.Equal(t, true, byID["Beta"]["enabled"], "enhanced Beta should supersede classic Beta")
	assert.Equal(t, false, byID["Gamma"]["enabled"])
	assert.Equal(t, true, byID["Delta"]["enabled"])
}

func TestDeduplicateFeatureFlags_KeepsLastAndRetainsBlankID(t *testing.T) {
	flags := []map[string]any{
		{featureFlagIdKey: "Beta", "enabled": false},
		{"enabled": true}, // no id -> must be retained
		{featureFlagIdKey: "Beta", "enabled": true},
		{"enabled": false}, // no id -> must be retained
	}

	result := deduplicateFeatureFlags(flags)

	// One deduplicated "Beta" (last wins) plus the two id-less flags.
	assert.Len(t, result, 3)

	var betaCount, idlessCount int
	for _, f := range result {
		m := f.(map[string]any)
		if id, ok := m[featureFlagIdKey].(string); ok && id == "Beta" {
			betaCount++
			assert.Equal(t, true, m["enabled"], "last Beta should win")
		} else {
			idlessCount++
		}
	}
	assert.Equal(t, 1, betaCount)
	assert.Equal(t, 2, idlessCount)
}

func TestPopulateTelemetryMetadata(t *testing.T) {
	etag := azcore.ETag("etag-1")

	// Telemetry enabled -> ETag and reference are injected.
	enabled := map[string]any{
		telemetryKey: map[string]any{enabledKey: true},
	}
	populateTelemetryMetadata(enabled, &etag, "https://fake.azconfig.io/ff/Beta")
	metadata := enabled[telemetryKey].(map[string]any)[metadataKey].(map[string]any)
	assert.Equal(t, "etag-1", metadata[eTagKey])
	assert.Equal(t, "https://fake.azconfig.io/ff/Beta", metadata[featureFlagReferenceKey])

	// Telemetry disabled -> nothing is added.
	disabled := map[string]any{
		telemetryKey: map[string]any{enabledKey: false},
	}
	populateTelemetryMetadata(disabled, &etag, "https://fake.azconfig.io/ff/Beta")
	_, hasMetadata := disabled[telemetryKey].(map[string]any)[metadataKey]
	assert.False(t, hasMetadata, "metadata should not be added when telemetry is disabled")
}

func TestGenerateFeatureFlagReference(t *testing.T) {
	assert.Equal(t, "https://fake.azconfig.io/kv/Beta",
		generateFeatureFlagReference("https://fake.azconfig.io", "kv", "Beta", nil))

	assert.Equal(t, "https://fake.azconfig.io/ff/Beta",
		generateFeatureFlagReference("https://fake.azconfig.io", "ff", "Beta", to.Ptr("")))

	assert.Equal(t, "https://fake.azconfig.io/ff/Beta?label=prod",
		generateFeatureFlagReference("https://fake.azconfig.io", "ff", "Beta", to.Ptr("prod")))
}

func TestLoadFeatureFlags_MergesClassicAndEnhanced(t *testing.T) {
	ctx := context.Background()

	classicClient := new(mockSettingsClient)
	classicClient.On("getSettings", ctx).Return(&settingsResponse{
		settings: []azappconfig.Setting{
			classicFFSetting("Beta", false),
			classicFFSetting("Gamma", false),
		},
		pageETags: map[comparableSelector][]*azcore.ETag{},
	}, nil)

	enhancedClient := new(mockSettingsClient)
	enhancedClient.On("getSettings", ctx).Return(&settingsResponse{
		featureFlags: []azappconfig.FeatureFlag{
			enhancedFeatureFlag("Beta", true),
			enhancedFeatureFlag("Delta", true),
		},
		pageETags: map[comparableSelector][]*azcore.ETag{},
	}, nil)

	azappcfg := &AzureAppConfiguration{
		clientManager: &appConfigClientManager{endpoint: "https://fake.azconfig.io"},
		featureFlags:  make(map[string]any),
	}

	err := azappcfg.loadFeatureFlags(ctx, classicClient, enhancedClient)
	assert.NoError(t, err)
	assert.True(t, azappcfg.tracingOptions.UseEnhancedFeatureFlag, "enhanced feature flag tracing should be enabled")

	featureManagement, ok := azappcfg.featureFlags[featureManagementSectionKey].(map[string]any)
	assert.True(t, ok)
	flags, ok := featureManagement[featureFlagSectionKey].([]any)
	assert.True(t, ok)
	assert.Len(t, flags, 3)

	byID := map[string]map[string]any{}
	for _, f := range flags {
		m := f.(map[string]any)
		byID[m[featureFlagIdKey].(string)] = m
	}
	assert.Equal(t, true, byID["Beta"]["enabled"], "enhanced Beta should win over classic Beta")
	assert.Equal(t, false, byID["Gamma"]["enabled"])
	assert.Equal(t, true, byID["Delta"]["enabled"])

	classicClient.AssertExpectations(t)
	enhancedClient.AssertExpectations(t)
}

func TestParseFeatureFlagValue(t *testing.T) {
	assert.Nil(t, parseFeatureFlagValue(nil))
	assert.Equal(t, true, parseFeatureFlagValue(to.Ptr("true")))
	assert.Equal(t, float64(42), parseFeatureFlagValue(to.Ptr("42")))
	assert.Equal(t, "plain text", parseFeatureFlagValue(to.Ptr("plain text")))
}

func TestEqualETagSlices(t *testing.T) {
	a := azcore.ETag("a")
	b := azcore.ETag("b")

	assert.True(t, equalETagSlices([]*azcore.ETag{&a, &b}, []*azcore.ETag{&a, &b}))
	assert.False(t, equalETagSlices([]*azcore.ETag{&a}, []*azcore.ETag{&a, &b}))
	assert.False(t, equalETagSlices([]*azcore.ETag{&a, &b}, []*azcore.ETag{&a, &a}))
	assert.True(t, equalETagSlices([]*azcore.ETag{nil}, []*azcore.ETag{nil}))
	assert.False(t, equalETagSlices([]*azcore.ETag{&a}, []*azcore.ETag{nil}))
}
