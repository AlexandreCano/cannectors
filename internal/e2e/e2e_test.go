//go:build e2e

package e2e

import (
	"bytes"
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strconv"
	"sync"
	"testing"
	"time"

	"github.com/cannectors/runtime/internal/modules/filter"
	"github.com/cannectors/runtime/internal/modules/input"
	"github.com/cannectors/runtime/internal/modules/output"
	"github.com/cannectors/runtime/internal/persistence"
	runtimepkg "github.com/cannectors/runtime/internal/runtime"
	"github.com/cannectors/runtime/pkg/connector"

	_ "modernc.org/sqlite"
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

// mockCursorPaginatedSource serves records in two cursor pages, exposing the
// pagination state to the test via the returned counter.
func mockCursorPaginatedSource(t *testing.T) (*httptest.Server, *int, func()) {
	t.Helper()
	requests := 0
	handlerErrs := &handlerErrors{}

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requests++
		if r.Method != http.MethodGet {
			handlerErrs.addf("source method = %s, want GET", r.Method)
			http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
			return
		}

		w.Header().Set("Content-Type", "application/json")
		var payload map[string]any
		switch r.URL.Query().Get("cursor") {
		case "":
			payload = map[string]any{
				"results":     []map[string]any{{"id": "1"}, {"id": "2"}},
				"next_cursor": "page2",
			}
		case "page2":
			payload = map[string]any{
				"results":     []map[string]any{{"id": "3"}},
				"next_cursor": "",
			}
		default:
			handlerErrs.addf("unexpected cursor %q", r.URL.Query().Get("cursor"))
			http.Error(w, "unexpected cursor", http.StatusBadRequest)
			return
		}
		if err := json.NewEncoder(w).Encode(payload); err != nil {
			handlerErrs.addf("encode cursor response: %v", err)
		}
	}))

	return server, &requests, func() { handlerErrs.assertNoErrors(t) }
}

func TestExampleCursorPaginationPipeline(t *testing.T) {
	source, requests, assertSource := mockCursorPaginatedSource(t)
	defer source.Close()
	destination, captured, assertDestination := mockDestinationServer(t)
	defer destination.Close()

	inputModule, err := input.NewHTTPPollingFromConfig(rawModuleConfig(t, "httpPolling", map[string]any{
		"endpoint":  source.URL,
		"timeoutMs": 1000,
		"dataField": "results",
		"pagination": map[string]any{
			"type":            "cursor",
			"param":           "cursor",
			"nextCursorField": "next_cursor",
		},
	}))
	if err != nil {
		t.Fatalf("create http polling input: %v", err)
	}

	outputModule, err := output.NewHTTPRequestFromConfig(rawModuleConfig(t, "httpRequest", map[string]any{
		"endpoint": destination.URL,
		"method":   "POST",
		"success":  map[string]any{"statusCodes": []any{202}},
	}))
	if err != nil {
		t.Fatalf("create http request output: %v", err)
	}

	pipeline := &connector.Pipeline{
		ID:      "07-pagination-cursor",
		Name:    "07-pagination-cursor",
		Version: "e2e",
		Input:   &connector.ModuleConfig{Type: "httpPolling"},
		Output:  &connector.ModuleConfig{Type: "httpRequest"},
	}
	executor := runtimepkg.NewExecutorWithModules(inputModule, nil, outputModule, false)
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	result, err := executor.ExecuteWithContext(ctx, pipeline)
	assertSource()
	assertDestination()
	if err != nil {
		t.Fatalf("ExecuteWithContext() error = %v", err)
	}

	if result.RecordsProcessed != 3 {
		t.Fatalf("RecordsProcessed = %d, want 3", result.RecordsProcessed)
	}
	if *requests != 2 {
		t.Fatalf("source requests = %d, want 2 (initial + cursor page)", *requests)
	}
	if len(captured()) != 3 {
		t.Fatalf("captured records = %d, want 3", len(captured()))
	}
}

func TestExampleHTTPPollingToDatabaseOutput(t *testing.T) {
	source, assertSource := mockSourceServer(t, []map[string]any{
		{"id": "1", "name": "Apple"},
		{"id": "2", "name": "Banana"},
	})
	defer source.Close()

	tmpDir := t.TempDir()
	dbPath := filepath.Join(tmpDir, "products.db")
	setupDB, err := sql.Open("sqlite", "file:"+dbPath)
	if err != nil {
		t.Fatalf("open setup sqlite: %v", err)
	}
	if _, err := setupDB.Exec("CREATE TABLE products (id TEXT PRIMARY KEY, name TEXT NOT NULL)"); err != nil {
		t.Fatalf("create products table: %v", err)
	}
	if closeErr := setupDB.Close(); closeErr != nil {
		t.Fatalf("close setup sqlite: %v", closeErr)
	}

	inputModule, err := input.NewHTTPPollingFromConfig(rawModuleConfig(t, "httpPolling", map[string]any{
		"endpoint":  source.URL,
		"timeoutMs": 1000,
	}))
	if err != nil {
		t.Fatalf("create http polling input: %v", err)
	}

	outputModule, err := output.NewDatabaseOutputFromConfig(rawModuleConfig(t, "database", map[string]any{
		"connectionString": "file:" + dbPath,
		"driver":           "sqlite",
		"query":            "INSERT INTO products (id, name) VALUES ({{record.id}}, {{record.name}})",
		"timeoutMs":        1000,
	}))
	if err != nil {
		t.Fatalf("create database output: %v", err)
	}
	defer func() {
		if closeErr := outputModule.Close(); closeErr != nil {
			t.Errorf("close database output: %v", closeErr)
		}
	}()

	pipeline := &connector.Pipeline{
		ID:      "30-database-output-insert",
		Name:    "30-database-output-insert",
		Version: "e2e",
		Input:   &connector.ModuleConfig{Type: "httpPolling"},
		Output:  &connector.ModuleConfig{Type: "database"},
	}
	executor := runtimepkg.NewExecutorWithModules(inputModule, nil, outputModule, false)
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	result, err := executor.ExecuteWithContext(ctx, pipeline)
	assertSource()
	if err != nil {
		t.Fatalf("ExecuteWithContext() error = %v", err)
	}
	if result.RecordsProcessed != 2 {
		t.Fatalf("RecordsProcessed = %d, want 2", result.RecordsProcessed)
	}

	verifyDB, err := sql.Open("sqlite", "file:"+dbPath)
	if err != nil {
		t.Fatalf("open verify sqlite: %v", err)
	}
	defer func() {
		if closeErr := verifyDB.Close(); closeErr != nil {
			t.Errorf("close verify sqlite: %v", closeErr)
		}
	}()

	var count int
	if err := verifyDB.QueryRow("SELECT COUNT(*) FROM products").Scan(&count); err != nil {
		t.Fatalf("count products: %v", err)
	}
	if count != 2 {
		t.Fatalf("inserted rows = %d, want 2", count)
	}
	var name string
	if err := verifyDB.QueryRow("SELECT name FROM products WHERE id = ?", "1").Scan(&name); err != nil {
		t.Fatalf("select id=1: %v", err)
	}
	if name != "Apple" {
		t.Fatalf("name for id=1 = %q, want Apple", name)
	}
}

// mockPagePaginatedSource serves three pages with `total_pages: 3` so the
// http_polling input issues exactly three GETs.
func mockPagePaginatedSource(t *testing.T) (*httptest.Server, *int, func()) {
	t.Helper()
	requests := 0
	handlerErrs := &handlerErrors{}

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requests++
		if r.Method != http.MethodGet {
			handlerErrs.addf("method = %s, want GET", r.Method)
			http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
			return
		}
		page, _ := strconv.Atoi(r.URL.Query().Get("page"))
		if page == 0 {
			page = 1
		}
		w.Header().Set("Content-Type", "application/json")
		payload := map[string]any{
			"users":       []map[string]any{{"id": fmt.Sprintf("u-%d", page)}},
			"total_pages": 3,
		}
		if err := json.NewEncoder(w).Encode(payload); err != nil {
			handlerErrs.addf("encode page payload: %v", err)
		}
	}))

	return server, &requests, func() { handlerErrs.assertNoErrors(t) }
}

func TestExamplePagePaginationPipeline(t *testing.T) {
	source, requests, assertSource := mockPagePaginatedSource(t)
	defer source.Close()
	destination, captured, assertDestination := mockDestinationServer(t)
	defer destination.Close()

	inputModule, err := input.NewHTTPPollingFromConfig(rawModuleConfig(t, "httpPolling", map[string]any{
		"endpoint":  source.URL,
		"timeoutMs": 1000,
		"dataField": "users",
		"pagination": map[string]any{
			"type":            "page",
			"pageParam":       "page",
			"totalPagesField": "total_pages",
		},
	}))
	if err != nil {
		t.Fatalf("create http polling input: %v", err)
	}
	outputModule, err := output.NewHTTPRequestFromConfig(rawModuleConfig(t, "httpRequest", map[string]any{
		"endpoint": destination.URL,
		"method":   "POST",
		"success":  map[string]any{"statusCodes": []any{202}},
	}))
	if err != nil {
		t.Fatalf("create http request output: %v", err)
	}

	pipeline := &connector.Pipeline{
		ID: "05-pagination", Name: "05-pagination", Version: "e2e",
		Input:  &connector.ModuleConfig{Type: "httpPolling"},
		Output: &connector.ModuleConfig{Type: "httpRequest"},
	}
	executor := runtimepkg.NewExecutorWithModules(inputModule, nil, outputModule, false)
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	result, err := executor.ExecuteWithContext(ctx, pipeline)
	assertSource()
	assertDestination()
	if err != nil {
		t.Fatalf("ExecuteWithContext() error = %v", err)
	}
	if result.RecordsProcessed != 3 {
		t.Fatalf("RecordsProcessed = %d, want 3", result.RecordsProcessed)
	}
	if *requests != 3 {
		t.Fatalf("source requests = %d, want 3 (one per page)", *requests)
	}
	if len(captured()) != 3 {
		t.Fatalf("captured records = %d, want 3", len(captured()))
	}
}

// mockOffsetPaginatedSource serves five records in chunks of two using
// offset/limit query params, terminating when the offset reaches `total`.
func mockOffsetPaginatedSource(t *testing.T) (*httptest.Server, *int, func()) {
	t.Helper()
	requests := 0
	handlerErrs := &handlerErrors{}
	all := []map[string]any{
		{"id": "1"}, {"id": "2"}, {"id": "3"}, {"id": "4"}, {"id": "5"},
	}

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requests++
		offset, _ := strconv.Atoi(r.URL.Query().Get("offset"))
		limit, _ := strconv.Atoi(r.URL.Query().Get("limit"))
		if limit <= 0 {
			limit = len(all)
		}
		end := offset + limit
		if end > len(all) {
			end = len(all)
		}
		page := []map[string]any{}
		if offset < len(all) {
			page = all[offset:end]
		}
		w.Header().Set("Content-Type", "application/json")
		payload := map[string]any{"items": page, "total": len(all)}
		if err := json.NewEncoder(w).Encode(payload); err != nil {
			handlerErrs.addf("encode offset payload: %v", err)
		}
	}))

	return server, &requests, func() { handlerErrs.assertNoErrors(t) }
}

func TestExampleOffsetPaginationPipeline(t *testing.T) {
	source, requests, assertSource := mockOffsetPaginatedSource(t)
	defer source.Close()
	destination, captured, assertDestination := mockDestinationServer(t)
	defer destination.Close()

	inputModule, err := input.NewHTTPPollingFromConfig(rawModuleConfig(t, "httpPolling", map[string]any{
		"endpoint":  source.URL,
		"timeoutMs": 1000,
		"dataField": "items",
		"pagination": map[string]any{
			"type":       "offset",
			"param":      "offset",
			"limitParam": "limit",
			"limit":      2,
			"totalField": "total",
		},
	}))
	if err != nil {
		t.Fatalf("create http polling input: %v", err)
	}
	outputModule, err := output.NewHTTPRequestFromConfig(rawModuleConfig(t, "httpRequest", map[string]any{
		"endpoint": destination.URL,
		"method":   "POST",
		"success":  map[string]any{"statusCodes": []any{202}},
	}))
	if err != nil {
		t.Fatalf("create http request output: %v", err)
	}

	pipeline := &connector.Pipeline{
		ID: "06-pagination-offset", Name: "06-pagination-offset", Version: "e2e",
		Input:  &connector.ModuleConfig{Type: "httpPolling"},
		Output: &connector.ModuleConfig{Type: "httpRequest"},
	}
	executor := runtimepkg.NewExecutorWithModules(inputModule, nil, outputModule, false)
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	result, err := executor.ExecuteWithContext(ctx, pipeline)
	assertSource()
	assertDestination()
	if err != nil {
		t.Fatalf("ExecuteWithContext() error = %v", err)
	}
	if result.RecordsProcessed != 5 {
		t.Fatalf("RecordsProcessed = %d, want 5", result.RecordsProcessed)
	}
	// 5 records / 2 per page → 3 successful pages (2,2,1).
	if *requests != 3 {
		t.Fatalf("source requests = %d, want 3 (5 records / page size 2)", *requests)
	}
	if len(captured()) != 5 {
		t.Fatalf("captured records = %d, want 5", len(captured()))
	}
}

func TestExampleDatabaseInputToHTTPPipeline(t *testing.T) {
	tmpDir := t.TempDir()
	dbPath := filepath.Join(tmpDir, "users.db")
	setupDB, err := sql.Open("sqlite", "file:"+dbPath)
	if err != nil {
		t.Fatalf("open setup sqlite: %v", err)
	}
	if _, err := setupDB.Exec(`
		CREATE TABLE users (id TEXT PRIMARY KEY, name TEXT NOT NULL, email TEXT NOT NULL);
		INSERT INTO users (id, name, email) VALUES
			('u-1', 'Alice', 'alice@example.com'),
			('u-2', 'Bob', 'bob@example.com');
	`); err != nil {
		t.Fatalf("seed database: %v", err)
	}
	if closeErr := setupDB.Close(); closeErr != nil {
		t.Fatalf("close setup sqlite: %v", closeErr)
	}

	destination, captured, assertDestination := mockDestinationServer(t)
	defer destination.Close()

	inputModule, err := input.NewDatabaseInputFromConfig(rawModuleConfig(t, "database", map[string]any{
		"connectionString": "file:" + dbPath,
		"driver":           "sqlite",
		"query":            "SELECT id, name, email FROM users ORDER BY id",
		"timeoutMs":        1000,
	}))
	if err != nil {
		t.Fatalf("create database input: %v", err)
	}
	defer func() {
		if closeErr := inputModule.Close(); closeErr != nil {
			t.Errorf("close database input: %v", closeErr)
		}
	}()

	outputModule, err := output.NewHTTPRequestFromConfig(rawModuleConfig(t, "httpRequest", map[string]any{
		"endpoint": destination.URL,
		"method":   "POST",
		"success":  map[string]any{"statusCodes": []any{202}},
	}))
	if err != nil {
		t.Fatalf("create http request output: %v", err)
	}

	pipeline := &connector.Pipeline{
		ID: "27-database-input", Name: "27-database-input", Version: "e2e",
		Input:  &connector.ModuleConfig{Type: "database"},
		Output: &connector.ModuleConfig{Type: "httpRequest"},
	}
	executor := runtimepkg.NewExecutorWithModules(inputModule, nil, outputModule, false)
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	result, err := executor.ExecuteWithContext(ctx, pipeline)
	assertDestination()
	if err != nil {
		t.Fatalf("ExecuteWithContext() error = %v", err)
	}
	if result.RecordsProcessed != 2 {
		t.Fatalf("RecordsProcessed = %d, want 2", result.RecordsProcessed)
	}
	got := captured()
	if len(got) != 2 {
		t.Fatalf("captured records = %d, want 2", len(got))
	}
	if got[0]["id"] != "u-1" || got[0]["email"] != "alice@example.com" {
		t.Fatalf("first record = %#v, want id=u-1 email=alice@example.com", got[0])
	}
}

// mockPersistenceSource captures the query parameters used by the polling
// input on each call so the test can verify state-driven filtering across
// successive runs.
func mockPersistenceSource(t *testing.T, payload func(reqIdx int) []map[string]any) (*httptest.Server, *[]map[string]string, func()) {
	t.Helper()
	calls := []map[string]string{}
	handlerErrs := &handlerErrors{}

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		params := map[string]string{}
		for k, v := range r.URL.Query() {
			if len(v) > 0 {
				params[k] = v[0]
			}
		}
		idx := len(calls)
		calls = append(calls, params)

		w.Header().Set("Content-Type", "application/json")
		if err := json.NewEncoder(w).Encode(payload(idx)); err != nil {
			handlerErrs.addf("encode persistence payload: %v", err)
		}
	}))

	return server, &calls, func() { handlerErrs.assertNoErrors(t) }
}

func TestExampleTimestampPersistencePipeline(t *testing.T) {
	source, calls, assertSource := mockPersistenceSource(t, func(idx int) []map[string]any {
		// First run: two records. Subsequent runs: empty.
		if idx == 0 {
			return []map[string]any{
				{"id": "1", "updated_at": "2026-01-01T10:00:00Z"},
				{"id": "2", "updated_at": "2026-01-02T10:00:00Z"},
			}
		}
		return []map[string]any{}
	})
	defer source.Close()
	destination, _, assertDestination := mockDestinationServer(t)
	defer destination.Close()

	stateStore := persistence.NewStateStore(t.TempDir())

	runOnce := func(name string) *connector.ExecutionResult {
		t.Helper()
		inputModule, err := input.NewHTTPPollingFromConfig(rawModuleConfig(t, "httpPolling", map[string]any{
			"endpoint":  source.URL,
			"timeoutMs": 1000,
			"statePersistence": map[string]any{
				"timestamp": map[string]any{
					"enabled":    true,
					"queryParam": "updated_after",
				},
			},
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

		executor := runtimepkg.NewExecutorWithModules(inputModule, nil, outputModule, false)
		executor.SetStateStore(stateStore)
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		pipeline := &connector.Pipeline{
			ID: "18-timestamp-persistence", Name: "18-timestamp-persistence", Version: "e2e",
			Input:  &connector.ModuleConfig{Type: "httpPolling"},
			Output: &connector.ModuleConfig{Type: "httpRequest"},
		}
		result, err := executor.ExecuteWithContext(ctx, pipeline)
		if err != nil {
			t.Fatalf("%s: ExecuteWithContext() error = %v", name, err)
		}
		return result
	}

	first := runOnce("first")
	if first.RecordsProcessed != 2 {
		t.Fatalf("first run RecordsProcessed = %d, want 2", first.RecordsProcessed)
	}
	second := runOnce("second")
	if second.RecordsProcessed != 0 {
		t.Fatalf("second run RecordsProcessed = %d, want 0 (state filters everything)", second.RecordsProcessed)
	}

	assertSource()
	assertDestination()
	if len(*calls) != 2 {
		t.Fatalf("source calls = %d, want 2", len(*calls))
	}
	if _, ok := (*calls)[0]["updated_after"]; ok {
		t.Fatalf("first call had updated_after = %q, want absent", (*calls)[0]["updated_after"])
	}
	if (*calls)[1]["updated_after"] == "" {
		t.Fatal("second call missing updated_after query param")
	}
}

func TestExampleIDPersistencePipeline(t *testing.T) {
	source, calls, assertSource := mockPersistenceSource(t, func(idx int) []map[string]any {
		if idx == 0 {
			return []map[string]any{{"id": "10"}, {"id": "11"}, {"id": "12"}}
		}
		return []map[string]any{}
	})
	defer source.Close()
	destination, _, assertDestination := mockDestinationServer(t)
	defer destination.Close()

	stateStore := persistence.NewStateStore(t.TempDir())

	runOnce := func(name string) *connector.ExecutionResult {
		t.Helper()
		inputModule, err := input.NewHTTPPollingFromConfig(rawModuleConfig(t, "httpPolling", map[string]any{
			"endpoint":  source.URL,
			"timeoutMs": 1000,
			"statePersistence": map[string]any{
				"id": map[string]any{
					"enabled":    true,
					"field":      "id",
					"queryParam": "since_id",
				},
			},
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

		executor := runtimepkg.NewExecutorWithModules(inputModule, nil, outputModule, false)
		executor.SetStateStore(stateStore)
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		pipeline := &connector.Pipeline{
			ID: "19-id-persistence", Name: "19-id-persistence", Version: "e2e",
			Input:  &connector.ModuleConfig{Type: "httpPolling"},
			Output: &connector.ModuleConfig{Type: "httpRequest"},
		}
		result, err := executor.ExecuteWithContext(ctx, pipeline)
		if err != nil {
			t.Fatalf("%s: ExecuteWithContext() error = %v", name, err)
		}
		return result
	}

	first := runOnce("first")
	if first.RecordsProcessed != 3 {
		t.Fatalf("first run RecordsProcessed = %d, want 3", first.RecordsProcessed)
	}
	second := runOnce("second")
	if second.RecordsProcessed != 0 {
		t.Fatalf("second run RecordsProcessed = %d, want 0 (state filters everything)", second.RecordsProcessed)
	}

	assertSource()
	assertDestination()
	if len(*calls) != 2 {
		t.Fatalf("source calls = %d, want 2", len(*calls))
	}
	if _, ok := (*calls)[0]["since_id"]; ok {
		t.Fatalf("first call had since_id = %q, want absent", (*calls)[0]["since_id"])
	}
	if (*calls)[1]["since_id"] != "12" {
		t.Fatalf("second call since_id = %q, want %q (max id from first run)", (*calls)[1]["since_id"], "12")
	}
}

func TestExampleWebhookPipeline(t *testing.T) {
	destination, captured, assertDestination := mockDestinationServer(t)
	defer destination.Close()

	webhook, err := input.NewWebhookFromConfig(rawModuleConfig(t, "webhook", map[string]any{
		"path":          "/webhook",
		"listenAddress": "127.0.0.1:0", // ephemeral port for parallel safety
		"timeoutMs":     1000,
	}))
	if err != nil {
		t.Fatalf("create webhook input: %v", err)
	}

	outputModule, err := output.NewHTTPRequestFromConfig(rawModuleConfig(t, "httpRequest", map[string]any{
		"endpoint": destination.URL,
		"method":   "POST",
		"success":  map[string]any{"statusCodes": []any{202}},
	}))
	if err != nil {
		t.Fatalf("create http request output: %v", err)
	}

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	// Start the webhook server in a goroutine; the handler forwards each
	// payload through the output module the same way the runtime executor
	// would for an event-driven input.
	serverErr := make(chan error, 1)
	go func() {
		serverErr <- webhook.Start(ctx, func(ctx context.Context, data []map[string]any) error {
			_, sendErr := outputModule.Send(ctx, data)
			return sendErr
		})
	}()

	// Wait for the listener to bind.
	deadline := time.Now().Add(2 * time.Second)
	for !webhook.IsRunning() {
		if time.Now().After(deadline) {
			t.Fatal("webhook did not start within 2s")
		}
		time.Sleep(10 * time.Millisecond)
	}

	body := bytes.NewBufferString(`{"id":"evt-1","type":"order.created"}`)
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, "http://"+webhook.Address()+"/webhook", body)
	if err != nil {
		t.Fatalf("build webhook request: %v", err)
	}
	req.Header.Set("Content-Type", "application/json")

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("send webhook request: %v", err)
	}
	if closeErr := resp.Body.Close(); closeErr != nil {
		t.Errorf("close webhook response: %v", closeErr)
	}
	if resp.StatusCode >= 400 {
		t.Fatalf("webhook response status = %d, want < 400", resp.StatusCode)
	}

	// Wait for the handler to dispatch to the destination, then shut down.
	deadline = time.Now().Add(2 * time.Second)
	for len(captured()) == 0 {
		if time.Now().After(deadline) {
			t.Fatal("destination never received forwarded record")
		}
		time.Sleep(10 * time.Millisecond)
	}

	cancel()
	if err := <-serverErr; err != nil && err != context.Canceled {
		t.Fatalf("webhook Start returned error: %v", err)
	}

	assertDestination()
	got := captured()
	if len(got) != 1 {
		t.Fatalf("captured = %d, want 1", len(got))
	}
	if got[0]["id"] != "evt-1" || got[0]["type"] != "order.created" {
		t.Fatalf("captured payload = %#v, want id=evt-1 type=order.created", got[0])
	}
}
