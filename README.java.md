# Java 应用（`kopsc java <子命令>`）

> 返回 [README.md](README.md)

按 JDK 版本自动套用调优参数并前台运行 Java 应用。你不需要逐条抄 JVM 参数，
kopsc 会根据宿主机（或容器）的实际资源算出堆大小，并按 JDK 版本选择对应的 GC 日志写法。

```bash
kopsc java run app.jar                                  # 一键启动，参数自动生成
kopsc java run app.jar -- --spring.profiles.active=prod # 应用参数写在 jar 之后
kopsc java run --jdk 17 --print-flags app.jar           # 只打印最终 java 命令行
```

## 运行应用（java.run）

`kopsc java run <app.jar> [-- 应用参数...]` 等价于用一组业界常用参数执行
`java <flags> -jar <app.jar> <应用参数>`，并在前台运行：日志直接打印到终端，Ctrl+C 可停止，
退出码原样返回给调用方。

```bash
kopsc java run myapp.jar
kopsc java run myapp.jar -- --server.port=9000 --spring.profiles.active=prod
kopsc java run --jdk 11 --heap 4g myapp.jar
kopsc java run --env staging --gc-log /var/log/app myapp.jar
kopsc java run --dry-run myapp.jar          # 只看将执行的命令，不启动
```

### 自动识别的内容

| 项目 | 规则 |
| --- | --- |
| JDK 版本 | 执行 `java -version` 解析版本，识别不到时回退 `-XshowSettings:properties`；支持 **8 / 11 / 17 / 21**，其它版本会提示用 `--jdk` 指定 |
| CPU 核数 | 取可用逻辑核数；容器内优先采用 cgroup 配额（`cpu.max` / `cfs_quota_us`） |
| 内存 | Linux 读 `/proc/meminfo`（容器内以 cgroup `memory.max` / `memory.limit_in_bytes` 为准）、macOS 用 `sysctl hw.memsize`、Windows 用 `wmic` |
| 堆大小 | 默认取可用内存的 **50%**，`-Xms` 与 `-Xmx` 同值；可用 `--heap` / `--xms` 覆盖 |
| 直接内存 | `-XX:MaxDirectMemorySize` 取堆的 1/4，最低 64m |
| 配置环境 | `-Dconfig.file=application-<env>.yaml`，来源依次为 `--env`、环境变量 `APP_ENV`、默认 `prod` |

### 生成的参数

| 类别 | 参数 |
| --- | --- |
| 通用 | `-server`、`-Xss256k`、`-XX:+SegmentedCodeCache`、`-XX:+ExplicitGCInvokesConcurrent`、`-Djava.security.egd=file:/dev/./urandom` |
| 堆 | `-Xmx<size>`、`-Xms<size>`、`-XX:MaxDirectMemorySize=<size>` |
| 压缩指针 | `-XX:+UseCompressedOops`、`-XX:+UseCompressedClassPointers`；**堆超过 30g 时自动省略** |
| GC | `-XX:+UseG1GC` |
| GC 日志（JDK 9+） | `-Xlog:gc*,safepoint:<dir>/gc.log:time,uptime:filecount=100,filesize=50M` |
| GC 日志（JDK 8） | `-verbose:gc`、`-Xloggc:<dir>/gc.log`、`-XX:+PrintGCDetails`、`-XX:+PrintGCDateStamps`、`-XX:+UseGCLogFileRotation`、`-XX:NumberOfGCLogFiles=100`、`-XX:GCLogFileSize=50M` |
| 命令回显 | `-XX:+PrintCommandLineFlags` |
| 可选 | `-XX:+HeapDumpOnOutOfMemoryError`（`--heap-dump-on-oom`） |

> **JDK 8 与 9+ 的 GC 日志写法不兼容**：`-Xlog:` 是 JDK 9 引入的，JDK 8 用它会导致 JVM 直接拒绝启动。
> kopsc 按检测到的版本自动切换，所以同一份启动命令在 JDK 8 和 JDK 21 上都能跑。

### 选项

| 选项 | 说明 |
| --- | --- |
| `<app.jar>` | 应用 jar，必填 |
| `-- <应用参数>...` | 交给应用自身的参数；写在 jar 之后的所有内容也会原样传给应用 |
| `--jdk <8\|11\|17\|21>` | 指定按哪个 JDK 版本的方案生成参数；可写 `1.8`、`jdk17` |
| `--java <路径>` | 指定使用的 java 可执行文件，默认取 PATH 中的 `java` |
| `--heap <大小>` / `--xmx <大小>` | 指定堆上限，如 `4g`、`512m`；默认取可用内存 50% |
| `--xms <大小>` | 指定初始堆，默认与堆上限相同 |
| `--env <环境>` | 指定配置环境，生成 `-Dconfig.file=application-<环境>.yaml` |
| `--gc-log <目录>` | 开启 GC 日志并指定输出目录；默认不开启 |
| `--heap-dump-on-oom` | OOM 时导出堆转储 |
| `--print-flags` | 只打印最终 java 命令行（一行），可直接复制到 Dockerfile / systemd |
| `-q`, `--quiet` | 不打印启动摘要 |
| `--dry-run` | 只展示将执行的命令与资源判定，不启动进程 |

### 注意

- 选项必须写在 **jar 之前**；jar 之后的内容全部视为应用参数，所以 `--spring.profiles.active=prod`
  不会被 kopsc 当成自己的选项。
- 应用参数里出现的 `-Xmx`、`-D...` 只是应用自己的参数，kopsc 不会解析它们。
- 前台运行依赖终端；长时间运行的场景请配合 `kopsc proc nohup` 或容器编排使用。
- `--gc-log` 指定的目录不存在时会自动创建。
