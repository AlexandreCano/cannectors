// Package output provides implementations for output modules.
// DatabaseOutput module writes records to databases using SQL operations.
package output

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"log/slog"
	"os"
	"strings"
	"time"

	"github.com/cannectors/runtime/internal/database"
	"github.com/cannectors/runtime/internal/logger"
	"github.com/cannectors/runtime/internal/moduleconfig"
	"github.com/cannectors/runtime/internal/pathutil"
	"github.com/cannectors/runtime/internal/template"
	"github.com/cannectors/runtime/pkg/connector"
)

// Default configuration values for database output
const (
	defaultDatabaseOutputTimeout = 30 * time.Second
)

// Template prefix constants
const (
	// RecordFieldPrefix is the prefix for record field access in templates
	RecordFieldPrefix = "record."
)

// Error types for database output module
var (
	ErrDatabaseOutputNilConfig      = errors.New("database output configuration is nil")
	ErrDatabaseOutputMissingConnStr = errors.New("connection string is required for database output")
	ErrDatabaseOutputMissingQuery   = errors.New("query or queryFile is required for database output")
)

// DatabaseOutputConfig holds configuration for the database output module.
type DatabaseOutputConfig struct {
	connector.ModuleBase
	moduleconfig.SQLRequestBase

	// Transaction configuration
	Transaction bool `json:"transaction"` // Wrap operations in transaction
}

// DatabaseOutput implements a database output module.
type DatabaseOutput struct {
	db      *sql.DB
	driver  string
	config  DatabaseOutputConfig
	timeout time.Duration
}

// NewDatabaseOutputFromConfig creates a new database output module from configuration.
func NewDatabaseOutputFromConfig(cfg *connector.ModuleConfig) (*DatabaseOutput, error) {
	if cfg == nil {
		return nil, ErrDatabaseOutputNilConfig
	}

	config, err := moduleconfig.ParseModuleConfig[DatabaseOutputConfig](*cfg)
	if err != nil {
		return nil, err
	}

	// Validate required fields
	if config.ConnectionString == "" && config.ConnectionStringRef == "" {
		return nil, ErrDatabaseOutputMissingConnStr
	}

	// Load query from file if queryFile is specified
	if config.QueryFile != "" && config.Query == "" {
		if err := pathutil.ValidateFilePath(config.QueryFile); err != nil {
			return nil, fmt.Errorf("query file path: %w", err)
		}
		queryBytes, readErr := os.ReadFile(config.QueryFile)
		if readErr != nil {
			return nil, fmt.Errorf("reading query file %s: %w", config.QueryFile, readErr)
		}
		config.Query = string(queryBytes)
	}

	// Validate query is present
	if config.Query == "" {
		return nil, ErrDatabaseOutputMissingQuery
	}

	// Validate template syntax in query
	if err := template.ValidateSyntax(config.Query); err != nil {
		return nil, fmt.Errorf("invalid template syntax in database output query: %w", err)
	}

	// Set defaults
	timeout := connector.GetTimeoutDuration(config.TimeoutMs, defaultDatabaseOutputTimeout)

	if config.OnError == "" {
		config.OnError = "fail"
	}

	// Create database config
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

	// Open database connection
	db, driver, err := database.Open(dbConfig)
	if err != nil {
		return nil, fmt.Errorf("creating database output connection: %w", err)
	}

	module := &DatabaseOutput{
		db:      db,
		driver:  driver,
		config:  config,
		timeout: timeout,
	}

	logger.Debug("database output module created",
		slog.String("driver", driver),
		slog.Bool("transaction", config.Transaction),
		slog.String("on_error", config.OnError),
	)

	return module, nil
}

// Send writes records to the database.
// Returns the number of records successfully processed and any error.
func (d *DatabaseOutput) Send(ctx context.Context, records []map[string]interface{}) (int, error) {
	if len(records) == 0 {
		return 0, nil
	}

	startTime := time.Now()

	logger.Info("database output send started",
		slog.String("module_type", "database"),
		slog.Int("record_count", len(records)),
		slog.Bool("transaction", d.config.Transaction),
	)

	var err error
	var sentCount int

	if d.config.Transaction {
		sentCount, err = d.sendWithTransaction(ctx, records)
	} else {
		sentCount, err = d.sendWithoutTransaction(ctx, records)
	}

	duration := time.Since(startTime)

	if err != nil {
		logger.Error("database output send failed",
			slog.String("module_type", "database"),
			slog.Duration("duration", duration),
			slog.Int("sent_count", sentCount),
			slog.String("error", err.Error()),
		)
		return sentCount, err
	}

	logger.Info("database output send completed",
		slog.String("module_type", "database"),
		slog.Int("record_count", len(records)),
		slog.Int("sent_count", sentCount),
		slog.Duration("duration", duration),
	)

	return sentCount, nil
}

// sendWithTransaction executes queries within a transaction.
func (d *DatabaseOutput) sendWithTransaction(ctx context.Context, records []map[string]interface{}) (int, error) {
	tx, err := d.db.BeginTx(ctx, nil)
	if err != nil {
		return 0, fmt.Errorf("beginning transaction: %w", err)
	}

	defer func() {
		if r := recover(); r != nil {
			_ = tx.Rollback()
			panic(r)
		}
	}()

	successCount := 0
	for i, record := range records {
		processed, err := d.processRecordInTransaction(ctx, tx, record, i)
		if err != nil {
			_ = tx.Rollback()
			return successCount, err
		}
		if processed {
			successCount++
		}
	}

	if err := tx.Commit(); err != nil {
		return successCount, fmt.Errorf("committing transaction: %w", err)
	}

	return successCount, nil
}

// processRecordInTransaction processes a single record within a transaction.
// Returns true if the record was successfully processed, false if skipped, and an error if processing should stop.
func (d *DatabaseOutput) processRecordInTransaction(ctx context.Context, tx *sql.Tx, record map[string]interface{}, recordIndex int) (bool, error) {
	query, args, err := d.buildParameterizedQuery(d.config.Query, record)
	if err != nil {
		return d.handleQueryBuildError(err, recordIndex)
	}

	queryCtx, cancel := context.WithTimeout(ctx, d.timeout)
	defer cancel()

	_, err = tx.ExecContext(queryCtx, query, args...)
	if err != nil {
		return d.handleDatabaseError(err, query, len(args), recordIndex)
	}

	return true, nil
}

// handleQueryBuildError handles errors during query building based on onError configuration.
//
//nolint:unparam // bool return is needed for interface consistency, even though it's always false for log/skip
func (d *DatabaseOutput) handleQueryBuildError(err error, recordIndex int) (bool, error) {
	switch d.config.OnError {
	case "skip":
		logger.Warn("skipping record due to query build error",
			slog.Int("record_index", recordIndex),
			slog.String("error", err.Error()),
		)
		return false, nil
	case "log":
		logger.Error("query build error (continuing)",
			slog.Int("record_index", recordIndex),
			slog.String("error", err.Error()),
		)
		return false, nil
	default: // "fail"
		return false, fmt.Errorf("building parameterized query: %w", err)
	}
}

// handleDatabaseError handles database execution errors based on onError configuration.
//
//nolint:unparam // bool return is needed for interface consistency, even though it's always false for log/skip
func (d *DatabaseOutput) handleDatabaseError(err error, query string, argCount int, recordIndex int) (bool, error) {
	dbErr := database.ClassifyDatabaseError(err, d.driver, "exec", query, argCount)

	switch d.config.OnError {
	case "skip":
		logger.Warn("skipping record due to database error",
			slog.Int("record_index", recordIndex),
			slog.String("error", dbErr.Error()),
		)
		return false, nil
	case "log":
		logger.Error("database error (continuing)",
			slog.Int("record_index", recordIndex),
			slog.String("error", dbErr.Error()),
		)
		return false, nil
	default: // "fail"
		return false, dbErr
	}
}

// sendWithoutTransaction executes queries without a transaction.
func (d *DatabaseOutput) sendWithoutTransaction(ctx context.Context, records []map[string]interface{}) (int, error) {
	successCount := 0
	for i, record := range records {
		processed, err := d.processRecordWithoutTransaction(ctx, record, i)
		if err != nil {
			return successCount, err
		}
		if processed {
			successCount++
		}
	}
	return successCount, nil
}

// processRecordWithoutTransaction processes a single record without a transaction.
// Returns true if the record was successfully processed, false if skipped, and an error if processing should stop.
func (d *DatabaseOutput) processRecordWithoutTransaction(ctx context.Context, record map[string]interface{}, recordIndex int) (bool, error) {
	query, args, err := d.buildParameterizedQuery(d.config.Query, record)
	if err != nil {
		return d.handleQueryBuildError(err, recordIndex)
	}

	queryCtx, cancel := context.WithTimeout(ctx, d.timeout)
	defer cancel()

	_, err = d.db.ExecContext(queryCtx, query, args...)
	if err != nil {
		return d.handleDatabaseError(err, query, len(args), recordIndex)
	}

	return true, nil
}

// buildParameterizedQuery builds a parameterized query from a template.
// Replaces {{record.field}} placeholders with parameterized values.
// Validates that all template placeholders are replaced to prevent SQL injection.
func (d *DatabaseOutput) buildParameterizedQuery(queryTemplate string, record map[string]interface{}) (string, []interface{}, error) {
	query := queryTemplate
	var args []interface{}

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

		placeholder := query[start:end]
		fieldPath := strings.TrimSpace(placeholder[2 : len(placeholder)-2])
		fieldPath = strings.TrimPrefix(fieldPath, RecordFieldPrefix)

		value := getDBFieldValue(record, fieldPath)

		paramPlaceholder := database.FormatPlaceholder(d.driver, paramIndex)
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

// getDBFieldValue extracts a field value from a record using dot notation.
func getDBFieldValue(record map[string]interface{}, field string) interface{} {
	parts := strings.Split(field, ".")
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

// Close releases resources.
func (d *DatabaseOutput) Close() error {
	if d.db != nil {
		return d.db.Close()
	}
	return nil
}
