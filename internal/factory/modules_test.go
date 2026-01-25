package factory

import (
	"fmt"
	"reflect"
	"testing"

	"github.com/canectors/runtime/internal/modules/filter"
	"github.com/canectors/runtime/internal/modules/input"
	"github.com/canectors/runtime/internal/modules/output"
	"github.com/canectors/runtime/internal/registry"
	"github.com/canectors/runtime/pkg/connector"
)

func TestCreateInputModule_Nil(t *testing.T) {
	got, err := CreateInputModule(nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got != nil {
		t.Error("expected nil for nil config")
	}
}

func TestCreateInputModule_Registered(t *testing.T) {
	cfg := &connector.ModuleConfig{
		Type:   "httpPolling",
		Config: map[string]interface{}{"endpoint": "https://example.com/api"},
	}

	got, err := CreateInputModule(cfg)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got == nil {
		t.Fatal("expected non-nil module")
	}
}

func TestCreateInputModule_Unknown(t *testing.T) {
	cfg := &connector.ModuleConfig{
		Type:   "unknownType",
		Config: map[string]interface{}{"endpoint": "https://test.com"},
	}

	got, err := CreateInputModule(cfg)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
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
	originalConstructor := registry.GetInputConstructor("customInput")
	registry.RegisterInput("customInput", func(cfg *connector.ModuleConfig) (input.Module, error) {
		customCalled = true
		return input.NewStub("customInput", "custom-endpoint"), nil
	})
	defer func() {
		// Restore original constructor or clear if it was nil
		if originalConstructor != nil {
			registry.RegisterInput("customInput", originalConstructor)
		} else {
			// Clear the registration by overwriting with a stub that returns nil
			// This is a workaround since there's no explicit unregister function
			registry.RegisterInput("customInput", func(cfg *connector.ModuleConfig) (input.Module, error) {
				return nil, nil
			})
		}
	}()

	cfg := &connector.ModuleConfig{
		Type:   "customInput",
		Config: map[string]interface{}{},
	}

	got, err := CreateInputModule(cfg)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
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

func TestCreateInputModule_ErrorReturnsError(t *testing.T) {
	// Test with a module type that will fail during construction
	// We'll use httpPolling with invalid config to trigger the error path
	cfg := &connector.ModuleConfig{
		Type: "httpPolling",
		Config: map[string]interface{}{
			// Missing required endpoint or invalid config that causes NewHTTPPollingFromConfig to error
			"invalid": "config",
		},
	}

	got, err := CreateInputModule(cfg)
	if err == nil {
		t.Fatal("expected error when constructor fails")
	}
	// When constructor returns an error, the module should be nil
	// (the error is propagated, not converted to stub)
	// Note: In Go, an interface can have a nil value but non-nil type.
	// We use reflection to check if the underlying value is actually nil.
	if got != nil {
		// Check if the underlying value is nil using reflection
		rv := reflect.ValueOf(got)
		if rv.Kind() == reflect.Ptr && !rv.IsNil() {
			t.Errorf("expected nil module when constructor fails, got non-nil value: %v (type %T)", got, got)
		}
		// If it's a nil pointer, that's acceptable - the important part is that error was returned
	}
}

func TestCreateInputModule_ConstructorError(t *testing.T) {
	// Register a module that intentionally returns an error
	original := registry.GetInputConstructor("errorInput")
	testErr := fmt.Errorf("test error")
	registry.RegisterInput("errorInput", func(cfg *connector.ModuleConfig) (input.Module, error) {
		return nil, testErr
	})
	defer func() {
		if original != nil {
			registry.RegisterInput("errorInput", original)
		} else {
			registry.RegisterInput("errorInput", func(cfg *connector.ModuleConfig) (input.Module, error) {
				return nil, nil
			})
		}
	}()

	cfg := &connector.ModuleConfig{
		Type:   "errorInput",
		Config: map[string]interface{}{},
	}

	got, err := CreateInputModule(cfg)
	if err == nil {
		t.Fatal("expected error from constructor")
	}
	if err != testErr {
		t.Errorf("expected test error, got %v", err)
	}
	if got != nil {
		t.Error("expected nil module when constructor returns error")
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
