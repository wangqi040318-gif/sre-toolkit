package output

import (
	"bytes"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func sampleDoc() Document {
	return Document{
		Title:       "Kubernetes pulse",
		Command:     "k8s-pulse",
		GeneratedAt: time.Date(2026, 11, 4, 12, 0, 0, 0, time.UTC),
		Demo:        true,
		Meta:        []Field{{Name: "Cluster", Value: "demo-cluster"}},
		Summary:     []Counter{{Name: "Nodes", Count: 4}},
		Findings: []Finding{{
			Severity:    SeverityCritical,
			Category:    "OOMKilled",
			Resource:    "observability/prometheus-0",
			Phenomenon:  "Container prometheus was OOMKilled.",
			LikelyCause: "The container crossed its memory limit.",
			NextStep:    "kubectl describe pod > /tmp/pod.txt",
			Details:     []Field{{Name: "container", Value: "prometheus"}},
		}},
		Notes: []string{"Demo mode uses synthetic data."},
	}
}

func TestWriteHumanOrderAndColor(t *testing.T) {
	var plain bytes.Buffer
	if err := WriteHuman(&plain, sampleDoc(), false); err != nil {
		t.Fatal(err)
	}
	body := plain.String()
	if strings.Contains(body, "\033[") {
		t.Fatalf("plain output contains ANSI: %q", body)
	}
	p := strings.Index(body, "Phenomenon")
	c := strings.Index(body, "Likely cause")
	n := strings.Index(body, "Next step")
	if p < 0 || !(p < c && c < n) {
		t.Fatalf("order p=%d c=%d n=%d\n%s", p, c, n, body)
	}
	for _, want := range []string{"Kubernetes pulse", "Findings (1)", "CRITICAL", "OOMKilled", "Demo mode uses synthetic data."} {
		if !strings.Contains(body, want) {
			t.Fatalf("missing %q\n%s", want, body)
		}
	}

	var colored bytes.Buffer
	if err := WriteHuman(&colored, sampleDoc(), true); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(colored.String(), "\033[") {
		t.Fatal("expected ANSI color")
	}
}

func TestWriteHumanEmptyFindings(t *testing.T) {
	doc := sampleDoc()
	doc.Findings = nil
	var buf bytes.Buffer
	if err := WriteHuman(&buf, doc, false); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(buf.String(), "Findings (0)") || !strings.Contains(buf.String(), "No findings in this scope.") {
		t.Fatalf("body = %s", buf.String())
	}
}

func TestWriteJSONDoesNotEscapeHTML(t *testing.T) {
	var buf bytes.Buffer
	if err := WriteJSON(&buf, sampleDoc()); err != nil {
		t.Fatal(err)
	}
	if strings.Contains(buf.String(), `\u003e`) {
		t.Fatalf("html escaped: %s", buf.String())
	}
	if !strings.Contains(buf.String(), "kubectl describe pod > /tmp/pod.txt") {
		t.Fatalf("body = %s", buf.String())
	}
	var doc Document
	if err := json.Unmarshal(buf.Bytes(), &doc); err != nil {
		t.Fatal(err)
	}
	if !doc.Demo || doc.Summary[0].Count != 4 || doc.Findings[0].Category != "OOMKilled" {
		t.Fatalf("decoded = %+v", doc)
	}
	if doc.Findings[0].Details[0].Value != "prometheus" {
		t.Fatalf("details = %+v", doc.Findings[0].Details)
	}
}

func TestMarkdownFileAndRender(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "pulse.md")
	var out, errb bytes.Buffer
	doc := sampleDoc()
	if err := Render(&out, &errb, Options{JSON: true, Report: path}, doc); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(out.String(), `"command": "k8s-pulse"`) {
		t.Fatalf("stdout = %s", out.String())
	}
	if strings.Contains(out.String(), "\033[") {
		t.Fatal("json stdout contains color")
	}
	if !strings.Contains(errb.String(), "wrote markdown report to "+path) {
		t.Fatalf("stderr = %s", errb.String())
	}
	raw, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	text := string(raw)
	if strings.Contains(text, "\033[") {
		t.Fatal("markdown contains color")
	}
	for _, want := range []string{"# Kubernetes pulse", "## Summary", "## Findings", "**Phenomenon:**", "**Likely cause:**", "**Next step:**", "container"} {
		if !strings.Contains(text, want) {
			t.Fatalf("markdown missing %q\n%s", want, text)
		}
	}

	if err := Render(&out, &errb, Options{JSON: true, Markdown: true}, doc); err == nil {
		t.Fatal("expected mutually exclusive formats")
	}
	if err := WriteMarkdownFile("-", doc); err == nil {
		t.Fatal("expected report path rejection")
	}
}

func TestColorOn(t *testing.T) {
	if colorOn(true, true) {
		t.Fatal("noColor flag should disable color")
	}
	if colorOn(false, false) {
		t.Fatal("non-tty should not color")
	}
	if _, ok := os.LookupEnv("NO_COLOR"); !ok && !colorOn(false, true) {
		t.Fatal("tty without NO_COLOR should color")
	}
}

func TestColorOnNoColorEnv(t *testing.T) {
	t.Setenv("NO_COLOR", "1")
	if colorOn(false, true) {
		t.Fatal("NO_COLOR should disable color")
	}
}
