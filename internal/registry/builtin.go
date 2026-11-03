package registry

// Builtin returns the registry compiled into kopsc. Every variant is explicit
// about the platform it targets; resolution picks the most specific match.
func Builtin() *Registry {
	return &Registry{Commands: []Command{
		// ----- read-only -----
		{
			ID:       "docker.ps",
			Group:    "docker",
			Summary:  "列出运行中的容器",
			Action:   "只读：等价 docker ps，参数可透传（如 --format、--filter）",
			Variants: runtimeVariants("ps"),
		},
		{
			ID:       "docker.psall",
			Group:    "docker",
			Summary:  "列出全部容器（含已停止）",
			Action:   "只读：等价 docker ps -a",
			Variants: runtimeVariants("ps", "-a"),
		},
		{
			ID:       "docker.images",
			Group:    "docker",
			Summary:  "列出本地镜像",
			Action:   "只读：等价 docker images",
			Variants: runtimeVariants("images"),
		},
		{
			ID:      "docker.stats",
			Group:   "docker",
			Summary: "一次性查看容器 CPU/内存/IO 占用",
			Action: "只读：等价 docker stats --no-stream，输出一次后退出；不给容器名时自动统计全部运行中的容器，" +
				"给了名称或 ID 时只统计这些（需正在运行）；--format 等选项可透传",
			Usage:    "[容器...]",
			Variants: runtimeBuiltinVariants("stats"),
		},
		{
			ID:      "docker.df",
			Group:   "docker",
			Summary: "磁盘占用排行（镜像/容器/卷/构建缓存）",
			Action: "只读：在 docker system df 之上按占用大小降序排列，输出「类型 / 项 / 可清理 / 大小 / 可回收」，" +
				"并给出总计与一句可回收空间提示",
			Variants: runtimeBuiltinVariants("df"),
		},
		{
			ID:       "docker.info",
			Group:    "docker",
			Summary:  "查看容器运行时与主机环境信息",
			Action:   "只读：等价 docker info",
			Variants: runtimeVariants("info"),
		},
		{
			ID:       "docker.version",
			Group:    "docker",
			Summary:  "查看容器运行时版本",
			Action:   "只读：等价 docker version",
			Variants: runtimeVariants("version"),
		},
		{
			ID:      "docker.logs",
			Group:   "docker",
			Summary: "查看容器日志（可过滤、可落盘）",
			Action: "等价 docker logs <容器>，并增加 kopsc 选项：--since 只看某时间段、--grep 只显示匹配行、--save 把显示的日志追加写入文件；" +
				"--since/--grep/--save 必须写在容器名之前，容器名之后的 -f、--tail 等原样交给 docker",
			Usage:    "<容器> [选项...]",
			MinArgs:  1,
			Variants: runtimeBuiltinVariants("logs"),
		},
		{
			ID:       "docker.inspect",
			Group:    "docker",
			Summary:  "查看容器/镜像/网络/卷的详细信息",
			Action:   "只读：等价 docker inspect <对象>",
			Usage:    "<对象>",
			MinArgs:  1,
			Variants: runtimeVariants("inspect"),
		},
		{
			ID:       "docker.networks",
			Group:    "docker",
			Summary:  "列出容器网络",
			Action:   "只读：等价 docker network ls",
			Variants: runtimeVariants("network", "ls"),
		},
		{
			ID:       "docker.volumes",
			Group:    "docker",
			Summary:  "列出数据卷",
			Action:   "只读：等价 docker volume ls",
			Variants: runtimeVariants("volume", "ls"),
		},

		// ----- lifecycle -----
		{
			ID:       "docker.start",
			Group:    "docker",
			Summary:  "启动已停止的容器",
			Action:   "启动一个或多个已存在的容器，不创建新容器",
			Usage:    "<容器>",
			MinArgs:  1,
			Variants: runtimeVariants("start"),
		},
		{
			ID:      "docker.stop",
			Group:   "docker",
			Summary: "停止运行中的容器（按名称或 ID）",
			Action: "等价 docker stop：容器按名称、ID 或唯一 ID 前缀解析，一次可停多个；" +
				"先发送 SIGTERM，超时后由 docker 强制结束。-t/--time、-s/--signal 可透传",
			Usage:    "<容器>...",
			MinArgs:  1,
			Variants: runtimeBuiltinVariants("stop"),
		},
		{
			ID:      "docker.restart",
			Group:   "docker",
			Summary: "重启容器（按名称或 ID）",
			Action: "等价 docker restart：容器按名称、ID 或唯一 ID 前缀解析，一次可重启多个；" +
				"-t/--time、-s/--signal 可透传",
			Usage:    "<容器>...",
			MinArgs:  1,
			Variants: runtimeBuiltinVariants("restart"),
		},
		{
			ID:      "docker.rm",
			Group:   "docker",
			Summary: "删除容器（按名称或 ID）",
			Action: "等价 docker rm：容器按名称、ID 或唯一 ID 前缀解析，一次可删多个；" +
				"运行中的容器不会被强制删除，需先 kopsc docker stop。不再接受 -f，需要强删请用 --yes 确认",
			Usage:          "<容器>...",
			MinArgs:        1,
			RequiresYes:    true,
			ConfirmBuiltin: "docker.rm.preview",
			Variants:       runtimeBuiltinVariants("rm"),
		},
		{
			ID:      "docker.rmi",
			Group:   "docker",
			Summary: "删除本地镜像（按名称或 ID）",
			Action: "等价 docker rmi：镜像按名称、ID、sha256:ID 或唯一 ID 前缀解析，一次可删多个；" +
				"未打标签的悬空镜像按 ID 删除。不再接受 -f，需要强删请用 --yes 确认",
			Usage:          "<镜像名|镜像ID>...",
			MinArgs:        1,
			RequiresYes:    true,
			ConfirmBuiltin: "docker.rmi.preview",
			Variants:       runtimeBuiltinVariants("rmi"),
		},
		{
			ID:      "docker.exec",
			Group:   "docker",
			Summary: "在运行中的容器内执行命令（默认交互）",
			Action: "等价 docker exec -it <容器> -- <命令...>：默认就是交互模式，无需再写 -it（显式传 -i/-t/-it 会报错）；" +
				"容器名之后的参数一律作为在容器内执行的命令，其中的 -i、-p 等不会被当成 docker 选项；" +
				"容器名之前的选项（如 -u、-w、-e）原样透传给 docker",
			Usage:    "<容器> <命令...>",
			MinArgs:  1,
			Variants: runtimeBuiltinVariants("exec"),
		},
		{
			ID:       "docker.pull",
			Group:    "docker",
			Summary:  "拉取镜像",
			Action:   "从镜像仓库拉取指定镜像",
			Usage:    "<镜像>",
			MinArgs:  1,
			Variants: runtimeVariants("pull"),
		},
		{
			ID:       "docker.tag",
			Group:    "docker",
			Summary:  "给镜像打标签",
			Action:   "把已有镜像标记为新的名称与标签",
			Usage:    "<源镜像> <目标镜像>",
			MinArgs:  2,
			Variants: runtimeVariants("tag"),
		},
		{
			ID:      "docker.save",
			Group:   "docker",
			Summary: "把镜像导出为 <镜像名>.tar",
			Action: "等价 docker save <镜像> -o <镜像名>.tar：归档文件按镜像的仓库名与标签命名，" +
				"标签的冒号改写为下划线（alpine:3.20 → alpine_3.20.tar，registry:5000/team/app:1.0 → registry_5000_team_app_1.0.tar）；" +
				"传入镜像 ID 时先解析出它对应的镜像名称，再据此命名；未打标签的镜像直接报错。" +
				"--platform 等选项原样透传给底层运行时",
			Usage:    "<镜像名|镜像ID>",
			MinArgs:  1,
			Variants: runtimeBuiltinVariants("save"),
		},
		{
			ID:      "docker.load",
			Group:   "docker",
			Summary: "从 <归档.tar> 导入镜像",
			Action: "等价 docker load -i <归档.tar>：归档文件作为位置参数（对应原本的 docker load < xxx.tar），" +
				"可一次给出多个；仍可用 -i/--input 指定，或传 - 表示从标准输入读取",
			Usage:    "<归档.tar>...",
			MinArgs:  1,
			Variants: runtimeBuiltinVariants("load"),
		},
		{
			ID:       "docker.cp",
			Group:    "docker",
			Summary:  "在容器与主机之间复制文件",
			Action:   "等价 docker cp <源> <目标>，任一方向均可",
			Usage:    "<源> <目标>",
			MinArgs:  2,
			Variants: runtimeVariants("cp"),
		},

		// ----- cleanup -----
		{
			ID:       "docker.clearsafe",
			Aliases:  []string{"docker.prunesafe"},
			Group:    "docker",
			Summary:  "安全清理未使用的容器、网络与构建缓存",
			Action:   "执行容器运行时的默认清理作用域：不清理全部未使用镜像（-a），不删除卷（--volumes），仅移除停止的容器、未使用的网络和悬空镜像",
			Variants: runtimeVariants("system", "prune", "-f"),
		},
		{
			ID:       "docker.pruneimages",
			Group:    "docker",
			Summary:  "清理悬空镜像（dangling）",
			Action:   "等价 docker image prune -f，只删除没有标签且未被引用的悬空镜像",
			Variants: runtimeVariants("image", "prune", "-f"),
		},
		{
			ID:       "docker.prunecontainers",
			Group:    "docker",
			Summary:  "清理已停止的容器",
			Action:   "等价 docker container prune -f，删除所有已停止的容器",
			Variants: runtimeVariants("container", "prune", "-f"),
		},
		{
			ID:       "docker.prunenets",
			Group:    "docker",
			Summary:  "清理未使用的网络",
			Action:   "等价 docker network prune -f，删除未被容器使用的网络",
			Variants: runtimeVariants("network", "prune", "-f"),
		},
		{
			ID:       "docker.prunebuilder",
			Group:    "docker",
			Summary:  "清理构建缓存",
			Action:   "等价 docker builder prune -f，删除未被使用的构建缓存",
			Variants: runtimeVariants("builder", "prune", "-f"),
		},
		{
			ID:          "docker.prunevolumes",
			Group:       "docker",
			Summary:     "清理未使用的数据卷（破坏性）",
			Action:      "等价 docker volume prune -f：删除所有未被容器使用的卷，卷内数据不可恢复；必须显式加 --yes 才会执行",
			RequiresYes: true,
			Variants:    runtimeVariants("volume", "prune", "-f"),
		},
		{
			ID:          "docker.clearall",
			Group:       "docker",
			Summary:     "深度清理全部未使用镜像与资源（破坏性）",
			Action:      "等价 docker system prune -a -f：删除所有未使用镜像（含已打标签但未被容器引用的镜像）、停止的容器、未使用网络与构建缓存，但不删除卷；必须显式加 --yes 才会执行",
			RequiresYes: true,
			Variants:    runtimeVariants("system", "prune", "-a", "-f"),
		},

		// ----- compose -----
		{
			ID:       "docker.compose.up",
			Group:    "docker",
			Summary:  "以守护方式启动 Compose 服务",
			Action:   "等价 docker compose up -d；服务名可透传指定",
			Variants: runtimeVariants("compose", "up", "-d"),
		},
		{
			ID:       "docker.compose.down",
			Group:    "docker",
			Summary:  "停止并移除 Compose 服务与网络",
			Action:   "等价 docker compose down；默认保留卷，删除卷需自行透传 -v",
			Variants: runtimeVariants("compose", "down"),
		},
		{
			ID:       "docker.compose.ps",
			Group:    "docker",
			Summary:  "列出 Compose 服务状态",
			Action:   "只读：等价 docker compose ps",
			Variants: runtimeVariants("compose", "ps"),
		},
		{
			ID:       "docker.compose.logs",
			Group:    "docker",
			Summary:  "查看 Compose 服务日志",
			Action:   "只读：等价 docker compose logs；-f、服务名可透传",
			Variants: runtimeVariants("compose", "logs"),
		},

		// ----- cross-runtime diagnosis -----
		{
			ID:         "diagnose",
			Group:      "diagnose",
			Summary:    "诊断容器或 Pod 当前状态的根因",
			Action:     "自动识别 Docker 容器或 Kubernetes Pod，采集状态、最近日志与事件，给出原因、依据与建议；只读，不修改任何对象。--kind 可强制来源，K8s 下 -n/--namespace 指定命名空间",
			Usage:      "<容器名或 Pod 名>",
			MinArgs:    1,
			Namespaced: true,
			Variants: []Variant{
				{
					Match:   Match{OS: "linux"},
					Builtin: "diagnose",
					Note:    "Linux 内置诊断（docker 与 kubectl 自动识别）",
				},
				{
					Match:   Match{OS: "darwin"},
					Builtin: "diagnose",
					Note:    "macOS 内置诊断（Docker Desktop 与 kubectl）",
				},
				{
					Match:   Match{OS: "windows"},
					Builtin: "diagnose",
					Note:    "Windows 内置诊断（Docker Desktop 与 kubectl）",
				},
			},
		},

		// ----- non-container examples -----
		{
			ID:      "disk.usage",
			Aliases: []string{"disk.du"},
			Group:   "disk",
			Summary: "看哪个目录或文件最占空间（按大小从大到小排）",
			Action: "等价于 du -x --max-depth=1 -h <路径> | sort -hr：统计每个直接子项（文件或目录递归）的占用，" +
				"不跨越文件系统边界，末尾输出该路径的合计。路径可省略（默认当前目录），也可作位置参数或用 --path 指定；" +
				"由于 Windows 没有等价的 du/sort，这里使用 kopsc 内置实现以保证三平台输出一致",
			Usage: "[路径]",
			Variants: []Variant{
				{
					Match:   Match{OS: "linux"},
					Builtin: "disk.usage",
					Note:    "Linux 内置实现（等价 du -x -d 1 -h | sort -hr）",
				},
				{
					Match:   Match{OS: "darwin"},
					Builtin: "disk.usage",
					Note:    "macOS 内置实现（BSD du 不支持 --max-depth）",
				},
				{
					Match:   Match{OS: "windows"},
					Builtin: "disk.usage",
					Note:    "Windows 内置实现（系统没有 du/sort）",
				},
			},
		},
		{
			ID:      "net.pids",
			Group:   "net",
			Summary: "查找占用指定端口的进程 PID",
			Action:  "等价 lsof -ti:<端口>：每行输出一个占用该端口的进程 PID。Linux/macOS 调用 lsof，Windows 解析 netstat -ano；无进程占用时无输出",
			Usage:   "<端口>",
			MinArgs: 1,
			Variants: []Variant{
				{
					Match:   Match{OS: "linux"},
					Builtin: "net.pids",
					Note:    "Linux 使用 lsof -ti",
				},
				{
					Match:   Match{OS: "darwin"},
					Builtin: "net.pids",
					Note:    "macOS 使用 lsof -ti",
				},
				{
					Match:   Match{OS: "windows"},
					Builtin: "net.pids",
					Note:    "Windows 解析 netstat -ano",
				},
			},
		},
		{
			ID:      "net.ports",
			Group:   "net",
			Summary: "列出本机正在监听的端口（表格）",
			Action: "只读：输出「端口 / 协议 / 地址 / PID / 进程」，按端口升序，三个平台格式一致；" +
				"--raw 保留平台原生命令（Linux ss -tulnp、macOS lsof、Windows netstat -ano）的原始输出",
			Variants: []Variant{
				{
					Match:   Match{OS: "linux"},
					Builtin: "net.ports",
					Note:    "Linux 内置实现（ss -tulnp 解析为表格）",
				},
				{
					Match:   Match{OS: "darwin"},
					Builtin: "net.ports",
					Note:    "macOS 内置实现（lsof 解析为表格）",
				},
				{
					Match:   Match{OS: "windows"},
					Builtin: "net.ports",
					Note:    "Windows 内置实现（netstat -ano 解析为表格）",
				},
			},
		},
		{
			ID:      "proc.nohup",
			Group:   "proc",
			Summary: "在后台运行命令，输出写入日志（等价 nohup <命令> > output.log 2>&1 &）",
			Action: "启动一个与当前终端脱离的命令并立即返回：标准输入接到空设备，" +
				"标准输出与标准错误都追加写入日志文件（默认 <程序名>.out，可用 -o 指定），" +
				"Unix 下进入新会话、无控制终端，Windows 下以 DETACHED_PROCESS 启动，关闭终端不会终止它。" +
				"选项在命令之前解析，命令之后的参数（含 -i 之类）原样透传，不加 shell；" +
				"命令自身的 -n/--dry-run 不会被 kopsc 截获；kopsc 自身的预览等选项写在命令之前或用 -- 分隔",
			Usage:   "<命令> [参数...]",
			MinArgs: 1,
			Variants: []Variant{
				{
					Match:   Match{OS: "linux"},
					Builtin: "proc.nohup",
					Note:    "Linux 内置实现（setsid 新会话 + 日志追加）",
				},
				{
					Match:   Match{OS: "darwin"},
					Builtin: "proc.nohup",
					Note:    "macOS 内置实现（setsid 新会话 + 日志追加）",
				},
				{
					Match:   Match{OS: "windows"},
					Builtin: "proc.nohup",
					Note:    "Windows 内置实现（DETACHED_PROCESS + 日志追加）",
				},
			},
		},
		{
			ID:      "proc.wait",
			Group:   "proc",
			Summary: "等待进程退出（关键字可多个，逻辑与；也可按 PID）",
			Action: "每秒检查一次目标进程是否还在：存在就继续等，直到它消失为止。" +
				"关键字在进程名与命令行中匹配（忽略大小写），多个关键字需全部命中（逻辑与）；" +
				"--pid 可直接指定进程 ID（逗号分隔可多个），与关键字同时给出时任一方仍存在都会继续等待。" +
				"默认一直等到进程退出；--timeout 可限时，超时以退出码 2 结束；--once 只检查一次不等待。" +
				"只读，不修改任何进程",
			Usage: "<关键字>... [--pid PID]...",
			Variants: []Variant{
				{
					Match:   Match{OS: "linux"},
					Builtin: "proc.wait",
					Note:    "Linux 内置实现（读 /proc，回退 ps）",
				},
				{
					Match:   Match{OS: "darwin"},
					Builtin: "proc.wait",
					Note:    "macOS 内置实现（ps -eo pid=,args=）",
				},
				{
					Match:   Match{OS: "windows"},
					Builtin: "proc.wait",
					Note:    "Windows 内置实现（tasklist，关键字只能匹配进程名）",
				},
			},
		},
		{
			ID:      "java.run",
			Group:   "java",
			Summary: "按 JDK 版本套用调优参数并前台运行 Java 应用",
			Action: "以 kopsc 内置的业界常用调优参数启动 Spring 风格应用：自动识别宿主机的 CPU 与可用内存" +
				"（容器内以 cgroup 限额为准），堆大小默认取可用内存的 50%；按检测到的 JDK 版本套用对应参数，" +
				"JDK 8 使用 -Xloggc 组，JDK 9+ 使用 -Xlog: 组；应用参数写在 jar 之后，或用 -- 分隔。" +
				"前台运行，Ctrl+C 可停止，退出码透传",
			Usage:          "<app.jar> [-- 应用参数...]",
			MinArgs:        1,
			ConfirmBuiltin: "java.run.preview",
			Variants: []Variant{
				{
					Match:   Match{OS: "linux"},
					Builtin: "java.run",
					Note:    "Linux 内置实现（/proc 与 cgroup 探测资源）",
				},
				{
					Match:   Match{OS: "darwin"},
					Builtin: "java.run",
					Note:    "macOS 内置实现（sysctl 探测内存）",
				},
				{
					Match:   Match{OS: "windows"},
					Builtin: "java.run",
					Note:    "Windows 内置实现（wmic 探测内存）",
				},
			},
		},
		{
			ID:      "git.retime",
			Group:   "git",
			Summary: "把指定日期范围的提交时间改到 21:00-23:00 并强推",
			Action: "读取本地分支在给定日期范围内（含两端）的提交，保留各自的原日期，" +
				"把时分秒重排到 21:00-23:00 之间：同一日期内相邻提交严格递增，间隔取 5-40 分钟的随机值（秒位随机），" +
				"也可用 --interval match-remote 参照远程分支的间隔；加 --start-date 时改为按顺序重排日期，" +
				"均匀铺在 [--start-date, --end-date] 之间。随后重建这段历史并以 --force-with-lease 强推。" +
				"改动前自动创建 refs/kopsc-backup/<分支>-<时间戳> 备份引用，工作区有未提交改动时拒绝执行；" +
				"默认只打印预览表并以退出码 2 结束，必须显式加 --yes 才会改写并推送",
			Usage:          "<开始日期> <结束日期> [选项]",
			MinArgs:        2,
			RequiresYes:    true,
			ConfirmBuiltin: "git.retime.preview",
			Variants: []Variant{
				{
					Match:   Match{OS: "linux"},
					Builtin: "git.retime",
					Note:    "Linux 内置实现（调用本机 git）",
				},
				{
					Match:   Match{OS: "darwin"},
					Builtin: "git.retime",
					Note:    "macOS 内置实现（调用本机 git）",
				},
				{
					Match:   Match{OS: "windows"},
					Builtin: "git.retime",
					Note:    "Windows 内置实现（调用本机 git）",
				},
			},
		},
		{
			ID:      "git.time",
			Group:   "git",
			Summary: "修改最后一次提交的时间并强推",
			Action: "把本地最后一次提交的作者时间和提交时间改为用户指定的时间，然后强制推送到远程。" +
				"时间格式必须是 yyyy-MM-dd HH:mm:ss（如 2026-11-05 22:30:00）。" +
				"同时修改 Author Date 和 Committer Date，确保远程页面显示正确。" +
				"默认只打印预览并以退出码 2 结束，必须显式加 --yes 才会改写并推送",
			Usage:       "<时间> [选项]",
			MinArgs:     1,
			RequiresYes: true,
			Variants: []Variant{
				{
					Match:   Match{OS: "linux"},
					Builtin: "git.time",
					Note:    "Linux 内置实现（调用本机 git）",
				},
				{
					Match:   Match{OS: "darwin"},
					Builtin: "git.time",
					Note:    "macOS 内置实现（调用本机 git）",
				},
				{
					Match:   Match{OS: "windows"},
					Builtin: "git.time",
					Note:    "Windows 内置实现（调用本机 git）",
				},
			},
		},
		{
			ID:      "pkg.list",
			Group:   "pkg",
			Summary: "列出已安装的软件包",
			Action: "列出系统中已安装的软件包。" +
				"Linux: dpkg -l（Debian/Ubuntu）/ rpm -qa（CentOS/RHEL）/ apk list（Alpine）；" +
				"macOS: brew list；Windows: winget list",
			Variants: threePlatformBuiltins("pkg.list"),
		},
		{
			ID:      "pkg.refreshindex",
			Group:   "pkg",
			Summary: "刷新系统包管理器的软件源索引",
			Action:  "只更新元数据索引，不升级或安装任何软件包",
			Variants: []Variant{
				{
					Match: Match{OS: "linux", Distro: "centos"},
					Argv:  []string{"dnf", "makecache"},
					Note:  "CentOS 使用 dnf",
				},
				{
					Match: Match{OS: "linux", DistroLike: []string{"rhel", "fedora"}},
					Argv:  []string{"dnf", "makecache"},
					Note:  "RHEL/Fedora 系发行版使用 dnf",
				},
				{
					Match: Match{OS: "linux", DistroLike: []string{"debian", "ubuntu"}},
					Argv:  []string{"apt-get", "update"},
					Note:  "Debian/Ubuntu 系发行版使用 apt-get",
				},
				{
					Match: Match{OS: "linux", Distro: "alpine"},
					Argv:  []string{"apk", "update"},
					Note:  "Alpine 使用 apk",
				},
				{
					Match: Match{OS: "darwin"},
					Argv:  []string{"brew", "update"},
					Note:  "macOS 使用 Homebrew",
				},
				{
					Match: Match{OS: "windows"},
					Argv:  []string{"winget", "source", "update"},
					Note:  "Windows 使用 winget",
				},
			},
		},
		{
			ID:      "user.list",
			Group:   "user",
			Summary: "列出系统用户",
			Action: "列出系统中所有用户账户。" +
				"Linux/macOS: getent passwd 或 /etc/passwd；Windows: net user",
			Variants: threePlatformBuiltins("user.list"),
		},
		{
			ID:      "user.grant",
			Group:   "user",
			Summary: "授予用户 root/sudo 权限",
			Action: "将指定用户添加到系统的管理员组，使其获得 root/sudo 权限。" +
				"Linux: 添加到 wheel（CentOS/RHEL）或 sudo（Debian/Ubuntu）组；" +
				"macOS: 添加到 admin 组；Windows: 添加到 Administrators 组。" +
				"需要当前用户已有 sudo/管理员权限；执行后用户需重新登录才能生效",
			Usage:       "<用户名>",
			MinArgs:     1,
			RequiresYes: true,
			Variants: []Variant{
				{
					Match:   Match{OS: "linux"},
					Builtin: "user.grant",
					Note:    "Linux 内置实现（usermod -aG wheel/sudo）",
				},
				{
					Match:   Match{OS: "darwin"},
					Builtin: "user.grant",
					Note:    "macOS 内置实现（dseditgroup -a admin）",
				},
				{
					Match:   Match{OS: "windows"},
					Builtin: "user.grant",
					Note:    "Windows 内置实现（net localgroup Administrators /add）",
				},
			},
		},
		{
			ID:      "user.revoke",
			Group:   "user",
			Summary: "移除用户的 root/sudo 权限",
			Action: "将指定用户从系统的管理员组移除，取消其 root/sudo 权限。" +
				"Linux: 从 wheel（CentOS/RHEL）或 sudo（Debian/Ubuntu）组移除；" +
				"macOS: 从 admin 组移除；Windows: 从 Administrators 组移除。" +
				"需要当前用户已有 sudo/管理员权限；执行后用户需重新登录才能生效",
			Usage:       "<用户名>",
			MinArgs:     1,
			RequiresYes: true,
			Variants: []Variant{
				{
					Match:   Match{OS: "linux"},
					Builtin: "user.revoke",
					Note:    "Linux 内置实现（gpasswd -d wheel/sudo）",
				},
				{
					Match:   Match{OS: "darwin"},
					Builtin: "user.revoke",
					Note:    "macOS 内置实现（dseditgroup -d admin）",
				},
				{
					Match:   Match{OS: "windows"},
					Builtin: "user.revoke",
					Note:    "Windows 内置实现（net localgroup Administrators /delete）",
				},
			},
		},

		// ----- file -----
		{
			ID:      "file.findbig",
			Group:   "file",
			Summary: "查找大于指定阈值的文件（按大小排序）",
			Action: "递归扫描目录，找出所有大于阈值的常规文件，按大小从大到小排序输出。" +
				"默认阈值 100M，默认路径当前目录；--threshold 可指定大小（支持 K/M/G/T 后缀）",
			Usage:    "[路径] [--threshold 大小]",
			Variants: threePlatformBuiltins("file.findbig"),
		},
		{
			ID:      "file.findrecent",
			Group:   "file",
			Summary: "查找最近 N 天内修改过的文件",
			Action: "递归扫描目录，找出最近 N 天内修改过的常规文件，按修改时间从新到旧排序输出。" +
				"默认 7 天，默认路径当前目录；--days 可指定天数",
			Usage:    "[路径] [--days 天数]",
			Variants: threePlatformBuiltins("file.findrecent"),
		},
		{
			ID:      "file.findname",
			Group:   "file",
			Summary: "按文件名查找文件（忽略大小写）",
			Action: "递归扫描目录，按文件名模式匹配（支持通配符 * 和 ?，忽略大小写）。" +
				"默认路径当前目录",
			Usage:    "<模式> [路径]",
			MinArgs:  1,
			Variants: threePlatformBuiltins("file.findname"),
		},

		// ----- svc -----
		{
			ID:      "svc.list",
			Group:   "svc",
			Summary: "列出所有服务",
			Action: "列出系统中所有服务。" +
				"Linux: systemctl list-unit-files；macOS: launchctl list；" +
				"Windows: sc query state= all",
			Variants: threePlatformBuiltins("svc.list"),
		},
		{
			ID:      "svc.status",
			Group:   "svc",
			Summary: "查看服务状态与最近日志",
			Action: "一步查看服务运行状态和最近 20 行日志。" +
				"Linux: systemctl status + journalctl；macOS: launchctl list + log show；" +
				"Windows: sc query",
			Usage:    "<服务名>",
			MinArgs:  1,
			Variants: threePlatformBuiltins("svc.status"),
		},
		{
			ID:      "svc.enable",
			Group:   "svc",
			Summary: "启用并启动服务（开机自启 + 立即启动）",
			Action: "将服务设为开机自启并立即启动。" +
				"Linux: systemctl enable + start；macOS: launchctl load -w；" +
				"Windows: sc config start=auto + sc start",
			Usage:       "<服务名>",
			MinArgs:     1,
			RequiresYes: true,
			Variants:    threePlatformBuiltins("svc.enable"),
		},
		{
			ID:      "svc.disable",
			Group:   "svc",
			Summary: "禁用并停止服务",
			Action: "停止服务并取消开机自启。" +
				"Linux: systemctl stop + disable；macOS: launchctl unload -w；" +
				"Windows: sc stop + sc config start=disabled",
			Usage:       "<服务名>",
			MinArgs:     1,
			RequiresYes: true,
			Variants:    threePlatformBuiltins("svc.disable"),
		},
		{
			ID:      "svc.logs",
			Group:   "svc",
			Summary: "查看服务日志（支持跟踪）",
			Action: "查看指定服务的日志。-f 可实时跟踪新日志，-n 指定行数（默认 50）。" +
				"Linux: journalctl -u；macOS: log show；Windows: wevtutil",
			Usage:    "<服务名> [-f] [-n 行数]",
			MinArgs:  1,
			Variants: threePlatformBuiltins("svc.logs"),
		},

		// ----- archive -----
		{
			ID:      "archive.extract",
			Aliases: []string{"archive.unzip", "archive.untar"},
			Group:   "archive",
			Summary: "自动识别格式并解压压缩文件",
			Action: "根据文件扩展名自动选择解压方式：支持 .tar.gz / .tgz / .tar.bz2 / .tar.xz / .tar / .zip / .gz。" +
				"默认解压到当前目录，--dir 可指定目标目录",
			Usage:    "<文件> [--dir 目录]",
			MinArgs:  1,
			Variants: threePlatformBuiltins("archive.extract"),
		},

		// ----- sysinfo -----
		{
			ID:      "sysinfo",
			Group:   "sys",
			Summary: "显示系统信息一览（OS、运行时间、CPU、内存、磁盘）",
			Action: "一次性输出操作系统、运行时间、CPU 型号与核心数、内存总量与可用量、磁盘使用情况。" +
				"Linux 读取 /proc 与 cgroup，macOS 使用 sysctl，Windows 使用 wmic",
			Variants: threePlatformBuiltins("sysinfo"),
		},

		// ----- proc additions -----
		{
			ID:       "proc.list",
			Group:    "proc",
			Summary:  "列出所有进程",
			Action:   "列出系统中所有正在运行的进程。Linux/macOS: ps aux；Windows: tasklist",
			Variants: threePlatformBuiltins("proc.list"),
		},
		{
			ID:       "proc.topmem",
			Group:    "proc",
			Summary:  "按内存占用排序显示进程（默认前 10）",
			Action:   "列出内存占用最高的 N 个进程，按 RSS 降序排列。默认显示前 10，-n 可指定数量",
			Usage:    "[数量]",
			Variants: threePlatformBuiltins("proc.topmem"),
		},
		{
			ID:      "proc.topcpu",
			Group:   "proc",
			Summary: "按 CPU 占用排序显示进程（默认前 10）",
			Action: "列出 CPU 占用最高的 N 个进程，按 %CPU 降序排列。默认显示前 10，-n 可指定数量。" +
				"Windows 不支持（tasklist 不报告 CPU 使用率）",
			Usage:    "[数量]",
			Variants: threePlatformBuiltins("proc.topcpu"),
		},
		{
			ID:      "proc.killname",
			Group:   "proc",
			Summary: "按进程名终止进程",
			Action: "查找名称或命令行中包含指定关键字的所有进程并终止它们。" +
				"默认只列出匹配进程，需加 --yes 确认终止",
			Usage:       "<进程名>",
			MinArgs:     1,
			RequiresYes: true,
			Variants:    threePlatformBuiltins("proc.killname"),
		},

		// ----- net additions -----
		{
			ID:      "net.ping",
			Group:   "net",
			Summary: "测试与目标主机的连通性",
			Action: "向目标主机发送 ICMP 回显请求，检测网络连通性与延迟。" +
				"默认发送 4 个包，-c 可指定数量",
			Usage:    "<主机> [-c 次数]",
			MinArgs:  1,
			Variants: threePlatformBuiltins("net.ping"),
		},
		{
			ID:      "net.trace",
			Group:   "net",
			Summary: "路由追踪（显示数据包经过的每一跳）",
			Action: "追踪到达目标主机经过的每一跳路由。优先使用 mtr（最佳输出），" +
				"回退到 traceroute（Linux/macOS）或 tracert（Windows）",
			Usage:    "<主机>",
			MinArgs:  1,
			Variants: threePlatformBuiltins("net.trace"),
		},
		{
			ID:      "net.dns",
			Group:   "net",
			Summary: "DNS 查询（解析域名）",
			Action: "查询域名的 DNS 记录。优先使用 dig，回退到 nslookup 或 host。" +
				"-t 指定记录类型（A/AAAA/MX/CNAME 等），-s 指定 DNS 服务器",
			Usage:    "<域名> [-t 类型] [-s 服务器]",
			MinArgs:  1,
			Variants: threePlatformBuiltins("net.dns"),
		},
		{
			ID:      "net.curl",
			Group:   "net",
			Summary: "HTTP 诊断（状态码、耗时、重定向链）",
			Action: "发送 HTTP 请求并显示诊断信息：状态码、DNS 解析耗时、TCP 连接耗时、TLS 握手耗时、" +
				"首字节耗时、总耗时、下载大小与速度。-L 跟随重定向，-X 指定方法，-H 添加 header",
			Usage:    "<URL> [-L] [-X 方法] [-H header]",
			MinArgs:  1,
			Variants: threePlatformBuiltins("net.curl"),
		},

		// ----- log -----
		{
			ID:      "log.list",
			Group:   "log",
			Summary: "列出可用的日志来源",
			Action: "列出系统中可用的日志来源。" +
				"Linux: journalctl --list-boots；macOS: 列出日志目录；Windows: wevtutil el",
			Variants: threePlatformBuiltins("log.list"),
		},
		{
			ID:      "log.search",
			Group:   "log",
			Summary: "在系统日志中搜索关键字",
			Action: "在系统日志中搜索包含指定关键字的条目。" +
				"Linux: journalctl --grep；macOS: log show --predicate；Windows: wevtutil。" +
				"--since 限定时间范围（如 1h、30m），-u 限定服务，-n 限定行数",
			Usage:    "<关键字> [--since 时长] [-u 服务] [-n 行数]",
			MinArgs:  1,
			Variants: threePlatformBuiltins("log.search"),
		},
		{
			ID:      "log.last",
			Group:   "log",
			Summary: "查看最近的系统日志",
			Action: "显示最近的系统日志条目。默认 50 行，-n 可指定行数，-u 可限定服务。" +
				"Linux: journalctl；macOS: log show；Windows: wevtutil",
			Usage:    "[行数] [-u 服务]",
			Variants: threePlatformBuiltins("log.last"),
		},

		// ----- fw -----
		{
			ID:      "fw.status",
			Group:   "fw",
			Summary: "查看防火墙状态与规则",
			Action: "显示当前防火墙状态与规则列表。" +
				"Linux: 自动检测 ufw / firewalld / iptables；macOS: ALF（应用防火墙）；" +
				"Windows: netsh advfirewall",
			Variants: threePlatformBuiltins("fw.status"),
		},
		{
			ID:      "fw.allow",
			Group:   "fw",
			Summary: "开放防火墙端口",
			Action: "在防火墙中开放指定端口。支持 8080/tcp 或 8080/udp 形式，默认 tcp。" +
				"Linux: ufw allow / firewall-cmd --add-port；Windows: netsh advfirewall add rule",
			Usage:       "<端口>[/<协议>]",
			MinArgs:     1,
			RequiresYes: true,
			Variants:    threePlatformBuiltins("fw.allow"),
		},
		{
			ID:      "fw.deny",
			Group:   "fw",
			Summary: "关闭防火墙端口",
			Action: "在防火墙中关闭指定端口。支持 8080/tcp 或 8080/udp 形式，默认 tcp。" +
				"Linux: ufw deny / firewall-cmd --remove-port；Windows: netsh advfirewall delete rule",
			Usage:       "<端口>[/<协议>]",
			MinArgs:     1,
			RequiresYes: true,
			Variants:    threePlatformBuiltins("fw.deny"),
		},

		// ----- net additions -----
		{
			ID:      "net.iface",
			Group:   "net",
			Summary: "查看网卡信息和 IP 地址",
			Action: "列出所有非回环网卡的信息：名称、状态、MAC 地址、IPv4/IPv6 地址、MTU。" +
				"纯 Go 实现，跨平台一致",
			Variants: threePlatformBuiltins("net.iface"),
		},
		{
			ID:      "net.conn",
			Group:   "net",
			Summary: "查看 TCP 连接状态统计",
			Action: "统计各状态的 TCP 连接数量（ESTABLISHED、TIME_WAIT、CLOSE_WAIT 等）。" +
				"Linux: ss -s 或 netstat；macOS: netstat；Windows: netstat",
			Variants: threePlatformBuiltins("net.conn"),
		},
		{
			ID:      "net.speed",
			Group:   "net",
			Summary: "简单网速测试（下载/上传）",
			Action: "从 Cloudflare 下载测试文件测量下载速度，向 httpbin.org 上传数据测量上传速度。" +
				"默认测试 10MB，--size 可指定大小，--download 或 --upload 可单独测试",
			Usage:    "[--size 大小] [--download] [--upload]",
			Variants: threePlatformBuiltins("net.speed"),
		},

		// ----- proc additions -----
		{
			ID:      "proc.tree",
			Group:   "proc",
			Summary: "查看进程树（父子关系）",
			Action: "以树形结构显示进程的父子关系。" +
				"Linux/macOS: pstree -p 或 ps -ejH；Windows: tasklist /tree",
			Variants: threePlatformBuiltins("proc.tree"),
		},

		// ----- file additions -----
		{
			ID:      "file.empty",
			Group:   "file",
			Summary: "查找空文件和空目录",
			Action: "递归扫描目录，找出大小为 0 的文件和没有任何内容的目录。" +
				"--files-only 只查找空文件，--dirs-only 只查找空目录",
			Usage:    "[路径] [--files-only] [--dirs-only]",
			Variants: threePlatformBuiltins("file.empty"),
		},

		// ----- user additions -----
		{
			ID:      "user.info",
			Group:   "user",
			Summary: "查看用户详细信息",
			Action: "显示用户的 UID、GID、组、家目录、Shell 等信息。" +
				"不指定用户名时显示当前用户。" +
				"Linux/macOS: id + /etc/passwd + groups；Windows: net user",
			Usage:    "[用户名]",
			Variants: threePlatformBuiltins("user.info"),
		},

		// ----- pkg additions -----
		{
			ID:      "pkg.install",
			Group:   "pkg",
			Summary: "安装软件包",
			Action: "安装指定的软件包。" +
				"Linux: apt-get install（Debian/Ubuntu）/ dnf install（CentOS/RHEL）/ apk add（Alpine）；" +
				"macOS: brew install；Windows: winget install",
			Usage:       "<包名> [包名...]",
			MinArgs:     1,
			RequiresYes: true,
			Variants:    threePlatformBuiltins("pkg.install"),
		},
		{
			ID:      "pkg.remove",
			Group:   "pkg",
			Summary: "卸载软件包",
			Action: "卸载指定的软件包。" +
				"Linux: apt-get remove（Debian/Ubuntu）/ dnf remove（CentOS/RHEL）/ apk del（Alpine）；" +
				"macOS: brew uninstall；Windows: winget uninstall",
			Usage:       "<包名> [包名...]",
			MinArgs:     1,
			RequiresYes: true,
			Variants:    threePlatformBuiltins("pkg.remove"),
		},
		{
			ID:      "pkg.search",
			Group:   "pkg",
			Summary: "搜索软件包",
			Action: "在软件源中搜索匹配关键字的软件包。" +
				"Linux: apt-cache search（Debian/Ubuntu）/ dnf search（CentOS/RHEL）/ apk search（Alpine）；" +
				"macOS: brew search；Windows: winget search",
			Usage:    "<关键字>",
			MinArgs:  1,
			Variants: threePlatformBuiltins("pkg.search"),
		},
		{
			ID:      "pkg.info",
			Group:   "pkg",
			Summary: "查看软件包详情",
			Action: "显示指定软件包的详细信息（版本、描述、依赖等）。" +
				"Linux: apt show / dpkg -s（Debian/Ubuntu）/ rpm -qi（CentOS/RHEL）/ apk info（Alpine）；" +
				"macOS: brew info；Windows: winget show",
			Usage:    "<包名>",
			MinArgs:  1,
			Variants: threePlatformBuiltins("pkg.info"),
		},

		// ----- archive additions -----
		{
			ID:      "archive.create",
			Group:   "archive",
			Summary: "创建压缩文件",
			Action: "将文件或目录压缩为归档文件。支持 tar.gz / tgz / tar / zip 格式，默认 tar.gz。" +
				"--format 可指定格式，-o 可指定输出文件名",
			Usage:    "<路径> [--format 格式] [-o 输出文件]",
			MinArgs:  1,
			Variants: threePlatformBuiltins("archive.create"),
		},
		{
			ID:      "archive.list",
			Group:   "archive",
			Summary: "列出压缩文件内容（不解压）",
			Action: "查看压缩文件的内容列表，显示权限、大小、修改时间和文件名。" +
				"支持 tar.gz / tgz / tar / zip 格式",
			Usage:    "<文件>",
			MinArgs:  1,
			Variants: threePlatformBuiltins("archive.list"),
		},

		// ----- net additions (http) -----
		{
			ID:      "net.http",
			Group:   "net",
			Summary: "启动简单的 HTTP 文件服务器",
			Action: "在当前目录或指定目录启动 HTTP 文件服务器，方便局域网内共享文件。" +
				"默认端口 8000，-p 可指定端口；--upload 启用上传功能",
			Usage:    "[目录] [-p 端口] [--upload]",
			Variants: threePlatformBuiltins("net.http"),
		},

		// ----- disk additions -----
		{
			ID:      "disk.inode",
			Group:   "disk",
			Summary: "查看 inode 使用情况",
			Action: "显示文件系统的 inode 使用统计。inode 耗尽会导致无法创建新文件，即使磁盘空间充足。" +
				"Linux/macOS: df -i；Windows 不支持（NTFS 无 inode 概念）",
			Usage:    "[路径]",
			Variants: threePlatformBuiltins("disk.inode"),
		},
		{
			ID:      "disk.check",
			Group:   "disk",
			Summary: "一键磁盘健康检查",
			Action: "综合检查磁盘健康状况：空间使用率、inode 使用率、大文件 TOP 5、大目录 TOP 5，并给出清理建议。" +
				"一条命令完成磁盘排障",
			Usage:    "[路径] [--all]",
			Variants: threePlatformBuiltins("disk.check"),
		},
	}}
}

// threePlatformBuiltins builds the platform matrix for a command that kopsc
// implements itself on all three platforms.
func threePlatformBuiltins(name string) []Variant {
	return []Variant{
		{Match: Match{OS: "linux"}, Builtin: name, Note: "Linux 内置实现"},
		{Match: Match{OS: "darwin"}, Builtin: name, Note: "macOS 内置实现"},
		{Match: Match{OS: "windows"}, Builtin: name, Note: "Windows 内置实现"},
	}
}

// runtimeBuiltinVariants builds the platform matrix for a container command
// that kopsc implements itself. Builtin variants carry no argv; the builtin
// learns the runtime's program name from Variant.Runtime instead.
func runtimeBuiltinVariants(name string) []Variant {
	makeVariant := func(osName, note string) Variant {
		return Variant{
			Match:   Match{OS: osName, Runtime: "docker"},
			Builtin: "docker." + name,
			Runtime: "docker",
			Note:    note,
		}
	}
	return []Variant{
		makeVariant("linux", "Linux 上的 Docker 运行时，由 kopsc 内置实现"),
		makeVariant("darwin", "macOS 上的 Docker Desktop 运行时，由 kopsc 内置实现"),
		makeVariant("windows", "Windows 上的 Docker Desktop 运行时，由 kopsc 内置实现"),
	}
}

// runtimeVariants builds the platform matrix for a container command: docker
// on every platform (Docker Desktop on macOS and Windows).
func runtimeVariants(args ...string) []Variant {
	makeVariant := func(osName, note string) Variant {
		argv := make([]string, 0, 1+len(args))
		argv = append(argv, "docker")
		argv = append(argv, args...)
		return Variant{
			Match: Match{OS: osName, Runtime: "docker"},
			Argv:  argv,
			Note:  note,
		}
	}

	return []Variant{
		makeVariant("linux", "Linux 上的 Docker 运行时"),
		makeVariant("darwin", "macOS 上的 Docker Desktop 运行时"),
		makeVariant("windows", "Windows 上的 Docker Desktop 运行时"),
	}
}
