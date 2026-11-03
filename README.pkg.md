# 系统软件包（`kopsc pkg <子命令>`）

> 返回 [README.md](README.md)

管理软件包：列出、搜索、安装、卸载、刷新索引。

```bash
kopsc pkg list              # 列出已安装的软件包
kopsc pkg search nginx      # 搜索软件包
kopsc pkg install nginx     # 安装软件包
kopsc pkg remove nginx      # 卸载软件包
kopsc pkg info nginx        # 查看软件包详情
kopsc pkg refreshindex      # 刷新包管理器索引
```

## 命令一览

| 命令 | 作用 |
| --- | --- |
| `kopsc pkg list` | 列出已安装的软件包 |
| `kopsc pkg search <关键字>` | 搜索软件包 |
| `kopsc pkg install <包名>` | 安装软件包（需 `--yes`） |
| `kopsc pkg remove <包名>` | 卸载软件包（需 `--yes`） |
| `kopsc pkg info <包名>` | 查看软件包详情 |
| `kopsc pkg refreshindex` | 刷新包管理器索引 |

## 列出已安装的软件包（pkg.list）

```bash
kopsc pkg list
```

列出系统中已安装的软件包。

| 平台 | 实际执行 |
| --- | --- |
| Linux（Debian/Ubuntu） | `dpkg -l` |
| Linux（CentOS/RHEL/Fedora） | `rpm -qa` |
| Linux（Alpine） | `apk list --installed` |
| macOS | `brew list` |
| Windows | `winget list` |

## 搜索软件包（pkg.search）

```bash
kopsc pkg search nginx
kopsc pkg search redis
```

在软件源中搜索匹配关键字的软件包。

| 平台 | 实际执行 |
| --- | --- |
| Linux（Debian/Ubuntu） | `apt-cache search` |
| Linux（CentOS/RHEL/Fedora） | `dnf search` |
| Linux（Alpine） | `apk search` |
| macOS | `brew search` |
| Windows | `winget search` |

## 安装软件包（pkg.install）

```bash
kopsc pkg install nginx              # 预览将执行的操作
kopsc pkg install nginx --yes        # 确认安装
kopsc pkg install nginx redis -y     # 安装多个包
```

安装指定的软件包。这是破坏性命令，需要 `--yes` 确认。

| 平台 | 实际执行 |
| --- | --- |
| Linux（Debian/Ubuntu） | `apt-get install` |
| Linux（CentOS/RHEL/Fedora） | `dnf install` |
| Linux（Alpine） | `apk add` |
| macOS | `brew install` |
| Windows | `winget install` |

## 卸载软件包（pkg.remove）

```bash
kopsc pkg remove nginx              # 预览将执行的操作
kopsc pkg remove nginx --yes        # 确认卸载
```

卸载指定的软件包。这是破坏性命令，需要 `--yes` 确认。

| 平台 | 实际执行 |
| --- | --- |
| Linux（Debian/Ubuntu） | `apt-get remove` |
| Linux（CentOS/RHEL/Fedora） | `dnf remove` |
| Linux（Alpine） | `apk del` |
| macOS | `brew uninstall` |
| Windows | `winget uninstall` |

## 查看软件包详情（pkg.info）

```bash
kopsc pkg info nginx
kopsc pkg info python3
```

显示指定软件包的详细信息（版本、描述、依赖等）。

| 平台 | 实际执行 |
| --- | --- |
| Linux（Debian/Ubuntu） | `apt show`（回退 `dpkg -s`） |
| Linux（CentOS/RHEL/Fedora） | `rpm -qi` |
| Linux（Alpine） | `apk info -a` |
| macOS | `brew info` |
| Windows | `winget show` |

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
