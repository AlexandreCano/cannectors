// Package connector provides public types and interfaces for connector pipelines.
// This package is intended to be importable by external projects that need
// to interact with the Cannectors runtime.
package connector

import (
	"encoding/json"
	"time"
)

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

	// DryRunOptions configures dry-run mode behavior
	DryRunOptions *DryRunOptions `json:"dryRunOptions,omitempty"`

	// Defaults holds module-level defaults (onError, timeoutMs, retry).
	Defaults *ModuleDefaults `json:"defaults,omitempty"`

	// Enabled indicates whether the pipeline is active
	Enabled bool `json:"enabled"`

	// CreatedAt is when the pipeline was created
	CreatedAt time.Time `json:"createdAt,omitempty"`

	// UpdatedAt is when the pipeline was last modified
	UpdatedAt time.Time `json:"updatedAt,omitempty"`
}

// ModuleConfig represents the configuration for a pipeline module.
// Modules can be Input, Filter, or Output types.
//
// Raw contains all module configuration fields (except "type") as JSON,
// with defaults (onError, timeoutMs, retry) already resolved.
// Modules parse this via moduleconfig.ParseModuleConfig[T] or json.Unmarshal.
type ModuleConfig struct {
	// Type identifies the module type (e.g., "http-polling", "mapping", "http-request")
	Type string `json:"type"`

	// Raw contains the full JSON of all module fields except "type", with defaults resolved.
	// Populated by the converter. Modules parse this via moduleconfig.ParseConfig[T].
	Raw json.RawMessage `json:"config"`
}

// AuthConfig defines authentication configuration for a module.
type AuthConfig struct {
	// Type is the authentication type (e.g., "basic", "bearer", "oauth2", "api-key")
	Type string `json:"type"`

	// Credentials contains the raw authentication credentials as JSON.
	// Each auth type unmarshals this into its own typed struct (e.g., CredentialsAPIKey).
	Credentials json.RawMessage `json:"credentials"`
}

// ModuleDefaults holds default settings for all modules. Overridable per module.
type ModuleDefaults struct {
	OnError   string       `json:"onError,omitempty"`
	TimeoutMs int          `json:"timeoutMs,omitempty"`
	Retry     *RetryConfig `json:"retry,omitempty"`
}

// DryRunOptions configures dry-run mode behavior.
type DryRunOptions struct {
	// ShowCredentials when true displays actual credentials instead of masked values
	// WARNING: Only use for debugging in secure environments
	ShowCredentials bool `json:"showCredentials,omitempty"`
}

// RetryInfo holds retry attempt information from module execution.
type RetryInfo struct {
	// TotalAttempts is the total number of attempts (initial + retries)
	TotalAttempts int `json:"totalAttempts,omitempty"`
	// RetryCount is the number of retries performed
	RetryCount int `json:"retryCount,omitempty"`
	// RetryDelaysMs is the delay before each retry in milliseconds
	RetryDelaysMs []int64 `json:"retryDelaysMs,omitempty"`
	// TotalDurationMs is the total time spent including retries, in milliseconds.
	TotalDurationMs int64 `json:"totalDurationMs,omitempty"`
}

// RetryInfoProvider is implemented by modules that perform retries (e.g. HTTP Polling, HTTP Request).
// The executor uses it to populate ExecutionResult.RetryInfo.
type RetryInfoProvider interface {
	GetRetryInfo() *RetryInfo
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

	// RetryInfo holds retry information from the last stage that performed retries (Input or Output)
	RetryInfo *RetryInfo `json:"retryInfo,omitempty"`

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

	// ErrorCategory is the classified category (e.g. "network", "authentication", "validation", "server")
	ErrorCategory string `json:"errorCategory,omitempty"`

	// ErrorType is the error type (e.g. "retryable", "fatal")
	ErrorType string `json:"errorType,omitempty"`

	// Details contains additional error context
	Details map[string]interface{} `json:"details,omitempty"`
}

// ModuleBase contains properties common to all modules.
// Mirrors common-schema.json#/$defs/moduleBase with resolved defaults.
type ModuleBase struct {
	ID          string   `json:"id,omitempty"`
	Name        string   `json:"name,omitempty"`
	Description string   `json:"description,omitempty"`
	Enabled     *bool    `json:"enabled,omitempty"`
	Tags        []string `json:"tags,omitempty"`
	OnError     string   `json:"onError,omitempty"`
}

// RetryConfig holds typed retry configuration.
// Mirrors common-schema.json#/$defs/retryConfig.
type RetryConfig struct {
	MaxAttempts          int     `json:"maxAttempts,omitempty"`
	DelayMs              int     `json:"delayMs,omitempty"`
	BackoffMultiplier    float64 `json:"backoffMultiplier,omitempty"`
	MaxDelayMs           int     `json:"maxDelayMs,omitempty"`
	RetryableStatusCodes []int   `json:"retryableStatusCodes,omitempty"`
	UseRetryAfterHeader  bool    `json:"useRetryAfterHeader,omitempty"`
	RetryHintFromBody    string  `json:"retryHintFromBody,omitempty"`
}

// CredentialsAPIKey holds credentials for api-key authentication.
type CredentialsAPIKey struct {
	Key        string `json:"key"`
	Location   string `json:"location,omitempty"`
	HeaderName string `json:"headerName,omitempty"`
	ParamName  string `json:"paramName,omitempty"`
}

// CredentialsBearer holds credentials for bearer token authentication.
type CredentialsBearer struct {
	Token string `json:"token"`
}

// CredentialsBasic holds credentials for basic HTTP authentication.
type CredentialsBasic struct {
	Username string `json:"username"`
	Password string `json:"password"`
}

// CredentialsOAuth2 holds credentials for OAuth2 client credentials.
type CredentialsOAuth2 struct {
	TokenURL     string   `json:"tokenUrl"`
	ClientID     string   `json:"clientId"`
	ClientSecret string   `json:"clientSecret"`
	Scopes       []string `json:"scopes,omitempty"`
}
