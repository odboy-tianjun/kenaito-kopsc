// Package platform detects the execution environment that kopsc uses to pick
// the most specific command variant: OS/arch, Linux distribution and available
// container runtimes.
package platform

import (
	"bufio"
	"bytes"
	"os"
	"os/exec"
	"runtime"
	"strings"
)

// OSReleasePath is where Linux distributions publish their identity.
const OSReleasePath = "/etc/os-release"

// Info is the detected environment.
type Info struct {
	OS         string   // runtime.GOOS, e.g. "linux", "darwin", "windows"
	Arch       string   // runtime.GOARCH, e.g. "amd64", "arm64"
	Distro     string   // lower-case os-release ID, empty when unknown
	DistroLike []string // lower-case os-release ID_LIKE entries
	Runtimes   []string // container runtimes found on PATH, e.g. "docker", "podman"
}

// Probe gathers Info. Its dependencies are injectable so detection can be
// exercised without touching the real host.
type Probe struct {
	GOOS              string
	GOARCH            string
	ReadFile          func(string) ([]byte, error)
	LookPath          func(string) (string, error)
	RuntimeCandidates []string
}

// System returns a Probe wired to the real process environment.
func System() Probe {
	return Probe{
		GOOS:              runtime.GOOS,
		GOARCH:            runtime.GOARCH,
		ReadFile:          os.ReadFile,
		LookPath:          exec.LookPath,
		RuntimeCandidates: []string{"docker", "podman"},
	}
}

// Detect inspects the environment. A missing or unreadable os-release file is
// not an error: it simply leaves Distro empty, which resolution treats as
// "unknown distribution".
func (p Probe) Detect() Info {
	info := Info{OS: p.GOOS, Arch: p.GOARCH}

	if p.GOOS == "linux" && p.ReadFile != nil {
		if data, err := p.ReadFile(OSReleasePath); err == nil {
			info.Distro, info.DistroLike = ParseOSRelease(data)
		}
	}

	candidates := p.RuntimeCandidates
	if candidates == nil {
		candidates = []string{"docker", "podman"}
	}
	if p.LookPath != nil {
		for _, name := range candidates {
			if _, err := p.LookPath(name); err == nil {
				info.Runtimes = append(info.Runtimes, name)
			}
		}
	}

	return info
}

// ParseOSRelease extracts ID and ID_LIKE from os-release content. Values are
// lower-cased and unquoted; parsing never fails, malformed lines are skipped.
func ParseOSRelease(data []byte) (distro string, like []string) {
	values := make(map[string]string)
	scanner := bufio.NewScanner(bytes.NewReader(data))
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		key, value, ok := strings.Cut(line, "=")
		if !ok {
			continue
		}
		values[strings.TrimSpace(key)] = strings.Trim(strings.TrimSpace(value), `"'`)
	}

	distro = strings.ToLower(strings.TrimSpace(values["ID"]))
	for _, field := range strings.Fields(values["ID_LIKE"]) {
		like = append(like, strings.ToLower(field))
	}
	return distro, like
}

// String renders Info for `kopsc platform`.
func (i Info) String() string {
	var b strings.Builder
	b.WriteString("os:      " + i.OS + "\n")
	b.WriteString("arch:    " + i.Arch + "\n")
	distro := i.Distro
	if distro == "" {
		distro = "-"
	}
	b.WriteString("distro:  " + distro + "\n")
	if len(i.DistroLike) > 0 {
		b.WriteString("like:    " + strings.Join(i.DistroLike, " ") + "\n")
	}
	runtimes := "-"
	if len(i.Runtimes) > 0 {
		runtimes = strings.Join(i.Runtimes, ", ")
	}
	b.WriteString("runtimes: " + runtimes + "\n")
	return b.String()
}
