# 网络与端口（`kopsc net <子命令>`）

> 返回 [README.md](README.md)

查看端口被谁占用、本机监听了哪些端口、测试连通性、DNS 查询、路由追踪、HTTP 诊断、网卡信息、连接统计、网速测试、启动 HTTP 文件服务器。

```bash
kopsc net pids 8080                # 谁占用了 8080 端口（输出 PID）
kopsc net ports                    # 本机正在监听哪些端口（统一表格）
kopsc net ping example.com         # 测试连通性
kopsc net trace example.com        # 路由追踪
kopsc net dns example.com          # DNS 查询
kopsc net curl https://example.com # HTTP 诊断
kopsc net iface                    # 查看网卡和 IP 地址
kopsc net conn                     # 查看 TCP 连接状态统计
kopsc net speed                    # 简单网速测试
kopsc net http                     # 启动 HTTP 文件服务器
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

## 连通性测试（net.ping）

```bash
kopsc net ping example.com         # 默认 4 个包
kopsc net ping example.com -c 10   # 发送 10 个包
```

底层调用系统 `ping` 命令。

## 路由追踪（net.trace）

```bash
kopsc net trace example.com
```

优先使用 `mtr --report`（最佳输出），回退到 `traceroute`（Linux/macOS）或 `tracert`（Windows）。

## DNS 查询（net.dns）

```bash
kopsc net dns example.com              # 默认 A 记录
kopsc net dns example.com -t MX        # 查询 MX 记录
kopsc net dns example.com -t AAAA      # 查询 AAAA 记录
kopsc net dns example.com -s 8.8.8.8   # 指定 DNS 服务器
```

优先使用 `dig`，回退到 `nslookup` 或 `host`。

### 选项

| 选项 | 说明 |
| --- | --- |
| `-t` / `--type` | 记录类型（A/AAAA/MX/CNAME 等） |
| `-s` / `--server` | DNS 服务器地址 |

## HTTP 诊断（net.curl）

```bash
kopsc net curl https://example.com          # 基本诊断
kopsc net curl https://example.com -L       # 跟随重定向
kopsc net curl https://api.example.com -X POST -H "Content-Type: application/json"
```

输出示例：

```
HTTP 状态码:  200
重定向次数:  0
DNS 解析:   0.005s
TCP 连接:   0.032s
TLS 握手:   0.089s
首字节:     0.145s
总耗时:     0.146s
下载大小:   1234 bytes
下载速度:   8452.05 bytes/s
```

### 选项

| 选项 | 说明 |
| --- | --- |
| `-L` / `--follow` | 跟随重定向 |
| `-X` / `--method` | HTTP 方法（GET/POST/PUT 等） |
| `-H` / `--header` | 添加请求头，可重复 |

## 网卡信息（net.iface）

```bash
kopsc net iface
```

列出所有非回环网卡的信息：名称、状态（UP/DOWN）、MAC 地址、IPv4/IPv6 地址、MTU。

纯 Go 实现，跨平台一致，不依赖外部命令。

## 连接统计（net.conn）

```bash
kopsc net conn
```

统计各状态的 TCP 连接数量，输出示例：

```
=== TCP 连接统计 (共 108 个) ===

ESTABLISHED     61
LISTEN          32
TIME_WAIT       14
CLOSE_WAIT      1
```

Linux 优先使用 `ss -tan`（更快），回退到 `netstat -tan`；macOS/Windows 使用 `netstat -tan`。

## 网速测试（net.speed）

```bash
kopsc net speed                    # 默认测试 10MB
kopsc net speed --size 50M         # 测试 50MB
kopsc net speed --download         # 只测下载
kopsc net speed --upload           # 只测上传
```

从 Cloudflare 下载测试文件测量下载速度，向 httpbin.org 上传数据测量上传速度。

输出示例：

```
=== 网络速度测试 ===

测试下载速度...
下载速度: 12.5M/s

测试上传速度...
上传速度: 3.2M/s
```

### 选项

| 选项 | 说明 |
| --- | --- |
| `--size` / `-s` | 测试文件大小（默认 10M，支持 K/M/G 后缀） |
| `--download` / `-d` | 只测下载 |
| `--upload` / `-u` | 只测上传 |

## HTTP 文件服务器（net.http）

```bash
kopsc net http                    # 当前目录起 HTTP 服务，端口 8000
kopsc net http -p 9000            # 指定端口
kopsc net http --dir /var/www     # 指定目录
kopsc net http --upload           # 启用上传功能
```

快速启动一个 HTTP 文件服务器，方便局域网内共享文件。纯 Go 实现，不依赖外部工具。

输出示例：

```
=== HTTP 文件服务器 ===

目录: /Users/odboy/projects
地址:
  http://localhost:8000
  http://192.168.1.100:8000

按 Ctrl+C 停止
```

### 选项

| 选项 | 说明 |
| --- | --- |
| `--dir` / `-d` | 服务目录（默认当前目录） |
| `--port` / `-p` | 端口号（默认 8000） |
| `--upload` / `-u` | 启用上传功能（POST /upload） |

### 上传功能

启用 `--upload` 后，可通过 POST 请求上传文件：

```bash
curl -F "file=@document.pdf" http://localhost:8000/upload
```

上传的文件会保存到服务目录，同名文件自动添加后缀避免覆盖。
