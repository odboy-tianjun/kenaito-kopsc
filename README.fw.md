# fw - 防火墙

统一管理 ufw / firewalld / iptables / Windows Firewall，不用记不同工具的命令。

## 命令

| 命令 | 作用 |
| --- | --- |
| `kopsc fw status` | 查看防火墙状态与规则 |
| `kopsc fw allow <端口>[/<协议>]` | 开放端口 |
| `kopsc fw deny <端口>[/<协议>]` | 关闭端口 |

## 示例

```bash
# 查看防火墙状态
kopsc fw status

# 开放 8080/tcp（默认协议）
kopsc fw allow 8080 --yes

# 开放 53/udp
kopsc fw allow 53/udp --yes

# 关闭 8080/tcp
kopsc fw deny 8080 --yes

# 也可以写成 8080/tcp 形式
kopsc fw allow 8080/tcp --yes
```

## 选项

| 选项 | 说明 |
| --- | --- |
| `-p` / `--protocol` | 协议（`tcp` 或 `udp`，默认 `tcp`） |
| 端口格式 | `8080` 或 `8080/tcp` 或 `8080/udp` |

## 平台实现

| 操作 | Linux | macOS | Windows |
| --- | --- | --- | --- |
| status | ufw / firewalld / iptables（自动检测） | ALF（应用防火墙） | `netsh advfirewall` |
| allow | `ufw allow` / `firewall-cmd --add-port` | 不支持 | `netsh advfirewall add rule` |
| deny | `ufw deny` / `firewall-cmd --remove-port` | 不支持 | `netsh advfirewall delete rule` |
