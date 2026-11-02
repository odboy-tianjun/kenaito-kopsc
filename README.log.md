# log - 系统日志

在系统日志中搜索关键字或查看最近日志，不用记 `journalctl` / `log show` / `wevtutil` 的区别。

## 命令

| 命令 | 作用 |
| --- | --- |
| `kopsc log search <关键字> [选项]` | 在系统日志中搜索关键字 |
| `kopsc log last [行数] [-u 服务]` | 查看最近的系统日志 |

## 示例

```bash
# 搜索最近 1 小时包含 "error" 的日志
kopsc log search "error" --since 1h

# 搜索 nginx 服务的日志
kopsc log search "502" -u nginx

# 查看最近 50 行系统日志（默认）
kopsc log last

# 查看最近 100 行
kopsc log last 100

# 查看 nginx 服务的最近日志
kopsc log last -u nginx
```

## 选项

### log search

| 选项 | 说明 |
| --- | --- |
| `--since` | 时间范围，如 `1h`、`30m`、`2h30m` |
| `-u` / `--unit` | 限定服务名 |
| `-n` / `--lines` | 最大行数（默认 100） |

### log last

| 选项 | 说明 |
| --- | --- |
| `-n` / `--lines` | 行数（默认 50） |
| `-u` / `--unit` | 限定服务名 |

## 平台实现

| 操作 | Linux | macOS | Windows |
| --- | --- | --- | --- |
| search | `journalctl --grep` | `log show --predicate` | `wevtutil` + 过滤 |
| last | `journalctl -n` | `log show --last` | `wevtutil /c:` |
