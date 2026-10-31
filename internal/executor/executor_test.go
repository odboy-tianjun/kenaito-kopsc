package executor

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	"gitea.odboy.cn/odboy/kenaito-kopsc/internal/platform"
	"gitea.odboy.cn/odboy/kenaito-kopsc/internal/registry"
	"gitea.odboy.cn/odboy/kenaito-kopsc/internal/resolve"
)

func samplePlan(argv ...string) Plan {
	if len(argv) == 0 {
		argv = []string{"docker", "system", "prune", "-f"}
	}
	return Plan{Result: resolve.Result{
		Command: &registry.Command{ID: "docker.clearsafe", Summary: "安全清理"},
		Info: platform.Info{
			OS: "linux", Arch: "amd64", Distro: "centos",
			DistroLike: []string{"rhel"}, Runtimes: []string{"docker"},
		},
		Variant:    registry.Variant{Argv: argv, Note: "CentOS 上的 Docker 运行时"},
		VariantIdx: 0,
		Score:      5,
		Candidates: []resolve.Candidate{{
			Index: 0, Matched: true, Score: 5,
			Reason: "匹配 os=linux, distro=centos (+3), runtime=docker (+2)",
		}},
	}}
}

func TestDryRunOnlyPrints(t *testing.T) {
	var out bytes.Buffer
	err := samplePlan().Run(context.Background(), Options{DryRun: true, Stdout: &out})
	if err != nil {
		t.Fatalf("dry-run returned error: %v", err)
	}

	text := out.String()
	for _, want := range []string{
		"[dry-run]",
		"docker system prune -f",
		"linux/amd64",
		"distro=centos",
		"变体 #0",
		"CentOS 上的 Docker 运行时",
		"依据",
	} {
		if !strings.Contains(text, want) {
			t.Errorf("dry-run output missing %q:\n%s", want, text)
		}
	}
}

func TestDryRunNeverExecutes(t *testing.T) {
	// A missing program must not matter in dry-run mode.
	err := samplePlan("kopsc-program-that-does-not-exist").Run(context.Background(), Options{
		DryRun: true,
		Stdout: io.Discard,
	})
	if err != nil {
		t.Fatalf("dry-run should not attempt execution: %v", err)
	}
}

func TestArgvAppendsExtraArgs(t *testing.T) {
	plan := samplePlan("docker", "system", "prune", "-f")
	plan.ExtraArgs = []string{"--filter", "until=24h"}
	want := []string{"docker", "system", "prune", "-f", "--filter", "until=24h"}
	if got := plan.Argv(); strings.Join(got, " ") != strings.Join(want, " ") {
		t.Fatalf("Argv() = %v, want %v", got, want)
	}
	if !strings.Contains(plan.Display(), "--filter until=24h") {
		t.Fatalf("Display() = %q", plan.Display())
	}
}

// TestHelperProcess is re-executed as a child by the tests below.
func TestHelperProcess(t *testing.T) {
	if os.Getenv("KOPSC_TEST_HELPER") != "1" {
		return
	}
	fmt.Fprintln(os.Stdout, "helper-stdout", strings.Join(os.Args[1:], " "))
	fmt.Fprintln(os.Stderr, "helper-stderr")
	os.Exit(7)
}

func TestRunPropagatesStdioAndExitCode(t *testing.T) {
	t.Setenv("KOPSC_TEST_HELPER", "1")

	plan := samplePlan(os.Args[0], "-test.run=TestHelperProcess", "--")
	plan.ExtraArgs = []string{"forwarded"}

	var stdout, stderr bytes.Buffer
	err := plan.Run(context.Background(), Options{Stdout: &stdout, Stderr: &stderr})
	if err == nil {
		t.Fatal("expected a non-zero exit error")
	}
	var exitErr *ExitError
	if !errors.As(err, &exitErr) {
		t.Fatalf("error type = %T, want *ExitError", err)
	}
	if exitErr.Code != 7 {
		t.Errorf("exit code = %d, want 7", exitErr.Code)
	}
	if ExitCode(err) != 7 {
		t.Errorf("ExitCode(err) = %d, want 7", ExitCode(err))
	}
	if !strings.Contains(stdout.String(), "helper-stdout") {
		t.Errorf("child stdout not forwarded:\n%s", stdout.String())
	}
	if !strings.Contains(stdout.String(), "forwarded") {
		t.Errorf("extra args not forwarded:\n%s", stdout.String())
	}
	if !strings.Contains(stderr.String(), "helper-stderr") {
		t.Errorf("child stderr not forwarded:\n%s", stderr.String())
	}
}

func TestRunFindsProgramOnPATH(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("shell script fixture is not executable on Windows")
	}

	dir := t.TempDir()
	script := filepath.Join(dir, "docker")
	body := "#!/bin/sh\necho fake-docker \"$@\"\nexit 3\n"
	if err := os.WriteFile(script, []byte(body), 0o755); err != nil {
		t.Fatalf("write fixture: %v", err)
	}
	t.Setenv("PATH", dir+string(os.PathListSeparator)+os.Getenv("PATH"))

	var out bytes.Buffer
	err := samplePlan().Run(context.Background(), Options{Stdout: &out, Stderr: io.Discard})
	if ExitCode(err) != 3 {
		t.Fatalf("exit code = %d, want 3 (err=%v)", ExitCode(err), err)
	}
	if !strings.Contains(out.String(), "fake-docker system prune -f") {
		t.Errorf("fixture output missing:\n%s", out.String())
	}
}

func TestRunNotFound(t *testing.T) {
	err := samplePlan("kopsc-program-that-does-not-exist").Run(context.Background(), Options{
		Stdout: io.Discard,
	})
	if err == nil {
		t.Fatal("expected an error for a missing program")
	}
	if ExitCode(err) != 1 {
		t.Errorf("ExitCode() = %d, want 1", ExitCode(err))
	}
}

func TestShellVariant(t *testing.T) {
	plan := samplePlan("echo", "hello from shell")
	plan.Result.Variant.Shell = true

	var out bytes.Buffer
	if err := plan.Run(context.Background(), Options{Stdout: &out, Stderr: io.Discard}); err != nil {
		t.Fatalf("shell run failed: %v", err)
	}
	if !strings.Contains(out.String(), "hello from shell") {
		t.Errorf("shell output missing:\n%s", out.String())
	}

	display := plan.Display()
	wantShell := "sh -c"
	if runtime.GOOS == "windows" {
		wantShell = "cmd /c"
	}
	if !strings.HasPrefix(display, wantShell) {
		t.Errorf("Display() = %q, want prefix %q", display, wantShell)
	}
}

func TestQuoteForShell(t *testing.T) {
	unixCases := map[string]string{
		"plain":        "plain",
		"with space":   "'with space'",
		"it's":         `'it'\''s'`,
		"":             "''",
		"/usr/bin/x":   "/usr/bin/x",
		"--filter=a,b": "--filter=a,b",
	}
	for in, want := range unixCases {
		if got := quoteForShell(in, "linux"); got != want {
			t.Errorf("quoteForShell(%q, linux) = %q, want %q", in, got, want)
		}
	}

	windowsCases := map[string]string{
		"plain":      "plain",
		"with space": `"with space"`,
	}
	for in, want := range windowsCases {
		if got := quoteForShell(in, "windows"); got != want {
			t.Errorf("quoteForShell(%q, windows) = %q, want %q", in, got, want)
		}
	}
}

func TestExitCode(t *testing.T) {
	if got := ExitCode(nil); got != 0 {
		t.Errorf("ExitCode(nil) = %d, want 0", got)
	}
	if got := ExitCode(errors.New("boom")); got != 1 {
		t.Errorf("ExitCode(generic) = %d, want 1", got)
	}
	if got := ExitCode(&ExitError{Code: 42}); got != 42 {
		t.Errorf("ExitCode(ExitError) = %d, want 42", got)
	}
}
