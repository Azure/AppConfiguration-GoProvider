# Azure App Configuration - Go Provider

[![PkgGoDev](https://pkg.go.dev/badge/github.com/Azure/AppConfiguration-GoProvider/azureappconfiguration)](https://pkg.go.dev/github.com/Azure/AppConfiguration-GoProvider/azureappconfiguration)

## Overview

[Azure App Configuration](https://docs.microsoft.com/azure/azure-app-configuration/overview) provides centralized configuration storage and management, allowing users to update their configurations without the need to rebuild and redeploy their applications. The App Configuration provider for Go is built on top of the [Azure Go SDK](https://pkg.go.dev/github.com/Azure/azure-sdk-for-go/sdk/data/azappconfig) and is designed to simplify data consumption in App Configuration with rich features. Users can consume App Configuration key-values as strongly-typed structs with data binding or load them into popular third-party configuration libraries, minimizing code changes. The Go provider offers features such as configuration composition from multiple labels, key prefix trimming, automatic resolution of Key Vault references, feature flags, failover with geo-replication for enhanced reliability, and many more.

## Installation

```bash
go get github.com/Azure/AppConfiguration-GoProvider/azureappconfiguration
```

## Getting Started

Official documentation on how to use the Azure App Configuration provider is available in the following quickstarts:

- [Create a Go console app with Azure App Configuration](https://learn.microsoft.com/azure/azure-app-configuration/quickstart-go-console-app?tabs=entra-id)
- [Enable dynamic configuration refresh in a Go console app](https://learn.microsoft.com/azure/azure-app-configuration/enable-dynamic-configuration-go-console-app)
- [Create a Go web app with Azure App Configuration](https://learn.microsoft.com/azure/azure-app-configuration/quickstart-go-web-app?tabs=entra-id)
- [Use dynamic configuration in a Gin web app](https://learn.microsoft.com/azure/azure-app-configuration/enable-dynamic-configuration-gin-web-app)

## Examples

- [Console Application](../example/console_app/)
- [Gin Web Application](../example/gin_web_app/)

## Data Collection

The software may collect information about you and your use of the software and send it to Microsoft. Microsoft may use this information to provide services and improve our products and services. You may turn off the telemetry by setting the environment variable `AZURE_APP_CONFIGURATION_TRACING_DISABLED` to `TRUE`. There are also some features in the software that may enable you and Microsoft to collect data from users of your applications. If you use these features, you must comply with applicable law, including providing appropriate notices to users of your applications together with a copy of Microsoft’s privacy statement. Our privacy statement is located at https://go.microsoft.com/fwlink/?LinkID=824704. You can learn more about data collection and use in the help documentation and our privacy statement. Your use of the software operates as your consent to these practices.

## Contributing

This project welcomes contributions and suggestions.  Most contributions require you to agree to a
Contributor License Agreement (CLA) declaring that you have the right to, and actually do, grant us
the rights to use your contribution. For details, visit https://cla.opensource.microsoft.com.

When you submit a pull request, a CLA bot will automatically determine whether you need to provide
a CLA and decorate the PR appropriately (e.g., status check, comment). Simply follow the instructions
provided by the bot. You will only need to do this once across all repos using our CLA.

This project has adopted the [Microsoft Open Source Code of Conduct](https://opensource.microsoft.com/codeofconduct/).
For more information see the [Code of Conduct FAQ](https://opensource.microsoft.com/codeofconduct/faq/) or
contact [opencode@microsoft.com](mailto:opencode@microsoft.com) with any additional questions or comments.

## Trademarks

This project may contain trademarks or logos for projects, products, or services. Authorized use of Microsoft 
trademarks or logos is subject to and must follow 
[Microsoft's Trademark & Brand Guidelines](https://www.microsoft.com/en-us/legal/intellectualproperty/trademarks/usage/general).
Use of Microsoft trademarks or logos in modified versions of this project must not cause confusion or imply Microsoft sponsorship.
Any use of third-party trademarks or logos are subject to those third-party's policies.
