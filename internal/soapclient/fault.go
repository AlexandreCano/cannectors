package soapclient

import (
	"encoding/xml"
	"fmt"
	"strings"
)

// SOAPFaultError is returned when a SOAP response contains a Fault element.
type SOAPFaultError struct {
	Version    SOAPVersion
	Code       string
	Reason     string
	Actor      string
	Detail     string
	StatusCode int
}

func (e *SOAPFaultError) Error() string {
	if e.Code == "" {
		return "SOAP fault: " + e.Reason
	}
	return fmt.Sprintf("SOAP fault %s: %s", e.Code, e.Reason)
}

type faultEnvelope struct {
	Body struct {
		Fault *faultPayload `xml:"Fault"`
	} `xml:"Body"`
}

type faultPayload struct {
	XMLName xml.Name

	Code11   string `xml:"faultcode"`
	Reason11 string `xml:"faultstring"`
	Actor11  string `xml:"faultactor"`
	Detail11 rawXML `xml:"detail"`

	Code12 struct {
		Value string `xml:"Value"`
	} `xml:"Code"`
	Reason12 struct {
		Text string `xml:"Text"`
	} `xml:"Reason"`
	Detail12 rawXML `xml:"Detail"`
}

type rawXML struct {
	Inner string `xml:",innerxml"`
}

// ParseSOAPFault extracts SOAP 1.1 or SOAP 1.2 fault details from an envelope.
func ParseSOAPFault(body []byte) (*SOAPFaultError, error) {
	var envelope faultEnvelope
	if err := xml.Unmarshal(body, &envelope); err != nil {
		return nil, err
	}
	if envelope.Body.Fault == nil {
		return nil, nil
	}
	fault := envelope.Body.Fault
	if fault.Code12.Value != "" || fault.Reason12.Text != "" || fault.XMLName.Space == NamespaceSOAP12 {
		return &SOAPFaultError{
			Version: SOAPVersion12,
			Code:    strings.TrimSpace(fault.Code12.Value),
			Reason:  strings.TrimSpace(fault.Reason12.Text),
			Detail:  strings.TrimSpace(fault.Detail12.Inner),
		}, nil
	}
	return &SOAPFaultError{
		Version: SOAPVersion11,
		Code:    strings.TrimSpace(fault.Code11),
		Reason:  strings.TrimSpace(fault.Reason11),
		Actor:   strings.TrimSpace(fault.Actor11),
		Detail:  strings.TrimSpace(fault.Detail11.Inner),
	}, nil
}
