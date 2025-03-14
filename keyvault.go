// Copyright (c) Microsoft Corporation.
// Licensed under the MIT License.

package azureappconfiguration

import (
	"context"
	"encoding/json"
	"fmt"
	"net/url"
	"strings"
	"sync"

	"github.com/Azure/azure-sdk-for-go/sdk/azcore"
	"github.com/Azure/azure-sdk-for-go/sdk/security/keyvault/azsecrets"
)

// keyVaultReferenceResolver resolves Key Vault references to their actual secret values
type keyVaultReferenceResolver struct {
	clients    sync.Map // map[string]secretClient
	resolver   SecretResolver
	credential azcore.TokenCredential
}

// secretMetadata contains parsed information about a Key Vault secret reference
type secretMetadata struct {
	host    string
	name    string
	version string
}

// keyVaultReference represents the JSON structure of a Key Vault reference
type keyVaultReference struct {
	URI string `json:"uri"`
}

type secretClient interface {
	GetSecret(ctx context.Context, name string, version string, options *azsecrets.GetSecretOptions) (azsecrets.GetSecretResponse, error)
}

// resolveSecret resolves a Key Vault reference to its actual secret value
func (r *keyVaultReferenceResolver) resolveSecret(ctx context.Context, keyVaultReference string) (string, error) {
	if r.resolver != nil {
		return r.resolver.ResolveSecret(ctx, keyVaultReference)
	}

	uri, err := r.extractKeyVaultURI(keyVaultReference)
	if err != nil {
		return "", fmt.Errorf("failed to parse Key Vault reference: %w", err)
	}

	// Parse the URI to get metadata (host, secret name, version)
	secretMeta, err := parse(uri)
	if err != nil {
		return "", fmt.Errorf("invalid Key Vault reference: %w", err)
	}

	vaultURL := fmt.Sprintf("https://%s", secretMeta.host)
	client, err := r.getSecretClient(vaultURL)
	if err != nil {
		return "", fmt.Errorf("failed to get Key Vault client: %w", err)
	}

	response, err := client.GetSecret(ctx, secretMeta.name, secretMeta.version, nil)
	if err != nil {
		return "", fmt.Errorf("failed to retrieve secret '%s' from Key Vault: %w", secretMeta.name, err)
	}

	if response.Value == nil {
		return "", nil
	}

	return *response.Value, nil
}

// extractKeyVaultURI tries to parse a Key Vault reference in various formats
func (r *keyVaultReferenceResolver) extractKeyVaultURI(reference string) (string, error) {
	// Valid Key Vault Reference setting value to parse
	// {
	// 	"uri":"https://{keyVaultName}.vaule.azure.net/secrets/{secretName}/{secretVersion}"
	// }
	var kvRef keyVaultReference
	if err := json.Unmarshal([]byte(reference), &kvRef); err == nil && kvRef.URI != "" {
		return kvRef.URI, nil
	}

	return "", fmt.Errorf("invalid Key Vault reference format: %s", reference)
}

// getSecretClient gets or creates a client for the specified vault URL
func (r *keyVaultReferenceResolver) getSecretClient(vaultURL string) (secretClient, error) {
	if client, ok := r.clients.Load(vaultURL); ok {
		return client.(secretClient), nil
	}

	if r.credential == nil {
		return nil, fmt.Errorf("no Key Vault credential or SecretResolver configured")
	}

	client, err := azsecrets.NewClient(vaultURL, r.credential, nil)
	if err != nil {
		return nil, fmt.Errorf("failed to create Key Vault client: %w", err)
	}

	// Store the client - if concurrent call already stored a client, use the existing one
	storedClient, loaded := r.clients.LoadOrStore(vaultURL, client)
	if loaded {
		// Another goroutine already created and stored a client
		return storedClient.(secretClient), nil
	}

	return client, nil
}

// parse extracts metadata from a Key Vault secret reference URI
func parse(reference string) (*secretMetadata, error) {
	secretURL, err := url.Parse(reference)
	if err != nil {
		return nil, fmt.Errorf("invalid URL format: %w", err)
	}

	trimmedPath := strings.TrimPrefix(secretURL.Path, "/")
	segments := strings.Split(trimmedPath, "/")

	if len(segments) < 2 || strings.ToLower(segments[0]) != "secrets" || segments[1] == "" {
		return nil, fmt.Errorf("invalid Key Vault URL format: %s", reference)
	}

	secretName := segments[1]
	var secretVersion string
	if len(segments) > 2 {
		secretVersion = segments[2]
	}

	return &secretMetadata{
		host:    strings.ToLower(secretURL.Host),
		name:    secretName,
		version: secretVersion,
	}, nil
}
