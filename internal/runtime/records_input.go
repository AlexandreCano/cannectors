package runtime

import (
	"context"

	"github.com/cannectors/runtime/internal/modules/input"
)

// recordsInputModule is an input.Module that returns a fixed set of pre-fetched
// records. It is used internally by ExecuteWithRecordsContext to reuse the
// same execution path as ExecuteWithContext, avoiding two parallel pipeline
// implementations that drifted out of sync (story 19.5).
type recordsInputModule struct {
	records []map[string]any
}

func newRecordsInput(records []map[string]any) *recordsInputModule {
	return &recordsInputModule{records: records}
}

// Fetch implements input.Module by returning the captured records.
func (r *recordsInputModule) Fetch(_ context.Context) ([]map[string]any, error) {
	return r.records, nil
}

// Close implements input.Module. There is nothing to release.
func (r *recordsInputModule) Close() error { return nil }

// Compile-time check.
var _ input.Module = (*recordsInputModule)(nil)
