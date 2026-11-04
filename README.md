# sre-toolkit

`sre` is a small command-line toolkit for SRE and cloud-native operations. `k8s-pulse` reads nodes, pods, and warning events. `certs` watches certificate expiry. `waste` looks for idle or oversized objects. `diff-review` compares two manifest sets. `runbook` prints a rollback checklist. `triage` builds an incident brief from Prometheus. `slo` reports **success ratio → error budget remaining → burn rates → release decision**.

`sre` 是一个面向 SRE 与云原生运维的命令行工具。`k8s-pulse` 读取节点、Pod 和 Warning 事件。`certs` 检查证书到期。`waste` 查找闲置或过大的对象。`diff-review` 对比两份清单。`runbook` 生成回滚手册。`triage` 用 Prometheus 生成排查简报。`slo` 报告 **成功率 → 错误预算剩余 → 燃烧率 → 是否冻结发布**。

The `daily` command is still a placeholder.

`daily` 仍为占位，执行时会打印 TODO。

## English

### Build

Requires Go 1.22 or newer.

```bash
make build
./bin/sre k8s-pulse --demo
./bin/sre certs --demo
./bin/sre waste --demo
./bin/sre diff-review --demo
./bin/sre runbook --demo
./bin/sre triage --demo
./bin/sre slo --demo
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

### Rollback runbook

```bash
./bin/sre runbook --demo
./bin/sre runbook --demo --markdown
./bin/sre runbook -n shop checkout
./bin/sre runbook -n shop checkout --revision 11 --report rollback.md
./bin/sre runbook -n data db --kind statefulset
```

`--demo` does not contact a cluster. It is a failed `shop/checkout` release: revision 12 is `ghcr.io/example/checkout:1.9.0` with 0/3 pods Ready and CrashLoopBackOff because `STRIPE_API_KEY` is missing. The checklist rolls back to revision 11 (`checkout:1.8.4`) with `kubectl rollout undo`, an image fallback, rollout status, pod and event checks, and abort criteria. Commands are printed, not executed.

```text
CRITICAL  State  Deployment shop/checkout
  Phenomenon    Deployment shop/checkout is at revision 12, image checkout=ghcr.io/example/checkout:1.9.0 (desired 3, ready 0, updated 3). 3 pods are CrashLoopBackOff...
  Likely cause  The current revision is not Ready. Revision 11 (checkout=ghcr.io/example/checkout:1.8.4) is the recorded rollback target.
  Next step     kubectl get deployment checkout -n shop -o wide

WARNING   Rollback  Deployment shop/checkout
  Phenomenon    Roll Deployment shop/checkout back to revision 11.
  Next step     kubectl rollout undo deployment/checkout -n shop --to-revision=11
```

Live mode reads the Deployment (or StatefulSet), its ReplicaSets or ControllerRevisions, and matching pods. `--revision` picks the undo target. The default is the newest revision below the current one.

### Triage demo (no Prometheus)

```bash
./bin/sre triage --demo
./bin/sre triage --demo checkout --json
./bin/sre triage checkout --prometheus-url http://prometheus:9090 --window 1h
./bin/sre triage checkout --since 30m --selector 'service="{service}"'
```

`--demo` does not call Prometheus. It is a one-hour `checkout` spike: the 5xx ratio climbs from about 0.2% to 14%, p95 from about 80ms to 1.4s, and request rate eases from 120 to about 90 req/s in the last 15 minutes. The brief states the hypothesis (bad release or a slow dependency) and prints kubectl plus the PromQL that would be sent live.

```text
CRITICAL  ErrorRate  service checkout
  Phenomenon    5xx ratio is 12.0% now (start 0.2%, peak 14.0%).
  Likely cause  The 5xx ratio crossed 5%. A bad release or a dependency returning errors is the first hypothesis.
  Next step     sum(rate(http_requests_total{job="checkout",code=~"5.."}[5m])) / ...
```

Default metrics, overridable with `--requests`, `--errors`, `--p95`, `--p99`, and `--selector`:

- request rate: `sum(rate(http_requests_total{job="{service}"}[5m]))`
- 5xx ratio: the same counter with `code=~"5.."` divided by all requests
- p95 and p99: `histogram_quantile` on `http_request_duration_seconds_bucket`

The URL is `--prometheus-url` or `PROMETHEUS_URL`. An optional bearer token is `--bearer-token` or `PROMETHEUS_TOKEN` and is not printed. `--window` or `--since` is the lookback (default `1h`). `--until` is an RFC3339 end time. Logs are not queried.

### SLO demo (no Prometheus)

```bash
./bin/sre slo --demo
./bin/sre slo --demo --json
./bin/sre slo checkout --prometheus-url http://prometheus:9090
./bin/sre slo checkout --slo 99.95 --window 5m,1h,6h
```

`--demo` does not call Prometheus. For a 99.9% availability target the checkout service is burning budget: 1h at 20x (page), 6h at 8x, and the 3-day window has already spent the budget (about -50% remaining). The recommendation is to page and freeze releases.

```text
CRITICAL  Recommendation  service checkout
  Phenomenon    Page and freeze. The 1h burn rate is 20.0x, at or above the 14.4x page threshold.
  Next step     sre triage checkout --window 1h; kubectl rollout history deployment/checkout
```

`--window` takes at least two durations, default `1h,6h,3d` (`1h/6h/3d` works too). Burn rate is the window's 5xx ratio divided by `(1 - SLO)`. The fast window pages at 14.4x. A 6h burn at 6x, or a multi-day burn at 1x, freezes non-urgent releases. The default query is `increase()` of `http_requests_total` with `code=~"5.."` over all requests, selected by `job="{service}"`. Override it with `--errors` or `--selector`. The same Prometheus URL and bearer token as `sre triage` are used.

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
| `runbook` | Rollback checklist for a Deployment or StatefulSet |
| `triage` | Prometheus RED brief: error rate, latency, traffic, next checks |
| `slo` | Error budget, multi-window burn rate, and release freeze decision |
| `daily` | Planned: one-page operator digest |
| `version` | Print the build version |

`k8s-pulse` flags: `-n/--namespace`, `-A/--all-namespaces`, `--demo`, `--kubeconfig`, `--context`, `--restart-threshold` (default 5), `--since` (default `1h`), `--event-limit` (default 20), `--timeout` (default `30s`).

`certs` flags: `-n/--namespace`, `-A/--all-namespaces`, `--demo`, `--kubeconfig`, `--context`, `--warn-days` (default 30), `--critical-days` (default 7), `--file`, `--dir`, `--timeout` (default `30s`).

Live `certs` needs `get` and `list` on `secrets` and `ingresses`. The command reads `tls.crt` only and does not print private keys. Listing secrets is still a privileged call; do not commit kubeconfig files or PEM bundles that contain keys.

`waste` flags: `-n/--namespace`, `-A/--all-namespaces`, `--demo`, `--kubeconfig`, `--context`, `--idle-for` (default `24h`), `--usage-ratio` (default `0.20`), `--include-config` (default true), `--timeout` (default `30s`).

Live `waste` needs `get` and `list` on `deployments`, `statefulsets`, `replicasets`, `pods`, `services`, `endpoints`, and `persistentvolumeclaims`. ConfigMaps, Secrets, and `pods.metrics.k8s.io` are optional. If metrics are missing, the command still reports idle objects, empty load balancers, PVCs, and suspiciously large requests.

`diff-review` reads local files only. It does not talk to a cluster and it does not run Helm.

`runbook` flags: `-n/--namespace`, workload name argument or `--name`, `--kind` (`deployment` or `statefulset`), `--revision`, `--demo`, `--kubeconfig`, `--context`, `--timeout`. Live mode needs `get` on the workload and `list` on `replicasets` or `controllerrevisions`, plus `list` on `pods`. It only prints kubectl commands.

`triage` flags: service argument or `--service`, `--window` or `--since` (default `1h`), `--until`, `--prometheus-url`, `--bearer-token`, `--selector`, `--requests`, `--errors`, `--p95`, `--p99`, `--timeout`, `--demo`. It only reads the Prometheus HTTP API.

`slo` flags: service argument or `--service`, `--slo` (default `99.9`), `--window` (default `1h,6h,3d`), `--prometheus-url`, `--bearer-token`, `--selector`, `--errors`, `--timeout`, `--demo`. It only reads the Prometheus HTTP API.

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
./bin/sre runbook --demo
./bin/sre triage --demo
./bin/sre slo --demo
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

回滚手册：

```bash
./bin/sre runbook --demo
./bin/sre runbook -n shop checkout
./bin/sre runbook -n data db --kind statefulset
```

`--demo` 是一次失败的 checkout 发布：revision 12 的镜像 `checkout:1.9.0` 没有 Ready Pod，因为缺少 `STRIPE_API_KEY`。手册给出回到 revision 11（`checkout:1.8.4`）的 `kubectl rollout undo`、备用 `set image`、验证和中止条件。命令只打印，不会执行。

值班排查：

```bash
./bin/sre triage --demo
./bin/sre triage checkout --prometheus-url http://prometheus:9090 --window 1h
```

`--demo` 不访问 Prometheus。默认指标是 `http_requests_total`（错误用 `code=~"5.."`）和 `http_request_duration_seconds_bucket`，标签是 `job="{service}"`。地址用 `--prometheus-url` 或环境变量 `PROMETHEUS_URL`。本命令不查日志。

错误预算：

```bash
./bin/sre slo --demo
./bin/sre slo checkout --prometheus-url http://prometheus:9090 --slo 99.9 --window 1h,6h,3d
```

`--demo` 不访问 Prometheus。99.9% 目标下，1 小时燃烧率约 20 倍（需要 page），6 小时约 8 倍，3 天窗口的预算已经用完。建议冻结发布并升级。默认查询与 triage 相同，用 `increase()` 计算窗口内的 5xx 比例。

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

`k8s-pulse`、`certs`、`waste`、`diff-review`、`runbook`、`triage` 和 `slo` 已实现。`daily` 会打印 TODO，留给后续版本。`sre version` 打印版本。

`slo` 计算可用性错误预算和 1h/6h/3d 燃烧率，并给出是否冻结发布。`--demo` 是预算已经烧穿的离线场景。

`triage` 通过 Prometheus HTTP API 读取请求速率、5xx 比例和 p95/p99，并给出假设和下一步 kubectl / PromQL。`--demo` 是离线尖峰。

`runbook` 根据 Deployment 或 StatefulSet 的当前镜像、副本和 revision 生成回滚清单。真实集群需要读取对应工作负载及其 ReplicaSet 或 ControllerRevision。

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
