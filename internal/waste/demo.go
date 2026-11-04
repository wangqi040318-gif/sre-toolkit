package waste

import (
	"time"

	"k8s.io/apimachinery/pkg/api/resource"
)

// DemoSnapshot is a fixed cost-hygiene story. Usage numbers are synthetic,
// so the demo does not need metrics-server or a cluster.
func DemoSnapshot(now time.Time) Snapshot {
	if now.IsZero() {
		now = time.Now().UTC()
	}
	now = now.UTC()
	cpu := func(s string) int64 {
		q := resource.MustParse(s)
		return q.MilliValue()
	}
	mem := func(s string) int64 {
		q := resource.MustParse(s)
		return q.Value()
	}
	return Snapshot{
		Cluster:          "demo-cluster",
		MetricsAvailable: true,
		Workloads: []Workload{
			{
				Kind: KindDeployment, Namespace: "shop", Name: "recommendations",
				Desired: 0, Ready: 0, Created: now.Add(-10 * 24 * time.Hour),
			},
			{
				Kind: KindDeployment, Namespace: "data", Name: "batch",
				Desired: 3, Ready: 0, Created: now.Add(-4 * 24 * time.Hour),
				CPURequestMilli: cpu("500m"), MemRequestBytes: mem("1Gi"),
			},
			{
				Kind: KindStatefulSet, Namespace: "observability", Name: "prometheus",
				Desired: 1, Ready: 1, Created: now.Add(-30 * 24 * time.Hour),
				CPURequestMilli: cpu("4"), MemRequestBytes: mem("8Gi"),
				CPULimitMilli: cpu("4"), MemLimitBytes: mem("8Gi"),
				HasCPULimit: true, HasMemLimit: true,
				CPUUsageMilli: cpu("150m"), MemUsageBytes: mem("300Mi"),
				HasUsage: true,
			},
			{
				Kind: KindDeployment, Namespace: "platform", Name: "edge",
				Desired: 2, Ready: 2, Created: now.Add(-30 * 24 * time.Hour),
				CPURequestMilli: cpu("100m"), MemRequestBytes: mem("128Mi"),
				CPUUsageMilli: cpu("80m"), MemUsageBytes: mem("100Mi"),
				HasUsage: true,
			},
			{
				Kind: KindReplicaSet, Namespace: "shop", Name: "recommendations-7d4c",
				Desired: 0, Ready: 0, Created: now.Add(-10 * 24 * time.Hour),
				OwnedByDeployment: true,
			},
		},
		Services: []Service{
			{Namespace: "shop", Name: "storefront", Type: "LoadBalancer", ReadyEndpoints: 0, Address: "203.0.113.10"},
			{Namespace: "platform", Name: "edge", Type: "ClusterIP", ReadyEndpoints: 2},
			{Namespace: "payments", Name: "api", Type: "LoadBalancer", ReadyEndpoints: 3, Address: "203.0.113.20"},
		},
		Claims: []Claim{
			{Namespace: "data", Name: "archive", Phase: "Bound", StorageBytes: mem("200Gi")},
			{Namespace: "shop", Name: "cache", Phase: "Pending", StorageBytes: mem("10Gi")},
			{Namespace: "platform", Name: "data", Phase: "Bound", StorageBytes: mem("10Gi"), Mounted: true},
		},
		Configs: []ConfigObject{
			{Kind: KindConfigMap, Namespace: "shop", Name: "feature-flags", Created: now.Add(-30 * 24 * time.Hour)},
			{Kind: KindSecret, Namespace: "shop", Name: "old-api-key", Created: now.Add(-30 * 24 * time.Hour), SecretType: "Opaque"},
			{Kind: KindConfigMap, Namespace: "platform", Name: "edge-config", Created: now.Add(-30 * 24 * time.Hour), Used: true},
			{Kind: KindSecret, Namespace: "shop", Name: "default-token-abc", Created: now.Add(-30 * 24 * time.Hour), SecretType: "kubernetes.io/service-account-token"},
			{Kind: KindConfigMap, Namespace: "shop", Name: "kube-root-ca.crt", Created: now.Add(-30 * 24 * time.Hour)},
		},
	}
}
