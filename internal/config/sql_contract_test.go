package config_test

import (
	"encoding/json"
	"fmt"
	"testing"

	"github.com/cannectors/runtime/internal/config"
)

// validateSQLPipeline assembles a minimal pipeline JSON around an input/filter/output
// fragment and runs schema validation. It returns the validation result.
func validateSQLPipeline(t *testing.T, raw string) *config.ValidationResult {
	t.Helper()
	var data map[string]any
	if err := json.Unmarshal([]byte(raw), &data); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	return config.ValidateConfig(data)
}

// TestSchemaRejectsBothConnectionFields locks Story 24.4 AC1: connectionString
// and connectionStringRef cannot both be provided.
func TestSchemaRejectsBothConnectionFields(t *testing.T) {
	raw := `{"name":"x","version":"1.0.0",
		"input":{"type":"database","connectionString":"sqlite::memory:","connectionStringRef":"${DB_URL}","query":"SELECT 1"},
		"output":{"type":"httpRequest","endpoint":"https://e.com","method":"POST"}}`
	res := validateSQLPipeline(t, raw)
	if res.Valid {
		t.Fatalf("expected schema to reject both connection fields, got valid")
	}
}

// TestSchemaRejectsMissingConnection locks Story 24.4 AC2.
func TestSchemaRejectsMissingConnection(t *testing.T) {
	raw := `{"name":"x","version":"1.0.0",
		"input":{"type":"database","query":"SELECT 1"},
		"output":{"type":"httpRequest","endpoint":"https://e.com","method":"POST"}}`
	res := validateSQLPipeline(t, raw)
	if res.Valid {
		t.Fatalf("expected schema to reject missing connection, got valid")
	}
}

// TestSchemaRejectsBothQueryFields locks Story 24.4 AC3.
func TestSchemaRejectsBothQueryFields(t *testing.T) {
	raw := `{"name":"x","version":"1.0.0",
		"input":{"type":"database","connectionString":"sqlite::memory:","query":"SELECT 1","queryFile":"/tmp/q.sql"},
		"output":{"type":"httpRequest","endpoint":"https://e.com","method":"POST"}}`
	res := validateSQLPipeline(t, raw)
	if res.Valid {
		t.Fatalf("expected schema to reject both query fields, got valid")
	}
}

// TestSchemaRejectsInvalidConnectionStringRef locks Story 24.4 AC4.
func TestSchemaRejectsInvalidConnectionStringRef(t *testing.T) {
	cases := []struct {
		ref       string
		wantValid bool
	}{
		{"${DB_URL}", true},
		{"${POSTGRES_CONN_STR}", true},
		{"DATABASE_URL", false},
		{"$DB_URL", false},
		{"${db_url}", false},
		{"${1ABC}", false},
	}
	for _, tc := range cases {
		t.Run(tc.ref, func(t *testing.T) {
			raw := fmt.Sprintf(`{"name":"x","version":"1.0.0",
				"input":{"type":"database","connectionStringRef":%q,"query":"SELECT 1"},
				"output":{"type":"httpRequest","endpoint":"https://e.com","method":"POST"}}`, tc.ref)
			res := validateSQLPipeline(t, raw)
			if res.Valid != tc.wantValid {
				t.Errorf("connectionStringRef=%q: got valid=%v, want %v (errors=%v)",
					tc.ref, res.Valid, tc.wantValid, res.Errors)
			}
		})
	}
}

// TestSchemaRejectsZeroNumericFields locks Story 24.4 AC5: when a numeric field
// is provided on a database connection or pagination, 0 is rejected.
func TestSchemaRejectsZeroNumericFields(t *testing.T) {
	cases := []struct {
		name  string
		field string
	}{
		{"timeoutMs", `"timeoutMs":0`},
		{"maxOpenConns", `"maxOpenConns":0`},
		{"maxIdleConns", `"maxIdleConns":0`},
		{"connMaxLifetimeSeconds", `"connMaxLifetimeSeconds":0`},
		{"connMaxIdleTimeSeconds", `"connMaxIdleTimeSeconds":0`},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			raw := fmt.Sprintf(`{"name":"x","version":"1.0.0",
				"input":{"type":"database","connectionString":"sqlite::memory:","query":"SELECT 1",%s},
				"output":{"type":"httpRequest","endpoint":"https://e.com","method":"POST"}}`, tc.field)
			res := validateSQLPipeline(t, raw)
			if res.Valid {
				t.Errorf("expected schema to reject %s=0", tc.name)
			}
		})
	}
}

// TestSchemaRejectsZeroPaginationLimit locks Story 24.4 AC15.
func TestSchemaRejectsZeroPaginationLimit(t *testing.T) {
	raw := `{"name":"x","version":"1.0.0",
		"input":{"type":"database","connectionString":"sqlite::memory:","query":"SELECT 1","pagination":{"type":"limit-offset","param":"offset","limit":0}},
		"output":{"type":"httpRequest","endpoint":"https://e.com","method":"POST"}}`
	res := validateSQLPipeline(t, raw)
	if res.Valid {
		t.Fatalf("expected schema to reject pagination.limit=0")
	}
}

// TestSchemaAcceptsPaginationWithoutLimit locks Story 24.4 AC14: limit is
// optional; runtime applies its default.
func TestSchemaAcceptsPaginationWithoutLimit(t *testing.T) {
	raw := `{"name":"x","version":"1.0.0",
		"input":{"type":"database","connectionString":"sqlite::memory:","query":"SELECT 1","pagination":{"type":"limit-offset","param":"offset"}},
		"output":{"type":"httpRequest","endpoint":"https://e.com","method":"POST"}}`
	res := validateSQLPipeline(t, raw)
	if !res.Valid {
		t.Fatalf("expected schema to accept pagination without limit, got errors=%v", res.Errors)
	}
}

// TestSchemaPaginationCanonicalParam locks Story 24.4 AC13: 'param' replaces
// the legacy cursorParam/offsetParam fields.
func TestSchemaPaginationCanonicalParam(t *testing.T) {
	cases := []struct {
		name      string
		body      string
		wantValid bool
	}{
		{"limit-offset with param", `{"type":"limit-offset","param":"offset"}`, true},
		{"cursor with param + cursorField", `{"type":"cursor","param":"after","cursorField":"id"}`, true},
		{"limit-offset with legacy offsetParam", `{"type":"limit-offset","offsetParam":"offset"}`, false},
		{"cursor with legacy cursorParam", `{"type":"cursor","cursorParam":"after","cursorField":"id"}`, false},
		{"cursor missing cursorField", `{"type":"cursor","param":"after"}`, false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			raw := fmt.Sprintf(`{"name":"x","version":"1.0.0",
				"input":{"type":"database","connectionString":"sqlite::memory:","query":"SELECT 1","pagination":%s},
				"output":{"type":"httpRequest","endpoint":"https://e.com","method":"POST"}}`, tc.body)
			res := validateSQLPipeline(t, raw)
			if res.Valid != tc.wantValid {
				t.Errorf("pagination=%s: got valid=%v, want %v (errors=%v)",
					tc.body, res.Valid, tc.wantValid, res.Errors)
			}
		})
	}
}

// TestSchemaDatabaseScheduleOptional locks Story 24.4 AC11/AC12: the database
// input schedule is optional.
func TestSchemaDatabaseScheduleOptional(t *testing.T) {
	cases := []struct {
		name      string
		fragment  string
		wantValid bool
	}{
		{"no schedule", ``, true},
		{"with schedule", `,"schedule":"* * * * * *"`, true},
		{"too short schedule", `,"schedule":"x"`, false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			raw := fmt.Sprintf(`{"name":"x","version":"1.0.0",
				"input":{"type":"database","connectionString":"sqlite::memory:","query":"SELECT 1"%s},
				"output":{"type":"httpRequest","endpoint":"https://e.com","method":"POST"}}`, tc.fragment)
			res := validateSQLPipeline(t, raw)
			if res.Valid != tc.wantValid {
				t.Errorf("fragment=%q: got valid=%v, want %v (errors=%v)",
					tc.fragment, res.Valid, tc.wantValid, res.Errors)
			}
		})
	}
}

// TestSchemaSQLCallAppendRequiresResultKey locks Story 24.4 AC9.
func TestSchemaSQLCallAppendRequiresResultKey(t *testing.T) {
	cases := []struct {
		name      string
		body      string
		wantValid bool
	}{
		{
			name:      "append without resultKey",
			body:      `{"type":"sql_call","connectionString":"sqlite::memory:","query":"SELECT 1","mergeStrategy":"append"}`,
			wantValid: false,
		},
		{
			name:      "append with resultKey",
			body:      `{"type":"sql_call","connectionString":"sqlite::memory:","query":"SELECT 1","mergeStrategy":"append","resultKey":"row"}`,
			wantValid: true,
		},
		{
			name:      "merge without resultKey",
			body:      `{"type":"sql_call","connectionString":"sqlite::memory:","query":"SELECT 1","mergeStrategy":"merge"}`,
			wantValid: true,
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			raw := fmt.Sprintf(`{"name":"x","version":"1.0.0",
				"input":{"type":"httpPolling","endpoint":"https://e.com","schedule":"* * * * * *"},
				"filters":[%s],
				"output":{"type":"httpRequest","endpoint":"https://e.com","method":"POST"}}`, tc.body)
			res := validateSQLPipeline(t, raw)
			if res.Valid != tc.wantValid {
				t.Errorf("body=%s: got valid=%v, want %v (errors=%v)",
					tc.body, res.Valid, tc.wantValid, res.Errors)
			}
		})
	}
}

// TestSchemaSQLCallRejectsRetry locks Story 24.4 AC17.
func TestSchemaSQLCallRejectsRetry(t *testing.T) {
	raw := `{"name":"x","version":"1.0.0",
		"input":{"type":"httpPolling","endpoint":"https://e.com","schedule":"* * * * * *"},
		"filters":[{"type":"sql_call","connectionString":"sqlite::memory:","query":"SELECT 1","retry":{"maxAttempts":2}}],
		"output":{"type":"httpRequest","endpoint":"https://e.com","method":"POST"}}`
	res := validateSQLPipeline(t, raw)
	if res.Valid {
		t.Fatalf("expected schema to reject retry on sql_call")
	}
}

// TestSchemaSQLCallCacheTTLSeconds locks Story 24.4 cache rename: only
// 'ttlSeconds' is accepted; 'defaultTTL' is rejected.
func TestSchemaSQLCallCacheTTLSeconds(t *testing.T) {
	cases := []struct {
		name      string
		cache     string
		wantValid bool
	}{
		{"ttlSeconds", `{"enabled":true,"ttlSeconds":60}`, true},
		{"legacy defaultTTL", `{"enabled":true,"defaultTTL":60}`, false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			raw := fmt.Sprintf(`{"name":"x","version":"1.0.0",
				"input":{"type":"httpPolling","endpoint":"https://e.com","schedule":"* * * * * *"},
				"filters":[{"type":"sql_call","connectionString":"sqlite::memory:","query":"SELECT 1","cache":%s}],
				"output":{"type":"httpRequest","endpoint":"https://e.com","method":"POST"}}`, tc.cache)
			res := validateSQLPipeline(t, raw)
			if res.Valid != tc.wantValid {
				t.Errorf("cache=%s: got valid=%v, want %v (errors=%v)",
					tc.cache, res.Valid, tc.wantValid, res.Errors)
			}
		})
	}
}

// TestSchemaRejectsInvalidMergeStrategy locks the strict mergeStrategy enum on sql_call.
func TestSchemaRejectsInvalidMergeStrategy(t *testing.T) {
	raw := `{"name":"x","version":"1.0.0",
		"input":{"type":"httpPolling","endpoint":"https://e.com","schedule":"* * * * * *"},
		"filters":[{"type":"sql_call","connectionString":"sqlite::memory:","query":"SELECT 1","mergeStrategy":"unknown"}],
		"output":{"type":"httpRequest","endpoint":"https://e.com","method":"POST"}}`
	res := validateSQLPipeline(t, raw)
	if res.Valid {
		t.Fatalf("expected schema to reject unknown mergeStrategy")
	}
}

// TestSchemaRejectsEmptyConnectionString locks the schema-side minLength on
// connectionString so the schema and runtime agree on what counts as "absent".
func TestSchemaRejectsEmptyConnectionString(t *testing.T) {
	raw := `{"name":"x","version":"1.0.0",
		"input":{"type":"database","connectionString":"","query":"SELECT 1"},
		"output":{"type":"httpRequest","endpoint":"https://e.com","method":"POST"}}`
	res := validateSQLPipeline(t, raw)
	if res.Valid {
		t.Fatalf("expected schema to reject empty connectionString")
	}
}

// TestSchemaQueryMinLength locks Story 24.4 Task 1: query/queryFile must be non-empty.
func TestSchemaQueryMinLength(t *testing.T) {
	raw := `{"name":"x","version":"1.0.0",
		"input":{"type":"database","connectionString":"sqlite::memory:","query":""},
		"output":{"type":"httpRequest","endpoint":"https://e.com","method":"POST"}}`
	res := validateSQLPipeline(t, raw)
	if res.Valid {
		t.Fatalf("expected schema to reject empty query")
	}
}
