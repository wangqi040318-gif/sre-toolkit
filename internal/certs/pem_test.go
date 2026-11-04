package certs

import (
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/pem"
	"math/big"
	"strings"
	"testing"
	"time"
)

func TestParsePEMSkipsKeyAndReadsLeaf(t *testing.T) {
	now := time.Date(2026, 11, 4, 12, 0, 0, 0, time.UTC)
	notAfter := now.Add(10 * 24 * time.Hour)
	certPEM, keyPEM, err := mintCert(now, "api.example.com", []string{"api.example.com", "www.example.com"}, notAfter, 7)
	if err != nil {
		t.Fatal(err)
	}
	bundle := append(append([]byte{}, keyPEM...), certPEM...)
	parsed, err := ParsePEM(bundle)
	if err != nil {
		t.Fatal(err)
	}
	if len(parsed) != 1 {
		t.Fatalf("certs = %d", len(parsed))
	}
	if parsed[0].Subject != "api.example.com" {
		t.Fatalf("subject = %s", parsed[0].Subject)
	}
	if !parsed[0].NotAfter.Equal(notAfter.UTC()) {
		t.Fatalf("notAfter = %s", parsed[0].NotAfter)
	}
	if len(parsed[0].DNSNames) != 2 || parsed[0].DNSNames[1] != "www.example.com" {
		t.Fatalf("dns = %v", parsed[0].DNSNames)
	}
	if parsed[0].IsCA {
		t.Fatal("leaf marked CA")
	}
}

func TestParsePEMChain(t *testing.T) {
	now := time.Date(2026, 6, 1, 0, 0, 0, 0, time.UTC)
	caPEM, ca, caKey := mintCA(t, now.Add(365*24*time.Hour))
	leafPEM := mintLeaf(t, ca, caKey, "app.example", now.Add(20*24*time.Hour))
	parsed, err := ParsePEM(append(append([]byte{}, leafPEM...), caPEM...))
	if err != nil {
		t.Fatal(err)
	}
	if len(parsed) != 2 {
		t.Fatalf("certs = %d", len(parsed))
	}
	if parsed[0].Subject != "app.example" || parsed[0].IsCA {
		t.Fatalf("leaf = %+v", parsed[0])
	}
	if parsed[1].Subject != "demo-ca" || !parsed[1].IsCA {
		t.Fatalf("ca = %+v", parsed[1])
	}
	if parsed[0].Issuer != "demo-ca" {
		t.Fatalf("issuer = %s", parsed[0].Issuer)
	}
}

func TestParsePEMErrors(t *testing.T) {
	if _, err := ParsePEM(nil); err == nil {
		t.Fatal("expected empty error")
	}
	if _, err := ParsePEM([]byte("not a pem")); err == nil {
		t.Fatal("expected no blocks error")
	}
	key, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	der, err := x509.MarshalPKCS8PrivateKey(key)
	if err != nil {
		t.Fatal(err)
	}
	onlyKey := pem.EncodeToMemory(&pem.Block{Type: "PRIVATE KEY", Bytes: der})
	if _, err := ParsePEM(onlyKey); err == nil || !strings.Contains(err.Error(), "no certificates") {
		t.Fatalf("err = %v", err)
	}
	bad := pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: []byte("nope")})
	if _, err := ParsePEM(bad); err == nil {
		t.Fatal("expected parse error")
	}
}

func mintCA(t *testing.T, notAfter time.Time) (pemBytes []byte, cert *x509.Certificate, key *ecdsa.PrivateKey) {
	t.Helper()
	key, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	tmpl := &x509.Certificate{
		SerialNumber:          big.NewInt(1),
		Subject:               pkix.Name{CommonName: "demo-ca"},
		NotBefore:             notAfter.Add(-2 * 365 * 24 * time.Hour),
		NotAfter:              notAfter,
		IsCA:                  true,
		BasicConstraintsValid: true,
		KeyUsage:              x509.KeyUsageCertSign | x509.KeyUsageDigitalSignature,
	}
	der, err := x509.CreateCertificate(rand.Reader, tmpl, tmpl, &key.PublicKey, key)
	if err != nil {
		t.Fatal(err)
	}
	parsed, err := x509.ParseCertificate(der)
	if err != nil {
		t.Fatal(err)
	}
	return pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: der}), parsed, key
}

func mintLeaf(t *testing.T, ca *x509.Certificate, caKey *ecdsa.PrivateKey, cn string, notAfter time.Time) []byte {
	t.Helper()
	key, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	tmpl := &x509.Certificate{
		SerialNumber: big.NewInt(2),
		Subject:      pkix.Name{CommonName: cn},
		DNSNames:     []string{cn},
		NotBefore:    notAfter.Add(-24 * time.Hour),
		NotAfter:     notAfter,
		KeyUsage:     x509.KeyUsageDigitalSignature,
		ExtKeyUsage:  []x509.ExtKeyUsage{x509.ExtKeyUsageServerAuth},
	}
	der, err := x509.CreateCertificate(rand.Reader, tmpl, ca, &key.PublicKey, caKey)
	if err != nil {
		t.Fatal(err)
	}
	return pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: der})
}
