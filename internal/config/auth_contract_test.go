package config_test

import (
	"encoding/json"
	"fmt"
	"testing"

	"github.com/cannectors/runtime/internal/config"
)

// pipelineWithAuth wraps a JSON auth object into a minimal valid pipeline so the
// schema can validate the auth subtree.
func pipelineWithAuth(authJSON string) string {
	return fmt.Sprintf(
		`{"name":"x","version":"1.0.0","input":{"type":"httpPolling","endpoint":"https://e.com","schedule":"* * * * * *","method":"GET","authentication":%s},"output":{"type":"httpRequest","endpoint":"https://e.com","method":"POST"}}`,
		authJSON,
	)
}

// TestSchemaAcceptsAPIKeyLocations locks Story 24.5 AC1-3: api-key location is
// strict and limited to header|query (header is the documented default).
func TestSchemaAcceptsAPIKeyLocations(t *testing.T) {
	cases := []struct {
		location  string
		wantValid bool
	}{
		{"header", true},
		{"query", true},
		{"body", false},
		{"cookie", false},
		{"", false},
	}
	for _, tc := range cases {
		t.Run(tc.location, func(t *testing.T) {
			authJSON := fmt.Sprintf(
				`{"type":"api-key","credentials":{"key":"k","location":%q}}`,
				tc.location,
			)
			var data map[string]any
			if err := json.Unmarshal([]byte(pipelineWithAuth(authJSON)), &data); err != nil {
				t.Fatalf("unmarshal: %v", err)
			}
			res := config.ValidateConfig(data)
			if res.Valid != tc.wantValid {
				t.Errorf("schema validation for location=%q: got valid=%v, want %v (errors: %v)",
					tc.location, res.Valid, tc.wantValid, res.Errors)
			}
		})
	}
}

// TestSchemaRejectsUnknownAPIKeyField locks Story 24.5 AC7: additionalProperties
// is false on credentialsApiKey.
func TestSchemaRejectsUnknownAPIKeyField(t *testing.T) {
	authJSON := `{"type":"api-key","credentials":{"key":"k","location":"header","unknown":"x"}}`
	var data map[string]any
	if err := json.Unmarshal([]byte(pipelineWithAuth(authJSON)), &data); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	res := config.ValidateConfig(data)
	if res.Valid {
		t.Errorf("expected schema to reject unknown api-key credentials field, got valid=true")
	}
}

// TestSchemaAcceptsOAuth2ScopeString locks Story 24.5 AC5/AC6: scope is a
// string (RFC 6749 §3.3); the legacy `scopes` array must be rejected.
func TestSchemaAcceptsOAuth2ScopeString(t *testing.T) {
	t.Run("scope string accepted", func(t *testing.T) {
		authJSON := `{"type":"oauth2","credentials":{"tokenUrl":"https://id.example/token","clientId":"c","clientSecret":"s","scope":"read write"}}`
		var data map[string]any
		if err := json.Unmarshal([]byte(pipelineWithAuth(authJSON)), &data); err != nil {
			t.Fatalf("unmarshal: %v", err)
		}
		res := config.ValidateConfig(data)
		if !res.Valid {
			t.Errorf("expected valid, got errors: %v", res.Errors)
		}
	})

	t.Run("scopes array rejected", func(t *testing.T) {
		authJSON := `{"type":"oauth2","credentials":{"tokenUrl":"https://id.example/token","clientId":"c","clientSecret":"s","scopes":["read","write"]}}`
		var data map[string]any
		if err := json.Unmarshal([]byte(pipelineWithAuth(authJSON)), &data); err != nil {
			t.Fatalf("unmarshal: %v", err)
		}
		res := config.ValidateConfig(data)
		if res.Valid {
			t.Errorf("expected schema to reject legacy `scopes` array on oauth2 credentials, got valid=true")
		}
	})
}
