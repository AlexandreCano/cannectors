// Package input provides implementations for input modules.
package input

import (
	"context"
	"log/slog"

	"github.com/canectors/runtime/internal/logger"
)

// StubModule is a placeholder input module for testing the pipeline flow.
// It returns sample data without connecting to any external system.
type StubModule struct {
	ModuleType string
	Endpoint   string
}

// NewStub creates a new stub input module.
func NewStub(moduleType, endpoint string) *StubModule {
	return &StubModule{
		ModuleType: moduleType,
		Endpoint:   endpoint,
	}
}

// Fetch returns sample data to demonstrate pipeline flow.
func (m *StubModule) Fetch(_ context.Context) ([]map[string]interface{}, error) {
	logger.Info("Input module fetching data",
		slog.String("type", m.ModuleType),
		slog.String("endpoint", m.Endpoint))

	return []map[string]interface{}{
		{"id": "1", "name": "Sample Record 1", "value": 100},
		{"id": "2", "name": "Sample Record 2", "value": 200},
	}, nil
}

// Close releases resources (no-op for stub).
func (m *StubModule) Close() error {
	return nil
}

// Verify StubModule implements Module
var _ Module = (*StubModule)(nil)
