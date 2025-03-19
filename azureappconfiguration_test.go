// Copyright (c) Microsoft Corporation.
// Licensed under the MIT License.

package azureappconfiguration

import (
	"context"
	"net/url"
	"sync"
	"testing"
	"time"

	"github.com/Azure/azure-sdk-for-go/sdk/azcore"
	"github.com/Azure/azure-sdk-for-go/sdk/data/azappconfig"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
)

type mockSettingsClient struct {
	mock.Mock
}

func (m *mockSettingsClient) getSettings(ctx context.Context) (*settingsResponse, error) {
	args := m.Called(ctx)
	return args.Get(0).(*settingsResponse), args.Error(1)
}

func TestLoadKeyValues_Success(t *testing.T) {
	ctx := context.Background()
	mockClient := new(mockSettingsClient)
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
		kvSelectors: deduplicateSelectors([]Selector{}),
		keyValues:   make(map[string]any),
	}

	err := azappcfg.loadKeyValues(ctx, mockClient)
	assert.NoError(t, err)
	assert.Equal(t, &value1, azappcfg.keyValues["key1"])
	assert.Equal(t, map[string]interface{}{"jsonKey": "jsonValue"}, azappcfg.keyValues["key2"])
}

func TestLoadKeyValues_WithKeyVaultReferences(t *testing.T) {
	ctx := context.Background()
	mockSettingsClient := new(mockSettingsClient)
	mockSecretResolver := new(mockSecretResolver)

	kvReference := `{"uri":"https://myvault.vault.azure.net/secrets/mysecret"}`
	mockResponse := &settingsResponse{
		settings: []azappconfig.Setting{
			{Key: toPtr("key1"), Value: toPtr("value1"), ContentType: toPtr("")},
			{Key: toPtr("secret1"), Value: toPtr(kvReference), ContentType: toPtr(secretReferenceContentType)},
		},
		eTags: map[Selector][]*azcore.ETag{},
	}

	mockSettingsClient.On("getSettings", ctx).Return(mockResponse, nil)
	expectedURL, _ := url.Parse("https://myvault.vault.azure.net/secrets/mysecret")
	mockSecretResolver.On("ResolveSecret", ctx, *expectedURL).Return("resolved-secret", nil)

	azappcfg := &AzureAppConfiguration{
		clientManager: &configurationClientManager{
			staticClient: &configurationClientWrapper{client: nil},
		},
		kvSelectors: deduplicateSelectors([]Selector{}),
		keyValues:   make(map[string]any),
		resolver: &keyVaultReferenceResolver{
			clients:        sync.Map{},
			secretResolver: mockSecretResolver,
		},
	}

	err := azappcfg.loadKeyValues(ctx, mockSettingsClient)

	assert.NoError(t, err)
	assert.Equal(t, "value1", *azappcfg.keyValues["key1"].(*string))
	assert.Equal(t, "resolved-secret", azappcfg.keyValues["secret1"])
	mockSettingsClient.AssertExpectations(t)
	mockSecretResolver.AssertExpectations(t)
}

func TestLoadKeyValues_WithTrimPrefix(t *testing.T) {
	ctx := context.Background()
	mockClient := new(mockSettingsClient)
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
		kvSelectors:  deduplicateSelectors([]Selector{}),
		trimPrefixes: []string{"prefix:", "other:"},
		keyValues:    make(map[string]any),
	}

	err := azappcfg.loadKeyValues(ctx, mockClient)
	assert.NoError(t, err)
	assert.Equal(t, &value1, azappcfg.keyValues["key1"])
	assert.Equal(t, &value2, azappcfg.keyValues["key2"])
	assert.Equal(t, &value3, azappcfg.keyValues["key3"])
}

func TestLoadKeyValues_EmptyKeyAfterTrim(t *testing.T) {
	ctx := context.Background()
	mockClient := new(mockSettingsClient)
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
		kvSelectors:  deduplicateSelectors([]Selector{}),
		trimPrefixes: []string{"prefix:"},
		keyValues:    make(map[string]any),
	}

	err := azappcfg.loadKeyValues(ctx, mockClient)
	assert.NoError(t, err)
	assert.Empty(t, azappcfg.keyValues)
}

func TestLoadKeyValues_InvalidJson(t *testing.T) {
	ctx := context.Background()
	mockClient := new(mockSettingsClient)
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
		kvSelectors: deduplicateSelectors([]Selector{}),
		keyValues:   make(map[string]any),
	}

	err := azappcfg.loadKeyValues(ctx, mockClient)
	assert.NoError(t, err)

	assert.Len(t, azappcfg.keyValues, 1)
	assert.Equal(t, &value1, azappcfg.keyValues["key1"])
	// The invalid JSON key should be skipped
	_, exists := azappcfg.keyValues["key2"]
	assert.False(t, exists)
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
				{KeyFilter: wildCard, LabelFilter: defaultLabel},
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
				{KeyFilter: "one*", LabelFilter: defaultLabel},
				{KeyFilter: "two*", LabelFilter: "dev"},
			},
		},
		{
			name: "duplicates after normalization",
			input: []Selector{
				{KeyFilter: "one*", LabelFilter: ""},
				{KeyFilter: "two*", LabelFilter: "dev"},
				{KeyFilter: "one*", LabelFilter: defaultLabel},
			},
			expectedOutput: []Selector{
				{KeyFilter: "two*", LabelFilter: "dev"},
				{KeyFilter: "one*", LabelFilter: defaultLabel},
			},
		},
		{
			name: "precedence - later duplicates override earlier ones",
			input: []Selector{
				{KeyFilter: "one*", LabelFilter: "prod"},
				{KeyFilter: "two*", LabelFilter: "dev"},
				{KeyFilter: "one*", LabelFilter: "prod"},
				{KeyFilter: "three*", LabelFilter: "test"},
			},
			expectedOutput: []Selector{
				{KeyFilter: "two*", LabelFilter: "dev"},
				{KeyFilter: "one*", LabelFilter: "prod"},
				{KeyFilter: "three*", LabelFilter: "test"},
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
		{
			name:           "multiple matching prefixes - only first match is trimmed",
			key:            "config:appSettings:theme",
			prefixesToTrim: []string{"config:", "appSettings:"},
			expected:       "appSettings:theme",
		},
		{
			name:           "nested prefixes - longer prefix should be prioritized",
			key:            "prefix:subprefix:value",
			prefixesToTrim: []string{"prefix:", "prefix:subprefix:"},
			expected:       "subprefix:value",
		},
		{
			name:           "nested prefixes in reverse order - first match is used",
			key:            "prefix:subprefix:value",
			prefixesToTrim: []string{"prefix:subprefix:", "prefix:"},
			expected:       "value",
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

// mockDelayedSecretResolver simulates a resolver with variable response times
type mockDelayedSecretResolver struct {
	mock.Mock
	delays map[string]time.Duration
	mu     sync.Mutex
	calls  []time.Time
}

func (m *mockDelayedSecretResolver) ResolveSecret(ctx context.Context, keyVaultReference url.URL) (string, error) {
	m.mu.Lock()
	m.calls = append(m.calls, time.Now())
	m.mu.Unlock()

	if delay, ok := m.delays[keyVaultReference.String()]; ok {
		time.Sleep(delay)
	}
	args := m.Called(ctx, keyVaultReference)
	return args.String(0), args.Error(1)
}

func TestLoadKeyValues_WithConcurrentKeyVaultReferences(t *testing.T) {
	ctx := context.Background()
	mockSettingsClient := new(mockSettingsClient)

	// Create a resolver with intentional delays to verify concurrent execution
	mockResolver := &mockDelayedSecretResolver{
		delays: map[string]time.Duration{
			"https://vault1.vault.azure.net/secrets/secret1": 50 * time.Millisecond,
			"https://vault1.vault.azure.net/secrets/secret2": 30 * time.Millisecond,
			"https://vault2.vault.azure.net/secrets/secret3": 40 * time.Millisecond,
		},
		calls: make([]time.Time, 0, 3),
	}

	// Create key vault references
	kvReference1 := `{"uri":"https://vault1.vault.azure.net/secrets/secret1"}`
	kvReference2 := `{"uri":"https://vault1.vault.azure.net/secrets/secret2"}`
	kvReference3 := `{"uri":"https://vault2.vault.azure.net/secrets/secret3"}`

	// Set up mock response with multiple key vault references
	mockResponse := &settingsResponse{
		settings: []azappconfig.Setting{
			{Key: toPtr("standard"), Value: toPtr("value1"), ContentType: toPtr("")},
			{Key: toPtr("secret1"), Value: toPtr(kvReference1), ContentType: toPtr(secretReferenceContentType)},
			{Key: toPtr("secret2"), Value: toPtr(kvReference2), ContentType: toPtr(secretReferenceContentType)},
			{Key: toPtr("secret3"), Value: toPtr(kvReference3), ContentType: toPtr(secretReferenceContentType)},
		},
		eTags: map[Selector][]*azcore.ETag{},
	}

	mockSettingsClient.On("getSettings", ctx).Return(mockResponse, nil)

	// Set up expectations for mock resolver
	secret1URL, _ := url.Parse("https://vault1.vault.azure.net/secrets/secret1")
	secret2URL, _ := url.Parse("https://vault1.vault.azure.net/secrets/secret2")
	secret3URL, _ := url.Parse("https://vault2.vault.azure.net/secrets/secret3")

	mockResolver.On("ResolveSecret", ctx, *secret1URL).Return("resolved-secret1", nil)
	mockResolver.On("ResolveSecret", ctx, *secret2URL).Return("resolved-secret2", nil)
	mockResolver.On("ResolveSecret", ctx, *secret3URL).Return("resolved-secret3", nil)

	// Create app configuration
	azappcfg := &AzureAppConfiguration{
		clientManager: &configurationClientManager{
			staticClient: &configurationClientWrapper{client: nil},
		},
		kvSelectors: deduplicateSelectors([]Selector{}),
		keyValues:   make(map[string]any),
		resolver: &keyVaultReferenceResolver{
			clients:        sync.Map{},
			secretResolver: mockResolver,
		},
	}

	// Record start time
	startTime := time.Now()

	// Load key values
	err := azappcfg.loadKeyValues(ctx, mockSettingsClient)

	// Record elapsed time
	elapsed := time.Since(startTime)

	// Verify results
	assert.NoError(t, err)
	assert.Equal(t, "value1", *azappcfg.keyValues["standard"].(*string))
	assert.Equal(t, "resolved-secret1", azappcfg.keyValues["secret1"])
	assert.Equal(t, "resolved-secret2", azappcfg.keyValues["secret2"])
	assert.Equal(t, "resolved-secret3", azappcfg.keyValues["secret3"])

	// Verify all resolver calls were made
	mockResolver.AssertNumberOfCalls(t, "ResolveSecret", 3)
	mockSettingsClient.AssertExpectations(t)

	// Verify concurrent execution by checking elapsed time
	// If executed sequentially, it would take at least 50+30+40=120ms
	// With concurrency, it should take closer to the longest delay (50ms) plus some overhead
	assert.Less(t, elapsed, 110*time.Millisecond, "Expected concurrent execution to complete faster than sequential execution")

	// Verify that calls started close to each other (within 10ms)
	// This indicates that the goroutines were started concurrently
	if len(mockResolver.calls) == 3 {
		firstCallTime := mockResolver.calls[0]
		for i := 1; i < len(mockResolver.calls); i++ {
			timeDiff := mockResolver.calls[i].Sub(firstCallTime)
			assert.Less(t, timeDiff, 10*time.Millisecond, "Expected resolver calls to start concurrently")
		}
	}
}
