package output

import (
	"fmt"
	"io"
	"strings"
	"time"
)

const (
	ansiReset  = "\033[0m"
	ansiBold   = "\033[1m"
	ansiRed    = "\033[31m"
	ansiYellow = "\033[33m"
	ansiCyan   = "\033[36m"
	ansiGreen  = "\033[32m"
	ansiDim    = "\033[2m"
)

// WriteHuman prints a scannable colored summary. color wraps severity and headers.
func WriteHuman(w io.Writer, doc Document, color bool) error {
	doc = doc.normalized()
	b := &bufw{w: w}
	b.f("%s\n", paint(color, ansiBold, doc.Title))
	if !doc.GeneratedAt.IsZero() {
		b.f("%s\n", paint(color, ansiDim, "generated "+doc.GeneratedAt.UTC().Format(time.RFC3339)))
	}
	if len(doc.Meta) > 0 {
		b.f("\n")
		width := nameWidth(fieldsToNames(doc.Meta), 0)
		for _, f := range doc.Meta {
			b.f("  %-*s  %s\n", width, f.Name, f.Value)
		}
	}
	if len(doc.Summary) > 0 {
		b.f("\n%s\n", paint(color, ansiBold, "Summary"))
		width := nameWidth(counterNames(doc.Summary), 0)
		for _, c := range doc.Summary {
			b.f("  %-*s  %d\n", width, c.Name, c.Count)
		}
	}
	if len(doc.Summary) > 0 || len(doc.Findings) > 0 {
		b.f("\n%s\n\n", paint(color, ansiBold, fmt.Sprintf("Findings (%d)", len(doc.Findings))))
		if len(doc.Findings) == 0 {
			b.f("  No findings in this scope.\n")
		}
		for i, f := range doc.Findings {
			if i > 0 {
				b.f("\n")
			}
			label := fmt.Sprintf("%-8s", strings.ToUpper(f.Severity))
			b.f("%s  %s  %s\n", paint(color, severityColor(f.Severity), label), f.Category, f.Resource)
			b.f("  %-12s  %s\n", "Phenomenon", f.Phenomenon)
			b.f("  %-12s  %s\n", "Likely cause", f.LikelyCause)
			b.f("  %-12s  %s\n", "Next step", f.NextStep)
			if line := detailLine(f.Details); line != "" {
				b.f("  %-12s  %s\n", "Details", line)
			}
		}
	}
	if len(doc.Notes) > 0 {
		b.f("\n%s\n", paint(color, ansiBold, "Notes"))
		for _, n := range doc.Notes {
			b.f("  %s\n", n)
		}
	}
	return b.err
}

func detailLine(details []Field) string {
	if len(details) == 0 {
		return ""
	}
	parts := make([]string, 0, len(details))
	for _, d := range details {
		if d.Name == "" && d.Value == "" {
			continue
		}
		parts = append(parts, d.Name+"="+d.Value)
	}
	return strings.Join(parts, "  ")
}

func fieldsToNames(fields []Field) []string {
	out := make([]string, len(fields))
	for i, f := range fields {
		out[i] = f.Name
	}
	return out
}

func counterNames(counters []Counter) []string {
	out := make([]string, len(counters))
	for i, c := range counters {
		out[i] = c.Name
	}
	return out
}

func nameWidth(names []string, min int) int {
	w := min
	for _, n := range names {
		if len(n) > w {
			w = len(n)
		}
	}
	return w
}

func severityColor(severity string) string {
	switch severity {
	case SeverityCritical:
		return ansiBold + ansiRed
	case SeverityWarning:
		return ansiBold + ansiYellow
	case SeverityInfo:
		return ansiBold + ansiCyan
	default:
		return ansiGreen
	}
}

func paint(enabled bool, code, text string) string {
	if !enabled || code == "" {
		return text
	}
	return code + text + ansiReset
}

type bufw struct {
	w   io.Writer
	err error
}

func (b *bufw) f(format string, args ...any) {
	if b.err != nil {
		return
	}
	_, b.err = fmt.Fprintf(b.w, format, args...)
}
