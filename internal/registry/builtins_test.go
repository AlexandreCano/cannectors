package registry

import (
	"context"
	"encoding/json"
	"testing"

	"github.com/cannectors/runtime/internal/modules/filter"
	"github.com/cannectors/runtime/internal/modules/output"
	"github.com/cannectors/runtime/pkg/connector"
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

func TestBuiltins_RegisterSOAPModules(t *testing.T) {
	inputCtor := GetInputConstructor("soapPolling")
	if inputCtor == nil {
		t.Fatal("soapPolling input constructor is not registered")
	}
	inputRaw := mustRegistryJSON(t, map[string]any{
		"endpoint":  "https://example.com/soap",
		"operation": "List",
		"body":      "<List/>",
	})
	inputModule, err := inputCtor(&connector.ModuleConfig{Type: "soapPolling", Raw: inputRaw})
	if err != nil {
		t.Fatalf("soapPolling constructor: %v", err)
	}
	if inputModule == nil {
		t.Fatal("soapPolling constructor returned nil")
	}
	_ = inputModule.Close()

	filterCtor := GetFilterConstructor("soap_call")
	if filterCtor == nil {
		t.Fatal("soap_call filter constructor is not registered")
	}
	filterRaw := mustRegistryJSON(t, map[string]any{
		"endpoint":  "https://example.com/soap",
		"operation": "Lookup",
		"body":      "<Lookup/>",
	})
	filterModule, err := filterCtor(connector.ModuleConfig{Type: "soap_call", Raw: filterRaw}, 0)
	if err != nil {
		t.Fatalf("soap_call constructor: %v", err)
	}
	if filterModule == nil {
		t.Fatal("soap_call constructor returned nil")
	}

	outputCtor := GetOutputConstructor("soapRequest")
	if outputCtor == nil {
		t.Fatal("soapRequest output constructor is not registered")
	}
	outputRaw := mustRegistryJSON(t, map[string]any{
		"endpoint":  "https://example.com/soap",
		"operation": "Submit",
		"body":      "<Submit/>",
	})
	outputModule, err := outputCtor(&connector.ModuleConfig{Type: "soapRequest", Raw: outputRaw})
	if err != nil {
		t.Fatalf("soapRequest constructor: %v", err)
	}
	if _, ok := outputModule.(output.PreviewableModule); !ok {
		t.Fatalf("soapRequest should implement PreviewableModule, got %T", outputModule)
	}
	_ = outputModule.Close()
}

func mustRegistryJSON(t *testing.T, cfg map[string]any) json.RawMessage {
	t.Helper()
	raw, err := json.Marshal(cfg)
	if err != nil {
		t.Fatal(err)
	}
	return raw
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
