module console-example-app

go 1.23.0

require github.com/Azure/AppConfiguration-GoProvider/azureappconfiguration v0.0.0-00010101000000-000000000000

require (
	github.com/Azure/azure-sdk-for-go/sdk/azcore v1.17.0 // indirect
	github.com/Azure/azure-sdk-for-go/sdk/data/azappconfig v1.2.0-beta.1 // indirect
	github.com/Azure/azure-sdk-for-go/sdk/internal v1.10.0 // indirect
	github.com/Azure/azure-sdk-for-go/sdk/security/keyvault/azsecrets v1.3.1 // indirect
	github.com/Azure/azure-sdk-for-go/sdk/security/keyvault/internal v1.1.1 // indirect
	github.com/go-viper/mapstructure/v2 v2.2.1 // indirect
	golang.org/x/net v0.35.0 // indirect
	golang.org/x/sync v0.11.0 // indirect
	golang.org/x/text v0.22.0 // indirect
)

replace github.com/Azure/AppConfiguration-GoProvider/azureappconfiguration => ..\..\
