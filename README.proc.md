# 本机进程（`kopsc proc <子命令>`）

> 返回 [README.md](README.md)

在后台运行命令并写日志，关掉终端也不会退出；也可以等某个进程退出后再继续；
还能按资源占用排序查看进程、按名称终止进程、查看进程树。

```bash
kopsc proc list                    # 列出所有进程
kopsc proc nohup ./server            # 后台运行命令，输出写日志
kopsc proc wait nginx                # 等 nginx 退出后返回
kopsc proc topmem                    # 内存占用前 10 的进程
kopsc proc topcpu                    # CPU 占用前 10 的进程
kopsc proc killname nginx --yes      # 终止名为 nginx 的进程
kopsc proc tree                      # 查看进程树
```

## 列出所有进程（proc.list）

```bash
kopsc proc list
```

列出系统中所有正在运行的进程。Linux/macOS 使用 `ps aux`，Windows 使用 `tasklist`。

## 后台运行（proc.nohup）

`kopsc proc nohup <命令> [参数...]` 等价于 `nohup <命令> > output.log 2>&1 &`：
启动命令后立刻返回，关闭终端不会终止它。

```bash
kopsc proc nohup ./server -port 8080                      # 日志写到 ./server.out（同 nohup.out）
kopsc proc nohup -o /var/log/server.log ./server          # 指定日志
kopsc proc nohup -C /srv/app --env=APP_ENV=prod ./run.sh  # 指定工作目录与环境变量
kopsc proc nohup -w ./migrate.sh                          # 等它结束，并回显日志、透传退出码
kopsc proc nohup --dry-run ./server                       # 只打印判定依据
```

- 标准输入接到空设备；标准输出与标准错误**都追加**写入日志文件（`>>` 语义），并自动创建日志目录。
- Unix 下子进程进入新会话、无控制终端（相当于 `setsid`），Windows 下以 `DETACHED_PROCESS` 启动。
- 选项只在**命令之前**解析：`kopsc proc nohup ffmpeg -i in.mp4` 中的 `-i` 原样给 ffmpeg，
  不会被 kopsc 截获（因此命令自身的 `-n`/`--dry-run` 不会触发 kopsc 的预览）。
  需要预览时把 `--dry-run` 写在命令之前、或用 `--` 分隔。
- 不经过 shell，`>`、`|`、`&&` 需要显式写 shell：`kopsc proc nohup sh -c 'a | b > c'`。

### 选项

| 选项 | 说明 |
| --- | --- |
| `-o`, `--out`, `--output` | 日志文件，默认 `<程序名>.out` |
| `-C`, `--dir`, `--chdir` | 子进程工作目录（相对日志路径也基于它），默认当前目录 |
| `-E`, `--env NAME=VALUE` | 追加环境变量，可重复 |
| `-q`, `--quiet` | 不打印启动摘要（PID、日志路径） |
| `-w`, `--wait` | 前台等待命令退出，回显其写入的日志内容并透传退出码 |
| `--detach` / `--no-detach` | 默认脱离会话；`--no-detach` 保留当前会话（类似只加 `&`） |

## 等待进程退出（proc.wait）

`kopsc proc wait <关键字>... [--pid PID]...` 每秒检查一次目标进程是否还在：
存在就继续等，直到它消失为止。适合在脚本里等一个服务真正停下来，
替代 `while pgrep nginx; do sleep 1; done` 这类写法。

```bash
kopsc proc wait nginx                       # 等所有名字/命令行含 nginx 的进程退出
kopsc proc wait nginx worker                # 关键字逻辑与：两个都命中才算目标
kopsc proc wait --pid 1234                  # 等指定 PID 退出
kopsc proc wait --pid 1234,5678             # 多个 PID（逗号分隔）
kopsc proc wait nginx --timeout 30          # 最多等 30 秒
kopsc proc wait nginx --once                # 只检查一次，不等待
kopsc proc wait nginx --interval 500ms -q   # 每 0.5 秒查一次，不打印进度
```

### 匹配规则

- 关键字在**进程名与完整命令行**中匹配，忽略大小写：
  `kopsc proc wait nginx` 能命中 `/usr/sbin/nginx -g daemon off;`。
- 给多个关键字时是**逻辑与**：全部命中才算目标进程。
  例如 `kopsc proc wait java myapp.jar` 只等「既是 java 进程、命令行里又含 myapp.jar」的那些。
- `--pid` 与关键字可以同时给：任一方仍有进程存在，就继续等待。
- Windows 的 `tasklist` 不提供命令行，关键字只能匹配进程名（如 `nginx.exe`）。

### 退出码

| 退出码 | 含义 |
| --- | --- |
| `0` | 目标进程已全部退出；或启动时本来就没有匹配的进程 |
| `2` | `--timeout` 到期时进程仍在运行，错误信息会列出残留的 PID |
| `1` | 参数错误，或无法列出进程（例如没有权限） |

### 选项

| 选项 | 说明 |
| --- | --- |
| `<关键字>...` | 要等待的进程关键字，可多个（逻辑与） |
| `--pid`, `-p` | 直接指定进程 ID；可写多次或用逗号分隔，如 `--pid 1234,5678` |
| `--timeout` | 最长等待时间，默认不超时；可写 `30`（秒）或 `30s`、`1m` |
| `--interval` | 检查间隔，默认 1 秒 |
| `--once` | 只检查一次并打印结果，不等待 |
| `-q`, `--quiet` | 不打印进度与结束提示 |

> 关键字请直接作为参数书写；若关键字以 `-` 开头，用 `--` 分隔，例如 `kopsc proc wait -- -weird`。
> `--dry-run` 只打印判定依据，不会去查询进程。

## 按资源排序查看进程（proc.topmem / proc.topcpu）

```bash
kopsc proc topmem                    # 内存占用前 10
kopsc proc topcpu                    # CPU 占用前 10
kopsc proc topmem 20                 # 内存占用前 20
kopsc proc topcpu -n 5               # CPU 占用前 5
```

输出示例：

```
=== 占用 CPU 前 10 的进程 ===

PID      CPU      进程                 命令
1234     45.2%    java                 java -jar app.jar --spring.profiles.active=prod
5678     12.3%    nginx                nginx: worker process
```

### 选项

| 选项 | 说明 |
| --- | --- |
| `-n` / `--count` / 位置参数 | 显示数量（默认 10） |

> Windows 下 `proc topcpu` 不可用（tasklist 不报告 CPU 使用率）。

## 按进程名终止进程（proc.killname）

```bash
kopsc proc killname nginx            # 列出匹配进程（不执行）
kopsc proc killname nginx --yes      # 确认终止
```

匹配规则：进程名或命令行中包含指定关键字（忽略大小写）。

> 这是破坏性命令，未加 `--yes` 时只列出匹配进程并以退出码 2 结束。

## 进程树（proc.tree）

```bash
kopsc proc tree
```

以树形结构显示进程的父子关系。

Linux/macOS 优先使用 `pstree -p`（最佳输出），回退到 `ps -ejH`（Linux）或 `ps -eo pid,ppid,comm`（macOS）；Windows 使用 `tasklist /tree`。
