// Package diffreview compares two Kubernetes manifest sets and flags release risks.
package diffreview

// Risk levels. They are also the severity strings so the summary prints
// CRITICAL, HIGH, MEDIUM, and LOW.
const (
	RiskCritical = "critical"
	RiskHigh     = "high"
	RiskMedium   = "medium"
	RiskLow      = "low"
)

// Field classes shown on each finding.
const (
	ClassImage         = "Image"
	ClassResources     = "Resources"
	ClassProbe         = "Probe"
	ClassReplicas      = "Replicas"
	ClassHPA           = "HPA"
	ClassVolume        = "Volume"
	ClassService       = "Service"
	ClassNetworkPolicy = "NetworkPolicy"
	ClassRBAC          = "RBAC"
	ClassSecurity      = "SecurityContext"
	ClassEnv           = "Env"
	ClassAdded         = "Added"
	ClassRemoved       = "Removed"
)

// Object is one Kubernetes document.
type Object struct {
	APIVersion string
	Kind       string
	Namespace  string
	Name       string
	Raw        map[string]any
}

// Key is stable across files. Namespace is empty for cluster-scoped objects.
func (o Object) Key() string {
	return o.APIVersion + "|" + o.Kind + "|" + o.Namespace + "|" + o.Name
}

// Ref is the human label kind ns/name.
func (o Object) Ref() string {
	if o.Namespace == "" {
		return o.Kind + " " + o.Name
	}
	return o.Kind + " " + o.Namespace + "/" + o.Name
}

// Field is one comparable value extracted from an object.
type Field struct {
	Path  string
	Value string
	Class string
}

// Finding is one risky change.
type Finding struct {
	Severity    string
	Category    string
	Resource    string
	Phenomenon  string
	LikelyCause string
	NextStep    string
	Details     []Detail
}

// Detail is a stable key/value for JSON and Markdown.
type Detail struct {
	Name  string
	Value string
}

// Summary counts objects and risk levels.
type Summary struct {
	ObjectsBefore int
	ObjectsAfter  int
	Critical      int
	High          int
	Medium        int
	Low           int
}

// Report is the review, independent of stdout format.
type Report struct {
	Demo     bool
	Before   string
	After    string
	Summary  Summary
	Findings []Finding
	Notes    []string
}
