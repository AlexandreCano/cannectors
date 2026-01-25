package registry

import (
	"testing"

	"github.com/canectors/runtime/internal/modules/filter"
	"github.com/canectors/runtime/internal/modules/input"
	"github.com/canectors/runtime/internal/modules/output"
	"github.com/canectors/runtime/pkg/connector"
)

func TestRegisterInput(t *testing.T) {
	ClearRegistries()
	defer ClearRegistries()

	called := false
	constructor := func(cfg *connector.ModuleConfig) (input.Module, error) {
		called = true
		return input.NewStub("test", ""), nil
	}

	RegisterInput("testInput", constructor)

	got := GetInputConstructor("testInput")
	if got == nil {
		t.Fatal("expected constructor, got nil")
	}

	_, _ = got(nil)
	if !called {
		t.Error("constructor was not called")
	}
}

func TestRegisterFilter(t *testing.T) {
	ClearRegistries()
	defer ClearRegistries()

	called := false
	constructor := func(cfg connector.ModuleConfig, index int) (filter.Module, error) {
		called = true
		return filter.NewStub("test", index), nil
	}

	RegisterFilter("testFilter", constructor)

	got := GetFilterConstructor("testFilter")
	if got == nil {
		t.Fatal("expected constructor, got nil")
	}

	_, _ = got(connector.ModuleConfig{}, 0)
	if !called {
		t.Error("constructor was not called")
	}
}

func TestRegisterOutput(t *testing.T) {
	ClearRegistries()
	defer ClearRegistries()

	called := false
	constructor := func(cfg *connector.ModuleConfig) (output.Module, error) {
		called = true
		return output.NewStub("test", "", ""), nil
	}

	RegisterOutput("testOutput", constructor)

	got := GetOutputConstructor("testOutput")
	if got == nil {
		t.Fatal("expected constructor, got nil")
	}

	_, _ = got(nil)
	if !called {
		t.Error("constructor was not called")
	}
}

func TestGetUnregisteredConstructor(t *testing.T) {
	ClearRegistries()
	defer ClearRegistries()

	if got := GetInputConstructor("unknown"); got != nil {
		t.Error("expected nil for unregistered input type")
	}
	if got := GetFilterConstructor("unknown"); got != nil {
		t.Error("expected nil for unregistered filter type")
	}
	if got := GetOutputConstructor("unknown"); got != nil {
		t.Error("expected nil for unregistered output type")
	}
}

func TestListTypes(t *testing.T) {
	ClearRegistries()
	defer ClearRegistries()

	RegisterInput("inputA", func(cfg *connector.ModuleConfig) (input.Module, error) { return nil, nil })
	RegisterInput("inputB", func(cfg *connector.ModuleConfig) (input.Module, error) { return nil, nil })
	RegisterFilter("filterA", func(cfg connector.ModuleConfig, index int) (filter.Module, error) { return nil, nil })
	RegisterOutput("outputA", func(cfg *connector.ModuleConfig) (output.Module, error) { return nil, nil })

	inputTypes := ListInputTypes()
	if len(inputTypes) != 2 {
		t.Errorf("expected 2 input types, got %d", len(inputTypes))
	}

	filterTypes := ListFilterTypes()
	if len(filterTypes) != 1 {
		t.Errorf("expected 1 filter type, got %d", len(filterTypes))
	}

	outputTypes := ListOutputTypes()
	if len(outputTypes) != 1 {
		t.Errorf("expected 1 output type, got %d", len(outputTypes))
	}
}

func TestOverwriteRegistration(t *testing.T) {
	ClearRegistries()
	defer ClearRegistries()

	callCount := 0

	RegisterInput("test", func(cfg *connector.ModuleConfig) (input.Module, error) {
		callCount = 1
		return nil, nil
	})

	RegisterInput("test", func(cfg *connector.ModuleConfig) (input.Module, error) {
		callCount = 2
		return nil, nil
	})

	got := GetInputConstructor("test")
	_, _ = got(nil)

	if callCount != 2 {
		t.Error("expected second constructor to be called after overwrite")
	}
}

func TestClearRegistries(t *testing.T) {
	RegisterInput("test", func(cfg *connector.ModuleConfig) (input.Module, error) { return nil, nil })
	RegisterFilter("test", func(cfg connector.ModuleConfig, index int) (filter.Module, error) { return nil, nil })
	RegisterOutput("test", func(cfg *connector.ModuleConfig) (output.Module, error) { return nil, nil })

	ClearRegistries()

	if len(ListInputTypes()) != 0 {
		t.Error("expected input registry to be empty after clear")
	}
	if len(ListFilterTypes()) != 0 {
		t.Error("expected filter registry to be empty after clear")
	}
	if len(ListOutputTypes()) != 0 {
		t.Error("expected output registry to be empty after clear")
	}
}
