package config_test

import (
	"path/filepath"
	"testing"

	"github.com/cannectors/runtime/internal/config"
)

func TestSOAPExamplesValidate(t *testing.T) {
	examples := []string{
		"40-soap-polling-basic-v11.yaml",
		"40b-soap-polling-basic-v12.yaml",
		"41-soap-polling-cursor.yaml",
		"42-soap-call-enrichment.yaml",
		"43-soap-output-batch.yaml",
		"44-soap-output-mtom-emission.yaml",
		"44b-soap-input-mtom-reception.yaml",
		"45-soap-output-wssecurity-passwordtext.yaml",
		"45b-soap-output-wssecurity-passworddigest.yaml",
	}
	for _, name := range examples {
		t.Run(name, func(t *testing.T) {
			path := filepath.Join("..", "..", "examples", name)
			result := config.ParseConfig(path)
			if len(result.ParseErrors) > 0 {
				t.Fatalf("parse errors: %+v", result.ParseErrors)
			}
			if len(result.ValidationErrors) > 0 {
				t.Fatalf("validation errors: %+v", result.ValidationErrors)
			}
			if _, err := config.ConvertToPipeline(result.Data); err != nil {
				t.Fatalf("ConvertToPipeline: %v", err)
			}
		})
	}
}

func TestSOAPTestLabPipelinesValidate(t *testing.T) {
	pipelines := []string{
		"soap-polling-v11.yaml",
		"soap-polling-v12.yaml",
		"soap-output-wssecurity-passwordtext.yaml",
		"soap-output-wssecurity-passworddigest.yaml",
		"soap-output-fault.yaml",
		"soap-output-retry-on-fault.yaml",
		"soap-output-mtom.yaml",
	}
	for _, name := range pipelines {
		t.Run(name, func(t *testing.T) {
			path := filepath.Join("..", "..", "test-lab", "pipelines", name)
			result := config.ParseConfig(path)
			if len(result.ParseErrors) > 0 {
				t.Fatalf("parse errors: %+v", result.ParseErrors)
			}
			if len(result.ValidationErrors) > 0 {
				t.Fatalf("validation errors: %+v", result.ValidationErrors)
			}
			if _, err := config.ConvertToPipeline(result.Data); err != nil {
				t.Fatalf("ConvertToPipeline: %v", err)
			}
		})
	}
}
