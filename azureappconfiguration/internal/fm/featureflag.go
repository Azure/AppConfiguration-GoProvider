// Copyright (c) Microsoft Corporation.
// Licensed under the MIT License.

package fm

type FeatureManagement struct {
	FeatureFlags []FeatureFlag `json:"feature_flags"`
}

// FeatureFlag represents a feature flag definition according to the v2.0.0 schema
type FeatureFlag struct {
	// ID uniquely identifies the feature
	ID string `json:"id"`
	// Description provides details about the feature's purpose
	Description string `json:"description,omitempty"`
	// DisplayName is a human-friendly name for display purposes
	DisplayName string `json:"display_name,omitempty"`
	// Enabled indicates if the feature is on or off
	Enabled bool `json:"enabled"`
	// Conditions defines when the feature should be dynamically enabled
	Conditions *Conditions `json:"conditions,omitempty"`
	// Variants represents different configurations of this feature
	Variants []VariantDefinition `json:"variants,omitempty"`
	// Allocation determines how variants are assigned to users
	Allocation *VariantAllocation `json:"allocation,omitempty"`
	// Telemetry contains feature flag telemetry configuration
	Telemetry *Telemetry `json:"telemetry,omitempty"`
}

// Conditions defines the rules for enabling a feature dynamically
type Conditions struct {
	// RequirementType determines if any or all filters must be satisfied
	// Values: "Any" or "All"
	RequirementType RequirementType `json:"requirement_type,omitempty"`
	// ClientFilters are the filter conditions that must be evaluated by the client
	ClientFilters []ClientFilter `json:"client_filters,omitempty"`
}

// ClientFilter represents a filter that must be evaluated for feature enablement
type ClientFilter struct {
	// Name is the identifier for this filter type
	Name string `json:"name"`
	// Parameters are the configuration values for the filter
	Parameters map[string]any `json:"parameters,omitempty"`
}

// VariantDefinition represents a feature configuration variant
type VariantDefinition struct {
	// Name uniquely identifies this variant
	Name string `json:"name"`
	// ConfigurationValue holds the value for this variant
	ConfigurationValue any `json:"configuration_value,omitempty"`
	// StatusOverride overrides the enabled state of the feature when this variant is assigned
	// Values: "None", "Enabled", "Disabled"
	StatusOverride StatusOverride `json:"status_override,omitempty"`
}

// VariantAllocation defines rules for assigning variants to users
type VariantAllocation struct {
	// DefaultWhenDisabled specifies which variant to use when feature is disabled
	DefaultWhenDisabled string `json:"default_when_disabled,omitempty"`
	// DefaultWhenEnabled specifies which variant to use when feature is enabled
	DefaultWhenEnabled string `json:"default_when_enabled,omitempty"`
	// User defines variant assignments for specific users
	User []UserAllocation `json:"user,omitempty"`
	// Group defines variant assignments for user groups
	Group []GroupAllocation `json:"group,omitempty"`
	// Percentile defines variant assignments by percentage ranges
	Percentile []PercentileAllocation `json:"percentile,omitempty"`
	// Seed is used to ensure consistent percentile calculations across features
	Seed string `json:"seed,omitempty"`
}

// UserAllocation assigns a variant to specific users
type UserAllocation struct {
	// Variant is the name of the variant to use
	Variant string `json:"variant"`
	// Users is the collection of user IDs to apply this variant to
	Users []string `json:"users"`
}

// GroupAllocation assigns a variant to specific user groups
type GroupAllocation struct {
	// Variant is the name of the variant to use
	Variant string `json:"variant"`
	// Groups is the collection of group IDs to apply this variant to
	Groups []string `json:"groups"`
}

// PercentileAllocation assigns a variant to a percentage range of users
type PercentileAllocation struct {
	// Variant is the name of the variant to use
	Variant string `json:"variant"`
	// From is the lower end of the percentage range (0-100)
	From float64 `json:"from"`
	// To is the upper end of the percentage range (0-100)
	To float64 `json:"to"`
}

// Telemetry contains options for feature flag telemetry
type Telemetry struct {
	// Enabled indicates if telemetry is enabled for this feature
	Enabled bool `json:"enabled,omitempty"`
	// Metadata contains additional data to include with telemetry
	Metadata map[string]string `json:"metadata,omitempty"`
}

// VariantAssignmentReason represents the reason a variant was assigned
type VariantAssignmentReason string

const (
	// VariantAssignmentReasonNone indicates no specific reason for variant assignment
	VariantAssignmentReasonNone VariantAssignmentReason = "None"
	// VariantAssignmentReasonDefaultWhenDisabled indicates the variant was assigned because it's the default for disabled features
	VariantAssignmentReasonDefaultWhenDisabled VariantAssignmentReason = "DefaultWhenDisabled"
	// VariantAssignmentReasonDefaultWhenEnabled indicates the variant was assigned because it's the default for enabled features
	VariantAssignmentReasonDefaultWhenEnabled VariantAssignmentReason = "DefaultWhenEnabled"
	// VariantAssignmentReasonUser indicates the variant was assigned based on the user's ID
	VariantAssignmentReasonUser VariantAssignmentReason = "User"
	// VariantAssignmentReasonGroup indicates the variant was assigned based on the user's group
	VariantAssignmentReasonGroup VariantAssignmentReason = "Group"
	// VariantAssignmentReasonPercentile indicates the variant was assigned based on percentile calculations
	VariantAssignmentReasonPercentile VariantAssignmentReason = "Percentile"
)

type RequirementType string

const (
	// RequirementTypeAny indicates that any of the filters must be satisfied
	RequirementTypeAny RequirementType = "Any"
	// RequirementTypeAll indicates that all filters must be satisfied
	RequirementTypeAll RequirementType = "All"
)

type StatusOverride string

const (
	// StatusOverrideNone indicates no override
	StatusOverrideNone StatusOverride = "None"
	// StatusOverrideEnabled indicates the feature is enabled
	StatusOverrideEnabled StatusOverride = "Enabled"
	// StatusOverrideDisabled indicates the feature is disabled
	StatusOverrideDisabled StatusOverride = "Disabled"
)
