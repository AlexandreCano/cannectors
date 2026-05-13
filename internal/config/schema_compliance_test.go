package config_test

import (
	"errors"
	"path/filepath"
	"strings"
	"testing"

	"github.com/cannectors/runtime/internal/config"
	"github.com/cannectors/runtime/internal/factory"
	"github.com/cannectors/runtime/pkg/connector"
)

// TestExampleConfigsAreSchemaCompliant walks every shipped example under
// examples and checks that it can be: (1) parsed, (2) validated
// against the JSON schema, (3) converted to a Pipeline, and (4) instantiated
// by the factory. A failure on any step is the canary that keeps the schema,
// the converter, and the runtime in sync.
//
// The test runs each example as a sub-test (t.Run) so failures point at the
// exact file. Modules that require external infrastructure (e.g. a live
// database connection) are skipped at the instantiation step in -short mode
// to keep the suite hermetic.
func TestExampleConfigsAreSchemaCompliant(t *testing.T) {
	// Stub credentials/connection strings for every ${VAR} appearing in the
	// examples. These never hit a network — they only need to satisfy the
	// constructors that read them eagerly (database.ResolveConnectionString,
	// in particular).
	envStubs := map[string]string{
		"API_KEY": "stub", "API_PASSWORD": "stub", "API_TOKEN": "stub",
		"API_USERNAME": "stub", "CRM_API_KEY": "stub", "CRM_API_TOKEN": "stub",
		"CRM_DATABASE_URL": "postgres://stub:stub@localhost:5432/stub?sslmode=disable",
		"DATABASE_URL":     "postgres://stub:stub@localhost:5432/stub?sslmode=disable",
		"DEST_API_KEY":     "stub", "DEST_API_TOKEN": "stub", "DEST_BEARER_TOKEN": "stub",
		"DEST_CLIENT_ID": "stub", "DEST_CLIENT_SECRET": "stub",
		"DEST_PASSWORD": "stub", "DEST_USERNAME": "stub",
		"DESTINATION_API_KEY": "stub", "DIRECTORY_PASSWORD": "stub", "DIRECTORY_USERNAME": "stub",
		"EVENTS_API_KEY": "stub", "LOOKUP_API_KEY": "stub",
		"OAUTH_CLIENT_ID": "stub", "OAUTH_CLIENT_SECRET": "stub",
		"REFERENCE_DATABASE_URL": "postgres://stub:stub@localhost:5432/stub?sslmode=disable",
		"ORDERS_API_TOKEN":       "stub", "SOURCE_API_KEY": "stub",
		"SOAP_PASSWORD":       "stub",
		"SOURCE_BEARER_TOKEN": "stub",
		"SOURCE_API_TOKEN":    "stub", "SOURCE_CLIENT_ID": "stub",
		"SOURCE_CLIENT_SECRET":   "stub",
		"SOURCE_DATABASE_URL":    "postgres://stub:stub@localhost:5432/stub?sslmode=disable",
		"TARGET_API_TOKEN":       "stub",
		"WAREHOUSE_API_TOKEN":    "stub",
		"WAREHOUSE_DATABASE_URL": "postgres://stub:stub@localhost:5432/stub?sslmode=disable",
		"WEBHOOK_SECRET":         "stub",
	}
	for k, v := range envStubs {
		t.Setenv(k, v)
	}

	matches, err := filepath.Glob(filepath.Join("..", "..", "examples", "*"))
	if err != nil {
		t.Fatalf("glob examples: %v", err)
	}
	if len(matches) == 0 {
		t.Fatal("no example configs found — did the fixtures move?")
	}

	for _, path := range matches {
		ext := strings.ToLower(filepath.Ext(path))
		if ext != ".yaml" && ext != ".yml" && ext != ".json" {
			continue
		}
		base := filepath.Base(path)
		t.Run(base, func(t *testing.T) {
			t.Parallel()
			assertExampleCompliant(t, path)
		})
	}
}

// assertExampleCompliant runs the four checks required by Story 17.7 AC #1.
// Modules that need external resources are tolerated when the failure is
// clearly an infrastructure problem (e.g. database connection refused) rather
// than a schema/runtime mismatch.
func assertExampleCompliant(t *testing.T, path string) {
	t.Helper()

	result := config.ParseConfig(path)
	if len(result.ParseErrors) > 0 {
		t.Fatalf("parse errors: %+v", result.ParseErrors)
	}
	if len(result.ValidationErrors) > 0 {
		t.Fatalf("schema validation errors: %+v", result.ValidationErrors)
	}
	normalizeExampleAssetPaths(t, result.Data)

	pipeline, err := config.ConvertToPipeline(result.Data)
	if err != nil {
		t.Fatalf("convert to pipeline: %v", err)
	}

	closer := instantiate(t, pipeline)
	closer()
}

// normalizeExampleAssetPaths makes example asset references work when this test
// runs from the internal/config package directory instead of the repository
// root. The example files intentionally keep repo-root-relative paths because
// that is how users run them from the CLI.
func normalizeExampleAssetPaths(t *testing.T, value any) {
	t.Helper()
	switch typed := value.(type) {
	case map[string]any:
		for key, child := range typed {
			if pathValue, ok := child.(string); ok && isExampleAssetPathKey(key) && strings.HasPrefix(pathValue, "examples/assets/") {
				abs, err := filepath.Abs(filepath.Join("..", "..", pathValue))
				if err != nil {
					t.Fatalf("resolve example asset path %q: %v", pathValue, err)
				}
				typed[key] = abs
				continue
			}
			normalizeExampleAssetPaths(t, child)
		}
	case []any:
		for _, child := range typed {
			normalizeExampleAssetPaths(t, child)
		}
	}
}

func isExampleAssetPathKey(key string) bool {
	return key == "scriptFile" || key == "queryFile" || key == "bodyTemplateFile"
}

// instantiate creates every module described by the pipeline via the public
// factory. It returns a teardown function that closes everything that was
// successfully created. Database modules need a live server and are skipped:
// their factory call is still issued, but a connection failure is reported
// as t.Skip rather than t.Fatal.
func instantiate(t *testing.T, pipeline *connector.Pipeline) func() {
	t.Helper()
	var teardowns []func()
	defer func() {
		if t.Failed() {
			for _, td := range teardowns {
				td()
			}
		}
	}()

	// Input module.
	if pipeline.Input != nil {
		if pipeline.Input.Type == "database" && testing.Short() {
			t.Skipf("skipping database input instantiation in -short mode")
		}
		in, err := factory.CreateInputModule(pipeline.Input)
		if err != nil {
			if isInfraError(err) {
				t.Skipf("input %q skipped — needs live infrastructure: %v", pipeline.Input.Type, err)
			}
			t.Fatalf("CreateInputModule(%s): %v", pipeline.Input.Type, err)
		}
		if in != nil {
			teardowns = append(teardowns, func() { _ = in.Close() })
		}
	}

	// Filters.
	filters, err := factory.CreateFilterModules(pipeline.Filters)
	if err != nil {
		if isInfraError(err) {
			t.Skipf("filter chain skipped — needs live infrastructure: %v", err)
		}
		t.Fatalf("CreateFilterModules: %v", err)
	}
	for _, f := range filters {
		f := f
		teardowns = append(teardowns, func() {
			if closer, ok := f.(interface{ Close() error }); ok {
				_ = closer.Close()
			}
		})
	}

	// Output module.
	if pipeline.Output != nil {
		if pipeline.Output.Type == "database" && testing.Short() {
			t.Skipf("skipping database output instantiation in -short mode")
		}
		out, err := factory.CreateOutputModule(pipeline.Output)
		if err != nil {
			if isInfraError(err) {
				t.Skipf("output %q skipped — needs live infrastructure: %v", pipeline.Output.Type, err)
			}
			t.Fatalf("CreateOutputModule(%s): %v", pipeline.Output.Type, err)
		}
		if out != nil {
			teardowns = append(teardowns, func() { _ = out.Close() })
		}
	}

	return func() {
		for _, td := range teardowns {
			td()
		}
	}
}

// isInfraError returns true when an error comes from a missing external
// resource (DB, file system, etc.) rather than a config/schema mismatch.
// We deliberately keep this list narrow — it must not mask schema bugs.
func isInfraError(err error) bool {
	if err == nil {
		return false
	}
	msg := err.Error()
	switch {
	case strings.Contains(msg, "creating database connection"),
		strings.Contains(msg, "creating database output connection"),
		strings.Contains(msg, "creating sql_call database connection"),
		strings.Contains(msg, "database connection failed"),
		strings.Contains(msg, "unknown driver"),
		strings.Contains(msg, "connection refused"),
		strings.Contains(msg, "no such host"),
		strings.Contains(msg, "dial tcp"):
		return true
	}
	// Unknown registry types are NOT infra errors — they're schema mismatches.
	return errors.Is(err, errMockedSentinelToHelpReadability)
}

// errMockedSentinelToHelpReadability exists only so the switch above can stay
// readable when more cases are added. It is intentionally unmatched.
var errMockedSentinelToHelpReadability = errors.New("__never_matched__")
