package output

import (
	"context"
	"database/sql"
	"testing"
	"time"

	"github.com/cannectors/runtime/internal/moduleconfig"

	_ "modernc.org/sqlite"
)

func setupDatabaseOutputSQLiteDB(t *testing.T) *sql.DB {
	t.Helper()

	db, err := sql.Open("sqlite", ":memory:")
	if err != nil {
		t.Fatalf("sql.Open sqlite: %v", err)
	}
	db.SetMaxOpenConns(1)
	t.Cleanup(func() {
		if closeErr := db.Close(); closeErr != nil {
			t.Errorf("closing sqlite db: %v", closeErr)
		}
	})

	_, err = db.Exec(`
		CREATE TABLE users (
			id INTEGER PRIMARY KEY,
			name TEXT NOT NULL,
			email TEXT UNIQUE NOT NULL
		);
	`)
	if err != nil {
		t.Fatalf("creating database output fixtures: %v", err)
	}

	return db
}

func newDatabaseOutputForTest(db *sql.DB, cfg DatabaseOutputConfig) *DatabaseOutput {
	if cfg.OnError == "" {
		cfg.OnError = "fail"
	}
	return &DatabaseOutput{
		db:      db,
		driver:  "sqlite",
		config:  cfg,
		timeout: 200 * time.Millisecond,
	}
}

func countOutputRows(t *testing.T, db *sql.DB) int {
	t.Helper()
	var count int
	if err := db.QueryRow("SELECT COUNT(*) FROM users").Scan(&count); err != nil {
		t.Fatalf("counting output rows: %v", err)
	}
	return count
}

func TestDatabaseOutput_SendModes(t *testing.T) {
	tests := []struct {
		name       string
		query      string
		seed       string
		records    []map[string]any
		wantCount  int
		wantNameID int
		wantName   string
	}{
		{
			name:      "insert",
			query:     "INSERT INTO users (id, name, email) VALUES ({{record.id}}, {{record.name}}, {{record.email}})",
			records:   []map[string]any{{"id": 1, "name": "Alice", "email": "alice@example.com"}},
			wantCount: 1,
		},
		{
			name:      "upsert sqlite replace",
			seed:      "INSERT INTO users (id, name, email) VALUES (1, 'Old', 'old@example.com')",
			query:     "REPLACE INTO users (id, name, email) VALUES ({{record.id}}, {{record.name}}, {{record.email}})",
			records:   []map[string]any{{"id": 1, "name": "Alice", "email": "alice@example.com"}},
			wantCount: 1, wantNameID: 1, wantName: "Alice",
		},
		{
			name:      "custom templated query",
			query:     "INSERT INTO users (id, name, email) SELECT {{record.id}}, upper({{record.name}}), {{record.email}}",
			records:   []map[string]any{{"id": 2, "name": "bob", "email": "bob@example.com"}},
			wantCount: 1, wantNameID: 2, wantName: "BOB",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			db := setupDatabaseOutputSQLiteDB(t)
			if tt.seed != "" {
				if _, err := db.Exec(tt.seed); err != nil {
					t.Fatalf("seeding output db: %v", err)
				}
			}
			output := newDatabaseOutputForTest(db, DatabaseOutputConfig{
				SQLRequestBase: moduleconfig.SQLRequestBase{Query: tt.query},
			})

			sent, err := output.Send(context.Background(), tt.records)
			if err != nil {
				t.Fatalf("Send() error = %v", err)
			}
			if sent != len(tt.records) {
				t.Fatalf("sent = %d, want %d", sent, len(tt.records))
			}
			if count := countOutputRows(t, db); count != tt.wantCount {
				t.Fatalf("row count = %d, want %d", count, tt.wantCount)
			}
			if tt.wantNameID != 0 {
				var name string
				if err := db.QueryRow("SELECT name FROM users WHERE id = ?", tt.wantNameID).Scan(&name); err != nil {
					t.Fatalf("selecting output name: %v", err)
				}
				if name != tt.wantName {
					t.Fatalf("name = %q, want %q", name, tt.wantName)
				}
			}
		})
	}
}

func TestDatabaseOutput_TransactionCommitAndRollback(t *testing.T) {
	t.Run("commit on success", func(t *testing.T) {
		db := setupDatabaseOutputSQLiteDB(t)
		output := newDatabaseOutputForTest(db, DatabaseOutputConfig{
			SQLRequestBase: moduleconfig.SQLRequestBase{Query: "INSERT INTO users (id, name, email) VALUES ({{record.id}}, {{record.name}}, {{record.email}})"},
			Transaction:    true,
		})

		sent, err := output.Send(context.Background(), []map[string]any{
			{"id": 1, "name": "Alice", "email": "alice@example.com"},
			{"id": 2, "name": "Bob", "email": "bob@example.com"},
		})
		if err != nil {
			t.Fatalf("Send() error = %v", err)
		}
		if sent != 2 || countOutputRows(t, db) != 2 {
			t.Fatalf("sent/count = %d/%d, want 2/2", sent, countOutputRows(t, db))
		}
	})

	t.Run("rollback on error", func(t *testing.T) {
		db := setupDatabaseOutputSQLiteDB(t)
		output := newDatabaseOutputForTest(db, DatabaseOutputConfig{
			SQLRequestBase: moduleconfig.SQLRequestBase{Query: "INSERT INTO users (id, name, email) VALUES ({{record.id}}, {{record.name}}, {{record.email}})"},
			Transaction:    true,
		})

		sent, err := output.Send(context.Background(), []map[string]any{
			{"id": 1, "name": "Alice", "email": "alice@example.com"},
			{"id": 2, "name": "Duplicate", "email": "alice@example.com"},
		})
		if err == nil {
			t.Fatal("Send() error = nil, want unique constraint error")
		}
		if sent != 1 {
			t.Fatalf("sent before rollback = %d, want 1", sent)
		}
		if count := countOutputRows(t, db); count != 0 {
			t.Fatalf("row count after rollback = %d, want 0", count)
		}
	})
}

func TestDatabaseOutput_OnErrorSkipAndFail(t *testing.T) {
	records := []map[string]any{
		{"id": 1, "name": "Alice", "email": "alice@example.com"},
		{"id": 2, "name": "Duplicate", "email": "alice@example.com"},
		{"id": 3, "name": "Charlie", "email": "charlie@example.com"},
	}

	t.Run("skip individual record", func(t *testing.T) {
		db := setupDatabaseOutputSQLiteDB(t)
		output := newDatabaseOutputForTest(db, DatabaseOutputConfig{
			SQLRequestBase: moduleconfig.SQLRequestBase{Query: "INSERT INTO users (id, name, email) VALUES ({{record.id}}, {{record.name}}, {{record.email}})"},
		})
		output.config.OnError = "skip"

		sent, err := output.Send(context.Background(), records)
		if err != nil {
			t.Fatalf("Send() error = %v", err)
		}
		if sent != 2 || countOutputRows(t, db) != 2 {
			t.Fatalf("sent/count = %d/%d, want 2/2", sent, countOutputRows(t, db))
		}
	})

	t.Run("fail stops at first invalid record", func(t *testing.T) {
		db := setupDatabaseOutputSQLiteDB(t)
		output := newDatabaseOutputForTest(db, DatabaseOutputConfig{
			SQLRequestBase: moduleconfig.SQLRequestBase{Query: "INSERT INTO users (id, name, email) VALUES ({{record.id}}, {{record.name}}, {{record.email}})"},
		})

		sent, err := output.Send(context.Background(), records)
		if err == nil {
			t.Fatal("Send() error = nil, want unique constraint error")
		}
		if sent != 1 || countOutputRows(t, db) != 1 {
			t.Fatalf("sent/count = %d/%d, want 1/1", sent, countOutputRows(t, db))
		}
	})
}

// TestDatabaseOutput_BatchVsIndividual contrasts the two write modes on the
// same input: with Transaction=true a mid-batch failure rolls back every
// preceding record, while Transaction=false commits each record independently
// so the failing one is the only loss.
func TestDatabaseOutput_BatchVsIndividual(t *testing.T) {
	records := []map[string]any{
		{"id": 1, "name": "Alice", "email": "alice@example.com"},
		{"id": 2, "name": "Duplicate", "email": "alice@example.com"}, // unique violation
		{"id": 3, "name": "Charlie", "email": "charlie@example.com"},
	}

	t.Run("batch transaction rolls back all on failure", func(t *testing.T) {
		db := setupDatabaseOutputSQLiteDB(t)
		output := newDatabaseOutputForTest(db, DatabaseOutputConfig{
			SQLRequestBase: moduleconfig.SQLRequestBase{Query: "INSERT INTO users (id, name, email) VALUES ({{record.id}}, {{record.name}}, {{record.email}})"},
			Transaction:    true,
		})

		_, err := output.Send(context.Background(), records)
		if err == nil {
			t.Fatal("Send() error = nil, want unique constraint error")
		}
		if count := countOutputRows(t, db); count != 0 {
			t.Fatalf("rows after rollback = %d, want 0 (transaction discarded prior inserts)", count)
		}
	})

	t.Run("individual mode commits prior records and stops on failure", func(t *testing.T) {
		db := setupDatabaseOutputSQLiteDB(t)
		output := newDatabaseOutputForTest(db, DatabaseOutputConfig{
			SQLRequestBase: moduleconfig.SQLRequestBase{Query: "INSERT INTO users (id, name, email) VALUES ({{record.id}}, {{record.name}}, {{record.email}})"},
			Transaction:    false,
		})

		sent, err := output.Send(context.Background(), records)
		if err == nil {
			t.Fatal("Send() error = nil, want unique constraint error")
		}
		if sent != 1 {
			t.Fatalf("sent before failure = %d, want 1", sent)
		}
		if count := countOutputRows(t, db); count != 1 {
			t.Fatalf("rows after failure = %d, want 1 (first record kept, no rollback in individual mode)", count)
		}
	})
}

func TestDatabaseOutput_BuildParameterizedQueryUnmatchedTemplate(t *testing.T) {
	output := newDatabaseOutputForTest(setupDatabaseOutputSQLiteDB(t), DatabaseOutputConfig{})
	_, _, err := output.buildParameterizedQuery("INSERT INTO users (name) VALUES ({{record.name)", map[string]any{"name": "Alice"})
	if err == nil {
		t.Fatal("buildParameterizedQuery() error = nil, want unmatched template error")
	}
}
