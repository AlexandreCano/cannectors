package filter

import (
	"github.com/cannectors/runtime/internal/cache"
)

// mergeData merges response data into record according to the module's
// configured mergeStrategy ("merge", "replace", or "append").
func (m *HTTPCallModule) mergeData(record, responseData map[string]interface{}) map[string]interface{} {
	switch m.mergeStrategy {
	case "replace":
		result := make(map[string]interface{}, len(record)+len(responseData))
		for k, v := range record {
			result[k] = v
		}
		for k, v := range responseData {
			result[k] = v
		}
		return result
	case "append":
		result := make(map[string]interface{}, len(record)+1)
		for k, v := range record {
			result[k] = v
		}
		result["_response"] = responseData
		return result
	default:
		return m.deepMerge(record, responseData)
	}
}

// deepMerge performs a recursive merge of two maps. Values from b override
// values from a at each level, except for nested maps which are merged.
func (m *HTTPCallModule) deepMerge(a, b map[string]interface{}) map[string]interface{} {
	result := make(map[string]interface{}, len(a)+len(b))
	for k, v := range a {
		result[k] = v
	}
	for k, vb := range b {
		if va, exists := result[k]; exists {
			if mapA, okA := va.(map[string]interface{}); okA {
				if mapB, okB := vb.(map[string]interface{}); okB {
					result[k] = m.deepMerge(mapA, mapB)
					continue
				}
			}
		}
		result[k] = vb
	}
	return result
}

// GetCacheStats returns the current cache statistics.
func (m *HTTPCallModule) GetCacheStats() cache.Stats {
	return m.cache.Stats()
}

// ClearCache clears all entries from the cache.
func (m *HTTPCallModule) ClearCache() {
	m.cache.Clear()
}
