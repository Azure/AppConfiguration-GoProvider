// Copyright (c) Microsoft Corporation.
// Licensed under the MIT License.

package tracing

import (
	"context"
	"net/http"
	"os"
	"strconv"
	"strings"
)

type RequestType string
type RequestTracingKey string
type HostType string

const (
	RequestTypeStartUp RequestType = "StartUp"
	RequestTypeWatch   RequestType = "Watch"

	HostTypeAzureFunction HostType = "AzureFunction"
	HostTypeAzureWebApp   HostType = "AzureWebApp"
	HostTypeContainerApp  HostType = "ContainerApp"
	HostTypeKubernetes    HostType = "Kubernetes"
	HostTypeServiceFabric HostType = "ServiceFabric"

	EnvVarTracingDisabled = "AZURE_APP_CONFIGURATION_TRACING_DISABLED"
	EnvVarAzureFunction   = "FUNCTIONS_EXTENSION_VERSION"
	EnvVarAzureWebApp     = "WEBSITE_SITE_NAME"
	EnvVarContainerApp    = "CONTAINER_APP_NAME"
	EnvVarKubernetes      = "KUBERNETES_PORT"
	// Documentation : https://docs.microsoft.com/en-us/azure/service-fabric/service-fabric-environment-variables-reference
	EnvVarServiceFabric = "Fabric_NodeName"

	RequestTypeKey                   = "RequestType"
	HostTypeKey                      = "Host"
	KeyVaultConfiguredTag            = "UsesKeyVault"
	KeyVaultRefreshConfiguredTag     = "RefreshesKeyVault"
	FeaturesKey                      = "Features"
	AIConfigurationTag               = "AI"
	AIChatCompletionConfigurationTag = "AICC"

	// Feature flag usage tracing
	FeatureFilterTypeKey = "Filter"
	CustomFilterKey      = "CSTM"
	TimeWindowFilterKey  = "TIME"
	TargetingFilterKey   = "TRGT"
	FFTelemetryUsedTag   = "Telemetry"
	FFMaxVariantsKey     = "MaxVariants"
	FFSeedUsedTag        = "Seed"
	FFFeaturesKey        = "FFFeatures"
	TimeWindowFilterName = "Microsoft.TimeWindow"
	TargetingFilterName  = "Microsoft.Targeting"

	AIMimeProfile               = "https://azconfig.io/mime-profiles/ai"
	AIChatCompletionMimeProfile = "https://azconfig.io/mime-profiles/ai/chat-completion"

	DelimiterPlus            = "+"
	DelimiterComma           = ","
	CorrelationContextHeader = "Correlation-Context"
)

type Options struct {
	Enabled                          bool
	InitialLoadFinished              bool
	Host                             HostType
	KeyVaultConfigured               bool
	KeyVaultRefreshConfigured        bool
	UseAIConfiguration               bool
	UseAIChatCompletionConfiguration bool
	FeatureFlagTracing               *FeatureFlagTracing
}

func GetHostType() HostType {
	if _, ok := os.LookupEnv(EnvVarAzureFunction); ok {
		return HostTypeAzureFunction
	} else if _, ok := os.LookupEnv(EnvVarAzureWebApp); ok {
		return HostTypeAzureWebApp
	} else if _, ok := os.LookupEnv(EnvVarContainerApp); ok {
		return HostTypeContainerApp
	} else if _, ok := os.LookupEnv(EnvVarKubernetes); ok {
		return HostTypeKubernetes
	} else if _, ok := os.LookupEnv(EnvVarServiceFabric); ok {
		return HostTypeServiceFabric
	}
	return ""
}

func CreateCorrelationContextHeader(ctx context.Context, options Options) http.Header {
	header := http.Header{}
	output := make([]string, 0)

	if !options.InitialLoadFinished {
		output = append(output, RequestTypeKey+"="+string(RequestTypeStartUp))
	} else {
		output = append(output, RequestTypeKey+"="+string(RequestTypeWatch))
	}

	if options.Host != "" {
		output = append(output, HostTypeKey+"="+string(options.Host))
	}

	if options.KeyVaultConfigured {
		output = append(output, KeyVaultConfiguredTag)
	}

	if options.KeyVaultRefreshConfigured {
		output = append(output, KeyVaultRefreshConfiguredTag)
	}

	features := make([]string, 0)
	if options.UseAIConfiguration {
		features = append(features, AIConfigurationTag)
	}

	if options.UseAIChatCompletionConfiguration {
		features = append(features, AIChatCompletionConfigurationTag)
	}

	if len(features) > 0 {
		featureStr := FeaturesKey + "=" + strings.Join(features, DelimiterPlus)
		output = append(output, featureStr)
	}

	if options.FeatureFlagTracing != nil {
		if options.FeatureFlagTracing.UsesAnyFeatureFilter() {
			output = append(output, FeatureFilterTypeKey+"="+options.FeatureFlagTracing.CreateFeatureFiltersString())
		}
		if options.FeatureFlagTracing.UsesAnyTracingFeature() {
			output = append(output, FFFeaturesKey+"="+options.FeatureFlagTracing.CreateFeaturesString())
		}
		if options.FeatureFlagTracing.MaxVariants > 0 {
			output = append(output, FFMaxVariantsKey+"="+strconv.Itoa(options.FeatureFlagTracing.MaxVariants))
		}
	}

	header.Add(CorrelationContextHeader, strings.Join(output, DelimiterComma))

	return header
}
