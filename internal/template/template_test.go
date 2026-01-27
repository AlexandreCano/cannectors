package template

import (
	"testing"
)

func TestEvaluator_BasicTemplates(t *testing.T) {
	e := NewEvaluator()

	t.Run("simple field access", func(t *testing.T) {
		template := "Hello {{name}}"
		record := map[string]interface{}{"name": "World"}
		result := e.Evaluate(template, record)
		if result != "Hello World" {
			t.Errorf("Evaluate() = %q, want %q", result, "Hello World")
		}
	})

	t.Run("nested field access", func(t *testing.T) {
		template := "User: {{user.name}}"
		record := map[string]interface{}{
			"user": map[string]interface{}{"name": "John"},
		}
		result := e.Evaluate(template, record)
		if result != "User: John" {
			t.Errorf("Evaluate() = %q, want %q", result, "User: John")
		}
	})
}

func TestEvaluator_MetadataAccess(t *testing.T) {
	e := NewEvaluator()

	t.Run("access metadata field directly", func(t *testing.T) {
		template := "Processed: {{_metadata.processed}}"
		record := map[string]interface{}{
			"name": "test",
			"_metadata": map[string]interface{}{
				"processed": "true",
			},
		}
		result := e.Evaluate(template, record)
		if result != "Processed: true" {
			t.Errorf("Evaluate() = %q, want %q", result, "Processed: true")
		}
	})

	t.Run("access nested metadata", func(t *testing.T) {
		template := "Timing: {{_metadata.timing.start}}"
		record := map[string]interface{}{
			"name": "test",
			"_metadata": map[string]interface{}{
				"timing": map[string]interface{}{
					"start": "2024-01-01T00:00:00Z",
				},
			},
		}
		result := e.Evaluate(template, record)
		if result != "Timing: 2024-01-01T00:00:00Z" {
			t.Errorf("Evaluate() = %q, want %q", result, "Timing: 2024-01-01T00:00:00Z")
		}
	})

	t.Run("metadata in URL template", func(t *testing.T) {
		template := "/api/resource/{{_metadata.resource_id}}"
		record := map[string]interface{}{
			"name": "test",
			"_metadata": map[string]interface{}{
				"resource_id": "abc123",
			},
		}
		result := e.EvaluateForURL(template, record)
		if result != "/api/resource/abc123" {
			t.Errorf("EvaluateForURL() = %q, want %q", result, "/api/resource/abc123")
		}
	})

	t.Run("metadata with default value", func(t *testing.T) {
		template := "Status: {{_metadata.status | default: \"unknown\"}}"
		record := map[string]interface{}{
			"name":      "test",
			"_metadata": map[string]interface{}{
				// status not set
			},
		}
		result := e.Evaluate(template, record)
		if result != "Status: unknown" {
			t.Errorf("Evaluate() = %q, want %q", result, "Status: unknown")
		}
	})

	t.Run("metadata in headers", func(t *testing.T) {
		headers := map[string]string{
			"X-Processed-At": "{{_metadata.processed_at}}",
			"X-Batch-ID":     "{{_metadata.batch_id}}",
		}
		record := map[string]interface{}{
			"name": "test",
			"_metadata": map[string]interface{}{
				"processed_at": "2024-01-01T12:00:00Z",
				"batch_id":     "batch-123",
			},
		}
		result := e.EvaluateHeaders(headers, record)
		if result["X-Processed-At"] != "2024-01-01T12:00:00Z" {
			t.Errorf("X-Processed-At = %q, want %q", result["X-Processed-At"], "2024-01-01T12:00:00Z")
		}
		if result["X-Batch-ID"] != "batch-123" {
			t.Errorf("X-Batch-ID = %q, want %q", result["X-Batch-ID"], "batch-123")
		}
	})

	t.Run("custom metadata field name", func(t *testing.T) {
		template := "Custom: {{_custom.field}}"
		record := map[string]interface{}{
			"name": "test",
			"_custom": map[string]interface{}{
				"field": "value",
			},
		}
		result := e.Evaluate(template, record)
		if result != "Custom: value" {
			t.Errorf("Evaluate() = %q, want %q", result, "Custom: value")
		}
	})
}

func TestGetNestedValue(t *testing.T) {
	t.Run("gets metadata from record", func(t *testing.T) {
		record := map[string]interface{}{
			"_metadata": map[string]interface{}{
				"processed": true,
			},
		}
		val, found := GetNestedValue(record, "_metadata.processed")
		if !found {
			t.Error("expected to find _metadata.processed")
		}
		if val != true {
			t.Errorf("GetNestedValue() = %v, want true", val)
		}
	})
}

func TestValidateSyntax(t *testing.T) {
	t.Run("valid metadata template", func(t *testing.T) {
		err := ValidateSyntax("{{_metadata.field}}")
		if err != nil {
			t.Errorf("ValidateSyntax() error = %v, want nil", err)
		}
	})
}
