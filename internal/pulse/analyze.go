package pulse

import (
	"fmt"
	"sort"
	"strconv"
	"strings"

	corev1 "k8s.io/api/core/v1"
)

// Analyze turns a snapshot into findings. Pod signals are one finding per
// category per pod: OOMKilled hides CrashLoopBackOff and high restarts on
// the same container, because the memory kill is the cause of the loop.
func Analyze(snap Snapshot, opts Options) Report {
	opts = opts.withDefaults()
	cluster := snap.Cluster
	if cluster == "" {
		cluster = "unknown"
	}

	pods := append([]corev1.Pod(nil), snap.Pods...)
	sort.Slice(pods, func(i, j int) bool {
		if pods[i].Namespace != pods[j].Namespace {
			return pods[i].Namespace < pods[j].Namespace
		}
		return pods[i].Name < pods[j].Name
	})
	nodes := append([]corev1.Node(nil), snap.Nodes...)
	sort.Slice(nodes, func(i, j int) bool { return nodes[i].Name < nodes[j].Name })

	var findings []Finding
	seq := 0
	emit := func(f Finding) {
		f.seq = seq
		seq++
		findings = append(findings, f)
	}

	podsScanned := 0
	for _, pod := range pods {
		if !opts.includeNS(pod.Namespace) {
			continue
		}
		podsScanned++
		if pod.Status.Phase == corev1.PodSucceeded {
			continue
		}
		for _, f := range podFindings(pod, opts.RestartThreshold) {
			emit(f)
		}
	}

	notReady := 0
	for _, node := range nodes {
		ready, bad := readyCondition(node)
		if !bad {
			continue
		}
		notReady++
		emit(nodeFinding(node, ready))
	}

	matched := eventsInScope(snap.Events, opts)
	sort.SliceStable(matched, func(i, j int) bool {
		return eventTime(matched[i]).After(eventTime(matched[j]))
	})
	shown := matched
	if len(shown) > opts.EventLimit {
		shown = shown[:opts.EventLimit]
	}
	for _, ev := range shown {
		emit(eventFinding(ev, opts))
	}

	sort.SliceStable(findings, func(i, j int) bool {
		if rankSev(findings[i].Severity) != rankSev(findings[j].Severity) {
			return rankSev(findings[i].Severity) < rankSev(findings[j].Severity)
		}
		if rankCat(findings[i].Category) != rankCat(findings[j].Category) {
			return rankCat(findings[i].Category) < rankCat(findings[j].Category)
		}
		return findings[i].seq < findings[j].seq
	})

	summary := Summary{
		NodesTotal:         len(nodes),
		NodesNotReady:      notReady,
		PodsScanned:        podsScanned,
		WarningEvents:      len(matched),
		WarningEventsShown: len(shown),
	}
	for _, f := range findings {
		switch f.Category {
		case CatCrashLoop:
			summary.CrashLoop++
		case CatOOM:
			summary.OOMKilled++
		case CatHighRestarts:
			summary.HighRestarts++
		}
	}

	notes := make([]string, 0, 4)
	if opts.Demo {
		notes = append(notes, "Demo mode uses synthetic data and does not contact a cluster.")
	}
	if !opts.AllNamespaces && opts.Namespace != "" {
		notes = append(notes, "NotReady nodes are cluster-scoped and are included even when a namespace is selected.")
	}
	if len(matched) > len(shown) {
		notes = append(notes, fmt.Sprintf(
			"Showing %d of %d warning events from the last %s. Raise --event-limit to see more.",
			len(shown), len(matched), shortDur(opts.Since),
		))
	}
	if len(findings) == 0 {
		notes = append(notes, "No CrashLoopBackOff, OOMKilled, high-restart, NotReady node, or recent Warning signals in this scope.")
	}

	return Report{
		GeneratedAt:      opts.Now.UTC(),
		Demo:             opts.Demo,
		Cluster:          cluster,
		Scope:            opts.ScopeLabel(),
		Since:            opts.Since,
		RestartThreshold: opts.RestartThreshold,
		Summary:          summary,
		Findings:         findings,
		Notes:            notes,
	}
}

func podFindings(pod corev1.Pod, threshold int32) []Finding {
	sigs := containerSignals(pod)
	var oom, crash, high *containerSignal
	for i := range sigs {
		s := &sigs[i]
		switch {
		case s.oom:
			if oom == nil || s.restarts > oom.restarts {
				oom = s
			}
		case s.crash:
			if crash == nil || s.restarts > crash.restarts {
				crash = s
			}
		case s.restarts >= threshold:
			if high == nil || s.restarts > high.restarts {
				high = s
			}
		}
	}
	id := resID(pod.Namespace, pod.Name)
	var out []Finding
	if oom != nil {
		out = append(out, Finding{
			Severity:    SevCritical,
			Category:    CatOOM,
			Resource:    id,
			Phenomenon:  oomPhenomenon(id, *oom),
			LikelyCause: oomCause(*oom),
			NextStep:    oomNext(pod.Namespace, pod.Name, *oom),
			Details:     containerDetails(pod, *oom),
		})
	}
	if crash != nil {
		out = append(out, Finding{
			Severity:    SevCritical,
			Category:    CatCrashLoop,
			Resource:    id,
			Phenomenon:  crashPhenomenon(id, *crash),
			LikelyCause: crashCause(*crash),
			NextStep:    crashNext(pod.Namespace, pod.Name, *crash),
			Details:     containerDetails(pod, *crash),
		})
	}
	if high != nil {
		out = append(out, Finding{
			Severity:    SevWarning,
			Category:    CatHighRestarts,
			Resource:    id,
			Phenomenon:  highPhenomenon(id, *high, threshold),
			LikelyCause: highCause(*high),
			NextStep:    highNext(pod.Namespace, pod.Name, *high),
			Details:     containerDetails(pod, *high),
		})
	}
	return out
}

func containerSignals(pod corev1.Pod) []containerSignal {
	var out []containerSignal
	add := func(list []corev1.ContainerStatus, init bool, specs []corev1.Container) {
		for _, st := range list {
			sig := containerSignal{
				name:     st.Name,
				init:     init,
				restarts: st.RestartCount,
				state:    stateLabel(st),
				node:     pod.Spec.NodeName,
				memLimit: memoryLimit(specs, st.Name),
			}
			if st.State.Waiting != nil && st.State.Waiting.Reason == "CrashLoopBackOff" {
				sig.crash = true
			}
			if last := st.LastTerminationState.Terminated; last != nil {
				applyTerm(&sig, last)
			}
			if term := st.State.Terminated; term != nil {
				if term.Reason == "OOMKilled" {
					sig.oom = true
				}
				// The current termination is the latest exit when the
				// container is neither running nor waiting.
				if st.State.Running == nil && st.State.Waiting == nil {
					sig.lastReason = term.Reason
					sig.lastMessage = term.Message
					sig.lastExit = term.ExitCode
				}
			}
			out = append(out, sig)
		}
	}
	add(pod.Status.InitContainerStatuses, true, pod.Spec.InitContainers)
	add(pod.Status.ContainerStatuses, false, pod.Spec.Containers)
	return out
}

func applyTerm(sig *containerSignal, term *corev1.ContainerStateTerminated) {
	if term.Reason == "OOMKilled" {
		sig.oom = true
	}
	sig.lastReason = term.Reason
	sig.lastMessage = term.Message
	sig.lastExit = term.ExitCode
}

func stateLabel(st corev1.ContainerStatus) string {
	switch {
	case st.State.Waiting != nil:
		if st.State.Waiting.Reason != "" {
			return st.State.Waiting.Reason
		}
		return "Waiting"
	case st.State.Running != nil:
		return "Running"
	case st.State.Terminated != nil:
		if st.State.Terminated.Reason != "" {
			return st.State.Terminated.Reason
		}
		return "Terminated"
	default:
		return "Unknown"
	}
}

func memoryLimit(specs []corev1.Container, name string) string {
	for _, c := range specs {
		if c.Name != name || c.Resources.Limits == nil {
			continue
		}
		q, ok := c.Resources.Limits[corev1.ResourceMemory]
		if !ok || q.IsZero() {
			continue
		}
		return q.String()
	}
	return ""
}

func containerDetails(pod corev1.Pod, s containerSignal) []Detail {
	var d []Detail
	d = addDetail(d, "container", s.displayName())
	d = addDetail(d, "restarts", strconv.FormatInt(int64(s.restarts), 10))
	d = addDetail(d, "memoryLimit", s.memLimit)
	d = addDetail(d, "node", pod.Spec.NodeName)
	return d
}

func readyCondition(node corev1.Node) (corev1.NodeCondition, bool) {
	for _, c := range node.Status.Conditions {
		if c.Type != corev1.NodeReady {
			continue
		}
		if c.Status == corev1.ConditionTrue {
			return c, false
		}
		return c, true
	}
	return corev1.NodeCondition{
		Type:    corev1.NodeReady,
		Status:  corev1.ConditionUnknown,
		Reason:  "ReadyConditionMissing",
		Message: "node has no Ready condition",
	}, true
}

func pressureTrue(node corev1.Node) []string {
	want := []corev1.NodeConditionType{
		corev1.NodeDiskPressure,
		corev1.NodeMemoryPressure,
		corev1.NodePIDPressure,
		corev1.NodeNetworkUnavailable,
	}
	var out []string
	for _, kind := range want {
		for _, c := range node.Status.Conditions {
			if c.Type == kind && c.Status == corev1.ConditionTrue {
				out = append(out, string(kind))
			}
		}
	}
	return out
}

func nodeFinding(node corev1.Node, ready corev1.NodeCondition) Finding {
	pressure := pressureTrue(node)
	pressureValue := "none"
	if len(pressure) > 0 {
		pressureValue = strings.Join(pressure, ", ")
	}
	var details []Detail
	details = addDetail(details, "readyReason", ready.Reason)
	details = addDetail(details, "pressure", pressureValue)
	if node.Spec.Unschedulable {
		details = addDetail(details, "unschedulable", "true")
	}
	return Finding{
		Severity:    SevCritical,
		Category:    CatNodeNotReady,
		Resource:    node.Name,
		Phenomenon:  nodePhenomenon(node, ready, pressure),
		LikelyCause: nodeCause(pressure),
		NextStep:    nodeNext(node.Name, pressure),
		Details:     details,
	}
}

func eventsInScope(events []corev1.Event, opts Options) []corev1.Event {
	var out []corev1.Event
	for _, ev := range events {
		if ev.Type != corev1.EventTypeWarning {
			continue
		}
		ns := ev.Namespace
		if ev.InvolvedObject.Namespace != "" {
			ns = ev.InvolvedObject.Namespace
		}
		// Cluster-scoped warnings have an empty namespace and stay visible
		// inside a single-namespace pulse.
		if ns != "" && !opts.includeNS(ns) {
			continue
		}
		if !eventInWindow(opts.Now, eventTime(ev), opts.Since) {
			continue
		}
		out = append(out, ev)
	}
	return out
}

func eventFinding(ev corev1.Event, opts Options) Finding {
	ref := ev.InvolvedObject
	id := resID(ref.Namespace, ref.Name)
	if id == "" {
		id = resID(ev.Namespace, ev.Name)
	}
	var details []Detail
	details = addDetail(details, "reason", ev.Reason)
	details = addDetail(details, "count", strconv.FormatInt(int64(eventCount(ev)), 10))
	details = addDetail(details, "age", age(opts.Now, eventTime(ev)))
	return Finding{
		Severity:    SevWarning,
		Category:    CatWarning,
		Resource:    id,
		Phenomenon:  eventPhenomenon(ev, opts.Now),
		LikelyCause: eventCause(ev),
		NextStep:    eventNext(ev),
		Details:     details,
	}
}

func rankSev(s string) int {
	switch s {
	case SevCritical:
		return 0
	case SevWarning:
		return 1
	case SevInfo:
		return 2
	default:
		return 3
	}
}

func rankCat(c string) int {
	switch c {
	case CatOOM:
		return 0
	case CatCrashLoop:
		return 1
	case CatNodeNotReady:
		return 2
	case CatHighRestarts:
		return 3
	case CatWarning:
		return 4
	default:
		return 5
	}
}
