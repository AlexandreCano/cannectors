// Package connector provides public types and interfaces for connector pipelines.
// This package is intended to be importable by external projects that need
// to interact with the Canectors runtime.
package connector

import "time"

// Pipeline represents a complete connector pipeline configuration.
// It contains all the modules (Input, Filters, Output) and metadata
// required to execute a data transfer between systems.
type Pipeline struct {
	// ID is the unique identifier for this pipeline
	ID string `json:"id"`

	// Name is the human-readable name of the pipeline
	Name string `json:"name"`

	// Description provides additional context about the pipeline
	Description string `json:"description,omitempty"`

	// Version is the pipeline configuration version
	Version string `json:"version"`

	// Input defines the data source module
	Input *ModuleConfig `json:"input"`

	// Filters is an ordered list of transformation modules
	Filters []ModuleConfig `json:"filters,omitempty"`

	// Output defines the data destination module
	Output *ModuleConfig `json:"output"`

	// Schedule defines the CRON expression for periodic execution
	Schedule string `json:"schedule,omitempty"`

	// DryRunOptions configures dry-run mode behavior
	DryRunOptions *DryRunOptions `json:"dryRunOptions,omitempty"`

	// ErrorHandling configures retry and failure behavior
	ErrorHandling *ErrorHandling `json:"errorHandling,omitempty"`

	// Enabled indicates whether the pipeline is active
	Enabled bool `json:"enabled"`

	// CreatedAt is when the pipeline was created
	CreatedAt time.Time `json:"createdAt,omitempty"`

	// UpdatedAt is when the pipeline was last modified
	UpdatedAt time.Time `json:"updatedAt,omitempty"`
}

// ModuleConfig represents the configuration for a pipeline module.
// Modules can be Input, Filter, or Output types.
type ModuleConfig struct {
	// Type identifies the module type (e.g., "http-polling", "mapping", "http-request")
	Type string `json:"type"`

	// Config contains the module-specific configuration
	Config map[string]interface{} `json:"config"`

	// Authentication references an authentication configuration
	Authentication *AuthConfig `json:"authentication,omitempty"`
}

// AuthConfig defines authentication configuration for a module.
type AuthConfig struct {
	// Type is the authentication type (e.g., "basic", "bearer", "oauth2", "api-key")
	Type string `json:"type"`

	// Credentials contains the authentication credentials
	Credentials map[string]string `json:"credentials"`
}

// ErrorHandling defines how errors should be handled during execution.
type ErrorHandling struct {
	// RetryCount is the number of retry attempts
	RetryCount int `json:"retryCount"`

	// RetryDelay is the delay between retries in milliseconds
	RetryDelay int `json:"retryDelay"`

	// OnError specifies the action on unrecoverable errors ("stop", "continue", "notify")
	OnError string `json:"onError"`
}

// DryRunOptions configures dry-run mode behavior.
type DryRunOptions struct {
	// ShowCredentials when true displays actual credentials instead of masked values
	// WARNING: Only use for debugging in secure environments
	ShowCredentials bool `json:"showCredentials,omitempty"`
}

// ExecutionResult represents the result of a pipeline execution.
type ExecutionResult struct {
	// PipelineID is the ID of the executed pipeline
	PipelineID string `json:"pipelineId"`

	// Status is the execution status ("success", "error", "partial")
	Status string `json:"status"`

	// StartedAt is when execution started
	StartedAt time.Time `json:"startedAt"`

	// CompletedAt is when execution completed
	CompletedAt time.Time `json:"completedAt"`

	// RecordsProcessed is the number of records processed
	RecordsProcessed int `json:"recordsProcessed"`

	// RecordsFailed is the number of records that failed
	RecordsFailed int `json:"recordsFailed"`

	// Error contains error details if execution failed
	Error *ExecutionError `json:"error,omitempty"`

	// DryRunPreview contains preview of requests that would be sent (only set in dry-run mode)
	// For output modules implementing PreviewableModule, this shows what would be sent
	DryRunPreview []RequestPreview `json:"dryRunPreview,omitempty"`
}

// RequestPreview contains the preview of an HTTP request that would be sent.
// Used in dry-run mode to show what would be sent without actually sending.
type RequestPreview struct {
	// Endpoint is the resolved URL including path parameters and query params
	Endpoint string `json:"endpoint"`

	// Method is the HTTP method (POST, PUT, PATCH)
	Method string `json:"method"`

	// Headers contains all request headers. Authentication-related headers may be
	// masked or unmasked depending on DryRunOptions/PreviewOptions (for example,
	// when ShowCredentials is enabled).
	Headers map[string]string `json:"headers"`

	// BodyPreview is the formatted JSON body that would be sent
	BodyPreview string `json:"bodyPreview"`

	// RecordCount is the number of records included in this request
	RecordCount int `json:"recordCount"`
}

// ExecutionError contains details about an execution failure.
type ExecutionError struct {
	// Code is the error code
	Code string `json:"code"`

	// Message is the human-readable error message
	Message string `json:"message"`

	// Module is the module where the error occurred
	Module string `json:"module,omitempty"`

	// Details contains additional error context
	Details map[string]interface{} `json:"details,omitempty"`
}
