package soapclient

import (
	"fmt"
	"mime"
)

// SOAPVersion identifies the SOAP envelope and HTTP binding version.
type SOAPVersion string

const (
	// SOAPVersion11 selects SOAP 1.1.
	SOAPVersion11 SOAPVersion = "1.1"
	// SOAPVersion12 selects SOAP 1.2.
	SOAPVersion12 SOAPVersion = "1.2"
)

const (
	// NamespaceSOAP11 is the SOAP 1.1 envelope namespace.
	NamespaceSOAP11 = "http://schemas.xmlsoap.org/soap/envelope/"
	// NamespaceSOAP12 is the SOAP 1.2 envelope namespace.
	NamespaceSOAP12 = "http://www.w3.org/2003/05/soap-envelope"
)

// NormalizeSOAPVersion applies the default SOAP version and validates explicit values.
func NormalizeSOAPVersion(version SOAPVersion) (SOAPVersion, error) {
	if version == "" {
		return SOAPVersion11, nil
	}
	switch version {
	case SOAPVersion11, SOAPVersion12:
		return version, nil
	default:
		return "", fmt.Errorf("unsupported SOAP version %q", version)
	}
}

// EnvelopeNamespace returns the envelope namespace for a SOAP version.
func EnvelopeNamespace(version SOAPVersion) (string, error) {
	version, err := NormalizeSOAPVersion(version)
	if err != nil {
		return "", err
	}
	if version == SOAPVersion12 {
		return NamespaceSOAP12, nil
	}
	return NamespaceSOAP11, nil
}

// RequestContentType returns the HTTP Content-Type value for a SOAP request.
func RequestContentType(version SOAPVersion, action string) (string, error) {
	version, err := NormalizeSOAPVersion(version)
	if err != nil {
		return "", err
	}
	if version == SOAPVersion12 {
		params := map[string]string{"charset": "utf-8"}
		if action != "" {
			params["action"] = action
		}
		return mime.FormatMediaType("application/soap+xml", params), nil
	}
	return mime.FormatMediaType("text/xml", map[string]string{"charset": "utf-8"}), nil
}
