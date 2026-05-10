package output_test

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"

	"github.com/cannectors/runtime/internal/modules/output"
)

// Example demonstrates a complete implementation of an output module.
// This example shows the correct way to implement output.Module interface,
// including context handling, resource cleanup, and interface compliance verification.
func Example() {
	// Mock HTTP server for demonstration
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var records []map[string]any
		_ = json.NewDecoder(r.Body).Decode(&records) // Example code - error handling omitted for clarity
		w.WriteHeader(http.StatusCreated)
		_ = json.NewEncoder(w).Encode(map[string]any{
			"status":  "success",
			"message": fmt.Sprintf("Received %d records", len(records)),
		})
	}))
	defer server.Close()

	// Create output module
	module := &ExampleOutputModule{
		endpoint: server.URL,
		client:   &http.Client{},
	}

	// Records to send
	records := []map[string]any{
		{"userId": 1, "fullName": "Alice", "emailAddress": "alice@example.com"},
		{"userId": 2, "fullName": "Bob", "emailAddress": "bob@example.com"},
	}

	// Send records
	ctx := context.Background()
	sent, err := module.Send(ctx, records)
	if err != nil {
		fmt.Printf("Error: %v\n", err)
		return
	}

	fmt.Printf("Successfully sent %d records\n", sent)

	// Clean up resources
	if err := module.Close(); err != nil {
		fmt.Printf("Error closing module: %v\n", err)
	}

	// Output:
	// Successfully sent 2 records
}

// ExampleOutputModule demonstrates a minimal output module implementation.
type ExampleOutputModule struct {
	endpoint string
	client   *http.Client
}

func (m *ExampleOutputModule) Send(ctx context.Context, records []map[string]any) (int, error) {
	// Respect context cancellation
	select {
	case <-ctx.Done():
		return 0, ctx.Err()
	default:
	}

	// Serialize records to JSON
	body, err := json.Marshal(records)
	if err != nil {
		return 0, err
	}

	// Send to destination
	req, err := http.NewRequestWithContext(ctx, "POST", m.endpoint, bytes.NewReader(body))
	if err != nil {
		return 0, err
	}
	req.Header.Set("Content-Type", "application/json")

	resp, err := m.client.Do(req)
	if err != nil {
		return 0, err
	}
	defer resp.Body.Close() //nolint:errcheck // Example code - error handling omitted for clarity

	if resp.StatusCode >= 400 {
		return 0, fmt.Errorf("request failed with status %d", resp.StatusCode)
	}

	return len(records), nil
}

func (m *ExampleOutputModule) Close() error {
	// Clean up HTTP client connections
	if m.client != nil {
		m.client.CloseIdleConnections()
	}
	return nil
}

// Verify interface compliance at compile time
var _ output.Module = (*ExampleOutputModule)(nil)
