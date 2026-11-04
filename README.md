# sre-toolkit

`sre` is a small command-line toolkit for SRE and cloud-native operations. `k8s-pulse` reads nodes, pods, and warning events. `certs` watches certificate expiry. `waste` looks for idle or oversized objects. `diff-review` compares two manifest sets and reports **object → field → old → new → risk → what to check**.

`sre` 是一个面向 SRE 与云原生运维的命令行工具。`k8s-pulse` 读取节点、Pod 和 Warning 事件。`certs` 检查证书到期。`waste` 查找闲置或过大的对象。`diff-review` 对比两份清单，并按 **对象 → 字段 → 旧值 → 新值 → 风险 → 评审检查项** 输出。

Other commands (`triage`, `slo`, `runbook`, `daily`) are reserved placeholders.

其余子命令（`triage`、`slo`、`runbook`、`daily`）仅为占位，执行时会打印 TODO。

## English

### Build

Requires Go 1.22 or newer.

```bash
make build
./bin/sre k8s-pulse --demo
./bin/sre certs --demo
./bin/sre waste --demo
./bin/sre diff-review --demo
```

`make test` runs the unit tests. `make demo` builds the binary and runs the demo pulse.

### Demo (no cluster)

```bash
./bin/sre k8s-pulse --demo
./bin/sre k8s-pulse --demo --json
./bin/sre k8s-pulse --demo --markdown
./bin/sre k8s-pulse --demo --report pulse.md
./bin/sre k8s-pulse --demo -n payments
```

`--demo` uses a fixed synthetic incident (CrashLoop from a missing env, an OOMKilled Prometheus, a pod with elevated restarts, a NotReady node under disk pressure, and recent warning events). It does not read your kubeconfig.

```text
Summary
  Nodes             4
  Nodes not ready   1
  Pods scanned      8
  CrashLoopBackOff  1
  OOMKilled         1
  High restarts     1
  Warning events    3

Findings (7)

CRITICAL  OOMKilled  observability/prometheus-0
  Phenomenon    Container prometheus ... was OOMKilled (memory limit 512Mi) ...
  Likely cause  The container crossed its memory limit (512Mi) and the kernel killed it.
  Next step     kubectl describe pod -n observability prometheus-0; kubectl logs ... --previous
```

### Certificate demo (no cluster)

```bash
./bin/sre certs --demo
./bin/sre certs --demo --json
./bin/sre certs --demo --report certs.md
./bin/sre certs --demo -n payments
./bin/sre certs --file ./tls.crt --dir ./pems
```

`--demo` uses synthetic certificates: one expired checkout cert, one API cert inside the critical window (≤7 days), one Grafana cert inside the warn window (≤30 days), one healthy edge cert, and an Ingress whose TLS secret is missing. It does not read kubeconfig. `--file` and `--dir` scan local PEMs and do not contact a cluster when used without `-n`, `-A`, or `--kubeconfig`.

```text
Summary
  Certificates      4
  Expired           1
  Critical          1
  Warning           1
  OK                1
  Missing secrets   1

Findings (5)

CRITICAL  Expired  secret payments/checkout-tls
  Phenomenon    Certificate checkout.payments.example expired 12 days ago (notAfter ...). Severity: expired.
  Likely cause  The certificate is past notAfter.
  Next step     kubectl describe certificate -n payments checkout; ...
```

Thresholds are `--critical-days` (default 7) and `--warn-days` (default 30). A certificate is critical when whole days remaining are less than or equal to the critical window, and a warning when they are less than or equal to the warn window.

### Waste demo (no cluster)

```bash
./bin/sre waste --demo
./bin/sre waste --demo --json
./bin/sre waste --demo -n shop
./bin/sre waste -A
```

`--demo` does not read kubeconfig. The synthetic story is a scaled-to-zero Deployment, a Deployment with no Ready pods for days, a StatefulSet whose 4 CPU / 8Gi request is far above usage, a LoadBalancer with an address and no endpoints, a 200Gi Bound PVC nobody mounts, an unbound PVC, and an unused ConfigMap and Secret. A healthy Deployment, a mounted PVC, and a referenced ConfigMap are present and not flagged. ReplicaSets owned by a Deployment are skipped.

```text
Summary
  Workloads       4
  Idle            2
  Oversized       1
  Load balancers  1
  Orphaned PVCs   1
  Unbound PVCs    1

Findings (8)

CRITICAL  Oversized  sts observability/prometheus
  Phenomenon    StatefulSet observability/prometheus reserves 4 CPU and 8Gi memory across 1 replica, but metrics show 150m CPU in use of 4 requested (3.8%).
  Likely cause  The request is what the scheduler reserves.
  Next step     Rightsize requests toward observed usage...
```

Without metrics.k8s.io, a live scan does not fail. It flags requests of at least 2 CPU or 4Gi instead of comparing usage. `--idle-for` defaults to 24h. `--usage-ratio` defaults to 0.20. Bound PVCs of 100Gi or more with no running mounter are critical.

### Diff review demo (no cluster)

```bash
./bin/sre diff-review --demo
./bin/sre diff-review --demo --json
./bin/sre diff-review --before ./before.yaml --after ./after.yaml
./bin/sre diff-review --before ./before-dir --after ./after-dir --report review.md
```

`--before` and `--after` are YAML or JSON files, or directories of them (including multi-document files). Render Helm yourself and pass the output; this command does not run Helm:

```bash
helm template rel chart -f old-values.yaml > /tmp/before.yaml
helm template rel chart -f new-values.yaml > /tmp/after.yaml
./bin/sre diff-review --before /tmp/before.yaml --after /tmp/after.yaml
```

`--demo` reviews a built-in checkout release. The image moves to `:latest`, replicas go to 0, the memory limit and liveness probe disappear, the container becomes privileged with `NET_ADMIN` and a hostPath, the API key secret key changes, the Service becomes a LoadBalancer, the HPA min is higher than max, the NetworkPolicy is deleted, and a cluster-admin binding is added. An unchanged Deployment is omitted.

```text
CRITICAL  Image  Deployment shop/checkout
  Phenomenon    Risk critical. spec.template.spec.containers[checkout].image: ghcr.io/example/checkout:1.4.2 -> ghcr.io/example/checkout:latest
  Likely cause  The image moved from a pinned tag to latest.
  Next step     Confirm the image is in the allowed registry and the tag is pinned.
```

Compared fields are images, requests and limits, probes, replicas, HPA, volumes and PVCs, Service type and ports, NetworkPolicy, RBAC, securityContext, and env/secretKeyRef. Risk is critical, high, medium, or low.

### Live cluster

```bash
./bin/sre k8s-pulse -A
./bin/sre k8s-pulse -n payments --since 30m
./bin/sre k8s-pulse --context prod --restart-threshold 10
```

The client is [client-go](https://github.com/kubernetes/client-go). It loads `KUBECONFIG` or `~/.kube/config`, then falls back to an in-cluster service account. Without `-n` or `-A`, the namespace comes from the current context (otherwise `default`). NotReady nodes are cluster-scoped and are always included.

The identity needs `get` and `list` on `nodes`, `pods`, and `events`:

```yaml
rules:
  - apiGroups: [""]
    resources: ["nodes", "pods", "events"]
    verbs: ["get", "list"]
```

Do not commit kubeconfig files.

### Output

| Flag | Effect |
| --- | --- |
| *(default)* | Colored summary on a terminal. Honors `NO_COLOR` and `--no-color`. |
| `--json` | JSON document on stdout. |
| `--markdown` | Markdown report on stdout. |
| `--report FILE` | Also write Markdown to `FILE`. |

`--json` and `--markdown` cannot be combined. A report file can be combined with either stdout format.

### Commands

| Command | Status |
| --- | --- |
| `k8s-pulse` (`pulse`) | CrashLoopBackOff, OOMKilled, high restarts, NotReady nodes, recent Warning events |
| `certs` | Ingress TLS, `kubernetes.io/tls` secrets, and local PEM expiry |
| `waste` | Idle workloads, oversized requests, empty load balancers, orphaned PVCs |
| `diff-review` | Risky field changes between two manifest sets |
| `triage` | Planned: guided triage from a pod, node, or error |
| `slo` | Planned: error-budget and burn-rate snapshot |
| `runbook` | Planned: fill a local runbook with cluster facts |
| `daily` | Planned: one-page operator digest |
| `version` | Print the build version |

`k8s-pulse` flags: `-n/--namespace`, `-A/--all-namespaces`, `--demo`, `--kubeconfig`, `--context`, `--restart-threshold` (default 5), `--since` (default `1h`), `--event-limit` (default 20), `--timeout` (default `30s`).

`certs` flags: `-n/--namespace`, `-A/--all-namespaces`, `--demo`, `--kubeconfig`, `--context`, `--warn-days` (default 30), `--critical-days` (default 7), `--file`, `--dir`, `--timeout` (default `30s`).

Live `certs` needs `get` and `list` on `secrets` and `ingresses`. The command reads `tls.crt` only and does not print private keys. Listing secrets is still a privileged call; do not commit kubeconfig files or PEM bundles that contain keys.

`waste` flags: `-n/--namespace`, `-A/--all-namespaces`, `--demo`, `--kubeconfig`, `--context`, `--idle-for` (default `24h`), `--usage-ratio` (default `0.20`), `--include-config` (default true), `--timeout` (default `30s`).

Live `waste` needs `get` and `list` on `deployments`, `statefulsets`, `replicasets`, `pods`, `services`, `endpoints`, and `persistentvolumeclaims`. ConfigMaps, Secrets, and `pods.metrics.k8s.io` are optional. If metrics are missing, the command still reports idle objects, empty load balancers, PVCs, and suspiciously large requests.

`diff-review` reads local files only. It does not talk to a cluster and it does not run Helm.

An OOMKilled container is reported as OOM even when it is also CrashLoopBackOff, because the memory kill is the cause of the loop. Completed (`Succeeded`) pods are scanned but not flagged for restarts.

## 中文

### 构建

需要 Go 1.22 或更高版本。

```bash
make build
./bin/sre k8s-pulse --demo
./bin/sre certs --demo
./bin/sre waste --demo
./bin/sre diff-review --demo
```

`make test` 运行单元测试。`make demo` 会先构建再跑演示。

### 演示（不需要集群）

```bash
./bin/sre k8s-pulse --demo
./bin/sre k8s-pulse --demo --json
./bin/sre k8s-pulse --demo --report pulse.md
./bin/sre k8s-pulse --demo -n payments
```

`--demo` 使用一套固定的合成故障：缺少环境变量导致的 CrashLoop、被 OOMKilled 的 Prometheus、重启次数偏高的 Pod、磁盘压力下的 NotReady 节点，以及最近的 Warning 事件。不会读取 kubeconfig，也不会访问 API。

证书演示同样不需要集群：

```bash
./bin/sre certs --demo
./bin/sre certs --demo -n payments
./bin/sre certs --file ./tls.crt
```

内置数据包含已过期、7 天内（critical）、30 天内（warn）、仍然健康的证书，以及一个找不到 TLS Secret 的 Ingress。`--critical-days` 默认 7，`--warn-days` 默认 30。单独使用 `--file` 或 `--dir` 时不会访问集群。

浪费演示同样不需要集群：

```bash
./bin/sre waste --demo
./bin/sre waste --demo -n shop
```

内置数据包含缩容到 0 的 Deployment、长时间没有 Ready Pod 的 Deployment、requests 远高于用量的 StatefulSet、没有端点的 LoadBalancer、已绑定但无人挂载的 200Gi PVC，以及未使用的 ConfigMap 和 Secret。没有 metrics-server 时命令不会失败，改为标记至少 2 CPU 或 4Gi 的 requests。`--idle-for` 默认 24h，`--usage-ratio` 默认 0.20。

清单对比演示：

```bash
./bin/sre diff-review --demo
./bin/sre diff-review --before ./before.yaml --after ./after.yaml
```

`--before` 和 `--after` 可以是 YAML/JSON 文件或目录。Helm 需要先 `helm template` 渲染成文件再传入，本命令不会调用 Helm。`--demo` 内置一次 checkout 发布：镜像变成 `:latest`、副本数降为 0、去掉内存 limit 和 liveness、容器变成 privileged 并挂载 hostPath、Service 改为 LoadBalancer、HPA 的 min 大于 max、删除 NetworkPolicy，并新增 cluster-admin 绑定。

### 真实集群

```bash
./bin/sre k8s-pulse -A
./bin/sre k8s-pulse -n payments --since 30m
```

通过 client-go 访问集群。默认读取 `KUBECONFIG` 或 `~/.kube/config`，否则使用集群内 ServiceAccount。未指定 `-n` / `-A` 时，使用当前 context 的命名空间（没有则是 `default`）。NotReady 节点是集群范围的，即使指定了命名空间也会显示。

账号需要对 `nodes`、`pods`、`events` 具备 `get` 和 `list` 权限。不要把 kubeconfig 提交到仓库。

### 输出

- 默认：终端彩色摘要。设置 `NO_COLOR` 或 `--no-color` 可关闭颜色。
- `--json`：向标准输出打印 JSON。
- `--markdown`：向标准输出打印 Markdown。
- `--report FILE`：额外把 Markdown 写到文件。可以和 `--json` 一起用。

### 命令

`k8s-pulse`、`certs`、`waste` 和 `diff-review` 已实现。`triage`、`slo`、`runbook`、`daily` 会打印 TODO，留给后续版本。`sre version` 打印版本。

`diff-review` 只读本地清单，对比镜像、resources、探针、副本、HPA、卷、Service、NetworkPolicy、RBAC、securityContext 和 env/secretKeyRef。

`waste` 检查 Deployment、StatefulSet、独立 ReplicaSet、LoadBalancer Service、PVC，以及可选的未引用 ConfigMap/Secret。真实集群需要相应资源的 `get`/`list`。用量来自 `metrics.k8s.io`，没有时自动降级。

`certs` 读取 Ingress 的 `spec.tls`、类型为 `kubernetes.io/tls` 的 Secret，以及本地 PEM。只解析证书，不打印私钥。真实集群需要 `secrets` 和 `ingresses` 的 `get`/`list` 权限。

同一容器如果上次退出原因是 OOMKilled，即使当前是 CrashLoopBackOff，也只归因为 OOM。`Succeeded` 的 Pod 会被计入扫描数，但不会因为重启次数被报出来。

## Development

```bash
make fmt
make vet
make test
make build
```

## License

MIT. See [LICENSE](LICENSE).
