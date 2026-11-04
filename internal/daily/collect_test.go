package daily

import (
	"context"
	"testing"
	"time"

	batchv1 "k8s.io/api/batch/v1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes/fake"
)

func TestCollectCluster(t *testing.T) {
	now := time.Date(2026, 11, 4, 12, 0, 0, 0, time.UTC)
	node := &corev1.Node{
		ObjectMeta: metav1.ObjectMeta{Name: "n1"},
		Status: corev1.NodeStatus{Conditions: []corev1.NodeCondition{
			{Type: corev1.NodeReady, Status: corev1.ConditionFalse, Reason: "KubeletNotReady"},
		}},
	}
	pod := &corev1.Pod{
		ObjectMeta: metav1.ObjectMeta{Namespace: "shop", Name: "checkout"},
		Status: corev1.PodStatus{Phase: corev1.PodRunning, ContainerStatuses: []corev1.ContainerStatus{{
			Name:  "app",
			State: corev1.ContainerState{Waiting: &corev1.ContainerStateWaiting{Reason: "CrashLoopBackOff"}},
		}}},
	}
	ev := &corev1.Event{
		ObjectMeta:     metav1.ObjectMeta{Namespace: "shop", Name: "e1"},
		InvolvedObject: corev1.ObjectReference{Kind: "Pod", Namespace: "shop", Name: "checkout"},
		Type:           corev1.EventTypeWarning,
		Reason:         "BackOff",
		Message:        "back-off",
		Count:          2,
		LastTimestamp:  metav1.NewTime(now.Add(-time.Minute)),
	}
	failed := &batchv1.Job{
		ObjectMeta: metav1.ObjectMeta{Namespace: "shop", Name: "nightly"},
		Status: batchv1.JobStatus{Conditions: []batchv1.JobCondition{{
			Type: batchv1.JobFailed, Status: corev1.ConditionTrue, Reason: "BackoffLimitExceeded",
		}}},
	}
	okJob := &batchv1.Job{
		ObjectMeta: metav1.ObjectMeta{Namespace: "shop", Name: "ok"},
		Status: batchv1.JobStatus{Conditions: []batchv1.JobCondition{{
			Type: batchv1.JobComplete, Status: corev1.ConditionTrue,
		}}},
	}
	client := fake.NewClientset(node, pod, ev, failed, okJob)
	got, err := CollectCluster(context.Background(), client, "prod-eu", now)
	if err != nil {
		t.Fatal(err)
	}
	if got.NodesNotReady != 1 || got.CrashLoop != 1 || got.FailedJobs != 1 || got.WarningEvents != 1 || got.OOMKilled != 0 {
		t.Fatalf("cluster = %+v", got)
	}
	if got.Err != "" {
		t.Fatalf("err = %s", got.Err)
	}
}
