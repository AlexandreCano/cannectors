// Package filter provides implementations for filter modules.
// SQLCall module executes SQL queries for record enrichment.
package filter

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"log/slog"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/canectors/runtime/internal/cache"
	"github.com/canectors/runtime/internal/database"
	"github.com/canectors/runtime/internal/logger"
	"github.com/canectors/runtime/internal/template"
)

// Default configuration values for sql_call module
const (
	defaultSQLCallTimeout   = 30 * time.Second
	defaultSQLCallCacheSize = 1000
	defaultSQLCallCacheTTL  = 300 // 5 minutes
	defaultSQLCallStrategy  = "merge"
	defaultSQLCallOnError   = "fail"
)

// Template prefix constants
const (
	// RecordFieldPrefix is the prefix for record field access in templates
	RecordFieldPrefix = "record."
)

// Note: OnErrorFail, OnErrorSkip, OnErrorLog constants are defined in mapping.go
// and are available in this package since they're in the same package.

// Error types for sql_call module
var (
	ErrSQLCallMissingConnection = errors.New("sql_call connection configuration is required")
	ErrSQLCallMissingQuery      = errors.New("sql_call query is required")
)

// SQLCallConfig represents the configuration for a sql_call filter module.
type SQLCallConfig struct {
	// Connection configuration
	ConnectionString    string `json:"connectionString"`
	ConnectionStringRef string `json:"connectionStringRef"`
	Driver              string `json:"driver"`

	// Query configuration - use query OR queryFile
	Query     string   `json:"query"`     // SQL query with {{record.field}} templates
	QueryFile string   `json:"queryFile"` // Path to SQL file with {{record.field}} templates
	Queries   []string `json:"queries"`   // Multiple queries (executed sequentially)

	// Result handling
	MergeStrategy string `json:"mergeStrategy"` // "merge", "replace", "append"
	DataField     string `json:"dataField"`     // Field to extract from result
	ResultKey     string `json:"resultKey"`     // Key for storing result in "append" mode

	// Error handling
	OnError string `json:"onError"` // "fail", "skip", "log"

	// Cache configuration
	Cache SQLCallCacheConfig `json:"cache"`

	// Pool configuration
	MaxOpenConns    int `json:"maxOpenConns"`
	MaxIdleConns    int `json:"maxIdleConns"`
	ConnMaxLifetime int `json:"connMaxLifetimeSeconds"`
	ConnMaxIdleTime int `json:"connMaxIdleTimeSeconds"`

	// Timeout
	TimeoutMs int `json:"timeoutMs"`
}

// SQLCallCacheConfig defines cache configuration for sql_call module.
type SQLCallCacheConfig struct {
	Enabled    bool   `json:"enabled"`
	MaxSize    int    `json:"maxSize"`
	DefaultTTL int    `json:"defaultTTL"` // in seconds
	Key        string `json:"key"`        // Cache key template
}

// SQLCallModule implements a filter that executes SQL queries for enrichment.
type SQLCallModule struct {
	db                *sql.DB
	driver            string
	query             string
	queries           []string
	mergeStrategy     string
	dataField         string
	resultKey         string
	onError           string
	cache             cache.Cache
	cacheEnabled      bool
	cacheTTL          time.Duration
	cacheKey          string
	timeout           time.Duration
	templateEvaluator *template.Evaluator
}

// NewSQLCallFromConfig creates a new sql_call filter module from configuration.
func NewSQLCallFromConfig(config SQLCallConfig) (*SQLCallModule, error) {
	if err := validateSQLCallConfig(config); err != nil {
		return nil, err
	}

	if err := loadQueryFromFile(&config); err != nil {
		return nil, err
	}

	mergeStrategy := resolveMergeStrategy(config.MergeStrategy)
	onError := resolveOnError(config.OnError)
	timeout := resolveTimeout(config.TimeoutMs)

	db, driver, err := createDatabaseConnection(config, timeout)
	if err != nil {
		return nil, err
	}

	cache, cacheTTL, cacheEnabled := setupCache(config)
	queries := buildQueriesList(config)

	module := &SQLCallModule{
		db:                db,
		driver:            driver,
		query:             config.Query,
		queries:           queries,
		mergeStrategy:     mergeStrategy,
		dataField:         config.DataField,
		resultKey:         config.ResultKey,
		onError:           onError,
		cache:             cache,
		cacheEnabled:      cacheEnabled,
		cacheTTL:          cacheTTL,
		cacheKey:          config.Cache.Key,
		timeout:           timeout,
		templateEvaluator: template.NewEvaluator(),
	}

	logSQLCallModuleInitialization(driver, len(queries), mergeStrategy, onError, cacheEnabled, timeout)

	return module, nil
}

// validateSQLCallConfig validates that required configuration fields are present.
func validateSQLCallConfig(config SQLCallConfig) error {
	if config.ConnectionString == "" && config.ConnectionStringRef == "" {
		return ErrSQLCallMissingConnection
	}
	if config.Query == "" && len(config.Queries) == 0 {
		return ErrSQLCallMissingQuery
	}
	return nil
}

// loadQueryFromFile loads the query from a file if queryFile is specified.
func loadQueryFromFile(config *SQLCallConfig) error {
	if config.QueryFile == "" || config.Query != "" {
		return nil
	}

	// Validate file path to prevent path traversal attacks
	if !filepath.IsAbs(config.QueryFile) {
		if strings.Contains(config.QueryFile, "..") {
			return fmt.Errorf("query file path contains invalid '..' component: %s", config.QueryFile)
		}
	}

	queryBytes, err := os.ReadFile(config.QueryFile)
	if err != nil {
		return fmt.Errorf("reading query file %s: %w", config.QueryFile, err)
	}

	config.Query = string(queryBytes)
	return nil
}

// resolveMergeStrategy returns the merge strategy with default if empty.
func resolveMergeStrategy(strategy string) string {
	if strategy == "" {
		return defaultSQLCallStrategy
	}
	return strategy
}

// resolveOnError returns the onError behavior with default if empty.
func resolveOnError(onError string) string {
	if onError == "" {
		return defaultSQLCallOnError
	}
	return onError
}

// resolveTimeout returns the timeout duration with default if not specified.
func resolveTimeout(timeoutMs int) time.Duration {
	if timeoutMs > 0 {
		return time.Duration(timeoutMs) * time.Millisecond
	}
	return defaultSQLCallTimeout
}

// createDatabaseConnection creates and opens a database connection.
func createDatabaseConnection(config SQLCallConfig, timeout time.Duration) (*sql.DB, string, error) {
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

	db, driver, err := database.Open(dbConfig)
	if err != nil {
		return nil, "", fmt.Errorf("creating sql_call database connection: %w", err)
	}

	return db, driver, nil
}

// setupCache configures and returns the cache if enabled.
func setupCache(config SQLCallConfig) (cache.Cache, time.Duration, bool) {
	if !config.Cache.Enabled {
		return nil, 0, false
	}

	cacheSize := config.Cache.MaxSize
	if cacheSize <= 0 {
		cacheSize = defaultSQLCallCacheSize
	}

	cacheTTLSeconds := config.Cache.DefaultTTL
	if cacheTTLSeconds <= 0 {
		cacheTTLSeconds = defaultSQLCallCacheTTL
	}

	cacheTTL := time.Duration(cacheTTLSeconds) * time.Second
	lruCache := cache.NewLRUCache(cacheSize, cacheTTL)

	return lruCache, cacheTTL, true
}

// buildQueriesList builds the list of queries from config.
func buildQueriesList(config SQLCallConfig) []string {
	queries := config.Queries
	if config.Query != "" {
		queries = append([]string{config.Query}, queries...)
	}
	return queries
}

// logSQLCallModuleInitialization logs the module initialization details.
func logSQLCallModuleInitialization(driver string, queryCount int, mergeStrategy, onError string, cacheEnabled bool, timeout time.Duration) {
	logger.Debug("sql_call module initialized",
		slog.String("driver", driver),
		slog.Int("query_count", queryCount),
		slog.String("merge_strategy", mergeStrategy),
		slog.String("on_error", onError),
		slog.Bool("cache_enabled", cacheEnabled),
		slog.Duration("timeout", timeout),
	)
}

// ParseSQLCallConfig parses sql_call filter configuration from raw config map.
func ParseSQLCallConfig(cfg map[string]interface{}) (SQLCallConfig, error) {
	config := SQLCallConfig{}

	parseConnectionSettings(&config, cfg)
	parseQuerySettings(&config, cfg)
	parseResultHandlingSettings(&config, cfg)
	parseErrorHandlingSettings(&config, cfg)
	parsePoolSettings(&config, cfg)
	parseCacheSettings(&config, cfg)

	return config, nil
}

// parseConnectionSettings extracts connection-related settings from config.
func parseConnectionSettings(config *SQLCallConfig, cfg map[string]interface{}) {
	if v, ok := cfg["connectionString"].(string); ok {
		config.ConnectionString = v
	}
	if v, ok := cfg["connectionStringRef"].(string); ok {
		config.ConnectionStringRef = v
	}
	if v, ok := cfg["driver"].(string); ok {
		config.Driver = v
	}
}

// parseQuerySettings extracts query-related settings from config.
func parseQuerySettings(config *SQLCallConfig, cfg map[string]interface{}) {
	if v, ok := cfg["query"].(string); ok {
		config.Query = v
	}
	if v, ok := cfg["queryFile"].(string); ok {
		config.QueryFile = v
	}
	if v, ok := cfg["queries"].([]interface{}); ok {
		for _, q := range v {
			if qs, ok := q.(string); ok {
				config.Queries = append(config.Queries, qs)
			}
		}
	}
}

// parseResultHandlingSettings extracts result handling settings from config.
func parseResultHandlingSettings(config *SQLCallConfig, cfg map[string]interface{}) {
	if v, ok := cfg["mergeStrategy"].(string); ok {
		config.MergeStrategy = v
	}
	if v, ok := cfg["dataField"].(string); ok {
		config.DataField = v
	}
	if v, ok := cfg["resultKey"].(string); ok {
		config.ResultKey = v
	}
}

// parseErrorHandlingSettings extracts error handling settings from config.
func parseErrorHandlingSettings(config *SQLCallConfig, cfg map[string]interface{}) {
	if v, ok := cfg["onError"].(string); ok {
		config.OnError = v
	}
}

// parsePoolSettings extracts connection pool settings from config.
func parsePoolSettings(config *SQLCallConfig, cfg map[string]interface{}) {
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
}

// parseCacheSettings extracts cache settings from config.
func parseCacheSettings(config *SQLCallConfig, cfg map[string]interface{}) {
	if cacheRaw, ok := cfg["cache"].(map[string]interface{}); ok {
		config.Cache = parseSQLCallCacheConfig(cacheRaw)
	}
}

// parseSQLCallCacheConfig parses cache configuration.
func parseSQLCallCacheConfig(cfg map[string]interface{}) SQLCallCacheConfig {
	config := SQLCallCacheConfig{}

	if v, ok := cfg["enabled"].(bool); ok {
		config.Enabled = v
	}
	if v, ok := cfg["maxSize"].(float64); ok {
		config.MaxSize = int(v)
	}
	if v, ok := cfg["defaultTTL"].(float64); ok {
		config.DefaultTTL = int(v)
	}
	if v, ok := cfg["key"].(string); ok {
		config.Key = v
	}

	return config
}

// Process executes SQL queries for each input record and enriches them.
func (m *SQLCallModule) Process(ctx context.Context, records []map[string]interface{}) ([]map[string]interface{}, error) {
	if records == nil {
		return []map[string]interface{}{}, nil
	}

	startTime := time.Now()
	inputCount := len(records)

	logger.Debug("sql_call filter processing started",
		slog.String("module_type", "sql_call"),
		slog.Int("input_records", inputCount),
		slog.String("on_error", m.onError),
	)

	result := make([]map[string]interface{}, 0, len(records))
	skippedCount := 0
	errorCount := 0
	cacheHits := 0
	cacheMisses := 0

	for recordIdx, record := range records {
		select {
		case <-ctx.Done():
			return nil, ctx.Err()
		default:
		}

		enrichedRecord, wasCacheHit, err := m.processRecord(ctx, record, recordIdx)
		if wasCacheHit {
			cacheHits++
		} else if err == nil {
			cacheMisses++
		}

		if err != nil {
			errorCount++
			switch m.onError {
			case OnErrorFail:
				duration := time.Since(startTime)
				logger.Error("sql_call filter processing failed",
					slog.String("module_type", "sql_call"),
					slog.Int("record_index", recordIdx),
					slog.Duration("duration", duration),
					slog.String("error", err.Error()),
				)
				return nil, err
			case OnErrorSkip:
				skippedCount++
				logger.Warn("skipping record due to sql_call error",
					slog.String("module_type", "sql_call"),
					slog.Int("record_index", recordIdx),
					slog.String("error", err.Error()),
				)
				continue
			case OnErrorLog:
				logger.Error("sql_call error (continuing)",
					slog.String("module_type", "sql_call"),
					slog.Int("record_index", recordIdx),
					slog.String("error", err.Error()),
				)
				result = append(result, record)
				continue
			}
		}
		result = append(result, enrichedRecord)
	}

	duration := time.Since(startTime)
	outputCount := len(result)

	logger.Info("sql_call filter processing completed",
		slog.String("module_type", "sql_call"),
		slog.Int("input_records", inputCount),
		slog.Int("output_records", outputCount),
		slog.Int("skipped_records", skippedCount),
		slog.Int("error_count", errorCount),
		slog.Int("cache_hits", cacheHits),
		slog.Int("cache_misses", cacheMisses),
		slog.Duration("duration", duration),
	)

	return result, nil
}

// processRecord executes SQL queries for a single record.
func (m *SQLCallModule) processRecord(ctx context.Context, record map[string]interface{}, recordIdx int) (map[string]interface{}, bool, error) {
	// Compute cache key once from original record (before any mutations)
	var cacheKey string
	if m.cacheEnabled {
		cacheKey = m.buildCacheKey(record)
		if cachedData, found := m.cache.Get(cacheKey); found {
			responseData, ok := cachedData.(map[string]interface{})
			if ok {
				logger.Debug("sql_call cache hit",
					slog.Int("record_index", recordIdx),
				)
				return m.mergeData(record, responseData), true, nil
			}
		}
	}

	// Execute queries
	var resultData map[string]interface{}
	var err error
	workingRecord := record // Use a copy for intermediate merges

	for i, query := range m.queries {
		resultData, err = m.executeQuery(ctx, query, workingRecord)
		if err != nil {
			return nil, false, fmt.Errorf("query %d failed: %w", i+1, err)
		}

		// Merge intermediate results for subsequent queries
		if i < len(m.queries)-1 && resultData != nil {
			workingRecord = m.mergeData(workingRecord, resultData)
		}
	}

	// Cache successful result using original cache key
	if m.cacheEnabled && resultData != nil {
		m.cache.Set(cacheKey, resultData, m.cacheTTL)
	}

	// Merge final result
	enrichedRecord := m.mergeData(record, resultData)

	return enrichedRecord, false, nil
}

// executeQuery executes a single SQL query with record data.
func (m *SQLCallModule) executeQuery(ctx context.Context, queryTemplate string, record map[string]interface{}) (map[string]interface{}, error) {
	// Build parameterized query from template
	query, args, err := m.buildParameterizedQuery(queryTemplate, record)
	if err != nil {
		return nil, fmt.Errorf("building parameterized query: %w", err)
	}

	// Execute with timeout
	ctx, cancel := context.WithTimeout(ctx, m.timeout)
	defer cancel()

	rows, err := m.db.QueryContext(ctx, query, args...)
	if err != nil {
		dbErr := database.ClassifyDatabaseError(err, m.driver, "select", query, len(args))
		return nil, dbErr
	}
	defer func() {
		_ = rows.Close()
	}()

	// Convert rows to map
	records, err := m.rowsToRecords(rows)
	if err != nil {
		return nil, err
	}

	// Extract result
	if len(records) == 0 {
		return make(map[string]interface{}), nil
	}

	// If dataField is configured, extract from first record
	if m.dataField != "" && len(records) > 0 {
		if data, exists := getNestedValue(records[0], m.dataField); exists {
			if dataMap, ok := data.(map[string]interface{}); ok {
				return dataMap, nil
			}
		}
		return records[0], nil
	}

	// Return first record
	return records[0], nil
}

// buildParameterizedQuery builds a parameterized query from a template.
// Replaces {{record.field}} with positional parameters and returns args.
// Validates that all template placeholders are replaced to prevent SQL injection.
func (m *SQLCallModule) buildParameterizedQuery(queryTemplate string, record map[string]interface{}) (string, []interface{}, error) {
	query := queryTemplate
	var args []interface{}

	// Find and replace all {{record.field}} patterns
	paramIndex := 1
	for {
		start := strings.Index(query, "{{")
		if start == -1 {
			break
		}
		end := strings.Index(query[start:], "}}")
		if end == -1 {
			// Unmatched opening brace - potential SQL injection risk
			return "", nil, fmt.Errorf("unmatched template placeholder in query: missing closing }}")
		}
		end += start + 2

		// Extract field path
		placeholder := query[start:end]
		fieldPath := strings.TrimSpace(placeholder[2 : len(placeholder)-2])

		// Remove "record." prefix if present
		fieldPath = strings.TrimPrefix(fieldPath, RecordFieldPrefix)

		// Get value from record
		value := getSQLNestedValue(record, fieldPath)

		// Replace with positional parameter
		paramPlaceholder := database.FormatPlaceholder(m.driver, paramIndex)
		query = query[:start] + paramPlaceholder + query[end:]
		args = append(args, value)
		paramIndex++
	}

	// Validate no unmatched braces remain (security check)
	if strings.Contains(query, "{{") || strings.Contains(query, "}}") {
		return "", nil, fmt.Errorf("unmatched template placeholders remain in query after processing")
	}

	return query, args, nil
}

// getSQLNestedValue extracts a value from a nested map using dot notation.
// Uses a different name to avoid conflict with the filter package's getNestedValue.
func getSQLNestedValue(record map[string]interface{}, path string) interface{} {
	parts := strings.Split(path, ".")
	current := interface{}(record)

	for _, part := range parts {
		if m, ok := current.(map[string]interface{}); ok {
			current = m[part]
		} else {
			return nil
		}
	}

	return current
}

// rowsToRecords converts sql.Rows to a slice of maps.
func (m *SQLCallModule) rowsToRecords(rows *sql.Rows) ([]map[string]interface{}, error) {
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
			// Convert []byte to string
			if b, ok := val.([]byte); ok {
				val = string(b)
			}
			record[col] = val
		}

		records = append(records, record)
	}

	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterating rows: %w", err)
	}

	return records, nil
}

// buildCacheKey builds a cache key from record data.
func (m *SQLCallModule) buildCacheKey(record map[string]interface{}) string {
	if m.cacheKey != "" {
		// Evaluate template
		return m.templateEvaluator.Evaluate(m.cacheKey, record)
	}

	// Default: use query hash + first few field values
	return fmt.Sprintf("%s::%v", m.query, record)
}

// mergeData merges query result into the record.
func (m *SQLCallModule) mergeData(record, result map[string]interface{}) map[string]interface{} {
	if result == nil {
		return record
	}

	switch m.mergeStrategy {
	case "merge":
		return deepMerge(record, result)
	case "replace":
		merged := make(map[string]interface{})
		for k, v := range record {
			merged[k] = v
		}
		for k, v := range result {
			merged[k] = v
		}
		return merged
	case "append":
		merged := make(map[string]interface{})
		for k, v := range record {
			merged[k] = v
		}
		key := m.resultKey
		if key == "" {
			key = "_sql_result"
		}
		merged[key] = result
		return merged
	default:
		return deepMerge(record, result)
	}
}

// deepMerge performs a deep merge of two maps.
func deepMerge(a, b map[string]interface{}) map[string]interface{} {
	result := make(map[string]interface{})

	for k, v := range a {
		result[k] = v
	}

	for k, vb := range b {
		if va, exists := result[k]; exists {
			if mapA, okA := va.(map[string]interface{}); okA {
				if mapB, okB := vb.(map[string]interface{}); okB {
					result[k] = deepMerge(mapA, mapB)
					continue
				}
			}
		}
		result[k] = vb
	}

	return result
}

// GetCacheStats returns cache statistics.
func (m *SQLCallModule) GetCacheStats() cache.Stats {
	if m.cache != nil {
		return m.cache.Stats()
	}
	return cache.Stats{}
}

// ClearCache clears the cache.
func (m *SQLCallModule) ClearCache() {
	if m.cache != nil {
		m.cache.Clear()
	}
}

// Close releases resources.
func (m *SQLCallModule) Close() error {
	if m.db != nil {
		return m.db.Close()
	}
	return nil
}
