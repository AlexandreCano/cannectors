package moduleconfig_test

import (
	"encoding/json"
	"testing"

	"github.com/cannectors/runtime/internal/moduleconfig"
	"github.com/cannectors/runtime/pkg/connector"
)

type simpleConfig struct {
	Target string      `json:"target"`
	Value  interface{} `json:"value"`
}

type httpConfig struct {
	Endpoint string            `json:"endpoint"`
	Method   string            `json:"method,omitempty"`
	Headers  map[string]string `json:"headers,omitempty"`
}

type nestedConfig struct {
	Name    string        `json:"name"`
	Nested  *nestedConfig `json:"nested,omitempty"`
	Enabled *bool         `json:"enabled,omitempty"`
}

func TestParseConfig_SimpleStruct(t *testing.T) {
	raw := json.RawMessage(`{"target":"user.name","value":"Alice"}`)

	cfg, err := moduleconfig.ParseConfig[simpleConfig](raw)
	if err != nil {
		t.Fatalf("ParseConfig() error = %v", err)
	}
	if cfg.Target != "user.name" {
		t.Errorf("Target = %q, want %q", cfg.Target, "user.name")
	}
	if cfg.Value != "Alice" {
		t.Errorf("Value = %v, want %q", cfg.Value, "Alice")
	}
}

func TestParseConfig_HTTPConfig(t *testing.T) {
	raw := json.RawMessage(`{
		"endpoint": "https://api.example.com/data",
		"method": "POST",
		"headers": {"Content-Type": "application/json", "Accept": "application/json"}
	}`)

	cfg, err := moduleconfig.ParseConfig[httpConfig](raw)
	if err != nil {
		t.Fatalf("ParseConfig() error = %v", err)
	}
	if cfg.Endpoint != "https://api.example.com/data" {
		t.Errorf("Endpoint = %q", cfg.Endpoint)
	}
	if cfg.Method != "POST" {
		t.Errorf("Method = %q", cfg.Method)
	}
	if len(cfg.Headers) != 2 {
		t.Errorf("Headers len = %d, want 2", len(cfg.Headers))
	}
}

func TestParseConfig_NestedStruct(t *testing.T) {
	raw := json.RawMessage(`{
		"name": "root",
		"nested": {"name": "child", "enabled": true}
	}`)

	cfg, err := moduleconfig.ParseConfig[nestedConfig](raw)
	if err != nil {
		t.Fatalf("ParseConfig() error = %v", err)
	}
	if cfg.Name != "root" {
		t.Errorf("Name = %q", cfg.Name)
	}
	if cfg.Nested == nil || cfg.Nested.Name != "child" {
		t.Errorf("Nested.Name = %v", cfg.Nested)
	}
	if cfg.Nested.Enabled == nil || !*cfg.Nested.Enabled {
		t.Error("Nested.Enabled should be true")
	}
}

func TestParseConfig_EmptyJSON(t *testing.T) {
	raw := json.RawMessage(`{}`)

	cfg, err := moduleconfig.ParseConfig[simpleConfig](raw)
	if err != nil {
		t.Fatalf("ParseConfig() error = %v", err)
	}
	if cfg.Target != "" {
		t.Errorf("Target should be empty, got %q", cfg.Target)
	}
}

func TestParseConfig_InvalidJSON(t *testing.T) {
	raw := json.RawMessage(`{invalid json}`)

	_, err := moduleconfig.ParseConfig[simpleConfig](raw)
	if err == nil {
		t.Fatal("Expected error for invalid JSON")
	}
}

func TestParseConfig_NilRaw(t *testing.T) {
	_, err := moduleconfig.ParseConfig[simpleConfig](nil)
	if err == nil {
		t.Fatal("Expected error for nil raw data")
	}
}

func TestParseConfig_ExtraFieldsIgnored(t *testing.T) {
	raw := json.RawMessage(`{"target":"x","value":1,"unknown":"ignored"}`)

	cfg, err := moduleconfig.ParseConfig[simpleConfig](raw)
	if err != nil {
		t.Fatalf("ParseConfig() error = %v", err)
	}
	if cfg.Target != "x" {
		t.Errorf("Target = %q", cfg.Target)
	}
}

func TestParseConfig_NumericValue(t *testing.T) {
	raw := json.RawMessage(`{"target":"count","value":42}`)

	cfg, err := moduleconfig.ParseConfig[simpleConfig](raw)
	if err != nil {
		t.Fatalf("ParseConfig() error = %v", err)
	}
	if cfg.Value != float64(42) {
		t.Errorf("Value = %v (type %T), want float64(42)", cfg.Value, cfg.Value)
	}
}

func TestParseModuleConfig_WithRaw(t *testing.T) {
	mc := connector.ModuleConfig{
		Type: "set",
		Raw:  json.RawMessage(`{"target":"user.name","value":"Alice"}`),
	}

	cfg, err := moduleconfig.ParseModuleConfig[simpleConfig](mc)
	if err != nil {
		t.Fatalf("ParseModuleConfig() error = %v", err)
	}
	if cfg.Target != "user.name" {
		t.Errorf("Target = %q", cfg.Target)
	}
}

func TestParseModuleConfig_FromRaw(t *testing.T) {
	mc := connector.ModuleConfig{
		Type: "set",
		Raw:  json.RawMessage(`{"target":"status","value":"active"}`),
	}

	cfg, err := moduleconfig.ParseModuleConfig[simpleConfig](mc)
	if err != nil {
		t.Fatalf("ParseModuleConfig() error = %v", err)
	}
	if cfg.Target != "status" {
		t.Errorf("Target = %q", cfg.Target)
	}
	if cfg.Value != "active" {
		t.Errorf("Value = %v", cfg.Value)
	}
}

func TestParseModuleConfig_NoData(t *testing.T) {
	mc := connector.ModuleConfig{Type: "set"}

	_, err := moduleconfig.ParseModuleConfig[simpleConfig](mc)
	if err == nil {
		t.Fatal("Expected error for empty ModuleConfig")
	}
}
