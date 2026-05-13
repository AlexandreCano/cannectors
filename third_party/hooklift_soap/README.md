# hooklift_soap vendor copy

Provenance: `github.com/hooklift/gowsdl/soap` at tag `v0.5.0`.

This directory vendors only the `soap` sub-package used by the SOAP support
plan. The runtime keeps it under `third_party/` so SOAP 1.2 behavior can be
patched locally without depending on WSDL code generation.

Local patch summary:

- add `SOAPVersion`, `SOAP11`, `SOAP12`, and `WithSOAPVersion`
- switch envelope namespace between SOAP 1.1 and SOAP 1.2
- emit SOAP 1.2 requests with `application/soap+xml; action="..."`
- omit the standalone `SOAPAction` header for SOAP 1.2
- allow namespace-agnostic response envelope decoding
- apply caller HTTP headers before protected SOAP headers so `Content-Type` and
  `SOAPAction` stay version-correct
- allow explicit raw XML MTOM usage by adding caller-provided MIME parts to the
  MTOM encoder; the raw XML body is expected to contain matching `xop:Include`
  elements
