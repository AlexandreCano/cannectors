package soapclient

import (
	"crypto/sha1"
	"encoding/base64"
	"strings"
	"testing"
	"time"
)

func TestBuildWSSecurityHeader_PasswordText(t *testing.T) {
	header, err := BuildWSSecurityHeader(WSSecurityConfig{
		Username:     "alice",
		Password:     "secret",
		PasswordType: WSSPasswordText,
		TokenID:      "token-1",
	})
	if err != nil {
		t.Fatalf("BuildWSSecurityHeader returned error: %v", err)
	}
	got := string(header)
	for _, want := range []string{
		`<wsse:Security`,
		`<wsse:Username>alice</wsse:Username>`,
		`#PasswordText`,
		`<wsse:Password`,
		`secret</wsse:Password>`,
		`<wsu:Created>`,
		`wsu:Id="token-1"`,
	} {
		if !strings.Contains(got, want) {
			t.Fatalf("header missing %q:\n%s", want, got)
		}
	}
}

func TestBuildWSSecurityHeader_PasswordDigest(t *testing.T) {
	nonce := []byte("1234567890123456")
	created := time.Date(2026, 5, 12, 10, 0, 0, 0, time.UTC)
	header, err := BuildWSSecurityHeader(WSSecurityConfig{
		Username:     "alice",
		Password:     "secret",
		PasswordType: WSSPasswordDigest,
		Nonce:        nonce,
		Created:      created,
	})
	if err != nil {
		t.Fatalf("BuildWSSecurityHeader returned error: %v", err)
	}

	sum := sha1.Sum(append(append(nonce, []byte(created.Format(time.RFC3339))...), []byte("secret")...))
	wantDigest := base64.StdEncoding.EncodeToString(sum[:])
	got := string(header)
	for _, want := range []string{
		`#PasswordDigest`,
		`<wsse:Nonce>` + base64.StdEncoding.EncodeToString(nonce) + `</wsse:Nonce>`,
		`<wsu:Created>2026-05-12T10:00:00Z</wsu:Created>`,
		wantDigest,
	} {
		if !strings.Contains(got, want) {
			t.Fatalf("header missing %q:\n%s", want, got)
		}
	}
}
