package input_test

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"

	"github.com/cannectors/runtime/internal/modules/input"
)

// Example demonstrates a complete implementation of an input module.
// This example shows the correct way to implement input.Module interface,
// including context handling, resource cleanup, and interface compliance verification.
func Example() {
	// Mock HTTP server for demonstration
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		data := []map[string]any{
			{"id": 1, "name": "Alice", "email": "alice@example.com"},
			{"id": 2, "name": "Bob", "email": "bob@example.com"},
		}
		_ = json.NewEncoder(w).Encode(data) // Example code - error handling omitted for clarity
	}))
	defer server.Close()

	// Create the input module
	// (In real code, you'd use factory.CreateInputModule or registry directly)
	module := &ExampleInputModule{
		url: server.URL,
	}

	// Fetch data from source
	ctx := context.Background()
	records, err := module.Fetch(ctx)
	if err != nil {
		fmt.Printf("Error: %v\n", err)
		return
	}

	// Print fetched records
	fmt.Printf("Fetched %d records\n", len(records))
	for _, record := range records {
		fmt.Printf("ID: %v, Name: %v\n", record["id"], record["name"])
	}

	// Clean up resources
	if err := module.Close(); err != nil {
		fmt.Printf("Error closing module: %v\n", err)
	}

	// Output:
	// Fetched 2 records
	// ID: 1, Name: Alice
	// ID: 2, Name: Bob
}

// ExampleInputModule demonstrates a minimal input module implementation.
type ExampleInputModule struct {
	url    string
	client *http.Client
}

func (m *ExampleInputModule) Fetch(ctx context.Context) ([]map[string]any, error) {
	// Respect context cancellation
	select {
	case <-ctx.Done():
		return nil, ctx.Err()
	default:
	}

	// Create HTTP client if not exists
	if m.client == nil {
		m.client = &http.Client{}
	}

	// Fetch data from source
	req, err := http.NewRequestWithContext(ctx, "GET", m.url, nil)
	if err != nil {
		return nil, err
	}

	resp, err := m.client.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close() //nolint:errcheck // Example code - error handling omitted for clarity

	// Parse response
	var records []map[string]any
	if err := json.NewDecoder(resp.Body).Decode(&records); err != nil {
		return nil, err
	}

	return records, nil
}

func (m *ExampleInputModule) Close() error {
	// Clean up HTTP client connections
	if m.client != nil {
		m.client.CloseIdleConnections()
	}
	return nil
}

// Verify interface compliance at compile time
var _ input.Module = (*ExampleInputModule)(nil)
