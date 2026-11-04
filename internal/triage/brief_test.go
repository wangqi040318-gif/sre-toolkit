package triage

import (
	"strings"
	"testing"
	"time"
)

func TestDemoBrief(t *testing.T) {
	end := time.Date(2026, 11, 4, 12, 0, 0, 0, time.UTC)
	q, err := (Templates{}).Render("checkout")
	if err != nil {
		t.Fatal(err)
	}
	rep := Assemble(WithQueries(DemoBundle("checkout", end), q), q)
	if !rep.Summary.HasErrors || rep.Summary.ErrorRatio < 0.1 {
		t.Fatalf("error ratio = %v %+v", rep.Summary.ErrorRatio, rep.Summary)
	}
	if !rep.Summary.HasP95 || rep.Summary.P95Seconds < 1 {
		t.Fatalf("p95 = %v", rep.Summary.P95Seconds)
	}
	blob := render(rep)
	for _, want := range []string{
		"5xx ratio",
		"14.0%",
		"p95",
		"1.40s",
		"bad rollout",
		"kubectl get pods -A -l app=checkout",
		"kubectl rollout history deployment/checkout",
		"sre runbook --name checkout",
		`job="checkout"`,
		"Logs are not queried",
		"synthetic",
	} {
		if !strings.Contains(blob, want) && !containsNote(rep, want) {
			t.Fatalf("missing %q\n%s\nnotes=%v", want, blob, rep.Notes)
		}
	}
	if find(rep, "ErrorRate").Severity != "critical" || find(rep, "Latency").Severity != "critical" {
		t.Fatalf("severities error=%s latency=%s", find(rep, "ErrorRate").Severity, find(rep, "Latency").Severity)
	}
	if find(rep, "Hypothesis").Severity != "critical" {
		t.Fatalf("hypothesis = %+v", find(rep, "Hypothesis"))
	}
}

func TestQuietAndMissing(t *testing.T) {
	end := time.Date(2026, 11, 4, 12, 0, 0, 0, time.UTC)
	start := end.Add(-time.Hour)
	step := 5 * time.Minute
	quiet := Bundle{
		Service:  "edge",
		Start:    start,
		End:      end,
		Requests: Signal{Name: "requests", Series: seriesFrom(start, step, 40, 41, 39, 40)},
		Errors:   Signal{Name: "errors", Series: seriesFrom(start, step, 0.001, 0.001, 0.002, 0.001)},
		P95:      Signal{Name: "p95", Series: seriesFrom(start, step, 0.05, 0.06, 0.05, 0.05)},
		P99:      Signal{Name: "p99", Series: seriesFrom(start, step, 0.08, 0.09, 0.08, 0.08)},
	}
	rep := Assemble(quiet, Queries{})
	if find(rep, "ErrorRate").Severity != "info" || find(rep, "Hypothesis").Severity != "info" {
		t.Fatalf("quiet should stay info: %+v", rep.Findings)
	}

	missing := Bundle{
		Service:  "ghost",
		Start:    start,
		End:      end,
		Requests: Signal{Name: "requests", Missing: true, Query: "req"},
		Errors:   Signal{Name: "errors", Missing: true, Query: "err"},
		P95:      Signal{Name: "p95", Missing: true, Query: "p95"},
		P99:      Signal{Name: "p99", Err: "server timeout", Query: "p99"},
	}
	rep = Assemble(missing, Queries{Errors: "err"})
	if find(rep, "Hypothesis").Severity != "warning" || !strings.Contains(find(rep, "Hypothesis").Phenomenon, "no series") {
		t.Fatalf("missing hypothesis = %+v", find(rep, "Hypothesis"))
	}
	if !containsNote(rep, "p99 query failed") {
		t.Fatalf("notes = %v", rep.Notes)
	}
}

func find(rep Report, category string) Finding {
	for _, f := range rep.Findings {
		if f.Category == category {
			return f
		}
	}
	return Finding{}
}

func render(rep Report) string {
	var b strings.Builder
	for _, f := range rep.Findings {
		b.WriteString(f.Category)
		b.WriteByte(' ')
		b.WriteString(f.Phenomenon)
		b.WriteByte('\n')
		b.WriteString(f.LikelyCause)
		b.WriteByte('\n')
		b.WriteString(f.NextStep)
		b.WriteByte('\n')
	}
	for _, n := range rep.Notes {
		b.WriteString(n)
		b.WriteByte('\n')
	}
	return b.String()
}

func containsNote(rep Report, sub string) bool {
	for _, n := range rep.Notes {
		if strings.Contains(n, sub) {
			return true
		}
	}
	return false
}
