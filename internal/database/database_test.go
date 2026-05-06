package database

import (
	"os"
	"strings"
	"testing"
)

func TestResolveEnvRef(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name     string
		ref      string
		envVar   string
		envValue string
		want     string
	}{
		{
			name:     "valid env ref",
			ref:      "${TEST_DB_CONN}",
			envVar:   "TEST_DB_CONN",
			envValue: "postgres://user:pass@host:5432/db",
			want:     "postgres://user:pass@host:5432/db",
		},
		{
			name:   "invalid format - no prefix",
			ref:    "TEST_DB_CONN}",
			envVar: "TEST_DB_CONN",
			want:   "",
		},
		{
			name:   "invalid format - no suffix",
			ref:    "${TEST_DB_CONN",
			envVar: "TEST_DB_CONN",
			want:   "",
		},
		{
			name:   "env var not set",
			ref:    "${NONEXISTENT_VAR_12345}",
			envVar: "",
			want:   "",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if tt.envVar != "" && tt.envValue != "" {
				_ = os.Setenv(tt.envVar, tt.envValue)
				t.Cleanup(func() { _ = os.Unsetenv(tt.envVar) })
			}

			got := ResolveEnvRef(tt.ref)
			if got != tt.want {
				t.Errorf("ResolveEnvRef(%q) = %q, want %q", tt.ref, got, tt.want)
			}
		})
	}
}

func TestDetectDriver(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name       string
		connString string
		want       string
		wantErr    bool
	}{
		{
			name:       "postgres URL",
			connString: "postgres://user:pass@localhost:5432/mydb",
			want:       DriverPostgres,
		},
		{
			name:       "postgresql URL",
			connString: "postgresql://user:pass@localhost:5432/mydb?sslmode=disable",
			want:       DriverPostgres,
		},
		{
			name:       "postgres URL with options",
			connString: "postgres://user:pass@localhost:5432/mydb?sslmode=require",
			want:       DriverPostgres,
		},
		{
			name:       "mysql tcp DSN",
			connString: "user:password@tcp(localhost:3306)/mydb",
			want:       DriverMySQL,
		},
		{
			name:       "mysql unix DSN",
			connString: "user:password@unix(/var/run/mysql.sock)/mydb",
			want:       DriverMySQL,
		},
		{
			name:       "sqlite file URL",
			connString: "file:./test.db",
			want:       DriverSQLite,
		},
		{
			name:       "sqlite .db extension",
			connString: "./data/mydb.db",
			want:       DriverSQLite,
		},
		{
			name:       "sqlite .sqlite extension",
			connString: "/path/to/database.sqlite",
			want:       DriverSQLite,
		},
		{
			name:       "unknown format",
			connString: "unknown://something",
			wantErr:    true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := DetectDriver(tt.connString)
			if (err != nil) != tt.wantErr {
				t.Errorf("DetectDriver(%q) error = %v, wantErr %v", tt.connString, err, tt.wantErr)
				return
			}
			if got != tt.want {
				t.Errorf("DetectDriver(%q) = %q, want %q", tt.connString, got, tt.want)
			}
		})
	}
}

func TestIsDriverSupported(t *testing.T) {
	t.Parallel()

	tests := []struct {
		driver string
		want   bool
	}{
		{DriverPostgres, true},
		{DriverMySQL, true},
		{DriverSQLite, true},
		{"oracle", false},
		{"sqlserver", false},
		{"", false},
	}

	for _, tt := range tests {
		t.Run(tt.driver, func(t *testing.T) {
			if got := IsDriverSupported(tt.driver); got != tt.want {
				t.Errorf("IsDriverSupported(%q) = %v, want %v", tt.driver, got, tt.want)
			}
		})
	}
}

func TestSanitizeConnectionString(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name       string
		connString string
		wantPrefix string
	}{
		{
			name:       "postgres URL with password",
			connString: "postgres://user:secretpass@localhost:5432/mydb",
			wantPrefix: "postgres://user:",
		},
		{
			name:       "postgres URL with special chars in password",
			connString: "postgres://user:p%40ssw0rd@localhost:5432/mydb",
			wantPrefix: "postgres://user:",
		},
		{
			name:       "mysql DSN with password",
			connString: "user:password@tcp(localhost:3306)/mydb",
			wantPrefix: "user:[REDACTED]@",
		},
		{
			name:       "sqlite path (no password)",
			connString: "file:./test.db",
			wantPrefix: "file:",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := SanitizeConnectionString(tt.connString)
			// For postgres URLs, verify password is redacted
			if tt.name != "sqlite path (no password)" && tt.name != "mysql DSN with password" {
				if !strings.Contains(got, "REDACTED") {
					t.Errorf("SanitizeConnectionString(%q) = %q, expected REDACTED in output", tt.connString, got)
				}
			}
			if !strings.HasPrefix(got, tt.wantPrefix) {
				t.Errorf("SanitizeConnectionString(%q) = %q, expected prefix %q", tt.connString, got, tt.wantPrefix)
			}
		})
	}
}

func TestGetPlaceholderStyle(t *testing.T) {
	t.Parallel()

	tests := []struct {
		driver string
		want   PlaceholderStyle
	}{
		{DriverPostgres, PlaceholderDollar},
		{DriverMySQL, PlaceholderQuestion},
		{DriverSQLite, PlaceholderQuestion},
	}

	for _, tt := range tests {
		t.Run(tt.driver, func(t *testing.T) {
			if got := GetPlaceholderStyle(tt.driver); got != tt.want {
				t.Errorf("GetPlaceholderStyle(%q) = %v, want %v", tt.driver, got, tt.want)
			}
		})
	}
}

func TestFormatPlaceholder(t *testing.T) {
	t.Parallel()

	tests := []struct {
		driver string
		index  int
		want   string
	}{
		{DriverPostgres, 1, "$1"},
		{DriverPostgres, 5, "$5"},
		{DriverMySQL, 1, "?"},
		{DriverMySQL, 5, "?"},
		{DriverSQLite, 1, "?"},
	}

	for _, tt := range tests {
		t.Run(tt.driver, func(t *testing.T) {
			if got := FormatPlaceholder(tt.driver, tt.index); got != tt.want {
				t.Errorf("FormatPlaceholder(%q, %d) = %q, want %q", tt.driver, tt.index, got, tt.want)
			}
		})
	}
}

func TestConvertPlaceholders(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name   string
		query  string
		driver string
		want   string
	}{
		{
			name:   "postgres conversion",
			query:  "SELECT * FROM users WHERE id = ? AND name = ?",
			driver: DriverPostgres,
			want:   "SELECT * FROM users WHERE id = $1 AND name = $2",
		},
		{
			name:   "mysql no conversion",
			query:  "SELECT * FROM users WHERE id = ? AND name = ?",
			driver: DriverMySQL,
			want:   "SELECT * FROM users WHERE id = ? AND name = ?",
		},
		{
			name:   "sqlite no conversion",
			query:  "SELECT * FROM users WHERE id = ?",
			driver: DriverSQLite,
			want:   "SELECT * FROM users WHERE id = ?",
		},
		{
			name:   "postgres multiple placeholders",
			query:  "INSERT INTO t (a, b, c) VALUES (?, ?, ?)",
			driver: DriverPostgres,
			want:   "INSERT INTO t (a, b, c) VALUES ($1, $2, $3)",
		},
		// Story 17.6 AC #1: `?` inside single-quoted string literals must not
		// be converted; only the bind-marker `?` becomes a numbered placeholder.
		{
			name:   "postgres ? inside single-quoted string is preserved",
			query:  "SELECT * FROM t WHERE a = ? AND b LIKE '%?%'",
			driver: DriverPostgres,
			want:   "SELECT * FROM t WHERE a = $1 AND b LIKE '%?%'",
		},
		// Story 17.6 AC #1: doubled quote inside string is the SQL escape and
		// must not terminate the literal early.
		{
			name:   "postgres escaped quote does not terminate literal",
			query:  "SELECT ? WHERE n = 'it''s ? a test'",
			driver: DriverPostgres,
			want:   "SELECT $1 WHERE n = 'it''s ? a test'",
		},
		// Story 17.6 AC #2: line-comment ?'s are preserved.
		{
			name:   "postgres ? in single-line comment preserved",
			query:  "SELECT ? -- ? is a joke\nFROM t",
			driver: DriverPostgres,
			want:   "SELECT $1 -- ? is a joke\nFROM t",
		},
		// Story 17.6 AC #2: block-comment ?'s are preserved.
		{
			name:   "postgres ? in block comment preserved",
			query:  "SELECT ? /* who? me? */ FROM t WHERE a = ?",
			driver: DriverPostgres,
			want:   "SELECT $1 /* who? me? */ FROM t WHERE a = $2",
		},
		// Story 17.6 AC #5: MySQL/SQLite drivers stay no-op.
		{
			name:   "mysql with ? in literal stays untouched",
			query:  "SELECT * FROM t WHERE a = ? AND b LIKE '%?%'",
			driver: DriverMySQL,
			want:   "SELECT * FROM t WHERE a = ? AND b LIKE '%?%'",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := ConvertPlaceholders(tt.query, tt.driver); got != tt.want {
				t.Errorf("ConvertPlaceholders(%q, %q) = %q, want %q", tt.query, tt.driver, got, tt.want)
			}
		})
	}
}

func TestResolveConnectionString(t *testing.T) {
	t.Parallel()

	t.Run("env ref takes precedence", func(t *testing.T) {
		_ = os.Setenv("TEST_CONN_REF", "postgres://from-env")
		t.Cleanup(func() { _ = os.Unsetenv("TEST_CONN_REF") })

		got, err := ResolveConnectionString("postgres://from-string", "${TEST_CONN_REF}")
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if got != "postgres://from-env" {
			t.Errorf("got %q, want postgres://from-env", got)
		}
	})

	t.Run("falls back to connection string", func(t *testing.T) {
		got, err := ResolveConnectionString("postgres://from-string", "${NONEXISTENT_VAR_XYZ}")
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if got != "postgres://from-string" {
			t.Errorf("got %q, want postgres://from-string", got)
		}
	})

	t.Run("error when no connection string", func(t *testing.T) {
		_, err := ResolveConnectionString("", "")
		if err != ErrMissingConnectionString {
			t.Errorf("got error %v, want ErrMissingConnectionString", err)
		}
	})
}

func TestGetDriverName(t *testing.T) {
	t.Parallel()

	tests := []struct {
		driver string
		want   string
	}{
		{DriverPostgres, "pgx"},
		{DriverMySQL, "mysql"},
		{DriverSQLite, "sqlite"},
		{"unknown", "unknown"},
	}

	for _, tt := range tests {
		t.Run(tt.driver, func(t *testing.T) {
			if got := GetDriverName(tt.driver); got != tt.want {
				t.Errorf("GetDriverName(%q) = %q, want %q", tt.driver, got, tt.want)
			}
		})
	}
}
