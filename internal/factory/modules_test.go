package factory

import (
	"testing"

	"github.com/canectors/runtime/internal/modules/filter"
	"github.com/canectors/runtime/internal/modules/input"
	"github.com/canectors/runtime/internal/modules/output"
	"github.com/canectors/runtime/internal/registry"
	"github.com/canectors/runtime/pkg/connector"
)

func TestCreateInputModule_Nil(t *testing.T) {
	got := CreateInputModule(nil)
	if got != nil {
		t.Error("expected nil for nil config")
	}
}

func TestCreateInputModule_Registered(t *testing.T) {
	cfg := &connector.ModuleConfig{
		Type:   "httpPolling",
		Config: map[string]interface{}{"endpoint": "https://example.com/api"},
	}

	got := CreateInputModule(cfg)
	if got == nil {
		t.Fatal("expected non-nil module")
	}
}

func TestCreateInputModule_Unknown(t *testing.T) {
	cfg := &connector.ModuleConfig{
		Type:   "unknownType",
		Config: map[string]interface{}{"endpoint": "https://test.com"},
	}

	got := CreateInputModule(cfg)
	if got == nil {
		t.Fatal("expected stub module for unknown type")
	}

	stub, ok := got.(*input.StubModule)
	if !ok {
		t.Fatal("expected StubModule for unknown type")
	}
	if stub.ModuleType != "unknownType" {
		t.Errorf("expected type 'unknownType', got %s", stub.ModuleType)
	}
}

func TestCreateFilterModules_Empty(t *testing.T) {
	got, err := CreateFilterModules(nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got != nil {
		t.Error("expected nil for empty config")
	}
}

func TestCreateFilterModules_Mapping(t *testing.T) {
	cfgs := []connector.ModuleConfig{
		{
			Type: "mapping",
			Config: map[string]interface{}{
				"mappings": []interface{}{
					map[string]interface{}{"source": "a", "target": "b"},
				},
			},
		},
	}

	got, err := CreateFilterModules(cfgs)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(got) != 1 {
		t.Fatalf("expected 1 module, got %d", len(got))
	}
}

func TestCreateFilterModules_Unknown(t *testing.T) {
	cfgs := []connector.ModuleConfig{
		{Type: "unknownFilter", Config: map[string]interface{}{}},
	}

	got, err := CreateFilterModules(cfgs)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(got) != 1 {
		t.Fatalf("expected 1 module, got %d", len(got))
	}

	stub, ok := got[0].(*filter.StubModule)
	if !ok {
		t.Fatal("expected StubModule for unknown type")
	}
	if stub.ModuleType != "unknownFilter" {
		t.Errorf("expected type 'unknownFilter', got %s", stub.ModuleType)
	}
}

func TestCreateOutputModule_Nil(t *testing.T) {
	got, err := CreateOutputModule(nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got != nil {
		t.Error("expected nil for nil config")
	}
}

func TestCreateOutputModule_Registered(t *testing.T) {
	cfg := &connector.ModuleConfig{
		Type: "httpRequest",
		Config: map[string]interface{}{
			"endpoint": "https://example.com/api",
			"method":   "POST",
		},
	}

	got, err := CreateOutputModule(cfg)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got == nil {
		t.Fatal("expected non-nil module")
	}
}

func TestCreateOutputModule_Unknown(t *testing.T) {
	cfg := &connector.ModuleConfig{
		Type: "unknownOutput",
		Config: map[string]interface{}{
			"endpoint": "https://test.com",
			"method":   "PUT",
		},
	}

	got, err := CreateOutputModule(cfg)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got == nil {
		t.Fatal("expected stub module for unknown type")
	}

	stub, ok := got.(*output.StubModule)
	if !ok {
		t.Fatal("expected StubModule for unknown type")
	}
	if stub.ModuleType != "unknownOutput" {
		t.Errorf("expected type 'unknownOutput', got %s", stub.ModuleType)
	}
}

func TestCreateInputModule_CustomRegistered(t *testing.T) {
	// Register a custom module for this test
	customCalled := false
	registry.RegisterInput("customInput", func(cfg *connector.ModuleConfig) input.Module {
		customCalled = true
		return input.NewStub("customInput", "custom-endpoint")
	})
	// Note: No cleanup needed - test isolation ensures this registration
	// doesn't affect other tests. The registry is package-level but tests
	// run sequentially, and built-in modules are re-registered on each init().

	cfg := &connector.ModuleConfig{
		Type:   "customInput",
		Config: map[string]interface{}{},
	}

	got := CreateInputModule(cfg)
	if !customCalled {
		t.Error("custom constructor was not called")
	}
	if got == nil {
		t.Fatal("expected non-nil module")
	}

	stub, ok := got.(*input.StubModule)
	if !ok {
		t.Fatal("expected StubModule")
	}
	if stub.ModuleType != "customInput" {
		t.Errorf("expected type 'customInput', got %s", stub.ModuleType)
	}
}

func TestParseConditionConfig_Valid(t *testing.T) {
	cfg := map[string]interface{}{
		"expression": "status == 'active'",
		"lang":       "simple",
		"onTrue":     "continue",
		"onFalse":    "skip",
	}

	got, err := ParseConditionConfig(cfg)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if got.Expression != "status == 'active'" {
		t.Errorf("expected expression 'status == \"active\"', got %s", got.Expression)
	}
	if got.Lang != "simple" {
		t.Errorf("expected lang 'simple', got %s", got.Lang)
	}
	if got.OnTrue != "continue" {
		t.Errorf("expected onTrue 'continue', got %s", got.OnTrue)
	}
	if got.OnFalse != "skip" {
		t.Errorf("expected onFalse 'skip', got %s", got.OnFalse)
	}
}

func TestParseConditionConfig_MissingExpression(t *testing.T) {
	cfg := map[string]interface{}{
		"lang": "simple",
	}

	_, err := ParseConditionConfig(cfg)
	if err == nil {
		t.Fatal("expected error for missing expression")
	}
}

func TestParseConditionConfig_NestedThen(t *testing.T) {
	cfg := map[string]interface{}{
		"expression": "status == 'active'",
		"then": map[string]interface{}{
			"type": "mapping",
			"mappings": []interface{}{
				map[string]interface{}{"source": "a", "target": "b"},
			},
		},
	}

	got, err := ParseConditionConfig(cfg)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if got.Then == nil {
		t.Fatal("expected 'then' to be parsed")
	}
	if got.Then.Type != "mapping" {
		t.Errorf("expected then type 'mapping', got %s", got.Then.Type)
	}
}
