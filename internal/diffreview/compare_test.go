package diffreview

import (
	"strconv"
	"strings"
	"testing"
)

func TestDemoRisks(t *testing.T) {
	before, after, err := DemoPair()
	if err != nil {
		t.Fatal(err)
	}
	rep := Compare(before, after, true, "demo-before", "demo-after")
	if rep.Summary.ObjectsBefore != 6 || rep.Summary.ObjectsAfter != 6 {
		t.Fatalf("objects = %d -> %d", rep.Summary.ObjectsBefore, rep.Summary.ObjectsAfter)
	}
	if rep.Summary.Critical < 4 || rep.Summary.High < 4 {
		t.Fatalf("summary = %+v", rep.Summary)
	}
	blob := render(rep)
	for _, want := range []string{
		"checkout:latest",
		"Risk critical",
		"privileged",
		"hostPath",
		"NET_ADMIN",
		"livenessProbe",
		"secretKeyRef checkout-secrets/api-v2",
		"LoadBalancer",
		"spec.replicas",
		"NetworkPolicy shop/checkout-net",
		"cluster-admin",
		"spec.minReplicas>spec.maxReplicas",
		"checkout-admin",
	} {
		if !strings.Contains(blob, want) {
			t.Fatalf("missing %q\n%s", want, blob)
		}
	}
	if strings.Contains(blob, "Deployment platform/edge") {
		t.Fatalf("unchanged edge was flagged\n%s", blob)
	}
	if !strings.Contains(rep.Notes[0], "built-in") {
		t.Fatalf("notes = %v", rep.Notes)
	}
}

func TestRiskClassification(t *testing.T) {
	cases := []struct {
		name          string
		before, after string
		wantRisk      string
		wantClass     string
		wantPath      string
	}{
		{
			name:      "tag only is medium",
			before:    deployImage("ghcr.io/example/checkout:1.4.2", 3),
			after:     deployImage("ghcr.io/example/checkout:1.4.3", 3),
			wantRisk:  RiskMedium,
			wantClass: ClassImage,
			wantPath:  "image",
		},
		{
			name:      "latest is critical",
			before:    deployImage("ghcr.io/example/checkout:1.4.2", 3),
			after:     deployImage("ghcr.io/example/checkout:latest", 3),
			wantRisk:  RiskCritical,
			wantClass: ClassImage,
		},
		{
			name:      "repo change is high",
			before:    deployImage("ghcr.io/example/checkout:1.4.2", 3),
			after:     deployImage("public.example/checkout:1.4.2", 3),
			wantRisk:  RiskHigh,
			wantClass: ClassImage,
		},
		{
			name:      "half replicas is high",
			before:    deployImage("ghcr.io/example/checkout:1.4.2", 4),
			after:     deployImage("ghcr.io/example/checkout:1.4.2", 2),
			wantRisk:  RiskHigh,
			wantClass: ClassReplicas,
		},
		{
			name:      "memory limit drop is high",
			before:    deployResources("256Mi", "128Mi"),
			after:     deployResources("128Mi", "128Mi"),
			wantRisk:  RiskHigh,
			wantClass: ClassResources,
		},
		{
			name:      "allow-all network policy is critical",
			before:    netpol(false),
			after:     netpol(true),
			wantRisk:  RiskCritical,
			wantClass: ClassNetworkPolicy,
		},
		{
			name:      "wildcard role is critical",
			before:    role(false),
			after:     role(true),
			wantRisk:  RiskCritical,
			wantClass: ClassRBAC,
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			b, err := ParseYAML("before", []byte(tc.before))
			if err != nil {
				t.Fatal(err)
			}
			a, err := ParseYAML("after", []byte(tc.after))
			if err != nil {
				t.Fatal(err)
			}
			rep := Compare(b, a, false, "before", "after")
			f := findClass(rep, tc.wantClass, tc.wantPath)
			if f.Severity != tc.wantRisk {
				t.Fatalf("severity=%s want %s\n%s", f.Severity, tc.wantRisk, f.Phenomenon)
			}
		})
	}
}

func TestNoChange(t *testing.T) {
	src := deployImage("ghcr.io/example/checkout:1.4.2", 3)
	b, err := ParseYAML("b", []byte(src))
	if err != nil {
		t.Fatal(err)
	}
	a, err := ParseYAML("a", []byte(src))
	if err != nil {
		t.Fatal(err)
	}
	rep := Compare(b, a, false, "b", "a")
	if len(rep.Findings) != 0 {
		t.Fatalf("findings = %+v", rep.Findings)
	}
}

func findClass(rep Report, class, pathSub string) Finding {
	for _, f := range rep.Findings {
		if f.Category != class {
			continue
		}
		if pathSub == "" || strings.Contains(f.Phenomenon, pathSub) {
			return f
		}
	}
	return Finding{}
}

func render(rep Report) string {
	var b strings.Builder
	for _, f := range rep.Findings {
		b.WriteString(f.Severity)
		b.WriteByte(' ')
		b.WriteString(f.Category)
		b.WriteByte(' ')
		b.WriteString(f.Resource)
		b.WriteByte('\n')
		b.WriteString(f.Phenomenon)
		b.WriteByte('\n')
		b.WriteString(f.LikelyCause)
		b.WriteByte('\n')
		b.WriteString(f.NextStep)
		b.WriteByte('\n')
	}
	return b.String()
}

func deployImage(image string, replicas int) string {
	return strings.ReplaceAll(strings.ReplaceAll(`apiVersion: apps/v1
kind: Deployment
metadata:
  name: checkout
  namespace: shop
spec:
  replicas: REPLICAS
  template:
    spec:
      containers:
      - name: checkout
        image: IMAGE
`, "IMAGE", image), "REPLICAS", itoa(replicas))
}

func itoa(n int) string {
	return strconv.Itoa(n)
}

func deployResources(limit, request string) string {
	return `apiVersion: apps/v1
kind: Deployment
metadata:
  name: checkout
  namespace: shop
spec:
  replicas: 1
  template:
    spec:
      containers:
      - name: checkout
        image: ghcr.io/example/checkout:1.4.2
        resources:
          requests:
            memory: ` + request + `
          limits:
            memory: ` + limit + `
`
}

func netpol(allowAll bool) string {
	rule := `  ingress:
  - from:
    - podSelector:
        matchLabels:
          app: api
`
	if allowAll {
		rule = `  ingress:
  - ports:
    - port: 80
`
	}
	return `apiVersion: networking.k8s.io/v1
kind: NetworkPolicy
metadata:
  name: checkout-net
  namespace: shop
spec:
  podSelector:
    matchLabels:
      app: checkout
  policyTypes:
  - Ingress
` + rule
}

func role(wildcard bool) string {
	rules := `rules:
- apiGroups: [""]
  resources: ["pods"]
  verbs: ["get"]
`
	if wildcard {
		rules = `rules:
- apiGroups: ["*"]
  resources: ["*"]
  verbs: ["*"]
`
	}
	return `apiVersion: rbac.authorization.k8s.io/v1
kind: ClusterRole
metadata:
  name: checkout
` + rules
}
