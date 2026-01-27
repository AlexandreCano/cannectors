package httpconfig

import (
	"testing"
)

func TestValidateBaseConfig(t *testing.T) {
	tests := []struct {
		name            string
		config          BaseConfig
		requireEndpoint bool
		wantErr         bool
	}{
		{
			name: "valid config",
			config: BaseConfig{
				Endpoint: "https://api.example.com",
				Headers: map[string]string{
					"X-Custom": "value",
				},
			},
			requireEndpoint: true,
			wantErr:         false,
		},
		{
			name:            "missing required endpoint",
			config:          BaseConfig{},
			requireEndpoint: true,
			wantErr:         true,
		},
		{
			name:            "endpoint not required",
			config:          BaseConfig{},
			requireEndpoint: false,
			wantErr:         false,
		},
		{
			name: "valid template in endpoint",
			config: BaseConfig{
				Endpoint: "https://api.example.com/users/{{record.id}}",
			},
			requireEndpoint: true,
			wantErr:         false,
		},
		{
			name: "invalid template in endpoint",
			config: BaseConfig{
				Endpoint: "https://api.example.com/users/{{record.id",
			},
			requireEndpoint: true,
			wantErr:         true,
		},
		{
			name: "valid template in header",
			config: BaseConfig{
				Endpoint: "https://api.example.com",
				Headers: map[string]string{
					"X-User-ID": "{{record.userId}}",
				},
			},
			requireEndpoint: true,
			wantErr:         false,
		},
		{
			name: "invalid template in header",
			config: BaseConfig{
				Endpoint: "https://api.example.com",
				Headers: map[string]string{
					"X-User-ID": "{{record.userId",
				},
			},
			requireEndpoint: true,
			wantErr:         true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := ValidateBaseConfig(tt.config, tt.requireEndpoint)
			if (err != nil) != tt.wantErr {
				t.Errorf("ValidateBaseConfig() error = %v, wantErr %v", err, tt.wantErr)
			}
		})
	}
}

func TestValidateMethod(t *testing.T) {
	tests := []struct {
		name           string
		method         string
		allowedMethods []string
		wantErr        bool
	}{
		{
			name:           "valid method",
			method:         "POST",
			allowedMethods: []string{"GET", "POST", "PUT"},
			wantErr:        false,
		},
		{
			name:           "invalid method",
			method:         "DELETE",
			allowedMethods: []string{"GET", "POST", "PUT"},
			wantErr:        true,
		},
		{
			name:           "empty method",
			method:         "",
			allowedMethods: []string{"GET", "POST", "PUT"},
			wantErr:        false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := ValidateMethod(tt.method, tt.allowedMethods)
			if (err != nil) != tt.wantErr {
				t.Errorf("ValidateMethod() error = %v, wantErr %v", err, tt.wantErr)
			}
		})
	}
}

func TestValidateOnError(t *testing.T) {
	tests := []struct {
		name    string
		onError string
		wantErr bool
	}{
		{name: "valid fail", onError: "fail", wantErr: false},
		{name: "valid skip", onError: "skip", wantErr: false},
		{name: "valid log", onError: "log", wantErr: false},
		{name: "empty", onError: "", wantErr: false},
		{name: "invalid", onError: "ignore", wantErr: true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := ValidateOnError(tt.onError)
			if (err != nil) != tt.wantErr {
				t.Errorf("ValidateOnError() error = %v, wantErr %v", err, tt.wantErr)
			}
		})
	}
}

func TestValidateTemplateFile(t *testing.T) {
	tests := []struct {
		name    string
		path    string
		content string
		wantErr bool
	}{
		{
			name:    "empty path",
			path:    "",
			content: "",
			wantErr: false,
		},
		{
			name:    "valid template",
			path:    "template.json",
			content: `{"id": "{{record.id}}"}`,
			wantErr: false,
		},
		{
			name:    "invalid template",
			path:    "template.json",
			content: `{"id": "{{record.id"}`,
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := ValidateTemplateFile(tt.path, tt.content)
			if (err != nil) != tt.wantErr {
				t.Errorf("ValidateTemplateFile() error = %v, wantErr %v", err, tt.wantErr)
			}
		})
	}
}

func TestValidationError_Error(t *testing.T) {
	err := &ValidationError{
		Field:   "endpoint",
		Message: "endpoint is required",
	}
	expected := "validation error for endpoint: endpoint is required"
	if err.Error() != expected {
		t.Errorf("Error() = %v, want %v", err.Error(), expected)
	}
}
