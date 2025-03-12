package main

import (
	"context"
	"fmt"
	"log"

	azureappconfiguration "github.com/Azure/appconfiguration-goprovider"
	"github.com/Azure/azure-sdk-for-go/sdk/azidentity"
)

// AppConfig represents the application configuration
type AppConfig struct {
	Server struct {
		Port string `json:"port"`
		Host string `json:"host"`
		// Protocol string `json:"protocol"`
		// Timeout  int    `json:"timeout"`
	} `json:"server"`
	Database struct {
		ConnectionString string `json:"connectionString"`
		Secret           string `json:"secret"`
		// MaxConnections   int    `json:"maxConnections"`
		// Timeout          int    `json:"timeout"`
	} `json:"database"`
	Logging struct {
		Level     string `json:"level"`
		FilePath  string `json:"filePath"`
		MaxSizeMB string `json:"maxSizeMB"`
		// UseConsole bool   `json:"useConsole"`
	} `json:"logging"`
	Features map[string]bool `json:"features"`
}

func main() {
	// Create Azure credential
	credential, err := azidentity.NewAzureCLICredential(nil)
	if err != nil {
		log.Fatalf("Failed to create Azure credential: %v", err)
	}

	// Create provider
	ctx := context.Background()
	provider, err := azureappconfiguration.Load(ctx, azureappconfiguration.AuthenticationOptions{
		ConnectionString: "Endpoint=https://junbchenconfig-test.azconfig.io;Id=Lc6t;Secret=o5evjNp4q+oxkF9pDct3GThx41Zbm/a5q4qBaRraY84=", // Replace with your connection string
	}, &azureappconfiguration.Options{
		Selectors: []azureappconfiguration.Selector{
			{
				KeyFilter:   "*",
				LabelFilter: "godemo",
			},
		},
		KeyVaultOptions: &azureappconfiguration.KeyVaultOptions{
			Credential: credential,
		},
	})

	if err != nil {
		log.Fatalf("Failed to initialize Azure App Configuration provider: %v", err)
	}

	// Create an instance of our configuration struct
	var config AppConfig

	// Unmarshal configuration into the struct
	// This assumes your Azure App Configuration store has keys like:
	// - server.port
	// - server.host
	// - database.connectionString
	// - logging.level
	// - features (JSON string with feature flags)
	err = provider.Unmarshal(&config, &azureappconfiguration.ConstructOptions{
		Separator: "__",
	})

	if err != nil {
		log.Fatalf("Failed to unmarshal configuration: %v", err)
	}

	// Access the configuration values through the struct
	fmt.Println("Server Configuration:")
	fmt.Printf("  Host: %s\n", config.Server.Host)
	fmt.Printf("  Port: %s\n", config.Server.Port)
	// fmt.Printf("  Protocol: %s\n", config.Server.Protocol)
	// fmt.Printf("  Timeout: %d seconds\n", config.Server.Timeout)

	fmt.Println("\nDatabase Configuration:")
	fmt.Printf("  Connection String: %s\n", config.Database.ConnectionString)
	fmt.Printf("  Secret: %s\n", config.Database.Secret)

	fmt.Println("\nLogging Configuration:")
	fmt.Printf("  Level: %s\n", config.Logging.Level)
	fmt.Printf("  File Path: %s\n", config.Logging.FilePath)
	// fmt.Printf("  Use Console: %t\n", config.Logging.UseConsole)

	// Use feature flags from the struct
	fmt.Println("\nFeatures:")
	for name, enabled := range config.Features {
		fmt.Printf("  %s: %t\n", name, enabled)
	}
}
