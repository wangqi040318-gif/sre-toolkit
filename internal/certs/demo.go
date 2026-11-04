package certs

import (
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/pem"
	"fmt"
	"math/big"
	"time"
)

// DemoMaterials is a fixed certificate incident for resumes and on-call walkthroughs.
// Dates are relative to now so the tiers stay stable. It does not contact a cluster.
func DemoMaterials(now time.Time) ([]Material, error) {
	if now.IsZero() {
		now = time.Now().UTC()
	}
	now = now.UTC()
	type spec struct {
		ns, name, cn, certName string
		hosts                  []string
		ingress                string
		notAfter               time.Time
		includeKey             bool
	}
	specs := []spec{
		{
			ns: "payments", name: "checkout-tls", cn: "checkout.payments.example",
			certName: "checkout", hosts: []string{"checkout.payments.example"},
			ingress: "ingress payments/checkout", notAfter: now.Add(-12 * 24 * time.Hour),
			includeKey: true,
		},
		{
			ns: "shop", name: "api-tls", cn: "api.shop.example",
			certName: "api", hosts: []string{"api.shop.example"},
			ingress: "ingress shop/api", notAfter: now.Add(3 * 24 * time.Hour),
		},
		{
			ns: "observability", name: "grafana-tls", cn: "grafana.observability.example",
			hosts: []string{"grafana.observability.example"}, ingress: "ingress observability/grafana",
			notAfter: now.Add(18 * 24 * time.Hour),
		},
		{
			ns: "platform", name: "edge-tls", cn: "edge.platform.example",
			hosts: []string{"edge.platform.example"}, ingress: "ingress platform/edge",
			notAfter: now.Add(120 * 24 * time.Hour),
		},
	}
	ca, caKey, err := mintDemoCA(now)
	if err != nil {
		return nil, err
	}
	out := make([]Material, 0, len(specs)+1)
	for i, sp := range specs {
		bundle, err := mintBundle(ca, caKey, sp.cn, sp.hosts, sp.notAfter, int64(i+1), sp.includeKey)
		if err != nil {
			return nil, err
		}
		ann := map[string]string{}
		if sp.certName != "" {
			ann["cert-manager.io/certificate-name"] = sp.certName
		}
		out = append(out, Material{
			Source:      SourceSecret,
			Namespace:   sp.ns,
			Name:        sp.name,
			SecretName:  sp.name,
			Resource:    "secret " + sp.ns + "/" + sp.name,
			Hosts:       append([]string(nil), sp.hosts...),
			Ingresses:   []string{sp.ingress},
			Annotations: ann,
			PEM:         bundle,
		})
	}
	out = append(out, Material{
		Source:     SourceIngress,
		Namespace:  "data",
		Name:       "batch",
		SecretName: "batch-tls",
		Resource:   "secret data/batch-tls",
		Hosts:      []string{"batch.internal"},
		Ingresses:  []string{"data/batch"},
		Missing:    true,
	})
	return out, nil
}

func mintBundle(ca *x509.Certificate, caKey *ecdsa.PrivateKey, cn string, dns []string, notAfter time.Time, serial int64, includeKey bool) ([]byte, error) {
	key, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		return nil, err
	}
	tmpl := &x509.Certificate{
		SerialNumber: big.NewInt(serial),
		Subject:      pkix.Name{CommonName: cn},
		DNSNames:     dns,
		NotBefore:    notAfter.Add(-365 * 24 * time.Hour),
		NotAfter:     notAfter,
		KeyUsage:     x509.KeyUsageDigitalSignature,
		ExtKeyUsage:  []x509.ExtKeyUsage{x509.ExtKeyUsageServerAuth},
	}
	der, err := x509.CreateCertificate(rand.Reader, tmpl, ca, &key.PublicKey, caKey)
	if err != nil {
		return nil, fmt.Errorf("create certificate %s: %w", cn, err)
	}
	certPEM := pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: der})
	if !includeKey {
		return certPEM, nil
	}
	// A tls.crt that also carries a key is a common mistake. The parser
	// must ignore the key and the report must not echo it.
	keyDER, err := x509.MarshalPKCS8PrivateKey(key)
	if err != nil {
		return nil, err
	}
	keyPEM := pem.EncodeToMemory(&pem.Block{Type: "PRIVATE KEY", Bytes: keyDER})
	return append(append([]byte{}, certPEM...), keyPEM...), nil
}

func mintDemoCA(now time.Time) (*x509.Certificate, *ecdsa.PrivateKey, error) {
	key, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		return nil, nil, err
	}
	notAfter := now.Add(365 * 24 * time.Hour)
	tmpl := &x509.Certificate{
		SerialNumber:          big.NewInt(1000),
		Subject:               pkix.Name{CommonName: "demo-cluster-ca"},
		NotBefore:             now.Add(-2 * 365 * 24 * time.Hour),
		NotAfter:              notAfter,
		IsCA:                  true,
		BasicConstraintsValid: true,
		KeyUsage:              x509.KeyUsageCertSign | x509.KeyUsageDigitalSignature,
	}
	der, err := x509.CreateCertificate(rand.Reader, tmpl, tmpl, &key.PublicKey, key)
	if err != nil {
		return nil, nil, err
	}
	parsed, err := x509.ParseCertificate(der)
	if err != nil {
		return nil, nil, err
	}
	return parsed, key, nil
}

func mintCert(now time.Time, cn string, dns []string, notAfter time.Time, serial int64) (certPEM, keyPEM []byte, err error) {
	key, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		return nil, nil, err
	}
	tmpl := &x509.Certificate{
		SerialNumber: big.NewInt(serial),
		Subject:      pkix.Name{CommonName: cn},
		DNSNames:     dns,
		NotBefore:    notAfter.Add(-365 * 24 * time.Hour),
		NotAfter:     notAfter,
		KeyUsage:     x509.KeyUsageDigitalSignature,
		ExtKeyUsage:  []x509.ExtKeyUsage{x509.ExtKeyUsageServerAuth},
	}
	der, err := x509.CreateCertificate(rand.Reader, tmpl, tmpl, &key.PublicKey, key)
	if err != nil {
		return nil, nil, fmt.Errorf("create certificate %s: %w", cn, err)
	}
	certPEM = pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: der})
	keyDER, err := x509.MarshalPKCS8PrivateKey(key)
	if err != nil {
		return nil, nil, err
	}
	keyPEM = pem.EncodeToMemory(&pem.Block{Type: "PRIVATE KEY", Bytes: keyDER})
	return certPEM, keyPEM, nil
}
