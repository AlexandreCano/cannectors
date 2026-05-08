//go:build e2e

package e2e

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"sync"
	"testing"
	"time"

	"github.com/cannectors/runtime/internal/modules/filter"
	"github.com/cannectors/runtime/internal/modules/input"
	"github.com/cannectors/runtime/internal/modules/output"
	runtimepkg "github.com/cannectors/runtime/internal/runtime"
	"github.com/cannectors/runtime/pkg/connector"
)

type e2eResult struct {
	execution *connector.ExecutionResult
	captured  []map[string]any
}

type handlerErrors struct {
	mu   sync.Mutex
	errs []error
}

func (e *handlerErrors) addf(format string, args ...any) {
	e.mu.Lock()
	defer e.mu.Unlock()
	e.errs = append(e.errs, fmt.Errorf(format, args...))
}

func (e *handlerErrors) assertNoErrors(t *testing.T) {
	t.Helper()
	e.mu.Lock()
	defer e.mu.Unlock()
	if len(e.errs) > 0 {
		t.Fatalf("httptest handler errors: %v", e.errs)
	}
}

func rawModuleConfig(t *testing.T, typ string, cfg map[string]any) *connector.ModuleConfig {
	t.Helper()
	raw, err := json.Marshal(cfg)
	if err != nil {
		t.Fatalf("marshal module config: %v", err)
	}
	return &connector.ModuleConfig{Type: typ, Raw: raw}
}

func mockSourceServer(t *testing.T, records []map[string]any) (*httptest.Server, func()) {
	t.Helper()
	handlerErrs := &handlerErrors{}
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet {
			handlerErrs.addf("source method = %s, want GET", r.Method)
			http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		if err := json.NewEncoder(w).Encode(records); err != nil {
			handlerErrs.addf("encode source response: %v", err)
			return
		}
	}))
	return server, func() { handlerErrs.assertNoErrors(t) }
}

func mockDestinationServer(t *testing.T) (*httptest.Server, func() []map[string]any, func()) {
	t.Helper()
	var mu sync.Mutex
	captured := make([]map[string]any, 0)
	handlerErrs := &handlerErrors{}

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			handlerErrs.addf("destination method = %s, want POST", r.Method)
			http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
			return
		}
		body, err := io.ReadAll(r.Body)
		if err != nil {
			handlerErrs.addf("read destination body: %v", err)
			http.Error(w, "read body failed", http.StatusBadRequest)
			return
		}

		var batch []map[string]any
		if err := json.Unmarshal(body, &batch); err == nil {
			mu.Lock()
			captured = append(captured, batch...)
			mu.Unlock()
			w.WriteHeader(http.StatusAccepted)
			return
		}

		var single map[string]any
		if err := json.Unmarshal(body, &single); err != nil {
			handlerErrs.addf("decode destination body %s: %v", string(body), err)
			http.Error(w, "invalid json", http.StatusBadRequest)
			return
		}
		mu.Lock()
		captured = append(captured, single)
		mu.Unlock()
		w.WriteHeader(http.StatusAccepted)
	}))

	snapshot := func() []map[string]any {
		mu.Lock()
		defer mu.Unlock()
		out := make([]map[string]any, len(captured))
		copy(out, captured)
		return out
	}

	return server, snapshot, func() { handlerErrs.assertNoErrors(t) }
}

func runHTTPPipeline(t *testing.T, name string, records []map[string]any, filters []filter.Module) e2eResult {
	t.Helper()

	source, assertSource := mockSourceServer(t, records)
	defer source.Close()
	destination, captured, assertDestination := mockDestinationServer(t)
	defer destination.Close()

	inputModule, err := input.NewHTTPPollingFromConfig(rawModuleConfig(t, "httpPolling", map[string]any{
		"endpoint":  source.URL,
		"timeoutMs": 1000,
	}))
	if err != nil {
		t.Fatalf("%s: create http polling input: %v", name, err)
	}

	outputModule, err := output.NewHTTPRequestFromConfig(rawModuleConfig(t, "httpRequest", map[string]any{
		"endpoint": destination.URL,
		"method":   "POST",
		"success":  map[string]any{"statusCodes": []any{202}},
	}))
	if err != nil {
		t.Fatalf("%s: create http request output: %v", name, err)
	}

	pipeline := &connector.Pipeline{
		ID:      name,
		Name:    name,
		Version: "e2e",
		Input:   &connector.ModuleConfig{Type: "httpPolling"},
		Output:  &connector.ModuleConfig{Type: "httpRequest"},
	}
	executor := runtimepkg.NewExecutorWithModules(inputModule, filters, outputModule, false)
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	result, err := executor.ExecuteWithContext(ctx, pipeline)
	assertSource()
	assertDestination()
	if err != nil {
		t.Fatalf("%s: ExecuteWithContext() error = %v", name, err)
	}
	return e2eResult{execution: result, captured: captured()}
}

func TestExampleHTTPPipelines(t *testing.T) {
	t.Run("01-simple HTTP polling to HTTP request", func(t *testing.T) {
		result := runHTTPPipeline(t, "01-simple", []map[string]any{
			{"id": "1", "name": "Alice"},
			{"id": "2", "name": "Bob"},
		}, nil)

		if result.execution.RecordsProcessed != 2 {
			t.Fatalf("RecordsProcessed = %d, want 2", result.execution.RecordsProcessed)
		}
		if len(result.captured) != 2 {
			t.Fatalf("captured records = %d, want 2", len(result.captured))
		}
	})

	t.Run("08-filters-mapping HTTP polling mapping to HTTP request", func(t *testing.T) {
		mapping, err := filter.NewMappingFromConfig([]filter.FieldMapping{
			{Source: stringPtr("name"), Target: "fullName"},
			{Source: stringPtr("id"), Target: "externalID"},
		}, "fail")
		if err != nil {
			t.Fatalf("create mapping filter: %v", err)
		}

		result := runHTTPPipeline(t, "08-filters-mapping", []map[string]any{
			{"id": "1", "name": "Alice"},
		}, []filter.Module{mapping})

		if got := result.captured[0]["fullName"]; got != "Alice" {
			t.Fatalf("fullName = %v, want Alice", got)
		}
		if got := result.captured[0]["externalID"]; got != "1" {
			t.Fatalf("externalID = %v, want 1", got)
		}
	})

	t.Run("09-filters-condition HTTP polling condition to HTTP request", func(t *testing.T) {
		condition, err := filter.NewConditionFromConfig(filter.ConditionConfig{
			Expression: `status == "active"`,
		}, nil)
		if err != nil {
			t.Fatalf("create condition filter: %v", err)
		}

		result := runHTTPPipeline(t, "09-filters-condition", []map[string]any{
			{"id": "1", "status": "active"},
			{"id": "2", "status": "inactive"},
		}, []filter.Module{condition})

		if len(result.captured) != 1 {
			t.Fatalf("captured records = %d, want 1", len(result.captured))
		}
		if got := result.captured[0]["id"]; got != "1" {
			t.Fatalf("captured id = %v, want 1", got)
		}
	})
}

func stringPtr(s string) *string {
	return &s
}
