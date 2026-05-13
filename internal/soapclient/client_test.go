package soapclient

import (
	"context"
	"errors"
	"io"
	"mime"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/cannectors/runtime/internal/httpclient"
	"github.com/cannectors/runtime/pkg/connector"
)

func TestClient_Call_SOAP11(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		mediaType, params, err := mime.ParseMediaType(r.Header.Get("Content-Type"))
		if err != nil {
			t.Fatalf("invalid content-type: %v", err)
		}
		if mediaType != `text/xml` || params["charset"] != "utf-8" {
			t.Fatalf("unexpected content-type: %q params=%v", mediaType, params)
		}
		if got := r.Header.Get("SOAPAction"); got != `"urn:Ping"` {
			t.Fatalf("unexpected SOAPAction: %q", got)
		}
		body, _ := io.ReadAll(r.Body)
		if !strings.Contains(string(body), NamespaceSOAP11) {
			t.Fatalf("SOAP 1.1 namespace missing from request:\n%s", body)
		}
		w.Header().Set("Content-Type", "text/xml")
		_, _ = w.Write([]byte(`<soap:Envelope xmlns:soap="http://schemas.xmlsoap.org/soap/envelope/"><soap:Body><PingResponse><Result>ok</Result></PingResponse></soap:Body></soap:Envelope>`))
	}))
	defer server.Close()

	client := NewClient(httpclient.NewClient(time.Second))
	resp, err := client.Call(context.Background(), SOAPOperation{
		Endpoint:   server.URL,
		SOAPAction: "urn:Ping",
		Version:    SOAPVersion11,
		Body:       `<Ping/>`,
		HTTPHeaders: map[string]string{
			"Content-Type": "application/json",
			"SOAPAction":   "wrong",
		},
	})
	if err != nil {
		t.Fatalf("Call returned error: %v", err)
	}
	if resp.StatusCode != http.StatusOK || len(resp.Data) == 0 {
		t.Fatalf("unexpected response: %#v", resp)
	}
}

func TestClient_Call_MTOMRoundTrip(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		envelope, attachments, multipartRequest, err := ParseMTOMResponse(r.Header.Get("Content-Type"), r.Body)
		if err != nil {
			t.Fatalf("ParseMTOMResponse request returned error: %v", err)
		}
		if !multipartRequest {
			t.Fatal("expected multipart MTOM request")
		}
		if !strings.Contains(string(envelope), `href="cid:doc-1"`) {
			t.Fatalf("request envelope missing xop include:\n%s", envelope)
		}
		if string(attachments["doc-1"].Data) != "pdf-bytes" {
			t.Fatalf("unexpected request attachment: %#v", attachments["doc-1"])
		}

		var b strings.Builder
		writer := multipart.NewWriter(&b)
		root, _ := writer.CreatePart(map[string][]string{
			"Content-Type": {"application/xop+xml; charset=utf-8"},
			"Content-ID":   {"<response-root>"},
		})
		_, _ = root.Write([]byte(`<soap:Envelope xmlns:soap="http://schemas.xmlsoap.org/soap/envelope/"><soap:Body><UploadResponse><Status>ok</Status><xop:Include xmlns:xop="http://www.w3.org/2004/08/xop/include" href="cid:receipt-1"/></UploadResponse></soap:Body></soap:Envelope>`))
		part, _ := writer.CreatePart(map[string][]string{
			"Content-Type": {"text/plain"},
			"Content-ID":   {"<receipt-1>"},
		})
		_, _ = part.Write([]byte("receipt-bytes"))
		_ = writer.Close()
		w.Header().Set("Content-Type", `multipart/related; type="application/xop+xml"; start="<response-root>"; start-info="application/soap+xml"; boundary="`+writer.Boundary()+`"`)
		_, _ = w.Write([]byte(b.String()))
	}))
	defer server.Close()

	client := NewClient(httpclient.NewClient(time.Second))
	resp, err := client.Call(context.Background(), SOAPOperation{
		Endpoint:   server.URL,
		SOAPAction: "urn:Upload",
		Version:    SOAPVersion11,
		Body:       `<Upload><File><xop:Include xmlns:xop="http://www.w3.org/2004/08/xop/include" href="cid:doc-1"/></File></Upload>`,
		MTOM: MTOMConfig{
			Enabled: true,
			Attachments: []MTOMAttachment{
				{ContentID: "doc-1", ContentType: "application/pdf", Data: []byte("pdf-bytes")},
			},
		},
	})
	if err != nil {
		t.Fatalf("Call returned error: %v", err)
	}
	if string(resp.Attachments["receipt-1"].Data) != "receipt-bytes" {
		t.Fatalf("unexpected response attachments: %#v", resp.Attachments)
	}
}

func TestClient_Call_WSSecurityHeader(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body, _ := io.ReadAll(r.Body)
		for _, want := range []string{wsseNamespace, `<Username`, `alice</Username>`, `#PasswordText`} {
			if !strings.Contains(string(body), want) {
				t.Fatalf("request missing %q:\n%s", want, body)
			}
		}
		w.Header().Set("Content-Type", "text/xml")
		_, _ = w.Write([]byte(`<soap:Envelope xmlns:soap="http://schemas.xmlsoap.org/soap/envelope/"><soap:Body><PingResponse><Result>ok</Result></PingResponse></soap:Body></soap:Envelope>`))
	}))
	defer server.Close()

	client := NewClient(httpclient.NewClient(time.Second))
	_, err := client.Call(context.Background(), SOAPOperation{
		Endpoint:   server.URL,
		SOAPAction: "urn:Ping",
		Version:    SOAPVersion11,
		Body:       `<Ping/>`,
		WSSecurity: &WSSecurityConfig{
			Username:     "alice",
			Password:     "secret",
			PasswordType: WSSPasswordText,
		},
	})
	if err != nil {
		t.Fatalf("Call returned error: %v", err)
	}
}

func TestClient_Call_Fault11(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "text/xml")
		w.WriteHeader(http.StatusInternalServerError)
		_, _ = w.Write([]byte(`<soap:Envelope xmlns:soap="http://schemas.xmlsoap.org/soap/envelope/"><soap:Body><soap:Fault><faultcode>soap:Server</faultcode><faultstring>Transient</faultstring></soap:Fault></soap:Body></soap:Envelope>`))
	}))
	defer server.Close()

	client := NewClient(httpclient.NewClient(time.Second))
	_, err := client.Call(context.Background(), SOAPOperation{
		Endpoint:   server.URL,
		SOAPAction: "urn:Ping",
		Body:       `<Ping/>`,
	})
	var fault *SOAPFaultError
	if !errors.As(err, &fault) {
		t.Fatalf("expected SOAPFaultError, got %T: %v", err, err)
	}
	if fault.Version != SOAPVersion11 || fault.Code != "soap:Server" {
		t.Fatalf("unexpected fault: %#v", fault)
	}
}

func TestClient_Call_Fault12(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/soap+xml")
		w.WriteHeader(http.StatusInternalServerError)
		_, _ = w.Write([]byte(`<env:Envelope xmlns:env="http://www.w3.org/2003/05/soap-envelope"><env:Body><env:Fault><env:Code><env:Value>env:Receiver</env:Value></env:Code><env:Reason><env:Text>Transient</env:Text></env:Reason></env:Fault></env:Body></env:Envelope>`))
	}))
	defer server.Close()

	client := NewClient(httpclient.NewClient(time.Second))
	_, err := client.Call(context.Background(), SOAPOperation{
		Endpoint:   server.URL,
		SOAPAction: "urn:Ping",
		Version:    SOAPVersion12,
		Body:       `<Ping/>`,
	})
	var fault *SOAPFaultError
	if !errors.As(err, &fault) {
		t.Fatalf("expected SOAPFaultError, got %T: %v", err, err)
	}
	if fault.Version != SOAPVersion12 || fault.Code != "env:Receiver" {
		t.Fatalf("unexpected fault: %#v", fault)
	}
}

func TestClient_Call_HTTP500WithoutFault(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "text/plain")
		w.WriteHeader(http.StatusInternalServerError)
		_, _ = w.Write([]byte(`broken`))
	}))
	defer server.Close()

	client := NewClient(httpclient.NewClient(time.Second))
	_, err := client.Call(context.Background(), SOAPOperation{
		Endpoint:   server.URL,
		SOAPAction: "urn:Ping",
		Body:       `<Ping/>`,
	})
	var httpErr *httpclient.Error
	if !errors.As(err, &httpErr) {
		t.Fatalf("expected httpclient.Error, got %T: %v", err, err)
	}
	if httpErr.StatusCode != http.StatusInternalServerError {
		t.Fatalf("unexpected status: %d", httpErr.StatusCode)
	}
}

func TestClient_Call_RelativeEndpointReturnsError(t *testing.T) {
	client := NewClient(httpclient.NewClient(time.Second))
	_, err := client.Call(context.Background(), SOAPOperation{
		Endpoint: "/relative",
		Body:     `<Ping/>`,
	})
	if err == nil {
		t.Fatal("expected endpoint validation error")
	}
}

func TestClient_Call_UsesRetryConfig(t *testing.T) {
	var calls int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		if atomic.AddInt32(&calls, 1) == 1 {
			w.WriteHeader(http.StatusInternalServerError)
			_, _ = w.Write([]byte(`temporary`))
			return
		}
		w.Header().Set("Content-Type", "text/xml")
		_, _ = w.Write([]byte(`<soap:Envelope xmlns:soap="http://schemas.xmlsoap.org/soap/envelope/"><soap:Body><PingResponse><Result>ok</Result></PingResponse></soap:Body></soap:Envelope>`))
	}))
	defer server.Close()

	client := NewClient(httpclient.NewClient(time.Second))
	resp, err := client.Call(context.Background(), SOAPOperation{
		Endpoint:   server.URL,
		SOAPAction: "urn:Ping",
		Body:       `<Ping/>`,
		Retry: &connector.RetryConfig{
			MaxAttempts:          1,
			DelayMs:              1,
			BackoffMultiplier:    1,
			MaxDelayMs:           1,
			RetryableStatusCodes: []int{500},
		},
	})
	if err != nil {
		t.Fatalf("Call returned error: %v", err)
	}
	if got := atomic.LoadInt32(&calls); got != 2 {
		t.Fatalf("calls = %d, want 2", got)
	}
	if resp.RetryInfo == nil || resp.RetryInfo.TotalAttempts != 2 || resp.RetryInfo.RetryCount != 1 {
		t.Fatalf("unexpected retry info: %#v", resp.RetryInfo)
	}
}

func TestClient_Call_SOAP12(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if got := r.Header.Get("SOAPAction"); got != "" {
			t.Fatalf("SOAP 1.2 must not send SOAPAction header, got %q", got)
		}
		contentType := r.Header.Get("Content-Type")
		if !strings.HasPrefix(contentType, `application/soap+xml`) || !strings.Contains(contentType, `action="urn:Ping"`) {
			t.Fatalf("unexpected content-type: %q", contentType)
		}
		body, _ := io.ReadAll(r.Body)
		if !strings.Contains(string(body), NamespaceSOAP12) {
			t.Fatalf("SOAP 1.2 namespace missing from request:\n%s", body)
		}
		w.Header().Set("Content-Type", "application/soap+xml")
		_, _ = w.Write([]byte(`<env:Envelope xmlns:env="http://www.w3.org/2003/05/soap-envelope"><env:Body><PingResponse><Result>ok</Result></PingResponse></env:Body></env:Envelope>`))
	}))
	defer server.Close()

	client := NewClient(httpclient.NewClient(time.Second))
	resp, err := client.Call(context.Background(), SOAPOperation{
		Endpoint:   server.URL,
		SOAPAction: "urn:Ping",
		Version:    SOAPVersion12,
		Body:       `<Ping/>`,
	})
	if err != nil {
		t.Fatalf("Call returned error: %v", err)
	}
	if resp.StatusCode != http.StatusOK || len(resp.Data) == 0 {
		t.Fatalf("unexpected response: %#v", resp)
	}
}
