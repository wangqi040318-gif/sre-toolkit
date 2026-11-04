package cli

import (
	"bytes"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

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
	out, _, err := execCLI(t, "triage")
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(out, "TODO:") || !strings.Contains(out, "计划中") {
		t.Fatalf("triage = %s", out)
	}
	longHelp, _, err := execCLI(t, "diff-review", "--help")
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

func TestVersion(t *testing.T) {
	out, _, err := execCLI(t, "version")
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(out, "sre ") {
		t.Fatalf("version = %s", out)
	}
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
