# 系统软件包（`kopsc pkg <子命令>`）

> 返回 [README.md](README.md)

刷新系统包管理器的软件源索引。

```bash
kopsc pkg refreshindex    # 刷新包管理器索引（dnf/apt/apk/brew/winget，按平台自动选）
```

## 刷新软件源索引（pkg.refreshindex）

`kopsc pkg refreshindex` 刷新本机包管理器的软件源索引，**只更新索引，不升级也不安装任何软件包**。
常用于「刚换完源 / 报错说找不到某个包」时先同步一次。

```bash
kopsc pkg refreshindex        # 按当前平台自动选择：
                              # Linux centos → dnf makecache
                              # Linux ID_LIKE rhel/fedora → dnf makecache
                              # Linux ID_LIKE debian/ubuntu → apt-get update
                              # Linux alpine → apk update
                              # macOS → brew update
                              # Windows → winget source update
```

| 平台 | 实际执行 |
| --- | --- |
| Linux（`/etc/os-release` 的 `ID=centos`） | `dnf makecache` |
| Linux（`ID_LIKE` 含 rhel / fedora） | `dnf makecache` |
| Linux（`ID_LIKE` 含 debian / ubuntu） | `apt-get update` |
| Linux（`ID=alpine`） | `apk update` |
| macOS | `brew update` |
| Windows | `winget source update` |

- 需要相应包管理器的权限时（如 `apt-get update`），请以 root / sudo 运行 kopsc。
- 更新索引本身不改动已安装的软件；要升级请用系统自带的包管理器命令。
