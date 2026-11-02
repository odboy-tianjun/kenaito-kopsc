package executor

import (
	"context"
	"fmt"
	"os/exec"
	"runtime"
	"strconv"
	"strings"
)

// runNetPing implements `kopsc net ping <host>`: connectivity check.
func runNetPing(ctx context.Context, req BuiltinRequest) error {
	opts, err := parseNetPingArgs(req.Args)
	if err != nil {
		return err
	}
	args := []string{}
	if opts.count > 0 {
		if runtime.GOOS == "windows" {
			args = append(args, "-n", strconv.Itoa(opts.count))
		} else {
			args = append(args, "-c", strconv.Itoa(opts.count))
		}
	}
	if opts.interval > 0 {
		if runtime.GOOS != "windows" {
			args = append(args, "-i", strconv.Itoa(opts.interval))
		}
	}
	args = append(args, opts.host)

	cmd := exec.CommandContext(ctx, "ping", args...)
	cmd.Stdout = req.Stdout
	cmd.Stderr = req.Stderr
	return cmd.Run()
}

type netPingOptions struct {
	host     string
	count    int
	interval int
}

func parseNetPingArgs(args []string) (netPingOptions, error) {
	opts := netPingOptions{count: 4}
	var positional []string

	for i := 0; i < len(args); i++ {
		arg := args[i]
		switch {
		case arg == "--":
			positional = append(positional, args[i+1:]...)
			i = len(args)
		case arg == "-c" || arg == "-n" || arg == "--count":
			if i+1 >= len(args) {
				return opts, fmt.Errorf("%s 缺少次数", arg)
			}
			i++
			n, err := strconv.Atoi(args[i])
			if err != nil || n < 1 {
				return opts, fmt.Errorf("次数需要正整数，收到 %q", args[i])
			}
			opts.count = n
		case arg == "-i" || arg == "--interval":
			if i+1 >= len(args) {
				return opts, fmt.Errorf("%s 缺少间隔秒数", arg)
			}
			i++
			n, err := strconv.Atoi(args[i])
			if err != nil || n < 1 {
				return opts, fmt.Errorf("间隔需要正整数，收到 %q", args[i])
			}
			opts.interval = n
		case strings.HasPrefix(arg, "-") && arg != "-":
			return opts, fmt.Errorf("未知选项 %q", arg)
		default:
			positional = append(positional, arg)
		}
	}
	if len(positional) == 0 {
		return opts, fmt.Errorf("缺少目标主机；用法: kopsc net ping <主机>")
	}
	opts.host = positional[0]
	return opts, nil
}

// runNetTrace implements `kopsc net trace <host>`: route tracing.
func runNetTrace(ctx context.Context, req BuiltinRequest) error {
	if len(req.Args) == 0 {
		return fmt.Errorf("缺少目标主机；用法: kopsc net trace <主机>")
	}
	host := req.Args[0]

	// Try mtr first (best output), then traceroute, then tracert on Windows.
	if path, err := exec.LookPath("mtr"); err == nil {
		cmd := exec.CommandContext(ctx, path, "--report", "--report-wide", host)
		cmd.Stdout = req.Stdout
		cmd.Stderr = req.Stderr
		return cmd.Run()
	}
	if runtime.GOOS == "windows" {
		cmd := exec.CommandContext(ctx, "tracert", host)
		cmd.Stdout = req.Stdout
		cmd.Stderr = req.Stderr
		return cmd.Run()
	}
	if path, err := exec.LookPath("traceroute"); err == nil {
		cmd := exec.CommandContext(ctx, path, host)
		cmd.Stdout = req.Stdout
		cmd.Stderr = req.Stderr
		return cmd.Run()
	}
	return fmt.Errorf("未找到 traceroute 或 mtr，请安装其中之一")
}

// runNetDNS implements `kopsc net dns <host>`: DNS lookup.
func runNetDNS(ctx context.Context, req BuiltinRequest) error {
	opts, err := parseNetDNSArgs(req.Args)
	if err != nil {
		return err
	}

	// Try dig first, then nslookup, then host.
	if path, err := exec.LookPath("dig"); err == nil {
		args := []string{opts.host}
		if opts.recordType != "" {
			args = []string{opts.host, opts.recordType}
		}
		if opts.server != "" {
			args = append(args, "@"+opts.server)
		}
		args = append(args, "+noall", "+answer")
		cmd := exec.CommandContext(ctx, path, args...)
		cmd.Stdout = req.Stdout
		cmd.Stderr = req.Stderr
		return cmd.Run()
	}
	if path, err := exec.LookPath("nslookup"); err == nil {
		args := []string{opts.host}
		if opts.server != "" {
			args = []string{opts.host, opts.server}
		}
		cmd := exec.CommandContext(ctx, path, args...)
		cmd.Stdout = req.Stdout
		cmd.Stderr = req.Stderr
		return cmd.Run()
	}
	if path, err := exec.LookPath("host"); err == nil {
		args := []string{opts.host}
		if opts.server != "" {
			args = append(args, opts.server)
		}
		cmd := exec.CommandContext(ctx, path, args...)
		cmd.Stdout = req.Stdout
		cmd.Stderr = req.Stderr
		return cmd.Run()
	}
	return fmt.Errorf("未找到 dig、nslookup 或 host，请安装其中之一")
}

type netDNSOptions struct {
	host       string
	recordType string
	server     string
}

func parseNetDNSArgs(args []string) (netDNSOptions, error) {
	var opts netDNSOptions
	var positional []string

	for i := 0; i < len(args); i++ {
		arg := args[i]
		switch {
		case arg == "--":
			positional = append(positional, args[i+1:]...)
			i = len(args)
		case arg == "-t" || arg == "--type":
			if i+1 >= len(args) {
				return opts, fmt.Errorf("%s 缺少记录类型", arg)
			}
			i++
			opts.recordType = strings.ToUpper(args[i])
		case strings.HasPrefix(arg, "--type="):
			opts.recordType = strings.ToUpper(strings.TrimPrefix(arg, "--type="))
		case arg == "-s" || arg == "--server":
			if i+1 >= len(args) {
				return opts, fmt.Errorf("%s 缺少 DNS 服务器", arg)
			}
			i++
			opts.server = args[i]
		case strings.HasPrefix(arg, "--server="):
			opts.server = strings.TrimPrefix(arg, "--server=")
		case strings.HasPrefix(arg, "-") && arg != "-":
			return opts, fmt.Errorf("未知选项 %q", arg)
		default:
			positional = append(positional, arg)
		}
	}
	if len(positional) == 0 {
		return opts, fmt.Errorf("缺少域名；用法: kopsc net dns <域名>")
	}
	opts.host = positional[0]
	return opts, nil
}

// runNetCurl implements `kopsc net curl <url>`: HTTP diagnostics showing
// status code, time breakdown, and redirect chain.
func runNetCurl(ctx context.Context, req BuiltinRequest) error {
	opts, err := parseNetCurlArgs(req.Args)
	if err != nil {
		return err
	}

	if _, err := exec.LookPath("curl"); err != nil {
		return fmt.Errorf("net curl 需要 curl，请先安装")
	}

	args := []string{"-sS", "-o", "/dev/null", "-w",
		"HTTP 状态码:  %{http_code}\n" +
			"重定向次数:  %{num_redirects}\n" +
			"DNS 解析:   %{time_namelookup}s\n" +
			"TCP 连接:   %{time_connect}s\n" +
			"TLS 握手:   %{time_appconnect}s\n" +
			"首字节:     %{time_starttransfer}s\n" +
			"总耗时:     %{time_total}s\n" +
			"下载大小:   %{size_download} bytes\n" +
			"下载速度:   %{speed_download} bytes/s\n",
	}
	if opts.follow {
		args = append(args, "-L")
	}
	if opts.method != "" {
		args = append(args, "-X", opts.method)
	}
	for _, h := range opts.headers {
		args = append(args, "-H", h)
	}
	args = append(args, opts.url)

	cmd := exec.CommandContext(ctx, "curl", args...)
	cmd.Stdout = req.Stdout
	cmd.Stderr = req.Stderr
	return cmd.Run()
}

type netCurlOptions struct {
	url     string
	follow  bool
	method  string
	headers []string
}

func parseNetCurlArgs(args []string) (netCurlOptions, error) {
	var opts netCurlOptions
	var positional []string

	for i := 0; i < len(args); i++ {
		arg := args[i]
		switch {
		case arg == "--":
			positional = append(positional, args[i+1:]...)
			i = len(args)
		case arg == "-L" || arg == "--follow":
			opts.follow = true
		case arg == "-X" || arg == "--method":
			if i+1 >= len(args) {
				return opts, fmt.Errorf("%s 缺少 HTTP 方法", arg)
			}
			i++
			opts.method = strings.ToUpper(args[i])
		case arg == "-H" || arg == "--header":
			if i+1 >= len(args) {
				return opts, fmt.Errorf("%s 缺少 header 值", arg)
			}
			i++
			opts.headers = append(opts.headers, args[i])
		case strings.HasPrefix(arg, "-") && arg != "-":
			return opts, fmt.Errorf("未知选项 %q", arg)
		default:
			positional = append(positional, arg)
		}
	}
	if len(positional) == 0 {
		return opts, fmt.Errorf("缺少 URL；用法: kopsc net curl <URL>")
	}
	opts.url = positional[0]
	return opts, nil
}
