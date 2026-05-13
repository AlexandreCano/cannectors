//nolint:all
package soap

import (
	"encoding/xml"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

type testPing struct {
	XMLName xml.Name `xml:"Ping"`
}

type testPingResponse struct {
	XMLName xml.Name `xml:"PingResponse"`
	Result  string   `xml:"Result"`
}

func TestClientCallSOAP12Headers(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if got := r.Header.Get("SOAPAction"); got != "" {
			t.Fatalf("SOAPAction header must be empty for SOAP 1.2, got %q", got)
		}
		contentType := r.Header.Get("Content-Type")
		if !strings.HasPrefix(contentType, `application/soap+xml`) || !strings.Contains(contentType, `action="urn:Ping"`) {
			t.Fatalf("unexpected content-type: %q", contentType)
		}
		body, _ := io.ReadAll(r.Body)
		if !strings.Contains(string(body), XmlNsSoapEnv12) {
			t.Fatalf("SOAP 1.2 namespace missing from request:\n%s", body)
		}
		_, _ = w.Write([]byte(`<env:Envelope xmlns:env="http://www.w3.org/2003/05/soap-envelope"><env:Body><PingResponse><Result>ok</Result></PingResponse></env:Body></env:Envelope>`))
	}))
	defer server.Close()

	client := NewClient(server.URL, WithSOAPVersion(SOAP12))
	var response testPingResponse
	if err := client.Call("urn:Ping", &testPing{}, &response); err != nil {
		t.Fatalf("Call returned error: %v", err)
	}
	if response.Result != "ok" {
		t.Fatalf("unexpected response: %#v", response)
	}
}
