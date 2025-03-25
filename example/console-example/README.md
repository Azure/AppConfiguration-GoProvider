# Azure App Configuration Console Example

This example demonstrates how to use Azure App Configuration in a console/command-line application.

## Overview

This simple console application:

1. Loads configuration values from Azure App Configuration
2. Displays them to the user


## Configuration Structure

The example uses a nested configuration structure:

```go
type Config struct {
	Font    Font
	Message string
}

type Font struct {
	Color string
	Size  int
}
```

## Running the Example

### Prerequisites

1. An Azure App Configuration store with the following keys:
   - `Config:Message` - A string value
   - `Config:Font:Color` - A string value (e.g., "blue", "red")
   - `Config:Font:Size` - An integer value

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
