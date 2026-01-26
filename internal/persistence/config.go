// Package persistence provides state persistence for pipeline execution.
package persistence

// StatePersistenceConfig holds the configuration for state persistence.
// Parsed from module configuration's "statePersistence" field.
type StatePersistenceConfig struct {
	// Timestamp persistence configuration
	Timestamp *TimestampConfig `json:"timestamp,omitempty"`

	// ID persistence configuration
	ID *IDConfig `json:"id,omitempty"`

	// StoragePath is the custom storage directory path.
	// Defaults to DefaultStatePath if empty.
	StoragePath string `json:"storagePath,omitempty"`
}

// TimestampConfig holds timestamp persistence configuration.
type TimestampConfig struct {
	// Enabled enables timestamp persistence.
	Enabled bool `json:"enabled"`

	// QueryParam is the query parameter name for API filtering.
	// If set, adds ?{QueryParam}={timestamp} to requests.
	QueryParam string `json:"queryParam,omitempty"`
}

// IDConfig holds ID persistence configuration.
type IDConfig struct {
	// Enabled enables ID persistence.
	Enabled bool `json:"enabled"`

	// Field is the field path to extract ID from records.
	// Supports dot notation for nested fields (e.g., "data.id").
	Field string `json:"field,omitempty"`

	// QueryParam is the query parameter name for API filtering.
	// If set, adds ?{QueryParam}={lastId} to requests.
	QueryParam string `json:"queryParam,omitempty"`
}

// IsEnabled returns true if any persistence is enabled.
func (c *StatePersistenceConfig) IsEnabled() bool {
	if c == nil {
		return false
	}
	return (c.Timestamp != nil && c.Timestamp.Enabled) ||
		(c.ID != nil && c.ID.Enabled)
}

// TimestampEnabled returns true if timestamp persistence is enabled.
func (c *StatePersistenceConfig) TimestampEnabled() bool {
	return c != nil && c.Timestamp != nil && c.Timestamp.Enabled
}

// IDEnabled returns true if ID persistence is enabled.
func (c *StatePersistenceConfig) IDEnabled() bool {
	return c != nil && c.ID != nil && c.ID.Enabled
}

// ParseStatePersistenceConfig parses state persistence configuration from a map.
// Returns nil if the map is nil or empty.
func ParseStatePersistenceConfig(config map[string]interface{}) *StatePersistenceConfig {
	if config == nil {
		return nil
	}

	spConfig, ok := config["statePersistence"].(map[string]interface{})
	if !ok || spConfig == nil {
		return nil
	}

	result := &StatePersistenceConfig{}

	// Parse timestamp config
	if tsConfig, ok := spConfig["timestamp"].(map[string]interface{}); ok {
		result.Timestamp = &TimestampConfig{}
		if enabled, ok := tsConfig["enabled"].(bool); ok {
			result.Timestamp.Enabled = enabled
		}
		if queryParam, ok := tsConfig["queryParam"].(string); ok {
			result.Timestamp.QueryParam = queryParam
		}
	}

	// Parse ID config
	if idConfig, ok := spConfig["id"].(map[string]interface{}); ok {
		result.ID = &IDConfig{}
		if enabled, ok := idConfig["enabled"].(bool); ok {
			result.ID.Enabled = enabled
		}
		if field, ok := idConfig["field"].(string); ok {
			result.ID.Field = field
		}
		if queryParam, ok := idConfig["queryParam"].(string); ok {
			result.ID.QueryParam = queryParam
		}
	}

	// Parse storage path
	if storagePath, ok := spConfig["storagePath"].(string); ok {
		result.StoragePath = storagePath
	}

	return result
}
