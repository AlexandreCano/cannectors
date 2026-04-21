//go:build integration

package modules

import (
	"context"
	"database/sql"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/cannectors/runtime/internal/moduleconfig"
	"github.com/cannectors/runtime/internal/modules/filter"
	"github.com/cannectors/runtime/internal/modules/input"
	"github.com/cannectors/runtime/internal/modules/output"
	"github.com/cannectors/runtime/pkg/connector"

	_ "modernc.org/sqlite"
)

// setupTestDB creates an in-memory SQLite database with test schema
func setupTestDB(t *testing.T) *sql.DB {
	t.Helper()
	db, err := sql.Open("sqlite", ":memory:")
	if err != nil {
		t.Fatalf("Failed to open SQLite: %v", err)
	}

	// Create test tables
	schema := `
		CREATE TABLE users (
			id INTEGER PRIMARY KEY,
			name TEXT NOT NULL,
			email TEXT UNIQUE,
			created_at DATETIME DEFAULT CURRENT_TIMESTAMP,
			updated_at DATETIME DEFAULT CURRENT_TIMESTAMP
		);

		CREATE TABLE orders (
			id INTEGER PRIMARY KEY,
			user_id INTEGER NOT NULL,
			total REAL NOT NULL,
			status TEXT DEFAULT 'pending',
			created_at DATETIME DEFAULT CURRENT_TIMESTAMP,
			FOREIGN KEY (user_id) REFERENCES users(id)
		);

		CREATE TABLE products (
			id INTEGER PRIMARY KEY,
			sku TEXT UNIQUE NOT NULL,
			name TEXT NOT NULL,
			price REAL NOT NULL,
			stock INTEGER DEFAULT 0
		);

		CREATE TABLE audit_log (
			id INTEGER PRIMARY KEY,
			entity_type TEXT NOT NULL,
			entity_id INTEGER NOT NULL,
			action TEXT NOT NULL,
			timestamp DATETIME DEFAULT CURRENT_TIMESTAMP
		);

		-- Seed test data
		INSERT INTO users (id, name, email) VALUES 
			(1, 'Alice', 'alice@example.com'),
			(2, 'Bob', 'bob@example.com'),
			(3, 'Charlie', 'charlie@example.com');

		INSERT INTO orders (id, user_id, total, status) VALUES
			(101, 1, 99.99, 'completed'),
			(102, 1, 149.50, 'pending'),
			(103, 2, 75.00, 'completed');

		INSERT INTO products (sku, name, price, stock) VALUES
			('SKU001', 'Widget', 9.99, 100),
			('SKU002', 'Gadget', 19.99, 50);
	`

	_, err = db.Exec(schema)
	if err != nil {
		t.Fatalf("Failed to create schema: %v", err)
	}

	return db
}

// createTempSQLFile creates a temporary SQL file for queryFile tests
func createTempSQLFile(t *testing.T, content string) string {
	t.Helper()
	tmpDir := t.TempDir()
	sqlFile := filepath.Join(tmpDir, "query.sql")
	err := os.WriteFile(sqlFile, []byte(content), 0644)
	if err != nil {
		t.Fatalf("Failed to create SQL file: %v", err)
	}
	return sqlFile
}

// TestDatabaseInputBasicQuery tests basic SELECT query execution
func TestDatabaseInputBasicQuery(t *testing.T) {
	t.Parallel()

	tmpFile := filepath.Join(t.TempDir(), "test.db")
	db, err := sql.Open("sqlite", tmpFile)
	if err != nil {
		t.Fatalf("Failed to create test db: %v", err)
	}
	defer db.Close()

	// Setup test data
	_, err = db.Exec(`
		CREATE TABLE users (id INTEGER PRIMARY KEY, name TEXT, email TEXT);
		INSERT INTO users (id, name, email) VALUES (1, 'Alice', 'alice@test.com');
		INSERT INTO users (id, name, email) VALUES (2, 'Bob', 'bob@test.com');
	`)
	if err != nil {
		t.Fatalf("Failed to setup test data: %v", err)
	}
	db.Close()

	// Test database input module
	cfg := &connector.ModuleConfig{
		Type: "database",
		Config: map[string]interface{}{
			"connectionString": "file:" + tmpFile,
			"driver":           "sqlite",
			"query":            "SELECT id, name, email FROM users ORDER BY id",
		},
	}

	inputModule, err := input.NewDatabaseInputFromConfig(cfg)
	if err != nil {
		t.Fatalf("Failed to create input module: %v", err)
	}
	defer inputModule.Close()

	ctx := context.Background()
	records, err := inputModule.Fetch(ctx)
	if err != nil {
		t.Fatalf("Fetch failed: %v", err)
	}

	if len(records) != 2 {
		t.Errorf("Expected 2 records, got %d", len(records))
	}

	// Check first record
	if records[0]["id"].(int64) != 1 {
		t.Errorf("First record id = %v, want 1", records[0]["id"])
	}
	if records[0]["name"].(string) != "Alice" {
		t.Errorf("First record name = %v, want Alice", records[0]["name"])
	}
}

// TestDatabaseInputWithQueryFile tests loading query from file
func TestDatabaseInputWithQueryFile(t *testing.T) {
	t.Parallel()

	tmpFile := filepath.Join(t.TempDir(), "test.db")
	db, err := sql.Open("sqlite", tmpFile)
	if err != nil {
		t.Fatalf("Failed to create test db: %v", err)
	}

	_, err = db.Exec(`
		CREATE TABLE products (id INTEGER PRIMARY KEY, name TEXT, price REAL);
		INSERT INTO products VALUES (1, 'Widget', 9.99);
	`)
	if err != nil {
		t.Fatalf("Failed to setup: %v", err)
	}
	db.Close()

	// Create SQL file
	sqlFile := createTempSQLFile(t, "SELECT id, name, price FROM products")

	cfg := &connector.ModuleConfig{
		Type: "database",
		Config: map[string]interface{}{
			"connectionString": "file:" + tmpFile,
			"driver":           "sqlite",
			"queryFile":        sqlFile,
		},
	}

	inputModule, err := input.NewDatabaseInputFromConfig(cfg)
	if err != nil {
		t.Fatalf("Failed to create input module: %v", err)
	}
	defer inputModule.Close()

	ctx := context.Background()
	records, err := inputModule.Fetch(ctx)
	if err != nil {
		t.Fatalf("Fetch failed: %v", err)
	}

	if len(records) != 1 {
		t.Errorf("Expected 1 record, got %d", len(records))
	}
	if records[0]["name"].(string) != "Widget" {
		t.Errorf("Product name = %v, want Widget", records[0]["name"])
	}
}

// TestDatabaseInputWithLastRunTimestamp tests {{lastRunTimestamp}} injection
func TestDatabaseInputWithLastRunTimestamp(t *testing.T) {
	t.Parallel()

	tmpFile := filepath.Join(t.TempDir(), "test.db")
	db, err := sql.Open("sqlite", tmpFile)
	if err != nil {
		t.Fatalf("Failed to create test db: %v", err)
	}

	now := time.Now()
	yesterday := now.Add(-24 * time.Hour)
	lastWeek := now.Add(-7 * 24 * time.Hour)

	_, err = db.Exec(`
		CREATE TABLE events (id INTEGER PRIMARY KEY, name TEXT, created_at DATETIME);
		INSERT INTO events (name, created_at) VALUES ('old_event', ?);
		INSERT INTO events (name, created_at) VALUES ('recent_event', ?);
	`, lastWeek.Format("2006-01-02 15:04:05"), yesterday.Format("2006-01-02 15:04:05"))
	if err != nil {
		t.Fatalf("Failed to setup: %v", err)
	}
	db.Close()

	// Query with lastRunTimestamp - should get all records on first run (epoch time)
	cfg := &connector.ModuleConfig{
		Type: "database",
		Config: map[string]interface{}{
			"connectionString": "file:" + tmpFile,
			"driver":           "sqlite",
			"query":            "SELECT id, name, created_at FROM events WHERE created_at > {{lastRunTimestamp}} ORDER BY id",
		},
	}

	inputModule, err := input.NewDatabaseInputFromConfig(cfg)
	if err != nil {
		t.Fatalf("Failed to create input module: %v", err)
	}
	defer inputModule.Close()

	ctx := context.Background()
	// First run - no lastState, should use epoch and get all records
	records, err := inputModule.Fetch(ctx)
	if err != nil {
		t.Fatalf("Fetch failed: %v", err)
	}

	if len(records) != 2 {
		t.Errorf("Expected 2 records on first run (epoch time), got %d", len(records))
	}
}

// TestSQLCallFilterEnrichment tests SQL call filter for record enrichment
func TestSQLCallFilterEnrichment(t *testing.T) {
	t.Parallel()

	tmpFile := filepath.Join(t.TempDir(), "test.db")
	db, err := sql.Open("sqlite", tmpFile)
	if err != nil {
		t.Fatalf("Failed to create test db: %v", err)
	}

	_, err = db.Exec(`
		CREATE TABLE user_details (user_id INTEGER PRIMARY KEY, department TEXT, manager TEXT);
		INSERT INTO user_details VALUES (1, 'Engineering', 'John');
		INSERT INTO user_details VALUES (2, 'Sales', 'Jane');
	`)
	if err != nil {
		t.Fatalf("Failed to setup: %v", err)
	}
	db.Close()

	cfg := filter.SQLCallConfig{
		SQLRequestBase: moduleconfig.SQLRequestBase{
			ConnectionString: "file:" + tmpFile,
			Driver:           "sqlite",
			Query:            "SELECT department, manager FROM user_details WHERE user_id = {{record.user_id}}",
		},
		MergeStrategy: "merge",
	}

	sqlFilter, err := filter.NewSQLCallFromConfig(cfg)
	if err != nil {
		t.Fatalf("Failed to create filter: %v", err)
	}
	defer sqlFilter.Close()

	// Input records to enrich
	inputRecords := []map[string]interface{}{
		{"user_id": 1, "name": "Alice"},
		{"user_id": 2, "name": "Bob"},
	}

	ctx := context.Background()
	result, err := sqlFilter.Process(ctx, inputRecords)
	if err != nil {
		t.Fatalf("Process failed: %v", err)
	}

	if len(result) != 2 {
		t.Fatalf("Expected 2 records, got %d", len(result))
	}

	// Check enrichment
	if result[0]["department"] != "Engineering" {
		t.Errorf("First record department = %v, want Engineering", result[0]["department"])
	}
	if result[1]["manager"] != "Jane" {
		t.Errorf("Second record manager = %v, want Jane", result[1]["manager"])
	}
}

// TestSQLCallFilterWithQueryFile tests SQL call filter loading query from file
func TestSQLCallFilterWithQueryFile(t *testing.T) {
	t.Parallel()

	tmpFile := filepath.Join(t.TempDir(), "test.db")
	db, err := sql.Open("sqlite", tmpFile)
	if err != nil {
		t.Fatalf("Failed to create test db: %v", err)
	}

	_, err = db.Exec(`
		CREATE TABLE inventory (sku TEXT PRIMARY KEY, quantity INTEGER);
		INSERT INTO inventory VALUES ('SKU001', 100);
	`)
	if err != nil {
		t.Fatalf("Failed to setup: %v", err)
	}
	db.Close()

	sqlFile := createTempSQLFile(t, "SELECT quantity FROM inventory WHERE sku = {{record.sku}}")

	cfg := filter.SQLCallConfig{
		SQLRequestBase: moduleconfig.SQLRequestBase{
			ConnectionString: "file:" + tmpFile,
			Driver:           "sqlite",
			QueryFile:        sqlFile,
		},
		MergeStrategy: "merge",
	}

	sqlFilter, err := filter.NewSQLCallFromConfig(cfg)
	if err != nil {
		t.Fatalf("Failed to create filter: %v", err)
	}
	defer sqlFilter.Close()

	inputRecords := []map[string]interface{}{
		{"sku": "SKU001", "name": "Widget"},
	}

	ctx := context.Background()
	result, err := sqlFilter.Process(ctx, inputRecords)
	if err != nil {
		t.Fatalf("Process failed: %v", err)
	}

	if result[0]["quantity"].(int64) != 100 {
		t.Errorf("Quantity = %v, want 100", result[0]["quantity"])
	}
}

// TestDatabaseOutputInsert tests basic INSERT operations
func TestDatabaseOutputInsert(t *testing.T) {
	t.Parallel()

	tmpFile := filepath.Join(t.TempDir(), "test.db")
	db, err := sql.Open("sqlite", tmpFile)
	if err != nil {
		t.Fatalf("Failed to create test db: %v", err)
	}

	_, err = db.Exec(`
		CREATE TABLE audit_log (id INTEGER PRIMARY KEY, action TEXT, entity TEXT, timestamp TEXT);
	`)
	if err != nil {
		t.Fatalf("Failed to setup: %v", err)
	}
	db.Close()

	cfg := &connector.ModuleConfig{
		Type: "database",
		Config: map[string]interface{}{
			"connectionString": "file:" + tmpFile,
			"driver":           "sqlite",
			"query":            "INSERT INTO audit_log (action, entity, timestamp) VALUES ({{record.action}}, {{record.entity}}, {{record.timestamp}})",
		},
	}

	outputModule, err := output.NewDatabaseOutputFromConfig(cfg)
	if err != nil {
		t.Fatalf("Failed to create output module: %v", err)
	}
	defer outputModule.Close()

	records := []map[string]interface{}{
		{"action": "CREATE", "entity": "user", "timestamp": "2024-01-01"},
		{"action": "UPDATE", "entity": "order", "timestamp": "2024-01-02"},
	}

	ctx := context.Background()
	_, err = outputModule.Send(ctx, records)
	if err != nil {
		t.Fatalf("Send failed: %v", err)
	}

	// Verify inserts
	db, _ = sql.Open("sqlite", tmpFile)
	defer db.Close()

	var count int
	err = db.QueryRow("SELECT COUNT(*) FROM audit_log").Scan(&count)
	if err != nil {
		t.Fatalf("Count query failed: %v", err)
	}
	if count != 2 {
		t.Errorf("Expected 2 rows, got %d", count)
	}

	var action string
	err = db.QueryRow("SELECT action FROM audit_log WHERE entity = 'user'").Scan(&action)
	if err != nil {
		t.Fatalf("Query failed: %v", err)
	}
	if action != "CREATE" {
		t.Errorf("Action = %q, want CREATE", action)
	}
}

// TestDatabaseOutputWithTransaction tests transaction mode
func TestDatabaseOutputWithTransaction(t *testing.T) {
	t.Parallel()

	tmpFile := filepath.Join(t.TempDir(), "test.db")
	db, err := sql.Open("sqlite", tmpFile)
	if err != nil {
		t.Fatalf("Failed to create test db: %v", err)
	}

	_, err = db.Exec(`
		CREATE TABLE items (id INTEGER PRIMARY KEY, name TEXT NOT NULL);
	`)
	if err != nil {
		t.Fatalf("Failed to setup: %v", err)
	}
	db.Close()

	cfg := &connector.ModuleConfig{
		Type: "database",
		Config: map[string]interface{}{
			"connectionString": "file:" + tmpFile,
			"driver":           "sqlite",
			"query":            "INSERT INTO items (id, name) VALUES ({{record.id}}, {{record.name}})",
			"transaction":      true,
		},
	}

	outputModule, err := output.NewDatabaseOutputFromConfig(cfg)
	if err != nil {
		t.Fatalf("Failed to create output module: %v", err)
	}
	defer outputModule.Close()

	records := []map[string]interface{}{
		{"id": 1, "name": "Item 1"},
		{"id": 2, "name": "Item 2"},
		{"id": 3, "name": "Item 3"},
	}

	ctx := context.Background()
	_, err = outputModule.Send(ctx, records)
	if err != nil {
		t.Fatalf("Send failed: %v", err)
	}

	// Verify all inserts (all or nothing)
	db, _ = sql.Open("sqlite", tmpFile)
	defer db.Close()

	var count int
	err = db.QueryRow("SELECT COUNT(*) FROM items").Scan(&count)
	if err != nil {
		t.Fatalf("Count query failed: %v", err)
	}
	if count != 3 {
		t.Errorf("Expected 3 rows in transaction, got %d", count)
	}
}

// TestDatabaseOutputUpsert tests INSERT OR REPLACE (SQLite upsert)
func TestDatabaseOutputUpsert(t *testing.T) {
	t.Parallel()

	tmpFile := filepath.Join(t.TempDir(), "test.db")
	db, err := sql.Open("sqlite", tmpFile)
	if err != nil {
		t.Fatalf("Failed to create test db: %v", err)
	}

	_, err = db.Exec(`
		CREATE TABLE products (sku TEXT PRIMARY KEY, name TEXT, price REAL);
		INSERT INTO products VALUES ('SKU001', 'Old Name', 9.99);
	`)
	if err != nil {
		t.Fatalf("Failed to setup: %v", err)
	}
	db.Close()

	cfg := &connector.ModuleConfig{
		Type: "database",
		Config: map[string]interface{}{
			"connectionString": "file:" + tmpFile,
			"driver":           "sqlite",
			"query":            "INSERT OR REPLACE INTO products (sku, name, price) VALUES ({{record.sku}}, {{record.name}}, {{record.price}})",
		},
	}

	outputModule, err := output.NewDatabaseOutputFromConfig(cfg)
	if err != nil {
		t.Fatalf("Failed to create output module: %v", err)
	}
	defer outputModule.Close()

	records := []map[string]interface{}{
		{"sku": "SKU001", "name": "Updated Name", "price": 19.99}, // Update existing
		{"sku": "SKU002", "name": "New Product", "price": 29.99},  // Insert new
	}

	ctx := context.Background()
	_, err = outputModule.Send(ctx, records)
	if err != nil {
		t.Fatalf("Send failed: %v", err)
	}

	// Verify upsert
	db, _ = sql.Open("sqlite", tmpFile)
	defer db.Close()

	var count int
	err = db.QueryRow("SELECT COUNT(*) FROM products").Scan(&count)
	if err != nil {
		t.Fatalf("Count query failed: %v", err)
	}
	if count != 2 {
		t.Errorf("Expected 2 products after upsert, got %d", count)
	}

	var name string
	var price float64
	err = db.QueryRow("SELECT name, price FROM products WHERE sku = 'SKU001'").Scan(&name, &price)
	if err != nil {
		t.Fatalf("Query failed: %v", err)
	}
	if name != "Updated Name" {
		t.Errorf("Name = %q, want 'Updated Name'", name)
	}
	if price != 19.99 {
		t.Errorf("Price = %v, want 19.99", price)
	}
}

// TestDatabaseOutputErrorHandling tests error handling modes
func TestDatabaseOutputErrorHandling(t *testing.T) {
	t.Parallel()

	tmpFile := filepath.Join(t.TempDir(), "test.db")
	db, err := sql.Open("sqlite", tmpFile)
	if err != nil {
		t.Fatalf("Failed to create test db: %v", err)
	}

	_, err = db.Exec(`
		CREATE TABLE unique_items (id INTEGER PRIMARY KEY, code TEXT UNIQUE);
		INSERT INTO unique_items VALUES (1, 'EXISTING');
	`)
	if err != nil {
		t.Fatalf("Failed to setup: %v", err)
	}
	db.Close()

	// Test with onError: skip - should continue after constraint violation
	cfg := &connector.ModuleConfig{
		Type: "database",
		Config: map[string]interface{}{
			"connectionString": "file:" + tmpFile,
			"driver":           "sqlite",
			"query":            "INSERT INTO unique_items (id, code) VALUES ({{record.id}}, {{record.code}})",
			"onError":          "skip",
		},
	}

	outputModule, err := output.NewDatabaseOutputFromConfig(cfg)
	if err != nil {
		t.Fatalf("Failed to create output module: %v", err)
	}
	defer outputModule.Close()

	records := []map[string]interface{}{
		{"id": 2, "code": "EXISTING"}, // Will fail - duplicate code
		{"id": 3, "code": "NEW"},      // Should succeed
	}

	ctx := context.Background()
	// With onError: skip, this should not fail
	_, err = outputModule.Send(ctx, records)
	if err != nil {
		t.Fatalf("Send with onError:skip should not fail: %v", err)
	}

	// Verify only the valid record was inserted
	db, _ = sql.Open("sqlite", tmpFile)
	defer db.Close()

	var count int
	err = db.QueryRow("SELECT COUNT(*) FROM unique_items").Scan(&count)
	if err != nil {
		t.Fatalf("Count query failed: %v", err)
	}
	if count != 2 {
		t.Errorf("Expected 2 rows (1 original + 1 new), got %d", count)
	}
}

// TestDatabaseOutputWithQueryFile tests loading query from file
func TestDatabaseOutputWithQueryFile(t *testing.T) {
	t.Parallel()

	tmpFile := filepath.Join(t.TempDir(), "test.db")
	db, err := sql.Open("sqlite", tmpFile)
	if err != nil {
		t.Fatalf("Failed to create test db: %v", err)
	}

	_, err = db.Exec(`CREATE TABLE logs (message TEXT);`)
	if err != nil {
		t.Fatalf("Failed to setup: %v", err)
	}
	db.Close()

	sqlFile := createTempSQLFile(t, "INSERT INTO logs (message) VALUES ({{record.msg}})")

	cfg := &connector.ModuleConfig{
		Type: "database",
		Config: map[string]interface{}{
			"connectionString": "file:" + tmpFile,
			"driver":           "sqlite",
			"queryFile":        sqlFile,
		},
	}

	outputModule, err := output.NewDatabaseOutputFromConfig(cfg)
	if err != nil {
		t.Fatalf("Failed to create output module: %v", err)
	}
	defer outputModule.Close()

	records := []map[string]interface{}{
		{"msg": "Log entry 1"},
		{"msg": "Log entry 2"},
	}

	ctx := context.Background()
	_, err = outputModule.Send(ctx, records)
	if err != nil {
		t.Fatalf("Send failed: %v", err)
	}

	// Verify
	db, _ = sql.Open("sqlite", tmpFile)
	defer db.Close()

	var count int
	err = db.QueryRow("SELECT COUNT(*) FROM logs").Scan(&count)
	if err != nil {
		t.Fatalf("Count failed: %v", err)
	}
	if count != 2 {
		t.Errorf("Expected 2 logs, got %d", count)
	}
}

// TestDatabaseOutputNestedFields tests accessing nested record fields
func TestDatabaseOutputNestedFields(t *testing.T) {
	t.Parallel()

	tmpFile := filepath.Join(t.TempDir(), "test.db")
	db, err := sql.Open("sqlite", tmpFile)
	if err != nil {
		t.Fatalf("Failed to create test db: %v", err)
	}

	_, err = db.Exec(`CREATE TABLE events (event_type TEXT, user_name TEXT);`)
	if err != nil {
		t.Fatalf("Failed to setup: %v", err)
	}
	db.Close()

	cfg := &connector.ModuleConfig{
		Type: "database",
		Config: map[string]interface{}{
			"connectionString": "file:" + tmpFile,
			"driver":           "sqlite",
			"query":            "INSERT INTO events (event_type, user_name) VALUES ({{record.event.type}}, {{record.user.name}})",
		},
	}

	outputModule, err := output.NewDatabaseOutputFromConfig(cfg)
	if err != nil {
		t.Fatalf("Failed to create output module: %v", err)
	}
	defer outputModule.Close()

	records := []map[string]interface{}{
		{
			"event": map[string]interface{}{"type": "login"},
			"user":  map[string]interface{}{"name": "alice"},
		},
	}

	ctx := context.Background()
	_, err = outputModule.Send(ctx, records)
	if err != nil {
		t.Fatalf("Send failed: %v", err)
	}

	// Verify
	db, _ = sql.Open("sqlite", tmpFile)
	defer db.Close()

	var eventType, userName string
	err = db.QueryRow("SELECT event_type, user_name FROM events").Scan(&eventType, &userName)
	if err != nil {
		t.Fatalf("Query failed: %v", err)
	}
	if eventType != "login" || userName != "alice" {
		t.Errorf("Got (%q, %q), want (login, alice)", eventType, userName)
	}
}
