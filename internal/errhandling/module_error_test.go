package errhandling_test

import (
	"errors"
	"fmt"
	"testing"

	"github.com/cannectors/runtime/internal/errhandling"
)

type fakeModuleError struct {
	code        string
	module      string
	recordIndex int
	details     map[string]interface{}
	msg         string
}

func (e *fakeModuleError) Error() string                        { return e.msg }
func (e *fakeModuleError) ErrorCode() string                    { return e.code }
func (e *fakeModuleError) ErrorModule() string                  { return e.module }
func (e *fakeModuleError) ErrorRecordIndex() int                { return e.recordIndex }
func (e *fakeModuleError) ErrorDetails() map[string]interface{} { return e.details }

func TestAsModuleError_Direct(t *testing.T) {
	t.Parallel()

	original := &fakeModuleError{
		code: "X", module: "fake", recordIndex: 7,
		details: map[string]interface{}{"k": "v"},
		msg:     "boom",
	}

	got, ok := errhandling.AsModuleError(original)
	if !ok {
		t.Fatalf("expected AsModuleError to succeed for direct ModuleError")
	}
	if got.ErrorCode() != "X" {
		t.Fatalf("ErrorCode: got %q, want %q", got.ErrorCode(), "X")
	}
	if got.ErrorModule() != "fake" {
		t.Fatalf("ErrorModule: got %q, want %q", got.ErrorModule(), "fake")
	}
	if got.ErrorRecordIndex() != 7 {
		t.Fatalf("ErrorRecordIndex: got %d, want 7", got.ErrorRecordIndex())
	}
	if got.ErrorDetails()["k"] != "v" {
		t.Fatalf("ErrorDetails: missing 'k' key")
	}
}

func TestAsModuleError_Wrapped(t *testing.T) {
	t.Parallel()

	inner := &fakeModuleError{code: "C", module: "m", msg: "inner"}
	wrapped := fmt.Errorf("context: %w", inner)

	got, ok := errhandling.AsModuleError(wrapped)
	if !ok {
		t.Fatalf("expected AsModuleError to unwrap to ModuleError")
	}
	if got.ErrorCode() != "C" {
		t.Fatalf("ErrorCode: got %q, want %q", got.ErrorCode(), "C")
	}
}

func TestAsModuleError_NotImplemented(t *testing.T) {
	t.Parallel()

	if _, ok := errhandling.AsModuleError(errors.New("plain")); ok {
		t.Fatalf("expected AsModuleError to return false for plain error")
	}
	if _, ok := errhandling.AsModuleError(nil); ok {
		t.Fatalf("expected AsModuleError to return false for nil")
	}
}
