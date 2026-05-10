package filter

import (
	"context"
	"database/sql"
	"errors"
	"os"
	"path/filepath"
	"reflect"
	"sync"
	"testing"
	"time"

	"github.com/cannectors/runtime/internal/errhandling"
	"github.com/cannectors/runtime/internal/moduleconfig"
	"github.com/cannectors/runtime/internal/recordpath"
	"github.com/cannectors/runtime/internal/template"
	"github.com/cannectors/runtime/pkg/connector"

	_ "modernc.org/sqlite"
)

// TestMarshalDeterministic_StableForPermutedKeys verifies AC #1 of Story 17.3:
// two records with the same data but keys inserted in different orders must
// produce the same cache key.
func TestMarshalDeterministic_StableForPermutedKeys(t *testing.T) {
	t.Parallel()

	a := map[string]any{"a": 1, "b": 2, "c": "three"}
	b := map[string]any{"c": "three", "a": 1, "b": 2}

	gotA, err := marshalDeterministic(a)
	if err != nil {
		t.Fatalf("marshalDeterministic(a) error = %v", err)
	}
	gotB, err := marshalDeterministic(b)
	if err != nil {
		t.Fatalf("marshalDeterministic(b) error = %v", err)
	}
	if string(gotA) != string(gotB) {
		t.Errorf("permuted keys produced different output:\n  a=%s\n  b=%s", gotA, gotB)
	}
}

// TestMarshalDeterministic_NestedKeysSorted verifies AC #3: nested maps must
// also be canonicalised so that {"u":{"x":1,"y":2}} and {"u":{"y":2,"x":1}}
// share the same serialization.
func TestMarshalDeterministic_NestedKeysSorted(t *testing.T) {
	t.Parallel()

	a := map[string]any{
		"user": map[string]any{"id": 1, "name": "x", "addr": map[string]any{"city": "Paris", "zip": "75001"}},
	}
	b := map[string]any{
		"user": map[string]any{"name": "x", "addr": map[string]any{"zip": "75001", "city": "Paris"}, "id": 1},
	}

	gotA, _ := marshalDeterministic(a)
	gotB, _ := marshalDeterministic(b)
	if string(gotA) != string(gotB) {
		t.Errorf("nested permutations produced different output:\n  a=%s\n  b=%s", gotA, gotB)
	}
}

// TestMarshalDeterministic_StressLoop verifies AC #4: 1000 iterations on a
// permuted record always yield the same cache key.
func TestMarshalDeterministic_StressLoop(t *testing.T) {
	t.Parallel()

	const iterations = 1000
	first, err := marshalDeterministic(map[string]any{
		"a": 1, "b": 2, "c": 3, "d": 4, "e": 5,
		"nested": map[string]any{"x": "X", "y": "Y", "z": "Z"},
	})
	if err != nil {
		t.Fatalf("marshalDeterministic error = %v", err)
	}
	for i := 0; i < iterations; i++ {
		// Each iteration uses a freshly-built map (Go map iteration is randomized).
		got, err := marshalDeterministic(map[string]any{
			"e": 5, "c": 3, "a": 1, "d": 4, "b": 2,
			"nested": map[string]any{"z": "Z", "x": "X", "y": "Y"},
		})
		if err != nil {
			t.Fatalf("iteration %d: marshalDeterministic error = %v", i, err)
		}
		if string(got) != string(first) {
			t.Fatalf("iteration %d produced divergent output:\n  first=%s\n  got=  %s", i, first, got)
		}
	}
}

func TestNewSQLCallFromConfig_Validation(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name    string
		config  SQLCallConfig
		wantErr error
	}{
		{
			name:    "missing connection string",
			config:  SQLCallConfig{SQLRequestBase: moduleconfig.SQLRequestBase{Query: "SELECT 1"}},
			wantErr: ErrSQLCallMissingConnection,
		},
		{
			name:    "missing query",
			config:  SQLCallConfig{SQLRequestBase: moduleconfig.SQLRequestBase{ConnectionString: "postgres://localhost/db"}},
			wantErr: ErrSQLCallMissingQuery,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := NewSQLCallFromConfig(tt.config)
			if err == nil {
				t.Error("expected error, got nil")
				return
			}
			if err != tt.wantErr {
				t.Errorf("error = %v, want %v", err, tt.wantErr)
			}
		})
	}
}

func TestGetSQLNestedValue(t *testing.T) {
	t.Parallel()

	record := map[string]any{
		"id":   123,
		"name": "test",
		"nested": map[string]any{
			"field": "value",
			"deep": map[string]any{
				"key": "deep_value",
			},
		},
	}

	tests := []struct {
		name string
		path string
		want any
	}{
		{
			name: "top-level field",
			path: "id",
			want: 123,
		},
		{
			name: "top-level string",
			path: "name",
			want: "test",
		},
		{
			name: "nested field",
			path: "nested.field",
			want: "value",
		},
		{
			name: "deeply nested field",
			path: "nested.deep.key",
			want: "deep_value",
		},
		{
			name: "non-existent field",
			path: "nonexistent",
			want: nil,
		},
		{
			name: "non-existent nested field",
			path: "nested.nonexistent",
			want: nil,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, _ := recordpath.Get(record, tt.path)
			if got != tt.want {
				t.Errorf("recordpath.Get(%q) = %v, want %v", tt.path, got, tt.want)
			}
		})
	}
}

func TestDeepMerge(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name string
		a    map[string]any
		b    map[string]any
		want map[string]any
	}{
		{
			name: "simple merge",
			a:    map[string]any{"a": 1, "b": 2},
			b:    map[string]any{"c": 3},
			want: map[string]any{"a": 1, "b": 2, "c": 3},
		},
		{
			name: "override value",
			a:    map[string]any{"a": 1, "b": 2},
			b:    map[string]any{"b": 10},
			want: map[string]any{"a": 1, "b": 10},
		},
		{
			name: "nested merge",
			a: map[string]any{
				"outer": map[string]any{
					"a": 1,
					"b": 2,
				},
			},
			b: map[string]any{
				"outer": map[string]any{
					"c": 3,
				},
			},
			want: map[string]any{
				"outer": map[string]any{
					"a": 1,
					"b": 2,
					"c": 3,
				},
			},
		},
		{
			name: "b nil map",
			a:    map[string]any{"a": 1},
			b:    map[string]any{},
			want: map[string]any{"a": 1},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := deepMerge(tt.a, tt.b)
			// Compare top-level keys
			for k, v := range tt.want {
				gotV, exists := got[k]
				if !exists {
					t.Errorf("key %q missing from result", k)
					continue
				}
				// For nested maps, just check they exist
				if _, ok := v.(map[string]any); ok {
					if _, ok := gotV.(map[string]any); !ok {
						t.Errorf("key %q should be a map", k)
					}
				} else if gotV != v {
					t.Errorf("got[%q] = %v, want %v", k, gotV, v)
				}
			}
		})
	}
}

func setupSQLCallSQLiteDB(t *testing.T) *sql.DB {
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
			tier TEXT NOT NULL,
			region TEXT NOT NULL,
			score INTEGER NOT NULL
		);
		INSERT INTO users (id, name, tier, region, score) VALUES
			(1, 'Alice', 'gold', 'EU', 91),
			(2, 'Bob', 'silver', 'US', 72),
			(3, 'Charlie', 'bronze', 'EU', 64);
	`)
	if err != nil {
		t.Fatalf("creating sql_call fixtures: %v", err)
	}

	return db
}

func newSQLCallTestModule(t *testing.T, db *sql.DB, cfg SQLCallConfig) *SQLCallModule {
	t.Helper()

	cacheStore, cacheTTL, cacheEnabled := setupCache(cfg)
	onError, err := errhandling.ParseOnErrorStrategy(cfg.OnError)
	if err != nil {
		t.Fatalf("parsing onError: %v", err)
	}
	return &SQLCallModule{
		db:                db,
		driver:            "sqlite",
		query:             cfg.Query,
		mergeStrategy:     resolveMergeStrategy(cfg.MergeStrategy),
		resultKey:         cfg.ResultKey,
		onError:           onError,
		cache:             cacheStore,
		cacheEnabled:      cacheEnabled,
		cacheTTL:          cacheTTL,
		cacheKey:          cfg.Cache.Key,
		timeout:           200 * time.Millisecond,
		templateEvaluator: template.NewEvaluator(),
	}
}

func assertSQLCallRecordsEqual(t *testing.T, got, want []map[string]any) {
	t.Helper()
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("records mismatch\ngot:  %#v\nwant: %#v", got, want)
	}
}

func TestSQLCall_QueryTemplating(t *testing.T) {
	db := setupSQLCallSQLiteDB(t)
	module := newSQLCallTestModule(t, db, SQLCallConfig{
		SQLRequestBase: moduleconfig.SQLRequestBase{
			Query: "SELECT name, tier, score FROM users WHERE id = {{record.user.id}} AND region = {{record.region}}",
		},
	})

	got, err := module.Process(context.Background(), []map[string]any{
		{"user": map[string]any{"id": 1}, "region": "EU", "request": map[string]any{"id": "r-1"}},
	})
	if err != nil {
		t.Fatalf("Process() error = %v", err)
	}

	assertSQLCallRecordsEqual(t, got, []map[string]any{{
		"user":    map[string]any{"id": 1},
		"region":  "EU",
		"request": map[string]any{"id": "r-1"},
		"name":    "Alice",
		"tier":    "gold",
		"score":   int64(91),
	}})
}

func TestSQLCall_CacheHitMissTTLAndEviction(t *testing.T) {
	t.Run("hit and miss", func(t *testing.T) {
		db := setupSQLCallSQLiteDB(t)
		module := newSQLCallTestModule(t, db, SQLCallConfig{
			SQLRequestBase: moduleconfig.SQLRequestBase{Query: "SELECT name FROM users WHERE id = {{record.id}}"},
			Cache:          moduleconfig.CacheConfig{Enabled: true, MaxSize: 4, DefaultTTL: 60},
		})

		record := []map[string]any{{"id": 1}}
		if _, err := module.Process(context.Background(), record); err != nil {
			t.Fatalf("first Process() error = %v", err)
		}
		if _, err := db.Exec("UPDATE users SET name = 'Changed' WHERE id = 1"); err != nil {
			t.Fatalf("updating fixture: %v", err)
		}
		got, err := module.Process(context.Background(), record)
		if err != nil {
			t.Fatalf("second Process() error = %v", err)
		}
		if got[0]["name"] != "Alice" {
			t.Fatalf("cached name = %v, want Alice", got[0]["name"])
		}

		stats := module.GetCacheStats()
		if stats.Misses != 1 || stats.Hits != 1 {
			t.Fatalf("cache stats = %+v, want 1 miss and 1 hit", stats)
		}
	})

	t.Run("ttl expiration", func(t *testing.T) {
		db := setupSQLCallSQLiteDB(t)
		module := newSQLCallTestModule(t, db, SQLCallConfig{
			SQLRequestBase: moduleconfig.SQLRequestBase{Query: "SELECT name FROM users WHERE id = {{record.id}}"},
			Cache:          moduleconfig.CacheConfig{Enabled: true, MaxSize: 4, DefaultTTL: 1},
		})
		module.cacheTTL = time.Nanosecond

		record := []map[string]any{{"id": 2}}
		if _, err := module.Process(context.Background(), record); err != nil {
			t.Fatalf("first Process() error = %v", err)
		}
		time.Sleep(time.Millisecond)
		if _, err := db.Exec("UPDATE users SET name = 'Robert' WHERE id = 2"); err != nil {
			t.Fatalf("updating fixture: %v", err)
		}
		got, err := module.Process(context.Background(), record)
		if err != nil {
			t.Fatalf("second Process() error = %v", err)
		}
		if got[0]["name"] != "Robert" {
			t.Fatalf("name after expired cache = %v, want Robert", got[0]["name"])
		}
	})

	t.Run("eviction", func(t *testing.T) {
		db := setupSQLCallSQLiteDB(t)
		module := newSQLCallTestModule(t, db, SQLCallConfig{
			SQLRequestBase: moduleconfig.SQLRequestBase{Query: "SELECT name FROM users WHERE id = {{record.id}}"},
			Cache:          moduleconfig.CacheConfig{Enabled: true, MaxSize: 1, DefaultTTL: 60},
		})

		if _, err := module.Process(context.Background(), []map[string]any{{"id": 1}}); err != nil {
			t.Fatalf("process id=1: %v", err)
		}
		if _, err := module.Process(context.Background(), []map[string]any{{"id": 2}}); err != nil {
			t.Fatalf("process id=2: %v", err)
		}
		if stats := module.GetCacheStats(); stats.Evictions != 1 {
			t.Fatalf("evictions = %d, want 1", stats.Evictions)
		}
	})
}

func TestSQLCall_MergeStrategies(t *testing.T) {
	tests := []struct {
		name   string
		cfg    SQLCallConfig
		record map[string]any
		want   map[string]any
	}{
		{
			name: "merge deep merges nested maps",
			cfg: SQLCallConfig{
				SQLRequestBase: moduleconfig.SQLRequestBase{Query: "SELECT 'gold' AS tier, 'Paris' AS city"},
				MergeStrategy:  "merge",
			},
			record: map[string]any{"id": 1, "profile": map[string]any{"active": true}},
			want:   map[string]any{"id": 1, "profile": map[string]any{"active": true}, "tier": "gold", "city": "Paris"},
		},
		{
			name: "replace overlays SQL columns",
			cfg: SQLCallConfig{
				SQLRequestBase: moduleconfig.SQLRequestBase{Query: "SELECT 'db-name' AS name"},
				MergeStrategy:  "replace",
			},
			record: map[string]any{"id": 1, "name": "input-name"},
			want:   map[string]any{"id": 1, "name": "db-name"},
		},
		{
			name: "append stores result under configured key",
			cfg: SQLCallConfig{
				SQLRequestBase: moduleconfig.SQLRequestBase{Query: "SELECT 'gold' AS tier"},
				MergeStrategy:  "append",
				ResultKey:      "enrichment",
			},
			record: map[string]any{"id": 1},
			want:   map[string]any{"id": 1, "enrichment": map[string]any{"tier": "gold"}},
		},
		{
			name: "append defaults result key",
			cfg: SQLCallConfig{
				SQLRequestBase: moduleconfig.SQLRequestBase{Query: "SELECT 'gold' AS tier"},
				MergeStrategy:  "append",
			},
			record: map[string]any{"id": 1},
			want:   map[string]any{"id": 1, "_sql_result": map[string]any{"tier": "gold"}},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			db := setupSQLCallSQLiteDB(t)
			module := newSQLCallTestModule(t, db, tt.cfg)

			got, err := module.Process(context.Background(), []map[string]any{tt.record})
			if err != nil {
				t.Fatalf("Process() error = %v", err)
			}
			assertSQLCallRecordsEqual(t, got, []map[string]any{tt.want})
		})
	}
}

func TestSQLCall_OnErrorStrategies(t *testing.T) {
	tests := []struct {
		name      string
		onError   string
		wantCount int
		wantErr   bool
	}{
		{name: "fail", onError: "fail", wantErr: true},
		{name: "skip", onError: "skip", wantCount: 0},
		{name: "log", onError: "log", wantCount: 1},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			db := setupSQLCallSQLiteDB(t)
			module := newSQLCallTestModule(t, db, SQLCallConfig{
				SQLRequestBase: moduleconfig.SQLRequestBase{Query: "SELECT missing_column FROM missing_table WHERE id = {{record.id}}"},
				ModuleBase:     connector.ModuleBase{OnError: tt.onError},
			})

			got, err := module.Process(context.Background(), []map[string]any{{"id": 1}})
			if tt.wantErr {
				if err == nil {
					t.Fatal("Process() error = nil, want error")
				}
				return
			}
			if err != nil {
				t.Fatalf("Process() error = %v", err)
			}
			if len(got) != tt.wantCount {
				t.Fatalf("len(result) = %d, want %d", len(got), tt.wantCount)
			}
		})
	}
}

func TestSQLCall_QueryFileAndEnvConnectionString(t *testing.T) {
	tmpDir := t.TempDir()
	dbPath := filepath.Join(tmpDir, "sql-call.db")
	db, err := sql.Open("sqlite", "file:"+dbPath)
	if err != nil {
		t.Fatalf("sql.Open sqlite file: %v", err)
	}
	_, err = db.Exec(`
		CREATE TABLE users (id INTEGER PRIMARY KEY, name TEXT NOT NULL);
		INSERT INTO users (id, name) VALUES (1, 'Alice');
	`)
	if err != nil {
		t.Fatalf("creating file db fixture: %v", err)
	}
	if closeErr := db.Close(); closeErr != nil {
		t.Fatalf("closing setup db: %v", closeErr)
	}

	queryPath := filepath.Join(tmpDir, "lookup.sql")
	if writeErr := os.WriteFile(queryPath, []byte("SELECT name FROM users WHERE id = {{record.id}}"), 0600); writeErr != nil {
		t.Fatalf("writing query file: %v", writeErr)
	}
	t.Setenv("SQL_CALL_TEST_DSN", "file:"+dbPath)

	module, err := NewSQLCallFromConfig(SQLCallConfig{
		SQLRequestBase: moduleconfig.SQLRequestBase{
			ConnectionStringRef: "${SQL_CALL_TEST_DSN}",
			Driver:              "sqlite",
			QueryFile:           queryPath,
		},
	})
	if err != nil {
		t.Fatalf("NewSQLCallFromConfig() error = %v", err)
	}
	defer func() {
		if closeErr := module.Close(); closeErr != nil {
			t.Errorf("closing sql_call module: %v", closeErr)
		}
	}()

	got, err := module.Process(context.Background(), []map[string]any{{"id": 1}})
	if err != nil {
		t.Fatalf("Process() error = %v", err)
	}
	if got[0]["name"] != "Alice" {
		t.Fatalf("name = %v, want Alice", got[0]["name"])
	}
}

func TestSQLCall_ContextCancellationAndEmptyResult(t *testing.T) {
	t.Run("canceled context", func(t *testing.T) {
		db := setupSQLCallSQLiteDB(t)
		module := newSQLCallTestModule(t, db, SQLCallConfig{
			SQLRequestBase: moduleconfig.SQLRequestBase{Query: "SELECT name FROM users WHERE id = {{record.id}}"},
		})

		ctx, cancel := context.WithCancel(context.Background())
		cancel()
		_, err := module.Process(ctx, []map[string]any{{"id": 1}})
		if !errors.Is(err, context.Canceled) {
			t.Fatalf("Process() error = %v, want context.Canceled", err)
		}
	})

	t.Run("module timeout expires during query", func(t *testing.T) {
		db := setupSQLCallSQLiteDB(t)
		module := newSQLCallTestModule(t, db, SQLCallConfig{
			SQLRequestBase: moduleconfig.SQLRequestBase{Query: "SELECT name FROM users WHERE id = {{record.id}}"},
		})
		// The module derives a per-query context via context.WithTimeout(ctx, m.timeout)
		// in executeQuery. Setting it to a nanosecond guarantees the deadline has
		// elapsed before QueryContext can run, exercising the timeout branch.
		module.timeout = time.Nanosecond

		_, err := module.Process(context.Background(), []map[string]any{{"id": 1}})
		if err == nil {
			t.Fatal("Process() error = nil, want deadline error")
		}
		if !errors.Is(err, context.DeadlineExceeded) {
			t.Fatalf("Process() error = %v, want context.DeadlineExceeded", err)
		}
	})

	t.Run("empty result", func(t *testing.T) {
		db := setupSQLCallSQLiteDB(t)
		module := newSQLCallTestModule(t, db, SQLCallConfig{
			SQLRequestBase: moduleconfig.SQLRequestBase{Query: "SELECT name FROM users WHERE id = {{record.id}}"},
			MergeStrategy:  "append",
		})

		got, err := module.Process(context.Background(), []map[string]any{{"id": 999}})
		if err != nil {
			t.Fatalf("Process() error = %v", err)
		}
		assertSQLCallRecordsEqual(t, got, []map[string]any{{"id": 999, "_sql_result": map[string]any{}}})
	})
}

func TestSQLCall_ConcurrentProcess(t *testing.T) {
	db := setupSQLCallSQLiteDB(t)
	module := newSQLCallTestModule(t, db, SQLCallConfig{
		SQLRequestBase: moduleconfig.SQLRequestBase{Query: "SELECT name FROM users WHERE id = {{record.id}}"},
		Cache:          moduleconfig.CacheConfig{Enabled: true, MaxSize: 16, DefaultTTL: 60},
	})

	var wg sync.WaitGroup
	errs := make(chan error, 100)
	for i := 0; i < 100; i++ {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			id := i%3 + 1
			got, err := module.Process(context.Background(), []map[string]any{{"id": id}})
			if err != nil {
				errs <- err
				return
			}
			if len(got) != 1 || got[0]["name"] == nil {
				errs <- errors.New("missing enriched name")
			}
		}(i)
	}
	wg.Wait()
	close(errs)

	for err := range errs {
		if err != nil {
			t.Fatal(err)
		}
	}
}
