# Docker 命令（`kopsc docker <子命令>`）

> 返回 [README.md](README.md)

Docker 容器与镜像管理：查看、启停、日志、清理、导出/导入、Compose。
所有命令在三个平台都调用 `docker`（macOS / Windows 上即 Docker Desktop），
参数按平台无关的意图解析，不要求你记住 docker 的细节。

```bash
kopsc docker ps                   # 列出运行中的容器
kopsc docker ps --filter name=web # 过滤：只看名字含 web 的容器
kopsc docker images               # 列出本地镜像（不知道有什么镜像时先看这个）
kopsc docker logs -f web          # 查看日志（容器名必填，选项可透传）
kopsc docker exec web sh          # 进入容器（默认就是交互模式，无需 -it）
kopsc docker clearsafe            # 日常清理：停掉的容器、未用网络、悬空镜像（推荐）
kopsc docker clearsafe --dry-run  # 只打印将执行的命令与判定依据
kopsc docker clearsafe -- --filter until=24h   # -- 之后的参数原样透传给 docker
```

## 只读（查看信息，不修改任何东西）

| 命令 | 作用 | 实际执行 / 说明 |
| --- | --- | --- |
| `kopsc docker ps` | 列出运行中的容器 | `docker ps`；`--filter`、`--format` 可透传 |
| `kopsc docker psall` | 列出全部容器（含已停止） | `docker ps -a` |
| `kopsc docker images` | 列出本地镜像（名称、标签、ID、大小） | `docker images`；不知道有什么镜像时先看这个 |
| `kopsc docker stats` | 一次性看容器 CPU / 内存 / IO 占用 | 单次快照（`--no-stream`）；不给容器名时统计全部运行中的容器 |
| `kopsc docker df` | 看镜像 / 容器 / 卷 / 构建缓存各占多少空间 | 按占用大小排序，并给出可回收空间（配合 `clearsafe` 清理） |
| `kopsc docker info` | 查看 Docker 守护进程与主机环境信息 | `docker info`；排查「Docker 没起来」时先看这个 |
| `kopsc docker version` | 查看 Docker 客户端与服务端版本 | `docker version`。**注意**：`kopsc version` 是 kopsc 自己的版本 |
| `kopsc docker logs <容器>` | 查看容器日志 | 支持 `--since` / `--grep` / `--save`，见[日志一节](#日志占用与实时状态) |
| `kopsc docker inspect <对象>` | 查看容器 / 镜像 / 网络 / 卷的完整配置（JSON） | `docker inspect`；想取某个字段可用 `--format` 透传 |
| `kopsc docker networks` | 列出 Docker 网络 | `docker network ls` |
| `kopsc docker volumes` | 列出数据卷 | `docker volume ls`；查看卷占空间用 `kopsc docker df` |

## 生命周期（启停、删除、搬运）

| 命令 | 作用 | 实际执行 / 说明 |
| --- | --- | --- |
| `kopsc docker start <容器>` | 启动已存在但已停止的容器 | `docker start`；不会创建新容器 |
| `kopsc docker stop <容器>` | 停止运行中的容器 | 先发 SIGTERM，超时后强制结束；`-t/--time` 可透传 |
| `kopsc docker restart <容器>` | 重启容器 | 等价先 stop 再 start，期间容器不可用 |
| `kopsc docker rm <容器>` | 删除已停止的容器 | 按名称 / ID / 唯一前缀定位；运行中的会提示先 `stop`；需 `--yes` |
| `kopsc docker rmi <镜像>` | 删除本地镜像 | 按名称 / ID / 唯一前缀定位；被容器引用的会删除失败；需 `--yes` |
| `kopsc docker exec <容器> <命令>` | 在运行中的容器里执行命令 / 进入容器 | 默认交互（自带 `-it`），无需手写；例：`kopsc docker exec web sh` |
| `kopsc docker pull <镜像>` | 从仓库拉取镜像 | `docker pull`；私有仓库需先 `docker login` |
| `kopsc docker tag <源镜像> <新名:标签>` | 给镜像打新标签 | `docker tag`；导出前常用来定一个可读的名字 |
| `kopsc docker save <镜像>` | 把镜像导出为 `<镜像名>.tar` | 按镜像名自动命名，传 ID 会先解析出名称 |
| `kopsc docker load <归档.tar>` | 从 `.tar` 归档导入镜像 | 归档文件直接作为参数，不必再写 `<` 重定向 |
| `kopsc docker cp <源> <目标>` | 在容器与主机之间复制文件 | 任一方向：`web:/etc/nginx nginx.bak` 或反过来 |

## 清理（全部不可恢复，执行前请确认范围）

| 命令 | 作用 | 实际执行 / 影响 |
| --- | --- | --- |
| `kopsc docker clearsafe` | **推荐的日常清理**：停掉的容器、没用到的网络、悬空镜像、构建缓存 | `docker system prune -f`；**不**删镜像标签、**不**删卷 |
| `kopsc docker pruneimages` | 只清理悬空镜像（没有标签的中间层） | `docker image prune -f` |
| `kopsc docker prunecontainers` | 只清理已停止的容器 | `docker container prune -f` |
| `kopsc docker prunenets` | 只清理没被容器使用的网络 | `docker network prune -f` |
| `kopsc docker prunebuilder` | 只清理构建缓存 | `docker builder prune -f`；下次构建会变慢 |
| `kopsc docker prunevolumes` | 清理没被使用的**数据卷** | `docker volume prune -f`；**卷内数据不可恢复**，需 `--yes` |
| `kopsc docker clearall` | 深度清理：所有未被引用的镜像（含已打标签的）与上述资源 | `docker system prune -a -f`；**需 `--yes`**，误删后需重新拉取镜像 |

## Compose（`docker-compose.yml` 所在目录下执行）

| 命令 | 作用 | 实际执行 / 说明 |
| --- | --- | --- |
| `kopsc docker compose up` | 后台启动 Compose 里定义的全部服务 | `docker compose up -d`；指定服务可透传服务名，如 `kopsc docker compose up web` |
| `kopsc docker compose down` | 停止并移除服务与网络 | `docker compose down`；默认**保留卷**，要删卷需自行透传 `-v` |
| `kopsc docker compose ps` | 查看 Compose 服务状态 | `docker compose ps` |
| `kopsc docker compose logs` | 查看 Compose 服务日志 | `docker compose logs`；`-f` 实时跟随，末尾加服务名只看某个服务 |

## 通用规则

- `logs`、`inspect`、`start`、`stop`、`restart`、`rm`、`rmi`、`exec`、`pull`、`tag`、`save`、`load`、`cp`
  需要位置参数，缺失时直接报错并给出用法。
- 破坏性命令是 `rm`、`rmi`、`clearall`（`system prune -a -f`）与 `prunevolumes`（`volume prune -f`）：
  未带 `--yes` 时先列出将删除的对象、再打印将执行的命令，并以退出码 2 结束。
- `docker compose` 对应 Compose v2。
- `docker clearsafe` 的安全契约：只加 `-f` 以便无人值守执行，**不**加 `-a`（不清理全部未使用镜像），
  **不**加 `--volumes`（不删除卷），仅清理停止的容器、未使用的网络与悬空镜像。
- 想确认某条命令在本平台实际会执行什么、为什么这样选：`kopsc explain docker.clearsafe`；
  想看某条命令自身的说明与选项：`kopsc docker clearsafe -h`。

## 镜像导出与导入（save / load）

`kopsc docker save <镜像名|镜像ID>` 等价于 `docker save <镜像> -o <镜像名>.tar`，归档文件自动命名：

```bash
kopsc docker save alpine:3.20                      # → alpine_3.20.tar
kopsc docker save 9c6f07244728                     # 按 ID 查到 alpine:3.20，同样保存为 alpine_3.20.tar
kopsc docker save sha256:9c6f07244728 --platform linux/arm64
kopsc docker load alpine_3.20.tar                  # 等价 docker load < alpine_3.20.tar
kopsc docker load a.tar b.tar                      # 一次导入多个
```

- 文件名取自镜像的**仓库名 + 标签**：标签前的冒号、路径分隔符 `/` 等无法用于文件名的字符改写为下划线，
  例如 `registry.example.com:5000/team/app:1.0` → `registry.example.com_5000_team_app_1.0.tar`；
  未打标签的镜像没有名称可依据，直接报错并提示先 `docker tag`。
- 传入镜像 ID（可省略 `sha256:` 前缀，也可用唯一前缀）时，先向 docker 查询它对应的镜像名称，再据此命名。
  同一 ID 有多个名称时要求直接指定名称，避免文件名含糊。
- `--platform` 等 docker 选项原样透传。
- `docker load` 的归档文件是**位置参数**（对应原来的 `docker load < xxx.tar`），可一次给出多个；
  `-i/--input` 仍然可用，传 `-` 仍表示从标准输入读取。

## 目标解析（exec / rm / rmi / stop / restart）

这几条命令按 kopsc 的意图做了解析，不再要求手写 docker 的细节：

```bash
kopsc docker exec web sh              # 等价 docker exec -it web -- sh（-it 已是默认）
kopsc docker exec -u 0 web id         # 容器名之前的 -u/-w/-e 照常透传给 docker
kopsc docker stop 3f2a1b              # 按 ID 前缀找到容器并停止
kopsc docker rm db                    # 删除已停止的容器；运行中的会提示先 stop
kopsc docker rmi sha256:5ca7b1d5e2fc   # 按 ID 或唯一前缀删除镜像
```

- 容器可按名称、ID 或唯一 ID 前缀定位；镜像可按名称、ID、`sha256:ID` 或唯一前缀定位。
  目标不存在或前缀有歧义时，kopsc 直接报错，不会调用 docker。
- `exec` 默认交互，命令之后的参数一律作为容器内命令，因此命令自己的 `-i`、`-p` 不会被当成 docker 选项；
  显式传 `-i`/`-t`/`-it` 会报错提示「已默认交互」。
- `rm`/`rmi` 属于破坏性命令：不带 `--yes` 时先列出将删除的对象（容器显示 ID/名称/镜像/状态，
  镜像显示 镜像/ID/创建/大小）再以退出码 2 结束；不再接受 `-f`，强删请用 `--yes`。
- `rm` 遇到运行中的容器会提示先 `kopsc docker stop <名称>`，不会替你强删。

## 日志、占用与实时状态

```bash
kopsc docker logs web --since 2h           # 只看最近 2 小时
kopsc docker logs --grep ERROR web         # 只看匹配行（kopsc 选项写在容器名之前）
kopsc docker logs --save /var/log/web.log web -f   # 同时落盘，-f 仍交给 docker
kopsc docker df                            # 按占用大小排序的类型排行
kopsc docker stats                         # 单次快照，默认统计全部运行中容器
kopsc docker stats web --format '{{.CPUPerc}}'
```

- `--since`/`--grep`/`--save` 是 kopsc 私有选项，必须写在容器名之前；容器名之后的参数原样交给 docker。
- `df` 输出「类型 / 项 / 可清理 / 大小 / 可回收」，末尾附总计与可回收提示。
- `stats` 总是单次快照（`--no-stream`），不会再刷屏；给容器名时只统计这些，且要求容器正在运行。
