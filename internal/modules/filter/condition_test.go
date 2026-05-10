// Package filter provides implementations for filter modules.
package filter

import (
	"context"
	"encoding/json"
	"errors"
	"testing"

	"github.com/cannectors/runtime/pkg/connector"
)

// strPtr is a helper to create string pointers for test cases.
func strPtrCond(s string) *string {
	return &s
}

// TestConditionBasicEquality tests basic equality comparisons (==, !=)
func TestConditionBasicEquality(t *testing.T) {
	tests := []struct {
		name       string
		expression string
		record     map[string]any
		wantPass   bool
	}{
		{
			name:       "string equality - match",
			expression: "status == 'active'",
			record:     map[string]any{"status": "active"},
			wantPass:   true,
		},
		{
			name:       "string equality - no match",
			expression: "status == 'active'",
			record:     map[string]any{"status": "inactive"},
			wantPass:   false,
		},
		{
			name:       "string inequality - match",
			expression: "status != 'deleted'",
			record:     map[string]any{"status": "active"},
			wantPass:   true,
		},
		{
			name:       "string inequality - no match",
			expression: "status != 'active'",
			record:     map[string]any{"status": "active"},
			wantPass:   false,
		},
		{
			name:       "number equality - match",
			expression: "total == 5",
			record:     map[string]any{"total": 5},
			wantPass:   true,
		},
		{
			name:       "number equality - no match",
			expression: "total == 5",
			record:     map[string]any{"total": 10},
			wantPass:   false,
		},
		{
			name:       "boolean equality - match true",
			expression: "isActive == true",
			record:     map[string]any{"isActive": true},
			wantPass:   true,
		},
		{
			name:       "boolean equality - match false",
			expression: "isActive == false",
			record:     map[string]any{"isActive": false},
			wantPass:   true,
		},
		{
			name:       "null equality - match",
			expression: "value == null",
			record:     map[string]any{"value": nil},
			wantPass:   true,
		},
		{
			name:       "null inequality - match",
			expression: "value != null",
			record:     map[string]any{"value": "something"},
			wantPass:   true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cond, err := NewConditionFromConfig(ConditionConfig{
				Expression: tt.expression,
				Else:       []*NestedModuleConfig{{Type: "drop"}},
			}, nil)
			if err != nil {
				t.Fatalf("NewConditionFromConfig() error = %v", err)
			}

			records := []map[string]any{tt.record}
			result, err := cond.Process(context.Background(), records)
			if err != nil {
				t.Fatalf("Process() error = %v", err)
			}

			if tt.wantPass && len(result) == 0 {
				t.Errorf("expected record to pass condition, but it was filtered out")
			}
			if !tt.wantPass && len(result) > 0 {
				t.Errorf("expected record to be filtered out, but it passed")
			}
		})
	}
}

// TestConditionNumericComparisons tests numeric comparison operators (<, >, <=, >=)
func TestConditionNumericComparisons(t *testing.T) {
	tests := []struct {
		name       string
		expression string
		record     map[string]any
		wantPass   bool
	}{
		{
			name:       "greater than - pass",
			expression: "amount > 100",
			record:     map[string]any{"amount": 150},
			wantPass:   true,
		},
		{
			name:       "greater than - fail",
			expression: "amount > 100",
			record:     map[string]any{"amount": 50},
			wantPass:   false,
		},
		{
			name:       "greater than - equal fails",
			expression: "amount > 100",
			record:     map[string]any{"amount": 100},
			wantPass:   false,
		},
		{
			name:       "less than - pass",
			expression: "amount < 100",
			record:     map[string]any{"amount": 50},
			wantPass:   true,
		},
		{
			name:       "less than - fail",
			expression: "amount < 100",
			record:     map[string]any{"amount": 150},
			wantPass:   false,
		},
		{
			name:       "greater than or equal - pass on greater",
			expression: "amount >= 100",
			record:     map[string]any{"amount": 150},
			wantPass:   true,
		},
		{
			name:       "greater than or equal - pass on equal",
			expression: "amount >= 100",
			record:     map[string]any{"amount": 100},
			wantPass:   true,
		},
		{
			name:       "greater than or equal - fail",
			expression: "amount >= 100",
			record:     map[string]any{"amount": 50},
			wantPass:   false,
		},
		{
			name:       "less than or equal - pass on less",
			expression: "amount <= 100",
			record:     map[string]any{"amount": 50},
			wantPass:   true,
		},
		{
			name:       "less than or equal - pass on equal",
			expression: "amount <= 100",
			record:     map[string]any{"amount": 100},
			wantPass:   true,
		},
		{
			name:       "less than or equal - fail",
			expression: "amount <= 100",
			record:     map[string]any{"amount": 150},
			wantPass:   false,
		},
		{
			name:       "float comparison",
			expression: "price > 19.99",
			record:     map[string]any{"price": 29.99},
			wantPass:   true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cond, err := NewConditionFromConfig(ConditionConfig{
				Expression: tt.expression,
				Else:       []*NestedModuleConfig{{Type: "drop"}},
			}, nil)
			if err != nil {
				t.Fatalf("NewConditionFromConfig() error = %v", err)
			}

			records := []map[string]any{tt.record}
			result, err := cond.Process(context.Background(), records)
			if err != nil {
				t.Fatalf("Process() error = %v", err)
			}

			if tt.wantPass && len(result) == 0 {
				t.Errorf("expected record to pass condition, but it was filtered out")
			}
			if !tt.wantPass && len(result) > 0 {
				t.Errorf("expected record to be filtered out, but it passed")
			}
		})
	}
}

// TestConditionLogicalOperators tests logical operators (&&, ||, !)
func TestConditionLogicalOperators(t *testing.T) {
	tests := []struct {
		name       string
		expression string
		record     map[string]any
		wantPass   bool
	}{
		{
			name:       "AND - both true",
			expression: "status == 'active' && qty > 0",
			record:     map[string]any{"status": "active", "qty": 5},
			wantPass:   true,
		},
		{
			name:       "AND - first true, second false",
			expression: "status == 'active' && qty > 0",
			record:     map[string]any{"status": "active", "qty": 0},
			wantPass:   false,
		},
		{
			name:       "AND - first false, second true",
			expression: "status == 'active' && qty > 0",
			record:     map[string]any{"status": "inactive", "qty": 5},
			wantPass:   false,
		},
		{
			name:       "AND - both false",
			expression: "status == 'active' && qty > 0",
			record:     map[string]any{"status": "inactive", "qty": 0},
			wantPass:   false,
		},
		{
			name:       "OR - both true",
			expression: "status == 'active' || status == 'pending'",
			record:     map[string]any{"status": "active"},
			wantPass:   true,
		},
		{
			name:       "OR - first true",
			expression: "status == 'active' || status == 'pending'",
			record:     map[string]any{"status": "active"},
			wantPass:   true,
		},
		{
			name:       "OR - second true",
			expression: "status == 'active' || status == 'pending'",
			record:     map[string]any{"status": "pending"},
			wantPass:   true,
		},
		{
			name:       "OR - both false",
			expression: "status == 'active' || status == 'pending'",
			record:     map[string]any{"status": "deleted"},
			wantPass:   false,
		},
		{
			name:       "NOT - negate true",
			expression: "!isDeleted",
			record:     map[string]any{"isDeleted": false},
			wantPass:   true,
		},
		{
			name:       "NOT - negate false",
			expression: "!isDeleted",
			record:     map[string]any{"isDeleted": true},
			wantPass:   false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cond, err := NewConditionFromConfig(ConditionConfig{
				Expression: tt.expression,
				Else:       []*NestedModuleConfig{{Type: "drop"}},
			}, nil)
			if err != nil {
				t.Fatalf("NewConditionFromConfig() error = %v", err)
			}

			records := []map[string]any{tt.record}
			result, err := cond.Process(context.Background(), records)
			if err != nil {
				t.Fatalf("Process() error = %v", err)
			}

			if tt.wantPass && len(result) == 0 {
				t.Errorf("expected record to pass condition, but it was filtered out")
			}
			if !tt.wantPass && len(result) > 0 {
				t.Errorf("expected record to be filtered out, but it passed")
			}
		})
	}
}

// TestConditionNestedFieldAccess tests dot notation for nested fields
func TestConditionNestedFieldAccess(t *testing.T) {
	tests := []struct {
		name       string
		expression string
		record     map[string]any
		wantPass   bool
	}{
		{
			name:       "single level nesting",
			expression: "user.status == 'active'",
			record: map[string]any{
				"user": map[string]any{"status": "active"},
			},
			wantPass: true,
		},
		{
			name:       "two level nesting",
			expression: "user.profile.verified == true",
			record: map[string]any{
				"user": map[string]any{
					"profile": map[string]any{"verified": true},
				},
			},
			wantPass: true,
		},
		{
			name:       "nested numeric comparison",
			expression: "order.total > 100",
			record: map[string]any{
				"order": map[string]any{"total": 150},
			},
			wantPass: true,
		},
		{
			name:       "missing nested field - fails",
			expression: "user.status == 'active'",
			record:     map[string]any{"user": map[string]any{}},
			wantPass:   false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cond, err := NewConditionFromConfig(ConditionConfig{
				Expression: tt.expression,
				Else:       []*NestedModuleConfig{{Type: "drop"}},
			}, nil)
			if err != nil {
				t.Fatalf("NewConditionFromConfig() error = %v", err)
			}

			records := []map[string]any{tt.record}
			result, err := cond.Process(context.Background(), records)
			if err != nil {
				t.Fatalf("Process() error = %v", err)
			}

			if tt.wantPass && len(result) == 0 {
				t.Errorf("expected record to pass condition, but it was filtered out")
			}
			if !tt.wantPass && len(result) > 0 {
				t.Errorf("expected record to be filtered out, but it passed")
			}
		})
	}
}

// TestConditionArrayAccess tests array element access with bracket notation
func TestConditionArrayAccess(t *testing.T) {
	tests := []struct {
		name       string
		expression string
		record     map[string]any
		wantPass   bool
	}{
		{
			name:       "array first element",
			expression: "items[0].price > 10",
			record: map[string]any{
				"items": []any{
					map[string]any{"price": 15},
				},
			},
			wantPass: true,
		},
		{
			name:       "array second element",
			expression: "items[1].name == 'test'",
			record: map[string]any{
				"items": []any{
					map[string]any{"name": "first"},
					map[string]any{"name": "test"},
				},
			},
			wantPass: true,
		},
		{
			name:       "array valid index",
			expression: "items[0].price > 5",
			record: map[string]any{
				"items": []any{
					map[string]any{"price": 15},
				},
			},
			wantPass: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cond, err := NewConditionFromConfig(ConditionConfig{
				Expression: tt.expression,
				Else:       []*NestedModuleConfig{{Type: "drop"}},
			}, nil)
			if err != nil {
				t.Fatalf("NewConditionFromConfig() error = %v", err)
			}

			records := []map[string]any{tt.record}
			result, err := cond.Process(context.Background(), records)
			if err != nil {
				t.Fatalf("Process() error = %v", err)
			}

			if tt.wantPass && len(result) == 0 {
				t.Errorf("expected record to pass condition, but it was filtered out")
			}
			if !tt.wantPass && len(result) > 0 {
				t.Errorf("expected record to be filtered out, but it passed")
			}
		})
	}
}

// TestConditionMultipleRecords tests filtering multiple records
func TestConditionMultipleRecords(t *testing.T) {
	cond, err := NewConditionFromConfig(ConditionConfig{
		Expression: "status == 'active'",
		Else:       []*NestedModuleConfig{{Type: "drop"}},
	}, nil)
	if err != nil {
		t.Fatalf("NewConditionFromConfig() error = %v", err)
	}

	records := []map[string]any{
		{"id": 1, "status": "active"},
		{"id": 2, "status": "inactive"},
		{"id": 3, "status": "active"},
		{"id": 4, "status": "deleted"},
		{"id": 5, "status": "active"},
	}

	result, err := cond.Process(context.Background(), records)
	if err != nil {
		t.Fatalf("Process() error = %v", err)
	}

	if len(result) != 3 {
		t.Errorf("expected 3 records to pass, got %d", len(result))
	}

	// Verify correct records passed
	expectedIDs := []int{1, 3, 5}
	for i, r := range result {
		if id, ok := r["id"].(int); !ok || id != expectedIDs[i] {
			t.Errorf("expected record with id=%d at position %d, got %v", expectedIDs[i], i, r["id"])
		}
	}
}

// TestConditionEmptyInput tests handling of empty input
func TestConditionEmptyInput(t *testing.T) {
	cond, err := NewConditionFromConfig(ConditionConfig{
		Expression: "status == 'active'",
	}, nil)
	if err != nil {
		t.Fatalf("NewConditionFromConfig() error = %v", err)
	}

	// Test with nil
	result, err := cond.Process(context.Background(), nil)
	if err != nil {
		t.Fatalf("Process(nil) error = %v", err)
	}
	if len(result) != 0 {
		t.Errorf("expected empty result for nil input, got %d records", len(result))
	}

	// Test with empty slice
	result, err = cond.Process(context.Background(), []map[string]any{})
	if err != nil {
		t.Fatalf("Process([]) error = %v", err)
	}
	if len(result) != 0 {
		t.Errorf("expected empty result for empty input, got %d records", len(result))
	}
}

// TestConditionConfigValidation tests configuration validation
func TestConditionConfigValidation(t *testing.T) {
	// Valid configuration
	_, err := NewConditionFromConfig(ConditionConfig{
		Expression: "status == 'active'",
	}, nil)
	if err != nil {
		t.Errorf("valid config should not error: %v", err)
	}
}

// ===========================================================================
// Task 2: Routing Behavior Tests (onTrue/onFalse)
// ===========================================================================

// TestConditionNullValueHandling tests handling of null values in conditions
func TestConditionNullValueHandling(t *testing.T) {
	tests := []struct {
		name       string
		expression string
		record     map[string]any
		wantPass   bool
	}{
		{
			name:       "null field equals null",
			expression: "value == null",
			record:     map[string]any{"value": nil},
			wantPass:   true,
		},
		{
			name:       "missing field treated as null",
			expression: "missing == null",
			record:     map[string]any{"other": "value"},
			wantPass:   true,
		},
		{
			name:       "null field not equal to value",
			expression: "value != null",
			record:     map[string]any{"value": nil},
			wantPass:   false,
		},
		{
			name:       "non-null field not equal to null",
			expression: "value != null",
			record:     map[string]any{"value": "something"},
			wantPass:   true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cond, err := NewConditionFromConfig(ConditionConfig{
				Expression: tt.expression,
				Else:       []*NestedModuleConfig{{Type: "drop"}},
			}, nil)
			if err != nil {
				t.Fatalf("NewConditionFromConfig() error = %v", err)
			}

			result, err := cond.Process(context.Background(), []map[string]any{tt.record})
			if err != nil {
				t.Fatalf("Process() error = %v", err)
			}

			if tt.wantPass && len(result) == 0 {
				t.Errorf("expected record to pass, but it was filtered out")
			}
			if !tt.wantPass && len(result) > 0 {
				t.Errorf("expected record to be filtered out, but it passed")
			}
		})
	}
}

// TestConditionTypeMismatchHandling tests handling of type mismatches in conditions
func TestConditionTypeMismatchHandling(t *testing.T) {
	tests := []struct {
		name       string
		expression string
		record     map[string]any
		wantPass   bool
	}{
		{
			name:       "string compared to number - not equal",
			expression: "value == 100",
			record:     map[string]any{"value": "100"},
			wantPass:   false, // string "100" != number 100
		},
		{
			name:       "int compared to float - equal",
			expression: "value == 100.0",
			record:     map[string]any{"value": 100},
			wantPass:   true, // int 100 == float 100.0
		},
		{
			name:       "boolean compared to number - not equal",
			expression: "active == 1",
			record:     map[string]any{"active": true},
			wantPass:   false, // boolean true != number 1
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cond, err := NewConditionFromConfig(ConditionConfig{
				Expression: tt.expression,
				Else:       []*NestedModuleConfig{{Type: "drop"}},
			}, nil)
			if err != nil {
				t.Fatalf("NewConditionFromConfig() error = %v", err)
			}

			result, err := cond.Process(context.Background(), []map[string]any{tt.record})
			if err != nil {
				t.Fatalf("Process() error = %v", err)
			}

			if tt.wantPass && len(result) == 0 {
				t.Errorf("expected record to pass, but it was filtered out")
			}
			if !tt.wantPass && len(result) > 0 {
				t.Errorf("expected record to be filtered out, but it passed")
			}
		})
	}
}

// ===========================================================================
// Task 2b: Nested Modules Tests
// ===========================================================================

func TestNestedModuleConfigUnmarshalPreservesTopLevelFilterShape(t *testing.T) {
	raw := []byte(`{
		"expression": "status == 'paid'",
		"then": [
			{"type": "set", "target": "routing.bucket", "value": "billable"},
			{"type": "mapping", "mappings": [{"source": "id", "target": "payment.id"}]}
		],
		"else": [
			{"type": "remove", "target": ["card.number", "card.cvv"]}
		]
	}`)

	var cfg ConditionConfig
	if err := json.Unmarshal(raw, &cfg); err != nil {
		t.Fatalf("unmarshal condition config: %v", err)
	}

	if got := cfg.Then[0].Config["target"]; got != "routing.bucket" {
		t.Fatalf("expected nested set target in Config, got %v", got)
	}
	if got := cfg.Then[0].Config["value"]; got != "billable" {
		t.Fatalf("expected nested set value in Config, got %v", got)
	}
	if len(cfg.Then[1].Mappings) != 1 {
		t.Fatalf("expected nested mapping mappings to be preserved, got %d", len(cfg.Then[1].Mappings))
	}

	targets, ok := cfg.Else[0].Config["target"].([]any)
	if !ok {
		t.Fatalf("expected nested remove target list in Config, got %T", cfg.Else[0].Config["target"])
	}
	if len(targets) != 2 || targets[0] != "card.number" || targets[1] != "card.cvv" {
		t.Fatalf("unexpected nested remove targets: %v", targets)
	}
}

// TestConditionNestedThenMapping tests 'then' with a nested mapping module
func TestConditionNestedThenMapping(t *testing.T) {
	cond, err := NewConditionFromConfig(ConditionConfig{
		Expression: "status == 'active'",
		Then: []*NestedModuleConfig{{
			Type: "mapping",
			Mappings: []FieldMapping{
				{Source: strPtrCond("name"), Target: "displayName"},
				{Source: strPtrCond("id"), Target: "userId"},
			},
		}},
	}, nil)
	if err != nil {
		t.Fatalf("NewConditionFromConfig() error = %v", err)
	}

	records := []map[string]any{
		{"id": 1, "name": "Alice", "status": "active"},
	}

	result, err := cond.Process(context.Background(), records)
	if err != nil {
		t.Fatalf("Process() error = %v", err)
	}

	if len(result) != 1 {
		t.Fatalf("expected 1 record, got %d", len(result))
	}

	// Verify mapping was applied
	if result[0]["displayName"] != "Alice" {
		t.Errorf("expected displayName='Alice', got %v", result[0]["displayName"])
	}
	if result[0]["userId"] != 1 {
		t.Errorf("expected userId=1, got %v", result[0]["userId"])
	}

	// Original fields should be preserved (in-place mapping)
	if result[0]["name"] != "Alice" {
		t.Errorf("expected 'name' field to be preserved, got %v", result[0]["name"])
	}
	if result[0]["status"] != "active" {
		t.Errorf("expected 'status' field to be preserved, got %v", result[0]["status"])
	}
}

// TestConditionNestedElseMapping tests 'else' with a nested mapping module.
// Records matching the condition are dropped explicitly via the `drop` filter
// in the `then` branch.
func TestConditionNestedElseMapping(t *testing.T) {
	cond, err := NewConditionFromConfig(ConditionConfig{
		Expression: "status == 'active'",
		Then:       []*NestedModuleConfig{{Type: "drop"}},
		Else: []*NestedModuleConfig{{
			Type: "mapping",
			Mappings: []FieldMapping{
				{Source: strPtrCond("id"), Target: "inactiveUserId"},
				{Source: strPtrCond("status"), Target: "currentStatus"},
			},
		}},
	}, nil)
	if err != nil {
		t.Fatalf("NewConditionFromConfig() error = %v", err)
	}

	records := []map[string]any{
		{"id": 1, "status": "active"},
		{"id": 2, "status": "inactive"},
	}

	result, err := cond.Process(context.Background(), records)
	if err != nil {
		t.Fatalf("Process() error = %v", err)
	}

	// Only the inactive record should be processed by else mapping
	if len(result) != 1 {
		t.Fatalf("expected 1 record, got %d", len(result))
	}

	if result[0]["inactiveUserId"] != 2 {
		t.Errorf("expected inactiveUserId=2, got %v", result[0]["inactiveUserId"])
	}
	if result[0]["currentStatus"] != "inactive" {
		t.Errorf("expected currentStatus='inactive', got %v", result[0]["currentStatus"])
	}
}

// TestConditionNestedBothThenElse tests both 'then' and 'else' with different mappings
func TestConditionNestedBothThenElse(t *testing.T) {
	cond, err := NewConditionFromConfig(ConditionConfig{
		Expression: "tier == 'premium'",
		Then: []*NestedModuleConfig{{
			Type: "mapping",
			Mappings: []FieldMapping{
				{Source: strPtrCond("id"), Target: "premiumId"},
				{Source: strPtrCond("name"), Target: "premiumName"},
			},
		}},
		Else: []*NestedModuleConfig{{
			Type: "mapping",
			Mappings: []FieldMapping{
				{Source: strPtrCond("id"), Target: "standardId"},
				{Source: strPtrCond("name"), Target: "standardName"},
			},
		}},
	}, nil)
	if err != nil {
		t.Fatalf("NewConditionFromConfig() error = %v", err)
	}

	records := []map[string]any{
		{"id": 1, "name": "Alice", "tier": "premium"},
		{"id": 2, "name": "Bob", "tier": "standard"},
	}

	result, err := cond.Process(context.Background(), records)
	if err != nil {
		t.Fatalf("Process() error = %v", err)
	}

	if len(result) != 2 {
		t.Fatalf("expected 2 records, got %d", len(result))
	}

	// First record (premium) should have premiumId/premiumName
	if result[0]["premiumId"] != 1 {
		t.Errorf("expected premiumId=1, got %v", result[0]["premiumId"])
	}
	if result[0]["premiumName"] != "Alice" {
		t.Errorf("expected premiumName='Alice', got %v", result[0]["premiumName"])
	}

	// Second record (standard) should have standardId/standardName
	if result[1]["standardId"] != 2 {
		t.Errorf("expected standardId=2, got %v", result[1]["standardId"])
	}
	if result[1]["standardName"] != "Bob" {
		t.Errorf("expected standardName='Bob', got %v", result[1]["standardName"])
	}
}

// TestConditionNestedRecursiveCondition tests nested condition within condition.
// Uses explicit `drop` filters to remove records, since branches without
// filters now keep records unchanged.
func TestConditionNestedRecursiveCondition(t *testing.T) {
	cond, err := NewConditionFromConfig(ConditionConfig{
		Expression: "level > 0",
		Then: []*NestedModuleConfig{{
			Type:       "condition",
			Expression: "level > 5",
			Else:       []*NestedModuleConfig{{Type: "drop"}}, // Medium level: drop
		}},
		Else: []*NestedModuleConfig{{Type: "drop"}}, // Level 0: drop
	}, nil)
	if err != nil {
		t.Fatalf("NewConditionFromConfig() error = %v", err)
	}

	records := []map[string]any{
		{"id": 1, "level": 0},  // level > 0: false -> skip
		{"id": 2, "level": 3},  // level > 0: true -> then (level > 5: false -> skip)
		{"id": 3, "level": 10}, // level > 0: true -> then (level > 5: true -> continue)
	}

	result, err := cond.Process(context.Background(), records)
	if err != nil {
		t.Fatalf("Process() error = %v", err)
	}

	// Only record with level=10 should pass
	if len(result) != 1 {
		t.Fatalf("expected 1 record, got %d", len(result))
	}

	if result[0]["id"] != 3 {
		t.Errorf("expected id=3, got %v", result[0]["id"])
	}
}

// ===========================================================================
// Task 3: Complex Expression Tests
// ===========================================================================

// TestConditionParenthesesGrouping tests parentheses for operator precedence
func TestConditionParenthesesGrouping(t *testing.T) {
	tests := []struct {
		name       string
		expression string
		record     map[string]any
		wantPass   bool
	}{
		{
			name:       "simple grouping - (a || b) && c - all true",
			expression: "(status == 'active' || status == 'pending') && qty > 0",
			record:     map[string]any{"status": "active", "qty": 5},
			wantPass:   true,
		},
		{
			name:       "simple grouping - (a || b) && c - first OR true, AND true",
			expression: "(status == 'active' || status == 'pending') && qty > 0",
			record:     map[string]any{"status": "pending", "qty": 5},
			wantPass:   true,
		},
		{
			name:       "simple grouping - (a || b) && c - first OR false, fails",
			expression: "(status == 'active' || status == 'pending') && qty > 0",
			record:     map[string]any{"status": "deleted", "qty": 5},
			wantPass:   false,
		},
		{
			name:       "simple grouping - (a || b) && c - AND false, fails",
			expression: "(status == 'active' || status == 'pending') && qty > 0",
			record:     map[string]any{"status": "active", "qty": 0},
			wantPass:   false,
		},
		{
			name:       "precedence without parens - a || b && c",
			expression: "a == true || b == true && c == true",
			record:     map[string]any{"a": true, "b": false, "c": false},
			wantPass:   true, // a || (b && c) => true || false => true
		},
		{
			name:       "nested parentheses - ((a && b) || c)",
			expression: "((x == 1 && y == 2) || z == 3)",
			record:     map[string]any{"x": 1, "y": 2, "z": 0},
			wantPass:   true, // (1 && 2) || 0 => true || false => true
		},
		{
			name:       "nested parentheses - z matches",
			expression: "((x == 1 && y == 2) || z == 3)",
			record:     map[string]any{"x": 0, "y": 0, "z": 3},
			wantPass:   true, // (false && false) || true => true
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cond, err := NewConditionFromConfig(ConditionConfig{
				Expression: tt.expression,
				Else:       []*NestedModuleConfig{{Type: "drop"}},
			}, nil)
			if err != nil {
				t.Fatalf("NewConditionFromConfig() error = %v", err)
			}

			result, err := cond.Process(context.Background(), []map[string]any{tt.record})
			if err != nil {
				t.Fatalf("Process() error = %v", err)
			}

			if tt.wantPass && len(result) == 0 {
				t.Errorf("expected record to pass, but it was filtered out")
			}
			if !tt.wantPass && len(result) > 0 {
				t.Errorf("expected record to be filtered out, but it passed")
			}
		})
	}
}

// TestConditionStringOperations tests string operations using expr syntax
// expr uses: str contains "substr", str startsWith "prefix", str endsWith "suffix"
func TestConditionStringOperations(t *testing.T) {
	tests := []struct {
		name       string
		expression string
		record     map[string]any
		wantPass   bool
	}{
		{
			name:       "contains - match",
			expression: `name contains "test"`,
			record:     map[string]any{"name": "this is a test string"},
			wantPass:   true,
		},
		{
			name:       "contains - no match",
			expression: `name contains "xyz"`,
			record:     map[string]any{"name": "this is a test string"},
			wantPass:   false,
		},
		{
			name:       "startsWith - match",
			expression: `email startsWith "admin"`,
			record:     map[string]any{"email": "admin@example.com"},
			wantPass:   true,
		},
		{
			name:       "startsWith - no match",
			expression: `email startsWith "user"`,
			record:     map[string]any{"email": "admin@example.com"},
			wantPass:   false,
		},
		{
			name:       "endsWith - match",
			expression: `path endsWith ".json"`,
			record:     map[string]any{"path": "/data/config.json"},
			wantPass:   true,
		},
		{
			name:       "endsWith - no match",
			expression: `path endsWith ".xml"`,
			record:     map[string]any{"path": "/data/config.json"},
			wantPass:   false,
		},
		{
			name:       "nested field with string operation",
			expression: `user.email endsWith "@example.com"`,
			record: map[string]any{
				"user": map[string]any{"email": "test@example.com"},
			},
			wantPass: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cond, err := NewConditionFromConfig(ConditionConfig{
				Expression: tt.expression,
				Else:       []*NestedModuleConfig{{Type: "drop"}},
			}, nil)
			if err != nil {
				t.Fatalf("NewConditionFromConfig() error = %v", err)
			}

			result, err := cond.Process(context.Background(), []map[string]any{tt.record})
			if err != nil {
				t.Fatalf("Process() error = %v", err)
			}

			if tt.wantPass && len(result) == 0 {
				t.Errorf("expected record to pass, but it was filtered out")
			}
			if !tt.wantPass && len(result) > 0 {
				t.Errorf("expected record to be filtered out, but it passed")
			}
		})
	}
}

// TestConditionDeepNestedFieldAccess tests deeply nested field access
func TestConditionDeepNestedFieldAccess(t *testing.T) {
	tests := []struct {
		name       string
		expression string
		record     map[string]any
		wantPass   bool
	}{
		{
			name:       "three level nesting",
			expression: "user.profile.settings.darkMode == true",
			record: map[string]any{
				"user": map[string]any{
					"profile": map[string]any{
						"settings": map[string]any{
							"darkMode": true,
						},
					},
				},
			},
			wantPass: true,
		},
		{
			name:       "array with nested object",
			expression: "orders[0].items[0].product.name == 'Widget'",
			record: map[string]any{
				"orders": []any{
					map[string]any{
						"items": []any{
							map[string]any{
								"product": map[string]any{
									"name": "Widget",
								},
							},
						},
					},
				},
			},
			wantPass: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cond, err := NewConditionFromConfig(ConditionConfig{
				Expression: tt.expression,
				Else:       []*NestedModuleConfig{{Type: "drop"}},
			}, nil)
			if err != nil {
				t.Fatalf("NewConditionFromConfig() error = %v", err)
			}

			result, err := cond.Process(context.Background(), []map[string]any{tt.record})
			if err != nil {
				t.Fatalf("Process() error = %v", err)
			}

			if tt.wantPass && len(result) == 0 {
				t.Errorf("expected record to pass, but it was filtered out")
			}
			if !tt.wantPass && len(result) > 0 {
				t.Errorf("expected record to be filtered out, but it passed")
			}
		})
	}
}

// ===========================================================================
// Task 4: Expression Language Support Tests
// ===========================================================================

// ===========================================================================
// Task 5: Error Handling and Validation Tests
// ===========================================================================

// TestConditionSyntaxErrorDetection tests that syntax errors are detected during construction
func TestConditionSyntaxErrorDetection(t *testing.T) {
	tests := []struct {
		name       string
		expression string
		wantErr    bool
	}{
		{
			name:       "valid expression",
			expression: "status == 'active'",
			wantErr:    false,
		},
		{
			name:       "empty expression",
			expression: "",
			wantErr:    true,
		},
		{
			name:       "whitespace only expression",
			expression: "   ",
			wantErr:    true,
		},
		{
			name:       "unclosed string",
			expression: "status == 'active",
			wantErr:    true,
		},
		{
			name:       "missing operator",
			expression: "status 'active'",
			wantErr:    true,
		},
		{
			name:       "invalid operator",
			expression: "status = 'active'",
			wantErr:    true, // should be ==
		},
		{
			name:       "single ampersand",
			expression: "a & b",
			wantErr:    true, // should be &&
		},
		{
			name:       "single pipe",
			expression: "a | b",
			wantErr:    true, // should be ||
		},
		{
			name:       "unclosed parenthesis",
			expression: "(status == 'active'",
			wantErr:    true,
		},
		{
			name:       "unclosed bracket",
			expression: "items[0.price",
			wantErr:    true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := NewConditionFromConfig(ConditionConfig{
				Expression: tt.expression,
			}, nil)
			if tt.wantErr && err == nil {
				t.Errorf("expected error for expression %q, got nil", tt.expression)
			}
			if !tt.wantErr && err != nil {
				t.Errorf("unexpected error for expression %q: %v", tt.expression, err)
			}
		})
	}
}

// TestConditionOnErrorFail tests onError="fail" (default) stops processing
func TestConditionOnErrorFail(t *testing.T) {
	// Create a condition that will cause an evaluation error
	// by using a method on a nil value
	cond, err := NewConditionFromConfig(ConditionConfig{
		Expression: "data.unknownMethod()",
		ModuleBase: connector.ModuleBase{OnError: "fail"},
	}, nil)
	if err != nil {
		t.Fatalf("NewConditionFromConfig() error = %v", err)
	}

	records := []map[string]any{
		{"data": "test"},
	}

	_, err = cond.Process(context.Background(), records)
	if err == nil {
		t.Error("expected error with onError='fail', got nil")
	}
}

// TestConditionOnErrorSkip tests onError="skip" skips problematic records
func TestConditionOnErrorSkip(t *testing.T) {
	// Create a condition that will cause an error when evaluating invalid operations
	// Using a method call that doesn't exist will cause an evaluation error
	cond, err := NewConditionFromConfig(ConditionConfig{
		Expression: "value.invalidMethod() > 0",
		ModuleBase: connector.ModuleBase{OnError: "skip"},
	}, nil)
	if err != nil {
		t.Fatalf("NewConditionFromConfig() error = %v", err)
	}

	records := []map[string]any{
		{"value": 10},                          // Will cause error (can't call method on int), should be skipped
		{"value": 5},                           // Will cause error (can't call method on int), should be skipped
		{"value": map[string]any{"nested": 1}}, // Will cause error, should be skipped
	}

	result, err := cond.Process(context.Background(), records)
	if err != nil {
		t.Fatalf("Process() error = %v", err)
	}

	// All records should be skipped due to evaluation errors
	if len(result) != 0 {
		t.Errorf("expected 0 records with onError='skip' (all should be skipped due to errors), got %d", len(result))
	}
}

// TestConditionOnErrorLog tests onError="log" logs errors and continues
func TestConditionOnErrorLog(t *testing.T) {
	// Create a condition that will cause an error when evaluating invalid operations
	cond, err := NewConditionFromConfig(ConditionConfig{
		Expression: "value.invalidMethod() > 0",
		ModuleBase: connector.ModuleBase{OnError: "log"},
	}, nil)
	if err != nil {
		t.Fatalf("NewConditionFromConfig() error = %v", err)
	}

	records := []map[string]any{
		{"value": 10},                          // Will cause error (can't call method on int), should be logged and skipped
		{"value": 5},                           // Will cause error (can't call method on int), should be logged and skipped
		{"value": map[string]any{"nested": 1}}, // Will cause error, should be logged and skipped
	}

	result, err := cond.Process(context.Background(), records)
	if err != nil {
		t.Fatalf("Process() error = %v", err)
	}

	// All records should be skipped due to evaluation errors, but processing should continue
	if len(result) != 0 {
		t.Errorf("expected 0 records with onError='log' (all should be skipped due to errors), got %d", len(result))
	}
}

// TestConditionInvalidOnErrorDefaultsToFail ensures invalid onError values are rejected at construction.
func TestConditionInvalidOnErrorDefaultsToFail(t *testing.T) {
	_, err := NewConditionFromConfig(ConditionConfig{
		Expression: "data.unknownMethod()",
		ModuleBase: connector.ModuleBase{OnError: "invalid"},
	}, nil)
	if err == nil {
		t.Fatal("NewConditionFromConfig() expected error for invalid onError, got nil")
	}
}

// TestConditionErrorDetails tests that ConditionError Details field is properly populated
func TestConditionErrorDetails(t *testing.T) {
	// Use an expression that will actually cause an evaluation error
	// Calling a method on a non-object type will cause an error
	cond, err := NewConditionFromConfig(ConditionConfig{
		Expression: "value.invalidMethod() > 100",
		ModuleBase: connector.ModuleBase{OnError: "fail"},
	}, nil)
	if err != nil {
		t.Fatalf("NewConditionFromConfig() error = %v", err)
	}

	// Create a record that will cause an evaluation error
	records := []map[string]any{
		{"value": 1}, // int doesn't have methods, will cause error
	}

	_, err = cond.Process(context.Background(), records)
	if err == nil {
		t.Fatal("Process() expected error, got nil")
	}

	// Check if it's a ConditionError
	var condErr *ConditionError
	if !errors.As(err, &condErr) {
		t.Fatalf("expected ConditionError, got %T", err)
	}

	// Verify Details field is populated
	if condErr.Details == nil {
		t.Error("expected Details to be populated, got nil")
	}

	// Verify underlying_error is in Details
	if _, ok := condErr.Details["underlying_error"]; !ok {
		t.Error("expected 'underlying_error' in Details, not found")
	}

	// Verify error_type is in Details
	if _, ok := condErr.Details["error_type"]; !ok {
		t.Error("expected 'error_type' in Details, not found")
	}

	// Verify record_index is in Details
	if idx, ok := condErr.Details["record_index"].(int); !ok || idx != 0 {
		t.Errorf("expected 'record_index' in Details to be 0, got %v", condErr.Details["record_index"])
	}
}

// TestConditionToBoolConversion tests that expressions returning different types are correctly converted to boolean
func TestConditionToBoolConversion(t *testing.T) {
	tests := []struct {
		name     string
		expr     string
		record   map[string]any
		expected bool
	}{
		{"bool true", "true", map[string]any{}, true},
		{"bool false", "false", map[string]any{}, false},
		{"int zero", "0", map[string]any{}, false},
		{"int non-zero", "5", map[string]any{}, true},
		{"float zero", "0.0", map[string]any{}, false},
		{"float non-zero", "3.14", map[string]any{}, true},
		{"empty string", `""`, map[string]any{}, false},
		{"non-empty string", `"hello"`, map[string]any{}, true},
		{"nil value", "nil", map[string]any{}, false},
		{"empty array", "[]", map[string]any{}, false},
		{"non-empty array", "[1, 2, 3]", map[string]any{}, true},
		{"empty map", "{}", map[string]any{}, false},
		{"non-empty map", `{"key": "value"}`, map[string]any{}, true},
		{"field with empty array", "arr", map[string]any{"arr": []any{}}, false},
		{"field with non-empty array", "arr", map[string]any{"arr": []any{1, 2}}, true},
		{"field with empty map", "m", map[string]any{"m": map[string]any{}}, false},
		{"field with non-empty map", "m", map[string]any{"m": map[string]any{"k": "v"}}, true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cond, err := NewConditionFromConfig(ConditionConfig{
				Expression: tt.expr,
				Else:       []*NestedModuleConfig{{Type: "drop"}},
			}, nil)
			if err != nil {
				t.Fatalf("NewConditionFromConfig() error = %v", err)
			}

			result, err := cond.Process(context.Background(), []map[string]any{tt.record})
			if err != nil {
				t.Fatalf("Process() error = %v", err)
			}

			got := len(result) > 0
			if got != tt.expected {
				t.Errorf("toBool conversion: expected %v, got %v (result length: %d)", tt.expected, got, len(result))
			}
		})
	}
}

// TestConditionInvalidExpressionError tests invalid expression detection
func TestConditionInvalidExpressionError(t *testing.T) {
	_, err := NewConditionFromConfig(ConditionConfig{
		Expression: "status = 'active'", // Invalid: single = instead of ==
	}, nil)
	if err == nil {
		t.Fatal("expected error for invalid expression")
	}

	// Error should wrap ErrInvalidExpression
	if !errors.Is(err, ErrInvalidExpression) {
		t.Errorf("expected ErrInvalidExpression, got %v", err)
	}
}

// ===========================================================================
// Task 6: Integration Tests
// ===========================================================================

// TestConditionImplementsModule verifies ConditionModule implements filter.Module interface
func TestConditionImplementsModule(t *testing.T) {
	cond, err := NewConditionFromConfig(ConditionConfig{
		Expression: "status == 'active'",
	}, nil)
	if err != nil {
		t.Fatalf("NewConditionFromConfig() error = %v", err)
	}

	// Verify it implements the Module interface
	var _ Module = cond
}

// TestConditionOutputFormat verifies output format is []map[string]any
func TestConditionOutputFormat(t *testing.T) {
	cond, err := NewConditionFromConfig(ConditionConfig{
		Expression: "status == 'active'",
	}, nil)
	if err != nil {
		t.Fatalf("NewConditionFromConfig() error = %v", err)
	}

	records := []map[string]any{
		{"id": 1, "status": "active", "data": map[string]any{"nested": "value"}},
	}

	result, err := cond.Process(context.Background(), records)
	if err != nil {
		t.Fatalf("Process() error = %v", err)
	}

	// Verify output type
	if result == nil {
		t.Fatal("expected non-nil result")
	}

	// Verify records preserved
	if len(result) != 1 {
		t.Fatalf("expected 1 record, got %d", len(result))
	}

	// Verify original data is preserved
	if result[0]["id"] != 1 {
		t.Errorf("expected id=1, got %v", result[0]["id"])
	}
	if result[0]["status"] != "active" {
		t.Errorf("expected status='active', got %v", result[0]["status"])
	}

	// Verify nested data is preserved
	if data, ok := result[0]["data"].(map[string]any); !ok || data["nested"] != "value" {
		t.Errorf("expected nested data to be preserved, got %v", result[0]["data"])
	}
}

// TestConditionChainWithMapping tests chaining condition with mapping module
func TestConditionChainWithMapping(t *testing.T) {
	// First apply mapping
	mapper, err := NewMappingFromConfig([]FieldMapping{
		{Source: strPtrCond("status"), Target: "currentStatus"},
		{Source: strPtrCond("amount"), Target: "value"},
	}, "fail")
	if err != nil {
		t.Fatalf("NewMappingFromConfig() error = %v", err)
	}

	// Then apply condition on mapped output
	cond, err := NewConditionFromConfig(ConditionConfig{
		Expression: "value > 100",
		Else:       []*NestedModuleConfig{{Type: "drop"}},
	}, nil)
	if err != nil {
		t.Fatalf("NewConditionFromConfig() error = %v", err)
	}

	records := []map[string]any{
		{"status": "active", "amount": 150},
		{"status": "inactive", "amount": 50},
	}

	// Chain: records → mapping → condition
	mapped, err := mapper.Process(context.Background(), records)
	if err != nil {
		t.Fatalf("mapper.Process() error = %v", err)
	}

	filtered, err := cond.Process(context.Background(), mapped)
	if err != nil {
		t.Fatalf("cond.Process() error = %v", err)
	}

	// Only record with value > 100 should pass
	if len(filtered) != 1 {
		t.Fatalf("expected 1 record after chaining, got %d", len(filtered))
	}

	if filtered[0]["value"] != 150 {
		t.Errorf("expected value=150, got %v", filtered[0]["value"])
	}
}

// TestConditionEmptyConditionBehavior tests behavior with empty input
func TestConditionEmptyConditionBehavior(t *testing.T) {
	cond, err := NewConditionFromConfig(ConditionConfig{
		Expression: "true", // Always true condition
	}, nil)
	if err != nil {
		t.Fatalf("NewConditionFromConfig() error = %v", err)
	}

	// Empty input should return empty output
	result, err := cond.Process(context.Background(), []map[string]any{})
	if err != nil {
		t.Fatalf("Process() error = %v", err)
	}

	if len(result) != 0 {
		t.Errorf("expected 0 records for empty input, got %d", len(result))
	}
}

// TestConditionPreservesRecordOrder tests that record order is preserved
func TestConditionPreservesRecordOrder(t *testing.T) {
	cond, err := NewConditionFromConfig(ConditionConfig{
		Expression: "keep == true",
		Else:       []*NestedModuleConfig{{Type: "drop"}},
	}, nil)
	if err != nil {
		t.Fatalf("NewConditionFromConfig() error = %v", err)
	}

	records := []map[string]any{
		{"id": 1, "keep": true},
		{"id": 2, "keep": false},
		{"id": 3, "keep": true},
		{"id": 4, "keep": false},
		{"id": 5, "keep": true},
	}

	result, err := cond.Process(context.Background(), records)
	if err != nil {
		t.Fatalf("Process() error = %v", err)
	}

	if len(result) != 3 {
		t.Fatalf("expected 3 records, got %d", len(result))
	}

	// Verify order is preserved
	expectedIDs := []int{1, 3, 5}
	for i, r := range result {
		if r["id"] != expectedIDs[i] {
			t.Errorf("expected id=%d at position %d, got %v", expectedIDs[i], i, r["id"])
		}
	}
}

// ===========================================================================
// Task 7: Deterministic Execution Tests
// ===========================================================================

// TestConditionDeterministicSameInputSameOutput verifies same input produces same output
func TestConditionDeterministicSameInputSameOutput(t *testing.T) {
	cond, err := NewConditionFromConfig(ConditionConfig{
		Expression: "status == 'active' && amount > 50",
		Else:       []*NestedModuleConfig{{Type: "drop"}},
	}, nil)
	if err != nil {
		t.Fatalf("NewConditionFromConfig() error = %v", err)
	}

	records := []map[string]any{
		{"id": 1, "status": "active", "amount": 100},
		{"id": 2, "status": "inactive", "amount": 100},
		{"id": 3, "status": "active", "amount": 25},
		{"id": 4, "status": "active", "amount": 75},
	}

	// Run multiple times and verify same results
	for run := 0; run < 10; run++ {
		result, err := cond.Process(context.Background(), records)
		if err != nil {
			t.Fatalf("Process() error on run %d = %v", run, err)
		}

		if len(result) != 2 {
			t.Errorf("run %d: expected 2 records, got %d", run, len(result))
		}

		// Verify exact same records in same order
		if result[0]["id"] != 1 {
			t.Errorf("run %d: expected first result id=1, got %v", run, result[0]["id"])
		}
		if result[1]["id"] != 4 {
			t.Errorf("run %d: expected second result id=4, got %v", run, result[1]["id"])
		}
	}
}

// TestConditionDeterministicEvaluationConsistency verifies expression evaluation is consistent
func TestConditionDeterministicEvaluationConsistency(t *testing.T) {
	expressions := []string{
		"a == 1",
		"a > b",
		"status == 'active' && qty > 0",
		"(x || y) && z",
		`name contains "test"`,
	}

	for _, expr := range expressions {
		t.Run(expr, func(t *testing.T) {
			cond, err := NewConditionFromConfig(ConditionConfig{
				Expression: expr,
				Else:       []*NestedModuleConfig{{Type: "drop"}},
			}, nil)
			if err != nil {
				t.Fatalf("NewConditionFromConfig() error = %v", err)
			}

			record := map[string]any{
				"a":      1,
				"b":      0,
				"status": "active",
				"qty":    5,
				"x":      true,
				"y":      false,
				"z":      true,
				"name":   "test-value",
			}

			// Evaluate the same record 100 times
			var firstResult []map[string]any
			for i := 0; i < 100; i++ {
				result, err := cond.Process(context.Background(), []map[string]any{record})
				if err != nil {
					t.Fatalf("Process() error on iteration %d = %v", i, err)
				}

				if i == 0 {
					firstResult = result
				} else {
					// Compare with first result
					if len(result) != len(firstResult) {
						t.Errorf("iteration %d: result length %d != first result length %d",
							i, len(result), len(firstResult))
					}
				}
			}
		})
	}
}

// TestConditionNoTimeDependent verifies no time-dependent behavior
func TestConditionNoTimeDependent(t *testing.T) {
	// This test verifies that condition evaluation doesn't depend on time
	// by running the same evaluation multiple times over a period
	cond, err := NewConditionFromConfig(ConditionConfig{
		Expression: "status == 'active'",
	}, nil)
	if err != nil {
		t.Fatalf("NewConditionFromConfig() error = %v", err)
	}

	record := map[string]any{"status": "active"}

	// Run many iterations quickly - if there was time-dependent behavior,
	// results might vary
	results := make([]int, 1000)
	for i := 0; i < 1000; i++ {
		result, err := cond.Process(context.Background(), []map[string]any{record})
		if err != nil {
			t.Fatalf("Process() error = %v", err)
		}
		results[i] = len(result)
	}

	// All results should be identical
	for i, r := range results {
		if r != 1 {
			t.Errorf("iteration %d: expected 1 record, got %d", i, r)
		}
	}
}

// TestConditionDeterministicErrorHandling verifies error handling is deterministic
func TestConditionDeterministicErrorHandling(t *testing.T) {
	// Create a condition with invalid expression to test error consistency
	_, err := NewConditionFromConfig(ConditionConfig{
		Expression: "status = 'active'", // Invalid expression - should always error
	}, nil)

	// Run 50 times to verify error is consistently returned
	for i := 0; i < 50; i++ {
		_, testErr := NewConditionFromConfig(ConditionConfig{
			Expression: "status = 'active'",
		}, nil)

		if testErr == nil {
			t.Errorf("iteration %d: expected error for invalid expression, got nil", i)
		}

		// Error should be consistent
		if !errors.Is(testErr, ErrInvalidExpression) {
			t.Errorf("iteration %d: expected ErrInvalidExpression, got %v", i, testErr)
		}
	}

	// Verify first error was also ErrInvalidExpression
	if !errors.Is(err, ErrInvalidExpression) {
		t.Errorf("first error: expected ErrInvalidExpression, got %v", err)
	}
}

// TestConditionNestedDisabledModuleSkipped ensures nested then/else filters
// with enabled:false are skipped at construction time, mirroring the top-level
// filter behavior in internal/factory.
func TestConditionNestedDisabledModuleSkipped(t *testing.T) {
	disabled := false
	enabled := true
	cond, err := NewConditionFromConfig(ConditionConfig{
		Expression: "true",
		Then: []*NestedModuleConfig{
			{Type: "set", Enabled: &enabled, Config: map[string]any{"target": "kept", "value": 1}},
			{Type: "set", Enabled: &disabled, Config: map[string]any{"target": "skipped", "value": 2}},
			{Type: "set", Config: map[string]any{"target": "default_enabled", "value": 3}},
		},
	}, func(cfg *NestedModuleConfig, _ int) (Module, error) {
		target, _ := cfg.Config["target"].(string)
		return NewSetFromConfig(SetConfig{
			Target:   target,
			Value:    cfg.Config["value"],
			HasValue: true,
		})
	})
	if err != nil {
		t.Fatalf("NewConditionFromConfig() error = %v", err)
	}

	out, err := cond.Process(context.Background(), []map[string]any{{}})
	if err != nil {
		t.Fatalf("Process error: %v", err)
	}
	if len(out) != 1 {
		t.Fatalf("expected 1 record, got %d", len(out))
	}
	rec := out[0]
	if _, ok := rec["kept"]; !ok {
		t.Error("expected 'kept' field set by enabled nested filter")
	}
	if _, ok := rec["default_enabled"]; !ok {
		t.Error("expected 'default_enabled' field set by filter without explicit enabled")
	}
	if _, ok := rec["skipped"]; ok {
		t.Error("expected 'skipped' field NOT to be set (filter had enabled:false)")
	}
}
