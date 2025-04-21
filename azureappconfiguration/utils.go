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

	return nil
}

func verifySelectors(selectors []Selector) error {
	for _, selector := range selectors {
		if selector.KeyFilter == "" {
			return fmt.Errorf("key filter cannot be empty")
		}

		if strings.Contains(selector.LabelFilter, "*") || strings.Contains(selector.LabelFilter, ",") {
			return fmt.Errorf("label filter cannot contain '*' or ','")
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
