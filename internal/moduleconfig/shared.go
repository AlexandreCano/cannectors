package moduleconfig

import (
	"github.com/cannectors/runtime/pkg/connector"
)

// HTTPRequestBase mirrors common-schema.json#/$defs/httpRequestBase.
// Embedded by all HTTP module configs (httpPolling, httpRequest, http_call).
type HTTPRequestBase struct {
	Endpoint       string                `json:"endpoint"`
	Method         string                `json:"method,omitempty"`
	Headers        map[string]string     `json:"headers,omitempty"`
	QueryParams    map[string]string     `json:"queryParams,omitempty"`
	Authentication *connector.AuthConfig `json:"authentication,omitempty"`
	TimeoutMs      int                   `json:"timeoutMs,omitempty"`
}

// SQLRequestBase mirrors common-schema.json#/$defs/sqlRequestBase.
// Embedded by all SQL module configs (database input, database output, sql_call).
type SQLRequestBase struct {
	ConnectionString       string `json:"connectionString,omitempty"`
	ConnectionStringRef    string `json:"connectionStringRef,omitempty"`
	Driver                 string `json:"driver,omitempty"`
	MaxOpenConns           int    `json:"maxOpenConns,omitempty"`
	MaxIdleConns           int    `json:"maxIdleConns,omitempty"`
	ConnMaxLifetimeSeconds int    `json:"connMaxLifetimeSeconds,omitempty"`
	ConnMaxIdleTimeSeconds int    `json:"connMaxIdleTimeSeconds,omitempty"`
	TimeoutMs              int    `json:"timeoutMs,omitempty"`
	Query                  string `json:"query,omitempty"`
	QueryFile              string `json:"queryFile,omitempty"`
}

// PaginationConfig mirrors common-schema.json#/$defs/pagination.
type PaginationConfig struct {
	Type            string `json:"type,omitempty"`
	PageParam       string `json:"pageParam,omitempty"`
	TotalPagesField string `json:"totalPagesField,omitempty"`
	OffsetParam     string `json:"offsetParam,omitempty"`
	LimitParam      string `json:"limitParam,omitempty"`
	Limit           int    `json:"limit,omitempty"`
	TotalField      string `json:"totalField,omitempty"`
	CursorParam     string `json:"cursorParam,omitempty"`
	NextCursorField string `json:"nextCursorField,omitempty"`
}

// DatabasePaginationConfig mirrors common-schema.json#/$defs/databasePaginationConfig.
type DatabasePaginationConfig struct {
	Type        string `json:"type"`
	Limit       int    `json:"limit"`
	OffsetParam string `json:"offsetParam"`
	CursorField string `json:"cursorField"`
	CursorParam string `json:"cursorParam"`
}

// KeyConfig mirrors common-schema.json#/$defs/httpCallKeyConfig.
type KeyConfig struct {
	Field     string `json:"field"`
	ParamType string `json:"paramType"`
	ParamName string `json:"paramName"`
}

// CacheConfig mirrors filter-schema.json#/$defs/cacheConfig.
type CacheConfig struct {
	Enabled    bool   `json:"enabled"`
	MaxSize    int    `json:"maxSize"`
	DefaultTTL int    `json:"defaultTTL"`
	Key        string `json:"key"`
}
