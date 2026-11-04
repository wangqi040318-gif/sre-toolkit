package pulse

import (
	"fmt"
	"strings"
	"time"

	corev1 "k8s.io/api/core/v1"
)

type containerSignal struct {
	name        string
	init        bool
	restarts    int32
	state       string
	oom         bool
	crash       bool
	lastReason  string
	lastMessage string
	lastExit    int32
	memLimit    string
	node        string
}

func (s containerSignal) displayName() string {
	if s.init {
		return s.name + " (init)"
	}
	return s.name
}

func crashPhenomenon(pod string, s containerSignal) string {
	if s.lastReason == "" && s.lastMessage == "" {
		return fmt.Sprintf("Container %s in %s is CrashLoopBackOff after %d restarts.", s.displayName(), pod, s.restarts)
	}
	msg := oneLine(s.lastMessage, 180)
	if msg == "" {
		return fmt.Sprintf("Container %s in %s is CrashLoopBackOff after %d restarts (last exit %d %s).", s.displayName(), pod, s.restarts, s.lastExit, s.lastReason)
	}
	return fmt.Sprintf("Container %s in %s is CrashLoopBackOff after %d restarts (last exit %d %s: %s).", s.displayName(), pod, s.restarts, s.lastExit, s.lastReason, msg)
}

func crashCause(s containerSignal) string {
	msg := strings.ToLower(s.lastMessage + " " + s.lastReason)
	switch {
	case strings.Contains(msg, "missing env") || strings.Contains(msg, "environment variable"):
		return "The process exits during startup because required configuration (an environment variable or Secret) is missing."
	case strings.Contains(msg, "secret") && (strings.Contains(msg, "not found") || strings.Contains(msg, "notfound")):
		return "The container cannot start because a Secret it mounts or reads is missing."
	case strings.Contains(msg, "configmap") && strings.Contains(msg, "not found"):
		return "The container cannot start because a ConfigMap it mounts or reads is missing."
	case strings.Contains(msg, "connection refused") || strings.Contains(msg, "no such host") || strings.Contains(msg, "i/o timeout"):
		return "The process starts, then fails while reaching a dependency (connection refused, DNS, or timeout)."
	case strings.Contains(msg, "probe") || strings.Contains(msg, "liveness") || strings.Contains(msg, "startup"):
		return "A liveness or startup probe is failing, so the kubelet keeps restarting the container."
	default:
		return "The process is exiting repeatedly during startup. Typical causes are bad config, a missing dependency, or a probe that kills a slow start."
	}
}

func crashNext(ns, name string, s containerSignal) string {
	base := fmt.Sprintf("kubectl logs -n %s %s -c %s --previous; kubectl describe pod -n %s %s", shellName(ns), shellName(name), shellName(s.name), shellName(ns), shellName(name))
	msg := strings.ToLower(s.lastMessage)
	if strings.Contains(msg, "secret") || strings.Contains(msg, "missing env") || strings.Contains(msg, "configmap") {
		return base + "; confirm the Secret or ConfigMap named in the log exists in this namespace"
	}
	return base
}

func oomPhenomenon(pod string, s containerSignal) string {
	limit := ""
	if s.memLimit != "" {
		limit = fmt.Sprintf(" (memory limit %s)", s.memLimit)
	}
	return fmt.Sprintf("Container %s in %s was OOMKilled%s and has restarted %d times; current state is %s.", s.displayName(), pod, limit, s.restarts, s.state)
}

func oomCause(s containerSignal) string {
	base := "The container crossed its memory limit and the kernel killed it."
	if s.memLimit != "" {
		base = fmt.Sprintf("The container crossed its memory limit (%s) and the kernel killed it.", s.memLimit)
	}
	if s.crash || s.state == "CrashLoopBackOff" {
		return base + " The kubelet is crash-looping it, so the pod will not recover until usage drops or the limit is raised."
	}
	return base + " It is running again, but the same spike will kill it unless the limit or the workload changes."
}

func oomNext(ns, name string, s containerSignal) string {
	return fmt.Sprintf("kubectl describe pod -n %s %s; kubectl logs -n %s %s -c %s --previous; kubectl top pod -n %s %s --containers", shellName(ns), shellName(name), shellName(ns), shellName(name), shellName(s.name), shellName(ns), shellName(name))
}

func highPhenomenon(pod string, s containerSignal, threshold int32) string {
	base := fmt.Sprintf("Container %s in %s has restarted %d times (threshold %d) and is currently %s.", s.displayName(), pod, s.restarts, threshold, s.state)
	if s.lastReason == "" {
		return base
	}
	msg := oneLine(s.lastMessage, 180)
	if msg == "" {
		return fmt.Sprintf("%s Last exit was %d (%s).", base, s.lastExit, s.lastReason)
	}
	return fmt.Sprintf("%s Last exit was %d (%s): %s.", base, s.lastExit, s.lastReason, msg)
}

func highCause(s containerSignal) string {
	msg := strings.ToLower(s.lastMessage)
	switch {
	case strings.Contains(msg, "connection refused") || strings.Contains(msg, "no such host") || strings.Contains(msg, "i/o timeout") || strings.Contains(msg, "timeout"):
		return "Restarts are above the threshold but the container is running now. The previous exit shows a dependency connection failure, which usually means a short downstream outage rather than a current crash loop."
	default:
		return "Restarts are above the threshold but the container is not crash-looping. This often follows a bad rollout, a probe flap, or a brief dependency outage."
	}
}

func highNext(ns, name string, s containerSignal) string {
	return fmt.Sprintf("kubectl describe pod -n %s %s; kubectl logs -n %s %s -c %s --previous", shellName(ns), shellName(name), shellName(ns), shellName(name), shellName(s.name))
}

func nodePhenomenon(n corev1.Node, ready corev1.NodeCondition, pressure []string) string {
	reason := ready.Reason
	if reason == "" {
		reason = string(ready.Status)
	}
	msg := oneLine(ready.Message, 180)
	var head string
	if msg == "" {
		head = fmt.Sprintf("Node %s is NotReady (%s).", n.Name, reason)
	} else {
		head = fmt.Sprintf("Node %s is NotReady (%s: %s).", n.Name, reason, msg)
	}
	pressureText := "No pressure condition is True."
	if len(pressure) > 0 {
		pressureText = "Pressure true: " + strings.Join(pressure, ", ") + "."
	}
	if n.Spec.Unschedulable {
		pressureText += " The node is also cordoned."
	}
	return head + " " + pressureText
}

func nodeCause(pressure []string) string {
	switch {
	case containsStr(pressure, string(corev1.NodeDiskPressure)):
		return "DiskPressure is true, so the kubelet is protecting the node by marking it NotReady and evicting pods that use ephemeral storage. Free disk or unused images, then confirm the kubelet heartbeat returns."
	case containsStr(pressure, string(corev1.NodeMemoryPressure)):
		return "MemoryPressure is true. The kubelet is under memory pressure and may evict pods. Free memory or move workloads before expecting Ready to return."
	case containsStr(pressure, string(corev1.NodePIDPressure)):
		return "PIDPressure is true. The node is out of process IDs, which stops new containers and can fail the kubelet heartbeat."
	case containsStr(pressure, string(corev1.NodeNetworkUnavailable)):
		return "The network plugin reports the node network is unavailable, so the node cannot become Ready."
	default:
		return "The Ready condition is not True and no resource-pressure condition is set. The kubelet or container runtime is usually down, or the node cannot reach the API server."
	}
}

func nodeNext(name string, pressure []string) string {
	base := fmt.Sprintf("kubectl describe node %s; kubectl get pods -A --field-selector spec.nodeName=%s", shellName(name), shellName(name))
	switch {
	case containsStr(pressure, string(corev1.NodeDiskPressure)):
		return base + "; on the node, free ephemeral storage and unused images, then confirm kubelet is Ready"
	case containsStr(pressure, string(corev1.NodeMemoryPressure)):
		return base + "; on the node, identify the memory consumer and move or resize workloads before expecting Ready"
	case containsStr(pressure, string(corev1.NodePIDPressure)):
		return base + "; on the node, find the PID spike and restart or evict the offender"
	case containsStr(pressure, string(corev1.NodeNetworkUnavailable)):
		return base + "; check the CNI plugin and node routes, then restart the network daemon if it is wedged"
	default:
		return base + "; check kubelet and the container runtime, and cordon the node if workloads should move"
	}
}

func eventPhenomenon(e corev1.Event, now time.Time) string {
	ref := e.InvolvedObject
	kind := ref.Kind
	if kind == "" {
		kind = "object"
	}
	id := resID(ref.Namespace, ref.Name)
	if id == "" {
		id = resID(e.Namespace, e.Name)
	}
	n := eventCount(e)
	unit := "times"
	if n == 1 {
		unit = "time"
	}
	msg := oneLine(e.Message, 300)
	head := fmt.Sprintf("Warning %s for %s %s (%d %s, %s)", e.Reason, kind, id, n, unit, age(now, eventTime(e)))
	if msg == "" {
		return head + "."
	}
	return head + ": " + msg
}

func eventCause(e corev1.Event) string {
	msg := strings.ToLower(e.Message)
	switch e.Reason {
	case "FailedScheduling":
		return "The scheduler cannot place this object. The message usually means not enough CPU or memory, untolerated taints, affinity rules, or a volume that is still unbound."
	case "FailedMount", "FailedAttachVolume":
		if strings.Contains(msg, "secret") || strings.Contains(msg, "configmap") {
			return "The pod references a Secret or ConfigMap that is missing or not permitted in this namespace."
		}
		return "A volume could not be attached or mounted. Check the claim, the volume plugin, and node capacity."
	case "Unhealthy":
		return "A probe failed. The container may be too slow to start, or the process is failing its health check."
	case "BackOff":
		return "The kubelet is backing off restarts after repeated container failures. Read the previous container logs for the exit reason."
	case "Failed":
		return "A container or sync step failed. The event message is the concrete error; the reason alone is only a hint."
	case "NodeNotReady":
		return "Workloads on this node are affected because the node is not Ready."
	case "Evicted":
		return "The kubelet evicted the pod under node pressure (disk, memory, or inodes). The pod will stay down until the node has room or the workload moves."
	case "OOMKilling":
		return "A cgroup or the node killed a process that exceeded its memory limit."
	case "FailedCreate", "FailedCreatePodSandBox":
		return "The pod sandbox or object could not be created. Common causes are CNI failures, a wedged runtime, or a quota."
	default:
		if strings.Contains(msg, "secret") && strings.Contains(msg, "not found") {
			return "A referenced Secret is missing. Create it or fix the pod spec before retrying."
		}
		return "A warning event was recorded. Treat the message as the primary signal and the reason as a hint."
	}
}

func eventNext(e corev1.Event) string {
	ref := e.InvolvedObject
	kind := ref.Kind
	name := ref.Name
	ns := ref.Namespace
	if name == "" {
		kind = "event"
		name = e.Name
		ns = e.Namespace
	}
	if kind == "" {
		kind = "event"
	}
	return describeCmd(kind, ns, name)
}

func describeCmd(kind, ns, name string) string {
	k := strings.ToLower(kind)
	switch k {
	case "node", "namespace", "persistentvolume", "pv":
		return fmt.Sprintf("kubectl describe %s %s", k, shellName(name))
	default:
		if ns == "" {
			return fmt.Sprintf("kubectl describe %s %s", k, shellName(name))
		}
		return fmt.Sprintf("kubectl describe %s -n %s %s", k, shellName(ns), shellName(name))
	}
}

func containsStr(list []string, want string) bool {
	for _, s := range list {
		if s == want {
			return true
		}
	}
	return false
}
