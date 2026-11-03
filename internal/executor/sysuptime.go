package executor

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"runtime"
	"strconv"
	"strings"
	"time"
)

// runSysUptime implements `kopsc sys uptime`: show system uptime.
func runSysUptime(ctx context.Context, req BuiltinRequest) error {
	switch runtime.GOOS {
	case "linux":
		return sysUptimeLinux(req)
	case "darwin":
		return sysUptimeDarwin(ctx, req)
	case "windows":
		return sysUptimeWindows(ctx, req)
	default:
		return fmt.Errorf("不支持的操作系统: %s", runtime.GOOS)
	}
}

func sysUptimeLinux(req BuiltinRequest) error {
	data, err := os.ReadFile("/proc/uptime")
	if err != nil {
		return fmt.Errorf("读取 /proc/uptime 失败: %w", err)
	}
	fields := strings.Fields(string(data))
	if len(fields) < 1 {
		return fmt.Errorf("/proc/uptime 格式异常")
	}
	seconds, err := strconv.ParseFloat(fields[0], 64)
	if err != nil {
		return fmt.Errorf("解析 uptime 失败: %w", err)
	}

	bootTime := time.Now().Add(-time.Duration(seconds) * time.Second)
	fmt.Fprintf(req.Stdout, "运行时间:  %s\n", formatDuration(time.Duration(seconds)*time.Second))
	fmt.Fprintf(req.Stdout, "启动时间:  %s\n", bootTime.Format("2006-01-02 15:04:05"))

	// Load average
	if len(fields) >= 2 {
		idle, _ := strconv.ParseFloat(fields[1], 64)
		fmt.Fprintf(req.Stdout, "空闲时间:  %s\n", formatDuration(time.Duration(idle)*time.Second))
	}

	// Load average from /proc/loadavg
	if loadavg, err := os.ReadFile("/proc/loadavg"); err == nil {
		parts := strings.Fields(string(loadavg))
		if len(parts) >= 3 {
			fmt.Fprintf(req.Stdout, "负载均值:  %s %s %s (1/5/15分钟)\n", parts[0], parts[1], parts[2])
		}
	}
	return nil
}

func sysUptimeDarwin(ctx context.Context, req BuiltinRequest) error {
	out, err := exec.CommandContext(ctx, "sysctl", "-n", "kern.boottime").Output()
	if err != nil {
		return fmt.Errorf("获取启动时间失败: %w", err)
	}
	// Output: { sec = 1234567890, usec = 123456 } Mon Jan 01 00:00:00 2024
	line := string(out)
	secStart := strings.Index(line, "sec = ")
	if secStart < 0 {
		return fmt.Errorf("无法解析 kern.boottime: %s", line)
	}
	secStr := strings.TrimSpace(line[secStart+6:])
	if idx := strings.Index(secStr, ","); idx >= 0 {
		secStr = secStr[:idx]
	}
	sec, err := strconv.ParseInt(secStr, 10, 64)
	if err != nil {
		return fmt.Errorf("解析启动时间失败: %w", err)
	}

	bootTime := time.Unix(sec, 0)
	uptime := time.Since(bootTime)
	fmt.Fprintf(req.Stdout, "运行时间:  %s\n", formatDuration(uptime))
	fmt.Fprintf(req.Stdout, "启动时间:  %s\n", bootTime.Format("2006-01-02 15:04:05"))

	// Load average
	if out, err := exec.CommandContext(ctx, "sysctl", "-n", "vm.loadavg").Output(); err == nil {
		fmt.Fprintf(req.Stdout, "负载均值:  %s\n", strings.TrimSpace(string(out)))
	}
	return nil
}

func sysUptimeWindows(ctx context.Context, req BuiltinRequest) error {
	// Use wmic to get boot time
	out, err := exec.CommandContext(ctx, "wmic", "os", "get", "LastBootUpTime", "/value").Output()
	if err != nil {
		return fmt.Errorf("获取启动时间失败: %w", err)
	}
	line := strings.TrimSpace(string(out))
	// LastBootUpTime=20240101000000.000000+480
	if !strings.HasPrefix(line, "LastBootUpTime=") {
		return fmt.Errorf("无法解析 wmic 输出: %s", line)
	}
	timeStr := strings.TrimPrefix(line, "LastBootUpTime=")
	// Parse: 20240101000000.000000+480
	if len(timeStr) < 14 {
		return fmt.Errorf("时间格式异常: %s", timeStr)
	}
	bootTime, err := time.Parse("20060102150405", timeStr[:14])
	if err != nil {
		return fmt.Errorf("解析时间失败: %w", err)
	}
	uptime := time.Since(bootTime)
	fmt.Fprintf(req.Stdout, "运行时间:  %s\n", formatDuration(uptime))
	fmt.Fprintf(req.Stdout, "启动时间:  %s\n", bootTime.Format("2006-01-02 15:04:05"))
	return nil
}

// formatDuration formats a duration into a human-readable string.
func formatDuration(d time.Duration) string {
	days := int(d.Hours()) / 24
	hours := int(d.Hours()) % 24
	minutes := int(d.Minutes()) % 60

	if days > 0 {
		return fmt.Sprintf("%d 天 %d 小时 %d 分钟", days, hours, minutes)
	}
	if hours > 0 {
		return fmt.Sprintf("%d 小时 %d 分钟", hours, minutes)
	}
	return fmt.Sprintf("%d 分钟", minutes)
}
