// Copyright (c) Microsoft Corporation.
// Licensed under the MIT License.

package tracing

import (
	"context"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestCreateCorrelationContextHeader(t *testing.T) {
	t.Run("empty options", func(t *testing.T) {
		ctx := context.Background()
		options := Options{}

		header := CreateCorrelationContextHeader(ctx, options)

		// The header should be empty but exist
		corrContext := header.Get(CorrelationContextHeader)
		assert.Equal(t, "RequestType=StartUp", corrContext)
	})

	t.Run("with RequestTypeStartUp", func(t *testing.T) {
		options := Options{}

		header := CreateCorrelationContextHeader(context.Background(), options)

		// Should contain RequestTypeStartUp
		corrContext := header.Get(CorrelationContextHeader)
		assert.Contains(t, corrContext, RequestTypeKey+"="+string(RequestTypeStartUp))
	})

	t.Run("with RequestTypeWatch", func(t *testing.T) {
		options := Options{
			InitialLoadFinished: true,
		}

		header := CreateCorrelationContextHeader(context.Background(), options)

		// Should contain RequestTypeWatch
		corrContext := header.Get(CorrelationContextHeader)
		assert.Contains(t, corrContext, RequestTypeKey+"="+string(RequestTypeWatch))
	})

	t.Run("with Host", func(t *testing.T) {
		ctx := context.Background()
		options := Options{
			Host: HostTypeAzureWebApp,
		}

		header := CreateCorrelationContextHeader(ctx, options)

		// Should contain Host
		corrContext := header.Get(CorrelationContextHeader)
		assert.Contains(t, corrContext, HostTypeKey+"="+string(HostTypeAzureWebApp))
	})

	t.Run("with KeyVault configured", func(t *testing.T) {
		ctx := context.Background()
		options := Options{
			KeyVaultConfigured: true,
		}

		header := CreateCorrelationContextHeader(ctx, options)

		// Should contain KeyVaultConfiguredTag
		corrContext := header.Get(CorrelationContextHeader)
		assert.Contains(t, corrContext, KeyVaultConfiguredTag)
	})

	t.Run("with KeyVaultRefresh configured", func(t *testing.T) {
		ctx := context.Background()
		options := Options{
			KeyVaultConfigured:        true,
			KeyVaultRefreshConfigured: true,
		}

		header := CreateCorrelationContextHeader(ctx, options)

		// Should contain KeyVaultRefreshConfiguredTag
		corrContext := header.Get(CorrelationContextHeader)
		assert.Contains(t, corrContext, KeyVaultRefreshConfiguredTag)
	})

	t.Run("with AI configuration", func(t *testing.T) {
		ctx := context.Background()
		options := Options{
			UseAIConfiguration: true,
		}

		header := CreateCorrelationContextHeader(ctx, options)

		// Should contain AIConfigurationTag
		corrContext := header.Get(CorrelationContextHeader)
		assert.Contains(t, corrContext, FeaturesKey+"="+AIConfigurationTag)
	})

	t.Run("with AI chat completion configuration", func(t *testing.T) {
		ctx := context.Background()
		options := Options{
			UseAIChatCompletionConfiguration: true,
		}

		header := CreateCorrelationContextHeader(ctx, options)

		// Should contain AIChatCompletionConfigurationTag
		corrContext := header.Get(CorrelationContextHeader)
		assert.Contains(t, corrContext, FeaturesKey+"="+AIChatCompletionConfigurationTag)
	})

	t.Run("with both AI configurations", func(t *testing.T) {
		ctx := context.Background()
		options := Options{
			UseAIConfiguration:               true,
			UseAIChatCompletionConfiguration: true,
		}

		header := CreateCorrelationContextHeader(ctx, options)

		// Should contain both AI configuration tags
		corrContext := header.Get(CorrelationContextHeader)
		assert.Contains(t, corrContext, FeaturesKey+"=")

		// Extract the Features part
		parts := strings.Split(corrContext, DelimiterComma)
		var featuresPart string
		for _, part := range parts {
			if strings.HasPrefix(part, FeaturesKey+"=") {
				featuresPart = part
				break
			}
		}

		// Check both tags are in the features part
		assert.Contains(t, featuresPart, AIConfigurationTag)
		assert.Contains(t, featuresPart, AIChatCompletionConfigurationTag)

		// Check the delimiter is correct
		features := strings.Split(strings.TrimPrefix(featuresPart, FeaturesKey+"="), DelimiterPlus)
		assert.Len(t, features, 2)
		assert.Contains(t, features, AIConfigurationTag)
		assert.Contains(t, features, AIChatCompletionConfigurationTag)
	})

	t.Run("with all options", func(t *testing.T) {
		options := Options{
			Host:                             HostTypeAzureFunction,
			KeyVaultConfigured:               true,
			UseAIConfiguration:               true,
			UseAIChatCompletionConfiguration: true,
		}

		header := CreateCorrelationContextHeader(context.Background(), options)

		// Check the complete header
		corrContext := header.Get(CorrelationContextHeader)

		assert.Contains(t, corrContext, RequestTypeKey+"="+string(RequestTypeStartUp))
		assert.Contains(t, corrContext, HostTypeKey+"="+string(HostTypeAzureFunction))
		assert.Contains(t, corrContext, KeyVaultConfiguredTag)

		// Extract the Features part
		parts := strings.Split(corrContext, DelimiterComma)
		var featuresPart string
		for _, part := range parts {
			if strings.HasPrefix(part, FeaturesKey+"=") {
				featuresPart = part
				break
			}
		}

		// Check both AI tags are in the features part
		assert.Contains(t, featuresPart, AIConfigurationTag)
		assert.Contains(t, featuresPart, AIChatCompletionConfigurationTag)

		// Verify the header format
		assert.Equal(t, 4, strings.Count(corrContext, DelimiterComma)+1, "Should have 4 parts")
	})

	t.Run("delimiter handling", func(t *testing.T) {
		options := Options{
			Host:               HostTypeAzureWebApp,
			KeyVaultConfigured: true,
		}

		header := CreateCorrelationContextHeader(context.Background(), options)

		// Check the complete header
		corrContext := header.Get(CorrelationContextHeader)

		// Verify there are exactly 3 parts separated by commas
		parts := strings.Split(corrContext, DelimiterComma)
		assert.Len(t, parts, 3, "Should have 3 parts separated by commas")
	})
}

func TestFeatureFlagTracing_UpdateFeatureFilterTracing(t *testing.T) {
	tests := []struct {
		name         string
		filterName   string
		expectCustom bool
		expectTime   bool
		expectTarget bool
	}{
		{
			name:         "Microsoft.TimeWindow filter",
			filterName:   TimeWindowFilterName,
			expectCustom: false,
			expectTime:   true,
			expectTarget: false,
		},
		{
			name:         "Microsoft.Targeting filter",
			filterName:   TargetingFilterName,
			expectCustom: false,
			expectTime:   false,
			expectTarget: true,
		},
		{
			name:         "Custom filter",
			filterName:   "Microsoft.CustomFilter",
			expectCustom: true,
			expectTime:   false,
			expectTarget: false,
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			tracing := FeatureFlagTracing{}
			tracing.UpdateFeatureFilterTracing(test.filterName)

			assert.Equal(t, test.expectCustom, tracing.UsesCustomFilter)
			assert.Equal(t, test.expectTime, tracing.UsesTimeWindowFilter)
			assert.Equal(t, test.expectTarget, tracing.UsesTargetingFilter)
		})
	}
}

func TestFeatureFlagTracing_UpdateMaxVariants(t *testing.T) {
	tests := []struct {
		name        string
		initialMax  int
		newValue    int
		expectedMax int
	}{
		{
			name:        "Update with larger value",
			initialMax:  2,
			newValue:    5,
			expectedMax: 5,
		},
		{
			name:        "Update with smaller value",
			initialMax:  5,
			newValue:    3,
			expectedMax: 5,
		},
		{
			name:        "Update with equal value",
			initialMax:  3,
			newValue:    3,
			expectedMax: 3,
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			tracing := FeatureFlagTracing{MaxVariants: test.initialMax}
			tracing.UpdateMaxVariants(test.newValue)

			assert.Equal(t, test.expectedMax, tracing.MaxVariants)
		})
	}
}

func TestFeatureFlagTracing_CreateFeatureFiltersString(t *testing.T) {
	tests := []struct {
		name           string
		tracing        FeatureFlagTracing
		expectedResult string
	}{
		{
			name: "No filters",
			tracing: FeatureFlagTracing{
				UsesCustomFilter:     false,
				UsesTimeWindowFilter: false,
				UsesTargetingFilter:  false,
			},
			expectedResult: "",
		},
		{
			name: "Only custom filter",
			tracing: FeatureFlagTracing{
				UsesCustomFilter:     true,
				UsesTimeWindowFilter: false,
				UsesTargetingFilter:  false,
			},
			expectedResult: CustomFilterKey,
		},
		{
			name: "Only time window filter",
			tracing: FeatureFlagTracing{
				UsesCustomFilter:     false,
				UsesTimeWindowFilter: true,
				UsesTargetingFilter:  false,
			},
			expectedResult: TimeWindowFilterKey,
		},
		{
			name: "Only targeting filter",
			tracing: FeatureFlagTracing{
				UsesCustomFilter:     false,
				UsesTimeWindowFilter: false,
				UsesTargetingFilter:  true,
			},
			expectedResult: TargetingFilterKey,
		},
		{
			name: "Multiple filters",
			tracing: FeatureFlagTracing{
				UsesCustomFilter:     true,
				UsesTimeWindowFilter: true,
				UsesTargetingFilter:  true,
			},
			expectedResult: CustomFilterKey + DelimiterPlus + TimeWindowFilterKey + DelimiterPlus + TargetingFilterKey,
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			result := test.tracing.CreateFeatureFiltersString()
			assert.Equal(t, test.expectedResult, result)
		})
	}
}

func TestFeatureFlagTracing_CreateFeaturesString(t *testing.T) {
	tests := []struct {
		name           string
		tracing        FeatureFlagTracing
		expectedResult string
	}{
		{
			name: "No features",
			tracing: FeatureFlagTracing{
				UsesSeed:      false,
				UsesTelemetry: false,
			},
			expectedResult: "",
		},
		{
			name: "Only seed",
			tracing: FeatureFlagTracing{
				UsesSeed:      true,
				UsesTelemetry: false,
			},
			expectedResult: FFSeedUsedTag,
		},
		{
			name: "Only telemetry",
			tracing: FeatureFlagTracing{
				UsesSeed:      false,
				UsesTelemetry: true,
			},
			expectedResult: FFTelemetryUsedTag,
		},
		{
			name: "Both features",
			tracing: FeatureFlagTracing{
				UsesSeed:      true,
				UsesTelemetry: true,
			},
			expectedResult: FFSeedUsedTag + DelimiterPlus + FFTelemetryUsedTag,
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			result := test.tracing.CreateFeaturesString()
			assert.Equal(t, test.expectedResult, result)
		})
	}
}

func TestCreateCorrelationContextHeader_WithFeatureFlagTracing(t *testing.T) {
	tests := []struct {
		name        string
		tracing     *FeatureFlagTracing
		expected    []string
		notExpected []string
	}{
		{
			name: "All feature flags features",
			tracing: &FeatureFlagTracing{
				UsesCustomFilter:     true,
				UsesTimeWindowFilter: true,
				UsesTargetingFilter:  true,
				UsesTelemetry:        true,
				UsesSeed:             true,
				MaxVariants:          3,
			},
			expected: []string{
				FeatureFilterTypeKey + "=" + CustomFilterKey + DelimiterPlus + TimeWindowFilterKey + DelimiterPlus + TargetingFilterKey,
				FFFeaturesKey + "=" + FFSeedUsedTag + DelimiterPlus + FFTelemetryUsedTag,
				FFMaxVariantsKey + "=3",
			},
			notExpected: []string{},
		},
		{
			name: "No feature flags features",
			tracing: &FeatureFlagTracing{
				UsesCustomFilter:     false,
				UsesTimeWindowFilter: false,
				UsesTargetingFilter:  false,
				UsesTelemetry:        false,
				UsesSeed:             false,
				MaxVariants:          0,
			},
			expected: []string{},
			notExpected: []string{
				FeatureFilterTypeKey,
				FFFeaturesKey,
				FFMaxVariantsKey,
			},
		},
		{
			name: "Only feature filters",
			tracing: &FeatureFlagTracing{
				UsesCustomFilter:     true,
				UsesTimeWindowFilter: false,
				UsesTargetingFilter:  true,
				UsesTelemetry:        false,
				UsesSeed:             false,
				MaxVariants:          0,
			},
			expected: []string{
				FeatureFilterTypeKey + "=" + CustomFilterKey + DelimiterPlus + TargetingFilterKey,
			},
			notExpected: []string{
				FFFeaturesKey,
				TimeWindowFilterKey,
			},
		},
		{
			name: "Only variant count",
			tracing: &FeatureFlagTracing{
				UsesCustomFilter:     false,
				UsesTimeWindowFilter: false,
				UsesTargetingFilter:  false,
				UsesTelemetry:        false,
				UsesSeed:             false,
				MaxVariants:          5,
			},
			expected: []string{
				FFMaxVariantsKey + "=5",
			},
			notExpected: []string{
				FeatureFilterTypeKey,
				FFFeaturesKey,
			},
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			ctx := context.Background()
			options := Options{
				FeatureFlagTracing: test.tracing,
			}

			header := CreateCorrelationContextHeader(ctx, options)
			correlationCtx := header.Get(CorrelationContextHeader)

			// Check expected strings
			for _, exp := range test.expected {
				assert.Contains(t, correlationCtx, exp)
			}

			// Check not expected strings
			for _, notExp := range test.notExpected {
				assert.NotContains(t, correlationCtx, notExp)
			}
		})
	}
}
