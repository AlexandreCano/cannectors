package soapclient

import "testing"

func TestParseXMLResponse_UsesMXJ(t *testing.T) {
	body := []byte(`<soap:Envelope xmlns:soap="http://schemas.xmlsoap.org/soap/envelope/"><soap:Body><GetResponse><Result>ok</Result></GetResponse></soap:Body></soap:Envelope>`)

	data, err := ParseXMLResponse(body)
	if err != nil {
		t.Fatalf("ParseXMLResponse returned error: %v", err)
	}
	if len(data) == 0 {
		t.Fatal("expected parsed map")
	}
}
