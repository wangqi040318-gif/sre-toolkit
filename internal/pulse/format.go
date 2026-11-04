package pulse

import (
	"fmt"
	"regexp"
	"strconv"
	"strings"
	"time"
	"unicode/utf8"

	corev1 "k8s.io/api/core/v1"
)

var safeName = regexp.MustCompile(`^[A-Za-z0-9][A-Za-z0-9_.-]*$`)

func oneLine(s string, max int) string {
	s = strings.Join(strings.Fields(s), " ")
	if s == "" || max <= 0 {
		return s
	}
	if utf8.RuneCountInString(s) <= max {
		return s
	}
	r := []rune(s)
	if max <= 3 {
		return string(r[:max])
	}
	return string(r[:max-3]) + "..."
}

func age(now, t time.Time) string {
	if t.IsZero() {
		return "unknown time"
	}
	d := now.Sub(t)
	if d < 0 {
		d = 0
	}
	d = d.Round(time.Second)
	switch {
	case d < time.Minute:
		return fmt.Sprintf("%ds ago", int(d.Seconds()))
	case d < time.Hour:
		return fmt.Sprintf("%dm ago", int(d.Minutes()))
	case d < 48*time.Hour:
		return fmt.Sprintf("%dh ago", int(d.Hours()))
	default:
		return fmt.Sprintf("%dd ago", int(d.Hours()/24))
	}
}

func shortDur(d time.Duration) string {
	if d <= 0 {
		return "0s"
	}
	if d%time.Hour == 0 && d >= time.Hour {
		return fmt.Sprintf("%dh", int(d/time.Hour))
	}
	if d%time.Minute == 0 && d >= time.Minute {
		return fmt.Sprintf("%dm", int(d/time.Minute))
	}
	return d.Truncate(time.Second).String()
}

func resID(ns, name string) string {
	switch {
	case ns == "" && name == "":
		return ""
	case ns == "":
		return name
	case name == "":
		return ns
	default:
		return ns + "/" + name
	}
}

func shellName(s string) string {
	if safeName.MatchString(s) {
		return s
	}
	return strconv.Quote(s)
}

func eventTime(e corev1.Event) time.Time {
	switch {
	case !e.LastTimestamp.IsZero():
		return e.LastTimestamp.Time
	case e.Series != nil && !e.Series.LastObservedTime.IsZero():
		return e.Series.LastObservedTime.Time
	case !e.EventTime.IsZero():
		return e.EventTime.Time
	case !e.FirstTimestamp.IsZero():
		return e.FirstTimestamp.Time
	default:
		return e.CreationTimestamp.Time
	}
}

func eventCount(e corev1.Event) int32 {
	n := e.Count
	if e.Series != nil && e.Series.Count > n {
		n = e.Series.Count
	}
	if n <= 0 {
		return 1
	}
	return n
}

func eventInWindow(now, ts time.Time, since time.Duration) bool {
	if ts.IsZero() {
		return false
	}
	if ts.After(now.Add(time.Minute)) {
		return false
	}
	return !ts.Before(now.Add(-since))
}

func addDetail(details []Detail, name, value string) []Detail {
	if value == "" {
		return details
	}
	return append(details, Detail{Name: name, Value: value})
}
