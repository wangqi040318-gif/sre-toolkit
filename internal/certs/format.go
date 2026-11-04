package certs

import (
	"fmt"
	"sort"
	"strconv"
	"strings"
	"time"
)

func formatExpiry(p Parsed, tier string, days, criticalDays, warnDays int) string {
	when := p.NotAfter.UTC().Format(time.RFC3339)
	switch tier {
	case TierExpired:
		if days <= 0 {
			return fmt.Sprintf("Certificate %s expired less than 1 day ago (notAfter %s). Severity: expired.", subjectLabel(p), when)
		}
		return fmt.Sprintf("Certificate %s expired %d days ago (notAfter %s). Severity: expired.", subjectLabel(p), days, when)
	case TierCritical:
		return fmt.Sprintf("Certificate %s expires %s (%d days remaining). Severity: critical (≤%dd).", subjectLabel(p), when, days, criticalDays)
	case TierWarn:
		return fmt.Sprintf("Certificate %s expires %s (%d days remaining). Severity: warn (≤%dd).", subjectLabel(p), when, days, warnDays)
	default:
		return fmt.Sprintf("Certificate %s expires %s (%d days remaining). Severity: ok (outside ≤%dd).", subjectLabel(p), when, days, warnDays)
	}
}

func subjectLabel(p Parsed) string {
	if p.IsCA {
		return "CA " + p.Subject
	}
	return p.Subject
}

func causeForTier(tier string, criticalDays, warnDays int) string {
	switch tier {
	case TierExpired:
		return "The certificate is past notAfter. TLS clients will fail the handshake until it is renewed and the serving secret or file is updated."
	case TierCritical:
		return fmt.Sprintf("%d or fewer days remain, inside the critical window (≤%dd). Renewal should already be in progress.", criticalDays, criticalDays)
	case TierWarn:
		return fmt.Sprintf("Inside the warn window (≤%dd) and outside the critical window (≤%dd). Schedule renewal before it becomes critical.", warnDays, criticalDays)
	default:
		return fmt.Sprintf("Outside the warn window (≤%dd). No rotation is due yet.", warnDays)
	}
}

func nextStep(m Material, tier string) string {
	certName := annotation(m, "cert-manager.io/certificate-name")
	switch {
	case m.Source == SourceFile:
		return fileNext(m.Resource, tier)
	case certName != "":
		return certManagerNext(m.Namespace, certName, m.Name, tier)
	default:
		return manualSecretNext(m.Namespace, m.Name, tier)
	}
}

func fileNext(path, tier string) string {
	switch tier {
	case TierExpired, TierCritical:
		return "Renew the certificate and replace " + path + ". Reload the process that reads it."
	case TierWarn:
		return "Plan renewal of " + path + " before the critical window. Reload the process after replacing it."
	default:
		return "No change needed for " + path + ". Check it again before the warn window."
	}
}

func certManagerNext(ns, certName, secretName, tier string) string {
	base := fmt.Sprintf("kubectl describe certificate -n %s %s; kubectl get certificaterequest -n %s", ns, certName, ns)
	switch tier {
	case TierExpired, TierCritical:
		return base + "; if the Certificate is not Ready, describe the issuer and the latest order, then confirm secret " + secretName + " rotates"
	case TierWarn:
		return base + "; confirm status.renewalTime is soon so secret " + secretName + " rotates before the critical window"
	default:
		return base + "; secret " + secretName + " is still inside its healthy window"
	}
}

func manualSecretNext(ns, name, tier string) string {
	ref := ns + "/" + name
	switch tier {
	case TierExpired, TierCritical:
		return "No cert-manager annotation was found. Renew the certificate and rotate secret " + ref + " (replace tls.crt and tls.key), then confirm serving workloads reloaded it. kubectl describe secret -n " + ns + " " + name
	case TierWarn:
		return "No cert-manager annotation was found. Schedule a rotation of secret " + ref + " before the critical window. kubectl describe secret -n " + ns + " " + name
	default:
		return "No cert-manager annotation was found. Secret " + ref + " is outside the warn window. Recheck before then. kubectl describe secret -n " + ns + " " + name
	}
}

func missingPhenomenon(m Material) string {
	hosts := "no hosts listed"
	if len(m.Hosts) > 0 {
		hosts = strings.Join(m.Hosts, ", ")
	}
	secret := m.SecretName
	if secret == "" {
		secret = secretTail(m.Resource)
	}
	return fmt.Sprintf("Ingress %s/%s references TLS secret %s/%s for %s, but that kubernetes.io/tls secret was not found. Severity: critical.", m.Namespace, m.Name, m.Namespace, secret, hosts)
}

func missingCause() string {
	return "The Ingress cannot serve that TLS host until the secret exists. The reference is broken, or the certificate has not been issued yet."
}

func contextSentence(m Material, p Parsed) string {
	var parts []string
	if len(m.Ingresses) > 0 {
		parts = append(parts, "Referenced by "+strings.Join(m.Ingresses, ", ")+".")
	}
	if len(m.Hosts) > 0 {
		parts = append(parts, "Ingress hosts: "+strings.Join(m.Hosts, ", ")+".")
	}
	missing := missingHosts(m.Hosts, p)
	if len(missing) > 0 {
		parts = append(parts, "Ingress hosts not on the certificate: "+strings.Join(missing, ", ")+".")
	}
	return strings.Join(parts, " ")
}

func missingHosts(hosts []string, p Parsed) []string {
	if len(hosts) == 0 {
		return nil
	}
	have := map[string]struct{}{}
	if p.Subject != "" {
		have[strings.ToLower(p.Subject)] = struct{}{}
	}
	for _, dns := range p.DNSNames {
		have[strings.ToLower(dns)] = struct{}{}
	}
	var missing []string
	for _, h := range hosts {
		if _, ok := have[strings.ToLower(h)]; !ok {
			missing = append(missing, h)
		}
	}
	return missing
}

func secretTail(resource string) string {
	if i := strings.LastIndex(resource, "/"); i >= 0 && i < len(resource)-1 {
		return resource[i+1:]
	}
	return resource
}

func annotation(m Material, key string) string {
	if m.Annotations == nil {
		return ""
	}
	return m.Annotations[key]
}

func certDetails(p Parsed, tier string, days int, m Material) []Detail {
	var d []Detail
	d = addDetail(d, "tier", tier)
	d = addDetail(d, "subject", p.Subject)
	d = addDetail(d, "issuer", p.Issuer)
	d = addDetail(d, "notAfter", p.NotAfter.UTC().Format(time.RFC3339))
	d = addDetail(d, "daysRemaining", strconv.Itoa(signedDays(tier, days)))
	if len(p.DNSNames) > 0 {
		d = addDetail(d, "dnsNames", strings.Join(p.DNSNames, ", "))
	}
	if len(m.Hosts) > 0 {
		d = addDetail(d, "hosts", strings.Join(m.Hosts, ", "))
	}
	if len(m.Ingresses) > 0 {
		d = addDetail(d, "ingresses", strings.Join(m.Ingresses, ", "))
	}
	if name := annotation(m, "cert-manager.io/certificate-name"); name != "" {
		d = addDetail(d, "certificate", name)
	}
	if p.IsCA {
		d = addDetail(d, "ca", "true")
	}
	return d
}

func signedDays(tier string, days int) int {
	if tier == TierExpired {
		if days == 0 {
			return 0
		}
		return -days
	}
	return days
}

func addDetail(details []Detail, name, value string) []Detail {
	if value == "" {
		return details
	}
	return append(details, Detail{Name: name, Value: value})
}

func uniqSorted(in []string) []string {
	if len(in) == 0 {
		return nil
	}
	seen := make(map[string]struct{}, len(in))
	out := make([]string, 0, len(in))
	for _, s := range in {
		s = strings.TrimSpace(s)
		if s == "" {
			continue
		}
		if _, ok := seen[s]; ok {
			continue
		}
		seen[s] = struct{}{}
		out = append(out, s)
	}
	sort.Strings(out)
	return out
}

func joinMeta(ns, name string) string {
	if ns == "" {
		return name
	}
	return ns + "/" + name
}
