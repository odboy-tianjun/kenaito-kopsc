package executor

import (
	"bytes"
	"context"
	"fmt"
	"os/exec"
	"runtime"
	"strconv"
	"strings"
)

// runSvcList implements `kopsc svc list`: list all services.
func runSvcList(ctx context.Context, req BuiltinRequest) error {
	switch runtime.GOOS {
	case "linux":
		cmd := exec.CommandContext(ctx, "systemctl", "list-unit-files", "--type=service", "--no-pager")
		cmd.Stdout = req.Stdout
		cmd.Stderr = req.Stderr
		return cmd.Run()
	case "darwin":
		cmd := exec.CommandContext(ctx, "launchctl", "list")
		cmd.Stdout = req.Stdout
		cmd.Stderr = req.Stderr
		return cmd.Run()
	case "windows":
		cmd := exec.CommandContext(ctx, "sc", "query", "state=", "all")
		cmd.Stdout = req.Stdout
		cmd.Stderr = req.Stderr
		return cmd.Run()
	default:
		return fmt.Errorf("不支持的操作系统: %s", runtime.GOOS)
	}
}

// runSvcStatus implements `kopsc svc status <service>`: show service status
// and recent log lines in one step.
func runSvcStatus(ctx context.Context, req BuiltinRequest) error {
	if len(req.Args) == 0 {
		return fmt.Errorf("缺少服务名；用法: kopsc svc status <服务名>")
	}
	name := req.Args[0]

	switch runtime.GOOS {
	case "linux":
		return svcStatusLinux(ctx, name, req)
	case "darwin":
		return svcStatusDarwin(ctx, name, req)
	case "windows":
		return svcStatusWindows(ctx, name, req)
	default:
		return fmt.Errorf("不支持的操作系统: %s", runtime.GOOS)
	}
}

func svcStatusLinux(ctx context.Context, name string, req BuiltinRequest) error {
	w := req.Stdout
	fmt.Fprintf(w, "=== 服务状态: %s ===\n\n", name)

	cmd := exec.CommandContext(ctx, "systemctl", "status", name, "--no-pager")
	cmd.Stdout = w
	cmd.Stderr = req.Stderr
	_ = cmd.Run()

	fmt.Fprintf(w, "\n=== 最近日志 ===\n\n")
	logCmd := exec.CommandContext(ctx, "journalctl", "-u", name, "-n", "20", "--no-pager")
	logCmd.Stdout = w
	logCmd.Stderr = req.Stderr
	_ = logCmd.Run()
	return nil
}

func svcStatusDarwin(ctx context.Context, name string, req BuiltinRequest) error {
	w := req.Stdout
	fmt.Fprintf(w, "=== 服务状态: %s ===\n\n", name)

	cmd := exec.CommandContext(ctx, "launchctl", "list", name)
	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr
	_ = cmd.Run()

	if stdout.Len() > 0 {
		fmt.Fprintln(w, stdout.String())
	} else {
		fmt.Fprintf(w, "服务 %q 未在 launchctl 中注册\n", name)
	}
	if stderr.Len() > 0 {
		fmt.Fprintln(req.Stderr, stderr.String())
	}

	fmt.Fprintf(w, "\n=== 日志（最近 20 行）===\n\n")
	logCmd := exec.CommandContext(ctx, "log", "show", "--predicate",
		fmt.Sprintf(`subsystem == "%s" OR process == "%s"`, name, name),
		"--last", "5m", "--style", "compact")
	logCmd.Stdout = w
	logCmd.Stderr = req.Stderr
	_ = logCmd.Run()
	return nil
}

func svcStatusWindows(ctx context.Context, name string, req BuiltinRequest) error {
	w := req.Stdout
	fmt.Fprintf(w, "=== 服务状态: %s ===\n\n", name)

	cmd := exec.CommandContext(ctx, "sc", "query", name)
	cmd.Stdout = w
	cmd.Stderr = req.Stderr
	_ = cmd.Run()
	return nil
}

// runSvcEnable implements `kopsc svc enable <service>`: enable + start service.
func runSvcEnable(ctx context.Context, req BuiltinRequest) error {
	if len(req.Args) == 0 {
		return fmt.Errorf("缺少服务名；用法: kopsc svc enable <服务名>")
	}
	name := req.Args[0]

	switch runtime.GOOS {
	case "linux":
		fmt.Fprintf(req.Stdout, "启用并启动服务: %s\n", name)
		if err := runCommand(ctx, "systemctl", []string{"enable", name}, req); err != nil {
			return fmt.Errorf("启用服务失败: %w", err)
		}
		return runCommand(ctx, "systemctl", []string{"start", name}, req)
	case "darwin":
		fmt.Fprintf(req.Stdout, "加载并启动服务: %s\n", name)
		return runCommand(ctx, "launchctl", []string{"load", "-w", "/Library/LaunchDaemons/" + name + ".plist"}, req)
	case "windows":
		fmt.Fprintf(req.Stdout, "设置自启并启动服务: %s\n", name)
		if err := runCommand(ctx, "sc", []string{"config", name, "start=auto"}, req); err != nil {
			return err
		}
		return runCommand(ctx, "sc", []string{"start", name}, req)
	default:
		return fmt.Errorf("不支持的操作系统: %s", runtime.GOOS)
	}
}

// runSvcDisable implements `kopsc svc disable <service>`: disable + stop service.
func runSvcDisable(ctx context.Context, req BuiltinRequest) error {
	if len(req.Args) == 0 {
		return fmt.Errorf("缺少服务名；用法: kopsc svc disable <服务名>")
	}
	name := req.Args[0]

	switch runtime.GOOS {
	case "linux":
		fmt.Fprintf(req.Stdout, "停止并禁用服务: %s\n", name)
		if err := runCommand(ctx, "systemctl", []string{"stop", name}, req); err != nil {
			return fmt.Errorf("停止服务失败: %w", err)
		}
		return runCommand(ctx, "systemctl", []string{"disable", name}, req)
	case "darwin":
		fmt.Fprintf(req.Stdout, "卸载服务: %s\n", name)
		return runCommand(ctx, "launchctl", []string{"unload", "-w", "/Library/LaunchDaemons/" + name + ".plist"}, req)
	case "windows":
		fmt.Fprintf(req.Stdout, "停止并禁用服务: %s\n", name)
		if err := runCommand(ctx, "sc", []string{"stop", name}, req); err != nil {
			return err
		}
		return runCommand(ctx, "sc", []string{"config", name, "start=disabled"}, req)
	default:
		return fmt.Errorf("不支持的操作系统: %s", runtime.GOOS)
	}
}

// runSvcLogs implements `kopsc svc logs <service>`: follow service logs.
func runSvcLogs(ctx context.Context, req BuiltinRequest) error {
	opts, err := parseSvcLogsArgs(req.Args)
	if err != nil {
		return err
	}

	switch runtime.GOOS {
	case "linux":
		return svcLogsLinux(ctx, opts, req)
	case "darwin":
		return svcLogsDarwin(ctx, opts, req)
	case "windows":
		return svcLogsWindows(ctx, opts, req)
	default:
		return fmt.Errorf("不支持的操作系统: %s", runtime.GOOS)
	}
}

type svcLogsOptions struct {
	service string
	follow  bool
	lines   int
}

func parseSvcLogsArgs(args []string) (svcLogsOptions, error) {
	opts := svcLogsOptions{lines: 50}
	var positional []string

	for i := 0; i < len(args); i++ {
		arg := args[i]
		switch {
		case arg == "--":
			positional = append(positional, args[i+1:]...)
			i = len(args)
		case arg == "-f" || arg == "--follow":
			opts.follow = true
		case arg == "--lines" || arg == "-n":
			if i+1 >= len(args) {
				return opts, fmt.Errorf("%s 缺少行数", arg)
			}
			i++
			n, err := strconv.Atoi(args[i])
			if err != nil || n < 1 {
				return opts, fmt.Errorf("--lines 需要正整数，收到 %q", args[i])
			}
			opts.lines = n
		case strings.HasPrefix(arg, "--lines="):
			n, err := strconv.Atoi(strings.TrimPrefix(arg, "--lines="))
			if err != nil || n < 1 {
				return opts, fmt.Errorf("--lines 需要正整数")
			}
			opts.lines = n
		case strings.HasPrefix(arg, "-") && arg != "-":
			return opts, fmt.Errorf("未知选项 %q", arg)
		default:
			positional = append(positional, arg)
		}
	}
	if len(positional) == 0 {
		return opts, fmt.Errorf("缺少服务名；用法: kopsc svc logs <服务名>")
	}
	opts.service = positional[0]
	return opts, nil
}

func svcLogsLinux(ctx context.Context, opts svcLogsOptions, req BuiltinRequest) error {
	args := []string{"-u", opts.service, "--no-pager", "-n", strconv.Itoa(opts.lines)}
	if opts.follow {
		args = append(args, "-f")
	}
	cmd := exec.CommandContext(ctx, "journalctl", args...)
	cmd.Stdout = req.Stdout
	cmd.Stderr = req.Stderr
	cmd.Stdin = req.Stdin
	return cmd.Run()
}

func svcLogsDarwin(ctx context.Context, opts svcLogsOptions, req BuiltinRequest) error {
	predicate := fmt.Sprintf(`subsystem == "%s" OR process == "%s"`, opts.service, opts.service)
	args := []string{"show", "--predicate", predicate, "--style", "compact", "--last", "5m"}
	cmd := exec.CommandContext(ctx, "log", args...)
	cmd.Stdout = req.Stdout
	cmd.Stderr = req.Stderr
	return cmd.Run()
}

func svcLogsWindows(ctx context.Context, opts svcLogsOptions, req BuiltinRequest) error {
	cmd := exec.CommandContext(ctx, "wevtutil", "qe", "System",
		"/q:*[System[Provider[@Name='"+opts.service+"']]]",
		"/c:"+strconv.Itoa(opts.lines), "/rd:true", "/f:text")
	cmd.Stdout = req.Stdout
	cmd.Stderr = req.Stderr
	return cmd.Run()
}
