# 磁盘与文件占用（`kopsc disk <子命令>`）

> 返回 [README.md](README.md)

看哪个目录、哪个文件最占空间，查看 inode 使用，一键磁盘健康检查。

```bash
kopsc disk usage                     # 当前目录的占用排行（路径可省略）
kopsc disk usage /var                # 指定目录
kopsc disk usage /var --depth 2      # 下钻一层
kopsc disk inode                     # 查看 inode 使用情况
kopsc disk check                     # 一键磁盘健康检查
kopsc disk check /var                # 检查指定路径
kopsc disk check --all               # 检查所有挂载点
```

## 命令一览

| 命令 | 作用 |
| --- | --- |
| `kopsc disk usage [路径]` | 查看目录占用排行 |
| `kopsc disk inode [路径]` | 查看 inode 使用情况 |
| `kopsc disk check [路径]` | 一键磁盘健康检查 |

## 磁盘占用排行（disk.usage）

`kopsc disk usage [路径]` 等价于 `du -x --max-depth=1 -h <路径> | sort -hr`：

```text
$ kopsc disk usage /var
1.2G     /var/lib
800.0M   /var/cache
4.0K     /var/tmp
0        /var/opt
总计      2.0G     /var
```

- 路径**可省略**，缺省为当前目录；也可用位置参数（`/var`）或 `--path /var`、`--path=/var`，一次一个。
- `--depth N`（`-d N` / `--depth=N`）控制列出到第几层，默认 1（只看直接子项）；
  `--depth 2` 会下钻一层，未下钻的目录仍按递归大小算。
- 统计每个条目：普通文件按自身大小，目录递归累计其下普通文件的大小；每个文件只计一次，
  合计不会因加深度而重复。
- 不跟随符号链接；Unix 上通过设备号实现 `-x`（不跨文件系统），Windows 上不跟随
  reparse point，目录遍历本身不会跨越卷。
- 输出按大小降序，大小采用与 `du -h` 相同的二进制单位（1.0K、1.5M、2.0G）。
- 末尾输出该路径的合计行。

### 选项

| 选项 | 说明 |
| --- | --- |
| `[路径]` / `--path <路径>` / `--path=<路径>` | 要统计的目录，默认当前目录 |
| `--depth N` / `-d N` / `--depth=N` | 列到的层数，默认 1；必须大于 0 |

> `--path`、`--depth` 是 kopsc 自己的选项，不是 `du` 的参数，因此可以写在任意位置。

## inode 使用查看（disk.inode）

```bash
kopsc disk inode                  # 当前文件系统
kopsc disk inode /var             # 指定路径所在文件系统
```

显示文件系统的 inode 使用统计。inode 耗尽会导致无法创建新文件，即使磁盘空间充足。

输出示例：

```
Filesystem      Inodes   IUsed   IFree IUse% Mounted on
/dev/vda1      3276800  180432 3096368    6% /
```

| 平台 | 实际执行 |
| --- | --- |
| Linux / macOS | `df -i` |
| Windows | 不支持（NTFS 使用 MFT，无 inode 概念） |

## 一键磁盘健康检查（disk.check）

```bash
kopsc disk check                  # 检查根目录
kopsc disk check /var             # 检查指定路径
kopsc disk check --all            # 检查所有挂载点
```

一条命令完成磁盘排障，综合检查 5 项内容：

输出示例：

```
=== 磁盘健康检查 ===

[1/5] 空间使用
  /dev/vda1  50G 总计  22G 已用  28G 可用  44% 使用率  ✓ 正常

[2/5] inode 使用
  /dev/vda1  3.2M 总计  180K 已用  3.0M 可用  5% 使用率  ✓ 正常

[3/5] 大文件 TOP 5（>100M）
  2.1G  /var/log/syslog
  1.5G  /var/lib/docker/overlay2/...
  800M  /opt/app/data.db

[4/5] 大目录 TOP 5
  15G   /var/lib/docker
  2.3G  /var/log
  1.2G  /opt/app

[5/5] 建议
  ✓ 磁盘空间充足
  ✓ inode 使用正常
```

### 检查项目

| 项目 | 说明 |
| --- | --- |
| 空间使用 | 磁盘使用率，>80% 警告，>90% 危险 |
| inode 使用 | inode 使用率（Unix only） |
| 大文件 | 列出 >100M 的文件 TOP 5 |
| 大目录 | 列出直接子目录占用 TOP 5 |
| 建议 | 根据检查结果给出清理建议 |

### 选项

| 选项 | 说明 |
| --- | --- |
| `[路径]` | 要检查的路径，默认 `/` |
| `--all` / `-a` | 检查所有挂载点 |
