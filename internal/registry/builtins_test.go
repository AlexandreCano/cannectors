package registry

import (
	"context"
	"testing"

	"github.com/cannectors/runtime/internal/modules/filter"
)

// TestResolveNestedFilter_PreservesMappingConfig locks the regression flagged
// during Story 24.9 review: nested mapping configs were re-emitted with only
// source/target, dropping transforms / onMissing / defaultValue before reaching
// NewMappingFromConfig. The resolver must serialize the full FieldMapping.
func TestResolveNestedFilter_PreservesMappingConfig(t *testing.T) {
	src := "name"
	nested := &filter.NestedModuleConfig{
		Type: "mapping",
		Mappings: []filter.FieldMapping{{
			Source:     &src,
			Target:     "name_lower",
			OnMissing:  "fail",
			Transforms: []filter.TransformOp{{Op: "lowercase"}},
		}},
	}

	module, err := resolveNestedFilter(nested, 0)
	if err != nil {
		t.Fatalf("resolveNestedFilter: %v", err)
	}

	out, err := module.Process(context.Background(), []map[string]any{{"name": "ALICE"}})
	if err != nil {
		t.Fatalf("Process: %v", err)
	}
	if len(out) != 1 || out[0]["name_lower"] != "alice" {
		t.Fatalf("expected lowercase transform applied, got %v", out)
	}

	// onMissing:fail should now surface (proves OnMissing reached the constructor).
	if _, err := module.Process(context.Background(), []map[string]any{{}}); err == nil {
		t.Fatal("expected error from onMissing:fail when source field is absent")
	}
}

// TestResolveNestedFilter_PreservesExplicitNullDefault locks the round-trip
// of FieldMapping.HasDefaultValue (cf. Story 24.8 fix) through the registry
// resolver: defaultValue:null with onMissing:useDefault must reach the
// nested mapping constructor as a present-but-null value.
func TestResolveNestedFilter_PreservesExplicitNullDefault(t *testing.T) {
	src := "missing"
	nested := &filter.NestedModuleConfig{
		Type: "mapping",
		Mappings: []filter.FieldMapping{{
			Source:          &src,
			Target:          "out",
			OnMissing:       "useDefault",
			DefaultValue:    nil,
			HasDefaultValue: true,
		}},
	}

	module, err := resolveNestedFilter(nested, 0)
	if err != nil {
		t.Fatalf("resolveNestedFilter: %v", err)
	}
	out, err := module.Process(context.Background(), []map[string]any{{}})
	if err != nil {
		t.Fatalf("Process: %v", err)
	}
	if len(out) != 1 {
		t.Fatalf("expected 1 record, got %d", len(out))
	}
	v, ok := out[0]["out"]
	if !ok {
		t.Fatal("expected target key 'out' to be set with explicit nil default")
	}
	if v != nil {
		t.Fatalf("expected nil value for explicit-null default, got %v", v)
	}
}
