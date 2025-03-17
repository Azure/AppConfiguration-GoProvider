// Copyright (c) Microsoft Corporation.
// Licensed under the MIT License.

package azureappconfiguration

import (
	"context"
	"sync"
	"testing"

	"github.com/Azure/azure-sdk-for-go/sdk/security/keyvault/azsecrets"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
)

// Mock implementation of the secretClient interface
type mockSecretClient struct {
	mock.Mock
}

func (m *mockSecretClient) GetSecret(ctx context.Context, name string, version string, options *azsecrets.GetSecretOptions) (azsecrets.GetSecretResponse, error) {
	args := m.Called(ctx, name, version, options)
	return args.Get(0).(azsecrets.GetSecretResponse), args.Error(1)
}

// Mock implementation of the SecretResolver interface
type mockSecretResolver struct {
	mock.Mock
}

func (m *mockSecretResolver) ResolveSecret(ctx context.Context, keyVaultReference string) (string, error) {
	args := m.Called(ctx, keyVaultReference)
	return args.String(0), args.Error(1)
}

func TestExtractKeyVaultURI(t *testing.T) {
	resolver := keyVaultReferenceResolver{
		clients: sync.Map{},
	}

	tests := []struct {
		name           string
		reference      string
		expectedURI    string
		expectedErrMsg string
	}{
		{
			name:        "Valid key vault reference",
			reference:   `{"uri":"https://myvault.vault.azure.net/secrets/mysecret"}`,
			expectedURI: "https://myvault.vault.azure.net/secrets/mysecret",
		},
		{
			name:        "Valid key vault reference with version",
			reference:   `{"uri":"https://myvault.vault.azure.net/secrets/mysecret/version1"}`,
			expectedURI: "https://myvault.vault.azure.net/secrets/mysecret/version1",
		},
		{
			name:           "Invalid JSON",
			reference:      `{"uri:invalid}`,
			expectedErrMsg: "invalid Key Vault reference format",
		},
		{
			name:           "Missing URI field",
			reference:      `{"id":"missing-uri"}`,
			expectedErrMsg: "invalid Key Vault reference format",
		},
		{
			name:           "Empty URI field",
			reference:      `{"uri":""}`,
			expectedErrMsg: "invalid Key Vault reference format",
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			uri, err := resolver.extractKeyVaultURI(test.reference)

			if test.expectedErrMsg != "" {
				assert.Error(t, err)
				assert.Contains(t, err.Error(), test.expectedErrMsg)
			} else {
				assert.NoError(t, err)
				assert.Equal(t, test.expectedURI, uri)
			}
		})
	}
}

func TestParseKeyVaultReference(t *testing.T) {
	tests := []struct {
		name           string
		reference      string
		expectedMeta   *secretMetadata
		expectedErrMsg string
	}{
		{
			name:      "Valid reference",
			reference: "https://myvault.vault.azure.net/secrets/mysecret",
			expectedMeta: &secretMetadata{
				host:    "myvault.vault.azure.net",
				name:    "mysecret",
				version: "",
			},
		},
		{
			name:      "Valid reference with version",
			reference: "https://myvault.vault.azure.net/secrets/mysecret/version1",
			expectedMeta: &secretMetadata{
				host:    "myvault.vault.azure.net",
				name:    "mysecret",
				version: "version1",
			},
		},
		{
			name:      "Case insensitive for host",
			reference: "https://MYVAULT.vault.azure.net/secrets/mysecret",
			expectedMeta: &secretMetadata{
				host:    "myvault.vault.azure.net",
				name:    "mysecret",
				version: "",
			},
		},
		{
			name:           "Invalid URL",
			reference:      "not-a-url",
			expectedErrMsg: "invalid Key Vault URL format: not-a-url",
		},
		{
			name:           "Missing secrets segment",
			reference:      "https://myvault.vault.azure.net/notsecrets/mysecret",
			expectedErrMsg: "invalid Key Vault URL format",
		},
		{
			name:           "Missing secret name",
			reference:      "https://myvault.vault.azure.net/secrets/",
			expectedErrMsg: "invalid Key Vault URL format",
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			meta, err := parse(test.reference)

			if test.expectedErrMsg != "" {
				assert.Error(t, err)
				assert.Contains(t, err.Error(), test.expectedErrMsg)
			} else {
				assert.NoError(t, err)
				assert.Equal(t, test.expectedMeta.host, meta.host)
				assert.Equal(t, test.expectedMeta.name, meta.name)
				assert.Equal(t, test.expectedMeta.version, meta.version)
			}
		})
	}
}

func TestResolveSecret_WithCustomResolver(t *testing.T) {
	ctx := context.Background()
	mockResolver := new(mockSecretResolver)

	resolver := keyVaultReferenceResolver{
		clients:  sync.Map{},
		resolver: mockResolver,
	}

	reference := `{"uri":"https://myvault.vault.azure.net/secrets/mysecret"}`
	mockResolver.On("ResolveSecret", ctx, mock.Anything).Return("resolved-secret", nil)

	secret, err := resolver.resolveSecret(ctx, reference)

	assert.NoError(t, err)
	assert.Equal(t, "resolved-secret", secret)
	mockResolver.AssertExpectations(t)
}

func TestResolveSecret_WithSecretClient(t *testing.T) {
	ctx := context.Background()
	mockClient := new(mockSecretClient)

	resolver := keyVaultReferenceResolver{
		clients: sync.Map{},
	}

	// Pre-populate the sync.Map with our mock client
	resolver.clients.Store("https://myvault.vault.azure.net", mockClient)

	secretValue := "mysecretvalue"
	mockResponse := azsecrets.GetSecretResponse{
		Secret: azsecrets.Secret{
			Value: &secretValue,
		},
	}

	mockClient.On("GetSecret", ctx, "mysecret", "", mock.Anything).Return(mockResponse, nil)

	secret, err := resolver.resolveSecret(ctx, `{"uri":"https://myvault.vault.azure.net/secrets/mysecret"}`)

	assert.NoError(t, err)
	assert.Equal(t, "mysecretvalue", secret)
	mockClient.AssertExpectations(t)
}
