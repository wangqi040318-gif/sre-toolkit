package triage

import (
	"fmt"
	"math"
	"strings"
	"time"
)

const (
	errorWarn = 0.01
	errorCrit = 0.05
	p95Warn   = 0.3
	p95Crit   = 1.0
	p99Crit   = 2.0
)

// Signal is one RED series after a query.
type Signal struct {
	Name    string
	Query   string
	Series  Series
	Missing bool
	Err     string
}

// Bundle is the four RED signals for one window.
type Bundle struct {
	Service  string
	Start    time.Time
	End      time.Time
	Requests Signal
	Errors   Signal
	P95      Signal
	P99      Signal
	Demo     bool
}

// Detail is a stable key/value.
type Detail struct {
	Name  string
	Value string
}

// Finding is one line of the brief.
type Finding struct {
	Severity    string
	Category    string
	Resource    string
	Phenomenon  string
	LikelyCause string
	NextStep    string
	Details     []Detail
}

// Summary is the headline numbers, using -1 when a signal is missing.
type Summary struct {
	RequestRate float64
	ErrorRatio  float64
	P95Seconds  float64
	P99Seconds  float64
	HasRequests bool
	HasErrors   bool
	HasP95      bool
	HasP99      bool
}

// Report is the triage brief.
type Report struct {
	Demo     bool
	Service  string
	Start    time.Time
	End      time.Time
	Summary  Summary
	Findings []Finding
	Notes    []string
	Queries  Queries
}

type stats struct {
	first, last, max float64
	ok               bool
}

func summarize(s Series) stats {
	var st stats
	for _, p := range s.Points {
		if math.IsNaN(p.Value) || math.IsInf(p.Value, 0) {
			continue
		}
		if !st.ok {
			st.first, st.last, st.max = p.Value, p.Value, p.Value
			st.ok = true
			continue
		}
		st.last = p.Value
		if p.Value > st.max {
			st.max = p.Value
		}
	}
	return st
}

// Assemble turns queried series into an on-call brief.
func Assemble(b Bundle, q Queries) Report {
	req := summarize(b.Requests.Series)
	errSt := summarize(b.Errors.Series)
	p95 := summarize(b.P95.Series)
	p99 := summarize(b.P99.Series)
	ref := "service " + b.Service

	var findings []Finding
	findings = append(findings, trafficFinding(ref, b.Requests, req))
	findings = append(findings, errorFinding(ref, b.Errors, errSt))
	findings = append(findings, latencyFinding(ref, b.P95, b.P99, p95, p99))
	findings = append(findings, hypothesisFinding(ref, b, req, errSt, p95, p99))
	findings = append(findings, checksFinding(ref, b.Service, q))

	notes := []string{"Logs are not queried. Use the metric brief, then a log tool for the same service and window if the cause is still unclear."}
	if b.Demo {
		notes = append([]string{"Demo mode uses a synthetic error and latency spike. It does not call Prometheus."}, notes...)
	}
	notes = append(notes, "Default metrics are http_requests_total (label code=~\"5..\" for errors) and http_request_duration_seconds_bucket, selected by job=\"<service>\". Override with --requests, --errors, --p95, --p99, or --selector.")
	for _, sig := range []Signal{b.Requests, b.Errors, b.P95, b.P99} {
		if sig.Err != "" {
			notes = append(notes, sig.Name+" query failed: "+sig.Err)
		} else if sig.Missing {
			notes = append(notes, sig.Name+" returned no series.")
		}
	}

	return Report{
		Demo:    b.Demo,
		Service: b.Service,
		Start:   b.Start.UTC(),
		End:     b.End.UTC(),
		Summary: Summary{
			RequestRate: req.last,
			ErrorRatio:  errSt.last,
			P95Seconds:  p95.last,
			P99Seconds:  p99.last,
			HasRequests: req.ok,
			HasErrors:   errSt.ok,
			HasP95:      p95.ok,
			HasP99:      p99.ok,
		},
		Findings: findings,
		Notes:    notes,
		Queries:  q,
	}
}

func trafficFinding(ref string, sig Signal, st stats) Finding {
	if !st.ok {
		return missingFinding(ref, "RequestRate", sig)
	}
	sev := "info"
	why := "Traffic is context for the error ratio. A drop with rising errors often means callers are backing off or the process is too slow to finish work."
	if st.first > 1 && st.last < st.first*0.5 {
		sev = "warning"
		why = "Request rate fell by half or more. Callers may be timing out, or the process stopped accepting work."
	}
	return Finding{
		Severity:    sev,
		Category:    "RequestRate",
		Resource:    ref,
		Phenomenon:  fmt.Sprintf("Request rate is %s now (start %s, peak %s).", rps(st.last), rps(st.first), rps(st.max)),
		LikelyCause: why,
		NextStep:    sig.Query,
		Details:     statDetails("req/s", st),
	}
}

func errorFinding(ref string, sig Signal, st stats) Finding {
	if !st.ok {
		return missingFinding(ref, "ErrorRate", sig)
	}
	sev := "info"
	why := "The 5xx ratio stays under 1%. Keep it as the baseline for this window."
	switch {
	case st.last >= errorCrit || st.max >= errorCrit:
		sev = "critical"
		why = "The 5xx ratio crossed 5%. A bad release or a dependency returning errors is the first hypothesis."
	case st.last >= errorWarn || st.max >= errorWarn:
		sev = "warning"
		why = "The 5xx ratio crossed 1%. It is worth a look before it becomes an incident."
	}
	return Finding{
		Severity:    sev,
		Category:    "ErrorRate",
		Resource:    ref,
		Phenomenon:  fmt.Sprintf("5xx ratio is %s now (start %s, peak %s).", pct(st.last), pct(st.first), pct(st.max)),
		LikelyCause: why,
		NextStep:    sig.Query,
		Details:     statDetails("ratio", st),
	}
}

func latencyFinding(ref string, p95Sig, p99Sig Signal, p95, p99 stats) Finding {
	if !p95.ok && !p99.ok {
		f := missingFinding(ref, "Latency", p95Sig)
		if p99Sig.Err != "" && p95Sig.Err == "" {
			f.NextStep = p99Sig.Query
		}
		return f
	}
	sev := "info"
	why := "The latency tail is inside the brief's thresholds (p95 under 300ms and p99 under 2s)."
	p95bad := p95.ok && (p95.last >= p95Crit || (p95.first > 0 && p95.last >= p95.first*2 && p95.last >= p95Warn))
	p99bad := p99.ok && p99.last >= p99Crit
	switch {
	case (p95.ok && p95.last >= p95Crit) || p99bad:
		sev = "critical"
		why = "The latency tail is at least 1s (p95) or 2s (p99). Timeouts and 5xx often follow a tail this slow."
	case p95bad || (p95.ok && p95.last >= p95Warn):
		sev = "warning"
		why = "p95 is elevated against the start of the window or the 300ms mark."
	}
	phenomenon := "Latency"
	if p95.ok {
		phenomenon = fmt.Sprintf("p95 is %s now (start %s, peak %s).", secs(p95.last), secs(p95.first), secs(p95.max))
	}
	if p99.ok {
		phenomenon += fmt.Sprintf(" p99 is %s now (peak %s).", secs(p99.last), secs(p99.max))
	}
	next := p95Sig.Query
	if next == "" {
		next = p99Sig.Query
	}
	return Finding{
		Severity:    sev,
		Category:    "Latency",
		Resource:    ref,
		Phenomenon:  strings.TrimSpace(phenomenon),
		LikelyCause: why,
		NextStep:    next,
		Details: []Detail{
			{Name: "p95", Value: secsOrMissing(p95)},
			{Name: "p99", Value: secsOrMissing(p99)},
		},
	}
}

func hypothesisFinding(ref string, b Bundle, req, errSt, p95, p99 stats) Finding {
	errBad := errSt.ok && (errSt.last >= errorCrit || errSt.max >= errorCrit)
	errWarn := errSt.ok && (errSt.last >= errorWarn || errSt.max >= errorWarn)
	latBad := (p95.ok && p95.last >= p95Crit) || (p99.ok && p99.last >= p99Crit)
	latWarn := p95.ok && (p95.last >= p95Warn || (p95.first > 0 && p95.last >= p95.first*2))
	drop := req.ok && req.first > 1 && req.last < req.first*0.5
	none := !errSt.ok && !p95.ok && !p99.ok && !req.ok

	sev := "info"
	var lines []string
	switch {
	case none:
		sev = "warning"
		lines = append(lines, "Prometheus returned no series for the RED queries. The job label or metric names likely do not match this service.")
	case errBad && latBad:
		sev = "critical"
		lines = append(lines, "Errors and latency rose together. That fits a bad rollout or a slow dependency more than a pure traffic flood.")
	case errBad && drop:
		sev = "critical"
		lines = append(lines, "5xx is high and request rate fell. Callers are likely failing or giving up.")
	case errBad:
		sev = "critical"
		lines = append(lines, "5xx is the leading signal. Start with the newest pods and the last deploy.")
	case latBad:
		sev = "critical"
		lines = append(lines, "The tail is slow without a matching 5xx breach. Look for saturation, a locked dependency, or a partial outage.")
	case errWarn || latWarn:
		sev = "warning"
		lines = append(lines, "The service is outside the quiet band but not yet a hard page. Confirm whether the move is new in this window.")
	default:
		lines = append(lines, "No error or latency threshold was breached in this window.")
	}
	if b.Demo && (errBad || latBad) {
		lines = append(lines, "In the demo, the spike is the last 15 minutes: 5xx climbs from about 0.2% to 14%, and p95 from about 80ms to 1.4s, while request rate eases from 120 to about 90 req/s.")
	}
	lines = append(lines,
		"Hypotheses to walk in order: a bad release, a dependency returning errors or timing out, then resource saturation (CPU, pool, or memory).",
	)
	return Finding{
		Severity:    sev,
		Category:    "Hypothesis",
		Resource:    ref,
		Phenomenon:  lines[0],
		LikelyCause: strings.Join(lines[1:], " "),
		NextStep:    checks(b.Service),
	}
}

func checksFinding(ref, service string, q Queries) Finding {
	return Finding{
		Severity:    "info",
		Category:    "Checks",
		Resource:    ref,
		Phenomenon:  "Next checks: pods and the latest rollout, then the PromQL that produced this brief.",
		LikelyCause: "The metric brief names the symptom. Pods and rollout history tell you if a release matches the spike. Logs are a later step.",
		NextStep:    checks(service) + "; error query: " + q.Errors,
	}
}

func checks(service string) string {
	return fmt.Sprintf("kubectl get pods -A -l app=%s; kubectl rollout history deployment/%s; sre k8s-pulse -A; sre runbook --name %s", service, service, service)
}

func missingFinding(ref, category string, sig Signal) Finding {
	why := "The query returned no series. Check the job label and the metric name documented for this command."
	if sig.Err != "" {
		why = "The query failed: " + sig.Err
	}
	return Finding{
		Severity:    "warning",
		Category:    category,
		Resource:    ref,
		Phenomenon:  category + " is unavailable for this window.",
		LikelyCause: why,
		NextStep:    sig.Query,
	}
}

func statDetails(unit string, st stats) []Detail {
	return []Detail{
		{Name: "unit", Value: unit},
		{Name: "start", Value: fmt.Sprintf("%.6g", st.first)},
		{Name: "last", Value: fmt.Sprintf("%.6g", st.last)},
		{Name: "peak", Value: fmt.Sprintf("%.6g", st.max)},
	}
}

func secsOrMissing(st stats) string {
	if !st.ok {
		return "missing"
	}
	return secs(st.last)
}

func pct(r float64) string {
	return fmt.Sprintf("%.1f%%", r*100)
}

func rps(v float64) string {
	return fmt.Sprintf("%.1f req/s", v)
}

func secs(s float64) string {
	if s < 1 {
		return fmt.Sprintf("%.0fms", s*1000)
	}
	return fmt.Sprintf("%.2fs", s)
}
