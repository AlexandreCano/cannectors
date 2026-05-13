package soapclient

import (
	"crypto/rand"
	"crypto/sha1"
	"encoding/base64"
	"fmt"
	"time"
)

// WSSPasswordType selects the UsernameToken password representation.
type WSSPasswordType string

const (
	// WSSPasswordText sends the password as plain text inside the UsernameToken.
	WSSPasswordText WSSPasswordType = "PasswordText"
	// WSSPasswordDigest sends Base64(SHA1(nonce + created + password)).
	WSSPasswordDigest WSSPasswordType = "PasswordDigest"

	wsseNamespace = "http://docs.oasis-open.org/wss/2004/01/oasis-200401-wss-wssecurity-secext-1.0.xsd"
	wsuNamespace  = "http://docs.oasis-open.org/wss/2004/01/oasis-200401-wss-wssecurity-utility-1.0.xsd"
)

// WSSecurityConfig configures a WS-Security UsernameToken header.
type WSSecurityConfig struct {
	Username       string
	Password       string
	PasswordType   WSSPasswordType
	TokenID        string
	MustUnderstand bool
	Nonce          []byte
	Created        time.Time
}

// BuildWSSecurityHeader builds a WS-Security UsernameToken SOAP header.
func BuildWSSecurityHeader(cfg WSSecurityConfig) ([]byte, error) {
	if cfg.Username == "" || cfg.Password == "" {
		return nil, fmt.Errorf("WS-Security UsernameToken requires username and password")
	}
	if cfg.PasswordType == "" {
		cfg.PasswordType = WSSPasswordText
	}
	if cfg.Created.IsZero() {
		cfg.Created = time.Now().UTC()
	}

	created := cfg.Created.UTC().Format(time.RFC3339)
	password := cfg.Password
	passwordTypeURI := "http://docs.oasis-open.org/wss/2004/01/oasis-200401-wss-username-token-profile-1.0#PasswordText"
	var nonceText string

	switch cfg.PasswordType {
	case WSSPasswordText:
	case WSSPasswordDigest:
		if len(cfg.Nonce) == 0 {
			cfg.Nonce = make([]byte, 16)
			if _, err := rand.Read(cfg.Nonce); err != nil {
				return nil, fmt.Errorf("generating WS-Security nonce: %w", err)
			}
		}
		digestInput := make([]byte, 0, len(cfg.Nonce)+len(created)+len(cfg.Password))
		digestInput = append(digestInput, cfg.Nonce...)
		digestInput = append(digestInput, []byte(created)...)
		digestInput = append(digestInput, []byte(cfg.Password)...)
		sum := sha1.Sum(digestInput)
		password = base64.StdEncoding.EncodeToString(sum[:])
		passwordTypeURI = "http://docs.oasis-open.org/wss/2004/01/oasis-200401-wss-username-token-profile-1.0#PasswordDigest"
		nonceText = base64.StdEncoding.EncodeToString(cfg.Nonce)
	default:
		return nil, fmt.Errorf("unsupported WS-Security password type %q", cfg.PasswordType)
	}

	tokenID := ""
	if cfg.TokenID != "" {
		tokenID = ` wsu:Id="` + escapeXMLText(cfg.TokenID) + `"`
	}
	mustUnderstand := ""
	if cfg.MustUnderstand {
		mustUnderstand = ` soap:mustUnderstand="1"`
	}

	header := `<wsse:Security xmlns:wsse="` + wsseNamespace + `" xmlns:wsu="` + wsuNamespace + `"` + mustUnderstand + `>` +
		`<wsse:UsernameToken` + tokenID + `>` +
		`<wsse:Username>` + escapeXMLText(cfg.Username) + `</wsse:Username>` +
		`<wsse:Password Type="` + passwordTypeURI + `">` + escapeXMLText(password) + `</wsse:Password>`
	if cfg.PasswordType == WSSPasswordDigest {
		header += `<wsse:Nonce>` + nonceText + `</wsse:Nonce>`
	}
	header += `<wsu:Created>` + escapeXMLText(created) + `</wsu:Created>`
	header += `</wsse:UsernameToken></wsse:Security>`
	return []byte(header), nil
}
