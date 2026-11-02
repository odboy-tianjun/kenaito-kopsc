# svc - 服务管理

统一管理 systemd / launchctl / sc 的服务操作，不用记不同平台的命令。

## 命令

| 命令 | 作用 |
| --- | --- |
| `kopsc svc status <服务名>` | 查看服务状态与最近 20 行日志 |
| `kopsc svc enable <服务名>` | 启用并启动服务（开机自启 + 立即启动） |
| `kopsc svc disable <服务名>` | 禁用并停止服务 |
| `kopsc svc logs <服务名> [-f] [-n 行数]` | 查看服务日志，`-f` 实时跟踪 |

## 示例

```bash
# 查看 nginx 状态和最近日志
kopsc svc status nginx

# 启用并启动 nginx
kopsc svc enable nginx --yes

# 禁用并停止 nginx
kopsc svc disable nginx --yes

# 查看 nginx 最近 100 行日志
kopsc svc logs nginx -n 100

# 实时跟踪 nginx 日志
kopsc svc logs nginx -f
```

## 平台实现

| 操作 | Linux | macOS | Windows |
| --- | --- | --- | --- |
| status | `systemctl status` + `journalctl` | `launchctl list` + `log show` | `sc query` |
| enable | `systemctl enable` + `start` | `launchctl load -w` | `sc config start=auto` + `start` |
| disable | `systemctl stop` + `disable` | `launchctl unload -w` | `sc stop` + `config start=disabled` |
| logs | `journalctl -u` | `log show` | `wevtutil` |
