package registry

// Builtin returns the registry compiled into kopsc. Every variant is explicit
// about the platform it targets; resolution picks the most specific match.
func Builtin() *Registry {
	return &Registry{Commands: []Command{
		{
			ID:      "docker.clearsafe",
			Aliases: []string{"docker.prunesafe"},
			Group:   "docker",
			Summary: "安全清理未使用的容器、网络与构建缓存",
			Action: "执行容器运行时的默认清理作用域：不清理全部未使用镜像（-a），" +
				"不删除卷（--volumes），仅移除停止的容器、未使用的网络和悬空镜像",
			Variants: []Variant{
				{
					Match: Match{OS: "linux", Distro: "centos", Runtime: "podman"},
					Argv:  []string{"podman", "system", "prune", "-f"},
					Note:  "CentOS 上的 Podman 运行时",
				},
				{
					Match: Match{OS: "linux", Distro: "centos", Runtime: "docker"},
					Argv:  []string{"docker", "system", "prune", "-f"},
					Note:  "CentOS 上的 Docker 运行时",
				},
				{
					Match: Match{OS: "linux", Runtime: "podman"},
					Argv:  []string{"podman", "system", "prune", "-f"},
					Note:  "任意 Linux 发行版上的 Podman 运行时",
				},
				{
					Match: Match{OS: "linux", Runtime: "docker"},
					Argv:  []string{"docker", "system", "prune", "-f"},
					Note:  "任意 Linux 发行版上的 Docker 运行时",
				},
				{
					Match: Match{OS: "darwin", Runtime: "docker"},
					Argv:  []string{"docker", "system", "prune", "-f"},
					Note:  "macOS 上的 Docker Desktop 运行时",
				},
				{
					Match: Match{OS: "windows", Runtime: "docker"},
					Argv:  []string{"docker", "system", "prune", "-f"},
					Note:  "Windows 上的 Docker Desktop 运行时",
				},
			},
		},
		{
			ID:      "net.ports",
			Group:   "net",
			Summary: "列出本机正在监听的 TCP/UDP 端口",
			Action:  "以只读方式列出监听中的套接字及持有进程",
			Variants: []Variant{
				{
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
