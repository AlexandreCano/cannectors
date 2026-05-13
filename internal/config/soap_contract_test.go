package config_test

import (
	"encoding/json"
	"fmt"
	"testing"

	"github.com/cannectors/runtime/internal/config"
)

func validatePipelineJSON(t *testing.T, raw string) bool {
	t.Helper()
	var data map[string]any
	if err := json.Unmarshal([]byte(raw), &data); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	return config.ValidateConfig(data).Valid
}

func TestSchemaSOAPModulesAccepted(t *testing.T) {
	raw := `{
		"name":"soap",
		"version":"1.0.0",
		"input":{
			"type":"soapPolling",
			"endpoint":"https://soap.example.com/service",
			"operation":"ListOrders",
			"body":"<m:ListOrders/>",
			"dataField":"soap:Envelope.soap:Body.ListOrdersResponse.Order",
			"retry":{"maxAttempts":3}
		},
		"filters":[{
			"type":"soap_call",
			"endpoint":"https://soap.example.com/enrich",
			"operation":"GetCustomer",
			"body":"<m:GetCustomer><m:Id>{{record.customerId}}</m:Id></m:GetCustomer>",
			"mergeStrategy":"append",
			"resultKey":"customer",
			"cache":{"enabled":true,"key":"{{record.customerId}}"}
		}],
		"output":{
			"type":"soapRequest",
			"endpoint":"https://soap.example.com/import",
			"soapVersion":"1.2",
			"soapAction":"urn:ImportOrders",
			"operation":"ImportOrders",
			"body":"<m:ImportOrders/>",
			"requestMode":"batch",
			"mtom":{"enabled":true,"attachments":[{"contentId":"{{record.id}}-pdf","contentType":"application/pdf","sourceField":"document","encoding":"base64"}]},
			"wsSecurity":{"username":"user","password":"pass","passwordType":"PasswordDigest","mustUnderstand":true}
		}
	}`
	if !validatePipelineJSON(t, raw) {
		t.Fatal("SOAP input/filter/output pipeline should validate")
	}
}

func TestSchemaSOAPMTOMAttachmentEncodingEnum(t *testing.T) {
	cases := []struct {
		name      string
		encoding  string
		wantValid bool
	}{
		{name: "binary", encoding: "binary", wantValid: true},
		{name: "base64", encoding: "base64", wantValid: true},
		{name: "invalid", encoding: "hex", wantValid: false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			output := fmt.Sprintf(
				`{"type":"soapRequest","endpoint":"https://e/x","operation":"Op","body":"<Op/>","mtom":{"enabled":true,"attachments":[{"contentId":"doc","contentType":"application/pdf","sourceField":"document","encoding":%q}]}}`,
				tc.encoding,
			)
			raw := fmt.Sprintf(`{"name":"x","version":"1.0.0","input":{"type":"webhook","path":"/in"},"output":%s}`, output)
			if got := validatePipelineJSON(t, raw); got != tc.wantValid {
				t.Fatalf("valid=%v want %v", got, tc.wantValid)
			}
		})
	}
}

func TestSchemaSOAPVersionAndSOAPActionHeaderRules(t *testing.T) {
	cases := []struct {
		name      string
		output    string
		wantValid bool
	}{
		{
			name:      "SOAP 1.1 allows SOAPAction HTTP header override",
			output:    `{"type":"soapRequest","endpoint":"https://e/x","operation":"Op","body":"<Op/>","soapVersion":"1.1","httpHeaders":{"SOAPAction":"legacy"}}`,
			wantValid: true,
		},
		{
			name:      "SOAP 1.2 rejects SOAPAction HTTP header",
			output:    `{"type":"soapRequest","endpoint":"https://e/x","operation":"Op","body":"<Op/>","soapVersion":"1.2","httpHeaders":{"SOAPAction":"legacy"}}`,
			wantValid: false,
		},
		{
			name:      "SOAP 1.2 rejects lowercase SOAPAction HTTP header",
			output:    `{"type":"soapRequest","endpoint":"https://e/x","operation":"Op","body":"<Op/>","soapVersion":"1.2","httpHeaders":{"soapaction":"legacy"}}`,
			wantValid: false,
		},
		{
			name:      "invalid version rejected",
			output:    `{"type":"soapRequest","endpoint":"https://e/x","operation":"Op","body":"<Op/>","soapVersion":"1.3"}`,
			wantValid: false,
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			raw := fmt.Sprintf(`{"name":"x","version":"1.0.0","input":{"type":"webhook","path":"/in"},"output":%s}`, tc.output)
			if got := validatePipelineJSON(t, raw); got != tc.wantValid {
				t.Fatalf("valid=%v want %v", got, tc.wantValid)
			}
		})
	}
}

func TestSchemaSOAPBaseRequiredFields(t *testing.T) {
	cases := []struct {
		name      string
		output    string
		wantValid bool
	}{
		{"all required", `{"type":"soapRequest","endpoint":"https://e/x","operation":"Op","body":"<Op/>"}`, true},
		{"templated endpoint", `{"type":"soapRequest","endpoint":"https://{{record.host}}/svc","operation":"Op","body":"<Op/>"}`, true},
		{"missing endpoint", `{"type":"soapRequest","operation":"Op","body":"<Op/>"}`, false},
		{"missing operation", `{"type":"soapRequest","endpoint":"https://e/x","body":"<Op/>"}`, false},
		{"missing body", `{"type":"soapRequest","endpoint":"https://e/x","operation":"Op"}`, false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			raw := fmt.Sprintf(`{"name":"x","version":"1.0.0","input":{"type":"webhook","path":"/in"},"output":%s}`, tc.output)
			if got := validatePipelineJSON(t, raw); got != tc.wantValid {
				t.Fatalf("valid=%v want %v", got, tc.wantValid)
			}
		})
	}
}

func TestSchemaSOAPWSSecurityPasswordTypeEnum(t *testing.T) {
	cases := []struct {
		name         string
		passwordType string
		wantValid    bool
	}{
		{"PasswordText", "PasswordText", true},
		{"PasswordDigest", "PasswordDigest", true},
		{"unknown", "Digest", false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			output := fmt.Sprintf(
				`{"type":"soapRequest","endpoint":"https://e/x","operation":"Op","body":"<Op/>","wsSecurity":{"username":"u","password":"p","passwordType":%q}}`,
				tc.passwordType,
			)
			raw := fmt.Sprintf(`{"name":"x","version":"1.0.0","input":{"type":"webhook","path":"/in"},"output":%s}`, output)
			if got := validatePipelineJSON(t, raw); got != tc.wantValid {
				t.Fatalf("valid=%v want %v", got, tc.wantValid)
			}
		})
	}
}

func TestSchemaSOAPCallAppendRequiresResultKey(t *testing.T) {
	cases := []struct {
		name      string
		filter    string
		wantValid bool
	}{
		{
			name:      "append with resultKey",
			filter:    `{"type":"soap_call","endpoint":"https://e/x","operation":"Op","body":"<Op/>","mergeStrategy":"append","resultKey":"soap"}`,
			wantValid: true,
		},
		{
			name:      "append without resultKey rejected",
			filter:    `{"type":"soap_call","endpoint":"https://e/x","operation":"Op","body":"<Op/>","mergeStrategy":"append"}`,
			wantValid: false,
		},
		{
			name:      "merge without resultKey",
			filter:    `{"type":"soap_call","endpoint":"https://e/x","operation":"Op","body":"<Op/>","mergeStrategy":"merge"}`,
			wantValid: true,
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := validateFilter(t, tc.filter); got != tc.wantValid {
				t.Fatalf("valid=%v want %v", got, tc.wantValid)
			}
		})
	}
}

func TestSOAPRetryDefaultsInherited(t *testing.T) {
	raw := `{
		"name":"soap",
		"version":"1.0.0",
		"defaults":{"retry":{"maxAttempts":5,"delayMs":250}},
		"input":{"type":"webhook","path":"/in"},
		"filters":[{"type":"soap_call","endpoint":"https://e/filter","operation":"F","body":"<F/>"}],
		"output":{"type":"soapRequest","endpoint":"https://e/output","operation":"O","body":"<O/>"}
	}`
	var data map[string]any
	if err := json.Unmarshal([]byte(raw), &data); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if res := config.ValidateConfig(data); !res.Valid {
		t.Fatalf("expected valid config, got: %v", res.Errors)
	}
	pipeline, err := config.ConvertToPipeline(data)
	if err != nil {
		t.Fatalf("ConvertToPipeline: %v", err)
	}
	for _, raw := range []json.RawMessage{pipeline.Filters[0].Raw, pipeline.Output.Raw} {
		var cfg map[string]any
		if err := json.Unmarshal(raw, &cfg); err != nil {
			t.Fatalf("unmarshal raw: %v", err)
		}
		retry, ok := cfg["retry"].(map[string]any)
		if !ok {
			t.Fatalf("expected retry in raw config: %v", cfg)
		}
		if retry["maxAttempts"] != float64(5) || retry["delayMs"] != float64(250) {
			t.Fatalf("unexpected retry defaults: %v", retry)
		}
	}
}
