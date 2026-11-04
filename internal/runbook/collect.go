package runbook

import (
	"context"
	"encoding/json"
	"fmt"
	"strconv"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/labels"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/client-go/kubernetes"
)

const revisionAnnotation = "deployment.kubernetes.io/revision"

// Collect loads a Deployment or StatefulSet and the revisions needed to name
// a rollback target. Pod list failures are recorded and do not fail the runbook.
func Collect(ctx context.Context, client kubernetes.Interface, kind, namespace, name string) (Snapshot, error) {
	if client == nil {
		return Snapshot{}, fmt.Errorf("kubernetes client is nil")
	}
	switch kind {
	case KindStatefulSet:
		return collectStatefulSet(ctx, client, namespace, name)
	case KindDeployment:
		return collectDeployment(ctx, client, namespace, name)
	default:
		return Snapshot{}, fmt.Errorf("unsupported kind %q", kind)
	}
}

func collectDeployment(ctx context.Context, client kubernetes.Interface, namespace, name string) (Snapshot, error) {
	dep, err := client.AppsV1().Deployments(namespace).Get(ctx, name, metav1.GetOptions{})
	if err != nil {
		return Snapshot{}, fmt.Errorf("get deployment %s/%s: %w", namespace, name, err)
	}
	list, err := client.AppsV1().ReplicaSets(namespace).List(ctx, metav1.ListOptions{})
	if err != nil {
		return Snapshot{}, fmt.Errorf("list replicasets: %w", err)
	}
	var revs []Revision
	for _, rs := range list.Items {
		if !ownedBy(rs.ObjectMeta, dep.UID, KindDeployment) {
			continue
		}
		n, ok := revisionOf(rs.ObjectMeta)
		if !ok {
			continue
		}
		revs = append(revs, Revision{Number: n, Images: imagesFromPodSpec(rs.Spec.Template.Spec)})
	}
	current, ok := revisionOf(dep.ObjectMeta)
	if !ok {
		current = maxRevision(revs)
	}
	snap := Snapshot{
		Kind:            KindDeployment,
		Namespace:       namespace,
		Name:            name,
		Selector:        selectorString(dep.Spec.Selector),
		CurrentRevision: current,
		Desired:         replicas(dep.Spec.Replicas),
		Ready:           dep.Status.ReadyReplicas,
		Updated:         dep.Status.UpdatedReplicas,
		Images:          imagesFromPodSpec(dep.Spec.Template.Spec),
		Revisions:       revs,
	}
	if snap.Selector != "" {
		if failure, note, err := podFailure(ctx, client, namespace, snap.Selector); err != nil {
			snap.Notes = append(snap.Notes, "pods were not listed: "+err.Error())
		} else {
			snap.Failure = failure
			if note != "" {
				snap.Notes = append(snap.Notes, note)
			}
		}
	}
	return snap, nil
}

func collectStatefulSet(ctx context.Context, client kubernetes.Interface, namespace, name string) (Snapshot, error) {
	sts, err := client.AppsV1().StatefulSets(namespace).Get(ctx, name, metav1.GetOptions{})
	if err != nil {
		return Snapshot{}, fmt.Errorf("get statefulset %s/%s: %w", namespace, name, err)
	}
	list, err := client.AppsV1().ControllerRevisions(namespace).List(ctx, metav1.ListOptions{})
	if err != nil {
		return Snapshot{}, fmt.Errorf("list controllerrevisions: %w", err)
	}
	var revs []Revision
	byName := map[string]int64{}
	for _, rev := range list.Items {
		if !ownedBy(rev.ObjectMeta, sts.UID, KindStatefulSet) {
			continue
		}
		revs = append(revs, Revision{Number: rev.Revision, Images: imagesFromRevisionData(rev.Data.Raw)})
		byName[rev.Name] = rev.Revision
	}
	current := byName[sts.Status.UpdateRevision]
	if current == 0 {
		current = byName[sts.Status.CurrentRevision]
	}
	if current == 0 {
		current = maxRevision(revs)
	}
	return Snapshot{
		Kind:            KindStatefulSet,
		Namespace:       namespace,
		Name:            name,
		Selector:        selectorString(sts.Spec.Selector),
		CurrentRevision: current,
		Desired:         replicas(sts.Spec.Replicas),
		Ready:           sts.Status.ReadyReplicas,
		Updated:         sts.Status.UpdatedReplicas,
		Images:          imagesFromPodSpec(sts.Spec.Template.Spec),
		Revisions:       revs,
	}, nil
}

func ownedBy(meta metav1.ObjectMeta, uid types.UID, kind string) bool {
	for _, o := range meta.OwnerReferences {
		if o.Kind == kind && o.UID == uid {
			return true
		}
	}
	return false
}

func revisionOf(meta metav1.ObjectMeta) (int64, bool) {
	raw := meta.Annotations[revisionAnnotation]
	if raw == "" {
		return 0, false
	}
	n, err := strconv.ParseInt(raw, 10, 64)
	if err != nil {
		return 0, false
	}
	return n, true
}

func maxRevision(revs []Revision) int64 {
	var max int64
	for _, r := range revs {
		if r.Number > max {
			max = r.Number
		}
	}
	return max
}

func replicas(p *int32) int32 {
	if p == nil {
		return 1
	}
	return *p
}

func selectorString(sel *metav1.LabelSelector) string {
	if sel == nil || len(sel.MatchLabels) == 0 {
		return ""
	}
	return labels.Set(sel.MatchLabels).String()
}

func imagesFromPodSpec(spec corev1.PodSpec) []ContainerImage {
	out := make([]ContainerImage, 0, len(spec.InitContainers)+len(spec.Containers))
	for _, c := range spec.InitContainers {
		out = append(out, ContainerImage{Name: c.Name, Image: c.Image, Init: true})
	}
	for _, c := range spec.Containers {
		out = append(out, ContainerImage{Name: c.Name, Image: c.Image})
	}
	return out
}

func imagesFromRevisionData(raw []byte) []ContainerImage {
	if len(raw) == 0 {
		return nil
	}
	var doc any
	if err := json.Unmarshal(raw, &doc); err != nil {
		return nil
	}
	var found []ContainerImage
	walkContainers(doc, &found)
	return found
}

func walkContainers(v any, out *[]ContainerImage) {
	switch n := v.(type) {
	case map[string]any:
		if cs, ok := n["containers"]; ok {
			appendContainerList(cs, false, out)
		}
		if cs, ok := n["initContainers"]; ok {
			appendContainerList(cs, true, out)
		}
		for k, child := range n {
			if k == "containers" || k == "initContainers" {
				continue
			}
			walkContainers(child, out)
		}
	case []any:
		for _, child := range n {
			walkContainers(child, out)
		}
	}
}

func appendContainerList(v any, init bool, out *[]ContainerImage) {
	items, ok := v.([]any)
	if !ok {
		return
	}
	for _, item := range items {
		m, ok := item.(map[string]any)
		if !ok {
			continue
		}
		name, _ := m["name"].(string)
		image, _ := m["image"].(string)
		if name == "" && image == "" {
			continue
		}
		*out = append(*out, ContainerImage{Name: name, Image: image, Init: init})
	}
}

func podFailure(ctx context.Context, client kubernetes.Interface, namespace, selector string) (string, string, error) {
	list, err := client.CoreV1().Pods(namespace).List(ctx, metav1.ListOptions{LabelSelector: selector})
	if err != nil {
		return "", "", err
	}
	crashes := 0
	for _, pod := range list.Items {
		for _, cs := range pod.Status.ContainerStatuses {
			if cs.State.Waiting != nil && cs.State.Waiting.Reason == "CrashLoopBackOff" {
				crashes++
			}
		}
	}
	if crashes == 0 {
		return "", "", nil
	}
	return fmt.Sprintf("%d container(s) are CrashLoopBackOff.", crashes), "", nil
}
