// Package daily builds a morning health digest across kubeconfig contexts.
package daily

// Cluster is one context's overnight counts.
type Cluster struct {
	Context       string
	NodesTotal    int
	NodesNotReady int
	CrashLoop     int
	OOMKilled     int
	FailedJobs    int
	WarningEvents int
	CertsDue      int
	CertsWarn     int
	Err           string
	Note          string
}

// Healthy is a cluster with no availability or certificate-due signals.
// Warning events and certificates inside the 30-day warn window are not enough
// to call the cluster healthy.
func (c Cluster) Healthy() bool {
	return c.Err == "" && c.NodesNotReady == 0 && c.CrashLoop == 0 && c.OOMKilled == 0 && c.FailedJobs == 0 && c.CertsDue == 0 && c.WarningEvents == 0 && c.CertsWarn == 0
}

// Attention is true when a human should look before the day starts.
func (c Cluster) Attention() bool {
	return !c.Healthy()
}

// Detail is a stable key/value.
type Detail struct {
	Name  string
	Value string
}

// Finding is one digest line.
type Finding struct {
	Severity    string
	Category    string
	Resource    string
	Phenomenon  string
	LikelyCause string
	NextStep    string
	Details     []Detail
}

// Report is the morning digest.
type Report struct {
	Demo     bool
	Clusters []Cluster
	Findings []Finding
	Notes    []string
}
