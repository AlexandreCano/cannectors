package config_test

import (
	"testing"
)

// TestSchemaLoopFilterBasic locks the canonical loop filter shape.
func TestSchemaLoopFilterBasic(t *testing.T) {
	cases := []struct {
		name      string
		filter    string
		wantValid bool
	}{
		{
			name:      "minimal loop accepted",
			filter:    `{"type":"loop","field":"cells","itemName":"cell","filters":[]}`,
			wantValid: true,
		},
		{
			name:      "loop with nested mapping accepted",
			filter:    `{"type":"loop","field":"cells","itemName":"cell","filters":[{"type":"mapping","mappings":[{"source":"cell.displayValue","target":"record.eventId"}]}]}`,
			wantValid: true,
		},
		{
			name:      "missing field rejected",
			filter:    `{"type":"loop","itemName":"cell","filters":[]}`,
			wantValid: false,
		},
		{
			name:      "missing itemName rejected",
			filter:    `{"type":"loop","field":"cells","filters":[]}`,
			wantValid: false,
		},
		{
			name:      "missing filters rejected",
			filter:    `{"type":"loop","field":"cells","itemName":"cell"}`,
			wantValid: false,
		},
		{
			name:      "empty field rejected",
			filter:    `{"type":"loop","field":"","itemName":"cell","filters":[]}`,
			wantValid: false,
		},
		{
			name:      "empty itemName rejected",
			filter:    `{"type":"loop","field":"cells","itemName":"","filters":[]}`,
			wantValid: false,
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := validateFilter(t, tc.filter); got != tc.wantValid {
				t.Errorf("valid=%v want %v", got, tc.wantValid)
			}
		})
	}
}

// TestSchemaLoopRejectsReservedItemNames locks the schema-level rejection of
// reserved aliases (record, _metadata, loop).
func TestSchemaLoopRejectsReservedItemNames(t *testing.T) {
	for _, reserved := range []string{"record", "_metadata", "loop"} {
		filter := `{"type":"loop","field":"cells","itemName":"` + reserved + `","filters":[]}`
		if validateFilter(t, filter) {
			t.Errorf("itemName=%q: expected invalid, got valid", reserved)
		}
	}
}

// TestSchemaLoopNestedLoopAccepted locks that loop filters are recursively
// composable via the registry path (filters[i] is itself a filterModule).
func TestSchemaLoopNestedLoopAccepted(t *testing.T) {
	filter := `{"type":"loop","field":"cells","itemName":"cell","filters":[` +
		`{"type":"loop","field":"cell.children","itemName":"x","filters":[]}` +
		`]}`
	if !validateFilter(t, filter) {
		t.Errorf("expected nested loop to be valid")
	}
}
