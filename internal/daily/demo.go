package daily

import (
	"time"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	"github.com/wangqi040318-gif/sre-toolkit/internal/certs"
	"github.com/wangqi040318-gif/sre-toolkit/internal/pulse"
)

// Demo returns prod-eu (needs attention) and prod-us (quiet).
// Pod, node, and certificate counts come from pulse.Analyze and certs.Analyze.
func Demo(now time.Time) []Cluster {
	if now.IsZero() {
		now = time.Now().UTC()
	}
	now = now.UTC()
	eu := fromPulse("prod-eu", euSnapshot(now), now)
	eu.FailedJobs = 1
	if mats, err := certs.DemoMaterials(now); err == nil {
		rep := certs.Analyze(mats, certs.Options{
			AllNamespaces: true,
			Now:           now,
			WarnDays:      30,
			CriticalDays:  7,
		})
		eu.CertsDue = rep.Summary.Expired + rep.Summary.Critical
		eu.CertsWarn = rep.Summary.Warning
	}
	us := fromPulse("prod-us", usSnapshot(), now)
	return []Cluster{eu, us}
}

func fromPulse(context string, snap pulse.Snapshot, now time.Time) Cluster {
	rep := pulse.Analyze(snap, pulse.Options{
		AllNamespaces: true,
		Now:           now,
		Since:         time.Hour,
		EventLimit:    20,
	})
	return Cluster{
		Context:       context,
		NodesTotal:    rep.Summary.NodesTotal,
		NodesNotReady: rep.Summary.NodesNotReady,
		CrashLoop:     rep.Summary.CrashLoop,
		OOMKilled:     rep.Summary.OOMKilled,
		WarningEvents: rep.Summary.WarningEvents,
	}
}

func euSnapshot(now time.Time) pulse.Snapshot {
	return pulse.Snapshot{
		Cluster: "prod-eu",
		Nodes: []corev1.Node{
			readyNode("eu-a"),
			readyNode("eu-b"),
			{
				ObjectMeta: metav1.ObjectMeta{Name: "eu-c"},
				Status: corev1.NodeStatus{Conditions: []corev1.NodeCondition{
					{Type: corev1.NodeReady, Status: corev1.ConditionFalse, Reason: "KubeletNotReady", Message: "disk pressure"},
				}},
			},
		},
		Pods: []corev1.Pod{
			crashPod("shop", "checkout"),
			crashPod("shop", "checkout-2"),
			oomPod("observability", "prometheus"),
			healthyPod("platform", "edge"),
		},
		Events: []corev1.Event{
			warning("shop", "checkout.1", "checkout", "BackOff", now.Add(-3*time.Minute)),
			warning("shop", "checkout.2", "checkout-2", "Unhealthy", now.Add(-4*time.Minute)),
			warning("observability", "prom.1", "prometheus", "OOMKilling", now.Add(-6*time.Minute)),
			warning("shop", "job.1", "nightly", "Failed", now.Add(-8*time.Minute)),
		},
	}
}

func usSnapshot() pulse.Snapshot {
	return pulse.Snapshot{
		Cluster: "prod-us",
		Nodes:   []corev1.Node{readyNode("us-a"), readyNode("us-b")},
		Pods:    []corev1.Pod{healthyPod("platform", "edge")},
	}
}

func readyNode(name string) corev1.Node {
	return corev1.Node{
		ObjectMeta: metav1.ObjectMeta{Name: name},
		Status: corev1.NodeStatus{Conditions: []corev1.NodeCondition{
			{Type: corev1.NodeReady, Status: corev1.ConditionTrue, Reason: "KubeletReady"},
		}},
	}
}

func crashPod(ns, name string) corev1.Pod {
	return corev1.Pod{
		ObjectMeta: metav1.ObjectMeta{Namespace: ns, Name: name},
		Status: corev1.PodStatus{
			Phase: corev1.PodRunning,
			ContainerStatuses: []corev1.ContainerStatus{{
				Name:         "app",
				RestartCount: 6,
				State:        corev1.ContainerState{Waiting: &corev1.ContainerStateWaiting{Reason: "CrashLoopBackOff"}},
			}},
		},
	}
}

func oomPod(ns, name string) corev1.Pod {
	return corev1.Pod{
		ObjectMeta: metav1.ObjectMeta{Namespace: ns, Name: name},
		Status: corev1.PodStatus{
			Phase: corev1.PodRunning,
			ContainerStatuses: []corev1.ContainerStatus{{
				Name:         "app",
				RestartCount: 4,
				State:        corev1.ContainerState{Waiting: &corev1.ContainerStateWaiting{Reason: "CrashLoopBackOff"}},
				LastTerminationState: corev1.ContainerState{Terminated: &corev1.ContainerStateTerminated{
					Reason: "OOMKilled", ExitCode: 137,
				}},
			}},
		},
	}
}

func healthyPod(ns, name string) corev1.Pod {
	return corev1.Pod{
		ObjectMeta: metav1.ObjectMeta{Namespace: ns, Name: name},
		Status: corev1.PodStatus{
			Phase: corev1.PodRunning,
			ContainerStatuses: []corev1.ContainerStatus{{
				Name:  "app",
				Ready: true,
				State: corev1.ContainerState{Running: &corev1.ContainerStateRunning{}},
			}},
		},
	}
}

func warning(ns, name, obj, reason string, when time.Time) corev1.Event {
	return corev1.Event{
		ObjectMeta:     metav1.ObjectMeta{Namespace: ns, Name: name},
		InvolvedObject: corev1.ObjectReference{Kind: "Pod", Namespace: ns, Name: obj},
		Type:           corev1.EventTypeWarning,
		Reason:         reason,
		Message:        reason,
		Count:          1,
		LastTimestamp:  metav1.NewTime(when),
	}
}
