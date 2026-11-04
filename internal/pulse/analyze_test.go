package pulse

import (
	"strings"
	"testing"
	"time"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

func TestDemoPulseStory(t *testing.T) {
	now := time.Date(2026, 11, 4, 12, 0, 0, 0, time.UTC)
	rep := Analyze(DemoSnapshot(now), Options{AllNamespaces: true, Now: now, Demo: true})

	if rep.Cluster != "demo-cluster" {
		t.Fatalf("cluster = %q", rep.Cluster)
	}
	if rep.Scope != "all namespaces" {
		t.Fatalf("scope = %q", rep.Scope)
	}
	if rep.Summary.NodesTotal != 4 || rep.Summary.NodesNotReady != 1 {
		t.Fatalf("nodes = %+v", rep.Summary)
	}
	if rep.Summary.PodsScanned != 8 {
		t.Fatalf("pods scanned = %d", rep.Summary.PodsScanned)
	}
	if rep.Summary.CrashLoop != 1 || rep.Summary.OOMKilled != 1 || rep.Summary.HighRestarts != 1 {
		t.Fatalf("pod signals = %+v", rep.Summary)
	}
	if rep.Summary.WarningEvents != 3 {
		t.Fatalf("warning events = %d, want 3 (old BackOff and Normal Pulled excluded)", rep.Summary.WarningEvents)
	}

	byCat := map[string][]Finding{}
	for _, f := range rep.Findings {
		if f.Phenomenon == "" || f.LikelyCause == "" || f.NextStep == "" {
			t.Fatalf("incomplete finding: %+v", f)
		}
		byCat[f.Category] = append(byCat[f.Category], f)
	}
	for _, cat := range []string{CatOOM, CatCrashLoop, CatNodeNotReady, CatHighRestarts, CatWarning} {
		if len(byCat[cat]) == 0 {
			t.Fatalf("missing category %s", cat)
		}
	}

	oom := byCat[CatOOM][0]
	if oom.Resource != "observability/prometheus-0" {
		t.Fatalf("oom resource = %s", oom.Resource)
	}
	if !strings.Contains(oom.Phenomenon, "memory limit 512Mi") || !strings.Contains(oom.Phenomenon, "CrashLoopBackOff") {
		t.Fatalf("oom phenomenon = %s", oom.Phenomenon)
	}
	if !strings.Contains(oom.LikelyCause, "512Mi") || !strings.Contains(oom.NextStep, "kubectl top pod") {
		t.Fatalf("oom advice = %s / %s", oom.LikelyCause, oom.NextStep)
	}
	if len(byCat[CatCrashLoop]) != 1 || byCat[CatCrashLoop][0].Resource == oom.Resource {
		t.Fatalf("prometheus should be OOM only, crash findings = %+v", byCat[CatCrashLoop])
	}

	crash := byCat[CatCrashLoop][0]
	if !strings.Contains(crash.Phenomenon, "STRIPE_API_KEY") {
		t.Fatalf("crash phenomenon = %s", crash.Phenomenon)
	}
	if !strings.Contains(crash.LikelyCause, "environment variable") || !strings.Contains(crash.NextStep, "--previous") {
		t.Fatalf("crash advice = %s / %s", crash.LikelyCause, crash.NextStep)
	}

	node := byCat[CatNodeNotReady][0]
	if node.Resource != "worker-3" || node.Severity != SevCritical {
		t.Fatalf("node finding = %+v", node)
	}
	if !strings.Contains(node.Phenomenon, "DiskPressure") || !strings.Contains(node.LikelyCause, "DiskPressure") {
		t.Fatalf("node text = %s / %s", node.Phenomenon, node.LikelyCause)
	}
	if !strings.Contains(node.NextStep, "ephemeral storage") {
		t.Fatalf("node next = %s", node.NextStep)
	}

	high := byCat[CatHighRestarts][0]
	if !strings.Contains(high.Phenomenon, "9 times") || !strings.Contains(high.LikelyCause, "dependency") {
		t.Fatalf("high restart = %s / %s", high.Phenomenon, high.LikelyCause)
	}

	var reasons []string
	for _, f := range byCat[CatWarning] {
		reasons = append(reasons, detailValue(f, "reason"))
		if !strings.Contains(f.Phenomenon, "Warning ") || !strings.Contains(f.NextStep, "kubectl describe") {
			t.Fatalf("event finding = %+v", f)
		}
	}
	joined := strings.Join(reasons, ",")
	for _, want := range []string{"FailedMount", "FailedScheduling", "Evicted"} {
		if !strings.Contains(joined, want) {
			t.Fatalf("reasons %s missing %s", joined, want)
		}
	}
	for _, f := range rep.Findings {
		if strings.Contains(f.Phenomenon, "Successfully pulled image") || strings.Contains(f.Phenomenon, "Back-off restarting failed container") {
			t.Fatalf("filtered event leaked: %s", f.Phenomenon)
		}
	}

	if rep.Findings[0].Category != CatOOM || rep.Findings[0].Severity != SevCritical {
		t.Fatalf("first finding = %+v", rep.Findings[0])
	}
	if !rep.Demo {
		t.Fatal("demo flag not preserved")
	}
	if len(rep.Notes) == 0 || !strings.Contains(rep.Notes[0], "synthetic") {
		t.Fatalf("notes = %v", rep.Notes)
	}
}

func TestAnalyzeNamespaceKeepsNotReadyNodes(t *testing.T) {
	now := time.Date(2026, 11, 4, 12, 0, 0, 0, time.UTC)
	rep := Analyze(DemoSnapshot(now), Options{Namespace: "payments", Now: now, Demo: true})
	if rep.Summary.PodsScanned != 2 {
		t.Fatalf("pods = %d", rep.Summary.PodsScanned)
	}
	if rep.Summary.CrashLoop != 1 || rep.Summary.OOMKilled != 0 || rep.Summary.HighRestarts != 0 {
		t.Fatalf("summary = %+v", rep.Summary)
	}
	if rep.Summary.NodesNotReady != 1 || rep.Summary.WarningEvents != 1 {
		t.Fatalf("summary = %+v", rep.Summary)
	}
	foundMount := false
	for _, f := range rep.Findings {
		if strings.Contains(f.Resource, "prometheus") || strings.Contains(f.Resource, "recommendations") || strings.Contains(f.Resource, "batch-worker") {
			t.Fatalf("out of scope finding %s", f.Resource)
		}
		if f.Category == CatWarning && strings.Contains(f.Phenomenon, "checkout-secrets") {
			foundMount = true
		}
	}
	if !foundMount {
		t.Fatal("expected FailedMount in payments")
	}
	var sawNodeNote bool
	for _, n := range rep.Notes {
		if strings.Contains(n, "cluster-scoped") {
			sawNodeNote = true
		}
	}
	if !sawNodeNote {
		t.Fatalf("notes = %v", rep.Notes)
	}
}

func TestAnalyzeOOMAndCrashOnDifferentContainers(t *testing.T) {
	pod := corev1.Pod{
		ObjectMeta: metav1.ObjectMeta{Namespace: "app", Name: "api"},
		Spec:       corev1.PodSpec{NodeName: "worker-a"},
		Status: corev1.PodStatus{
			Phase: corev1.PodRunning,
			ContainerStatuses: []corev1.ContainerStatus{
				{
					Name:         "api",
					RestartCount: 3,
					State:        corev1.ContainerState{Running: &corev1.ContainerStateRunning{}},
					LastTerminationState: corev1.ContainerState{Terminated: &corev1.ContainerStateTerminated{
						Reason: "OOMKilled", ExitCode: 137,
					}},
				},
				{
					Name:         "sidecar",
					RestartCount: 2,
					State:        corev1.ContainerState{Waiting: &corev1.ContainerStateWaiting{Reason: "CrashLoopBackOff"}},
					LastTerminationState: corev1.ContainerState{Terminated: &corev1.ContainerStateTerminated{
						Reason: "Error", ExitCode: 1, Message: "configmap app missing not found",
					}},
				},
			},
		},
	}
	rep := Analyze(Snapshot{Cluster: "c", Pods: []corev1.Pod{pod}}, Options{AllNamespaces: true})
	if rep.Summary.OOMKilled != 1 || rep.Summary.CrashLoop != 1 || rep.Summary.HighRestarts != 0 {
		t.Fatalf("summary = %+v findings=%d", rep.Summary, len(rep.Findings))
	}
}

func TestAnalyzeRestartThresholdAndSucceededPods(t *testing.T) {
	running := podWithRestarts("ns", "edge", 5, "Running")
	below := podWithRestarts("ns", "quiet", 4, "Running")
	done := podWithRestarts("ns", "job", 30, "Succeeded")
	done.Status.Phase = corev1.PodSucceeded
	rep := Analyze(Snapshot{Pods: []corev1.Pod{running, below, done}}, Options{
		AllNamespaces:    true,
		RestartThreshold: 5,
	})
	if rep.Summary.PodsScanned != 3 {
		t.Fatalf("scanned = %d", rep.Summary.PodsScanned)
	}
	if rep.Summary.HighRestarts != 1 || len(rep.Findings) != 1 {
		t.Fatalf("findings = %+v", rep.Findings)
	}
	if rep.Findings[0].Resource != "ns/edge" {
		t.Fatalf("resource = %s", rep.Findings[0].Resource)
	}
}

func TestAnalyzeInitContainerAndQuotes(t *testing.T) {
	pod := corev1.Pod{
		ObjectMeta: metav1.ObjectMeta{Namespace: "jobs", Name: "migrate"},
		Status: corev1.PodStatus{
			Phase: corev1.PodPending,
			InitContainerStatuses: []corev1.ContainerStatus{{
				Name:         "bad name",
				RestartCount: 4,
				State:        corev1.ContainerState{Waiting: &corev1.ContainerStateWaiting{Reason: "CrashLoopBackOff"}},
				LastTerminationState: corev1.ContainerState{Terminated: &corev1.ContainerStateTerminated{
					Reason: "Error", ExitCode: 1, Message: "liveness probe said no",
				}},
			}},
		},
	}
	rep := Analyze(Snapshot{Pods: []corev1.Pod{pod}}, Options{AllNamespaces: true})
	if len(rep.Findings) != 1 || rep.Findings[0].Category != CatCrashLoop {
		t.Fatalf("findings = %+v", rep.Findings)
	}
	if !strings.Contains(rep.Findings[0].Phenomenon, "bad name (init)") {
		t.Fatalf("phenomenon = %s", rep.Findings[0].Phenomenon)
	}
	if !strings.Contains(rep.Findings[0].LikelyCause, "probe") {
		t.Fatalf("cause = %s", rep.Findings[0].LikelyCause)
	}
	if !strings.Contains(rep.Findings[0].NextStep, `"bad name"`) {
		t.Fatalf("next = %s", rep.Findings[0].NextStep)
	}
}

func TestAnalyzeEventWindowLimitAndNodeRules(t *testing.T) {
	now := time.Date(2026, 11, 4, 12, 0, 0, 0, time.UTC)
	events := []corev1.Event{
		warningEvent("ns", "new", "Pod", "ns", "api", "Failed", "boom", 2, now.Add(-10*time.Minute)),
		warningEvent("ns", "edge", "Pod", "ns", "api", "Failed", "on the edge", 1, now.Add(-time.Hour)),
		warningEvent("ns", "old", "Pod", "ns", "api", "Failed", "too old", 1, now.Add(-time.Hour-time.Second)),
		warningEvent("ns", "future", "Pod", "ns", "api", "Failed", "from the future", 1, now.Add(2*time.Minute)),
		{
			ObjectMeta:     metav1.ObjectMeta{Namespace: "ns", Name: "series"},
			InvolvedObject: corev1.ObjectReference{Kind: "Pod", Namespace: "ns", Name: "api"},
			Type:           corev1.EventTypeWarning,
			Reason:         "Unhealthy",
			Message:        "Liveness probe failed",
			Series: &corev1.EventSeries{
				Count:            9,
				LastObservedTime: metav1.NewMicroTime(now.Add(-2 * time.Minute)),
			},
		},
		{
			ObjectMeta: metav1.ObjectMeta{
				Namespace:         "other",
				Name:              "cluster",
				CreationTimestamp: metav1.NewTime(now.Add(-5 * time.Minute)),
			},
			Type:    corev1.EventTypeWarning,
			Reason:  "NodeNotReady",
			Message: "node vanished",
		},
	}
	readyPressured := readyNode("ok")
	readyPressured.Status.Conditions = append(readyPressured.Status.Conditions, corev1.NodeCondition{
		Type: corev1.NodeDiskPressure, Status: corev1.ConditionTrue,
	})
	// readyNode already sets DiskPressure False first. Replace conditions.
	readyPressured.Status.Conditions = []corev1.NodeCondition{
		{Type: corev1.NodeReady, Status: corev1.ConditionTrue},
		{Type: corev1.NodeDiskPressure, Status: corev1.ConditionTrue},
	}
	missing := corev1.Node{ObjectMeta: metav1.ObjectMeta{Name: "no-ready"}}

	rep := Analyze(Snapshot{
		Nodes:  []corev1.Node{readyPressured, missing},
		Events: events,
	}, Options{Namespace: "ns", Now: now, Since: time.Hour, EventLimit: 2})

	if rep.Summary.NodesNotReady != 1 {
		t.Fatalf("not ready = %d", rep.Summary.NodesNotReady)
	}
	if rep.Findings[0].Resource != "no-ready" || !strings.Contains(rep.Findings[0].Phenomenon, "ReadyConditionMissing") {
		t.Fatalf("node finding = %+v", rep.Findings[0])
	}
	// In window: new, edge (exactly 1h), series. Old and future dropped.
	// Cluster event has empty involved namespace; object namespace is "other",
	// so it is outside namespace ns.
	if rep.Summary.WarningEvents != 3 {
		t.Fatalf("warnings = %d", rep.Summary.WarningEvents)
	}
	if rep.Summary.WarningEventsShown != 2 {
		t.Fatalf("shown = %d", rep.Summary.WarningEventsShown)
	}
	var sawLimit bool
	for _, n := range rep.Notes {
		if strings.Contains(n, "Showing 2 of 3") {
			sawLimit = true
		}
	}
	if !sawLimit {
		t.Fatalf("notes = %v", rep.Notes)
	}
	// Newest first among the shown pair: series (2m) then new (10m). Edge is 1h.
	var eventsShown []Finding
	for _, f := range rep.Findings {
		if f.Category == CatWarning {
			eventsShown = append(eventsShown, f)
		}
	}
	if len(eventsShown) != 2 || detailValue(eventsShown[0], "reason") != "Unhealthy" {
		t.Fatalf("shown events = %+v", eventsShown)
	}
	if detailValue(eventsShown[0], "count") != "9" {
		t.Fatalf("series count = %s", detailValue(eventsShown[0], "count"))
	}
}

func TestAnalyzeEmptyScope(t *testing.T) {
	rep := Analyze(Snapshot{Cluster: "c", Nodes: []corev1.Node{readyNode("a")}}, Options{AllNamespaces: true})
	if len(rep.Findings) != 0 {
		t.Fatalf("findings = %+v", rep.Findings)
	}
	if len(rep.Notes) == 0 || !strings.Contains(rep.Notes[len(rep.Notes)-1], "No CrashLoopBackOff") {
		t.Fatalf("notes = %v", rep.Notes)
	}
}

func TestAnalyzeClusterScopedWarningStaysVisible(t *testing.T) {
	now := time.Date(2026, 11, 4, 12, 0, 0, 0, time.UTC)
	ev := corev1.Event{
		ObjectMeta: metav1.ObjectMeta{Name: "node-event", CreationTimestamp: metav1.NewTime(now.Add(-time.Minute))},
		InvolvedObject: corev1.ObjectReference{
			Kind: "Node", Name: "worker-3",
		},
		Type:    corev1.EventTypeWarning,
		Reason:  "NodeNotReady",
		Message: "node down",
	}
	rep := Analyze(Snapshot{Events: []corev1.Event{ev}}, Options{Namespace: "payments", Now: now})
	if rep.Summary.WarningEvents != 1 {
		t.Fatalf("warnings = %d", rep.Summary.WarningEvents)
	}
	if !strings.Contains(rep.Findings[0].NextStep, "kubectl describe node worker-3") {
		t.Fatalf("next = %s", rep.Findings[0].NextStep)
	}
}

func podWithRestarts(ns, name string, restarts int32, state string) corev1.Pod {
	st := corev1.ContainerStatus{Name: "app", RestartCount: restarts}
	if state == "Running" || state == "Succeeded" {
		st.State.Running = &corev1.ContainerStateRunning{}
	}
	phase := corev1.PodRunning
	if state == "Succeeded" {
		phase = corev1.PodSucceeded
	}
	return corev1.Pod{
		ObjectMeta: metav1.ObjectMeta{Namespace: ns, Name: name},
		Status: corev1.PodStatus{
			Phase:             phase,
			ContainerStatuses: []corev1.ContainerStatus{st},
		},
	}
}

func detailValue(f Finding, name string) string {
	for _, d := range f.Details {
		if d.Name == name {
			return d.Value
		}
	}
	return ""
}
