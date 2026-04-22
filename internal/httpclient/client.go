// Package httpclient provides a shared HTTP client for runtime modules.
//
// It factors out the *http.Client construction (timeout + optimized transport),
// the unified HTTP error type, the RFC 7230 header validation helpers, and the
// shared retry loop.
//
// The package depends only on errhandling, logger, and pkg/connector; it has
// no dependency on any module (input / filter / output).
package httpclient

import (
	"net/http"
	"time"
)

// Default transport settings shared by all HTTP modules.
const (
	// DefaultMaxIdleConns is the maximum global number of idle connections
	// kept by the transport.
	DefaultMaxIdleConns = 100

	// DefaultMaxIdleConnsPerHost is the maximum number of idle connections
	// kept per host.
	DefaultMaxIdleConnsPerHost = 10

	// DefaultIdleConnTimeout is the maximum duration an idle connection is
	// kept before being closed.
	DefaultIdleConnTimeout = 90 * time.Second
)

// Client is a thin wrapper around *http.Client with a pre-configured
// transport (connection pooling).
//
// Client is safe for concurrent use.
type Client struct {
	*http.Client
}

// NewClient builds a Client with the given timeout and a transport using the
// default settings (MaxIdleConns=100, MaxIdleConnsPerHost=10,
// IdleConnTimeout=90s).
//
// A zero timeout disables the global request timeout.
func NewClient(timeout time.Duration) *Client {
	transport := &http.Transport{
		MaxIdleConns:        DefaultMaxIdleConns,
		MaxIdleConnsPerHost: DefaultMaxIdleConnsPerHost,
		IdleConnTimeout:     DefaultIdleConnTimeout,
	}
	return &Client{
		Client: &http.Client{
			Timeout:   timeout,
			Transport: transport,
		},
	}
}

// CloseIdleConnections closes any idle connections held by the transport.
// Useful on module shutdown to release resources. Safe to call on a nil
// receiver.
func (c *Client) CloseIdleConnections() {
	if c == nil || c.Client == nil {
		return
	}
	c.Client.CloseIdleConnections()
}
