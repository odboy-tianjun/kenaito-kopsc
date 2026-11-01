# kopsc

跨平台命令助手。用一条平台无关的意图命令，在受支持的平台上执行对应的底层命令：

```bash
$ kopsc docker clearsafe --dry-run
[dry-run] 未执行任何命令
将执行: docker system prune -f
平台:   linux/amd64, distro=centos, runtimes=docker
方式:   直接执行
命中:   变体 #1（2 分）Linux 上的 Docker 运行时
依据:   匹配 os=linux, runtime=docker (+2)
```

在 Linux 上它执行 `docker system prune -f`，macOS / Windows 上使用 Docker Desktop 的同一命令。
你只记住一个意图，kopsc 负责挑出当前平台可用的那条命令。

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
kopsc docker ps --filter name=web # 过滤：只看名字含 web 的容器
kopsc docker psall                # 含已停止的容器
kopsc docker images               # 列出本地镜像（不知道有什么镜像时先看这个）
kopsc docker inspect web          # 查看容器完整配置（JSON）
kopsc docker pull nginx:1.25      # 拉取镜像
kopsc docker tag web myorg/web:1.0  # 给镜像打一个可读的标签
kopsc docker cp web:/etc/nginx nginx.bak   # 把容器内目录复制到主机
kopsc docker logs -f web          # 查看日志（容器名必填，选项可透传）
kopsc docker exec web sh          # 进入容器（默认就是交互模式，无需 -it）
kopsc docker compose up           # 启动 Compose 服务
kopsc docker clearsafe            # 日常清理：停掉的容器、未用网络、悬空镜像（推荐）
kopsc docker pruneimages          # 只清悬空镜像
kopsc docker prunecontainers      # 只清已停止的容器
kopsc docker clearall --yes       # 深度清理（破坏性命令，必须显式 --yes）
kopsc docker save alpine:3.20     # 自动保存为 alpine_3.20.tar（传镜像 ID 会先解析出名称）
kopsc docker load alpine_3.20.tar # 等价 docker load < alpine_3.20.tar

kopsc docker clearsafe            # 直接执行当前平台对应的清理命令
kopsc docker clearsafe --dry-run  # 只打印将执行的命令与判定依据
kopsc docker clearsafe -h         # 该意图的说明、当前平台命中结果与选项
kopsc docker clearsafe -- --filter until=24h   # -- 之后的参数原样透传给底层命令

kopsc pkg refreshindex             # 刷新包管理器索引（dnf/apt/apk/brew/winget，按平台自动选）

kopsc diagnose web                 # 自动识别 Docker 容器或 K8s Pod，给出根因
kopsc diagnose api-0 -n prod       # K8s：本命令里 -n 表示 namespace
kopsc diagnose api-0 --kind k8s --context staging
kopsc diagnose web --lines 200 --no-events
# 目标名写错时不用自己查：kopsc 会提示「你可能要找的是: web-1, web-2」

kopsc net pids 8080                  # 等价 lsof -ti:8080，输出占用端口的 PID
kopsc net ports                      # 列出监听中的端口（统一表格，--raw 看原始输出）

kopsc disk usage                     # 当前目录的占用排行（路径可省略）
kopsc disk usage /var --depth 2      # 下钻一层（du -x --max-depth=2 -h | sort -hr 的跨平台版）
kopsc disk usage --path /var         # 同上，用命名参数指定路径
kopsc disk usage /var --dry-run      # 预览（内置命令同样支持）
kopsc proc nohup ./server            # 后台运行命令，输出写日志（等价 nohup ./server > server.out 2>&1 &）

kopsc list                        # 全部命令及当前平台可用性
kopsc explain docker.clearsafe    # 匹配过程、全部候选及各自被选中/拒绝的原因
kopsc explain docker clearsafe    # 同上，空格写法也支持
kopsc explain net                 # 只看分组：列出该组每条命令在本平台的匹配结果
kopsc platform                    # 探测到的 OS/ARCH/发行版/运行时
kopsc version                     # 版本与平台
```

### 选项解析规则

叶子命令禁用了 cobra 的 flag 解析，自行处理：

| 参数 | 行为 |
| --- | --- |
| `-n`, `--dry-run`, `--dry-run=true` | 置为预览模式，不执行。**例外**：`kopsc diagnose` 中 `-n` 表示 namespace，预览请用 `--dry-run` |
| `--dry-run=false` | 关闭预览模式 |
| `-y`, `--yes` | 确认执行破坏性命令；未提供时只打印影响范围并以退出码 2 结束 |
| `-h`, `--help` | 打印该命令的帮助 |
| `--` | 其后的所有参数原样透传给底层命令（并全部计入位置参数） |
| 其它任何参数 | 原样透传给底层命令（包括未知 flag） |

例外（这些命令会对参数做额外解析，写错会报错而不是透传）：

| 命令 | 例外 |
| --- | --- |
| `docker exec` | 容器名之后的参数一律作为容器内命令；显式写 `-i`/`-t`/`-it` 会报错（已默认交互） |
| `docker rm` / `docker rmi` | 不接受 `-f`/`--force`，强删请用 `--yes` |
| `docker logs` | `--since`/`--grep`/`--save` 是 kopsc 选项，须写在容器名之前 |
| `docker save` / `docker rmi` | 目标除了名称，还接受 ID、`sha256:ID` 或唯一 ID 前缀 |
| `proc nohup` | kopsc 选项（`-o`/`-C`/`-E`/`-q`/`-w`）须写在要运行的命令之前 |
| `disk usage` | `--path`/`--depth` 为 kopsc 选项，不是 `du` 的参数 |

因此底层命令的 flag 不会与 kopsc 自身的选项冲突。

需要容器名/镜像名的命令会做必填校验（例如 `kopsc docker logs` 缺容器名时直接报错，
并给出 `kopsc docker logs <容器>` 的用法），避免把参数错误留给底层命令才发现。

## 命令一览

kopsc 的命令按用途分组，每组的完整说明（命令表、选项、示例）在单独的文档里：

| 分组 | 用途 | 文档 |
| --- | --- | --- |
| **docker** | 容器与镜像：查看、启停、日志、清理、导出/导入、Compose | [README.docker.md](README.docker.md) |
| **net** | 网络与端口：端口被谁占用、本机监听了哪些端口 | [README.net.md](README.net.md) |
| **disk** | 磁盘与文件占用：哪个目录、哪个文件最占空间 | [README.disk.md](README.disk.md) |
| **diagnose** | 容器 / Pod 状态根因诊断（自动识别 Docker 与 K8s） | [README.diagnose.md](README.diagnose.md) |
| **proc** | 本机进程：后台运行命令并写日志 | [README.proc.md](README.proc.md) |
| **pkg** | 系统软件包：刷新包管理器索引 | [README.pkg.md](README.pkg.md) |

辅助命令（不涉及具体资源，直接用即可）：

| 命令 | 用法 |
| --- | --- |
| `kopsc list` | 列出全部命令及在当前平台实际会执行什么 |
| `kopsc explain <命令>` | 解释为什么选中某条底层命令，列出全部候选与被排除的原因；支持 `docker.clearsafe` 与 `docker clearsafe` 两种写法，也支持分组名（`kopsc explain docker`） |
| `kopsc platform` | 显示探测到的 OS/ARCH/发行版/运行时 |
| `kopsc version` | kopsc 自身版本与平台 |
| `kopsc <命令> -h` | 任意命令的说明、当前平台命中结果与选项 |

### 各模块入口速查

| 想要 | 命令 |
| --- | --- |
| 看容器/镜像 | `kopsc docker psall`、`kopsc docker images`、`kopsc docker logs -f web` |
| 进容器 / 跑命令 | `kopsc docker exec web sh` |
| 清理磁盘 | `kopsc docker df` → `kopsc docker clearsafe` |
| 搬镜像 | `kopsc docker save alpine:3.20` / `kopsc docker load alpine_3.20.tar` |
| 查端口 | `kopsc net pids 8080`、`kopsc net ports` |
| 查磁盘占用 | `kopsc disk usage`、`kopsc disk usage /var --depth 2` |
| 排障 | `kopsc diagnose web` |
| 后台跑服务 | `kopsc proc nohup ./server` |
| 刷新软件源 | `kopsc pkg refreshindex` |

## 平台识别

`kopsc platform` 展示探测结果，来源：

- OS/ARCH：编译目标 `runtime.GOOS` / `runtime.GOARCH`
- Linux 发行版：`/etc/os-release` 的 `ID` 与 `ID_LIKE`（文件缺失时视为未知，不报错）
- 容器运行时：`exec.LookPath` 探测 `docker`

## 匹配规则

每个命令由若干**变体**组成，变体声明它适用的环境约束（`OS`、`Arch`、`Distro`、
`DistroLike`、`Runtime`，留空表示不限制）。

1. 变体声明的每一个约束都必须满足，否则该变体被排除。
2. 满足的变体按特异性打分：`Distro` 精确命中 +3，`ID_LIKE` 命中 +2，`Runtime` 命中 +2，无约束的通用变体 0 分。
3. 取最高分。并列时先按运行时偏好（有运行时约束者优先，且 `docker` 在前），再按声明约束的数量（更具体者优先）。
4. 仍并列则报歧义错误；无任何变体匹配则报无匹配错误，并提示 `kopsc explain <命令>`。
5. 取 `--dry-run` 或 `explain` 可看到每个候选的得分与被拒原因。

例如 CentOS 上执行 `kopsc explain docker.clearsafe`：

```text
✓ #0 2 分  匹配 os=linux, runtime=docker (+2) -> docker system prune -f
✗ #1 0 分  os=darwin 不匹配（当前 linux）      -> docker system prune -f
✗ #2 0 分  os=windows 不匹配（当前 linux）     -> docker system prune -f
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

### 文档要写在哪

文档按命令分组拆分，新增命令时按分组补充：

| 分组 | 文档 |
| --- | --- |
| docker | [`README.docker.md`](README.docker.md) |
| net | [`README.net.md`](README.net.md) |
| disk | [`README.disk.md`](README.disk.md) |
| diagnose | [`README.diagnose.md`](README.diagnose.md) |
| proc | [`README.proc.md`](README.proc.md) |
| pkg | [`README.pkg.md`](README.pkg.md) |

新分组需要新建 `README.<分组>.md`，并在本文件「命令一览」的表格里加一行链接。
每条命令至少要写清：**作用**、**实际执行什么**、**示例**；有独有选项时补选项表。

## 许可证

本项目基于 [Apache License 2.0](LICENSE) 发布。

```
Copyright 2026 Odboy

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

    http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
```

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
internal/executor    进程执行、内置动作、dry-run、退出码透传
internal/diagnose    Docker 与 Kubernetes 的状态根因诊断（内置动作）
internal/cli         cobra 命令树与辅助命令
scripts/build        跨平台构建脚本（go run ./scripts/build）
```
