# 用户管理

跨平台的用户权限管理命令，简化 Linux 和 macOS 上繁琐的 root/sudo 权限操作。

## 命令一览

| 命令 | 用途 | 实际执行 |
| --- | --- | --- |
| `kopsc user list` | 列出系统用户 | Linux/macOS: `getent passwd` 或 `/etc/passwd`<br>Windows: `net user` |
| `kopsc user grant <用户名>` | 授予用户 root/sudo 权限 | Linux: `usermod -aG wheel/sudo <用户名>`<br>macOS: `dseditgroup -o edit -t user -a <用户名> admin`<br>Windows: `net localgroup Administrators <用户名> /add` |
| `kopsc user revoke <用户名>` | 移除用户的 root/sudo 权限 | Linux: `gpasswd -d <用户名> wheel/sudo`<br>macOS: `dseditgroup -o edit -t user -d <用户名> admin`<br>Windows: `net localgroup Administrators <用户名> /delete` |
| `kopsc user info [用户名]` | 查看用户详细信息 | Linux/macOS: `id` + `/etc/passwd` + `groups`<br>Windows: `net user` |

## 用法

### 列出系统用户

```bash
kopsc user list
```

列出系统中所有用户账户。Linux/macOS 优先使用 `getent passwd`，回退到读取 `/etc/passwd`；Windows 使用 `net user`。

### 授予用户 root/sudo 权限

```bash
# 预览将执行的操作
kopsc user grant alice

# 确认执行（需要当前用户已有 sudo/管理员权限）
kopsc user grant alice --yes

# 简写
kopsc user grant alice -y
```

**执行后效果**：
- **Linux**：用户 `alice` 被添加到 `wheel` 组（CentOS/RHEL）或 `sudo` 组（Debian/Ubuntu），可以使用 `sudo` 执行管理员命令
- **macOS**：用户 `alice` 被添加到 `admin` 组，获得管理员权限
- **Windows**：用户 `alice` 被添加到 `Administrators` 组，获得管理员权限

### 移除用户的 root/sudo 权限

```bash
# 预览将执行的操作
kopsc user revoke alice

# 确认执行
kopsc user revoke alice --yes
```

**执行后效果**：
- **Linux**：用户 `alice` 从 `wheel` 或 `sudo` 组移除，失去 sudo 权限
- **macOS**：用户 `alice` 从 `admin` 组移除，失去管理员权限
- **Windows**：用户 `alice` 从 `Administrators` 组移除，失去管理员权限

## 注意事项

1. **需要管理员权限**：执行这些命令的当前用户必须已有 sudo/管理员权限
2. **重新登录生效**：权限变更需要用户重新登录后才能生效
3. **破坏性命令**：这两个命令都需要 `--yes` 确认，避免误操作
4. **自动检测发行版**：Linux 上会自动检测应该使用 `wheel` 组还是 `sudo` 组

## 平台差异

### Linux

不同发行版使用不同的管理员组：
- **CentOS/RHEL/Rocky/AlmaLinux**：使用 `wheel` 组
- **Debian/Ubuntu/Mint**：使用 `sudo` 组

kopsc 会自动检测系统中存在的组，优先使用 `wheel`，其次是 `sudo`。

### macOS

macOS 使用 `admin` 组管理管理员权限，通过 `dseditgroup` 命令操作。

### Windows

Windows 使用 `Administrators` 本地组，通过 `net localgroup` 命令操作。

## 示例场景

### 场景 1：新入职的运维工程师需要 sudo 权限

```bash
# 创建用户（系统命令）
sudo useradd -m -s /bin/bash ops01

# 授予 sudo 权限（kopsc 命令）
kopsc user grant ops01 --yes

# 通知用户重新登录
echo "请重新登录以启用 sudo 权限"
```

### 场景 2：离职员工需要移除管理员权限

```bash
# 移除管理员权限
kopsc user revoke former_employee --yes

# 可选：禁用账户（系统命令）
sudo usermod -L former_employee
```

### 场景 3：批量管理用户权限

```bash
# 批量授权
for user in alice bob charlie; do
  kopsc user grant $user --yes
done

# 批量撤销
for user in alice bob charlie; do
  kopsc user revoke $user --yes
done
```

## 故障排查

### 问题：提示"未找到 wheel 或 sudo 组"

**原因**：系统使用了非标准的管理员组名称

**解决**：
1. 查看系统的管理员组：`cat /etc/group | grep -E '^(wheel|sudo|admin)'`
2. 手动执行命令：`sudo usermod -aG <组名> <用户名>`

### 问题：执行后用户仍无法使用 sudo

**原因**：权限变更需要重新登录才能生效

**解决**：让用户退出并重新登录，或执行 `su - <用户名>` 切换到该用户

### 问题：提示权限不足

**原因**：当前用户没有 sudo/管理员权限

**解决**：使用已有管理员权限的账户执行，或临时使用 `sudo` 前缀

## 查看用户信息（user.info）

```bash
# 查看当前用户信息
kopsc user info

# 查看指定用户信息
kopsc user info alice
```

输出示例：

```
用户: uid=501(alice) gid=20(staff) groups=20(staff),80(admin),204(_developer)
UID:     501
GID:     20
描述:    Alice Wang
家目录:  /Users/alice
Shell:   /bin/zsh
组:      staff admin _developer
```

显示用户的 UID、GID、所属组、描述信息、家目录、默认 Shell 等。不指定用户名时显示当前用户信息。
