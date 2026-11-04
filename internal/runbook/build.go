package runbook

import (
	"fmt"
	"sort"
	"strconv"
	"strings"
)

// Build fills a rollback checklist from a snapshot.
// A positive Options.Revision must exist and must not be the current revision.
// Revision 0 selects the highest recorded revision below current.
func Build(snap Snapshot, opts Options) (Report, error) {
	if snap.Namespace == "" || snap.Name == "" {
		return Report{}, fmt.Errorf("namespace and name are required")
	}
	if snap.Kind == "" {
		snap.Kind = KindDeployment
	}
	target, err := selectTarget(snap, opts.Revision)
	if err != nil {
		return Report{}, err
	}
	ref := snap.Kind + " " + snap.Namespace + "/" + snap.Name
	kname := kubectlKind(snap.Kind)
	currentImage := formatImages(snap.Images)
	targetImage := formatImages(target.Images)
	if targetImage == "" {
		targetImage = "unknown"
	}

	steps := []Step{
		stateStep(snap, ref, currentImage, target, targetImage),
		{
			Severity:    SevInfo,
			Category:    StepPreCheck,
			Resource:    ref,
			Phenomenon:  fmt.Sprintf("Confirm you are in the right cluster and that %s %s/%s is the failed release before changing it.", snap.Kind, snap.Namespace, snap.Name),
			LikelyCause: "A rollback in the wrong context is worse than the failed release.",
			NextStep:    fmt.Sprintf("kubectl config current-context; kubectl get %s %s -n %s -o wide", kname, snap.Name, snap.Namespace),
		},
		{
			Severity:    SevInfo,
			Category:    StepPreCheck,
			Resource:    ref,
			Phenomenon:  fmt.Sprintf("Read rollout history and confirm revision %d is the last good template (%s).", target.Number, targetImage),
			LikelyCause: "undo without reading history can return to an older bad revision.",
			NextStep:    fmt.Sprintf("kubectl rollout history %s/%s -n %s", kname, snap.Name, snap.Namespace),
		},
		{
			Severity:    SevWarning,
			Category:    StepRollback,
			Resource:    ref,
			Phenomenon:  fmt.Sprintf("Roll %s %s/%s back to revision %d.", snap.Kind, snap.Namespace, snap.Name, target.Number),
			LikelyCause: "rollout undo restores that revision's pod template, including its image.",
			NextStep:    fmt.Sprintf("kubectl rollout undo %s/%s -n %s --to-revision=%d", kname, snap.Name, snap.Namespace, target.Number),
			Details: []Detail{
				{Name: "revision", Value: strconv.FormatInt(target.Number, 10)},
				{Name: "image", Value: targetImage},
			},
		},
		imageStep(snap, ref, kname, target.Images, targetImage),
		{
			Severity:    SevInfo,
			Category:    StepVerify,
			Resource:    ref,
			Phenomenon:  "Wait until the rollback reports that it has successfully rolled out.",
			LikelyCause: "Do not close the incident while new pods are still starting.",
			NextStep:    fmt.Sprintf("kubectl rollout status %s/%s -n %s --timeout=180s", kname, snap.Name, snap.Namespace),
		},
		{
			Severity:    SevInfo,
			Category:    StepVerify,
			Resource:    ref,
			Phenomenon:  fmt.Sprintf("Ready pods should return to %d and the image should be %s.", snap.Desired, targetImage),
			LikelyCause: "rollout status can succeed while a later probe still fails.",
			NextStep:    podGet(snap),
		},
		{
			Severity:    SevInfo,
			Category:    StepVerify,
			Resource:    ref,
			Phenomenon:  "Scan recent events for BackOff, Unhealthy, or Failed after the rollback.",
			LikelyCause: "A fresh warning means the previous image is not healthy either.",
			NextStep:    fmt.Sprintf("kubectl get events -n %s --sort-by=.lastTimestamp", snap.Namespace),
		},
		{
			Severity:    SevWarning,
			Category:    StepAbort,
			Resource:    ref,
			Phenomenon:  fmt.Sprintf("Abort if ready stays below %d after 180s, or CrashLoopBackOff returns on %s.", snap.Desired, targetImage),
			LikelyCause: "A second undo without a new fact only thrashes revisions.",
			NextStep:    fmt.Sprintf("Stop. Capture kubectl describe %s %s -n %s and the events above, then page the service owner. Do not run rollout undo again.", kname, snap.Name, snap.Namespace),
		},
	}

	notes := []string{"Commands are printed for the on-call engineer. This command does not change the cluster."}
	if opts.Demo {
		notes = append([]string{"Demo mode is a failed checkout release. It does not contact a cluster."}, notes...)
	}
	if opts.Revision == 0 {
		notes = append(notes, fmt.Sprintf("Target revision %d is the highest recorded revision below %d.", target.Number, snap.CurrentRevision))
	}
	notes = append(notes, snap.Notes...)

	return Report{
		Demo:            opts.Demo,
		Cluster:         snap.Cluster,
		Kind:            snap.Kind,
		Namespace:       snap.Namespace,
		Name:            snap.Name,
		CurrentRevision: snap.CurrentRevision,
		TargetRevision:  target.Number,
		CurrentImage:    currentImage,
		TargetImage:     targetImage,
		Desired:         snap.Desired,
		Ready:           snap.Ready,
		Updated:         snap.Updated,
		Steps:           steps,
		Notes:           notes,
	}, nil
}

func selectTarget(snap Snapshot, want int64) (Revision, error) {
	if len(snap.Revisions) == 0 {
		return Revision{}, fmt.Errorf("no recorded revisions for %s %s/%s", snap.Kind, snap.Namespace, snap.Name)
	}
	if want > 0 {
		rev, ok := findRevision(snap.Revisions, want)
		if !ok {
			return Revision{}, fmt.Errorf("revision %d not found; known revisions: %s", want, revisionList(snap.Revisions))
		}
		if rev.Number == snap.CurrentRevision {
			return Revision{}, fmt.Errorf("revision %d is the current revision", want)
		}
		return rev, nil
	}
	rev, ok := previousRevision(snap.Revisions, snap.CurrentRevision)
	if !ok {
		return Revision{}, fmt.Errorf("no revision older than %d for %s %s/%s", snap.CurrentRevision, snap.Kind, snap.Namespace, snap.Name)
	}
	return rev, nil
}

func findRevision(revs []Revision, n int64) (Revision, bool) {
	for _, r := range revs {
		if r.Number == n {
			return r, true
		}
	}
	return Revision{}, false
}

func previousRevision(revs []Revision, current int64) (Revision, bool) {
	var best Revision
	ok := false
	for _, r := range revs {
		if r.Number < current && (!ok || r.Number > best.Number) {
			best = r
			ok = true
		}
	}
	return best, ok
}

func revisionList(revs []Revision) string {
	nums := make([]int, 0, len(revs))
	for _, r := range revs {
		nums = append(nums, int(r.Number))
	}
	sort.Ints(nums)
	parts := make([]string, len(nums))
	for i, n := range nums {
		parts[i] = strconv.Itoa(n)
	}
	return strings.Join(parts, ", ")
}

func stateStep(snap Snapshot, ref, currentImage string, target Revision, targetImage string) Step {
	health := fmt.Sprintf("desired %d, ready %d, updated %d", snap.Desired, snap.Ready, snap.Updated)
	phenomenon := fmt.Sprintf("%s is at revision %d, image %s (%s).", ref, snap.CurrentRevision, currentImage, health)
	if snap.Failure != "" {
		phenomenon += " " + snap.Failure
	}
	sev := SevInfo
	why := fmt.Sprintf("Revision %d (%s) is the recorded rollback target.", target.Number, targetImage)
	if snap.Ready < snap.Desired {
		sev = SevCritical
		why = fmt.Sprintf("The current revision is not Ready. Revision %d (%s) is the recorded rollback target.", target.Number, targetImage)
	}
	return Step{
		Severity:    sev,
		Category:    StepState,
		Resource:    ref,
		Phenomenon:  phenomenon,
		LikelyCause: why,
		NextStep:    fmt.Sprintf("kubectl get %s %s -n %s -o wide", kubectlKind(snap.Kind), snap.Name, snap.Namespace),
		Details: []Detail{
			{Name: "revision", Value: strconv.FormatInt(snap.CurrentRevision, 10)},
			{Name: "image", Value: currentImage},
			{Name: "targetRevision", Value: strconv.FormatInt(target.Number, 10)},
			{Name: "targetImage", Value: targetImage},
		},
	}
}

func imageStep(snap Snapshot, ref, kname string, target []ContainerImage, targetImage string) Step {
	pairs := setImagePairs(snap.Images, target)
	cmd := fmt.Sprintf("kubectl set image %s/%s %s -n %s", kname, snap.Name, pairs, snap.Namespace)
	return Step{
		Severity:    SevWarning,
		Category:    StepRollback,
		Resource:    ref,
		Phenomenon:  fmt.Sprintf("If rollout undo is not available, set the image back to %s.", targetImage),
		LikelyCause: "set image creates a new revision with the known-good image instead of reusing the old ReplicaSet or ControllerRevision.",
		NextStep:    cmd,
		Details:     []Detail{{Name: "image", Value: targetImage}},
	}
}

func setImagePairs(current, target []ContainerImage) string {
	if len(target) == 0 {
		return ""
	}
	byName := map[string]string{}
	for _, c := range target {
		byName[c.Name] = c.Image
	}
	var pairs []string
	if len(current) == 0 {
		for _, c := range target {
			pairs = append(pairs, c.Name+"="+c.Image)
		}
	} else {
		for _, c := range current {
			img := byName[c.Name]
			if img == "" {
				img = c.Image
			}
			pairs = append(pairs, c.Name+"="+img)
		}
	}
	return strings.Join(pairs, " ")
}

func podGet(snap Snapshot) string {
	if snap.Selector != "" {
		return fmt.Sprintf("kubectl get pods -n %s -l %s -o wide", snap.Namespace, snap.Selector)
	}
	return fmt.Sprintf("kubectl get pods -n %s -o wide", snap.Namespace)
}

func kubectlKind(kind string) string {
	switch kind {
	case KindStatefulSet:
		return "statefulset"
	default:
		return "deployment"
	}
}

func formatImages(images []ContainerImage) string {
	if len(images) == 0 {
		return ""
	}
	parts := make([]string, 0, len(images))
	for _, c := range images {
		label := c.Name
		if c.Init {
			label += " (init)"
		}
		parts = append(parts, label+"="+c.Image)
	}
	return strings.Join(parts, ", ")
}
