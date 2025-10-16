// Copyright (c) Microsoft Corporation.
// Licensed under the MIT License.

package azureappconfiguration

import (
	"fmt"
	"regexp"
	"strings"

	"github.com/Azure/AppConfiguration-GoProvider/azureappconfiguration/internal/tracing"
)

func verifyAuthenticationOptions(authOptions AuthenticationOptions) error {
	if authOptions.ConnectionString == "" &&
		!(authOptions.Endpoint != "" && authOptions.Credential != nil) {
		return fmt.Errorf("either connection string or endpoint and credential must be provided")
	}

	return nil
}

func verifyOptions(options *Options) error {
	if options == nil {
		return nil
	}

	if err := verifySelectors(options.Selectors); err != nil {
		return err
	}

	if options.RefreshOptions.Enabled {
		if options.RefreshOptions.Interval != 0 &&
			options.RefreshOptions.Interval < minimalRefreshInterval {
			return fmt.Errorf("key value refresh interval cannot be less than %s", minimalRefreshInterval)
		}

		for _, watchedSetting := range options.RefreshOptions.WatchedSettings {
			if watchedSetting.Key == "" {
				return fmt.Errorf("watched setting key cannot be empty")
			}

			if strings.Contains(watchedSetting.Key, "*") || strings.Contains(watchedSetting.Key, ",") {
				return fmt.Errorf("watched setting key cannot contain '*' or ','")
			}

			if watchedSetting.Label != "" &&
				(strings.Contains(watchedSetting.Label, "*") || strings.Contains(watchedSetting.Label, ",")) {
				return fmt.Errorf("watched setting label cannot contain '*' or ','")
			}
		}
	}

	if options.KeyVaultOptions.RefreshOptions.Enabled {
		if options.KeyVaultOptions.RefreshOptions.Interval != 0 &&
			options.KeyVaultOptions.RefreshOptions.Interval < minimalKeyVaultRefreshInterval {
			return fmt.Errorf("refresh interval of Key Vault secrets cannot be less than %s", minimalKeyVaultRefreshInterval)
		}
	}

	if options.FeatureFlagOptions.Enabled {
		if err := verifySelectors(options.FeatureFlagOptions.Selectors); err != nil {
			return err
		}

		if options.FeatureFlagOptions.RefreshOptions.Enabled {
			if options.FeatureFlagOptions.RefreshOptions.Interval != 0 &&
				options.FeatureFlagOptions.RefreshOptions.Interval < minimalRefreshInterval {
				return fmt.Errorf("feature flag refresh interval cannot be less than %s", minimalRefreshInterval)
			}
		}
	}

	return nil
}

func verifySelectors(selectors []Selector) error {
	for _, selector := range selectors {
		if selector.SnapshotName != "" {
			if selector.KeyFilter != "" || selector.LabelFilter != "" || selector.TagFilter != nil {
				return fmt.Errorf("key, label and tag filters should not be used if snapshot name is provided")
			}
		} else {
			if selector.KeyFilter == "" {
				return fmt.Errorf("one of key filter or snapshot name must be provided")
			}

			if strings.Contains(selector.LabelFilter, "*") || strings.Contains(selector.LabelFilter, ",") {
				return fmt.Errorf("label filter cannot contain '*' or ','")
			}

			if err := validateTagFilters(selector.TagFilter); err != nil {
				return err
			}
		}
	}

	return nil
}

// validateTagFilters validates that each tag filter follows the required format "tagName=tagValue"
// and ensures no more than 5 tag filters are provided.
func validateTagFilters(tagFilters []string) error {
	if len(tagFilters) > 5 {
		return fmt.Errorf("up to 5 tag filters can be provided, got %d", len(tagFilters))
	}

	for _, tagFilter := range tagFilters {
		if tagFilter == "" {
			return fmt.Errorf("tag filter cannot be empty")
		}

		parts := strings.Split(tagFilter, "=")
		if len(parts) != 2 || parts[0] == "" {
			return fmt.Errorf("invalid tag filter: %s. Tag filter must follow the format \"tagName=tagValue\"", tagFilter)
		}
	}

	return nil
}

func reverse(arr []Selector) {
	for i, j := 0, len(arr)-1; i < j; i, j = i+1, j-1 {
		arr[i], arr[j] = arr[j], arr[i]
	}
}

func verifySeparator(separator string) error {
	isValid := false
	validSeparators := []string{".", ",", ";", "-", "_", "__", "/", ":"}
	for _, valid := range validSeparators {
		if separator == valid {
			isValid = true
			break
		}
	}

	if !isValid {
		return fmt.Errorf("invalid separator '%s'. Supported values: %s", separator, strings.Join(validSeparators, ", "))
	}

	return nil
}

func isJsonContentType(contentType *string) bool {
	if contentType == nil {
		return false
	}
	contentTypeStr := strings.ToLower(strings.Trim(*contentType, " "))
	matched, _ := regexp.MatchString("^application\\/(?:[^\\/]+\\+)?json(;.*)?$", contentTypeStr)
	return matched
}

func isAIConfigurationContentType(contentType *string) bool {
	return hasProfile(*contentType, tracing.AIMimeProfile)
}

func isAIChatCompletionContentType(contentType *string) bool {
	return hasProfile(*contentType, tracing.AIChatCompletionMimeProfile)
}

// hasProfile checks if a content type contains a specific profile parameter
func hasProfile(contentType, profileValue string) bool {
	// Split by semicolons to get content type parts
	parts := strings.Split(contentType, ";")

	// Check each part after the content type for profile parameter
	for i := 1; i < len(parts); i++ {
		part := strings.TrimSpace(parts[i])

		// Look for profile="value" pattern
		if strings.HasPrefix(part, "profile=") {
			// Extract the profile value (handling quoted values)
			profile := part[len("profile="):]
			profile = strings.Trim(profile, "\"'")

			if profile == profileValue {
				return true
			}
		}
	}

	return false
}
