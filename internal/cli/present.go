package cli

import (
	"strconv"
	"time"

	"github.com/wangqi040318-gif/sre-toolkit/internal/certs"
	"github.com/wangqi040318-gif/sre-toolkit/internal/diffreview"
	"github.com/wangqi040318-gif/sre-toolkit/internal/output"
	"github.com/wangqi040318-gif/sre-toolkit/internal/pulse"
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
