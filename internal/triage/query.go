package triage

import (
	"fmt"
	"strings"
	"time"
)

// Default PromQL for common RED metrics. {service} is the service or job
// name. {selector} defaults to job="{service}". Override any query with a flag.
const (
	DefaultSelector = `job="{service}"`
	DefaultRequests = `sum(rate(http_requests_total{{selector}}[5m]))`
	DefaultErrors   = `sum(rate(http_requests_total{{selector},code=~"5.."}[5m])) / sum(rate(http_requests_total{{selector}}[5m]))`
	DefaultP95      = `histogram_quantile(0.95, sum by (le) (rate(http_request_duration_seconds_bucket{{selector}}[5m])))`
	DefaultP99      = `histogram_quantile(0.99, sum by (le) (rate(http_request_duration_seconds_bucket{{selector}}[5m])))`
)

// Templates are PromQL strings. Empty fields use the defaults.
type Templates struct {
	Selector string
	Requests string
	Errors   string
	P95      string
	P99      string
}

// Queries are rendered PromQL for one service.
type Queries struct {
	Requests string
	Errors   string
	P95      string
	P99      string
}

// Render substitutes {service} and {selector}. Service names that would break
// a PromQL label matcher are rejected.
func (t Templates) Render(service string) (Queries, error) {
	if !validService(service) {
		return Queries{}, fmt.Errorf("service name %q must match [A-Za-z0-9_.:-] and be at most 128 characters", service)
	}
	sel := t.Selector
	if strings.TrimSpace(sel) == "" {
		sel = DefaultSelector
	}
	sel = strings.ReplaceAll(sel, "{service}", service)
	apply := func(q, fallback string) string {
		if strings.TrimSpace(q) == "" {
			q = fallback
		}
		q = strings.ReplaceAll(q, "{selector}", sel)
		q = strings.ReplaceAll(q, "{service}", service)
		return q
	}
	return Queries{
		Requests: apply(t.Requests, DefaultRequests),
		Errors:   apply(t.Errors, DefaultErrors),
		P95:      apply(t.P95, DefaultP95),
		P99:      apply(t.P99, DefaultP99),
	}, nil
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

// Step picks a range-query step that stays readable for the window.
func Step(window time.Duration) time.Duration {
	step := window / 30
	if step < 15*time.Second {
		step = 15 * time.Second
	}
	if step > 5*time.Minute {
		step = 5 * time.Minute
	}
	return step
}
