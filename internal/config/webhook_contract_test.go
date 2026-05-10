package config_test

import (
	"encoding/json"
	"fmt"
	"testing"

	"github.com/cannectors/runtime/internal/config"
)

func pipelineWithWebhook(inputJSON string) string {
	return fmt.Sprintf(
		`{"name":"x","version":"1.0.0","input":%s,"output":{"type":"httpRequest","endpoint":"https://e.com","method":"POST"}}`,
		inputJSON,
	)
}

// TestSchemaWebhookTimeoutField locks Story 24.7 AC2/AC3: requestTimeoutMs is
// the canonical field; legacy `timeoutMs` is rejected.
func TestSchemaWebhookTimeoutField(t *testing.T) {
	cases := []struct {
		name      string
		input     string
		wantValid bool
	}{
		{"requestTimeoutMs accepted", `{"type":"webhook","path":"/in","requestTimeoutMs":5000}`, true},
		{"timeoutMs rejected", `{"type":"webhook","path":"/in","timeoutMs":5000}`, false},
		{"requestTimeoutMs zero rejected", `{"type":"webhook","path":"/in","requestTimeoutMs":0}`, false},
		{"omitted", `{"type":"webhook","path":"/in"}`, true},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			var data map[string]any
			if err := json.Unmarshal([]byte(pipelineWithWebhook(tc.input)), &data); err != nil {
				t.Fatalf("unmarshal: %v", err)
			}
			res := config.ValidateConfig(data)
			if res.Valid != tc.wantValid {
				t.Errorf("got valid=%v want %v (errors: %v)", res.Valid, tc.wantValid, res.Errors)
			}
		})
	}
}

// TestSchemaWebhookSignature locks Story 24.7 AC4/AC5: header is optional
// (runtime defaults to X-Hub-Signature-256), secret is required.
func TestSchemaWebhookSignature(t *testing.T) {
	cases := []struct {
		name      string
		sigJSON   string
		wantValid bool
	}{
		{"type+secret only", `{"type":"hmac-sha256","secret":"s"}`, true},
		{"with header", `{"type":"hmac-sha256","secret":"s","header":"X-Foo"}`, true},
		{"missing secret", `{"type":"hmac-sha256"}`, false},
		{"missing type", `{"secret":"s"}`, false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			input := fmt.Sprintf(`{"type":"webhook","path":"/in","signature":%s}`, tc.sigJSON)
			var data map[string]any
			if err := json.Unmarshal([]byte(pipelineWithWebhook(input)), &data); err != nil {
				t.Fatalf("unmarshal: %v", err)
			}
			res := config.ValidateConfig(data)
			if res.Valid != tc.wantValid {
				t.Errorf("got valid=%v want %v (errors: %v)", res.Valid, tc.wantValid, res.Errors)
			}
		})
	}
}

// TestSchemaWebhookQueueAndConcurrency locks Story 24.7 AC6: queueSize and
// maxConcurrent must be >= 1 when provided.
func TestSchemaWebhookQueueAndConcurrency(t *testing.T) {
	cases := []struct {
		name      string
		input     string
		wantValid bool
	}{
		{"omitted", `{"type":"webhook","path":"/in"}`, true},
		{"queueSize zero", `{"type":"webhook","path":"/in","queueSize":0}`, false},
		{"queueSize positive", `{"type":"webhook","path":"/in","queueSize":10}`, true},
		{"maxConcurrent zero", `{"type":"webhook","path":"/in","maxConcurrent":0}`, false},
		{"maxConcurrent positive", `{"type":"webhook","path":"/in","maxConcurrent":4}`, true},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			var data map[string]any
			if err := json.Unmarshal([]byte(pipelineWithWebhook(tc.input)), &data); err != nil {
				t.Fatalf("unmarshal: %v", err)
			}
			res := config.ValidateConfig(data)
			if res.Valid != tc.wantValid {
				t.Errorf("got valid=%v want %v (errors: %v)", res.Valid, tc.wantValid, res.Errors)
			}
		})
	}
}

// TestSchemaWebhookRateLimit locks Story 24.7 AC8: rateLimit requires
// requestsPerSecond >= 1; burst is optional but must be >= 1 if provided.
func TestSchemaWebhookRateLimit(t *testing.T) {
	cases := []struct {
		name      string
		rlJSON    string
		wantValid bool
	}{
		{"rps positive", `{"requestsPerSecond":10}`, true},
		{"rps with burst", `{"requestsPerSecond":10,"burst":20}`, true},
		{"rps missing", `{"burst":5}`, false},
		{"rps zero", `{"requestsPerSecond":0}`, false},
		{"burst zero", `{"requestsPerSecond":10,"burst":0}`, false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			input := fmt.Sprintf(`{"type":"webhook","path":"/in","rateLimit":%s}`, tc.rlJSON)
			var data map[string]any
			if err := json.Unmarshal([]byte(pipelineWithWebhook(input)), &data); err != nil {
				t.Fatalf("unmarshal: %v", err)
			}
			res := config.ValidateConfig(data)
			if res.Valid != tc.wantValid {
				t.Errorf("got valid=%v want %v (errors: %v)", res.Valid, tc.wantValid, res.Errors)
			}
		})
	}
}
