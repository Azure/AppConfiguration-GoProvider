# Azure App Configuration Gin Web Example

This example demonstrates how to use Azure App Configuration in a web application built with the Gin framework.

## Overview

This web application:

1. Loads configuration values from Azure App Configuration
2. Configures the Gin web framework based on those values

## Configuration Structure

The example uses a nested configuration structure:

```go
type Config struct {
	App     App
	Message string
}

type App struct {
	Name      string
	DebugMode bool
}
```

## Running the Example

### Prerequisites

1. An Azure App Configuration store with the following keys:
   - `Config:Message` - A string message to display on the home page
   - `Config:App:Name` - A string for the application name
   - `Config:App:DebugMode` - A boolean to control debug mode

2. Set the connection string as an environment variable:

```bash
# Windows
set AZURE_APPCONFIG_CONNECTION_STRING=your-connection-string

# Linux/macOS
export AZURE_APPCONFIG_CONNECTION_STRING=your-connection-string
```

### Run the Application

```bash
go run main.go
```

Then navigate to `http://localhost:8080` in your web browser.
