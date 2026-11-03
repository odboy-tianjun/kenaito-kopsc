package executor

import (
	"context"
	"fmt"
	"os/exec"
	"runtime"
	"strings"

	"gitea.odboy.cn/odboy/kenaito-kopsc/internal/platform"
)

// runPkgInstall implements `kopsc pkg install <package>`: install a package.
func runPkgInstall(ctx context.Context, req BuiltinRequest) error {
	opts, err := parsePkgManageArgs(req.Args, "install")
	if err != nil {
		return err
	}
	if len(opts.packages) == 0 {
		return fmt.Errorf("缺少包名；用法: kopsc pkg install <包名> [包名...]")
	}

	switch runtime.GOOS {
	case "linux":
		return pkgInstallLinux(ctx, req, opts.packages)
	case "darwin":
		args := append([]string{"install"}, opts.packages...)
		cmd := exec.CommandContext(ctx, "brew", args...)
		cmd.Stdout = req.Stdout
		cmd.Stderr = req.Stderr
		return cmd.Run()
	case "windows":
		args := append([]string{"install"}, opts.packages...)
		cmd := exec.CommandContext(ctx, "winget", args...)
		cmd.Stdout = req.Stdout
		cmd.Stderr = req.Stderr
		return cmd.Run()
	default:
		return fmt.Errorf("不支持的操作系统: %s", runtime.GOOS)
	}
}

func pkgInstallLinux(ctx context.Context, req BuiltinRequest, packages []string) error {
	info := platform.System().Detect()

	// Debian/Ubuntu: apt-get install
	if distroLikeContains(info, "debian", "ubuntu") {
		args := append([]string{"install"}, packages...)
		cmd := exec.CommandContext(ctx, "apt-get", args...)
		cmd.Stdout = req.Stdout
		cmd.Stderr = req.Stderr
		return cmd.Run()
	}

	// CentOS/RHEL/Fedora: dnf install
	if distroLikeContains(info, "rhel", "fedora") || info.Distro == "centos" {
		args := append([]string{"install"}, packages...)
		cmd := exec.CommandContext(ctx, "dnf", args...)
		cmd.Stdout = req.Stdout
		cmd.Stderr = req.Stderr
		return cmd.Run()
	}

	// Alpine: apk add
	if info.Distro == "alpine" {
		args := append([]string{"add"}, packages...)
		cmd := exec.CommandContext(ctx, "apk", args...)
		cmd.Stdout = req.Stdout
		cmd.Stderr = req.Stderr
		return cmd.Run()
	}

	return fmt.Errorf("不支持的 Linux 发行版: %s", info.Distro)
}

// runPkgRemove implements `kopsc pkg remove <package>`: remove a package.
func runPkgRemove(ctx context.Context, req BuiltinRequest) error {
	opts, err := parsePkgManageArgs(req.Args, "remove")
	if err != nil {
		return err
	}
	if len(opts.packages) == 0 {
		return fmt.Errorf("缺少包名；用法: kopsc pkg remove <包名> [包名...]")
	}

	switch runtime.GOOS {
	case "linux":
		return pkgRemoveLinux(ctx, req, opts.packages)
	case "darwin":
		args := append([]string{"uninstall"}, opts.packages...)
		cmd := exec.CommandContext(ctx, "brew", args...)
		cmd.Stdout = req.Stdout
		cmd.Stderr = req.Stderr
		return cmd.Run()
	case "windows":
		args := append([]string{"uninstall"}, opts.packages...)
		cmd := exec.CommandContext(ctx, "winget", args...)
		cmd.Stdout = req.Stdout
		cmd.Stderr = req.Stderr
		return cmd.Run()
	default:
		return fmt.Errorf("不支持的操作系统: %s", runtime.GOOS)
	}
}

func pkgRemoveLinux(ctx context.Context, req BuiltinRequest, packages []string) error {
	info := platform.System().Detect()

	// Debian/Ubuntu: apt-get remove
	if distroLikeContains(info, "debian", "ubuntu") {
		args := append([]string{"remove"}, packages...)
		cmd := exec.CommandContext(ctx, "apt-get", args...)
		cmd.Stdout = req.Stdout
		cmd.Stderr = req.Stderr
		return cmd.Run()
	}

	// CentOS/RHEL/Fedora: dnf remove
	if distroLikeContains(info, "rhel", "fedora") || info.Distro == "centos" {
		args := append([]string{"remove"}, packages...)
		cmd := exec.CommandContext(ctx, "dnf", args...)
		cmd.Stdout = req.Stdout
		cmd.Stderr = req.Stderr
		return cmd.Run()
	}

	// Alpine: apk del
	if info.Distro == "alpine" {
		args := append([]string{"del"}, packages...)
		cmd := exec.CommandContext(ctx, "apk", args...)
		cmd.Stdout = req.Stdout
		cmd.Stderr = req.Stderr
		return cmd.Run()
	}

	return fmt.Errorf("不支持的 Linux 发行版: %s", info.Distro)
}

// runPkgSearch implements `kopsc pkg search <keyword>`: search for packages.
func runPkgSearch(ctx context.Context, req BuiltinRequest) error {
	opts, err := parsePkgManageArgs(req.Args, "search")
	if err != nil {
		return err
	}
	if len(opts.packages) == 0 {
		return fmt.Errorf("缺少搜索关键字；用法: kopsc pkg search <关键字>")
	}
	keyword := opts.packages[0]

	switch runtime.GOOS {
	case "linux":
		return pkgSearchLinux(ctx, req, keyword)
	case "darwin":
		cmd := exec.CommandContext(ctx, "brew", "search", keyword)
		cmd.Stdout = req.Stdout
		cmd.Stderr = req.Stderr
		return cmd.Run()
	case "windows":
		cmd := exec.CommandContext(ctx, "winget", "search", keyword)
		cmd.Stdout = req.Stdout
		cmd.Stderr = req.Stderr
		return cmd.Run()
	default:
		return fmt.Errorf("不支持的操作系统: %s", runtime.GOOS)
	}
}

func pkgSearchLinux(ctx context.Context, req BuiltinRequest, keyword string) error {
	info := platform.System().Detect()

	// Debian/Ubuntu: apt-cache search
	if distroLikeContains(info, "debian", "ubuntu") {
		cmd := exec.CommandContext(ctx, "apt-cache", "search", keyword)
		cmd.Stdout = req.Stdout
		cmd.Stderr = req.Stderr
		return cmd.Run()
	}

	// CentOS/RHEL/Fedora: dnf search
	if distroLikeContains(info, "rhel", "fedora") || info.Distro == "centos" {
		cmd := exec.CommandContext(ctx, "dnf", "search", keyword)
		cmd.Stdout = req.Stdout
		cmd.Stderr = req.Stderr
		return cmd.Run()
	}

	// Alpine: apk search
	if info.Distro == "alpine" {
		cmd := exec.CommandContext(ctx, "apk", "search", keyword)
		cmd.Stdout = req.Stdout
		cmd.Stderr = req.Stderr
		return cmd.Run()
	}

	return fmt.Errorf("不支持的 Linux 发行版: %s", info.Distro)
}

// runPkgInfo implements `kopsc pkg info <package>`: show package details.
func runPkgInfo(ctx context.Context, req BuiltinRequest) error {
	opts, err := parsePkgManageArgs(req.Args, "info")
	if err != nil {
		return err
	}
	if len(opts.packages) == 0 {
		return fmt.Errorf("缺少包名；用法: kopsc pkg info <包名>")
	}
	pkg := opts.packages[0]

	switch runtime.GOOS {
	case "linux":
		return pkgInfoLinux(ctx, req, pkg)
	case "darwin":
		cmd := exec.CommandContext(ctx, "brew", "info", pkg)
		cmd.Stdout = req.Stdout
		cmd.Stderr = req.Stderr
		return cmd.Run()
	case "windows":
		cmd := exec.CommandContext(ctx, "winget", "show", pkg)
		cmd.Stdout = req.Stdout
		cmd.Stderr = req.Stderr
		return cmd.Run()
	default:
		return fmt.Errorf("不支持的操作系统: %s", runtime.GOOS)
	}
}

func pkgInfoLinux(ctx context.Context, req BuiltinRequest, pkg string) error {
	info := platform.System().Detect()

	// Debian/Ubuntu: apt show (fallback to dpkg -s)
	if distroLikeContains(info, "debian", "ubuntu") {
		cmd := exec.CommandContext(ctx, "apt", "show", pkg)
		cmd.Stdout = req.Stdout
		cmd.Stderr = req.Stderr
		if err := cmd.Run(); err != nil {
			// Fallback to dpkg -s
			cmd = exec.CommandContext(ctx, "dpkg", "-s", pkg)
			cmd.Stdout = req.Stdout
			cmd.Stderr = req.Stderr
			return cmd.Run()
		}
		return nil
	}

	// CentOS/RHEL/Fedora: rpm -qi
	if distroLikeContains(info, "rhel", "fedora") || info.Distro == "centos" {
		cmd := exec.CommandContext(ctx, "rpm", "-qi", pkg)
		cmd.Stdout = req.Stdout
		cmd.Stderr = req.Stderr
		return cmd.Run()
	}

	// Alpine: apk info
	if info.Distro == "alpine" {
		cmd := exec.CommandContext(ctx, "apk", "info", "-a", pkg)
		cmd.Stdout = req.Stdout
		cmd.Stderr = req.Stderr
		return cmd.Run()
	}

	return fmt.Errorf("不支持的 Linux 发行版: %s", info.Distro)
}

type pkgManageOptions struct {
	packages []string
}

func parsePkgManageArgs(args []string, action string) (pkgManageOptions, error) {
	var opts pkgManageOptions
	var positional []string

	for i := 0; i < len(args); i++ {
		arg := args[i]
		switch {
		case arg == "--":
			positional = append(positional, args[i+1:]...)
			i = len(args)
		case strings.HasPrefix(arg, "-") && arg != "-":
			// Ignore unknown flags, pass through
			positional = append(positional, arg)
		default:
			positional = append(positional, arg)
		}
	}

	opts.packages = positional
	return opts, nil
}
