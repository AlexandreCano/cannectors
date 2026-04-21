package input

import (
	"encoding/json"
	"testing"

	"github.com/cannectors/runtime/internal/moduleconfig"
	"github.com/cannectors/runtime/pkg/connector"
)

// parseDatabaseInputConfigFromMap is a test helper that converts a map to DatabaseInputConfig via JSON.

func mustJSON(v interface{}) json.RawMessage {
	b, err := json.Marshal(v)
	if err != nil {
		panic(err)
	}
	return b
}

func parseDatabaseInputConfigFromMap(cfg map[string]interface{}) DatabaseInputConfig {
	data, _ := json.Marshal(cfg)
	var config DatabaseInputConfig
	_ = json.Unmarshal(data, &config)
	return config
}

func TestParseDatabaseInputConfig(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name    string
		cfg     map[string]interface{}
		wantErr bool
		check   func(t *testing.T, config DatabaseInputConfig)
	}{
		{
			name: "basic config",
			cfg: map[string]interface{}{
				"connectionString": "postgres://user:pass@localhost:5432/db",
				"query":            "SELECT * FROM users",
			},
			check: func(t *testing.T, config DatabaseInputConfig) {
				if config.ConnectionString != "postgres://user:pass@localhost:5432/db" {
					t.Errorf("ConnectionString = %q, want postgres://...", config.ConnectionString)
				}
				if config.Query != "SELECT * FROM users" {
					t.Errorf("Query = %q, want SELECT * FROM users", config.Query)
				}
			},
		},
		{
			name: "config with env ref",
			cfg: map[string]interface{}{
				"connectionStringRef": "${DATABASE_URL}",
				"query":               "SELECT * FROM orders",
				"driver":              "postgres",
			},
			check: func(t *testing.T, config DatabaseInputConfig) {
				if config.ConnectionStringRef != "${DATABASE_URL}" {
					t.Errorf("ConnectionStringRef = %q, want ${DATABASE_URL}", config.ConnectionStringRef)
				}
				if config.Driver != "postgres" {
					t.Errorf("Driver = %q, want postgres", config.Driver)
				}
			},
		},
		{
			name: "config with pagination",
			cfg: map[string]interface{}{
				"connectionString": "postgres://localhost/db",
				"query":            "SELECT * FROM items",
				"pagination": map[string]interface{}{
					"type":        "limit-offset",
					"limit":       float64(500),
					"offsetParam": "offset",
				},
			},
			check: func(t *testing.T, config DatabaseInputConfig) {
				if config.Pagination == nil {
					t.Fatal("Pagination should not be nil")
				}
				if config.Pagination.Type != "limit-offset" {
					t.Errorf("Pagination.Type = %q, want limit-offset", config.Pagination.Type)
				}
				if config.Pagination.Limit != 500 {
					t.Errorf("Pagination.Limit = %d, want 500", config.Pagination.Limit)
				}
			},
		},
		{
			name: "config with incremental",
			cfg: map[string]interface{}{
				"connectionString": "postgres://localhost/db",
				"query":            "SELECT * FROM events WHERE created_at > :since",
				"incremental": map[string]interface{}{
					"enabled":        true,
					"timestampField": "created_at",
					"timestampParam": "since",
				},
			},
			check: func(t *testing.T, config DatabaseInputConfig) {
				if config.Incremental == nil {
					t.Fatal("Incremental should not be nil")
				}
				if !config.Incremental.Enabled {
					t.Error("Incremental.Enabled should be true")
				}
				if config.Incremental.TimestampField != "created_at" {
					t.Errorf("Incremental.TimestampField = %q, want created_at", config.Incremental.TimestampField)
				}
				if config.Incremental.TimestampParam != "since" {
					t.Errorf("Incremental.TimestampParam = %q, want since", config.Incremental.TimestampParam)
				}
			},
		},
		{
			name: "config with pool settings",
			cfg: map[string]interface{}{
				"connectionString":       "postgres://localhost/db",
				"query":                  "SELECT 1",
				"maxOpenConns":           float64(20),
				"maxIdleConns":           float64(10),
				"connMaxLifetimeSeconds": float64(3600),
				"connMaxIdleTimeSeconds": float64(600),
				"timeoutMs":              float64(60000),
			},
			check: func(t *testing.T, config DatabaseInputConfig) {
				if config.MaxOpenConns != 20 {
					t.Errorf("MaxOpenConns = %d, want 20", config.MaxOpenConns)
				}
				if config.MaxIdleConns != 10 {
					t.Errorf("MaxIdleConns = %d, want 10", config.MaxIdleConns)
				}
				if config.ConnMaxLifetimeSeconds != 3600 {
					t.Errorf("ConnMaxLifetime = %d, want 3600", config.ConnMaxLifetimeSeconds)
				}
				if config.ConnMaxIdleTimeSeconds != 600 {
					t.Errorf("ConnMaxIdleTime = %d, want 600", config.ConnMaxIdleTimeSeconds)
				}
				if config.TimeoutMs != 60000 {
					t.Errorf("TimeoutMs = %d, want 60000", config.TimeoutMs)
				}
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			config := parseDatabaseInputConfigFromMap(tt.cfg)
			if tt.wantErr {
				t.Error("parseDatabaseInputConfig() should not return error, but test expects error")
				return
			}
			if tt.check != nil {
				tt.check(t, config)
			}
		})
	}
}

func TestParseDatabasePaginationConfig(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name  string
		cfg   map[string]interface{}
		check func(t *testing.T, config *moduleconfig.DatabasePaginationConfig)
	}{
		{
			name: "limit-offset pagination",
			cfg: map[string]interface{}{
				"type":        "limit-offset",
				"limit":       float64(100),
				"offsetParam": "offset",
			},
			check: func(t *testing.T, config *moduleconfig.DatabasePaginationConfig) {
				if config.Type != "limit-offset" {
					t.Errorf("Type = %q, want limit-offset", config.Type)
				}
				if config.Limit != 100 {
					t.Errorf("Limit = %d, want 100", config.Limit)
				}
				if config.OffsetParam != "offset" {
					t.Errorf("OffsetParam = %q, want offset", config.OffsetParam)
				}
			},
		},
		{
			name: "cursor pagination",
			cfg: map[string]interface{}{
				"type":        "cursor",
				"limit":       float64(50),
				"cursorField": "id",
				"cursorParam": "after_id",
			},
			check: func(t *testing.T, config *moduleconfig.DatabasePaginationConfig) {
				if config.Type != "cursor" {
					t.Errorf("Type = %q, want cursor", config.Type)
				}
				if config.CursorField != "id" {
					t.Errorf("CursorField = %q, want id", config.CursorField)
				}
				if config.CursorParam != "after_id" {
					t.Errorf("CursorParam = %q, want after_id", config.CursorParam)
				}
			},
		},
		{
			name: "default limit",
			cfg: map[string]interface{}{
				"type": "limit-offset",
			},
			check: func(t *testing.T, config *moduleconfig.DatabasePaginationConfig) {
				// Limit defaults are applied at usage time (not parse time), so 0 is expected
				if config.Limit != 0 {
					t.Errorf("Limit = %d, want 0 (default applied at usage)", config.Limit)
				}
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			data, _ := json.Marshal(tt.cfg)
			config := &moduleconfig.DatabasePaginationConfig{}
			_ = json.Unmarshal(data, config)
			if tt.check != nil {
				tt.check(t, config)
			}
		})
	}
}

func TestParseIncrementalConfig(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name  string
		cfg   map[string]interface{}
		check func(t *testing.T, config *IncrementalConfig)
	}{
		{
			name: "timestamp-based incremental",
			cfg: map[string]interface{}{
				"enabled":        true,
				"timestampField": "updated_at",
				"timestampParam": "since",
			},
			check: func(t *testing.T, config *IncrementalConfig) {
				if !config.Enabled {
					t.Error("Enabled should be true")
				}
				if config.TimestampField != "updated_at" {
					t.Errorf("TimestampField = %q, want updated_at", config.TimestampField)
				}
				if config.TimestampParam != "since" {
					t.Errorf("TimestampParam = %q, want since", config.TimestampParam)
				}
			},
		},
		{
			name: "id-based incremental",
			cfg: map[string]interface{}{
				"enabled": true,
				"idField": "id",
				"idParam": "after_id",
			},
			check: func(t *testing.T, config *IncrementalConfig) {
				if !config.Enabled {
					t.Error("Enabled should be true")
				}
				if config.IDField != "id" {
					t.Errorf("IDField = %q, want id", config.IDField)
				}
				if config.IDParam != "after_id" {
					t.Errorf("IDParam = %q, want after_id", config.IDParam)
				}
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			data, _ := json.Marshal(tt.cfg)
			config := &IncrementalConfig{}
			_ = json.Unmarshal(data, config)
			if tt.check != nil {
				tt.check(t, config)
			}
		})
	}
}

func TestNewDatabaseInputFromConfig_Validation(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name    string
		cfg     *connector.ModuleConfig
		wantErr error
	}{
		{
			name:    "nil config",
			cfg:     nil,
			wantErr: ErrDatabaseNilConfig,
		},
		{
			name: "missing query",
			cfg: &connector.ModuleConfig{
				Type: "database",
				Raw: mustJSON(map[string]interface{}{
					"connectionString": "postgres://localhost/db",
				}),
			},
			wantErr: ErrDatabaseMissingQuery,
		},
		{
			name: "missing connection string",
			cfg: &connector.ModuleConfig{
				Type: "database",
				Raw: mustJSON(map[string]interface{}{
					"query": "SELECT * FROM users",
				}),
			},
			wantErr: ErrDatabaseMissingConnStr,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := NewDatabaseInputFromConfig(tt.cfg)
			if err == nil {
				t.Error("expected error, got nil")
				return
			}
			if err != tt.wantErr {
				t.Errorf("error = %v, want %v", err, tt.wantErr)
			}
		})
	}
}
