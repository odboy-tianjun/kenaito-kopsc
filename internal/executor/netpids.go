package executor

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"runtime"
	"sort"
	"strconv"
	"strings"
)

// pidPattern extracts "pid=1234" from ss output.
var pidPattern = regexp.MustCompile(`pid=(\d+)`)

// runNetPIDs implements `lsof -ti:<port>`: print the PID of every process
// holding the given port, one per line.
//
// The kopsc command and its output are identical on every platform; only the
// backend differs:
//   - Linux:   lsof -> ss -> /proc (first available wins)
//   - macOS:   lsof (the system ships it; no /proc fallback)
//   - Windows: netstat -ano
func runNetPIDs(ctx context.Context, req BuiltinRequest) error {
	port, err := parsePortArgs(req.Args)
	if err != nil {
		return err
	}

	if runtime.GOOS == "windows" {
		return netPIDsWindows(ctx, port, req)
	}
	return netPIDsUnix(ctx, port, req)
}

func parsePortArgs(args []string) (int, error) {
	var value string
	for i := 0; i < len(args); i++ {
		arg := args[i]
		switch {
		case arg == "--port":
			if i+1 >= len(args) {
				return 0, fmt.Errorf("--port 缺少端口号")
			}
			i++
			value = args[i]
		case strings.HasPrefix(arg, "--port="):
			value = strings.TrimPrefix(arg, "--port=")
		case strings.HasPrefix(arg, "-") && arg != "-":
			return 0, fmt.Errorf("未知选项 %q", arg)
		default:
			if value != "" {
				return 0, fmt.Errorf("一次只支持一个端口，收到多个参数")
			}
			value = arg
		}
	}
	if value == "" {
		return 0, fmt.Errorf("必须提供端口，例如 kopsc net pids 8080")
	}

	port, err := strconv.Atoi(value)
	if err != nil || port < 1 || port > 65535 {
		return 0, fmt.Errorf("端口必须是 1-65535 的整数，收到 %q", value)
	}
	return port, nil
}

func netPIDsUnix(ctx context.Context, port int, req BuiltinRequest) error {
	pids, err := unixPIDs(ctx, port)
	if err != nil {
		return err
	}
	writePIDs(req.Stdout, pids)
	return nil
}

// unixPIDs tries lsof, then ss, then /proc on Linux.
func unixPIDs(ctx context.Context, port int) ([]string, error) {
	var lsofErr error
	if path, err := exec.LookPath("lsof"); err == nil {
		pids, matched, err := lsofPIDs(ctx, path, port)
		if err == nil {
			if !matched {
				return nil, nil
			}
			return pids, nil
		}
		lsofErr = err
	}

	var ssErr error
	if path, err := exec.LookPath("ss"); err == nil {
		pids, err := ssPIDs(ctx, path, port)
		if err == nil {
			return pids, nil
		}
		ssErr = err
	}

	if runtime.GOOS == "linux" {
		if pids, err := procPIDs(port, "/proc"); err == nil {
			return pids, nil
		} else if lsofErr == nil && ssErr == nil {
			return nil, err
		}
	}

	switch {
	case lsofErr != nil:
		return nil, lsofErr
	case ssErr != nil:
		return nil, ssErr
	}
	return nil, fmt.Errorf("未找到 lsof 或 ss（macOS 自带 lsof；Linux 请安装 lsof 或 iproute2）")
}

func lsofPIDs(ctx context.Context, path string, port int) ([]string, bool, error) {
	cmd := exec.CommandContext(ctx, path, "-ti:"+strconv.Itoa(port))
	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr

	err := cmd.Run()
	if err != nil {
		var exitErr *exec.ExitError
		// lsof exits 1 with no output when nothing holds the port.
		if errors.As(err, &exitErr) && exitErr.ExitCode() == 1 && stdout.Len() == 0 {
			return nil, false, nil
		}
		message := strings.TrimSpace(stderr.String())
		if message == "" {
			message = err.Error()
		}
		return nil, false, fmt.Errorf("lsof -ti:%d 失败: %s", port, message)
	}
	return normalizePIDs(strings.Fields(stdout.String())), true, nil
}

func ssPIDs(ctx context.Context, path string, port int) ([]string, error) {
	cmd := exec.CommandContext(ctx, path, "-H", "-t", "-u", "-n", "-p")
	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr

	if err := cmd.Run(); err != nil {
		message := strings.TrimSpace(stderr.String())
		if message == "" {
			message = err.Error()
		}
		return nil, fmt.Errorf("ss -t -u -n -p 失败: %s", message)
	}
	return parseSSPIDs(stdout.String(), port), nil
}

// parseSSPIDs extracts PIDs for a local port from `ss -t -u -n -p` output.
func parseSSPIDs(output string, port int) []string {
	suffix := ":" + strconv.Itoa(port)
	var pids []string

	for _, line := range strings.Split(output, "\n") {
		fields := strings.Fields(line)
		if len(fields) < 5 {
			continue
		}
		// Netid State Recv-Q Send-Q Local:Port Peer:Port Process
		if !strings.HasSuffix(fields[4], suffix) {
			continue
		}
		match := pidPattern.FindStringSubmatch(line)
		if len(match) == 2 {
			pids = append(pids, match[1])
		}
	}
	return normalizePIDs(pids)
}

// procPIDs derives PIDs from /proc without external tools (Linux only).
func procPIDs(port int, procRoot string) ([]string, error) {
	inodes := make(map[string]bool)

	for _, name := range []string{"net/tcp", "net/tcp6", "net/udp", "net/udp6"} {
		data, err := os.ReadFile(filepath.Join(procRoot, filepath.FromSlash(name)))
		if err != nil {
			continue
		}
		lines := strings.Split(string(data), "\n")
		if len(lines) > 0 {
			lines = lines[1:] // header
		}
		for _, line := range lines {
			fields := strings.Fields(line)
			if len(fields) < 10 {
				continue
			}
			local := fields[1]
			idx := strings.LastIndex(local, ":")
			if idx < 0 {
				continue
			}
			hexPort, err := strconv.ParseInt(local[idx+1:], 16, 32)
			if err != nil || int(hexPort) != port {
				continue
			}
			inodes[fields[9]] = true // inode column
		}
	}

	if len(inodes) == 0 {
		return nil, nil
	}

	entries, err := os.ReadDir(procRoot)
	if err != nil {
		return nil, fmt.Errorf("读取 %s 失败: %w", procRoot, err)
	}

	var pids []string
	for _, entry := range entries {
		if !entry.IsDir() {
			continue
		}
		pid := entry.Name()
		if _, err := strconv.Atoi(pid); err != nil {
			continue
		}

		fdDir := filepath.Join(procRoot, pid, "fd")
		fds, err := os.ReadDir(fdDir)
		if err != nil {
			continue // permission denied for other users
		}
		for _, fd := range fds {
			link, err := os.Readlink(filepath.Join(fdDir, fd.Name()))
			if err != nil {
				continue
			}
			if !strings.HasPrefix(link, "socket:[") {
				continue
			}
			inode := strings.TrimSuffix(strings.TrimPrefix(link, "socket:["), "]")
			if inodes[inode] {
				pids = append(pids, pid)
				break
			}
		}
	}
	return normalizePIDs(pids), nil
}

func netPIDsWindows(ctx context.Context, port int, req BuiltinRequest) error {
	cmd := exec.CommandContext(ctx, "netstat", "-ano")
	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr

	if err := cmd.Run(); err != nil {
		message := strings.TrimSpace(stderr.String())
		if message == "" {
			message = err.Error()
		}
		return fmt.Errorf("netstat -ano 失败: %s", message)
	}

	writePIDs(req.Stdout, parseNetstatPIDs(stdout.String(), port))
	return nil
}

// parseNetstatPIDs extracts unique owning PIDs for a local port from
// `netstat -ano` output.
func parseNetstatPIDs(output string, port int) []string {
	suffix := ":" + strconv.Itoa(port)
	var pids []string

	for _, line := range strings.Split(output, "\n") {
		fields := strings.Fields(line)
		if len(fields) < 2 {
			continue
		}
		proto := strings.ToUpper(fields[0])
		if proto != "TCP" && proto != "UDP" {
			continue
		}
		if !strings.HasSuffix(fields[1], suffix) {
			continue
		}

		pid := fields[len(fields)-1]
		if _, err := strconv.Atoi(pid); err != nil {
			continue
		}
		pids = append(pids, pid)
	}
	return normalizePIDs(pids)
}

// normalizePIDs de-duplicates and sorts PIDs numerically so every platform
// prints the same order.
func normalizePIDs(pids []string) []string {
	seen := make(map[string]bool)
	out := make([]string, 0, len(pids))
	for _, pid := range pids {
		pid = strings.TrimSpace(pid)
		if pid == "" {
			continue
		}
		if _, err := strconv.Atoi(pid); err != nil {
			continue
		}
		if seen[pid] {
			continue
		}
		seen[pid] = true
		out = append(out, pid)
	}
	sort.Slice(out, func(i, j int) bool {
		a, _ := strconv.Atoi(out[i])
		b, _ := strconv.Atoi(out[j])
		return a < b
	})
	return out
}

func writePIDs(w interface{ Write([]byte) (int, error) }, pids []string) {
	for _, pid := range pids {
		fmt.Fprintln(w, pid)
	}
}
