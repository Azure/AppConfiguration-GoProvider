// Copyright (c) Microsoft Corporation.
// Licensed under the MIT License.

package tree

import (
	"reflect"
	"strings"
	"testing"
)

func TestTree_Insert_Unflatten_FeatureFlags(t *testing.T) {
	// Test case for feature flags with nested arrays
	tree := &Tree{}

	// Insert a feature flag with variants
	tree.Insert(strings.Split("feature_flags.0.name", "."), "my-feature")
	tree.Insert(strings.Split("feature_flags.0.enabled", "."), true)
	tree.Insert(strings.Split("feature_flags.0.variants.0.configuration_value", "."), "value1")
	tree.Insert(strings.Split("feature_flags.0.variants.1.configuration_value", "."), "value2")
	tree.Insert(strings.Split("feature_flags.1.name", "."), "second-feature")
	tree.Insert(strings.Split("feature_flags.1.enabled", "."), false)

	result := tree.Build()

	// Expected nested structure with arrays
	expected := map[string]interface{}{
		"feature_flags": []interface{}{
			map[string]interface{}{
				"name":    "my-feature",
				"enabled": true,
				"variants": []interface{}{
					map[string]interface{}{
						"configuration_value": "value1",
					},
					map[string]interface{}{
						"configuration_value": "value2",
					},
				},
			},
			map[string]interface{}{
				"name":    "second-feature",
				"enabled": false,
			},
		},
	}

	if !reflect.DeepEqual(result, expected) {
		t.Errorf("Expected %v, got %v", expected, result)
	}
}

func TestTree_Insert_Unflatten_ComplexNestedStructure(t *testing.T) {
	// Test case for complex nested structure with mixed arrays and maps
	tree := &Tree{}

	// Insert a complex configuration with multiple levels of nesting
	tree.Insert(strings.Split("app.settings.timeout", "."), 30)
	tree.Insert(strings.Split("app.endpoints.0.url", "."), "https://api.example.com")
	tree.Insert(strings.Split("app.endpoints.0.methods.0", "."), "GET")
	tree.Insert(strings.Split("app.endpoints.0.methods.1", "."), "POST")
	tree.Insert(strings.Split("app.endpoints.1.url", "."), "https://backup.example.com")
	tree.Insert(strings.Split("app.endpoints.1.methods.0", "."), "GET")

	result := tree.Build()

	// Expected nested structure with maps containing arrays
	expected := map[string]interface{}{
		"app": map[string]interface{}{
			"settings": map[string]interface{}{
				"timeout": 30,
			},
			"endpoints": []interface{}{
				map[string]interface{}{
					"url": "https://api.example.com",
					"methods": []interface{}{
						"GET",
						"POST",
					},
				},
				map[string]interface{}{
					"url": "https://backup.example.com",
					"methods": []interface{}{
						"GET",
					},
				},
			},
		},
	}

	if !reflect.DeepEqual(result, expected) {
		t.Errorf("Expected %v, got %v", expected, result)
	}
}

func TestTree_EmptyArraysAndMaps(t *testing.T) {
	// Test handling of empty arrays and maps
	tree := &Tree{}

	// Create empty array at locations
	tree.Insert(strings.Split("config.locations.0", "."), nil)
	tree.Insert(strings.Split("config.locations.1", "."), nil)

	// Create empty map at settings
	tree.Insert(strings.Split("config.settings", "."), map[string]interface{}{})

	result := tree.Build()

	expected := map[string]interface{}{
		"config": map[string]interface{}{
			"locations": []interface{}{
				nil,
				nil,
			},
			"settings": nil,
		},
	}

	if !reflect.DeepEqual(result, expected) {
		t.Errorf("Expected %v, got %v", expected, result)
	}
}

func TestTree_NonSequentialArrayIndices(t *testing.T) {
	// Test handling of non-sequential array indices
	tree := &Tree{}

	// Insert at indices 0, 2, 5 (skipping 1, 3, 4)
	tree.Insert(strings.Split("items.0", "."), "first")
	tree.Insert(strings.Split("items.2", "."), "third")
	tree.Insert(strings.Split("items.5", "."), "sixth")

	result := tree.Build()

	// Should be interpreted as a map, not an array due to gaps
	expected := map[string]interface{}{
		"items": map[string]interface{}{
			"0": "first",
			"2": "third",
			"5": "sixth",
		},
	}

	if !reflect.DeepEqual(result, expected) {
		t.Errorf("Expected %v, got %v", expected, result)
	}
}

func TestTree_MixedTypes(t *testing.T) {
	// Test handling of mixed types within the same structure
	tree := &Tree{}

	tree.Insert(strings.Split("config.string_value", "."), "string")
	tree.Insert(strings.Split("config.int_value", "."), 42)
	tree.Insert(strings.Split("config.bool_value", "."), true)
	tree.Insert(strings.Split("config.nested.value", "."), "nested value")
	tree.Insert(strings.Split("config.array.0", "."), "array item 1")
	tree.Insert(strings.Split("config.array.1", "."), 123)
	tree.Insert(strings.Split("config.array.2", "."), true)

	result := tree.Build()

	expected := map[string]interface{}{
		"config": map[string]interface{}{
			"string_value": "string",
			"int_value":    42,
			"bool_value":   true,
			"nested": map[string]interface{}{
				"value": "nested value",
			},
			"array": []interface{}{
				"array item 1",
				123,
				true,
			},
		},
	}

	if !reflect.DeepEqual(result, expected) {
		t.Errorf("Expected %v, got %v", expected, result)
	}
}
