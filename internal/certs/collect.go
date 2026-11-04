package certs

import (
	"context"
	"errors"
	"fmt"

	corev1 "k8s.io/api/core/v1"
	networkingv1 "k8s.io/api/networking/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes"
)

// Collect lists Ingress TLS references and kubernetes.io/tls secrets.
// namespace is ignored when allNamespaces is set. Secrets of other types are skipped.
// Secret key material is not copied into the result; only tls.crt is kept.
func Collect(ctx context.Context, client kubernetes.Interface, namespace string, allNamespaces bool) ([]Material, error) {
	if client == nil {
		return nil, errors.New("kubernetes client is nil")
	}
	if !allNamespaces && namespace == "" {
		return nil, errors.New("namespace is required when all-namespaces is false")
	}
	ns := namespace
	if allNamespaces {
		ns = ""
	}

	var secrets []corev1.Secret
	err := listEvery(ctx, func(token string) (string, error) {
		list, err := client.CoreV1().Secrets(ns).List(ctx, metav1.ListOptions{Limit: 500, Continue: token})
		if err != nil {
			return "", fmt.Errorf("list secrets: %w", err)
		}
		for _, item := range list.Items {
			if item.Type == corev1.SecretTypeTLS {
				secrets = append(secrets, item)
			}
		}
		return list.Continue, nil
	})
	if err != nil {
		return nil, err
	}

	var ingresses []networkingv1.Ingress
	err = listEvery(ctx, func(token string) (string, error) {
		list, err := client.NetworkingV1().Ingresses(ns).List(ctx, metav1.ListOptions{Limit: 500, Continue: token})
		if err != nil {
			return "", fmt.Errorf("list ingresses: %w", err)
		}
		ingresses = append(ingresses, list.Items...)
		return list.Continue, nil
	})
	if err != nil {
		return nil, err
	}
	return materialsFrom(secrets, ingresses), nil
}

func materialsFrom(secrets []corev1.Secret, ingresses []networkingv1.Ingress) []Material {
	byKey := map[string]*Material{}
	var order []string
	for _, secret := range secrets {
		key := joinMeta(secret.Namespace, secret.Name)
		m := &Material{
			Source:      SourceSecret,
			Namespace:   secret.Namespace,
			Name:        secret.Name,
			SecretName:  secret.Name,
			Resource:    "secret " + key,
			Annotations: copyMap(secret.Annotations),
			PEM:         secretCertPEM(secret),
		}
		byKey[key] = m
		order = append(order, key)
	}
	var missing []Material
	for _, ing := range ingresses {
		ref := "ingress " + joinMeta(ing.Namespace, ing.Name)
		for _, tls := range ing.Spec.TLS {
			if tls.SecretName == "" {
				continue
			}
			key := joinMeta(ing.Namespace, tls.SecretName)
			if m, ok := byKey[key]; ok {
				m.Hosts = append(m.Hosts, tls.Hosts...)
				m.Ingresses = append(m.Ingresses, ref)
				continue
			}
			missing = append(missing, Material{
				Source:     SourceIngress,
				Namespace:  ing.Namespace,
				Name:       ing.Name,
				SecretName: tls.SecretName,
				Resource:   "secret " + key,
				Hosts:      append([]string(nil), tls.Hosts...),
				Ingresses:  []string{ref},
				Missing:    true,
			})
		}
	}
	out := make([]Material, 0, len(order)+len(missing))
	for _, key := range order {
		out = append(out, *byKey[key])
	}
	out = append(out, missing...)
	return out
}

func secretCertPEM(secret corev1.Secret) []byte {
	if b := secret.Data[corev1.TLSCertKey]; len(b) > 0 {
		return append([]byte(nil), b...)
	}
	if secret.StringData != nil {
		if v := secret.StringData[corev1.TLSCertKey]; v != "" {
			return []byte(v)
		}
	}
	return nil
}

func copyMap(in map[string]string) map[string]string {
	if len(in) == 0 {
		return nil
	}
	out := make(map[string]string, len(in))
	for k, v := range in {
		out[k] = v
	}
	return out
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
