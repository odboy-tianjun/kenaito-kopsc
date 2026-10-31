package cli

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"gitea.odboy.cn/odboy/kenaito-kopsc/internal/platform"
	"gitea.odboy.cn/odboy/kenaito-kopsc/internal/registry"
)

func centos(runtimes ...string) platform.Info {
	return platform.Info{
		OS: "linux", Arch: "amd64",
		Distro: "centos", DistroLike: []string{"rhel", "fedora"},
		Runtimes: runtimes,
	}
}

func testApp(info platform.Info) (*App, *bytes.Buffer, *bytes.Buffer) {
	stdout, stderr := &bytes.Buffer{}, &bytes.Buffer{}
	return &App{
		Registry: registry.Builtin(),
		Info:     info,
		Version:  "test",
		Stdin:    strings.NewReader(""),
		Stdout:   stdout,
		Stderr:   stderr,
	}, stdout, stderr
}

func TestLeafDryRun(t *testing.T) {
	app, stdout, stderr := testApp(centos("docker"))

	code := app.Execute([]string{"docker", "clearsafe", "--dry-run"})
	if code != 0 {
		t.Fatalf("exit code = %d, stderr=%s", code, stderr.String())
	}
	out := stdout.String()
	for _, want := range []string{"[dry-run]", "docker system prune -f", "CentOS 上的 Docker 运行时", "依据"} {
		if !strings.Contains(out, want) {
			t.Errorf("dry-run output missing %q:\n%s", want, out)
		}
	}
}

func TestLeafDryRunPrefersPodman(t *testing.T) {
	app, stdout, _ := testApp(centos("docker", "podman"))

	if code := app.Execute([]string{"docker", "clearsafe", "--dry-run"}); code != 0 {
		t.Fatalf("exit code = %d", code)
	}
	if !strings.Contains(stdout.String(), "podman system prune -f") {
		t.Errorf("expected podman variant:\n%s", stdout.String())
	}
	if strings.Contains(stdout.String(), "docker system prune -f") {
		t.Errorf("docker variant should not be chosen when podman is present:\n%s", stdout.String())
	}
}

func TestLeafHelp(t *testing.T) {
	app, stdout, _ := testApp(centos("docker"))

	if code := app.Execute([]string{"docker", "clearsafe", "-h"}); code != 0 {
		t.Fatalf("exit code = %d", code)
	}
	out := stdout.String()
	for _, want := range []string{"用法: kopsc docker clearsafe", "安全清理", "--dry-run", "将执行: docker system prune -f"} {
		if !strings.Contains(out, want) {
			t.Errorf("help output missing %q:\n%s", want, out)
		}
	}
}

func TestLeafPassesThroughArgs(t *testing.T) {
	app, stdout, _ := testApp(centos("docker"))

	code := app.Execute([]string{"net", "ports", "--dry-run", "--", "-l"})
	if code != 0 {
		t.Fatalf("exit code = %d", code)
	}
	if !strings.Contains(stdout.String(), "ss -tulnp -l") {
		t.Errorf("passthrough args missing:\n%s", stdout.String())
	}
}

func TestLeafNoMatch(t *testing.T) {
	app, _, stderr := testApp(centos()) // no container runtime

	code := app.Execute([]string{"docker", "clearsafe"})
	if code != 1 {
		t.Fatalf("exit code = %d, want 1", code)
	}
	if !strings.Contains(stderr.String(), "不支持当前平台") {
		t.Errorf("stderr lacks explanation:\n%s", stderr.String())
	}
	if !strings.Contains(stderr.String(), "kopsc explain") {
		t.Errorf("stderr lacks hint:\n%s", stderr.String())
	}
}

func TestList(t *testing.T) {
	app, stdout, _ := testApp(centos("docker"))

	if code := app.Execute([]string{"list"}); code != 0 {
		t.Fatalf("exit code = %d", code)
	}
	out := stdout.String()
	for _, want := range []string{"docker.clearsafe", "net.ports", "pkg.refreshindex", "可用", "docker system prune -f"} {
		if !strings.Contains(out, want) {
			t.Errorf("list output missing %q:\n%s", want, out)
		}
	}
}

func TestListMarksUnavailable(t *testing.T) {
	app, stdout, _ := testApp(platform.Info{OS: "plan9", Arch: "mips"})

	if code := app.Execute([]string{"list"}); code != 0 {
		t.Fatalf("exit code = %d", code)
	}
	if !strings.Contains(stdout.String(), "不可用") {
		t.Errorf("unavailable commands not marked:\n%s", stdout.String())
	}
}

func TestExplain(t *testing.T) {
	app, stdout, _ := testApp(centos("docker", "podman"))

	if code := app.Execute([]string{"explain", "docker.clearsafe"}); code != 0 {
		t.Fatalf("exit code = %d", code)
	}
	out := stdout.String()
	for _, want := range []string{
		"命令: docker.clearsafe",
		"平台:",
		"命中变体 #0",
		"候选:",
		"✓",
		"✗",
		"podman system prune -f",
		"distro=centos",
	} {
		if !strings.Contains(out, want) {
			t.Errorf("explain output missing %q:\n%s", want, out)
		}
	}
}

func TestExplainUnavailableStillListsCandidates(t *testing.T) {
	app, stdout, _ := testApp(centos()) // no runtime

	if code := app.Execute([]string{"explain", "docker.clearsafe"}); code != 0 {
		t.Fatalf("exit code = %d", code)
	}
	out := stdout.String()
	if !strings.Contains(out, "当前平台不可用") {
		t.Errorf("missing unavailable marker:\n%s", out)
	}
	if strings.Count(out, "✗") != len(registry.Builtin().Commands[0].Variants) {
		t.Errorf("expected every candidate to be listed:\n%s", out)
	}
}

func TestExplainUnknownCommand(t *testing.T) {
	app, _, stderr := testApp(centos("docker"))

	if code := app.Execute([]string{"explain", "nope"}); code != 1 {
		t.Fatalf("exit code = %d, want 1", code)
	}
	if !strings.Contains(stderr.String(), "未知命令") {
		t.Errorf("stderr = %s", stderr.String())
	}
}

func TestPlatform(t *testing.T) {
	app, stdout, _ := testApp(centos("docker", "podman"))

	if code := app.Execute([]string{"platform"}); code != 0 {
		t.Fatalf("exit code = %d", code)
	}
	out := stdout.String()
	for _, want := range []string{"linux", "amd64", "centos", "rhel fedora", "docker, podman"} {
		if !strings.Contains(out, want) {
			t.Errorf("platform output missing %q:\n%s", want, out)
		}
	}
}

func TestVersion(t *testing.T) {
	app, stdout, _ := testApp(centos("docker"))

	if code := app.Execute([]string{"version"}); code != 0 {
		t.Fatalf("exit code = %d", code)
	}
	if !strings.Contains(stdout.String(), "kopsc test linux/amd64") {
		t.Errorf("version output = %s", stdout.String())
	}
}

func TestGroupHelp(t *testing.T) {
	app, stdout, _ := testApp(centos("docker"))

	if code := app.Execute([]string{"docker"}); code != 0 {
		t.Fatalf("exit code = %d", code)
	}
	if !strings.Contains(stdout.String(), "clearsafe") {
		t.Errorf("group help missing subcommand:\n%s", stdout.String())
	}
}

func TestParseLeafArgs(t *testing.T) {
	tests := []struct {
		name       string
		args       []string
		wantDryRun bool
		wantExtra  []string
		wantHelp   bool
	}{
		{"empty", nil, false, nil, false},
		{"dry-run short", []string{"-n"}, true, nil, false},
		{"dry-run long", []string{"--dry-run"}, true, nil, false},
		{"dry-run false", []string{"--dry-run=false"}, false, nil, false},
		{"help", []string{"-h"}, false, nil, true},
		{"passthrough", []string{"--", "--dry-run", "-h"}, false, []string{"--dry-run", "-h"}, false},
		{"unknown flags forwarded", []string{"--filter", "until=1h"}, false, []string{"--filter", "until=1h"}, false},
		{"mixed", []string{"--dry-run", "arg", "--", "-n"}, true, []string{"arg", "-n"}, false},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			dryRun, extra, help := parseLeafArgs(tc.args)
			if dryRun != tc.wantDryRun || help != tc.wantHelp {
				t.Errorf("dryRun=%v help=%v, want %v/%v", dryRun, help, tc.wantDryRun, tc.wantHelp)
			}
			if strings.Join(extra, "|") != strings.Join(tc.wantExtra, "|") {
				t.Errorf("extra = %v, want %v", extra, tc.wantExtra)
			}
		})
	}
}

func TestDiskUsageCommandThroughCLI(t *testing.T) {
	root := t.TempDir()
	if err := os.WriteFile(filepath.Join(root, "a.bin"), bytes.Repeat([]byte("x"), 4096), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(filepath.Join(root, "sub"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, "sub", "b.bin"), bytes.Repeat([]byte("x"), 1024), 0o644); err != nil {
		t.Fatal(err)
	}

	app, stdout, stderr := testApp(centos("docker"))
	if code := app.Execute([]string{"disk", "usage", root}); code != 0 {
		t.Fatalf("exit code = %d, stderr=%s", code, stderr.String())
	}
	out := stdout.String()
	for _, want := range []string{"4.0K", "1.0K", "a.bin", "sub", "总计 " + root} {
		if !strings.Contains(out, want) {
			t.Errorf("disk usage output missing %q:\n%s", want, out)
		}
	}
}

func TestDiskUsageAcceptsPathFlagThroughCLI(t *testing.T) {
	root := t.TempDir()

	app, stdout, stderr := testApp(centos("docker"))
	if code := app.Execute([]string{"disk", "usage", "--path", root}); code != 0 {
		t.Fatalf("exit code = %d, stderr=%s", code, stderr.String())
	}
	if !strings.Contains(stdout.String(), "总计 "+root) {
		t.Errorf("unexpected output:\n%s", stdout.String())
	}
}

func TestDiskUsageRequiresPathThroughCLI(t *testing.T) {
	app, _, stderr := testApp(centos("docker"))

	if code := app.Execute([]string{"disk", "usage"}); code != 1 {
		t.Fatalf("exit code = %d, want 1", code)
	}
	if !strings.Contains(stderr.String(), "必须提供路径") {
		t.Errorf("stderr = %s", stderr.String())
	}
}

func TestDiskUsageDryRunDoesNotTouchPath(t *testing.T) {
	app, stdout, _ := testApp(centos("docker"))

	code := app.Execute([]string{"disk", "usage", "/path/that/does/not/exist", "--dry-run"})
	if code != 0 {
		t.Fatalf("exit code = %d", code)
	}
	out := stdout.String()
	if !strings.Contains(out, "(内置) disk.usage /path/that/does/not/exist") {
		t.Errorf("dry-run output missing builtin plan:\n%s", out)
	}
	if strings.Contains(out, "无法访问") {
		t.Errorf("dry-run should not execute the builtin:\n%s", out)
	}
}

func TestDiskUsageHelp(t *testing.T) {
	app, stdout, _ := testApp(centos("docker"))

	if code := app.Execute([]string{"disk", "usage", "-h"}); code != 0 {
		t.Fatalf("exit code = %d", code)
	}
	if !strings.Contains(stdout.String(), "(内置) disk.usage") {
		t.Errorf("help output missing builtin target:\n%s", stdout.String())
	}
}

// TestCLIHelperProcess is re-executed as a child to verify exit-code passthrough.
func TestCLIHelperProcess(t *testing.T) {
	if os.Getenv("KOPSC_CLI_HELPER") != "1" {
		return
	}
	os.Exit(9)
}

func TestExecutionPreservesExitCode(t *testing.T) {
	t.Setenv("KOPSC_CLI_HELPER", "1")

	app, _, _ := testApp(centos("docker"))
	app.Registry = &registry.Registry{Commands: []registry.Command{{
		ID:      "demo.fail",
		Summary: "always fails",
		Variants: []registry.Variant{{
			Match: registry.Match{OS: "linux"},
			Argv:  []string{os.Args[0], "-test.run=TestCLIHelperProcess"},
		}},
	}}}

	if code := app.Execute([]string{"demo", "fail"}); code != 9 {
		t.Fatalf("exit code = %d, want 9", code)
	}
}
