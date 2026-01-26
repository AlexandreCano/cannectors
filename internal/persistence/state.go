// Package persistence provides state persistence for pipeline execution.
// It supports persisting last execution timestamp and last processed ID
// for polling input modules to enable reliable resumption after restarts.
package persistence

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sync"
	"time"

	"github.com/canectors/runtime/internal/logger"
)

// DefaultStatePath is the default directory for state files.
const DefaultStatePath = "./canectors-data/state"

// Common errors
var (
	// ErrInvalidPipelineID is returned when pipeline ID is empty.
	ErrInvalidPipelineID = errors.New("pipeline ID is required")

	// ErrNilState is returned when state is nil.
	ErrNilState = errors.New("state is nil")
)

// State represents the persisted state for a pipeline.
// Both LastTimestamp and LastID are optional and can be used independently.
type State struct {
	// PipelineID is the unique identifier for the pipeline.
	PipelineID string `json:"pipelineId"`

	// LastTimestamp is the execution start timestamp from the last successful execution.
	// Used for timestamp-based filtering (e.g., ?since=2026-01-26T10:30:00Z).
	LastTimestamp *time.Time `json:"lastTimestamp,omitempty"`

	// LastID is the maximum ID from records processed in the last successful execution.
	// Used for ID-based filtering (e.g., ?after_id=12345).
	LastID *string `json:"lastId,omitempty"`

	// UpdatedAt is when this state was last updated.
	UpdatedAt time.Time `json:"updatedAt"`
}

// FormatTimestamp returns the LastTimestamp formatted as RFC3339 (ISO 8601).
// Returns empty string if LastTimestamp is nil.
func (s *State) FormatTimestamp() string {
	if s.LastTimestamp == nil {
		return ""
	}
	return s.LastTimestamp.Format(time.RFC3339)
}

// StateStore provides thread-safe persistence of pipeline state.
// State files are stored as JSON in the configured base path.
type StateStore struct {
	basePath string
	mu       sync.RWMutex
}

// NewStateStore creates a new StateStore with the specified base path.
// If basePath is empty, DefaultStatePath is used.
func NewStateStore(basePath string) *StateStore {
	if basePath == "" {
		basePath = DefaultStatePath
	}
	return &StateStore{
		basePath: basePath,
	}
}

// filePath returns the full path for a pipeline's state file.
func (s *StateStore) filePath(pipelineID string) string {
	// Sanitize pipeline ID to prevent directory traversal
	safeName := filepath.Base(pipelineID)
	return filepath.Join(s.basePath, safeName+".json")
}

// Save persists the state for a pipeline.
// Uses atomic write (temp file + rename) to prevent corruption.
// Creates the base directory if it doesn't exist.
func (s *StateStore) Save(pipelineID string, state *State) error {
	if pipelineID == "" {
		return ErrInvalidPipelineID
	}
	if state == nil {
		return ErrNilState
	}

	s.mu.Lock()
	defer s.mu.Unlock()

	// Ensure directory exists
	// Handle race condition: if directory is created by another process between check and create,
	// MkdirAll will return nil (success) if the directory already exists
	if err := os.MkdirAll(s.basePath, 0700); err != nil {
		// Check if directory exists now (another process might have created it)
		if _, statErr := os.Stat(s.basePath); statErr == nil {
			// Directory exists now, continue (race condition resolved)
			logger.Debug("state directory created by another process",
				"path", s.basePath,
			)
		} else {
			logger.Warn("failed to create state directory",
				"path", s.basePath,
				"error", err.Error(),
			)
			return fmt.Errorf("creating state directory: %w", err)
		}
	}

	// Ensure state has the correct pipeline ID
	state.PipelineID = pipelineID

	// Marshal state to JSON
	data, err := json.MarshalIndent(state, "", "  ")
	if err != nil {
		logger.Warn("failed to marshal state",
			"pipeline_id", pipelineID,
			"error", err.Error(),
		)
		return fmt.Errorf("marshaling state: %w", err)
	}

	// Write to temp file first (atomic write)
	filePath := s.filePath(pipelineID)
	tempPath := filePath + ".tmp"

	if err := os.WriteFile(tempPath, data, 0600); err != nil {
		logger.Warn("failed to write temp state file",
			"pipeline_id", pipelineID,
			"path", tempPath,
			"error", err.Error(),
		)
		return fmt.Errorf("writing temp state file: %w", err)
	}

	// Rename temp file to final path (atomic on POSIX)
	if err := os.Rename(tempPath, filePath); err != nil {
		// Clean up temp file on error
		_ = os.Remove(tempPath)
		logger.Warn("failed to rename state file",
			"pipeline_id", pipelineID,
			"temp_path", tempPath,
			"final_path", filePath,
			"error", err.Error(),
		)
		return fmt.Errorf("renaming state file: %w", err)
	}

	logger.Debug("state saved",
		"pipeline_id", pipelineID,
		"path", filePath,
		"has_timestamp", state.LastTimestamp != nil,
		"has_id", state.LastID != nil,
	)

	return nil
}

// Load retrieves the state for a pipeline.
// Returns nil, nil if the state file doesn't exist (first execution).
func (s *StateStore) Load(pipelineID string) (*State, error) {
	if pipelineID == "" {
		return nil, ErrInvalidPipelineID
	}

	s.mu.RLock()
	defer s.mu.RUnlock()

	filePath := s.filePath(pipelineID)

	data, err := os.ReadFile(filePath)
	if err != nil {
		if os.IsNotExist(err) {
			// No state file = first execution, not an error
			logger.Debug("no state file found (first execution)",
				"pipeline_id", pipelineID,
				"path", filePath,
			)
			return nil, nil
		}
		logger.Warn("failed to read state file",
			"pipeline_id", pipelineID,
			"path", filePath,
			"error", err.Error(),
		)
		return nil, fmt.Errorf("reading state file: %w", err)
	}

	var state State
	if err := json.Unmarshal(data, &state); err != nil {
		logger.Warn("failed to unmarshal state",
			"pipeline_id", pipelineID,
			"path", filePath,
			"error", err.Error(),
		)
		return nil, fmt.Errorf("unmarshaling state: %w", err)
	}

	logger.Debug("state loaded",
		"pipeline_id", pipelineID,
		"path", filePath,
		"has_timestamp", state.LastTimestamp != nil,
		"has_id", state.LastID != nil,
	)

	return &state, nil
}

// Delete removes the state file for a pipeline.
// Returns nil if the file doesn't exist.
func (s *StateStore) Delete(pipelineID string) error {
	if pipelineID == "" {
		return ErrInvalidPipelineID
	}

	s.mu.Lock()
	defer s.mu.Unlock()

	filePath := s.filePath(pipelineID)

	if err := os.Remove(filePath); err != nil {
		if os.IsNotExist(err) {
			// File doesn't exist, nothing to delete
			return nil
		}
		logger.Warn("failed to delete state file",
			"pipeline_id", pipelineID,
			"path", filePath,
			"error", err.Error(),
		)
		return fmt.Errorf("deleting state file: %w", err)
	}

	logger.Debug("state deleted",
		"pipeline_id", pipelineID,
		"path", filePath,
	)

	return nil
}

// Exists checks if a state file exists for a pipeline.
func (s *StateStore) Exists(pipelineID string) (bool, error) {
	if pipelineID == "" {
		return false, ErrInvalidPipelineID
	}

	s.mu.RLock()
	defer s.mu.RUnlock()

	filePath := s.filePath(pipelineID)
	_, err := os.Stat(filePath)
	if err != nil {
		if os.IsNotExist(err) {
			return false, nil
		}
		return false, fmt.Errorf("checking state file: %w", err)
	}
	return true, nil
}
