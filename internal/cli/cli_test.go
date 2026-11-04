package cli

import (
	"bytes"
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/json"
	"encoding/pem"
	"math/big"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/wangqi040318-gif/sre-toolkit/internal/output"
)

func TestPulseDemoJSON(t *testing.T) {
	out, _, err := execCLI(t, "k8s-pulse", "--demo", "--json", "--no-color")
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(out, "\033[") {
		t.Fatal("json contains color")
	}
	var doc output.Document
	if err := json.Unmarshal([]byte(out), &doc); err != nil {
		t.Fatalf("unmarshal: %v\n%s", err, out)
	}
	if !doc.Demo || doc.Command != "k8s-pulse" {
		t.Fatalf("doc = %+v", doc)
	}
	counts := map[string]int{}
	for _, c := range doc.Summary {
		counts[c.Name] = c.Count
	}
	if counts["CrashLoopBackOff"] != 1 || counts["OOMKilled"] != 1 || counts["High restarts"] != 1 || counts["Nodes not ready"] != 1 || counts["Warning events"] != 3 {
		t.Fatalf("counts = %v", counts)
	}
	var sawCause, sawNext bool
	for _, f := range doc.Findings {
		if f.LikelyCause != "" {
			sawCause = true
		}
		if strings.Contains(f.NextStep, "kubectl") {
			sawNext = true
		}
	}
	if !sawCause || !sawNext {
		t.Fatal("expected likely cause and kubectl next step")
	}
}

func TestPulseDemoHumanAndReport(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "pulse.md")
	out, stderr, err := execCLI(t, "k8s-pulse", "--demo", "--no-color", "--report", path)
	if err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{"Phenomenon", "Likely cause", "Next step", "STRIPE_API_KEY", "worker-3", "DiskPressure", "synthetic"} {
		if !strings.Contains(out, want) {
			t.Fatalf("stdout missing %q\n%s", want, out)
		}
	}
	p := strings.Index(out, "Phenomenon")
	c := strings.Index(out, "Likely cause")
	n := strings.Index(out, "Next step")
	if !(p >= 0 && p < c && c < n) {
		t.Fatalf("order p=%d c=%d n=%d", p, c, n)
	}
	if strings.Contains(out, "\033[") {
		t.Fatal("no-color still colored")
	}
	if !strings.Contains(stderr, path) {
		t.Fatalf("stderr = %s", stderr)
	}
	raw, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(raw), "## Findings") || strings.Contains(string(raw), "\033[") {
		t.Fatalf("report = %s", raw)
	}
}

func TestPulseDemoNamespaceFilter(t *testing.T) {
	out, _, err := execCLI(t, "k8s-pulse", "--demo", "-n", "shop", "--json")
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(out, "checkout-7d4c8f9b6") || strings.Contains(out, "prometheus-0") {
		t.Fatalf("namespace filter leaked\n%s", out)
	}
	if !strings.Contains(out, "recommendations") || !strings.Contains(out, "worker-3") {
		t.Fatalf("expected shop finding and not-ready node\n%s", out)
	}
}

func TestPulseFlagErrors(t *testing.T) {
	_, _, err := execCLI(t, "k8s-pulse", "--demo", "-A", "-n", "payments")
	if err == nil || !strings.Contains(err.Error(), "either") {
		t.Fatalf("err = %v", err)
	}
	_, _, err = execCLI(t, "k8s-pulse", "--demo", "--json", "--markdown")
	if err == nil || !strings.Contains(err.Error(), "either") {
		t.Fatalf("err = %v", err)
	}
	_, _, err = execCLI(t, "k8s-pulse", "--demo", "-n", "Not_A_Namespace")
	if err == nil || !strings.Contains(err.Error(), "invalid namespace") {
		t.Fatalf("err = %v", err)
	}
}

func TestPlannedCommandsAndHelp(t *testing.T) {
	help, _, err := execCLI(t, "--help")
	if err != nil {
		t.Fatal(err)
	}
	for _, name := range []string{"k8s-pulse", "diff-review", "triage", "slo", "certs", "waste", "runbook", "daily", "version"} {
		if !strings.Contains(help, name) {
			t.Fatalf("help missing %s\n%s", name, help)
		}
	}
	out, _, err := execCLI(t, "daily")
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(out, "TODO:") || !strings.Contains(out, "计划中") {
		t.Fatalf("daily = %s", out)
	}
	longHelp, _, err := execCLI(t, "daily", "--help")
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(longHelp, "TODO:") || !strings.Contains(longHelp, "计划中") {
		t.Fatalf("help = %s", longHelp)
	}
	js, _, err := execCLI(t, "daily", "--json")
	if err != nil {
		t.Fatal(err)
	}
	var doc output.Document
	if err := json.Unmarshal([]byte(js), &doc); err != nil {
		t.Fatal(err)
	}
	if doc.Command != "daily" || len(doc.Notes) == 0 || !strings.Contains(doc.Notes[0], "TODO:") {
		t.Fatalf("doc = %+v", doc)
	}
}

func TestCertsDemoJSON(t *testing.T) {
	out, _, err := execCLI(t, "certs", "--demo", "--json", "--no-color")
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(out, "PRIVATE KEY") || strings.Contains(out, "BEGIN ") {
		t.Fatal("json leaked key material")
	}
	var doc output.Document
	if err := json.Unmarshal([]byte(out), &doc); err != nil {
		t.Fatalf("unmarshal: %v\n%s", err, out)
	}
	if !doc.Demo || doc.Command != "certs" {
		t.Fatalf("doc = %+v", doc)
	}
	counts := map[string]int{}
	for _, c := range doc.Summary {
		counts[c.Name] = c.Count
	}
	if counts["Expired"] != 1 || counts["Critical"] != 1 || counts["Warning"] != 1 || counts["OK"] != 1 || counts["Missing secrets"] != 1 {
		t.Fatalf("counts = %v", counts)
	}
	var sawExpiry, sawNext bool
	for _, f := range doc.Findings {
		if strings.Contains(f.Phenomenon, "notAfter") || strings.Contains(f.Phenomenon, "expired") {
			sawExpiry = true
		}
		if strings.Contains(f.NextStep, "kubectl") {
			sawNext = true
		}
	}
	if !sawExpiry || !sawNext {
		t.Fatal("expected expiry and a kubectl next step")
	}
}

func TestCertsDemoHumanNamespace(t *testing.T) {
	out, _, err := execCLI(t, "certs", "--demo", "-n", "payments", "--no-color")
	if err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{"Phenomenon", "Likely cause", "Next step", "checkout.payments.example", "expired 12 days ago", "Severity: expired"} {
		if !strings.Contains(out, want) {
			t.Fatalf("missing %q\n%s", want, out)
		}
	}
	if strings.Contains(out, "api.shop.example") || strings.Contains(out, "PRIVATE KEY") {
		t.Fatalf("namespace filter leaked\n%s", out)
	}
}

func TestCertsFlagAndHelp(t *testing.T) {
	_, _, err := execCLI(t, "certs", "--demo", "-A", "-n", "payments")
	if err == nil || !strings.Contains(err.Error(), "either") {
		t.Fatalf("err = %v", err)
	}
	_, _, err = execCLI(t, "certs", "--demo", "--warn-days", "7", "--critical-days", "7")
	if err == nil || !strings.Contains(err.Error(), "warn-days") {
		t.Fatalf("err = %v", err)
	}
	help, _, err := execCLI(t, "certs", "--help")
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(help, "not implemented") || !strings.Contains(help, "--warn-days") || !strings.Contains(help, "--file") {
		t.Fatalf("help = %s", help)
	}
}

func TestCertsFileWithoutCluster(t *testing.T) {
	t.Setenv("KUBECONFIG", "")
	t.Setenv("HOME", t.TempDir())
	dir := t.TempDir()
	path := filepath.Join(dir, "leaf.pem")
	if err := os.WriteFile(path, mustCertPEM(t, "file.example"), 0o644); err != nil {
		t.Fatal(err)
	}
	out, _, err := execCLI(t, "certs", "--file", path, "--json", "--critical-days", "7", "--warn-days", "30")
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(out, "PRIVATE KEY") {
		t.Fatal("leaked key")
	}
	var doc output.Document
	if err := json.Unmarshal([]byte(out), &doc); err != nil {
		t.Fatal(err)
	}
	if doc.Demo || doc.Command != "certs" || len(doc.Findings) != 1 {
		t.Fatalf("doc findings = %d demo=%v", len(doc.Findings), doc.Demo)
	}
	if doc.Findings[0].Resource != "file "+path {
		t.Fatalf("resource = %s", doc.Findings[0].Resource)
	}
	if !strings.Contains(doc.Findings[0].Phenomenon, "file.example") {
		t.Fatalf("phenomenon = %s", doc.Findings[0].Phenomenon)
	}
}

func TestWasteDemoJSON(t *testing.T) {
	out, _, err := execCLI(t, "waste", "--demo", "--json", "--no-color")
	if err != nil {
		t.Fatal(err)
	}
	var doc output.Document
	if err := json.Unmarshal([]byte(out), &doc); err != nil {
		t.Fatalf("unmarshal: %v\n%s", err, out)
	}
	if !doc.Demo || doc.Command != "waste" {
		t.Fatalf("doc = %+v", doc)
	}
	counts := map[string]int{}
	for _, c := range doc.Summary {
		counts[c.Name] = c.Count
	}
	if counts["Idle"] != 2 || counts["Oversized"] != 1 || counts["Load balancers"] != 1 || counts["Orphaned PVCs"] != 1 || counts["Unbound PVCs"] != 1 {
		t.Fatalf("counts = %v", counts)
	}
	var sawWhy, sawNext bool
	for _, f := range doc.Findings {
		if f.Phenomenon != "" {
			sawWhy = true
		}
		if strings.Contains(f.NextStep, "kubectl") {
			sawNext = true
		}
	}
	if !sawWhy || !sawNext {
		t.Fatal("expected a reason and a kubectl next step")
	}
}

func TestWasteDemoNamespaceAndHelp(t *testing.T) {
	out, _, err := execCLI(t, "waste", "--demo", "-n", "shop", "--no-color")
	if err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{"Phenomenon", "Likely cause", "Next step", "recommendations", "storefront", "feature-flags"} {
		if !strings.Contains(out, want) {
			t.Fatalf("missing %q\n%s", want, out)
		}
	}
	if strings.Contains(out, "prometheus") || strings.Contains(out, "data/archive") {
		t.Fatalf("namespace leak\n%s", out)
	}
	_, _, err = execCLI(t, "waste", "--demo", "-A", "-n", "shop")
	if err == nil || !strings.Contains(err.Error(), "either") {
		t.Fatalf("err = %v", err)
	}
	help, _, err := execCLI(t, "waste", "--help")
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(help, "not implemented") || !strings.Contains(help, "--idle-for") || !strings.Contains(help, "--usage-ratio") {
		t.Fatalf("help = %s", help)
	}
}

func TestDiffReviewDemo(t *testing.T) {
	out, _, err := execCLI(t, "diff-review", "--demo", "--json", "--no-color")
	if err != nil {
		t.Fatal(err)
	}
	var doc output.Document
	if err := json.Unmarshal([]byte(out), &doc); err != nil {
		t.Fatalf("unmarshal: %v\n%s", err, out)
	}
	if !doc.Demo || doc.Command != "diff-review" {
		t.Fatalf("doc = %+v", doc)
	}
	counts := map[string]int{}
	for _, c := range doc.Summary {
		counts[c.Name] = c.Count
	}
	if counts["Critical"] < 1 || counts["High"] < 1 {
		t.Fatalf("counts = %v", counts)
	}
	if !strings.Contains(out, "checkout:latest") || !strings.Contains(out, "cluster-admin") || !strings.Contains(out, "hostPath") {
		t.Fatalf("demo missing review signals\n%s", out)
	}
	var sawOld, sawNew, sawRisk bool
	for _, f := range doc.Findings {
		for _, d := range f.Details {
			switch d.Name {
			case "old":
				if d.Value != "" {
					sawOld = true
				}
			case "new":
				if d.Value != "" {
					sawNew = true
				}
			case "risk":
				sawRisk = true
			}
		}
	}
	if !sawOld || !sawNew || !sawRisk {
		t.Fatal("expected old, new, and risk on findings")
	}
	help, _, err := execCLI(t, "diff-review", "--help")
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(help, "not implemented") || !strings.Contains(help, "--before") || !strings.Contains(help, "helm template") {
		t.Fatalf("help = %s", help)
	}
	_, _, err = execCLI(t, "diff-review", "--before", "only-one")
	if err == nil || !strings.Contains(err.Error(), "--before") {
		t.Fatalf("err = %v", err)
	}
}

func TestRunbookDemo(t *testing.T) {
	out, _, err := execCLI(t, "runbook", "--demo", "--json", "--no-color")
	if err != nil {
		t.Fatal(err)
	}
	var doc output.Document
	if err := json.Unmarshal([]byte(out), &doc); err != nil {
		t.Fatalf("unmarshal: %v\n%s", err, out)
	}
	if !doc.Demo || doc.Command != "runbook" {
		t.Fatalf("doc = %+v", doc)
	}
	if !strings.Contains(out, "kubectl rollout undo deployment/checkout -n shop --to-revision=11") {
		t.Fatalf("missing undo\n%s", out)
	}
	if !strings.Contains(out, "1.9.0") || !strings.Contains(out, "1.8.4") || !strings.Contains(out, "CrashLoopBackOff") {
		t.Fatalf("missing narrative\n%s", out)
	}
	if doc.Findings[0].Category != "State" || doc.Findings[len(doc.Findings)-1].Category != "Abort" {
		t.Fatalf("order = %s ... %s", doc.Findings[0].Category, doc.Findings[len(doc.Findings)-1].Category)
	}
	_, _, err = execCLI(t, "runbook", "--demo", "--revision", "9")
	if err == nil || !strings.Contains(err.Error(), "not found") {
		t.Fatalf("err = %v", err)
	}
	help, _, err := execCLI(t, "runbook", "--help")
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(help, "not implemented") || !strings.Contains(help, "--revision") {
		t.Fatalf("help = %s", help)
	}
}

func TestTriageDemo(t *testing.T) {
	out, _, err := execCLI(t, "triage", "--demo", "--json", "--no-color")
	if err != nil {
		t.Fatal(err)
	}
	var doc output.Document
	if err := json.Unmarshal([]byte(out), &doc); err != nil {
		t.Fatalf("unmarshal: %v\n%s", err, out)
	}
	if !doc.Demo || doc.Command != "triage" {
		t.Fatalf("doc = %+v", doc)
	}
	for _, want := range []string{"5xx ratio", "checkout", "p95", "kubectl get pods", "Logs are not queried", "http_requests_total"} {
		if !strings.Contains(out, want) {
			t.Fatalf("missing %q\n%s", want, out)
		}
	}
	_, _, err = execCLI(t, "triage", "checkout")
	if err == nil || !strings.Contains(err.Error(), "prometheus URL") {
		t.Fatalf("err = %v", err)
	}
	help, _, err := execCLI(t, "triage", "--help")
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(help, "not implemented") || !strings.Contains(help, "--prometheus-url") {
		t.Fatalf("help = %s", help)
	}
}

func TestSLODemo(t *testing.T) {
	out, _, err := execCLI(t, "slo", "--demo", "--json", "--no-color")
	if err != nil {
		t.Fatal(err)
	}
	var doc output.Document
	if err := json.Unmarshal([]byte(out), &doc); err != nil {
		t.Fatalf("unmarshal: %v\n%s", err, out)
	}
	if !doc.Demo || doc.Command != "slo" {
		t.Fatalf("doc = %+v", doc)
	}
	for _, want := range []string{"Page and freeze", "20.0x", "99.90%", "checkout", "http_requests_total", "[3d]"} {
		if !strings.Contains(out, want) {
			t.Fatalf("missing %q\n%s", want, out)
		}
	}
	_, _, err = execCLI(t, "slo", "checkout", "--window", "1h")
	if err == nil || !strings.Contains(err.Error(), "at least two") {
		t.Fatalf("err = %v", err)
	}
	help, _, err := execCLI(t, "slo", "--help")
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(help, "not implemented") || !strings.Contains(help, "--slo") {
		t.Fatalf("help = %s", help)
	}
}

func TestVersion(t *testing.T) {
	out, _, err := execCLI(t, "version")
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(out, "sre ") {
		t.Fatalf("version = %s", out)
	}
}

func mustCertPEM(t *testing.T, cn string) []byte {
	t.Helper()
	key, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	now := time.Now().UTC()
	tmpl := &x509.Certificate{
		SerialNumber: big.NewInt(1),
		Subject:      pkix.Name{CommonName: cn},
		DNSNames:     []string{cn},
		NotBefore:    now.Add(-time.Hour),
		NotAfter:     now.Add(90 * 24 * time.Hour),
		KeyUsage:     x509.KeyUsageDigitalSignature,
		ExtKeyUsage:  []x509.ExtKeyUsage{x509.ExtKeyUsageServerAuth},
	}
	der, err := x509.CreateCertificate(rand.Reader, tmpl, tmpl, &key.PublicKey, key)
	if err != nil {
		t.Fatal(err)
	}
	return pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: der})
}

func execCLI(t *testing.T, args ...string) (string, string, error) {
	t.Helper()
	root := NewRoot()
	var out, errb bytes.Buffer
	root.SetOut(&out)
	root.SetErr(&errb)
	root.SetArgs(args)
	err := root.Execute()
	return out.String(), errb.String(), err
}
