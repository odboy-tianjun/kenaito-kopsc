package executor

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
)

// processInfo is one running process as kopsc sees it.
type processInfo struct {
	PID     int
	Name    string // executable name, e.g. "nginx"
	Command string // full command line when available
}

// Matches reports whether the process satisfies every keyword: a keyword hits
// when it appears in the process name or anywhere in its command line. All
// keywords must match (logical AND).
func (p processInfo) Matches(keywords []string) bool {
	haystack := strings.ToLower(p.Name + " " + p.Command)
	for _, kw := range keywords {
		if !strings.Contains(haystack, strings.ToLower(kw)) {
			return false
		}
	}
	return true
}

// processLister is how kopsc enumerates processes. It is a variable so the
// wait command can be tested without depending on the host's ps/proc.
var processLister = listProcesses

// listProcesses returns every process visible to the current user, using the
// cheapest mechanism the platform offers.
func listProcesses(ctx context.Context) ([]processInfo, error) {
	switch runtime.GOOS {
	case "linux":
		procs, err := listProcessesProc("/proc")
		if err == nil {
			return procs, nil
		}
		// A hardened container may hide /proc entries; fall through to ps.
		return listProcessesPS(ctx)
	case "windows":
		return listProcessesTasklist(ctx)
	default:
		return listProcessesPS(ctx)
	}
}

// listProcessesProc reads /proc/<pid>/comm and cmdline. Unreadable entries
// (other users' processes, kernel threads) are skipped rather than failing.
func listProcessesProc(procRoot string) ([]processInfo, error) {
	entries, err := os.ReadDir(procRoot)
	if err != nil {
		return nil, err
	}

	var procs []processInfo
	for _, entry := range entries {
		pid, err := strconv.Atoi(entry.Name())
		if err != nil {
			continue
		}
		dir := filepath.Join(procRoot, entry.Name())

		name := ""
		if data, err := os.ReadFile(filepath.Join(dir, "comm")); err == nil {
			name = strings.TrimSpace(string(data))
		}
		command := ""
		if data, err := os.ReadFile(filepath.Join(dir, "cmdline")); err == nil {
			// cmdline is NUL-separated.
			command = strings.TrimSpace(strings.ReplaceAll(string(data), "\x00", " "))
		}
		if name == "" && command == "" {
			continue
		}
		procs = append(procs, processInfo{PID: pid, Name: name, Command: command})
	}
	if len(procs) == 0 {
		return nil, fmt.Errorf("无法从 %s 读取任何进程", procRoot)
	}
	return procs, nil
}

// listProcessesPS parses `ps -eo pid=,args=`, which works on macOS and on Linux
// hosts where /proc is unavailable.
func listProcessesPS(ctx context.Context) ([]processInfo, error) {
	out, err := exec.CommandContext(ctx, "ps", "-eo", "pid=,args=").Output()
	if err != nil {
		return nil, fmt.Errorf("无法列出进程（ps -eo pid=,args= 执行失败: %v）；"+
			"请确认 ps 可用且当前用户有权限", err)
	}
	return parsePS(string(out)), nil
}

// parsePS parses rows of "  1234 /usr/sbin/nginx -g daemon off;".
func parsePS(out string) []processInfo {
	var procs []processInfo
	for _, line := range strings.Split(out, "\n") {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}
		pidField, rest, _ := strings.Cut(line, " ")
		pid, err := strconv.Atoi(pidField)
		if err != nil {
			continue
		}
		rest = strings.TrimSpace(rest)
		name := rest
		if i := strings.IndexByte(name, ' '); i >= 0 {
			name = name[:i]
		}
		name = filepath.Base(name)
		procs = append(procs, processInfo{PID: pid, Name: name, Command: rest})
	}
	return procs
}

// listProcessesTasklist parses `tasklist /fo csv /nh` on Windows.
func listProcessesTasklist(ctx context.Context) ([]processInfo, error) {
	out, err := exec.CommandContext(ctx, "tasklist", "/fo", "csv", "/nh").Output()
	if err != nil {
		return nil, fmt.Errorf("无法列出进程（tasklist 执行失败: %v）", err)
	}
	return parseTasklist(string(out)), nil
}

// parseTasklist parses CSV rows: "nginx.exe","1234","Console","1","12,345 K".
// tasklist does not report the command line, so keywords match the image name.
func parseTasklist(out string) []processInfo {
	var procs []processInfo
	for _, line := range strings.Split(out, "\n") {
		fields := strings.Split(strings.TrimSpace(line), ",")
		if len(fields) < 2 {
			continue
		}
		name := strings.Trim(fields[0], `"`)
		// The PID column may be quoted and is never comma-grouped.
		pid, err := strconv.Atoi(strings.Trim(fields[1], `" `))
		if err != nil {
			continue
		}
		procs = append(procs, processInfo{PID: pid, Name: name})
	}
	return procs
}

// processesMatching returns the processes that satisfy all keywords.
func processesMatching(procs []processInfo, keywords []string) []processInfo {
	if len(keywords) == 0 {
		return nil
	}
	var out []processInfo
	for _, p := range procs {
		if p.Matches(keywords) {
			out = append(out, p)
		}
	}
	return out
}

// processesWithPID returns the process with the given PID, if it exists.
func processesWithPID(procs []processInfo, pid int) (processInfo, bool) {
	for _, p := range procs {
		if p.PID == pid {
			return p, true
		}
	}
	return processInfo{}, false
}
