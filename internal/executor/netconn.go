package executor

import (
	"bufio"
	"context"
	"fmt"
	"os/exec"
	"runtime"
	"sort"
	"strings"
)

// runNetConn implements `kopsc net conn`: show network connection statistics.
func runNetConn(ctx context.Context, req BuiltinRequest) error {
	switch runtime.GOOS {
	case "linux":
		return netConnLinux(ctx, req)
	case "darwin":
		return netConnDarwin(ctx, req)
	case "windows":
		return netConnWindows(ctx, req)
	default:
		return fmt.Errorf("不支持的操作系统: %s", runtime.GOOS)
	}
}

func netConnLinux(ctx context.Context, req BuiltinRequest) error {
	// Try ss first (faster)
	if path, err := exec.LookPath("ss"); err == nil {
		return netConnSS(ctx, path, req)
	}
	// Fallback to netstat
	return netConnNetstat(ctx, req)
}

func netConnSS(ctx context.Context, path string, req BuiltinRequest) error {
	out, err := exec.CommandContext(ctx, path, "-tan").Output()
	if err != nil {
		return fmt.Errorf("ss 执行失败: %w", err)
	}
	return parseNetstatOutput(string(out), req)
}

func netConnNetstat(ctx context.Context, req BuiltinRequest) error {
	out, err := exec.CommandContext(ctx, "netstat", "-tan").Output()
	if err != nil {
		return fmt.Errorf("netstat 执行失败: %w", err)
	}
	return parseNetstatOutput(string(out), req)
}

func netConnDarwin(ctx context.Context, req BuiltinRequest) error {
	out, err := exec.CommandContext(ctx, "netstat", "-tan").Output()
	if err != nil {
		return fmt.Errorf("netstat 执行失败: %w", err)
	}
	return parseNetstatOutput(string(out), req)
}

func netConnWindows(ctx context.Context, req BuiltinRequest) error {
	out, err := exec.CommandContext(ctx, "netstat", "-an").Output()
	if err != nil {
		return fmt.Errorf("netstat 执行失败: %w", err)
	}
	return parseNetstatOutput(string(out), req)
}

func parseNetstatOutput(output string, req BuiltinRequest) error {
	stats := make(map[string]int)
	total := 0

	scanner := bufio.NewScanner(strings.NewReader(output))
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if line == "" {
			continue
		}
		fields := strings.Fields(line)
		if len(fields) < 4 {
			continue
		}

		// Detect state column (usually last for TCP)
		state := fields[len(fields)-1]
		if isConnectionState(state) {
			stats[state]++
			total++
		}
	}

	if total == 0 {
		fmt.Fprintln(req.Stdout, "没有活动的 TCP 连接")
		return nil
	}

	fmt.Fprintf(req.Stdout, "=== TCP 连接统计 (共 %d 个) ===\n\n", total)

	// Sort by count descending
	type stateCount struct {
		state string
		count int
	}
	var sorted []stateCount
	for state, count := range stats {
		sorted = append(sorted, stateCount{state, count})
	}
	sort.Slice(sorted, func(i, j int) bool {
		return sorted[i].count > sorted[j].count
	})

	for _, sc := range sorted {
		fmt.Fprintf(req.Stdout, "%-15s %d\n", sc.state, sc.count)
	}

	return nil
}

func isConnectionState(s string) bool {
	switch s {
	case "ESTABLISHED", "SYN_SENT", "SYN_RECV", "FIN_WAIT1", "FIN_WAIT2",
		"TIME_WAIT", "CLOSE", "CLOSE_WAIT", "LAST_ACK", "LISTEN", "CLOSING":
		return true
	}
	return false
}
