package cli

import (
	"strconv"
	"time"

	"github.com/wangqi040318-gif/sre-toolkit/internal/certs"
	"github.com/wangqi040318-gif/sre-toolkit/internal/diffreview"
	"github.com/wangqi040318-gif/sre-toolkit/internal/output"
	"github.com/wangqi040318-gif/sre-toolkit/internal/pulse"
	"github.com/wangqi040318-gif/sre-toolkit/internal/runbook"
	"github.com/wangqi040318-gif/sre-toolkit/internal/triage"
	"github.com/wangqi040318-gif/sre-toolkit/internal/waste"
)

func pulseDocument(rep pulse.Report, mode string) output.Document {
	meta := []output.Field{
		{Name: "Cluster", Value: rep.Cluster},
		{Name: "Scope", Value: rep.Scope},
		{Name: "Mode", Value: mode},
		{Name: "Window", Value: shortWindow(rep.Since)},
		{Name: "Restart threshold", Value: strconv.FormatInt(int64(rep.RestartThreshold), 10)},
	}
	summary := []output.Counter{
		{Name: "Nodes", Count: rep.Summary.NodesTotal},
		{Name: "Nodes not ready", Count: rep.Summary.NodesNotReady},
		{Name: "Pods scanned", Count: rep.Summary.PodsScanned},
		{Name: "CrashLoopBackOff", Count: rep.Summary.CrashLoop},
		{Name: "OOMKilled", Count: rep.Summary.OOMKilled},
		{Name: "High restarts", Count: rep.Summary.HighRestarts},
		{Name: "Warning events", Count: rep.Summary.WarningEvents},
	}
	findings := make([]output.Finding, 0, len(rep.Findings))
	for _, f := range rep.Findings {
		details := make([]output.Field, 0, len(f.Details))
		for _, d := range f.Details {
			details = append(details, output.Field{Name: d.Name, Value: d.Value})
		}
		findings = append(findings, output.Finding{
			Severity:    f.Severity,
			Category:    f.Category,
			Resource:    f.Resource,
			Phenomenon:  f.Phenomenon,
			LikelyCause: f.LikelyCause,
			NextStep:    f.NextStep,
			Details:     details,
		})
	}
	notes := append([]string(nil), rep.Notes...)
	return output.Document{
		Title:       "Kubernetes pulse",
		Command:     "k8s-pulse",
		GeneratedAt: rep.GeneratedAt,
		Demo:        rep.Demo,
		Meta:        meta,
		Summary:     summary,
		Findings:    findings,
		Notes:       notes,
	}
}

func certsDocument(rep certs.Report, mode string) output.Document {
	meta := []output.Field{
		{Name: "Cluster", Value: rep.Cluster},
		{Name: "Scope", Value: rep.Scope},
		{Name: "Mode", Value: mode},
		{Name: "Warn days", Value: strconv.Itoa(rep.WarnDays)},
		{Name: "Critical days", Value: strconv.Itoa(rep.CriticalDays)},
	}
	summary := []output.Counter{
		{Name: "Certificates", Count: rep.Summary.Certificates},
		{Name: "Expired", Count: rep.Summary.Expired},
		{Name: "Critical", Count: rep.Summary.Critical},
		{Name: "Warning", Count: rep.Summary.Warning},
		{Name: "OK", Count: rep.Summary.OK},
		{Name: "Missing secrets", Count: rep.Summary.Missing},
		{Name: "Unreadable", Count: rep.Summary.Unreadable},
	}
	findings := make([]output.Finding, 0, len(rep.Findings))
	for _, f := range rep.Findings {
		details := make([]output.Field, 0, len(f.Details))
		for _, d := range f.Details {
			details = append(details, output.Field{Name: d.Name, Value: d.Value})
		}
		findings = append(findings, output.Finding{
			Severity:    f.Severity,
			Category:    f.Category,
			Resource:    f.Resource,
			Phenomenon:  f.Phenomenon,
			LikelyCause: f.LikelyCause,
			NextStep:    f.NextStep,
			Details:     details,
		})
	}
	return output.Document{
		Title:       "Certificate expiry",
		Command:     "certs",
		GeneratedAt: rep.GeneratedAt,
		Demo:        rep.Demo,
		Meta:        meta,
		Summary:     summary,
		Findings:    findings,
		Notes:       append([]string(nil), rep.Notes...),
	}
}

func wasteDocument(rep waste.Report, mode string) output.Document {
	meta := []output.Field{
		{Name: "Cluster", Value: rep.Cluster},
		{Name: "Scope", Value: rep.Scope},
		{Name: "Mode", Value: mode},
		{Name: "Idle for", Value: prettyDuration(rep.IdleFor)},
		{Name: "Usage ratio", Value: strconv.FormatFloat(rep.UsageRatio, 'f', 2, 64)},
	}
	summary := []output.Counter{
		{Name: "Workloads", Count: rep.Summary.Workloads},
		{Name: "Idle", Count: rep.Summary.Idle},
		{Name: "Oversized", Count: rep.Summary.Oversized},
		{Name: "High requests", Count: rep.Summary.HighRequests},
		{Name: "Load balancers", Count: rep.Summary.LoadBalancers},
		{Name: "Orphaned PVCs", Count: rep.Summary.OrphanedPVCs},
		{Name: "Unbound PVCs", Count: rep.Summary.UnboundPVCs},
		{Name: "Unused config", Count: rep.Summary.UnusedConfig},
		{Name: "Unused secrets", Count: rep.Summary.UnusedSecrets},
	}
	findings := make([]output.Finding, 0, len(rep.Findings))
	for _, f := range rep.Findings {
		details := make([]output.Field, 0, len(f.Details))
		for _, d := range f.Details {
			details = append(details, output.Field{Name: d.Name, Value: d.Value})
		}
		findings = append(findings, output.Finding{
			Severity:    f.Severity,
			Category:    f.Category,
			Resource:    f.Resource,
			Phenomenon:  f.Phenomenon,
			LikelyCause: f.LikelyCause,
			NextStep:    f.NextStep,
			Details:     details,
		})
	}
	return output.Document{
		Title:       "Resource waste",
		Command:     "waste",
		GeneratedAt: rep.GeneratedAt,
		Demo:        rep.Demo,
		Meta:        meta,
		Summary:     summary,
		Findings:    findings,
		Notes:       append([]string(nil), rep.Notes...),
	}
}

func diffDocument(rep diffreview.Report) output.Document {
	mode := "files"
	if rep.Demo {
		mode = "demo"
	}
	meta := []output.Field{
		{Name: "Before", Value: rep.Before},
		{Name: "After", Value: rep.After},
		{Name: "Mode", Value: mode},
	}
	summary := []output.Counter{
		{Name: "Objects before", Count: rep.Summary.ObjectsBefore},
		{Name: "Objects after", Count: rep.Summary.ObjectsAfter},
		{Name: "Critical", Count: rep.Summary.Critical},
		{Name: "High", Count: rep.Summary.High},
		{Name: "Medium", Count: rep.Summary.Medium},
		{Name: "Low", Count: rep.Summary.Low},
	}
	findings := make([]output.Finding, 0, len(rep.Findings))
	for _, f := range rep.Findings {
		details := make([]output.Field, 0, len(f.Details))
		for _, d := range f.Details {
			details = append(details, output.Field{Name: d.Name, Value: d.Value})
		}
		findings = append(findings, output.Finding{
			Severity:    f.Severity,
			Category:    f.Category,
			Resource:    f.Resource,
			Phenomenon:  f.Phenomenon,
			LikelyCause: f.LikelyCause,
			NextStep:    f.NextStep,
			Details:     details,
		})
	}
	return output.Document{
		Title:       "Manifest diff review",
		Command:     "diff-review",
		GeneratedAt: time.Now().UTC(),
		Demo:        rep.Demo,
		Meta:        meta,
		Summary:     summary,
		Findings:    findings,
		Notes:       append([]string(nil), rep.Notes...),
	}
}

func runbookDocument(rep runbook.Report, mode string) output.Document {
	meta := []output.Field{
		{Name: "Cluster", Value: rep.Cluster},
		{Name: "Namespace", Value: rep.Namespace},
		{Name: "Workload", Value: rep.Kind + " " + rep.Namespace + "/" + rep.Name},
		{Name: "Mode", Value: mode},
		{Name: "Current revision", Value: strconv.FormatInt(rep.CurrentRevision, 10)},
		{Name: "Current image", Value: rep.CurrentImage},
		{Name: "Target revision", Value: strconv.FormatInt(rep.TargetRevision, 10)},
		{Name: "Target image", Value: rep.TargetImage},
	}
	summary := []output.Counter{
		{Name: "Desired", Count: int(rep.Desired)},
		{Name: "Ready", Count: int(rep.Ready)},
		{Name: "Updated", Count: int(rep.Updated)},
		{Name: "Steps", Count: len(rep.Steps)},
	}
	findings := make([]output.Finding, 0, len(rep.Steps))
	for _, s := range rep.Steps {
		details := make([]output.Field, 0, len(s.Details))
		for _, d := range s.Details {
			details = append(details, output.Field{Name: d.Name, Value: d.Value})
		}
		findings = append(findings, output.Finding{
			Severity:    s.Severity,
			Category:    s.Category,
			Resource:    s.Resource,
			Phenomenon:  s.Phenomenon,
			LikelyCause: s.LikelyCause,
			NextStep:    s.NextStep,
			Details:     details,
		})
	}
	return output.Document{
		Title:       "Rollback runbook",
		Command:     "runbook",
		GeneratedAt: time.Now().UTC(),
		Demo:        rep.Demo,
		Meta:        meta,
		Summary:     summary,
		Findings:    findings,
		Notes:       append([]string(nil), rep.Notes...),
	}
}

func triageDocument(rep triage.Report, mode string) output.Document {
	meta := []output.Field{
		{Name: "Service", Value: rep.Service},
		{Name: "Window", Value: rep.Start.Format(time.RFC3339) + " to " + rep.End.Format(time.RFC3339)},
		{Name: "Mode", Value: mode},
		{Name: "Request rate", Value: metricText(rep.Summary.HasRequests, formatRPS(rep.Summary.RequestRate))},
		{Name: "5xx ratio", Value: metricText(rep.Summary.HasErrors, formatPct(rep.Summary.ErrorRatio))},
		{Name: "p95", Value: metricText(rep.Summary.HasP95, formatSecs(rep.Summary.P95Seconds))},
		{Name: "p99", Value: metricText(rep.Summary.HasP99, formatSecs(rep.Summary.P99Seconds))},
	}
	var critical, warning, info int
	findings := make([]output.Finding, 0, len(rep.Findings))
	for _, f := range rep.Findings {
		switch f.Severity {
		case "critical":
			critical++
		case "warning":
			warning++
		default:
			info++
		}
		details := make([]output.Field, 0, len(f.Details))
		for _, d := range f.Details {
			details = append(details, output.Field{Name: d.Name, Value: d.Value})
		}
		findings = append(findings, output.Finding{
			Severity:    f.Severity,
			Category:    f.Category,
			Resource:    f.Resource,
			Phenomenon:  f.Phenomenon,
			LikelyCause: f.LikelyCause,
			NextStep:    f.NextStep,
			Details:     details,
		})
	}
	return output.Document{
		Title:       "Triage brief",
		Command:     "triage",
		GeneratedAt: rep.End,
		Demo:        rep.Demo,
		Meta:        meta,
		Summary: []output.Counter{
			{Name: "Critical", Count: critical},
			{Name: "Warning", Count: warning},
			{Name: "Info", Count: info},
		},
		Findings: findings,
		Notes:    append([]string(nil), rep.Notes...),
	}
}

func metricText(ok bool, v string) string {
	if !ok {
		return "missing"
	}
	return v
}

func formatRPS(v float64) string { return strconv.FormatFloat(v, 'f', 1, 64) + " req/s" }
func formatPct(v float64) string { return strconv.FormatFloat(v*100, 'f', 1, 64) + "%" }
func formatSecs(v float64) string {
	if v < 1 {
		return strconv.FormatFloat(v*1000, 'f', 0, 64) + "ms"
	}
	return strconv.FormatFloat(v, 'f', 2, 64) + "s"
}

func prettyDuration(d time.Duration) string {
	if d > 0 && d%time.Hour == 0 {
		return strconv.FormatInt(int64(d/time.Hour), 10) + "h"
	}
	if d > 0 && d%time.Minute == 0 {
		return strconv.FormatInt(int64(d/time.Minute), 10) + "m"
	}
	return d.String()
}

func shortWindow(d time.Duration) string {
	if d <= 0 {
		return "0s"
	}
	if d%time.Hour == 0 && d >= time.Hour {
		return strconv.FormatInt(int64(d/time.Hour), 10) + "h"
	}
	if d%time.Minute == 0 && d >= time.Minute {
		return strconv.FormatInt(int64(d/time.Minute), 10) + "m"
	}
	return d.Truncate(time.Second).String()
}
