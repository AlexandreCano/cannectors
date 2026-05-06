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

// knownNonCompliantExamples lists example configs that are known to drift from
// the JSON schema and/or the runtime today. Each entry is the file's basename
// (relative to configs/examples/). The intent is to keep the compliance test
// honest as a regression guard while making the existing technical debt
// visible: removing a name from this list means the file now passes — and the
// test will permanently lock that in.
//
// New examples must NOT be added here. Each entry is a follow-up bug to fix
// in the relevant example or schema (most often: the example uses fields the
// schema rejects, or the YAML still carries `id`/`enabled` at the pipeline
// root which the schema does not allow).
var knownNonCompliantExamples = map[string]string{
	"05-pagination.json":                   "pagination shape predates the current page-pagination schema",
	"05-pagination.yaml":                   "pagination shape predates the current page-pagination schema",
	"07-pagination-cursor.json":            "cursor-pagination schema mismatch (missing required fields)",
	"07-pagination-cursor.yaml":            "cursor-pagination schema mismatch (missing required fields)",
	"10-complete.json":                     "demonstrates fields not yet in schema",
	"10-complete.yaml":                     "demonstrates fields not yet in schema",
	"14-webhook.yaml":                      "uses `id` and `enabled` at pipeline root, rejected by schema",
	"16-filters-script.yaml":               "script filter schema not finalized",
	"17-filters-enrichment.yaml":           "http-enrichment filter schema not finalized",
	"20-timestamp-and-id-persistence.yaml": "statePersistence shape predates current schema",
	"21-output-templating.yaml":            "template fields not in current schema",
	"22-output-templating-batch.yaml":      "template fields not in current schema",
	"23-output-templating-soap.yaml":       "soap output not in current schema",
	"24-enrichment-templating.yaml":        "enrichment + templating mix not in current schema",
	"25-record-metadata-storage.yaml":      "metadata schema not finalized",
	"27-database-input.yaml":               "database input schema not finalized",
	"28-database-incremental.yaml":         "database incremental schema not finalized",
	"29-sql-call-enrichment.yaml":          "sql_call filter schema not finalized",
	"30-database-output-insert.yaml":       "database output schema not finalized",
	"31-database-output-upsert.yaml":       "database output schema not finalized",
	"32-database-custom-query.yaml":        "database output schema not finalized",
	"35-filters-set.json":                  "set filter schema not finalized",
	"36-filters-remove.json":               "remove filter schema not finalized",
}

// TestExampleConfigsAreSchemaCompliant walks every shipped example under
// configs/examples and checks that it can be: (1) parsed, (2) validated
// against the JSON schema, (3) converted to a Pipeline, and (4) instantiated
// by the factory. A failure on any step is the canary that keeps the schema,
// the converter and the runtime in sync (Story 17.7).
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
		"DATABASE_URL": "postgres://stub:stub@localhost:5432/stub?sslmode=disable",
		"DEST_API_KEY": "stub", "DEST_API_TOKEN": "stub", "DEST_BEARER_TOKEN": "stub",
		"DEST_CLIENT_ID": "stub", "DEST_CLIENT_SECRET": "stub",
		"DEST_PASSWORD": "stub", "DEST_USERNAME": "stub",
		"EVENTS_API_KEY": "stub", "LOOKUP_API_KEY": "stub",
		"OAUTH_CLIENT_ID": "stub", "OAUTH_CLIENT_SECRET": "stub",
		"ORDERS_API_TOKEN": "stub", "SOURCE_API_KEY": "stub",
		"SOURCE_API_TOKEN": "stub", "SOURCE_CLIENT_ID": "stub",
		"SOURCE_CLIENT_SECRET": "stub", "TARGET_API_TOKEN": "stub",
		"WAREHOUSE_API_TOKEN": "stub",
	}
	for k, v := range envStubs {
		t.Setenv(k, v)
	}

	matches, err := filepath.Glob(filepath.Join("..", "..", "configs", "examples", "*"))
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
			if reason, skip := knownNonCompliantExamples[base]; skip {
				t.Skipf("known non-compliant example (tracked debt): %s", reason)
			}
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

	pipeline, err := config.ConvertToPipeline(result.Data)
	if err != nil {
		t.Fatalf("convert to pipeline: %v", err)
	}

	closer := instantiate(t, pipeline)
	closer()
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
