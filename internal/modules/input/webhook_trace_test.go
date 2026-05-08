package input

import (
	"bytes"
	"context"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/cannectors/runtime/internal/logger"
)

// dispatchAndCaptureTrace runs a single synthetic POST through createHandler and
// returns the trace ID that the handler observed.
func dispatchAndCaptureTrace(t *testing.T, headers http.Header) string {
	t.Helper()
	w := &Webhook{endpoint: "/webhook/test"}

	var captured string
	handler := func(ctx context.Context, _ []map[string]any) error {
		captured = logger.TraceIDFrom(ctx)
		return nil
	}

	req := httptest.NewRequest(http.MethodPost, "/webhook/test", bytes.NewBufferString(`[{"id":1}]`))
	for k, vs := range headers {
		for _, v := range vs {
			req.Header.Add(k, v)
		}
	}
	rr := httptest.NewRecorder()

	w.createHandler(handler).ServeHTTP(rr, req)

	if rr.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200; body=%q", rr.Code, rr.Body.String())
	}
	return captured
}

func TestWebhook_TraceID_GeneratedWhenAbsent(t *testing.T) {
	got := dispatchAndCaptureTrace(t, http.Header{})
	if got == "" {
		t.Fatal("expected handler context to carry a generated trace ID, got empty")
	}
}

func TestWebhook_TraceID_ReusesXRequestID(t *testing.T) {
	h := http.Header{}
	h.Set("X-Request-Id", "client-supplied-42")
	got := dispatchAndCaptureTrace(t, h)
	if got != "client-supplied-42" {
		t.Fatalf("trace ID = %q, want client-supplied-42", got)
	}
}

func TestWebhook_TraceID_ReusesTraceparent(t *testing.T) {
	h := http.Header{}
	h.Set("Traceparent", "00-0af7651916cd43dd8448eb211c80319c-b7ad6b7169203331-01")
	got := dispatchAndCaptureTrace(t, h)
	if got != "0af7651916cd43dd8448eb211c80319c" {
		t.Fatalf("trace ID = %q, want trace-id portion of Traceparent", got)
	}
}
