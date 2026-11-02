package executor

import (
	"context"
	"fmt"
	"os/exec"
	"runtime"
	"strings"
)

// runFwStatus implements `kopsc fw status`: show current firewall rules.
func runFwStatus(ctx context.Context, req BuiltinRequest) error {
	switch runtime.GOOS {
	case "linux":
		return fwStatusLinux(ctx, req)
	case "darwin":
		return fwStatusDarwin(ctx, req)
	case "windows":
		return fwStatusWindows(ctx, req)
	default:
		return fmt.Errorf("不支持的操作系统: %s", runtime.GOOS)
	}
}

func fwStatusLinux(ctx context.Context, req BuiltinRequest) error {
	// Try ufw first, then firewalld, then iptables.
	if path, err := exec.LookPath("ufw"); err == nil {
		cmd := exec.CommandContext(ctx, path, "status", "verbose")
		cmd.Stdout = req.Stdout
		cmd.Stderr = req.Stderr
		return cmd.Run()
	}
	if path, err := exec.LookPath("firewall-cmd"); err == nil {
		fmt.Fprintln(req.Stdout, "=== firewalld 状态 ===")
		cmd := exec.CommandContext(ctx, path, "--state")
		cmd.Stdout = req.Stdout
		cmd.Stderr = req.Stderr
		_ = cmd.Run()

		fmt.Fprintln(req.Stdout, "\n=== 已开放端口 ===")
		listCmd := exec.CommandContext(ctx, path, "--list-ports")
		listCmd.Stdout = req.Stdout
		listCmd.Stderr = req.Stderr
		_ = listCmd.Run()

		fmt.Fprintln(req.Stdout, "\n=== 已允许服务 ===")
		svcCmd := exec.CommandContext(ctx, path, "--list-services")
		svcCmd.Stdout = req.Stdout
		svcCmd.Stderr = req.Stderr
		return svcCmd.Run()
	}
	if path, err := exec.LookPath("iptables"); err == nil {
		cmd := exec.CommandContext(ctx, path, "-L", "-n", "--line-numbers")
		cmd.Stdout = req.Stdout
		cmd.Stderr = req.Stderr
		return cmd.Run()
	}
	return fmt.Errorf("未找到 ufw、firewall-cmd 或 iptables")
}

func fwStatusDarwin(ctx context.Context, req BuiltinRequest) error {
	fmt.Fprintln(req.Stdout, "macOS 使用内置应用防火墙（ALF）")
	cmd := exec.CommandContext(ctx, "/usr/libexec/ApplicationFirewall/socketfilterfw", "--getglobalstate")
	cmd.Stdout = req.Stdout
	cmd.Stderr = req.Stderr
	_ = cmd.Run()

	fmt.Fprintln(req.Stdout, "\n=== 已允许的应用 ===")
	listCmd := exec.CommandContext(ctx, "/usr/libexec/ApplicationFirewall/socketfilterfw", "--listapps")
	listCmd.Stdout = req.Stdout
	listCmd.Stderr = req.Stderr
	return listCmd.Run()
}

func fwStatusWindows(ctx context.Context, req BuiltinRequest) error {
	cmd := exec.CommandContext(ctx, "netsh", "advfirewall", "firewall", "show", "rule", "name=all", "dir=in", "status=enabled")
	cmd.Stdout = req.Stdout
	cmd.Stderr = req.Stderr
	return cmd.Run()
}

// runFwAllow implements `kopsc fw allow <port>/<protocol>`: open a port.
func runFwAllow(ctx context.Context, req BuiltinRequest) error {
	opts, err := parseFwPortArgs(req.Args, "allow")
	if err != nil {
		return err
	}

	switch runtime.GOOS {
	case "linux":
		return fwAllowLinux(ctx, opts, req)
	case "darwin":
		return fmt.Errorf("macOS 不支持通过命令行开放端口，请在系统偏好设置中配置")
	case "windows":
		return fwAllowWindows(ctx, opts, req)
	default:
		return fmt.Errorf("不支持的操作系统: %s", runtime.GOOS)
	}
}

func fwAllowLinux(ctx context.Context, opts fwPortOptions, req BuiltinRequest) error {
	portProto := fmt.Sprintf("%d/%s", opts.port, opts.protocol)
	if path, err := exec.LookPath("ufw"); err == nil {
		fmt.Fprintf(req.Stdout, "开放端口: %s (ufw)\n", portProto)
		return runCommand(ctx, path, []string{"allow", portProto}, req)
	}
	if path, err := exec.LookPath("firewall-cmd"); err == nil {
		fmt.Fprintf(req.Stdout, "开放端口: %s (firewalld)\n", portProto)
		if err := runCommand(ctx, path, []string{"--add-port=" + portProto, "--permanent"}, req); err != nil {
			return err
		}
		return runCommand(ctx, path, []string{"--reload"}, req)
	}
	return fmt.Errorf("未找到 ufw 或 firewall-cmd")
}

func fwAllowWindows(ctx context.Context, opts fwPortOptions, req BuiltinRequest) error {
	name := fmt.Sprintf("kopsc-allow-%d-%s", opts.port, opts.protocol)
	fmt.Fprintf(req.Stdout, "开放端口: %d/%s (Windows Firewall)\n", opts.port, opts.protocol)
	return runCommand(ctx, "netsh", []string{
		"advfirewall", "firewall", "add", "rule",
		"name=" + name, "dir=in", "action=allow",
		"protocol=" + opts.protocol, "localport=" + fmt.Sprintf("%d", opts.port),
	}, req)
}

// runFwDeny implements `kopsc fw deny <port>/<protocol>`: close a port.
func runFwDeny(ctx context.Context, req BuiltinRequest) error {
	opts, err := parseFwPortArgs(req.Args, "deny")
	if err != nil {
		return err
	}

	switch runtime.GOOS {
	case "linux":
		return fwDenyLinux(ctx, opts, req)
	case "darwin":
		return fmt.Errorf("macOS 不支持通过命令行关闭端口，请在系统偏好设置中配置")
	case "windows":
		return fwDenyWindows(ctx, opts, req)
	default:
		return fmt.Errorf("不支持的操作系统: %s", runtime.GOOS)
	}
}

func fwDenyLinux(ctx context.Context, opts fwPortOptions, req BuiltinRequest) error {
	portProto := fmt.Sprintf("%d/%s", opts.port, opts.protocol)
	if path, err := exec.LookPath("ufw"); err == nil {
		fmt.Fprintf(req.Stdout, "关闭端口: %s (ufw)\n", portProto)
		return runCommand(ctx, path, []string{"deny", portProto}, req)
	}
	if path, err := exec.LookPath("firewall-cmd"); err == nil {
		fmt.Fprintf(req.Stdout, "关闭端口: %s (firewalld)\n", portProto)
		if err := runCommand(ctx, path, []string{"--remove-port=" + portProto, "--permanent"}, req); err != nil {
			return err
		}
		return runCommand(ctx, path, []string{"--reload"}, req)
	}
	return fmt.Errorf("未找到 ufw 或 firewall-cmd")
}

func fwDenyWindows(ctx context.Context, opts fwPortOptions, req BuiltinRequest) error {
	name := fmt.Sprintf("kopsc-allow-%d-%s", opts.port, opts.protocol)
	fmt.Fprintf(req.Stdout, "关闭端口: %d/%s (Windows Firewall)\n", opts.port, opts.protocol)
	return runCommand(ctx, "netsh", []string{
		"advfirewall", "firewall", "delete", "rule",
		"name=" + name,
	}, req)
}

type fwPortOptions struct {
	port     int
	protocol string
}

func parseFwPortArgs(args []string, action string) (fwPortOptions, error) {
	opts := fwPortOptions{protocol: "tcp"}
	var positional []string

	for i := 0; i < len(args); i++ {
		arg := args[i]
		switch {
		case arg == "--":
			positional = append(positional, args[i+1:]...)
			i = len(args)
		case arg == "-p" || arg == "--protocol":
			if i+1 >= len(args) {
				return opts, fmt.Errorf("%s 缺少协议", arg)
			}
			i++
			proto := strings.ToLower(args[i])
			if proto != "tcp" && proto != "udp" {
				return opts, fmt.Errorf("协议必须是 tcp 或 udp，收到 %q", args[i])
			}
			opts.protocol = proto
		case strings.HasPrefix(arg, "--protocol="):
			proto := strings.ToLower(strings.TrimPrefix(arg, "--protocol="))
			if proto != "tcp" && proto != "udp" {
				return opts, fmt.Errorf("协议必须是 tcp 或 udp，收到 %q", strings.TrimPrefix(arg, "--protocol="))
			}
			opts.protocol = proto
		case strings.HasPrefix(arg, "-") && arg != "-":
			return opts, fmt.Errorf("未知选项 %q", arg)
		default:
			positional = append(positional, arg)
		}
	}
	if len(positional) == 0 {
		return opts, fmt.Errorf("缺少端口号；用法: kopsc fw %s <端口>[/<协议>]", action)
	}

	portStr := positional[0]
	if idx := strings.Index(portStr, "/"); idx >= 0 {
		proto := strings.ToLower(portStr[idx+1:])
		if proto != "tcp" && proto != "udp" {
			return opts, fmt.Errorf("协议必须是 tcp 或 udp，收到 %q", proto)
		}
		opts.protocol = proto
		portStr = portStr[:idx]
	}

	n, err := parseInt(portStr)
	if err != nil || n < 1 || n > 65535 {
		return opts, fmt.Errorf("端口必须是 1-65535 的整数，收到 %q", positional[0])
	}
	opts.port = n
	return opts, nil
}

func parseInt(s string) (int, error) {
	n := 0
	for _, c := range s {
		if c < '0' || c > '9' {
			return 0, fmt.Errorf("不是整数: %q", s)
		}
		n = n*10 + int(c-'0')
	}
	return n, nil
}
