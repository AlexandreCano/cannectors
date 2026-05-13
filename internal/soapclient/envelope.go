package soapclient

import (
	"bytes"
	"encoding/xml"
	"fmt"
	"strings"

	"github.com/cannectors/runtime/internal/recordpath"
	recordtemplate "github.com/cannectors/runtime/internal/template"
)

// SOAPHeaderTemplate is a raw XML SOAP header fragment with optional record templating.
type SOAPHeaderTemplate struct {
	XML string
}

// EnvelopeOptions configures SOAP envelope construction.
type EnvelopeOptions struct {
	Version    SOAPVersion
	Body       string
	Record     map[string]any
	Headers    []SOAPHeaderTemplate
	WSSecurity *WSSecurityConfig
}

// BuildEnvelope wraps raw body XML in a SOAP envelope and escapes record substitutions.
// Client.Call delegates envelope emission to hooklift; this helper remains for
// low-level tests and callers that need to inspect an envelope without sending it.
func BuildEnvelope(opts EnvelopeOptions) ([]byte, error) {
	namespace, err := EnvelopeNamespace(opts.Version)
	if err != nil {
		return nil, err
	}
	if strings.TrimSpace(opts.Body) == "" {
		return nil, fmt.Errorf("SOAP body is required")
	}

	body, err := EvaluateXMLTemplate(opts.Body, opts.Record)
	if err != nil {
		return nil, err
	}

	var headers []string
	for i, h := range opts.Headers {
		if strings.TrimSpace(h.XML) == "" {
			continue
		}
		evaluated, evalErr := EvaluateXMLTemplate(h.XML, opts.Record)
		if evalErr != nil {
			return nil, fmt.Errorf("evaluating SOAP header %d: %w", i, evalErr)
		}
		headers = append(headers, evaluated)
	}
	if opts.WSSecurity != nil {
		header, secErr := BuildWSSecurityHeader(*opts.WSSecurity)
		if secErr != nil {
			return nil, secErr
		}
		headers = append(headers, string(header))
	}

	var b strings.Builder
	b.WriteString(`<soap:Envelope xmlns:soap="`)
	b.WriteString(namespace)
	b.WriteString(`">`)
	if len(headers) > 0 {
		b.WriteString(`<soap:Header>`)
		for _, header := range headers {
			b.WriteString(header)
		}
		b.WriteString(`</soap:Header>`)
	}
	b.WriteString(`<soap:Body>`)
	b.WriteString(body)
	b.WriteString(`</soap:Body></soap:Envelope>`)
	return []byte(b.String()), nil
}

// EvaluateXMLTemplate evaluates record templates and XML-escapes substituted values.
func EvaluateXMLTemplate(raw string, record map[string]any) (string, error) {
	if !recordtemplate.HasVariables(raw) {
		return raw, nil
	}
	evaluator := recordtemplate.NewEvaluator()
	variables := evaluator.ParseVariables(raw)
	result := raw
	for _, variable := range variables {
		value, err := resolveTemplateVariable(variable, record)
		if err != nil {
			return "", err
		}
		result = strings.Replace(result, variable.FullMatch, escapeXMLText(value), 1)
	}
	return result, nil
}

func resolveTemplateVariable(variable recordtemplate.Variable, record map[string]any) (string, error) {
	path := strings.TrimPrefix(variable.Path, "record.")
	value, found := recordpath.Get(record, path)
	if !found || value == nil {
		if variable.HasDefault {
			return variable.DefaultValue, nil
		}
		return "", fmt.Errorf("template variable %q is missing and has no default", variable.Path)
	}
	return recordtemplate.ValueToString(value), nil
}

func escapeXMLText(value string) string {
	var b bytes.Buffer
	_ = xml.EscapeText(&b, []byte(value))
	return b.String()
}
