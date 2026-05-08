// Trace ID propagation tests for the runtime executor.
package runtime

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/cannectors/runtime/internal/logger"
	"github.com/cannectors/runtime/internal/modules/output"
	"github.com/cannectors/runtime/pkg/connector"
)

type safeBuffer struct {
	mu  sync.Mutex
	buf strings.Builder
}

func (b *safeBuffer) Write(p []byte) (int, error) {
	b.mu.Lock()
	defer b.mu.Unlock()
	return b.buf.Write(p)
}

func (b *safeBuffer) String() string {
	b.mu.Lock()
	defer b.mu.Unlock()
	return b.buf.String()
}

// captureLogger swaps the package-global logger for one writing to a buffer
// and returns the buffer plus a restore function.
func captureLogger(t *testing.T) (*safeBuffer, func()) {
	t.Helper()
	buf := &safeBuffer{}
	original := logger.Logger
	logger.Logger = slog.New(slog.NewJSONHandler(buf, &slog.HandlerOptions{
		Level: slog.LevelDebug,
	}))
	return buf, func() { logger.Logger = original }
}

// collectTraceIDs scans every JSON log line and returns the unique set of
// trace_id values it found.
func collectTraceIDs(t *testing.T, raw string) map[string]int {
	t.Helper()
	out := map[string]int{}
	for _, line := range strings.Split(strings.TrimSpace(raw), "\n") {
		if line == "" {
			continue
		}
		var entry map[string]any
		if err := json.Unmarshal([]byte(line), &entry); err != nil {
			t.Fatalf("invalid JSON log line %q: %v", line, err)
		}
		if id, ok := entry["trace_id"].(string); ok && id != "" {
			out[id]++
		}
	}
	return out
}

func TestExecute_PropagatesTraceID_AllLogsShareSameID(t *testing.T) {
	buf, restore := captureLogger(t)
	defer restore()

	mockInput := NewMockInputModule([]map[string]any{{"id": "1"}}, nil)
	mockOutput := NewMockOutputModule(nil)
	pipeline := &connector.Pipeline{
		ID:      "trace-test",
		Name:    "Trace Test",
		Version: "1.0.0",
		Enabled: true,
	}
	executor := NewExecutorWithModules(mockInput, nil, mockOutput, false)

	if _, err := executor.Execute(pipeline); err != nil {
		t.Fatalf("Execute returned error: %v", err)
	}

	ids := collectTraceIDs(t, buf.String())
	if len(ids) != 1 {
		t.Fatalf("expected exactly 1 unique trace_id across logs, got %d: %#v", len(ids), ids)
	}
	for id, count := range ids {
		if id == "" {
			t.Fatal("trace_id is empty")
		}
		// Sanity check: at minimum execution start + each stage start + execution end.
		if count < 3 {
			t.Fatalf("expected trace_id %q to appear in at least 3 log lines, got %d", id, count)
		}
	}
}

func TestExecute_ParallelExecutions_HaveDistinctTraceIDs(t *testing.T) {
	buf, restore := captureLogger(t)
	defer restore()

	pipeline := &connector.Pipeline{
		ID:      "trace-parallel",
		Name:    "Trace Parallel",
		Version: "1.0.0",
		Enabled: true,
	}

	const N = 5
	var wg sync.WaitGroup
	wg.Add(N)
	for i := 0; i < N; i++ {
		go func() {
			defer wg.Done()
			mockInput := NewMockInputModule([]map[string]any{{"id": "1"}}, nil)
			mockOutput := NewMockOutputModule(nil)
			executor := NewExecutorWithModules(mockInput, nil, mockOutput, false)
			if _, err := executor.Execute(pipeline); err != nil {
				t.Errorf("Execute returned error: %v", err)
			}
		}()
	}
	wg.Wait()

	ids := collectTraceIDs(t, buf.String())
	if len(ids) < N {
		t.Fatalf("expected >= %d distinct trace_ids, got %d: %#v", N, len(ids), ids)
	}
}

func TestExecuteWithContext_KeepsCallerTraceID(t *testing.T) {
	buf, restore := captureLogger(t)
	defer restore()

	const presetID = "caller-supplied-trace-id"
	ctx := logger.WithTraceID(context.Background(), presetID)

	mockInput := NewMockInputModule([]map[string]any{{"id": "1"}}, nil)
	mockOutput := NewMockOutputModule(nil)
	pipeline := &connector.Pipeline{
		ID:      "trace-preset",
		Name:    "Trace Preset",
		Version: "1.0.0",
		Enabled: true,
	}
	executor := NewExecutorWithModules(mockInput, nil, mockOutput, false)

	if _, err := executor.ExecuteWithContext(ctx, pipeline); err != nil {
		t.Fatalf("ExecuteWithContext returned error: %v", err)
	}

	ids := collectTraceIDs(t, buf.String())
	if _, ok := ids[presetID]; !ok {
		t.Fatalf("expected preset trace_id %q to appear in logs, got %#v", presetID, ids)
	}
}

func TestExecuteWithRecordsContext_KeepsCallerTraceID(t *testing.T) {
	buf, restore := captureLogger(t)
	defer restore()

	const presetID = "webhook-trace-id"
	ctx := logger.WithTraceID(context.Background(), presetID)

	mockOutput := NewMockOutputModule(nil)
	pipeline := &connector.Pipeline{
		ID:      "trace-records",
		Name:    "Trace Records",
		Version: "1.0.0",
		Enabled: true,
	}
	executor := NewExecutorWithModules(nil, nil, mockOutput, false)

	if _, err := executor.ExecuteWithRecordsContext(ctx, pipeline, []map[string]any{{"id": "1"}}); err != nil {
		t.Fatalf("ExecuteWithRecordsContext returned error: %v", err)
	}

	ids := collectTraceIDs(t, buf.String())
	if _, ok := ids[presetID]; !ok {
		t.Fatalf("expected preset trace_id %q to appear in logs, got %#v", presetID, ids)
	}
}

type collectingOutputModule struct {
	mu   sync.Mutex
	seen map[string]int
}

func (m *collectingOutputModule) Send(_ context.Context, records []map[string]any) (int, error) {
	time.Sleep(2 * time.Millisecond) // Encourage overlap between goroutines.
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.seen == nil {
		m.seen = make(map[string]int)
	}
	for _, record := range records {
		id, _ := record["id"].(string)
		m.seen[id]++
	}
	return len(records), nil
}

func (m *collectingOutputModule) Close() error { return nil }

var _ output.Module = (*collectingOutputModule)(nil)

func TestExecuteWithRecordsContext_ParallelCalls_IsolatedInputRecords(t *testing.T) {
	output := &collectingOutputModule{}
	executor := NewExecutorWithModules(nil, nil, output, false)
	pipeline := &connector.Pipeline{
		ID:      "records-parallel",
		Name:    "Records Parallel",
		Version: "1.0.0",
		Enabled: true,
	}

	const N = 50
	var (
		wg    sync.WaitGroup
		errMu sync.Mutex
		errs  []error
	)
	wg.Add(N)
	for i := 0; i < N; i++ {
		i := i
		go func() {
			defer wg.Done()
			records := []map[string]any{{"id": fmt.Sprintf("r-%d", i)}}
			if _, err := executor.ExecuteWithRecordsContext(context.Background(), pipeline, records); err != nil {
				errMu.Lock()
				errs = append(errs, err)
				errMu.Unlock()
			}
		}()
	}
	wg.Wait()

	if len(errs) > 0 {
		t.Fatalf("parallel ExecuteWithRecordsContext returned errors: %v", errs)
	}

	output.mu.Lock()
	defer output.mu.Unlock()
	if len(output.seen) != N {
		t.Fatalf("expected %d distinct record IDs, got %d (%v)", N, len(output.seen), output.seen)
	}
	for i := 0; i < N; i++ {
		id := fmt.Sprintf("r-%d", i)
		if output.seen[id] != 1 {
			t.Fatalf("record %s processed %d times, want 1", id, output.seen[id])
		}
	}
}
