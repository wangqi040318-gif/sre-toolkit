package slo

import (
	"fmt"
	"strings"
	"time"
)

const (
	SevCritical = "critical"
	SevWarning  = "warning"
	SevInfo     = "info"
)

// Detail is a stable key/value.
type Detail struct {
	Name  string
	Value string
}

// Finding is one line of the SLO brief.
type Finding struct {
	Severity    string
	Category    string
	Resource    string
	Phenomenon  string
	LikelyCause string
	NextStep    string
	Details     []Detail
}

// Report is the error-budget brief.
type Report struct {
	Demo      bool
	Service   string
	Target    float64
	Budget    float64
	Remaining float64
	HasBudget bool
	Decision  Decision
	Burns     []Burn
	Findings  []Finding
	Notes     []string
}

// Assemble turns burn windows into a release decision.
func Assemble(service string, target float64, burns []Burn, demo bool) Report {
	for i := range burns {
		burns[i] = Compute(target, burns[i])
	}
	decision := Decide(target, burns)
	long, hasLong := Longest(burns)
	remaining := 0.0
	if hasLong {
		remaining = Remaining(target, long.ErrorRatio)
	}
	ref := "service " + service
	var findings []Finding
	if hasLong {
		findings = append(findings, budgetFinding(ref, service, target, long, remaining))
	}
	for _, b := range burns {
		findings = append(findings, burnFinding(ref, service, target, b))
	}
	findings = append(findings, decisionFinding(ref, service, decision))

	notes := []string{
		"Burn rate is the window's error ratio divided by (1 - SLO). A burn rate of 1x spends the whole budget over the SLO period.",
		"Page when the fast window (about 1h) is at or above 14.4x, or the long-window budget is exhausted. Freeze when a mid window is at or above 6x or a multi-day window is at or above 1x.",
		"Default SLI is increase() of http_requests_total with code=~\"5..\" over http_requests_total, selected by job=\"<service>\". Override with --errors or --selector.",
	}
	if demo {
		notes = append([]string{"Demo mode is a burning checkout budget. It does not call Prometheus."}, notes...)
	}
	for _, b := range burns {
		if b.Err != "" {
			notes = append(notes, b.Window.Label+" query failed: "+b.Err)
		} else if b.Missing {
			notes = append(notes, b.Window.Label+" returned no series.")
		}
	}
	return Report{
		Demo:      demo,
		Service:   service,
		Target:    target,
		Budget:    Budget(target),
		Remaining: remaining,
		HasBudget: hasLong,
		Decision:  decision,
		Burns:     burns,
		Findings:  findings,
		Notes:     notes,
	}
}

func budgetFinding(ref, service string, target float64, long Burn, remaining float64) Finding {
	sev := SevInfo
	switch {
	case remaining <= 0:
		sev = SevCritical
	case remaining < 0.2:
		sev = SevWarning
	}
	return Finding{
		Severity: sev,
		Category: "Budget",
		Resource: ref,
		Phenomenon: fmt.Sprintf("%s success ratio is %s against a %s SLO. Error budget remaining on the %s window is %s.",
			strings.ToUpper(service[:1])+service[1:], pct(long.SuccessRatio), pct(target), long.Window.Label, pct(remaining)),
		LikelyCause: fmt.Sprintf("The target allows %s failed requests. This window observed %s.", pct(Budget(target)), pct(long.ErrorRatio)),
		NextStep:    long.Query,
		Details: []Detail{
			{Name: "window", Value: long.Window.Label},
			{Name: "successRatio", Value: pct(long.SuccessRatio)},
			{Name: "errorRatio", Value: pct(long.ErrorRatio)},
			{Name: "remaining", Value: pct(remaining)},
		},
	}
}

func burnFinding(ref, service string, target float64, b Burn) Finding {
	if b.Missing || b.Err != "" {
		why := "The query returned no series."
		if b.Err != "" {
			why = b.Err
		}
		return Finding{
			Severity:    SevWarning,
			Category:    "Burn",
			Resource:    ref,
			Phenomenon:  fmt.Sprintf("%s burn rate is unavailable.", b.Window.Label),
			LikelyCause: why,
			NextStep:    b.Query,
			Details:     []Detail{{Name: "window", Value: b.Window.Label}},
		}
	}
	sev := SevInfo
	switch {
	case b.Window.Duration <= 2*time.Hour && b.BurnRate >= 14.4:
		sev = SevCritical
	case b.Window.Duration <= 12*time.Hour && b.BurnRate >= 6:
		sev = SevWarning
	case b.Window.Duration >= 24*time.Hour && b.BurnRate >= 1:
		sev = SevWarning
	case b.BurnRate >= 1:
		sev = SevWarning
	}
	return Finding{
		Severity: sev,
		Category: "Burn",
		Resource: ref,
		Phenomenon: fmt.Sprintf("%s error ratio is %s (success %s), burn rate %s.",
			b.Window.Label, pct(b.ErrorRatio), pct(b.SuccessRatio), burn(b.BurnRate)),
		LikelyCause: fmt.Sprintf("The %s SLO allows %s errors. This window is consuming that budget at %s.", pct(target), pct(Budget(target)), burn(b.BurnRate)),
		NextStep:    b.Query,
		Details: []Detail{
			{Name: "window", Value: b.Window.Label},
			{Name: "burnRate", Value: burn(b.BurnRate)},
			{Name: "errorRatio", Value: pct(b.ErrorRatio)},
		},
	}
}

func decisionFinding(ref, service string, d Decision) Finding {
	sev := SevInfo
	action := "Ship"
	switch d.Action {
	case ActionPage:
		sev = SevCritical
		action = "Page and freeze"
	case ActionFreeze:
		sev = SevWarning
		action = "Freeze releases"
	case ActionWatch:
		sev = SevWarning
		action = "Watch"
	case ActionUnknown:
		sev = SevWarning
		action = "No decision"
	}
	return Finding{
		Severity:    sev,
		Category:    "Recommendation",
		Resource:    ref,
		Phenomenon:  action + ". " + d.Why,
		LikelyCause: "Multi-window burn rate is the release gate. A fast burn pages; a slow burn still freezes non-urgent deploys.",
		NextStep:    fmt.Sprintf("sre triage %s --window 1h; kubectl rollout history deployment/%s", service, service),
	}
}

func pct(v float64) string {
	return fmt.Sprintf("%.2f%%", v*100)
}

func burn(v float64) string {
	return fmt.Sprintf("%.1fx", v)
}
