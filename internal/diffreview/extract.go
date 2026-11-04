package diffreview

import (
	"fmt"
	"sort"
	"strings"

	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
)

func extract(obj Object) []Field {
	var fields []Field
	fields = append(fields, extractPod(obj)...)
	fields = append(fields, extractReplicas(obj)...)
	fields = append(fields, extractHPA(obj)...)
	fields = append(fields, extractService(obj)...)
	fields = append(fields, extractNetworkPolicy(obj)...)
	fields = append(fields, extractRBAC(obj)...)
	return fields
}

func extractReplicas(obj Object) []Field {
	switch obj.Kind {
	case "Deployment", "StatefulSet", "ReplicaSet", "ReplicationController":
	default:
		return nil
	}
	v, found, _ := unstructured.NestedFieldNoCopy(obj.Raw, "spec", "replicas")
	if !found || v == nil {
		return nil
	}
	return []Field{{Path: "spec.replicas", Value: scalar(v), Class: ClassReplicas}}
}

func extractPod(obj Object) []Field {
	spec, prefix := podSpec(obj)
	if spec == nil {
		return nil
	}
	var fields []Field
	fields = append(fields, securityFields(prefix+"securityContext", asMap(spec["securityContext"]))...)
	for _, key := range []string{"hostNetwork", "hostPID", "hostIPC"} {
		if v, ok := spec[key]; ok && v != nil {
			fields = append(fields, Field{Path: prefix + key, Value: scalar(v), Class: ClassSecurity})
		}
	}
	fields = append(fields, containerFields(prefix+"containers", spec["containers"])...)
	fields = append(fields, containerFields(prefix+"initContainers", spec["initContainers"])...)
	fields = append(fields, volumeFields(prefix+"volumes", spec["volumes"])...)
	return fields
}

func podSpec(obj Object) (map[string]any, string) {
	switch obj.Kind {
	case "Pod":
		return asMap(nested(obj.Raw, "spec")), "spec."
	case "CronJob":
		return asMap(nested(obj.Raw, "spec", "jobTemplate", "spec", "template", "spec")), "spec.jobTemplate.spec.template.spec."
	case "Deployment", "StatefulSet", "DaemonSet", "ReplicaSet", "Job":
		return asMap(nested(obj.Raw, "spec", "template", "spec")), "spec.template.spec."
	default:
		return nil, ""
	}
}

func containerFields(prefix string, raw any) []Field {
	var fields []Field
	for _, c := range asMaps(raw) {
		name := scalar(c["name"])
		if name == "" {
			name = "?"
		}
		base := prefix + "[" + name + "]"
		if v, ok := c["image"]; ok && v != nil {
			fields = append(fields, Field{Path: base + ".image", Value: scalar(v), Class: ClassImage})
		}
		fields = append(fields, resourceFields(base+".resources", asMap(c["resources"]))...)
		for _, probe := range []string{"livenessProbe", "readinessProbe", "startupProbe"} {
			if c[probe] != nil {
				fields = append(fields, Field{Path: base + "." + probe, Value: probeValue(asMap(c[probe])), Class: ClassProbe})
			}
		}
		fields = append(fields, envFields(base+".env", c["env"])...)
		fields = append(fields, securityFields(base+".securityContext", asMap(c["securityContext"]))...)
		fields = append(fields, mountFields(base+".volumeMounts", c["volumeMounts"])...)
	}
	return fields
}

func resourceFields(prefix string, res map[string]any) []Field {
	if res == nil {
		return nil
	}
	var fields []Field
	for _, bucket := range []string{"requests", "limits"} {
		m := asMap(res[bucket])
		if m == nil {
			continue
		}
		keys := make([]string, 0, len(m))
		for k := range m {
			keys = append(keys, k)
		}
		sort.Strings(keys)
		for _, k := range keys {
			fields = append(fields, Field{
				Path:  prefix + "." + bucket + "." + k,
				Value: scalar(m[k]),
				Class: ClassResources,
			})
		}
	}
	return fields
}

func probeValue(p map[string]any) string {
	if p == nil {
		return ""
	}
	var kind, detail string
	switch {
	case p["httpGet"] != nil:
		kind = "httpGet"
		h := asMap(p["httpGet"])
		detail = fmt.Sprintf("path=%s port=%s", scalar(h["path"]), scalar(h["port"]))
	case p["tcpSocket"] != nil:
		kind = "tcpSocket"
		detail = "port=" + scalar(asMap(p["tcpSocket"])["port"])
	case p["exec"] != nil:
		kind = "exec"
		detail = "command=" + scalar(asMap(p["exec"])["command"])
	case p["grpc"] != nil:
		kind = "grpc"
		detail = "port=" + scalar(asMap(p["grpc"])["port"])
	default:
		kind = "custom"
	}
	parts := []string{kind, detail}
	for _, k := range []string{"initialDelaySeconds", "periodSeconds", "timeoutSeconds", "failureThreshold", "successThreshold"} {
		if p[k] != nil {
			parts = append(parts, k+"="+scalar(p[k]))
		}
	}
	return strings.Join(parts, " ")
}

func envFields(prefix string, raw any) []Field {
	var fields []Field
	for _, env := range asMaps(raw) {
		name := scalar(env["name"])
		if name == "" {
			continue
		}
		val := "(present)"
		switch {
		case env["value"] != nil:
			val = "value=" + scalar(env["value"])
		case env["valueFrom"] != nil:
			from := asMap(env["valueFrom"])
			if sk := asMap(from["secretKeyRef"]); sk != nil {
				val = fmt.Sprintf("secretKeyRef %s/%s", scalar(sk["name"]), scalar(sk["key"]))
			} else if cm := asMap(from["configMapKeyRef"]); cm != nil {
				val = fmt.Sprintf("configMapKeyRef %s/%s", scalar(cm["name"]), scalar(cm["key"]))
			} else {
				val = "valueFrom"
			}
		}
		fields = append(fields, Field{Path: prefix + "[" + name + "]", Value: val, Class: ClassEnv})
	}
	return fields
}

func securityFields(prefix string, sc map[string]any) []Field {
	if sc == nil {
		return nil
	}
	var fields []Field
	for _, k := range []string{"privileged", "runAsUser", "runAsGroup", "runAsNonRoot", "allowPrivilegeEscalation", "readOnlyRootFilesystem"} {
		if sc[k] != nil {
			fields = append(fields, Field{Path: prefix + "." + k, Value: scalar(sc[k]), Class: ClassSecurity})
		}
	}
	if caps := asMap(sc["capabilities"]); caps != nil {
		if v := capList(caps["add"]); v != "" {
			fields = append(fields, Field{Path: prefix + ".capabilities.add", Value: v, Class: ClassSecurity})
		}
		if v := capList(caps["drop"]); v != "" {
			fields = append(fields, Field{Path: prefix + ".capabilities.drop", Value: v, Class: ClassSecurity})
		}
	}
	return fields
}

func capList(raw any) string {
	var items []string
	for _, v := range asSlice(raw) {
		items = append(items, scalar(v))
	}
	sort.Strings(items)
	return strings.Join(items, ",")
}

func mountFields(prefix string, raw any) []Field {
	var fields []Field
	for _, m := range asMaps(raw) {
		name := scalar(m["name"])
		if name == "" {
			continue
		}
		val := "mountPath=" + scalar(m["mountPath"])
		if m["readOnly"] != nil {
			val += " readOnly=" + scalar(m["readOnly"])
		}
		fields = append(fields, Field{Path: prefix + "[" + name + "]", Value: val, Class: ClassVolume})
	}
	return fields
}

func volumeFields(prefix string, raw any) []Field {
	var fields []Field
	for _, v := range asMaps(raw) {
		name := scalar(v["name"])
		if name == "" {
			continue
		}
		val := volumeValue(v)
		fields = append(fields, Field{Path: prefix + "[" + name + "]", Value: val, Class: ClassVolume})
	}
	return fields
}

func volumeValue(v map[string]any) string {
	switch {
	case v["hostPath"] != nil:
		return "hostPath path=" + scalar(asMap(v["hostPath"])["path"])
	case v["persistentVolumeClaim"] != nil:
		return "persistentVolumeClaim claimName=" + scalar(asMap(v["persistentVolumeClaim"])["claimName"])
	case v["secret"] != nil:
		return "secret " + scalar(asMap(v["secret"])["secretName"])
	case v["configMap"] != nil:
		return "configMap " + scalar(asMap(v["configMap"])["name"])
	case v["emptyDir"] != nil:
		return "emptyDir"
	default:
		keys := make([]string, 0, len(v))
		for k := range v {
			if k != "name" {
				keys = append(keys, k)
			}
		}
		sort.Strings(keys)
		if len(keys) == 0 {
			return "volume"
		}
		return keys[0]
	}
}

func extractHPA(obj Object) []Field {
	if obj.Kind != "HorizontalPodAutoscaler" {
		return nil
	}
	var fields []Field
	if v := nested(obj.Raw, "spec", "minReplicas"); v != nil {
		fields = append(fields, Field{Path: "spec.minReplicas", Value: scalar(v), Class: ClassHPA})
	}
	if v := nested(obj.Raw, "spec", "maxReplicas"); v != nil {
		fields = append(fields, Field{Path: "spec.maxReplicas", Value: scalar(v), Class: ClassHPA})
	}
	if v := nested(obj.Raw, "spec", "targetCPUUtilizationPercentage"); v != nil {
		fields = append(fields, Field{Path: "spec.targetCPUUtilizationPercentage", Value: scalar(v), Class: ClassHPA})
	}
	for _, m := range asMaps(nested(obj.Raw, "spec", "metrics")) {
		name := scalar(m["type"])
		if res := asMap(m["resource"]); res != nil {
			name = name + "/" + scalar(res["name"])
			if tgt := asMap(res["target"]); tgt != nil {
				val := scalar(tgt["type"])
				if tgt["averageUtilization"] != nil {
					val += "/" + scalar(tgt["averageUtilization"])
				}
				if tgt["averageValue"] != nil {
					val += "/" + scalar(tgt["averageValue"])
				}
				fields = append(fields, Field{Path: "spec.metrics[" + name + "].target", Value: val, Class: ClassHPA})
			}
		}
	}
	return fields
}

func extractService(obj Object) []Field {
	if obj.Kind != "Service" {
		return nil
	}
	var fields []Field
	if v := nested(obj.Raw, "spec", "type"); v != nil {
		fields = append(fields, Field{Path: "spec.type", Value: scalar(v), Class: ClassService})
	} else {
		fields = append(fields, Field{Path: "spec.type", Value: "ClusterIP", Class: ClassService})
	}
	for _, p := range asMaps(nested(obj.Raw, "spec", "ports")) {
		name := scalar(p["name"])
		if name == "" {
			name = scalar(p["port"])
		}
		val := fmt.Sprintf("port=%s targetPort=%s protocol=%s", scalar(p["port"]), scalar(p["targetPort"]), scalarOr(p["protocol"], "TCP"))
		fields = append(fields, Field{Path: "spec.ports[" + name + "]", Value: val, Class: ClassService})
	}
	return fields
}

func extractNetworkPolicy(obj Object) []Field {
	if obj.Kind != "NetworkPolicy" {
		return nil
	}
	spec := asMap(nested(obj.Raw, "spec"))
	if spec == nil {
		return nil
	}
	val := networkPolicyValue(spec)
	return []Field{{Path: "spec", Value: val, Class: ClassNetworkPolicy}}
}

func networkPolicyValue(spec map[string]any) string {
	parts := []string{}
	if spec["podSelector"] != nil {
		parts = append(parts, "podSelector="+compact(spec["podSelector"]))
	}
	if spec["policyTypes"] != nil {
		parts = append(parts, "policyTypes="+compact(spec["policyTypes"]))
	}
	parts = append(parts, ruleSummary("ingress", spec["ingress"])...)
	parts = append(parts, ruleSummary("egress", spec["egress"])...)
	return strings.Join(parts, " ")
}

func ruleSummary(kind string, raw any) []string {
	rules := asMaps(raw)
	if raw == nil {
		return nil
	}
	if len(rules) == 0 {
		return []string{kind + "=deny-all"}
	}
	var out []string
	for i, rule := range rules {
		peer := "from"
		if kind == "egress" {
			peer = "to"
		}
		mode := "restricted"
		if _, ok := rule[peer]; !ok {
			mode = "allow-all"
		}
		out = append(out, fmt.Sprintf("%s[%d]=%s", kind, i, mode))
	}
	return out
}

func extractRBAC(obj Object) []Field {
	var fields []Field
	switch obj.Kind {
	case "Role", "ClusterRole":
		for i, rule := range asMaps(nested(obj.Raw, "rules")) {
			val := fmt.Sprintf("apiGroups=%s resources=%s verbs=%s", compact(rule["apiGroups"]), compact(rule["resources"]), compact(rule["verbs"]))
			fields = append(fields, Field{Path: fmt.Sprintf("rules[%d]", i), Value: val, Class: ClassRBAC})
		}
	case "RoleBinding", "ClusterRoleBinding":
		if ref := asMap(nested(obj.Raw, "roleRef")); ref != nil {
			fields = append(fields, Field{
				Path:  "roleRef",
				Value: scalar(ref["kind"]) + "/" + scalar(ref["name"]),
				Class: ClassRBAC,
			})
		}
		var subs []string
		for _, s := range asMaps(nested(obj.Raw, "subjects")) {
			subs = append(subs, fmt.Sprintf("%s %s/%s", scalar(s["kind"]), scalar(s["namespace"]), scalar(s["name"])))
		}
		sort.Strings(subs)
		if len(subs) > 0 {
			fields = append(fields, Field{Path: "subjects", Value: strings.Join(subs, ","), Class: ClassRBAC})
		}
	}
	return fields
}
