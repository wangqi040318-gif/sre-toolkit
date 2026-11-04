package runbook

import (
	"context"
	"strings"
	"testing"

	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/client-go/kubernetes/fake"
)

func TestCollectDeploymentRollbackTarget(t *testing.T) {
	replicas := int32(3)
	uid := types.UID("dep-checkout")
	dep := &appsv1.Deployment{
		ObjectMeta: metav1.ObjectMeta{
			Namespace: "shop", Name: "checkout", UID: uid,
			Annotations: map[string]string{revisionAnnotation: "12"},
		},
		Spec: appsv1.DeploymentSpec{
			Replicas: &replicas,
			Selector: &metav1.LabelSelector{MatchLabels: map[string]string{"app": "checkout"}},
			Template: corev1.PodTemplateSpec{
				ObjectMeta: metav1.ObjectMeta{Labels: map[string]string{"app": "checkout"}},
				Spec: corev1.PodSpec{Containers: []corev1.Container{{
					Name: "checkout", Image: "ghcr.io/example/checkout:1.9.0",
				}}},
			},
		},
		Status: appsv1.DeploymentStatus{ReadyReplicas: 0, UpdatedReplicas: 3},
	}
	oldRS := rs(uid, "checkout-old", "11", "ghcr.io/example/checkout:1.8.4")
	newRS := rs(uid, "checkout-new", "12", "ghcr.io/example/checkout:1.9.0")
	other := rs(types.UID("other"), "api-old", "4", "ghcr.io/example/api:1")
	other.Namespace = "shop"
	pod := &corev1.Pod{
		ObjectMeta: metav1.ObjectMeta{Namespace: "shop", Name: "checkout-new-aaa", Labels: map[string]string{"app": "checkout"}},
		Status: corev1.PodStatus{ContainerStatuses: []corev1.ContainerStatus{{
			Name:  "checkout",
			State: corev1.ContainerState{Waiting: &corev1.ContainerStateWaiting{Reason: "CrashLoopBackOff"}},
		}}},
	}
	client := fake.NewClientset(dep, oldRS, newRS, other, pod)
	snap, err := Collect(context.Background(), client, KindDeployment, "shop", "checkout")
	if err != nil {
		t.Fatal(err)
	}
	if snap.CurrentRevision != 12 || snap.Ready != 0 || snap.Selector != "app=checkout" {
		t.Fatalf("snap = %+v", snap)
	}
	if len(snap.Revisions) != 2 {
		t.Fatalf("revisions = %+v", snap.Revisions)
	}
	if !strings.Contains(snap.Failure, "CrashLoopBackOff") {
		t.Fatalf("failure = %q", snap.Failure)
	}
	rep, err := Build(snap, Options{})
	if err != nil {
		t.Fatal(err)
	}
	if rep.TargetRevision != 11 || !strings.Contains(rep.TargetImage, "1.8.4") {
		t.Fatalf("target rev=%d image=%s", rep.TargetRevision, rep.TargetImage)
	}
	blob := render(rep)
	if !strings.Contains(blob, "kubectl rollout undo deployment/checkout -n shop --to-revision=11") {
		t.Fatalf("undo missing\n%s", blob)
	}
}

func TestCollectStatefulSetRevisions(t *testing.T) {
	replicas := int32(1)
	uid := types.UID("sts-db")
	sts := &appsv1.StatefulSet{
		ObjectMeta: metav1.ObjectMeta{Namespace: "data", Name: "db", UID: uid},
		Spec: appsv1.StatefulSetSpec{
			Replicas: &replicas,
			Selector: &metav1.LabelSelector{MatchLabels: map[string]string{"app": "db"}},
			Template: corev1.PodTemplateSpec{Spec: corev1.PodSpec{Containers: []corev1.Container{{
				Name: "db", Image: "postgres:16",
			}}}},
		},
		Status: appsv1.StatefulSetStatus{
			ReadyReplicas:   0,
			UpdatedReplicas: 1,
			CurrentRevision: "db-old",
			UpdateRevision:  "db-new",
		},
	}
	old := controllerRev(uid, "db-old", 3, "postgres:15")
	newer := controllerRev(uid, "db-new", 4, "postgres:16")
	client := fake.NewClientset(sts, old, newer)
	snap, err := Collect(context.Background(), client, KindStatefulSet, "data", "db")
	if err != nil {
		t.Fatal(err)
	}
	if snap.CurrentRevision != 4 {
		t.Fatalf("current = %d", snap.CurrentRevision)
	}
	rep, err := Build(snap, Options{})
	if err != nil {
		t.Fatal(err)
	}
	if rep.TargetRevision != 3 || !strings.Contains(rep.TargetImage, "postgres:15") {
		t.Fatalf("target rev=%d image=%s revs=%+v", rep.TargetRevision, rep.TargetImage, snap.Revisions)
	}
	if !strings.Contains(render(rep), "kubectl rollout undo statefulset/db -n data --to-revision=3") {
		t.Fatalf("undo missing\n%s", render(rep))
	}
}

func TestCollectRejectsMissing(t *testing.T) {
	if _, err := Collect(context.Background(), nil, KindDeployment, "shop", "checkout"); err == nil {
		t.Fatal("expected nil client error")
	}
	_, err := Collect(context.Background(), fake.NewClientset(), KindDeployment, "shop", "missing")
	if err == nil || !strings.Contains(err.Error(), "missing") {
		t.Fatalf("err = %v", err)
	}
}

func rs(owner types.UID, name, rev, image string) *appsv1.ReplicaSet {
	yes := true
	return &appsv1.ReplicaSet{
		ObjectMeta: metav1.ObjectMeta{
			Namespace:   "shop",
			Name:        name,
			Annotations: map[string]string{revisionAnnotation: rev},
			OwnerReferences: []metav1.OwnerReference{{
				APIVersion: "apps/v1", Kind: KindDeployment, Name: "checkout", UID: owner, Controller: &yes,
			}},
		},
		Spec: appsv1.ReplicaSetSpec{Template: corev1.PodTemplateSpec{Spec: corev1.PodSpec{Containers: []corev1.Container{{
			Name: "checkout", Image: image,
		}}}}},
	}
}

func controllerRev(owner types.UID, name string, rev int64, image string) *appsv1.ControllerRevision {
	yes := true
	raw := []byte(`{"spec":{"template":{"spec":{"containers":[{"name":"db","image":"` + image + `"}]}}}}`)
	return &appsv1.ControllerRevision{
		ObjectMeta: metav1.ObjectMeta{
			Namespace: "data",
			Name:      name,
			OwnerReferences: []metav1.OwnerReference{{
				APIVersion: "apps/v1", Kind: KindStatefulSet, Name: "db", UID: owner, Controller: &yes,
			}},
		},
		Revision: rev,
		Data:     runtime.RawExtension{Raw: raw},
	}
}
