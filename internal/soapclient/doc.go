// Package soapclient provides raw XML SOAP request support for Cannectors modules.
//
// The package supports SOAP 1.1 and SOAP 1.2 HTTP bindings, WS-Security
// UsernameToken authentication with PasswordText or PasswordDigest, SOAP Fault
// parsing, and XML response conversion to map data through mxj.
//
// Request bodies are raw XML fragments supplied by pipeline configuration.
// Record substitutions in those fragments are XML-escaped before the request is
// sent. There is no WSDL parsing or code generation at runtime.
//
// MTOM is supported for raw XML by using explicit XOP references: the SOAP body
// must contain xop:Include href="cid:..." elements, and MTOMConfig supplies the
// MIME parts for the matching content IDs. Automatic Binary field rewriting is
// only available in hooklift's struct-based API and is intentionally not exposed
// by this raw XML layer.
//
// NewClient is the main entry point. It wraps the shared Cannectors HTTP client
// while delegating SOAP envelope and transport details to the vendored hooklift
// SOAP package.
package soapclient
