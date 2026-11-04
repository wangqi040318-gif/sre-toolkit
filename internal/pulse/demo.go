package pulse

import (
	"time"

	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// DemoSnapshot is a fixed incident story for resumes and interviews.
// It does not contact a cluster. Pass the same now value used by Analyze
// so event ages stay inside the default one-hour window.
func DemoSnapshot(now time.Time) Snapshot {
	if now.IsZero() {
		now = time.Now()
	}
	return Snapshot{
		Cluster: "demo-cluster",
		Nodes:   demoNodes(),
		Pods:    demoPods(now),
		Events:  demoEvents(now),
	}
}

func demoNodes() []corev1.Node {
	return []corev1.Node{
		readyNode("cp-1"),
		readyNode("worker-a"),
		readyNode("worker-b"),
		{
			ObjectMeta: metav1.ObjectMeta{Name: "worker-3"},
			Spec:       corev1.NodeSpec{Unschedulable: true},
			Status: corev1.NodeStatus{
				Conditions: []corev1.NodeCondition{
					{
						Type:    corev1.NodeReady,
						Status:  corev1.ConditionFalse,
						Reason:  "KubeletNotReady",
						Message: "kubelet is not posting node status; ephemeral storage is exhausted",
					},
					{
						Type:    corev1.NodeDiskPressure,
						Status:  corev1.ConditionTrue,
						Reason:  "KubeletHasDiskPressure",
						Message: "available ephemeral storage is below the eviction threshold",
					},
					{Type: corev1.NodeMemoryPressure, Status: corev1.ConditionFalse},
					{Type: corev1.NodePIDPressure, Status: corev1.ConditionFalse},
					{Type: corev1.NodeNetworkUnavailable, Status: corev1.ConditionFalse},
				},
			},
		},
	}
}

func readyNode(name string) corev1.Node {
	return corev1.Node{
		ObjectMeta: metav1.ObjectMeta{Name: name},
		Status: corev1.NodeStatus{
			Conditions: []corev1.NodeCondition{
				{Type: corev1.NodeReady, Status: corev1.ConditionTrue, Reason: "KubeletReady"},
				{Type: corev1.NodeDiskPressure, Status: corev1.ConditionFalse},
				{Type: corev1.NodeMemoryPressure, Status: corev1.ConditionFalse},
				{Type: corev1.NodePIDPressure, Status: corev1.ConditionFalse},
			},
		},
	}
}

func demoPods(now time.Time) []corev1.Pod {
	checkout := basePod("payments", "checkout-7d4c8f9b6-xk2lp", "worker-a", corev1.PodRunning)
	checkout.Spec.Containers = []corev1.Container{containerSpec("checkout", "256Mi")}
	checkout.Status.ContainerStatuses = []corev1.ContainerStatus{{
		Name:         "checkout",
		RestartCount: 14,
		State: corev1.ContainerState{Waiting: &corev1.ContainerStateWaiting{
			Reason:  "CrashLoopBackOff",
			Message: "back-off 2m40s restarting failed container",
		}},
		LastTerminationState: corev1.ContainerState{Terminated: &corev1.ContainerStateTerminated{
			Reason:   "Error",
			ExitCode: 1,
			Message:  "missing env STRIPE_API_KEY",
		}},
	}}

	prom := basePod("observability", "prometheus-0", "worker-b", corev1.PodRunning)
	prom.Spec.Containers = []corev1.Container{containerSpec("prometheus", "512Mi")}
	prom.Status.ContainerStatuses = []corev1.ContainerStatus{{
		Name:         "prometheus",
		RestartCount: 6,
		State: corev1.ContainerState{Waiting: &corev1.ContainerStateWaiting{
			Reason:  "CrashLoopBackOff",
			Message: "back-off 5m0s restarting failed container",
		}},
		LastTerminationState: corev1.ContainerState{Terminated: &corev1.ContainerStateTerminated{
			Reason:   "OOMKilled",
			ExitCode: 137,
			Message:  "Container exceeded memory limit",
		}},
	}}

	recs := basePod("shop", "recommendations-5c8f9d7b-q9k2l", "worker-a", corev1.PodRunning)
	recs.Spec.Containers = []corev1.Container{containerSpec("recommendations", "256Mi")}
	recs.Status.ContainerStatuses = []corev1.ContainerStatus{{
		Name:         "recommendations",
		RestartCount: 9,
		State:        corev1.ContainerState{Running: &corev1.ContainerStateRunning{StartedAt: metav1.NewTime(now.Add(-2 * time.Minute))}},
		LastTerminationState: corev1.ContainerState{Terminated: &corev1.ContainerStateTerminated{
			Reason:   "Error",
			ExitCode: 1,
			Message:  "dial tcp catalog:8080: connect: connection refused",
		}},
	}}

	batch := basePod("data", "batch-worker-2", "", corev1.PodPending)
	batch.Spec.Containers = []corev1.Container{containerSpec("worker", "1Gi")}
	batch.Status.Conditions = []corev1.PodCondition{{
		Type:    corev1.PodScheduled,
		Status:  corev1.ConditionFalse,
		Reason:  "Unschedulable",
		Message: "0/4 nodes are available: 3 Insufficient memory, 1 node(s) were not ready.",
	}}

	gateway := healthyPod("payments", "api-gateway-6b8d4c9f-m2n4p", "worker-b", "gateway", now)
	store := healthyPod("shop", "storefront-0", "worker-a", "storefront", now)
	store.Status.ContainerStatuses[0].RestartCount = 1
	grafana := healthyPod("observability", "grafana-5f6d7c8b-h4k2s", "worker-b", "grafana", now)
	dns := healthyPod("kube-system", "coredns-7f8b9d6c-p9xq2", "cp-1", "coredns", now)

	return []corev1.Pod{checkout, prom, recs, batch, gateway, store, grafana, dns}
}

func demoEvents(now time.Time) []corev1.Event {
	return []corev1.Event{
		warningEvent(
			"payments", "checkout.18a",
			"Pod", "payments", "checkout-7d4c8f9b6-xk2lp",
			"FailedMount",
			`MountVolume.SetUp failed for volume "stripe": secret "checkout-secrets" not found`,
			11, now.Add(-4*time.Minute),
		),
		warningEvent(
			"data", "batch-worker-2.16c",
			"Pod", "data", "batch-worker-2",
			"FailedScheduling",
			"0/4 nodes are available: 1 node(s) had untolerated taint, 3 Insufficient memory.",
			6, now.Add(-8*time.Minute),
		),
		warningEvent(
			"shop", "cache-evict.9",
			"Pod", "shop", "cache-evict-7c9b",
			"Evicted",
			"The node was low on resource: ephemeral-storage. Threshold quantity: 10%, available: 4%.",
			1, now.Add(-15*time.Minute),
		),
		warningEvent(
			"payments", "checkout.old",
			"Pod", "payments", "checkout-7d4c8f9b6-xk2lp",
			"BackOff",
			"Back-off restarting failed container",
			40, now.Add(-3*time.Hour),
		),
		{
			ObjectMeta: metav1.ObjectMeta{Namespace: "payments", Name: "checkout.pulled"},
			InvolvedObject: corev1.ObjectReference{
				Kind: "Pod", Namespace: "payments", Name: "checkout-7d4c8f9b6-xk2lp",
			},
			Type:          corev1.EventTypeNormal,
			Reason:        "Pulled",
			Message:       "Successfully pulled image",
			Count:         1,
			LastTimestamp: metav1.NewTime(now.Add(-1 * time.Minute)),
		},
	}
}

func basePod(ns, name, node string, phase corev1.PodPhase) corev1.Pod {
	return corev1.Pod{
		ObjectMeta: metav1.ObjectMeta{Namespace: ns, Name: name},
		Spec:       corev1.PodSpec{NodeName: node},
		Status:     corev1.PodStatus{Phase: phase},
	}
}

func containerSpec(name, memory string) corev1.Container {
	return corev1.Container{
		Name: name,
		Resources: corev1.ResourceRequirements{
			Limits: corev1.ResourceList{
				corev1.ResourceMemory: resource.MustParse(memory),
			},
		},
	}
}

func healthyPod(ns, name, node, container string, now time.Time) corev1.Pod {
	p := basePod(ns, name, node, corev1.PodRunning)
	p.Spec.Containers = []corev1.Container{containerSpec(container, "128Mi")}
	p.Status.ContainerStatuses = []corev1.ContainerStatus{{
		Name:         container,
		RestartCount: 0,
		Ready:        true,
		State:        corev1.ContainerState{Running: &corev1.ContainerStateRunning{StartedAt: metav1.NewTime(now.Add(-6 * time.Hour))}},
	}}
	return p
}

func warningEvent(ns, name, kind, objNS, objName, reason, message string, count int32, when time.Time) corev1.Event {
	return corev1.Event{
		ObjectMeta: metav1.ObjectMeta{Namespace: ns, Name: name},
		InvolvedObject: corev1.ObjectReference{
			Kind:      kind,
			Namespace: objNS,
			Name:      objName,
		},
		Type:          corev1.EventTypeWarning,
		Reason:        reason,
		Message:       message,
		Count:         count,
		LastTimestamp: metav1.NewTime(when),
	}
}
