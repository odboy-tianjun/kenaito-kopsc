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
			ID:       "docker.stats",
			Group:    "docker",
			Summary:  "一次性查看容器 CPU/内存/IO 占用",
			Action:   "只读：等价 docker stats --no-stream，输出一次后退出",
			Variants: runtimeVariants("stats", "--no-stream"),
		},
		{
			ID:       "docker.df",
			Group:    "docker",
			Summary:  "查看镜像/容器/卷/构建缓存的空间占用",
			Action:   "只读：等价 docker system df",
			Variants: runtimeVariants("system", "df"),
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
			ID:       "docker.logs",
			Group:    "docker",
			Summary:  "查看容器日志",
			Action:   "只读：等价 docker logs <容器>；-f、--tail 等选项可透传",
			Usage:    "<容器>",
			MinArgs:  1,
			Variants: runtimeVariants("logs"),
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
			ID:       "docker.stop",
			Group:    "docker",
			Summary:  "停止运行中的容器",
			Action:   "向容器发送 SIGTERM，超时后由运行时强制结束",
			Usage:    "<容器>",
			MinArgs:  1,
			Variants: runtimeVariants("stop"),
		},
		{
			ID:       "docker.restart",
			Group:    "docker",
			Summary:  "重启容器",
			Action:   "停止并重新启动一个或多个容器",
			Usage:    "<容器>",
			MinArgs:  1,
			Variants: runtimeVariants("restart"),
		},
		{
			ID:       "docker.rm",
			Group:    "docker",
			Summary:  "删除已停止的容器",
			Action:   "删除指定容器；运行中的容器需要先停止（不会自动加 -f）",
			Usage:    "<容器>",
			MinArgs:  1,
			Variants: runtimeVariants("rm"),
		},
		{
			ID:       "docker.rmi",
			Group:    "docker",
			Summary:  "删除本地镜像",
			Action:   "删除指定镜像；被容器引用的镜像会被运行时拒绝删除",
			Usage:    "<镜像>",
			MinArgs:  1,
			Variants: runtimeVariants("rmi"),
		},
		{
			ID:       "docker.exec",
			Group:    "docker",
			Summary:  "在运行中的容器内执行命令",
			Action:   "等价 docker exec <容器> <命令...>；交互场景可透传 -it",
			Usage:    "<容器> <命令...>",
			MinArgs:  1,
			Variants: runtimeVariants("exec"),
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
			ID:       "docker.save",
			Group:    "docker",
			Summary:  "把镜像导出为归档文件",
			Action:   "等价 docker save；通常配合 -o 透传输出文件",
			Usage:    "<镜像>",
			MinArgs:  1,
			Variants: runtimeVariants("save"),
		},
		{
			ID:       "docker.load",
			Group:    "docker",
			Summary:  "从归档文件导入镜像",
			Action:   "等价 docker load；通常配合 -i 透传输入文件",
			Variants: runtimeVariants("load"),
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
			Action:     "自动识别 Docker/Podman 容器或 Kubernetes Pod，采集状态、最近日志与事件，给出原因、依据与建议；只读，不修改任何对象。--kind 可强制来源，K8s 下 -n/--namespace 指定命名空间",
			Usage:      "<容器名或 Pod 名>",
			MinArgs:    1,
			Namespaced: true,
			Variants: []Variant{
				{
					Match:   Match{OS: "linux"},
					Builtin: "diagnose",
					Note:    "Linux 内置诊断（docker/podman 与 kubectl 自动识别）",
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
			Summary: "列出目录下各直接子项的占用空间，按大小降序",
			Action: "等价于 du -x --max-depth=1 -h <路径> | sort -hr：统计每个直接子项（文件或目录递归）的占用，" +
				"不跨越文件系统边界，末尾输出该路径的合计。路径必填，可作位置参数或用 --path 指定；" +
				"由于 Windows 没有等价的 du/sort，这里使用 kopsc 内置实现以保证三平台输出一致",
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
			Summary: "列出本机正在监听的 TCP/UDP 端口",
			Action:  "以只读方式列出监听中的套接字及持有进程",
			Variants: []Variant{{
				Match: Match{OS: "linux"},
				Argv:  []string{"ss", "-tulnp"},
				Note:  "Linux 使用 iproute2 的 ss",
			},
				{
					Match: Match{OS: "darwin"},
					Argv:  []string{"lsof", "-iTCP", "-sTCP:LISTEN", "-nP"},
					Note:  "macOS 使用 lsof",
				},
				{
					Match: Match{OS: "windows"},
					Argv:  []string{"netstat", "-ano"},
					Note:  "Windows 使用 netstat",
				},
			},
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
	}}
}

// runtimeVariants builds the platform matrix for a container command. Linux
// gets both podman and docker variants (resolution prefers podman when both
// exist); macOS and Windows only have Docker Desktop.
func runtimeVariants(args ...string) []Variant {
	program := func(runtime string) string {
		if runtime == "podman" {
			return "podman"
		}
		return "docker"
	}

	makeVariant := func(osName, runtime, note string) Variant {
		argv := make([]string, 0, 1+len(args))
		argv = append(argv, program(runtime))
		argv = append(argv, args...)
		return Variant{
			Match: Match{OS: osName, Runtime: runtime},
			Argv:  argv,
			Note:  note,
		}
	}

	return []Variant{
		makeVariant("linux", "podman", "Linux 上的 Podman 运行时"),
		makeVariant("linux", "docker", "Linux 上的 Docker 运行时"),
		makeVariant("darwin", "docker", "macOS 上的 Docker Desktop 运行时"),
		makeVariant("windows", "docker", "Windows 上的 Docker Desktop 运行时"),
	}
}
