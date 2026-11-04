package waste

import (
	"context"
	"fmt"
	"strings"

	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/rest"
	metricsclient "k8s.io/metrics/pkg/client/clientset/versioned"
)

// PodUsage is the summed container usage for one Pod.
type PodUsage struct {
	CPUMilli int64
	MemBytes int64
}

// UsageSource lists pod metrics. A nil source means metrics are not configured.
type UsageSource interface {
	ListPodUsage(ctx context.Context, namespace string) (map[string]PodUsage, error)
}

// NewUsageSource builds a metrics.k8s.io client. The caller treats list errors
// as "metrics unavailable" and still reports the rest of the scan.
func NewUsageSource(cfg *rest.Config) (UsageSource, error) {
	if cfg == nil {
		return nil, fmt.Errorf("rest config is nil")
	}
	client, err := metricsclient.NewForConfig(cfg)
	if err != nil {
		return nil, err
	}
	return metricsSource{client: client}, nil
}

type metricsSource struct {
	client *metricsclient.Clientset
}

func (m metricsSource) ListPodUsage(ctx context.Context, namespace string) (map[string]PodUsage, error) {
	list, err := m.client.MetricsV1beta1().PodMetricses(namespace).List(ctx, metav1.ListOptions{})
	if err != nil {
		return nil, err
	}
	out := make(map[string]PodUsage, len(list.Items))
	for _, item := range list.Items {
		var usage PodUsage
		for _, c := range item.Containers {
			if q, ok := c.Usage[corev1.ResourceCPU]; ok {
				usage.CPUMilli += q.MilliValue()
			}
			if q, ok := c.Usage[corev1.ResourceMemory]; ok {
				usage.MemBytes += q.Value()
			}
		}
		out[item.Namespace+"/"+item.Name] = usage
	}
	return out, nil
}

func metricsUnavailable(err error) bool {
	if err == nil {
		return false
	}
	if apierrors.IsNotFound(err) || apierrors.IsServiceUnavailable(err) || apierrors.IsForbidden(err) || apierrors.IsNotAcceptable(err) {
		return true
	}
	msg := strings.ToLower(err.Error())
	return strings.Contains(msg, "metrics.k8s.io") ||
		strings.Contains(msg, "the server could not find the requested resource") ||
		strings.Contains(msg, "server doesn't have a resource type")
}
