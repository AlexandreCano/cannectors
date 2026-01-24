// Package output provides implementations for output modules.
package output

import (
	"context"
	"fmt"
	"log/slog"

	"github.com/canectors/runtime/internal/logger"
)

// StubModule is a placeholder output module for testing the pipeline flow.
// It implements both Module and PreviewableModule for dry-run support.
type StubModule struct {
	ModuleType string
	Endpoint   string
	Method     string
}

// NewStub creates a new stub output module.
func NewStub(moduleType, endpoint, method string) *StubModule {
	return &StubModule{
		ModuleType: moduleType,
		Endpoint:   endpoint,
		Method:     method,
	}
}

// Send simulates sending records (stub behavior).
func (m *StubModule) Send(_ context.Context, records []map[string]interface{}) (int, error) {
	logger.Info("Output module sending data",
		slog.String("type", m.ModuleType),
		slog.String("endpoint", m.Endpoint),
		slog.String("method", m.Method),
		slog.Int("records", len(records)))

	return len(records), nil
}

// Close releases resources (no-op for stub).
func (m *StubModule) Close() error {
	return nil
}

// PreviewRequest generates a preview of what would be sent (for dry-run mode).
func (m *StubModule) PreviewRequest(records []map[string]interface{}, _ PreviewOptions) ([]RequestPreview, error) {
	if len(records) == 0 {
		return nil, nil
	}

	bodyPreview := fmt.Sprintf("[%d records would be sent]", len(records))

	return []RequestPreview{
		{
			Endpoint: m.Endpoint,
			Method:   m.Method,
			Headers: map[string]string{
				"Content-Type": "application/json",
				"User-Agent":   "Canectors-Runtime/1.0 (stub)",
			},
			BodyPreview: bodyPreview,
			RecordCount: len(records),
		},
	}, nil
}

// Verify StubModule implements Module and PreviewableModule
var _ Module = (*StubModule)(nil)
var _ PreviewableModule = (*StubModule)(nil)
