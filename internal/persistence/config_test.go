// Package persistence provides state persistence for pipeline execution.
package persistence

import (
	"testing"
)

func TestParseStatePersistenceConfig_Nil(t *testing.T) {
	result := ParseStatePersistenceConfig(nil)
	if result != nil {
		t.Errorf("ParseStatePersistenceConfig(nil) = %v, want nil", result)
	}
}

func TestParseStatePersistenceConfig_Empty(t *testing.T) {
	result := ParseStatePersistenceConfig(map[string]interface{}{})
	if result != nil {
		t.Errorf("ParseStatePersistenceConfig({}) = %v, want nil", result)
	}
}

func TestParseStatePersistenceConfig_NoStatePersistence(t *testing.T) {
	config := map[string]interface{}{
		"endpoint": "https://api.example.com",
	}
	result := ParseStatePersistenceConfig(config)
	if result != nil {
		t.Errorf("ParseStatePersistenceConfig without statePersistence = %v, want nil", result)
	}
}

func TestParseStatePersistenceConfig_TimestampOnly(t *testing.T) {
	config := map[string]interface{}{
		"statePersistence": map[string]interface{}{
			"timestamp": map[string]interface{}{
				"enabled":    true,
				"queryParam": "since",
			},
		},
	}

	result := ParseStatePersistenceConfig(config)
	if result == nil {
		t.Fatal("ParseStatePersistenceConfig returned nil")
	}

	if !result.TimestampEnabled() {
		t.Error("TimestampEnabled() = false, want true")
	}
	if result.IDEnabled() {
		t.Error("IDEnabled() = true, want false")
	}
	if result.Timestamp.QueryParam != "since" {
		t.Errorf("Timestamp.QueryParam = %q, want %q", result.Timestamp.QueryParam, "since")
	}
}

func TestParseStatePersistenceConfig_IDOnly(t *testing.T) {
	config := map[string]interface{}{
		"statePersistence": map[string]interface{}{
			"id": map[string]interface{}{
				"enabled":    true,
				"field":      "data.id",
				"queryParam": "after_id",
			},
		},
	}

	result := ParseStatePersistenceConfig(config)
	if result == nil {
		t.Fatal("ParseStatePersistenceConfig returned nil")
	}

	if result.TimestampEnabled() {
		t.Error("TimestampEnabled() = true, want false")
	}
	if !result.IDEnabled() {
		t.Error("IDEnabled() = false, want true")
	}
	if result.ID.Field != "data.id" {
		t.Errorf("ID.Field = %q, want %q", result.ID.Field, "data.id")
	}
	if result.ID.QueryParam != "after_id" {
		t.Errorf("ID.QueryParam = %q, want %q", result.ID.QueryParam, "after_id")
	}
}

func TestParseStatePersistenceConfig_Both(t *testing.T) {
	config := map[string]interface{}{
		"statePersistence": map[string]interface{}{
			"timestamp": map[string]interface{}{
				"enabled":    true,
				"queryParam": "modified_since",
			},
			"id": map[string]interface{}{
				"enabled":    true,
				"field":      "cursor",
				"queryParam": "cursor",
			},
			"storagePath": "/custom/path/state",
		},
	}

	result := ParseStatePersistenceConfig(config)
	if result == nil {
		t.Fatal("ParseStatePersistenceConfig returned nil")
	}

	if !result.TimestampEnabled() {
		t.Error("TimestampEnabled() = false, want true")
	}
	if !result.IDEnabled() {
		t.Error("IDEnabled() = false, want true")
	}
	if !result.IsEnabled() {
		t.Error("IsEnabled() = false, want true")
	}
	if result.StoragePath != "/custom/path/state" {
		t.Errorf("StoragePath = %q, want %q", result.StoragePath, "/custom/path/state")
	}
}

func TestStatePersistenceConfig_IsEnabled(t *testing.T) {
	tests := []struct {
		name   string
		config *StatePersistenceConfig
		want   bool
	}{
		{"nil config", nil, false},
		{"empty config", &StatePersistenceConfig{}, false},
		{"timestamp enabled", &StatePersistenceConfig{Timestamp: &TimestampConfig{Enabled: true}}, true},
		{"timestamp disabled", &StatePersistenceConfig{Timestamp: &TimestampConfig{Enabled: false}}, false},
		{"id enabled", &StatePersistenceConfig{ID: &IDConfig{Enabled: true}}, true},
		{"id disabled", &StatePersistenceConfig{ID: &IDConfig{Enabled: false}}, false},
		{"both enabled", &StatePersistenceConfig{
			Timestamp: &TimestampConfig{Enabled: true},
			ID:        &IDConfig{Enabled: true},
		}, true},
		{"both disabled", &StatePersistenceConfig{
			Timestamp: &TimestampConfig{Enabled: false},
			ID:        &IDConfig{Enabled: false},
		}, false},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got := tc.config.IsEnabled()
			if got != tc.want {
				t.Errorf("IsEnabled() = %v, want %v", got, tc.want)
			}
		})
	}
}
