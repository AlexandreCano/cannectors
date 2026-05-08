// Package recordpath provides navigation helpers for nested record values
// expressed as map[string]any (decoded JSON). It supports dot
// notation (e.g. "user.profile.name") and array indexing
// (e.g. "items[0].label").
//
// recordpath is a leaf package: it has no internal dependencies, so it can
// be imported from any module (template engine, runtime metadata, output
// modules, filter modules, etc.) without creating cycles.
package recordpath

import (
	"errors"
	"fmt"
	"strconv"
	"strings"
)

// Path parsing errors.
var (
	ErrEmptyPath         = errors.New("empty path")
	ErrInvalidArrayIndex = errors.New("invalid array index in path")
)

// IsNested reports whether path uses dot notation or array indexing.
func IsNested(path string) bool {
	for _, c := range path {
		if c == '.' || c == '[' {
			return true
		}
	}
	return false
}

// Get extracts a value from a nested object using dot notation.
// Supports paths like "user.profile.name" and array indexing like
// "items[0].name".
func Get(obj map[string]any, path string) (any, bool) {
	value, _, ok := navigate(obj, path, false)
	return value, ok
}

// Set sets a value in a nested object using dot notation. Intermediate maps
// and arrays are created as needed.
func Set(obj map[string]any, path string, value any) error {
	if path == "" {
		return ErrEmptyPath
	}

	parts := strings.Split(path, ".")
	current := obj

	for i := 0; i < len(parts); i++ {
		part, index, hasIndex, err := ParsePart(parts[i])
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

// Delete removes a value from a nested object using dot notation. Missing
// paths are silently ignored.
func Delete(obj map[string]any, path string) {
	parent, lastPart, ok := navigate(obj, path, true)
	if !ok {
		return
	}
	deleteLeafFromParent(parent, lastPart)
}

// ParsePart parses a single path segment ("foo" or "foo[3]") and returns its
// key, optional array index, and a flag indicating whether an index was
// present.
func ParsePart(part string) (key string, index int, hasIndex bool, err error) {
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
func navigate(obj map[string]any, path string, stopBeforeLast bool) (current any, lastPart string, ok bool) {
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
	current = any(obj)
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

func navigateStep(current any, part string) (next any, ok bool) {
	key, arrayIdx, hasIndex, err := ParsePart(part)
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

func getFromMap(current any, key string) (any, bool) {
	m, ok := current.(map[string]any)
	if !ok {
		return nil, false
	}
	val, ok := m[key]
	return val, ok
}

func getFromArray(current any, index int) (any, bool) {
	arr, ok := current.([]any)
	if !ok || index < 0 || index >= len(arr) {
		return nil, false
	}
	return arr[index], true
}

func deleteLeafFromParent(parent any, lastPart string) {
	key, arrayIdx, hasIndex, err := ParsePart(lastPart)
	if err != nil {
		return
	}
	switch p := parent.(type) {
	case map[string]any:
		deleteFromMap(p, key, arrayIdx, hasIndex)
	case []any:
		deleteFromArray(p, key, arrayIdx, hasIndex)
	}
}

func deleteFromMap(parent map[string]any, key string, arrayIdx int, hasIndex bool) {
	if hasIndex {
		arr, ok := parent[key].([]any)
		if !ok || arrayIdx >= len(arr) {
			return
		}
		parent[key] = append(arr[:arrayIdx], arr[arrayIdx+1:]...)
	} else {
		delete(parent, key)
	}
}

func deleteFromArray(parent []any, key string, arrayIdx int, hasIndex bool) {
	if hasIndex && arrayIdx < len(parent) {
		if elem, ok := parent[arrayIdx].(map[string]any); ok {
			delete(elem, key)
		}
	}
}

func ensureMapAtPath(current map[string]any, key string) map[string]any {
	next, ok := current[key].(map[string]any)
	if !ok {
		next = make(map[string]any)
		current[key] = next
	}
	return next
}

func setArrayElement(current map[string]any, key string, index int, value any) error {
	arr := ensureArrayAtPath(current, key)
	if len(arr) <= index {
		arr = append(arr, make([]any, index+1-len(arr))...)
		current[key] = arr
	}
	arr[index] = value
	return nil
}

func ensureArrayAtPath(current map[string]any, key string) []any {
	arr, ok := current[key].([]any)
	if !ok {
		arr = make([]any, 0)
		current[key] = arr
	}
	return arr
}

func ensureMapInArray(current map[string]any, key string, index int) map[string]any {
	arr := ensureArrayAtPath(current, key)
	if len(arr) <= index {
		arr = append(arr, make([]any, index+1-len(arr))...)
		current[key] = arr
	}
	if arr[index] == nil {
		arr[index] = make(map[string]any)
	}
	next, ok := arr[index].(map[string]any)
	if !ok {
		next = make(map[string]any)
		arr[index] = next
	}
	return next
}
