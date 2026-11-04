package cli

import (
	"strconv"
	"time"

	"github.com/wangqi040318-gif/sre-toolkit/internal/output"
	"github.com/wangqi040318-gif/sre-toolkit/internal/pulse"
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
