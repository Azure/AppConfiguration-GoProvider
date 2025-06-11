// Copyright (c) Microsoft Corporation.
// Licensed under the MIT License.

package azureappconfiguration

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestVerifyOptions(t *testing.T) {
	tests := []struct {
		name          string
		options       *Options
		expectedError bool
	}{
		{
			name:          "nil options",
			options:       nil,
			expectedError: false,
		},
		{
			name: "valid options with no enabled refresh",
			options: &Options{
				Selectors: []Selector{
					{KeyFilter: "app1*", LabelFilter: "prod"},
				},
			},
			expectedError: false,
		},
		{
			name: "empty key filter in selector",
			options: &Options{
				Selectors: []Selector{
					{KeyFilter: "", LabelFilter: "prod"},
				},
			},
			expectedError: true,
		},
		{
			name: "label filter with wildcard",
			options: &Options{
				Selectors: []Selector{
					{KeyFilter: "app*", LabelFilter: "prod*"},
				},
			},
			expectedError: true,
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			err := verifyOptions(test.options)
			if test.expectedError {
				assert.Error(t, err)
			} else {
				assert.NoError(t, err)
			}
		})
	}
}

func TestVerifySelectors(t *testing.T) {
	tests := []struct {
		name          string
		selectors     []Selector
		expectedError bool
	}{
		{
			name:          "empty selector list",
			selectors:     []Selector{},
			expectedError: false,
		},
		{
			name: "valid selectors",
			selectors: []Selector{
				{KeyFilter: "app1*", LabelFilter: "prod"},
				{KeyFilter: "app2*", LabelFilter: "dev"},
			},
			expectedError: false,
		},
		{
			name: "empty key filter",
			selectors: []Selector{
				{KeyFilter: "", LabelFilter: "prod"},
			},
			expectedError: true,
		},
		{
			name: "label filter with wildcard",
			selectors: []Selector{
				{KeyFilter: "app*", LabelFilter: "prod*"},
			},
			expectedError: true,
		},
		{
			name: "label filter with comma",
			selectors: []Selector{
				{KeyFilter: "app*", LabelFilter: "prod,test"},
			},
			expectedError: true,
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			err := verifySelectors(test.selectors)
			if test.expectedError {
				assert.Error(t, err)
			} else {
				assert.NoError(t, err)
			}
		})
	}
}

func TestReverse(t *testing.T) {
	tests := []struct {
		name     string
		input    []Selector
		expected []Selector
	}{
		{
			name:     "empty list",
			input:    []Selector{},
			expected: []Selector{},
		},
		{
			name: "single item",
			input: []Selector{
				{KeyFilter: "key1", LabelFilter: "label1"},
			},
			expected: []Selector{
				{KeyFilter: "key1", LabelFilter: "label1"},
			},
		},
		{
			name: "multiple items",
			input: []Selector{
				{KeyFilter: "key1", LabelFilter: "label1"},
				{KeyFilter: "key2", LabelFilter: "label2"},
				{KeyFilter: "key3", LabelFilter: "label3"},
			},
			expected: []Selector{
				{KeyFilter: "key3", LabelFilter: "label3"},
				{KeyFilter: "key2", LabelFilter: "label2"},
				{KeyFilter: "key1", LabelFilter: "label1"},
			},
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			// Create a copy of the input to avoid modifying the test case
			input := make([]Selector, len(test.input))
			copy(input, test.input)

			reverse(input)
			assert.Equal(t, test.expected, input)
		})
	}
}

func TestVerifySeparator(t *testing.T) {
	tests := []struct {
		name          string
		separator     string
		expectedError bool
	}{
		{
			name:          "dot separator",
			separator:     ".",
			expectedError: false,
		},
		{
			name:          "comma separator",
			separator:     ",",
			expectedError: false,
		},
		{
			name:          "semicolon separator",
			separator:     ";",
			expectedError: false,
		},
		{
			name:          "dash separator",
			separator:     "-",
			expectedError: false,
		},
		{
			name:          "underscore separator",
			separator:     "_",
			expectedError: false,
		},
		{
			name:          "double underscore separator",
			separator:     "__",
			expectedError: false,
		},
		{
			name:          "slash separator",
			separator:     "/",
			expectedError: false,
		},
		{
			name:          "colon separator",
			separator:     ":",
			expectedError: false,
		},
		{
			name:          "invalid separator",
			separator:     "|",
			expectedError: true,
		},
		{
			name:          "invalid separator (space)",
			separator:     " ",
			expectedError: true,
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			err := verifySeparator(test.separator)
			if test.expectedError {
				assert.Error(t, err)
			} else {
				assert.NoError(t, err)
			}
		})
	}
}

func TestIsAIConfigurationContentType(t *testing.T) {
	tests := []struct {
		name        string
		contentType *string
		expected    bool
	}{
		{
			name:        "valid AI configuration content type",
			contentType: strPtr("application/json; profile=\"https://azconfig.io/mime-profiles/ai\""),
			expected:    true,
		},
		{
			name:        "valid AI configuration content type with extra parameters",
			contentType: strPtr("application/json; charset=utf-8; profile=\"https://azconfig.io/mime-profiles/ai\"; param=value"),
			expected:    true,
		},
		{
			name:        "invalid AI configuration content type - missing profile keyword",
			contentType: strPtr("application/json; \"https://azconfig.io/mime-profiles/ai\""),
			expected:    false,
		},
		{
			name:        "invalid content type - wrong profile",
			contentType: strPtr("application/json; profile=\"https://azconfig.io/mime-profiles/other\""),
			expected:    false,
		},
		{
			name:        "invalid content type - partial match",
			contentType: strPtr("application/json; profile=\"prefix-https://azconfig.io/mime-profiles/ai\""),
			expected:    false,
		},
		{
			name:        "invalid content type - not JSON",
			contentType: strPtr("text/plain; profile=\"https://azconfig.io/mime-profiles/ai\""),
			expected:    false,
		},
		{
			name:        "empty content type",
			contentType: strPtr(""),
			expected:    false,
		},
		{
			name:        "nil content type",
			contentType: nil,
			expected:    false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := isJsonContentType(tt.contentType) && isAIConfigurationContentType(tt.contentType)
			if result != tt.expected {
				t.Errorf("isAIConfigurationContentType(%v) = %v, want %v",
					tt.contentType, result, tt.expected)
			}
		})
	}
}

func TestIsAIChatCompletionContentType(t *testing.T) {
	tests := []struct {
		name        string
		contentType *string
		expected    bool
	}{
		{
			name:        "valid AI chat completion content type",
			contentType: strPtr("application/json; profile=\"https://azconfig.io/mime-profiles/ai/chat-completion\""),
			expected:    true,
		},
		{
			name:        "valid AI chat completion with multiple parameters",
			contentType: strPtr("application/json; charset=utf-8; profile=\"https://azconfig.io/mime-profiles/ai/chat-completion\"; param=value"),
			expected:    true,
		},
		{
			name:        "invalid content type - missing profile keyword",
			contentType: strPtr("application/json; \"https://azconfig.io/mime-profiles/ai/chat-completion\""),
			expected:    false,
		},
		{
			name:        "invalid content type - wrong profile",
			contentType: strPtr("application/json; profile=\"https://azconfig.io/mime-profiles/other\""),
			expected:    false,
		},
		{
			name:        "invalid content type - partial match",
			contentType: strPtr("application/json; profile=\"prefix-https://azconfig.io/mime-profiles/ai/chat-completion\""),
			expected:    false,
		},
		{
			name:        "invalid content type - not JSON",
			contentType: strPtr("text/plain; profile=\"https://azconfig.io/mime-profiles/ai/chat-completion\""),
			expected:    false,
		},
		{
			name:        "JSON content type without AI chat completion profile",
			contentType: strPtr("application/json"),
			expected:    false,
		},
		{
			name:        "empty content type",
			contentType: strPtr(""),
			expected:    false,
		},
		{
			name:        "nil content type",
			contentType: nil,
			expected:    false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := isJsonContentType(tt.contentType) && isAIChatCompletionContentType(tt.contentType)
			if result != tt.expected {
				t.Errorf("isAIChatCompletionContentType(%v) = %v, want %v",
					tt.contentType, result, tt.expected)
			}
		})
	}
}

func TestVerifyRefreshOptions(t *testing.T) {
	tests := []struct {
		name          string
		options       *Options
		expectedError string // Empty string means no error expected
	}{
		// KeyValue refresh options tests
		{
			name: "valid key value refresh interval",
			options: &Options{
				RefreshOptions: KeyValueRefreshOptions{
					Enabled:  true,
					Interval: 5 * minimalRefreshInterval,
				},
			},
			expectedError: "",
		},
		{
			name: "too small key value refresh interval",
			options: &Options{
				RefreshOptions: KeyValueRefreshOptions{
					Enabled:  true,
					Interval: minimalRefreshInterval / 2,
				},
			},
			expectedError: "key value refresh interval cannot be less than",
		},
		{
			name: "valid watched settings",
			options: &Options{
				RefreshOptions: KeyValueRefreshOptions{
					Enabled: true,
					WatchedSettings: []WatchedSetting{
						{Key: "validKey", Label: "validLabel"},
						{Key: "anotherKey"},
					},
				},
			},
			expectedError: "",
		},
		{
			name: "empty key in watched setting",
			options: &Options{
				RefreshOptions: KeyValueRefreshOptions{
					Enabled: true,
					WatchedSettings: []WatchedSetting{
						{Key: "", Label: "validLabel"},
					},
				},
			},
			expectedError: "watched setting key cannot be empty",
		},
		{
			name: "wildcard in key of watched setting",
			options: &Options{
				RefreshOptions: KeyValueRefreshOptions{
					Enabled: true,
					WatchedSettings: []WatchedSetting{
						{Key: "invalid*Key", Label: "validLabel"},
					},
				},
			},
			expectedError: "watched setting key cannot contain",
		},
		{
			name: "comma in key of watched setting",
			options: &Options{
				RefreshOptions: KeyValueRefreshOptions{
					Enabled: true,
					WatchedSettings: []WatchedSetting{
						{Key: "invalid,Key", Label: "validLabel"},
					},
				},
			},
			expectedError: "watched setting key cannot contain",
		},
		{
			name: "wildcard in label of watched setting",
			options: &Options{
				RefreshOptions: KeyValueRefreshOptions{
					Enabled: true,
					WatchedSettings: []WatchedSetting{
						{Key: "validKey", Label: "invalid*Label"},
					},
				},
			},
			expectedError: "watched setting label cannot contain",
		},
		{
			name: "comma in label of watched setting",
			options: &Options{
				RefreshOptions: KeyValueRefreshOptions{
					Enabled: true,
					WatchedSettings: []WatchedSetting{
						{Key: "validKey", Label: "invalid,Label"},
					},
				},
			},
			expectedError: "watched setting label cannot contain",
		},
		{
			name: "empty label is allowed in watched setting",
			options: &Options{
				RefreshOptions: KeyValueRefreshOptions{
					Enabled: true,
					WatchedSettings: []WatchedSetting{
						{Key: "validKey", Label: ""},
					},
				},
			},
			expectedError: "",
		},

		// KeyVault refresh options tests
		{
			name: "valid Key Vault refresh interval",
			options: &Options{
				KeyVaultOptions: KeyVaultOptions{
					RefreshOptions: RefreshOptions{
						Enabled:  true,
						Interval: 5 * minimalKeyVaultRefreshInterval,
					},
				},
			},
			expectedError: "",
		},
		{
			name: "too small Key Vault refresh interval",
			options: &Options{
				KeyVaultOptions: KeyVaultOptions{
					RefreshOptions: RefreshOptions{
						Enabled:  true,
						Interval: minimalKeyVaultRefreshInterval / 2,
					},
				},
			},
			expectedError: "refresh interval of Key Vault secrets cannot be less than",
		},

		// Feature Flag refresh options tests
		{
			name: "valid feature flag refresh interval",
			options: &Options{
				FeatureFlagOptions: FeatureFlagOptions{
					Enabled: true,
					RefreshOptions: RefreshOptions{
						Enabled:  true,
						Interval: 5 * minimalRefreshInterval,
					},
				},
			},
			expectedError: "",
		},
		{
			name: "too small feature flag refresh interval",
			options: &Options{
				FeatureFlagOptions: FeatureFlagOptions{
					Enabled: true,
					RefreshOptions: RefreshOptions{
						Enabled:  true,
						Interval: minimalRefreshInterval / 2,
					},
				},
			},
			expectedError: "feature flag refresh interval cannot be less than",
		},
		{
			name: "valid feature flag selectors",
			options: &Options{
				FeatureFlagOptions: FeatureFlagOptions{
					Enabled: true,
					Selectors: []Selector{
						{KeyFilter: "validKey", LabelFilter: "validLabel"},
					},
				},
			},
			expectedError: "",
		},
		{
			name: "invalid feature flag selectors - empty key filter",
			options: &Options{
				FeatureFlagOptions: FeatureFlagOptions{
					Enabled: true,
					Selectors: []Selector{
						{KeyFilter: "", LabelFilter: "validLabel"},
					},
				},
			},
			expectedError: "key filter cannot be empty",
		},

		// Combined scenarios
		{
			name: "multiple valid refresh options",
			options: &Options{
				RefreshOptions: KeyValueRefreshOptions{
					Enabled:  true,
					Interval: 10 * minimalRefreshInterval,
				},
				KeyVaultOptions: KeyVaultOptions{
					RefreshOptions: RefreshOptions{
						Enabled:  true,
						Interval: 10 * minimalKeyVaultRefreshInterval,
					},
				},
				FeatureFlagOptions: FeatureFlagOptions{
					Enabled: true,
					RefreshOptions: RefreshOptions{
						Enabled:  true,
						Interval: 10 * minimalRefreshInterval,
					},
				},
			},
			expectedError: "",
		},
		{
			name: "multiple refresh options with one invalid",
			options: &Options{
				RefreshOptions: KeyValueRefreshOptions{
					Enabled:  true,
					Interval: 10 * minimalRefreshInterval,
				},
				KeyVaultOptions: KeyVaultOptions{
					RefreshOptions: RefreshOptions{
						Enabled:  true,
						Interval: minimalKeyVaultRefreshInterval / 2, // Invalid
					},
				},
				FeatureFlagOptions: FeatureFlagOptions{
					Enabled: true,
					RefreshOptions: RefreshOptions{
						Enabled:  true,
						Interval: 10 * minimalRefreshInterval,
					},
				},
			},
			expectedError: "refresh interval of Key Vault secrets cannot be less than",
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			err := verifyOptions(test.options)
			if test.expectedError == "" {
				assert.NoError(t, err)
			} else {
				assert.Error(t, err)
				assert.Contains(t, err.Error(), test.expectedError)
			}
		})
	}
}

func TestGetFeatureFlagSelectors(t *testing.T) {
	tests := []struct {
		name     string
		input    []Selector
		expected []Selector
	}{
		{
			name: "single selector",
			input: []Selector{
				{KeyFilter: "Beta", LabelFilter: "dev"},
			},
			expected: []Selector{
				{KeyFilter: featureFlagKeyPrefix + "Beta", LabelFilter: "dev"},
			},
		},
		{
			name: "multiple selectors",
			input: []Selector{
				{KeyFilter: "Beta", LabelFilter: "dev"},
				{KeyFilter: "Alpha", LabelFilter: "prod"},
				{KeyFilter: "*", LabelFilter: ""},
			},
			expected: []Selector{
				{KeyFilter: featureFlagKeyPrefix + "Beta", LabelFilter: "dev"},
				{KeyFilter: featureFlagKeyPrefix + "Alpha", LabelFilter: "prod"},
				{KeyFilter: featureFlagKeyPrefix + "*", LabelFilter: ""},
			},
		},
		{
			name:     "empty input",
			input:    []Selector{},
			expected: []Selector{},
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			result := getFeatureFlagSelectors(test.input)
			assert.Equal(t, test.expected, result)
		})
	}
}

// Helper function to create string pointers for tests
func strPtr(s string) *string {
	return &s
}
