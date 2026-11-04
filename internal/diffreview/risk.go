package diffreview

import (
	"strconv"
	"strings"

	"k8s.io/apimachinery/pkg/api/resource"
)

type verdict struct {
	Level     string
	Why       string
	Checklist string
}

func classify(class, path, old, new string) verdict {
	switch class {
	case ClassImage:
		return imageRisk(old, new)
	case ClassResources:
		return resourceRisk(path, old, new)
	case ClassProbe:
		return probeRisk(path, old, new)
	case ClassReplicas:
		return replicaRisk(old, new)
	case ClassHPA:
		return hpaRisk(path, old, new)
	case ClassVolume:
		return volumeRisk(old, new)
	case ClassService:
		return serviceRisk(path, old, new)
	case ClassNetworkPolicy:
		return networkRisk(old, new)
	case ClassRBAC:
		return rbacRisk(path, old, new)
	case ClassSecurity:
		return securityRisk(path, old, new)
	case ClassEnv:
		return envRisk(old, new)
	default:
		return verdict{RiskMedium, "This field changed.", "Read the old and new values before accepting the release."}
	}
}

func imageRisk(old, new string) verdict {
	check := "Confirm the image is in the allowed registry, the tag is pinned (or digest-pinned), and this build is the one the release intends to roll."
	if new == "" {
		return verdict{RiskHigh, "The container image was removed.", check}
	}
	on, ot, od := splitImage(old)
	nn, nt, nd := splitImage(new)
	if (nt == "" || nt == "latest") && ot != "" && ot != "latest" {
		return verdict{RiskCritical, "The image moved from a pinned tag to latest or an implicit latest. The next pull can change the binary without another manifest edit.", check}
	}
	if od != "" && nd == "" {
		return verdict{RiskHigh, "The image digest pin was removed. The tag can now move under you.", check}
	}
	if on != nn {
		return verdict{RiskHigh, "The image repository changed, not just the tag. Confirm this is the same artifact and the registry is trusted.", check}
	}
	return verdict{RiskMedium, "The image tag changed. This is the rollout. Confirm the tag matches the release build.", check}
}

func splitImage(ref string) (name, tag, digest string) {
	rest := ref
	if i := strings.Index(ref, "@"); i >= 0 {
		digest = ref[i+1:]
		rest = ref[:i]
	}
	slash := strings.LastIndex(rest, "/")
	colon := strings.LastIndex(rest, ":")
	if colon > slash {
		return rest[:colon], rest[colon+1:], digest
	}
	return rest, "", digest
}

func resourceRisk(path, old, new string) verdict {
	check := "Compare the new requests and limits with recent usage (kubectl top or the metrics in sre waste) before the rollout."
	if new == "" && old != "" && strings.Contains(path, "limits") {
		return verdict{RiskHigh, "A limit was removed. The container can burst without a ceiling, which is an OOM or noisy-neighbor risk on the node.", check}
	}
	if old == "" {
		return verdict{RiskLow, "A request or limit was added.", check}
	}
	qo, err1 := resource.ParseQuantity(old)
	qn, err2 := resource.ParseQuantity(new)
	if err1 != nil || err2 != nil {
		return verdict{RiskMedium, "The resource quantity changed.", check}
	}
	cmp := qn.Cmp(qo)
	switch {
	case cmp < 0 && strings.Contains(path, "limits.memory"):
		return verdict{RiskHigh, "The memory limit went down. A rollout here is a common source of OOMKilled pods.", check}
	case cmp < 0 && strings.Contains(path, "limits"):
		return verdict{RiskMedium, "A limit went down. The process may throttle or fail under the same traffic.", check}
	case cmp < 0 && strings.Contains(path, "requests") && droppedHalf(qo, qn):
		return verdict{RiskMedium, "The request dropped by half or more. Scheduling and burst behavior will change.", check}
	case cmp < 0:
		return verdict{RiskLow, "The request or limit decreased.", check}
	case cmp > 0:
		return verdict{RiskLow, "The request or limit increased. This reserves or allows more capacity.", check}
	default:
		return verdict{RiskLow, "The quantity text changed but the value is the same.", check}
	}
}

func droppedHalf(old, new resource.Quantity) bool {
	if old.IsZero() {
		return false
	}
	// Compare in milli-units so CPU and memory both work.
	return new.MilliValue()*2 <= old.MilliValue()
}

func probeRisk(path, old, new string) verdict {
	check := "Confirm the probe matches the process startup time and the port that actually serves health. A bad liveness probe crash-loops a healthy pod."
	if new == "" && old != "" {
		switch {
		case strings.Contains(path, "livenessProbe"):
			return verdict{RiskHigh, "The liveness probe was removed. A wedged process will no longer be restarted.", check}
		case strings.Contains(path, "readinessProbe"):
			return verdict{RiskMedium, "The readiness probe was removed. Pods may receive traffic before they can serve.", check}
		default:
			return verdict{RiskMedium, "A startup probe was removed. Slow starts may now be killed by liveness.", check}
		}
	}
	if old == "" {
		return verdict{RiskLow, "A probe was added.", check}
	}
	if probeKind(old) != probeKind(new) {
		return verdict{RiskHigh, "The probe type changed (http, tcp, exec, or grpc). The success condition is different.", check}
	}
	return verdict{RiskMedium, "The probe timing or target changed. A tighter failure threshold or a new path can restart the pod or pull it out of Service.", check}
}

func probeKind(v string) string {
	if i := strings.IndexByte(v, ' '); i > 0 {
		return v[:i]
	}
	return v
}

func replicaRisk(old, new string) verdict {
	check := "Check the HPA, PodDisruptionBudget, and current traffic before accepting the new replica count."
	ni, errN := strconv.Atoi(new)
	oi, errO := strconv.Atoi(old)
	if new == "0" {
		return verdict{RiskHigh, "Replicas are scaled to zero. The workload will stop serving.", check}
	}
	if errN == nil && errO == nil && oi > 0 && ni*2 <= oi {
		return verdict{RiskHigh, "Replicas dropped by half or more. Capacity for the current traffic may disappear.", check}
	}
	return verdict{RiskMedium, "The replica count changed. Capacity and disruption budget should be rechecked.", check}
}

func hpaRisk(path, old, new string) verdict {
	check := "Confirm min and max still bracket real traffic, and that the metric target will not flap or pin the workload at the ceiling."
	if strings.HasSuffix(path, "maxReplicas") {
		oi, errO := strconv.Atoi(old)
		ni, errN := strconv.Atoi(new)
		if errO == nil && errN == nil && ni < oi {
			return verdict{RiskMedium, "maxReplicas went down. A traffic spike can now sit at a lower ceiling.", check}
		}
	}
	if strings.Contains(path, "target") || strings.Contains(path, "Utilization") {
		return verdict{RiskMedium, "The HPA metric target changed. The workload will scale sooner or later than it does today.", check}
	}
	return verdict{RiskMedium, "The HPA range changed.", check}
}

func volumeRisk(old, new string) verdict {
	check := "Confirm the volume still points at the right claim or path, and that a hostPath is an explicit exception."
	if strings.Contains(new, "hostPath") && !strings.Contains(old, "hostPath") {
		return verdict{RiskCritical, "A hostPath volume was added. The container can read or write the node filesystem.", check}
	}
	if strings.Contains(old, "persistentVolumeClaim") && new == "" {
		return verdict{RiskHigh, "A PVC mount was removed. The process will lose that data path.", check}
	}
	if strings.Contains(new, "persistentVolumeClaim") && claimName(old) != claimName(new) {
		return verdict{RiskHigh, "The PersistentVolumeClaim name changed. The pod may attach the wrong disk.", check}
	}
	if new == "" {
		return verdict{RiskHigh, "A volume or mount was removed.", check}
	}
	return verdict{RiskMedium, "A volume mount changed.", check}
}

func claimName(v string) string {
	const key = "claimName="
	i := strings.Index(v, key)
	if i < 0 {
		return ""
	}
	return strings.TrimSpace(v[i+len(key):])
}

func serviceRisk(path, old, new string) verdict {
	check := "Confirm clients, Ingress, and NetworkPolicy still match the Service type and ports."
	if path == "spec.type" {
		if new == "LoadBalancer" || new == "NodePort" {
			if old != new {
				return verdict{RiskHigh, "The Service is now exposed outside the cluster (" + new + "). That can publish a port and create a cloud load balancer.", check}
			}
		}
		return verdict{RiskMedium, "The Service type changed. Traffic policy and exposure change with it.", check}
	}
	if new == "" {
		return verdict{RiskHigh, "A Service port was removed.", check}
	}
	return verdict{RiskHigh, "A Service port or targetPort changed. Callers and probes still using the old number will miss.", check}
}

func networkRisk(old, new string) verdict {
	check := "Walk every ingress and egress peer against the callers that must still connect, and the ones that must not."
	if strings.Contains(new, "allow-all") && !strings.Contains(old, "allow-all") {
		return verdict{RiskCritical, "A NetworkPolicy rule now allows all peers. That opens traffic the policy used to restrict.", check}
	}
	if new == "" {
		return verdict{RiskHigh, "The NetworkPolicy spec was cleared.", check}
	}
	return verdict{RiskHigh, "The NetworkPolicy spec changed. A small selector edit can drop live traffic or open a path.", check}
}

func rbacRisk(path, old, new string) verdict {
	check := "Confirm the subject should have this access. Wildcard verbs and cluster-admin need an explicit exception."
	if isWildcard(new) && !isWildcard(old) {
		return verdict{RiskCritical, "The role gained a wildcard apiGroup, resource, or verb.", check}
	}
	if strings.Contains(new, "cluster-admin") && !strings.Contains(old, "cluster-admin") {
		return verdict{RiskCritical, "The binding now references cluster-admin.", check}
	}
	if path == "subjects" {
		return verdict{RiskHigh, "The binding subjects changed. A different account now receives this role.", check}
	}
	if path == "roleRef" {
		return verdict{RiskHigh, "roleRef changed. The binding grants a different role.", check}
	}
	return verdict{RiskHigh, "RBAC rules changed. This is an authorization change, not a cosmetic one.", check}
}

func isWildcard(v string) bool {
	return strings.Contains(v, "verbs=*") || strings.Contains(v, "resources=*") || strings.Contains(v, "apiGroups=*")
}

func securityRisk(path, old, new string) verdict {
	check := "Privileged, host-namespace, and extra capabilities need an explicit exception and a named owner."
	if strings.HasSuffix(path, "privileged") && new == "true" && old != "true" {
		return verdict{RiskCritical, "The container is now privileged. It can escape most of the pod isolation boundary.", check}
	}
	if strings.Contains(path, "capabilities.add") && dangerousCaps(new) && !dangerousCaps(old) {
		return verdict{RiskCritical, "A powerful capability was added (SYS_ADMIN, NET_ADMIN, SYS_PTRACE, SYS_MODULE, or ALL).", check}
	}
	if (strings.HasSuffix(path, "hostNetwork") || strings.HasSuffix(path, "hostPID") || strings.HasSuffix(path, "hostIPC")) && new == "true" && old != "true" {
		return verdict{RiskCritical, "The pod shares a host namespace. That is node-level access.", check}
	}
	if strings.HasSuffix(path, "runAsUser") && new == "0" && old != "0" {
		return verdict{RiskHigh, "The container now runs as root (runAsUser 0).", check}
	}
	if strings.HasSuffix(path, "runAsNonRoot") && new == "false" && old == "true" {
		return verdict{RiskHigh, "runAsNonRoot was turned off. The process may run as root.", check}
	}
	if strings.HasSuffix(path, "allowPrivilegeEscalation") && new == "true" && old == "false" {
		return verdict{RiskHigh, "allowPrivilegeEscalation was turned on.", check}
	}
	if strings.HasSuffix(path, "privileged") && old == "true" && new == "false" {
		return verdict{RiskLow, "privileged was turned off. This is a hardening change; still confirm the process starts.", check}
	}
	return verdict{RiskMedium, "The securityContext changed.", check}
}

func dangerousCaps(v string) bool {
	for _, c := range strings.Split(v, ",") {
		switch strings.TrimSpace(c) {
		case "ALL", "SYS_ADMIN", "NET_ADMIN", "SYS_MODULE", "SYS_PTRACE", "DAC_OVERRIDE":
			return true
		}
	}
	return false
}

func envRisk(old, new string) verdict {
	check := "Confirm the Secret or ConfigMap exists in this namespace and the key is the one the process reads."
	if strings.Contains(old, "secretKeyRef") || strings.Contains(new, "secretKeyRef") {
		if old != new {
			return verdict{RiskHigh, "A secretKeyRef name or key changed. The process will read a different secret, or fail if the key is missing.", check}
		}
	}
	if new == "" {
		return verdict{RiskMedium, "An environment variable was removed.", check}
	}
	if old == "" {
		return verdict{RiskMedium, "An environment variable was added.", check}
	}
	return verdict{RiskMedium, "An environment value changed. If this used to be a secret, it may now be plain text in the manifest.", check}
}

func addedVerdict(obj Object) verdict {
	check := "Read the new object end to end. There is no previous baseline."
	if role := roleRef(obj); role == "cluster-admin" || hasWildcardRules(obj) {
		return verdict{RiskCritical, "The new object grants cluster-admin or wildcard RBAC.", "Name the owner and confirm this subject should have cluster-wide access."}
	}
	if obj.Kind == "ClusterRoleBinding" || obj.Kind == "ClusterRole" {
		return verdict{RiskHigh, "A cluster-scoped RBAC object was added.", check}
	}
	if hasPrivileged(obj) || hasHostPath(obj) {
		return verdict{RiskCritical, "The new object is privileged or mounts a hostPath.", check}
	}
	if obj.Kind == "NetworkPolicy" {
		return verdict{RiskMedium, "A new NetworkPolicy can block traffic that flows today.", "Apply it in a cluster where you can watch the callers."}
	}
	return verdict{RiskMedium, "A new object was added.", check}
}

func removedVerdict(obj Object) verdict {
	check := "Confirm the object is intentionally retired and that nothing still selects or routes to it."
	switch obj.Kind {
	case "NetworkPolicy":
		return verdict{RiskCritical, "Removing a NetworkPolicy opens whatever paths that policy was denying.", "Name the flows that become allowed and accept them on purpose."}
	case "Role", "ClusterRole", "RoleBinding", "ClusterRoleBinding":
		return verdict{RiskHigh, "RBAC was removed. Confirm no controller still needs those permissions, and that this is not hiding a grant you meant to keep.", check}
	case "Deployment", "StatefulSet", "DaemonSet", "Service":
		return verdict{RiskHigh, "Removing this object stops its traffic or its pods.", check}
	default:
		return verdict{RiskMedium, "The object was removed.", check}
	}
}

func roleRef(obj Object) string {
	v := nested(obj.Raw, "roleRef", "name")
	return scalar(v)
}

func hasWildcardRules(obj Object) bool {
	for _, rule := range asMaps(nested(obj.Raw, "rules")) {
		if compact(rule["verbs"]) == "*" || strings.Contains(compact(rule["verbs"]), "*") && compact(rule["resources"]) == "*" {
			return true
		}
		if containsStar(rule["verbs"]) && containsStar(rule["resources"]) {
			return true
		}
	}
	return false
}

func containsStar(v any) bool {
	for _, item := range asSlice(v) {
		if scalar(item) == "*" {
			return true
		}
	}
	return scalar(v) == "*"
}

func hasPrivileged(obj Object) bool {
	spec, _ := podSpec(obj)
	if spec == nil {
		return false
	}
	for _, list := range []string{"containers", "initContainers"} {
		for _, c := range asMaps(spec[list]) {
			if scalar(asMap(c["securityContext"])["privileged"]) == "true" {
				return true
			}
		}
	}
	return false
}

func hasHostPath(obj Object) bool {
	spec, _ := podSpec(obj)
	if spec == nil {
		return false
	}
	for _, v := range asMaps(spec["volumes"]) {
		if v["hostPath"] != nil {
			return true
		}
	}
	return false
}

func hpaInverted(fields map[string]Field) bool {
	min, okMin := fields["spec.minReplicas"]
	max, okMax := fields["spec.maxReplicas"]
	if !okMin || !okMax {
		return false
	}
	ni, err1 := strconv.Atoi(min.Value)
	xi, err2 := strconv.Atoi(max.Value)
	return err1 == nil && err2 == nil && ni > xi
}
