// Package pulse builds a health pulse from nodes, pods, and warning events.
package pulse

import (
	"time"

	corev1 "k8s.io/api/core/v1"
)

// Categories reported by the pulse.
const (
	CatCrashLoop    = "CrashLoopBackOff"
	CatOOM          = "OOMKilled"
	CatHighRestarts = "HighRestarts"
	CatNodeNotReady = "NodeNotReady"
	CatWarning      = "WarningEvent"
)

// Severities ordered for display.
const (
	SevCritical = "critical"
	SevWarning  = "warning"
	SevInfo     = "info"
)

// Snapshot is a point-in-time copy of the objects the pulse inspects.
type Snapshot struct {
	Cluster string
	Nodes   []corev1.Node
	Pods    []corev1.Pod
	Events  []corev1.Event
}

// Options controls thresholds and scope. Zero values pick the defaults
// used by `sre k8s-pulse` (threshold 5, window 1h, 20 events).
type Options struct {
	Namespace        string
	AllNamespaces    bool
	RestartThreshold int32
	Since            time.Duration
	EventLimit       int
	Now              time.Time
	Demo             bool
}

// Detail is one stable key/value attached to a finding.
type Detail struct {
	Name  string
	Value string
}

// Finding is a single operational signal.
type Finding struct {
	Severity    string
	Category    string
	Resource    string
	Phenomenon  string
	LikelyCause string
	NextStep    string
	Details     []Detail
	seq         int
}

// Summary counts what the pulse scanned and what it flagged.
type Summary struct {
	NodesTotal         int
	NodesNotReady      int
	PodsScanned        int
	CrashLoop          int
	OOMKilled          int
	HighRestarts       int
	WarningEvents      int
	WarningEventsShown int
}

// Report is the analyzed pulse, independent of stdout format.
type Report struct {
	GeneratedAt      time.Time
	Demo             bool
	Cluster          string
	Scope            string
	Since            time.Duration
	RestartThreshold int32
	Summary          Summary
	Findings         []Finding
	Notes            []string
}

func (o Options) withDefaults() Options {
	if o.RestartThreshold <= 0 {
		o.RestartThreshold = 5
	}
	if o.Since <= 0 {
		o.Since = time.Hour
	}
	if o.EventLimit <= 0 {
		o.EventLimit = 20
	}
	if o.Now.IsZero() {
		o.Now = time.Now()
	}
	return o
}

// ScopeLabel describes the namespace filter.
func (o Options) ScopeLabel() string {
	if o.AllNamespaces || o.Namespace == "" {
		return "all namespaces"
	}
	return "namespace " + o.Namespace
}

func (o Options) includeNS(ns string) bool {
	if o.AllNamespaces || o.Namespace == "" {
		return true
	}
	return ns == o.Namespace
}
