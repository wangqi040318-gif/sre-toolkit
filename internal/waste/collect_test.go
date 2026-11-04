package waste

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes/fake"
)

func TestCollectUsageAndOrphans(t *testing.T) {
	now := time.Date(2026, 11, 4, 12, 0, 0, 0, time.UTC)
	replicas := int32(1)
	yes := true
	cpu := resource.MustParse("4")
	mem := resource.MustParse("8Gi")
	deploy := &appsv1.Deployment{
		ObjectMeta: metav1.ObjectMeta{Namespace: "payments", Name: "api", CreationTimestamp: metav1.NewTime(now.Add(-48 * time.Hour))},
		Spec: appsv1.DeploymentSpec{
			Replicas: &replicas,
			Template: corev1.PodTemplateSpec{Spec: corev1.PodSpec{Containers: []corev1.Container{{
				Name: "api",
				Resources: corev1.ResourceRequirements{Requests: corev1.ResourceList{
					corev1.ResourceCPU: cpu, corev1.ResourceMemory: mem,
				}},
			}}}},
		},
		Status: appsv1.DeploymentStatus{ReadyReplicas: 1},
	}
	rs := &appsv1.ReplicaSet{
		ObjectMeta: metav1.ObjectMeta{
			Namespace: "payments", Name: "api-xyz",
			OwnerReferences: []metav1.OwnerReference{{
				APIVersion: "apps/v1", Kind: "Deployment", Name: "api", Controller: &yes,
			}},
		},
		Spec:   appsv1.ReplicaSetSpec{Replicas: &replicas},
		Status: appsv1.ReplicaSetStatus{ReadyReplicas: 1},
	}
	pod := &corev1.Pod{
		ObjectMeta: metav1.ObjectMeta{
			Namespace: "payments", Name: "api-xyz-aaa",
			OwnerReferences: []metav1.OwnerReference{{
				APIVersion: "apps/v1", Kind: "ReplicaSet", Name: "api-xyz", Controller: &yes,
			}},
		},
		Spec:   corev1.PodSpec{Containers: []corev1.Container{{Name: "api"}}},
		Status: corev1.PodStatus{Phase: corev1.PodRunning},
	}
	svc := &corev1.Service{
		ObjectMeta: metav1.ObjectMeta{Namespace: "payments", Name: "api-lb"},
		Spec:       corev1.ServiceSpec{Type: corev1.ServiceTypeLoadBalancer},
		Status: corev1.ServiceStatus{LoadBalancer: corev1.LoadBalancerStatus{Ingress: []corev1.LoadBalancerIngress{{
			IP: "198.51.100.10",
		}}}},
	}
	ep := &corev1.Endpoints{ObjectMeta: metav1.ObjectMeta{Namespace: "payments", Name: "api-lb"}}
	pvc := &corev1.PersistentVolumeClaim{
		ObjectMeta: metav1.ObjectMeta{Namespace: "payments", Name: "data"},
		Spec: corev1.PersistentVolumeClaimSpec{Resources: corev1.VolumeResourceRequirements{Requests: corev1.ResourceList{
			corev1.ResourceStorage: resource.MustParse("10Gi"),
		}}},
		Status: corev1.PersistentVolumeClaimStatus{Phase: corev1.ClaimBound},
	}
	mounted := &corev1.PersistentVolumeClaim{
		ObjectMeta: metav1.ObjectMeta{Namespace: "payments", Name: "mounted"},
		Spec: corev1.PersistentVolumeClaimSpec{Resources: corev1.VolumeResourceRequirements{Requests: corev1.ResourceList{
			corev1.ResourceStorage: resource.MustParse("1Gi"),
		}}},
		Status: corev1.PersistentVolumeClaimStatus{Phase: corev1.ClaimBound},
	}
	mounter := &corev1.Pod{
		ObjectMeta: metav1.ObjectMeta{Namespace: "payments", Name: "mounter"},
		Spec: corev1.PodSpec{Volumes: []corev1.Volume{{
			Name:         "data",
			VolumeSource: corev1.VolumeSource{PersistentVolumeClaim: &corev1.PersistentVolumeClaimVolumeSource{ClaimName: "mounted"}},
		}}},
		Status: corev1.PodStatus{Phase: corev1.PodRunning},
	}
	cm := &corev1.ConfigMap{ObjectMeta: metav1.ObjectMeta{
		Namespace: "payments", Name: "unused", CreationTimestamp: metav1.NewTime(now.Add(-72 * time.Hour)),
	}}
	usedCM := &corev1.ConfigMap{ObjectMeta: metav1.ObjectMeta{
		Namespace: "payments", Name: "app-config", CreationTimestamp: metav1.NewTime(now.Add(-72 * time.Hour)),
	}}
	pod.Spec.Containers[0].EnvFrom = []corev1.EnvFromSource{{ConfigMapRef: &corev1.ConfigMapEnvSource{LocalObjectReference: corev1.LocalObjectReference{Name: "app-config"}}}}

	client := fake.NewClientset(deploy, rs, pod, svc, ep, pvc, mounted, mounter, cm, usedCM)
	usage := mapUsage{items: map[string]PodUsage{
		"payments/api-xyz-aaa": {CPUMilli: qtyMilli("100m"), MemBytes: qtyBytes("100Mi")},
	}}
	snap, err := Collect(context.Background(), client, usage, "payments", false)
	if err != nil {
		t.Fatal(err)
	}
	if !snap.MetricsAvailable {
		t.Fatal("expected metrics")
	}
	rep := Analyze(snap, Options{AllNamespaces: true, Now: now, IncludeConfig: true, UsageRatio: 0.2})
	if rep.Summary.Oversized != 1 || rep.Summary.LoadBalancers != 1 || rep.Summary.OrphanedPVCs != 1 || rep.Summary.UnusedConfig != 1 {
		t.Fatalf("summary = %+v", rep.Summary)
	}
	blob := render(rep)
	if !strings.Contains(blob, "deploy payments/api") || strings.Contains(blob, "rs payments/api-xyz") {
		t.Fatalf("workload attribution\n%s", blob)
	}
	if strings.Contains(blob, "pvc payments/mounted") || strings.Contains(blob, "app-config") {
		t.Fatalf("used objects flagged\n%s", blob)
	}
	if !strings.Contains(blob, "198.51.100.10") {
		t.Fatalf("missing lb address\n%s", blob)
	}
}

func TestCollectMetricsDegrade(t *testing.T) {
	replicas := int32(1)
	deploy := &appsv1.Deployment{
		ObjectMeta: metav1.ObjectMeta{Namespace: "ns", Name: "api", CreationTimestamp: metav1.NewTime(time.Now().Add(-48 * time.Hour))},
		Spec: appsv1.DeploymentSpec{
			Replicas: &replicas,
			Template: corev1.PodTemplateSpec{Spec: corev1.PodSpec{Containers: []corev1.Container{{
				Name: "api",
				Resources: corev1.ResourceRequirements{Requests: corev1.ResourceList{
					corev1.ResourceCPU: resource.MustParse("4"),
				}},
			}}}},
		},
		Status: appsv1.DeploymentStatus{ReadyReplicas: 1},
	}
	client := fake.NewClientset(deploy)
	snap, err := Collect(context.Background(), client, errUsage{err: errors.New("the server could not find the requested resource")}, "ns", false)
	if err != nil {
		t.Fatal(err)
	}
	if snap.MetricsAvailable || !strings.Contains(snap.MetricsError, "could not find") {
		t.Fatalf("metrics = %v %s", snap.MetricsAvailable, snap.MetricsError)
	}
	rep := Analyze(snap, Options{AllNamespaces: true, Now: time.Now(), UsageRatio: 0.2})
	if rep.Summary.HighRequests != 1 || rep.Summary.Oversized != 0 {
		t.Fatalf("summary = %+v", rep.Summary)
	}
	if !strings.Contains(rep.Notes[0], "Usage metrics are unavailable") && !containsNote(rep, "Usage metrics are unavailable") {
		t.Fatalf("notes = %v", rep.Notes)
	}
}

func TestCollectRejectsBadInput(t *testing.T) {
	if _, err := Collect(context.Background(), nil, nil, "ns", false); err == nil {
		t.Fatal("expected nil client")
	}
	if _, err := Collect(context.Background(), fake.NewClientset(), nil, "", false); err == nil {
		t.Fatal("expected namespace error")
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if _, err := Collect(ctx, fake.NewClientset(), nil, "ns", false); err == nil {
		t.Fatal("expected canceled context")
	}
}

type mapUsage struct {
	items map[string]PodUsage
}

func (m mapUsage) ListPodUsage(context.Context, string) (map[string]PodUsage, error) {
	return m.items, nil
}

type errUsage struct{ err error }

func (e errUsage) ListPodUsage(context.Context, string) (map[string]PodUsage, error) {
	return nil, e.err
}

func qtyMilli(s string) int64 {
	q := resource.MustParse(s)
	return q.MilliValue()
}

func qtyBytes(s string) int64 {
	q := resource.MustParse(s)
	return q.Value()
}

func containsNote(rep Report, sub string) bool {
	for _, n := range rep.Notes {
		if strings.Contains(n, sub) {
			return true
		}
	}
	return false
}
