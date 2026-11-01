# 网络与端口（`kopsc net <子命令>`）

> 返回 [README.md](README.md)

查看端口被谁占用、本机监听了哪些端口。两个命令都只读，不需要 `--yes`。

```bash
kopsc net pids 8080    # 谁占用了 8080 端口（输出 PID）
kopsc net ports        # 本机正在监听哪些端口（统一表格）
```

## 端口占用查询（net.pids）

`kopsc net pids <端口>` 等价 `lsof -ti:<端口>`：输出占用该端口的进程 PID，每行一个、升序、去重。

```bash
kopsc net pids 8080
kopsc net pids --port 8080
kopsc net pids 8080 --pid-file /tmp/web.pid   # 同时写出逗号分隔的 PID
kopsc net pids 8080 --dry-run
```

- 没有进程占用时 **stdout 仍为空**、退出码仍为 0，但会在 stderr 说明「没有进程占用端口 N」，
  避免命令行下对着空输出发懵；脚本读 stdout 不受影响。
- `--pid-file <路径>`：把 PID 以逗号分隔写进文件（如 `123,456`），无占用时写空文件，
  便于 `kill $(cat web.pid)`；写完会在 stderr 提示位置。

底层按平台自动选择，但对外的命令与输出保持一致：

| 平台 | 依次尝试的底层来源 |
| --- | --- |
| Linux | `lsof -ti:<port>` → `ss -H -t -u -n -p`（解析 `pid=`） → 读 `/proc/net/*` 与 `/proc/*/fd`（纯内置，零外部依赖） |
| macOS | `lsof -ti:<port>`（系统自带，无 `/proc` 回退） |
| Windows | `netstat -ano`（解析本地地址与 PID） |

- 端口必须是 1–65535；非法端口直接报错，不执行任何外部命令。
- `lsof` 无匹配时退出码为 1 且无输出，kopsc 视为「没有进程占用」，返回 0。
- 只读命令，不需要 `--yes`。

## 监听端口（net.ports）

`kopsc net ports` 列出本机正在监听的端口，**默认输出跨平台一致的表格**（按端口升序）：

```text
$ kopsc net ports
端口    协议  地址         PID    进程
22      TCP   0.0.0.0      901    sshd
6379    TCP   127.0.0.1    1500   redis-server
53      UDP   0.0.0.0      -      -
```

- 三个平台底层分别使用 `ss -tulnp`（Linux）、`lsof -iTCP -sTCP:LISTEN -nP`（macOS）、
  `netstat -ano`（Windows），kopsc 解析后统一成同一张表；取不到属主时显示 `-`。
- 通配地址 `0.0.0.0` 与 `::` 统一显示为 `*`。
- `--raw` 保留平台原生命令的原始输出，供依赖原生格式的脚本使用。
- 没有监听端口时 stdout 为空，stderr 提示「没有正在监听的端口」。
