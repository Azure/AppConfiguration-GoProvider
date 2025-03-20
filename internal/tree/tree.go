// Copyright (c) Microsoft Corporation.
// Licensed under the MIT License.

package tree

import (
	"strconv"
)

type Tree struct {
	children map[string]*Tree
	value    any
}

func (t *Tree) Insert(parts []string, value any) {
	tree := t
	for i, part := range parts {
		if tree.children == nil {
			tree.children = make(map[string]*Tree)
		}

		childTree, ok := tree.children[part]
		if !ok {
			childTree = &Tree{}
			tree.children[part] = childTree
		}

		tree = childTree
		if i == len(parts)-1 {
			switch obj := value.(type) {
			case map[string]interface{}:
				for k, v := range obj {
					tree.Insert([]string{k}, v)
				}
			case []interface{}:
				for k, v := range obj {
					tree.Insert([]string{strconv.Itoa(k)}, v)
				}
			default:
				tree.value = value
			}
		}
	}
}

func (t *Tree) Build() map[string]interface{} {
	result := make(map[string]interface{})
	for k, v := range t.children {
		result[k] = v.Unflatten()
	}

	return result
}

func (t *Tree) Unflatten() interface{} {
	if len(t.children) == 0 {
		return t.value
	}

	isArray := true
	childrenArray := make([]*Tree, len(t.children))

	for k, v := range t.children {
		idx, err := strconv.Atoi(k)
		if err != nil || idx >= len(t.children) || idx < 0 {
			isArray = false
			break
		}
		childrenArray[idx] = v
	}

	if isArray {
		result := make([]interface{}, len(childrenArray))
		for i, child := range childrenArray {
			result[i] = child.Unflatten()
		}
		return result
	}

	result := make(map[string]interface{})
	for k, child := range t.children {
		result[k] = child.Unflatten()
	}

	return result
}
