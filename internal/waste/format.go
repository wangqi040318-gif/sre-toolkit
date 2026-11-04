package waste

import (
	"fmt"
	"math"
	"time"

	"k8s.io/apimachinery/pkg/api/resource"
)

func workloadRef(w Workload) string {
	switch w.Kind {
	case KindStatefulSet:
		return "sts " + w.Namespace + "/" + w.Name
	case KindReplicaSet:
		return "rs " + w.Namespace + "/" + w.Name
	default:
		return "deploy " + w.Namespace + "/" + w.Name
	}
}

func formatCPU(milli int64) string {
	return resource.NewMilliQuantity(milli, resource.DecimalSI).String()
}

func formatMem(bytes int64) string {
	return resource.NewQuantity(bytes, resource.BinarySI).String()
}

func formatPercent(r float64) string {
	pct := r * 100
	if pct < 10 {
		return fmt.Sprintf("%.1f%%", pct)
	}
	return fmt.Sprintf("%.0f%%", math.Round(pct))
}

func agePhrase(d time.Duration) string {
	if d < time.Hour {
		return "under 1h"
	}
	if d < 48*time.Hour {
		return fmt.Sprintf("%dh", int(d.Hours()))
	}
	days := int(d.Hours()) / 24
	if days < 1 {
		days = 1
	}
	return fmt.Sprintf("%dd", days)
}

func qosPhrase(w Workload) string {
	cpuMatch := w.HasCPULimit && w.CPULimitMilli == w.CPURequestMilli && w.CPURequestMilli > 0
	memMatch := w.HasMemLimit && w.MemLimitBytes == w.MemRequestBytes && w.MemRequestBytes > 0
	if cpuMatch || memMatch {
		return " Request equals limit, so the scheduler reserves the whole amount (Guaranteed or nearly so)."
	}
	return ""
}

func idleNext(w Workload, scaledToZero bool) string {
	kind := kubectlKind(w.Kind)
	ref := w.Namespace + "/" + w.Name
	if scaledToZero {
		return fmt.Sprintf("Delete %s %s if the feature is gone, or leave it at zero only when scale-to-zero is intentional (KEDA or a cron). kubectl get %s -n %s %s", w.Kind, ref, kind, w.Namespace, w.Name)
	}
	return fmt.Sprintf("Scale %s %s to zero or delete it if it is abandoned. If it should be serving, inspect pods before treating it as waste. kubectl scale %s -n %s %s --replicas=0", w.Kind, ref, kind, w.Namespace, w.Name)
}

func kubectlKind(kind string) string {
	switch kind {
	case KindStatefulSet:
		return "statefulset"
	case KindReplicaSet:
		return "replicaset"
	default:
		return "deploy"
	}
}

func oversizedNext(w Workload) string {
	kind := kubectlKind(w.Kind)
	return fmt.Sprintf("Rightsize requests toward observed usage and keep a little headroom, then watch the rollout. kubectl top pod -n %s; kubectl describe %s -n %s %s", w.Namespace, kind, w.Namespace, w.Name)
}

func highRequestNext(w Workload) string {
	kind := kubectlKind(w.Kind)
	return fmt.Sprintf("Install or repair metrics-server, then lower requests that stay far above usage. Until then, treat this reservation as suspect. kubectl top pod -n %s; kubectl describe %s -n %s %s", w.Namespace, kind, w.Namespace, w.Name)
}

func lbNext(s Service) string {
	return fmt.Sprintf("Delete Service %s/%s or change it to ClusterIP if the cloud load balancer is unused. An address with no endpoints still bills. kubectl get svc -n %s %s", s.Namespace, s.Name, s.Namespace, s.Name)
}

func pvcNext(c Claim, bound bool) string {
	if bound {
		return fmt.Sprintf("Delete PVC %s/%s if the data is retired, or mount it from the workload that should use it. Deleting a Bound claim can delete the volume. kubectl describe pvc -n %s %s", c.Namespace, c.Name, c.Namespace, c.Name)
	}
	return fmt.Sprintf("Delete PVC %s/%s if nothing will bind it, or fix the StorageClass. kubectl describe pvc -n %s %s", c.Namespace, c.Name, c.Namespace, c.Name)
}

func configNext(o ConfigObject) string {
	kind := "configmap"
	if o.Kind == KindSecret {
		kind = "secret"
	}
	return fmt.Sprintf("Confirm no external consumer (CI, admission, a chart) still needs %s %s/%s, then delete it. kubectl delete %s -n %s %s", kind, o.Namespace, o.Name, kind, o.Namespace, o.Name)
}
