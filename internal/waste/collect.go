package waste

import (
	"context"
	"errors"
	"fmt"
	"time"

	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes"
)

// Collect lists workloads, services, PVCs, and cheap unused config references.
// metrics may be nil. A metrics list error is recorded on the snapshot and
// does not fail the scan. ConfigMap or Secret list errors are also recorded
// and skipped so missing RBAC does not hide the rest of the report.
func Collect(ctx context.Context, client kubernetes.Interface, metrics UsageSource, namespace string, allNamespaces bool) (Snapshot, error) {
	if client == nil {
		return Snapshot{}, errors.New("kubernetes client is nil")
	}
	if !allNamespaces && namespace == "" {
		return Snapshot{}, errors.New("namespace is required when all-namespaces is false")
	}
	ns := namespace
	if allNamespaces {
		ns = ""
	}

	deploys, err := listDeployments(ctx, client, ns)
	if err != nil {
		return Snapshot{}, err
	}
	sets, err := listStatefulSets(ctx, client, ns)
	if err != nil {
		return Snapshot{}, err
	}
	rss, err := listReplicaSets(ctx, client, ns)
	if err != nil {
		return Snapshot{}, err
	}
	pods, err := listPods(ctx, client, ns)
	if err != nil {
		return Snapshot{}, err
	}
	svcs, err := listServices(ctx, client, ns)
	if err != nil {
		return Snapshot{}, err
	}
	endpoints, err := listEndpoints(ctx, client, ns)
	if err != nil {
		return Snapshot{}, err
	}
	claims, err := listClaims(ctx, client, ns)
	if err != nil {
		return Snapshot{}, err
	}

	var extra []string
	configMaps, err := listConfigMaps(ctx, client, ns)
	if err != nil {
		extra = append(extra, "configmaps were not listed: "+err.Error())
		configMaps = nil
	}
	secrets, err := listSecrets(ctx, client, ns)
	if err != nil {
		extra = append(extra, "secrets were not listed: "+err.Error())
		secrets = nil
	}

	snap := Snapshot{}
	usage := map[string]PodUsage{}
	switch {
	case metrics == nil:
		snap.MetricsError = "metrics client was not configured"
	default:
		listed, err := metrics.ListPodUsage(ctx, ns)
		if err != nil {
			snap.MetricsError = err.Error()
		} else {
			snap.MetricsAvailable = true
			usage = listed
		}
	}
	snap.Notes = extra

	snap.Workloads = buildWorkloads(deploys, sets, rss, pods, usage)
	snap.Services = buildServices(svcs, endpoints)
	snap.Claims = buildClaims(claims, pods)
	snap.Configs = buildConfigs(configMaps, secrets, pods)
	return snap, nil
}

func listDeployments(ctx context.Context, client kubernetes.Interface, ns string) ([]appsv1.Deployment, error) {
	var out []appsv1.Deployment
	err := listEvery(ctx, func(token string) (string, error) {
		list, err := client.AppsV1().Deployments(ns).List(ctx, metav1.ListOptions{Limit: 200, Continue: token})
		if err != nil {
			return "", fmt.Errorf("list deployments: %w", err)
		}
		out = append(out, list.Items...)
		return list.Continue, nil
	})
	return out, err
}

func listStatefulSets(ctx context.Context, client kubernetes.Interface, ns string) ([]appsv1.StatefulSet, error) {
	var out []appsv1.StatefulSet
	err := listEvery(ctx, func(token string) (string, error) {
		list, err := client.AppsV1().StatefulSets(ns).List(ctx, metav1.ListOptions{Limit: 200, Continue: token})
		if err != nil {
			return "", fmt.Errorf("list statefulsets: %w", err)
		}
		out = append(out, list.Items...)
		return list.Continue, nil
	})
	return out, err
}

func listReplicaSets(ctx context.Context, client kubernetes.Interface, ns string) ([]appsv1.ReplicaSet, error) {
	var out []appsv1.ReplicaSet
	err := listEvery(ctx, func(token string) (string, error) {
		list, err := client.AppsV1().ReplicaSets(ns).List(ctx, metav1.ListOptions{Limit: 200, Continue: token})
		if err != nil {
			return "", fmt.Errorf("list replicasets: %w", err)
		}
		out = append(out, list.Items...)
		return list.Continue, nil
	})
	return out, err
}

func listPods(ctx context.Context, client kubernetes.Interface, ns string) ([]corev1.Pod, error) {
	var out []corev1.Pod
	err := listEvery(ctx, func(token string) (string, error) {
		list, err := client.CoreV1().Pods(ns).List(ctx, metav1.ListOptions{Limit: 500, Continue: token})
		if err != nil {
			return "", fmt.Errorf("list pods: %w", err)
		}
		out = append(out, list.Items...)
		return list.Continue, nil
	})
	return out, err
}

func listServices(ctx context.Context, client kubernetes.Interface, ns string) ([]corev1.Service, error) {
	var out []corev1.Service
	err := listEvery(ctx, func(token string) (string, error) {
		list, err := client.CoreV1().Services(ns).List(ctx, metav1.ListOptions{Limit: 200, Continue: token})
		if err != nil {
			return "", fmt.Errorf("list services: %w", err)
		}
		out = append(out, list.Items...)
		return list.Continue, nil
	})
	return out, err
}

func listEndpoints(ctx context.Context, client kubernetes.Interface, ns string) ([]corev1.Endpoints, error) {
	var out []corev1.Endpoints
	err := listEvery(ctx, func(token string) (string, error) {
		list, err := client.CoreV1().Endpoints(ns).List(ctx, metav1.ListOptions{Limit: 200, Continue: token})
		if err != nil {
			return "", fmt.Errorf("list endpoints: %w", err)
		}
		out = append(out, list.Items...)
		return list.Continue, nil
	})
	return out, err
}

func listClaims(ctx context.Context, client kubernetes.Interface, ns string) ([]corev1.PersistentVolumeClaim, error) {
	var out []corev1.PersistentVolumeClaim
	err := listEvery(ctx, func(token string) (string, error) {
		list, err := client.CoreV1().PersistentVolumeClaims(ns).List(ctx, metav1.ListOptions{Limit: 200, Continue: token})
		if err != nil {
			return "", fmt.Errorf("list persistentvolumeclaims: %w", err)
		}
		out = append(out, list.Items...)
		return list.Continue, nil
	})
	return out, err
}

func listConfigMaps(ctx context.Context, client kubernetes.Interface, ns string) ([]corev1.ConfigMap, error) {
	var out []corev1.ConfigMap
	err := listEvery(ctx, func(token string) (string, error) {
		list, err := client.CoreV1().ConfigMaps(ns).List(ctx, metav1.ListOptions{Limit: 200, Continue: token})
		if err != nil {
			return "", fmt.Errorf("list configmaps: %w", err)
		}
		out = append(out, list.Items...)
		return list.Continue, nil
	})
	return out, err
}

func listSecrets(ctx context.Context, client kubernetes.Interface, ns string) ([]corev1.Secret, error) {
	var out []corev1.Secret
	err := listEvery(ctx, func(token string) (string, error) {
		list, err := client.CoreV1().Secrets(ns).List(ctx, metav1.ListOptions{Limit: 200, Continue: token})
		if err != nil {
			return "", fmt.Errorf("list secrets: %w", err)
		}
		out = append(out, list.Items...)
		return list.Continue, nil
	})
	return out, err
}

type usageSum struct {
	cpu     int64
	mem     int64
	samples int
}

func buildWorkloads(deploys []appsv1.Deployment, sets []appsv1.StatefulSet, rss []appsv1.ReplicaSet, pods []corev1.Pod, usage map[string]PodUsage) []Workload {
	rsOwner := map[string]string{}
	for _, rs := range rss {
		for _, o := range rs.OwnerReferences {
			if o.Kind == "Deployment" && o.Controller != nil && *o.Controller {
				rsOwner[rs.Namespace+"/"+rs.Name] = o.Name
			}
		}
	}
	byWorkload := map[string]usageSum{}
	for _, pod := range pods {
		kind, name, ok := podWorkload(pod, rsOwner)
		if !ok {
			continue
		}
		sample, found := usage[pod.Namespace+"/"+pod.Name]
		if !found {
			continue
		}
		key := kind + "/" + pod.Namespace + "/" + name
		sum := byWorkload[key]
		sum.cpu += sample.CPUMilli
		sum.mem += sample.MemBytes
		sum.samples++
		byWorkload[key] = sum
	}

	var out []Workload
	for _, d := range deploys {
		w := fromPodSpec(KindDeployment, d.Namespace, d.Name, nreplicas(d.Spec.Replicas), d.Status.ReadyReplicas, d.CreationTimestamp.Time, d.Spec.Template.Spec)
		applyUsage(&w, byWorkload)
		out = append(out, w)
	}
	for _, s := range sets {
		w := fromPodSpec(KindStatefulSet, s.Namespace, s.Name, nreplicas(s.Spec.Replicas), s.Status.ReadyReplicas, s.CreationTimestamp.Time, s.Spec.Template.Spec)
		applyUsage(&w, byWorkload)
		out = append(out, w)
	}
	for _, rs := range rss {
		w := fromPodSpec(KindReplicaSet, rs.Namespace, rs.Name, nreplicas(rs.Spec.Replicas), rs.Status.ReadyReplicas, rs.CreationTimestamp.Time, rs.Spec.Template.Spec)
		if _, ok := rsOwner[rs.Namespace+"/"+rs.Name]; ok {
			w.OwnedByDeployment = true
		}
		applyUsage(&w, byWorkload)
		out = append(out, w)
	}
	return out
}

func applyUsage(w *Workload, sums map[string]usageSum) {
	sum, ok := sums[w.Kind+"/"+w.Namespace+"/"+w.Name]
	if !ok || sum.samples == 0 {
		return
	}
	w.HasUsage = true
	w.CPUUsageMilli = sum.cpu
	w.MemUsageBytes = sum.mem
}

func fromPodSpec(kind, ns, name string, desired, ready int32, created time.Time, spec corev1.PodSpec) Workload {
	w := Workload{
		Kind: kind, Namespace: ns, Name: name,
		Desired: desired, Ready: ready, Created: created,
	}
	for _, c := range spec.Containers {
		if q, ok := c.Resources.Requests[corev1.ResourceCPU]; ok {
			w.CPURequestMilli += q.MilliValue()
		}
		if q, ok := c.Resources.Requests[corev1.ResourceMemory]; ok {
			w.MemRequestBytes += q.Value()
		}
		if q, ok := c.Resources.Limits[corev1.ResourceCPU]; ok {
			w.CPULimitMilli += q.MilliValue()
			w.HasCPULimit = true
		}
		if q, ok := c.Resources.Limits[corev1.ResourceMemory]; ok {
			w.MemLimitBytes += q.Value()
			w.HasMemLimit = true
		}
	}
	return w
}

func podWorkload(pod corev1.Pod, rsOwner map[string]string) (kind, name string, ok bool) {
	for _, o := range pod.OwnerReferences {
		if o.Controller == nil || !*o.Controller {
			continue
		}
		switch o.Kind {
		case "ReplicaSet":
			if dep, found := rsOwner[pod.Namespace+"/"+o.Name]; found {
				return KindDeployment, dep, true
			}
			return KindReplicaSet, o.Name, true
		case "StatefulSet":
			return KindStatefulSet, o.Name, true
		}
	}
	return "", "", false
}

func nreplicas(p *int32) int32 {
	if p == nil {
		return 1
	}
	return *p
}

func buildServices(svcs []corev1.Service, endpoints []corev1.Endpoints) []Service {
	ready := map[string]int{}
	for _, ep := range endpoints {
		n := 0
		for _, subset := range ep.Subsets {
			n += len(subset.Addresses)
		}
		ready[ep.Namespace+"/"+ep.Name] = n
	}
	out := make([]Service, 0, len(svcs))
	for _, svc := range svcs {
		out = append(out, Service{
			Namespace:      svc.Namespace,
			Name:           svc.Name,
			Type:           string(svc.Spec.Type),
			ReadyEndpoints: ready[svc.Namespace+"/"+svc.Name],
			Address:        lbAddress(svc),
		})
	}
	return out
}

func lbAddress(svc corev1.Service) string {
	for _, ing := range svc.Status.LoadBalancer.Ingress {
		if ing.IP != "" {
			return ing.IP
		}
		if ing.Hostname != "" {
			return ing.Hostname
		}
	}
	return ""
}

func buildClaims(claims []corev1.PersistentVolumeClaim, pods []corev1.Pod) []Claim {
	mounted := map[string]struct{}{}
	for _, pod := range pods {
		if pod.Status.Phase != corev1.PodRunning {
			continue
		}
		for _, vol := range pod.Spec.Volumes {
			if vol.PersistentVolumeClaim == nil {
				continue
			}
			mounted[pod.Namespace+"/"+vol.PersistentVolumeClaim.ClaimName] = struct{}{}
		}
	}
	out := make([]Claim, 0, len(claims))
	for _, pvc := range claims {
		var storage int64
		if q, ok := pvc.Spec.Resources.Requests[corev1.ResourceStorage]; ok {
			storage = q.Value()
		}
		_, isMounted := mounted[pvc.Namespace+"/"+pvc.Name]
		out = append(out, Claim{
			Namespace:    pvc.Namespace,
			Name:         pvc.Name,
			Phase:        string(pvc.Status.Phase),
			StorageBytes: storage,
			Mounted:      isMounted,
		})
	}
	return out
}

func buildConfigs(cms []corev1.ConfigMap, secrets []corev1.Secret, pods []corev1.Pod) []ConfigObject {
	usedCM, usedSecret := referencedConfig(pods)
	out := make([]ConfigObject, 0, len(cms)+len(secrets))
	for _, cm := range cms {
		_, used := usedCM[cm.Namespace+"/"+cm.Name]
		out = append(out, ConfigObject{
			Kind: KindConfigMap, Namespace: cm.Namespace, Name: cm.Name,
			Created: cm.CreationTimestamp.Time, Used: used,
		})
	}
	for _, secret := range secrets {
		_, used := usedSecret[secret.Namespace+"/"+secret.Name]
		out = append(out, ConfigObject{
			Kind: KindSecret, Namespace: secret.Namespace, Name: secret.Name,
			Created: secret.CreationTimestamp.Time, Used: used, SecretType: string(secret.Type),
		})
	}
	return out
}

func referencedConfig(pods []corev1.Pod) (cms, secrets map[string]struct{}) {
	cms = map[string]struct{}{}
	secrets = map[string]struct{}{}
	addCM := func(ns, name string) {
		if name != "" {
			cms[ns+"/"+name] = struct{}{}
		}
	}
	addSecret := func(ns, name string) {
		if name != "" {
			secrets[ns+"/"+name] = struct{}{}
		}
	}
	consider := func(ns string, spec corev1.PodSpec) {
		for _, pull := range spec.ImagePullSecrets {
			addSecret(ns, pull.Name)
		}
		for _, vol := range spec.Volumes {
			if vol.ConfigMap != nil {
				addCM(ns, vol.ConfigMap.Name)
			}
			if vol.Secret != nil {
				addSecret(ns, vol.Secret.SecretName)
			}
			if vol.Projected != nil {
				for _, src := range vol.Projected.Sources {
					if src.ConfigMap != nil {
						addCM(ns, src.ConfigMap.Name)
					}
					if src.Secret != nil {
						addSecret(ns, src.Secret.Name)
					}
				}
			}
		}
		containers := append([]corev1.Container{}, spec.Containers...)
		containers = append(containers, spec.InitContainers...)
		for _, c := range containers {
			for _, from := range c.EnvFrom {
				if from.ConfigMapRef != nil {
					addCM(ns, from.ConfigMapRef.Name)
				}
				if from.SecretRef != nil {
					addSecret(ns, from.SecretRef.Name)
				}
			}
			for _, env := range c.Env {
				if env.ValueFrom == nil {
					continue
				}
				if env.ValueFrom.ConfigMapKeyRef != nil {
					addCM(ns, env.ValueFrom.ConfigMapKeyRef.Name)
				}
				if env.ValueFrom.SecretKeyRef != nil {
					addSecret(ns, env.ValueFrom.SecretKeyRef.Name)
				}
			}
		}
	}
	for _, pod := range pods {
		consider(pod.Namespace, pod.Spec)
	}
	return cms, secrets
}

func listEvery(ctx context.Context, page func(token string) (string, error)) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	token := ""
	seen := map[string]struct{}{}
	for i := 0; i < 10000; i++ {
		if err := ctx.Err(); err != nil {
			return err
		}
		next, err := page(token)
		if err != nil {
			return err
		}
		if next == "" {
			return nil
		}
		if _, ok := seen[next]; ok {
			return fmt.Errorf("list pagination stalled at continue token %q", next)
		}
		seen[next] = struct{}{}
		token = next
	}
	return errors.New("list pagination exceeded 10000 pages")
}
