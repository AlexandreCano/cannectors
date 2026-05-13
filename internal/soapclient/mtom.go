package soapclient

import (
	"bytes"
	"fmt"
	"io"
	"mime"
	"mime/multipart"
	"net/textproto"
	"strings"
)

// MTOMConfig enables outgoing MTOM attachments for a SOAP operation.
type MTOMConfig struct {
	Enabled     bool
	Attachments []MTOMAttachment
}

// MTOMAttachment contains one MIME part keyed by Content-ID.
type MTOMAttachment struct {
	ContentID   string
	ContentType string
	Data        []byte
}

// BuildMTOMRequest serializes a SOAP envelope and attachments as multipart/related.
// Client.Call delegates request emission to hooklift; this helper remains for
// low-level tests and callers that need a standalone multipart payload.
func BuildMTOMRequest(envelope []byte, version SOAPVersion, attachments []MTOMAttachment) ([]byte, string, error) {
	var b bytes.Buffer
	writer := multipart.NewWriter(&b)

	rootHeader := make(textproto.MIMEHeader)
	rootHeader.Set("Content-Type", `application/xop+xml; charset=utf-8; type="application/soap+xml"`)
	rootHeader.Set("Content-Transfer-Encoding", "8bit")
	rootHeader.Set("Content-ID", "<root.message@cannectors>")
	root, err := writer.CreatePart(rootHeader)
	if err != nil {
		return nil, "", err
	}
	if _, err := root.Write(envelope); err != nil {
		return nil, "", err
	}

	for _, attachment := range attachments {
		if attachment.ContentID == "" {
			return nil, "", fmt.Errorf("MTOM attachment content ID is required")
		}
		contentType := attachment.ContentType
		if contentType == "" {
			contentType = "application/octet-stream"
		}
		header := make(textproto.MIMEHeader)
		header.Set("Content-Type", contentType)
		header.Set("Content-Transfer-Encoding", "binary")
		header.Set("Content-ID", "<"+strings.Trim(attachment.ContentID, "<>")+">")
		part, err := writer.CreatePart(header)
		if err != nil {
			return nil, "", err
		}
		if _, err := part.Write(attachment.Data); err != nil {
			return nil, "", err
		}
	}

	if err := writer.Close(); err != nil {
		return nil, "", err
	}
	startInfo := "text/xml"
	if version == SOAPVersion12 {
		startInfo = "application/soap+xml"
	}
	contentType := mime.FormatMediaType("multipart/related", map[string]string{
		"type":       "application/xop+xml",
		"start":      "<root.message@cannectors>",
		"start-info": startInfo,
		"boundary":   writer.Boundary(),
	})
	return b.Bytes(), contentType, nil
}

// ParseMTOMResponse separates a multipart SOAP response into envelope XML and attachments.
func ParseMTOMResponse(contentType string, body io.Reader) ([]byte, map[string]MTOMAttachment, bool, error) {
	mediaType, params, err := mime.ParseMediaType(contentType)
	if err != nil || !strings.HasPrefix(mediaType, "multipart/") {
		data, readErr := io.ReadAll(body)
		return data, nil, false, readErr
	}
	boundary := params["boundary"]
	if boundary == "" {
		return nil, nil, true, fmt.Errorf("multipart response missing boundary")
	}
	startCID := strings.Trim(params["start"], "<>")

	reader := multipart.NewReader(body, boundary)
	attachments := make(map[string]MTOMAttachment)
	var envelope []byte
	partIndex := 0
	for {
		part, partErr := reader.NextPart()
		if partErr == io.EOF {
			break
		}
		if partErr != nil {
			return nil, nil, true, partErr
		}
		data, readErr := io.ReadAll(part)
		if readErr != nil {
			return nil, nil, true, readErr
		}
		contentID := strings.Trim(part.Header.Get("Content-ID"), "<>")
		partType := part.Header.Get("Content-Type")
		isStartPart := startCID != "" && contentID == startCID
		isFallbackRoot := startCID == "" && partIndex == 0
		partIndex++
		if envelope == nil && (isStartPart || isFallbackRoot) {
			envelope = data
			continue
		}
		if contentID == "" {
			return nil, nil, true, fmt.Errorf("multipart attachment missing Content-ID")
		}
		attachments[contentID] = MTOMAttachment{
			ContentID:   contentID,
			ContentType: partType,
			Data:        data,
		}
	}
	if envelope == nil {
		return nil, nil, true, fmt.Errorf("multipart response missing SOAP envelope part")
	}
	return envelope, attachments, true, nil
}
