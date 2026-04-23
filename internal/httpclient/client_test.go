package httpclient

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"
)

func TestNewClient_AppliesTimeoutAndTransport(t *testing.T) {
	const timeout = 7 * time.Second

	c := NewClient(timeout)
	if c == nil || c.Client == nil {
		t.Fatal("NewClient returned nil client")
	}
	if c.Timeout != timeout {
		t.Errorf("Timeout = %v, want %v", c.Timeout, timeout)
	}

	transport, ok := c.Transport.(*http.Transport)
	if !ok {
		t.Fatalf("Transport is not *http.Transport, got %T", c.Transport)
	}
	if transport.MaxIdleConns != DefaultMaxIdleConns {
		t.Errorf("MaxIdleConns = %d, want %d", transport.MaxIdleConns, DefaultMaxIdleConns)
	}
	if transport.MaxIdleConnsPerHost != DefaultMaxIdleConnsPerHost {
		t.Errorf("MaxIdleConnsPerHost = %d, want %d", transport.MaxIdleConnsPerHost, DefaultMaxIdleConnsPerHost)
	}
	if transport.IdleConnTimeout != DefaultIdleConnTimeout {
		t.Errorf("IdleConnTimeout = %v, want %v", transport.IdleConnTimeout, DefaultIdleConnTimeout)
	}
}

func TestNewClient_ZeroTimeoutMeansNoTimeout(t *testing.T) {
	c := NewClient(0)
	if c.Timeout != 0 {
		t.Errorf("Timeout = %v, want 0", c.Timeout)
	}
}

func TestClient_TimeoutRespected(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		time.Sleep(200 * time.Millisecond)
		w.WriteHeader(http.StatusOK)
	}))
	defer srv.Close()

	c := NewClient(50 * time.Millisecond)
	req, err := http.NewRequestWithContext(context.Background(), http.MethodGet, srv.URL, nil)
	if err != nil {
		t.Fatalf("NewRequestWithContext: %v", err)
	}

	start := time.Now()
	_, err = c.Do(req)
	elapsed := time.Since(start)

	if err == nil {
		t.Fatal("expected timeout error, got nil")
	}
	if elapsed > 150*time.Millisecond {
		t.Errorf("timeout not enforced: elapsed=%v", elapsed)
	}
}

func TestClient_CloseIdleConnections_NilSafe(t *testing.T) {
	// Appel sur receveur nil ne doit pas paniquer.
	var c *Client
	c.CloseIdleConnections()

	// Appel normal ne doit pas paniquer.
	NewClient(time.Second).CloseIdleConnections()
}
