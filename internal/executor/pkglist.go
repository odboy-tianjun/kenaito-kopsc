package executor

import (
	"context"
	"fmt"
	"os/exec"
	"runtime"
	"strings"

	"gitea.odboy.cn/odboy/kenaito-kopsc/internal/platform"
)

// runPkgList implements `kopsc pkg list`: list installed packages.
func runPkgList(ctx context.Context, req BuiltinRequest) error {
	switch runtime.GOOS {
	case "linux":
		return pkgListLinux(ctx, req)
	case "darwin":
		cmd := exec.CommandContext(ctx, "brew", "list")
		cmd.Stdout = req.Stdout
		cmd.Stderr = req.Stderr
		return cmd.Run()
	case "windows":
		cmd := exec.CommandContext(ctx, "winget", "list")
		cmd.Stdout = req.Stdout
		cmd.Stderr = req.Stderr
		return cmd.Run()
	default:
		return fmt.Errorf("不支持的操作系统: %s", runtime.GOOS)
	}
}

func pkgListLinux(ctx context.Context, req BuiltinRequest) error {
	info := platform.System().Detect()

	// Debian/Ubuntu: dpkg -l
	if distroLikeContains(info, "debian", "ubuntu") {
		cmd := exec.CommandContext(ctx, "dpkg", "-l")
		cmd.Stdout = req.Stdout
		cmd.Stderr = req.Stderr
		return cmd.Run()
	}

	// CentOS/RHEL/Fedora: rpm -qa
	if distroLikeContains(info, "rhel", "fedora") || info.Distro == "centos" {
		cmd := exec.CommandContext(ctx, "rpm", "-qa", "--qf", "%{NAME}-%{VERSION}-%{RELEASE}.%{ARCH}\n")
		cmd.Stdout = req.Stdout
		cmd.Stderr = req.Stderr
		return cmd.Run()
	}

	// Alpine: apk list --installed
	if info.Distro == "alpine" {
		cmd := exec.CommandContext(ctx, "apk", "list", "--installed")
		cmd.Stdout = req.Stdout
		cmd.Stderr = req.Stderr
		return cmd.Run()
	}

	return fmt.Errorf("不支持的 Linux 发行版: %s", info.Distro)
}

func distroLikeContains(info platform.Info, values ...string) bool {
	all := make([]string, 0, 1+len(info.DistroLike))
	if info.Distro != "" {
		all = append(all, info.Distro)
	}
	all = append(all, info.DistroLike...)
	for _, a := range all {
		for _, v := range values {
			if strings.EqualFold(a, v) {
				return true
			}
		}
	}
	return false
}
