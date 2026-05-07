package errhandling

import "errors"

// ModuleError is the optional interface implemented by typed module errors so
// the runtime can extract a uniform set of fields (code, module, record
// index, details) for ExecutionError without needing to know the concrete
// type. Modules should embed their existing fields freely; the methods below
// are accessors over them.
//
// The methods use an "Error" prefix to avoid colliding with struct fields of
// the same name — Go disallows a struct field and a method to share an
// identifier, and most existing module error types already expose Code,
// RecordIndex, Details as public fields.
type ModuleError interface {
	error
	ErrorCode() string
	ErrorModule() string
	ErrorRecordIndex() int
	ErrorDetails() map[string]interface{}
}

// AsModuleError reports whether err (or any error in its Unwrap chain)
// implements ModuleError, and returns the matching value.
func AsModuleError(err error) (ModuleError, bool) {
	if err == nil {
		return nil, false
	}
	var me ModuleError
	if !errors.As(err, &me) {
		return nil, false
	}
	return me, true
}
