package daily

import (
	"context"
	"fmt"
	"time"

	batchv1 "k8s.io/api/batch/v1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes"

	"github.com/wangqi040318-gif/sre-toolkit/internal/certs"
	"github.com/wangqi040318-gif/sre-toolkit/internal/pulse"
)

// CollectCluster reads one context. Pulse listing errors fail the cluster.
// Job and certificate listing errors are kept on the cluster and do not hide
// the pulse counts.
func CollectCluster(ctx context.Context, client kubernetes.Interface, contextName string, now time.Time) (Cluster, error) {
	if client == nil {
		return Cluster{}, fmt.Errorf("kubernetes client is nil")
	}
	if now.IsZero() {
		now = time.Now()
	}
	snap, err := pulse.Collect(ctx, client, "", true)
	if err != nil {
		return Cluster{}, err
	}
	snap.Cluster = contextName
	rep := pulse.Analyze(snap, pulse.Options{
		AllNamespaces: true,
		Now:           now,
		Since:         time.Hour,
		EventLimit:    20,
	})
	out := Cluster{
		Context:       contextName,
		NodesTotal:    rep.Summary.NodesTotal,
		NodesNotReady: rep.Summary.NodesNotReady,
		CrashLoop:     rep.Summary.CrashLoop,
		OOMKilled:     rep.Summary.OOMKilled,
		WarningEvents: rep.Summary.WarningEvents,
	}
	jobs, err := failedJobs(ctx, client)
	if err != nil {
		out = withNote(out, err)
	} else {
		out.FailedJobs = jobs
	}
	due, warn, err := certCounts(ctx, client, now)
	if err != nil {
		out = withNote(out, err)
	} else {
		out.CertsDue = due
		out.CertsWarn = warn
	}
	return out, nil
}

func withNote(c Cluster, err error) Cluster {
	if c.Note != "" {
		c.Note += "; "
	}
	c.Note += err.Error()
	return c
}

func failedJobs(ctx context.Context, client kubernetes.Interface) (int, error) {
	var n int
	token := ""
	for {
		list, err := client.BatchV1().Jobs("").List(ctx, metav1.ListOptions{Limit: 200, Continue: token})
		if err != nil {
			return 0, fmt.Errorf("list jobs: %w", err)
		}
		for _, job := range list.Items {
			if jobFailed(job) {
				n++
			}
		}
		if list.Continue == "" {
			return n, nil
		}
		token = list.Continue
	}
}

func jobFailed(job batchv1.Job) bool {
	for _, cond := range job.Status.Conditions {
		if cond.Type == batchv1.JobFailed && cond.Status == corev1.ConditionTrue {
			return true
		}
	}
	return false
}

func certCounts(ctx context.Context, client kubernetes.Interface, now time.Time) (due, warn int, err error) {
	mats, err := certs.Collect(ctx, client, "", true)
	if err != nil {
		return 0, 0, fmt.Errorf("list certificates: %w", err)
	}
	rep := certs.Analyze(mats, certs.Options{
		AllNamespaces: true,
		Now:           now,
		WarnDays:      30,
		CriticalDays:  7,
	})
	return rep.Summary.Expired + rep.Summary.Critical, rep.Summary.Warning, nil
}
