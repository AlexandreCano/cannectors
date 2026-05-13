package filter

import (
	"context"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"

	"github.com/cannectors/runtime/internal/moduleconfig"
	"github.com/cannectors/runtime/internal/modules/soaputil"
	"github.com/cannectors/runtime/internal/soapclient"
	"github.com/cannectors/runtime/pkg/connector"
)

func TestSOAPCall_ProcessAppendResultKeyAndCache(t *testing.T) {
	var calls int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		atomic.AddInt32(&calls, 1)
		body, _ := io.ReadAll(r.Body)
		if !strings.Contains(string(body), `<ID>42</ID>`) {
			t.Fatalf("request body missing templated ID:\n%s", body)
		}
		w.Header().Set("Content-Type", "text/xml")
		_, _ = w.Write([]byte(`<Envelope><Body><LookupResponse><Customer><name>Alice</name></Customer></LookupResponse></Body></Envelope>`))
	}))
	defer server.Close()

	module, err := NewSOAPCallFromConfig(SOAPCallConfig{
		SOAPRequestBase: moduleconfig.SOAPRequestBase{
			Endpoint:  server.URL,
			Operation: "Lookup",
			Body:      `<Lookup><ID>{{record.customerId}}</ID></Lookup>`,
		},
		DataField:     "Envelope.Body.LookupResponse.Customer",
		MergeStrategy: "append",
		ResultKey:     "soap",
		Cache: moduleconfig.CacheConfig{
			Enabled:    true,
			MaxSize:    10,
			TTLSeconds: 60,
			Key:        "{{record.customerId}}",
		},
	})
	if err != nil {
		t.Fatalf("NewSOAPCallFromConfig: %v", err)
	}

	out, err := module.Process(context.Background(), []map[string]any{
		{"customerId": "42"},
		{"customerId": "42"},
	})
	if err != nil {
		t.Fatalf("Process: %v", err)
	}
	if got := atomic.LoadInt32(&calls); got != 1 {
		t.Fatalf("expected one SOAP call due to cache, got %d", got)
	}
	if len(out) != 2 {
		t.Fatalf("expected 2 records, got %d", len(out))
	}
	soapData, ok := out[0]["soap"].(map[string]any)
	if !ok || soapData["name"] != "Alice" {
		t.Fatalf("unexpected appended response: %#v", out[0])
	}
}

func TestSOAPCall_AppendRequiresResultKey(t *testing.T) {
	_, err := NewSOAPCallFromConfig(SOAPCallConfig{
		SOAPRequestBase: moduleconfig.SOAPRequestBase{
			Endpoint:  "https://example.com/service",
			Operation: "Lookup",
			Body:      `<Lookup/>`,
		},
		MergeStrategy: "append",
	})
	if err == nil {
		t.Fatal("expected error")
	}
}

func TestSOAPCall_MergeStrategies(t *testing.T) {
	module := &SOAPCallModule{mergeStrategy: "merge"}
	merged := module.mergeData(
		map[string]any{"id": "1", "profile": map[string]any{"name": "old", "keep": "yes"}},
		map[string]any{"profile": map[string]any{"name": "new"}},
	)
	profile, ok := merged["profile"].(map[string]any)
	if !ok {
		t.Fatalf("profile = %T, want map[string]any", merged["profile"])
	}
	if profile["name"] != "new" || profile["keep"] != "yes" {
		t.Fatalf("unexpected merge result: %#v", merged)
	}

	module.mergeStrategy = "replace"
	replaced := module.mergeData(map[string]any{"id": "1", "name": "old"}, map[string]any{"name": "new"})
	if replaced["id"] != "1" || replaced["name"] != "new" {
		t.Fatalf("unexpected replace result: %#v", replaced)
	}
}

func TestSOAPCall_OnErrorSkipAndLog(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
		_, _ = w.Write([]byte(`broken`))
	}))
	defer server.Close()

	for _, tt := range []struct {
		name    string
		onError string
		wantLen int
	}{
		{name: "skip", onError: "skip", wantLen: 0},
		{name: "log", onError: "log", wantLen: 1},
	} {
		t.Run(tt.name, func(t *testing.T) {
			module, err := NewSOAPCallFromConfig(SOAPCallConfig{
				ModuleBase: connector.ModuleBase{OnError: tt.onError},
				SOAPRequestBase: moduleconfig.SOAPRequestBase{
					Endpoint:  server.URL,
					Operation: "Lookup",
					Body:      `<Lookup/>`,
				},
			})
			if err != nil {
				t.Fatalf("NewSOAPCallFromConfig: %v", err)
			}
			out, err := module.Process(context.Background(), []map[string]any{{"id": "1"}})
			if err != nil {
				t.Fatalf("Process: %v", err)
			}
			if len(out) != tt.wantLen {
				t.Fatalf("len(out) = %d, want %d: %#v", len(out), tt.wantLen, out)
			}
		})
	}
}

func TestSOAPCall_DataFieldMissingReturnsError(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "text/xml")
		_, _ = w.Write([]byte(`<Envelope><Body><LookupResponse/></Body></Envelope>`))
	}))
	defer server.Close()

	module, err := NewSOAPCallFromConfig(SOAPCallConfig{
		SOAPRequestBase: moduleconfig.SOAPRequestBase{
			Endpoint:  server.URL,
			Operation: "Lookup",
			Body:      `<Lookup/>`,
		},
		DataField: "Envelope.Body.LookupResponse.Customer",
	})
	if err != nil {
		t.Fatalf("NewSOAPCallFromConfig: %v", err)
	}
	_, err = module.Process(context.Background(), []map[string]any{{"id": "1"}})
	if !errors.Is(err, soaputil.ErrInvalidDataField) {
		t.Fatalf("expected ErrInvalidDataField, got %T: %v", err, err)
	}
}

func TestSOAPCall_FaultPropagatesTypedError(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "text/xml")
		w.WriteHeader(http.StatusInternalServerError)
		_, _ = w.Write([]byte(`<soap:Envelope xmlns:soap="http://schemas.xmlsoap.org/soap/envelope/"><soap:Body><soap:Fault><faultcode>soap:Server</faultcode><faultstring>Transient</faultstring></soap:Fault></soap:Body></soap:Envelope>`))
	}))
	defer server.Close()

	module, err := NewSOAPCallFromConfig(SOAPCallConfig{
		SOAPRequestBase: moduleconfig.SOAPRequestBase{
			Endpoint:  server.URL,
			Operation: "Lookup",
			Body:      `<Lookup/>`,
		},
	})
	if err != nil {
		t.Fatalf("NewSOAPCallFromConfig: %v", err)
	}
	_, err = module.Process(context.Background(), []map[string]any{{"id": "1"}})
	var fault *soapclient.SOAPFaultError
	if !errors.As(err, &fault) {
		t.Fatalf("expected SOAPFaultError, got %T: %v", err, err)
	}
}
