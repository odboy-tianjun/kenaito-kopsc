package executor

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"os/user"
	"runtime"
	"strings"
)

// runUserInfo implements `kopsc user info`: show user details.
func runUserInfo(ctx context.Context, req BuiltinRequest) error {
	var username string
	if len(req.Args) == 0 {
		// Current user
		u, err := user.Current()
		if err != nil {
			return fmt.Errorf("获取当前用户失败: %w", err)
		}
		username = u.Username
	} else {
		username = req.Args[0]
	}

	switch runtime.GOOS {
	case "linux", "darwin":
		return userInfoUnix(ctx, username, req)
	case "windows":
		return userInfoWindows(ctx, username, req)
	default:
		return fmt.Errorf("不支持的操作系统: %s", runtime.GOOS)
	}
}

func userInfoUnix(ctx context.Context, username string, req BuiltinRequest) error {
	// Use id command for comprehensive info
	cmd := exec.CommandContext(ctx, "id", username)
	out, err := cmd.Output()
	if err != nil {
		return fmt.Errorf("用户 %q 不存在或 id 命令失败: %w", username, err)
	}
	fmt.Fprintf(req.Stdout, "用户: %s\n", strings.TrimSpace(string(out)))

	// Get home directory and shell from /etc/passwd
	if data, err := os.ReadFile("/etc/passwd"); err == nil {
		for _, line := range strings.Split(string(data), "\n") {
			fields := strings.Split(line, ":")
			if len(fields) >= 7 && fields[0] == username {
				fmt.Fprintf(req.Stdout, "UID:     %s\n", fields[2])
				fmt.Fprintf(req.Stdout, "GID:     %s\n", fields[3])
				fmt.Fprintf(req.Stdout, "描述:    %s\n", fields[4])
				fmt.Fprintf(req.Stdout, "家目录:  %s\n", fields[5])
				fmt.Fprintf(req.Stdout, "Shell:   %s\n", fields[6])
				break
			}
		}
	}

	// Groups
	groupsCmd := exec.CommandContext(ctx, "groups", username)
	if groupsOut, err := groupsCmd.Output(); err == nil {
		fmt.Fprintf(req.Stdout, "组:      %s\n", strings.TrimSpace(string(groupsOut)))
	}

	return nil
}

func userInfoWindows(ctx context.Context, username string, req BuiltinRequest) error {
	cmd := exec.CommandContext(ctx, "net", "user", username)
	out, err := cmd.Output()
	if err != nil {
		return fmt.Errorf("用户 %q 不存在或 net user 失败: %w", username, err)
	}
	fmt.Fprintln(req.Stdout, string(out))
	return nil
}
