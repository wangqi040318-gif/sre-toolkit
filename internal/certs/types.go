// Package certs reports certificate expiry for Ingress TLS, TLS secrets, and local PEM files.
package certs

import "time"

// Severity tiers. Expired and critical both render as critical in the shared
// summary; the category carries the finer tier.
const (
	TierExpired  = "expired"
	TierCritical = "critical"
	TierWarn     = "warn"
	TierOK       = "ok"

	SevCritical = "critical"
	SevWarning  = "warning"
	SevOK       = "ok"

	CatExpired    = "Expired"
	CatDueSoon    = "DueSoon"
	CatExpiring   = "Expiring"
	CatHealthy    = "Healthy"
	CatMissing    = "Missing"
	CatUnreadable = "Unreadable"

	SourceSecret  = "secret"
	SourceIngress = "ingress"
	SourceFile    = "file"
)

// Material is one PEM blob or one Ingress that points at a secret we could not read.
type Material struct {
	Source      string
	Namespace   string
	Name        string
	SecretName  string
	Resource    string
	Hosts       []string
	Ingresses   []string
	Annotations map[string]string
	PEM         []byte
	Missing     bool
}

// Options controls the clock and the warn / critical windows.
// Zero WarnDays and CriticalDays pick the defaults (30 and 7).
type Options struct {
	Namespace     string
	AllNamespaces bool
	WarnDays      int
	CriticalDays  int
	Now           time.Time
	Demo          bool
	Cluster       string
	// FilesScoped is true when local PEM paths were included.
	FilesScoped bool
}

// Detail is one stable key/value on a finding.
type Detail struct {
	Name  string
	Value string
}

// Finding is one certificate, missing secret, or unreadable PEM.
type Finding struct {
	Severity    string
	Category    string
	Resource    string
	Phenomenon  string
	LikelyCause string
	NextStep    string
	Details     []Detail
	days        int
	seq         int
}

// Summary counts what the scan saw.
type Summary struct {
	Certificates int
	Expired      int
	Critical     int
	Warning      int
	OK           int
	Missing      int
	Unreadable   int
	Secrets      int
	Ingresses    int
	Files        int
}

// Report is the analyzed scan, independent of stdout format.
type Report struct {
	GeneratedAt  time.Time
	Demo         bool
	Cluster      string
	Scope        string
	WarnDays     int
	CriticalDays int
	Summary      Summary
	Findings     []Finding
	Notes        []string
}

func (o Options) withDefaults() Options {
	// Both zero means "use the documented defaults". An explicit
	// critical window of 0 is kept when WarnDays is set.
	if o.WarnDays == 0 && o.CriticalDays == 0 {
		o.WarnDays = 30
		o.CriticalDays = 7
	}
	if o.Now.IsZero() {
		o.Now = time.Now()
	}
	return o
}

// ScopeLabel describes the namespace filter.
func (o Options) ScopeLabel() string {
	var scope string
	if o.AllNamespaces || o.Namespace == "" {
		scope = "all namespaces"
	} else {
		scope = "namespace " + o.Namespace
	}
	if o.FilesScoped {
		scope += " + local files"
	}
	return scope
}

func (o Options) includeNS(ns string) bool {
	if o.AllNamespaces || o.Namespace == "" {
		return true
	}
	return ns == o.Namespace
}
