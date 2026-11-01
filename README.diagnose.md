# 容器/Pod 诊断（`kopsc diagnose`）

> 返回 [README.md](README.md)

`kopsc diagnose <名称>` 自动判断目标是 Docker 容器还是 Kubernetes Pod，
输出状态、根因、依据、建议，并附最近日志与事件。只读，不修改任何对象。

```bash
kopsc diagnose web                 # 自动识别 Docker 容器或 K8s Pod，给出根因
kopsc diagnose api-0 -n prod       # K8s：本命令里 -n 表示 namespace
kopsc diagnose api-0 --kind k8s --context staging
kopsc diagnose web --lines 200 --no-events
# 目标名写错时不用自己查：kopsc 会提示「你可能要找的是: web-1, web-2」
```

## 输出示例

```text
目标:   web
来源:   Docker (docker)
状态:   exited  ExitCode=137  OOMKilled=true  RestartCount=3
镜像:   nginx:1.25
判定:   ❌ 容器因内存超限被 OOM kill
依据:   State.Status=exited, ExitCode=137, OOMKilled=true, RestartCount=3
建议:   提高容器的内存限制，或降低应用峰值内存；可用 kopsc docker stats 观察

最近日志 (50 行):
...
最近事件 (1h):
...
```

## 选项

| 参数 | 说明 |
| --- | --- |
| `--kind docker\|k8s` | 强制来源；默认自动识别：先 `docker inspect`，未命中再 `kubectl get pod` |
| `-n`, `-N`, `--namespace` | **仅 diagnose**：Kubernetes 命名空间。默认取 kubeconfig 当前上下文，未设置则为 `default` |
| `--context` | kubectl 上下文 |
| `-c`, `--container` | 多容器 Pod 只看指定容器（默认输出全部容器） |
| `--lines N` | 日志行数，默认 50 |
| `--no-logs` / `--no-events` | 跳过日志 / 事件采集 |
| `--dry-run` | 只预览将执行的诊断动作，不调用 docker/kubectl |

> `-n` 只在 `kopsc diagnose` 中表示 namespace；其它命令里 `-n` 仍是 `--dry-run` 的简写。
> Docker 模式下传 `--namespace`/`--context` 会直接报错，不会静默忽略。

## 目标名写错时

找不到目标时，kopsc 会在 Docker 容器名与 K8s Pod 名里找名字相近的（前缀、包含匹配，忽略大小写），
最多列 5 个：

```text
kopsc: 未找到目标 "web"：...
你可能要找的是: web-1, web-2
```

一个都不相似时，改为列出当前存在的对象，便于对照拼写。

## 判定覆盖

- Docker：OOMKilled、ExitCode 137/143/126/127、正常退出、重启循环、unhealthy、created/paused/dead
- Kubernetes：CrashLoopBackOff、ImagePullBackOff/ErrImagePull、CreateContainerConfigError、OOMKilled、
  Pending（结合 FailedScheduling/FailedMount 事件）、Evicted
