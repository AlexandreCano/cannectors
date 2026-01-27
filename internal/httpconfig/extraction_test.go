package httpconfig

import (
	"testing"
	"time"

	"github.com/canectors/runtime/pkg/connector"
)

func TestBaseConfig_GetTimeout(t *testing.T) {
	tests := []struct {
		name      string
		timeoutMs int
		want      time.Duration
	}{
		{
			name:      "custom timeout",
			timeoutMs: 5000,
			want:      5 * time.Second,
		},
		{
			name:      "zero uses default",
			timeoutMs: 0,
			want:      DefaultTimeout,
		},
		{
			name:      "negative uses default",
			timeoutMs: -1,
			want:      DefaultTimeout,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			c := &BaseConfig{TimeoutMs: tt.timeoutMs}
			if got := c.GetTimeout(); got != tt.want {
				t.Errorf("GetTimeout() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestExtractBaseConfig(t *testing.T) {
	tests := []struct {
		name   string
		config *connector.ModuleConfig
		want   BaseConfig
	}{
		{
			name:   "nil config",
			config: nil,
			want:   BaseConfig{},
		},
		{
			name: "full config",
			config: &connector.ModuleConfig{
				Config: map[string]interface{}{
					"endpoint":  "https://api.example.com",
					"method":    "POST",
					"timeoutMs": float64(5000),
					"headers": map[string]interface{}{
						"X-Custom": "value",
					},
				},
				Authentication: &connector.AuthConfig{
					Type: "bearer",
				},
			},
			want: BaseConfig{
				Endpoint:  "https://api.example.com",
				Method:    "POST",
				TimeoutMs: 5000,
				Headers: map[string]string{
					"X-Custom": "value",
				},
				Auth: &connector.AuthConfig{
					Type: "bearer",
				},
			},
		},
		{
			name: "legacy timeout in seconds",
			config: &connector.ModuleConfig{
				Config: map[string]interface{}{
					"endpoint": "https://api.example.com",
					"timeout":  float64(10), // 10 seconds
				},
			},
			want: BaseConfig{
				Endpoint:  "https://api.example.com",
				TimeoutMs: 10000,
				Headers:   map[string]string{},
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := ExtractBaseConfig(tt.config)
			if got.Endpoint != tt.want.Endpoint {
				t.Errorf("Endpoint = %v, want %v", got.Endpoint, tt.want.Endpoint)
			}
			if got.Method != tt.want.Method {
				t.Errorf("Method = %v, want %v", got.Method, tt.want.Method)
			}
			if got.TimeoutMs != tt.want.TimeoutMs {
				t.Errorf("TimeoutMs = %v, want %v", got.TimeoutMs, tt.want.TimeoutMs)
			}
		})
	}
}

func TestExtractDynamicParamsConfig(t *testing.T) {
	tests := []struct {
		name   string
		config map[string]interface{}
		want   DynamicParamsConfig
	}{
		{
			name:   "nil config",
			config: nil,
			want: DynamicParamsConfig{
				PathParams:        map[string]string{},
				QueryParams:       map[string]string{},
				QueryFromRecord:   map[string]string{},
				HeadersFromRecord: map[string]string{},
			},
		},
		{
			name: "root level params",
			config: map[string]interface{}{
				"pathParams": map[string]interface{}{
					"id": "record.id",
				},
				"queryParams": map[string]interface{}{
					"format": "json",
				},
			},
			want: DynamicParamsConfig{
				PathParams: map[string]string{
					"id": "record.id",
				},
				QueryParams: map[string]string{
					"format": "json",
				},
				QueryFromRecord:   map[string]string{},
				HeadersFromRecord: map[string]string{},
			},
		},
		{
			name: "request sub-object params",
			config: map[string]interface{}{
				"request": map[string]interface{}{
					"pathParams": map[string]interface{}{
						"id": "record.id",
					},
					"query": map[string]interface{}{
						"format": "json",
					},
				},
			},
			want: DynamicParamsConfig{
				PathParams: map[string]string{
					"id": "record.id",
				},
				QueryParams: map[string]string{
					"format": "json",
				},
				QueryFromRecord:   map[string]string{},
				HeadersFromRecord: map[string]string{},
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := ExtractDynamicParamsConfig(tt.config)
			if len(got.PathParams) != len(tt.want.PathParams) {
				t.Errorf("PathParams length = %v, want %v", len(got.PathParams), len(tt.want.PathParams))
			}
			if len(got.QueryParams) != len(tt.want.QueryParams) {
				t.Errorf("QueryParams length = %v, want %v", len(got.QueryParams), len(tt.want.QueryParams))
			}
		})
	}
}

func TestExtractErrorHandlingConfig(t *testing.T) {
	tests := []struct {
		name   string
		config map[string]interface{}
		want   ErrorHandlingConfig
	}{
		{
			name:   "nil config",
			config: nil,
			want:   ErrorHandlingConfig{},
		},
		{
			name: "with onError",
			config: map[string]interface{}{
				"onError": "skip",
			},
			want: ErrorHandlingConfig{OnError: "skip"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := ExtractErrorHandlingConfig(tt.config)
			if got.OnError != tt.want.OnError {
				t.Errorf("OnError = %v, want %v", got.OnError, tt.want.OnError)
			}
		})
	}
}

func TestGetTimeoutDuration(t *testing.T) {
	tests := []struct {
		name           string
		timeoutMs      int
		defaultTimeout time.Duration
		want           time.Duration
	}{
		{
			name:           "custom timeout",
			timeoutMs:      5000,
			defaultTimeout: 30 * time.Second,
			want:           5 * time.Second,
		},
		{
			name:           "zero uses default",
			timeoutMs:      0,
			defaultTimeout: 30 * time.Second,
			want:           30 * time.Second,
		},
		{
			name:           "negative uses default",
			timeoutMs:      -1,
			defaultTimeout: 30 * time.Second,
			want:           30 * time.Second,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := GetTimeoutDuration(tt.timeoutMs, tt.defaultTimeout); got != tt.want {
				t.Errorf("GetTimeoutDuration() = %v, want %v", got, tt.want)
			}
		})
	}
}
