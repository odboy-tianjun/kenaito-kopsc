# 本机进程（`kopsc proc <子命令>`）

> 返回 [README.md](README.md)

在后台运行命令并写日志，关掉终端也不会退出。

```bash
kopsc proc nohup ./server            # 后台运行命令，输出写日志
```

## 后台运行（proc.nohup）

`kopsc proc nohup <命令> [参数...]` 等价于 `nohup <命令> > output.log 2>&1 &`：
启动命令后立刻返回，关闭终端不会终止它。

```bash
kopsc proc nohup ./server -port 8080                      # 日志写到 ./server.out（同 nohup.out）
kopsc proc nohup -o /var/log/server.log ./server          # 指定日志
kopsc proc nohup -C /srv/app --env=APP_ENV=prod ./run.sh  # 指定工作目录与环境变量
kopsc proc nohup -w ./migrate.sh                          # 等它结束，并回显日志、透传退出码
kopsc proc nohup --dry-run ./server                       # 只打印判定依据
```

- 标准输入接到空设备；标准输出与标准错误**都追加**写入日志文件（`>>` 语义），并自动创建日志目录。
- Unix 下子进程进入新会话、无控制终端（相当于 `setsid`），Windows 下以 `DETACHED_PROCESS` 启动。
- 选项只在**命令之前**解析：`kopsc proc nohup ffmpeg -i in.mp4` 中的 `-i` 原样给 ffmpeg，
  不会被 kopsc 截获（因此命令自身的 `-n`/`--dry-run` 不会触发 kopsc 的预览）。
  需要预览时把 `--dry-run` 写在命令之前、或用 `--` 分隔。
- 不经过 shell，`>`、`|`、`&&` 需要显式写 shell：`kopsc proc nohup sh -c 'a | b > c'`。

### 选项

| 选项 | 说明 |
| --- | --- |
| `-o`, `--out`, `--output` | 日志文件，默认 `<程序名>.out` |
| `-C`, `--dir`, `--chdir` | 子进程工作目录（相对日志路径也基于它），默认当前目录 |
| `-E`, `--env NAME=VALUE` | 追加环境变量，可重复 |
| `-q`, `--quiet` | 不打印启动摘要（PID、日志路径） |
| `-w`, `--wait` | 前台等待命令退出，回显其写入的日志内容并透传退出码 |
| `--detach` / `--no-detach` | 默认脱离会话；`--no-detach` 保留当前会话（类似只加 `&`） |
