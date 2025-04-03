# Azure App Configuration - Go Provider

[![PkgGoDev](https://pkg.go.dev/badge/github.com/Azure/AppConfiguration-GoProvider/azureappconfiguration)](https://pkg.go.dev/github.com/Azure/AppConfiguration-GoProvider/azureappconfiguration)

## Overview

Azure App Configuration Go Provider is a client library that simplifies using [Azure App Configuration](https://docs.microsoft.com/en-us/azure/azure-app-configuration/overview) in Go applications. It provides a high-level abstraction over the [Azure SDK for Go](https://pkg.go.dev/github.com/Azure/azure-sdk-for-go/sdk/data/azappconfig) that makes managing application settings easier and more intuitive.

## Features

- **Simple, Strongly-Typed Configuration**: Easily bind hierarchical configuration to Go struct
- **Key Filtering and Prefix Management**: Selectively load configuration and remove prefixes for cleaner code
- **Key Vault reference resolution**: Transparently resolve Key Vault references as part of the configuration loading process
- **JSON Content Support**: Automatic parsing of JSON values into native Go types

## Installation

```bash
go get github.com/Azure/AppConfiguration-GoProvider/azureappconfiguration
```

## Examples

The repository includes complete examples showing how to use the Go Provider in different scenarios:

- [Console Application](./example/console-example/): Simple CLI app using App Configuration
- [Web Application](./example/gin-example/): Gin web app with App Configuration integration

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
