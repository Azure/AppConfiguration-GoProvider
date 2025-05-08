// Copyright (c) Microsoft Corporation.
// Licensed under the MIT License.

package tracing

import (
	"context"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestCreateCorrelationContextHeader(t *testing.T) {
	t.Run("empty options", func(t *testing.T) {
		ctx := context.Background()
		options := Options{}

		header := CreateCorrelationContextHeader(ctx, options)

		// The header should be empty but exist
		corrContext := header.Get(CorrelationContextHeader)
		assert.Equal(t, "RequestType=StartUp", corrContext)
	})

	t.Run("with RequestTypeStartUp", func(t *testing.T) {
		options := Options{}

		header := CreateCorrelationContextHeader(context.Background(), options)

		// Should contain RequestTypeStartUp
		corrContext := header.Get(CorrelationContextHeader)
		assert.Contains(t, corrContext, RequestTypeKey+"="+string(RequestTypeStartUp))
	})

	t.Run("with RequestTypeWatch", func(t *testing.T) {
		options := Options{
			InitialLoadFinished: true,
		}

		header := CreateCorrelationContextHeader(context.Background(), options)

		// Should contain RequestTypeWatch
		corrContext := header.Get(CorrelationContextHeader)
		assert.Contains(t, corrContext, RequestTypeKey+"="+string(RequestTypeWatch))
	})

	t.Run("with Host", func(t *testing.T) {
		ctx := context.Background()
		options := Options{
			Host: HostTypeAzureWebApp,
		}

		header := CreateCorrelationContextHeader(ctx, options)

		// Should contain Host
		corrContext := header.Get(CorrelationContextHeader)
		assert.Contains(t, corrContext, HostTypeKey+"="+string(HostTypeAzureWebApp))
	})

	t.Run("with KeyVault configured", func(t *testing.T) {
		ctx := context.Background()
		options := Options{
			KeyVaultConfigured: true,
		}

		header := CreateCorrelationContextHeader(ctx, options)

		// Should contain KeyVaultConfiguredTag
		corrContext := header.Get(CorrelationContextHeader)
		assert.Contains(t, corrContext, KeyVaultConfiguredTag)
	})

	t.Run("with KeyVaultRefresh configured", func(t *testing.T) {
		ctx := context.Background()
		options := Options{
			KeyVaultConfigured:        true,
			KeyVaultRefreshConfigured: true,
		}

		header := CreateCorrelationContextHeader(ctx, options)

		// Should contain KeyVaultRefreshConfiguredTag
		corrContext := header.Get(CorrelationContextHeader)
		assert.Contains(t, corrContext, KeyVaultRefreshConfiguredTag)
	})

	t.Run("with AI configuration", func(t *testing.T) {
		ctx := context.Background()
		options := Options{
			UseAIConfiguration: true,
		}

		header := CreateCorrelationContextHeader(ctx, options)

		// Should contain AIConfigurationTag
		corrContext := header.Get(CorrelationContextHeader)
		assert.Contains(t, corrContext, FeaturesKey+"="+AIConfigurationTag)
	})

	t.Run("with AI chat completion configuration", func(t *testing.T) {
		ctx := context.Background()
		options := Options{
			UseAIChatCompletionConfiguration: true,
		}

		header := CreateCorrelationContextHeader(ctx, options)

		// Should contain AIChatCompletionConfigurationTag
		corrContext := header.Get(CorrelationContextHeader)
		assert.Contains(t, corrContext, FeaturesKey+"="+AIChatCompletionConfigurationTag)
	})

	t.Run("with both AI configurations", func(t *testing.T) {
		ctx := context.Background()
		options := Options{
			UseAIConfiguration:               true,
			UseAIChatCompletionConfiguration: true,
		}

		header := CreateCorrelationContextHeader(ctx, options)

		// Should contain both AI configuration tags
		corrContext := header.Get(CorrelationContextHeader)
		assert.Contains(t, corrContext, FeaturesKey+"=")

		// Extract the Features part
		parts := strings.Split(corrContext, DelimiterComma)
		var featuresPart string
		for _, part := range parts {
			if strings.HasPrefix(part, FeaturesKey+"=") {
				featuresPart = part
				break
			}
		}

		// Check both tags are in the features part
		assert.Contains(t, featuresPart, AIConfigurationTag)
		assert.Contains(t, featuresPart, AIChatCompletionConfigurationTag)

		// Check the delimiter is correct
		features := strings.Split(strings.TrimPrefix(featuresPart, FeaturesKey+"="), DelimiterPlus)
		assert.Len(t, features, 2)
		assert.Contains(t, features, AIConfigurationTag)
		assert.Contains(t, features, AIChatCompletionConfigurationTag)
	})

	t.Run("with all options", func(t *testing.T) {
		options := Options{
			Host:                             HostTypeAzureFunction,
			KeyVaultConfigured:               true,
			UseAIConfiguration:               true,
			UseAIChatCompletionConfiguration: true,
		}

		header := CreateCorrelationContextHeader(context.Background(), options)

		// Check the complete header
		corrContext := header.Get(CorrelationContextHeader)

		assert.Contains(t, corrContext, RequestTypeKey+"="+string(RequestTypeStartUp))
		assert.Contains(t, corrContext, HostTypeKey+"="+string(HostTypeAzureFunction))
		assert.Contains(t, corrContext, KeyVaultConfiguredTag)

		// Extract the Features part
		parts := strings.Split(corrContext, DelimiterComma)
		var featuresPart string
		for _, part := range parts {
			if strings.HasPrefix(part, FeaturesKey+"=") {
				featuresPart = part
				break
			}
		}

		// Check both AI tags are in the features part
		assert.Contains(t, featuresPart, AIConfigurationTag)
		assert.Contains(t, featuresPart, AIChatCompletionConfigurationTag)

		// Verify the header format
		assert.Equal(t, 4, strings.Count(corrContext, DelimiterComma)+1, "Should have 4 parts")
	})

	t.Run("delimiter handling", func(t *testing.T) {
		options := Options{
			Host:               HostTypeAzureWebApp,
			KeyVaultConfigured: true,
		}

		header := CreateCorrelationContextHeader(context.Background(), options)

		// Check the complete header
		corrContext := header.Get(CorrelationContextHeader)

		// Verify there are exactly 3 parts separated by commas
		parts := strings.Split(corrContext, DelimiterComma)
		assert.Len(t, parts, 3, "Should have 3 parts separated by commas")
	})
}
