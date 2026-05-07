package input

import (
	"fmt"
	"net/url"

	"github.com/cannectors/runtime/internal/logger"
	"github.com/cannectors/runtime/internal/persistence"
)

// SetPipelineID sets the pipeline ID for state persistence.
// Must be called before Fetch if state persistence is enabled.
func (h *HTTPPolling) SetPipelineID(pipelineID string) {
	h.pipelineID = pipelineID
}

// LoadState loads the last persisted state for this pipeline.
// Returns nil, nil if no state exists (first execution) or if persistence is disabled.
// Returns nil, error if state loading fails (caller should decide whether to continue).
// Should be called before Fetch to enable state-based filtering.
func (h *HTTPPolling) LoadState() (*persistence.State, error) {
	if h.stateStore == nil || h.pipelineID == "" {
		return nil, nil
	}

	state, err := h.stateStore.Load(h.pipelineID)
	if err != nil {
		logger.Warn("failed to load state",
			"pipeline_id", h.pipelineID,
			"error", err.Error(),
		)
		// Return error instead of silently continuing
		// Caller (pipeline executor) will log and continue gracefully
		return nil, err
	}

	h.lastState = state
	return state, nil
}

// GetPersistenceConfig returns the state persistence configuration.
func (h *HTTPPolling) GetPersistenceConfig() *persistence.StatePersistenceConfig {
	return h.persistenceConfig
}

// GetLastState returns the last loaded state.
func (h *HTTPPolling) GetLastState() *persistence.State {
	return h.lastState
}

// SetStateStore sets the state store to use for persistence.
// Overrides the state store created during module initialization.
func (h *HTTPPolling) SetStateStore(store *persistence.StateStore) {
	h.stateStore = store
}

// buildEndpointWithState builds the endpoint URL with state-based query parameters.
// If state persistence is enabled and state exists, adds appropriate query params.
func (h *HTTPPolling) buildEndpointWithState(endpoint string) (string, error) {
	if h.persistenceConfig == nil || !h.persistenceConfig.IsEnabled() || h.lastState == nil {
		return endpoint, nil
	}

	parsedURL, err := url.Parse(endpoint)
	if err != nil {
		return "", fmt.Errorf(errMsgParsingEndpointURL, err)
	}

	q := parsedURL.Query()
	modified := false

	// Add timestamp query param if configured
	if h.persistenceConfig.TimestampEnabled() && h.persistenceConfig.Timestamp.QueryParam != "" {
		if h.lastState.LastTimestamp != nil {
			q.Set(h.persistenceConfig.Timestamp.QueryParam, h.lastState.FormatTimestamp())
			modified = true
			logger.Debug("added timestamp query param for state persistence",
				"pipeline_id", h.pipelineID,
				"param", h.persistenceConfig.Timestamp.QueryParam,
				"value", h.lastState.FormatTimestamp(),
			)
		}
	}

	// Add ID query param if configured
	if h.persistenceConfig.IDEnabled() && h.persistenceConfig.ID.QueryParam != "" {
		if h.lastState.LastID != nil {
			q.Set(h.persistenceConfig.ID.QueryParam, *h.lastState.LastID)
			modified = true
			logger.Debug("added ID query param for state persistence",
				"pipeline_id", h.pipelineID,
				"param", h.persistenceConfig.ID.QueryParam,
				"value", *h.lastState.LastID,
			)
		}
	}

	if modified {
		parsedURL.RawQuery = q.Encode()
	}

	return parsedURL.String(), nil
}
