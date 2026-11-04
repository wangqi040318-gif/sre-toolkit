package certs

import (
	"context"
	"strings"
	"testing"
	"time"

	corev1 "k8s.io/api/core/v1"
	networkingv1 "k8s.io/api/networking/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes/fake"
)

func TestCollectIngressAndSecret(t *testing.T) {
	now := time.Date(2026, 11, 4, 12, 0, 0, 0, time.UTC)
	pemBytes, _, err := mintCert(now, "pay.example", []string{"pay.example"}, now.Add(5*24*time.Hour), 11)
	if err != nil {
		t.Fatal(err)
	}
	secret := &corev1.Secret{
		ObjectMeta: metav1.ObjectMeta{
			Namespace: "payments",
			Name:      "pay-tls",
			Annotations: map[string]string{
				"cert-manager.io/certificate-name": "pay",
			},
		},
		Type: corev1.SecretTypeTLS,
		Data: map[string][]byte{corev1.TLSCertKey: pemBytes},
	}
	other := &corev1.Secret{
		ObjectMeta: metav1.ObjectMeta{Namespace: "payments", Name: "opaque"},
		Type:       corev1.SecretTypeOpaque,
		Data:       map[string][]byte{"tls.crt": pemBytes},
	}
	shop := &corev1.Secret{
		ObjectMeta: metav1.ObjectMeta{Namespace: "shop", Name: "shop-tls"},
		Type:       corev1.SecretTypeTLS,
		Data:       map[string][]byte{corev1.TLSCertKey: pemBytes},
	}
	ing := &networkingv1.Ingress{
		ObjectMeta: metav1.ObjectMeta{Namespace: "payments", Name: "pay"},
		Spec: networkingv1.IngressSpec{
			TLS: []networkingv1.IngressTLS{{
				Hosts:      []string{"pay.example", "alias.example"},
				SecretName: "pay-tls",
			}},
		},
	}
	missing := &networkingv1.Ingress{
		ObjectMeta: metav1.ObjectMeta{Namespace: "payments", Name: "legacy"},
		Spec: networkingv1.IngressSpec{
			TLS: []networkingv1.IngressTLS{{
				Hosts:      []string{"old.example"},
				SecretName: "missing-tls",
			}},
		},
	}
	client := fake.NewClientset(secret, other, shop, ing, missing)
	all, err := Collect(context.Background(), client, "", true)
	if err != nil {
		t.Fatal(err)
	}
	if len(all) != 3 {
		t.Fatalf("materials = %d %+v", len(all), all)
	}
	rep := Analyze(all, Options{AllNamespaces: true, Now: now, CriticalDays: 7, WarnDays: 30, Cluster: "test"})
	if rep.Summary.Certificates != 2 || rep.Summary.Missing != 1 || rep.Summary.Critical != 2 {
		t.Fatalf("summary = %+v", rep.Summary)
	}
	var sawHost, sawCert bool
	for _, f := range rep.Findings {
		if f.Resource == "secret payments/pay-tls" {
			for _, d := range f.Details {
				if d.Name == "hosts" && d.Value == "alias.example, pay.example" {
					sawHost = true
				}
				if d.Name == "certificate" && d.Value == "pay" {
					sawCert = true
				}
			}
			if !strings.Contains(f.NextStep, "kubectl describe certificate -n payments pay") {
				t.Fatalf("next = %s", f.NextStep)
			}
		}
	}
	if !sawHost || !sawCert {
		t.Fatalf("findings = %+v", rep.Findings)
	}

	one, err := Collect(context.Background(), client, "shop", false)
	if err != nil {
		t.Fatal(err)
	}
	if len(one) != 1 || one[0].Name != "shop-tls" {
		t.Fatalf("shop = %+v", one)
	}
}

func TestCollectRejectsBadInput(t *testing.T) {
	if _, err := Collect(context.Background(), nil, "default", false); err == nil {
		t.Fatal("expected nil client error")
	}
	client := fake.NewClientset()
	if _, err := Collect(context.Background(), client, "", false); err == nil {
		t.Fatal("expected missing namespace error")
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if _, err := Collect(ctx, client, "default", false); err == nil {
		t.Fatal("expected canceled context")
	}
}
