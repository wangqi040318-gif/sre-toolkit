package waste

import (
	"strings"
	"testing"
	"time"

	"k8s.io/apimachinery/pkg/api/resource"
)

func TestDemoPath(t *testing.T) {
	now := time.Date(2026, 11, 4, 12, 0, 0, 0, time.UTC)
	rep := Analyze(DemoSnapshot(now), Options{
		AllNamespaces: true, Now: now, Demo: true, Cluster: "demo-cluster", IncludeConfig: true,
	})
	if rep.Summary.Workloads != 4 || rep.Summary.Idle != 2 || rep.Summary.Oversized != 1 || rep.Summary.LoadBalancers != 1 || rep.Summary.OrphanedPVCs != 1 || rep.Summary.UnboundPVCs != 1 || rep.Summary.UnusedConfig != 1 || rep.Summary.UnusedSecrets != 1 {
		t.Fatalf("summary = %+v", rep.Summary)
	}
	if len(rep.Findings) != 8 {
		t.Fatalf("findings = %d", len(rep.Findings))
	}
	if rep.Findings[0].Category != CatOversized || rep.Findings[0].Severity != SevCritical {
		t.Fatalf("first = %+v", rep.Findings[0])
	}
	if rep.Findings[1].Category != CatOrphanedPVC || rep.Findings[1].Severity != SevCritical {
		t.Fatalf("second = %+v", rep.Findings[1])
	}
	blob := render(rep)
	for _, want := range []string{
		"observability/prometheus",
		"150m CPU in use of 4 requested",
		"data/archive",
		"200Gi",
		"desires 0 replicas",
		"shop/recommendations",
		"storefront",
		"203.0.113.10",
		"feature-flags",
		"old-api-key",
		"Rightsize requests",
		"Delete Service",
	} {
		if !strings.Contains(blob, want) {
			t.Fatalf("missing %q\n%s", want, blob)
		}
	}
	for _, absent := range []string{"deploy platform/edge", "default-token", "kube-root-ca", "edge-config", "recommendations-7d4c"} {
		if strings.Contains(blob, absent) {
			t.Fatalf("should not flag %q\n%s", absent, blob)
		}
	}
	if !strings.Contains(rep.Notes[0], "synthetic") {
		t.Fatalf("notes = %v", rep.Notes)
	}

	shop := Analyze(DemoSnapshot(now), Options{Namespace: "shop", Now: now, Demo: true, IncludeConfig: true})
	if shop.Summary.Idle != 1 || shop.Summary.Oversized != 0 || shop.Summary.OrphanedPVCs != 0 || shop.Summary.LoadBalancers != 1 || shop.Summary.UnboundPVCs != 1 {
		t.Fatalf("shop summary = %+v", shop.Summary)
	}
	if strings.Contains(render(shop), "data/batch") || strings.Contains(render(shop), "prometheus") {
		t.Fatalf("namespace leak\n%s", render(shop))
	}
}

func TestClassifyIdleAndSize(t *testing.T) {
	now := time.Date(2026, 11, 4, 12, 0, 0, 0, time.UTC)
	cpu := func(s string) int64 {
		q := resource.MustParse(s)
		return q.MilliValue()
	}
	mem := func(s string) int64 {
		q := resource.MustParse(s)
		return q.Value()
	}
	base := Options{AllNamespaces: true, Now: now, IdleFor: 24 * time.Hour, UsageRatio: 0.2}

	young := Analyze(Snapshot{Workloads: []Workload{{
		Kind: KindDeployment, Namespace: "ns", Name: "new",
		Desired: 2, Ready: 0, Created: now.Add(-time.Hour),
	}}}, base)
	if young.Summary.Idle != 0 || len(young.Findings) != 0 {
		t.Fatalf("young unready should be quiet: %+v", young.Findings)
	}

	zero := Analyze(Snapshot{Workloads: []Workload{{
		Kind: KindDeployment, Namespace: "ns", Name: "parked",
		Desired: 0, Ready: 0, Created: now.Add(-time.Hour),
	}}}, base)
	if zero.Summary.Idle != 1 {
		t.Fatalf("scaled to zero: %+v", zero.Summary)
	}

	bare := Analyze(Snapshot{Workloads: []Workload{{
		Kind: KindReplicaSet, Namespace: "ns", Name: "owned",
		Desired: 0, OwnedByDeployment: true, Created: now.Add(-48 * time.Hour),
	}}}, base)
	if bare.Summary.Workloads != 0 || len(bare.Findings) != 0 {
		t.Fatalf("owned rs leaked: %+v", bare.Findings)
	}

	warn := Analyze(Snapshot{MetricsAvailable: true, Workloads: []Workload{{
		Kind: KindDeployment, Namespace: "ns", Name: "roomy",
		Desired: 1, Ready: 1, Created: now.Add(-48 * time.Hour),
		CPURequestMilli: cpu("4"), CPUUsageMilli: cpu("600m"), HasUsage: true,
		MemRequestBytes: mem("128Mi"), MemUsageBytes: mem("100Mi"),
	}}}, base)
	if warn.Summary.Oversized != 1 || warn.Findings[0].Severity != SevWarning {
		t.Fatalf("15%% cpu should warn: %+v", warn.Findings)
	}

	exact := Analyze(Snapshot{MetricsAvailable: true, Workloads: []Workload{{
		Kind: KindDeployment, Namespace: "ns", Name: "fit",
		Desired: 1, Ready: 1, Created: now.Add(-48 * time.Hour),
		CPURequestMilli: cpu("4"), CPUUsageMilli: cpu("800m"), HasUsage: true,
	}}}, base)
	if exact.Summary.Oversized != 0 {
		t.Fatalf("exactly 20%% should not flag: %+v", exact.Findings)
	}

	tiny := Analyze(Snapshot{MetricsAvailable: true, Workloads: []Workload{{
		Kind: KindDeployment, Namespace: "ns", Name: "tiny",
		Desired: 1, Ready: 1, Created: now.Add(-48 * time.Hour),
		CPURequestMilli: cpu("100m"), CPUUsageMilli: cpu("1m"), HasUsage: true,
	}}}, base)
	if len(tiny.Findings) != 0 {
		t.Fatalf("tiny request should not flag: %+v", tiny.Findings)
	}

	high := Analyze(Snapshot{Workloads: []Workload{{
		Kind: KindDeployment, Namespace: "ns", Name: "big",
		Desired: 2, Ready: 2, Created: now.Add(-48 * time.Hour),
		CPURequestMilli: cpu("2"), MemRequestBytes: mem("1Gi"),
		HasCPULimit: true, CPULimitMilli: cpu("2"),
	}}}, base)
	if high.Summary.HighRequests != 1 || !strings.Contains(high.Findings[0].Phenomenon, "No usage sample") {
		t.Fatalf("high request = %+v", high.Findings)
	}
	if !strings.Contains(high.Findings[0].LikelyCause, "Guaranteed") {
		t.Fatalf("qos hint missing: %s", high.Findings[0].LikelyCause)
	}

	withMetricsNoSample := Analyze(Snapshot{MetricsAvailable: true, Workloads: []Workload{{
		Kind: KindDeployment, Namespace: "ns", Name: "big",
		Desired: 2, Ready: 2, Created: now.Add(-48 * time.Hour),
		CPURequestMilli: cpu("4"),
	}}}, base)
	if len(withMetricsNoSample.Findings) != 0 {
		t.Fatalf("metrics up but no sample should not use the fallback: %+v", withMetricsNoSample.Findings)
	}
}

func TestClassifyServicesPVCsAndConfig(t *testing.T) {
	now := time.Date(2026, 11, 4, 12, 0, 0, 0, time.UTC)
	mem := func(s string) int64 {
		q := resource.MustParse(s)
		return q.Value()
	}
	opts := Options{AllNamespaces: true, Now: now, IncludeConfig: true, IdleFor: 24 * time.Hour}
	rep := Analyze(Snapshot{
		Services: []Service{
			{Namespace: "ns", Name: "lb", Type: "LoadBalancer", Address: "198.51.100.8"},
			{Namespace: "ns", Name: "ok", Type: "LoadBalancer", ReadyEndpoints: 2, Address: "198.51.100.9"},
			{Namespace: "ns", Name: "cip", Type: "ClusterIP"},
		},
		Claims: []Claim{
			{Namespace: "ns", Name: "big", Phase: "Bound", StorageBytes: mem("200Gi")},
			{Namespace: "ns", Name: "small", Phase: "Bound", StorageBytes: mem("1Gi")},
			{Namespace: "ns", Name: "pending", Phase: "Pending", StorageBytes: mem("5Gi")},
			{Namespace: "ns", Name: "used", Phase: "Bound", StorageBytes: mem("500Gi"), Mounted: true},
		},
		Configs: []ConfigObject{
			{Kind: KindConfigMap, Namespace: "ns", Name: "old", Created: now.Add(-48 * time.Hour)},
			{Kind: KindConfigMap, Namespace: "ns", Name: "fresh", Created: now.Add(-time.Hour)},
			{Kind: KindConfigMap, Namespace: "ns", Name: "used", Created: now.Add(-48 * time.Hour), Used: true},
			{Kind: KindSecret, Namespace: "ns", Name: "token", Created: now.Add(-48 * time.Hour), SecretType: "kubernetes.io/service-account-token"},
			{Kind: KindSecret, Namespace: "ns", Name: "helm", Created: now.Add(-48 * time.Hour), SecretType: "helm.sh/release.v1"},
			{Kind: KindConfigMap, Namespace: "ns", Name: "kube-root-ca.crt", Created: now.Add(-48 * time.Hour)},
		},
	}, opts)
	if rep.Summary.LoadBalancers != 1 || rep.Summary.OrphanedPVCs != 2 || rep.Summary.UnboundPVCs != 1 || rep.Summary.UnusedConfig != 1 {
		t.Fatalf("summary = %+v", rep.Summary)
	}
	var sawBigCritical bool
	for _, f := range rep.Findings {
		if f.Resource == "pvc ns/big" && f.Severity == SevCritical {
			sawBigCritical = true
		}
		if f.Resource == "pvc ns/used" || strings.Contains(f.Resource, "token") || strings.Contains(f.Resource, "kube-root-ca") {
			t.Fatalf("unexpected %s", f.Resource)
		}
	}
	if !sawBigCritical {
		t.Fatalf("findings = %+v", rep.Findings)
	}
	off := opts
	off.IncludeConfig = false
	quiet := Analyze(Snapshot{Configs: []ConfigObject{{
		Kind: KindConfigMap, Namespace: "ns", Name: "old", Created: now.Add(-48 * time.Hour),
	}}}, off)
	if len(quiet.Findings) != 0 {
		t.Fatalf("include-config off still flagged %+v", quiet.Findings)
	}
}

func render(rep Report) string {
	var b strings.Builder
	for _, f := range rep.Findings {
		b.WriteString(f.Severity)
		b.WriteByte(' ')
		b.WriteString(f.Category)
		b.WriteByte(' ')
		b.WriteString(f.Resource)
		b.WriteByte('\n')
		b.WriteString(f.Phenomenon)
		b.WriteByte('\n')
		b.WriteString(f.LikelyCause)
		b.WriteByte('\n')
		b.WriteString(f.NextStep)
		b.WriteByte('\n')
	}
	return b.String()
}
