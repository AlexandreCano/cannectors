package soapclient

import (
	"bytes"
	"io"
	"mime"
	"mime/multipart"
	"strings"
	"testing"
)

func TestBuildMTOMRequest_EmitsMultipartRelated(t *testing.T) {
	payload, contentType, err := BuildMTOMRequest([]byte(`<Envelope/>`), SOAPVersion12, []MTOMAttachment{
		{ContentID: "doc-1", ContentType: "application/pdf", Data: []byte("pdf-bytes")},
	})
	if err != nil {
		t.Fatalf("BuildMTOMRequest returned error: %v", err)
	}
	mediaType, params, err := mime.ParseMediaType(contentType)
	if err != nil {
		t.Fatalf("invalid content type: %v", err)
	}
	if mediaType != "multipart/related" || params["type"] != "application/xop+xml" || params["start-info"] != "application/soap+xml" {
		t.Fatalf("unexpected content type: %s params=%v", mediaType, params)
	}

	reader := multipart.NewReader(bytes.NewReader(payload), params["boundary"])
	root, err := reader.NextPart()
	if err != nil {
		t.Fatalf("missing root part: %v", err)
	}
	rootBody, _ := io.ReadAll(root)
	if string(rootBody) != `<Envelope/>` {
		t.Fatalf("unexpected root body: %s", rootBody)
	}
	attachment, err := reader.NextPart()
	if err != nil {
		t.Fatalf("missing attachment part: %v", err)
	}
	if got := attachment.Header.Get("Content-ID"); got != "<doc-1>" {
		t.Fatalf("unexpected attachment content id: %q", got)
	}
}

func TestParseMTOMResponse_ReturnsEnvelopeAndAttachments(t *testing.T) {
	var b bytes.Buffer
	writer := multipart.NewWriter(&b)
	root, _ := writer.CreatePart(map[string][]string{
		"Content-Type": {"application/xop+xml; charset=utf-8"},
		"Content-ID":   {"<root>"},
	})
	_, _ = root.Write([]byte(`<Envelope><Body><xop:Include xmlns:xop="http://www.w3.org/2004/08/xop/include" href="cid:doc-1"/></Body></Envelope>`))
	part, _ := writer.CreatePart(map[string][]string{
		"Content-Type": {"application/pdf"},
		"Content-ID":   {"<doc-1>"},
	})
	_, _ = part.Write([]byte("pdf-bytes"))
	_ = writer.Close()

	contentType := `multipart/related; type="application/xop+xml"; boundary="` + writer.Boundary() + `"`
	envelope, attachments, multipartResponse, err := ParseMTOMResponse(contentType, strings.NewReader(b.String()))
	if err != nil {
		t.Fatalf("ParseMTOMResponse returned error: %v", err)
	}
	if !multipartResponse {
		t.Fatal("expected multipart response")
	}
	if !bytes.Contains(envelope, []byte(`xop:Include`)) {
		t.Fatalf("unexpected envelope: %s", envelope)
	}
	if string(attachments["doc-1"].Data) != "pdf-bytes" {
		t.Fatalf("unexpected attachment: %#v", attachments["doc-1"])
	}
}

func TestParseMTOMResponse_UsesStartParameterForRootPart(t *testing.T) {
	var b bytes.Buffer
	writer := multipart.NewWriter(&b)
	attachment, _ := writer.CreatePart(map[string][]string{
		"Content-Type": {"text/xml"},
		"Content-ID":   {"<business-xml>"},
	})
	_, _ = attachment.Write([]byte(`<NotEnvelope/>`))
	root, _ := writer.CreatePart(map[string][]string{
		"Content-Type": {"application/xop+xml; charset=utf-8"},
		"Content-ID":   {"<root-envelope>"},
	})
	_, _ = root.Write([]byte(`<Envelope><Body>ok</Body></Envelope>`))
	_ = writer.Close()

	contentType := `multipart/related; type="application/xop+xml"; start="<root-envelope>"; boundary="` + writer.Boundary() + `"`
	envelope, attachments, _, err := ParseMTOMResponse(contentType, strings.NewReader(b.String()))
	if err != nil {
		t.Fatalf("ParseMTOMResponse returned error: %v", err)
	}
	if string(envelope) != `<Envelope><Body>ok</Body></Envelope>` {
		t.Fatalf("unexpected envelope: %s", envelope)
	}
	if string(attachments["business-xml"].Data) != `<NotEnvelope/>` {
		t.Fatalf("unexpected attachment map: %#v", attachments)
	}
}
