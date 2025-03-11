// Copyright (c) Microsoft Corporation.
// Licensed under the MIT License.

package azureappconfiguration

import (
	"context"
	"testing"

	"github.com/Azure/azure-sdk-for-go/sdk/azcore"
	"github.com/Azure/azure-sdk-for-go/sdk/data/azappconfig"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
)

type MockSettingsClient struct {
	mock.Mock
}

func (m *MockSettingsClient) getSettings(ctx context.Context) (*settingsResponse, error) {
	args := m.Called(ctx)
	return args.Get(0).(*settingsResponse), args.Error(1)
}

func TestLoadKeyValues_Success(t *testing.T) {
	ctx := context.Background()
	mockClient := new(MockSettingsClient)
	value1 := "value1"
	value2 := `{"jsonKey": "jsonValue"}`
	mockResponse := &settingsResponse{
		settings: []azappconfig.Setting{
			{Key: toPtr("key1"), Value: &value1, ContentType: toPtr("")},
			{Key: toPtr("key2"), Value: &value2, ContentType: toPtr("application/json")},
		},
		eTags: map[Selector][]*azcore.ETag{},
	}

	mockClient.On("getSettings", ctx).Return(mockResponse, nil)

	azappcfg := &AzureAppConfiguration{
		clientManager: &configurationClientManager{
			staticClient: &configurationClientWrapper{client: &azappconfig.Client{}},
		},
		kvSelectors:    deduplicateSelectors([]Selector{}),
		settingsClient: mockClient,
		keyValues:      make(map[string]any),
	}

	err := azappcfg.loadKeyValues(ctx)
	assert.NoError(t, err)
	// We should expect pointer values in the keyValues map
	assert.Equal(t, &value1, azappcfg.keyValues["key1"])
	// For JSON content types, we still expect the parsed value
	assert.Equal(t, map[string]interface{}{"jsonKey": "jsonValue"}, azappcfg.keyValues["key2"])
}

func TestLoadKeyValues_WithTrimPrefix(t *testing.T) {
	ctx := context.Background()
	mockClient := new(MockSettingsClient)
	value1 := "value1"
	value2 := "value2"
	value3 := "value3"
	mockResponse := &settingsResponse{
		settings: []azappconfig.Setting{
			{Key: toPtr("prefix:key1"), Value: &value1, ContentType: toPtr("")},
			{Key: toPtr("other:key2"), Value: &value2, ContentType: toPtr("")},
			{Key: toPtr("key3"), Value: &value3, ContentType: toPtr("")},
		},
		eTags: map[Selector][]*azcore.ETag{},
	}

	mockClient.On("getSettings", ctx).Return(mockResponse, nil)

	azappcfg := &AzureAppConfiguration{
		clientManager: &configurationClientManager{
			staticClient: &configurationClientWrapper{client: &azappconfig.Client{}},
		},
		kvSelectors:    deduplicateSelectors([]Selector{}),
		trimPrefixes:   []string{"prefix:", "other:"},
		settingsClient: mockClient,
		keyValues:      make(map[string]any),
	}

	err := azappcfg.loadKeyValues(ctx)
	assert.NoError(t, err)
	// We should expect pointer values in the keyValues map
	assert.Equal(t, &value1, azappcfg.keyValues["key1"])
	assert.Equal(t, &value2, azappcfg.keyValues["key2"])
	assert.Equal(t, &value3, azappcfg.keyValues["key3"])
}

func TestLoadKeyValues_EmptyKeyAfterTrim(t *testing.T) {
	ctx := context.Background()
	mockClient := new(MockSettingsClient)
	value1 := "value1"
	mockResponse := &settingsResponse{
		settings: []azappconfig.Setting{
			{Key: toPtr("prefix:"), Value: &value1, ContentType: toPtr("")},
		},
		eTags: map[Selector][]*azcore.ETag{},
	}

	mockClient.On("getSettings", ctx).Return(mockResponse, nil)

	azappcfg := &AzureAppConfiguration{
		clientManager: &configurationClientManager{
			staticClient: &configurationClientWrapper{client: &azappconfig.Client{}},
		},
		kvSelectors:    deduplicateSelectors([]Selector{}),
		trimPrefixes:   []string{"prefix:"},
		settingsClient: mockClient,
		keyValues:      make(map[string]any),
	}

	err := azappcfg.loadKeyValues(ctx)
	assert.NoError(t, err)
	assert.Empty(t, azappcfg.keyValues)
}

func TestLoadKeyValues_InvalidJson(t *testing.T) {
	ctx := context.Background()
	mockClient := new(MockSettingsClient)
	value1 := "value1"
	value2 := `{"jsonKey": invalid}`
	mockResponse := &settingsResponse{
		settings: []azappconfig.Setting{
			{Key: toPtr("key1"), Value: &value1, ContentType: toPtr("")},
			{Key: toPtr("key2"), Value: &value2, ContentType: toPtr("application/json")},
		},
		eTags: map[Selector][]*azcore.ETag{},
	}

	mockClient.On("getSettings", ctx).Return(mockResponse, nil)

	azappcfg := &AzureAppConfiguration{
		clientManager: &configurationClientManager{
			staticClient: &configurationClientWrapper{client: &azappconfig.Client{}},
		},
		kvSelectors:    deduplicateSelectors([]Selector{}),
		settingsClient: mockClient,
		keyValues:      make(map[string]any),
	}

	err := azappcfg.loadKeyValues(ctx)
	assert.NoError(t, err)
	assert.Len(t, azappcfg.keyValues, 1)
	assert.Equal(t, &value1, azappcfg.keyValues["key1"])
}

func TestDeduplicateSelectors(t *testing.T) {
	tests := []struct {
		name           string
		input          []Selector
		expectedOutput []Selector
	}{
		{
			name:  "empty input",
			input: []Selector{},
			expectedOutput: []Selector{
				{KeyFilter: wildCard, LabelFilter: nullLabel},
			},
		},
		{
			name: "no duplicates",
			input: []Selector{
				{KeyFilter: "one*", LabelFilter: "prod"},
				{KeyFilter: "two*", LabelFilter: "dev"},
			},
			expectedOutput: []Selector{
				{KeyFilter: "one*", LabelFilter: "prod"},
				{KeyFilter: "two*", LabelFilter: "dev"},
			},
		},
		{
			name: "with duplicates",
			input: []Selector{
				{KeyFilter: "one*", LabelFilter: "prod"},
				{KeyFilter: "two*", LabelFilter: "dev"},
				{KeyFilter: "one*", LabelFilter: "prod"},
			},
			expectedOutput: []Selector{
				{KeyFilter: "two*", LabelFilter: "dev"},
				{KeyFilter: "one*", LabelFilter: "prod"},
			},
		},
		{
			name: "normalize empty label",
			input: []Selector{
				{KeyFilter: "one*", LabelFilter: ""},
				{KeyFilter: "two*", LabelFilter: "dev"},
			},
			expectedOutput: []Selector{
				{KeyFilter: "one*", LabelFilter: nullLabel},
				{KeyFilter: "two*", LabelFilter: "dev"},
			},
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			result := deduplicateSelectors(test.input)
			assert.Equal(t, test.expectedOutput, result)
		})
	}
}

func TestTrimPrefix(t *testing.T) {
	tests := []struct {
		name           string
		key            string
		prefixesToTrim []string
		expected       string
	}{
		{
			name:           "no prefixes to trim",
			key:            "appSettings:theme",
			prefixesToTrim: []string{},
			expected:       "appSettings:theme",
		},
		{
			name:           "empty prefixes list",
			key:            "appSettings:theme",
			prefixesToTrim: nil,
			expected:       "appSettings:theme",
		},
		{
			name:           "matching prefix",
			key:            "appSettings:theme",
			prefixesToTrim: []string{"appSettings:"},
			expected:       "theme",
		},
		{
			name:           "non-matching prefix",
			key:            "appSettings:theme",
			prefixesToTrim: []string{"config:"},
			expected:       "appSettings:theme",
		},
		{
			name:           "multiple prefixes with match",
			key:            "appSettings:theme",
			prefixesToTrim: []string{"config:", "appSettings:", "settings:"},
			expected:       "theme",
		},
		{
			name:           "multiple prefixes with no match",
			key:            "appSettings:theme",
			prefixesToTrim: []string{"config:", "settings:"},
			expected:       "appSettings:theme",
		},
		{
			name:           "prefix equals key",
			key:            "appSettings:",
			prefixesToTrim: []string{"appSettings:"},
			expected:       "",
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			azappcfg := &AzureAppConfiguration{
				trimPrefixes: test.prefixesToTrim,
			}
			result := azappcfg.trimPrefix(test.key)
			assert.Equal(t, test.expected, result)
		})
	}
}

func TestIsJsonContentType(t *testing.T) {
	tests := []struct {
		name        string
		contentType *string
		expected    bool
	}{
		{
			name:        "nil content type",
			contentType: nil,
			expected:    false,
		},
		{
			name:        "empty content type",
			contentType: toPtr(""),
			expected:    false,
		},
		{
			name:        "standard JSON content type",
			contentType: toPtr("application/json"),
			expected:    true,
		},
		{
			name:        "JSON content type with charset",
			contentType: toPtr("application/json; charset=utf-8"),
			expected:    true,
		},
		{
			name:        "JSON content type with vendor prefix",
			contentType: toPtr("application/vnd+json"),
			expected:    true,
		},
		{
			name:        "non-JSON content type",
			contentType: toPtr("text/plain"),
			expected:    false,
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			result := isJsonContentType(test.contentType)
			assert.Equal(t, test.expected, result)
		})
	}
}

func toPtr(s string) *string {
	return &s
}
