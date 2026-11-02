package executor

import (
	"bufio"
	"context"
	"fmt"
	"os/exec"
	"runtime"
	"strconv"
	"strings"
	"time"
)

// runLogSearch implements `kopsc log search <keyword>`: search system logs.
func runLogSearch(ctx context.Context, req BuiltinRequest) error {
	opts, err := parseLogSearchArgs(req.Args)
	if err != nil {
		return err
	}

	switch runtime.GOOS {
	case "linux":
		return logSearchLinux(ctx, opts, req)
	case "darwin":
		return logSearchDarwin(ctx, opts, req)
	case "windows":
		return logSearchWindows(ctx, opts, req)
	default:
		return fmt.Errorf("不支持的操作系统: %s", runtime.GOOS)
	}
}

type logSearchOptions struct {
	keyword string
	since   string
	unit    string
	lines   int
}

func parseLogSearchArgs(args []string) (logSearchOptions, error) {
	opts := logSearchOptions{lines: 100}
	var positional []string

	for i := 0; i < len(args); i++ {
		arg := args[i]
		switch {
		case arg == "--":
			positional = append(positional, args[i+1:]...)
			i = len(args)
		case arg == "--since":
			if i+1 >= len(args) {
				return opts, fmt.Errorf("--since 缺少时长，例如 --since 1h")
			}
			i++
			opts.since = args[i]
		case strings.HasPrefix(arg, "--since="):
			opts.since = strings.TrimPrefix(arg, "--since=")
		case arg == "-u" || arg == "--unit":
			if i+1 >= len(args) {
				return opts, fmt.Errorf("%s 缺少服务名", arg)
			}
			i++
			opts.unit = args[i]
		case strings.HasPrefix(arg, "--unit="):
			opts.unit = strings.TrimPrefix(arg, "--unit=")
		case arg == "-n" || arg == "--lines":
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
		return opts, fmt.Errorf("缺少搜索关键字；用法: kopsc log search <关键字>")
	}
	opts.keyword = positional[0]
	return opts, nil
}

func logSearchLinux(ctx context.Context, opts logSearchOptions, req BuiltinRequest) error {
	args := []string{"--no-pager", "-n", strconv.Itoa(opts.lines), "--grep=" + opts.keyword}
	if opts.since != "" {
		since, err := normalizeSince(opts.since)
		if err != nil {
			return fmt.Errorf("--since: %w", err)
		}
		args = append(args, "--since="+since)
	}
	if opts.unit != "" {
		args = append(args, "-u", opts.unit)
	}
	cmd := exec.CommandContext(ctx, "journalctl", args...)
	cmd.Stdout = req.Stdout
	cmd.Stderr = req.Stderr
	return cmd.Run()
}

func logSearchDarwin(ctx context.Context, opts logSearchOptions, req BuiltinRequest) error {
	predicate := fmt.Sprintf(`eventMessage CONTAINS[c] "%s"`, opts.keyword)
	if opts.unit != "" {
		predicate = fmt.Sprintf(`(subsystem == "%s" OR process == "%s") AND %s`, opts.unit, opts.unit, predicate)
	}
	since := "1h"
	if opts.since != "" {
		since = opts.since
	}
	cmd := exec.CommandContext(ctx, "log", "show", "--predicate", predicate, "--last", since, "--style", "compact")
	cmd.Stdout = req.Stdout
	cmd.Stderr = req.Stderr
	return cmd.Run()
}

func logSearchWindows(ctx context.Context, opts logSearchOptions, req BuiltinRequest) error {
	args := []string{"qe", "System", "/q:*[System[Provider]]", "/c:" + strconv.Itoa(opts.lines), "/rd:true", "/f:text"}
	cmd := exec.CommandContext(ctx, "wevtutil", args...)
	var stdout strings.Builder
	cmd.Stdout = &stdout
	cmd.Stderr = req.Stderr
	if err := cmd.Run(); err != nil {
		return err
	}
	scanner := bufio.NewScanner(strings.NewReader(stdout.String()))
	count := 0
	for scanner.Scan() {
		line := scanner.Text()
		if strings.Contains(strings.ToLower(line), strings.ToLower(opts.keyword)) {
			fmt.Fprintln(req.Stdout, line)
			count++
		}
	}
	if count == 0 {
		fmt.Fprintf(req.Stdout, "没有找到包含 %q 的日志\n", opts.keyword)
	}
	return nil
}

// normalizeSince converts shorthand like "1h", "30m" to journalctl --since format.
func normalizeSince(s string) (string, error) {
	s = strings.TrimSpace(s)
	if s == "" {
		return "", fmt.Errorf("时长不能为空")
	}
	if d, err := time.ParseDuration(s); err == nil {
		t := time.Now().Add(-d)
		return t.Format("2006-01-02 15:04:05"), nil
	}
	return s, nil
}

// runLogLast implements `kopsc log last`: show recent system log entries.
func runLogLast(ctx context.Context, req BuiltinRequest) error {
	opts, err := parseLogLastArgs(req.Args)
	if err != nil {
		return err
	}

	switch runtime.GOOS {
	case "linux":
		return logLastLinux(ctx, opts, req)
	case "darwin":
		return logLastDarwin(ctx, opts, req)
	case "windows":
		return logLastWindows(ctx, opts, req)
	default:
		return fmt.Errorf("不支持的操作系统: %s", runtime.GOOS)
	}
}

type logLastOptions struct {
	lines int
	unit  string
}

func parseLogLastArgs(args []string) (logLastOptions, error) {
	opts := logLastOptions{lines: 50}
	for i := 0; i < len(args); i++ {
		arg := args[i]
		switch {
		case arg == "--":
			remaining := args[i+1:]
			if len(remaining) > 0 {
				n, err := strconv.Atoi(remaining[0])
				if err != nil || n < 1 {
					return opts, fmt.Errorf("行数需要正整数，收到 %q", remaining[0])
				}
				opts.lines = n
			}
			i = len(args)
		case arg == "-n" || arg == "--lines":
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
		case arg == "-u" || arg == "--unit":
			if i+1 >= len(args) {
				return opts, fmt.Errorf("%s 缺少服务名", arg)
			}
			i++
			opts.unit = args[i]
		case strings.HasPrefix(arg, "--unit="):
			opts.unit = strings.TrimPrefix(arg, "--unit=")
		case strings.HasPrefix(arg, "-") && arg != "-":
			return opts, fmt.Errorf("未知选项 %q", arg)
		default:
			n, err := strconv.Atoi(arg)
			if err != nil || n < 1 {
				return opts, fmt.Errorf("行数需要正整数，收到 %q", arg)
			}
			opts.lines = n
		}
	}
	return opts, nil
}

func logLastLinux(ctx context.Context, opts logLastOptions, req BuiltinRequest) error {
	args := []string{"--no-pager", "-n", strconv.Itoa(opts.lines)}
	if opts.unit != "" {
		args = append(args, "-u", opts.unit)
	}
	cmd := exec.CommandContext(ctx, "journalctl", args...)
	cmd.Stdout = req.Stdout
	cmd.Stderr = req.Stderr
	return cmd.Run()
}

func logLastDarwin(ctx context.Context, opts logLastOptions, req BuiltinRequest) error {
	predicate := `eventMessage != ""`
	if opts.unit != "" {
		predicate = fmt.Sprintf(`subsystem == "%s" OR process == "%s"`, opts.unit, opts.unit)
	}
	cmd := exec.CommandContext(ctx, "log", "show", "--predicate", predicate, "--last", "5m", "--style", "compact")
	cmd.Stdout = req.Stdout
	cmd.Stderr = req.Stderr
	return cmd.Run()
}

func logLastWindows(ctx context.Context, opts logLastOptions, req BuiltinRequest) error {
	cmd := exec.CommandContext(ctx, "wevtutil", "qe", "System",
		"/c:"+strconv.Itoa(opts.lines), "/rd:true", "/f:text")
	cmd.Stdout = req.Stdout
	cmd.Stderr = req.Stderr
	return cmd.Run()
}
