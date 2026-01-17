package runtime

import (
	"bytes"
	"context"
	"net/http"
	"testing"
	"time"

	"github.com/canectors/runtime/internal/modules/input"
	"github.com/canectors/runtime/pkg/connector"
)

// waitForWebhook waits for the webhook server to be ready (address available).
// Returns true if server is ready, false if timeout.
func waitForWebhook(w *input.Webhook, timeout time.Duration) bool {
	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		if w.Address() != "" {
			return true
		}
		time.Sleep(10 * time.Millisecond)
	}
	return false
}

// waitForRecords waits for the output module to receive the expected number of records.
// Returns true if expected records received, false if timeout.
func waitForRecords(outputModule *MockOutputModule, expected int, timeout time.Duration) bool {
	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		if len(outputModule.sentRecords) >= expected {
			return true
		}
		time.Sleep(10 * time.Millisecond)
	}
	return false
}

func TestWebhook_ExecutorIntegration(t *testing.T) {
	config := &connector.ModuleConfig{
		Type: "webhook",
		Config: map[string]interface{}{
			"endpoint":      "/webhook/test",
			"listenAddress": "127.0.0.1:0",
		},
	}

	webhook, err := input.NewWebhookFromConfig(config)
	if err != nil {
		t.Fatalf("NewWebhookFromConfig() error = %v", err)
	}

	pipeline := &connector.Pipeline{
		ID:      "pipeline-1",
		Name:    "Test Pipeline",
		Version: "1.0.0",
		Enabled: true,
	}

	outputModule := NewMockOutputModule(nil)
	executor := NewExecutorWithModules(nil, nil, outputModule, false)

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	handler := func(data []map[string]interface{}) error {
		_, execErr := executor.ExecuteWithRecords(pipeline, data)
		return execErr
	}

	go func() {
		_ = webhook.Start(ctx, handler)
	}()

	if !waitForWebhook(webhook, 2*time.Second) {
		t.Fatal("Webhook server did not start within timeout")
	}

	addr := webhook.Address()
	payload := `[{"id": 1}, {"id": 2}]`
	resp, err := http.Post("http://"+addr+"/webhook/test", "application/json", bytes.NewBufferString(payload))
	if err != nil {
		t.Fatalf("POST request failed: %v", err)
	}
	defer func() {
		if closeErr := resp.Body.Close(); closeErr != nil {
			t.Logf("failed to close response body: %v", closeErr)
		}
	}()

	if resp.StatusCode != http.StatusOK {
		t.Fatalf("Response status = %d, want %d", resp.StatusCode, http.StatusOK)
	}

	if !waitForRecords(outputModule, 2, 2*time.Second) {
		t.Fatalf("Timed out waiting for 2 records, got %d", len(outputModule.sentRecords))
	}
}
