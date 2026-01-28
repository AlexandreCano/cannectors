package pathutil

import (
	"testing"
)

func TestValidateFilePath(t *testing.T) {
	tests := []struct {
		name    string
		path    string
		wantErr bool
	}{
		{"empty", "", true},
		{"null byte", "a\x00b", true},
		{"simple segment", "..", true},
		{"leading segment", "../foo", true},
		{"middle segment", "foo/../bar", true},
		{"valid relative", "queries/select.sql", false},
		{"valid nested", "dir/queries/select.sql", false},
		{"single segment", "file.sql", false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := ValidateFilePath(tt.path)
			if (err != nil) != tt.wantErr {
				t.Errorf("ValidateFilePath(%q) err = %v, wantErr %v", tt.path, err, tt.wantErr)
			}
		})
	}
}
