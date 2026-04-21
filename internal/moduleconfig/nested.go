package moduleconfig

import (
	"errors"
	"fmt"
	"strconv"
	"strings"
)

// Path parsing errors
var (
	ErrEmptyPath         = errors.New("empty path")
	ErrInvalidArrayIndex = errors.New("invalid array index in path")
)

// IsNestedPath checks if a path contains dot notation or array indexing.
func IsNestedPath(path string) bool {
	for _, c := range path {
		if c == '.' || c == '[' {
			return true
		}
	}
	return false
}

// GetNestedValue extracts a value from a nested object using dot notation.
// Supports paths like "user.profile.name" and array indexing like "items[0].name".
func GetNestedValue(obj map[string]interface{}, path string) (interface{}, bool) {
	value, _, ok := navigate(obj, path, false)
	return value, ok
}

// SetNestedValue sets a value in a nested object using dot notation.
// Creates intermediate objects as needed.
func SetNestedValue(obj map[string]interface{}, path string, value interface{}) error {
	if path == "" {
		return ErrEmptyPath
	}

	parts := strings.Split(path, ".")
	current := obj

	for i := 0; i < len(parts); i++ {
		part, index, hasIndex, err := ParsePathPart(parts[i])
		if err != nil {
			return err
		}
		isLast := i == len(parts)-1

		if !hasIndex {
			if isLast {
				current[part] = value
				return nil
			}
			current = ensureMapAtPath(current, part)
			continue
		}

		if isLast {
			return setArrayElement(current, part, index, value)
		}
		current = ensureMapInArray(current, part, index)
	}

	return nil
}

// DeleteNestedValue removes a value from a nested object using dot notation.
func DeleteNestedValue(obj map[string]interface{}, path string) {
	parent, lastPart, ok := navigate(obj, path, true)
	if !ok {
		return
	}
	deleteLeafFromParent(parent, lastPart)
}

// ParsePathPart parses a path segment and extracts the key and optional array index.
func ParsePathPart(part string) (key string, index int, hasIndex bool, err error) {
	idx := strings.Index(part, "[")
	if idx == -1 {
		return part, -1, false, nil
	}
	endIdx := strings.Index(part, "]")
	if endIdx == -1 || endIdx < idx+1 {
		return "", -1, false, fmt.Errorf("%w: %q", ErrInvalidArrayIndex, part)
	}
	if endIdx != len(part)-1 {
		return "", -1, false, fmt.Errorf("%w: %q", ErrInvalidArrayIndex, part)
	}
	arrayIndex, parseErr := strconv.Atoi(part[idx+1 : endIdx])
	if parseErr != nil || arrayIndex < 0 {
		return "", -1, false, fmt.Errorf("%w: %q", ErrInvalidArrayIndex, part)
	}
	return part[:idx], arrayIndex, true, nil
}

// navigate walks a path through nested maps and arrays.
func navigate(obj map[string]interface{}, path string, stopBeforeLast bool) (current interface{}, lastPart string, ok bool) {
	if path == "" {
		return nil, "", false
	}
	parts := strings.Split(path, ".")
	if len(parts) == 0 {
		return nil, "", false
	}
	endIdx := len(parts)
	if stopBeforeLast {
		endIdx = len(parts) - 1
		if endIdx < 0 {
			return nil, "", false
		}
	}
	current = interface{}(obj)
	for i := 0; i < endIdx; i++ {
		current, ok = navigateStep(current, parts[i])
		if !ok {
			return nil, "", false
		}
	}
	if stopBeforeLast {
		lastPart = parts[len(parts)-1]
	}
	return current, lastPart, true
}

func navigateStep(current interface{}, part string) (next interface{}, ok bool) {
	key, arrayIdx, hasIndex, err := ParsePathPart(part)
	if err != nil {
		return nil, false
	}
	next, ok = getFromMap(current, key)
	if !ok {
		return nil, false
	}
	if hasIndex {
		next, ok = getFromArray(next, arrayIdx)
	}
	return next, ok
}

func getFromMap(current interface{}, key string) (interface{}, bool) {
	m, ok := current.(map[string]interface{})
	if !ok {
		return nil, false
	}
	val, ok := m[key]
	return val, ok
}

func getFromArray(current interface{}, index int) (interface{}, bool) {
	arr, ok := current.([]interface{})
	if !ok || index < 0 || index >= len(arr) {
		return nil, false
	}
	return arr[index], true
}

func deleteLeafFromParent(parent interface{}, lastPart string) {
	key, arrayIdx, hasIndex, err := ParsePathPart(lastPart)
	if err != nil {
		return
	}
	switch p := parent.(type) {
	case map[string]interface{}:
		deleteFromMap(p, key, arrayIdx, hasIndex)
	case []interface{}:
		deleteFromArray(p, key, arrayIdx, hasIndex)
	}
}

func deleteFromMap(parent map[string]interface{}, key string, arrayIdx int, hasIndex bool) {
	if hasIndex {
		arr, ok := parent[key].([]interface{})
		if !ok || arrayIdx >= len(arr) {
			return
		}
		parent[key] = append(arr[:arrayIdx], arr[arrayIdx+1:]...)
	} else {
		delete(parent, key)
	}
}

func deleteFromArray(parent []interface{}, key string, arrayIdx int, hasIndex bool) {
	if hasIndex && arrayIdx < len(parent) {
		if elem, ok := parent[arrayIdx].(map[string]interface{}); ok {
			delete(elem, key)
		}
	}
}

func ensureMapAtPath(current map[string]interface{}, key string) map[string]interface{} {
	next, ok := current[key].(map[string]interface{})
	if !ok {
		next = make(map[string]interface{})
		current[key] = next
	}
	return next
}

func setArrayElement(current map[string]interface{}, key string, index int, value interface{}) error {
	arr := ensureArrayAtPath(current, key)
	if len(arr) <= index {
		arr = append(arr, make([]interface{}, index+1-len(arr))...)
		current[key] = arr
	}
	arr[index] = value
	return nil
}

func ensureArrayAtPath(current map[string]interface{}, key string) []interface{} {
	arr, ok := current[key].([]interface{})
	if !ok {
		arr = make([]interface{}, 0)
		current[key] = arr
	}
	return arr
}

func ensureMapInArray(current map[string]interface{}, key string, index int) map[string]interface{} {
	arr := ensureArrayAtPath(current, key)
	if len(arr) <= index {
		arr = append(arr, make([]interface{}, index+1-len(arr))...)
		current[key] = arr
	}
	if arr[index] == nil {
		arr[index] = make(map[string]interface{})
	}
	next, ok := arr[index].(map[string]interface{})
	if !ok {
		next = make(map[string]interface{})
		arr[index] = next
	}
	return next
}
