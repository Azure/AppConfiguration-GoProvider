// Copyright (c) Microsoft Corporation.
// Licensed under the MIT License.

package azureappconfiguration

import (
	"context"
	"net/http"
	"net/url"
	"sync"
	"testing"
	"time"

	"encoding/json"

	"github.com/Azure/AppConfiguration-GoProvider/azureappconfiguration/internal/fm"
	"github.com/Azure/AppConfiguration-GoProvider/azureappconfiguration/internal/tracing"
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

func TestLoadFeatureFlags_Success(t *testing.T) {
	ctx := context.Background()
	mockClient := new(mockSettingsClient)

	value1 := `{
		"id": "Beta",
		"description": "",
		"enabled": false,
		"conditions": {
			"client_filters": []
		}
	}`

	value2 := `{
		"id": "Alpha",
		"description": "Test feature",
		"enabled": true,
		"conditions": {
			"client_filters": [
				{
					"name": "TestGroup",
					"parameters": {"Users": ["user1@example.com", "user2@example.com"]}
				}
			]
		}
	}`

	mockResponse := &settingsResponse{
		settings: []azappconfig.Setting{
			{Key: toPtr(".appconfig.featureflag/Beta"), Value: &value1, ContentType: toPtr(featureFlagContentType)},
			{Key: toPtr(".appconfig.featureflag/Alpha"), Value: &value2, ContentType: toPtr(featureFlagContentType)},
		},
		pageETags: map[Selector][]*azcore.ETag{},
	}

	mockClient.On("getSettings", ctx).Return(mockResponse, nil)

	azappcfg := &AzureAppConfiguration{
		clientManager: &configurationClientManager{
			staticClient: &configurationClientWrapper{client: &azappconfig.Client{}},
		},
		ffSelectors:  getFeatureFlagSelectors([]Selector{}),
		featureFlags: make(map[string]any),
	}

	err := azappcfg.loadFeatureFlags(ctx, mockClient)
	assert.NoError(t, err)
	// Verify feature flag structure is created correctly
	assert.Contains(t, azappcfg.featureFlags, featureManagementSectionKey)
	featureManagement, ok := azappcfg.featureFlags[featureManagementSectionKey].(map[string]any)
	assert.True(t, ok, "feature_management should be a map")

	// Verify feature_flags array exists
	assert.Contains(t, featureManagement, featureFlagSectionKey)
	featureFlags, ok := featureManagement[featureFlagSectionKey].([]any)
	assert.True(t, ok, "feature_flags should be an array")

	// Verify we have 2 feature flags
	assert.Len(t, featureFlags, 2)

	// Verify the feature flags are properly unmarshaled
	foundBeta := false
	foundAlpha := false
	for _, flag := range featureFlags {
		flagMap, ok := flag.(map[string]any)
		assert.True(t, ok, "feature flag should be a map")

		if id, ok := flagMap["id"].(string); ok {
			switch id {
			case "Beta":
				foundBeta = true
				assert.Equal(t, false, flagMap["enabled"])
			case "Alpha":
				foundAlpha = true
				assert.Equal(t, true, flagMap["enabled"])
				assert.Equal(t, "Test feature", flagMap["description"])

				// Verify conditions structure
				conditions, ok := flagMap["conditions"].(map[string]any)
				assert.True(t, ok, "conditions should be a map")

				clientFilters, ok := conditions["client_filters"].([]any)
				assert.True(t, ok, "client_filters should be an array")
				assert.Len(t, clientFilters, 1)
			}
		}
	}

	assert.True(t, foundBeta, "Should have found Beta feature flag")
	assert.True(t, foundAlpha, "Should have found Alpha feature flag")
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

func TestUnmarshal_BasicTypes(t *testing.T) {
	// Define a simple struct with basic types
	type Config struct {
		String  string
		Int     int
		Bool    bool
		Float   float64
		Slice   []string
		Timeout time.Duration
	}

	// Setup test data
	azappcfg := &AzureAppConfiguration{
		keyValues: map[string]interface{}{
			"String":  "hello world",
			"Int":     "42", // Test string to int conversion
			"Bool":    "true",
			"Float":   3.14,
			"Slice":   "item1,item2,item3",
			"Timeout": "5s",
		},
	}

	// Unmarshal into the struct
	var config Config
	err := azappcfg.Unmarshal(&config, nil)

	// Verify results
	assert.NoError(t, err)
	assert.Equal(t, "hello world", config.String)
	assert.Equal(t, 42, config.Int)
	assert.Equal(t, true, config.Bool)
	assert.Equal(t, 3.14, config.Float)
	assert.Equal(t, []string{"item1", "item2", "item3"}, config.Slice)
	assert.Equal(t, 5*time.Second, config.Timeout)
}

func TestUnmarshal_NestedStructs(t *testing.T) {
	// Define nested structs
	type Database struct {
		Host     string
		Port     int
		Username string
		Password string
		SSL      bool
	}

	type Cache struct {
		TTL       time.Duration
		MaxSize   int
		Endpoints []string
	}

	type Config struct {
		AppName  string
		Version  string
		Database Database
		Cache    Cache
		Debug    bool
	}

	// Setup test data
	azappcfg := &AzureAppConfiguration{
		keyValues: map[string]interface{}{
			"AppName":           "MyApp",
			"Version":           "1.0.0",
			"Database.Host":     "localhost",
			"Database.Port":     "5432",
			"Database.Username": "admin",
			"Database.Password": "secret",
			"Database.SSL":      true,
			"Cache.TTL":         "30s",
			"Cache.MaxSize":     1024,
			"Cache.Endpoints":   "endpoint1.com,endpoint2.com",
			"Debug":             "true",
		},
	}

	// Unmarshal into the struct
	var config Config
	err := azappcfg.Unmarshal(&config, nil)

	// Verify results
	assert.NoError(t, err)
	assert.Equal(t, "MyApp", config.AppName)
	assert.Equal(t, "1.0.0", config.Version)

	// Database checks
	assert.Equal(t, "localhost", config.Database.Host)
	assert.Equal(t, 5432, config.Database.Port)
	assert.Equal(t, "admin", config.Database.Username)
	assert.Equal(t, "secret", config.Database.Password)
	assert.Equal(t, true, config.Database.SSL)

	// Cache checks
	assert.Equal(t, 30*time.Second, config.Cache.TTL)
	assert.Equal(t, 1024, config.Cache.MaxSize)
	assert.Equal(t, []string{"endpoint1.com", "endpoint2.com"}, config.Cache.Endpoints)

	// Debug check
	assert.Equal(t, true, config.Debug)
}

func TestUnmarshal_CustomTags(t *testing.T) {
	// Define a struct with custom field tags
	type Config struct {
		AppName      string `json:"application_name"`
		MaxConnCount int    `json:"connection_limit"`
		Endpoints    struct {
			Primary   string `json:"main_endpoint"`
			Secondary string `json:"backup_endpoint"`
		} `json:"endpoints"`
		FeatureEnabled bool     `json:"is_feature_enabled"`
		AllowedIPs     []string `json:"allowed_ip_addresses"`
	}

	// Setup test data
	azappcfg := &AzureAppConfiguration{
		keyValues: map[string]interface{}{
			"application_name":          "CustomTagApp",
			"connection_limit":          100,
			"endpoints.main_endpoint":   "https://primary.example.com",
			"endpoints.backup_endpoint": "https://secondary.example.com",
			"is_feature_enabled":        "true",
			"allowed_ip_addresses":      "192.168.1.1,10.0.0.1,172.16.0.1",
		},
	}

	// Unmarshal into the struct
	var config Config
	err := azappcfg.Unmarshal(&config, nil)

	// Verify results
	assert.NoError(t, err)
	assert.Equal(t, "CustomTagApp", config.AppName)
	assert.Equal(t, 100, config.MaxConnCount)
	assert.Equal(t, "https://primary.example.com", config.Endpoints.Primary)
	assert.Equal(t, "https://secondary.example.com", config.Endpoints.Secondary)
	assert.Equal(t, true, config.FeatureEnabled)
	assert.Equal(t, []string{"192.168.1.1", "10.0.0.1", "172.16.0.1"}, config.AllowedIPs)
}

func TestUnmarshal_EmptyValues(t *testing.T) {
	// Define a struct with default values
	type Config struct {
		String      string
		StringPtr   *string
		Int         int
		IntPtr      *int
		Bool        bool
		BoolPtr     *bool
		StringSlice []string
	}

	// Setup test data with some empty values
	defaultString := "default"
	defaultInt := 42
	defaultBool := true

	config := Config{
		String:      defaultString,
		StringPtr:   &defaultString,
		Int:         defaultInt,
		IntPtr:      &defaultInt,
		Bool:        defaultBool,
		BoolPtr:     &defaultBool,
		StringSlice: []string{"default1", "default2"},
	}

	azappcfg := &AzureAppConfiguration{
		keyValues: map[string]interface{}{
			// Intentionally empty map to test empty values
		},
	}

	// Unmarshal into the struct with existing default values
	err := azappcfg.Unmarshal(&config, nil)

	// Verify results - default values should remain unchanged
	assert.NoError(t, err)
	assert.Equal(t, defaultString, config.String)
	assert.Equal(t, &defaultString, config.StringPtr)
	assert.Equal(t, defaultInt, config.Int)
	assert.Equal(t, &defaultInt, config.IntPtr)
	assert.Equal(t, defaultBool, config.Bool)
	assert.Equal(t, &defaultBool, config.BoolPtr)
	assert.Equal(t, []string{"default1", "default2"}, config.StringSlice)
}

func TestUnmarshal_EmptyValues_2(t *testing.T) {
	// Define a struct with default values
	type Config struct {
		String      string
		StringPtr   *string
		Int         int
		IntPtr      *int
		Bool        bool
		BoolPtr     *bool
		StringSlice []string
	}

	// Setup test data with some empty values
	defaultString := "default"
	defaultInt := 42
	defaultBool := true

	// Partially initialize config
	config := Config{
		StringPtr: &defaultString,
		IntPtr:    &defaultInt,
		BoolPtr:   &defaultBool,
	}

	azappcfg := &AzureAppConfiguration{
		keyValues: map[string]interface{}{
			// Intentionally empty map to test empty values
		},
	}

	// Unmarshal into the struct with existing default values
	err := azappcfg.Unmarshal(&config, nil)

	// Verify results - default values should remain unchanged
	assert.NoError(t, err)
	assert.Equal(t, "", config.String)
	assert.Equal(t, &defaultString, config.StringPtr)
	assert.Equal(t, 0, config.Int)
	assert.Equal(t, &defaultInt, config.IntPtr)
	assert.Equal(t, false, config.Bool)
	assert.Equal(t, &defaultBool, config.BoolPtr)
	assert.Nil(t, config.StringSlice)
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

// mockTracingClient is a mock client that captures the HTTP header containing the correlation context
type mockTracingClient struct {
	mock.Mock
	capturedHeader http.Header
}

func (m *mockTracingClient) getSettings(ctx context.Context) (*settingsResponse, error) {
	// Extract header from context
	if header, ok := ctx.Value(tracing.CorrelationContextHeader).(http.Header); ok {
		m.capturedHeader = header
	}

	args := m.Called(ctx)
	return args.Get(0).(*settingsResponse), args.Error(1)
}

func TestLoadKeyValues_WithAIContentTypes(t *testing.T) {
	ctx := context.Background()
	mockClient := new(mockSettingsClient)

	// Create settings with different content types
	value1 := "regular value"
	value2 := `{"ai": "configuration"}`
	value3 := `{"ai": "chat completion"}`
	mockResponse := &settingsResponse{
		settings: []azappconfig.Setting{
			{Key: toPtr("key1"), Value: &value1, ContentType: toPtr("text/plain")},
			{Key: toPtr("key2"), Value: &value2, ContentType: toPtr("application/json; profile=\"https://azconfig.io/mime-profiles/ai\"")},
			{Key: toPtr("key3"), Value: &value3, ContentType: toPtr("application/json; profile=\"https://azconfig.io/mime-profiles/ai/chat-completion\"")},
		},
	}
	mockClient.On("getSettings", ctx).Return(mockResponse, nil)

	// Create the app configuration with tracing enabled
	azappcfg := &AzureAppConfiguration{
		clientManager: &configurationClientManager{
			staticClient: &configurationClientWrapper{client: &azappconfig.Client{}},
		},
		kvSelectors: deduplicateSelectors([]Selector{}),
		keyValues:   make(map[string]any),
		tracingOptions: tracing.Options{
			Enabled: true,
		},
	}

	// Load the key values
	err := azappcfg.loadKeyValues(ctx, mockClient)
	assert.NoError(t, err)

	// Verify the tracing options were updated correctly
	assert.True(t, azappcfg.tracingOptions.UseAIConfiguration, "UseAIConfiguration flag should be set to true")
	assert.True(t, azappcfg.tracingOptions.UseAIChatCompletionConfiguration, "UseAIChatCompletionConfiguration flag should be set to true")

	// Verify the data was loaded correctly
	assert.Equal(t, &value1, azappcfg.keyValues["key1"])
	assert.Equal(t, map[string]interface{}{"ai": "configuration"}, azappcfg.keyValues["key2"])
	assert.Equal(t, map[string]interface{}{"ai": "chat completion"}, azappcfg.keyValues["key3"])
}

func TestCorrelationContextHeader(t *testing.T) {
	ctx := context.Background()
	mockClient := new(mockTracingClient)

	// Create settings with different content types
	value1 := "regular value"
	value2 := `{"ai": "configuration"}`
	value3 := `{"ai": "chat completion"}`
	mockResponse := &settingsResponse{
		settings: []azappconfig.Setting{
			{Key: toPtr("key1"), Value: &value1, ContentType: toPtr("text/plain")},
			{Key: toPtr("key2"), Value: &value2, ContentType: toPtr("application/json; profile=\"https://azconfig.io/mime-profiles/ai\"")},
			{Key: toPtr("key3"), Value: &value3, ContentType: toPtr("application/json; profile=\"https://azconfig.io/mime-profiles/ai/chat-completion\"")},
		},
	}
	mockClient.On("getSettings", ctx).Return(mockResponse, nil)

	// Create app configuration with key vault configured
	tracingOptions := tracing.Options{
		Enabled:            true,
		KeyVaultConfigured: true,
		Host:               tracing.HostTypeAzureWebApp,
	}

	azappcfg := &AzureAppConfiguration{
		clientManager: &configurationClientManager{
			staticClient: &configurationClientWrapper{client: &azappconfig.Client{}},
		},
		kvSelectors:    deduplicateSelectors([]Selector{}),
		keyValues:      make(map[string]any),
		tracingOptions: tracingOptions,
	}

	// Load the key values
	err := azappcfg.loadKeyValues(ctx, mockClient)
	assert.NoError(t, err)

	// Verify the header contains all expected values
	header := tracing.CreateCorrelationContextHeader(ctx, azappcfg.tracingOptions)
	correlationCtx := header.Get(tracing.CorrelationContextHeader)

	assert.Contains(t, correlationCtx, tracing.HostTypeKey+"="+string(tracing.HostTypeAzureWebApp))
	assert.Contains(t, correlationCtx, tracing.KeyVaultConfiguredTag)

	// Verify AI features are detected and included in the header
	assert.True(t, azappcfg.tracingOptions.UseAIConfiguration)
	assert.True(t, azappcfg.tracingOptions.UseAIChatCompletionConfiguration)
	assert.Contains(t, correlationCtx, tracing.FeaturesKey+"="+
		tracing.AIConfigurationTag+tracing.DelimiterPlus+tracing.AIChatCompletionConfigurationTag)
}

func TestUnmarshal_FeatureManagement(t *testing.T) {
	// Setup a feature flag configuration
	azappcfg := &AzureAppConfiguration{
		ffEnabled: true,
		featureFlags: map[string]any{
			"feature_management": map[string]any{
				"feature_flags": []any{
					map[string]any{
						"id":          "BasicFlag",
						"description": "A simple feature flag",
						"enabled":     true,
					},
					map[string]any{
						"id":          "FlagWithConditions",
						"description": "A flag with conditions",
						"enabled":     false,
						"conditions": map[string]any{
							"client_filters": []any{
								map[string]any{
									"name": "Microsoft.TimeWindow",
									"parameters": map[string]any{
										"Start": "2023-01-01T00:00:00Z",
										"End":   "2023-12-31T23:59:59Z",
									},
								},
							},
						},
					},
					map[string]any{
						"id":          "FlagWithVariants",
						"description": "A flag with variants",
						"enabled":     true,
						"variants": []any{
							map[string]any{
								"name":                "variantA",
								"configuration_value": "value-a",
							},
							map[string]any{
								"name":                "variantB",
								"configuration_value": "value-b",
								"status_override":     "Disabled",
							},
						},
						"allocation": map[string]any{
							"default_when_enabled": "variantA",
							"percentile": []any{
								map[string]any{
									"variant": "variantB",
									"from":    0.0,
									"to":      50.0,
								},
								map[string]any{
									"variant": "variantA",
									"from":    50.0,
									"to":      100.0,
								},
							},
						},
					},
				},
			},
		},
		keyValues: make(map[string]any),
	}

	// Create a structure to unmarshal into
	type ConfigWithFeatureManagement struct {
		FeatureManagement fm.FeatureManagement `json:"feature_management"`
	}

	// Unmarshal into the struct
	var config ConfigWithFeatureManagement
	err := azappcfg.Unmarshal(&config, nil)

	// Verify results
	assert.NoError(t, err)
	assert.NotNil(t, config.FeatureManagement)
	assert.Len(t, config.FeatureManagement.FeatureFlags, 3)

	// Verify BasicFlag
	basicFlag := findFeatureFlag(config.FeatureManagement.FeatureFlags, "BasicFlag")
	assert.NotNil(t, basicFlag)
	assert.Equal(t, "A simple feature flag", basicFlag.Description)
	assert.True(t, basicFlag.Enabled)
	assert.Nil(t, basicFlag.Conditions)
	assert.Nil(t, basicFlag.Variants)

	// Verify FlagWithConditions
	conditionsFlag := findFeatureFlag(config.FeatureManagement.FeatureFlags, "FlagWithConditions")
	assert.NotNil(t, conditionsFlag)
	assert.Equal(t, "A flag with conditions", conditionsFlag.Description)
	assert.False(t, conditionsFlag.Enabled)
	assert.NotNil(t, conditionsFlag.Conditions)
	assert.Len(t, conditionsFlag.Conditions.ClientFilters, 1)
	assert.Equal(t, "Microsoft.TimeWindow", conditionsFlag.Conditions.ClientFilters[0].Name)
	assert.Contains(t, conditionsFlag.Conditions.ClientFilters[0].Parameters, "Start")
	assert.Contains(t, conditionsFlag.Conditions.ClientFilters[0].Parameters, "End")

	// Verify FlagWithVariants
	variantsFlag := findFeatureFlag(config.FeatureManagement.FeatureFlags, "FlagWithVariants")
	assert.NotNil(t, variantsFlag)
	assert.Equal(t, "A flag with variants", variantsFlag.Description)
	assert.True(t, variantsFlag.Enabled)
	assert.Nil(t, variantsFlag.Conditions)
	assert.Len(t, variantsFlag.Variants, 2)
	assert.Equal(t, "variantA", variantsFlag.Variants[0].Name)
	assert.Equal(t, "value-a", variantsFlag.Variants[0].ConfigurationValue)
	assert.Equal(t, "variantB", variantsFlag.Variants[1].Name)
	assert.Equal(t, "value-b", variantsFlag.Variants[1].ConfigurationValue)
	assert.Equal(t, fm.StatusOverrideDisabled, variantsFlag.Variants[1].StatusOverride)

	// Verify allocation
	assert.NotNil(t, variantsFlag.Allocation)
	assert.Equal(t, "variantA", variantsFlag.Allocation.DefaultWhenEnabled)
	assert.Len(t, variantsFlag.Allocation.Percentile, 2)
	assert.Equal(t, "variantB", variantsFlag.Allocation.Percentile[0].Variant)
	assert.Equal(t, 0.0, variantsFlag.Allocation.Percentile[0].From)
	assert.Equal(t, 50.0, variantsFlag.Allocation.Percentile[0].To)
	assert.Equal(t, "variantA", variantsFlag.Allocation.Percentile[1].Variant)
	assert.Equal(t, 50.0, variantsFlag.Allocation.Percentile[1].From)
	assert.Equal(t, 100.0, variantsFlag.Allocation.Percentile[1].To)
}

// Helper function to find a feature flag by ID
func findFeatureFlag(flags []fm.FeatureFlag, id string) *fm.FeatureFlag {
	for i := range flags {
		if flags[i].ID == id {
			return &flags[i]
		}
	}
	return nil
}

func TestUnmarshal_FeatureFlagsWithKeyValues(t *testing.T) {
	// Setup an AzureAppConfiguration with both feature flags and key-values
	azappcfg := &AzureAppConfiguration{
		ffEnabled: true,
		featureFlags: map[string]any{
			"feature_management": map[string]any{
				"feature_flags": []any{
					map[string]any{
						"id":      "BetaFeature",
						"enabled": true,
					},
					map[string]any{
						"id":      "AlphaFeature",
						"enabled": false,
					},
				},
			},
		},
		keyValues: map[string]any{
			"AppName":       "TestApp",
			"Version":       "1.0.0",
			"Database.Host": "localhost",
			"Database.Port": 5432,
			"EnableLogging": true,
		},
	}

	// Create a structure that has both feature flags and regular config
	type Database struct {
		Host string
		Port int
	}

	type Config struct {
		AppName           string
		Version           string
		Database          Database
		EnableLogging     bool
		FeatureManagement fm.FeatureManagement `json:"feature_management"`
	}

	// Unmarshal into the combined struct
	var config Config
	err := azappcfg.Unmarshal(&config, nil)

	// Verify results
	assert.NoError(t, err)

	// Check regular config values
	assert.Equal(t, "TestApp", config.AppName)
	assert.Equal(t, "1.0.0", config.Version)
	assert.Equal(t, "localhost", config.Database.Host)
	assert.Equal(t, 5432, config.Database.Port)
	assert.True(t, config.EnableLogging)

	// Check feature flags
	assert.Len(t, config.FeatureManagement.FeatureFlags, 2)

	betaFlag := findFeatureFlag(config.FeatureManagement.FeatureFlags, "BetaFeature")
	assert.NotNil(t, betaFlag)
	assert.True(t, betaFlag.Enabled)

	alphaFlag := findFeatureFlag(config.FeatureManagement.FeatureFlags, "AlphaFeature")
	assert.NotNil(t, alphaFlag)
	assert.False(t, alphaFlag.Enabled)
}

// Test for complex conditional feature flag scenario
func TestUnmarshal_ComplexFeatureFlag(t *testing.T) {
	// Set up a complex feature flag with multiple filter types
	azappcfg := &AzureAppConfiguration{
		ffEnabled: true,
		featureFlags: map[string]any{
			"feature_management": map[string]any{
				"feature_flags": []any{
					map[string]any{
						"id":          "ComplexFlag",
						"description": "Complex feature flag with multiple conditions",
						"enabled":     true,
						"conditions": map[string]any{
							"requirement_type": "All",
							"client_filters": []any{
								map[string]any{
									"name": "Microsoft.Targeting",
									"parameters": map[string]any{
										"Audience": map[string]any{
											"Users": []any{
												"user1@example.com",
												"user2@example.com",
											},
											"Groups": []any{
												"Developers",
												"Testers",
											},
											"DefaultRolloutPercentage": 25,
										},
									},
								},
								map[string]any{
									"name": "Microsoft.TimeWindow",
									"parameters": map[string]any{
										"Start": "2023-01-01T00:00:00Z",
										"End":   "2023-12-31T23:59:59Z",
									},
								},
							},
						},
						"telemetry": map[string]any{
							"enabled": true,
							"metadata": map[string]any{
								"source":  "unit-test",
								"version": "1.0",
							},
						},
					},
				},
			},
		},
		keyValues: make(map[string]any),
	}

	// Create the struct to unmarshal into
	var config struct {
		FeatureManagement fm.FeatureManagement `json:"feature_management"`
	}

	// Unmarshal the data
	err := azappcfg.Unmarshal(&config, nil)

	// Verify results
	assert.NoError(t, err)
	assert.Len(t, config.FeatureManagement.FeatureFlags, 1)

	complexFlag := config.FeatureManagement.FeatureFlags[0]
	assert.Equal(t, "ComplexFlag", complexFlag.ID)
	assert.Equal(t, "Complex feature flag with multiple conditions", complexFlag.Description)
	assert.True(t, complexFlag.Enabled)

	// Check conditions
	assert.NotNil(t, complexFlag.Conditions)
	assert.Equal(t, fm.RequirementTypeAll, complexFlag.Conditions.RequirementType)
	assert.Len(t, complexFlag.Conditions.ClientFilters, 2)

	// Check targeting filter
	targetingFilter := findClientFilter(complexFlag.Conditions.ClientFilters, "Microsoft.Targeting")
	assert.NotNil(t, targetingFilter)

	audienceParam, ok := targetingFilter.Parameters["Audience"].(map[string]any)
	assert.True(t, ok, "Audience parameter should be a map")

	users, ok := audienceParam["Users"].([]any)
	assert.True(t, ok, "Users should be an array")
	assert.Len(t, users, 2)
	assert.Contains(t, users, "user1@example.com")
	assert.Contains(t, users, "user2@example.com")

	// Check time window filter
	timeFilter := findClientFilter(complexFlag.Conditions.ClientFilters, "Microsoft.TimeWindow")
	assert.NotNil(t, timeFilter)
	assert.Equal(t, "2023-01-01T00:00:00Z", timeFilter.Parameters["Start"])

	// Check telemetry
	assert.NotNil(t, complexFlag.Telemetry)
	assert.True(t, complexFlag.Telemetry.Enabled)
	assert.Equal(t, "unit-test", complexFlag.Telemetry.Metadata["source"])
	assert.Equal(t, "1.0", complexFlag.Telemetry.Metadata["version"])
}

// Helper function to find a client filter by name
func findClientFilter(filters []fm.ClientFilter, name string) *fm.ClientFilter {
	for i := range filters {
		if filters[i].Name == name {
			return &filters[i]
		}
	}
	return nil
}

func TestGetBytes_SimpleKeyValues(t *testing.T) {
	// Setup a provider with only key values
	azappcfg := &AzureAppConfiguration{
		keyValues: map[string]any{
			"AppName":       "TestApp",
			"Version":       "1.0.0",
			"Database.Host": "localhost",
			"Database.Port": 5432,
			"EnableLogging": true,
		},
		featureFlags: make(map[string]any),
	}

	// Get the JSON bytes
	bytes, err := azappcfg.GetBytes(nil)
	assert.NoError(t, err)
	assert.NotEmpty(t, bytes)

	// Parse the JSON back into a map to verify content
	var result map[string]any
	err = json.Unmarshal(bytes, &result)
	assert.NoError(t, err)

	// Verify the expected values are present
	assert.Equal(t, "TestApp", result["AppName"])
	assert.Equal(t, "1.0.0", result["Version"])

	// Verify nested structure
	db, ok := result["Database"].(map[string]any)
	assert.True(t, ok, "Database should be a nested map")
	assert.Equal(t, "localhost", db["Host"])
	assert.Equal(t, float64(5432), db["Port"]) // JSON numbers are parsed as float64

	// Verify bool value
	assert.Equal(t, true, result["EnableLogging"])

	// Verify feature flags section does not exist
	_, hasFM := result["feature_management"]
	assert.False(t, hasFM, "feature_management section should not exist")
}

func TestGetBytes_FeatureFlags(t *testing.T) {
	// Setup a provider with only feature flags
	azappcfg := &AzureAppConfiguration{
		ffEnabled: true,
		featureFlags: map[string]any{
			"feature_management": map[string]any{
				"feature_flags": []any{
					map[string]any{
						"id":      "Beta",
						"enabled": true,
					},
					map[string]any{
						"id":      "Alpha",
						"enabled": false,
					},
				},
			},
		},
		keyValues: make(map[string]any),
	}

	// Get the JSON bytes
	bytes, err := azappcfg.GetBytes(nil)
	assert.NoError(t, err)
	assert.NotEmpty(t, bytes)

	// Parse the JSON back into a map to verify content
	var result map[string]any
	err = json.Unmarshal(bytes, &result)
	assert.NoError(t, err)

	// Verify feature_management section exists
	fm, hasFM := result["feature_management"].(map[string]any)
	assert.True(t, hasFM, "feature_management section should exist")

	// Verify feature_flags array exists
	flags, hasFlags := fm["feature_flags"].([]any)
	assert.True(t, hasFlags, "feature_flags array should exist")
	assert.Len(t, flags, 2, "There should be 2 feature flags")

	// Verify the two feature flags
	foundBeta, foundAlpha := false, false
	for _, flag := range flags {
		if flagMap, ok := flag.(map[string]any); ok {
			if id, ok := flagMap["id"].(string); ok {
				switch id {
				case "Beta":
					foundBeta = true
					assert.Equal(t, true, flagMap["enabled"])
				case "Alpha":
					foundAlpha = true
					assert.Equal(t, false, flagMap["enabled"])
				}
			}
		}
	}
	assert.True(t, foundBeta, "Beta feature flag should exist")
	assert.True(t, foundAlpha, "Alpha feature flag should exist")
}

func TestGetBytes_CombinedKeyValuesAndFeatureFlags(t *testing.T) {
	// Setup a provider with both key values and feature flags
	azappcfg := &AzureAppConfiguration{
		ffEnabled: true,
		keyValues: map[string]any{
			"AppName":       "TestApp",
			"Version":       "1.0.0",
			"Database.Host": "localhost",
		},
		featureFlags: map[string]any{
			"feature_management": map[string]any{
				"feature_flags": []any{
					map[string]any{
						"id":      "Beta",
						"enabled": true,
					},
				},
			},
		},
	}

	// Get the JSON bytes
	bytes, err := azappcfg.GetBytes(nil)
	assert.NoError(t, err)
	assert.NotEmpty(t, bytes)

	// Parse the JSON back into a map to verify content
	var result map[string]any
	err = json.Unmarshal(bytes, &result)
	assert.NoError(t, err)

	// Verify regular key values
	assert.Equal(t, "TestApp", result["AppName"])
	assert.Equal(t, "1.0.0", result["Version"])
	db, ok := result["Database"].(map[string]any)
	assert.True(t, ok)
	assert.Equal(t, "localhost", db["Host"])

	// Verify feature_management section
	fm, hasFM := result["feature_management"].(map[string]any)
	assert.True(t, hasFM, "feature_management section should exist")

	flags, hasFlags := fm["feature_flags"].([]any)
	assert.True(t, hasFlags, "feature_flags array should exist")
	assert.Len(t, flags, 1, "There should be 1 feature flag")

	flag, ok := flags[0].(map[string]any)
	assert.True(t, ok)
	assert.Equal(t, "Beta", flag["id"])
	assert.Equal(t, true, flag["enabled"])
}

func TestGetBytes_CustomSeparator(t *testing.T) {
	// Setup a provider with hierarchical keys using a custom separator
	azappcfg := &AzureAppConfiguration{
		keyValues: map[string]any{
			"App:Name":      "TestApp",
			"App:Version":   "1.0.0",
			"Database:Host": "localhost",
			"Database:Port": 5432,
		},
		featureFlags: make(map[string]any),
	}

	// Get the JSON bytes with custom separator
	bytes, err := azappcfg.GetBytes(&ConstructionOptions{Separator: ":"})
	assert.NoError(t, err)
	assert.NotEmpty(t, bytes)

	// Parse the JSON back into a map to verify content
	var result map[string]any
	err = json.Unmarshal(bytes, &result)
	assert.NoError(t, err)

	// Verify hierarchical structure with custom separator
	app, hasApp := result["App"].(map[string]any)
	assert.True(t, hasApp, "App should be a nested map")
	assert.Equal(t, "TestApp", app["Name"])
	assert.Equal(t, "1.0.0", app["Version"])

	db, hasDb := result["Database"].(map[string]any)
	assert.True(t, hasDb, "Database should be a nested map")
	assert.Equal(t, "localhost", db["Host"])
	assert.Equal(t, float64(5432), db["Port"])
}

func TestGetBytes_InvalidSeparator(t *testing.T) {
	azappcfg := &AzureAppConfiguration{
		keyValues: map[string]any{
			"App|Name": "TestApp",
		},
	}

	// Invalid separator should cause an error
	_, err := azappcfg.GetBytes(&ConstructionOptions{Separator: "|"})
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "invalid separator")
}
