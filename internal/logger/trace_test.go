package logger

import (
	"context"
	"net/http"
	"testing"
)

func TestNewTraceID_Unique(t *testing.T) {
	a := NewTraceID()
	b := NewTraceID()
	if a == "" || b == "" {
		t.Fatalf("NewTraceID returned empty: %q %q", a, b)
	}
	if a == b {
		t.Fatalf("NewTraceID returned identical IDs: %q", a)
	}
}

func TestWithTraceID_Roundtrip(t *testing.T) {
	ctx := WithTraceID(context.Background(), "abc-123")
	if got := TraceIDFrom(ctx); got != "abc-123" {
		t.Fatalf("TraceIDFrom = %q, want abc-123", got)
	}
}

func TestWithTraceID_EmptyIsNoop(t *testing.T) {
	parent := WithTraceID(context.Background(), "abc-123")
	got := WithTraceID(parent, "")
	if id := TraceIDFrom(got); id != "abc-123" {
		t.Fatalf("WithTraceID(\"\") should not overwrite, got %q", id)
	}
}

func TestTraceIDFrom_EmptyContext(t *testing.T) {
	if got := TraceIDFrom(context.Background()); got != "" {
		t.Fatalf("TraceIDFrom(empty ctx) = %q, want empty", got)
	}
}

func TestEnsureTraceID_GeneratesIfMissing(t *testing.T) {
	ctx, id := EnsureTraceID(context.Background())
	if id == "" {
		t.Fatal("EnsureTraceID returned empty id")
	}
	if got := TraceIDFrom(ctx); got != id {
		t.Fatalf("ctx mismatch: TraceIDFrom = %q, want %q", got, id)
	}
}

func TestEnsureTraceID_KeepsExisting(t *testing.T) {
	parent := WithTraceID(context.Background(), "preset-id")
	ctx, id := EnsureTraceID(parent)
	if id != "preset-id" {
		t.Fatalf("EnsureTraceID id = %q, want preset-id", id)
	}
	if got := TraceIDFrom(ctx); got != "preset-id" {
		t.Fatalf("ctx mismatch: %q", got)
	}
}

func TestTraceIDFromHTTPHeader_XRequestID(t *testing.T) {
	h := http.Header{}
	h.Set("X-Request-Id", "req-42")
	if got := TraceIDFromHTTPHeader(h); got != "req-42" {
		t.Fatalf("TraceIDFromHTTPHeader = %q, want req-42", got)
	}
}

func TestTraceIDFromHTTPHeader_Traceparent(t *testing.T) {
	h := http.Header{}
	h.Set("Traceparent", "00-0af7651916cd43dd8448eb211c80319c-b7ad6b7169203331-01")
	if got := TraceIDFromHTTPHeader(h); got != "0af7651916cd43dd8448eb211c80319c" {
		t.Fatalf("TraceIDFromHTTPHeader = %q, want trace-id portion", got)
	}
}

func TestTraceIDFromHTTPHeader_XRequestIDPriority(t *testing.T) {
	h := http.Header{}
	h.Set("X-Request-Id", "req-xyz")
	h.Set("Traceparent", "00-0af7651916cd43dd8448eb211c80319c-b7ad6b7169203331-01")
	if got := TraceIDFromHTTPHeader(h); got != "req-xyz" {
		t.Fatalf("TraceIDFromHTTPHeader = %q, want X-Request-Id to win", got)
	}
}

func TestTraceIDFromHTTPHeader_Empty(t *testing.T) {
	h := http.Header{}
	if got := TraceIDFromHTTPHeader(h); got != "" {
		t.Fatalf("TraceIDFromHTTPHeader = %q, want empty", got)
	}
	if got := TraceIDFromHTTPHeader(nil); got != "" {
		t.Fatalf("TraceIDFromHTTPHeader(nil) = %q, want empty", got)
	}
}

func TestTraceIDFromHTTPHeader_MalformedTraceparent(t *testing.T) {
	h := http.Header{}
	h.Set("Traceparent", "garbage")
	if got := TraceIDFromHTTPHeader(h); got != "" {
		t.Fatalf("TraceIDFromHTTPHeader = %q, want empty for malformed", got)
	}
}
