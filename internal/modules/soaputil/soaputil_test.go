package soaputil

import "testing"

func TestExtractIntField(t *testing.T) {
	data := map[string]any{
		"meta": map[string]any{
			"totalPages": float64(7),
			"totalInt":   int(42),
			"totalI64":   int64(99),
			"totalStr":   "12",
			"bad":        "not-a-number",
		},
	}
	cases := []struct {
		name  string
		field string
		want  int
	}{
		{"empty field returns zero", "", 0},
		{"missing field returns zero", "meta.unknown", 0},
		{"float64 path", "meta.totalPages", 7},
		{"int path", "meta.totalInt", 42},
		{"int64 path", "meta.totalI64", 99},
		{"numeric string path", "meta.totalStr", 12},
		{"unconvertible string returns zero", "meta.bad", 0},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := ExtractIntField(data, tc.field); got != tc.want {
				t.Fatalf("got %d, want %d", got, tc.want)
			}
		})
	}
}

func TestExtractStringField(t *testing.T) {
	data := map[string]any{
		"meta": map[string]any{
			"next":   "cursor-abc",
			"empty":  "",
			"nullV":  nil,
			"number": float64(123),
		},
	}
	cases := []struct {
		name  string
		field string
		want  string
	}{
		{"empty field returns empty", "", ""},
		{"missing field returns empty", "meta.unknown", ""},
		{"nil value returns empty", "meta.nullV", ""},
		{"string path", "meta.next", "cursor-abc"},
		{"empty string path", "meta.empty", ""},
		{"non-string value is stringified", "meta.number", "123"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := ExtractStringField(data, tc.field); got != tc.want {
				t.Fatalf("got %q, want %q", got, tc.want)
			}
		})
	}
}
