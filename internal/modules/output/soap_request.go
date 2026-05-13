package output

import (
	"context"
	"fmt"
	"log/slog"
	"time"

	"github.com/expr-lang/expr"
	"github.com/expr-lang/expr/vm"

	"github.com/cannectors/runtime/internal/auth"
	"github.com/cannectors/runtime/internal/errhandling"
	"github.com/cannectors/runtime/internal/httpclient"
	"github.com/cannectors/runtime/internal/logger"
	"github.com/cannectors/runtime/internal/moduleconfig"
	"github.com/cannectors/runtime/internal/modules/soaputil"
	"github.com/cannectors/runtime/internal/soapclient"
	"github.com/cannectors/runtime/pkg/connector"
)

// SOAPRequestOutputConfig holds typed configuration for soapRequest output.
type SOAPRequestOutputConfig struct {
	connector.ModuleBase
	moduleconfig.SOAPRequestBase
	RequestMode string                  `json:"requestMode,omitempty"`
	Success     *SuccessConditionConfig `json:"success,omitempty"`
	Retry       *connector.RetryConfig  `json:"retry,omitempty"`
}

// SOAPRequestModule sends records to a SOAP endpoint.
type SOAPRequestModule struct {
	base              moduleconfig.SOAPRequestBase
	timeout           time.Duration
	requestMode       string
	retry             connector.RetryConfig
	authHandler       auth.Handler
	httpClient        *httpclient.Client
	soapClient        soapclient.SOAPClient
	onError           errhandling.OnErrorStrategy
	lastRetryInfo     *connector.RetryInfo
	successCodes      []int
	successProgram    *vm.Program
	successExpression string
}

// NewSOAPRequestFromConfig creates a SOAP output module from config.
func NewSOAPRequestFromConfig(config *connector.ModuleConfig) (*SOAPRequestModule, error) {
	if config == nil {
		return nil, ErrNilConfig
	}
	cfg, err := moduleconfig.ParseModuleConfig[SOAPRequestOutputConfig](*config)
	if err != nil {
		return nil, err
	}
	if baseErr := soaputil.ValidateBase(cfg.SOAPRequestBase); baseErr != nil {
		return nil, baseErr
	}
	requestMode, err := normalizeRequestMode(cfg.RequestMode)
	if err != nil {
		return nil, err
	}
	onError, err := errhandling.ParseOnErrorStrategy(cfg.OnError)
	if err != nil {
		return nil, err
	}
	successCodes, successProgram, successExpression, err := buildSuccessCondition(cfg.Success)
	if err != nil {
		return nil, err
	}
	retryConfig := soaputil.ToRetryConfig(cfg.Retry)
	if retryErr := retryConfig.Validate(); retryErr != nil {
		return nil, fmt.Errorf("soapRequest retry config invalid: %w", retryErr)
	}

	timeout := connector.GetTimeoutDuration(cfg.TimeoutMs, defaultHTTPTimeout)
	httpClient := httpclient.NewClient(timeout)
	authHandler, err := auth.NewHandler(cfg.Authentication, httpClient.Client)
	if err != nil {
		return nil, fmt.Errorf("creating soapRequest auth handler: %w", err)
	}

	module := &SOAPRequestModule{
		base:              cfg.SOAPRequestBase,
		timeout:           timeout,
		requestMode:       requestMode,
		retry:             retryConfig,
		authHandler:       authHandler,
		httpClient:        httpClient,
		soapClient:        soapclient.NewClient(httpClient),
		onError:           onError,
		successCodes:      successCodes,
		successProgram:    successProgram,
		successExpression: successExpression,
	}
	logger.Debug("soapRequest output module created",
		slog.String("endpoint", httpclient.SanitizeURL(cfg.Endpoint)),
		slog.String("operation", cfg.Operation),
		slog.String("request_mode", requestMode),
		slog.Duration("timeout", timeout),
		slog.Bool("has_auth", authHandler != nil),
	)
	return module, nil
}

// Send transmits records to the SOAP endpoint.
func (s *SOAPRequestModule) Send(ctx context.Context, records []map[string]any) (int, error) {
	if len(records) == 0 {
		return 0, nil
	}
	if s.requestMode == "single" {
		return s.sendSingle(ctx, records)
	}
	return s.sendBatch(ctx, records)
}

func (s *SOAPRequestModule) sendBatch(ctx context.Context, records []map[string]any) (int, error) {
	record := batchRecord(records)
	if err := s.sendRecord(ctx, record); err != nil {
		return 0, err
	}
	return len(records), nil
}

func (s *SOAPRequestModule) sendSingle(ctx context.Context, records []map[string]any) (int, error) {
	sent := 0
	for i, record := range records {
		select {
		case <-ctx.Done():
			return sent, ctx.Err()
		default:
		}
		err := s.sendRecord(ctx, record)
		if err != nil {
			switch s.onError {
			case errhandling.OnErrorFail:
				return sent, fmt.Errorf("soapRequest record %d: %w", i, err)
			case errhandling.OnErrorSkip, errhandling.OnErrorLog:
				logger.Error("soapRequest record failed",
					slog.Int("record_index", i),
					slog.String("operation", s.base.Operation),
					slog.String("error", err.Error()),
					slog.String("on_error", string(s.onError)),
				)
				continue
			}
		}
		sent++
	}
	return sent, nil
}

func (s *SOAPRequestModule) sendRecord(ctx context.Context, record map[string]any) error {
	op, err := soaputil.BuildOperation(soaputil.OperationOptions{
		Base:        s.base,
		Record:      record,
		AuthHandler: s.authHandler,
		Retry:       &s.retry,
	})
	if err != nil {
		return err
	}
	resp, err := s.soapClient.Call(ctx, op)
	s.lastRetryInfo = resp.RetryInfo
	if err != nil {
		return err
	}
	ok, err := s.isResponseSuccess(resp)
	if err != nil {
		return err
	}
	if !ok {
		return fmt.Errorf("soapRequest response status %d did not match success condition", resp.StatusCode)
	}
	return nil
}

func (s *SOAPRequestModule) isResponseSuccess(resp soapclient.SOAPResponse) (bool, error) {
	if len(s.successCodes) > 0 {
		matched := false
		for _, code := range s.successCodes {
			if resp.StatusCode == code {
				matched = true
				break
			}
		}
		if !matched {
			return false, nil
		}
	}
	if s.successProgram == nil {
		return true, nil
	}
	env := map[string]any{
		"statusCode": resp.StatusCode,
		"headers":    map[string][]string(resp.Headers),
		"body":       resp.Data,
	}
	result, err := expr.Run(s.successProgram, env)
	if err != nil {
		return false, fmt.Errorf("soapRequest success.expression evaluation failed: %w", err)
	}
	matched, ok := result.(bool)
	if !ok {
		return false, fmt.Errorf("soapRequest success.expression must return a boolean (got %T)", result)
	}
	return matched, nil
}

// GetRetryInfo is exposed for consistency with HTTP output modules. Retry
// execution lives inside soapclient and does not currently surface attempts.
func (s *SOAPRequestModule) GetRetryInfo() *connector.RetryInfo {
	return s.lastRetryInfo
}

// Close releases HTTP resources.
func (s *SOAPRequestModule) Close() error {
	s.httpClient.CloseIdleConnections()
	return nil
}

// PreviewRequest builds SOAP request previews without network I/O.
func (s *SOAPRequestModule) PreviewRequest(records []map[string]any, opts PreviewOptions) ([]RequestPreview, error) {
	if len(records) == 0 {
		return []RequestPreview{}, nil
	}
	if s.requestMode == "single" {
		previews := make([]RequestPreview, 0, len(records))
		for _, record := range records {
			preview, err := s.previewRecord(record, 1, opts)
			if err != nil {
				return nil, err
			}
			previews = append(previews, preview)
		}
		return previews, nil
	}
	preview, err := s.previewRecord(batchRecord(records), len(records), opts)
	if err != nil {
		return nil, err
	}
	return []RequestPreview{preview}, nil
}

func (s *SOAPRequestModule) previewRecord(record map[string]any, recordCount int, opts PreviewOptions) (RequestPreview, error) {
	op, err := soaputil.BuildOperation(soaputil.OperationOptions{
		Base:        s.base,
		Record:      record,
		AuthHandler: s.authHandler,
		Retry:       &s.retry,
	})
	if err != nil {
		return RequestPreview{}, err
	}
	envelope, err := soapclient.BuildEnvelope(soapclient.EnvelopeOptions{
		Version:    op.Version,
		Body:       op.Body,
		Record:     op.Record,
		Headers:    op.Headers,
		WSSecurity: op.WSSecurity,
	})
	if err != nil {
		return RequestPreview{}, err
	}
	headers, err := s.previewHeaders(op, opts)
	if err != nil {
		return RequestPreview{}, err
	}
	return RequestPreview{
		Endpoint:    op.Endpoint,
		Method:      "POST",
		Headers:     headers,
		BodyPreview: string(envelope),
		RecordCount: recordCount,
	}, nil
}

func (s *SOAPRequestModule) previewHeaders(op soapclient.SOAPOperation, opts PreviewOptions) (map[string]string, error) {
	contentType, err := soapclient.RequestContentType(op.Version, op.SOAPAction)
	if err != nil {
		return nil, err
	}
	headers := make(map[string]string, len(op.HTTPHeaders)+3)
	for k, v := range op.HTTPHeaders {
		headers[k] = v
	}
	headers["Content-Type"] = contentType
	headers["Accept"] = "text/xml, application/soap+xml, multipart/related"
	if op.SOAPAction != "" && op.Version != soapclient.SOAPVersion12 {
		headers["SOAPAction"] = op.SOAPAction
	}
	if s.authHandler != nil {
		if opts.ShowCredentials {
			headers["Authorization"] = "[PREVIEW-AUTH-UNAVAILABLE]"
		} else {
			headers["Authorization"] = "Bearer " + maskValue("auth")
		}
	}
	return headers, nil
}

func batchRecord(records []map[string]any) map[string]any {
	items := make([]any, 0, len(records))
	for _, record := range records {
		items = append(items, record)
	}
	return map[string]any{
		"records":     items,
		"recordCount": len(records),
	}
}

var _ PreviewableModule = (*SOAPRequestModule)(nil)
