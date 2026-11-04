package diffreview

import (
	"fmt"
	"sort"
)

// Compare diffs two manifest sets. Only the allowlisted risk fields are
// compared. Identical objects produce no findings.
func Compare(before, after []Object, demo bool, beforePath, afterPath string) Report {
	bIndex := index(before)
	aIndex := index(after)
	keys := make([]string, 0, len(bIndex)+len(aIndex))
	seen := map[string]struct{}{}
	for k := range bIndex {
		seen[k] = struct{}{}
		keys = append(keys, k)
	}
	for k := range aIndex {
		if _, ok := seen[k]; !ok {
			keys = append(keys, k)
		}
	}
	sort.Strings(keys)

	var findings []Finding
	for _, k := range keys {
		b, bok := bIndex[k]
		a, aok := aIndex[k]
		switch {
		case bok && aok:
			findings = append(findings, diffObject(b, a)...)
		case aok:
			findings = append(findings, objectVerdict(a, addedVerdict(a), "(absent)", shortAdded(a)))
		default:
			findings = append(findings, objectVerdict(b, removedVerdict(b), b.Ref(), "(absent)"))
		}
	}
	sort.SliceStable(findings, func(i, j int) bool {
		if rank(findings[i].Severity) != rank(findings[j].Severity) {
			return rank(findings[i].Severity) < rank(findings[j].Severity)
		}
		if findings[i].Resource != findings[j].Resource {
			return findings[i].Resource < findings[j].Resource
		}
		return detail(findings[i], "field") < detail(findings[j], "field")
	})

	summary := Summary{ObjectsBefore: len(before), ObjectsAfter: len(after)}
	for _, f := range findings {
		switch f.Severity {
		case RiskCritical:
			summary.Critical++
		case RiskHigh:
			summary.High++
		case RiskMedium:
			summary.Medium++
		default:
			summary.Low++
		}
	}
	notes := []string{
		"Only release-risk fields are compared: images, resources, probes, replicas, HPA, volumes, Services, NetworkPolicy, RBAC, securityContext, and env/secretKeyRef.",
		"Render Helm with `helm template` and pass the two output directories as --before and --after. This command does not run Helm.",
	}
	if demo {
		notes = append([]string{"Demo mode uses built-in before/after manifests and does not read your files."}, notes...)
	}
	if len(findings) == 0 {
		notes = append(notes, "No high-risk field changes between these manifest sets.")
	}
	return Report{
		Demo:     demo,
		Before:   beforePath,
		After:    afterPath,
		Summary:  summary,
		Findings: findings,
		Notes:    notes,
	}
}

func index(objs []Object) map[string]Object {
	out := make(map[string]Object, len(objs))
	for _, o := range objs {
		out[o.Key()] = o
	}
	return out
}

func diffObject(before, after Object) []Finding {
	bf := fieldMap(extract(before))
	af := fieldMap(extract(after))
	paths := make([]string, 0, len(bf)+len(af))
	seen := map[string]struct{}{}
	for p := range bf {
		seen[p] = struct{}{}
		paths = append(paths, p)
	}
	for p := range af {
		if _, ok := seen[p]; !ok {
			paths = append(paths, p)
		}
	}
	sort.Strings(paths)
	var out []Finding
	for _, p := range paths {
		old, newv := bf[p].Value, af[p].Value
		if old == newv {
			continue
		}
		class := bf[p].Class
		if class == "" {
			class = af[p].Class
		}
		v := classify(class, p, old, newv)
		out = append(out, changeFinding(after, class, p, old, newv, v))
	}
	if after.Kind == "HorizontalPodAutoscaler" && hpaInverted(af) && !hpaInverted(bf) {
		v := verdict{
			Level:     RiskCritical,
			Why:       "minReplicas is greater than maxReplicas. The HPA spec is invalid and autoscaling will not do what the release expects.",
			Checklist: "Fix the range so min is less than or equal to max before shipping.",
		}
		out = append(out, changeFinding(after, ClassHPA, "spec.minReplicas>spec.maxReplicas", bf["spec.minReplicas"].Value+".."+bf["spec.maxReplicas"].Value, af["spec.minReplicas"].Value+".."+af["spec.maxReplicas"].Value, v))
	}
	return out
}

func fieldMap(fields []Field) map[string]Field {
	out := make(map[string]Field, len(fields))
	for _, f := range fields {
		out[f.Path] = f
	}
	return out
}

func changeFinding(obj Object, class, path, old, newv string, v verdict) Finding {
	if old == "" {
		old = "(absent)"
	}
	if newv == "" {
		newv = "(absent)"
	}
	return Finding{
		Severity:    v.Level,
		Category:    class,
		Resource:    obj.Ref(),
		Phenomenon:  fmt.Sprintf("Risk %s. %s: %s -> %s", v.Level, path, old, newv),
		LikelyCause: v.Why,
		NextStep:    v.Checklist,
		Details: []Detail{
			{Name: "risk", Value: v.Level},
			{Name: "field", Value: path},
			{Name: "old", Value: old},
			{Name: "new", Value: newv},
		},
	}
}

func objectVerdict(obj Object, v verdict, old, newv string) Finding {
	class := ClassAdded
	if v.Why != "" && newv == "(absent)" {
		class = ClassRemoved
	}
	if old == "(absent)" {
		class = ClassAdded
	}
	return changeFinding(obj, class, "object", old, newv, v)
}

func shortAdded(obj Object) string {
	if role := roleRef(obj); role != "" {
		return obj.Ref() + " roleRef=" + role
	}
	return obj.Ref()
}

func detail(f Finding, name string) string {
	for _, d := range f.Details {
		if d.Name == name {
			return d.Value
		}
	}
	return ""
}

func rank(level string) int {
	switch level {
	case RiskCritical:
		return 0
	case RiskHigh:
		return 1
	case RiskMedium:
		return 2
	case RiskLow:
		return 3
	default:
		return 4
	}
}
