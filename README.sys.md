# sys - 系统信息

一次性查看 OS、CPU、内存、磁盘概况，不用分别跑 `uname`、`lscpu`、`free`、`df`。

## 命令

| 命令 | 作用 |
| --- | --- |
| `kopsc sysinfo` | 显示系统信息一览 |
| `kopsc sys uptime` | 查看系统运行时间和启动时间 |

## 示例

### sysinfo

```bash
kopsc sysinfo
```

输出示例：

```
=== 系统信息 ===

操作系统:  linux/amd64
发行版:    centos
主机名:    web-01
内核:      5.15.0-91-generic

=== CPU ===

逻辑核心:  4
型号:      Intel(R) Xeon(R) CPU E5-2680 v4 @ 2.40GHz

=== 内存 ===

总计:      16.0G
检测方式:  /proc/meminfo
可用:      8.2G

=== 磁盘 ===

Filesystem      Size  Used Avail Use% Mounted on
/dev/vda1        50G   22G   28G  44% /
```

### sys uptime

```bash
kopsc sys uptime
```

输出示例：

```
运行时间:  15 天 3 小时 42 分钟
启动时间:  2026-10-19 08:30:15
负载均值:  0.15 0.22 0.18 (1/5/15分钟)
```

## 平台实现

| 信息 | Linux | macOS | Windows |
| --- | --- | --- | --- |
| OS | `/etc/os-release` | runtime.GOOS | runtime.GOOS |
| CPU | `/proc/cpuinfo` | `sysctl` | runtime |
| 内存 | `/proc/meminfo` + cgroup | `sysctl hw.memsize` | `wmic` |
| 磁盘 | `df -h` | `df -h` | `wmic logicaldisk` |
| 运行时间 | `/proc/uptime` | `sysctl kern.boottime` | `wmic os get LastBootUpTime` |
