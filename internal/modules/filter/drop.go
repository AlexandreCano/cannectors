// Package filter provides implementations for filter modules.
// This file implements the "drop" filter module which removes every record it
// receives from the stream. It is typically placed inside a `condition` branch
// to make record filtering explicit (see Story 24.9).
package filter

import (
	"context"
	"log/slog"

	"github.com/cannectors/runtime/internal/logger"
)

// DropModule is the explicit drop filter: it discards every record it receives.
type DropModule struct{}

// NewDrop creates a new drop filter module.
func NewDrop() *DropModule {
	return &DropModule{}
}

// Process implements filter.Module by returning an empty slice regardless of
// input. Context cancellation is honored to keep behavior consistent with the
// other filter modules.
func (m *DropModule) Process(ctx context.Context, records []map[string]any) ([]map[string]any, error) {
	select {
	case <-ctx.Done():
		return nil, ctx.Err()
	default:
	}
	if len(records) > 0 {
		logger.Debug("drop filter discarded records", slog.Int("count", len(records)))
	}
	return []map[string]any{}, nil
}

// Verify interface compliance at compile time
var _ Module = (*DropModule)(nil)
