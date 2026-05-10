package httpclient

import "testing"

func TestNormalizeAndValidateMethod(t *testing.T) {
	tests := []struct {
		in      string
		want    string
		wantErr bool
	}{
		{"", "GET", false},
		{"  ", "GET", false},
		{"get", "GET", false},
		{"POST", "POST", false},
		{"Patch", "PATCH", false},
		{"DELETE", "DELETE", false},
		{"HEAD", "HEAD", false},
		{"OPTIONS", "OPTIONS", false},
		{"PURGE", "PURGE", false},
		{"BAD METHOD", "", true},
		{"BAD\r\nMETHOD", "", true},
	}
	for _, tc := range tests {
		got, err := NormalizeAndValidateMethod(tc.in)
		if (err != nil) != tc.wantErr {
			t.Errorf("NormalizeAndValidateMethod(%q) err=%v wantErr=%v", tc.in, err, tc.wantErr)
		}
		if !tc.wantErr && got != tc.want {
			t.Errorf("NormalizeAndValidateMethod(%q) = %q, want %q", tc.in, got, tc.want)
		}
	}
}

func TestValidateAbsoluteURL(t *testing.T) {
	tests := []struct {
		in      string
		wantErr bool
	}{
		{"https://api.example.com/v1/things", false},
		{"http://localhost:8080/x", false},
		{"", true},
		{"  ", true},
		{"ftp://example.com/x", true},
		{"https://", true},
		{"not a url", true},
		{"/relative/path", true},
	}
	for _, tc := range tests {
		err := ValidateAbsoluteURL(tc.in)
		if (err != nil) != tc.wantErr {
			t.Errorf("ValidateAbsoluteURL(%q) err=%v wantErr=%v", tc.in, err, tc.wantErr)
		}
	}
}
