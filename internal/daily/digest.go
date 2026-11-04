package daily

import (
	"fmt"
	"strconv"
)

const (
	SevCritical = "critical"
	SevWarning  = "warning"
	SevInfo     = "info"
)

// Digest rolls cluster counts into a Slack-sized brief.
// Cluster order is preserved so a contexts file stays stable.
func Digest(clusters []Cluster, demo bool) Report {
	findings := make([]Finding, 0, len(clusters)+1)
	findings = append(findings, headline(clusters))
	for _, c := range clusters {
		findings = append(findings, clusterFinding(c))
	}
	notes := []string{
		"Counts come from the same signals as sre k8s-pulse (NotReady nodes, CrashLoopBackOff, OOMKilled, Warning events) plus failed Jobs and the sre certs 7-day and 30-day windows.",
		"Paste `sre daily --markdown` into Slack or Feishu. A cron or GitHub Actions schedule can run that each morning.",
		"This command only reads the clusters. It does not change them.",
	}
	if demo {
		notes = append([]string{"Demo mode is two synthetic contexts, prod-eu and prod-us. It does not contact a cluster."}, notes...)
	}
	return Report{Demo: demo, Clusters: clusters, Findings: findings, Notes: notes}
}

func headline(clusters []Cluster) Finding {
	need := make([]string, 0, len(clusters))
	quiet := 0
	unreachable := 0
	for _, c := range clusters {
		if c.Err != "" {
			unreachable++
			need = append(need, c.Context)
			continue
		}
		if c.Attention() {
			need = append(need, c.Context)
			continue
		}
		quiet++
	}
	sev := SevInfo
	why := "Nothing in the digest is above the quiet line. Warning events and certificates inside 30 days still show on the cluster line."
	switch {
	case unreachable > 0 || hasCritical(clusters):
		sev = SevCritical
		why = "At least one context has NotReady nodes, CrashLoop or OOM pods, a failed Job, a certificate inside 7 days, or could not be read."
	case len(need) > 0:
		sev = SevWarning
		why = "No hard outage signal, but a context has Warning events or certificates inside 30 days."
	}
	what := fmt.Sprintf("%d contexts. %d quiet.", len(clusters), quiet)
	if len(need) > 0 {
		what = fmt.Sprintf("%d contexts. Look at %s before the day starts. %d quiet.", len(clusters), join(need), quiet)
	}
	next := "sre k8s-pulse -A"
	for _, c := range clusters {
		if c.Attention() {
			next = c.Next()
			break
		}
	}
	return Finding{
		Severity:    sev,
		Category:    "Digest",
		Resource:    "morning digest",
		Phenomenon:  what,
		LikelyCause: why,
		NextStep:    next,
		Details: []Detail{
			{Name: "contexts", Value: strconv.Itoa(len(clusters))},
			{Name: "attention", Value: strconv.Itoa(len(need))},
		},
	}
}

func hasCritical(clusters []Cluster) bool {
	for _, c := range clusters {
		if c.Severity() == SevCritical {
			return true
		}
	}
	return false
}

func clusterFinding(c Cluster) Finding {
	return Finding{
		Severity:    c.Severity(),
		Category:    "Cluster",
		Resource:    c.Context,
		Phenomenon:  c.Line(),
		LikelyCause: c.Why(),
		NextStep:    c.Next(),
		Details: []Detail{
			{Name: "nodesNotReady", Value: strconv.Itoa(c.NodesNotReady)},
			{Name: "nodes", Value: strconv.Itoa(c.NodesTotal)},
			{Name: "crashLoop", Value: strconv.Itoa(c.CrashLoop)},
			{Name: "oomKilled", Value: strconv.Itoa(c.OOMKilled)},
			{Name: "failedJobs", Value: strconv.Itoa(c.FailedJobs)},
			{Name: "warningEvents", Value: strconv.Itoa(c.WarningEvents)},
			{Name: "certsDue", Value: strconv.Itoa(c.CertsDue)},
			{Name: "certsWarn", Value: strconv.Itoa(c.CertsWarn)},
		},
	}
}

// Severity is critical for availability failures, warning for softer signals.
func (c Cluster) Severity() string {
	if c.Err != "" || c.NodesNotReady > 0 || c.CrashLoop > 0 || c.OOMKilled > 0 || c.FailedJobs > 0 || c.CertsDue > 0 {
		return SevCritical
	}
	if c.WarningEvents > 0 || c.CertsWarn > 0 {
		return SevWarning
	}
	return SevInfo
}

// Line is the one-line digest for Slack or Feishu.
func (c Cluster) Line() string {
	if c.Err != "" {
		return "Unreachable: " + c.Err
	}
	return fmt.Sprintf("%d/%d nodes NotReady · crashloop %d · oom %d · failed jobs %d · warnings %d · certs due %d · certs warn %d",
		c.NodesNotReady, c.NodesTotal, c.CrashLoop, c.OOMKilled, c.FailedJobs, c.WarningEvents, c.CertsDue, c.CertsWarn)
}

// Why explains the line in one sentence.
func (c Cluster) Why() string {
	if c.Err != "" {
		return "The context could not be read. The rest of the digest does not include this cluster."
	}
	if c.Severity() == SevInfo {
		return "Nodes are Ready, no CrashLoop or OOM pods, no failed Jobs, and no certificate inside 30 days."
	}
	if c.NodesNotReady > 0 || c.CrashLoop > 0 || c.OOMKilled > 0 || c.FailedJobs > 0 {
		msg := "Availability signals are non-zero. Start with k8s-pulse before treating the certificate count."
		if c.Note != "" {
			msg += " " + c.Note
		}
		return msg
	}
	msg := "Availability looks quiet. Certificates or warning events still need a look."
	if c.Note != "" {
		msg += " " + c.Note
	}
	return msg
}

// Next is the follow-up command for this context.
func (c Cluster) Next() string {
	if c.Err != "" {
		return "kubectl config get-contexts " + c.Context
	}
	cmd := fmt.Sprintf("sre k8s-pulse --context %s -A", c.Context)
	if c.CertsDue > 0 || c.CertsWarn > 0 {
		cmd += fmt.Sprintf("; sre certs --context %s -A", c.Context)
	}
	if c.FailedJobs > 0 {
		cmd += fmt.Sprintf("; kubectl get jobs -A --context %s", c.Context)
	}
	return cmd
}

func join(names []string) string {
	switch len(names) {
	case 0:
		return ""
	case 1:
		return names[0]
	case 2:
		return names[0] + " and " + names[1]
	default:
		out := ""
		for i, n := range names {
			if i == len(names)-1 {
				out += "and " + n
				break
			}
			out += n + ", "
		}
		return out
	}
}
