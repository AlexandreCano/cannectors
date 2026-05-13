package soapclient

import "testing"

func TestParseSOAPFault_SOAP11(t *testing.T) {
	body := []byte(`<soap:Envelope xmlns:soap="http://schemas.xmlsoap.org/soap/envelope/"><soap:Body><soap:Fault><faultcode>soap:Client</faultcode><faultstring>Bad request</faultstring><detail><code>E1</code></detail></soap:Fault></soap:Body></soap:Envelope>`)

	fault, err := ParseSOAPFault(body)
	if err != nil {
		t.Fatalf("ParseSOAPFault returned error: %v", err)
	}
	if fault == nil {
		t.Fatal("expected fault")
	}
	if fault.Version != SOAPVersion11 || fault.Code != "soap:Client" || fault.Reason != "Bad request" || fault.Detail != "<code>E1</code>" {
		t.Fatalf("unexpected fault: %#v", fault)
	}
}

func TestParseSOAPFault_SOAP12(t *testing.T) {
	body := []byte(`<env:Envelope xmlns:env="http://www.w3.org/2003/05/soap-envelope"><env:Body><env:Fault><env:Code><env:Value>env:Sender</env:Value></env:Code><env:Reason><env:Text xml:lang="en">Invalid</env:Text></env:Reason><env:Detail><code>E2</code></env:Detail></env:Fault></env:Body></env:Envelope>`)

	fault, err := ParseSOAPFault(body)
	if err != nil {
		t.Fatalf("ParseSOAPFault returned error: %v", err)
	}
	if fault == nil {
		t.Fatal("expected fault")
	}
	if fault.Version != SOAPVersion12 || fault.Code != "env:Sender" || fault.Reason != "Invalid" || fault.Detail != "<code>E2</code>" {
		t.Fatalf("unexpected fault: %#v", fault)
	}
}
