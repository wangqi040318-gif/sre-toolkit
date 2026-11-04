# sre-toolkit

`sre` is a small command-line toolkit for SRE and cloud-native operations. The first command, `k8s-pulse`, reads nodes, pods, and warning events and prints each signal as **phenomenon → likely cause → next step**.

`sre` 是一个面向 SRE 与云原生运维的命令行工具。当前实现 `k8s-pulse`：读取节点、Pod 和 Warning 事件，并按 **现象 → 可能原因 → 下一步** 输出。

Other commands (`diff-review`, `triage`, `slo`, `certs`, `waste`, `runbook`, `daily`) are reserved placeholders.

其余子命令（`diff-review`、`triage`、`slo`、`certs`、`waste`、`runbook`、`daily`）仅为占位，执行时会打印 TODO。

## English

### Build

Requires Go 1.22 or newer.

```bash
make build
./bin/sre k8s-pulse --demo
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
| `diff-review` | Planned: compare manifests or live objects and call out operational risk |
| `triage` | Planned: guided triage from a pod, node, or error |
| `slo` | Planned: error-budget and burn-rate snapshot |
| `certs` | Planned: certificate expiry watch |
| `waste` | Planned: idle workloads, oversized requests, unused PVCs |
| `runbook` | Planned: fill a local runbook with cluster facts |
| `daily` | Planned: one-page operator digest |
| `version` | Print the build version |

`k8s-pulse` flags: `-n/--namespace`, `-A/--all-namespaces`, `--demo`, `--kubeconfig`, `--context`, `--restart-threshold` (default 5), `--since` (default `1h`), `--event-limit` (default 20), `--timeout` (default `30s`).

An OOMKilled container is reported as OOM even when it is also CrashLoopBackOff, because the memory kill is the cause of the loop. Completed (`Succeeded`) pods are scanned but not flagged for restarts.

## 中文

### 构建

需要 Go 1.22 或更高版本。

```bash
make build
./bin/sre k8s-pulse --demo
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

`k8s-pulse` 已实现。`diff-review`、`triage`、`slo`、`certs`、`waste`、`runbook`、`daily` 会打印 TODO，留给后续版本。`sre version` 打印版本。

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
