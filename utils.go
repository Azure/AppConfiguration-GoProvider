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

	if options.RefreshOptions.Enabled {
		if options.RefreshOptions.Interval > 0 &&
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

func compare(a *string, b *string) bool {
	if a == b {
		return true
	}
	if a == nil || b == nil {
		return false
	}
	return strings.Compare(*a, *b) == 0
}

func reverse(arr []Selector) {
	for i, j := 0, len(arr)-1; i < j; i, j = i+1, j-1 {
		arr[i], arr[j] = arr[j], arr[i]
	}
}

func verifySeparator(separator string) error {
	if separator == "" {
		separator = defaultSeparator
	}

	validSeparators := []string{".", ",", ";", "-", "_", "__", "/", ":"}

	isValid := false
	for _, valid := range validSeparators {
		if separator == valid {
			isValid = true
			break
		}
	}

	if !isValid {
		return fmt.Errorf("invalid separator '%s'. Supported values: %s.", separator, strings.Join(validSeparators, ", "))
	}
	return nil
}
