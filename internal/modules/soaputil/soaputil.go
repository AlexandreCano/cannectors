// Package soaputil contains shared runtime wiring for SOAP modules.
package soaputil

import (
	"encoding/base64"
	"errors"
	"fmt"
	"net/url"
	"strconv"
	"strings"

	"github.com/cannectors/runtime/internal/auth"
	"github.com/cannectors/runtime/internal/httpclient"
	"github.com/cannectors/runtime/internal/moduleconfig"
	"github.com/cannectors/runtime/internal/recordpath"
	"github.com/cannectors/runtime/internal/soapclient"
	"github.com/cannectors/runtime/internal/template"
	"github.com/cannectors/runtime/pkg/connector"
)

// ErrInvalidDataField is returned when a SOAP response dataField cannot be resolved.
var ErrInvalidDataField = errors.New("invalid SOAP dataField")

// ValidateBase validates static SOAP request configuration shared by modules.
func ValidateBase(base moduleconfig.SOAPRequestBase) error {
	if strings.TrimSpace(base.Endpoint) == "" {
		return fmt.Errorf("SOAP endpoint is required")
	}
	if strings.TrimSpace(base.Operation) == "" {
		return fmt.Errorf("SOAP operation is required")
	}
	if strings.TrimSpace(base.Body) == "" {
		return fmt.Errorf("SOAP body is required")
	}
	if _, err := soapclient.NormalizeSOAPVersion(soapclient.SOAPVersion(base.SOAPVersion)); err != nil {
		return err
	}
	if err := template.ValidateSyntax(base.Endpoint); err != nil {
		return fmt.Errorf("invalid SOAP endpoint template: %w", err)
	}
	if err := template.ValidateSyntax(base.Body); err != nil {
		return fmt.Errorf("invalid SOAP body template: %w", err)
	}
	for i, header := range base.Headers {
		if err := template.ValidateSyntax(header.XML); err != nil {
			return fmt.Errorf("invalid SOAP header template %d: %w", i, err)
		}
	}
	for name, value := range base.HTTPHeaders {
		if err := httpclient.ValidateHeaderName(name); err != nil {
			return fmt.Errorf("invalid SOAP HTTP header: %w", err)
		}
		if err := template.ValidateSyntax(value); err != nil {
			return fmt.Errorf("invalid SOAP HTTP header template %q: %w", name, err)
		}
	}
	if base.MTOM.Enabled {
		for i, attachment := range base.MTOM.Attachments {
			if strings.TrimSpace(attachment.ContentID) == "" {
				return fmt.Errorf("SOAP MTOM attachment %d contentId is required", i)
			}
			if strings.TrimSpace(attachment.ContentType) == "" {
				return fmt.Errorf("SOAP MTOM attachment %d contentType is required", i)
			}
			if strings.TrimSpace(attachment.SourceField) == "" {
				return fmt.Errorf("SOAP MTOM attachment %d sourceField is required", i)
			}
			if attachment.Encoding != "" && attachment.Encoding != "binary" && attachment.Encoding != "base64" {
				return fmt.Errorf("SOAP MTOM attachment %d encoding must be 'binary' or 'base64'", i)
			}
			if err := template.ValidateSyntax(attachment.ContentID); err != nil {
				return fmt.Errorf("invalid SOAP MTOM attachment %d contentId template: %w", i, err)
			}
		}
	}
	return nil
}

// ToRetryConfig applies SOAP module retry defaults while preserving an
// explicit maxAttempts: 0 as the protocol-level "retry disabled" signal.
func ToRetryConfig(rc *connector.RetryConfig) connector.RetryConfig {
	if rc == nil {
		return connector.DefaultRetryConfig()
	}
	cfg := moduleconfig.ToRetryConfig(rc)
	cfg.MaxAttempts = rc.MaxAttempts
	return cfg
}

// OperationOptions contains per-call values added to the shared SOAP base.
type OperationOptions struct {
	Base        moduleconfig.SOAPRequestBase
	Record      map[string]any
	AuthHandler auth.Handler
	Retry       *connector.RetryConfig
	Endpoint    string
	HTTPHeaders map[string]string
}

// BuildOperation converts module configuration into a soapclient operation.
func BuildOperation(opts OperationOptions) (soapclient.SOAPOperation, error) {
	record := opts.Record
	if record == nil {
		record = map[string]any{}
	}

	evaluator := template.NewEvaluator()
	endpoint := opts.Endpoint
	if endpoint == "" {
		endpoint = evaluator.EvaluateForURL(opts.Base.Endpoint, record)
	}
	if err := httpclient.ValidateAbsoluteURL(endpoint); err != nil {
		return soapclient.SOAPOperation{}, fmt.Errorf("invalid SOAP endpoint: %w", err)
	}

	headers := make(map[string]string, len(opts.Base.HTTPHeaders)+len(opts.HTTPHeaders))
	for name, value := range opts.Base.HTTPHeaders {
		if err := httpclient.ValidateHeaderName(name); err != nil {
			return soapclient.SOAPOperation{}, fmt.Errorf("invalid SOAP HTTP header: %w", err)
		}
		evaluated := evaluator.Evaluate(value, record)
		if err := httpclient.ValidateHeaderValue(evaluated); err != nil {
			return soapclient.SOAPOperation{}, fmt.Errorf("invalid SOAP HTTP header %q: %w", name, err)
		}
		headers[name] = evaluated
	}
	for name, value := range opts.HTTPHeaders {
		if err := httpclient.ValidateHeaderName(name); err != nil {
			return soapclient.SOAPOperation{}, fmt.Errorf("invalid SOAP HTTP header: %w", err)
		}
		if err := httpclient.ValidateHeaderValue(value); err != nil {
			return soapclient.SOAPOperation{}, fmt.Errorf("invalid SOAP HTTP header %q: %w", name, err)
		}
		headers[name] = value
	}

	soapHeaders := make([]soapclient.SOAPHeaderTemplate, 0, len(opts.Base.Headers))
	for _, header := range opts.Base.Headers {
		soapHeaders = append(soapHeaders, soapclient.SOAPHeaderTemplate{XML: header.XML})
	}

	mtom, err := BuildMTOMConfig(opts.Base.MTOM, record)
	if err != nil {
		return soapclient.SOAPOperation{}, err
	}

	return soapclient.SOAPOperation{
		Endpoint:       endpoint,
		SOAPAction:     opts.Base.SOAPAction,
		Version:        soapclient.SOAPVersion(opts.Base.SOAPVersion),
		Body:           opts.Base.Body,
		Record:         record,
		Headers:        soapHeaders,
		HTTPHeaders:    headers,
		Authentication: opts.Base.Authentication,
		AuthHandler:    opts.AuthHandler,
		WSSecurity:     BuildWSSecurityConfig(opts.Base.WSSecurity),
		MTOM:           mtom,
		Retry:          opts.Retry,
	}, nil
}

// BuildWSSecurityConfig converts config-side WS-Security settings.
func BuildWSSecurityConfig(cfg *moduleconfig.SOAPSecurityConfig) *soapclient.WSSecurityConfig {
	if cfg == nil {
		return nil
	}
	passwordType := soapclient.WSSPasswordType(cfg.PasswordType)
	if passwordType == "" {
		passwordType = soapclient.WSSPasswordText
	}
	return &soapclient.WSSecurityConfig{
		Username:       cfg.Username,
		Password:       cfg.Password,
		PasswordType:   passwordType,
		TokenID:        cfg.TokenID,
		MustUnderstand: cfg.MustUnderstand,
	}
}

// BuildMTOMConfig resolves configured attachments against a record.
func BuildMTOMConfig(cfg moduleconfig.MTOMTemplateConfig, record map[string]any) (soapclient.MTOMConfig, error) {
	if !cfg.Enabled {
		return soapclient.MTOMConfig{}, nil
	}
	evaluator := template.NewEvaluator()
	attachments := make([]soapclient.MTOMAttachment, 0, len(cfg.Attachments))
	for i, attachment := range cfg.Attachments {
		value, found := recordpath.Get(record, attachment.SourceField)
		if !found || value == nil {
			return soapclient.MTOMConfig{}, fmt.Errorf("SOAP MTOM attachment %d sourceField %q not found", i, attachment.SourceField)
		}
		data, err := attachmentBytes(value, attachment.Encoding)
		if err != nil {
			return soapclient.MTOMConfig{}, fmt.Errorf("SOAP MTOM attachment %d sourceField %q: %w", i, attachment.SourceField, err)
		}
		attachments = append(attachments, soapclient.MTOMAttachment{
			ContentID:   evaluator.Evaluate(attachment.ContentID, record),
			ContentType: attachment.ContentType,
			Data:        data,
		})
	}
	return soapclient.MTOMConfig{Enabled: true, Attachments: attachments}, nil
}

func attachmentBytes(value any, encoding string) ([]byte, error) {
	if encoding == "" {
		encoding = "binary"
	}
	switch v := value.(type) {
	case []byte:
		if encoding == "base64" {
			return decodeBase64(string(v))
		}
		return v, nil
	case string:
		if encoding == "base64" {
			return decodeBase64(v)
		}
		return []byte(v), nil
	default:
		return nil, fmt.Errorf("expected []byte or string, got %T", value)
	}
}

func decodeBase64(value string) ([]byte, error) {
	trimmed := strings.TrimSpace(value)
	data, err := base64.StdEncoding.DecodeString(trimmed)
	if err == nil {
		return data, nil
	}
	rawData, rawErr := base64.RawStdEncoding.DecodeString(trimmed)
	if rawErr == nil {
		return rawData, nil
	}
	return nil, fmt.Errorf("decoding base64 attachment: %w", err)
}

// ExtractDataField returns the configured sub-tree, or the full data map when field is empty.
func ExtractDataField(data map[string]any, field string) (any, error) {
	if field == "" {
		return data, nil
	}
	value, found := recordpath.Get(data, field)
	if !found {
		return nil, fmt.Errorf("%w: field %q not found", ErrInvalidDataField, field)
	}
	return value, nil
}

// ExtractIntField reads an integer value at the given record path. Returns 0
// when the field is empty, missing, or holds an unconvertible value.
func ExtractIntField(data map[string]any, field string) int {
	if field == "" {
		return 0
	}
	value, found := recordpath.Get(data, field)
	if !found {
		return 0
	}
	switch v := value.(type) {
	case int:
		return v
	case int64:
		return int(v)
	case float64:
		return int(v)
	case string:
		i, _ := strconv.Atoi(v)
		return i
	default:
		return 0
	}
}

// ExtractStringField reads a string value at the given record path. Returns
// an empty string when the field is empty, missing, or holds a nil value.
func ExtractStringField(data map[string]any, field string) string {
	if field == "" {
		return ""
	}
	value, found := recordpath.Get(data, field)
	if !found || value == nil {
		return ""
	}
	return fmt.Sprintf("%v", value)
}

// ValueAsRecordMap converts SOAP response data into a mergeable record map.
func ValueAsRecordMap(value any) (map[string]any, error) {
	switch v := value.(type) {
	case map[string]any:
		return v, nil
	case []any:
		if len(v) == 1 {
			if m, ok := v[0].(map[string]any); ok {
				return m, nil
			}
		}
		return map[string]any{"items": v}, nil
	default:
		return map[string]any{"value": v}, nil
	}
}

// ValueAsRecords converts a SOAP response sub-tree into input records.
func ValueAsRecords(value any) ([]map[string]any, error) {
	switch v := value.(type) {
	case []map[string]any:
		return v, nil
	case []any:
		records := make([]map[string]any, 0, len(v))
		for i, item := range v {
			record, ok := item.(map[string]any)
			if !ok {
				return nil, fmt.Errorf("%w: item %d is %T", ErrInvalidDataField, i, item)
			}
			records = append(records, record)
		}
		return records, nil
	case map[string]any:
		return []map[string]any{v}, nil
	default:
		return nil, fmt.Errorf("%w: expected array or object, got %T", ErrInvalidDataField, value)
	}
}

// BuildEndpointWithKeys applies http_call-style path and query key values.
func BuildEndpointWithKeys(endpoint string, record map[string]any, keys []moduleconfig.KeyConfig, values map[string]string) (string, error) {
	evaluator := template.NewEvaluator()
	resolved := evaluator.EvaluateForURL(endpoint, record)
	for _, key := range keys {
		if key.ParamType == "path" {
			resolved = strings.Replace(resolved, "{"+key.ParamName+"}", url.PathEscape(values[key.ParamName]), 1)
		}
	}
	parsedURL, err := url.Parse(resolved)
	if err != nil {
		return "", fmt.Errorf("parsing SOAP endpoint URL: %w", err)
	}
	q := parsedURL.Query()
	modified := false
	for _, key := range keys {
		if key.ParamType == "query" {
			q.Set(key.ParamName, values[key.ParamName])
			modified = true
		}
	}
	if modified {
		parsedURL.RawQuery = q.Encode()
	}
	return parsedURL.String(), nil
}

// BuildHeaderOverridesWithKeys returns headers populated from key values.
func BuildHeaderOverridesWithKeys(keys []moduleconfig.KeyConfig, values map[string]string) map[string]string {
	headers := make(map[string]string)
	for _, key := range keys {
		if key.ParamType == "header" && values[key.ParamName] != "" {
			headers[key.ParamName] = values[key.ParamName]
		}
	}
	return headers
}

// ExtractKeyValues reads configured key values from a record.
func ExtractKeyValues(record map[string]any, keys []moduleconfig.KeyConfig) (map[string]string, error) {
	values := make(map[string]string, len(keys))
	for _, key := range keys {
		value, found := recordpath.Get(record, key.Field)
		if !found || value == nil {
			return nil, fmt.Errorf("field %q is missing", key.Field)
		}
		text := template.ValueToString(value)
		if text == "" {
			return nil, fmt.Errorf("field %q is empty", key.Field)
		}
		values[key.ParamName] = text
	}
	return values, nil
}
