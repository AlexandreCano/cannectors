package moduleconfig_test

import (
	"encoding/json"
	"testing"

	"github.com/cannectors/runtime/internal/moduleconfig"
)

func TestParseConfigSOAPRequestBase(t *testing.T) {
	raw := json.RawMessage(`{
		"endpoint":"https://soap.example.com/service",
		"soapVersion":"1.2",
		"soapAction":"urn:Import",
		"operation":"Import",
		"body":"<m:Import/>",
		"headers":[{"xml":"<m:Trace>abc</m:Trace>"}],
		"httpHeaders":{"X-Correlation-ID":"abc"},
		"wsSecurity":{"username":"alice","password":"secret","passwordType":"PasswordDigest","mustUnderstand":true},
		"mtom":{"enabled":true,"attachments":[{"contentId":"doc-1","contentType":"application/pdf","sourceField":"document","encoding":"base64"}]},
		"timeoutMs":5000
	}`)

	cfg, err := moduleconfig.ParseConfig[moduleconfig.SOAPRequestBase](raw)
	if err != nil {
		t.Fatalf("ParseConfig() error = %v", err)
	}
	if cfg.Endpoint != "https://soap.example.com/service" || cfg.SOAPVersion != "1.2" || cfg.Operation != "Import" {
		t.Fatalf("unexpected base fields: %#v", cfg)
	}
	if cfg.WSSecurity == nil || !cfg.WSSecurity.MustUnderstand || cfg.WSSecurity.PasswordType != "PasswordDigest" {
		t.Fatalf("unexpected wsSecurity: %#v", cfg.WSSecurity)
	}
	if !cfg.MTOM.Enabled || len(cfg.MTOM.Attachments) != 1 || cfg.MTOM.Attachments[0].SourceField != "document" || cfg.MTOM.Attachments[0].Encoding != "base64" {
		t.Fatalf("unexpected mtom: %#v", cfg.MTOM)
	}
	if len(cfg.Headers) != 1 || cfg.Headers[0].XML == "" {
		t.Fatalf("unexpected SOAP headers: %#v", cfg.Headers)
	}
}
