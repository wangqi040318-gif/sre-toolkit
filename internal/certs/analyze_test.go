package certs

import (
	"strings"
	"testing"
	"time"
)

func TestDemoPath(t *testing.T) {
	now := time.Date(2026, 11, 4, 12, 0, 0, 0, time.UTC)
	materials, err := DemoMaterials(now)
	if err != nil {
		t.Fatal(err)
	}
	rep := Analyze(materials, Options{AllNamespaces: true, Now: now, Demo: true, Cluster: "demo-cluster"})
	if rep.Cluster != "demo-cluster" || !rep.Demo {
		t.Fatalf("report = %+v", rep)
	}
	if rep.Summary.Certificates != 4 || rep.Summary.Expired != 1 || rep.Summary.Critical != 1 || rep.Summary.Warning != 1 || rep.Summary.OK != 1 || rep.Summary.Missing != 1 {
		t.Fatalf("summary = %+v", rep.Summary)
	}
	if rep.Findings[0].Category != CatExpired || !strings.Contains(rep.Findings[0].Resource, "payments/checkout-tls") {
		t.Fatalf("first = %+v", rep.Findings[0])
	}
	blob := renderFindings(rep)
	if strings.Contains(blob, "PRIVATE KEY") || strings.Contains(blob, "BEGIN") {
		t.Fatal("report leaked key material")
	}
	for _, want := range []string{
		"checkout.payments.example",
		"expired 12 days ago",
		"api.shop.example",
		"3 days remaining",
		"critical",
		"grafana.observability.example",
		"warn",
		"edge.platform.example",
		"batch-tls",
		"not found",
		"kubectl describe certificate -n payments checkout",
		"No cert-manager annotation",
		"issuer=demo-cluster-ca",
	} {
		if !strings.Contains(blob, want) {
			t.Fatalf("missing %q\n%s", want, blob)
		}
	}
	if !strings.Contains(rep.Notes[0], "synthetic") {
		t.Fatalf("notes = %v", rep.Notes)
	}

	shop := Analyze(materials, Options{Namespace: "shop", Now: now, Demo: true})
	if shop.Summary.Certificates != 1 || shop.Summary.Critical != 1 || shop.Summary.Expired != 0 || shop.Summary.Missing != 0 {
		t.Fatalf("shop summary = %+v", shop.Summary)
	}
	if !strings.Contains(shop.Findings[0].Resource, "shop/api-tls") {
		t.Fatalf("shop finding = %+v", shop.Findings[0])
	}
}

func TestAnalyzeHostMismatchAndUnreadable(t *testing.T) {
	now := time.Date(2026, 11, 4, 12, 0, 0, 0, time.UTC)
	pemBytes, _, err := mintCert(now, "only.example", []string{"only.example"}, now.Add(2*24*time.Hour), 4)
	if err != nil {
		t.Fatal(err)
	}
	rep := Analyze([]Material{
		{
			Source: SourceSecret, Namespace: "ns", Name: "web", SecretName: "web",
			Resource: "secret ns/web", Hosts: []string{"only.example", "extra.example"},
			PEM: pemBytes,
		},
		{
			Source: SourceFile, Name: "bad.pem", Resource: "file bad.pem", PEM: []byte("nope"),
		},
	}, Options{AllNamespaces: true, Now: now, CriticalDays: 7, WarnDays: 30})
	if rep.Summary.Critical != 1 || rep.Summary.Unreadable != 1 {
		t.Fatalf("summary = %+v", rep.Summary)
	}
	if !strings.Contains(rep.Findings[0].Phenomenon, "extra.example") || !strings.Contains(rep.Findings[0].NextStep, "SAN extra.example") {
		t.Fatalf("mismatch finding = %+v", rep.Findings[0])
	}
	if rep.Findings[1].Category != CatUnreadable {
		t.Fatalf("second = %+v", rep.Findings[1])
	}
}

func TestAnalyzeZeroOptionsUseDefaults(t *testing.T) {
	now := time.Date(2026, 11, 4, 0, 0, 0, 0, time.UTC)
	pemBytes, _, err := mintCert(now, "a.example", []string{"a.example"}, now.Add(10*24*time.Hour), 3)
	if err != nil {
		t.Fatal(err)
	}
	rep := Analyze([]Material{{
		Source: SourceSecret, Namespace: "ns", Name: "a", Resource: "secret ns/a", PEM: pemBytes,
	}}, Options{AllNamespaces: true, Now: now})
	if rep.WarnDays != 30 || rep.CriticalDays != 7 {
		t.Fatalf("windows = %d %d", rep.WarnDays, rep.CriticalDays)
	}
	if rep.Summary.Warning != 1 {
		t.Fatalf("summary = %+v", rep.Summary)
	}
}

func renderFindings(rep Report) string {
	var b strings.Builder
	for _, f := range rep.Findings {
		b.WriteString(f.Severity)
		b.WriteByte(' ')
		b.WriteString(f.Category)
		b.WriteByte(' ')
		b.WriteString(f.Resource)
		b.WriteByte('\n')
		b.WriteString(f.Phenomenon)
		b.WriteByte('\n')
		b.WriteString(f.LikelyCause)
		b.WriteByte('\n')
		b.WriteString(f.NextStep)
		b.WriteByte('\n')
		for _, d := range f.Details {
			b.WriteString(d.Name)
			b.WriteByte('=')
			b.WriteString(d.Value)
			b.WriteByte('\n')
		}
	}
	return b.String()
}
