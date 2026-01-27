// Package output provides implementations for output modules.
// Output modules are responsible for sending data to destination systems.
package output

import (
	"github.com/canectors/runtime/internal/template"
)

// Re-export template types and functions from the shared package for backward compatibility

// Template syntax constants
const (
	TemplatePrefix              = template.TemplatePrefix
	TemplateSuffix              = template.TemplateSuffix
	DefaultValueSeparator       = template.DefaultValueSeparator
	DefaultKeyword              = template.DefaultKeyword
	ErrMsgInvalidTemplateSyntax = template.ErrMsgInvalidTemplateSyntax
	ErrMsgMissingClosingBrace   = template.ErrMsgMissingClosingBrace
	ErrMsgEmptyVariablePath     = template.ErrMsgEmptyVariablePath
)

// TemplateVariable represents a parsed template variable (alias for template.Variable)
type TemplateVariable = template.Variable

// TemplateEvaluator wraps the shared template.Evaluator with backward-compatible method names.
type TemplateEvaluator struct {
	*template.Evaluator
}

// NewTemplateEvaluator creates a new template evaluator.
func NewTemplateEvaluator() *TemplateEvaluator {
	return &TemplateEvaluator{
		Evaluator: template.NewEvaluator(),
	}
}

// EvaluateTemplate evaluates a template string using the provided record data.
// Backward-compatible wrapper for template.Evaluator.Evaluate.
func (e *TemplateEvaluator) EvaluateTemplate(templateStr string, record map[string]interface{}) string {
	return e.Evaluate(templateStr, record)
}

// EvaluateTemplateForURL evaluates a template string for use in URLs.
// Backward-compatible wrapper for template.Evaluator.EvaluateForURL.
func (e *TemplateEvaluator) EvaluateTemplateForURL(templateStr string, record map[string]interface{}) string {
	return e.EvaluateForURL(templateStr, record)
}

// ParseTemplateVariables extracts all template variables from a template string.
// Backward-compatible wrapper for template.Evaluator.ParseVariables.
func (e *TemplateEvaluator) ParseTemplateVariables(templateStr string) []TemplateVariable {
	return e.ParseVariables(templateStr)
}

// HasTemplateVariables checks if a string contains template variables.
func HasTemplateVariables(s string) bool {
	return template.HasVariables(s)
}

// ValidateTemplateSyntax validates that a template string has valid syntax.
func ValidateTemplateSyntax(templateStr string) error {
	return template.ValidateSyntax(templateStr)
}
