package waste

import (
	"fmt"
	"sort"
	"strconv"
	"strings"
	"time"
)

// Analyze classifies a snapshot. ReplicaSets owned by a Deployment are skipped
// so a scaled workload is not reported twice. Usage ratios are used only when
// metrics are available; otherwise large requests are the fallback.
func Analyze(snap Snapshot, opts Options) Report {
	opts = opts.withDefaults()
	cluster := opts.Cluster
	if cluster == "" {
		cluster = snap.Cluster
	}
	if cluster == "" {
		cluster = "unknown"
	}

	var findings []Finding
	seq := 0
	emit := func(f Finding) {
		f.seq = seq
		seq++
		findings = append(findings, f)
	}
	var summary Summary

	for _, w := range snap.Workloads {
		if !opts.includeNS(w.Namespace) || w.OwnedByDeployment {
			continue
		}
		summary.Workloads++
		if f, ok := idleFinding(w, opts); ok {
			summary.Idle++
			emit(f)
			continue
		}
		if f, ok := sizeFinding(w, opts, snap.MetricsAvailable); ok {
			if f.Category == CatOversized {
				summary.Oversized++
			} else {
				summary.HighRequests++
			}
			emit(f)
		}
	}
	for _, svc := range snap.Services {
		if !opts.includeNS(svc.Namespace) {
			continue
		}
		if f, ok := serviceFinding(svc); ok {
			summary.LoadBalancers++
			emit(f)
		}
	}
	for _, claim := range snap.Claims {
		if !opts.includeNS(claim.Namespace) {
			continue
		}
		if f, ok := claimFinding(claim); ok {
			if f.Category == CatUnboundPVC {
				summary.UnboundPVCs++
			} else {
				summary.OrphanedPVCs++
			}
			emit(f)
		}
	}

	var configs []Finding
	if opts.IncludeConfig {
		for _, obj := range snap.Configs {
			if !opts.includeNS(obj.Namespace) || obj.Used || skipConfig(obj) {
				continue
			}
			if opts.Now.Sub(obj.Created) < opts.IdleFor {
				continue
			}
			f := configFinding(obj, opts)
			if obj.Kind == KindSecret {
				summary.UnusedSecrets++
			} else {
				summary.UnusedConfig++
			}
			configs = append(configs, f)
		}
	}
	omitted := 0
	if len(configs) > maxConfigFindings {
		omitted = len(configs) - maxConfigFindings
		sort.Slice(configs, func(i, j int) bool {
			return configs[i].Resource < configs[j].Resource
		})
		configs = configs[:maxConfigFindings]
	}
	for _, f := range configs {
		emit(f)
	}

	sort.SliceStable(findings, func(i, j int) bool {
		if rankSev(findings[i].Severity) != rankSev(findings[j].Severity) {
			return rankSev(findings[i].Severity) < rankSev(findings[j].Severity)
		}
		if rankCat(findings[i].Category) != rankCat(findings[j].Category) {
			return rankCat(findings[i].Category) < rankCat(findings[j].Category)
		}
		if findings[i].Resource != findings[j].Resource {
			return findings[i].Resource < findings[j].Resource
		}
		return findings[i].seq < findings[j].seq
	})

	notes := make([]string, 0, 4)
	if opts.Demo {
		notes = append(notes, "Demo mode uses synthetic objects and does not contact a cluster.")
	}
	if snap.MetricsAvailable {
		source := "Pod usage came from metrics.k8s.io."
		if opts.Demo {
			source = "Demo usage numbers are synthetic samples in the shape of metrics.k8s.io."
		}
		notes = append(notes, fmt.Sprintf("%s A workload is oversized when usage is under %.0f%% of a request of at least %s CPU or %s memory.", source, opts.UsageRatio*100, formatCPU(minCPUMilli), formatMem(minMemBytes)))
	} else {
		reason := snap.MetricsError
		if reason == "" {
			reason = "metrics.k8s.io is not available"
		}
		notes = append(notes, "Usage metrics are unavailable ("+reason+"). Large requests are flagged instead: "+formatCPU(highCPUMilli)+" CPU or "+formatMem(highMemBytes)+" memory.")
	}
	notes = append(notes, "ReplicaSets owned by a Deployment are not reported separately.")
	if opts.IncludeConfig {
		notes = append(notes, "Unused ConfigMaps and Secrets are info-only. Service-account tokens, Helm releases, and kube-root-ca.crt are skipped.")
	}
	if omitted > 0 {
		notes = append(notes, fmt.Sprintf("Showing %d of %d unused ConfigMaps and Secrets. The rest are omitted to keep the report short.", maxConfigFindings, maxConfigFindings+omitted))
	}
	notes = append(notes, snap.Notes...)
	if len(findings) == 0 {
		notes = append(notes, "No idle workloads, oversized requests, empty load balancers, or orphaned PVCs in this scope.")
	}

	return Report{
		GeneratedAt: opts.Now.UTC(),
		Demo:        opts.Demo,
		Cluster:     cluster,
		Scope:       opts.ScopeLabel(),
		IdleFor:     opts.IdleFor,
		UsageRatio:  opts.UsageRatio,
		Summary:     summary,
		Findings:    findings,
		Notes:       notes,
	}
}

func idleFinding(w Workload, opts Options) (Finding, bool) {
	age := opts.Now.Sub(w.Created)
	if age < 0 {
		age = 0
	}
	if w.Desired == 0 {
		return Finding{
			Severity: SevWarning,
			Category: CatIdle,
			Resource: workloadRef(w),
			Phenomenon: fmt.Sprintf("%s %s/%s desires 0 replicas (age %s). It is scaled to zero and is not serving traffic.",
				w.Kind, w.Namespace, w.Name, agePhrase(age)),
			LikelyCause: "A scaled-to-zero workload still costs attention, and any PVC it left behind keeps billing. Confirm this is intentional before leaving it.",
			NextStep:    idleNext(w, true),
			Details:     workloadDetails(w, age),
		}, true
	}
	if w.Ready == 0 && age >= opts.IdleFor {
		return Finding{
			Severity: SevWarning,
			Category: CatIdle,
			Resource: workloadRef(w),
			Phenomenon: fmt.Sprintf("%s %s/%s desires %d replicas and has 0 Ready (age %s, idle threshold %s).",
				w.Kind, w.Namespace, w.Name, w.Desired, agePhrase(age), agePhrase(opts.IdleFor)),
			LikelyCause: "Nothing is Ready, so the workload is not earning its place. Pending or crash-looping pods may also be holding claims.",
			NextStep:    idleNext(w, false),
			Details:     workloadDetails(w, age),
		}, true
	}
	return Finding{}, false
}

func sizeFinding(w Workload, opts Options, metrics bool) (Finding, bool) {
	if w.Desired <= 0 {
		return Finding{}, false
	}
	totalCPU := w.CPURequestMilli * int64(w.Desired)
	totalMem := w.MemRequestBytes * int64(w.Desired)
	if metrics && w.HasUsage {
		return oversizedFinding(w, opts, totalCPU, totalMem)
	}
	if metrics {
		return Finding{}, false
	}
	if w.Ready == 0 {
		return Finding{}, false
	}
	return highRequestFinding(w, totalCPU, totalMem)
}

func oversizedFinding(w Workload, opts Options, totalCPU, totalMem int64) (Finding, bool) {
	var bits []string
	critical := false
	flagged := false
	if totalCPU >= minCPUMilli {
		ratio := float64(w.CPUUsageMilli) / float64(totalCPU)
		if ratio < opts.UsageRatio {
			flagged = true
			if ratio < opts.UsageRatio/2 {
				critical = true
			}
			bits = append(bits, fmt.Sprintf("%s CPU in use of %s requested (%s)", formatCPU(w.CPUUsageMilli), formatCPU(totalCPU), formatPercent(ratio)))
		}
	}
	if totalMem >= minMemBytes {
		ratio := float64(w.MemUsageBytes) / float64(totalMem)
		if ratio < opts.UsageRatio {
			flagged = true
			if ratio < opts.UsageRatio/2 {
				critical = true
			}
			bits = append(bits, fmt.Sprintf("%s memory in use of %s requested (%s)", formatMem(w.MemUsageBytes), formatMem(totalMem), formatPercent(ratio)))
		}
	}
	if !flagged {
		return Finding{}, false
	}
	sev := SevWarning
	if critical {
		sev = SevCritical
	}
	return Finding{
		Severity: sev,
		Category: CatOversized,
		Resource: workloadRef(w),
		Phenomenon: fmt.Sprintf("%s %s/%s reserves %s CPU and %s memory across %s, but metrics show %s.",
			w.Kind, w.Namespace, w.Name, formatCPU(totalCPU), formatMem(totalMem), replicaWord(w.Desired), strings.Join(bits, "; ")),
		LikelyCause: "The request is what the scheduler reserves. Usage well under the request is capacity you pay for and do not use." + qosPhrase(w),
		NextStep:    oversizedNext(w),
		Details: []Detail{
			{Name: "cpuRequest", Value: formatCPU(totalCPU)},
			{Name: "cpuUsage", Value: formatCPU(w.CPUUsageMilli)},
			{Name: "memoryRequest", Value: formatMem(totalMem)},
			{Name: "memoryUsage", Value: formatMem(w.MemUsageBytes)},
			{Name: "replicas", Value: strconv.Itoa(int(w.Desired))},
		},
	}, true
}

func highRequestFinding(w Workload, totalCPU, totalMem int64) (Finding, bool) {
	if totalCPU < highCPUMilli && totalMem < highMemBytes {
		return Finding{}, false
	}
	return Finding{
		Severity: SevWarning,
		Category: CatHighRequest,
		Resource: workloadRef(w),
		Phenomenon: fmt.Sprintf("%s %s/%s requests %s CPU and %s memory across %s. No usage sample is available to compare.",
			w.Kind, w.Namespace, w.Name, formatCPU(totalCPU), formatMem(totalMem), replicaWord(w.Desired)),
		LikelyCause: "Without metrics.k8s.io, a request at or above " + formatCPU(highCPUMilli) + " CPU or " + formatMem(highMemBytes) + " memory is treated as suspicious. The request is reserved even when the process is idle." + qosPhrase(w),
		NextStep:    highRequestNext(w),
		Details: []Detail{
			{Name: "cpuRequest", Value: formatCPU(totalCPU)},
			{Name: "memoryRequest", Value: formatMem(totalMem)},
			{Name: "replicas", Value: strconv.Itoa(int(w.Desired))},
		},
	}, true
}

func serviceFinding(s Service) (Finding, bool) {
	if s.Type != "LoadBalancer" || s.ReadyEndpoints > 0 {
		return Finding{}, false
	}
	addr := "no address allocated yet"
	if s.Address != "" {
		addr = "address " + s.Address
	}
	return Finding{
		Severity:    SevWarning,
		Category:    CatLoadBalancer,
		Resource:    "svc " + s.Namespace + "/" + s.Name,
		Phenomenon:  fmt.Sprintf("Service %s/%s is type LoadBalancer with %s and 0 ready endpoints.", s.Namespace, s.Name, addr),
		LikelyCause: "A cloud load balancer can bill for as long as the Service exists, even when no Pod is ready behind it.",
		NextStep:    lbNext(s),
		Details: []Detail{
			{Name: "endpoints", Value: "0"},
			{Name: "address", Value: s.Address},
		},
	}, true
}

func claimFinding(c Claim) (Finding, bool) {
	if c.Mounted {
		return Finding{}, false
	}
	storage := "an unknown size"
	if c.StorageBytes > 0 {
		storage = formatMem(c.StorageBytes)
	}
	phase := c.Phase
	if phase == "" {
		phase = "Unknown"
	}
	if phase == "Bound" {
		sev := SevWarning
		if c.StorageBytes >= criticalPVCBytes {
			sev = SevCritical
		}
		return Finding{
			Severity:    sev,
			Category:    CatOrphanedPVC,
			Resource:    "pvc " + c.Namespace + "/" + c.Name,
			Phenomenon:  fmt.Sprintf("PersistentVolumeClaim %s/%s is Bound for %s and no running Pod mounts it.", c.Namespace, c.Name, storage),
			LikelyCause: "A Bound claim usually has a real disk behind it. Unused disks are a common quiet cloud bill.",
			NextStep:    pvcNext(c, true),
			Details: []Detail{
				{Name: "phase", Value: phase},
				{Name: "storage", Value: storage},
			},
		}, true
	}
	return Finding{
		Severity:    SevWarning,
		Category:    CatUnboundPVC,
		Resource:    "pvc " + c.Namespace + "/" + c.Name,
		Phenomenon:  fmt.Sprintf("PersistentVolumeClaim %s/%s is %s (unbound) for %s.", c.Namespace, c.Name, phase, storage),
		LikelyCause: "An unbound claim is not always billing yet, but it is leftover intent. A later bind will create a disk nobody mounts.",
		NextStep:    pvcNext(c, false),
		Details: []Detail{
			{Name: "phase", Value: phase},
			{Name: "storage", Value: storage},
		},
	}, true
}

func configFinding(o ConfigObject, opts Options) Finding {
	age := opts.Now.Sub(o.Created)
	kind := o.Kind
	cat := CatUnusedConfig
	sev := SevInfo
	if o.Kind == KindSecret {
		cat = CatUnusedSecret
	}
	word := "ConfigMap"
	if o.Kind == KindSecret {
		word = "Secret"
	}
	return Finding{
		Severity:    sev,
		Category:    cat,
		Resource:    strings.ToLower(kind) + " " + o.Namespace + "/" + o.Name,
		Phenomenon:  fmt.Sprintf("%s %s/%s is %s old and no Pod in this scope references it.", word, o.Namespace, o.Name, agePhrase(age)),
		LikelyCause: "Nothing in the scanned Pod specs mounts or imports it. It may still be used outside the cluster, so the next step is a check, not a blind delete.",
		NextStep:    configNext(o),
		Details: []Detail{
			{Name: "age", Value: agePhrase(age)},
		},
	}
}

func skipConfig(o ConfigObject) bool {
	switch o.Name {
	case "kube-root-ca.crt", "istio-ca-root-cert":
		return true
	}
	if strings.HasPrefix(o.Name, "sh.helm.release.") {
		return true
	}
	if o.Kind == KindSecret {
		if o.SecretType == "kubernetes.io/service-account-token" || strings.HasPrefix(o.SecretType, "helm.sh/") {
			return true
		}
	}
	return false
}

func replicaWord(n int32) string {
	if n == 1 {
		return "1 replica"
	}
	return fmt.Sprintf("%d replicas", n)
}

func workloadDetails(w Workload, age time.Duration) []Detail {
	return []Detail{
		{Name: "desired", Value: strconv.Itoa(int(w.Desired))},
		{Name: "ready", Value: strconv.Itoa(int(w.Ready))},
		{Name: "age", Value: agePhrase(age)},
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
	case CatOversized:
		return 0
	case CatOrphanedPVC:
		return 1
	case CatIdle:
		return 2
	case CatHighRequest:
		return 3
	case CatLoadBalancer:
		return 4
	case CatUnboundPVC:
		return 5
	case CatUnusedSecret:
		return 6
	case CatUnusedConfig:
		return 7
	default:
		return 8
	}
}
