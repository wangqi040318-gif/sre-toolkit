package pulse

import (
	"context"
	"errors"
	"fmt"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes"
)

// Collect lists nodes, pods, and events. namespace is ignored when
// allNamespaces is set. Nodes are always cluster-scoped.
func Collect(ctx context.Context, client kubernetes.Interface, namespace string, allNamespaces bool) (Snapshot, error) {
	if client == nil {
		return Snapshot{}, errors.New("kubernetes client is nil")
	}
	if !allNamespaces && namespace == "" {
		return Snapshot{}, errors.New("namespace is required when all-namespaces is false")
	}
	podNS := namespace
	if allNamespaces {
		podNS = ""
	}

	var snap Snapshot
	err := listEvery(ctx, func(token string) (string, error) {
		list, err := client.CoreV1().Nodes().List(ctx, metav1.ListOptions{Limit: 500, Continue: token})
		if err != nil {
			return "", fmt.Errorf("list nodes: %w", err)
		}
		snap.Nodes = append(snap.Nodes, list.Items...)
		return list.Continue, nil
	})
	if err != nil {
		return Snapshot{}, err
	}

	err = listEvery(ctx, func(token string) (string, error) {
		list, err := client.CoreV1().Pods(podNS).List(ctx, metav1.ListOptions{Limit: 500, Continue: token})
		if err != nil {
			return "", fmt.Errorf("list pods: %w", err)
		}
		snap.Pods = append(snap.Pods, list.Items...)
		return list.Continue, nil
	})
	if err != nil {
		return Snapshot{}, err
	}

	err = listEvery(ctx, func(token string) (string, error) {
		list, err := client.CoreV1().Events(podNS).List(ctx, metav1.ListOptions{Limit: 500, Continue: token})
		if err != nil {
			return "", fmt.Errorf("list events: %w", err)
		}
		snap.Events = append(snap.Events, list.Items...)
		return list.Continue, nil
	})
	if err != nil {
		return Snapshot{}, err
	}
	return snap, nil
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
