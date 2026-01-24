// Package input provides implementations for input modules.
package input

import (
	"bytes"
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/canectors/runtime/pkg/connector"
)

// waitForServer waits for the webhook server to be ready (address available)
// Returns true if server is ready, false if timeout
func waitForServer(w *Webhook, timeout time.Duration) bool { //nolint:unparam
	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		if w.Address() != "" {
			return true
		}
		time.Sleep(10 * time.Millisecond)
	}
	return false
}

// =============================================================================
// Task 1: HTTP webhook server tests
// =============================================================================

func TestNewWebhookFromConfig_ValidConfig(t *testing.T) {
	config := &connector.ModuleConfig{
		Type: "webhook",
		Config: map[string]interface{}{
			"endpoint":      "/webhook/orders",
			"listenAddress": "127.0.0.1:0",
		},
	}

	w, err := NewWebhookFromConfig(config)
	if err != nil {
		t.Fatalf("NewWebhookFromConfig() error = %v, want nil", err)
	}

	if w == nil {
		t.Fatal("NewWebhookFromConfig() returned nil, want Webhook")
	}

	if w.endpoint != "/webhook/orders" {
		t.Errorf("Webhook.endpoint = %q, want %q", w.endpoint, "/webhook/orders")
	}
}

func TestNewWebhookFromConfig_NilConfig(t *testing.T) {
	_, err := NewWebhookFromConfig(nil)
	if err == nil {
		t.Fatal("NewWebhookFromConfig(nil) error = nil, want error")
	}
	if err != ErrNilConfig {
		t.Errorf("NewWebhookFromConfig(nil) error = %v, want %v", err, ErrNilConfig)
	}
}

func TestNewWebhookFromConfig_MissingEndpoint(t *testing.T) {
	config := &connector.ModuleConfig{
		Type: "webhook",
		Config: map[string]interface{}{
			"listenAddress": "127.0.0.1:0",
		},
	}

	_, err := NewWebhookFromConfig(config)
	if err == nil {
		t.Fatal("NewWebhookFromConfig() with missing endpoint error = nil, want error")
	}
	if err != ErrMissingEndpoint {
		t.Errorf("NewWebhookFromConfig() error = %v, want %v", err, ErrMissingEndpoint)
	}
}

func TestNewWebhookFromConfig_DefaultListenAddress(t *testing.T) {
	config := &connector.ModuleConfig{
		Type: "webhook",
		Config: map[string]interface{}{
			"endpoint": "/webhook/test",
		},
	}

	w, err := NewWebhookFromConfig(config)
	if err != nil {
		t.Fatalf("NewWebhookFromConfig() error = %v, want nil", err)
	}

	if w.listenAddress != defaultListenAddress {
		t.Errorf("Webhook.listenAddress = %q, want %q", w.listenAddress, defaultListenAddress)
	}
}

func TestNewWebhookFromConfig_SignatureMissingType(t *testing.T) {
	config := &connector.ModuleConfig{
		Type: "webhook",
		Config: map[string]interface{}{
			"endpoint": "/webhook/test",
			"signature": map[string]interface{}{
				"secret": "test-secret",
			},
		},
	}

	_, err := NewWebhookFromConfig(config)
	if err != ErrMissingSignatureType {
		t.Errorf("NewWebhookFromConfig() error = %v, want %v", err, ErrMissingSignatureType)
	}
}

func TestNewWebhookFromConfig_SignatureUnsupportedType(t *testing.T) {
	config := &connector.ModuleConfig{
		Type: "webhook",
		Config: map[string]interface{}{
			"endpoint": "/webhook/test",
			"signature": map[string]interface{}{
				"type":   "rsa-sha256",
				"secret": "test-secret",
			},
		},
	}

	_, err := NewWebhookFromConfig(config)
	if err != ErrUnsupportedSignature {
		t.Errorf("NewWebhookFromConfig() error = %v, want %v", err, ErrUnsupportedSignature)
	}
}

func TestWebhook_Start_StartsServer(t *testing.T) {
	config := &connector.ModuleConfig{
		Type: "webhook",
		Config: map[string]interface{}{
			"endpoint":      "/webhook/test",
			"listenAddress": "127.0.0.1:0", // Port 0 for dynamic allocation
		},
	}

	w, err := NewWebhookFromConfig(config)
	if err != nil {
		t.Fatalf("NewWebhookFromConfig() error = %v", err)
	}

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	// Start the webhook server
	errChan := make(chan error, 1)
	go func() {
		errChan <- w.Start(ctx, nil)
	}()

	// Wait for server to start
	if !waitForServer(w, 2*time.Second) {
		t.Fatal("Webhook.Address() returned empty string, server not started")
	}

	// Stop the server
	cancel()

	// Wait for shutdown
	select {
	case err := <-errChan:
		if err != nil && err != context.Canceled && err != http.ErrServerClosed {
			t.Errorf("Webhook.Start() returned error = %v", err)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("Server did not stop within timeout")
	}
}

func TestWebhook_Start_UsesConfiguredTimeout(t *testing.T) {
	config := &connector.ModuleConfig{
		Type: "webhook",
		Config: map[string]interface{}{
			"endpoint":      "/webhook/test",
			"listenAddress": "127.0.0.1:0",
			"timeout":       2.0,
		},
	}

	w, err := NewWebhookFromConfig(config)
	if err != nil {
		t.Fatalf("NewWebhookFromConfig() error = %v", err)
	}

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	errChan := make(chan error, 1)
	go func() {
		errChan <- w.Start(ctx, nil)
	}()

	if !waitForServer(w, 2*time.Second) {
		t.Fatal("Server did not start within timeout")
	}

	if w.server == nil {
		t.Fatal("Webhook.server is nil after start")
	}
	if w.server.ReadTimeout != 2*time.Second || w.server.WriteTimeout != 2*time.Second {
		t.Errorf("Timeouts = %v/%v, want 2s/2s", w.server.ReadTimeout, w.server.WriteTimeout)
	}

	cancel()
	<-errChan
}

func TestWebhook_GracefulShutdown(t *testing.T) {
	config := &connector.ModuleConfig{
		Type: "webhook",
		Config: map[string]interface{}{
			"endpoint":      "/webhook/test",
			"listenAddress": "127.0.0.1:0",
		},
	}

	w, err := NewWebhookFromConfig(config)
	if err != nil {
		t.Fatalf("NewWebhookFromConfig() error = %v", err)
	}

	ctx, cancel := context.WithCancel(context.Background())

	// Start the webhook server
	errChan := make(chan error, 1)
	go func() {
		errChan <- w.Start(ctx, nil)
	}()

	// Wait for server to start
	if !waitForServer(w, 2*time.Second) {
		t.Fatal("Server did not start within timeout")
	}

	// Request graceful shutdown
	cancel()
	shutdownStart := time.Now()

	// Wait for shutdown
	select {
	case err := <-errChan:
		if err != nil && err != context.Canceled && err != http.ErrServerClosed {
			t.Errorf("Webhook.Start() returned error = %v", err)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("Server did not shut down within 5 seconds")
	}

	shutdownDuration := time.Since(shutdownStart)
	if shutdownDuration > 3*time.Second {
		t.Errorf("Shutdown took too long: %v (expected < 3s)", shutdownDuration)
	}
}

func TestWebhook_Stop_StopsServer(t *testing.T) {
	config := &connector.ModuleConfig{
		Type: "webhook",
		Config: map[string]interface{}{
			"endpoint":      "/webhook/test",
			"listenAddress": "127.0.0.1:0",
		},
	}

	w, err := NewWebhookFromConfig(config)
	if err != nil {
		t.Fatalf("NewWebhookFromConfig() error = %v", err)
	}

	ctx := context.Background()

	// Start the webhook server
	errChan := make(chan error, 1)
	go func() {
		errChan <- w.Start(ctx, nil)
	}()

	// Wait for server to start
	if !waitForServer(w, 2*time.Second) {
		t.Fatal("Server did not start within timeout")
	}

	// Stop using Stop() method
	err = w.Stop()
	if err != nil {
		t.Errorf("Webhook.Stop() error = %v, want nil", err)
	}

	// Wait for server to actually stop
	select {
	case err := <-errChan:
		if err != nil && err != http.ErrServerClosed {
			t.Errorf("Webhook.Start() returned unexpected error = %v", err)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("Server did not stop within timeout")
	}
}

func TestWebhook_HTTPPostHandler_RegisteredCorrectly(t *testing.T) {
	config := &connector.ModuleConfig{
		Type: "webhook",
		Config: map[string]interface{}{
			"endpoint":      "/webhook/orders",
			"listenAddress": "127.0.0.1:0",
		},
	}

	w, err := NewWebhookFromConfig(config)
	if err != nil {
		t.Fatalf("NewWebhookFromConfig() error = %v", err)
	}

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	// Track received data
	var receivedData []map[string]interface{}
	var mu sync.Mutex
	handler := func(data []map[string]interface{}) error { //nolint:unparam
		mu.Lock()
		receivedData = data
		mu.Unlock()
		return nil
	}

	// Start the webhook server with handler
	errChan := make(chan error, 1)
	go func() {
		errChan <- w.Start(ctx, handler)
	}()

	// Wait for server to start
	if !waitForServer(w, 2*time.Second) {
		t.Fatal("Server did not start within timeout")
	}
	addr := w.Address()

	// Send a POST request
	payload := `[{"id": 1, "name": "Test Order"}]`
	resp, err := http.Post("http://"+addr+"/webhook/orders", "application/json", bytes.NewBufferString(payload))
	if err != nil {
		t.Fatalf("POST request failed: %v", err)
	}
	defer func() {
		if closeErr := resp.Body.Close(); closeErr != nil {
			t.Logf("failed to close response body: %v", closeErr)
		}
	}()

	if resp.StatusCode != http.StatusOK {
		t.Errorf("POST response status = %d, want %d", resp.StatusCode, http.StatusOK)
	}

	// Verify handler received data
	time.Sleep(50 * time.Millisecond)
	mu.Lock()
	if len(receivedData) != 1 {
		t.Errorf("Handler received %d records, want 1", len(receivedData))
	}
	mu.Unlock()

	cancel()
}

// =============================================================================
// Task 2: Webhook payload reception and parsing tests
// =============================================================================

func TestWebhook_ParsePayload_JSONArray(t *testing.T) {
	config := &connector.ModuleConfig{
		Type: "webhook",
		Config: map[string]interface{}{
			"endpoint":      "/webhook/test",
			"listenAddress": "127.0.0.1:0",
		},
	}

	w, err := NewWebhookFromConfig(config)
	if err != nil {
		t.Fatalf("NewWebhookFromConfig() error = %v", err)
	}

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	var receivedData []map[string]interface{}
	var mu sync.Mutex
	handler := func(data []map[string]interface{}) error { //nolint:unparam
		mu.Lock()
		receivedData = data
		mu.Unlock()
		return nil
	}

	go func() {
		_ = w.Start(ctx, handler)
	}()

	if !waitForServer(w, 2*time.Second) {
		t.Fatal("Server did not start within timeout")
	}
	addr := w.Address()
	payload := `[{"id": 1}, {"id": 2}, {"id": 3}]`
	resp, err := http.Post("http://"+addr+"/webhook/test", "application/json", bytes.NewBufferString(payload))
	if err != nil {
		t.Fatalf("POST request failed: %v", err)
	}
	defer func() {
		if closeErr := resp.Body.Close(); closeErr != nil {
			t.Logf("failed to close response body: %v", closeErr)
		}
	}()

	if resp.StatusCode != http.StatusOK {
		t.Errorf("Response status = %d, want %d", resp.StatusCode, http.StatusOK)
	}

	time.Sleep(50 * time.Millisecond)
	mu.Lock()
	if len(receivedData) != 3 {
		t.Errorf("Received %d records, want 3", len(receivedData))
	}
	mu.Unlock()
}

func TestWebhook_ParsePayload_SingleObject(t *testing.T) {
	config := &connector.ModuleConfig{
		Type: "webhook",
		Config: map[string]interface{}{
			"endpoint":      "/webhook/test",
			"listenAddress": "127.0.0.1:0",
		},
	}

	w, err := NewWebhookFromConfig(config)
	if err != nil {
		t.Fatalf("NewWebhookFromConfig() error = %v", err)
	}

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	var receivedData []map[string]interface{}
	var mu sync.Mutex
	handler := func(data []map[string]interface{}) error { //nolint:unparam
		mu.Lock()
		receivedData = data
		mu.Unlock()
		return nil
	}

	go func() {
		_ = w.Start(ctx, handler)
	}()

	if !waitForServer(w, 2*time.Second) {
		t.Fatal("Server did not start within timeout")
	}
	addr := w.Address()
	payload := `{"id": 123, "name": "Single Item"}`
	resp, err := http.Post("http://"+addr+"/webhook/test", "application/json", bytes.NewBufferString(payload))
	if err != nil {
		t.Fatalf("POST request failed: %v", err)
	}
	defer func() {
		if closeErr := resp.Body.Close(); closeErr != nil {
			t.Logf("failed to close response body: %v", closeErr)
		}
	}()

	if resp.StatusCode != http.StatusOK {
		t.Errorf("Response status = %d, want %d", resp.StatusCode, http.StatusOK)
	}

	time.Sleep(50 * time.Millisecond)
	mu.Lock()
	if len(receivedData) != 1 {
		t.Errorf("Received %d records, want 1 (single object wrapped in array)", len(receivedData))
	}
	if len(receivedData) == 1 {
		if receivedData[0]["id"] != float64(123) {
			t.Errorf("Received record id = %v, want 123", receivedData[0]["id"])
		}
	}
	mu.Unlock()
}

func TestWebhook_ParsePayload_WithDataField(t *testing.T) {
	config := &connector.ModuleConfig{
		Type: "webhook",
		Config: map[string]interface{}{
			"endpoint":      "/webhook/test",
			"listenAddress": "127.0.0.1:0",
			"dataField":     "items",
		},
	}

	w, err := NewWebhookFromConfig(config)
	if err != nil {
		t.Fatalf("NewWebhookFromConfig() error = %v", err)
	}

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	var receivedData []map[string]interface{}
	var mu sync.Mutex
	handler := func(data []map[string]interface{}) error { //nolint:unparam
		mu.Lock()
		receivedData = data
		mu.Unlock()
		return nil
	}

	go func() {
		_ = w.Start(ctx, handler)
	}()

	if !waitForServer(w, 2*time.Second) {
		t.Fatal("Server did not start within timeout")
	}
	addr := w.Address()
	payload := `{"items": [{"id": 1}, {"id": 2}], "metadata": {"count": 2}}`
	resp, err := http.Post("http://"+addr+"/webhook/test", "application/json", bytes.NewBufferString(payload))
	if err != nil {
		t.Fatalf("POST request failed: %v", err)
	}
	defer func() {
		if closeErr := resp.Body.Close(); closeErr != nil {
			t.Logf("failed to close response body: %v", closeErr)
		}
	}()

	if resp.StatusCode != http.StatusOK {
		t.Errorf("Response status = %d, want %d", resp.StatusCode, http.StatusOK)
	}

	time.Sleep(50 * time.Millisecond)
	mu.Lock()
	if len(receivedData) != 2 {
		t.Errorf("Received %d records, want 2", len(receivedData))
	}
	mu.Unlock()
}

func TestWebhook_ParsePayload_MalformedJSON(t *testing.T) {
	config := &connector.ModuleConfig{
		Type: "webhook",
		Config: map[string]interface{}{
			"endpoint":      "/webhook/test",
			"listenAddress": "127.0.0.1:0",
		},
	}

	w, err := NewWebhookFromConfig(config)
	if err != nil {
		t.Fatalf("NewWebhookFromConfig() error = %v", err)
	}

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	handler := func(_ []map[string]interface{}) error {
		return nil
	}

	go func() {
		_ = w.Start(ctx, handler)
	}()

	if !waitForServer(w, 2*time.Second) {
		t.Fatal("Server did not start within timeout")
	}
	addr := w.Address()
	payload := `{invalid json`
	resp, err := http.Post("http://"+addr+"/webhook/test", "application/json", bytes.NewBufferString(payload))
	if err != nil {
		t.Fatalf("POST request failed: %v", err)
	}
	defer func() {
		if closeErr := resp.Body.Close(); closeErr != nil {
			t.Logf("failed to close response body: %v", closeErr)
		}
	}()

	if resp.StatusCode != http.StatusBadRequest {
		t.Errorf("Response status = %d, want %d (Bad Request for malformed JSON)", resp.StatusCode, http.StatusBadRequest)
	}
}

func TestWebhook_ParsePayload_DataFieldWithNonObject(t *testing.T) {
	config := &connector.ModuleConfig{
		Type: "webhook",
		Config: map[string]interface{}{
			"endpoint":      "/webhook/test",
			"listenAddress": "127.0.0.1:0",
			"dataField":     "items",
		},
	}

	w, err := NewWebhookFromConfig(config)
	if err != nil {
		t.Fatalf("NewWebhookFromConfig() error = %v", err)
	}

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	handler := func(_ []map[string]interface{}) error {
		return nil
	}

	go func() {
		_ = w.Start(ctx, handler)
	}()

	if !waitForServer(w, 2*time.Second) {
		t.Fatal("Server did not start within timeout")
	}
	addr := w.Address()
	payload := `{"items": ["bad"]}`
	resp, err := http.Post("http://"+addr+"/webhook/test", "application/json", bytes.NewBufferString(payload))
	if err != nil {
		t.Fatalf("POST request failed: %v", err)
	}
	defer func() {
		if closeErr := resp.Body.Close(); closeErr != nil {
			t.Logf("failed to close response body: %v", closeErr)
		}
	}()

	if resp.StatusCode != http.StatusBadRequest {
		t.Errorf("Response status = %d, want %d (Bad Request for invalid dataField)", resp.StatusCode, http.StatusBadRequest)
	}
}

func TestWebhook_RejectNonPOST(t *testing.T) {
	config := &connector.ModuleConfig{
		Type: "webhook",
		Config: map[string]interface{}{
			"endpoint":      "/webhook/test",
			"listenAddress": "127.0.0.1:0",
		},
	}

	w, err := NewWebhookFromConfig(config)
	if err != nil {
		t.Fatalf("NewWebhookFromConfig() error = %v", err)
	}

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	handler := func(_ []map[string]interface{}) error {
		return nil
	}

	go func() {
		_ = w.Start(ctx, handler)
	}()

	if !waitForServer(w, 2*time.Second) {
		t.Fatal("Server did not start within timeout")
	}
	addr := w.Address()

	// Test GET request
	resp, err := http.Get("http://" + addr + "/webhook/test")
	if err != nil {
		t.Fatalf("GET request failed: %v", err)
	}
	defer func() {
		if closeErr := resp.Body.Close(); closeErr != nil {
			t.Logf("failed to close response body: %v", closeErr)
		}
	}()

	if resp.StatusCode != http.StatusMethodNotAllowed {
		t.Errorf("GET response status = %d, want %d (Method Not Allowed)", resp.StatusCode, http.StatusMethodNotAllowed)
	}
}

func TestWebhook_MissingBody(t *testing.T) {
	config := &connector.ModuleConfig{
		Type: "webhook",
		Config: map[string]interface{}{
			"endpoint":      "/webhook/test",
			"listenAddress": "127.0.0.1:0",
		},
	}

	w, err := NewWebhookFromConfig(config)
	if err != nil {
		t.Fatalf("NewWebhookFromConfig() error = %v", err)
	}

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	handler := func(_ []map[string]interface{}) error {
		return nil
	}

	go func() {
		_ = w.Start(ctx, handler)
	}()

	if !waitForServer(w, 2*time.Second) {
		t.Fatal("Server did not start within timeout")
	}
	addr := w.Address()
	resp, err := http.Post("http://"+addr+"/webhook/test", "application/json", nil)
	if err != nil {
		t.Fatalf("POST request failed: %v", err)
	}
	defer func() {
		if closeErr := resp.Body.Close(); closeErr != nil {
			t.Logf("failed to close response body: %v", closeErr)
		}
	}()

	if resp.StatusCode != http.StatusBadRequest {
		t.Errorf("Response status = %d, want %d (Bad Request for empty body)", resp.StatusCode, http.StatusBadRequest)
	}
}

// =============================================================================
// Task 3: Webhook signature validation tests
// =============================================================================

func TestWebhook_SignatureValidation_Valid(t *testing.T) {
	secret := "test-webhook-secret"
	config := &connector.ModuleConfig{
		Type: "webhook",
		Config: map[string]interface{}{
			"endpoint":      "/webhook/test",
			"listenAddress": "127.0.0.1:0",
			"signature": map[string]interface{}{
				"type":   "hmac-sha256",
				"header": "X-Webhook-Signature",
				"secret": secret,
			},
		},
	}

	w, err := NewWebhookFromConfig(config)
	if err != nil {
		t.Fatalf("NewWebhookFromConfig() error = %v", err)
	}

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	var receivedData []map[string]interface{}
	var mu sync.Mutex
	handler := func(data []map[string]interface{}) error { //nolint:unparam
		mu.Lock()
		receivedData = data
		mu.Unlock()
		return nil
	}

	go func() {
		_ = w.Start(ctx, handler)
	}()

	if !waitForServer(w, 2*time.Second) {
		t.Fatal("Server did not start within timeout")
	}
	addr := w.Address()
	payload := `[{"id": 1}]`

	// Compute HMAC-SHA256 signature
	mac := hmac.New(sha256.New, []byte(secret))
	mac.Write([]byte(payload))
	signature := hex.EncodeToString(mac.Sum(nil))

	req, _ := http.NewRequest("POST", "http://"+addr+"/webhook/test", bytes.NewBufferString(payload))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("X-Webhook-Signature", signature)

	client := &http.Client{}
	resp, err := client.Do(req)
	if err != nil {
		t.Fatalf("POST request failed: %v", err)
	}
	defer func() {
		if closeErr := resp.Body.Close(); closeErr != nil {
			t.Logf("failed to close response body: %v", closeErr)
		}
	}()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		t.Errorf("Response status = %d, want %d. Body: %s", resp.StatusCode, http.StatusOK, string(body))
	}

	time.Sleep(50 * time.Millisecond)
	mu.Lock()
	if len(receivedData) != 1 {
		t.Errorf("Received %d records, want 1", len(receivedData))
	}
	mu.Unlock()
}

func TestWebhook_SignatureValidation_Invalid(t *testing.T) {
	secret := "test-webhook-secret"
	config := &connector.ModuleConfig{
		Type: "webhook",
		Config: map[string]interface{}{
			"endpoint":      "/webhook/test",
			"listenAddress": "127.0.0.1:0",
			"signature": map[string]interface{}{
				"type":   "hmac-sha256",
				"header": "X-Webhook-Signature",
				"secret": secret,
			},
		},
	}

	w, err := NewWebhookFromConfig(config)
	if err != nil {
		t.Fatalf("NewWebhookFromConfig() error = %v", err)
	}

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	handler := func(_ []map[string]interface{}) error {
		return nil
	}

	go func() {
		_ = w.Start(ctx, handler)
	}()

	if !waitForServer(w, 2*time.Second) {
		t.Fatal("Server did not start within timeout")
	}
	addr := w.Address()
	payload := `[{"id": 1}]`

	req, _ := http.NewRequest("POST", "http://"+addr+"/webhook/test", bytes.NewBufferString(payload))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("X-Webhook-Signature", "invalid-signature")

	client := &http.Client{}
	resp, err := client.Do(req)
	if err != nil {
		t.Fatalf("POST request failed: %v", err)
	}
	defer func() {
		if closeErr := resp.Body.Close(); closeErr != nil {
			t.Logf("failed to close response body: %v", closeErr)
		}
	}()

	if resp.StatusCode != http.StatusUnauthorized {
		t.Errorf("Response status = %d, want %d (Unauthorized for invalid signature)", resp.StatusCode, http.StatusUnauthorized)
	}
}

func TestWebhook_SignatureValidation_MissingSignature(t *testing.T) {
	config := &connector.ModuleConfig{
		Type: "webhook",
		Config: map[string]interface{}{
			"endpoint":      "/webhook/test",
			"listenAddress": "127.0.0.1:0",
			"signature": map[string]interface{}{
				"type":   "hmac-sha256",
				"header": "X-Webhook-Signature",
				"secret": "test-secret",
			},
		},
	}

	w, err := NewWebhookFromConfig(config)
	if err != nil {
		t.Fatalf("NewWebhookFromConfig() error = %v", err)
	}

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	handler := func(_ []map[string]interface{}) error {
		return nil
	}

	go func() {
		_ = w.Start(ctx, handler)
	}()

	if !waitForServer(w, 2*time.Second) {
		t.Fatal("Server did not start within timeout")
	}
	addr := w.Address()
	payload := `[{"id": 1}]`

	// Don't include signature header
	resp, err := http.Post("http://"+addr+"/webhook/test", "application/json", bytes.NewBufferString(payload))
	if err != nil {
		t.Fatalf("POST request failed: %v", err)
	}
	defer func() {
		if closeErr := resp.Body.Close(); closeErr != nil {
			t.Logf("failed to close response body: %v", closeErr)
		}
	}()

	if resp.StatusCode != http.StatusUnauthorized {
		t.Errorf("Response status = %d, want %d (Unauthorized for missing signature)", resp.StatusCode, http.StatusUnauthorized)
	}
}

func TestWebhook_SignatureValidation_CustomHeader(t *testing.T) {
	secret := "my-secret"
	config := &connector.ModuleConfig{
		Type: "webhook",
		Config: map[string]interface{}{
			"endpoint":      "/webhook/test",
			"listenAddress": "127.0.0.1:0",
			"signature": map[string]interface{}{
				"type":   "hmac-sha256",
				"header": "X-Custom-Signature",
				"secret": secret,
			},
		},
	}

	w, err := NewWebhookFromConfig(config)
	if err != nil {
		t.Fatalf("NewWebhookFromConfig() error = %v", err)
	}

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	var receivedData []map[string]interface{}
	var mu sync.Mutex
	handler := func(data []map[string]interface{}) error { //nolint:unparam
		mu.Lock()
		receivedData = data
		mu.Unlock()
		return nil
	}

	go func() {
		_ = w.Start(ctx, handler)
	}()

	if !waitForServer(w, 2*time.Second) {
		t.Fatal("Server did not start within timeout")
	}
	addr := w.Address()
	payload := `[{"id": 1}]`

	// Compute signature
	mac := hmac.New(sha256.New, []byte(secret))
	mac.Write([]byte(payload))
	signature := hex.EncodeToString(mac.Sum(nil))

	req, _ := http.NewRequest("POST", "http://"+addr+"/webhook/test", bytes.NewBufferString(payload))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("X-Custom-Signature", signature)

	client := &http.Client{}
	resp, err := client.Do(req)
	if err != nil {
		t.Fatalf("POST request failed: %v", err)
	}
	defer func() {
		if closeErr := resp.Body.Close(); closeErr != nil {
			t.Logf("failed to close response body: %v", closeErr)
		}
	}()

	if resp.StatusCode != http.StatusOK {
		t.Errorf("Response status = %d, want %d", resp.StatusCode, http.StatusOK)
	}

	time.Sleep(50 * time.Millisecond)
	mu.Lock()
	if len(receivedData) != 1 {
		t.Errorf("Received %d records, want 1", len(receivedData))
	}
	mu.Unlock()
}

func TestWebhook_RateLimiting(t *testing.T) {
	// Use very low rate (1 req/10sec) to ensure second request is rate limited
	// Burst of 1 means only 1 token available initially
	config := &connector.ModuleConfig{
		Type: "webhook",
		Config: map[string]interface{}{
			"endpoint":      "/webhook/test",
			"listenAddress": "127.0.0.1:0",
			"rateLimit": map[string]interface{}{
				"requestsPerSecond": float64(1), // 1 token per second refill
				"burst":             float64(1), // Only 1 token available
			},
		},
	}

	w, err := NewWebhookFromConfig(config)
	if err != nil {
		t.Fatalf("NewWebhookFromConfig() error = %v", err)
	}

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	handler := func(_ []map[string]interface{}) error {
		return nil
	}

	go func() {
		_ = w.Start(ctx, handler)
	}()

	if !waitForServer(w, 2*time.Second) {
		t.Fatal("Server did not start within timeout")
	}
	addr := w.Address()
	payload := `[{"id": 1}]`

	// First request consumes the only token
	resp1, err := http.Post("http://"+addr+"/webhook/test", "application/json", bytes.NewBufferString(payload))
	if err != nil {
		t.Fatalf("POST request failed: %v", err)
	}
	_ = resp1.Body.Close()
	if resp1.StatusCode != http.StatusOK {
		t.Fatalf("First request status = %d, want %d", resp1.StatusCode, http.StatusOK)
	}

	// Second request immediately after should be rate limited (no tokens left)
	// Send multiple requests in quick succession to ensure at least one gets rate limited
	rateLimited := false
	for i := 0; i < 5; i++ {
		resp, err := http.Post("http://"+addr+"/webhook/test", "application/json", bytes.NewBufferString(payload))
		if err != nil {
			t.Fatalf("POST request %d failed: %v", i+2, err)
		}
		_ = resp.Body.Close()
		if resp.StatusCode == http.StatusTooManyRequests {
			rateLimited = true
			break
		}
	}

	if !rateLimited {
		t.Error("Expected at least one request to be rate limited (429)")
	}
}

// =============================================================================
// Task 4: Concurrent request handling tests
// =============================================================================

func TestWebhook_ConcurrentRequests(t *testing.T) {
	config := &connector.ModuleConfig{
		Type: "webhook",
		Config: map[string]interface{}{
			"endpoint":      "/webhook/test",
			"listenAddress": "127.0.0.1:0",
		},
	}

	w, err := NewWebhookFromConfig(config)
	if err != nil {
		t.Fatalf("NewWebhookFromConfig() error = %v", err)
	}

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	var receivedCount int64
	handler := func(data []map[string]interface{}) error { //nolint:unparam
		atomic.AddInt64(&receivedCount, int64(len(data)))
		return nil
	}

	go func() {
		_ = w.Start(ctx, handler)
	}()

	if !waitForServer(w, 2*time.Second) {
		t.Fatal("Server did not start within timeout")
	}
	addr := w.Address()
	numRequests := 50

	var wg sync.WaitGroup
	wg.Add(numRequests)

	for i := 0; i < numRequests; i++ {
		go func(id int) {
			defer wg.Done()
			payload := fmt.Sprintf(`[{"id": %d}]`, id)
			resp, err := http.Post("http://"+addr+"/webhook/test", "application/json", bytes.NewBufferString(payload))
			if err != nil {
				t.Errorf("Request %d failed: %v", id, err)
				return
			}
			defer func() {
				if closeErr := resp.Body.Close(); closeErr != nil {
					t.Logf("failed to close response body: %v", closeErr)
				}
			}()
			if resp.StatusCode != http.StatusOK {
				t.Errorf("Request %d status = %d, want %d", id, resp.StatusCode, http.StatusOK)
			}
		}(i)
	}

	wg.Wait()
	time.Sleep(100 * time.Millisecond)

	finalCount := atomic.LoadInt64(&receivedCount)
	if finalCount != int64(numRequests) {
		t.Errorf("Received %d total records, want %d", finalCount, numRequests)
	}
}

func TestWebhook_ThreadSafeDataProcessing(t *testing.T) {
	config := &connector.ModuleConfig{
		Type: "webhook",
		Config: map[string]interface{}{
			"endpoint":      "/webhook/test",
			"listenAddress": "127.0.0.1:0",
		},
	}

	w, err := NewWebhookFromConfig(config)
	if err != nil {
		t.Fatalf("NewWebhookFromConfig() error = %v", err)
	}

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	var allData [][]map[string]interface{}
	var mu sync.Mutex
	handler := func(data []map[string]interface{}) error { //nolint:unparam
		mu.Lock()
		allData = append(allData, data)
		mu.Unlock()
		return nil
	}

	go func() {
		_ = w.Start(ctx, handler)
	}()

	if !waitForServer(w, 2*time.Second) {
		t.Fatal("Server did not start within timeout")
	}
	addr := w.Address()
	numRequests := 20

	var wg sync.WaitGroup
	wg.Add(numRequests)

	for i := 0; i < numRequests; i++ {
		go func(id int) {
			defer wg.Done()
			payload := fmt.Sprintf(`[{"requestId": %d, "value": "data-%d"}]`, id, id)
			resp, err := http.Post("http://"+addr+"/webhook/test", "application/json", bytes.NewBufferString(payload))
			if err != nil {
				t.Errorf("Request %d failed: %v", id, err)
				return
			}
			defer func() {
				if closeErr := resp.Body.Close(); closeErr != nil {
					t.Logf("failed to close response body: %v", closeErr)
				}
			}()
		}(i)
	}

	wg.Wait()
	time.Sleep(100 * time.Millisecond)

	mu.Lock()
	if len(allData) != numRequests {
		t.Errorf("Received %d batches, want %d", len(allData), numRequests)
	}
	mu.Unlock()
}

func TestWebhook_QueueBackpressure(t *testing.T) {
	// Test queue backpressure behavior:
	// - queueSize=1: only 1 item can wait in queue
	// - maxConcurrent=1: only 1 worker processing at a time
	// - Handler blocks, so worker holds 1 item and queue can hold 1 more
	// - Total capacity = 2 (1 processing + 1 queued)
	// - 3rd request should be rejected
	config := &connector.ModuleConfig{
		Type: "webhook",
		Config: map[string]interface{}{
			"endpoint":      "/webhook/test",
			"listenAddress": "127.0.0.1:0",
			"queueSize":     float64(1),
			"maxConcurrent": float64(1),
		},
	}

	w, err := NewWebhookFromConfig(config)
	if err != nil {
		t.Fatalf("NewWebhookFromConfig() error = %v", err)
	}

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	// Handler blocks until we signal it - this ensures queue fills up
	blocker := make(chan struct{})
	var handled int64
	handler := func(data []map[string]interface{}) error { //nolint:unparam
		<-blocker
		atomic.AddInt64(&handled, int64(len(data)))
		return nil
	}

	go func() {
		_ = w.Start(ctx, handler)
	}()

	if !waitForServer(w, 2*time.Second) {
		t.Fatal("Server did not start within timeout")
	}

	addr := w.Address()
	payload := `[{"id": 1}]`

	// First request - goes to queue, worker picks it up and blocks
	resp1, err := http.Post("http://"+addr+"/webhook/test", "application/json", bytes.NewBufferString(payload))
	if err != nil {
		t.Fatalf("POST request 1 failed: %v", err)
	}
	_ = resp1.Body.Close()
	if resp1.StatusCode != http.StatusAccepted {
		t.Fatalf("First request status = %d, want %d", resp1.StatusCode, http.StatusAccepted)
	}

	// Give the worker time to pick up the first item
	time.Sleep(50 * time.Millisecond)

	// Second request - goes to queue (worker is blocked, queue has 1 slot)
	resp2, err := http.Post("http://"+addr+"/webhook/test", "application/json", bytes.NewBufferString(payload))
	if err != nil {
		t.Fatalf("POST request 2 failed: %v", err)
	}
	_ = resp2.Body.Close()
	if resp2.StatusCode != http.StatusAccepted {
		t.Fatalf("Second request status = %d, want %d", resp2.StatusCode, http.StatusAccepted)
	}

	// Third request - queue is full (1 processing + 1 queued), should be rejected
	resp3, err := http.Post("http://"+addr+"/webhook/test", "application/json", bytes.NewBufferString(payload))
	if err != nil {
		t.Fatalf("POST request 3 failed: %v", err)
	}
	_ = resp3.Body.Close()
	if resp3.StatusCode != http.StatusTooManyRequests {
		t.Errorf("Third request status = %d, want %d (queue full)", resp3.StatusCode, http.StatusTooManyRequests)
	}

	// Unblock handler and let it process
	close(blocker)

	// Wait for processing to complete (should process 2 items)
	deadline := time.Now().Add(2 * time.Second)
	for atomic.LoadInt64(&handled) < 2 && time.Now().Before(deadline) {
		time.Sleep(10 * time.Millisecond)
	}

	finalHandled := atomic.LoadInt64(&handled)
	if finalHandled != 2 {
		t.Errorf("Handled %d records, want 2", finalHandled)
	}
}

// =============================================================================
// Task 5: Pipeline integration tests
// =============================================================================

func TestWebhook_Fetch_NotImplemented(t *testing.T) {
	config := &connector.ModuleConfig{
		Type: "webhook",
		Config: map[string]interface{}{
			"endpoint":      "/webhook/test",
			"listenAddress": "127.0.0.1:0",
		},
	}

	w, err := NewWebhookFromConfig(config)
	if err != nil {
		t.Fatalf("NewWebhookFromConfig() error = %v", err)
	}

	// Webhooks are push-based, Fetch() should return an error or empty result
	data, err := w.Fetch(context.Background())
	if err != ErrNotImplemented {
		t.Errorf("Webhook.Fetch() error = %v, want %v (webhooks use callback pattern)", err, ErrNotImplemented)
	}
	if data != nil {
		t.Errorf("Webhook.Fetch() data = %v, want nil", data)
	}
}

func TestWebhook_DataFlowToHandler(t *testing.T) {
	config := &connector.ModuleConfig{
		Type: "webhook",
		Config: map[string]interface{}{
			"endpoint":      "/webhook/test",
			"listenAddress": "127.0.0.1:0",
		},
	}

	w, err := NewWebhookFromConfig(config)
	if err != nil {
		t.Fatalf("NewWebhookFromConfig() error = %v", err)
	}

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	receivedChan := make(chan []map[string]interface{}, 10)
	handler := func(data []map[string]interface{}) error { //nolint:unparam
		receivedChan <- data
		return nil
	}

	go func() {
		_ = w.Start(ctx, handler)
	}()

	if !waitForServer(w, 2*time.Second) {
		t.Fatal("Server did not start within timeout")
	}
	addr := w.Address()
	expectedData := []map[string]interface{}{
		{"orderId": 1, "product": "Widget A", "quantity": 5},
		{"orderId": 2, "product": "Widget B", "quantity": 3},
	}

	payloadBytes, _ := json.Marshal(expectedData)
	resp, err := http.Post("http://"+addr+"/webhook/test", "application/json", bytes.NewBuffer(payloadBytes))
	if err != nil {
		t.Fatalf("POST request failed: %v", err)
	}
	_ = resp.Body.Close()

	// Wait for data
	select {
	case received := <-receivedChan:
		if len(received) != 2 {
			t.Errorf("Received %d records, want 2", len(received))
		}
		// Verify data integrity
		if received[0]["orderId"] != float64(1) {
			t.Errorf("First record orderId = %v, want 1", received[0]["orderId"])
		}
	case <-time.After(2 * time.Second):
		t.Fatal("Did not receive data within timeout")
	}
}

func TestWebhook_HandlerError_ReturnsServerError(t *testing.T) {
	config := &connector.ModuleConfig{
		Type: "webhook",
		Config: map[string]interface{}{
			"endpoint":      "/webhook/test",
			"listenAddress": "127.0.0.1:0",
		},
	}

	w, err := NewWebhookFromConfig(config)
	if err != nil {
		t.Fatalf("NewWebhookFromConfig() error = %v", err)
	}

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	// Handler that returns an error
	handler := func(_ []map[string]interface{}) error {
		return fmt.Errorf("pipeline execution failed")
	}

	go func() {
		_ = w.Start(ctx, handler)
	}()

	if !waitForServer(w, 2*time.Second) {
		t.Fatal("Server did not start within timeout")
	}
	addr := w.Address()
	payload := `[{"id": 1}]`
	resp, err := http.Post("http://"+addr+"/webhook/test", "application/json", bytes.NewBufferString(payload))
	if err != nil {
		t.Fatalf("POST request failed: %v", err)
	}
	defer func() {
		if closeErr := resp.Body.Close(); closeErr != nil {
			t.Logf("failed to close response body: %v", closeErr)
		}
	}()

	// When handler returns error, webhook should return 500 Internal Server Error
	if resp.StatusCode != http.StatusInternalServerError {
		t.Errorf("Response status = %d, want %d", resp.StatusCode, http.StatusInternalServerError)
	}
}

// =============================================================================
// Task 6: Error handling and logging tests
// =============================================================================

func TestWebhook_InvalidEndpoint_Returns404(t *testing.T) {
	config := &connector.ModuleConfig{
		Type: "webhook",
		Config: map[string]interface{}{
			"endpoint":      "/webhook/orders",
			"listenAddress": "127.0.0.1:0",
		},
	}

	w, err := NewWebhookFromConfig(config)
	if err != nil {
		t.Fatalf("NewWebhookFromConfig() error = %v", err)
	}

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	handler := func(_ []map[string]interface{}) error {
		return nil
	}

	go func() {
		_ = w.Start(ctx, handler)
	}()

	if !waitForServer(w, 2*time.Second) {
		t.Fatal("Server did not start within timeout")
	}
	addr := w.Address()

	// POST to wrong endpoint
	resp, err := http.Post("http://"+addr+"/webhook/wrong", "application/json", bytes.NewBufferString(`{}`))
	if err != nil {
		t.Fatalf("POST request failed: %v", err)
	}
	defer func() {
		if closeErr := resp.Body.Close(); closeErr != nil {
			t.Logf("failed to close response body: %v", closeErr)
		}
	}()

	if resp.StatusCode != http.StatusNotFound {
		t.Errorf("Response status = %d, want %d (Not Found for wrong endpoint)", resp.StatusCode, http.StatusNotFound)
	}
}

func TestWebhook_DeterministicBehavior(t *testing.T) {
	config := &connector.ModuleConfig{
		Type: "webhook",
		Config: map[string]interface{}{
			"endpoint":      "/webhook/test",
			"listenAddress": "127.0.0.1:0",
		},
	}

	w, err := NewWebhookFromConfig(config)
	if err != nil {
		t.Fatalf("NewWebhookFromConfig() error = %v", err)
	}

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	var results [][]map[string]interface{}
	var mu sync.Mutex
	handler := func(data []map[string]interface{}) error { //nolint:unparam
		mu.Lock()
		results = append(results, data)
		mu.Unlock()
		return nil
	}

	go func() {
		_ = w.Start(ctx, handler)
	}()

	if !waitForServer(w, 2*time.Second) {
		t.Fatal("Server did not start within timeout")
	}
	addr := w.Address()
	payload := `[{"id": 1, "value": "test"}]`

	// Send same payload multiple times
	for i := 0; i < 5; i++ {
		resp, err := http.Post("http://"+addr+"/webhook/test", "application/json", bytes.NewBufferString(payload))
		if err != nil {
			t.Fatalf("Request %d failed: %v", i, err)
		}
		_ = resp.Body.Close()
	}

	time.Sleep(100 * time.Millisecond)

	mu.Lock()
	defer mu.Unlock()

	// All results should be identical
	for i, result := range results {
		if len(result) != 1 {
			t.Errorf("Result %d has %d records, want 1", i, len(result))
			continue
		}
		if result[0]["id"] != float64(1) || result[0]["value"] != "test" {
			t.Errorf("Result %d = %v, want {id:1, value:test}", i, result[0])
		}
	}
}

// =============================================================================
// Benchmark tests
// =============================================================================

func BenchmarkWebhook_HandleRequest(b *testing.B) {
	config := &connector.ModuleConfig{
		Type: "webhook",
		Config: map[string]interface{}{
			"endpoint":      "/webhook/bench",
			"listenAddress": "127.0.0.1:0",
		},
	}

	w, _ := NewWebhookFromConfig(config)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	handler := func(_ []map[string]interface{}) error {
		return nil
	}

	go func() {
		_ = w.Start(ctx, handler)
	}()

	if !waitForServer(w, 2*time.Second) {
		b.Fatal("Server did not start within timeout")
	}

	addr := w.Address()
	payload := `[{"id": 1, "name": "Test", "value": 123.45}]`

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		resp, err := http.Post("http://"+addr+"/webhook/bench", "application/json", bytes.NewBufferString(payload))
		if err != nil {
			b.Fatalf("Request failed: %v", err)
		}
		_ = resp.Body.Close()
	}
}

// =============================================================================
// httptest-based unit tests (testing handler in isolation)
// =============================================================================

func TestWebhookHandler_ParsesJSONArray(t *testing.T) {
	w := &Webhook{
		endpoint: "/webhook/test",
	}

	var receivedData []map[string]interface{}
	handler := func(data []map[string]interface{}) error { //nolint:unparam
		receivedData = data
		return nil
	}

	// Create handler
	h := w.createHandler(handler)

	// Create test request
	payload := `[{"id": 1}, {"id": 2}]`
	req := httptest.NewRequest("POST", "/webhook/test", bytes.NewBufferString(payload))
	req.Header.Set("Content-Type", "application/json")
	rr := httptest.NewRecorder()

	h.ServeHTTP(rr, req)

	if rr.Code != http.StatusOK {
		t.Errorf("Handler returned status %d, want %d", rr.Code, http.StatusOK)
	}

	if len(receivedData) != 2 {
		t.Errorf("Handler received %d records, want 2", len(receivedData))
	}
}

func TestWebhookHandler_RejectsPUT(t *testing.T) {
	w := &Webhook{
		endpoint: "/webhook/test",
	}

	handler := func(_ []map[string]interface{}) error {
		return nil
	}

	h := w.createHandler(handler)

	req := httptest.NewRequest("PUT", "/webhook/test", bytes.NewBufferString(`{}`))
	rr := httptest.NewRecorder()

	h.ServeHTTP(rr, req)

	if rr.Code != http.StatusMethodNotAllowed {
		t.Errorf("Handler returned status %d, want %d", rr.Code, http.StatusMethodNotAllowed)
	}
}
