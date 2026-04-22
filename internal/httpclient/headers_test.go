package httpclient

import (
	"testing"
)

func TestValidateHeaderName(t *testing.T) {
	tests := []struct {
		name    string
		input   string
		wantErr bool
	}{
		{"empty", "", true},
		{"alphanumeric", "X-Custom-Header42", false},
		{"valid symbols", "X-A_b.c!#$%&'*+-.^_`|~", false},
		{"contains colon", "X:Y", true},
		{"contains space", "X Y", true},
		{"contains CR", "X\rY", true},
		{"contains LF", "X\nY", true},
		{"contains control", "X\x01Y", true},
		{"contains DEL", "X\x7fY", true},
		{"contains UTF-8", "Xé", true},
		{"contains slash", "X/Y", true},
		{"contains paren", "X(Y", true},
		{"contains comma", "X,Y", true},
		{"contains at", "X@Y", true},
		{"contains equals", "X=Y", true},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			err := ValidateHeaderName(tc.input)
			if tc.wantErr && err == nil {
				t.Errorf("expected error for %q, got nil", tc.input)
			}
			if !tc.wantErr && err != nil {
				t.Errorf("unexpected error for %q: %v", tc.input, err)
			}
		})
	}
}

func TestValidateHeaderValue(t *testing.T) {
	tests := []struct {
		name    string
		input   string
		wantErr bool
	}{
		{"empty", "", false},
		{"ascii printable", "application/json; charset=utf-8", false},
		{"with HTAB", "foo\tbar", false},
		{"with CR", "foo\rbar", true},
		{"with LF", "foo\nbar", true},
		{"with CRLF injection", "foo\r\nX-Injected: yes", true},
		{"with NUL", "foo\x00bar", true},
		{"with control 0x1F", "foo\x1fbar", true},
		{"with DEL 0x7F", "foo\x7fbar", true},
		{"with SP only", "foo bar", false},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			err := ValidateHeaderValue(tc.input)
			if tc.wantErr && err == nil {
				t.Errorf("expected error for %q, got nil", tc.input)
			}
			if !tc.wantErr && err != nil {
				t.Errorf("unexpected error for %q: %v", tc.input, err)
			}
		})
	}
}

func TestTryAddValidHeader(t *testing.T) {
	tests := []struct {
		name  string
		key   string
		value string
		added bool
	}{
		{"valid", "X-Foo", "bar", true},
		{"invalid name", "X Foo", "bar", false},
		{"invalid value", "X-Foo", "bar\r\nX-Injected: yes", false},
		{"empty name", "", "bar", false},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			headers := map[string]string{}
			TryAddValidHeader(headers, tc.key, tc.value)
			_, present := headers[tc.key]
			if tc.added && !present {
				t.Errorf("expected header %q to be added", tc.key)
			}
			if !tc.added && present {
				t.Errorf("header %q was added despite invalid input", tc.key)
			}
		})
	}
}
