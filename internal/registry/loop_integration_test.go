package registry

import (
	"context"
	"encoding/json"
	"testing"

	"github.com/cannectors/runtime/pkg/connector"
)

// TestLoop_RegistryRoundtrip locks that the loop filter is registered and
// composable through the registry's resolveNestedFilter path (i.e. nested
// loops resolve via the same registry lookup as any other filter type).
func TestLoop_RegistryRoundtrip(t *testing.T) {
	ctor := GetFilterConstructor("loop")
	if ctor == nil {
		t.Fatal("loop filter constructor is not registered")
	}

	cfg := map[string]any{
		"field":    "cells",
		"itemName": "cell",
		"filters": []any{
			map[string]any{
				"type":       "condition",
				"expression": `cell.columnId == 1`,
				"then": []any{
					map[string]any{
						"type": "mapping",
						"mappings": []any{
							map[string]any{"source": "cell.displayValue", "target": "record.eventId"},
						},
					},
				},
			},
		},
	}
	raw, _ := json.Marshal(cfg)
	mod, err := ctor(connector.ModuleConfig{Type: "loop", Raw: raw}, 0)
	if err != nil {
		t.Fatalf("loop constructor: %v", err)
	}

	record := map[string]any{
		"cells": []any{
			map[string]any{"columnId": 1, "displayValue": "evt-42"},
			map[string]any{"columnId": 2, "displayValue": "ignored"},
		},
	}
	out, err := mod.Process(context.Background(), []map[string]any{record})
	if err != nil {
		t.Fatalf("Process: %v", err)
	}
	if got := out[0]["eventId"]; got != "evt-42" {
		t.Errorf("record.eventId = %v, want evt-42", got)
	}
}

// TestLoop_NestedLoopViaRegistry locks that loop-on-loop is composable via
// the registry path (no inline support needed in createNestedModule).
func TestLoop_NestedLoopViaRegistry(t *testing.T) {
	ctor := GetFilterConstructor("loop")
	if ctor == nil {
		t.Fatal("loop filter constructor is not registered")
	}

	cfg := map[string]any{
		"field":    "cells",
		"itemName": "cell",
		"filters": []any{
			map[string]any{
				"type":     "loop",
				"field":    "cell.children",
				"itemName": "x",
				"filters": []any{
					map[string]any{
						"type": "mapping",
						"mappings": []any{
							map[string]any{"source": "x.label", "target": "record.lastLabel"},
						},
					},
				},
			},
		},
	}
	raw, _ := json.Marshal(cfg)
	mod, err := ctor(connector.ModuleConfig{Type: "loop", Raw: raw}, 0)
	if err != nil {
		t.Fatalf("loop constructor: %v", err)
	}

	record := map[string]any{
		"cells": []any{
			map[string]any{
				"children": []any{
					map[string]any{"label": "alpha-0"},
					map[string]any{"label": "alpha-1"},
				},
			},
		},
	}
	out, err := mod.Process(context.Background(), []map[string]any{record})
	if err != nil {
		t.Fatalf("Process: %v", err)
	}
	if got := out[0]["lastLabel"]; got != "alpha-1" {
		t.Errorf("record.lastLabel = %v, want alpha-1", got)
	}
}
