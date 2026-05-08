package filter

import (
	"context"
	"reflect"
	"strings"
	"testing"
)

// Duplicate setup patterns identified in mapping_test.go:
//  1. Inline []map[string]any records with repeated nested map literals.
//  2. Inline FieldMapping values with repeated source pointer creation.
//  3. Repeated Process + records comparison boilerplate in table tests.

func strPtr(s string) *string {
	return &s
}

type recordBuilder struct {
	record map[string]any
}

func rb() *recordBuilder {
	return &recordBuilder{record: make(map[string]any)}
}

func (b *recordBuilder) Set(key string, value any) *recordBuilder {
	b.record[key] = value
	return b
}

func (b *recordBuilder) Build() map[string]any {
	out := make(map[string]any, len(b.record))
	for k, v := range b.record {
		out[k] = v
	}
	return out
}

type mappingBuilder struct {
	mapping FieldMapping
}

func mb() *mappingBuilder {
	return &mappingBuilder{}
}

func (b *mappingBuilder) Source(source string) *mappingBuilder {
	b.mapping.Source = strPtr(source)
	return b
}

func (b *mappingBuilder) Target(target string) *mappingBuilder {
	b.mapping.Target = target
	return b
}

func (b *mappingBuilder) DefaultValue(value any) *mappingBuilder {
	b.mapping.DefaultValue = value
	return b
}

func (b *mappingBuilder) OnMissing(onMissing string) *mappingBuilder {
	b.mapping.OnMissing = onMissing
	return b
}

func (b *mappingBuilder) Transforms(transforms ...TransformOp) *mappingBuilder {
	b.mapping.Transforms = append([]TransformOp(nil), transforms...)
	return b
}

func (b *mappingBuilder) Build() FieldMapping {
	return b.mapping
}

type transformBuilder struct {
	op TransformOp
}

func tb(op string) *transformBuilder {
	return &transformBuilder{op: TransformOp{Op: op}}
}

func (b *transformBuilder) Format(format string) *transformBuilder {
	b.op.Format = format
	return b
}

func (b *transformBuilder) Pattern(pattern string) *transformBuilder {
	b.op.Pattern = pattern
	return b
}

func (b *transformBuilder) Replacement(replacement string) *transformBuilder {
	b.op.Replacement = replacement
	return b
}

func (b *transformBuilder) Separator(separator string) *transformBuilder {
	b.op.Separator = separator
	return b
}

func (b *transformBuilder) Build() TransformOp {
	return b.op
}

func oneRecord(values ...any) []map[string]any {
	if len(values)%2 != 0 {
		panic("oneRecord requires key/value pairs")
	}
	record := make(map[string]any, len(values)/2)
	for i := 0; i < len(values); i += 2 {
		key, ok := values[i].(string)
		if !ok {
			panic("oneRecord keys must be strings")
		}
		record[key] = values[i+1]
	}
	return []map[string]any{record}
}

func assertRecordsEqual(t *testing.T, got, want []map[string]any) {
	t.Helper()
	if !recordsEqual(got, want) {
		t.Fatalf("records mismatch\ngot:  %#v\nwant: %#v", got, want)
	}
}

func assertMapped(t *testing.T, input []map[string]any, mappings []FieldMapping, want []map[string]any) {
	t.Helper()
	assertMappedWithOnError(t, input, mappings, "fail", want)
}

func assertMappedWithOnError(t *testing.T, input []map[string]any, mappings []FieldMapping, onError string, want []map[string]any) {
	t.Helper()
	mapper, err := NewMappingFromConfig(mappings, onError)
	if err != nil {
		t.Fatalf("NewMappingFromConfig() error = %v", err)
	}
	got, err := mapper.Process(context.Background(), input)
	if err != nil {
		t.Fatalf("Process() error = %v", err)
	}
	assertRecordsEqual(t, got, want)
}

// assertMappingFails runs the mapping with onError="fail" and expects an error,
// matching the boilerplate used by tabular tests with `wantErr: true`.
func assertMappingFails(t *testing.T, input []map[string]any, mappings []FieldMapping) {
	t.Helper()
	mapper, err := NewMappingFromConfig(mappings, "fail")
	if err != nil {
		// Construction-time failure also satisfies the contract.
		return
	}
	if _, err := mapper.Process(context.Background(), input); err == nil {
		t.Fatalf("Process() error = nil, want error")
	}
}

func containsString(s, substr string) bool {
	return strings.Contains(s, substr)
}

func recordsEqual(a, b []map[string]any) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if !mapsEqual(a[i], b[i]) {
			return false
		}
	}
	return true
}

func mapsEqual(a, b map[string]any) bool {
	if len(a) != len(b) {
		return false
	}
	for k, v := range a {
		bv, ok := b[k]
		if !ok {
			return false
		}
		if !valuesEqual(v, bv) {
			return false
		}
	}
	return true
}

func valuesEqual(a, b any) bool {
	if a == nil || b == nil {
		return a == nil && b == nil
	}

	aMap, aIsMap := a.(map[string]any)
	bMap, bIsMap := b.(map[string]any)
	if aIsMap && bIsMap {
		return mapsEqual(aMap, bMap)
	}

	aSlice, aIsSlice := a.([]any)
	bSlice, bIsSlice := b.([]any)
	if aIsSlice && bIsSlice {
		if len(aSlice) != len(bSlice) {
			return false
		}
		for i := range aSlice {
			if !valuesEqual(aSlice[i], bSlice[i]) {
				return false
			}
		}
		return true
	}

	return reflect.DeepEqual(a, b)
}

func TestMappingFixtureBuilders(t *testing.T) {
	input := []map[string]any{
		rb().
			Set("name", " Alice ").
			Set("tags", "red|blue").
			Build(),
	}
	mappings := []FieldMapping{
		mb().
			Source("name").
			Target("fullName").
			DefaultValue("unknown").
			OnMissing(OnMissingUseDefault).
			Transforms(tb("trim").Format("YYYY").Pattern(`\s+`).Replacement(" ").Separator("|").Build()).
			Build(),
		mb().
			Source("tags").
			Target("labels").
			Transforms(tb("split").Separator("|").Build()).
			Build(),
	}

	assertMapped(t, input, mappings, []map[string]any{{
		"name":     " Alice ",
		"tags":     "red|blue",
		"fullName": "Alice",
		"labels":   []any{"red", "blue"},
	}})
	assertRecordsEqual(t, oneRecord("id", 1), []map[string]any{{"id": 1}})
}
