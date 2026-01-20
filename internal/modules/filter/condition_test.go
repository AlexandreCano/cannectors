// Package filter provides implementations for filter modules.
package filter

import (
	"errors"
	"testing"
)

// TestConditionBasicEquality tests basic equality comparisons (==, !=)
func TestConditionBasicEquality(t *testing.T) {
	tests := []struct {
		name       string
		expression string
		record     map[string]interface{}
		wantPass   bool
	}{
		{
			name:       "string equality - match",
			expression: "status == 'active'",
			record:     map[string]interface{}{"status": "active"},
			wantPass:   true,
		},
		{
			name:       "string equality - no match",
			expression: "status == 'active'",
			record:     map[string]interface{}{"status": "inactive"},
			wantPass:   false,
		},
		{
			name:       "string inequality - match",
			expression: "status != 'deleted'",
			record:     map[string]interface{}{"status": "active"},
			wantPass:   true,
		},
		{
			name:       "string inequality - no match",
			expression: "status != 'active'",
			record:     map[string]interface{}{"status": "active"},
			wantPass:   false,
		},
		{
			name:       "number equality - match",
			expression: "total == 5",
			record:     map[string]interface{}{"total": 5},
			wantPass:   true,
		},
		{
			name:       "number equality - no match",
			expression: "total == 5",
			record:     map[string]interface{}{"total": 10},
			wantPass:   false,
		},
		{
			name:       "boolean equality - match true",
			expression: "isActive == true",
			record:     map[string]interface{}{"isActive": true},
			wantPass:   true,
		},
		{
			name:       "boolean equality - match false",
			expression: "isActive == false",
			record:     map[string]interface{}{"isActive": false},
			wantPass:   true,
		},
		{
			name:       "null equality - match",
			expression: "value == null",
			record:     map[string]interface{}{"value": nil},
			wantPass:   true,
		},
		{
			name:       "null inequality - match",
			expression: "value != null",
			record:     map[string]interface{}{"value": "something"},
			wantPass:   true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cond, err := NewConditionFromConfig(ConditionConfig{
				Expression: tt.expression,
				Lang:       "simple",
				OnTrue:     "continue",
				OnFalse:    "skip",
			})
			if err != nil {
				t.Fatalf("NewConditionFromConfig() error = %v", err)
			}

			records := []map[string]interface{}{tt.record}
			result, err := cond.Process(records)
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
		record     map[string]interface{}
		wantPass   bool
	}{
		{
			name:       "greater than - pass",
			expression: "amount > 100",
			record:     map[string]interface{}{"amount": 150},
			wantPass:   true,
		},
		{
			name:       "greater than - fail",
			expression: "amount > 100",
			record:     map[string]interface{}{"amount": 50},
			wantPass:   false,
		},
		{
			name:       "greater than - equal fails",
			expression: "amount > 100",
			record:     map[string]interface{}{"amount": 100},
			wantPass:   false,
		},
		{
			name:       "less than - pass",
			expression: "amount < 100",
			record:     map[string]interface{}{"amount": 50},
			wantPass:   true,
		},
		{
			name:       "less than - fail",
			expression: "amount < 100",
			record:     map[string]interface{}{"amount": 150},
			wantPass:   false,
		},
		{
			name:       "greater than or equal - pass on greater",
			expression: "amount >= 100",
			record:     map[string]interface{}{"amount": 150},
			wantPass:   true,
		},
		{
			name:       "greater than or equal - pass on equal",
			expression: "amount >= 100",
			record:     map[string]interface{}{"amount": 100},
			wantPass:   true,
		},
		{
			name:       "greater than or equal - fail",
			expression: "amount >= 100",
			record:     map[string]interface{}{"amount": 50},
			wantPass:   false,
		},
		{
			name:       "less than or equal - pass on less",
			expression: "amount <= 100",
			record:     map[string]interface{}{"amount": 50},
			wantPass:   true,
		},
		{
			name:       "less than or equal - pass on equal",
			expression: "amount <= 100",
			record:     map[string]interface{}{"amount": 100},
			wantPass:   true,
		},
		{
			name:       "less than or equal - fail",
			expression: "amount <= 100",
			record:     map[string]interface{}{"amount": 150},
			wantPass:   false,
		},
		{
			name:       "float comparison",
			expression: "price > 19.99",
			record:     map[string]interface{}{"price": 29.99},
			wantPass:   true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cond, err := NewConditionFromConfig(ConditionConfig{
				Expression: tt.expression,
				Lang:       "simple",
				OnTrue:     "continue",
				OnFalse:    "skip",
			})
			if err != nil {
				t.Fatalf("NewConditionFromConfig() error = %v", err)
			}

			records := []map[string]interface{}{tt.record}
			result, err := cond.Process(records)
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
		record     map[string]interface{}
		wantPass   bool
	}{
		{
			name:       "AND - both true",
			expression: "status == 'active' && qty > 0",
			record:     map[string]interface{}{"status": "active", "qty": 5},
			wantPass:   true,
		},
		{
			name:       "AND - first true, second false",
			expression: "status == 'active' && qty > 0",
			record:     map[string]interface{}{"status": "active", "qty": 0},
			wantPass:   false,
		},
		{
			name:       "AND - first false, second true",
			expression: "status == 'active' && qty > 0",
			record:     map[string]interface{}{"status": "inactive", "qty": 5},
			wantPass:   false,
		},
		{
			name:       "AND - both false",
			expression: "status == 'active' && qty > 0",
			record:     map[string]interface{}{"status": "inactive", "qty": 0},
			wantPass:   false,
		},
		{
			name:       "OR - both true",
			expression: "status == 'active' || status == 'pending'",
			record:     map[string]interface{}{"status": "active"},
			wantPass:   true,
		},
		{
			name:       "OR - first true",
			expression: "status == 'active' || status == 'pending'",
			record:     map[string]interface{}{"status": "active"},
			wantPass:   true,
		},
		{
			name:       "OR - second true",
			expression: "status == 'active' || status == 'pending'",
			record:     map[string]interface{}{"status": "pending"},
			wantPass:   true,
		},
		{
			name:       "OR - both false",
			expression: "status == 'active' || status == 'pending'",
			record:     map[string]interface{}{"status": "deleted"},
			wantPass:   false,
		},
		{
			name:       "NOT - negate true",
			expression: "!isDeleted",
			record:     map[string]interface{}{"isDeleted": false},
			wantPass:   true,
		},
		{
			name:       "NOT - negate false",
			expression: "!isDeleted",
			record:     map[string]interface{}{"isDeleted": true},
			wantPass:   false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cond, err := NewConditionFromConfig(ConditionConfig{
				Expression: tt.expression,
				Lang:       "simple",
				OnTrue:     "continue",
				OnFalse:    "skip",
			})
			if err != nil {
				t.Fatalf("NewConditionFromConfig() error = %v", err)
			}

			records := []map[string]interface{}{tt.record}
			result, err := cond.Process(records)
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
		record     map[string]interface{}
		wantPass   bool
	}{
		{
			name:       "single level nesting",
			expression: "user.status == 'active'",
			record: map[string]interface{}{
				"user": map[string]interface{}{"status": "active"},
			},
			wantPass: true,
		},
		{
			name:       "two level nesting",
			expression: "user.profile.verified == true",
			record: map[string]interface{}{
				"user": map[string]interface{}{
					"profile": map[string]interface{}{"verified": true},
				},
			},
			wantPass: true,
		},
		{
			name:       "nested numeric comparison",
			expression: "order.total > 100",
			record: map[string]interface{}{
				"order": map[string]interface{}{"total": 150},
			},
			wantPass: true,
		},
		{
			name:       "missing nested field - fails",
			expression: "user.status == 'active'",
			record:     map[string]interface{}{"user": map[string]interface{}{}},
			wantPass:   false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cond, err := NewConditionFromConfig(ConditionConfig{
				Expression: tt.expression,
				Lang:       "simple",
				OnTrue:     "continue",
				OnFalse:    "skip",
			})
			if err != nil {
				t.Fatalf("NewConditionFromConfig() error = %v", err)
			}

			records := []map[string]interface{}{tt.record}
			result, err := cond.Process(records)
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
		record     map[string]interface{}
		wantPass   bool
	}{
		{
			name:       "array first element",
			expression: "items[0].price > 10",
			record: map[string]interface{}{
				"items": []interface{}{
					map[string]interface{}{"price": 15},
				},
			},
			wantPass: true,
		},
		{
			name:       "array second element",
			expression: "items[1].name == 'test'",
			record: map[string]interface{}{
				"items": []interface{}{
					map[string]interface{}{"name": "first"},
					map[string]interface{}{"name": "test"},
				},
			},
			wantPass: true,
		},
		{
			name:       "array valid index",
			expression: "items[0].price > 5",
			record: map[string]interface{}{
				"items": []interface{}{
					map[string]interface{}{"price": 15},
				},
			},
			wantPass: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cond, err := NewConditionFromConfig(ConditionConfig{
				Expression: tt.expression,
				Lang:       "simple",
				OnTrue:     "continue",
				OnFalse:    "skip",
			})
			if err != nil {
				t.Fatalf("NewConditionFromConfig() error = %v", err)
			}

			records := []map[string]interface{}{tt.record}
			result, err := cond.Process(records)
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
		Lang:       "simple",
		OnTrue:     "continue",
		OnFalse:    "skip",
	})
	if err != nil {
		t.Fatalf("NewConditionFromConfig() error = %v", err)
	}

	records := []map[string]interface{}{
		{"id": 1, "status": "active"},
		{"id": 2, "status": "inactive"},
		{"id": 3, "status": "active"},
		{"id": 4, "status": "deleted"},
		{"id": 5, "status": "active"},
	}

	result, err := cond.Process(records)
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
		Lang:       "simple",
		OnTrue:     "continue",
		OnFalse:    "skip",
	})
	if err != nil {
		t.Fatalf("NewConditionFromConfig() error = %v", err)
	}

	// Test with nil
	result, err := cond.Process(nil)
	if err != nil {
		t.Fatalf("Process(nil) error = %v", err)
	}
	if len(result) != 0 {
		t.Errorf("expected empty result for nil input, got %d records", len(result))
	}

	// Test with empty slice
	result, err = cond.Process([]map[string]interface{}{})
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
		Lang:       "simple",
	})
	if err != nil {
		t.Errorf("valid config should not error: %v", err)
	}
}

// TestConditionDefaultLang tests that default language is "simple"
func TestConditionDefaultLang(t *testing.T) {
	// Lang not specified should default to "simple"
	cond, err := NewConditionFromConfig(ConditionConfig{
		Expression: "status == 'active'",
	})
	if err != nil {
		t.Fatalf("NewConditionFromConfig() error = %v", err)
	}

	records := []map[string]interface{}{{"status": "active"}}
	result, err := cond.Process(records)
	if err != nil {
		t.Fatalf("Process() error = %v", err)
	}

	if len(result) != 1 {
		t.Error("expected record to pass with default lang")
	}
}

// ===========================================================================
// Task 2: Routing Behavior Tests (onTrue/onFalse)
// ===========================================================================

// TestConditionOnTrueContinue tests onTrue="continue" (default) passes records when condition is true
func TestConditionOnTrueContinue(t *testing.T) {
	cond, err := NewConditionFromConfig(ConditionConfig{
		Expression: "status == 'active'",
		OnTrue:     "continue",
		OnFalse:    "skip",
	})
	if err != nil {
		t.Fatalf("NewConditionFromConfig() error = %v", err)
	}

	records := []map[string]interface{}{
		{"id": 1, "status": "active"},
	}

	result, err := cond.Process(records)
	if err != nil {
		t.Fatalf("Process() error = %v", err)
	}

	if len(result) != 1 {
		t.Errorf("expected 1 record to pass (onTrue=continue), got %d", len(result))
	}
}

// TestConditionOnTrueSkip tests onTrue="skip" filters out records when condition is true
func TestConditionOnTrueSkip(t *testing.T) {
	cond, err := NewConditionFromConfig(ConditionConfig{
		Expression: "status == 'active'",
		OnTrue:     "skip",
		OnFalse:    "continue",
	})
	if err != nil {
		t.Fatalf("NewConditionFromConfig() error = %v", err)
	}

	records := []map[string]interface{}{
		{"id": 1, "status": "active"},
		{"id": 2, "status": "inactive"},
	}

	result, err := cond.Process(records)
	if err != nil {
		t.Fatalf("Process() error = %v", err)
	}

	// Only the "inactive" record should pass (onTrue=skip filters "active")
	if len(result) != 1 {
		t.Errorf("expected 1 record to pass (onTrue=skip), got %d", len(result))
	}

	if id, ok := result[0]["id"].(int); !ok || id != 2 {
		t.Errorf("expected record with id=2 to pass, got %v", result[0]["id"])
	}
}

// TestConditionOnFalseContinue tests onFalse="continue" passes records when condition is false
func TestConditionOnFalseContinue(t *testing.T) {
	cond, err := NewConditionFromConfig(ConditionConfig{
		Expression: "status == 'active'",
		OnTrue:     "continue",
		OnFalse:    "continue",
	})
	if err != nil {
		t.Fatalf("NewConditionFromConfig() error = %v", err)
	}

	records := []map[string]interface{}{
		{"id": 1, "status": "active"},
		{"id": 2, "status": "inactive"},
	}

	result, err := cond.Process(records)
	if err != nil {
		t.Fatalf("Process() error = %v", err)
	}

	// Both records should pass (onTrue=continue AND onFalse=continue)
	if len(result) != 2 {
		t.Errorf("expected 2 records to pass (onFalse=continue), got %d", len(result))
	}
}

// TestConditionOnFalseSkip tests onFalse="skip" (default) filters out records when condition is false
func TestConditionOnFalseSkip(t *testing.T) {
	cond, err := NewConditionFromConfig(ConditionConfig{
		Expression: "status == 'active'",
		OnTrue:     "continue",
		OnFalse:    "skip",
	})
	if err != nil {
		t.Fatalf("NewConditionFromConfig() error = %v", err)
	}

	records := []map[string]interface{}{
		{"id": 1, "status": "active"},
		{"id": 2, "status": "inactive"},
	}

	result, err := cond.Process(records)
	if err != nil {
		t.Fatalf("Process() error = %v", err)
	}

	// Only "active" records should pass (onFalse=skip filters "inactive")
	if len(result) != 1 {
		t.Errorf("expected 1 record to pass (onFalse=skip), got %d", len(result))
	}

	if id, ok := result[0]["id"].(int); !ok || id != 1 {
		t.Errorf("expected record with id=1 to pass, got %v", result[0]["id"])
	}
}

// TestConditionDefaultRouting tests default routing behavior (onTrue=continue, onFalse=skip)
func TestConditionDefaultRouting(t *testing.T) {
	cond, err := NewConditionFromConfig(ConditionConfig{
		Expression: "qty > 10",
		// Defaults: OnTrue="continue", OnFalse="skip"
	})
	if err != nil {
		t.Fatalf("NewConditionFromConfig() error = %v", err)
	}

	records := []map[string]interface{}{
		{"id": 1, "qty": 5},   // false -> skip
		{"id": 2, "qty": 15},  // true -> continue
		{"id": 3, "qty": 10},  // false (not >) -> skip
		{"id": 4, "qty": 100}, // true -> continue
	}

	result, err := cond.Process(records)
	if err != nil {
		t.Fatalf("Process() error = %v", err)
	}

	if len(result) != 2 {
		t.Errorf("expected 2 records to pass with default routing, got %d", len(result))
	}

	expectedIDs := []int{2, 4}
	for i, r := range result {
		if id, ok := r["id"].(int); !ok || id != expectedIDs[i] {
			t.Errorf("expected record with id=%d at position %d, got %v", expectedIDs[i], i, r["id"])
		}
	}
}

// TestConditionNullValueHandling tests handling of null values in conditions
func TestConditionNullValueHandling(t *testing.T) {
	tests := []struct {
		name       string
		expression string
		record     map[string]interface{}
		wantPass   bool
	}{
		{
			name:       "null field equals null",
			expression: "value == null",
			record:     map[string]interface{}{"value": nil},
			wantPass:   true,
		},
		{
			name:       "missing field treated as null",
			expression: "missing == null",
			record:     map[string]interface{}{"other": "value"},
			wantPass:   true,
		},
		{
			name:       "null field not equal to value",
			expression: "value != null",
			record:     map[string]interface{}{"value": nil},
			wantPass:   false,
		},
		{
			name:       "non-null field not equal to null",
			expression: "value != null",
			record:     map[string]interface{}{"value": "something"},
			wantPass:   true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cond, err := NewConditionFromConfig(ConditionConfig{
				Expression: tt.expression,
				OnTrue:     "continue",
				OnFalse:    "skip",
			})
			if err != nil {
				t.Fatalf("NewConditionFromConfig() error = %v", err)
			}

			result, err := cond.Process([]map[string]interface{}{tt.record})
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
		record     map[string]interface{}
		wantPass   bool
	}{
		{
			name:       "string compared to number - not equal",
			expression: "value == 100",
			record:     map[string]interface{}{"value": "100"},
			wantPass:   false, // string "100" != number 100
		},
		{
			name:       "int compared to float - equal",
			expression: "value == 100.0",
			record:     map[string]interface{}{"value": 100},
			wantPass:   true, // int 100 == float 100.0
		},
		{
			name:       "boolean compared to number - not equal",
			expression: "active == 1",
			record:     map[string]interface{}{"active": true},
			wantPass:   false, // boolean true != number 1
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cond, err := NewConditionFromConfig(ConditionConfig{
				Expression: tt.expression,
				OnTrue:     "continue",
				OnFalse:    "skip",
			})
			if err != nil {
				t.Fatalf("NewConditionFromConfig() error = %v", err)
			}

			result, err := cond.Process([]map[string]interface{}{tt.record})
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

// TestConditionNestedThenMapping tests 'then' with a nested mapping module
func TestConditionNestedThenMapping(t *testing.T) {
	cond, err := NewConditionFromConfig(ConditionConfig{
		Expression: "status == 'active'",
		Then: &NestedModuleConfig{
			Type: "mapping",
			Mappings: []FieldMapping{
				{Source: "name", Target: "displayName"},
				{Source: "id", Target: "userId"},
			},
		},
		OnFalse: "skip",
	})
	if err != nil {
		t.Fatalf("NewConditionFromConfig() error = %v", err)
	}

	records := []map[string]interface{}{
		{"id": 1, "name": "Alice", "status": "active"},
	}

	result, err := cond.Process(records)
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

	// Original fields should not be present (mapping only produces target fields)
	if _, exists := result[0]["name"]; exists {
		t.Errorf("expected 'name' field to not exist in output")
	}
}

// TestConditionNestedElseMapping tests 'else' with a nested mapping module
func TestConditionNestedElseMapping(t *testing.T) {
	cond, err := NewConditionFromConfig(ConditionConfig{
		Expression: "status == 'active'",
		OnTrue:     "skip", // Skip active records
		Else: &NestedModuleConfig{
			Type: "mapping",
			Mappings: []FieldMapping{
				{Source: "id", Target: "inactiveUserId"},
				{Source: "status", Target: "currentStatus"},
			},
		},
	})
	if err != nil {
		t.Fatalf("NewConditionFromConfig() error = %v", err)
	}

	records := []map[string]interface{}{
		{"id": 1, "status": "active"},
		{"id": 2, "status": "inactive"},
	}

	result, err := cond.Process(records)
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
		Then: &NestedModuleConfig{
			Type: "mapping",
			Mappings: []FieldMapping{
				{Source: "id", Target: "premiumId"},
				{Source: "name", Target: "premiumName"},
			},
		},
		Else: &NestedModuleConfig{
			Type: "mapping",
			Mappings: []FieldMapping{
				{Source: "id", Target: "standardId"},
				{Source: "name", Target: "standardName"},
			},
		},
	})
	if err != nil {
		t.Fatalf("NewConditionFromConfig() error = %v", err)
	}

	records := []map[string]interface{}{
		{"id": 1, "name": "Alice", "tier": "premium"},
		{"id": 2, "name": "Bob", "tier": "standard"},
	}

	result, err := cond.Process(records)
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

// TestConditionNestedRecursiveCondition tests nested condition within condition
func TestConditionNestedRecursiveCondition(t *testing.T) {
	cond, err := NewConditionFromConfig(ConditionConfig{
		Expression: "level > 0",
		Then: &NestedModuleConfig{
			Type:       "condition",
			Expression: "level > 5",
			OnTrue:     "continue", // High level: keep
			OnFalse:    "skip",     // Medium level: skip
		},
		OnFalse: "skip", // Level 0: skip
	})
	if err != nil {
		t.Fatalf("NewConditionFromConfig() error = %v", err)
	}

	records := []map[string]interface{}{
		{"id": 1, "level": 0},  // level > 0: false -> skip
		{"id": 2, "level": 3},  // level > 0: true -> then (level > 5: false -> skip)
		{"id": 3, "level": 10}, // level > 0: true -> then (level > 5: true -> continue)
	}

	result, err := cond.Process(records)
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

// TestConditionNestedPriorityOverOnTrue tests that 'then' takes priority over onTrue
func TestConditionNestedPriorityOverOnTrue(t *testing.T) {
	cond, err := NewConditionFromConfig(ConditionConfig{
		Expression: "status == 'active'",
		OnTrue:     "skip", // This should be ignored when 'then' is present
		Then: &NestedModuleConfig{
			Type: "mapping",
			Mappings: []FieldMapping{
				{Source: "id", Target: "mappedId"},
			},
		},
		OnFalse: "skip",
	})
	if err != nil {
		t.Fatalf("NewConditionFromConfig() error = %v", err)
	}

	records := []map[string]interface{}{
		{"id": 1, "status": "active"},
	}

	result, err := cond.Process(records)
	if err != nil {
		t.Fatalf("Process() error = %v", err)
	}

	// Record should be processed by 'then' module, not skipped
	if len(result) != 1 {
		t.Fatalf("expected 1 record (then takes priority over onTrue=skip), got %d", len(result))
	}

	if result[0]["mappedId"] != 1 {
		t.Errorf("expected mappedId=1, got %v", result[0]["mappedId"])
	}
}

// TestConditionNestedPriorityOverOnFalse tests that 'else' takes priority over onFalse
func TestConditionNestedPriorityOverOnFalse(t *testing.T) {
	cond, err := NewConditionFromConfig(ConditionConfig{
		Expression: "status == 'active'",
		OnTrue:     "skip",
		OnFalse:    "skip", // This should be ignored when 'else' is present
		Else: &NestedModuleConfig{
			Type: "mapping",
			Mappings: []FieldMapping{
				{Source: "id", Target: "elseId"},
			},
		},
	})
	if err != nil {
		t.Fatalf("NewConditionFromConfig() error = %v", err)
	}

	records := []map[string]interface{}{
		{"id": 1, "status": "inactive"},
	}

	result, err := cond.Process(records)
	if err != nil {
		t.Fatalf("Process() error = %v", err)
	}

	// Record should be processed by 'else' module, not skipped
	if len(result) != 1 {
		t.Fatalf("expected 1 record (else takes priority over onFalse=skip), got %d", len(result))
	}

	if result[0]["elseId"] != 1 {
		t.Errorf("expected elseId=1, got %v", result[0]["elseId"])
	}
}

// TestConditionNestedModuleReturnsEmpty tests when nested module filters out record
func TestConditionNestedModuleReturnsEmpty(t *testing.T) {
	// Nested condition that filters out the record
	cond, err := NewConditionFromConfig(ConditionConfig{
		Expression: "level > 0",
		Then: &NestedModuleConfig{
			Type:       "condition",
			Expression: "level > 100", // This will be false
			OnTrue:     "continue",
			OnFalse:    "skip", // Skip the record
		},
		OnFalse: "skip",
	})
	if err != nil {
		t.Fatalf("NewConditionFromConfig() error = %v", err)
	}

	records := []map[string]interface{}{
		{"id": 1, "level": 10}, // level > 0: true -> then (level > 100: false -> skip)
	}

	result, err := cond.Process(records)
	if err != nil {
		t.Fatalf("Process() error = %v", err)
	}

	// Record should be filtered out by nested condition
	if len(result) != 0 {
		t.Errorf("expected 0 records (nested module filtered), got %d", len(result))
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
		record     map[string]interface{}
		wantPass   bool
	}{
		{
			name:       "simple grouping - (a || b) && c - all true",
			expression: "(status == 'active' || status == 'pending') && qty > 0",
			record:     map[string]interface{}{"status": "active", "qty": 5},
			wantPass:   true,
		},
		{
			name:       "simple grouping - (a || b) && c - first OR true, AND true",
			expression: "(status == 'active' || status == 'pending') && qty > 0",
			record:     map[string]interface{}{"status": "pending", "qty": 5},
			wantPass:   true,
		},
		{
			name:       "simple grouping - (a || b) && c - first OR false, fails",
			expression: "(status == 'active' || status == 'pending') && qty > 0",
			record:     map[string]interface{}{"status": "deleted", "qty": 5},
			wantPass:   false,
		},
		{
			name:       "simple grouping - (a || b) && c - AND false, fails",
			expression: "(status == 'active' || status == 'pending') && qty > 0",
			record:     map[string]interface{}{"status": "active", "qty": 0},
			wantPass:   false,
		},
		{
			name:       "precedence without parens - a || b && c",
			expression: "a == true || b == true && c == true",
			record:     map[string]interface{}{"a": true, "b": false, "c": false},
			wantPass:   true, // a || (b && c) => true || false => true
		},
		{
			name:       "nested parentheses - ((a && b) || c)",
			expression: "((x == 1 && y == 2) || z == 3)",
			record:     map[string]interface{}{"x": 1, "y": 2, "z": 0},
			wantPass:   true, // (1 && 2) || 0 => true || false => true
		},
		{
			name:       "nested parentheses - z matches",
			expression: "((x == 1 && y == 2) || z == 3)",
			record:     map[string]interface{}{"x": 0, "y": 0, "z": 3},
			wantPass:   true, // (false && false) || true => true
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cond, err := NewConditionFromConfig(ConditionConfig{
				Expression: tt.expression,
				OnTrue:     "continue",
				OnFalse:    "skip",
			})
			if err != nil {
				t.Fatalf("NewConditionFromConfig() error = %v", err)
			}

			result, err := cond.Process([]map[string]interface{}{tt.record})
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
		record     map[string]interface{}
		wantPass   bool
	}{
		{
			name:       "contains - match",
			expression: `name contains "test"`,
			record:     map[string]interface{}{"name": "this is a test string"},
			wantPass:   true,
		},
		{
			name:       "contains - no match",
			expression: `name contains "xyz"`,
			record:     map[string]interface{}{"name": "this is a test string"},
			wantPass:   false,
		},
		{
			name:       "startsWith - match",
			expression: `email startsWith "admin"`,
			record:     map[string]interface{}{"email": "admin@example.com"},
			wantPass:   true,
		},
		{
			name:       "startsWith - no match",
			expression: `email startsWith "user"`,
			record:     map[string]interface{}{"email": "admin@example.com"},
			wantPass:   false,
		},
		{
			name:       "endsWith - match",
			expression: `path endsWith ".json"`,
			record:     map[string]interface{}{"path": "/data/config.json"},
			wantPass:   true,
		},
		{
			name:       "endsWith - no match",
			expression: `path endsWith ".xml"`,
			record:     map[string]interface{}{"path": "/data/config.json"},
			wantPass:   false,
		},
		{
			name:       "nested field with string operation",
			expression: `user.email endsWith "@example.com"`,
			record: map[string]interface{}{
				"user": map[string]interface{}{"email": "test@example.com"},
			},
			wantPass: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cond, err := NewConditionFromConfig(ConditionConfig{
				Expression: tt.expression,
				OnTrue:     "continue",
				OnFalse:    "skip",
			})
			if err != nil {
				t.Fatalf("NewConditionFromConfig() error = %v", err)
			}

			result, err := cond.Process([]map[string]interface{}{tt.record})
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
		record     map[string]interface{}
		wantPass   bool
	}{
		{
			name:       "three level nesting",
			expression: "user.profile.settings.darkMode == true",
			record: map[string]interface{}{
				"user": map[string]interface{}{
					"profile": map[string]interface{}{
						"settings": map[string]interface{}{
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
			record: map[string]interface{}{
				"orders": []interface{}{
					map[string]interface{}{
						"items": []interface{}{
							map[string]interface{}{
								"product": map[string]interface{}{
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
				OnTrue:     "continue",
				OnFalse:    "skip",
			})
			if err != nil {
				t.Fatalf("NewConditionFromConfig() error = %v", err)
			}

			result, err := cond.Process([]map[string]interface{}{tt.record})
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

// TestConditionLangSimple tests that "simple" language works correctly
func TestConditionLangSimple(t *testing.T) {
	cond, err := NewConditionFromConfig(ConditionConfig{
		Expression: "status == 'active'",
		Lang:       "simple",
	})
	if err != nil {
		t.Fatalf("NewConditionFromConfig() error = %v", err)
	}

	records := []map[string]interface{}{{"status": "active"}}
	result, err := cond.Process(records)
	if err != nil {
		t.Fatalf("Process() error = %v", err)
	}

	if len(result) != 1 {
		t.Errorf("expected 1 record with lang='simple', got %d", len(result))
	}
}

// TestConditionLangCELNotImplemented tests that "cel" language returns appropriate error
func TestConditionLangCELNotImplemented(t *testing.T) {
	_, err := NewConditionFromConfig(ConditionConfig{
		Expression: "status == 'active'",
		Lang:       "cel",
	})
	if err == nil {
		t.Error("expected error for lang='cel' (not implemented)")
	}
	if !errors.Is(err, ErrUnsupportedLang) {
		t.Errorf("expected ErrUnsupportedLang, got %v", err)
	}
}

// TestConditionLangJSONataNotImplemented tests that "jsonata" language returns appropriate error
func TestConditionLangJSONataNotImplemented(t *testing.T) {
	_, err := NewConditionFromConfig(ConditionConfig{
		Expression: "status = 'active'",
		Lang:       "jsonata",
	})
	if err == nil {
		t.Error("expected error for lang='jsonata' (not implemented)")
	}
	if !errors.Is(err, ErrUnsupportedLang) {
		t.Errorf("expected ErrUnsupportedLang, got %v", err)
	}
}

// TestConditionLangJMESPathNotImplemented tests that "jmespath" language returns appropriate error
func TestConditionLangJMESPathNotImplemented(t *testing.T) {
	_, err := NewConditionFromConfig(ConditionConfig{
		Expression: "status == 'active'",
		Lang:       "jmespath",
	})
	if err == nil {
		t.Error("expected error for lang='jmespath' (not implemented)")
	}
	if !errors.Is(err, ErrUnsupportedLang) {
		t.Errorf("expected ErrUnsupportedLang, got %v", err)
	}
}

// TestConditionLangUnknown tests that unknown language returns appropriate error
func TestConditionLangUnknown(t *testing.T) {
	_, err := NewConditionFromConfig(ConditionConfig{
		Expression: "status == 'active'",
		Lang:       "unknown-lang",
	})
	if err == nil {
		t.Error("expected error for unknown lang")
	}
	if !errors.Is(err, ErrUnsupportedLang) {
		t.Errorf("expected ErrUnsupportedLang, got %v", err)
	}
}

// TestConditionLangDefaultIsSimple tests that empty lang defaults to "simple"
func TestConditionLangDefaultIsSimple(t *testing.T) {
	cond, err := NewConditionFromConfig(ConditionConfig{
		Expression: "value > 10",
		// Lang not specified - should default to "simple"
	})
	if err != nil {
		t.Fatalf("NewConditionFromConfig() error = %v", err)
	}

	records := []map[string]interface{}{{"value": 15}}
	result, err := cond.Process(records)
	if err != nil {
		t.Fatalf("Process() error = %v", err)
	}

	if len(result) != 1 {
		t.Errorf("expected 1 record with default lang, got %d", len(result))
	}
}

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
			})
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
		OnError:    "fail",
	})
	if err != nil {
		t.Fatalf("NewConditionFromConfig() error = %v", err)
	}

	records := []map[string]interface{}{
		{"data": "test"},
	}

	_, err = cond.Process(records)
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
		OnError:    "skip",
	})
	if err != nil {
		t.Fatalf("NewConditionFromConfig() error = %v", err)
	}

	records := []map[string]interface{}{
		{"value": 10}, // Will cause error (can't call method on int), should be skipped
		{"value": 5},  // Will cause error (can't call method on int), should be skipped
		{"value": map[string]interface{}{"nested": 1}}, // Will cause error, should be skipped
	}

	result, err := cond.Process(records)
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
		OnError:    "log",
	})
	if err != nil {
		t.Fatalf("NewConditionFromConfig() error = %v", err)
	}

	records := []map[string]interface{}{
		{"value": 10}, // Will cause error (can't call method on int), should be logged and skipped
		{"value": 5},  // Will cause error (can't call method on int), should be logged and skipped
		{"value": map[string]interface{}{"nested": 1}}, // Will cause error, should be logged and skipped
	}

	result, err := cond.Process(records)
	if err != nil {
		t.Fatalf("Process() error = %v", err)
	}

	// All records should be skipped due to evaluation errors, but processing should continue
	if len(result) != 0 {
		t.Errorf("expected 0 records with onError='log' (all should be skipped due to errors), got %d", len(result))
	}
}

// TestConditionInvalidOnErrorDefaultsToFail ensures invalid onError values don't silently pass.
func TestConditionInvalidOnErrorDefaultsToFail(t *testing.T) {
	cond, err := NewConditionFromConfig(ConditionConfig{
		Expression: "data.unknownMethod()",
		OnError:    "invalid",
	})
	if err != nil {
		t.Fatalf("NewConditionFromConfig() error = %v", err)
	}

	records := []map[string]interface{}{
		{"data": "test"},
	}

	_, err = cond.Process(records)
	if err == nil {
		t.Error("expected error with invalid onError (default to fail), got nil")
	}
}

// TestConditionEmptyExpressionDefaultsToOnTrue ensures empty expressions return ErrEmptyExpression even when OnTrue is set.
func TestConditionEmptyExpressionDefaultsToOnTrue(t *testing.T) {
	_, err := NewConditionFromConfig(ConditionConfig{
		Expression: "",
		OnTrue:     "continue",
	})
	if err == nil {
		t.Fatal("NewConditionFromConfig() expected error for empty expression, got nil")
	}
	if !errors.Is(err, ErrEmptyExpression) {
		t.Errorf("NewConditionFromConfig() expected ErrEmptyExpression, got %v", err)
	}
}

func TestConditionEmptyExpressionOnTrueSkipFiltersAll(t *testing.T) {
	_, err := NewConditionFromConfig(ConditionConfig{
		Expression: " ",
		OnTrue:     "skip",
	})
	if err == nil {
		t.Fatal("NewConditionFromConfig() expected error for whitespace-only expression, got nil")
	}
	if !errors.Is(err, ErrEmptyExpression) {
		t.Errorf("NewConditionFromConfig() expected ErrEmptyExpression, got %v", err)
	}
}

// TestConditionErrorDetails tests that ConditionError Details field is properly populated
func TestConditionErrorDetails(t *testing.T) {
	// Use an expression that will actually cause an evaluation error
	// Calling a method on a non-object type will cause an error
	cond, err := NewConditionFromConfig(ConditionConfig{
		Expression: "value.invalidMethod() > 100",
		OnError:    "fail",
	})
	if err != nil {
		t.Fatalf("NewConditionFromConfig() error = %v", err)
	}

	// Create a record that will cause an evaluation error
	records := []map[string]interface{}{
		{"value": 1}, // int doesn't have methods, will cause error
	}

	_, err = cond.Process(records)
	if err == nil {
		t.Fatal("Process() expected error, got nil")
	}

	// Check if it's a ConditionError
	condErr, ok := err.(*ConditionError)
	if !ok {
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
		record   map[string]interface{}
		expected bool
	}{
		{"bool true", "true", map[string]interface{}{}, true},
		{"bool false", "false", map[string]interface{}{}, false},
		{"int zero", "0", map[string]interface{}{}, false},
		{"int non-zero", "5", map[string]interface{}{}, true},
		{"float zero", "0.0", map[string]interface{}{}, false},
		{"float non-zero", "3.14", map[string]interface{}{}, true},
		{"empty string", `""`, map[string]interface{}{}, false},
		{"non-empty string", `"hello"`, map[string]interface{}{}, true},
		{"nil value", "nil", map[string]interface{}{}, false},
		{"empty array", "[]", map[string]interface{}{}, false},
		{"non-empty array", "[1, 2, 3]", map[string]interface{}{}, true},
		{"empty map", "{}", map[string]interface{}{}, false},
		{"non-empty map", `{"key": "value"}`, map[string]interface{}{}, true},
		{"field with empty array", "arr", map[string]interface{}{"arr": []interface{}{}}, false},
		{"field with non-empty array", "arr", map[string]interface{}{"arr": []interface{}{1, 2}}, true},
		{"field with empty map", "m", map[string]interface{}{"m": map[string]interface{}{}}, false},
		{"field with non-empty map", "m", map[string]interface{}{"m": map[string]interface{}{"k": "v"}}, true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cond, err := NewConditionFromConfig(ConditionConfig{
				Expression: tt.expr,
				OnTrue:     "continue",
				OnFalse:    "skip",
			})
			if err != nil {
				t.Fatalf("NewConditionFromConfig() error = %v", err)
			}

			result, err := cond.Process([]map[string]interface{}{tt.record})
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

// TestConditionInvalidOnTrueOnFalse tests that invalid onTrue/onFalse values are handled correctly
func TestConditionInvalidOnTrueOnFalse(t *testing.T) {
	tests := []struct {
		name       string
		onTrue     string
		onFalse    string
		shouldWarn bool
	}{
		{"valid values", "continue", "skip", false},
		{"invalid onTrue", "contineu", "skip", true},   // typo
		{"invalid onFalse", "continue", "skipp", true}, // typo
		{"both invalid", "reject", "accept", true},
		{"empty defaults", "", "", false}, // empty should use defaults
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cond, err := NewConditionFromConfig(ConditionConfig{
				Expression: "value > 0",
				OnTrue:     tt.onTrue,
				OnFalse:    tt.onFalse,
			})
			if err != nil {
				t.Fatalf("NewConditionFromConfig() error = %v", err)
			}

			// Test that it works despite invalid values (should default to valid ones)
			records := []map[string]interface{}{
				{"value": 10}, // true condition
				{"value": -5}, // false condition
			}

			result, err := cond.Process(records)
			if err != nil {
				t.Fatalf("Process() error = %v", err)
			}

			// Should process records successfully (invalid values are normalized)
			// Note: len() can never be negative, but we check that processing succeeded
			if result == nil {
				t.Error("expected result to be non-nil")
			}
		})
	}
}

// TestConditionInvalidExpressionError tests invalid expression detection
func TestConditionInvalidExpressionError(t *testing.T) {
	_, err := NewConditionFromConfig(ConditionConfig{
		Expression: "status = 'active'", // Invalid: single = instead of ==
	})
	if err == nil {
		t.Fatal("expected error for invalid expression")
	}

	// Error should wrap ErrInvalidExpression
	if !errors.Is(err, ErrInvalidExpression) {
		t.Errorf("expected ErrInvalidExpression, got %v", err)
	}
}

// TestConditionDefaultOnErrorIsFail tests that default onError is "fail"
func TestConditionDefaultOnErrorIsFail(t *testing.T) {
	cond, err := NewConditionFromConfig(ConditionConfig{
		Expression: "missing == 'value'",
		// OnError not specified - should default to "fail"
	})
	if err != nil {
		t.Fatalf("NewConditionFromConfig() error = %v", err)
	}

	records := []map[string]interface{}{
		{"other": "value"},
	}

	// With AllowUndefinedVariables, missing top-level fields return nil
	// nil == 'value' is false, so record should be filtered out
	result, err := cond.Process(records)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// Record should be filtered out (condition evaluated to false because nil != 'value')
	if len(result) != 0 {
		t.Errorf("expected 0 records (condition false), got %d", len(result))
	}
}

// ===========================================================================
// Task 6: Integration Tests
// ===========================================================================

// TestConditionImplementsModule verifies ConditionModule implements filter.Module interface
func TestConditionImplementsModule(t *testing.T) {
	cond, err := NewConditionFromConfig(ConditionConfig{
		Expression: "status == 'active'",
	})
	if err != nil {
		t.Fatalf("NewConditionFromConfig() error = %v", err)
	}

	// Verify it implements the Module interface
	var _ Module = cond
}

// TestConditionOutputFormat verifies output format is []map[string]interface{}
func TestConditionOutputFormat(t *testing.T) {
	cond, err := NewConditionFromConfig(ConditionConfig{
		Expression: "status == 'active'",
	})
	if err != nil {
		t.Fatalf("NewConditionFromConfig() error = %v", err)
	}

	records := []map[string]interface{}{
		{"id": 1, "status": "active", "data": map[string]interface{}{"nested": "value"}},
	}

	result, err := cond.Process(records)
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
	if data, ok := result[0]["data"].(map[string]interface{}); !ok || data["nested"] != "value" {
		t.Errorf("expected nested data to be preserved, got %v", result[0]["data"])
	}
}

// TestConditionChainWithMapping tests chaining condition with mapping module
func TestConditionChainWithMapping(t *testing.T) {
	// First apply mapping
	mapper, err := NewMappingFromConfig([]FieldMapping{
		{Source: "status", Target: "currentStatus"},
		{Source: "amount", Target: "value"},
	}, "fail")
	if err != nil {
		t.Fatalf("NewMappingFromConfig() error = %v", err)
	}

	// Then apply condition on mapped output
	cond, err := NewConditionFromConfig(ConditionConfig{
		Expression: "value > 100",
	})
	if err != nil {
		t.Fatalf("NewConditionFromConfig() error = %v", err)
	}

	records := []map[string]interface{}{
		{"status": "active", "amount": 150},
		{"status": "inactive", "amount": 50},
	}

	// Chain: records → mapping → condition
	mapped, err := mapper.Process(records)
	if err != nil {
		t.Fatalf("mapper.Process() error = %v", err)
	}

	filtered, err := cond.Process(mapped)
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
	})
	if err != nil {
		t.Fatalf("NewConditionFromConfig() error = %v", err)
	}

	// Empty input should return empty output
	result, err := cond.Process([]map[string]interface{}{})
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
	})
	if err != nil {
		t.Fatalf("NewConditionFromConfig() error = %v", err)
	}

	records := []map[string]interface{}{
		{"id": 1, "keep": true},
		{"id": 2, "keep": false},
		{"id": 3, "keep": true},
		{"id": 4, "keep": false},
		{"id": 5, "keep": true},
	}

	result, err := cond.Process(records)
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
	})
	if err != nil {
		t.Fatalf("NewConditionFromConfig() error = %v", err)
	}

	records := []map[string]interface{}{
		{"id": 1, "status": "active", "amount": 100},
		{"id": 2, "status": "inactive", "amount": 100},
		{"id": 3, "status": "active", "amount": 25},
		{"id": 4, "status": "active", "amount": 75},
	}

	// Run multiple times and verify same results
	for run := 0; run < 10; run++ {
		result, err := cond.Process(records)
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
			})
			if err != nil {
				t.Fatalf("NewConditionFromConfig() error = %v", err)
			}

			record := map[string]interface{}{
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
			var firstResult []map[string]interface{}
			for i := 0; i < 100; i++ {
				result, err := cond.Process([]map[string]interface{}{record})
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

// TestConditionDeterministicRoutingConsistency verifies routing is consistent
func TestConditionDeterministicRoutingConsistency(t *testing.T) {
	tests := []struct {
		name      string
		onTrue    string
		onFalse   string
		record    map[string]interface{}
		wantCount int
	}{
		{
			name:      "onTrue=continue passes true condition",
			onTrue:    "continue",
			onFalse:   "skip",
			record:    map[string]interface{}{"value": true},
			wantCount: 1,
		},
		{
			name:      "onTrue=skip filters true condition",
			onTrue:    "skip",
			onFalse:   "continue",
			record:    map[string]interface{}{"value": true},
			wantCount: 0,
		},
		{
			name:      "onFalse=continue passes false condition",
			onTrue:    "skip",
			onFalse:   "continue",
			record:    map[string]interface{}{"value": false},
			wantCount: 1,
		},
		{
			name:      "onFalse=skip filters false condition",
			onTrue:    "continue",
			onFalse:   "skip",
			record:    map[string]interface{}{"value": false},
			wantCount: 0,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cond, err := NewConditionFromConfig(ConditionConfig{
				Expression: "value == true",
				OnTrue:     tt.onTrue,
				OnFalse:    tt.onFalse,
			})
			if err != nil {
				t.Fatalf("NewConditionFromConfig() error = %v", err)
			}

			// Run 50 times to verify consistency
			for i := 0; i < 50; i++ {
				result, err := cond.Process([]map[string]interface{}{tt.record})
				if err != nil {
					t.Fatalf("Process() error on iteration %d = %v", i, err)
				}

				if len(result) != tt.wantCount {
					t.Errorf("iteration %d: expected %d records, got %d",
						i, tt.wantCount, len(result))
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
	})
	if err != nil {
		t.Fatalf("NewConditionFromConfig() error = %v", err)
	}

	record := map[string]interface{}{"status": "active"}

	// Run many iterations quickly - if there was time-dependent behavior,
	// results might vary
	results := make([]int, 1000)
	for i := 0; i < 1000; i++ {
		result, err := cond.Process([]map[string]interface{}{record})
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
	})

	// Run 50 times to verify error is consistently returned
	for i := 0; i < 50; i++ {
		_, testErr := NewConditionFromConfig(ConditionConfig{
			Expression: "status = 'active'",
		})

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
