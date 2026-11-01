package cli

import (
	"bytes"
	"os"
	"path/filepath"
	"runtime"
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
	for _, want := range []string{"[dry-run]", "docker system prune -f", "Linux 上的 Docker 运行时", "依据"} {
		if !strings.Contains(out, want) {
			t.Errorf("dry-run output missing %q:\n%s", want, out)
		}
	}
}

func TestLeafDryRunPicksDockerVariant(t *testing.T) {
	app, stdout, _ := testApp(centos("docker"))

	if code := app.Execute([]string{"docker", "clearsafe", "--dry-run"}); code != 0 {
		t.Fatalf("exit code = %d", code)
	}
	if !strings.Contains(stdout.String(), "docker system prune -f") {
		t.Errorf("expected the docker variant:\n%s", stdout.String())
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
	// net.ports is a builtin now, so use a docker command whose argv is fixed
	// and whose passthrough the dry-run output shows.
	app, stdout, _ := testApp(centos("docker"))

	code := app.Execute([]string{"docker", "ps", "--dry-run", "--", "--filter", "status=running"})
	if code != 0 {
		t.Fatalf("exit code = %d", code)
	}
	out := stdout.String()
	if !strings.Contains(out, "docker ps") || !strings.Contains(out, "--filter status=running") {
		t.Errorf("passthrough args missing:\n%s", out)
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
	app, stdout, _ := testApp(centos("docker"))

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
		"docker system prune -f",
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
	app, stdout, _ := testApp(centos("docker"))

	if code := app.Execute([]string{"platform"}); code != 0 {
		t.Fatalf("exit code = %d", code)
	}
	out := stdout.String()
	for _, want := range []string{"linux", "amd64", "centos", "rhel fedora", "docker"} {
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
	plain := &registry.Command{ID: "docker.logs"}
	namespaced := &registry.Command{ID: "diagnose", Namespaced: true}

	tests := []struct {
		name       string
		cmd        *registry.Command
		args       []string
		wantDryRun bool
		wantYes    bool
		wantExtra  []string
		wantHelp   bool
		wantErr    bool
	}{
		{"empty", plain, nil, false, false, nil, false, false},
		{"plain -n is dry-run", plain, []string{"-n"}, true, false, nil, false, false},
		{"dry-run long", plain, []string{"--dry-run"}, true, false, nil, false, false},
		{"dry-run false", plain, []string{"--dry-run=false"}, false, false, nil, false, false},
		{"yes long", plain, []string{"--yes"}, false, true, nil, false, false},
		{"yes short", plain, []string{"-y"}, false, true, nil, false, false},
		{"help", plain, []string{"-h"}, false, false, nil, true, false},
		{"passthrough", plain, []string{"--", "--dry-run", "-h", "--yes"}, false, false, []string{"--dry-run", "-h", "--yes"}, false, false},
		{"unknown flags forwarded", plain, []string{"--filter", "until=1h"}, false, false, []string{"--filter", "until=1h"}, false, false},
		{"mixed", plain, []string{"--dry-run", "arg", "--", "-n"}, true, false, []string{"arg", "-n"}, false, false},

		{"namespaced -n", namespaced, []string{"api-0", "-n", "prod"}, false, false, []string{"api-0", "--namespace", "prod"}, false, false},
		{"namespaced -n=value", namespaced, []string{"api-0", "-n=prod"}, false, false, []string{"api-0", "--namespace=prod"}, false, false},
		{"namespaced -N", namespaced, []string{"api-0", "-N", "prod"}, false, false, []string{"api-0", "--namespace", "prod"}, false, false},
		{"namespaced --namespace", namespaced, []string{"api-0", "--namespace", "prod"}, false, false, []string{"api-0", "--namespace", "prod"}, false, false},
		{"namespaced --namespace=value", namespaced, []string{"api-0", "--namespace=prod"}, false, false, []string{"api-0", "--namespace=prod"}, false, false},
		{"namespaced --dry-run still previews", namespaced, []string{"api-0", "--dry-run"}, true, false, []string{"api-0"}, false, false},
		{"namespaced -n without value", namespaced, []string{"api-0", "-n"}, false, false, nil, false, true},
		{"namespaced passthrough keeps -n", namespaced, []string{"--", "-n", "prod"}, false, false, []string{"-n", "prod"}, false, false},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			dryRun, yes, extra, help, err := parseLeafArgs(tc.cmd, tc.args)
			if tc.wantErr {
				if err == nil {
					t.Fatal("expected an error")
				}
				return
			}
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if dryRun != tc.wantDryRun || yes != tc.wantYes || help != tc.wantHelp {
				t.Errorf("dryRun=%v yes=%v help=%v, want %v/%v/%v",
					dryRun, yes, help, tc.wantDryRun, tc.wantYes, tc.wantHelp)
			}
			if strings.Join(extra, "|") != strings.Join(tc.wantExtra, "|") {
				t.Errorf("extra = %v, want %v", extra, tc.wantExtra)
			}
		})
	}
}

func TestDestructiveCommandRequiresYes(t *testing.T) {
	app, stdout, stderr := testApp(centos("docker"))

	code := app.Execute([]string{"docker", "clearall"})
	if code != 2 {
		t.Fatalf("exit code = %d, want 2", code)
	}
	out := stdout.String()
	for _, want := range []string{"破坏性命令", "将执行: docker system prune -a -f", "影响", "kopsc docker clearall --yes"} {
		if !strings.Contains(out, want) {
			t.Errorf("confirmation output missing %q:\n%s", want, out)
		}
	}
	if stderr.Len() != 0 {
		t.Errorf("unexpected stderr: %s", stderr.String())
	}
}

func TestDestructiveCommandDryRunNeedsNoYes(t *testing.T) {
	app, stdout, _ := testApp(centos("docker"))

	if code := app.Execute([]string{"docker", "prunevolumes", "--dry-run"}); code != 0 {
		t.Fatalf("exit code = %d, want 0", code)
	}
	if !strings.Contains(stdout.String(), "docker volume prune -f") {
		t.Errorf("dry-run output:\n%s", stdout.String())
	}
}

func TestYesIsNotForwarded(t *testing.T) {
	app, stdout, _ := testApp(centos("docker"))

	if code := app.Execute([]string{"docker", "clearall", "--yes", "--dry-run"}); code != 0 {
		t.Fatalf("exit code = %d, want 0", code)
	}
	out := stdout.String()
	if strings.Contains(out, "--yes") {
		t.Errorf("--yes leaked into the forwarded args:\n%s", out)
	}
	if !strings.Contains(out, "docker system prune -a -f") {
		t.Errorf("unexpected dry-run output:\n%s", out)
	}
}

func TestMissingRequiredArgs(t *testing.T) {
	app, _, stderr := testApp(centos("docker"))

	if code := app.Execute([]string{"docker", "logs"}); code != 1 {
		t.Fatalf("exit code = %d, want 1", code)
	}
	msg := stderr.String()
	for _, want := range []string{"缺少参数", "kopsc docker logs <容器>"} {
		if !strings.Contains(msg, want) {
			t.Errorf("stderr missing %q:\n%s", want, msg)
		}
	}
}

func TestMissingArgsWhenOnlyOptionsGiven(t *testing.T) {
	app, _, stderr := testApp(centos("docker"))

	if code := app.Execute([]string{"docker", "rm", "-f"}); code != 1 {
		t.Fatalf("exit code = %d, want 1", code)
	}
	if !strings.Contains(stderr.String(), "缺少参数") {
		t.Errorf("stderr = %s", stderr.String())
	}
}

func TestDestructiveHelpMentionsYes(t *testing.T) {
	app, stdout, _ := testApp(centos("docker"))

	if code := app.Execute([]string{"docker", "clearall", "-h"}); code != 0 {
		t.Fatalf("exit code = %d", code)
	}
	out := stdout.String()
	if !strings.Contains(out, "--yes") || !strings.Contains(out, "状态码 2") {
		t.Errorf("help missing --yes documentation:\n%s", out)
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

func TestDiskUsageDefaultsToCurrentDirectoryThroughCLI(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "payload.bin"), bytes.Repeat([]byte("x"), 2048), 0o644); err != nil {
		t.Fatal(err)
	}
	old, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	if err := os.Chdir(dir); err != nil {
		t.Fatal(err)
	}
	defer func() { _ = os.Chdir(old) }()

	app, stdout, stderr := testApp(centos("docker"))
	if code := app.Execute([]string{"disk", "usage"}); code != 0 {
		t.Fatalf("exit code = %d, stderr=%s", code, stderr.String())
	}
	if !strings.Contains(stdout.String(), "payload.bin") {
		t.Errorf("omitting the path should analyse the current directory:\n%s", stdout.String())
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

func TestDiagnoseDryRun(t *testing.T) {
	app, stdout, stderr := testApp(centos("docker"))

	if code := app.Execute([]string{"diagnose", "web", "--dry-run"}); code != 0 {
		t.Fatalf("exit code = %d, stderr=%s", code, stderr.String())
	}
	out := stdout.String()
	for _, want := range []string{"[dry-run]", "(内置) diagnose web", "内置实现"} {
		if !strings.Contains(out, want) {
			t.Errorf("dry-run output missing %q:\n%s", want, out)
		}
	}
}

func TestDiagnoseNamespaceTranslationThroughCLI(t *testing.T) {
	app, stdout, _ := testApp(centos("docker"))

	if code := app.Execute([]string{"diagnose", "api-0", "-n", "prod", "--dry-run"}); code != 0 {
		t.Fatalf("exit code = %d", code)
	}
	if !strings.Contains(stdout.String(), "api-0 --namespace prod") {
		t.Errorf("-n was not translated to --namespace:\n%s", stdout.String())
	}
}

func TestDiagnoseRequiresTarget(t *testing.T) {
	app, _, stderr := testApp(centos("docker"))

	if code := app.Execute([]string{"diagnose"}); code != 1 {
		t.Fatalf("exit code = %d, want 1", code)
	}
	if !strings.Contains(stderr.String(), "缺少参数") {
		t.Errorf("stderr = %s", stderr.String())
	}
}

func TestNetPidsDryRun(t *testing.T) {
	app, stdout, stderr := testApp(centos("docker"))

	if code := app.Execute([]string{"net", "pids", "8080", "--dry-run"}); code != 0 {
		t.Fatalf("exit code = %d, stderr=%s", code, stderr.String())
	}
	out := stdout.String()
	if !strings.Contains(out, "(内置) net.pids 8080") {
		t.Errorf("dry-run output missing builtin plan:\n%s", out)
	}
}

func TestNetPidsRequiresPort(t *testing.T) {
	app, _, stderr := testApp(centos("docker"))

	if code := app.Execute([]string{"net", "pids"}); code != 1 {
		t.Fatalf("exit code = %d, want 1", code)
	}
	if !strings.Contains(stderr.String(), "缺少参数") {
		t.Errorf("stderr = %s", stderr.String())
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

func TestProcNohupThroughCLI(t *testing.T) {
	dir := t.TempDir()
	logPath := filepath.Join(dir, "run.log")

	app, stdout, stderr := testApp(centos("docker"))
	// A command-local -n must reach the child: kopsc's leaf parser disables
	// flag parsing for this command.
	if code := app.Execute([]string{"proc", "nohup", "-o", logPath, "-w", "sh", "-c", "echo out; echo err >&2"}); code != 0 {
		t.Fatalf("exit code = %d, stderr=%s", code, stderr.String())
	}
	out := stdout.String()
	if !strings.Contains(out, "已在后台启动") || !strings.Contains(out, logPath) {
		t.Errorf("launch summary missing:\n%s", out)
	}
	if !strings.Contains(out, "out") || !strings.Contains(out, "err") {
		t.Errorf("--wait did not relay output:\n%s", out)
	}
	data, err := os.ReadFile(logPath)
	if err != nil {
		t.Fatalf("log not written: %v", err)
	}
	if !strings.Contains(string(data), "out") || !strings.Contains(string(data), "err") {
		t.Errorf("log = %q", data)
	}
}

func TestProcNohupDryRunDoesNotLaunch(t *testing.T) {
	app, stdout, stderr := testApp(centos("docker"))
	if code := app.Execute([]string{"proc", "nohup", "--dry-run", "sh", "-c", "echo hi"}); code != 0 {
		t.Fatalf("exit code = %d, stderr=%s", code, stderr.String())
	}
	if !strings.Contains(stdout.String(), "[dry-run]") {
		t.Errorf("dry-run output missing:\n%s", stdout.String())
	}
}

func TestProcNohupRequiresCommand(t *testing.T) {
	app, _, stderr := testApp(centos("docker"))
	if code := app.Execute([]string{"proc", "nohup", "-q"}); code == 0 {
		t.Fatal("expected a non-zero exit code without a command")
	}
	if !strings.Contains(stderr.String(), "缺少参数") {
		t.Errorf("stderr = %q", stderr.String())
	}
}

func TestProcNohupKeepsCommandLocalDryRun(t *testing.T) {
	app, stdout, stderr := testApp(centos("docker"))
	// -n after the program belongs to the program, so kopsc must actually run.
	if code := app.Execute([]string{"proc", "nohup", "-q", "-w", "sh", "-c", "echo ran"}); code != 0 {
		t.Fatalf("exit code = %d, stderr=%s", code, stderr.String())
	}
	if strings.Contains(stdout.String(), "[dry-run]") {
		t.Errorf("command-local arguments were intercepted as kopsc options:\n%s", stdout.String())
	}
}

// fakeRuntimeScript installs a fake docker on PATH that answers
// `images --format` with one image and records every invocation in calls.log.
func fakeRuntimeScript(t *testing.T, images string) string {
	t.Helper()
	dir := t.TempDir()
	logPath := filepath.Join(dir, "calls.log")
	script := "#!/bin/sh\n" +
		"echo \"$*\" >> " + logPath + "\n" +
		"case \"$1\" in\n" +
		"  images) cat <<'EOT'\n" + images + "\nEOT\n" +
		"    ;;\n" +
		"esac\n"
	if err := os.WriteFile(filepath.Join(dir, "docker"), []byte(script), 0o755); err != nil {
		t.Fatal(err)
	}
	t.Setenv("PATH", dir+string(os.PathListSeparator)+os.Getenv("PATH"))
	return logPath
}

func TestDockerSaveThroughCLINamesArchiveAfterImageID(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("fake runtime is a shell script")
	}
	logPath := fakeRuntimeScript(t, "IMAGE ID\tREPOSITORY\tTAG\tDIGEST\naaaa11112222\talpine\t3.20\t")

	app, stdout, stderr := testApp(centos("docker"))
	if code := app.Execute([]string{"docker", "save", "aaaa11112222"}); code != 0 {
		t.Fatalf("exit code = %d, stderr=%s stdout=%s", code, stderr.String(), stdout.String())
	}
	if !strings.Contains(stdout.String(), "alpine_3.20.tar") {
		t.Errorf("summary missing archive name:\n%s", stdout.String())
	}
	data, _ := os.ReadFile(logPath)
	if !strings.Contains(string(data), "save -o alpine_3.20.tar aaaa11112222") {
		t.Errorf("calls = %q", data)
	}
}

func TestDockerLoadThroughCLITakesArchive(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("fake runtime is a shell script")
	}
	archive := filepath.Join(t.TempDir(), "alpine_3.20.tar")
	if err := os.WriteFile(archive, []byte("x"), 0o644); err != nil {
		t.Fatal(err)
	}
	logPath := fakeRuntimeScript(t, "")

	app, _, stderr := testApp(centos("docker"))
	if code := app.Execute([]string{"docker", "load", archive}); code != 0 {
		t.Fatalf("exit code = %d, stderr=%s", code, stderr.String())
	}
	data, _ := os.ReadFile(logPath)
	if !strings.Contains(string(data), "load -i "+archive) {
		t.Errorf("calls = %q", data)
	}
}

func TestDockerLoadThroughCLIRequiresArchive(t *testing.T) {
	app, _, stderr := testApp(centos("docker"))
	if code := app.Execute([]string{"docker", "load"}); code == 0 {
		t.Fatal("expected a non-zero exit code without an archive")
	}
	if !strings.Contains(stderr.String(), "缺少参数") {
		t.Errorf("stderr = %q", stderr.String())
	}
}

// cliRuntime installs a fake docker answering both `images` and `ps`, and
// returns the path of the invocation log.
func cliRuntime(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()
	logPath := filepath.Join(dir, "calls.log")
	script := "#!/bin/sh\n" +
		"echo \"$*\" >> " + logPath + "\n" +
		"case \"$1\" in\n" +
		"  images) cat <<'EOT'\nIMAGE ID\tREPOSITORY\tTAG\tDIGEST\n9c6f07244728\talpine\t3.20\t\nEOT\n" +
		"    ;;\n" +
		"  ps) cat <<'EOT'\n3f2a1b0c9d8e\tweb\tnginx:1.25\tUp 2 hours\nEOT\n" +
		"    ;;\n" +
		"esac\n"
	if err := os.WriteFile(filepath.Join(dir, "docker"), []byte(script), 0o755); err != nil {
		t.Fatal(err)
	}
	t.Setenv("PATH", dir+string(os.PathListSeparator)+os.Getenv("PATH"))
	return logPath
}

func TestDockerExecThroughCLIDefaultsToInteractive(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("fake runtime is a shell script")
	}
	logPath := cliRuntime(t)

	app, _, stderr := testApp(centos("docker"))
	if code := app.Execute([]string{"docker", "exec", "web", "sh", "-c", "echo hi"}); code != 0 {
		t.Fatalf("exit code = %d, stderr=%s", code, stderr.String())
	}
	data, _ := os.ReadFile(logPath)
	if !strings.Contains(string(data), "exec -it web -- sh -c echo hi") {
		t.Errorf("docker calls = %q, want the -it/-- form", data)
	}
}

func TestDockerExecRejectsDashItThroughCLI(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("fake runtime is a shell script")
	}
	cliRuntime(t)

	app, _, stderr := testApp(centos("docker"))
	if code := app.Execute([]string{"docker", "exec", "-it", "web", "sh"}); code == 0 {
		t.Fatal("expected a non-zero exit code for an explicit -it")
	}
	if !strings.Contains(stderr.String(), "已默认以交互方式") {
		t.Errorf("stderr = %q", stderr.String())
	}
}

func TestDockerStopThroughCLIResolvesContainerName(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("fake runtime is a shell script")
	}
	logPath := cliRuntime(t)

	app, _, stderr := testApp(centos("docker"))
	if code := app.Execute([]string{"docker", "stop", "3f2a1b"}); code != 0 {
		t.Fatalf("exit code = %d, stderr=%s", code, stderr.String())
	}
	data, _ := os.ReadFile(logPath)
	if !strings.Contains(string(data), "stop web") {
		t.Errorf("docker calls = %q, want `stop web`", data)
	}
}

func TestDockerRMWithoutYesListsTargetsAndExitsTwo(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("fake runtime is a shell script")
	}
	logPath := cliRuntime(t)

	app, stdout, stderr := testApp(centos("docker"))
	if code := app.Execute([]string{"docker", "rm", "web"}); code != 2 {
		t.Fatalf("exit code = %d, want 2; stderr=%s", code, stderr.String())
	}
	out := stdout.String()
	for _, want := range []string{"将影响:", "3f2a1b0c9d8e", "web", "nginx:1.25", "Up 2 hours", "这是破坏性命令", "--yes"} {
		if !strings.Contains(out, want) {
			t.Errorf("confirmation missing %q:\n%s", want, out)
		}
	}
	for _, call := range readCalls(logPath) {
		if strings.HasPrefix(call, "rm ") {
			t.Fatalf("docker rm must not run without --yes, calls = %v", readCalls(logPath))
		}
	}
}

func readCalls(path string) []string {
	data, _ := os.ReadFile(path)
	var out []string
	for _, line := range strings.Split(string(data), "\n") {
		if line = strings.TrimSpace(line); line != "" {
			out = append(out, line)
		}
	}
	return out
}
