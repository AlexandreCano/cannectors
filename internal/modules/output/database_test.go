package output

import (
	"testing"

	"github.com/canectors/runtime/pkg/connector"
)

func TestParseDatabaseOutputConfig(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name    string
		cfg     map[string]interface{}
		wantErr bool
		check   func(t *testing.T, config DatabaseOutputConfig)
	}{
		{
			name: "basic query config",
			cfg: map[string]interface{}{
				"connectionString": "postgres://localhost/db",
				"query":            "INSERT INTO users (name, email) VALUES ({{record.name}}, {{record.email}})",
			},
			check: func(t *testing.T, config DatabaseOutputConfig) {
				if config.ConnectionString != "postgres://localhost/db" {
					t.Errorf("ConnectionString = %q, want postgres://localhost/db", config.ConnectionString)
				}
				if config.Query == "" {
					t.Error("Query should not be empty")
				}
			},
		},
		{
			name: "config with env ref",
			cfg: map[string]interface{}{
				"connectionStringRef": "${DATABASE_URL}",
				"driver":              "mysql",
				"query":               "INSERT INTO orders (id, total) VALUES ({{record.id}}, {{record.total}})",
			},
			check: func(t *testing.T, config DatabaseOutputConfig) {
				if config.ConnectionStringRef != "${DATABASE_URL}" {
					t.Errorf("ConnectionStringRef = %q, want ${DATABASE_URL}", config.ConnectionStringRef)
				}
				if config.Driver != "mysql" {
					t.Errorf("Driver = %q, want mysql", config.Driver)
				}
			},
		},
		{
			name: "config with queryFile",
			cfg: map[string]interface{}{
				"connectionString": "postgres://localhost/db",
				"queryFile":        "/path/to/insert.sql",
			},
			check: func(t *testing.T, config DatabaseOutputConfig) {
				if config.QueryFile != "/path/to/insert.sql" {
					t.Errorf("QueryFile = %q, want /path/to/insert.sql", config.QueryFile)
				}
			},
		},
		{
			name: "transaction config",
			cfg: map[string]interface{}{
				"connectionString": "postgres://localhost/db",
				"query":            "INSERT INTO events (data) VALUES ({{record.data}})",
				"transaction":      true,
			},
			check: func(t *testing.T, config DatabaseOutputConfig) {
				if !config.Transaction {
					t.Error("Transaction should be true")
				}
			},
		},
		{
			name: "pool and timeout config",
			cfg: map[string]interface{}{
				"connectionString":       "postgres://localhost/db",
				"query":                  "INSERT INTO data (value) VALUES ({{record.value}})",
				"maxOpenConns":           float64(30),
				"maxIdleConns":           float64(15),
				"connMaxLifetimeSeconds": float64(1800),
				"connMaxIdleTimeSeconds": float64(300),
				"timeoutMs":              float64(60000),
			},
			check: func(t *testing.T, config DatabaseOutputConfig) {
				if config.MaxOpenConns != 30 {
					t.Errorf("MaxOpenConns = %d, want 30", config.MaxOpenConns)
				}
				if config.MaxIdleConns != 15 {
					t.Errorf("MaxIdleConns = %d, want 15", config.MaxIdleConns)
				}
				if config.ConnMaxLifetime != 1800 {
					t.Errorf("ConnMaxLifetime = %d, want 1800", config.ConnMaxLifetime)
				}
				if config.ConnMaxIdleTime != 300 {
					t.Errorf("ConnMaxIdleTime = %d, want 300", config.ConnMaxIdleTime)
				}
				if config.TimeoutMs != 60000 {
					t.Errorf("TimeoutMs = %d, want 60000", config.TimeoutMs)
				}
			},
		},
		{
			name: "error handling config",
			cfg: map[string]interface{}{
				"connectionString": "postgres://localhost/db",
				"query":            "INSERT INTO data (value) VALUES ({{record.value}})",
				"onError":          "skip",
			},
			check: func(t *testing.T, config DatabaseOutputConfig) {
				if config.OnError != "skip" {
					t.Errorf("OnError = %q, want skip", config.OnError)
				}
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			config := parseDatabaseOutputConfig(tt.cfg)
			if tt.wantErr {
				t.Error("parseDatabaseOutputConfig() should not return error, but test expects error")
				return
			}
			if tt.check != nil {
				tt.check(t, config)
			}
		})
	}
}

func TestNewDatabaseOutputFromConfig_Validation(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name    string
		cfg     *connector.ModuleConfig
		wantErr error
	}{
		{
			name:    "nil config",
			cfg:     nil,
			wantErr: ErrDatabaseOutputNilConfig,
		},
		{
			name: "missing connection string",
			cfg: &connector.ModuleConfig{
				Type: "database",
				Config: map[string]interface{}{
					"query": "INSERT INTO users (name) VALUES ({{record.name}})",
				},
			},
			wantErr: ErrDatabaseOutputMissingConnStr,
		},
		{
			name: "missing query",
			cfg: &connector.ModuleConfig{
				Type: "database",
				Config: map[string]interface{}{
					"connectionString": "postgres://localhost/db",
				},
			},
			wantErr: ErrDatabaseOutputMissingQuery,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := NewDatabaseOutputFromConfig(tt.cfg)
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

func TestGetDBFieldValue(t *testing.T) {
	t.Parallel()

	record := map[string]interface{}{
		"id":   1,
		"name": "test",
		"data": map[string]interface{}{
			"nested": "value",
			"deep": map[string]interface{}{
				"key": 42,
			},
		},
	}

	tests := []struct {
		name  string
		field string
		want  interface{}
	}{
		{
			name:  "top-level int",
			field: "id",
			want:  1,
		},
		{
			name:  "top-level string",
			field: "name",
			want:  "test",
		},
		{
			name:  "nested field",
			field: "data.nested",
			want:  "value",
		},
		{
			name:  "deeply nested field",
			field: "data.deep.key",
			want:  42,
		},
		{
			name:  "non-existent",
			field: "nonexistent",
			want:  nil,
		},
		{
			name:  "non-existent nested",
			field: "data.nonexistent",
			want:  nil,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := getDBFieldValue(record, tt.field)
			if got != tt.want {
				t.Errorf("getDBFieldValue(%q) = %v, want %v", tt.field, got, tt.want)
			}
		})
	}
}

func TestDefaultValues(t *testing.T) {
	t.Parallel()

	// batchSize removed - not used in implementation
}
