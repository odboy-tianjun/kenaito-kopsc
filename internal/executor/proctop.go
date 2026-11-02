package executor

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"runtime"
	"sort"
	"strconv"
	"strings"
)

// runProcTopMem implements `kopsc proc topmem`: show top N processes by memory.
func runProcTopMem(ctx context.Context, req BuiltinRequest) error {
	opts, err := parseProcTopArgs(req.Args)
	if err != nil {
		return err
	}
	procs, err := listProcessesWithMem(ctx)
	if err != nil {
		return err
	}
	sort.Slice(procs, func(i, j int) bool { return procs[i].mem > procs[j].mem })
	printProcTop(req, procs, opts.count, "内存")
	return nil
}

// runProcTopCPU implements `kopsc proc topcpu`: show top N processes by CPU.
func runProcTopCPU(ctx context.Context, req BuiltinRequest) error {
	opts, err := parseProcTopArgs(req.Args)
	if err != nil {
		return err
	}
	procs, err := listProcessesWithCPU(ctx)
	if err != nil {
		return err
	}
	sort.Slice(procs, func(i, j int) bool { return procs[i].cpu > procs[j].cpu })
	printProcTop(req, procs, opts.count, "CPU")
	return nil
}

type procTopOptions struct {
	count int
}

func parseProcTopArgs(args []string) (procTopOptions, error) {
	opts := procTopOptions{count: 10}
	for i := 0; i < len(args); i++ {
		arg := args[i]
		switch {
		case arg == "--":
			remaining := args[i+1:]
			if len(remaining) > 0 {
				n, err := strconv.Atoi(remaining[0])
				if err != nil || n < 1 {
					return opts, fmt.Errorf("数量需要正整数，收到 %q", remaining[0])
				}
				opts.count = n
			}
			i = len(args)
		case arg == "-n" || arg == "--count":
			if i+1 >= len(args) {
				return opts, fmt.Errorf("%s 缺少数量", arg)
			}
			i++
			n, err := strconv.Atoi(args[i])
			if err != nil || n < 1 {
				return opts, fmt.Errorf("--count 需要正整数，收到 %q", args[i])
			}
			opts.count = n
		case strings.HasPrefix(arg, "--count="):
			n, err := strconv.Atoi(strings.TrimPrefix(arg, "--count="))
			if err != nil || n < 1 {
				return opts, fmt.Errorf("--count 需要正整数")
			}
			opts.count = n
		case strings.HasPrefix(arg, "-") && arg != "-":
			return opts, fmt.Errorf("未知选项 %q", arg)
		default:
			n, err := strconv.Atoi(arg)
			if err != nil || n < 1 {
				return opts, fmt.Errorf("数量需要正整数，收到 %q", arg)
			}
			opts.count = n
		}
	}
	return opts, nil
}

type procEntry struct {
	PID     int
	Name    string
	Command string
	mem     float64
	cpu     float64
}

func listProcessesWithMem(ctx context.Context) ([]procEntry, error) {
	if runtime.GOOS == "linux" || runtime.GOOS == "darwin" {
		return listProcMemPS(ctx)
	}
	return listProcMemTasklist(ctx)
}

func listProcessesWithCPU(ctx context.Context) ([]procEntry, error) {
	if runtime.GOOS == "linux" || runtime.GOOS == "darwin" {
		return listProcCPUPS(ctx)
	}
	return listProcCPUTasklist(ctx)
}

func listProcMemPS(ctx context.Context) ([]procEntry, error) {
	out, err := exec.CommandContext(ctx, "ps", "-eo", "pid=,rss=,comm=,args=").Output()
	if err != nil {
		return nil, fmt.Errorf("ps 执行失败: %w", err)
	}
	var procs []procEntry
	for _, line := range strings.Split(string(out), "\n") {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}
		fields := strings.Fields(line)
		if len(fields) < 3 {
			continue
		}
		pid, _ := strconv.Atoi(fields[0])
		rss, _ := strconv.ParseFloat(fields[1], 64)
		name := fields[2]
		command := strings.Join(fields[3:], " ")
		if pid > 0 {
			procs = append(procs, procEntry{PID: pid, Name: name, Command: command, mem: rss})
		}
	}
	return procs, nil
}

func listProcCPUPS(ctx context.Context) ([]procEntry, error) {
	out, err := exec.CommandContext(ctx, "ps", "-eo", "pid=,%cpu=,comm=,args=").Output()
	if err != nil {
		return nil, fmt.Errorf("ps 执行失败: %w", err)
	}
	var procs []procEntry
	for _, line := range strings.Split(string(out), "\n") {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}
		fields := strings.Fields(line)
		if len(fields) < 3 {
			continue
		}
		pid, _ := strconv.Atoi(fields[0])
		cpu, _ := strconv.ParseFloat(fields[1], 64)
		name := fields[2]
		command := strings.Join(fields[3:], " ")
		if pid > 0 {
			procs = append(procs, procEntry{PID: pid, Name: name, Command: command, cpu: cpu})
		}
	}
	return procs, nil
}

func listProcMemTasklist(ctx context.Context) ([]procEntry, error) {
	out, err := exec.CommandContext(ctx, "tasklist", "/fo", "csv", "/nh").Output()
	if err != nil {
		return nil, fmt.Errorf("tasklist 执行失败: %w", err)
	}
	var procs []procEntry
	for _, line := range strings.Split(string(out), "\n") {
		fields := strings.Split(strings.TrimSpace(line), ",")
		if len(fields) < 5 {
			continue
		}
		name := strings.Trim(fields[0], `"`)
		pid, _ := strconv.Atoi(strings.Trim(fields[1], `"`))
		memStr := strings.Trim(fields[4], ` "K`)
		memStr = strings.ReplaceAll(memStr, ",", "")
		mem, _ := strconv.ParseFloat(memStr, 64)
		if pid > 0 {
			procs = append(procs, procEntry{PID: pid, Name: name, mem: mem})
		}
	}
	return procs, nil
}

func listProcCPUTasklist(ctx context.Context) ([]procEntry, error) {
	return nil, fmt.Errorf("Windows 下 tasklist 不报告 CPU 使用率，请使用任务管理器")
}

func printProcTop(req BuiltinRequest, procs []procEntry, count int, label string) {
	if len(procs) == 0 {
		fmt.Fprintln(req.Stdout, "没有获取到进程信息")
		return
	}
	if count > len(procs) {
		count = len(procs)
	}
	w := req.Stdout
	fmt.Fprintf(w, "=== 占用 %s 前 %d 的进程 ===\n\n", label, count)
	fmt.Fprintf(w, "%-8s %-8s %-20s %s\n", "PID", label, "进程", "命令")
	for i := 0; i < count; i++ {
		p := procs[i]
		memStr := ""
		if label == "内存" {
			memStr = humanSize(int64(p.mem * 1024))
		} else {
			memStr = fmt.Sprintf("%.1f%%", p.cpu)
		}
		cmd := p.Command
		if len(cmd) > 50 {
			cmd = cmd[:50] + "..."
		}
		fmt.Fprintf(w, "%-8d %-8s %-20s %s\n", p.PID, memStr, p.Name, cmd)
	}
}

// runProcKillName implements `kopsc proc killname <name>`: kill processes by name.
func runProcKillName(ctx context.Context, req BuiltinRequest) error {
	opts, err := parseProcKillNameArgs(req.Args)
	if err != nil {
		return err
	}

	procs, err := listProcesses(ctx)
	if err != nil {
		return err
	}

	var targets []processInfo
	for _, p := range procs {
		if strings.EqualFold(p.Name, opts.name) || strings.Contains(strings.ToLower(p.Command), strings.ToLower(opts.name)) {
			targets = append(targets, p)
		}
	}

	if len(targets) == 0 {
		fmt.Fprintf(req.Stdout, "没有找到名为 %q 的进程\n", opts.name)
		return nil
	}

	fmt.Fprintf(req.Stdout, "找到 %d 个匹配进程:\n", len(targets))
	for _, p := range targets {
		fmt.Fprintf(req.Stdout, "  %d  %s  %s\n", p.PID, p.Name, p.Command)
	}

	if !opts.confirmed {
		fmt.Fprintf(req.Stdout, "\n这是破坏性操作，未执行。加 --yes 确认终止以上进程\n")
		return &ExitError{Code: 2}
	}

	for _, p := range targets {
		if err := killPID(p.PID); err != nil {
			fmt.Fprintf(req.Stderr, "终止 PID %d 失败: %v\n", p.PID, err)
		} else {
			fmt.Fprintf(req.Stdout, "已终止 PID %d (%s)\n", p.PID, p.Name)
		}
	}
	return nil
}

type procKillNameOptions struct {
	name      string
	confirmed bool
}

func parseProcKillNameArgs(args []string) (procKillNameOptions, error) {
	var opts procKillNameOptions
	var positional []string

	for i := 0; i < len(args); i++ {
		arg := args[i]
		switch {
		case arg == "--":
			positional = append(positional, args[i+1:]...)
			i = len(args)
		case arg == "-y" || arg == "--yes":
			opts.confirmed = true
		case strings.HasPrefix(arg, "-") && arg != "-":
			return opts, fmt.Errorf("未知选项 %q", arg)
		default:
			positional = append(positional, arg)
		}
	}
	if len(positional) == 0 {
		return opts, fmt.Errorf("缺少进程名；用法: kopsc proc killname <进程名>")
	}
	opts.name = positional[0]
	return opts, nil
}

func killPID(pid int) error {
	if runtime.GOOS == "windows" {
		return exec.Command("taskkill", "/PID", strconv.Itoa(pid), "/F").Run()
	}
	proc, err := os.FindProcess(pid)
	if err != nil {
		return err
	}
	return proc.Kill()
}
