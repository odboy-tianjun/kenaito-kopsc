# file - 文件查找

按大小、修改时间、名称查找文件，查找空文件和空目录，不用记 `find` 的复杂语法。

## 命令

| 命令 | 作用 |
| --- | --- |
| `kopsc file findbig [路径] [--threshold 大小]` | 查找大于指定阈值的文件，按大小排序 |
| `kopsc file findrecent [路径] [--days 天数]` | 查找最近 N 天内修改过的文件 |
| `kopsc file findname <模式> [路径]` | 按文件名查找（忽略大小写，支持 `*` `?` 通配符） |
| `kopsc file empty [路径]` | 查找空文件和空目录 |

## 示例

```bash
# 找当前目录下大于 100M 的文件（默认阈值）
kopsc file findbig

# 找 /var 下大于 500M 的文件
kopsc file findbig /var --threshold 500M

# 找最近 3 天修改过的文件
kopsc file findrecent --days 3

# 找 /var/log 下所有 .log 文件
kopsc file findname "*.log" /var/log

# 找所有 .conf 文件（忽略大小写）
kopsc file findname "*.CONF"
```

## 选项

### findbig

| 选项 | 默认值 | 说明 |
| --- | --- | --- |
| `--threshold` / `-s` | `100M` | 大小阈值，支持 K/M/G/T 后缀 |
| 位置参数 | `.` | 搜索路径 |

### findrecent

| 选项 | 默认值 | 说明 |
| --- | --- | --- |
| `--days` / `-d` | `7` | 天数 |
| 位置参数 | `.` | 搜索路径 |

### findname

| 选项 | 说明 |
| --- | --- |
| 第一个位置参数 | 文件名模式（必填） |
| 第二个位置参数 | 搜索路径（默认 `.`） |

### empty

```bash
# 查找当前目录下的空文件和空目录
kopsc file empty

# 查找指定目录
kopsc file empty /var/log

# 只查找空文件
kopsc file empty --files-only

# 只查找空目录
kopsc file empty --dirs-only
```

| 选项 | 说明 |
| --- | --- |
| `--files-only` | 只查找空文件 |
| `--dirs-only` | 只查找空目录 |
| 位置参数 | 搜索路径（默认 `.`） |
