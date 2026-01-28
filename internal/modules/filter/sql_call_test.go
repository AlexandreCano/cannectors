package filter

import (
	"testing"
)

func TestParseSQLCallConfig(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name    string
		cfg     map[string]interface{}
		wantErr bool
		check   func(t *testing.T, config SQLCallConfig)
	}{
		{
			name: "basic config with single query",
			cfg: map[string]interface{}{
				"connectionString": "postgres://localhost/db",
				"query":            "SELECT name FROM users WHERE id = {{record.user_id}}",
			},
			check: func(t *testing.T, config SQLCallConfig) {
				if config.ConnectionString != "postgres://localhost/db" {
					t.Errorf("ConnectionString = %q, want postgres://localhost/db", config.ConnectionString)
				}
				if config.Query != "SELECT name FROM users WHERE id = {{record.user_id}}" {
					t.Errorf("Query mismatch")
				}
			},
		},
		{
			name: "config with env ref",
			cfg: map[string]interface{}{
				"connectionStringRef": "${DATABASE_URL}",
				"query":               "SELECT 1",
				"driver":              "mysql",
			},
			check: func(t *testing.T, config SQLCallConfig) {
				if config.ConnectionStringRef != "${DATABASE_URL}" {
					t.Errorf("ConnectionStringRef = %q, want ${DATABASE_URL}", config.ConnectionStringRef)
				}
				if config.Driver != "mysql" {
					t.Errorf("Driver = %q, want mysql", config.Driver)
				}
			},
		},
		{
			name: "config with multiple queries",
			cfg: map[string]interface{}{
				"connectionString": "postgres://localhost/db",
				"queries": []interface{}{
					"SELECT a FROM t1 WHERE id = {{record.id}}",
					"SELECT b FROM t2 WHERE a_id = {{record.a}}",
				},
			},
			check: func(t *testing.T, config SQLCallConfig) {
				if len(config.Queries) != 2 {
					t.Errorf("Queries length = %d, want 2", len(config.Queries))
				}
			},
		},
		{
			name: "config with condition",
			cfg: map[string]interface{}{
				"connectionString": "postgres://localhost/db",
				"query":            "SELECT * FROM enrichment WHERE key = {{record.key}}",
			},
			check: func(t *testing.T, config SQLCallConfig) {
				// Condition support removed - use condition module instead
			},
		},
		{
			name: "config with merge strategy",
			cfg: map[string]interface{}{
				"connectionString": "postgres://localhost/db",
				"query":            "SELECT * FROM t WHERE id = {{record.id}}",
				"mergeStrategy":    "append",
				"resultKey":        "_db_data",
			},
			check: func(t *testing.T, config SQLCallConfig) {
				if config.MergeStrategy != "append" {
					t.Errorf("MergeStrategy = %q, want append", config.MergeStrategy)
				}
				if config.ResultKey != "_db_data" {
					t.Errorf("ResultKey = %q, want _db_data", config.ResultKey)
				}
			},
		},
		{
			name: "config with cache",
			cfg: map[string]interface{}{
				"connectionString": "postgres://localhost/db",
				"query":            "SELECT * FROM t WHERE id = {{record.id}}",
				"cache": map[string]interface{}{
					"enabled":    true,
					"maxSize":    float64(500),
					"defaultTTL": float64(600),
					"key":        "{{record.cache_key}}",
				},
			},
			check: func(t *testing.T, config SQLCallConfig) {
				if !config.Cache.Enabled {
					t.Error("Cache.Enabled should be true")
				}
				if config.Cache.MaxSize != 500 {
					t.Errorf("Cache.MaxSize = %d, want 500", config.Cache.MaxSize)
				}
				if config.Cache.DefaultTTL != 600 {
					t.Errorf("Cache.DefaultTTL = %d, want 600", config.Cache.DefaultTTL)
				}
				if config.Cache.Key != "{{record.cache_key}}" {
					t.Errorf("Cache.Key = %q, want {{record.cache_key}}", config.Cache.Key)
				}
			},
		},
		{
			name: "config with pool settings",
			cfg: map[string]interface{}{
				"connectionString":       "postgres://localhost/db",
				"query":                  "SELECT 1",
				"maxOpenConns":           float64(25),
				"maxIdleConns":           float64(10),
				"connMaxLifetimeSeconds": float64(1800),
				"connMaxIdleTimeSeconds": float64(300),
				"timeoutMs":              float64(45000),
			},
			check: func(t *testing.T, config SQLCallConfig) {
				if config.MaxOpenConns != 25 {
					t.Errorf("MaxOpenConns = %d, want 25", config.MaxOpenConns)
				}
				if config.MaxIdleConns != 10 {
					t.Errorf("MaxIdleConns = %d, want 10", config.MaxIdleConns)
				}
				if config.ConnMaxLifetime != 1800 {
					t.Errorf("ConnMaxLifetime = %d, want 1800", config.ConnMaxLifetime)
				}
				if config.ConnMaxIdleTime != 300 {
					t.Errorf("ConnMaxIdleTime = %d, want 300", config.ConnMaxIdleTime)
				}
				if config.TimeoutMs != 45000 {
					t.Errorf("TimeoutMs = %d, want 45000", config.TimeoutMs)
				}
			},
		},
		{
			name: "config with error handling",
			cfg: map[string]interface{}{
				"connectionString": "postgres://localhost/db",
				"query":            "SELECT * FROM t WHERE id = {{record.id}}",
				"onError":          "skip",
			},
			check: func(t *testing.T, config SQLCallConfig) {
				if config.OnError != "skip" {
					t.Errorf("OnError = %q, want skip", config.OnError)
				}
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			config, err := ParseSQLCallConfig(tt.cfg)
			if (err != nil) != tt.wantErr {
				t.Errorf("ParseSQLCallConfig() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			if tt.check != nil {
				tt.check(t, config)
			}
		})
	}
}

func TestParseSQLCallCacheConfig(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name  string
		cfg   map[string]interface{}
		check func(t *testing.T, config SQLCallCacheConfig)
	}{
		{
			name: "full cache config",
			cfg: map[string]interface{}{
				"enabled":    true,
				"maxSize":    float64(1000),
				"defaultTTL": float64(300),
				"key":        "{{record.id}}",
			},
			check: func(t *testing.T, config SQLCallCacheConfig) {
				if !config.Enabled {
					t.Error("Enabled should be true")
				}
				if config.MaxSize != 1000 {
					t.Errorf("MaxSize = %d, want 1000", config.MaxSize)
				}
				if config.DefaultTTL != 300 {
					t.Errorf("DefaultTTL = %d, want 300", config.DefaultTTL)
				}
				if config.Key != "{{record.id}}" {
					t.Errorf("Key = %q, want {{record.id}}", config.Key)
				}
			},
		},
		{
			name: "disabled cache",
			cfg: map[string]interface{}{
				"enabled": false,
			},
			check: func(t *testing.T, config SQLCallCacheConfig) {
				if config.Enabled {
					t.Error("Enabled should be false")
				}
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			config := parseSQLCallCacheConfig(tt.cfg)
			if tt.check != nil {
				tt.check(t, config)
			}
		})
	}
}

func TestNewSQLCallFromConfig_Validation(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name    string
		config  SQLCallConfig
		wantErr error
	}{
		{
			name:    "missing connection string",
			config:  SQLCallConfig{Query: "SELECT 1"},
			wantErr: ErrSQLCallMissingConnection,
		},
		{
			name:    "missing query",
			config:  SQLCallConfig{ConnectionString: "postgres://localhost/db"},
			wantErr: ErrSQLCallMissingQuery,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := NewSQLCallFromConfig(tt.config)
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

func TestGetSQLNestedValue(t *testing.T) {
	t.Parallel()

	record := map[string]interface{}{
		"id":   123,
		"name": "test",
		"nested": map[string]interface{}{
			"field": "value",
			"deep": map[string]interface{}{
				"key": "deep_value",
			},
		},
	}

	tests := []struct {
		name string
		path string
		want interface{}
	}{
		{
			name: "top-level field",
			path: "id",
			want: 123,
		},
		{
			name: "top-level string",
			path: "name",
			want: "test",
		},
		{
			name: "nested field",
			path: "nested.field",
			want: "value",
		},
		{
			name: "deeply nested field",
			path: "nested.deep.key",
			want: "deep_value",
		},
		{
			name: "non-existent field",
			path: "nonexistent",
			want: nil,
		},
		{
			name: "non-existent nested field",
			path: "nested.nonexistent",
			want: nil,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := getSQLNestedValue(record, tt.path)
			if got != tt.want {
				t.Errorf("getSQLNestedValue(%q) = %v, want %v", tt.path, got, tt.want)
			}
		})
	}
}

func TestDeepMerge(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name string
		a    map[string]interface{}
		b    map[string]interface{}
		want map[string]interface{}
	}{
		{
			name: "simple merge",
			a:    map[string]interface{}{"a": 1, "b": 2},
			b:    map[string]interface{}{"c": 3},
			want: map[string]interface{}{"a": 1, "b": 2, "c": 3},
		},
		{
			name: "override value",
			a:    map[string]interface{}{"a": 1, "b": 2},
			b:    map[string]interface{}{"b": 10},
			want: map[string]interface{}{"a": 1, "b": 10},
		},
		{
			name: "nested merge",
			a: map[string]interface{}{
				"outer": map[string]interface{}{
					"a": 1,
					"b": 2,
				},
			},
			b: map[string]interface{}{
				"outer": map[string]interface{}{
					"c": 3,
				},
			},
			want: map[string]interface{}{
				"outer": map[string]interface{}{
					"a": 1,
					"b": 2,
					"c": 3,
				},
			},
		},
		{
			name: "b nil map",
			a:    map[string]interface{}{"a": 1},
			b:    map[string]interface{}{},
			want: map[string]interface{}{"a": 1},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := deepMerge(tt.a, tt.b)
			// Compare top-level keys
			for k, v := range tt.want {
				gotV, exists := got[k]
				if !exists {
					t.Errorf("key %q missing from result", k)
					continue
				}
				// For nested maps, just check they exist
				if _, ok := v.(map[string]interface{}); ok {
					if _, ok := gotV.(map[string]interface{}); !ok {
						t.Errorf("key %q should be a map", k)
					}
				} else if gotV != v {
					t.Errorf("got[%q] = %v, want %v", k, gotV, v)
				}
			}
		})
	}
}
