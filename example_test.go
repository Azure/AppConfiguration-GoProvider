package azureappconfiguration_test

import (
	"context"
	"fmt"
	"log"
	"os"

	"github.com/Azure/AppConfiguration-GoProvider/azureappconfiguration"
)

func ExampleLoad() {
	// Create authentication options
	authOptions := azureappconfiguration.AuthenticationOptions{
		ConnectionString: os.Getenv("AZURE_APPCONFIG_CONNECTION_STRING"),
	}

	// Create a context
	ctx := context.Background()

	// Load configuration with default options
	provider, err := azureappconfiguration.Load(ctx, authOptions, nil)
	if err != nil {
		log.Fatalf("Failed to load configuration: %v", err)
	}

	// Use the provider...
	fmt.Printf("Configuration loaded successfully: %v\n", provider != nil)

	// Output:
	// Configuration loaded successfully: true
}

func ExampleAzureAppConfiguration_Unmarshal() {
	// Assume we have already loaded the provider
	provider, _ := loadExampleProvider()

	// Define a configuration structure
	type ServerConfig struct {
		Port    int      `json:"port"`
		Host    string   `json:"host"`
		Debug   bool     `json:"debug"`
		Timeout int      `json:"timeout"`
		Tags    []string `json:"tags"`
	}

	// Unmarshal into the struct
	var config ServerConfig
	err := provider.Unmarshal(&config, &azureappconfiguration.ConstructionOptions{
		Separator: ":", // Use ":" as the separator in hierarchical keys
	})
	if err != nil {
		log.Fatalf("Failed to unmarshal configuration: %v", err)
	}

	fmt.Printf("Server will run at %s:%d\n", config.Host, config.Port)
	fmt.Printf("Debug mode: %v, Timeout: %d seconds\n", config.Debug, config.Timeout)
	fmt.Printf("Tags: %v\n", config.Tags)

	// Output:
	// Server will run at localhost:8080
	// Debug mode: true, Timeout: 30 seconds
	// Tags: [production primary]
}

func ExampleAzureAppConfiguration_GetBytes() {
	// Assume we have already loaded the provider
	provider, _ := loadExampleProvider()

	// Get configuration as JSON bytes
	jsonBytes, err := provider.GetBytes(&azureappconfiguration.ConstructionOptions{
		Separator: ":", // Use ":" as the separator in hierarchical keys
	})
	if err != nil {
		log.Fatalf("Failed to get configuration as bytes: %v", err)
	}

	// Print the JSON string for demonstration
	fmt.Println(string(jsonBytes))

	// Output would be JSON like:
	// {"database":{"host":"localhost","port":5432,"username":"admin"},"debug":true,"timeout":30}
}

// Helper function to simulate loading a provider for examples
func loadExampleProvider() (*azureappconfiguration.AzureAppConfiguration, error) {
	// Since we can't actually connect in an example, this is just a placeholder
	return nil, nil // In real examples, this would return a valid provider
}
