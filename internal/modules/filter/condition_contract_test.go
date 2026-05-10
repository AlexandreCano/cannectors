package filter

import (
	"context"
	"errors"
	"testing"
)

// TestDrop_DiscardsAllRecords locks Story 24.9 AC8.
func TestDrop_DiscardsAllRecords(t *testing.T) {
	d := NewDrop()
	out, err := d.Process(context.Background(), []map[string]any{{"a": 1}, {"a": 2}})
	if err != nil {
		t.Fatalf("Process: %v", err)
	}
	if len(out) != 0 {
		t.Errorf("expected 0 records, got %d", len(out))
	}
}

// TestCondition_AbsentBranchKeepsRecord locks Story 24.9 AC7.
func TestCondition_AbsentBranchKeepsRecord(t *testing.T) {
	cond, err := NewConditionFromConfig(ConditionConfig{Expression: "value > 10"}, nil)
	if err != nil {
		t.Fatalf("NewConditionFromConfig: %v", err)
	}
	out, err := cond.Process(context.Background(), []map[string]any{
		{"value": 20}, // matches → no then → keep
		{"value": 5},  // no match → no else → keep
	})
	if err != nil {
		t.Fatalf("Process: %v", err)
	}
	if len(out) != 2 {
		t.Fatalf("expected 2 records (both branches keep), got %d", len(out))
	}
}

// TestCondition_DropInsideBranch locks Story 24.9 AC8 in context: the only
// way to remove records is via the explicit drop filter.
func TestCondition_DropInsideBranch(t *testing.T) {
	cond, err := NewConditionFromConfig(ConditionConfig{
		Expression: "value > 10",
		Else:       []*NestedModuleConfig{{Type: "drop"}},
	}, nil)
	if err != nil {
		t.Fatalf("NewConditionFromConfig: %v", err)
	}
	out, err := cond.Process(context.Background(), []map[string]any{
		{"value": 20}, // matches → kept
		{"value": 5},  // no match → else (drop)
	})
	if err != nil {
		t.Fatalf("Process: %v", err)
	}
	if len(out) != 1 {
		t.Fatalf("expected 1 record, got %d", len(out))
	}
}

// TestCondition_RejectsEmptyExpression locks Story 24.9 AC3 (runtime).
func TestCondition_RejectsEmptyExpression(t *testing.T) {
	for _, expr := range []string{"", "   ", "\t\n"} {
		_, err := NewConditionFromConfig(ConditionConfig{Expression: expr}, nil)
		if err == nil {
			t.Errorf("expected error for empty/whitespace expression %q", expr)
			continue
		}
		if !errors.Is(err, ErrEmptyExpression) {
			t.Errorf("want ErrEmptyExpression, got %v", err)
		}
	}
}

// TestCondition_RejectsInvalidExpression locks Story 24.9 AC4 — invalid
// syntax surfaces at construction.
func TestCondition_RejectsInvalidExpression(t *testing.T) {
	_, err := NewConditionFromConfig(ConditionConfig{Expression: "value =="}, nil)
	if err == nil {
		t.Fatal("expected error for invalid syntax")
	}
	if !errors.Is(err, ErrInvalidExpression) {
		t.Errorf("want ErrInvalidExpression, got %v", err)
	}
}

// TestCondition_NoArtificialDepthLimit locks Story 24.9 AC9: the runtime
// builds arbitrary-depth condition trees without rejection.
func TestCondition_NoArtificialDepthLimit(t *testing.T) {
	build := func(depth int) ConditionConfig {
		inner := &NestedModuleConfig{Type: "drop"}
		for i := 0; i < depth; i++ {
			inner = &NestedModuleConfig{
				Type:       "condition",
				Expression: "true",
				Then:       []*NestedModuleConfig{inner},
			}
		}
		return ConditionConfig{Expression: "true", Then: []*NestedModuleConfig{inner}}
	}
	if _, err := NewConditionFromConfig(build(60), nil); err != nil {
		t.Fatalf("expected 60-deep condition tree to build, got %v", err)
	}
}
