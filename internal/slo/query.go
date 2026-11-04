package slo

import (
	"fmt"
	"strings"
)

// Default error-ratio PromQL. {window} is the Prometheus duration (1h, 6h, 3d).
// {selector} defaults to job="{service}". The same counter family as sre triage.
const (
	DefaultSelector = `job="{service}"`
	DefaultErrors   = `sum(increase(http_requests_total{{selector},code=~"5.."}[{window}])) / sum(increase(http_requests_total{{selector}}[{window}]))`
)

// Templates are PromQL with {service}, {selector}, and {window}.
type Templates struct {
	Selector string
	Errors   string
}

// Render builds one query per window.
func (t Templates) Render(service string, windows []Window) ([]string, error) {
	if !validService(service) {
		return nil, fmt.Errorf("service name %q must match [A-Za-z0-9_.:-] and be at most 128 characters", service)
	}
	sel := strings.TrimSpace(t.Selector)
	if sel == "" {
		sel = DefaultSelector
	}
	sel = strings.ReplaceAll(sel, "{service}", service)
	base := strings.TrimSpace(t.Errors)
	if base == "" {
		base = DefaultErrors
	}
	out := make([]string, len(windows))
	for i, w := range windows {
		q := strings.ReplaceAll(base, "{selector}", sel)
		q = strings.ReplaceAll(q, "{window}", w.Label)
		q = strings.ReplaceAll(q, "{service}", service)
		out[i] = q
	}
	return out, nil
}

func validService(s string) bool {
	if s == "" || len(s) > 128 {
		return false
	}
	for _, r := range s {
		switch {
		case r >= 'a' && r <= 'z':
		case r >= 'A' && r <= 'Z':
		case r >= '0' && r <= '9':
		case r == '_' || r == '.' || r == ':' || r == '-':
		default:
			return false
		}
	}
	return true
}
