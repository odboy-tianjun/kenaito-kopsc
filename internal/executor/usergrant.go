package executor

import (
	"context"
	"fmt"
	"os/exec"
	"runtime"
	"strings"
)

// runUserGrant implements `kopsc user grant <username>`: add a user to the
// sudo/admin group to grant root privileges.
//
// Linux: adds to 'wheel' (CentOS/RHEL) or 'sudo' (Debian/Ubuntu)
// macOS: adds to 'admin' group
// Windows: adds to 'Administrators' group
//
// This is a destructive operation requiring sudo/admin privileges.
func runUserGrant(ctx context.Context, req BuiltinRequest) error {
	if len(req.Args) == 0 {
		return fmt.Errorf("缺少用户名；用法: kopsc user grant <用户名>")
	}
	username := req.Args[0]

	if err := checkUserExists(username); err != nil {
		return err
	}

	switch runtime.GOOS {
	case "linux":
		return grantLinux(ctx, username, req)
	case "darwin":
		return grantDarwin(ctx, username, req)
	case "windows":
		return grantWindows(ctx, username, req)
	default:
		return fmt.Errorf("不支持的操作系统: %s", runtime.GOOS)
	}
}

// runUserRevoke implements `kopsc user revoke <username>`: remove a user from
// the sudo/admin group to revoke root privileges.
func runUserRevoke(ctx context.Context, req BuiltinRequest) error {
	if len(req.Args) == 0 {
		return fmt.Errorf("缺少用户名；用法: kopsc user revoke <用户名>")
	}
	username := req.Args[0]

	if err := checkUserExists(username); err != nil {
		return err
	}

	switch runtime.GOOS {
	case "linux":
		return revokeLinux(ctx, username, req)
	case "darwin":
		return revokeDarwin(ctx, username, req)
	case "windows":
		return revokeWindows(ctx, username, req)
	default:
		return fmt.Errorf("不支持的操作系统: %s", runtime.GOOS)
	}
}

// grantLinux adds user to wheel (CentOS/RHEL) or sudo (Debian/Ubuntu) group.
func grantLinux(ctx context.Context, username string, req BuiltinRequest) error {
	// Detect distro by checking which group exists
	if groupExists("wheel") {
		return runCommand(ctx, "usermod", []string{"-aG", "wheel", username}, req)
	}
	if groupExists("sudo") {
		return runCommand(ctx, "usermod", []string{"-aG", "sudo", username}, req)
	}
	return fmt.Errorf("未找到 wheel 或 sudo 组，请确认系统配置")
}

// revokeLinux removes user from wheel or sudo group.
func revokeLinux(ctx context.Context, username string, req BuiltinRequest) error {
	if groupExists("wheel") {
		return runCommand(ctx, "gpasswd", []string{"-d", username, "wheel"}, req)
	}
	if groupExists("sudo") {
		return runCommand(ctx, "gpasswd", []string{"-d", username, "sudo"}, req)
	}
	return fmt.Errorf("未找到 wheel 或 sudo 组，请确认系统配置")
}

// grantDarwin adds user to admin group on macOS.
func grantDarwin(ctx context.Context, username string, req BuiltinRequest) error {
	return runCommand(ctx, "dseditgroup", []string{"-o", "edit", "-t", "user", "-a", username, "admin"}, req)
}

// revokeDarwin removes user from admin group on macOS.
func revokeDarwin(ctx context.Context, username string, req BuiltinRequest) error {
	return runCommand(ctx, "dseditgroup", []string{"-o", "edit", "-t", "user", "-d", username, "admin"}, req)
}

// grantWindows adds user to Administrators group.
func grantWindows(ctx context.Context, username string, req BuiltinRequest) error {
	return runCommand(ctx, "net", []string{"localgroup", "Administrators", username, "/add"}, req)
}

// revokeWindows removes user from Administrators group.
func revokeWindows(ctx context.Context, username string, req BuiltinRequest) error {
	return runCommand(ctx, "net", []string{"localgroup", "Administrators", username, "/delete"}, req)
}

// runCommand executes a command and streams output to the request's stdout/stderr.
func runCommand(ctx context.Context, name string, args []string, req BuiltinRequest) error {
	cmd := exec.CommandContext(ctx, name, args...)
	cmd.Stdout = req.Stdout
	cmd.Stderr = req.Stderr
	cmd.Stdin = req.Stdin

	if err := cmd.Run(); err != nil {
		return fmt.Errorf("执行失败: %w", err)
	}
	return nil
}

// groupExists checks if a Unix group exists on the system.
func groupExists(name string) bool {
	out, err := exec.Command("getent", "group", name).Output()
	if err != nil {
		return false
	}
	return strings.TrimSpace(string(out)) != ""
}

// checkUserExists verifies that the given username exists on the system.
// Returns a user-friendly error if the user does not exist.
func checkUserExists(username string) error {
	var exists bool

	switch runtime.GOOS {
	case "linux", "darwin":
		// id command works on both Linux and macOS
		err := exec.Command("id", username).Run()
		exists = (err == nil)
	case "windows":
		// net user command on Windows
		err := exec.Command("net", "user", username).Run()
		exists = (err == nil)
	default:
		return fmt.Errorf("不支持的操作系统: %s", runtime.GOOS)
	}

	if !exists {
		return fmt.Errorf("用户 %q 不存在", username)
	}
	return nil
}
