package input

import (
	"context"
	"fmt"
	"log/slog"
	"time"

	"github.com/cannectors/runtime/internal/auth"
	"github.com/cannectors/runtime/internal/httpclient"
	"github.com/cannectors/runtime/internal/logger"
	"github.com/cannectors/runtime/internal/moduleconfig"
	"github.com/cannectors/runtime/internal/modules/soaputil"
	"github.com/cannectors/runtime/internal/persistence"
	"github.com/cannectors/runtime/internal/soapclient"
	"github.com/cannectors/runtime/pkg/connector"
)

// SOAPPollingInputConfig holds typed configuration for the soapPolling module.
type SOAPPollingInputConfig struct {
	connector.ModuleBase
	moduleconfig.SOAPRequestBase
	DataField        string                              `json:"dataField,omitempty"`
	Pagination       *moduleconfig.PaginationConfig      `json:"pagination,omitempty"`
	Retry            *connector.RetryConfig              `json:"retry,omitempty"`
	StatePersistence *persistence.StatePersistenceConfig `json:"statePersistence,omitempty"`
}

// SOAPPolling implements polling-based SOAP input fetching.
type SOAPPolling struct {
	base              moduleconfig.SOAPRequestBase
	timeout           time.Duration
	dataField         string
	pagination        *moduleconfig.PaginationConfig
	httpClient        *httpclient.Client
	soapClient        soapclient.SOAPClient
	authHandler       auth.Handler
	retryConfig       connector.RetryConfig
	lastRetryInfo     *connector.RetryInfo
	persistenceConfig *persistence.StatePersistenceConfig
	stateStore        *persistence.StateStore
	pipelineID        string
	lastState         *persistence.State
}

// NewSOAPPollingFromConfig creates a soapPolling input module from config.
func NewSOAPPollingFromConfig(config *connector.ModuleConfig) (*SOAPPolling, error) {
	if config == nil {
		return nil, ErrNilConfig
	}
	cfg, err := moduleconfig.ParseModuleConfig[SOAPPollingInputConfig](*config)
	if err != nil {
		return nil, err
	}
	if baseErr := soaputil.ValidateBase(cfg.SOAPRequestBase); baseErr != nil {
		return nil, baseErr
	}
	if paginationErr := cfg.Pagination.Validate(); paginationErr != nil {
		return nil, fmt.Errorf("soapPolling pagination invalid: %w", paginationErr)
	}

	retryConfig := soaputil.ToRetryConfig(cfg.Retry)
	if retryErr := retryConfig.Validate(); retryErr != nil {
		return nil, fmt.Errorf("soapPolling retry config invalid: %w", retryErr)
	}

	timeout := connector.GetTimeoutDuration(cfg.TimeoutMs, defaultTimeout)
	httpClient := httpclient.NewClient(timeout)
	authHandler, err := auth.NewHandler(cfg.Authentication, httpClient.Client)
	if err != nil {
		return nil, fmt.Errorf("creating soapPolling auth handler: %w", err)
	}

	module := &SOAPPolling{
		base:              cfg.SOAPRequestBase,
		timeout:           timeout,
		dataField:         cfg.DataField,
		pagination:        cfg.Pagination,
		httpClient:        httpClient,
		soapClient:        soapclient.NewClient(httpClient),
		authHandler:       authHandler,
		retryConfig:       retryConfig,
		persistenceConfig: cfg.StatePersistence,
	}
	if cfg.StatePersistence != nil && cfg.StatePersistence.IsEnabled() {
		storagePath := cfg.StatePersistence.StoragePath
		if storagePath == "" {
			storagePath = persistence.DefaultStatePath
		}
		module.stateStore = persistence.NewStateStore(storagePath)
	}

	logger.Debug("soapPolling input module created",
		slog.String("endpoint", httpclient.SanitizeURL(cfg.Endpoint)),
		slog.String("operation", cfg.Operation),
		slog.Duration("timeout", timeout),
		slog.Bool("has_auth", authHandler != nil),
		slog.Bool("has_pagination", cfg.Pagination != nil),
	)
	return module, nil
}

// Fetch retrieves records from a SOAP endpoint.
func (s *SOAPPolling) Fetch(ctx context.Context) ([]map[string]any, error) {
	start := time.Now()
	logger.Info("input fetch started",
		slog.String("module_type", "soapPolling"),
		slog.String("endpoint", httpclient.SanitizeURL(s.base.Endpoint)),
		slog.String("operation", s.base.Operation),
		slog.Bool("has_pagination", s.pagination != nil),
	)

	var (
		records []map[string]any
		err     error
	)
	if s.pagination == nil {
		records, err = s.fetchSingle(ctx, s.stateRecord(nil))
	} else {
		records, err = s.fetchWithPagination(ctx)
	}
	if err != nil {
		logger.Error("input fetch failed",
			slog.String("module_type", "soapPolling"),
			slog.String("operation", s.base.Operation),
			slog.Duration("duration", time.Since(start)),
			slog.String("error", err.Error()),
		)
		return nil, err
	}

	logger.Info("input fetch completed",
		slog.String("module_type", "soapPolling"),
		slog.String("operation", s.base.Operation),
		slog.Int("record_count", len(records)),
		slog.Duration("duration", time.Since(start)),
	)
	return records, nil
}

func (s *SOAPPolling) fetchSingle(ctx context.Context, record map[string]any) ([]map[string]any, error) {
	resp, err := s.call(ctx, record)
	if err != nil {
		return nil, err
	}
	return s.recordsFromResponse(resp.Data)
}

func (s *SOAPPolling) call(ctx context.Context, record map[string]any) (soapclient.SOAPResponse, error) {
	op, err := soaputil.BuildOperation(soaputil.OperationOptions{
		Base:        s.base,
		Record:      record,
		AuthHandler: s.authHandler,
		Retry:       &s.retryConfig,
	})
	if err != nil {
		return soapclient.SOAPResponse{}, err
	}
	resp, err := s.soapClient.Call(ctx, op)
	s.lastRetryInfo = resp.RetryInfo
	return resp, err
}

func (s *SOAPPolling) recordsFromResponse(data map[string]any) ([]map[string]any, error) {
	value, err := soaputil.ExtractDataField(data, s.dataField)
	if err != nil {
		return nil, err
	}
	return soaputil.ValueAsRecords(value)
}

func (s *SOAPPolling) fetchWithPagination(ctx context.Context) ([]map[string]any, error) {
	switch s.pagination.Type {
	case "page":
		return s.fetchPageBased(ctx)
	case "offset":
		return s.fetchOffsetBased(ctx)
	case "cursor":
		return s.fetchCursorBased(ctx)
	default:
		return nil, fmt.Errorf("unknown pagination type %q (expected 'cursor', 'page' or 'offset')", s.pagination.Type)
	}
}

func (s *SOAPPolling) fetchPageBased(ctx context.Context) ([]map[string]any, error) {
	var all []map[string]any
	for page := 1; page <= maxPaginationPages; page++ {
		record := s.stateRecord(map[string]any{
			"type": s.pagination.Type,
			"page": page,
		})
		record[s.pagination.Param] = page
		resp, err := s.call(ctx, record)
		if err != nil {
			return nil, err
		}
		records, err := s.recordsFromResponse(resp.Data)
		if err != nil {
			return nil, err
		}
		all = append(all, records...)
		totalPages := soaputil.ExtractIntField(resp.Data, s.pagination.TotalPagesField)
		if totalPages > 0 && page >= totalPages {
			break
		}
		if len(records) == 0 {
			break
		}
	}
	return all, nil
}

func (s *SOAPPolling) fetchOffsetBased(ctx context.Context) ([]map[string]any, error) {
	var all []map[string]any
	limit := s.pagination.Limit
	if limit == 0 {
		limit = 100
	}
	for offset := 0; offset < maxPaginationPages*limit; offset += limit {
		record := s.stateRecord(map[string]any{
			"type":   s.pagination.Type,
			"offset": offset,
			"limit":  limit,
		})
		record[s.pagination.Param] = offset
		record[s.pagination.LimitParam] = limit
		resp, err := s.call(ctx, record)
		if err != nil {
			return nil, err
		}
		records, err := s.recordsFromResponse(resp.Data)
		if err != nil {
			return nil, err
		}
		all = append(all, records...)
		total := soaputil.ExtractIntField(resp.Data, s.pagination.TotalField)
		if total > 0 && len(all) >= total {
			break
		}
		if len(records) == 0 || len(records) < limit {
			break
		}
	}
	return all, nil
}

func (s *SOAPPolling) fetchCursorBased(ctx context.Context) ([]map[string]any, error) {
	var all []map[string]any
	cursor := ""
	for iteration := 0; iteration < maxPaginationPages; iteration++ {
		record := s.stateRecord(map[string]any{
			"type":   s.pagination.Type,
			"cursor": cursor,
		})
		if cursor != "" {
			record[s.pagination.Param] = cursor
		}
		resp, err := s.call(ctx, record)
		if err != nil {
			return nil, err
		}
		records, err := s.recordsFromResponse(resp.Data)
		if err != nil {
			return nil, err
		}
		all = append(all, records...)
		nextCursor := soaputil.ExtractStringField(resp.Data, s.pagination.NextCursorField)
		if nextCursor == "" {
			break
		}
		cursor = nextCursor
	}
	return all, nil
}

func (s *SOAPPolling) stateRecord(pagination map[string]any) map[string]any {
	record := make(map[string]any)
	if pagination != nil {
		record["pagination"] = pagination
	}
	if s.persistenceConfig == nil || s.lastState == nil {
		return record
	}
	state := map[string]any{}
	if s.persistenceConfig.TimestampEnabled() && s.lastState.LastTimestamp != nil {
		value := s.lastState.FormatTimestamp()
		state["lastTimestamp"] = value
		if s.persistenceConfig.Timestamp.QueryParam != "" {
			record[s.persistenceConfig.Timestamp.QueryParam] = value
		}
	}
	if s.persistenceConfig.IDEnabled() && s.lastState.LastID != nil {
		state["lastID"] = *s.lastState.LastID
		if s.persistenceConfig.ID.QueryParam != "" {
			record[s.persistenceConfig.ID.QueryParam] = *s.lastState.LastID
		}
	}
	if len(state) > 0 {
		record["state"] = state
	}
	return record
}

// GetRetryInfo is exposed for consistency with HTTP modules. Retry execution
// lives inside soapclient and does not currently surface attempt metadata.
func (s *SOAPPolling) GetRetryInfo() *connector.RetryInfo {
	return s.lastRetryInfo
}

// Close releases HTTP resources.
func (s *SOAPPolling) Close() error {
	s.httpClient.CloseIdleConnections()
	return nil
}

// SetPipelineID sets the pipeline ID for state persistence.
func (s *SOAPPolling) SetPipelineID(pipelineID string) {
	s.pipelineID = pipelineID
}

// LoadState loads the last persisted state for this pipeline.
func (s *SOAPPolling) LoadState() (*persistence.State, error) {
	if s.stateStore == nil || s.pipelineID == "" {
		return nil, nil
	}
	state, err := s.stateStore.Load(s.pipelineID)
	if err != nil {
		return nil, err
	}
	s.lastState = state
	return state, nil
}

// GetPersistenceConfig returns state persistence settings.
func (s *SOAPPolling) GetPersistenceConfig() *persistence.StatePersistenceConfig {
	return s.persistenceConfig
}

// GetLastState returns the last loaded state.
func (s *SOAPPolling) GetLastState() *persistence.State {
	return s.lastState
}

// SetStateStore overrides the persistence store.
func (s *SOAPPolling) SetStateStore(store *persistence.StateStore) {
	s.stateStore = store
}
