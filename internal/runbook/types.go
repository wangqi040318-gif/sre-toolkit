// Package runbook builds a rollback checklist for a Deployment or StatefulSet.
package runbook

// Kinds supported by the runbook.
const (
	KindDeployment  = "Deployment"
	KindStatefulSet = "StatefulSet"
)

// Step categories, in checklist order.
const (
	StepState    = "State"
	StepPreCheck = "PreCheck"
	StepRollback = "Rollback"
	StepVerify   = "Verify"
	StepAbort    = "Abort"
)

const (
	SevCritical = "critical"
	SevWarning  = "warning"
	SevInfo     = "info"
)

// ContainerImage is one container in a pod template.
type ContainerImage struct {
	Name  string
	Image string
	Init  bool
}

// Revision is one recorded rollout of the workload.
type Revision struct {
	Number int64
	Images []ContainerImage
}

// Snapshot is the live or demo state the checklist is filled from.
type Snapshot struct {
	Cluster         string
	Kind            string
	Namespace       string
	Name            string
	Selector        string
	CurrentRevision int64
	Desired         int32
	Ready           int32
	Updated         int32
	Images          []ContainerImage
	Revisions       []Revision
	Failure         string
	Notes           []string
}

// Options selects the undo target. Revision 0 means the revision just below current.
type Options struct {
	Revision int64
	Demo     bool
}

// Detail is one stable key/value.
type Detail struct {
	Name  string
	Value string
}

// Step is one checklist item.
type Step struct {
	Severity    string
	Category    string
	Resource    string
	Phenomenon  string
	LikelyCause string
	NextStep    string
	Details     []Detail
}

// Report is the runbook, independent of stdout format.
type Report struct {
	Demo            bool
	Cluster         string
	Kind            string
	Namespace       string
	Name            string
	CurrentRevision int64
	TargetRevision  int64
	CurrentImage    string
	TargetImage     string
	Desired         int32
	Ready           int32
	Updated         int32
	Steps           []Step
	Notes           []string
}
