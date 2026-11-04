// Package output renders a command result as colored text, JSON, or Markdown.
package output

import (
	"errors"
	"io"
	"os"
	"time"
)

// Severity labels understood by the human renderer.
const (
	SeverityCritical = "critical"
	SeverityWarning  = "warning"
	SeverityInfo     = "info"
)

// Options selects how a Document is presented.
type Options struct {
	JSON     bool
	Markdown bool
	NoColor  bool
	Report   string
}

// Validate rejects combinations that would mix two stdout formats.
func (o Options) Validate() error {
	if o.JSON && o.Markdown {
		return errors.New("use either --json or --markdown")
	}
	return nil
}

// Field is a labeled string used for metadata and finding details.
type Field struct {
	Name  string `json:"name"`
	Value string `json:"value"`
}

// Counter is one numeric line in the summary block.
type Counter struct {
	Name  string `json:"name"`
	Count int    `json:"count"`
}

// Finding is one phenomenon, the likely cause, and the next step.
type Finding struct {
	Severity    string  `json:"severity"`
	Category    string  `json:"category"`
	Resource    string  `json:"resource"`
	Phenomenon  string  `json:"phenomenon"`
	LikelyCause string  `json:"likelyCause"`
	NextStep    string  `json:"nextStep"`
	Details     []Field `json:"details"`
}

// Document is the shared envelope for every sre command.
type Document struct {
	Title       string    `json:"title"`
	Command     string    `json:"command"`
	GeneratedAt time.Time `json:"generatedAt"`
	Demo        bool      `json:"demo"`
	Meta        []Field   `json:"meta"`
	Summary     []Counter `json:"summary"`
	Findings    []Finding `json:"findings"`
	Notes       []string  `json:"notes"`
}

// Render writes doc to stdout in the selected format and, when Report is set,
// also writes a Markdown file. A short confirmation goes to stderr.
func Render(stdout, stderr io.Writer, opts Options, doc Document) error {
	if err := opts.Validate(); err != nil {
		return err
	}
	doc = doc.normalized()
	if opts.Report != "" {
		if err := WriteMarkdownFile(opts.Report, doc); err != nil {
			return err
		}
	}
	var err error
	switch {
	case opts.JSON:
		err = WriteJSON(stdout, doc)
	case opts.Markdown:
		err = WriteMarkdown(stdout, doc)
	default:
		err = WriteHuman(stdout, doc, UseColor(stdout, opts.NoColor))
	}
	if err != nil {
		return err
	}
	if opts.Report != "" && stderr != nil {
		if _, err := io.WriteString(stderr, "wrote markdown report to "+opts.Report+"\n"); err != nil {
			return err
		}
	}
	return nil
}

func (d Document) normalized() Document {
	if d.Meta == nil {
		d.Meta = []Field{}
	}
	if d.Summary == nil {
		d.Summary = []Counter{}
	}
	if d.Notes == nil {
		d.Notes = []string{}
	}
	if d.Findings == nil {
		d.Findings = []Finding{}
		return d
	}
	cp := make([]Finding, len(d.Findings))
	copy(cp, d.Findings)
	for i := range cp {
		if cp[i].Details == nil {
			cp[i].Details = []Field{}
		}
	}
	d.Findings = cp
	return d
}

// UseColor reports whether ANSI color should be applied to w.
func UseColor(w io.Writer, noColor bool) bool {
	return colorOn(noColor, isCharDevice(w))
}

func colorOn(noColor bool, isTTY bool) bool {
	if noColor {
		return false
	}
	if _, ok := os.LookupEnv("NO_COLOR"); ok {
		return false
	}
	return isTTY
}

func isCharDevice(w io.Writer) bool {
	f, ok := w.(*os.File)
	if !ok {
		return false
	}
	st, err := f.Stat()
	if err != nil {
		return false
	}
	return st.Mode()&os.ModeCharDevice != 0
}
