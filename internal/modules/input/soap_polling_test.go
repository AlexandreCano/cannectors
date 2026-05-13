package input

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/cannectors/runtime/internal/modules/soaputil"
	"github.com/cannectors/runtime/internal/persistence"
	"github.com/cannectors/runtime/pkg/connector"
)

func TestSOAPPolling_FetchExtractsDataField(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body, _ := io.ReadAll(r.Body)
		if !strings.Contains(string(body), `<GetOrders`) {
			t.Fatalf("request body missing operation:\n%s", body)
		}
		w.Header().Set("Content-Type", "text/xml")
		_, _ = w.Write([]byte(`<Envelope><Body><GetOrdersResponse><Orders><Order><id>1</id></Order><Order><id>2</id></Order></Orders></GetOrdersResponse></Body></Envelope>`))
	}))
	defer server.Close()

	raw, err := json.Marshal(map[string]any{
		"endpoint":  server.URL,
		"operation": "GetOrders",
		"body":      `<GetOrders/>`,
		"dataField": "Envelope.Body.GetOrdersResponse.Orders.Order",
	})
	if err != nil {
		t.Fatal(err)
	}
	module, err := NewSOAPPollingFromConfig(&connector.ModuleConfig{Type: "soapPolling", Raw: raw})
	if err != nil {
		t.Fatalf("NewSOAPPollingFromConfig: %v", err)
	}
	defer func() { _ = module.Close() }()

	records, err := module.Fetch(context.Background())
	if err != nil {
		t.Fatalf("Fetch: %v", err)
	}
	if len(records) != 2 {
		t.Fatalf("expected 2 records, got %d: %#v", len(records), records)
	}
	if records[0]["id"] != "1" || records[1]["id"] != "2" {
		t.Fatalf("unexpected records: %#v", records)
	}
}

func TestSOAPPolling_DataFieldMissingReturnsError(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "text/xml")
		_, _ = w.Write([]byte(`<Envelope><Body><Response/></Body></Envelope>`))
	}))
	defer server.Close()

	module, err := NewSOAPPollingFromConfig(&connector.ModuleConfig{Type: "soapPolling", Raw: mustSOAPJSON(map[string]any{
		"endpoint":  server.URL,
		"operation": "List",
		"body":      `<List/>`,
		"dataField": "Envelope.Body.Response.Items.Item",
	})})
	if err != nil {
		t.Fatalf("NewSOAPPollingFromConfig: %v", err)
	}
	defer func() { _ = module.Close() }()

	_, err = module.Fetch(context.Background())
	if !errors.Is(err, soaputil.ErrInvalidDataField) {
		t.Fatalf("expected ErrInvalidDataField, got %T: %v", err, err)
	}
}

func TestSOAPPolling_PaginationStrategies(t *testing.T) {
	t.Run("page", func(t *testing.T) {
		var calls int32
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			call := atomic.AddInt32(&calls, 1)
			body, _ := io.ReadAll(r.Body)
			if !strings.Contains(string(body), "<Page>"+strconvI32(call)+"</Page>") {
				t.Fatalf("request missing page %d:\n%s", call, body)
			}
			w.Header().Set("Content-Type", "text/xml")
			_, _ = w.Write([]byte(`<Envelope><Body><Response><Items><Item><id>` + strconvI32(call) + `</id></Item></Items><TotalPages>2</TotalPages></Response></Body></Envelope>`))
		}))
		defer server.Close()
		module := newSOAPTestPolling(t, map[string]any{
			"endpoint":   server.URL,
			"operation":  "List",
			"body":       `<List><Page>{{record.pagination.page}}</Page></List>`,
			"dataField":  "Envelope.Body.Response.Items.Item",
			"pagination": map[string]any{"type": "page", "param": "page", "totalPagesField": "Envelope.Body.Response.TotalPages"},
		})
		defer func() { _ = module.Close() }()
		records, err := module.Fetch(context.Background())
		if err != nil {
			t.Fatalf("Fetch: %v", err)
		}
		if len(records) != 2 {
			t.Fatalf("records = %#v", records)
		}
	})

	t.Run("offset", func(t *testing.T) {
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			body, _ := io.ReadAll(r.Body)
			offset := "0"
			if strings.Contains(string(body), "<Offset>2</Offset>") {
				offset = "2"
			}
			w.Header().Set("Content-Type", "text/xml")
			_, _ = w.Write([]byte(`<Envelope><Body><Response><Items><Item><id>` + offset + `a</id></Item><Item><id>` + offset + `b</id></Item></Items><Total>4</Total></Response></Body></Envelope>`))
		}))
		defer server.Close()
		module := newSOAPTestPolling(t, map[string]any{
			"endpoint":   server.URL,
			"operation":  "List",
			"body":       `<List><Offset>{{record.pagination.offset}}</Offset><Limit>{{record.pagination.limit}}</Limit></List>`,
			"dataField":  "Envelope.Body.Response.Items.Item",
			"pagination": map[string]any{"type": "offset", "param": "offset", "limitParam": "limit", "limit": 2, "totalField": "Envelope.Body.Response.Total"},
		})
		defer func() { _ = module.Close() }()
		records, err := module.Fetch(context.Background())
		if err != nil {
			t.Fatalf("Fetch: %v", err)
		}
		if len(records) != 4 {
			t.Fatalf("records = %#v", records)
		}
	})

	t.Run("cursor", func(t *testing.T) {
		var calls int32
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			call := atomic.AddInt32(&calls, 1)
			body, _ := io.ReadAll(r.Body)
			if call == 2 && !strings.Contains(string(body), "<Cursor>next</Cursor>") {
				t.Fatalf("request missing next cursor:\n%s", body)
			}
			next := "next"
			if call == 2 {
				next = ""
			}
			w.Header().Set("Content-Type", "text/xml")
			_, _ = w.Write([]byte(`<Envelope><Body><Response><Items><Item><id>` + strconvI32(call) + `</id></Item></Items><Next>` + next + `</Next></Response></Body></Envelope>`))
		}))
		defer server.Close()
		module := newSOAPTestPolling(t, map[string]any{
			"endpoint":   server.URL,
			"operation":  "List",
			"body":       `<List><Cursor>{{record.pagination.cursor}}</Cursor></List>`,
			"dataField":  "Envelope.Body.Response.Items.Item",
			"pagination": map[string]any{"type": "cursor", "param": "cursor", "nextCursorField": "Envelope.Body.Response.Next"},
		})
		defer func() { _ = module.Close() }()
		records, err := module.Fetch(context.Background())
		if err != nil {
			t.Fatalf("Fetch: %v", err)
		}
		if len(records) != 2 {
			t.Fatalf("records = %#v", records)
		}
	})
}

func TestSOAPPolling_StatePersistenceValuesAreTemplated(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body, _ := io.ReadAll(r.Body)
		for _, want := range []string{"<Since>2026-05-12T10:00:00Z</Since>", "<After>abc</After>"} {
			if !strings.Contains(string(body), want) {
				t.Fatalf("request missing %q:\n%s", want, body)
			}
		}
		w.Header().Set("Content-Type", "text/xml")
		_, _ = w.Write([]byte(`<Envelope><Body><Response><Items><Item><id>1</id></Item></Items></Response></Body></Envelope>`))
	}))
	defer server.Close()

	module := newSOAPTestPolling(t, map[string]any{
		"endpoint":  server.URL,
		"operation": "List",
		"body":      `<List><Since>{{record.since}}</Since><After>{{record.after_id}}</After></List>`,
		"dataField": "Envelope.Body.Response.Items.Item",
		"statePersistence": map[string]any{
			"timestamp": map[string]any{"enabled": true, "queryParam": "since"},
			"id":        map[string]any{"enabled": true, "queryParam": "after_id"},
		},
	})
	defer func() { _ = module.Close() }()
	ts := time.Date(2026, 5, 12, 10, 0, 0, 0, time.UTC)
	id := "abc"
	module.lastState = &persistence.State{LastTimestamp: &ts, LastID: &id}

	if _, err := module.Fetch(context.Background()); err != nil {
		t.Fatalf("Fetch: %v", err)
	}
}

func TestSOAPPolling_RetryInfoAndDisabledRetry(t *testing.T) {
	var calls int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		if atomic.AddInt32(&calls, 1) == 1 {
			w.WriteHeader(http.StatusInternalServerError)
			_, _ = w.Write([]byte(`temporary`))
			return
		}
		w.Header().Set("Content-Type", "text/xml")
		_, _ = w.Write([]byte(`<Envelope><Body><Response><Items><Item><id>1</id></Item></Items></Response></Body></Envelope>`))
	}))
	defer server.Close()

	module := newSOAPTestPolling(t, map[string]any{
		"endpoint":  server.URL,
		"operation": "List",
		"body":      `<List/>`,
		"dataField": "Envelope.Body.Response.Items.Item",
		"retry":     map[string]any{"maxAttempts": 1, "delayMs": 1, "backoffMultiplier": 1, "maxDelayMs": 1, "retryableStatusCodes": []any{500}},
	})
	defer func() { _ = module.Close() }()
	if _, err := module.Fetch(context.Background()); err != nil {
		t.Fatalf("Fetch: %v", err)
	}
	if info := module.GetRetryInfo(); info == nil || info.TotalAttempts != 2 || info.RetryCount != 1 {
		t.Fatalf("retry info = %#v", info)
	}

	var disabledCalls int32
	disabledServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		atomic.AddInt32(&disabledCalls, 1)
		w.WriteHeader(http.StatusInternalServerError)
		_, _ = w.Write([]byte(`temporary`))
	}))
	defer disabledServer.Close()
	disabled := newSOAPTestPolling(t, map[string]any{
		"endpoint":  disabledServer.URL,
		"operation": "List",
		"body":      `<List/>`,
		"dataField": "Envelope.Body.Response.Items.Item",
		"retry":     map[string]any{"maxAttempts": 0, "delayMs": 1, "backoffMultiplier": 1, "maxDelayMs": 1, "retryableStatusCodes": []any{500}},
	})
	defer func() { _ = disabled.Close() }()
	if _, err := disabled.Fetch(context.Background()); err == nil {
		t.Fatal("expected fetch error")
	}
	if got := atomic.LoadInt32(&disabledCalls); got != 1 {
		t.Fatalf("disabled retry calls = %d, want 1", got)
	}
}

func newSOAPTestPolling(t *testing.T, cfg map[string]any) *SOAPPolling {
	t.Helper()
	module, err := NewSOAPPollingFromConfig(&connector.ModuleConfig{Type: "soapPolling", Raw: mustSOAPJSON(cfg)})
	if err != nil {
		t.Fatalf("NewSOAPPollingFromConfig: %v", err)
	}
	return module
}

func mustSOAPJSON(cfg map[string]any) []byte {
	raw, err := json.Marshal(cfg)
	if err != nil {
		panic(err)
	}
	return raw
}

func strconvI32(v int32) string {
	return strconv.FormatInt(int64(v), 10)
}
