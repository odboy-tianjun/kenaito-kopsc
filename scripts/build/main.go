// Command build cross-compiles kopsc for the supported platforms.
//
// Usage:
//
//	go run ./scripts/build                          # every supported target
//	go run ./scripts/build -host                    # current OS/arch only
//	go run ./scripts/build -targets linux/amd64,windows/arm64
//	go run ./scripts/build -version 1.2.3 -out bin
//
// Output names are kopsc-<version>-<os>-<arch>[.exe] under -out (default: dist).
package main

import (
	"errors"
	"flag"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
)

type target struct {
	os   string
	arch string
}

// defaultTargets is the release matrix.
var defaultTargets = []target{
	{"linux", "amd64"},
	{"linux", "arm64"},
	{"darwin", "amd64"},
	{"darwin", "arm64"},
	{"windows", "amd64"},
	{"windows", "arm64"},
}

func main() {
	if err := run(os.Args[1:]); err != nil {
		fmt.Fprintln(os.Stderr, "build: "+err.Error())
		os.Exit(1)
	}
}

func run(args []string) error {
	fs := flag.NewFlagSet("build", flag.ContinueOnError)
	fs.SetOutput(os.Stderr)

	var (
		outDir     = fs.String("out", "dist", "output directory")
		versionArg = fs.String("version", "", "version string (default: git describe --tags --always --dirty)")
		targetArg  = fs.String("targets", "", "comma-separated os/arch list, e.g. linux/amd64,windows/arm64")
		hostOnly   = fs.Bool("host", false, "build only the current os/arch")
		noStrip    = fs.Bool("no-strip", false, "keep symbols (do not pass -s -w)")
		quiet      = fs.Bool("quiet", false, "only print errors")
	)
	if err := fs.Parse(args); err != nil {
		return err
	}
	if fs.NArg() > 0 {
		return fmt.Errorf("unexpected arguments: %s", strings.Join(fs.Args(), " "))
	}

	var (
		targets []target
		err     error
	)
	switch {
	case *hostOnly && *targetArg != "":
		return errors.New("-host and -targets cannot be used together")
	case *hostOnly:
		targets = []target{{runtime.GOOS, runtime.GOARCH}}
	default:
		targets, err = parseTargets(*targetArg)
		if err != nil {
			return err
		}
	}

	version := *versionArg
	if version == "" {
		version = detectVersion()
	}

	if err := os.MkdirAll(*outDir, 0o755); err != nil {
		return fmt.Errorf("create output directory: %w", err)
	}

	logf := func(format string, a ...any) {
		if !*quiet {
			fmt.Printf(format, a...)
		}
	}
	logf("kopsc %s -> %s/\n", version, filepath.ToSlash(*outDir))

	for _, t := range targets {
		path, size, err := build(t, version, *outDir, !*noStrip)
		if err != nil {
			return fmt.Errorf("%s/%s: %w", t.os, t.arch, err)
		}
		logf("  %-8s %-6s %-42s %6.1f MiB\n", t.os, t.arch, path, float64(size)/(1<<20))
	}

	logf("done: %d target(s)\n", len(targets))
	return nil
}

// parseTargets parses "os/arch,..." and falls back to the release matrix.
func parseTargets(spec string) ([]target, error) {
	if strings.TrimSpace(spec) == "" {
		return defaultTargets, nil
	}

	var targets []target
	seen := make(map[target]bool)
	for _, item := range strings.Split(spec, ",") {
		item = strings.TrimSpace(item)
		if item == "" {
			continue
		}
		osName, arch, ok := strings.Cut(item, "/")
		osName, arch = strings.TrimSpace(osName), strings.TrimSpace(arch)
		if !ok || osName == "" || arch == "" {
			return nil, fmt.Errorf("invalid target %q, want os/arch such as linux/amd64", item)
		}
		t := target{osName, arch}
		if seen[t] {
			continue
		}
		seen[t] = true
		targets = append(targets, t)
	}
	if len(targets) == 0 {
		return nil, errors.New("no targets to build")
	}
	return targets, nil
}

// artifactName returns the file name for a target.
func artifactName(version string, t target) string {
	name := fmt.Sprintf("kopsc-%s-%s-%s", version, t.os, t.arch)
	if t.os == "windows" {
		name += ".exe"
	}
	return name
}

// build compiles one target and reports the artifact path and size.
func build(t target, version, outDir string, strip bool) (string, int64, error) {
	path := filepath.Join(outDir, artifactName(version, t))

	ldflags := "-X main.version=" + version
	if strip {
		ldflags = "-s -w " + ldflags
	}

	cmd := exec.Command("go", "build",
		"-trimpath",
		"-ldflags", ldflags,
		"-o", path,
		"./cmd/kopsc",
	)
	cmd.Env = append(os.Environ(),
		"CGO_ENABLED=0",
		"GOOS="+t.os,
		"GOARCH="+t.arch,
	)
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr

	if err := cmd.Run(); err != nil {
		return "", 0, fmt.Errorf("go build failed: %w", err)
	}

	info, err := os.Stat(path)
	if err != nil {
		return "", 0, fmt.Errorf("stat artifact: %w", err)
	}
	return path, info.Size(), nil
}

// detectVersion asks git for a version, falling back to "dev".
func detectVersion() string {
	out, err := exec.Command("git", "describe", "--tags", "--always", "--dirty").Output()
	if err != nil {
		return "dev"
	}
	version := strings.TrimSpace(string(out))
	if version == "" {
		return "dev"
	}
	return version
}
