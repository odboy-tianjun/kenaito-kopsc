# 磁盘与文件占用（`kopsc disk <子命令>`）

> 返回 [README.md](README.md)

看哪个目录、哪个文件最占空间。由 kopsc 内置实现，Linux、macOS、Windows 输出完全一致，
不依赖 `du`、`sort` 或 PowerShell。

```bash
kopsc disk usage                     # 当前目录的占用排行（路径可省略）
kopsc disk usage /var                # 指定目录
kopsc disk usage /var --depth 2      # 下钻一层
kopsc disk usage --path /var         # 用命名参数指定路径
```

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

## 选项

| 选项 | 说明 |
| --- | --- |
| `[路径]` / `--path <路径>` / `--path=<路径>` | 要统计的目录，默认当前目录 |
| `--depth N` / `-d N` / `--depth=N` | 列到的层数，默认 1；必须大于 0 |

> `--path`、`--depth` 是 kopsc 自己的选项，不是 `du` 的参数，因此可以写在任意位置。
