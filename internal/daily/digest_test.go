package daily

import (
	"strings"
	"testing"
	"time"
)

func TestDemoDigest(t *testing.T) {
	now := time.Date(2026, 11, 4, 12, 0, 0, 0, time.UTC)
	clusters := Demo(now)
	if len(clusters) != 2 {
		t.Fatalf("clusters = %d", len(clusters))
	}
	eu, us := clusters[0], clusters[1]
	if eu.Context != "prod-eu" || us.Context != "prod-us" {
		t.Fatalf("names = %s %s", eu.Context, us.Context)
	}
	if eu.NodesNotReady != 1 || eu.CrashLoop != 2 || eu.OOMKilled != 1 || eu.FailedJobs != 1 || eu.WarningEvents != 4 {
		t.Fatalf("eu = %+v", eu)
	}
	if eu.CertsDue < 1 || eu.CertsWarn < 1 {
		t.Fatalf("eu certs = due %d warn %d", eu.CertsDue, eu.CertsWarn)
	}
	if !us.Healthy() || us.NodesTotal != 2 || us.NodesNotReady != 0 {
		t.Fatalf("us = %+v", us)
	}
	rep := Digest(clusters, true)
	if rep.Findings[0].Category != "Digest" || rep.Findings[0].Severity != SevCritical {
		t.Fatalf("headline = %+v", rep.Findings[0])
	}
	if !strings.Contains(rep.Findings[0].Phenomenon, "prod-eu") || strings.Contains(rep.Findings[0].Phenomenon, "prod-us") {
		t.Fatalf("headline text = %s", rep.Findings[0].Phenomenon)
	}
	blob := render(rep)
	for _, want := range []string{"crashloop 2", "oom 1", "failed jobs 1", "certs due", "sre k8s-pulse --context prod-eu -A", "sre certs --context prod-eu -A", "2/2 nodes NotReady"} {
		if want == "2/2 nodes NotReady" {
			if !strings.Contains(blob, "0/2 nodes NotReady") {
				t.Fatalf("missing quiet node line\n%s", blob)
			}
			continue
		}
		if !strings.Contains(blob, want) {
			t.Fatalf("missing %q\n%s", want, blob)
		}
	}
	if !strings.Contains(rep.Notes[0], "does not contact") {
		t.Fatalf("notes = %v", rep.Notes)
	}
}

func TestMergeContexts(t *testing.T) {
	path := t.TempDir() + "/contexts"
	body := "# morning\nprod-eu\n\nprod-us\nprod-eu\n# skip\n"
	if err := writeFile(path, body); err != nil {
		t.Fatal(err)
	}
	got, err := MergeContexts([]string{"prod-eu", " staging "}, path)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Join(got, ",") != "prod-eu,staging,prod-us" {
		t.Fatalf("got %v", got)
	}
	if _, err := MergeContexts(nil, path+"-missing"); err == nil {
		t.Fatal("expected missing file")
	}
	empty, err := MergeContexts(nil, "")
	if err != nil || len(empty) != 0 {
		t.Fatalf("empty = %v %v", empty, err)
	}
}

func render(rep Report) string {
	var b strings.Builder
	for _, f := range rep.Findings {
		b.WriteString(f.Resource)
		b.WriteByte(' ')
		b.WriteString(f.Phenomenon)
		b.WriteByte('\n')
		b.WriteString(f.NextStep)
		b.WriteByte('\n')
	}
	return b.String()
}
