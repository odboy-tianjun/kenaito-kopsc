package executor

import (
	"context"
	"fmt"
	"os/exec"
	"runtime"
	"strings"
)

// runUserList implements `kopsc user list`: list all users.
func runUserList(ctx context.Context, req BuiltinRequest) error {
	switch runtime.GOOS {
	case "linux", "darwin":
		return userListUnix(ctx, req)
	case "windows":
		cmd := exec.CommandContext(ctx, "net", "user")
		cmd.Stdout = req.Stdout
		cmd.Stderr = req.Stderr
		return cmd.Run()
	default:
		return fmt.Errorf("不支持的操作系统: %s", runtime.GOOS)
	}
}

func userListUnix(ctx context.Context, req BuiltinRequest) error {
	// Try getent first (more portable), fall back to reading /etc/passwd
	if path, err := exec.LookPath("getent"); err == nil {
		cmd := exec.CommandContext(ctx, path, "passwd")
		cmd.Stdout = req.Stdout
		cmd.Stderr = req.Stderr
		return cmd.Run()
	}
	// Fall back to /etc/passwd
	out, err := exec.CommandContext(ctx, "cat", "/etc/passwd").Output()
	if err != nil {
		return fmt.Errorf("读取用户列表失败: %w", err)
	}
	fmt.Fprintln(req.Stdout, "用户名:UID:GID:描述:家目录:Shell")
	for _, line := range strings.Split(string(out), "\n") {
		if line == "" {
			continue
		}
		fields := strings.Split(line, ":")
		if len(fields) >= 7 {
			fmt.Fprintf(req.Stdout, "%s:%s:%s:%s:%s:%s\n",
				fields[0], fields[2], fields[3], fields[4], fields[5], fields[6])
		}
	}
	return nil
}
