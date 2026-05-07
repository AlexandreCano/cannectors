// Package filter provides implementations for filter modules.
// SQLCall module executes SQL queries for record enrichment.
package filter

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"os"
	"sort"
	"strings"
	"time"

	"github.com/cannectors/runtime/internal/cache"
	"github.com/cannectors/runtime/internal/database"
	"github.com/cannectors/runtime/internal/errhandling"
	"github.com/cannectors/runtime/internal/logger"
	"github.com/cannectors/runtime/internal/moduleconfig"
	"github.com/cannectors/runtime/internal/pathutil"
	"github.com/cannectors/runtime/internal/recordpath"
	"github.com/cannectors/runtime/internal/template"
	"github.com/cannectors/runtime/pkg/connector"
)

// Default configuration values for sql_call module
const (
	defaultSQLCallTimeout   = 30 * time.Second
	defaultSQLCallCacheSize = 1000
	defaultSQLCallCacheTTL  = 300 // 5 minutes
	defaultSQLCallStrategy  = "merge"
)

// Template prefix constants
const (
	// RecordFieldPrefix is the prefix for record field access in templates
	RecordFieldPrefix = "record."
)

// Error types for sql_call module
var (
	ErrSQLCallMissingConnection = errors.New("sql_call connection configuration is required")
	ErrSQLCallMissingQuery      = errors.New("sql_call query is required")
)

// SQLCallConfig represents the configuration for a sql_call filter module.
type SQLCallConfig struct {
	connector.ModuleBase
	moduleconfig.SQLRequestBase

	// Result handling
	MergeStrategy string `json:"mergeStrategy"` // "merge", "replace", "append"
	ResultKey     string `json:"resultKey"`     // Key for storing result in "append" mode

	// Cache configuration
	Cache moduleconfig.CacheConfig `json:"cache"`
}

// SQLCallModule implements a filter that executes SQL queries for enrichment.
type SQLCallModule struct {
	db                *sql.DB
	driver            string
	query             string
	mergeStrategy     string
	resultKey         string
	onError           errhandling.OnErrorStrategy
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

	// Validate template syntax in query
	if err := template.ValidateSyntax(config.Query); err != nil {
		return nil, fmt.Errorf("invalid template syntax in sql_call query: %w", err)
	}

	// Validate cache key template syntax if configured
	if config.Cache.Key != "" {
		if err := template.ValidateSyntax(config.Cache.Key); err != nil {
			return nil, fmt.Errorf("invalid template syntax in sql_call cache key: %w", err)
		}
	}

	mergeStrategy := resolveMergeStrategy(config.MergeStrategy)
	onError := errhandling.ParseOnErrorStrategy(config.OnError)
	timeout := connector.GetTimeoutDuration(config.TimeoutMs, defaultSQLCallTimeout)

	db, driver, err := createDatabaseConnection(config, timeout)
	if err != nil {
		return nil, err
	}

	cache, cacheTTL, cacheEnabled := setupCache(config)

	module := &SQLCallModule{
		db:                db,
		driver:            driver,
		query:             config.Query,
		mergeStrategy:     mergeStrategy,
		resultKey:         config.ResultKey,
		onError:           onError,
		cache:             cache,
		cacheEnabled:      cacheEnabled,
		cacheTTL:          cacheTTL,
		cacheKey:          config.Cache.Key,
		timeout:           timeout,
		templateEvaluator: template.NewEvaluator(),
	}

	logSQLCallModuleInitialization(driver, mergeStrategy, onError, cacheEnabled, timeout)

	return module, nil
}

// validateSQLCallConfig validates that required configuration fields are present.
func validateSQLCallConfig(config SQLCallConfig) error {
	if config.ConnectionString == "" && config.ConnectionStringRef == "" {
		return ErrSQLCallMissingConnection
	}
	if config.Query == "" && config.QueryFile == "" {
		return ErrSQLCallMissingQuery
	}
	return nil
}

// loadQueryFromFile loads the query from a file if queryFile is specified.
func loadQueryFromFile(config *SQLCallConfig) error {
	if config.QueryFile == "" || config.Query != "" {
		return nil
	}

	if err := pathutil.ValidateFilePath(config.QueryFile); err != nil {
		return fmt.Errorf("query file path: %w", err)
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

// createDatabaseConnection creates and opens a database connection.
func createDatabaseConnection(config SQLCallConfig, timeout time.Duration) (*sql.DB, string, error) {
	dbConfig := database.Config{
		ConnectionString:    config.ConnectionString,
		ConnectionStringRef: config.ConnectionStringRef,
		Driver:              config.Driver,
		MaxOpenConns:        config.MaxOpenConns,
		MaxIdleConns:        config.MaxIdleConns,
		ConnMaxLifetime:     time.Duration(config.ConnMaxLifetimeSeconds) * time.Second,
		ConnMaxIdleTime:     time.Duration(config.ConnMaxIdleTimeSeconds) * time.Second,
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

// logSQLCallModuleInitialization logs the module initialization details.
func logSQLCallModuleInitialization(driver, mergeStrategy string, onError errhandling.OnErrorStrategy, cacheEnabled bool, timeout time.Duration) {
	logger.Debug("sql_call module initialized",
		slog.String("driver", driver),
		slog.String("merge_strategy", mergeStrategy),
		slog.String("on_error", string(onError)),
		slog.Bool("cache_enabled", cacheEnabled),
		slog.Duration("timeout", timeout),
	)
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
		slog.String("on_error", string(m.onError)),
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
			case errhandling.OnErrorFail:
				duration := time.Since(startTime)
				logger.Error("sql_call filter processing failed",
					slog.String("module_type", "sql_call"),
					slog.Int("record_index", recordIdx),
					slog.Duration("duration", duration),
					slog.String("error", err.Error()),
				)
				return nil, err
			case errhandling.OnErrorSkip:
				skippedCount++
				logger.Warn("skipping record due to sql_call error",
					slog.String("module_type", "sql_call"),
					slog.Int("record_index", recordIdx),
					slog.String("error", err.Error()),
				)
				continue
			case errhandling.OnErrorLog:
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

// processRecord executes the SQL query for a single record.
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

	// Execute query
	resultData, err := m.executeQuery(ctx, m.query, record)
	if err != nil {
		return nil, false, err
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
		value, _ := recordpath.Get(record, fieldPath)

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

	// Default: use query + sorted JSON-encoded record for deterministic key
	recordJSON, err := marshalDeterministic(record)
	if err != nil {
		logger.Warn("failed to marshal record for cache key, using query only",
			slog.String("error", err.Error()),
		)
		return m.query
	}
	return fmt.Sprintf("%s::%s", m.query, string(recordJSON))
}

// marshalDeterministic produces deterministic JSON by recursively sorting map
// keys. The standard library's json.Marshal already sorts top-level keys for
// map[string]T, but only when the value is itself a map of the same type.
// This helper normalises every nested map[string]interface{} (the canonical
// Go shape produced by JSON/YAML parsers) so the cache key is stable for
// records that carry the same data with different insertion order.
func marshalDeterministic(record map[string]interface{}) ([]byte, error) {
	return json.Marshal(canonicalize(record))
}

// canonicalize returns a value where every map[string]interface{} is replaced
// by an *orderedMap with sorted keys. Slices are walked recursively. Other
// values are returned unchanged. Combined with json.Marshal, this guarantees
// a byte-for-byte identical output for two semantically equal inputs.
func canonicalize(v interface{}) interface{} {
	switch t := v.(type) {
	case map[string]interface{}:
		keys := make([]string, 0, len(t))
		for k := range t {
			keys = append(keys, k)
		}
		sort.Strings(keys)
		out := make([][2]interface{}, len(keys))
		for i, k := range keys {
			out[i] = [2]interface{}{k, canonicalize(t[k])}
		}
		return orderedMap(out)
	case []interface{}:
		out := make([]interface{}, len(t))
		for i, item := range t {
			out[i] = canonicalize(item)
		}
		return out
	default:
		return v
	}
}

// orderedMap is a slice of (key, value) pairs that marshals to a JSON object
// while preserving the slice's order. Pre-sorted keys then yield deterministic
// output.
type orderedMap [][2]interface{}

func (m orderedMap) MarshalJSON() ([]byte, error) {
	var buf strings.Builder
	buf.WriteByte('{')
	for i, kv := range m {
		if i > 0 {
			buf.WriteByte(',')
		}
		key, err := json.Marshal(kv[0])
		if err != nil {
			return nil, err
		}
		buf.Write(key)
		buf.WriteByte(':')
		val, err := json.Marshal(kv[1])
		if err != nil {
			return nil, err
		}
		buf.Write(val)
	}
	buf.WriteByte('}')
	return []byte(buf.String()), nil
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
