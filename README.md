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
kopsc docker clearsafe            # 直接执行当前平台对应的清理命令
kopsc docker clearsafe --dry-run  # 只打印将执行的命令与判定依据
kopsc docker clearsafe -h         # 该意图的说明、当前平台命中结果与选项
kopsc docker clearsafe -- --filter until=24h   # -- 之后的参数原样透传给底层命令

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
| `-h`, `--help` | 打印该命令的帮助 |
| `--` | 其后的所有参数原样透传给底层命令 |
| 其它任何参数 | 原样透传给底层命令（包括未知 flag） |

因此底层命令的 flag 不会与 kopsc 自身的选项冲突。

## 内置命令

| 命令 | 说明 | 平台变体 |
| --- | --- | --- |
| `docker.clearsafe` | 安全清理容器/网络/构建缓存 | Linux(centos)+podman、Linux(centos)+docker、Linux+podman、Linux+docker、darwin+docker、windows+docker，均为 `system prune -f` |
| `disk.usage` | 目录占用排行（等价 `du -x --max-depth=1 -h <路径> \| sort -hr`） | Linux/macOS/Windows 均使用 kopsc 内置实现，保证输出一致 |
| `net.ports` | 列出监听中的端口 | Linux `ss -tulnp`、macOS `lsof -iTCP -sTCP:LISTEN -nP`、Windows `netstat -ano` |
| `pkg.refreshindex` | 刷新包管理器索引 | centos `dnf makecache`、ID_LIKE rhel/fedora `dnf makecache`、ID_LIKE debian/ubuntu `apt-get update`、alpine `apk update`、macOS `brew update`、Windows `winget source update` |

`docker clearsafe` 的安全契约：只加 `-f` 以便无人值守执行，**不**加 `-a`（不清理全部未使用镜像），
**不**加 `--volumes`（不删除卷），仅清理停止的容器、未使用的网络与悬空镜像。

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
