package filter_test

import (
	"context"
	"fmt"

	"github.com/cannectors/runtime/internal/modules/filter"
)

// Example demonstrates a complete implementation of a filter module.
// This example shows the correct way to implement filter.Module interface,
// including context handling and interface compliance verification.
func Example() {
	// Create a simple field mapping filter
	module := &ExampleFilterModule{
		mappings: map[string]string{
			"id":    "userId",
			"name":  "fullName",
			"email": "emailAddress",
		},
	}

	// Input records
	inputRecords := []map[string]any{
		{"id": 1, "name": "Alice", "email": "alice@example.com"},
		{"id": 2, "name": "Bob", "email": "bob@example.com"},
	}

	// Process records
	ctx := context.Background()
	outputRecords, err := module.Process(ctx, inputRecords)
	if err != nil {
		fmt.Printf("Error: %v\n", err)
		return
	}

	// Print transformed records
	fmt.Printf("Transformed %d records\n", len(outputRecords))
	for _, record := range outputRecords {
		fmt.Printf("User ID: %v, Full Name: %v, Email: %v\n",
			record["userId"], record["fullName"], record["emailAddress"])
	}

	// Output:
	// Transformed 2 records
	// User ID: 1, Full Name: Alice, Email: alice@example.com
	// User ID: 2, Full Name: Bob, Email: bob@example.com
}

// ExampleFilterModule demonstrates a minimal filter module implementation.
type ExampleFilterModule struct {
	mappings map[string]string // source -> target field mappings
}

func (f *ExampleFilterModule) Process(ctx context.Context, records []map[string]any) ([]map[string]any, error) {
	// Respect context cancellation
	select {
	case <-ctx.Done():
		return nil, ctx.Err()
	default:
	}

	// Transform records
	result := make([]map[string]any, len(records))
	for i, record := range records {
		transformed := make(map[string]any)
		for sourceField, targetField := range f.mappings {
			if value, exists := record[sourceField]; exists {
				transformed[targetField] = value
			}
		}
		result[i] = transformed
	}

	return result, nil
}

// Verify interface compliance at compile time
var _ filter.Module = (*ExampleFilterModule)(nil)
