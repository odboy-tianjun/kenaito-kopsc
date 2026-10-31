# kopsc

跨平台命令助手。用一条平台无关的意图命令，在受支持的平台上执行对应的底层命令：

```bash
$ kopsc docker clearsafe --dry-run
[dry-run] 未执行任何命令
将执行: docker system prune -f
平台:   linux/amd64, distro=centos, runtimes=docker
方式:   直接执行
命中:   变体 #1（5 分）CentOS 上的 Docker 运行时
依据:   匹配 os=linux, distro=centos (+3), runtime=docker (+2)
```

在 CentOS 上它执行 `docker system prune -f`；在装有 Podman 的 CentOS 上自动改走
`podman system prune -f`；在 macOS / Windows 上同样有各自的变体。你只记住一个意图。

## 构建

本机构建：

```bash
go build -o bin/kopsc ./cmd/kopsc
```

### 跨平台编译脚本

`scripts/build` 是纯 Go 编写的构建脚本，用 `go run` 运行，不依赖 make / bash，在 Windows、macOS、Linux 上行为一致：

```bash
go run ./scripts/build                       # 默认编译全部 6 个目标到 dist/
go run ./scripts/build -host                 # 只编译当前 OS/ARCH
go run ./scripts/build -targets linux/amd64,windows/arm64
go run ./scripts/build -version 1.2.3        # 指定版本（默认 git describe --tags --always --dirty）
go run ./scripts/build -out bin -no-strip    # 自定义输出目录、保留符号表
```

默认目标矩阵：`linux/{amd64,arm64}`、`darwin/{amd64,arm64}`、`windows/{amd64,arm64}`。
脚本统一设置 `CGO_ENABLED=0`（纯 Go 静态产物）与 `-trimpath`，产物命名为
`kopsc-<version>-<os>-<arch>[.exe]`，任一目标失败即以非零码退出。

等价的单条命令写法：

```bash
GOOS=linux GOARCH=amd64 CGO_ENABLED=0 go build -trimpath \
  -ldflags "-s -w -X main.version=1.0.0" -o dist/kopsc-linux-amd64 ./cmd/kopsc
```

## 用法

```bash
kopsc docker ps                   # 列出运行中的容器
kopsc docker psall                # 含已停止的容器
kopsc docker logs -f web          # 查看日志（容器名必填，选项可透传）
kopsc docker exec -it web sh      # 进入容器
kopsc docker compose up           # 启动 Compose 服务
kopsc docker clearall --yes       # 深度清理（破坏性命令，必须显式 --yes）

kopsc docker clearsafe            # 直接执行当前平台对应的清理命令
kopsc docker clearsafe --dry-run  # 只打印将执行的命令与判定依据
kopsc docker clearsafe -h         # 该意图的说明、当前平台命中结果与选项
kopsc docker clearsafe -- --filter until=24h   # -- 之后的参数原样透传给底层命令

kopsc diagnose web                 # 自动识别 Docker 容器或 K8s Pod，给出根因
kopsc diagnose api-0 -n prod       # K8s：本命令里 -n 表示 namespace
kopsc diagnose api-0 --kind k8s --context staging
kopsc diagnose web --lines 200 --no-events

kopsc disk usage /var                # 磁盘占用排行（du -x --max-depth=1 -h | sort -hr 的跨平台版）
kopsc disk usage --path /var         # 同上，用命名参数指定路径
kopsc disk usage /var --dry-run      # 预览（内置命令同样支持）

kopsc list                        # 全部命令及当前平台可用性
kopsc explain docker.clearsafe    # 匹配过程、全部候选及各自被选中/拒绝的原因
kopsc platform                    # 探测到的 OS/ARCH/发行版/运行时
kopsc version                     # 版本与平台
```

### 磁盘占用排行（disk.usage）

`kopsc disk usage <路径>` 等价于 `du -x --max-depth=1 -h <路径> | sort -hr`，但由 kopsc
内置实现，因此 Linux、macOS、Windows 输出完全一致，不依赖 `du`、`sort` 或 PowerShell：

```text
$ kopsc disk usage /var
1.2G     /var/lib
800.0M   /var/cache
4.0K     /var/tmp
0        /var/opt
总计      2.0G     /var
```

- 路径**必填**：位置参数（`/var`）或 `--path /var`、`--path=/var` 均可，一次一个。
- 统计每个直接子项：普通文件按自身大小，目录递归累计其下普通文件的大小。
- 不跟随符号链接；Unix 上通过设备号实现 `-x`（不跨文件系统），Windows 上不跟随
  reparse point，目录遍历本身不会跨越卷。
- 输出按大小降序，大小采用与 `du -h` 相同的二进制单位（1.0K、1.5M、2.0G）。
- 末尾输出该路径的合计行。

### 选项解析规则

叶子命令禁用了 cobra 的 flag 解析，自行处理：

| 参数 | 行为 |
| --- | --- |
| `-n`, `--dry-run`, `--dry-run=true` | 置为预览模式，不执行 |
| `--dry-run=false` | 关闭预览模式 |
| `-y`, `--yes` | 确认执行破坏性命令；未提供时只打印影响范围并以退出码 2 结束 |
| `-h`, `--help` | 打印该命令的帮助 |
| `--` | 其后的所有参数原样透传给底层命令（并全部计入位置参数） |
| 其它任何参数 | 原样透传给底层命令（包括未知 flag） |

因此底层命令的 flag 不会与 kopsc 自身的选项冲突。

需要容器名/镜像名的命令会做必填校验（例如 `kopsc docker logs` 缺容器名时直接报错，
并给出 `kopsc docker logs <容器>` 的用法），避免把参数错误留给底层命令才发现。

## 内置命令

### Docker（`kopsc docker <子命令>`）

| 分组 | 命令 |
| --- | --- |
| 只读 | `ps`、`psall`、`images`、`stats`、`df`、`info`、`version`、`logs`、`inspect`、`networks`、`volumes` |
| 生命周期 | `start`、`stop`、`restart`、`rm`、`rmi`、`exec`、`pull`、`tag`、`save`、`load`、`cp` |
| 清理 | `clearsafe`、`pruneimages`、`prunecontainers`、`prunenets`、`prunebuilder`、`prunevolumes`、`clearall` |
| Compose | `compose up`、`compose down`、`compose ps`、`compose logs` |

- 所有 Docker 命令在 Linux 上同时提供 `docker` 与 `podman` 变体，且 podman 优先；
  macOS / Windows 使用 Docker Desktop。
- `logs`、`inspect`、`start`、`stop`、`restart`、`rm`、`rmi`、`exec`、`pull`、`tag`、`save`、`cp`
  需要位置参数，缺失时直接报错并给出用法。
- 破坏性命令只有 `clearall`（`system prune -a -f`）与 `prunevolumes`（`volume prune -f`）：
  未带 `--yes` 时只打印影响范围与将执行的命令，并以退出码 2 结束。
- `docker compose` 对应 Compose v2；`podman compose` 需要 podman 4+。
- `docker clearsafe` 的安全契约：只加 `-f` 以便无人值守执行，**不**加 `-a`（不清理全部未使用镜像），
  **不**加 `--volumes`（不删除卷），仅清理停止的容器、未使用的网络与悬空镜像。

### 容器/Pod 诊断（diagnose）

`kopsc diagnose <名称>` 自动判断目标是 Docker/Podman 容器还是 Kubernetes Pod，输出状态、根因、依据、建议，并附最近日志与事件。只读，不修改任何对象。

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

判定覆盖：Docker 的 OOMKilled、ExitCode 137/143/126/127、正常退出、重启循环、unhealthy、created/paused/dead；
Kubernetes 的 CrashLoopBackOff、ImagePullBackOff/ErrImagePull、CreateContainerConfigError、OOMKilled、
Pending（结合 FailedScheduling/FailedMount 事件）、Evicted 等。

### 其它

| 命令 | 说明 | 平台变体 |
| --- | --- | --- |
| `diagnose` | 容器/Pod 状态根因诊断（自动识别 Docker 与 Kubernetes） | Linux/macOS/Windows 均使用 kopsc 内置实现 |
| `disk.usage` | 目录占用排行（等价 `du -x --max-depth=1 -h <路径> \| sort -hr`） | Linux/macOS/Windows 均使用 kopsc 内置实现，保证输出一致 |
| `net.ports` | 列出监听中的端口 | Linux `ss -tulnp`、macOS `lsof -iTCP -sTCP:LISTEN -nP`、Windows `netstat -ano` |
| `pkg.refreshindex` | 刷新包管理器索引 | centos `dnf makecache`、ID_LIKE rhel/fedora `dnf makecache`、ID_LIKE debian/ubuntu `apt-get update`、alpine `apk update`、macOS `brew update`、Windows `winget source update` |

## 平台识别

`kopsc platform` 展示探测结果，来源：

- OS/ARCH：编译目标 `runtime.GOOS` / `runtime.GOARCH`
- Linux 发行版：`/etc/os-release` 的 `ID` 与 `ID_LIKE`（文件缺失时视为未知，不报错）
- 容器运行时：`exec.LookPath` 探测 `docker`、`podman`

## 匹配规则

每个命令由若干**变体**组成，变体声明它适用的环境约束（`OS`、`Arch`、`Distro`、
`DistroLike`、`Runtime`，留空表示不限制）。

1. 变体声明的每一个约束都必须满足，否则该变体被排除。
2. 满足的变体按特异性打分：`Distro` 精确命中 +3，`ID_LIKE` 命中 +2，`Runtime` 命中 +2，无约束的通用变体 0 分。
3. 取最高分。并列时先按运行时偏好（podman 优先于 docker），再按声明约束的数量（更具体者优先）。
4. 仍并列则报歧义错误；无任何变体匹配则报无匹配错误，并提示 `kopsc explain <命令>`。
5. 取 `--dry-run` 或 `explain` 可看到每个候选的得分与被拒原因。

例如 CentOS 上同时装了 docker 与 podman：

```text
✓ #0 5 分  匹配 os=linux, distro=centos (+3), runtime=podman (+2) -> podman system prune -f
✓ #1 5 分  匹配 os=linux, distro=centos (+3), runtime=docker (+2)  -> docker system prune -f
✓ #2 2 分  匹配 os=linux, runtime=podman (+2)                      -> podman system prune -f
✗ #4 0 分  os=darwin 不匹配（当前 linux）                           -> docker system prune -f
```

## 新增命令或平台变体

内置映射位于 [`internal/registry/builtin.go`](internal/registry/builtin.go)（纯 Go 注册表，无外部配置文件）。
新增一个变体即向对应 `Command.Variants` 追加一项：

```go
{
    Match: Match{OS: "linux", Distro: "rocky", Runtime: "docker"},
    Argv:  []string{"docker", "system", "prune", "-f"},
    Note:  "Rocky Linux 上的 Docker",
},
```

- `Argv[0]` 是程序，其余是参数；需要 shell 语义时设 `Shell: true`（Linux/macOS 用 `sh -c`，Windows 用 `cmd /c`）。
- 当平台没有可移植的系统命令时，用 `Builtin` 代替 `Argv`（二者互斥）：
  `Builtin: "disk.usage"` 指向 [`internal/executor`](internal/executor) 中注册的实现，
  这样三平台共用同一份逻辑。新增内置动作需在 `executor.builtins` 中注册。
- 新增命令时给 `Command` 填 `ID`（点号分隔，决定 CLI 层级）、`Summary`、`Action`，别名不得重复。
- 提交前跑 `go test ./...`：`registry.Validate()` 与快照测试会拦截重复 ID/别名、空变体、空 argv。

## 开发

```bash
gofmt -l .        # 应无输出
go vet ./...
go test ./...
```

包结构：

```text
cmd/kopsc            入口，注入版本号
internal/platform    环境探测（可注入，便于测试）
internal/registry    命令模型、校验、内置注册表
internal/resolve     变体打分与选择
internal/executor    进程执行、dry-run、退出码透传
internal/cli         cobra 命令树与辅助命令
scripts/build        跨平台构建脚本（go run ./scripts/build）
```
