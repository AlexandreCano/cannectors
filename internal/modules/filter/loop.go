// Package filter provides implementations for filter modules.
// Loop module iterates over an array field on each record, running a nested
// filter chain for every item and writing the processed item back into the
// original array.
package filter

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"maps"
	"strings"
	"time"

	"github.com/cannectors/runtime/internal/errhandling"
	"github.com/cannectors/runtime/internal/logger"
	"github.com/cannectors/runtime/internal/recordpath"
	"github.com/cannectors/runtime/pkg/connector"
)

// Reserved scope keys that cannot be used as a loop itemName.
const (
	loopScopeRecord   = "record"
	loopScopeMetadata = "_metadata"
	loopScopeLoop     = "loop"
)

// Loop module errors.
var (
	ErrLoopFieldRequired    = errors.New("loop filter requires non-empty field")
	ErrLoopItemNameRequired = errors.New("loop filter requires non-empty itemName")
	ErrLoopReservedItemName = errors.New("loop itemName is reserved")
	ErrLoopFieldNotArray    = errors.New("loop field is not an array")
	ErrLoopItemExpansion    = errors.New("loop nested filters returned more than one record for an item")
	ErrLoopMetadataMutated  = errors.New("nested filters wrote to _metadata.loop which is read-only")
	ErrLoopAliasConflict    = errors.New("loop itemName conflicts with an active parent loop alias")
	ErrLoopNonObjectItem    = errors.New("loop nested filters cannot write sub-fields on a non-object item")
)

// LoopConfig represents the configuration for a loop filter module.
type LoopConfig struct {
	connector.ModuleBase
	Field    string                `json:"field"`
	ItemName string                `json:"itemName"`
	Filters  []*NestedModuleConfig `json:"filters,omitempty"`
}

// LoopModule implements iteration over an array field with a nested filter chain.
type LoopModule struct {
	field    string
	itemName string
	onError  errhandling.OnErrorStrategy
	modules  []Module
}

// NewLoopFromConfig creates a new loop filter module from configuration.
// The nestedCreator is used to instantiate every nested filter via the
// registry, mirroring the condition module's pattern.
func NewLoopFromConfig(config LoopConfig, nestedCreator NestedModuleCreator) (*LoopModule, error) {
	field := strings.TrimSpace(config.Field)
	if field == "" {
		return nil, fmt.Errorf("%w", ErrLoopFieldRequired)
	}
	itemName := strings.TrimSpace(config.ItemName)
	if itemName == "" {
		return nil, fmt.Errorf("%w", ErrLoopItemNameRequired)
	}
	switch itemName {
	case loopScopeRecord, loopScopeMetadata, loopScopeLoop:
		return nil, fmt.Errorf("%w: %q", ErrLoopReservedItemName, itemName)
	}

	onError, err := errhandling.ParseOnErrorStrategy(config.OnError)
	if err != nil {
		return nil, err
	}

	modules, err := createLoopNestedModules(config.Filters, nestedCreator)
	if err != nil {
		return nil, fmt.Errorf("creating nested filters: %w", err)
	}

	logger.Debug("loop module initialized",
		slog.String("field", field),
		slog.String("item_name", itemName),
		slog.String("on_error", string(onError)),
		slog.Int("nested_count", len(modules)),
	)

	return &LoopModule{
		field:    field,
		itemName: itemName,
		onError:  onError,
		modules:  modules,
	}, nil
}

// createLoopNestedModules instantiates the nested filter chain. Disabled
// modules (enabled:false) are skipped at construction time, matching the
// condition module behavior.
func createLoopNestedModules(configs []*NestedModuleConfig, nestedCreator NestedModuleCreator) ([]Module, error) {
	if len(configs) == 0 {
		return nil, nil
	}
	modules := make([]Module, 0, len(configs))
	for i, cfg := range configs {
		if cfg == nil {
			continue
		}
		if cfg.Enabled != nil && !*cfg.Enabled {
			logger.Debug("skipping disabled nested loop filter", "index", i, "type", cfg.Type)
			continue
		}
		m, err := createNestedModule(cfg, nestedCreator)
		if err != nil {
			return nil, fmt.Errorf("nested filter at index %d: %w", i, err)
		}
		if m != nil {
			modules = append(modules, m)
		}
	}
	return modules, nil
}

// Process iterates over each input record's array field and applies the nested
// filter chain to every item. Input records are returned (mutated in place);
// records dropped via onError:skip are filtered out.
func (l *LoopModule) Process(ctx context.Context, records []map[string]any) ([]map[string]any, error) {
	if records == nil {
		return []map[string]any{}, nil
	}

	startTime := time.Now()
	inputCount := len(records)

	logger.Debug("filter processing started",
		slog.String("module_type", "loop"),
		slog.String("field", l.field),
		slog.String("item_name", l.itemName),
		slog.Int("input_records", inputCount),
		slog.String("on_error", string(l.onError)),
	)

	result := make([]map[string]any, 0, len(records))
	for recordIdx, record := range records {
		if err := ctx.Err(); err != nil {
			return nil, err
		}
		if err := l.processRecord(ctx, record, recordIdx); err != nil {
			switch l.onError {
			case errhandling.OnErrorFail:
				return nil, err
			case errhandling.OnErrorSkip:
				logger.Warn("skipping record due to loop error",
					slog.Int("record_index", recordIdx),
					slog.String("field", l.field),
					slog.String("item_name", l.itemName),
					slog.String("error", err.Error()),
				)
				continue
			case errhandling.OnErrorLog:
				logger.Error("loop error (continuing)",
					slog.Int("record_index", recordIdx),
					slog.String("field", l.field),
					slog.String("item_name", l.itemName),
					slog.String("error", err.Error()),
				)
				// Keep the record (possibly partially mutated).
			default:
				return nil, err
			}
		}
		result = append(result, record)
	}

	logger.Info("filter processing completed",
		slog.String("module_type", "loop"),
		slog.String("field", l.field),
		slog.String("item_name", l.itemName),
		slog.Int("input_records", inputCount),
		slog.Int("output_records", len(result)),
		slog.Duration("duration", time.Since(startTime)),
	)

	return result, nil
}

// processRecord runs the loop for a single input record. The record is mutated
// in place: nested filter writes to `record.*` modify the root record, and
// writes to the item alias are persisted back into the array.
func (l *LoopModule) processRecord(ctx context.Context, R map[string]any, recordIdx int) error {
	raw, found := recordpath.Get(R, l.field)
	if !found || raw == nil {
		return nil
	}
	arr, ok := raw.([]any)
	if !ok {
		return fmt.Errorf("%w: field %q is %T at record %d", ErrLoopFieldNotArray, l.field, raw, recordIdx)
	}

	// Detect whether the input record is itself a scope produced by a parent
	// loop. Scope records always carry a `record` key pointing to the root
	// map; otherwise R is the root record itself.
	rootRecord, isNested := R[loopScopeRecord].(map[string]any)
	if !isNested {
		rootRecord = R
	}

	metadata, metadataPreExisted := ensureScopeMap(rootRecord, loopScopeMetadata)
	loopMeta, loopMetaPreExisted := ensureScopeMap(metadata, loopScopeLoop)

	if _, exists := loopMeta[l.itemName]; exists {
		return fmt.Errorf("%w: alias %q already active", ErrLoopAliasConflict, l.itemName)
	}

	aliasState := make(map[string]any, 1)
	loopMeta[l.itemName] = aliasState

	defer func() {
		// Re-read from rootRecord so cleanup acts on the live state, not on
		// a stale reference if a nested filter replaced _metadata.
		md, mdOK := rootRecord[loopScopeMetadata].(map[string]any)
		if !mdOK {
			return
		}
		lm, lmOK := md[loopScopeLoop].(map[string]any)
		if lmOK {
			delete(lm, l.itemName)
			if !loopMetaPreExisted && len(lm) == 0 {
				delete(md, loopScopeLoop)
			}
		}
		if !metadataPreExisted && len(md) == 0 {
			delete(rootRecord, loopScopeMetadata)
		}
	}()

	newArr := make([]any, 0, len(arr))
	for i, item := range arr {
		if err := ctx.Err(); err != nil {
			return err
		}

		aliasState["index"] = i

		_, originalIsMap := item.(map[string]any)
		scope := buildLoopScope(R, rootRecord, metadata, l.itemName, item, isNested)

		snap, err := jsonSnapshot(loopBranchFromRoot(rootRecord))
		if err != nil {
			return fmt.Errorf("snapshotting loop metadata: %w", err)
		}

		processed, err := l.runNested(ctx, scope)
		if err != nil {
			return fmt.Errorf("nested filters failed at item %d: %w", i, err)
		}

		if !jsonEqualSnapshot(loopBranchFromRoot(rootRecord), snap) {
			return fmt.Errorf("%w (alias %q, index %d)", ErrLoopMetadataMutated, l.itemName, i)
		}

		switch len(processed) {
		case 0:
			// Item dropped by nested filters — exclude from output array.
			continue
		case 1:
			updatedItem, hasAlias := processed[0][l.itemName]
			if !hasAlias {
				continue
			}
			if !originalIsMap {
				if _, newIsMap := updatedItem.(map[string]any); newIsMap {
					return fmt.Errorf("%w (item %d, type %T): nested writes silently auto-create a map under the item alias", ErrLoopNonObjectItem, i, item)
				}
			}
			newArr = append(newArr, updatedItem)
		default:
			return fmt.Errorf("%w: %d records returned at item %d", ErrLoopItemExpansion, len(processed), i)
		}
	}

	if err := recordpath.Set(R, l.field, newArr); err != nil {
		return fmt.Errorf("writing back array to %q: %w", l.field, err)
	}
	return nil
}

// runNested executes the nested filter chain on a single scope record.
func (l *LoopModule) runNested(ctx context.Context, scope map[string]any) ([]map[string]any, error) {
	records := []map[string]any{scope}
	for _, m := range l.modules {
		processed, err := m.Process(ctx, records)
		if err != nil {
			return nil, err
		}
		records = processed
		if len(records) == 0 {
			return records, nil
		}
	}
	return records, nil
}

// buildLoopScope returns the scope map passed to the nested filter chain.
//
// For the outermost loop (isNested=false) the scope is a fresh map containing
// only the reserved keys plus the current item alias. For nested loops the
// scope is a shallow copy of the parent scope so parent aliases remain
// visible to the inner filter chain.
func buildLoopScope(R, root, metadata map[string]any, itemName string, item any, isNested bool) map[string]any {
	if isNested {
		scope := make(map[string]any, len(R)+1)
		maps.Copy(scope, R)
		scope[itemName] = item
		return scope
	}
	return map[string]any{
		loopScopeRecord:   root,
		loopScopeMetadata: metadata,
		itemName:          item,
	}
}

// loopBranchFromRoot resolves rootRecord._metadata.loop by re-reading each
// reference so the loop module catches writes that replaced or deleted
// _metadata (or its loop child), not just in-place edits of the original
// map. Returns nil when any ancestor is absent or non-map.
func loopBranchFromRoot(root map[string]any) any {
	md, ok := root[loopScopeMetadata].(map[string]any)
	if !ok {
		return nil
	}
	return md[loopScopeLoop]
}

// ensureScopeMap returns the map stored under key, creating it on parent when
// absent. The boolean reports whether the map already existed; used by the
// caller to decide whether to clean up on exit.
func ensureScopeMap(parent map[string]any, key string) (map[string]any, bool) {
	if existing, ok := parent[key].(map[string]any); ok {
		return existing, true
	}
	created := make(map[string]any)
	parent[key] = created
	return created, false
}

// jsonSnapshot serializes a value to JSON. The serialized form is used as a
// stable, structural snapshot for write-detection on _metadata.loop. JSON is
// chosen because record values are already constrained to JSON-compatible
// types and map keys serialize deterministically (sorted).
func jsonSnapshot(v any) ([]byte, error) {
	return json.Marshal(v)
}

// jsonEqualSnapshot reports whether the JSON serialization of v matches snap.
func jsonEqualSnapshot(v any, snap []byte) bool {
	current, err := json.Marshal(v)
	if err != nil {
		return false
	}
	if len(current) != len(snap) {
		return false
	}
	for i := range current {
		if current[i] != snap[i] {
			return false
		}
	}
	return true
}

// Verify interface compliance at compile time.
var _ Module = (*LoopModule)(nil)
