package azureappconfiguration

import (
	"context"
	"errors"
	"fmt"
	"net/url"
	"strings"

	"github.com/Azure/azure-sdk-for-go/sdk/security/keyvault/azsecrets"
)

// KeyVaultContentType is the content type used to identify Key Vault references
const KeyVaultContentType = "application/vnd.microsoft.appconfig.keyvaultref+json;charset=utf-8"

// resolveSecret resolves a secret using the configured resolver or by connecting to Key Vault.
func (cfg *AzureAppConfiguration) resolveSecret(ctx context.Context, uri string) (string, error) {
	// Try to use the custom resolver if provided
	if cfg.keyVaultOptions.SecretResolver != nil {
		secret, err := cfg.keyVaultOptions.SecretResolver.ResolveSecret(ctx, uri)
		if err == nil {
			return secret, nil
		}
		// If the resolver failed, try other methods
	}

	// Use an existing secret client if available
	client, err := cfg.getSecretClient(ctx, uri)
	if err != nil {
		return "", err
	}

	// Parse the URI to extract vault name and secret name
	parsedURI, err := url.Parse(uri)
	if err != nil {
		return "", fmt.Errorf("invalid Key Vault URI: %w", err)
	}

	// Extract secret name and version from the URI path
	// Format is typically /secrets/secretName[/version]
	pathParts := strings.Split(strings.TrimPrefix(parsedURI.Path, "/"), "/")
	if len(pathParts) < 2 || pathParts[0] != "secrets" {
		return "", fmt.Errorf("invalid Key Vault secret path: %s", parsedURI.Path)
	}

	secretName := pathParts[1]
	var version string
	if len(pathParts) > 2 {
		version = pathParts[2]
	}

	// Get the secret from Key Vault
	resp, err := client.GetSecret(ctx, secretName, version, nil)
	if err != nil {
		return "", fmt.Errorf("failed to get secret from Key Vault: %w", err)
	}

	if resp.Value == nil {
		return "", errors.New("secret value is nil")
	}

	return *resp.Value, nil
}

// getSecretClient returns a secret client for the given Key Vault URI.
func (cfg *AzureAppConfiguration) getSecretClient(ctx context.Context, uri string) (*azsecrets.Client, error) {
	// Parse the URI to extract the vault URL
	parsedURI, err := url.Parse(uri)
	if err != nil {
		return nil, fmt.Errorf("invalid Key Vault URI: %w", err)
	}

	vaultURL := fmt.Sprintf("%s://%s", parsedURI.Scheme, parsedURI.Host)

	// Check if we already have a client for this vault
	if cfg.keyVaultOptions.SecretClients != nil {
		if client, found := cfg.keyVaultOptions.SecretClients[vaultURL]; found && client != nil {
			return client, nil
		}
	}

	// Ensure we have the credential needed to create a new client
	if cfg.keyVaultOptions.Credential == nil {
		return nil, errors.New("no credential provided for Key Vault authentication")
	}

	// Create a new client
	client, err := azsecrets.NewClient(vaultURL, cfg.keyVaultOptions.Credential, nil)
	if err != nil {
		return nil, fmt.Errorf("failed to create Key Vault secret client: %w", err)
	}

	// Store the client for future use if the map exists
	if cfg.keyVaultOptions.SecretClients != nil {
		cfg.keyVaultOptions.SecretClients[vaultURL] = client
	} else {
		// Initialize the map with this client
		cfg.keyVaultOptions.SecretClients = map[string]*azsecrets.Client{
			vaultURL: client,
		}
	}

	return client, nil
}
