// Copyright (c) Microsoft Corporation.
// Licensed under the MIT License.

package tracing

type FeatureFlagTracing struct {
	UsesCustomFilter     bool
	UsesTimeWindowFilter bool
	UsesTargetingFilter  bool
	UsesTelemetry        bool
	UsesSeed             bool
	MaxVariants          int
}

func (f *FeatureFlagTracing) UpdateFeatureFilterTracing(filterName string) {
	if filterName == TimeWindowFilterName {
		f.UsesTimeWindowFilter = true
	} else if filterName == TargetingFilterName {
		f.UsesTargetingFilter = true
	} else {
		f.UsesCustomFilter = true
	}
}

func (f *FeatureFlagTracing) UpdateMaxVariants(currentVariants int) {
	if currentVariants > f.MaxVariants {
		f.MaxVariants = currentVariants
	}
}

func (f *FeatureFlagTracing) UsesAnyFeatureFilter() bool {
	return f.UsesCustomFilter || f.UsesTimeWindowFilter || f.UsesTargetingFilter
}

func (f *FeatureFlagTracing) UsesAnyTracingFeature() bool {
	return f.UsesSeed || f.UsesTelemetry
}

func (f *FeatureFlagTracing) CreateFeatureFiltersString() string {
	if !f.UsesAnyFeatureFilter() {
		return ""
	}

	var result string

	if f.UsesCustomFilter {
		result += CustomFilterKey
	}

	if f.UsesTimeWindowFilter {
		if result != "" {
			result += DelimiterPlus
		}
		result += TimeWindowFilterKey
	}

	if f.UsesTargetingFilter {
		if result != "" {
			result += DelimiterPlus
		}
		result += TargetingFilterKey
	}

	return result
}

// CreateFeaturesString creates a string representation of the used tracing features
func (f *FeatureFlagTracing) CreateFeaturesString() string {
	if !f.UsesAnyTracingFeature() {
		return ""
	}

	var result string

	if f.UsesSeed {
		result += FFSeedUsedTag
	}

	if f.UsesTelemetry {
		if result != "" {
			result += DelimiterPlus
		}
		result += FFTelemetryUsedTag
	}

	return result
}
