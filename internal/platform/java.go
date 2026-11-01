package platform

import (
	"os/exec"
	"regexp"
	"strconv"
	"strings"
)

// Java describes the JVM kopsc will launch.
type Java struct {
	Path    string // resolved executable, e.g. "/usr/bin/java"; empty when not found
	Version string // normalised release: "8", "11", "17", "21" or "" when unknown
	Raw     string // the full version string, e.g. "17.0.9" (for messages)
}

// javaProbe gathers Java. Executable lookup and command execution are
// injectable so detection can be tested without a JVM installed.
type javaProbe struct {
	LookPath func(string) (string, error)
	Run      func(name string, args ...string) (stdout, stderr []byte, err error)
}

func systemJavaProbe() javaProbe {
	return javaProbe{
		LookPath: exec.LookPath,
		Run: func(name string, args ...string) ([]byte, []byte, error) {
			cmd := exec.Command(name, args...)
			var stdout, stderr strings.Builder
			cmd.Stdout = &stdout
			cmd.Stderr = &stderr
			err := cmd.Run()
			return []byte(stdout.String()), []byte(stderr.String()), err
		},
	}
}

// DetectJava finds the java executable and its release version.
func DetectJava() Java {
	return systemJavaProbe().detect()
}

// DetectJavaAt inspects a specific java executable (used by --java).
func DetectJavaAt(path string) Java {
	probe := systemJavaProbe()
	return probe.detectAt(path)
}

func (p javaProbe) detect() Java {
	path, err := p.LookPath("java")
	if err != nil {
		return Java{}
	}
	return p.detectAt(path)
}

func (p javaProbe) detectAt(path string) Java {
	info := Java{Path: path}

	// `java -version` writes everything to stderr.
	stdout, stderr, _ := p.Run(path, "-version")
	raw := firstNonEmpty(string(stderr), string(stdout))
	info.Raw = parseJavaRelease(raw)
	info.Version = normalizeJavaVersion(info.Raw)

	// Fall back to the properties dump, which is stable across vendors and
	// also available on JVMs that print an unexpected -version banner.
	if info.Version == "" {
		out, errOut, _ := p.Run(path, "-XshowSettings:properties", "-version")
		raw = firstNonEmpty(string(errOut), string(out))
		if v := parseJavaProperty(raw, "java.specification.version"); v != "" {
			info.Raw = v
			info.Version = normalizeJavaVersion(v)
		} else if v := parseJavaProperty(raw, "java.version"); v != "" {
			info.Raw = v
			info.Version = normalizeJavaVersion(v)
		}
	}
	return info
}

func firstNonEmpty(values ...string) string {
	for _, v := range values {
		if strings.TrimSpace(v) != "" {
			return v
		}
	}
	return ""
}

// javaVersionPattern matches the quoted version in `java -version` output:
//
//	openjdk version "17.0.9" 2023-10-17
//	java version "1.8.0_392"
//	openjdk 21.0.1 2023-10-17 LTS   (some vendors omit "version")
var javaVersionPattern = regexp.MustCompile(`(?:version\s+)?"?(\d+(?:\.\d+)*(?:[._]\d+)?)"?`)

// parseJavaRelease extracts the release string from `java -version` output.
func parseJavaRelease(banner string) string {
	m := javaVersionPattern.FindStringSubmatch(banner)
	if len(m) < 2 {
		return ""
	}
	return m[1]
}

// parseJavaProperty reads one java.*system property from
// `java -XshowSettings:properties -version` output:
//
//	java.specification.version = 17
func parseJavaProperty(output, key string) string {
	for _, line := range strings.Split(output, "\n") {
		name, value, ok := strings.Cut(strings.TrimSpace(line), "=")
		if !ok || strings.TrimSpace(name) != key {
			continue
		}
		return strings.TrimSpace(value)
	}
	return ""
}

// normalizeJavaVersion maps a release string to the major feature version:
// "1.8.0_392" -> "8", "17.0.9" -> "17", "21" -> "21". Versions kopsc has no
// tuning profile for are still returned, so callers can reject them with a
// helpful message.
func normalizeJavaVersion(release string) string {
	release = strings.TrimSpace(release)
	if release == "" {
		return ""
	}
	// 1.8.0_392 / 1.8.0
	if strings.HasPrefix(release, "1.") {
		rest := strings.TrimPrefix(release, "1.")
		major := rest
		if i := strings.IndexAny(rest, "._"); i > 0 {
			major = rest[:i]
		}
		if n, err := strconv.Atoi(major); err == nil {
			return strconv.Itoa(n)
		}
		return ""
	}
	major := release
	if i := strings.IndexAny(release, ".-_"); i > 0 {
		major = release[:i]
	}
	if n, err := strconv.Atoi(major); err == nil {
		return strconv.Itoa(n)
	}
	return ""
}
