package filter

import (
	"errors"
	"strings"
	"testing"
)

func sp(s string) *string { return &s }

// TestMapping_RejectsUnknownOnMissing locks Story 24.8 AC3.
func TestMapping_RejectsUnknownOnMissing(t *testing.T) {
	_, err := NewMappingFromConfig([]FieldMapping{{Source: sp("a"), Target: "b", OnMissing: "bogus"}}, "fail")
	if err == nil {
		t.Fatal("expected error for unknown onMissing")
	}
	if !errors.Is(err, ErrInvalidMapping) {
		t.Errorf("want ErrInvalidMapping, got %v", err)
	}
	if !strings.Contains(err.Error(), "unknown onMissing") {
		t.Errorf("error message: %v", err)
	}
}

// TestMapping_UseDefaultRequiresDefaultValue locks Story 24.8 AC4 (runtime).
func TestMapping_UseDefaultRequiresDefaultValue(t *testing.T) {
	_, err := NewMappingFromConfig([]FieldMapping{{Source: sp("a"), Target: "b", OnMissing: OnMissingUseDefault}}, "fail")
	if err == nil {
		t.Fatal("expected error for useDefault without defaultValue")
	}
	if !errors.Is(err, ErrInvalidMapping) {
		t.Errorf("want ErrInvalidMapping, got %v", err)
	}
}

// TestMapping_UseDefaultAcceptsExplicitNull locks Story 24.8 AC4: an
// explicitly-provided null defaultValue is a valid configuration; only an
// absent defaultValue should be rejected.
func TestMapping_UseDefaultAcceptsExplicitNull(t *testing.T) {
	mappings, err := ParseFieldMappings([]any{
		map[string]any{
			"source":       "a",
			"target":       "b",
			"onMissing":    "useDefault",
			"defaultValue": nil,
		},
	})
	if err != nil {
		t.Fatalf("ParseFieldMappings: %v", err)
	}
	if _, err := NewMappingFromConfig(mappings, "fail"); err != nil {
		t.Fatalf("expected explicit null defaultValue to be accepted, got %v", err)
	}
}

// TestMapping_RejectsUnknownTransformOp locks Story 24.8 AC5 at runtime.
func TestMapping_RejectsUnknownTransformOp(t *testing.T) {
	_, err := NewMappingFromConfig([]FieldMapping{{
		Source: sp("a"), Target: "b",
		Transforms: []TransformOp{{Op: "bogus"}},
	}}, "fail")
	if err == nil {
		t.Fatal("expected error for unknown transform op")
	}
	if !errors.Is(err, ErrInvalidMapping) {
		t.Errorf("want ErrInvalidMapping, got %v", err)
	}
}

// TestMapping_ReplaceRequiresPattern locks Story 24.8 AC6 at runtime.
func TestMapping_ReplaceRequiresPattern(t *testing.T) {
	_, err := NewMappingFromConfig([]FieldMapping{{
		Source: sp("a"), Target: "b",
		Transforms: []TransformOp{{Op: "replace"}},
	}}, "fail")
	if err == nil {
		t.Fatal("expected error for replace without pattern")
	}
	if !errors.Is(err, ErrInvalidMapping) {
		t.Errorf("want ErrInvalidMapping, got %v", err)
	}
}
