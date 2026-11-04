package runbook

import (
	"strings"
	"testing"
)

func TestDemoRunbook(t *testing.T) {
	rep, err := Build(DemoSnapshot(), Options{Demo: true})
	if err != nil {
		t.Fatal(err)
	}
	if rep.Namespace != "shop" || rep.Name != "checkout" || rep.CurrentRevision != 12 || rep.TargetRevision != 11 {
		t.Fatalf("report = %+v", rep)
	}
	if rep.CurrentImage != "checkout=ghcr.io/example/checkout:1.9.0" || rep.TargetImage != "checkout=ghcr.io/example/checkout:1.8.4" {
		t.Fatalf("images current=%s target=%s", rep.CurrentImage, rep.TargetImage)
	}
	blob := render(rep)
	for _, want := range []string{
		"CrashLoopBackOff",
		"STRIPE_API_KEY",
		"kubectl rollout undo deployment/checkout -n shop --to-revision=11",
		"kubectl set image deployment/checkout checkout=ghcr.io/example/checkout:1.8.4 -n shop",
		"kubectl rollout status deployment/checkout -n shop --timeout=180s",
		"kubectl get pods -n shop -l app=checkout -o wide",
		"kubectl get events -n shop --sort-by=.lastTimestamp",
		"Do not run rollout undo again",
		"kubectl rollout history deployment/checkout -n shop",
	} {
		if !strings.Contains(blob, want) {
			t.Fatalf("missing %q\n%s", want, blob)
		}
	}
	if rep.Steps[0].Category != StepState || rep.Steps[0].Severity != SevCritical {
		t.Fatalf("first step = %+v", rep.Steps[0])
	}
	if rep.Steps[len(rep.Steps)-1].Category != StepAbort {
		t.Fatalf("last step = %+v", rep.Steps[len(rep.Steps)-1])
	}
	if !strings.Contains(rep.Notes[0], "does not contact a cluster") {
		t.Fatalf("notes = %v", rep.Notes)
	}
}

func TestExplicitRevisionAndErrors(t *testing.T) {
	snap := DemoSnapshot()
	rep, err := Build(snap, Options{Revision: 11})
	if err != nil {
		t.Fatal(err)
	}
	if rep.TargetRevision != 11 {
		t.Fatalf("target = %d", rep.TargetRevision)
	}
	if _, err := Build(snap, Options{Revision: 12}); err == nil || !strings.Contains(err.Error(), "current") {
		t.Fatalf("err = %v", err)
	}
	if _, err := Build(snap, Options{Revision: 9}); err == nil || !strings.Contains(err.Error(), "not found") {
		t.Fatalf("err = %v", err)
	}
	snap.Revisions = []Revision{{Number: 12, Images: snap.Images}}
	if _, err := Build(snap, Options{}); err == nil || !strings.Contains(err.Error(), "older") {
		t.Fatalf("err = %v", err)
	}
}

func TestStatefulSetCommands(t *testing.T) {
	snap := DemoSnapshot()
	snap.Kind = KindStatefulSet
	rep, err := Build(snap, Options{})
	if err != nil {
		t.Fatal(err)
	}
	blob := render(rep)
	if !strings.Contains(blob, "kubectl rollout undo statefulset/checkout -n shop --to-revision=11") {
		t.Fatalf("missing sts undo\n%s", blob)
	}
	if !strings.Contains(blob, "kubectl set image statefulset/checkout checkout=ghcr.io/example/checkout:1.8.4 -n shop") {
		t.Fatalf("missing sts set image\n%s", blob)
	}
}

func render(rep Report) string {
	var b strings.Builder
	for _, s := range rep.Steps {
		b.WriteString(s.Category)
		b.WriteByte(' ')
		b.WriteString(s.Phenomenon)
		b.WriteByte('\n')
		b.WriteString(s.LikelyCause)
		b.WriteByte('\n')
		b.WriteString(s.NextStep)
		b.WriteByte('\n')
	}
	return b.String()
}
