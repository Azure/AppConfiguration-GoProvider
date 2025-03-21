// Copyright (c) Microsoft Corporation.
// Licensed under the MIT License.

package azureappconfiguration

import (
	"testing"
	"time"

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
			name: "invalid refresh interval",
			options: &Options{
				RefreshOptions: KeyValueRefreshOptions{
					Enabled:  true,
					Interval: time.Millisecond * 500, // Less than minimum (1 second)
				},
			},
			expectedError: true,
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
		{
			name: "invalid watched setting key",
			options: &Options{
				RefreshOptions: KeyValueRefreshOptions{
					Enabled: true,
					WatchedSettings: []WatchedSetting{
						{Key: "app*", Label: "prod"},
					},
				},
			},
			expectedError: true,
		},
		{
			name: "invalid watched setting label",
			options: &Options{
				RefreshOptions: KeyValueRefreshOptions{
					Enabled: true,
					WatchedSettings: []WatchedSetting{
						{Key: "app", Label: "prod*"},
					},
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
