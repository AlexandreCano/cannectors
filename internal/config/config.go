// Package config provides functionality for parsing and validating
// pipeline configuration files (JSON/YAML).
//
// This package will be implemented in Story 2.2: Implement Configuration Parser.
package config

import (
	"errors"

	"github.com/canectors/runtime/pkg/connector"
)

// ErrNotImplemented is returned when a feature is not yet implemented.
var ErrNotImplemented = errors.New("not implemented: will be added in Story 2.2")

// Loader is responsible for loading pipeline configurations from files.
type Loader struct {
	// basePath is the base directory for configuration files
	basePath string
}

// NewLoader creates a new configuration loader.
func NewLoader(basePath string) *Loader {
	return &Loader{
		basePath: basePath,
	}
}

// Load reads and parses a pipeline configuration from a file.
// Supports both JSON and YAML formats.
//
// TODO: Implement in Story 2.2
func (l *Loader) Load(filepath string) (*connector.Pipeline, error) {
	return nil, ErrNotImplemented
}

// Validate checks a pipeline configuration against the JSON schema.
//
// TODO: Implement in Story 2.2
func (l *Loader) Validate(pipeline *connector.Pipeline) error {
	return ErrNotImplemented
}
