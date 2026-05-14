package filter

import (
	"context"
	"encoding/json"
	"errors"
	"maps"
	"reflect"
	"testing"

	"github.com/cannectors/runtime/pkg/connector"
)

// loopTestNestedCreator returns a NestedModuleCreator that resolves nested
// "loop" modules (with full recursion) plus the inline-supported types
// (condition, mapping, drop) used by createNestedModule's fallback. Tests
// in this package run without the registry, so loop-on-loop nesting needs
// an explicit creator wired here.
func loopTestNestedCreator() NestedModuleCreator {
	var creator NestedModuleCreator
	creator = func(config *NestedModuleConfig, index int) (Module, error) {
		if config == nil {
			return nil, nil
		}
		if config.Type == "loop" {
			loopCfg, err := loopConfigFromNested(config)
			if err != nil {
				return nil, err
			}
			return NewLoopFromConfig(loopCfg, creator)
		}
		return createNestedModule(config, nil)
	}
	return creator
}

// loopConfigFromNested converts a NestedModuleConfig describing a loop into a
// LoopConfig. The non-structural keys (field, itemName, filters) are stored
// in Config by NestedModuleConfig.UnmarshalJSON.
func loopConfigFromNested(config *NestedModuleConfig) (LoopConfig, error) {
	raw := make(map[string]any, len(config.Config))
	maps.Copy(raw, config.Config)
	if config.OnError != "" {
		raw["onError"] = config.OnError
	}
	b, err := json.Marshal(raw)
	if err != nil {
		return LoopConfig{}, err
	}
	var out LoopConfig
	if err := json.Unmarshal(b, &out); err != nil {
		return LoopConfig{}, err
	}
	return out, nil
}

// TestLoop_IteratesAndWritesItemBack covers the happy path: the loop iterates
// the array and writes each processed item back into the same position.
func TestLoop_IteratesAndWritesItemBack(t *testing.T) {
	src := "cell.displayValue"
	tgt := "cell.value"
	mod, err := NewLoopFromConfig(LoopConfig{
		Field:    "cells",
		ItemName: "cell",
		Filters: []*NestedModuleConfig{
			{Type: "mapping", Mappings: []FieldMapping{{Source: &src, Target: tgt}}},
		},
	}, nil)
	if err != nil {
		t.Fatalf("NewLoopFromConfig: %v", err)
	}

	record := map[string]any{
		"cells": []any{
			map[string]any{"displayValue": "alpha"},
			map[string]any{"displayValue": "beta"},
		},
	}

	out, err := mod.Process(context.Background(), []map[string]any{record})
	if err != nil {
		t.Fatalf("Process: %v", err)
	}
	if len(out) != 1 {
		t.Fatalf("expected 1 record, got %d", len(out))
	}
	cells, ok := out[0]["cells"].([]any)
	if !ok || len(cells) != 2 {
		t.Fatalf("unexpected cells %v", out[0]["cells"])
	}
	for i, want := range []string{"alpha", "beta"} {
		c, _ := cells[i].(map[string]any)
		if c["value"] != want {
			t.Errorf("cells[%d].value = %v, want %v", i, c["value"], want)
		}
	}
}

// TestLoop_WritesRootFieldsFromInside locks the requirement that nested
// filters can mutate the root record via the `record` alias.
func TestLoop_WritesRootFieldsFromInside(t *testing.T) {
	src := "cell.displayValue"
	tgt := "record.eventId"
	mod, err := NewLoopFromConfig(LoopConfig{
		Field:    "cells",
		ItemName: "cell",
		Filters: []*NestedModuleConfig{
			{
				Type:       "condition",
				Expression: "cell.columnId == 1",
				Then: []*NestedModuleConfig{
					{Type: "mapping", Mappings: []FieldMapping{{Source: &src, Target: tgt}}},
				},
			},
		},
	}, nil)
	if err != nil {
		t.Fatalf("NewLoopFromConfig: %v", err)
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
	if _, ok := out[0]["_metadata"]; ok {
		t.Errorf("expected _metadata cleaned up, got %v", out[0]["_metadata"])
	}
}

// TestLoop_RemovesItemOnZeroRecords covers v1 behavior: when nested filters
// drop all records for an item, that item is removed from the array.
func TestLoop_RemovesItemOnZeroRecords(t *testing.T) {
	mod, err := NewLoopFromConfig(LoopConfig{
		Field:    "items",
		ItemName: "it",
		Filters: []*NestedModuleConfig{
			{
				Type:       "condition",
				Expression: "it.keep == true",
				Else:       []*NestedModuleConfig{{Type: "drop"}},
			},
		},
	}, nil)
	if err != nil {
		t.Fatalf("NewLoopFromConfig: %v", err)
	}

	record := map[string]any{
		"items": []any{
			map[string]any{"keep": true, "id": 1},
			map[string]any{"keep": false, "id": 2},
			map[string]any{"keep": true, "id": 3},
		},
	}
	out, err := mod.Process(context.Background(), []map[string]any{record})
	if err != nil {
		t.Fatalf("Process: %v", err)
	}
	items, _ := out[0]["items"].([]any)
	if len(items) != 2 {
		t.Fatalf("expected 2 items, got %d (%v)", len(items), items)
	}
}

// TestLoop_FailsOnItemExpansion locks the v1 rule: nested filters returning
// more than one record for a single item is an error.
func TestLoop_FailsOnItemExpansion(t *testing.T) {
	// Use a sentinel filter that doubles input records.
	doubler := moduleFunc(func(_ context.Context, recs []map[string]any) ([]map[string]any, error) {
		out := make([]map[string]any, 0, len(recs)*2)
		for _, r := range recs {
			out = append(out, r, r)
		}
		return out, nil
	})

	mod := &LoopModule{
		field:    "items",
		itemName: "it",
		modules:  []Module{doubler},
	}

	_, err := mod.Process(context.Background(), []map[string]any{
		{"items": []any{map[string]any{"id": 1}}},
	})
	if !errors.Is(err, ErrLoopItemExpansion) {
		t.Fatalf("want ErrLoopItemExpansion, got %v", err)
	}
}

// TestLoop_RejectsReservedItemName locks alias validation at construction.
func TestLoop_RejectsReservedItemName(t *testing.T) {
	for _, name := range []string{"record", "_metadata", "loop"} {
		_, err := NewLoopFromConfig(LoopConfig{Field: "items", ItemName: name}, nil)
		if !errors.Is(err, ErrLoopReservedItemName) {
			t.Errorf("itemName=%q: want ErrLoopReservedItemName, got %v", name, err)
		}
	}
}

// TestLoop_RejectsEmptyFieldOrItemName locks required-field validation.
func TestLoop_RejectsEmptyFieldOrItemName(t *testing.T) {
	if _, err := NewLoopFromConfig(LoopConfig{Field: "  ", ItemName: "x"}, nil); !errors.Is(err, ErrLoopFieldRequired) {
		t.Errorf("empty field: want ErrLoopFieldRequired, got %v", err)
	}
	if _, err := NewLoopFromConfig(LoopConfig{Field: "items", ItemName: " "}, nil); !errors.Is(err, ErrLoopItemNameRequired) {
		t.Errorf("empty itemName: want ErrLoopItemNameRequired, got %v", err)
	}
}

// TestLoop_NonArrayFieldErrors locks the rule that the field must be an array.
func TestLoop_NonArrayFieldErrors(t *testing.T) {
	mod, err := NewLoopFromConfig(LoopConfig{Field: "items", ItemName: "it"}, nil)
	if err != nil {
		t.Fatalf("NewLoopFromConfig: %v", err)
	}
	_, err = mod.Process(context.Background(), []map[string]any{
		{"items": "not-an-array"},
	})
	if !errors.Is(err, ErrLoopFieldNotArray) {
		t.Fatalf("want ErrLoopFieldNotArray, got %v", err)
	}
}

// TestLoop_MissingOrNullFieldIsNoop covers benign no-op cases.
func TestLoop_MissingOrNullFieldIsNoop(t *testing.T) {
	mod, err := NewLoopFromConfig(LoopConfig{Field: "items", ItemName: "it"}, nil)
	if err != nil {
		t.Fatalf("NewLoopFromConfig: %v", err)
	}
	for name, record := range map[string]map[string]any{
		"missing": {"other": "x"},
		"null":    {"items": nil},
	} {
		out, err := mod.Process(context.Background(), []map[string]any{record})
		if err != nil {
			t.Fatalf("%s: Process: %v", name, err)
		}
		if len(out) != 1 {
			t.Errorf("%s: expected 1 record, got %d", name, len(out))
		}
	}
}

// TestLoop_OnErrorSkipDropsFailingRecord locks onError: skip semantics.
func TestLoop_OnErrorSkipDropsFailingRecord(t *testing.T) {
	mod, err := NewLoopFromConfig(LoopConfig{
		ModuleBase: connector.ModuleBase{OnError: "skip"},
		Field:      "items",
		ItemName:   "it",
	}, nil)
	if err != nil {
		t.Fatalf("NewLoopFromConfig: %v", err)
	}
	good := map[string]any{"items": []any{map[string]any{"id": 1}}}
	bad := map[string]any{"items": "not-an-array"}
	out, err := mod.Process(context.Background(), []map[string]any{good, bad})
	if err != nil {
		t.Fatalf("Process: %v", err)
	}
	if len(out) != 1 {
		t.Fatalf("expected 1 record (bad skipped), got %d", len(out))
	}
}

// TestLoop_OnErrorLogKeepsFailingRecord locks onError: log semantics.
func TestLoop_OnErrorLogKeepsFailingRecord(t *testing.T) {
	mod, err := NewLoopFromConfig(LoopConfig{
		ModuleBase: connector.ModuleBase{OnError: "log"},
		Field:      "items",
		ItemName:   "it",
	}, nil)
	if err != nil {
		t.Fatalf("NewLoopFromConfig: %v", err)
	}
	bad := map[string]any{"items": 42}
	out, err := mod.Process(context.Background(), []map[string]any{bad})
	if err != nil {
		t.Fatalf("Process: %v", err)
	}
	if len(out) != 1 {
		t.Errorf("expected 1 record retained, got %d", len(out))
	}
}

// TestLoop_OnErrorFailPropagates locks the default failing behavior.
func TestLoop_OnErrorFailPropagates(t *testing.T) {
	mod, err := NewLoopFromConfig(LoopConfig{Field: "items", ItemName: "it"}, nil)
	if err != nil {
		t.Fatalf("NewLoopFromConfig: %v", err)
	}
	_, err = mod.Process(context.Background(), []map[string]any{{"items": "no"}})
	if err == nil {
		t.Fatal("expected error to propagate")
	}
}

// TestLoop_RejectsMetadataLoopWrites locks the read-only invariant on
// _metadata.loop. A nested filter that writes there must be rejected.
func TestLoop_RejectsMetadataLoopWrites(t *testing.T) {
	src := "_metadata.loop.it.index"
	tgt := "_metadata.loop.it.tampered"
	mod, err := NewLoopFromConfig(LoopConfig{
		Field:    "items",
		ItemName: "it",
		Filters: []*NestedModuleConfig{
			{Type: "mapping", Mappings: []FieldMapping{{Source: &src, Target: tgt}}},
		},
	}, nil)
	if err != nil {
		t.Fatalf("NewLoopFromConfig: %v", err)
	}
	_, err = mod.Process(context.Background(), []map[string]any{
		{"items": []any{map[string]any{"id": 1}}},
	})
	if !errors.Is(err, ErrLoopMetadataMutated) {
		t.Fatalf("want ErrLoopMetadataMutated, got %v", err)
	}
}

// TestLoop_RejectsMetadataReplacement covers the harder variant of the
// read-only invariant: a nested filter that replaces the entire `_metadata`
// map (rather than editing _metadata.loop in place) must still be detected.
// The check has to re-read through rootRecord, not compare a stale local
// reference to the map captured at scope construction.
func TestLoop_RejectsMetadataReplacement(t *testing.T) {
	replacer := moduleFunc(func(_ context.Context, recs []map[string]any) ([]map[string]any, error) {
		for _, r := range recs {
			root, _ := r[loopScopeRecord].(map[string]any)
			if root == nil {
				root = r
			}
			root[loopScopeMetadata] = map[string]any{"loop": map[string]any{}}
		}
		return recs, nil
	})

	mod := &LoopModule{
		field:    "items",
		itemName: "it",
		modules:  []Module{replacer},
	}
	_, err := mod.Process(context.Background(), []map[string]any{
		{"items": []any{map[string]any{"id": 1}}},
	})
	if !errors.Is(err, ErrLoopMetadataMutated) {
		t.Fatalf("want ErrLoopMetadataMutated for _metadata replacement, got %v", err)
	}
}

// TestLoop_RejectsMetadataDeletion covers the deletion variant: a nested
// filter that drops `_metadata` entirely must be flagged.
func TestLoop_RejectsMetadataDeletion(t *testing.T) {
	deleter := moduleFunc(func(_ context.Context, recs []map[string]any) ([]map[string]any, error) {
		for _, r := range recs {
			root, _ := r[loopScopeRecord].(map[string]any)
			if root == nil {
				root = r
			}
			delete(root, loopScopeMetadata)
		}
		return recs, nil
	})
	mod := &LoopModule{
		field:    "items",
		itemName: "it",
		modules:  []Module{deleter},
	}
	_, err := mod.Process(context.Background(), []map[string]any{
		{"items": []any{map[string]any{"id": 1}}},
	})
	if !errors.Is(err, ErrLoopMetadataMutated) {
		t.Fatalf("want ErrLoopMetadataMutated for _metadata deletion, got %v", err)
	}
}

// TestLoop_RejectsNonObjectItemSubpathWrite locks the rule that nested
// writes cannot silently turn a scalar item into a map via recordpath's
// auto-create behavior.
func TestLoop_RejectsNonObjectItemSubpathWrite(t *testing.T) {
	mod, err := NewLoopFromConfig(LoopConfig{
		Field:    "items",
		ItemName: "it",
		Filters: []*NestedModuleConfig{
			{Type: "mapping", Mappings: []FieldMapping{{Source: ptrStr("record.injected"), Target: "it.foo"}}},
		},
	}, nil)
	if err != nil {
		t.Fatalf("NewLoopFromConfig: %v", err)
	}
	_, err = mod.Process(context.Background(), []map[string]any{
		{"injected": "x", "items": []any{"scalar-value"}},
	})
	if !errors.Is(err, ErrLoopNonObjectItem) {
		t.Fatalf("want ErrLoopNonObjectItem, got %v", err)
	}
}

// TestLoop_AcceptsNonObjectItemsWhenUntouched covers the safe path: scalar
// items pass through unchanged when nested filters don't touch them.
func TestLoop_AcceptsNonObjectItemsWhenUntouched(t *testing.T) {
	mod, err := NewLoopFromConfig(LoopConfig{Field: "tags", ItemName: "tag"}, nil)
	if err != nil {
		t.Fatalf("NewLoopFromConfig: %v", err)
	}
	out, err := mod.Process(context.Background(), []map[string]any{
		{"tags": []any{"a", "b", "c"}},
	})
	if err != nil {
		t.Fatalf("Process: %v", err)
	}
	tags, _ := out[0]["tags"].([]any)
	if !reflect.DeepEqual(tags, []any{"a", "b", "c"}) {
		t.Errorf("tags = %v, want unchanged", tags)
	}
}

// TestLoop_RespectsContextCancellation locks ctx.Done propagation.
func TestLoop_RespectsContextCancellation(t *testing.T) {
	mod, err := NewLoopFromConfig(LoopConfig{Field: "items", ItemName: "it"}, nil)
	if err != nil {
		t.Fatalf("NewLoopFromConfig: %v", err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	_, err = mod.Process(ctx, []map[string]any{
		{"items": []any{map[string]any{"id": 1}}},
	})
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("want context.Canceled, got %v", err)
	}
}

// TestLoop_NestedLoopsExposeAllAliases covers the multi-alias scope: the
// inner filters can read `record`, the parent alias (`cell`), the current
// alias (`x`), and the alias metadata for both loops.
func TestLoop_NestedLoopsExposeAllAliases(t *testing.T) {
	creator := loopTestNestedCreator()

	innerFilters := []*NestedModuleConfig{
		{
			Type: "mapping",
			Mappings: []FieldMapping{
				{Source: ptrStr("x.label"), Target: "record.lastInnerLabel"},
				{Source: ptrStr("cell.name"), Target: "record.lastParentName"},
				{Source: ptrStr("_metadata.loop.cell.index"), Target: "record.lastCellIndex"},
				{Source: ptrStr("_metadata.loop.x.index"), Target: "record.lastInnerIndex"},
			},
		},
	}

	outer, err := NewLoopFromConfig(LoopConfig{
		Field:    "cells",
		ItemName: "cell",
		Filters: []*NestedModuleConfig{
			{
				Type: "loop",
				Config: map[string]any{
					"field":    "cell.children",
					"itemName": "x",
					"filters": []any{
						mustNestedRaw(innerFilters[0]),
					},
				},
			},
		},
	}, creator)
	if err != nil {
		t.Fatalf("NewLoopFromConfig outer: %v", err)
	}

	record := map[string]any{
		"cells": []any{
			map[string]any{
				"name": "alpha",
				"children": []any{
					map[string]any{"label": "alpha-0"},
					map[string]any{"label": "alpha-1"},
				},
			},
			map[string]any{
				"name": "beta",
				"children": []any{
					map[string]any{"label": "beta-0"},
				},
			},
		},
	}

	out, err := outer.Process(context.Background(), []map[string]any{record})
	if err != nil {
		t.Fatalf("Process: %v", err)
	}
	// Last item processed is cells[1] = beta, children[0] = beta-0.
	if got := out[0]["lastInnerLabel"]; got != "beta-0" {
		t.Errorf("lastInnerLabel = %v, want beta-0", got)
	}
	if got := out[0]["lastParentName"]; got != "beta" {
		t.Errorf("lastParentName = %v, want beta", got)
	}
	if got := out[0]["lastCellIndex"]; got != 1 {
		t.Errorf("lastCellIndex = %v, want 1", got)
	}
	if got := out[0]["lastInnerIndex"]; got != 0 {
		t.Errorf("lastInnerIndex = %v, want 0", got)
	}
}

// TestLoop_NestedAliasCollisionRejected covers the rule that an inner loop
// cannot reuse a parent alias.
func TestLoop_NestedAliasCollisionRejected(t *testing.T) {
	creator := loopTestNestedCreator()

	outer, err := NewLoopFromConfig(LoopConfig{
		Field:    "cells",
		ItemName: "cell",
		Filters: []*NestedModuleConfig{
			{
				Type: "loop",
				Config: map[string]any{
					"field":    "cell.children",
					"itemName": "cell",
					"filters":  []any{},
				},
			},
		},
	}, creator)
	if err != nil {
		t.Fatalf("NewLoopFromConfig outer: %v", err)
	}

	_, err = outer.Process(context.Background(), []map[string]any{
		{"cells": []any{map[string]any{"children": []any{map[string]any{}}}}},
	})
	if !errors.Is(err, ErrLoopAliasConflict) {
		t.Fatalf("want ErrLoopAliasConflict, got %v", err)
	}
}

// TestLoop_MetadataCleanupAfterProcessing locks that _metadata.loop entries
// are not leaked into the output when the loop owns the alias.
func TestLoop_MetadataCleanupAfterProcessing(t *testing.T) {
	mod, err := NewLoopFromConfig(LoopConfig{Field: "items", ItemName: "it"}, nil)
	if err != nil {
		t.Fatalf("NewLoopFromConfig: %v", err)
	}
	record := map[string]any{
		"items": []any{map[string]any{"id": 1}},
	}
	if _, err := mod.Process(context.Background(), []map[string]any{record}); err != nil {
		t.Fatalf("Process: %v", err)
	}
	if md, ok := record["_metadata"]; ok {
		t.Errorf("expected _metadata removed, got %v", md)
	}
}

// TestLoop_PreservesPreExistingMetadata locks that user-provided _metadata
// is preserved after the loop runs.
func TestLoop_PreservesPreExistingMetadata(t *testing.T) {
	mod, err := NewLoopFromConfig(LoopConfig{Field: "items", ItemName: "it"}, nil)
	if err != nil {
		t.Fatalf("NewLoopFromConfig: %v", err)
	}
	record := map[string]any{
		"_metadata": map[string]any{"traceId": "abc"},
		"items":     []any{map[string]any{"id": 1}},
	}
	if _, err := mod.Process(context.Background(), []map[string]any{record}); err != nil {
		t.Fatalf("Process: %v", err)
	}
	md, _ := record["_metadata"].(map[string]any)
	if md == nil || md["traceId"] != "abc" {
		t.Errorf("expected preserved traceId, got %v", record["_metadata"])
	}
	if _, has := md["loop"]; has {
		t.Errorf("expected _metadata.loop cleaned up, got %v", md["loop"])
	}
}

// TestLoop_EmptyArrayWritesBack ensures an empty array stays empty.
func TestLoop_EmptyArrayWritesBack(t *testing.T) {
	mod, err := NewLoopFromConfig(LoopConfig{Field: "items", ItemName: "it"}, nil)
	if err != nil {
		t.Fatalf("NewLoopFromConfig: %v", err)
	}
	record := map[string]any{"items": []any{}}
	out, err := mod.Process(context.Background(), []map[string]any{record})
	if err != nil {
		t.Fatalf("Process: %v", err)
	}
	items, _ := out[0]["items"].([]any)
	if !reflect.DeepEqual(items, []any{}) {
		t.Errorf("expected empty array, got %v", items)
	}
}

// moduleFunc adapts a function to the Module interface.
type moduleFunc func(ctx context.Context, records []map[string]any) ([]map[string]any, error)

func (m moduleFunc) Process(ctx context.Context, records []map[string]any) ([]map[string]any, error) {
	return m(ctx, records)
}

func ptrStr(s string) *string { return &s }

// mustNestedRaw round-trips a NestedModuleConfig through JSON so it can be
// embedded under a parent loop's Config["filters"] as raw map data (the shape
// produced by NestedModuleConfig.UnmarshalJSON for unknown keys).
func mustNestedRaw(n *NestedModuleConfig) any {
	b, err := json.Marshal(n)
	if err != nil {
		panic(err)
	}
	var out any
	if err := json.Unmarshal(b, &out); err != nil {
		panic(err)
	}
	return out
}
