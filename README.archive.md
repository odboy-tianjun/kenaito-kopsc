# archive - 压缩文件

创建、查看、解压压缩文件，不用记 `tar -czf` / `tar -xzf` / `unzip` 的区别。

## 命令

| 命令 | 作用 |
| --- | --- |
| `kopsc archive create <路径> [选项]` | 创建压缩文件 |
| `kopsc archive list <文件>` | 列出压缩文件内容（不解压） |
| `kopsc archive extract <文件> [--dir 目录]` | 自动识别格式并解压 |

别名：`kopsc archive unzip`、`kopsc archive untar`

## 支持的格式

| 扩展名 | 创建 | 查看 | 解压 |
| --- | --- | --- | --- |
| `.tar.gz` / `.tgz` | ✓ | ✓ | ✓ |
| `.tar` | ✓ | ✓ | ✓ |
| `.zip` | ✓ | ✓ | ✓ |
| `.tar.bz2` / `.tbz2` | - | - | ✓ |
| `.tar.xz` / `.txz` | - | - | ✓ |
| `.gz` | - | - | ✓ |

## 创建压缩文件（archive.create）

```bash
# 压缩目录为 tar.gz（默认格式）
kopsc archive create ./mydir

# 压缩为 zip 格式
kopsc archive create ./mydir --format zip

# 指定输出文件名
kopsc archive create ./mydir -o backup.tar.gz

# 压缩单个文件
kopsc archive create config.yaml
```

### 选项

| 选项 | 默认值 | 说明 |
| --- | --- | --- |
| `--format` / `-f` | `tar.gz` | 压缩格式（`tar.gz` / `tgz` / `tar` / `zip`） |
| `--output` / `-o` | `<目录名>.<格式>` | 输出文件名 |

## 查看压缩文件内容（archive.list）

```bash
# 列出 tar.gz 内容
kopsc archive list backup.tar.gz

# 列出 zip 内容
kopsc archive list app.zip
```

输出示例：

```
权限        大小       修改时间               名称
------------------------------------------------------------
d0755       0         2026-11-05 10:30:00  mydir/
-0644       1234      2026-11-05 10:30:00  mydir/file1.txt
-0755       5678      2026-11-05 10:30:00  mydir/script.sh
------------------------------------------------------------
共 3 项，总大小 6.9K
```

## 解压文件（archive.extract）

```bash
# 解压到当前目录
kopsc archive extract app.tar.gz

# 解压到指定目录
kopsc archive extract backup.zip --dir /opt/backup

# 解压 tar.bz2
kopsc archive extract data.tar.bz2
```

### 选项

| 选项 | 说明 |
| --- | --- |
| `--dir` / `-d` / `-C` | 目标目录（默认当前目录） |
