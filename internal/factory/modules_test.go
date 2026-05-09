package factory

import (
	"context"
	"encoding/json"
	"fmt"
	"reflect"
	"testing"

	"github.com/cannectors/runtime/internal/modules/filter"
	"github.com/cannectors/runtime/internal/modules/input"
	"github.com/cannectors/runtime/internal/registry"
	"github.com/cannectors/runtime/pkg/connector"
)

func mustJSON(v any) json.RawMessage {
	b, err := json.Marshal(v)
	if err != nil {
		panic(err)
	}
	return b
}

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
		Type: "httpPolling",
		Raw:  mustJSON(map[string]any{"endpoint": "https://example.com/api"}),
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
		Type: "unknownType",
		Raw:  mustJSON(map[string]any{"endpoint": "https://test.com"}),
	}

	got, err := CreateInputModule(cfg)
	if err == nil {
		t.Fatal("expected error for unknown type")
	}
	if got != nil {
		t.Fatal("expected nil module for unknown type")
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
			Raw: mustJSON(map[string]any{
				"mappings": []any{
					map[string]any{"source": "a", "target": "b"},
				},
			}),
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

func TestCreateFilterModules_ConditionNestedTopLevelModuleShape(t *testing.T) {
	cfgs := []connector.ModuleConfig{
		{
			Type: "condition",
			Raw: mustJSON(map[string]any{
				"expression": "status == 'paid'",
				"then": []any{
					map[string]any{
						"type":   "set",
						"target": "routing.bucket",
						"value":  "billable",
					},
				},
				"else": []any{
					map[string]any{
						"type":    "remove",
						"targets": []any{"card.number", "card.cvv"},
					},
				},
			}),
		},
	}

	modules, err := CreateFilterModules(cfgs)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(modules) != 1 {
		t.Fatalf("expected 1 module, got %d", len(modules))
	}

	result, err := modules[0].Process(context.Background(), []map[string]any{
		{"status": "paid"},
		{"status": "pending", "card": map[string]any{"number": "4111111111111111", "cvv": "123", "last4": "1111"}},
	})
	if err != nil {
		t.Fatalf("processing nested condition: %v", err)
	}
	if len(result) != 2 {
		t.Fatalf("expected 2 records, got %d", len(result))
	}

	routing, ok := result[0]["routing"].(map[string]any)
	if !ok {
		t.Fatalf("expected routing map, got %T", result[0]["routing"])
	}
	if routing["bucket"] != "billable" {
		t.Fatalf("expected billable bucket, got %v", routing["bucket"])
	}

	card, ok := result[1]["card"].(map[string]any)
	if !ok {
		t.Fatalf("expected card map, got %T", result[1]["card"])
	}
	if _, exists := card["number"]; exists {
		t.Fatal("expected card.number to be removed")
	}
	if _, exists := card["cvv"]; exists {
		t.Fatal("expected card.cvv to be removed")
	}
	if card["last4"] != "1111" {
		t.Fatalf("expected card.last4 to remain, got %v", card["last4"])
	}
}

func TestCreateFilterModules_Unknown(t *testing.T) {
	cfgs := []connector.ModuleConfig{
		{Type: "unknownFilter", Raw: mustJSON(map[string]any{})},
	}

	got, err := CreateFilterModules(cfgs)
	if err == nil {
		t.Fatal("expected error for unknown filter type")
	}
	if got != nil {
		t.Fatal("expected nil modules for unknown type")
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
		Raw: mustJSON(map[string]any{
			"endpoint": "https://example.com/api",
			"method":   "POST",
		}),
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
		Raw: mustJSON(map[string]any{
			"endpoint": "https://test.com",
			"method":   "PUT",
		}),
	}

	got, err := CreateOutputModule(cfg)
	if err == nil {
		t.Fatal("expected error for unknown output type")
	}
	if got != nil {
		t.Fatal("expected nil module for unknown type")
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
		Type: "customInput",
		Raw:  mustJSON(map[string]any{}),
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
		Raw: mustJSON(map[string]any{
			// Missing required endpoint or invalid config that causes NewHTTPPollingFromConfig to error
			"invalid": "config",
		}),
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
		if rv.Kind() == reflect.Pointer && !rv.IsNil() {
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
		Type: "errorInput",
		Raw:  mustJSON(map[string]any{}),
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

// stubFilter is a no-op filter used to verify that custom filter types
// registered via registry.RegisterFilter are reachable from the nested
// `then` block of a condition module.
type stubFilter struct {
	called bool
}

func (s *stubFilter) Process(_ context.Context, records []map[string]any) ([]map[string]any, error) {
	s.called = true
	return records, nil
}

// TestCondition_NestedThen_ResolvesCustomRegisteredFilter ensures that a
// filter type registered only in the test (i.e. not built-in) can be
// instantiated inside a condition's `then` block via the registry-injected
// nestedCreator. Regression guard for Story 16.6 (eliminates the prior
// double-registration that bypassed the registry for nested modules).
func TestCondition_NestedThen_ResolvesCustomRegisteredFilter(t *testing.T) {
	const customType = "stub-nested-then"

	stub := &stubFilter{}
	registry.RegisterFilter(customType, func(_ connector.ModuleConfig, _ int) (filter.Module, error) {
		return stub, nil
	})

	cfg := connector.ModuleConfig{
		Type: "condition",
		Raw: mustJSON(map[string]any{
			"expression": "true",
			"then": []map[string]any{
				{"type": customType, "config": map[string]any{}},
			},
		}),
	}

	mods, err := CreateFilterModules([]connector.ModuleConfig{cfg})
	if err != nil {
		t.Fatalf("CreateFilterModules error: %v", err)
	}
	if len(mods) != 1 {
		t.Fatalf("expected 1 condition module, got %d", len(mods))
	}

	if _, err := mods[0].Process(context.Background(), []map[string]any{{"x": 1}}); err != nil {
		t.Fatalf("Process error: %v", err)
	}
	if !stub.called {
		t.Fatal("custom filter registered for nested `then` was not invoked through the registry")
	}
}
