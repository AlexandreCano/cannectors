package config_test

import (
	"encoding/json"
	"fmt"
	"testing"

	"github.com/cannectors/runtime/internal/config"
)

// TestSchemaAcceptsOnErrorMixedCase locks in Story 24.2 AC9: the JSON schema
// accepts onError in any casing (the runtime normalizes downstream).
func TestSchemaAcceptsOnErrorMixedCase(t *testing.T) {
	cases := []struct {
		value     string
		wantValid bool
	}{
		{"fail", true},
		{"FAIL", true},
		{"Skip", true},
		{"sKiP", true},
		{"LOG", true},
		{"invalid", false},
		{"continue", false},
	}
	for _, tc := range cases {
		t.Run(tc.value, func(t *testing.T) {
			raw := fmt.Sprintf(`{"name":"x","version":"1.0.0","input":{"type":"httpPolling","endpoint":"https://e.com","schedule":"* * * * * *","onError":"%s"},"output":{"type":"httpRequest","endpoint":"https://e.com","method":"POST"}}`, tc.value)
			var data map[string]any
			if err := json.Unmarshal([]byte(raw), &data); err != nil {
				t.Fatalf("unmarshal: %v", err)
			}
			res := config.ValidateConfig(data)
			if res.Valid != tc.wantValid {
				t.Errorf("schema validation for onError=%q: got valid=%v, want %v (errors: %v)", tc.value, res.Valid, tc.wantValid, res.Errors)
			}
		})
	}
}
