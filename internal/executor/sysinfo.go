package executor

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"runtime"
	"strconv"
	"strings"

	"gitea.odboy.cn/odboy/kenaito-kopsc/internal/platform"
)

// runSysInfo implements `kopsc sysinfo`: show OS, CPU, memory, disk overview.
func runSysInfo(ctx context.Context, req BuiltinRequest) error {
	w := req.Stdout
	res := platform.DetectResources()

	fmt.Fprintf(w, "=== 系统信息 ===\n\n")

	fmt.Fprintf(w, "操作系统:  %s/%s\n", runtime.GOOS, runtime.GOARCH)
	if runtime.GOOS == "linux" {
		if data, err := os.ReadFile("/etc/os-release"); err == nil {
			distro, _ := platform.ParseOSRelease(data)
			if distro != "" {
				fmt.Fprintf(w, "发行版:    %s\n", distro)
			}
		}
		if hostname, err := os.Hostname(); err == nil {
			fmt.Fprintf(w, "主机名:    %s\n", hostname)
		}
		if data, err := os.ReadFile("/proc/version"); err == nil {
			fields := strings.Fields(string(data))
			if len(fields) >= 3 {
				fmt.Fprintf(w, "内核:      %s\n", fields[2])
			}
		}
	}

	fmt.Fprintf(w, "\n=== CPU ===\n\n")
	fmt.Fprintf(w, "逻辑核心:  %d\n", res.CPUs)
	if runtime.GOOS == "linux" {
		if model := cpuModelLinux(); model != "" {
			fmt.Fprintf(w, "型号:      %s\n", model)
		}
	} else if runtime.GOOS == "darwin" {
		if model := cpuModelDarwin(ctx); model != "" {
			fmt.Fprintf(w, "型号:      %s\n", model)
		}
	}

	fmt.Fprintf(w, "\n=== 内存 ===\n\n")
	if res.Memory > 0 {
		fmt.Fprintf(w, "总计:      %s\n", humanSize(res.Memory))
		fmt.Fprintf(w, "检测方式:  %s\n", res.Detected)
	}
	if runtime.GOOS == "linux" {
		if free := memFreeLinux(); free > 0 {
			fmt.Fprintf(w, "可用:      %s\n", humanSize(free))
		}
	}

	fmt.Fprintf(w, "\n=== 磁盘 ===\n\n")
	printDiskInfo(ctx, w)

	return nil
}

func cpuModelLinux() string {
	data, err := os.ReadFile("/proc/cpuinfo")
	if err != nil {
		return ""
	}
	for _, line := range strings.Split(string(data), "\n") {
		if strings.HasPrefix(line, "model name") {
			_, value, ok := strings.Cut(line, ":")
			if ok {
				return strings.TrimSpace(value)
			}
		}
	}
	return ""
}

func cpuModelDarwin(ctx context.Context) string {
	out, err := exec.CommandContext(ctx, "sysctl", "-n", "machdep.cpu.brand_string").Output()
	if err != nil {
		return ""
	}
	return strings.TrimSpace(string(out))
}

func memFreeLinux() int64 {
	data, err := os.ReadFile("/proc/meminfo")
	if err != nil {
		return 0
	}
	for _, line := range strings.Split(string(data), "\n") {
		fields := strings.Fields(line)
		if len(fields) < 2 {
			continue
		}
		if fields[0] == "MemAvailable:" || fields[0] == "MemFree:" {
			n, err := strconv.ParseInt(fields[1], 10, 64)
			if err != nil {
				return 0
			}
			return n * 1024
		}
	}
	return 0
}

func printDiskInfo(ctx context.Context, w interface{ Write([]byte) (int, error) }) {
	if runtime.GOOS == "windows" {
		cmd := exec.CommandContext(ctx, "wmic", "logicaldisk", "get", "size,freespace,caption", "/format:list")
		out, err := cmd.Output()
		if err != nil {
			fmt.Fprintln(w, "无法获取磁盘信息")
			return
		}
		parseWMICDisk(string(out), w)
		return
	}

	cmd := exec.CommandContext(ctx, "df", "-h")
	out, err := cmd.Output()
	if err != nil {
		fmt.Fprintln(w, "无法获取磁盘信息")
		return
	}
	lines := strings.Split(strings.TrimSpace(string(out)), "\n")
	if len(lines) == 0 {
		return
	}
	for _, line := range lines {
		fmt.Fprintln(w, line)
	}
}

func parseWMICDisk(out string, w interface{ Write([]byte) (int, error) }) {
	var caption, freeStr, sizeStr string
	for _, line := range strings.Split(out, "\n") {
		line = strings.TrimSpace(line)
		if strings.HasPrefix(line, "Caption=") {
			caption = strings.TrimPrefix(line, "Caption=")
		} else if strings.HasPrefix(line, "FreeSpace=") {
			freeStr = strings.TrimPrefix(line, "FreeSpace=")
		} else if strings.HasPrefix(line, "Size=") {
			sizeStr = strings.TrimPrefix(line, "Size=")
			if caption != "" && sizeStr != "" {
				size, _ := strconv.ParseInt(sizeStr, 10, 64)
				free, _ := strconv.ParseInt(freeStr, 10, 64)
				fmt.Fprintf(w, "%-6s 总计 %-10s 可用 %-10s\n", caption, humanSize(size), humanSize(free))
				caption, freeStr, sizeStr = "", "", ""
			}
		}
	}
}
