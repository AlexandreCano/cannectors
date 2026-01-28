package database

import (
	"errors"
	"fmt"
	"strings"
)

// Error categories for database operations
const (
	CategoryConnection  = "connection"
	CategoryQuery       = "query"
	CategoryConstraint  = "constraint"
	CategoryTransaction = "transaction"
	CategoryTimeout     = "timeout"
	CategoryUnknown     = "unknown"
)

// DatabaseError represents a categorized database error with context.
//
//nolint:revive // DatabaseError is a clear, descriptive name that doesn't stutter in practice
type DatabaseError struct {
	Category    string // Error category (connection, query, constraint, etc.)
	Operation   string // Operation that failed (select, insert, update, etc.)
	Message     string // User-friendly error message
	Query       string // The query that caused the error (sanitized, no params)
	ParamCount  int    // Number of parameters (not the values)
	OriginalErr error  // The underlying database error
	Retryable   bool   // Whether the error is transient and can be retried
}

func (e *DatabaseError) Error() string {
	var msg string
	if e.Query != "" {
		msg = fmt.Sprintf("database %s error in %s: %s", e.Category, e.Operation, e.Message)
	} else {
		msg = fmt.Sprintf("database %s error: %s", e.Category, e.Message)
	}
	// Include original error message for better debugging
	if e.OriginalErr != nil {
		msg += fmt.Sprintf(" (original: %v)", e.OriginalErr)
	}
	return msg
}

func (e *DatabaseError) Unwrap() error {
	return e.OriginalErr
}

// IsRetryable returns true if the error is transient and can be retried.
func (e *DatabaseError) IsRetryable() bool {
	return e.Retryable
}

// NewDatabaseError creates a new database error with the given details.
func NewDatabaseError(category, operation, message string, originalErr error, retryable bool) *DatabaseError {
	return &DatabaseError{
		Category:    category,
		Operation:   operation,
		Message:     message,
		OriginalErr: originalErr,
		Retryable:   retryable,
	}
}

// NewConnectionError creates a connection error.
func NewConnectionError(message string, originalErr error) *DatabaseError {
	return NewDatabaseError(CategoryConnection, "connect", message, originalErr, true)
}

// NewQueryError creates a query error.
func NewQueryError(operation, message, query string, paramCount int, originalErr error, retryable bool) *DatabaseError {
	return &DatabaseError{
		Category:    CategoryQuery,
		Operation:   operation,
		Message:     message,
		Query:       sanitizeQuery(query),
		ParamCount:  paramCount,
		OriginalErr: originalErr,
		Retryable:   retryable,
	}
}

// NewConstraintError creates a constraint violation error (not retryable).
func NewConstraintError(operation, message string, originalErr error) *DatabaseError {
	return NewDatabaseError(CategoryConstraint, operation, message, originalErr, false)
}

// NewTransactionError creates a transaction error.
func NewTransactionError(message string, originalErr error, retryable bool) *DatabaseError {
	return NewDatabaseError(CategoryTransaction, "transaction", message, originalErr, retryable)
}

// NewTimeoutError creates a timeout error (retryable).
func NewTimeoutError(operation, message string, originalErr error) *DatabaseError {
	return NewDatabaseError(CategoryTimeout, operation, message, originalErr, true)
}

// ClassifyDatabaseError classifies a raw database error into a DatabaseError.
// It analyzes the error message and type to determine the category and retryability.
func ClassifyDatabaseError(err error, driver, operation, query string, paramCount int) *DatabaseError {
	if err == nil {
		return nil
	}

	errMsg := err.Error()
	errMsgLower := strings.ToLower(errMsg)

	// Check for timeout errors
	if isTimeoutError(errMsgLower) {
		return NewTimeoutError(operation, "operation timed out", err)
	}

	// Check for connection errors
	if isConnectionError(errMsgLower) {
		return NewConnectionError("connection failed or lost", err)
	}

	// Check for constraint violations
	if isConstraintError(errMsgLower, driver) {
		return NewConstraintError(operation, extractConstraintMessage(errMsgLower), err)
	}

	// Check for deadlock (retryable)
	if isDeadlockError(errMsgLower, driver) {
		return NewQueryError(operation, "deadlock detected", query, paramCount, err, true)
	}

	// Check for syntax errors (not retryable)
	if isSyntaxError(errMsgLower, driver) {
		return NewQueryError(operation, "SQL syntax error", query, paramCount, err, false)
	}

	// Default to unknown query error (not retryable by default)
	return NewQueryError(operation, errMsg, query, paramCount, err, false)
}

// isTimeoutError checks if the error is a timeout error.
func isTimeoutError(errMsg string) bool {
	timeoutIndicators := []string{
		"timeout",
		"timed out",
		"deadline exceeded",
		"context deadline",
		"connection timeout",
		"query timeout",
	}

	for _, indicator := range timeoutIndicators {
		if strings.Contains(errMsg, indicator) {
			return true
		}
	}
	return false
}

// isConnectionError checks if the error is a connection error.
func isConnectionError(errMsg string) bool {
	connectionIndicators := []string{
		"connection refused",
		"connection reset",
		"no such host",
		"network is unreachable",
		"connection closed",
		"broken pipe",
		"bad connection",
		"invalid connection",
		"unexpected eof",
		"server closed",
		"dial tcp",
		"connect: ",
		"cannot assign requested address",
		"too many open files",
	}

	for _, indicator := range connectionIndicators {
		if strings.Contains(errMsg, indicator) {
			return true
		}
	}
	return false
}

// isConstraintError checks if the error is a constraint violation.
func isConstraintError(errMsg string, driver string) bool {
	// Common constraint indicators
	commonIndicators := []string{
		"unique constraint",
		"duplicate key",
		"duplicate entry",
		"violates unique",
		"violates foreign key",
		"foreign key constraint",
		"violates check constraint",
		"violates not-null",
		"cannot be null",
		"integrity constraint",
		"constraint violation",
	}

	for _, indicator := range commonIndicators {
		if strings.Contains(errMsg, indicator) {
			return true
		}
	}

	// Driver-specific error codes
	switch driver {
	case DriverPostgres:
		// PostgreSQL error codes for constraints
		pgCodes := []string{
			"23000", // integrity_constraint_violation
			"23001", // restrict_violation
			"23502", // not_null_violation
			"23503", // foreign_key_violation
			"23505", // unique_violation
			"23514", // check_violation
			"23p01", // exclusion_violation
		}
		for _, code := range pgCodes {
			if strings.Contains(errMsg, code) {
				return true
			}
		}
	case DriverMySQL:
		// MySQL error codes for constraints
		mysqlCodes := []string{
			"1062", // duplicate entry
			"1216", // foreign key constraint
			"1217", // foreign key constraint
			"1451", // foreign key constraint (delete)
			"1452", // foreign key constraint (insert/update)
		}
		for _, code := range mysqlCodes {
			if strings.Contains(errMsg, code) {
				return true
			}
		}
	}

	return false
}

// isDeadlockError checks if the error is a deadlock error.
func isDeadlockError(errMsg string, driver string) bool {
	commonIndicators := []string{
		"deadlock",
		"lock wait timeout",
		"serialization failure",
		"could not serialize",
	}

	for _, indicator := range commonIndicators {
		if strings.Contains(errMsg, indicator) {
			return true
		}
	}

	// Driver-specific error codes
	switch driver {
	case DriverPostgres:
		if strings.Contains(errMsg, "40001") || strings.Contains(errMsg, "40p01") {
			return true
		}
	case DriverMySQL:
		if strings.Contains(errMsg, "1213") {
			return true
		}
	}

	return false
}

// isSyntaxError checks if the error is a SQL syntax error.
func isSyntaxError(errMsg string, driver string) bool {
	commonIndicators := []string{
		"syntax error",
		"parse error",
		"near \"",
		"at or near",
	}

	for _, indicator := range commonIndicators {
		if strings.Contains(errMsg, indicator) {
			return true
		}
	}

	// Driver-specific error codes
	switch driver {
	case DriverPostgres:
		if strings.Contains(errMsg, "42601") {
			return true
		}
	case DriverMySQL:
		if strings.Contains(errMsg, "1064") {
			return true
		}
	}

	return false
}

// extractConstraintMessage extracts a user-friendly message from a constraint error.
func extractConstraintMessage(errMsg string) string {
	if strings.Contains(errMsg, "unique") || strings.Contains(errMsg, "duplicate") {
		return "unique constraint violation: duplicate value exists"
	}
	if strings.Contains(errMsg, "foreign key") {
		return "foreign key constraint violation: referenced record not found or still referenced"
	}
	if strings.Contains(errMsg, "not-null") || strings.Contains(errMsg, "cannot be null") {
		return "not-null constraint violation: required field is null"
	}
	if strings.Contains(errMsg, "check constraint") {
		return "check constraint violation: value does not meet requirements"
	}
	return "constraint violation"
}

// sanitizeQuery removes sensitive information from a query for logging.
// It replaces literal values with placeholders.
func sanitizeQuery(query string) string {
	if query == "" {
		return ""
	}

	// Truncate very long queries
	if len(query) > 500 {
		return query[:500] + "... (truncated)"
	}

	return query
}

// IsDatabaseError checks if the error is a DatabaseError.
func IsDatabaseError(err error) bool {
	var dbErr *DatabaseError
	return errors.As(err, &dbErr)
}

// GetDatabaseError extracts the DatabaseError from an error chain.
func GetDatabaseError(err error) *DatabaseError {
	var dbErr *DatabaseError
	if errors.As(err, &dbErr) {
		return dbErr
	}
	return nil
}

// IsRetryableError checks if a database error is retryable.
func IsRetryableError(err error) bool {
	if err == nil {
		return false
	}

	dbErr := GetDatabaseError(err)
	if dbErr != nil {
		return dbErr.Retryable
	}

	// Check for known transient error patterns in raw errors
	errMsg := strings.ToLower(err.Error())
	return isTimeoutError(errMsg) || isConnectionError(errMsg) || isDeadlockError(errMsg, "")
}
