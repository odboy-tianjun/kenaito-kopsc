package platform

import (
	"errors"
	"testing"
)

// fakeHardwareProbe builds a probe backed by an in-memory file table.
func fakeHardwareProbe(goos string, cpus int, files map[string]string) hardwareProbe {
	return hardwareProbe{
		GOOS:   goos,
		NumCPU: cpus,
		ReadFile: func(path string) ([]byte, error) {
			content, ok := files[path]
			if !ok {
				return nil, errors.New("no such file")
			}
			return []byte(content), nil
		},
		Run: func(name string, args ...string) ([]byte, error) {
			if content, ok := files[name+" "+joinArgs(args)]; ok {
				return []byte(content), nil
			}
			return nil, errors.New("command not found")
		},
	}
}

func joinArgs(args []string) string {
	out := ""
	for i, a := range args {
		if i > 0 {
			out += " "
		}
		out += a
	}
	return out
}

func TestDetectResourcesLinuxHost(t *testing.T) {
	p := fakeHardwareProbe("linux", 8, map[string]string{
		"/proc/meminfo": "MemTotal:       16384000 kB\nMemFree:  100 kB\n",
	})
	res := p.detect()
	if res.CPUs != 8 {
		t.Errorf("CPUs = %d, want 8", res.CPUs)
	}
	if want := int64(16384000) * 1024; res.Memory != want {
		t.Errorf("Memory = %d, want %d", res.Memory, want)
	}
	if res.Detected != "/proc/meminfo" {
		t.Errorf("Detected = %q", res.Detected)
	}
}

func TestDetectResourcesLinuxCgroupV2Wins(t *testing.T) {
	p := fakeHardwareProbe("linux", 64, map[string]string{
		"/proc/meminfo":             "MemTotal:       65536000 kB\n",
		"/sys/fs/cgroup/memory.max": "4294967296\n",    // 4 GiB
		"/sys/fs/cgroup/cpu.max":    "200000 100000\n", // 2 CPUs
	})
	res := p.detect()
	if res.Memory != 4<<30 {
		t.Errorf("Memory = %d, want 4GiB (cgroup limit must win over the host)", res.Memory)
	}
	if res.Detected != "cgroup v2 memory.max" {
		t.Errorf("Detected = %q", res.Detected)
	}
	if res.CPUs != 2 {
		t.Errorf("CPUs = %d, want 2 (cgroup quota)", res.CPUs)
	}
}

func TestDetectResourcesLinuxCgroupUnlimited(t *testing.T) {
	p := fakeHardwareProbe("linux", 4, map[string]string{
		"/proc/meminfo":             "MemTotal:       8192000 kB\n",
		"/sys/fs/cgroup/memory.max": "max\n",
		"/sys/fs/cgroup/cpu.max":    "max 100000\n",
	})
	res := p.detect()
	if want := int64(8192000) * 1024; res.Memory != want {
		t.Errorf("Memory = %d, want the host total %d", res.Memory, want)
	}
	if res.CPUs != 4 {
		t.Errorf("CPUs = %d, want the host count 4", res.CPUs)
	}
}

func TestDetectResourcesLinuxCgroupV1(t *testing.T) {
	p := fakeHardwareProbe("linux", 32, map[string]string{
		"/proc/meminfo": "MemTotal:       33554432 kB\n",
		"/sys/fs/cgroup/memory/memory.limit_in_bytes": "2147483648\n", // 2 GiB
		"/sys/fs/cgroup/cpu/cpu.cfs_quota_us":         "150000\n",
		"/sys/fs/cgroup/cpu/cpu.cfs_period_us":        "100000\n",
	})
	res := p.detect()
	if res.Memory != 2<<30 {
		t.Errorf("Memory = %d, want 2GiB", res.Memory)
	}
	if res.CPUs != 2 {
		t.Errorf("CPUs = %d, want 2 (1.5 CPU quota rounds up)", res.CPUs)
	}
}

func TestDetectResourcesDarwin(t *testing.T) {
	p := fakeHardwareProbe("darwin", 10, map[string]string{
		"sysctl -n hw.memsize": "17179869184\n", // 16 GiB
	})
	res := p.detect()
	if res.Memory != 16<<30 {
		t.Errorf("Memory = %d, want 16GiB", res.Memory)
	}
	if res.Detected != "hw.memsize" || res.CPUs != 10 {
		t.Errorf("res = %+v", res)
	}
}

func TestDetectResourcesWindows(t *testing.T) {
	p := fakeHardwareProbe("windows",
		12,
		map[string]string{
			"wmic ComputerSystem get TotalPhysicalMemory /value": "\nTotalPhysicalMemory=8589934592\n\n",
		})
	res := p.detect()
	if res.Memory != 8<<30 {
		t.Errorf("Memory = %d, want 8GiB", res.Memory)
	}
}

func TestDetectResourcesUnknownMemory(t *testing.T) {
	p := fakeHardwareProbe("linux", 2, map[string]string{})
	res := p.detect()
	if res.Memory != 0 {
		t.Errorf("Memory = %d, want 0 when nothing can be read", res.Memory)
	}
	if res.CPUs != 2 {
		t.Errorf("CPUs = %d, want 2", res.CPUs)
	}
}

func TestParseMemTotal(t *testing.T) {
	if got := parseMemTotal("MemTotal:       16384000 kB\n"); got != int64(16384000)*1024 {
		t.Errorf("parseMemTotal = %d", got)
	}
	if got := parseMemTotal("garbage\n"); got != 0 {
		t.Errorf("parseMemTotal(garbage) = %d, want 0", got)
	}
}

func TestParseCgroupLimit(t *testing.T) {
	cases := []struct {
		in   string
		want int64
		ok   bool
	}{
		{"max", 0, false},
		{"", 0, false},
		{"9223372036854771712", 0, false}, // cgroup v1 "unlimited"
		{"4294967296", 4294967296, true},
	}
	for _, tc := range cases {
		got, ok := parseCgroupLimit(tc.in)
		if got != tc.want || ok != tc.ok {
			t.Errorf("parseCgroupLimit(%q) = (%d, %v), want (%d, %v)", tc.in, got, ok, tc.want, tc.ok)
		}
	}
}

func TestCeilDiv(t *testing.T) {
	cases := []struct {
		a, b int64
		want int
	}{
		{200000, 100000, 2},
		{150000, 100000, 2},
		{50000, 100000, 1},
	}
	for _, tc := range cases {
		if got := ceilDiv(tc.a, tc.b); got != tc.want {
			t.Errorf("ceilDiv(%d, %d) = %d, want %d", tc.a, tc.b, got, tc.want)
		}
	}
}

func TestLinuxCgroupCPUQuotaBeatsHostCores(t *testing.T) {
	// A 64-core host with a container limited to 4 CPUs must report 4.
	p := fakeHardwareProbe("linux", 64, map[string]string{
		"/proc/meminfo":          "MemTotal:       65536000 kB\n",
		"/sys/fs/cgroup/cpu.max": "400000 100000\n",
	})
	if res := p.detect(); res.CPUs != 4 {
		t.Fatalf("CPUs = %d, want 4 (cgroup quota)", res.CPUs)
	}
}

func TestLinuxCgroupCPUFractionalQuotaRoundsUp(t *testing.T) {
	// 50% of one CPU still needs one core's worth of threads.
	p := fakeHardwareProbe("linux", 8, map[string]string{
		"/proc/meminfo":          "MemTotal:       8192000 kB\n",
		"/sys/fs/cgroup/cpu.max": "50000 100000\n",
	})
	if res := p.detect(); res.CPUs != 1 {
		t.Fatalf("CPUs = %d, want 1", res.CPUs)
	}
}

func TestLinuxCgroupCPUUnlimitedKeepsHostCores(t *testing.T) {
	p := fakeHardwareProbe("linux", 12, map[string]string{
		"/proc/meminfo":          "MemTotal:       8192000 kB\n",
		"/sys/fs/cgroup/cpu.max": "max 100000\n",
	})
	if res := p.detect(); res.CPUs != 12 {
		t.Fatalf("CPUs = %d, want the host count 12", res.CPUs)
	}
}

func TestLinuxCgroupV1QuotaBeatsHostCores(t *testing.T) {
	p := fakeHardwareProbe("linux", 32, map[string]string{
		"/proc/meminfo":                        "MemTotal:       8192000 kB\n",
		"/sys/fs/cgroup/cpu/cpu.cfs_quota_us":  "300000\n",
		"/sys/fs/cgroup/cpu/cpu.cfs_period_us": "100000\n",
	})
	if res := p.detect(); res.CPUs != 3 {
		t.Fatalf("CPUs = %d, want 3", res.CPUs)
	}
}

func TestLinuxCgroupV1NegativeQuotaMeansUnlimited(t *testing.T) {
	p := fakeHardwareProbe("linux", 6, map[string]string{
		"/proc/meminfo":                       "MemTotal:       8192000 kB\n",
		"/sys/fs/cgroup/cpu/cpu.cfs_quota_us": "-1\n",
	})
	if res := p.detect(); res.CPUs != 6 {
		t.Fatalf("CPUs = %d, want the host count 6 (quota -1 means unlimited)", res.CPUs)
	}
}

// fakeJavaProbe returns canned output for specific java invocations.
func fakeJavaProbe(versionStderr, propsStderr string) javaProbe {
	return javaProbe{
		LookPath: func(name string) (string, error) {
			if name != "java" {
				return "", errors.New("not found")
			}
			return "/usr/bin/java", nil
		},
		Run: func(name string, args ...string) ([]byte, []byte, error) {
			for _, a := range args {
				if a == "-XshowSettings:properties" {
					return nil, []byte(propsStderr), nil
				}
			}
			return nil, []byte(versionStderr), nil
		},
	}
}

func TestDetectJavaVersions(t *testing.T) {
	cases := []struct {
		name    string
		banner  string
		want    string
		rawWant string
	}{
		{"jdk8", `java version "1.8.0_392"` + "\n", "8", "1.8.0_392"},
		{"jdk11", `openjdk version "11.0.21" 2023-10-17`, "11", "11.0.21"},
		{"jdk17", `openjdk version "17.0.9" 2023-10-17`, "17", "17.0.9"},
		{"jdk21", `openjdk version "21.0.1" 2023-10-17 LTS`, "21", "21.0.1"},
		{"vendor banner without 'version'", `openjdk 21.0.1 2023-10-17 LTS`, "21", "21.0.1"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			info := fakeJavaProbe(tc.banner, "").detectAt("/usr/bin/java")
			if info.Version != tc.want {
				t.Errorf("Version = %q, want %q", info.Version, tc.want)
			}
			if info.Raw != tc.rawWant {
				t.Errorf("Raw = %q, want %q", info.Raw, tc.rawWant)
			}
			if info.Path != "/usr/bin/java" {
				t.Errorf("Path = %q", info.Path)
			}
		})
	}
}

func TestDetectJavaFallsBackToProperties(t *testing.T) {
	// An unparsable banner: kopsc must use the properties dump instead.
	props := "    java.specification.version = 17\n    java.version = 17.0.9\n"
	info := fakeJavaProbe("weird banner without a version", props).detectAt("/opt/jdk/bin/java")
	if info.Version != "17" {
		t.Fatalf("Version = %q, want 17 from java.specification.version", info.Version)
	}
}

func TestDetectJavaMissing(t *testing.T) {
	probe := javaProbe{
		LookPath: func(string) (string, error) { return "", errors.New("not found") },
	}
	if info := probe.detect(); info.Path != "" || info.Version != "" {
		t.Fatalf("info = %+v, want empty when java is not installed", info)
	}
}

func TestNormalizeJavaVersion(t *testing.T) {
	cases := map[string]string{
		"1.8.0_392": "8",
		"1.8.0":     "8",
		"8":         "8",
		"11.0.21":   "11",
		"17":        "17",
		"21.0.1":    "21",
		"24.0.2":    "24",
		"":          "",
		"garbage":   "",
	}
	for in, want := range cases {
		if got := normalizeJavaVersion(in); got != want {
			t.Errorf("normalizeJavaVersion(%q) = %q, want %q", in, got, want)
		}
	}
}

func TestParseJavaRelease(t *testing.T) {
	cases := map[string]string{
		`java version "1.8.0_392"`:      "1.8.0_392",
		`openjdk version "17.0.9"`:      "17.0.9",
		`openjdk 21.0.1 2023-10-17 LTS`: "21.0.1",
		`no version here`:               "",
	}
	for in, want := range cases {
		if got := parseJavaRelease(in); got != want {
			t.Errorf("parseJavaRelease(%q) = %q, want %q", in, got, want)
		}
	}
}

// TestProbesAreInjectable documents the contract that keeps these detectors
// testable: nothing in detect()/detectAt() touches the real host directly.
func TestProbesAreInjectable(t *testing.T) {
	// A hardware probe with no dependencies wired must not panic and must
	// report what the injected values say.
	hw := hardwareProbe{
		GOOS:   "linux",
		NumCPU: 3,
		ReadFile: func(string) ([]byte, error) {
			return nil, errors.New("denied")
		},
	}
	if res := hw.detect(); res.CPUs != 3 || res.Memory != 0 {
		t.Fatalf("injected probe = %+v, want CPUs 3 and unknown memory", res)
	}

	// A java probe whose LookPath and Run are both faked needs no JVM.
	jp := javaProbe{
		LookPath: func(string) (string, error) { return "/fake/java", nil },
		Run: func(string, ...string) ([]byte, []byte, error) {
			return nil, []byte(`openjdk version "21.0.1"`), nil
		},
	}
	if info := jp.detect(); info.Version != "21" || info.Path != "/fake/java" {
		t.Fatalf("injected java probe = %+v", info)
	}
}
