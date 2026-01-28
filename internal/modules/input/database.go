// Package input provides implementations for input modules.
// DatabaseInput module fetches data from databases using SQL queries.
package input

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/canectors/runtime/internal/database"
	"github.com/canectors/runtime/internal/logger"
	"github.com/canectors/runtime/internal/persistence"
	"github.com/canectors/runtime/pkg/connector"
)

// Default configuration values for database input
const (
	defaultDatabaseTimeout = 30 * time.Second
	defaultQueryLimit      = 1000
)

// Template placeholder constants
const (
	// LastRunTimestampPlaceholder is the template placeholder for the last execution timestamp
	LastRunTimestampPlaceholder = "{{lastRunTimestamp}}"
)

// Error types for database input module
var (
	ErrDatabaseNilConfig      = errors.New("database input configuration is nil")
	ErrDatabaseMissingQuery   = errors.New("query is required for database input")
	ErrDatabaseMissingConnStr = errors.New("connection string is required for database input")
)

// DatabaseInputConfig holds configuration for the database input module.
type DatabaseInputConfig struct {
	// Connection configuration
	ConnectionString    string `json:"connectionString"`
	ConnectionStringRef string `json:"connectionStringRef"`
	Driver              string `json:"driver"`

	// Query configuration - use query OR queryFile
	Query      string                 `json:"query"`     // Inline SQL query
	QueryFile  string                 `json:"queryFile"` // Path to SQL file
	Parameters map[string]interface{} `json:"parameters"`

	// Pagination configuration
	Pagination *DatabasePaginationConfig `json:"pagination"`

	// Incremental query configuration
	Incremental *IncrementalConfig `json:"incremental"`

	// Pool configuration
	MaxOpenConns    int `json:"maxOpenConns"`
	MaxIdleConns    int `json:"maxIdleConns"`
	ConnMaxLifetime int `json:"connMaxLifetimeSeconds"`
	ConnMaxIdleTime int `json:"connMaxIdleTimeSeconds"`

	// Timeout
	TimeoutMs int `json:"timeoutMs"`
}

// DatabasePaginationConfig defines pagination behavior for database queries.
type DatabasePaginationConfig struct {
	// Type: "limit-offset" or "cursor"
	Type string `json:"type"`
	// Limit: number of records per page
	Limit int `json:"limit"`
	// OffsetParam: for limit-offset pagination
	OffsetParam string `json:"offsetParam"`
	// CursorField: field to use for cursor-based pagination
	CursorField string `json:"cursorField"`
	// CursorParam: parameter name for cursor value
	CursorParam string `json:"cursorParam"`
}

// IncrementalConfig defines incremental query configuration.
type IncrementalConfig struct {
	// Enabled: whether to enable incremental queries
	Enabled bool `json:"enabled"`
	// TimestampField: field name for timestamp-based incremental queries
	TimestampField string `json:"timestampField"`
	// TimestampParam: parameter name for timestamp in query
	TimestampParam string `json:"timestampParam"`
	// IDField: field name for ID-based incremental queries
	IDField string `json:"idField"`
	// IDParam: parameter name for ID in query
	IDParam string `json:"idParam"`
}

// DatabaseInput implements a database input module.
// It executes SQL queries to fetch data from databases.
type DatabaseInput struct {
	config     DatabaseInputConfig
	db         *sql.DB
	driver     string
	timeout    time.Duration
	pipelineID string
	stateStore *persistence.StateStore
	lastState  *persistence.State
}

// NewDatabaseInputFromConfig creates a new database input module from configuration.
func NewDatabaseInputFromConfig(cfg *connector.ModuleConfig) (*DatabaseInput, error) {
	if cfg == nil {
		return nil, ErrDatabaseNilConfig
	}

	config := parseDatabaseInputConfig(cfg.Config)

	// Load query from file if queryFile is specified
	if config.QueryFile != "" && config.Query == "" {
		// Validate file path to prevent path traversal attacks
		if !filepath.IsAbs(config.QueryFile) {
			// For relative paths, ensure they don't contain ".."
			if strings.Contains(config.QueryFile, "..") {
				return nil, fmt.Errorf("query file path contains invalid '..' component: %s", config.QueryFile)
			}
		}
		queryBytes, readErr := os.ReadFile(config.QueryFile)
		if readErr != nil {
			return nil, fmt.Errorf("reading query file %s: %w", config.QueryFile, readErr)
		}
		config.Query = string(queryBytes)
	}

	// Validate required fields
	if config.Query == "" {
		return nil, ErrDatabaseMissingQuery
	}
	if config.ConnectionString == "" && config.ConnectionStringRef == "" {
		return nil, ErrDatabaseMissingConnStr
	}

	// Set timeout
	timeout := defaultDatabaseTimeout
	if config.TimeoutMs > 0 {
		timeout = time.Duration(config.TimeoutMs) * time.Millisecond
	}

	// Create database config
	dbConfig := database.Config{
		ConnectionString:    config.ConnectionString,
		ConnectionStringRef: config.ConnectionStringRef,
		Driver:              config.Driver,
		MaxOpenConns:        config.MaxOpenConns,
		MaxIdleConns:        config.MaxIdleConns,
		ConnMaxLifetime:     time.Duration(config.ConnMaxLifetime) * time.Second,
		ConnMaxIdleTime:     time.Duration(config.ConnMaxIdleTime) * time.Second,
		ConnectTimeout:      timeout,
	}

	// Open database connection
	db, driver, err := database.Open(dbConfig)
	if err != nil {
		return nil, fmt.Errorf("creating database connection: %w", err)
	}

	module := &DatabaseInput{
		config:  config,
		db:      db,
		driver:  driver,
		timeout: timeout,
	}

	// Initialize state store if incremental is enabled
	if config.Incremental != nil && config.Incremental.Enabled {
		module.stateStore = persistence.NewStateStore(persistence.DefaultStatePath)
	}

	logger.Debug("database input module created",
		"driver", driver,
		"timeout", timeout.String(),
		"has_pagination", config.Pagination != nil,
		"has_incremental", config.Incremental != nil && config.Incremental.Enabled,
	)

	return module, nil
}

// parseDatabaseInputConfig parses the raw configuration map into DatabaseInputConfig.
func parseDatabaseInputConfig(cfg map[string]interface{}) DatabaseInputConfig {
	config := DatabaseInputConfig{}

	// Connection settings
	if v, ok := cfg["connectionString"].(string); ok {
		config.ConnectionString = v
	}
	if v, ok := cfg["connectionStringRef"].(string); ok {
		config.ConnectionStringRef = v
	}
	if v, ok := cfg["driver"].(string); ok {
		config.Driver = v
	}

	// Query settings
	if v, ok := cfg["query"].(string); ok {
		config.Query = v
	}
	if v, ok := cfg["queryFile"].(string); ok {
		config.QueryFile = v
	}
	if v, ok := cfg["parameters"].(map[string]interface{}); ok {
		config.Parameters = v
	}

	// Pool settings
	if v, ok := cfg["maxOpenConns"].(float64); ok {
		config.MaxOpenConns = int(v)
	}
	if v, ok := cfg["maxIdleConns"].(float64); ok {
		config.MaxIdleConns = int(v)
	}
	if v, ok := cfg["connMaxLifetimeSeconds"].(float64); ok {
		config.ConnMaxLifetime = int(v)
	}
	if v, ok := cfg["connMaxIdleTimeSeconds"].(float64); ok {
		config.ConnMaxIdleTime = int(v)
	}
	if v, ok := cfg["timeoutMs"].(float64); ok {
		config.TimeoutMs = int(v)
	}

	// Parse pagination
	if paginationRaw, ok := cfg["pagination"].(map[string]interface{}); ok {
		config.Pagination = parseDatabasePaginationConfig(paginationRaw)
	}

	// Parse incremental
	if incrementalRaw, ok := cfg["incremental"].(map[string]interface{}); ok {
		config.Incremental = parseIncrementalConfig(incrementalRaw)
	}

	return config
}

// parseDatabasePaginationConfig parses pagination configuration.
func parseDatabasePaginationConfig(cfg map[string]interface{}) *DatabasePaginationConfig {
	config := &DatabasePaginationConfig{
		Type:  "limit-offset",
		Limit: defaultQueryLimit,
	}

	if v, ok := cfg["type"].(string); ok {
		config.Type = v
	}
	if v, ok := cfg["limit"].(float64); ok && v > 0 {
		config.Limit = int(v)
	}
	if v, ok := cfg["offsetParam"].(string); ok {
		config.OffsetParam = v
	}
	if v, ok := cfg["cursorField"].(string); ok {
		config.CursorField = v
	}
	if v, ok := cfg["cursorParam"].(string); ok {
		config.CursorParam = v
	}

	return config
}

// parseIncrementalConfig parses incremental query configuration.
func parseIncrementalConfig(cfg map[string]interface{}) *IncrementalConfig {
	config := &IncrementalConfig{}

	if v, ok := cfg["enabled"].(bool); ok {
		config.Enabled = v
	}
	if v, ok := cfg["timestampField"].(string); ok {
		config.TimestampField = v
	}
	if v, ok := cfg["timestampParam"].(string); ok {
		config.TimestampParam = v
	}
	if v, ok := cfg["idField"].(string); ok {
		config.IDField = v
	}
	if v, ok := cfg["idParam"].(string); ok {
		config.IDParam = v
	}

	return config
}

// Fetch retrieves data from the database.
func (d *DatabaseInput) Fetch(ctx context.Context) ([]map[string]interface{}, error) {
	startTime := time.Now()

	// Load state if state store is initialized (for incremental queries)
	if d.stateStore != nil && d.pipelineID != "" {
		state, err := d.LoadState()
		if err != nil {
			logger.Warn("failed to load state for database input, continuing without incremental support",
				"pipeline_id", d.pipelineID,
				"error", err.Error(),
			)
		} else if state != nil {
			d.lastState = state
		}
	}

	logger.Info("database input fetch started",
		"module_type", "database",
		"driver", d.driver,
		"has_pagination", d.config.Pagination != nil,
		"has_incremental", d.config.Incremental != nil && d.config.Incremental.Enabled,
	)

	// Build query with parameters
	query, args := d.buildQuery()

	// Execute query based on pagination configuration
	var records []map[string]interface{}
	var err error

	if d.config.Pagination != nil && d.config.Pagination.Type != "" {
		records, err = d.fetchWithPagination(ctx, query, args)
	} else {
		records, err = d.fetchSingle(ctx, query, args)
	}

	duration := time.Since(startTime)

	if err != nil {
		logger.Error("database input fetch failed",
			"module_type", "database",
			"duration", duration,
			"error", err.Error(),
		)
		return nil, err
	}

	logger.Info("database input fetch completed",
		"module_type", "database",
		"record_count", len(records),
		"duration", duration,
	)

	return records, nil
}

// buildQuery builds the SQL query with parameters.
func (d *DatabaseInput) buildQuery() (string, []interface{}) {
	query := d.config.Query
	var args []interface{}

	// Replace {{lastRunTimestamp}} placeholder with parameterized value
	// This allows injecting the last execution timestamp directly in the query
	if strings.Contains(query, LastRunTimestampPlaceholder) {
		if d.lastState != nil && d.lastState.LastTimestamp != nil {
			query = strings.ReplaceAll(query, LastRunTimestampPlaceholder,
				database.FormatPlaceholder(d.driver, len(args)+1))
			args = append(args, *d.lastState.LastTimestamp)
		} else {
			// First run: use epoch time (1970-01-01) to get all records
			query = strings.ReplaceAll(query, LastRunTimestampPlaceholder,
				database.FormatPlaceholder(d.driver, len(args)+1))
			args = append(args, time.Unix(0, 0))
		}
	}

	// Add incremental parameters if configured (legacy support)
	if d.config.Incremental != nil && d.config.Incremental.Enabled && d.lastState != nil {
		if d.config.Incremental.TimestampParam != "" && d.lastState.LastTimestamp != nil {
			query = strings.ReplaceAll(query, ":"+d.config.Incremental.TimestampParam,
				database.FormatPlaceholder(d.driver, len(args)+1))
			args = append(args, *d.lastState.LastTimestamp)
		}
		if d.config.Incremental.IDParam != "" && d.lastState.LastID != nil {
			query = strings.ReplaceAll(query, ":"+d.config.Incremental.IDParam,
				database.FormatPlaceholder(d.driver, len(args)+1))
			args = append(args, *d.lastState.LastID)
		}
	}

	// Add static parameters - process in query order to maintain parameter order
	// Scan query left-to-right for :paramName tokens and append args in that order
	paramOrder := extractParameterOrder(query)
	for _, paramName := range paramOrder {
		if value, exists := d.config.Parameters[paramName]; exists {
			placeholder := ":" + paramName
			query = strings.ReplaceAll(query, placeholder,
				database.FormatPlaceholder(d.driver, len(args)+1))
			args = append(args, value)
		}
	}

	return query, args
}

// extractParameterOrder extracts parameter names from query in left-to-right order.
// This ensures args slice matches placeholder order for MySQL/SQLite compatibility.
func extractParameterOrder(query string) []string {
	var order []string
	seen := make(map[string]bool)

	// Find all :paramName patterns in order
	start := 0
	for {
		idx := strings.Index(query[start:], ":")
		if idx == -1 {
			break
		}
		idx += start

		// Find end of parameter name (alphanumeric + underscore)
		end := idx + 1
		for end < len(query) && (isAlphanumeric(query[end]) || query[end] == '_') {
			end++
		}

		if end > idx+1 {
			paramName := query[idx+1 : end]
			if !seen[paramName] {
				order = append(order, paramName)
				seen[paramName] = true
			}
		}

		start = end
	}

	return order
}

// isAlphanumeric checks if a byte is alphanumeric.
func isAlphanumeric(b byte) bool {
	return (b >= 'a' && b <= 'z') || (b >= 'A' && b <= 'Z') || (b >= '0' && b <= '9')
}

// fetchSingle executes a single query without pagination.
func (d *DatabaseInput) fetchSingle(ctx context.Context, query string, args []interface{}) ([]map[string]interface{}, error) {
	ctx, cancel := context.WithTimeout(ctx, d.timeout)
	defer cancel()

	rows, err := d.db.QueryContext(ctx, query, args...)
	if err != nil {
		dbErr := database.ClassifyDatabaseError(err, d.driver, "select", query, len(args))
		return nil, dbErr
	}
	defer func() {
		_ = rows.Close()
	}()

	return d.rowsToRecords(rows)
}

// fetchWithPagination executes queries with pagination.
func (d *DatabaseInput) fetchWithPagination(ctx context.Context, query string, args []interface{}) ([]map[string]interface{}, error) {
	switch d.config.Pagination.Type {
	case "limit-offset":
		return d.fetchLimitOffset(ctx, query, args)
	case "cursor":
		return d.fetchCursor(ctx, query, args)
	default:
		return d.fetchSingle(ctx, query, args)
	}
}

// fetchLimitOffset implements LIMIT/OFFSET pagination.
func (d *DatabaseInput) fetchLimitOffset(ctx context.Context, query string, args []interface{}) ([]map[string]interface{}, error) {
	var allRecords []map[string]interface{}
	offset := 0
	limit := d.config.Pagination.Limit
	if limit <= 0 {
		limit = defaultQueryLimit
	}

	// Check if query uses offsetParam placeholder
	offsetParam := d.config.Pagination.OffsetParam
	usesOffsetParam := offsetParam != "" && strings.Contains(query, ":"+offsetParam)

	for {
		var paginatedQuery string
		var paginatedArgs []interface{}

		if usesOffsetParam {
			// Replace :offsetParam placeholder with parameterized value
			paginatedQuery = strings.ReplaceAll(query, ":"+offsetParam,
				database.FormatPlaceholder(d.driver, len(args)+1))
			paginatedArgs = append(args, offset)
			paginatedQuery = fmt.Sprintf("%s LIMIT %d", paginatedQuery, limit)
		} else {
			// Use literal LIMIT/OFFSET syntax
			paginatedQuery = fmt.Sprintf("%s LIMIT %d OFFSET %d", query, limit, offset)
			paginatedArgs = args
		}

		queryCtx, cancel := context.WithTimeout(ctx, d.timeout)
		rows, err := d.db.QueryContext(queryCtx, paginatedQuery, paginatedArgs...)
		if err != nil {
			cancel()
			dbErr := database.ClassifyDatabaseError(err, d.driver, "select", paginatedQuery, len(paginatedArgs))
			return nil, dbErr
		}

		records, err := d.rowsToRecords(rows)
		_ = rows.Close()
		cancel()

		if err != nil {
			return nil, err
		}

		allRecords = append(allRecords, records...)

		if len(records) < limit {
			break
		}

		offset += limit
	}

	return allRecords, nil
}

// fetchCursor implements cursor-based pagination.
func (d *DatabaseInput) fetchCursor(ctx context.Context, query string, args []interface{}) ([]map[string]interface{}, error) {
	var allRecords []map[string]interface{}
	var cursor interface{}
	limit := d.config.Pagination.Limit
	if limit <= 0 {
		limit = defaultQueryLimit
	}

	cursorField := d.config.Pagination.CursorField
	cursorParam := d.config.Pagination.CursorParam

	for {
		cursorQuery := query
		cursorArgs := make([]interface{}, len(args))
		copy(cursorArgs, args)

		if cursorParam != "" {
			placeholder := ":" + cursorParam
			if strings.Contains(cursorQuery, placeholder) {
				cursorQuery = strings.ReplaceAll(cursorQuery, placeholder,
					database.FormatPlaceholder(d.driver, len(cursorArgs)+1))
				// Always append the current cursor value, which may be nil on the first page.
				cursorArgs = append(cursorArgs, cursor)
			}
		}

		cursorQuery = fmt.Sprintf("%s LIMIT %d", cursorQuery, limit)

		queryCtx, cancel := context.WithTimeout(ctx, d.timeout)
		rows, err := d.db.QueryContext(queryCtx, cursorQuery, cursorArgs...)
		if err != nil {
			cancel()
			dbErr := database.ClassifyDatabaseError(err, d.driver, "select", cursorQuery, len(cursorArgs))
			return nil, dbErr
		}

		records, err := d.rowsToRecords(rows)
		_ = rows.Close()
		cancel()

		if err != nil {
			return nil, err
		}

		allRecords = append(allRecords, records...)

		if len(records) < limit {
			break
		}

		if cursorField != "" && len(records) > 0 {
			lastRecord := records[len(records)-1]
			cursor = lastRecord[cursorField]
		} else {
			break
		}
	}

	return allRecords, nil
}

// rowsToRecords converts sql.Rows to a slice of map records.
func (d *DatabaseInput) rowsToRecords(rows *sql.Rows) ([]map[string]interface{}, error) {
	columns, err := rows.Columns()
	if err != nil {
		return nil, fmt.Errorf("getting column names: %w", err)
	}

	var records []map[string]interface{}

	for rows.Next() {
		values := make([]interface{}, len(columns))
		valuePtrs := make([]interface{}, len(columns))
		for i := range values {
			valuePtrs[i] = &values[i]
		}

		if err := rows.Scan(valuePtrs...); err != nil {
			return nil, fmt.Errorf("scanning row: %w", err)
		}

		record := make(map[string]interface{}, len(columns))
		for i, col := range columns {
			val := values[i]
			record[col] = convertDatabaseValue(val)
		}

		records = append(records, record)
	}

	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterating rows: %w", err)
	}

	return records, nil
}

// convertDatabaseValue converts database values to appropriate Go types.
func convertDatabaseValue(val interface{}) interface{} {
	if val == nil {
		return nil
	}

	if b, ok := val.([]byte); ok {
		return string(b)
	}

	if t, ok := val.(time.Time); ok {
		return t.Format(time.RFC3339)
	}

	return val
}

// Close releases resources held by the database input module.
func (d *DatabaseInput) Close() error {
	if d.db != nil {
		return d.db.Close()
	}
	return nil
}

// SetPipelineID sets the pipeline ID for state persistence.
func (d *DatabaseInput) SetPipelineID(pipelineID string) {
	d.pipelineID = pipelineID
}

// LoadState loads the last persisted state for this pipeline.
func (d *DatabaseInput) LoadState() (*persistence.State, error) {
	if d.stateStore == nil || d.pipelineID == "" {
		return nil, nil
	}

	state, err := d.stateStore.Load(d.pipelineID)
	if err != nil {
		logger.Warn("failed to load state for database input",
			"pipeline_id", d.pipelineID,
			"error", err.Error(),
		)
		return nil, err
	}

	d.lastState = state
	return state, nil
}

// GetPersistenceConfig returns the state persistence configuration.
func (d *DatabaseInput) GetPersistenceConfig() *persistence.StatePersistenceConfig {
	if d.config.Incremental == nil || !d.config.Incremental.Enabled {
		return nil
	}

	return &persistence.StatePersistenceConfig{
		Timestamp: &persistence.TimestampConfig{
			Enabled: d.config.Incremental.TimestampField != "",
		},
		ID: &persistence.IDConfig{
			Enabled: d.config.Incremental.IDField != "",
			Field:   d.config.Incremental.IDField,
		},
	}
}

// GetLastState returns the last loaded state.
func (d *DatabaseInput) GetLastState() *persistence.State {
	return d.lastState
}
