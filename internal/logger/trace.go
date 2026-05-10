package logger

import (
	"context"
	"net/http"
	"strings"

	"github.com/google/uuid"
)

// traceIDKey is the unexported context key used to carry the trace ID.
type traceIDKey struct{}

// TraceIDField is the structured log field name used for the trace ID.
const TraceIDField = "trace_id"

// NewTraceID generates a new UUID v4 trace identifier.
func NewTraceID() string {
	return uuid.NewString()
}

// WithTraceID returns a new context carrying the given trace ID.
// If id is empty, the original context is returned unchanged.
func WithTraceID(ctx context.Context, id string) context.Context {
	if ctx == nil {
		ctx = context.Background()
	}
	if id == "" {
		return ctx
	}
	return context.WithValue(ctx, traceIDKey{}, id)
}

// TraceIDFrom returns the trace ID stored in ctx, or "" if none is present.
func TraceIDFrom(ctx context.Context) string {
	if ctx == nil {
		return ""
	}
	id, _ := ctx.Value(traceIDKey{}).(string)
	return id
}

// EnsureTraceID returns a context that has a trace ID set: either the one
// already in ctx, or a freshly generated UUID v4. The resulting trace ID is
// returned alongside the context.
func EnsureTraceID(ctx context.Context) (context.Context, string) {
	if id := TraceIDFrom(ctx); id != "" {
		return ctx, id
	}
	id := NewTraceID()
	return WithTraceID(ctx, id), id
}

// TraceIDFromHTTPHeader extracts a trace ID from incoming HTTP headers.
// Priority order:
//  1. X-Request-Id (custom convention)
//  2. W3C `traceparent` (the trace-id portion: second hex segment)
//
// Returns "" if no usable header is present.
func TraceIDFromHTTPHeader(h http.Header) string {
	if h == nil {
		return ""
	}
	if v := strings.TrimSpace(h.Get("X-Request-Id")); v != "" {
		return v
	}
	// W3C trace context: version "-" trace-id "-" parent-id "-" trace-flags
	// e.g. 00-0af7651916cd43dd8448eb211c80319c-b7ad6b7169203331-01
	if tp := strings.TrimSpace(h.Get("Traceparent")); tp != "" {
		parts := strings.Split(tp, "-")
		if len(parts) >= 2 && parts[1] != "" {
			return parts[1]
		}
	}
	return ""
}
