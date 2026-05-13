package soapclient

import (
	"bytes"
	"context"
	"encoding/xml"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"

	"github.com/cannectors/runtime/internal/auth"
	"github.com/cannectors/runtime/internal/httpclient"
	"github.com/cannectors/runtime/pkg/connector"
	hooksoap "github.com/cannectors/runtime/third_party/hooklift_soap"
)

// SOAPClient is the interface implemented by SOAP callers.
type SOAPClient interface {
	Call(ctx context.Context, op SOAPOperation) (SOAPResponse, error)
}

// SOAPOperation contains one raw XML SOAP request.
type SOAPOperation struct {
	Endpoint       string
	SOAPAction     string
	Version        SOAPVersion
	Body           string
	Record         map[string]any
	Headers        []SOAPHeaderTemplate
	HTTPHeaders    map[string]string
	Authentication *connector.AuthConfig
	// AuthHandler lets long-lived modules reuse an auth handler across calls.
	// Phase 3 SOAP modules should set this for OAuth2 token cache reuse.
	AuthHandler auth.Handler
	WSSecurity  *WSSecurityConfig
	MTOM        MTOMConfig
	Retry       *connector.RetryConfig
}

// Client sends SOAP requests through the shared Cannectors HTTP client.
type Client struct {
	httpClient *httpclient.Client
}

// NewClient creates a SOAP client backed by the provided HTTP client.
func NewClient(client *httpclient.Client) *Client {
	if client == nil {
		client = httpclient.NewClient(30 * time.Second)
	}
	return &Client{httpClient: client}
}

// Call builds, sends, and parses a SOAP operation.
func (c *Client) Call(ctx context.Context, op SOAPOperation) (SOAPResponse, error) {
	if err := httpclient.ValidateAbsoluteURL(op.Endpoint); err != nil {
		return SOAPResponse{}, fmt.Errorf("invalid SOAP endpoint: %w", err)
	}
	version, err := NormalizeSOAPVersion(op.Version)
	if err != nil {
		return SOAPResponse{}, err
	}
	body, err := EvaluateXMLTemplate(op.Body, op.Record)
	if err != nil {
		return SOAPResponse{}, err
	}
	if strings.TrimSpace(body) == "" {
		return SOAPResponse{}, fmt.Errorf("SOAP body is required")
	}

	httpHeaders := map[string]string{
		"Accept": "text/xml, application/soap+xml, multipart/related",
	}
	for name, value := range op.HTTPHeaders {
		if validateErr := httpclient.ValidateHeaderName(name); validateErr != nil {
			return SOAPResponse{}, fmt.Errorf("invalid SOAP HTTP header: %w", validateErr)
		}
		if validateErr := httpclient.ValidateHeaderValue(value); validateErr != nil {
			return SOAPResponse{}, fmt.Errorf("invalid SOAP HTTP header %q: %w", name, validateErr)
		}
		httpHeaders[name] = value
	}
	authHandler := op.AuthHandler
	if authHandler == nil && op.Authentication != nil {
		// TODO(phase-3): SOAP modules should build this once per module so
		// OAuth2 token caches survive across records.
		handler, authErr := auth.NewHandler(op.Authentication, c.httpClient.Client)
		if authErr != nil {
			return SOAPResponse{}, fmt.Errorf("creating SOAP HTTP auth handler: %w", authErr)
		}
		authHandler = handler
	}

	capturingClient := &capturingHTTPClient{
		base:        c.httpClient,
		authHandler: authHandler,
		retry:       op.Retry,
	}
	opts := []hooksoap.Option{
		hooksoap.WithHTTPClient(capturingClient),
		hooksoap.WithSOAPVersion(toHookSOAPVersion(version)),
		hooksoap.WithHTTPHeaders(httpHeaders),
	}
	if op.MTOM.Enabled {
		opts = append(opts, hooksoap.WithMTOM())
	}

	hookClient := hooksoap.NewClient(op.Endpoint, opts...)
	for _, header := range op.Headers {
		if strings.TrimSpace(header.XML) == "" {
			continue
		}
		evaluated, evalErr := EvaluateXMLTemplate(header.XML, op.Record)
		if evalErr != nil {
			return SOAPResponse{}, fmt.Errorf("evaluating SOAP header: %w", evalErr)
		}
		hookClient.AddHeader(rawXMLFragment(evaluated))
	}
	if op.WSSecurity != nil {
		header, secErr := BuildWSSecurityHeader(*op.WSSecurity)
		if secErr != nil {
			return SOAPResponse{}, secErr
		}
		hookClient.AddHeader(rawXMLFragment(header))
	}
	if op.MTOM.Enabled {
		for _, attachment := range op.MTOM.Attachments {
			hookClient.AddMIMEMultipartAttachment(hooksoap.MIMEMultipartAttachment{
				Name:        strings.Trim(attachment.ContentID, "<>"),
				ContentType: attachment.ContentType,
				Data:        attachment.Data,
			})
		}
	}

	var response rawXMLSink
	callErr := hookClient.CallContext(ctx, op.SOAPAction, rawXMLFragment(body), &response)
	captured := capturingClient.response
	if captured == nil {
		if callErr != nil {
			return SOAPResponse{}, fmt.Errorf("executing SOAP request: %w", callErr)
		}
		return SOAPResponse{}, fmt.Errorf("executing SOAP request: missing HTTP response")
	}

	xmlBody, attachments, _, err := ParseMTOMResponse(captured.Header.Get("Content-Type"), bytes.NewReader(captured.Body))
	if err != nil {
		return SOAPResponse{}, fmt.Errorf("reading SOAP response: %w", err)
	}

	soapResp := SOAPResponse{
		StatusCode:  captured.StatusCode,
		Headers:     captured.Header.Clone(),
		EnvelopeXML: xmlBody,
		Attachments: attachments,
	}

	fault, faultErr := ParseSOAPFault(xmlBody)
	if faultErr != nil {
		if captured.StatusCode >= 400 {
			fault = nil
		} else {
			return soapResp, fmt.Errorf("parsing SOAP fault: %w", faultErr)
		}
	}
	if faultErr == nil && fault != nil {
		fault.StatusCode = captured.StatusCode
		return soapResp, fault
	}
	if captured.StatusCode >= 400 {
		body, _ := io.ReadAll(io.LimitReader(bytes.NewReader(xmlBody), 4096))
		return soapResp, &httpclient.Error{
			StatusCode:      captured.StatusCode,
			Status:          captured.Status,
			Endpoint:        op.Endpoint,
			Method:          http.MethodPost,
			Message:         captured.Status,
			ResponseBody:    string(body),
			ResponseHeaders: captured.Header.Clone(),
		}
	}

	data, err := ParseXMLResponse(xmlBody)
	if err != nil {
		if callErr != nil {
			return soapResp, fmt.Errorf("executing SOAP request: %w", callErr)
		}
		return soapResp, err
	}
	soapResp.Data = data
	return soapResp, nil
}

type capturedHTTPResponse struct {
	StatusCode int
	Status     string
	Header     http.Header
	Body       []byte
}

type capturingHTTPClient struct {
	base        *httpclient.Client
	authHandler auth.Handler
	retry       *connector.RetryConfig
	response    *capturedHTTPResponse
}

func (c *capturingHTTPClient) Do(req *http.Request) (*http.Response, error) {
	if c.authHandler != nil {
		if err := c.authHandler.ApplyAuth(req.Context(), req); err != nil {
			return nil, fmt.Errorf("applying SOAP HTTP auth: %w", err)
		}
	}

	resp, err := c.do(req)
	if resp != nil {
		c.capture(resp)
	}
	return resp, err
}

func (c *capturingHTTPClient) do(req *http.Request) (*http.Response, error) {
	if c.retry != nil {
		return c.base.DoWithRetry(req.Context(), req, *c.retry, httpclient.RetryHooks{})
	}
	return c.base.Do(req)
}

func (c *capturingHTTPClient) capture(resp *http.Response) {
	body, readErr := io.ReadAll(resp.Body)
	_ = resp.Body.Close()
	if readErr != nil {
		resp.Body = io.NopCloser(bytes.NewReader(nil))
		return
	}
	resp.Body = io.NopCloser(bytes.NewReader(body))
	c.response = &capturedHTTPResponse{
		StatusCode: resp.StatusCode,
		Status:     resp.Status,
		Header:     resp.Header.Clone(),
		Body:       body,
	}
}

type rawXMLFragment string

func (r rawXMLFragment) MarshalXML(enc *xml.Encoder, _ xml.StartElement) error {
	dec := xml.NewDecoder(strings.NewReader(string(r)))
	for {
		token, err := dec.Token()
		if errors.Is(err, io.EOF) {
			return enc.Flush()
		}
		if err != nil {
			return err
		}
		if start, ok := token.(xml.StartElement); ok {
			token = withoutNamespaceDeclarationAttrs(start)
		}
		if err := enc.EncodeToken(token); err != nil {
			return err
		}
	}
}

// rawXMLSink is a no-op receiver for hooklift; parsing uses captured HTTP bytes.
type rawXMLSink struct {
	Inner string `xml:",innerxml"`
}

func toHookSOAPVersion(version SOAPVersion) hooksoap.SOAPVersion {
	if version == SOAPVersion12 {
		return hooksoap.SOAP12
	}
	return hooksoap.SOAP11
}

func withoutNamespaceDeclarationAttrs(start xml.StartElement) xml.StartElement {
	// Drop xmlns declarations so raw fragments do not conflict with hooklift's envelope bindings.
	if len(start.Attr) == 0 {
		return start
	}
	attrs := start.Attr[:0]
	for _, attr := range start.Attr {
		if attr.Name.Local == "xmlns" || attr.Name.Space == "xmlns" {
			continue
		}
		attrs = append(attrs, attr)
	}
	start.Attr = attrs
	return start
}
