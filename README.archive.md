# archive - 压缩文件

自动识别压缩格式并解压，不用记 `tar -xzf` / `tar -xjf` / `unzip` 的区别。

## 命令

| 命令 | 作用 |
| --- | --- |
| `kopsc archive extract <文件> [--dir 目录]` | 自动识别格式并解压 |

别名：`kopsc archive unzip`、`kopsc archive untar`

## 支持的格式

| 扩展名 | 解压方式 |
| --- | --- |
| `.tar.gz` / `.tgz` | Go 内置实现 |
| `.tar.bz2` / `.tbz2` | 调用 `bzip2 -dc` + tar 解析 |
| `.tar.xz` / `.txz` | 调用 `xz -dc` + tar 解析 |
| `.tar` | Go 内置实现 |
| `.zip` / `.jar` / `.war` | Go 内置实现 |
| `.gz` | Go 内置 gzip 解压 |

## 示例

```bash
# 解压到当前目录
kopsc archive extract app.tar.gz

# 解压到指定目录
kopsc archive extract backup.zip --dir /opt/backup

# 解压 tar.bz2
kopsc archive extract data.tar.bz2
```

## 选项

| 选项 | 说明 |
| --- | --- |
| `--dir` / `-d` / `-C` | 目标目录（默认当前目录） |
