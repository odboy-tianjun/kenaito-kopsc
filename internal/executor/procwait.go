package executor

import (
	"context"
	"errors"
	"fmt"
	"strconv"
	"strings"
	"time"
)

// waitOptions are the parsed arguments of `kopsc proc wait`.
type waitOptions struct {
	keywords []string      // must all match (logical AND)
	pids     []int         // explicit PIDs to watch
	interval time.Duration // poll period, 1s by default
	timeout  time.Duration // 0 means wait forever
	once     bool          // check once and report, do not wait
	quiet    bool          // do not print progress
}

// runProcWait implements `kopsc proc wait`: poll until the matched processes
// are gone, so a script can block like `while pgrep nginx; do sleep 1; done`
// but with an explicit target and optional timeout.
//
// Exit codes: 0 when the processes have exited (or --once found none),
// 2 when --timeout expires first, 1 on usage or runtime errors.
func runProcWait(ctx context.Context, req BuiltinRequest) error {
	opts, err := parseProcWaitArgs(req.Args)
	if err != nil {
		return err
	}
	if len(opts.keywords) == 0 && len(opts.pids) == 0 {
		return errors.New("缺少要等待的进程；用法: kopsc proc wait <关键字...> [--pid <PID>]")
	}

	describe := describeWaitTarget(opts)
	start := time.Now()

	remaining, err := findWaiting(ctx, opts)
	if err != nil {
		return err
	}
	if len(remaining) == 0 {
		if !opts.once {
			fmt.Fprintf(req.Stdout, "%s 当前没有运行\n", describe)
		}
		return nil
	}
	if !opts.quiet {
		fmt.Fprintf(req.Stdout, "等待 %s 退出（共 %d 个进程）: %s\n", describe, len(remaining), summarizeProcesses(remaining))
	}
	if opts.once {
		return nil
	}

	var timeout <-chan time.Time
	if opts.timeout > 0 {
		timer := time.NewTimer(opts.timeout)
		defer timer.Stop()
		timeout = timer.C
	}

	ticker := time.NewTicker(opts.interval)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-timeout:
			left, _ := findWaiting(ctx, opts)
			return &ExitError{Code: 2, Err: fmt.Errorf(
				"等待超时（%s）：%s 仍在运行: %s", opts.timeout, describe, summarizeProcesses(left))}
		case <-ticker.C:
			left, err := findWaiting(ctx, opts)
			if err != nil {
				return err
			}
			if len(left) == 0 {
				if !opts.quiet {
					fmt.Fprintf(req.Stdout, "%s 已退出（等待 %s）\n", describe, time.Since(start).Round(time.Second))
				}
				return nil
			}
		}
	}
}

// findWaiting returns the processes that still satisfy the wait target.
func findWaiting(ctx context.Context, opts waitOptions) ([]processInfo, error) {
	procs, err := processLister(ctx)
	if err != nil {
		return nil, err
	}

	var remaining []processInfo
	seen := map[int]bool{}
	for _, pid := range opts.pids {
		ref, ok := processesWithPID(procs, pid)
		if ok {
			remaining = append(remaining, ref)
			seen[pid] = true
		}
	}
	for _, ref := range processesMatching(procs, opts.keywords) {
		if !seen[ref.PID] {
			remaining = append(remaining, ref)
		}
	}
	return remaining, nil
}

// describeWaitTarget renders what is being waited for, for the progress lines.
func describeWaitTarget(opts waitOptions) string {
	var parts []string
	if len(opts.keywords) > 0 {
		if len(opts.keywords) == 1 {
			parts = append(parts, "进程 "+opts.keywords[0])
		} else {
			parts = append(parts, "进程 "+strings.Join(opts.keywords, " 且 "))
		}
	}
	if len(opts.pids) > 0 {
		ids := make([]string, len(opts.pids))
		for i, pid := range opts.pids {
			ids[i] = strconv.Itoa(pid)
		}
		parts = append(parts, "PID "+strings.Join(ids, ","))
	}
	return strings.Join(parts, "，")
}

// summarizeProcesses renders "1234(nginx), 5678(nginx: worker)" for messages,
// keeping the line readable when many processes match.
func summarizeProcesses(procs []processInfo) string {
	const max = 5
	parts := make([]string, 0, max)
	for i, p := range procs {
		if i == max {
			parts = append(parts, fmt.Sprintf("…共 %d 个", len(procs)))
			break
		}
		name := p.Name
		if name == "" {
			name = "?"
		}
		parts = append(parts, fmt.Sprintf("%d(%s)", p.PID, name))
	}
	return strings.Join(parts, ", ")
}

// parseProcWaitArgs parses kopsc's options; everything else is a keyword.
func parseProcWaitArgs(args []string) (waitOptions, error) {
	opts := waitOptions{interval: time.Second}
	positionalOnly := false

	for i := 0; i < len(args); i++ {
		arg := args[i]

		if arg == "--" {
			positionalOnly = true
			continue
		}

		if !positionalOnly && strings.HasPrefix(arg, "-") && arg != "-" {
			name, value := arg, ""
			if eq := strings.IndexByte(arg, '='); eq >= 0 && strings.HasPrefix(arg, "--") {
				name, value = arg[:eq], arg[eq+1:]
			}
			takeValue := func() (string, error) {
				if value != "" {
					return value, nil
				}
				if i+1 >= len(args) {
					return "", fmt.Errorf("%s 缺少参数", name)
				}
				i++
				return args[i], nil
			}

			switch name {
			case "--pid", "-p":
				raw, err := takeValue()
				if err != nil {
					return opts, err
				}
				// Allow comma-separated lists: --pid 1234,5678
				for _, field := range strings.Split(raw, ",") {
					pid, err := strconv.Atoi(strings.TrimSpace(field))
					if err != nil || pid <= 0 {
						return opts, fmt.Errorf("--pid 需要正整数 PID，收到 %q", field)
					}
					opts.pids = append(opts.pids, pid)
				}
			case "--interval":
				raw, err := takeValue()
				if err != nil {
					return opts, err
				}
				d, err := parseWaitDuration(raw)
				if err != nil {
					return opts, fmt.Errorf("--interval: %w", err)
				}
				opts.interval = d
			case "--timeout":
				raw, err := takeValue()
				if err != nil {
					return opts, err
				}
				d, err := parseWaitDuration(raw)
				if err != nil {
					return opts, fmt.Errorf("--timeout: %w", err)
				}
				opts.timeout = d
			case "--once":
				opts.once = true
			case "-q", "--quiet":
				opts.quiet = true
			default:
				return opts, fmt.Errorf("未知选项 %q；进程关键字请直接作为参数传入，或用 -- 分隔", arg)
			}
			continue
		}

		opts.keywords = append(opts.keywords, arg)
	}

	if opts.interval <= 0 {
		return opts, errors.New("--interval 必须大于 0")
	}
	return opts, nil
}

// parseWaitDuration accepts a Go duration ("1.5s", "500ms") or plain seconds
// ("30"), which is what most users type.
func parseWaitDuration(raw string) (time.Duration, error) {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return 0, errors.New("缺少时长")
	}
	if secs, err := strconv.Atoi(raw); err == nil {
		if secs < 0 {
			return 0, fmt.Errorf("时长不能为负: %q", raw)
		}
		return time.Duration(secs) * time.Second, nil
	}
	d, err := time.ParseDuration(raw)
	if err != nil || d <= 0 {
		return 0, fmt.Errorf("无法识别时长 %q，可用 30（秒）或 30s、500ms、1m", raw)
	}
	return d, nil
}
