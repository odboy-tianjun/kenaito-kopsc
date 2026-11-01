package platform

import (
	"bufio"
	"os"
	"os/exec"
	"runtime"
	"strconv"
	"strings"
)

// Resources describes the CPU and memory a process may actually use. Inside a
// container the cgroup limits win over the host's totals, so a JVM run in a
// 2-core / 4 GiB container is not tuned for the 64-core host underneath it.
type Resources struct {
	CPUs     int    // usable logical CPUs, at least 1
	Memory   int64  // usable memory in bytes, 0 when unknown
	Detected string // how Memory was determined, for --print-flags output
}

// hardwareProbe gathers Resources. Both file reads and command execution are
// injectable so the detection can be tested on any host.
type hardwareProbe struct {
	GOOS     string
	ReadFile func(string) ([]byte, error)
	Run      func(name string, args ...string) ([]byte, error)
	NumCPU   int
}

func systemHardwareProbe() hardwareProbe {
	return hardwareProbe{
		GOOS:     runtime.GOOS,
		ReadFile: os.ReadFile,
		Run: func(name string, args ...string) ([]byte, error) {
			return exec.Command(name, args...).Output()
		},
		NumCPU: runtime.NumCPU(),
	}
}

// DetectResources reports the CPUs and memory available to this process.
func DetectResources() Resources {
	return systemHardwareProbe().detect()
}

func (p hardwareProbe) detect() Resources {
	res := Resources{CPUs: p.NumCPU}
	if res.CPUs < 1 {
		res.CPUs = 1
	}

	switch p.GOOS {
	case "linux":
		host := p.linuxMemory()
		res.Memory, res.Detected = p.linuxCgroupMemory(host)
		if cpus, ok := p.linuxCgroupCPU(); ok {
			res.CPUs = cpus
		}
	case "darwin":
		res.Memory = p.darwinMemory()
		res.Detected = "hw.memsize"
	case "windows":
		res.Memory = p.windowsMemory()
		res.Detected = "GlobalMemoryStatusEx"
	}
	if res.Memory < 0 {
		res.Memory = 0
	}
	return res
}

// linuxMemory reads MemTotal from /proc/meminfo.
func (p hardwareProbe) linuxMemory() int64 {
	data, err := p.ReadFile("/proc/meminfo")
	if err != nil {
		return 0
	}
	return parseMemTotal(string(data))
}

// parseMemTotal returns the MemTotal line in bytes (the file reports KiB).
func parseMemTotal(data string) int64 {
	scanner := bufio.NewScanner(strings.NewReader(data))
	for scanner.Scan() {
		fields := strings.Fields(scanner.Text())
		if len(fields) < 2 || fields[0] != "MemTotal:" {
			continue
		}
		kib, err := strconv.ParseInt(fields[1], 10, 64)
		if err != nil {
			return 0
		}
		return kib * 1024
	}
	return 0
}

// linuxCgroupMemory narrows the host total to the container's limit. An
// "unlimited" limit (very large number, or the "max" keyword) is ignored.
func (p hardwareProbe) linuxCgroupMemory(host int64) (int64, string) {
	// cgroup v2
	if limit, ok := p.readCgroupLimit("/sys/fs/cgroup/memory.max"); ok {
		return minNonZero(host, limit), "cgroup v2 memory.max"
	}
	// cgroup v1
	if limit, ok := p.readCgroupLimit("/sys/fs/cgroup/memory/memory.limit_in_bytes"); ok {
		return minNonZero(host, limit), "cgroup v1 memory.limit_in_bytes"
	}
	return host, "/proc/meminfo"
}

// readCgroupLimit reads a cgroup memory limit, returning false when the file is
// missing or the limit means "unlimited".
func (p hardwareProbe) readCgroupLimit(path string) (int64, bool) {
	data, err := p.ReadFile(path)
	if err != nil {
		return 0, false
	}
	return parseCgroupLimit(strings.TrimSpace(string(data)))
}

// unlimitedCgroup is the sentinel cgroup v1 writes for "no limit" (2^63-4096,
// earlier kernels: a page-aligned near-max value).
const unlimitedCgroup = int64(1) << 62

// parseCgroupLimit interprets a cgroup memory limit value.
func parseCgroupLimit(value string) (int64, bool) {
	if value == "" || value == "max" {
		return 0, false
	}
	n, err := strconv.ParseInt(value, 10, 64)
	if err != nil || n <= 0 || n >= unlimitedCgroup {
		return 0, false
	}
	return n, true
}

// linuxCgroupCPU applies the container's CPU quota, so a container limited to
// 2 CPUs is not tuned for the host's 64.
func (p hardwareProbe) linuxCgroupCPU() (int, bool) {
	// cgroup v2: "<quota> <period>" or "max <period>"
	if data, err := p.ReadFile("/sys/fs/cgroup/cpu.max"); err == nil {
		fields := strings.Fields(strings.TrimSpace(string(data)))
		if len(fields) >= 1 && fields[0] != "max" {
			quota, qerr := strconv.ParseInt(fields[0], 10, 64)
			period := int64(100000)
			if len(fields) >= 2 {
				if v, perr := strconv.ParseInt(fields[1], 10, 64); perr == nil && v > 0 {
					period = v
				}
			}
			if qerr == nil && quota > 0 && period > 0 {
				return ceilDiv(quota, period), true
			}
		}
	}
	// cgroup v1: quota in cpu.cfs_quota_us, period in cpu.cfs_period_us
	quotaData, err := p.ReadFile("/sys/fs/cgroup/cpu/cpu.cfs_quota_us")
	if err != nil {
		return 0, false
	}
	quota, err := strconv.ParseInt(strings.TrimSpace(string(quotaData)), 10, 64)
	if err != nil || quota <= 0 {
		return 0, false
	}
	period := int64(100000)
	if periodData, perr := p.ReadFile("/sys/fs/cgroup/cpu/cpu.cfs_period_us"); perr == nil {
		if v, perr := strconv.ParseInt(strings.TrimSpace(string(periodData)), 10, 64); perr == nil && v > 0 {
			period = v
		}
	}
	return ceilDiv(quota, period), true
}

// darwinMemory asks sysctl for hw.memsize.
func (p hardwareProbe) darwinMemory() int64 {
	out, err := p.Run("sysctl", "-n", "hw.memsize")
	if err != nil {
		return 0
	}
	n, err := strconv.ParseInt(strings.TrimSpace(string(out)), 10, 64)
	if err != nil {
		return 0
	}
	return n
}

// windowsMemory reads TotalPhys from GlobalMemoryStatusEx.
func (p hardwareProbe) windowsMemory() int64 {
	out, err := p.Run("wmic", "ComputerSystem", "get", "TotalPhysicalMemory", "/value")
	if err != nil {
		return 0
	}
	return parseWMICMemory(string(out))
}

// parseWMICMemory extracts TotalPhysicalMemory from wmic key=value output.
func parseWMICMemory(out string) int64 {
	for _, line := range strings.Split(out, "\n") {
		key, value, ok := strings.Cut(strings.TrimSpace(line), "=")
		if !ok || strings.TrimSpace(key) != "TotalPhysicalMemory" {
			continue
		}
		n, err := strconv.ParseInt(strings.TrimSpace(value), 10, 64)
		if err != nil {
			return 0
		}
		return n
	}
	return 0
}

// minNonZero returns the smaller positive value.
func minNonZero(a, b int64) int64 {
	if a <= 0 {
		return b
	}
	if b <= 0 {
		return a
	}
	if a < b {
		return a
	}
	return b
}

// ceilDiv is integer division rounding up, used for CPU quotas that are not a
// whole number of cores (quota 150000 / period 100000 = 2 CPUs).
func ceilDiv(a, b int64) int {
	n := (a + b - 1) / b
	if n < 1 {
		return 1
	}
	return int(n)
}
