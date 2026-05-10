package moduleconfig

import (
	"fmt"
	"regexp"

	"github.com/cannectors/runtime/pkg/connector"
)

// HTTPRequestBase mirrors common-schema.json#/$defs/httpRequestBase.
// Embedded by all HTTP module configs (httpPolling, httpRequest, http_call).
type HTTPRequestBase struct {
	Endpoint         string                `json:"endpoint"`
	Method           string                `json:"method,omitempty"`
	Headers          map[string]string     `json:"headers,omitempty"`
	QueryParams      map[string]string     `json:"queryParams,omitempty"`
	Body             string                `json:"body,omitempty"`
	BodyTemplateFile string                `json:"bodyTemplateFile,omitempty"`
	Authentication   *connector.AuthConfig `json:"authentication,omitempty"`
	TimeoutMs        int                   `json:"timeoutMs,omitempty"`
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

// connectionStringRefPattern matches ${ENV_VAR_NAME} format used by
// connectionStringRef. It mirrors the pattern declared in common-schema.json.
var connectionStringRefPattern = regexp.MustCompile(`^\$\{[A-Z_][A-Z0-9_]*\}$`)

// Validate enforces the SQL contract defined in common-schema.json#/$defs/sqlRequestBase.
// It is meant to be called by module constructors so that runtime instantiation
// surfaces the same errors as the schema validator (defense in depth, since
// some callers in tests build configs directly without going through schema
// validation).
func (s SQLRequestBase) Validate() error {
	hasConn := s.ConnectionString != ""
	hasConnRef := s.ConnectionStringRef != ""
	switch {
	case hasConn && hasConnRef:
		return fmt.Errorf("connectionString and connectionStringRef are mutually exclusive; provide exactly one")
	case !hasConn && !hasConnRef:
		return fmt.Errorf("connectionString or connectionStringRef is required")
	}
	if hasConnRef && !connectionStringRefPattern.MatchString(s.ConnectionStringRef) {
		return fmt.Errorf("connectionStringRef %q does not match required format ${ENV_VAR_NAME}", s.ConnectionStringRef)
	}

	hasQuery := s.Query != ""
	hasQueryFile := s.QueryFile != ""
	switch {
	case hasQuery && hasQueryFile:
		return fmt.Errorf("query and queryFile are mutually exclusive; provide exactly one")
	case !hasQuery && !hasQueryFile:
		return fmt.Errorf("query or queryFile is required")
	}
	return nil
}

// PaginationConfig mirrors common-schema.json#/$defs/pagination.
// Param is the canonical query parameter name used to inject the page,
// offset, or cursor value into the URL (depends on Type).
type PaginationConfig struct {
	Type            string `json:"type,omitempty"`
	Param           string `json:"param,omitempty"`
	LimitParam      string `json:"limitParam,omitempty"`
	Limit           int    `json:"limit,omitempty"`
	TotalPagesField string `json:"totalPagesField,omitempty"`
	TotalField      string `json:"totalField,omitempty"`
	NextCursorField string `json:"nextCursorField,omitempty"`
}

// Validate enforces the runtime contract for HTTP pagination: pagination.type
// must be one of the supported strategies and the strategy-specific required
// fields must be present.
func (p *PaginationConfig) Validate() error {
	if p == nil {
		return nil
	}
	switch p.Type {
	case "":
		return fmt.Errorf("pagination.type is required (expected 'cursor', 'page' or 'offset')")
	case "cursor":
		if p.Param == "" {
			return fmt.Errorf("pagination.param is required when pagination.type is %q", p.Type)
		}
		if p.NextCursorField == "" {
			return fmt.Errorf("pagination.nextCursorField is required when pagination.type is %q", p.Type)
		}
		return nil
	case "page":
		if p.Param == "" {
			return fmt.Errorf("pagination.param is required when pagination.type is %q", p.Type)
		}
		if p.TotalPagesField == "" {
			return fmt.Errorf("pagination.totalPagesField is required when pagination.type is %q", p.Type)
		}
		return nil
	case "offset":
		if p.Param == "" {
			return fmt.Errorf("pagination.param is required when pagination.type is %q", p.Type)
		}
		if p.LimitParam == "" {
			return fmt.Errorf("pagination.limitParam is required when pagination.type is %q", p.Type)
		}
		if p.TotalField == "" {
			return fmt.Errorf("pagination.totalField is required when pagination.type is %q", p.Type)
		}
		return nil
	default:
		return fmt.Errorf("unknown pagination.type %q (expected 'cursor', 'page' or 'offset')", p.Type)
	}
}

// DatabasePaginationConfig mirrors common-schema.json#/$defs/databasePaginationConfig.
// Param is the canonical SQL placeholder name used to inject either the cursor
// or the offset value into the query (e.g. ':offset' or ':cursor').
type DatabasePaginationConfig struct {
	Type        string `json:"type"`
	Limit       int    `json:"limit,omitempty"`
	Param       string `json:"param,omitempty"`
	CursorField string `json:"cursorField,omitempty"`
}

// Validate enforces the runtime contract: pagination.type must be one of
// the supported strategies. Both strategies require param; cursor additionally
// requires cursorField (mirrors common-schema.json#/$defs/databasePaginationConfig).
func (p *DatabasePaginationConfig) Validate() error {
	if p == nil {
		return nil
	}
	switch p.Type {
	case "limit-offset", "cursor":
		if p.Param == "" {
			return fmt.Errorf("pagination.param is required when pagination.type is %q", p.Type)
		}
		if p.Type == "cursor" && p.CursorField == "" {
			return fmt.Errorf("pagination.cursorField is required when pagination.type is %q", p.Type)
		}
		return nil
	case "":
		return fmt.Errorf("pagination.type is required (expected 'limit-offset' or 'cursor')")
	default:
		return fmt.Errorf("unknown pagination.type %q (expected 'limit-offset' or 'cursor')", p.Type)
	}
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
	TTLSeconds int    `json:"ttlSeconds"`
	Key        string `json:"key"`
}
