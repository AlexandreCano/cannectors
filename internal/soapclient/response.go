package soapclient

import (
	"fmt"
	"net/http"

	"github.com/clbanning/mxj/v2"

	"github.com/cannectors/runtime/pkg/connector"
)

// SOAPResponse contains the parsed SOAP response plus raw envelope metadata.
type SOAPResponse struct {
	StatusCode  int
	Headers     http.Header
	EnvelopeXML []byte
	Data        map[string]any
	Attachments map[string]MTOMAttachment
	RetryInfo   *connector.RetryInfo
}

// ParseXMLResponse converts SOAP XML to map data using mxj.
func ParseXMLResponse(body []byte) (map[string]any, error) {
	mv, err := mxj.NewMapXml(body)
	if err != nil {
		return nil, fmt.Errorf("parsing SOAP XML response: %w", err)
	}
	return map[string]any(mv), nil
}
