package certs

import (
	"fmt"
	"sort"
	"strings"
)

// Analyze parses each material and classifies notAfter.
// Local files (empty namespace, source file) stay visible when a namespace is selected.
func Analyze(materials []Material, opts Options) Report {
	opts = opts.withDefaults()
	cluster := opts.Cluster
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
	for _, m := range materials {
		if m.Source != SourceFile && !opts.includeNS(m.Namespace) {
			continue
		}
		switch m.Source {
		case SourceSecret:
			summary.Secrets++
		case SourceIngress:
			summary.Ingresses++
		case SourceFile:
			summary.Files++
		}
		m.Hosts = uniqSorted(m.Hosts)
		m.Ingresses = uniqSorted(m.Ingresses)
		if m.Missing {
			summary.Missing++
			emit(missingFinding(m))
			continue
		}
		parsed, err := ParsePEM(m.PEM)
		if err != nil {
			summary.Unreadable++
			emit(unreadableFinding(m, err))
			continue
		}
		for _, cert := range parsed {
			summary.Certificates++
			tier, days := Classify(cert.NotAfter, opts.Now, opts.CriticalDays, opts.WarnDays)
			switch tier {
			case TierExpired:
				summary.Expired++
			case TierCritical:
				summary.Critical++
			case TierWarn:
				summary.Warning++
			default:
				summary.OK++
			}
			emit(certFinding(m, cert, tier, days, opts))
		}
	}

	sort.SliceStable(findings, func(i, j int) bool {
		if rankTier(findings[i]) != rankTier(findings[j]) {
			return rankTier(findings[i]) < rankTier(findings[j])
		}
		if findings[i].days != findings[j].days {
			// More overdue (larger positive stored days on expired) first.
			// Sooner future expiry (smaller days) first.
			if findings[i].Category == CatExpired && findings[j].Category == CatExpired {
				return findings[i].days > findings[j].days
			}
			return findings[i].days < findings[j].days
		}
		return findings[i].seq < findings[j].seq
	})

	notes := make([]string, 0, 4)
	if opts.Demo {
		notes = append(notes, "Demo mode uses synthetic certificates and does not contact a cluster.")
	}
	notes = append(notes, "Private key material is never printed.")
	if !opts.AllNamespaces && opts.Namespace != "" && opts.FilesScoped {
		notes = append(notes, "Local PEM files are included even when a namespace is selected.")
	}
	if summary.Certificates == 0 && summary.Missing == 0 && summary.Unreadable == 0 {
		notes = append(notes, "No TLS secrets, Ingress TLS references, or local certificates were found in this scope.")
	}

	return Report{
		GeneratedAt:  opts.Now.UTC(),
		Demo:         opts.Demo,
		Cluster:      cluster,
		Scope:        opts.ScopeLabel(),
		WarnDays:     opts.WarnDays,
		CriticalDays: opts.CriticalDays,
		Summary:      summary,
		Findings:     findings,
		Notes:        notes,
	}
}

func certFinding(m Material, p Parsed, tier string, days int, opts Options) Finding {
	phenomenon := formatExpiry(p, tier, days, opts.CriticalDays, opts.WarnDays)
	if extra := contextSentence(m, p); extra != "" {
		phenomenon = phenomenon + " " + extra
	}
	if missed := missingHosts(m.Hosts, p); len(missed) > 0 && tier != TierExpired {
		phenomenon += " Reissue the certificate so those hosts are on the SAN list."
	}
	sev, cat := tierPresentation(tier)
	storedDays := days
	if tier == TierExpired {
		storedDays = days
	}
	return Finding{
		Severity:    sev,
		Category:    cat,
		Resource:    m.Resource,
		Phenomenon:  phenomenon,
		LikelyCause: causeForTier(tier, opts.CriticalDays, opts.WarnDays),
		NextStep:    augmentNext(nextStep(m, tier), missingHosts(m.Hosts, p)),
		Details:     certDetails(p, tier, days, m),
		days:        storedDays,
	}
}

func augmentNext(step string, missing []string) string {
	if len(missing) == 0 {
		return step
	}
	return step + "; reissue with SAN " + strings.Join(missing, ", ")
}

func missingFinding(m Material) Finding {
	secretName := m.SecretName
	if secretName == "" {
		secretName = secretTail(m.Resource)
	}
	return Finding{
		Severity:    SevCritical,
		Category:    CatMissing,
		Resource:    "ingress " + joinMeta(m.Namespace, m.Name),
		Phenomenon:  missingPhenomenon(m),
		LikelyCause: missingCause(),
		NextStep:    fmt.Sprintf("kubectl describe ingress -n %s %s; create kubernetes.io/tls secret %s or fix spec.tls.secretName. If cert-manager manages this Ingress, check the Certificate and Issuer.", m.Namespace, m.Name, secretName),
		Details: []Detail{
			{Name: "tier", Value: TierCritical},
			{Name: "secret", Value: joinMeta(m.Namespace, secretName)},
			{Name: "hosts", Value: strings.Join(m.Hosts, ", ")},
		},
		days: -1,
	}
}

func unreadableFinding(m Material, err error) Finding {
	return Finding{
		Severity:    SevCritical,
		Category:    CatUnreadable,
		Resource:    m.Resource,
		Phenomenon:  fmt.Sprintf("%s could not be parsed: %s. Severity: critical.", m.Resource, err.Error()),
		LikelyCause: "The scanner found a TLS secret or PEM file but tls.crt is empty, not a certificate, or truncated.",
		NextStep:    "Replace the certificate PEM. For a secret, confirm tls.crt contains a CERTIFICATE block and tls.key is a matching key. Do not paste the key into logs.",
		Details: []Detail{
			{Name: "tier", Value: "unreadable"},
			{Name: "error", Value: err.Error()},
		},
		days: -1,
	}
}

func tierPresentation(tier string) (severity, category string) {
	switch tier {
	case TierExpired:
		return SevCritical, CatExpired
	case TierCritical:
		return SevCritical, CatDueSoon
	case TierWarn:
		return SevWarning, CatExpiring
	default:
		return SevOK, CatHealthy
	}
}

func rankTier(f Finding) int {
	switch f.Category {
	case CatExpired:
		return 0
	case CatDueSoon:
		return 1
	case CatMissing:
		return 2
	case CatUnreadable:
		return 3
	case CatExpiring:
		return 4
	case CatHealthy:
		return 5
	default:
		return 6
	}
}
