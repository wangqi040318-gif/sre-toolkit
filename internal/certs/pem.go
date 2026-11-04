package certs

import (
	"crypto/x509"
	"encoding/pem"
	"fmt"
	"strings"
	"time"
)

// Parsed is one certificate taken from a PEM bundle. Private keys are ignored.
type Parsed struct {
	Subject   string
	Issuer    string
	DNSNames  []string
	NotBefore time.Time
	NotAfter  time.Time
	IsCA      bool
}

// ParsePEM returns every CERTIFICATE block in data, in order.
// Key blocks are skipped. An empty bundle or a bundle with no certificates
// is an error. A certificate block that does not parse fails the bundle.
func ParsePEM(data []byte) ([]Parsed, error) {
	if len(strings.TrimSpace(string(data))) == 0 {
		return nil, fmt.Errorf("empty PEM")
	}
	var out []Parsed
	rest := data
	blocks := 0
	for {
		var block *pem.Block
		block, rest = pem.Decode(rest)
		if block == nil {
			break
		}
		blocks++
		if !isCertificateBlock(block.Type) {
			continue
		}
		cert, err := x509.ParseCertificate(block.Bytes)
		if err != nil {
			return nil, fmt.Errorf("parse certificate block %d: %w", blocks, err)
		}
		out = append(out, parsedFrom(cert))
	}
	if blocks == 0 {
		return nil, fmt.Errorf("no PEM blocks found")
	}
	if len(out) == 0 {
		return nil, fmt.Errorf("no certificates found in PEM")
	}
	return out, nil
}

func isCertificateBlock(typ string) bool {
	t := strings.ToUpper(strings.TrimSpace(typ))
	if strings.Contains(t, "REQUEST") || strings.Contains(t, "KEY") {
		return false
	}
	switch t {
	case "CERTIFICATE", "X509 CERTIFICATE", "TRUSTED CERTIFICATE":
		return true
	default:
		return false
	}
}

func parsedFrom(cert *x509.Certificate) Parsed {
	subject := cert.Subject.CommonName
	if subject == "" && len(cert.DNSNames) > 0 {
		subject = cert.DNSNames[0]
	}
	if subject == "" {
		subject = cert.Subject.String()
	}
	issuer := cert.Issuer.CommonName
	if issuer == "" {
		issuer = cert.Issuer.String()
	}
	return Parsed{
		Subject:   subject,
		Issuer:    issuer,
		DNSNames:  append([]string(nil), cert.DNSNames...),
		NotBefore: cert.NotBefore.UTC(),
		NotAfter:  cert.NotAfter.UTC(),
		IsCA:      cert.IsCA,
	}
}
