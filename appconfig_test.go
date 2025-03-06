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

func (m *MockSettingsClient) getSettings(ctx context.Context, client *azappconfig.Client) (*settingsResponse, error) {
	args := m.Called(ctx, client)
	return args.Get(0).(*settingsResponse), args.Error(1)
}

func TestLoadKeyValues_Success(t *testing.T) {
	ctx := context.Background()
	mockClient := new(MockSettingsClient)
	mockResponse := &settingsResponse{
		settings: []azappconfig.Setting{
			{Key: toPtr("key1"), Value: toPtr("value1"), ContentType: toPtr("")},
			{Key: toPtr("key2"), Value: toPtr(`{"jsonKey": "jsonValue"}`), ContentType: toPtr("application/json")},
		},

		eTags: map[Selector][]*azcore.ETag{},
	}

	mockClient.On("getSettings", ctx, mock.Anything).Return(mockResponse, nil)

	azappcfg := &AzureAppConfiguration{
		clientManager: &configurationClientManager{
			staticClients: []*configurationClientWrapper{{Client: &azappconfig.Client{}}},
		},
		kvSelectors:    getValidKeyValuesSelectors([]Selector{}),
		settingsClient: mockClient,
	}

	err := azappcfg.loadKeyValues(ctx)
	assert.NoError(t, err)
	assert.Equal(t, "value1", azappcfg.keyValues["key1"])
	assert.Equal(t, map[string]interface{}{"jsonKey": "jsonValue"}, azappcfg.keyValues["key2"])
}

func TestLoadKeyValues_WithTrimPrefix(t *testing.T) {
	ctx := context.Background()
	mockClient := new(MockSettingsClient)
	mockResponse := &settingsResponse{
		settings: []azappconfig.Setting{
			{Key: toPtr("prefix:key1"), Value: toPtr("value1"), ContentType: toPtr("")},
			{Key: toPtr("other:key2"), Value: toPtr("value2"), ContentType: toPtr("")},
			{Key: toPtr("key3"), Value: toPtr("value3"), ContentType: toPtr("")},
		},
		eTags: map[Selector][]*azcore.ETag{},
	}

	mockClient.On("getSettings", ctx, mock.Anything).Return(mockResponse, nil)

	azappcfg := &AzureAppConfiguration{
		clientManager: &configurationClientManager{
			staticClients: []*configurationClientWrapper{{Client: &azappconfig.Client{}}},
		},
		kvSelectors:    getValidKeyValuesSelectors([]Selector{}),
		trimPrefixes:   []string{"prefix:", "other:"},
		settingsClient: mockClient,
		keyValues:      make(map[string]any),
	}

	err := azappcfg.loadKeyValues(ctx)
	assert.NoError(t, err)
	assert.Equal(t, "value1", azappcfg.keyValues["key1"])
	assert.Equal(t, "value2", azappcfg.keyValues["key2"])
	assert.Equal(t, "value3", azappcfg.keyValues["key3"])
}

func TestLoadKeyValues_EmptyKeyAfterTrim(t *testing.T) {
	ctx := context.Background()
	mockClient := new(MockSettingsClient)
	mockResponse := &settingsResponse{
		settings: []azappconfig.Setting{
			{Key: toPtr("prefix:"), Value: toPtr("value1"), ContentType: toPtr("")},
		},
		eTags: map[Selector][]*azcore.ETag{},
	}

	mockClient.On("getSettings", ctx, mock.Anything).Return(mockResponse, nil)

	azappcfg := &AzureAppConfiguration{
		clientManager: &configurationClientManager{
			staticClients: []*configurationClientWrapper{{Client: &azappconfig.Client{}}},
		},
		kvSelectors:    getValidKeyValuesSelectors([]Selector{}),
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
	mockResponse := &settingsResponse{
		settings: []azappconfig.Setting{
			{Key: toPtr("key1"), Value: toPtr("value1"), ContentType: toPtr("")},
			{Key: toPtr("key2"), Value: toPtr(`{"jsonKey": invalid}`), ContentType: toPtr("application/json")},
		},
		eTags: map[Selector][]*azcore.ETag{},
	}

	mockClient.On("getSettings", ctx, mock.Anything).Return(mockResponse, nil)

	azappcfg := &AzureAppConfiguration{
		clientManager: &configurationClientManager{
			staticClients: []*configurationClientWrapper{{Client: &azappconfig.Client{}}},
		},
		kvSelectors:    getValidKeyValuesSelectors([]Selector{}),
		settingsClient: mockClient,
		keyValues:      make(map[string]any),
	}

	err := azappcfg.loadKeyValues(ctx)
	assert.NoError(t, err)
	assert.Equal(t, "value1", azappcfg.keyValues["key1"])
	// Even though JSON is invalid, the string value should still be stored
	assert.Equal(t, `{"jsonKey": invalid}`, azappcfg.keyValues["key2"])
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
				{KeyFilter: WildCard, LabelFilter: NullLabel},
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
				{KeyFilter: "one*", LabelFilter: NullLabel},
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
