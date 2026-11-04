// Package waste reports idle and oversized Kubernetes objects for cost hygiene.
package waste

import "time"

// Severities understood by the shared renderer.
const (
	SevCritical = "critical"
	SevWarning  = "warning"
	SevInfo     = "info"
)

// Categories are the waste signals.
const (
	CatIdle         = "Idle"
	CatOversized    = "Oversized"
	CatHighRequest  = "HighRequest"
	CatLoadBalancer = "IdleLoadBalancer"
	CatOrphanedPVC  = "OrphanedPVC"
	CatUnboundPVC   = "UnboundPVC"
	CatUnusedConfig = "UnusedConfig"
	CatUnusedSecret = "UnusedSecret"
)

const (
	KindDeployment  = "Deployment"
	KindStatefulSet = "StatefulSet"
	KindReplicaSet  = "ReplicaSet"
	KindConfigMap   = "ConfigMap"
	KindSecret      = "Secret"
)

// Floors keep the scan from flagging tiny sandboxes as cost problems.
const (
	minCPUMilli       int64 = 500
	minMemBytes       int64 = 512 * 1024 * 1024
	highCPUMilli      int64 = 2000
	highMemBytes      int64 = 4 * 1024 * 1024 * 1024
	criticalPVCBytes  int64 = 100 * 1024 * 1024 * 1024
	maxConfigFindings       = 15
)

// Workload is one Deployment, StatefulSet, or bare ReplicaSet.
// Requests are per pod. Usage is the sum of metrics samples for its pods.
type Workload struct {
	Kind              string
	Namespace         string
	Name              string
	Desired           int32
	Ready             int32
	Created           time.Time
	CPURequestMilli   int64
	MemRequestBytes   int64
	CPULimitMilli     int64
	MemLimitBytes     int64
	HasCPULimit       bool
	HasMemLimit       bool
	CPUUsageMilli     int64
	MemUsageBytes     int64
	HasUsage          bool
	OwnedByDeployment bool
}

// Service is a Service plus how many ready endpoints it has.
type Service struct {
	Namespace      string
	Name           string
	Type           string
	ReadyEndpoints int
	Address        string
}

// Claim is a PersistentVolumeClaim and whether a running Pod mounts it.
type Claim struct {
	Namespace    string
	Name         string
	Phase        string
	StorageBytes int64
	Mounted      bool
}

// ConfigObject is a ConfigMap or Secret that may be referenced by a Pod.
type ConfigObject struct {
	Kind       string
	Namespace  string
	Name       string
	Created    time.Time
	Used       bool
	SecretType string
}

// Snapshot is the point-in-time input to Analyze.
type Snapshot struct {
	Cluster          string
	MetricsAvailable bool
	MetricsError     string
	Notes            []string
	Workloads        []Workload
	Services         []Service
	Claims           []Claim
	Configs          []ConfigObject
}

// Options controls idle age and the usage ratio.
// Zero IdleFor and UsageRatio pick 24h and 0.20.
type Options struct {
	Namespace     string
	AllNamespaces bool
	Now           time.Time
	Demo          bool
	Cluster       string
	IdleFor       time.Duration
	UsageRatio    float64
	IncludeConfig bool
}

// Detail is one stable key/value on a finding.
type Detail struct {
	Name  string
	Value string
}

// Finding is one waste signal.
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

// Summary counts what the scan judged.
type Summary struct {
	Workloads     int
	Idle          int
	Oversized     int
	HighRequests  int
	LoadBalancers int
	OrphanedPVCs  int
	UnboundPVCs   int
	UnusedConfig  int
	UnusedSecrets int
}

// Report is the analyzed scan, independent of stdout format.
type Report struct {
	GeneratedAt time.Time
	Demo        bool
	Cluster     string
	Scope       string
	IdleFor     time.Duration
	UsageRatio  float64
	Summary     Summary
	Findings    []Finding
	Notes       []string
}

func (o Options) withDefaults() Options {
	if o.IdleFor <= 0 {
		o.IdleFor = 24 * time.Hour
	}
	if o.UsageRatio <= 0 {
		o.UsageRatio = 0.20
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
