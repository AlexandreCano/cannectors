package input

import (
	"context"
	"database/sql"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
	"time"

	"github.com/cannectors/runtime/internal/moduleconfig"
	"github.com/cannectors/runtime/internal/persistence"
	"github.com/cannectors/runtime/pkg/connector"

	_ "modernc.org/sqlite"
)

func setupDatabaseInputSQLiteDB(t *testing.T) *sql.DB {
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
		CREATE TABLE records (
			id INTEGER PRIMARY KEY,
			name TEXT NOT NULL,
			updated_at TEXT NOT NULL
		);
		INSERT INTO records (id, name, updated_at) VALUES
			(1, 'old', '2026-01-01T00:00:00Z'),
			(2, 'middle', '2026-01-02T00:00:00Z'),
			(3, 'new', '2026-01-03T00:00:00Z'),
			(4, 'newer', '2026-01-04T00:00:00Z');
	`)
	if err != nil {
		t.Fatalf("creating database input fixtures: %v", err)
	}

	return db
}

func newDatabaseInputForTest(db *sql.DB, cfg DatabaseInputConfig, state *persistence.State) *DatabaseInput {
	return &DatabaseInput{
		config:    cfg,
		db:        db,
		driver:    "sqlite",
		timeout:   200 * time.Millisecond,
		lastState: state,
	}
}

func databaseInputState(ts *time.Time, id string) *persistence.State {
	state := &persistence.State{PipelineID: "test", LastTimestamp: ts}
	if id != "" {
		state.LastID = &id
	}
	return state
}

func assertDatabaseInputNames(t *testing.T, got []map[string]any, want []string) {
	t.Helper()
	names := make([]string, len(got))
	for i, record := range got {
		names[i], _ = record["name"].(string)
	}
	if !reflect.DeepEqual(names, want) {
		t.Fatalf("names = %#v, want %#v", names, want)
	}
}

func TestDatabaseInput_IncrementalQueries(t *testing.T) {
	since := time.Date(2026, 1, 2, 0, 0, 0, 0, time.UTC)

	tests := []struct {
		name  string
		cfg   DatabaseInputConfig
		state *persistence.State
		want  []string
	}{
		{
			name: "timestamp incremental",
			cfg: DatabaseInputConfig{
				SQLRequestBase: moduleconfig.SQLRequestBase{Query: "SELECT id, name FROM records WHERE updated_at > :since ORDER BY id"},
				Incremental:    &IncrementalConfig{Enabled: true, TimestampField: "updated_at", TimestampParam: "since"},
			},
			state: databaseInputState(&since, ""),
			want:  []string{"new", "newer"},
		},
		{
			name: "id incremental",
			cfg: DatabaseInputConfig{
				SQLRequestBase: moduleconfig.SQLRequestBase{Query: "SELECT id, name FROM records WHERE id > :after_id ORDER BY id"},
				Incremental:    &IncrementalConfig{Enabled: true, IDField: "id", IDParam: "after_id"},
			},
			state: databaseInputState(nil, "2"),
			want:  []string{"new", "newer"},
		},
		{
			name: "timestamp and id incremental",
			cfg: DatabaseInputConfig{
				SQLRequestBase: moduleconfig.SQLRequestBase{Query: "SELECT id, name FROM records WHERE updated_at > :since AND id > :after_id ORDER BY id"},
				Incremental:    &IncrementalConfig{Enabled: true, TimestampField: "updated_at", TimestampParam: "since", IDField: "id", IDParam: "after_id"},
			},
			state: databaseInputState(&since, "2"),
			want:  []string{"new", "newer"},
		},
		{
			name: "first run uses epoch timestamp",
			cfg: DatabaseInputConfig{
				SQLRequestBase: moduleconfig.SQLRequestBase{Query: "SELECT id, name FROM records WHERE updated_at > {{lastRunTimestamp}} ORDER BY id"},
			},
			want: []string{"old", "middle", "new", "newer"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			db := setupDatabaseInputSQLiteDB(t)
			input := newDatabaseInputForTest(db, tt.cfg, tt.state)

			got, err := input.Fetch(context.Background())
			if err != nil {
				t.Fatalf("Fetch() error = %v", err)
			}
			assertDatabaseInputNames(t, got, tt.want)
		})
	}
}

func TestDatabaseInput_Pagination(t *testing.T) {
	tests := []struct {
		name string
		cfg  DatabaseInputConfig
		want []string
	}{
		{
			name: "limit offset",
			cfg: DatabaseInputConfig{
				SQLRequestBase: moduleconfig.SQLRequestBase{Query: "SELECT id, name FROM records ORDER BY id"},
				Pagination:     &moduleconfig.DatabasePaginationConfig{Type: "limit-offset", Limit: 2},
			},
			want: []string{"old", "middle", "new", "newer"},
		},
		{
			name: "cursor",
			cfg: DatabaseInputConfig{
				SQLRequestBase: moduleconfig.SQLRequestBase{Query: "SELECT id, name FROM records WHERE id > COALESCE(:after_id, 0) ORDER BY id"},
				Pagination:     &moduleconfig.DatabasePaginationConfig{Type: "cursor", Limit: 2, CursorField: "id", Param: "after_id"},
			},
			want: []string{"old", "middle", "new", "newer"},
		},
		{
			name: "limit offset with param",
			cfg: DatabaseInputConfig{
				SQLRequestBase: moduleconfig.SQLRequestBase{Query: "SELECT id, name FROM records WHERE id > :offset ORDER BY id"},
				Pagination:     &moduleconfig.DatabasePaginationConfig{Type: "limit-offset", Limit: 2, Param: "offset"},
			},
			want: []string{"old", "middle", "new", "newer"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			db := setupDatabaseInputSQLiteDB(t)
			input := newDatabaseInputForTest(db, tt.cfg, nil)

			got, err := input.Fetch(context.Background())
			if err != nil {
				t.Fatalf("Fetch() error = %v", err)
			}
			assertDatabaseInputNames(t, got, tt.want)
		})
	}
}

func TestDatabaseInput_NewFromConfig_QueryFileAndEnvConnectionString(t *testing.T) {
	tmpDir := t.TempDir()
	dbPath := filepath.Join(tmpDir, "input.db")

	db, err := sql.Open("sqlite", "file:"+dbPath)
	if err != nil {
		t.Fatalf("sql.Open sqlite file: %v", err)
	}
	_, err = db.Exec(`
		CREATE TABLE records (id INTEGER PRIMARY KEY, name TEXT NOT NULL);
		INSERT INTO records (id, name) VALUES (1, 'Alice');
	`)
	if err != nil {
		t.Fatalf("creating sqlite file fixture: %v", err)
	}
	if closeErr := db.Close(); closeErr != nil {
		t.Fatalf("closing sqlite file fixture: %v", closeErr)
	}

	queryPath := filepath.Join(tmpDir, "input.sql")
	if writeErr := os.WriteFile(queryPath, []byte("SELECT id, name FROM records ORDER BY id"), 0600); writeErr != nil {
		t.Fatalf("writing query file: %v", writeErr)
	}
	t.Setenv("DATABASE_INPUT_TEST_DSN", "file:"+dbPath)

	input, err := NewDatabaseInputFromConfig(&connector.ModuleConfig{
		Type: "database",
		Raw: mustJSON(map[string]any{
			"connectionStringRef": "${DATABASE_INPUT_TEST_DSN}",
			"driver":              "sqlite",
			"queryFile":           queryPath,
			"incremental": map[string]any{
				"enabled":        true,
				"timestampField": "updated_at",
			},
		}),
	})
	if err != nil {
		t.Fatalf("NewDatabaseInputFromConfig() error = %v", err)
	}
	defer func() {
		if closeErr := input.Close(); closeErr != nil {
			t.Errorf("closing database input: %v", closeErr)
		}
	}()

	got, err := input.Fetch(context.Background())
	if err != nil {
		t.Fatalf("Fetch() error = %v", err)
	}
	assertDatabaseInputNames(t, got, []string{"Alice"})

	if cfg := input.GetPersistenceConfig(); cfg == nil || !cfg.TimestampEnabled() {
		t.Fatalf("persistence config = %#v, want timestamp enabled", cfg)
	}
}

func TestDatabaseInput_BuildQueryDriverSpecificPlaceholders(t *testing.T) {
	ts := time.Date(2026, 1, 2, 3, 4, 5, 0, time.UTC)
	input := &DatabaseInput{
		config: DatabaseInputConfig{
			SQLRequestBase: moduleconfig.SQLRequestBase{Query: "SELECT * FROM records WHERE updated_at > {{lastRunTimestamp}} AND id > :after_id AND name = :name"},
			Parameters:     map[string]any{"name": "Alice"},
			Incremental:    &IncrementalConfig{Enabled: true, IDField: "id", IDParam: "after_id"},
		},
		driver:    "postgres",
		lastState: databaseInputState(&ts, "42"),
	}

	query, args := input.buildQuery()
	if query != "SELECT * FROM records WHERE updated_at > $1 AND id > $2 AND name = $3" {
		t.Fatalf("query = %q", query)
	}
	if len(args) != 3 || args[0] != ts.Format(time.RFC3339) || args[1] != "42" || args[2] != "Alice" {
		t.Fatalf("args = %#v", args)
	}

	input.driver = "mysql"
	query, args = input.buildQuery()
	if strings.Count(query, "?") != 3 {
		t.Fatalf("mysql query = %q, want three ? placeholders", query)
	}
	if len(args) != 3 {
		t.Fatalf("mysql args len = %d, want 3", len(args))
	}
}
