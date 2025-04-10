// Copyright (c) Microsoft Corporation.
// Licensed under the MIT License.

package azureappconfiguration

import (
	"fmt"
	"strings"
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
