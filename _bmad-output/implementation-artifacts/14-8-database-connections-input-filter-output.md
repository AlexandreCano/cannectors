# Story 14.8: Database Connections (Input, Filter, Output)

Status: ready-for-dev

<!-- Note: Validation is optional. Run validate-create-story for quality check before dev-story. -->

## Story

As a developer,
I want to connect to databases for input, filtering, and output operations,
so that I can integrate with database systems as data sources, perform SQL queries in filters for enrichment, and write data to databases as destinations.

## Acceptance Criteria

1. **Given** I have a connector with Database Input module configured
   **When** The runtime executes the Input module
   **Then** The runtime connects to the configured database (PostgreSQL, MySQL, SQLite, etc.)
   **And** The runtime executes SQL queries to fetch data from the database
   **And** The runtime supports parameterized queries to prevent SQL injection
   **And** The runtime handles database connection errors gracefully
   **And** The runtime supports connection pooling for performance
   **And** The runtime supports CRON scheduling for periodic database polling
   **And** The runtime returns query results as records (`[]map[string]interface{}`)
   **And** The runtime handles pagination for large result sets
   **And** The runtime supports incremental queries using last timestamp or cursor-based pagination

2. **Given** I have a connector with SQL Call Filter module configured
   **When** The runtime executes the Filter module with input records
   **Then** The runtime connects to the configured database
   **And** The runtime executes SQL queries using data from input records (parameterized queries)
   **And** The runtime supports templating in SQL queries using record data (e.g., `SELECT * FROM users WHERE id = {{record.user_id}}`)
   **And** The runtime merges query results with input records (enrichment pattern)
   **And** The runtime supports caching of query results to avoid redundant database calls
   **And** The runtime handles database connection errors without failing the entire pipeline
   **And** The runtime supports multiple SQL queries per filter (sequential execution)
   **And** The runtime supports conditional SQL execution based on record data

3. **Given** I have a connector with Database Output module configured
   **When** The runtime executes the Output module with transformed records
   **Then** The runtime connects to the configured database
   **And** The runtime executes INSERT, UPDATE, or UPSERT operations based on configuration
   **And** The runtime supports batch operations for performance (bulk insert/update)
   **And** The runtime handles database constraint violations gracefully (unique constraints, foreign keys)
   **And** The runtime supports transaction management (commit on success, rollback on error)
   **And** The runtime supports upsert operations (INSERT ... ON CONFLICT UPDATE)
   **And** The runtime maps record fields to database columns based on configuration
   **And** The runtime handles database connection errors and retries transient failures
   **And** The runtime supports both single record and batch record modes

4. **Given** I have a connector with database modules configured
   **When** The runtime validates the connector configuration
   **Then** The configuration schema supports database connection parameters (host, port, database, username, password)
   **And** The configuration schema supports SQL query definitions for input and filter modules
   **And** The configuration schema supports table and column mappings for output module
   **And** The configuration validator checks database connection parameters are valid
   **And** The configuration validator checks SQL query syntax is valid (basic validation)
   **And** The configuration supports secure credential storage (environment variables, secret management)
   **And** The configuration supports connection pooling parameters (max connections, timeout, etc.)

5. **Given** I have a connector with database modules using credentials
   **When** The runtime executes database operations
   **Then** Credentials are retrieved from secure storage (environment variables, secret files)
   **And** Credentials are never logged or exposed in error messages
   **And** Database connections use encrypted connections (TLS/SSL) when supported
   **And** Credential validation occurs at connection time with clear error messages
   **And** The runtime supports different authentication methods (password, certificate, etc.)

6. **Given** I have a connector with database modules
   **When** Database operations encounter errors
   **Then** The runtime categorizes errors (connection errors, query errors, constraint violations)
   **And** The runtime applies retry logic for transient errors (connection timeouts, deadlocks)
   **And** The runtime does not retry fatal errors (syntax errors, constraint violations)
   **And** The runtime logs errors with context (query, parameters, error type) without exposing sensitive data
   **And** The runtime provides clear error messages for configuration issues
   **And** The runtime handles database-specific error codes appropriately

7. **Given** I have a connector with database modules
   **When** The runtime executes database operations
   **Then** The runtime supports multiple database drivers (PostgreSQL via `pgx`, MySQL via `go-sql-driver/mysql`, SQLite via `modernc.org/sqlite`)
   **And** The runtime detects the database type from connection string or configuration
   **And** The runtime uses appropriate SQL dialect for each database type
   **And** The runtime handles database-specific features (PostgreSQL arrays, MySQL JSON, etc.)
   **And** The runtime supports connection string format: `postgres://user:pass@host:port/db?sslmode=require`

## Tasks / Subtasks

- [ ] Task 1: Design database module architecture and interfaces (AC: #1, #2, #3, #4)
  - [ ] Define database connection interface and abstraction
  - [ ] Design database input module interface (extends `input.Module`)
  - [ ] Design SQL call filter module interface (extends `filter.Module`)
  - [ ] Design database output module interface (extends `output.Module`)
  - [ ] Design database connection configuration schema
  - [ ] Design SQL query configuration schema
  - [ ] Design credential management approach
  - [ ] Document database module architecture

- [ ] Task 2: Implement database connection manager (AC: #1, #2, #3, #4, #5)
  - [ ] Create database connection pool manager
  - [ ] Support multiple database drivers (PostgreSQL, MySQL, SQLite)
  - [ ] Implement connection string parsing and validation
  - [ ] Implement connection pooling with configurable parameters
  - [ ] Implement secure credential retrieval (env vars, secret files)
  - [ ] Implement TLS/SSL connection support
  - [ ] Implement connection health checks
  - [ ] Add unit tests for connection manager

- [ ] Task 3: Implement Database Input module (AC: #1, #4, #5, #6, #7)
  - [ ] Create `internal/modules/input/database.go` implementing `input.Module`
  - [ ] Implement SQL query execution with parameterized queries
  - [ ] Implement result set conversion to records (`[]map[string]interface{}`)
  - [ ] Implement pagination support (LIMIT/OFFSET or cursor-based)
  - [ ] Implement incremental query support (last timestamp, cursor)
  - [ ] Integrate with CRON scheduler for periodic polling
  - [ ] Implement connection error handling and retries
  - [ ] Add integration tests with test databases
  - [ ] Register module in registry

- [ ] Task 4: Implement SQL Call Filter module (AC: #2, #4, #5, #6, #7)
  - [ ] Create `internal/modules/filter/sql_call.go` implementing `filter.Module`
  - [ ] Implement SQL query execution with record data templating
  - [ ] Implement parameterized query construction from record data
  - [ ] Implement result merging with input records (enrichment)
  - [ ] Integrate with cache system (reuse from enrichment filter)
  - [ ] Support multiple SQL queries per filter (sequential)
  - [ ] Support conditional SQL execution
  - [ ] Implement connection error handling (non-fatal)
  - [ ] Add integration tests with test databases
  - [ ] Register module in registry

- [ ] Task 5: Implement Database Output module (AC: #3, #4, #5, #6, #7)
  - [ ] Create `internal/modules/output/database.go` implementing `output.Module`
  - [ ] Implement INSERT operation with record-to-column mapping
  - [ ] Implement UPDATE operation with WHERE clause from record data
  - [ ] Implement UPSERT operation (INSERT ... ON CONFLICT)
  - [ ] Implement batch operations (bulk insert/update)
  - [ ] Implement transaction management (begin, commit, rollback)
  - [ ] Implement constraint violation handling
  - [ ] Support both single record and batch record modes
  - [ ] Implement connection error handling and retries
  - [ ] Add integration tests with test databases
  - [ ] Register module in registry

- [ ] Task 6: Add database configuration to pipeline schema (AC: #4)
  - [ ] Add database connection configuration to `pipeline-schema.json`
  - [ ] Add SQL query configuration for input module
  - [ ] Add SQL query configuration for filter module
  - [ ] Add table/column mapping configuration for output module
  - [ ] Add connection pooling configuration
  - [ ] Add credential configuration (env var references)
  - [ ] Update schema validator to check database configurations
  - [ ] Document database configuration options

- [ ] Task 7: Implement SQL templating support (AC: #2, #3)
  - [ ] Integrate SQL query templating with existing template engine
  - [ ] Support record data access in SQL queries (e.g., `{{record.field}}`)
  - [ ] Support parameterized query construction from templates
  - [ ] Validate SQL template syntax (basic validation)
  - [ ] Handle SQL injection prevention (parameterized queries only)
  - [ ] Add unit tests for SQL templating
  - [ ] Document SQL templating syntax and examples

- [ ] Task 8: Implement database error handling and retry logic (AC: #6)
  - [ ] Categorize database errors (connection, query, constraint, etc.)
  - [ ] Implement retry logic for transient errors
  - [ ] Implement error logging with context (without sensitive data)
  - [ ] Handle database-specific error codes
  - [ ] Provide clear error messages for configuration issues
  - [ ] Add unit tests for error handling
  - [ ] Document error handling behavior

- [ ] Task 9: Add comprehensive tests for database modules (AC: #1, #2, #3, #4, #5, #6, #7)
  - [ ] Set up test databases (PostgreSQL, MySQL, SQLite) using Docker or testcontainers
  - [ ] Test database input module with various SQL queries
  - [ ] Test SQL call filter module with enrichment patterns
  - [ ] Test database output module with INSERT, UPDATE, UPSERT
  - [ ] Test connection pooling and connection management
  - [ ] Test credential management and security
  - [ ] Test error handling and retry logic
  - [ ] Test pagination and incremental queries
  - [ ] Test batch operations
  - [ ] Test transaction management
  - [ ] Test SQL templating with record data
  - [ ] Test multiple database drivers

- [ ] Task 10: Update documentation and examples (AC: #4, #5, #6, #7)
  - [ ] Document database modules in README.md
  - [ ] Create example configurations for database input module
  - [ ] Create example configurations for SQL call filter module
  - [ ] Create example configurations for database output module
  - [ ] Document database connection configuration
  - [ ] Document SQL query configuration
  - [ ] Document credential management
  - [ ] Document error handling and troubleshooting
  - [ ] Add database module examples to `configs/examples/`

## Dev Notes

### Relevant Architecture Patterns and Constraints

**Database Module Design:**
- Database modules follow the same interface pattern as HTTP modules (`input.Module`, `filter.Module`, `output.Module`)
- Database connections are managed through a connection pool manager
- Database modules support multiple database drivers (PostgreSQL, MySQL, SQLite)
- Database operations use parameterized queries to prevent SQL injection
- Database credentials are stored securely (environment variables, secret files)

**Database Connection Management:**
- Connection pooling is managed centrally to avoid connection leaks
- Connections are reused across module executions when possible
- Connection health checks ensure connections are valid before use
- TLS/SSL connections are supported for secure database access
- Connection timeouts and retry logic handle transient connection failures

**SQL Query Execution:**
- All SQL queries use parameterized queries (prepared statements)
- SQL templating allows dynamic query construction using record data
- Query results are converted to `map[string]interface{}` records
- Pagination supports both LIMIT/OFFSET and cursor-based approaches
- Incremental queries use last timestamp or cursor for efficient polling

**Database Input Module:**
- Executes SQL queries to fetch data from databases
- Supports CRON scheduling for periodic polling
- Returns query results as `[]map[string]interface{}` records
- Supports pagination for large result sets
- Supports incremental queries using last timestamp or cursor

**SQL Call Filter Module:**
- Executes SQL queries using data from input records
- Supports templating in SQL queries (e.g., `WHERE id = {{record.user_id}}`)
- Merges query results with input records (enrichment pattern)
- Supports caching to avoid redundant database calls
- Handles connection errors gracefully (non-fatal)

**Database Output Module:**
- Executes INSERT, UPDATE, or UPSERT operations
- Supports batch operations for performance
- Supports transaction management (commit on success, rollback on error)
- Maps record fields to database columns based on configuration
- Handles constraint violations gracefully

**Credential Management:**
- Credentials are retrieved from environment variables or secret files
- Credentials are never logged or exposed in error messages
- Database connections use encrypted connections (TLS/SSL) when supported
- Credential validation occurs at connection time

**Error Handling:**
- Database errors are categorized (connection, query, constraint, etc.)
- Retry logic applies to transient errors (connection timeouts, deadlocks)
- Fatal errors (syntax errors, constraint violations) are not retried
- Errors are logged with context without exposing sensitive data

### Project Structure Notes

**Files to Create:**
- `internal/database/connection.go` - Database connection manager
- `internal/database/connection_test.go` - Connection manager tests
- `internal/modules/input/database.go` - Database input module
- `internal/modules/input/database_test.go` - Database input tests
- `internal/modules/filter/sql_call.go` - SQL call filter module
- `internal/modules/filter/sql_call_test.go` - SQL call filter tests
- `internal/modules/output/database.go` - Database output module
- `internal/modules/output/database_test.go` - Database output tests
- `configs/examples/25-database-input.yaml` - Example with database input
- `configs/examples/26-sql-call-filter.yaml` - Example with SQL call filter
- `configs/examples/27-database-output.yaml` - Example with database output

**Files to Modify:**
- `internal/registry/registry.go` - Register database modules
- `internal/config/schema/pipeline-schema.json` - Add database configuration schemas
- `internal/config/validator.go` - Add database configuration validation
- `README.md` - Document database modules
- `go.mod` - Add database driver dependencies

**New Dependencies:**
```go
// PostgreSQL driver
github.com/jackc/pgx/v5

// MySQL driver
github.com/go-sql-driver/mysql

// SQLite driver
modernc.org/sqlite

// Optional: Database connection testing
github.com/testcontainers/testcontainers-go
```

**Database Driver Selection:**
- PostgreSQL: `pgx/v5` (recommended, high performance)
- MySQL: `go-sql-driver/mysql` (standard, widely used)
- SQLite: `modernc.org/sqlite` (pure Go, no CGO)

**Connection String Format:**
- PostgreSQL: `postgres://user:pass@host:port/db?sslmode=require`
- MySQL: `user:pass@tcp(host:port)/db?tls=true`
- SQLite: `file:path/to/database.db`

**SQL Query Configuration:**
- Input module: `query` field with SQL SELECT statement
- Filter module: `query` field with SQL SELECT statement (can use templating)
- Output module: `table` field with table name, `operation` field (insert/update/upsert), `mapping` field for column mapping

### References

- [Source: internal/modules/input/http_polling.go] - HTTP polling input module (reference for polling patterns)
- [Source: internal/modules/filter/enrichment.go] - Enrichment filter module (reference for enrichment and caching patterns)
- [Source: internal/modules/output/http_request.go] - HTTP request output module (reference for output patterns)
- [Source: internal/runtime/pipeline.go] - Pipeline execution engine (reference for module integration)
- [Source: _bmad-output/implementation-artifacts/14-4-dynamic-enrichment-inside-filters-input-inside-filter-cache.md] - Enrichment and caching patterns
- [Source: _bmad-output/implementation-artifacts/14-5-last-timestamp-persistence-for-polling-inputs.md] - State persistence patterns (reference for incremental queries)
- [Source: _bmad-output/planning-artifacts/architecture.md] - Architecture patterns and constraints
- [Source: _bmad-output/planning-artifacts/prd.md] - PRD mentions SQL Query Input as post-MVP (FR66, FR68)
- [Source: _bmad-output/project-context.md] - Project context and critical rules

### Previous Story Intelligence

**From Story 14.7 (Record Metadata Storage):**
- Metadata storage patterns
- Record structure and field access patterns

**From Story 14.6 (Output Templating):**
- Templating syntax and evaluation patterns
- Template variable access patterns
- Integration with output module

**From Story 14.5 (Last Timestamp Persistence):**
- State persistence patterns
- Incremental query patterns
- Thread-safe operations

**From Story 14.4 (Dynamic Enrichment Inside Filters):**
- Cache implementation patterns
- Filter module integration patterns
- Record transformation patterns

**From Story 14.2 (Script Filter Module):**
- Filter module implementation patterns
- Module registration patterns

**Git Intelligence:**
- Recent commits show focus on filter modules (enrichment, script, logging)
- Pattern: Create new packages for new features (e.g., `internal/cache/`, `internal/persistence/`)
- Pattern: Comprehensive test coverage (unit tests + integration tests)
- Pattern: Update pipeline schema for new features
- Pattern: Create example configurations for new features

### Latest Technical Information

**Go Database Drivers:**
- PostgreSQL: `github.com/jackc/pgx/v5` - High performance, native protocol
- MySQL: `github.com/go-sql-driver/mysql` - Standard driver, widely used
- SQLite: `modernc.org/sqlite` - Pure Go, no CGO dependencies

**Database Connection Patterns:**
- Use `database/sql` interface for database-agnostic code
- Use driver-specific features when needed (e.g., `pgx` for PostgreSQL-specific features)
- Connection pooling via `sql.DB` with `SetMaxOpenConns`, `SetMaxIdleConns`, `SetConnMaxLifetime`
- Use context for query timeouts and cancellation

**SQL Injection Prevention:**
- Always use parameterized queries (prepared statements)
- Never concatenate user input into SQL queries
- Use `?` placeholders for MySQL/SQLite, `$1, $2, ...` for PostgreSQL
- Validate and sanitize SQL template variables before use

**Transaction Management:**
- Use `BeginTx` for transactions with context support
- Always defer `Rollback()` and call `Commit()` on success
- Handle transaction errors appropriately
- Support nested transactions if needed (savepoints)

**Batch Operations:**
- Use `Exec` with multiple values for bulk inserts
- Use transactions for batch operations to ensure atomicity
- Consider batch size limits for large datasets
- Support both single record and batch record modes

**Error Handling:**
- Categorize errors: connection errors (retryable), query errors (may be retryable), constraint violations (not retryable)
- Use `errors.Is` and `errors.As` for error type checking
- Log errors with context (query type, parameters count) without sensitive data
- Provide clear error messages for configuration issues

## Dev Agent Record

### Agent Model Used

### Debug Log References

### Completion Notes List

### File List
