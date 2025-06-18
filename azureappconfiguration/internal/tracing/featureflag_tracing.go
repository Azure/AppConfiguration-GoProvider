// Copyright (c) Microsoft Corporation.
// Licensed under the MIT License.

package tracing

import "strings"

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

	res := make([]string, 0, 3)

	if f.UsesCustomFilter {
		res = append(res, CustomFilterKey)
	}

	if f.UsesTimeWindowFilter {
		res = append(res, TimeWindowFilterKey)
	}

	if f.UsesTargetingFilter {
		res = append(res, TargetingFilterKey)
	}

	return strings.Join(res, DelimiterPlus)
}

// CreateFeaturesString creates a string representation of the used tracing features
func (f *FeatureFlagTracing) CreateFeaturesString() string {
	if !f.UsesAnyTracingFeature() {
		return ""
	}

	res := make([]string, 0, 2)

	if f.UsesSeed {
		res = append(res, FFSeedUsedTag)
	}

	if f.UsesTelemetry {
		res = append(res, FFTelemetryUsedTag)
	}

	return strings.Join(res, DelimiterPlus)
}
