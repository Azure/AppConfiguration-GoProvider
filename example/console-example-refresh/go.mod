module console-example-refresh

go 1.23.2

require github.com/Azure/AppConfiguration-GoProvider/azureappconfiguration v0.0.0-00010101000000-000000000000

require (
	github.com/Azure/azure-sdk-for-go/sdk/azcore v1.18.0 // indirect
	github.com/Azure/azure-sdk-for-go/sdk/data/azappconfig v1.2.0 // indirect
	github.com/Azure/azure-sdk-for-go/sdk/internal v1.11.1 // indirect
	github.com/Azure/azure-sdk-for-go/sdk/security/keyvault/azsecrets v1.3.1 // indirect
	github.com/Azure/azure-sdk-for-go/sdk/security/keyvault/internal v1.1.1 // indirect
	github.com/go-viper/mapstructure/v2 v2.2.1 // indirect
	golang.org/x/net v0.39.0 // indirect
	golang.org/x/sync v0.13.0 // indirect
	golang.org/x/text v0.24.0 // indirect
)

replace github.com/Azure/AppConfiguration-GoProvider/azureappconfiguration => ..\..\azureappconfiguration
