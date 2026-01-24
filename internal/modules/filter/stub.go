// Package filter provides implementations for filter modules.
package filter

import (
	"context"
	"log/slog"

	"github.com/canectors/runtime/internal/logger"
)

// StubModule is a placeholder filter module for testing the pipeline flow.
// It passes through records unchanged.
type StubModule struct {
	ModuleType string
	Index      int
}

// NewStub creates a new stub filter module.
func NewStub(moduleType string, index int) *StubModule {
	return &StubModule{
		ModuleType: moduleType,
		Index:      index,
	}
}

// Process passes through records unchanged (stub behavior).
func (m *StubModule) Process(_ context.Context, records []map[string]interface{}) ([]map[string]interface{}, error) {
	logger.Info("Filter module processing data",
		slog.String("type", m.ModuleType),
		slog.Int("index", m.Index),
		slog.Int("records", len(records)))

	return records, nil
}

// Verify StubModule implements Module
var _ Module = (*StubModule)(nil)
