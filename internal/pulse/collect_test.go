package pulse

import (
	"context"
	"testing"
	"time"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes/fake"
)

func TestCollectNamespaceAndAll(t *testing.T) {
	now := time.Now().UTC().Truncate(time.Second)
	pay := basePod("payments", "checkout", "worker-a", corev1.PodRunning)
	shop := basePod("shop", "web", "worker-b", corev1.PodRunning)
	node := readyNode("worker-a")
	ev := warningEvent("shop", "web.1", "Pod", "shop", "web", "Unhealthy", "probe failed", 1, now)
	client := fake.NewClientset(&pay, &shop, &node, &ev)

	ctx := context.Background()
	all, err := Collect(ctx, client, "", true)
	if err != nil {
		t.Fatal(err)
	}
	if len(all.Pods) != 2 || len(all.Nodes) != 1 || len(all.Events) != 1 {
		t.Fatalf("all = pods %d nodes %d events %d", len(all.Pods), len(all.Nodes), len(all.Events))
	}

	one, err := Collect(ctx, client, "payments", false)
	if err != nil {
		t.Fatal(err)
	}
	if len(one.Pods) != 1 || one.Pods[0].Name != "checkout" {
		t.Fatalf("pods = %+v", one.Pods)
	}
	if len(one.Nodes) != 1 {
		t.Fatalf("nodes = %d", len(one.Nodes))
	}
	if len(one.Events) != 0 {
		t.Fatalf("events = %+v", one.Events)
	}

	all.Cluster = "demo"
	rep := Analyze(all, Options{AllNamespaces: true, Now: now})
	if rep.Summary.WarningEvents != 1 || rep.Cluster != "demo" {
		t.Fatalf("report = %+v", rep.Summary)
	}
}

func TestCollectRejectsBadInput(t *testing.T) {
	if _, err := Collect(context.Background(), nil, "default", false); err == nil {
		t.Fatal("expected nil client error")
	}
	client := fake.NewClientset()
	if _, err := Collect(context.Background(), client, "", false); err == nil {
		t.Fatal("expected missing namespace error")
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if _, err := Collect(ctx, client, "default", false); err == nil {
		t.Fatal("expected canceled context error")
	}
}

func TestCollectObjectMetaTypes(t *testing.T) {
	// Keep a typed object in the test so a client-go upgrade that drops
	// corev1.Event from the fake scheme fails here instead of at runtime.
	ev := &corev1.Event{ObjectMeta: metav1.ObjectMeta{Namespace: "ns", Name: "e"}}
	client := fake.NewClientset(ev)
	snap, err := Collect(context.Background(), client, "ns", false)
	if err != nil {
		t.Fatal(err)
	}
	if len(snap.Events) != 1 {
		t.Fatalf("events = %d", len(snap.Events))
	}
}
