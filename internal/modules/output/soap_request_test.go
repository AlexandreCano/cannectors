package output

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"

	"github.com/cannectors/runtime/internal/soapclient"
	"github.com/cannectors/runtime/pkg/connector"
)

func TestSOAPRequest_SendSingle(t *testing.T) {
	var calls int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		atomic.AddInt32(&calls, 1)
		body, _ := io.ReadAll(r.Body)
		if !strings.Contains(string(body), `<Name>Alice</Name>`) && !strings.Contains(string(body), `<Name>Bob</Name>`) {
			t.Fatalf("request body missing templated name:\n%s", body)
		}
		w.Header().Set("Content-Type", "text/xml")
		_, _ = w.Write([]byte(`<Envelope><Body><SubmitResponse><ok>true</ok></SubmitResponse></Body></Envelope>`))
	}))
	defer server.Close()

	raw, err := json.Marshal(map[string]any{
		"endpoint":    server.URL,
		"operation":   "Submit",
		"body":        `<Submit><Name>{{record.name}}</Name></Submit>`,
		"requestMode": "single",
	})
	if err != nil {
		t.Fatal(err)
	}
	module, err := NewSOAPRequestFromConfig(&connector.ModuleConfig{Type: "soapRequest", Raw: raw})
	if err != nil {
		t.Fatalf("NewSOAPRequestFromConfig: %v", err)
	}
	defer func() { _ = module.Close() }()

	sent, err := module.Send(context.Background(), []map[string]any{
		{"name": "Alice"},
		{"name": "Bob"},
	})
	if err != nil {
		t.Fatalf("Send: %v", err)
	}
	if sent != 2 {
		t.Fatalf("sent = %d, want 2", sent)
	}
	if got := atomic.LoadInt32(&calls); got != 2 {
		t.Fatalf("calls = %d, want 2", got)
	}
}

func TestSOAPRequest_PreviewBatch(t *testing.T) {
	raw, err := json.Marshal(map[string]any{
		"endpoint":    "https://example.com/soap",
		"operation":   "SubmitBatch",
		"body":        `<SubmitBatch><First>{{record.records[0].name}}</First></SubmitBatch>`,
		"requestMode": "batch",
	})
	if err != nil {
		t.Fatal(err)
	}
	module, err := NewSOAPRequestFromConfig(&connector.ModuleConfig{Type: "soapRequest", Raw: raw})
	if err != nil {
		t.Fatalf("NewSOAPRequestFromConfig: %v", err)
	}
	defer func() { _ = module.Close() }()

	previews, err := module.PreviewRequest([]map[string]any{{"name": "Alice"}}, PreviewOptions{})
	if err != nil {
		t.Fatalf("PreviewRequest: %v", err)
	}
	if len(previews) != 1 {
		t.Fatalf("expected one preview, got %d", len(previews))
	}
	if previews[0].Method != "POST" || previews[0].RecordCount != 1 {
		t.Fatalf("unexpected preview metadata: %#v", previews[0])
	}
	if got := previews[0].Headers["Content-Type"]; got != `text/xml; charset=utf-8` {
		t.Fatalf("Content-Type = %q", got)
	}
	if !strings.Contains(previews[0].BodyPreview, `<First>Alice</First>`) {
		t.Fatalf("unexpected body preview:\n%s", previews[0].BodyPreview)
	}
}

func TestSOAPRequest_SendBatch(t *testing.T) {
	var calls int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		atomic.AddInt32(&calls, 1)
		body, _ := io.ReadAll(r.Body)
		if !strings.Contains(string(body), `<First>Alice</First>`) {
			t.Fatalf("request body missing batch data:\n%s", body)
		}
		w.Header().Set("Content-Type", "text/xml")
		_, _ = w.Write([]byte(`<Envelope><Body><SubmitResponse><ok>true</ok></SubmitResponse></Body></Envelope>`))
	}))
	defer server.Close()

	module := newSOAPRequestTestModule(t, map[string]any{
		"endpoint":    server.URL,
		"operation":   "SubmitBatch",
		"body":        `<SubmitBatch><First>{{record.records[0].name}}</First></SubmitBatch>`,
		"requestMode": "batch",
	})
	defer func() { _ = module.Close() }()

	sent, err := module.Send(context.Background(), []map[string]any{{"name": "Alice"}, {"name": "Bob"}})
	if err != nil {
		t.Fatalf("Send: %v", err)
	}
	if sent != 2 || atomic.LoadInt32(&calls) != 1 {
		t.Fatalf("sent=%d calls=%d", sent, atomic.LoadInt32(&calls))
	}
}

func TestSOAPRequest_OnErrorSkipAndLog(t *testing.T) {
	for _, tt := range []struct {
		name    string
		onError string
	}{
		{name: "skip", onError: "skip"},
		{name: "log", onError: "log"},
	} {
		t.Run(tt.name, func(t *testing.T) {
			var calls int32
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
				if atomic.AddInt32(&calls, 1) == 1 {
					w.WriteHeader(http.StatusInternalServerError)
					_, _ = w.Write([]byte(`broken`))
					return
				}
				w.Header().Set("Content-Type", "text/xml")
				_, _ = w.Write([]byte(`<Envelope><Body><SubmitResponse><ok>true</ok></SubmitResponse></Body></Envelope>`))
			}))
			defer server.Close()

			module := newSOAPRequestTestModule(t, map[string]any{
				"endpoint":    server.URL,
				"operation":   "Submit",
				"body":        `<Submit/>`,
				"requestMode": "single",
				"onError":     tt.onError,
				"retry":       map[string]any{"maxAttempts": 0, "delayMs": 1, "backoffMultiplier": 1, "maxDelayMs": 1},
			})
			defer func() { _ = module.Close() }()
			sent, err := module.Send(context.Background(), []map[string]any{{"id": "1"}, {"id": "2"}})
			if err != nil {
				t.Fatalf("Send: %v", err)
			}
			if sent != 1 {
				t.Fatalf("sent = %d, want 1", sent)
			}
		})
	}
}

func TestSOAPRequest_MTOMBase64Attachment(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, attachments, multipartRequest, err := soapclient.ParseMTOMResponse(r.Header.Get("Content-Type"), r.Body)
		if err != nil {
			t.Fatalf("ParseMTOMResponse: %v", err)
		}
		if !multipartRequest {
			t.Fatal("expected multipart request")
		}
		if got := string(attachments["doc-1"].Data); got != "pdf-bytes" {
			t.Fatalf("attachment data = %q", got)
		}
		w.Header().Set("Content-Type", "text/xml")
		_, _ = w.Write([]byte(`<Envelope><Body><UploadResponse><ok>true</ok></UploadResponse></Body></Envelope>`))
	}))
	defer server.Close()

	module := newSOAPRequestTestModule(t, map[string]any{
		"endpoint":    server.URL,
		"operation":   "Upload",
		"requestMode": "single",
		"body":        `<Upload><File><xop:Include xmlns:xop="http://www.w3.org/2004/08/xop/include" href="cid:doc-1"/></File></Upload>`,
		"mtom": map[string]any{
			"enabled": true,
			"attachments": []any{map[string]any{
				"contentId":   "doc-1",
				"contentType": "application/pdf",
				"sourceField": "document",
				"encoding":    "base64",
			}},
		},
	})
	defer func() { _ = module.Close() }()
	sent, err := module.Send(context.Background(), []map[string]any{{"document": base64.StdEncoding.EncodeToString([]byte("pdf-bytes"))}})
	if err != nil {
		t.Fatalf("Send: %v", err)
	}
	if sent != 1 {
		t.Fatalf("sent = %d", sent)
	}
}

func TestSOAPRequest_WSSecurityAndFault(t *testing.T) {
	t.Run("ws-security header", func(t *testing.T) {
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			body, _ := io.ReadAll(r.Body)
			for _, want := range []string{"Security", "alice", "#PasswordText"} {
				if !strings.Contains(string(body), want) {
					t.Fatalf("request missing %q:\n%s", want, body)
				}
			}
			w.Header().Set("Content-Type", "text/xml")
			_, _ = w.Write([]byte(`<Envelope><Body><SubmitResponse><ok>true</ok></SubmitResponse></Body></Envelope>`))
		}))
		defer server.Close()
		module := newSOAPRequestTestModule(t, map[string]any{
			"endpoint":   server.URL,
			"operation":  "Submit",
			"body":       `<Submit/>`,
			"wsSecurity": map[string]any{"username": "alice", "password": "secret", "passwordType": "PasswordText"},
		})
		defer func() { _ = module.Close() }()
		if _, err := module.Send(context.Background(), []map[string]any{{"id": "1"}}); err != nil {
			t.Fatalf("Send: %v", err)
		}
	})

	t.Run("fault", func(t *testing.T) {
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
			w.Header().Set("Content-Type", "text/xml")
			w.WriteHeader(http.StatusInternalServerError)
			_, _ = w.Write([]byte(`<soap:Envelope xmlns:soap="http://schemas.xmlsoap.org/soap/envelope/"><soap:Body><soap:Fault><faultcode>soap:Server</faultcode><faultstring>Transient</faultstring></soap:Fault></soap:Body></soap:Envelope>`))
		}))
		defer server.Close()
		module := newSOAPRequestTestModule(t, map[string]any{
			"endpoint":  server.URL,
			"operation": "Submit",
			"body":      `<Submit/>`,
		})
		defer func() { _ = module.Close() }()
		_, err := module.Send(context.Background(), []map[string]any{{"id": "1"}})
		var fault *soapclient.SOAPFaultError
		if !errors.As(err, &fault) {
			t.Fatalf("expected SOAPFaultError, got %T: %v", err, err)
		}
	})
}

func TestSOAPRequest_RetryInfoAndPreviewSOAP12Headers(t *testing.T) {
	var calls int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		if atomic.AddInt32(&calls, 1) == 1 {
			w.WriteHeader(http.StatusInternalServerError)
			_, _ = w.Write([]byte(`temporary`))
			return
		}
		w.Header().Set("Content-Type", "text/xml")
		_, _ = w.Write([]byte(`<Envelope><Body><SubmitResponse><ok>true</ok></SubmitResponse></Body></Envelope>`))
	}))
	defer server.Close()

	module := newSOAPRequestTestModule(t, map[string]any{
		"endpoint":    server.URL,
		"operation":   "Submit",
		"soapVersion": "1.2",
		"soapAction":  "urn:Submit",
		"body":        `<Submit/>`,
		"retry":       map[string]any{"maxAttempts": 1, "delayMs": 1, "backoffMultiplier": 1, "maxDelayMs": 1, "retryableStatusCodes": []any{500}},
	})
	defer func() { _ = module.Close() }()
	if _, err := module.Send(context.Background(), []map[string]any{{"id": "1"}}); err != nil {
		t.Fatalf("Send: %v", err)
	}
	if info := module.GetRetryInfo(); info == nil || info.TotalAttempts != 2 || info.RetryCount != 1 {
		t.Fatalf("retry info = %#v", info)
	}
	previews, err := module.PreviewRequest([]map[string]any{{"id": "1"}}, PreviewOptions{})
	if err != nil {
		t.Fatalf("PreviewRequest: %v", err)
	}
	contentType := previews[0].Headers["Content-Type"]
	if !strings.HasPrefix(contentType, "application/soap+xml") || !strings.Contains(contentType, `action="urn:Submit"`) {
		t.Fatalf("SOAP 1.2 Content-Type = %q", contentType)
	}
	if _, ok := previews[0].Headers["SOAPAction"]; ok {
		t.Fatalf("SOAPAction should be absent for SOAP 1.2 preview: %#v", previews[0].Headers)
	}
}

func newSOAPRequestTestModule(t *testing.T, cfg map[string]any) *SOAPRequestModule {
	t.Helper()
	raw, err := json.Marshal(cfg)
	if err != nil {
		t.Fatal(err)
	}
	module, err := NewSOAPRequestFromConfig(&connector.ModuleConfig{Type: "soapRequest", Raw: raw})
	if err != nil {
		t.Fatalf("NewSOAPRequestFromConfig: %v", err)
	}
	return module
}
