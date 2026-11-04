// Package slo computes availability error budget and multi-window burn rate.
package slo

import (
	"fmt"
	"strings"
	"time"
)

// Recommendation is the release decision for the current burn.
const (
	ActionPage    = "page"
	ActionFreeze  = "freeze"
	ActionWatch   = "watch"
	ActionShip    = "ship"
	ActionUnknown = "unknown"
)

// Window is one burn-rate lookback.
type Window struct {
	Label    string
	Duration time.Duration
}

// Burn is the SLI math for one window.
type Burn struct {
	Window       Window
	ErrorRatio   float64
	SuccessRatio float64
	BurnRate     float64
	Query        string
	Missing      bool
	Err          string
}

// Decision is whether to page, freeze releases, watch, or ship.
type Decision struct {
	Action string
	Why    string
}

// ParseSLO accepts a percentage (99.9) or a ratio (0.999).
func ParseSLO(v float64) (float64, error) {
	switch {
	case v > 1 && v < 100:
		return v / 100, nil
	case v > 0 && v <= 1:
		return v, nil
	default:
		return 0, fmt.Errorf("--slo must be a percentage such as 99.9 or a ratio such as 0.999")
	}
}

// ParseWindows accepts "1h,6h,3d" or "1h/6h/3d". At least two windows are required.
// Day units are supported because Prometheus accepts them and Go's time.ParseDuration does not.
func ParseWindows(spec string) ([]Window, error) {
	spec = strings.TrimSpace(spec)
	if spec == "" {
		spec = "1h,6h,3d"
	}
	parts := strings.Split(spec, ",")
	if len(parts) == 1 && strings.Contains(spec, "/") {
		parts = strings.Split(spec, "/")
	}
	var out []Window
	seen := map[string]struct{}{}
	for _, p := range parts {
		p = strings.TrimSpace(p)
		if p == "" {
			continue
		}
		d, err := parseDuration(p)
		if err != nil {
			return nil, fmt.Errorf("window %q: %w", p, err)
		}
		if d < time.Minute || d > 90*24*time.Hour {
			return nil, fmt.Errorf("window %s must be between 1m and 90d", p)
		}
		if _, ok := seen[p]; ok {
			continue
		}
		seen[p] = struct{}{}
		out = append(out, Window{Label: p, Duration: d})
	}
	if len(out) < 2 {
		return nil, fmt.Errorf("provide at least two windows, for example 1h,6h,3d")
	}
	return out, nil
}

func parseDuration(s string) (time.Duration, error) {
	if strings.HasSuffix(s, "d") {
		n := strings.TrimSuffix(s, "d")
		var days int
		if _, err := fmt.Sscan(n, &days); err != nil || days <= 0 {
			return 0, fmt.Errorf("invalid day duration")
		}
		return time.Duration(days) * 24 * time.Hour, nil
	}
	d, err := time.ParseDuration(s)
	if err != nil || d <= 0 {
		return 0, fmt.Errorf("invalid duration")
	}
	return d, nil
}

// Budget is 1 - SLO. A 99.9% target has a 0.1% error budget.
func Budget(slo float64) float64 {
	return 1 - slo
}

// Compute fills success ratio and burn rate. Burn rate is error ratio divided by the budget.
// A burn rate of 1 consumes the entire budget exactly over the SLO period.
func Compute(slo float64, b Burn) Burn {
	if b.Missing || b.Err != "" {
		return b
	}
	b.SuccessRatio = 1 - b.ErrorRatio
	budget := Budget(slo)
	if budget <= 0 {
		b.BurnRate = 0
		return b
	}
	b.BurnRate = b.ErrorRatio / budget
	return b
}

// Remaining is the fraction of error budget left on this window.
// 1 means untouched, 0 means exhausted, negative means overspent.
func Remaining(slo float64, errorRatio float64) float64 {
	budget := Budget(slo)
	if budget <= 0 {
		return 0
	}
	return 1 - (errorRatio / budget)
}

// Longest returns the longest window that has data. ok is false when every window is missing.
func Longest(burns []Burn) (Burn, bool) {
	var best Burn
	ok := false
	for _, b := range burns {
		if b.Missing || b.Err != "" {
			continue
		}
		if !ok || b.Window.Duration > best.Window.Duration {
			best = b
			ok = true
		}
	}
	return best, ok
}

// Decide applies a multi-window policy modeled on the SRE workbook:
// a fast 1h burn at or above 14.4x pages, a 6h burn at or above 6x freezes
// releases, and a multi-day burn at or above 1x also freezes. An exhausted
// long-window budget pages as well.
func Decide(slo float64, burns []Burn) Decision {
	long, ok := Longest(burns)
	if !ok {
		return Decision{Action: ActionUnknown, Why: "No window returned an error ratio. Check the Prometheus URL, the job label, and the metric names before making a release decision."}
	}
	remaining := Remaining(slo, long.ErrorRatio)
	var fast, mid, slow *Burn
	for i := range burns {
		b := &burns[i]
		if b.Missing || b.Err != "" {
			continue
		}
		switch {
		case b.Window.Duration <= 2*time.Hour && (fast == nil || b.BurnRate > fast.BurnRate):
			fast = b
		case b.Window.Duration <= 12*time.Hour && (mid == nil || b.BurnRate > mid.BurnRate):
			mid = b
		case b.Window.Duration >= 24*time.Hour && (slow == nil || b.BurnRate > slow.BurnRate):
			slow = b
		}
	}
	switch {
	case remaining <= 0 || (fast != nil && fast.BurnRate >= 14.4):
		why := fmt.Sprintf("The %s budget is %.1f%% remaining.", long.Window.Label, remaining*100)
		if fast != nil && fast.BurnRate >= 14.4 {
			why = fmt.Sprintf("The %s burn rate is %.1fx, at or above the 14.4x page threshold. %s", fast.Window.Label, fast.BurnRate, why)
		} else {
			why = "The long-window error budget is exhausted. " + why
		}
		return Decision{Action: ActionPage, Why: why + " Freeze releases and page the service owner."}
	case (mid != nil && mid.BurnRate >= 6) || (slow != nil && slow.BurnRate >= 1) || remaining < 0.2:
		return Decision{Action: ActionFreeze, Why: fmt.Sprintf("Burn is above the ticket threshold or the %s budget is under 20%% (%.1f%% left). Freeze non-urgent releases until the long window is back under 1x.", long.Window.Label, remaining*100)}
	case remaining < 0.5 || anyBurn(burns, 1):
		return Decision{Action: ActionWatch, Why: fmt.Sprintf("The %s budget is %.1f%% remaining and at least one window is burning at 1x or more. Releases need an owner watching this SLO.", long.Window.Label, remaining*100)}
	default:
		return Decision{Action: ActionShip, Why: fmt.Sprintf("The %s error budget is %.1f%% remaining and no window is burning at 1x. This SLO does not require a release freeze.", long.Window.Label, remaining*100)}
	}
}

func anyBurn(burns []Burn, min float64) bool {
	for _, b := range burns {
		if !b.Missing && b.Err == "" && b.BurnRate >= min {
			return true
		}
	}
	return false
}
