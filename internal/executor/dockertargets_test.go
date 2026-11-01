//go:build !windows

package executor

import (
	"bytes"
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"gitea.odboy.cn/odboy/kenaito-kopsc/internal/registry"
	"gitea.odboy.cn/odboy/kenaito-kopsc/internal/resolve"
)

func TestMatchContainerRefPrecedence(t *testing.T) {
	refs := []containerRef{
		{ID: "3f2a1b0c9d8e", Names: "web", Image: "nginx:1.25", Status: "Up 2 hours"},
		{ID: "3f2a99aa0011", Names: "worker", Image: "busybox", Status: "Exited (0) 1 day ago"},
		{ID: "aabbccdd1122", Names: "db,primary", Image: "postgres:16", Status: "Exited (0) 3 days ago"},
		{ID: "deadbeef0001", Names: "deadbeef", Image: "alpine", Status: "Exited (1) 1 hour ago"},
	}

	tests := []struct {
		name    string
		target  string
		wantID  string
		wantErr string
	}{
		{name: "exact name", target: "web", wantID: "3f2a1b0c9d8e"},
		{name: "second alias", target: "primary", wantID: "aabbccdd1122"},
		{name: "exact id", target: "aabbccdd1122", wantID: "aabbccdd1122"},
		{name: "unique id prefix", target: "3f2a1b", wantID: "3f2a1b0c9d8e"},
		{name: "unique id prefix two", target: "3f2a99", wantID: "3f2a99aa0011"},
		{name: "name wins over id prefix", target: "deadbeef", wantID: "deadbeef0001"},
		{name: "unknown", target: "nope", wantErr: "本地没有容器"},
		{name: "ambiguous prefix", target: "3f2a", wantErr: "匹配多个容器"},
		{name: "empty target", target: "", wantErr: "本地没有容器"},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			ref, err := matchContainerRef(refs, tc.target)
			if tc.wantErr != "" {
				if err == nil || !strings.Contains(err.Error(), tc.wantErr) {
					t.Fatalf("err = %v, want %q", err, tc.wantErr)
				}
				return
			}
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if ref.ID != tc.wantID {
				t.Fatalf("resolved %q to %q, want %q", tc.target, ref.ID, tc.wantID)
			}
		})
	}
}

func TestContainerRefRunning(t *testing.T) {
	cases := map[string]bool{
		"Up 2 hours":                   true,
		"Up 3 seconds (healthy)":       true,
		"Exited (0) 3 days ago":        false,
		"Created":                      false,
		"Restarting (1) 5 seconds ago": false,
		"":                             false,
	}
	for status, want := range cases {
		if got := (containerRef{Status: status}).Running(); got != want {
			t.Errorf("Status %q: Running() = %v, want %v", status, got, want)
		}
	}
}

func TestListContainersParsesDockerPs(t *testing.T) {
	// The fake script answers `ps -a --format ...` with tab-separated rows,
	// the same shape docker prints with that format.
	logPath := fakeRuntimeScript(t, "",
		"3f2a1b0c9d8e\t/web\tnginx:1.25\tUp 2 hours\n"+
			"aabbccdd1122\tdb\tpostgres:16\tExited (0) 3 days ago", "")

	refs, err := listContainers(context.Background())
	if err != nil {
		t.Fatalf("listContainers: %v", err)
	}
	if len(refs) != 2 {
		t.Fatalf("got %d containers, want 2: %+v", len(refs), refs)
	}
	if refs[0].Names != "web" || refs[1].Names != "db" {
		t.Errorf("names = %q, %q; want web, db (leading / must be stripped)", refs[0].Names, refs[1].Names)
	}
	if !refs[0].Running() || refs[1].Running() {
		t.Errorf("unexpected running state: %+v", refs)
	}
	if calls := calls(t, logPath); len(calls) == 0 || !strings.HasPrefix(calls[0], "ps -a --format") {
		t.Errorf("docker calls = %q, want a `ps -a --format` invocation", calls)
	}
}

func TestDockerRunStreamsOutput(t *testing.T) {
	fakeRuntimeScript(t, "", "", "  version) echo '  Server: 27.0'\n    ;;\n")

	var out bytes.Buffer
	err := dockerRun(context.Background(), []string{"version"}, BuiltinRequest{Stdout: &out, Stderr: &out})
	if err != nil {
		t.Fatalf("dockerRun: %v", err)
	}
	if !strings.Contains(out.String(), "Server: 27.0") {
		t.Errorf("stdout not streamed: %q", out.String())
	}
}

func TestDockerRunPropagatesExitCode(t *testing.T) {
	fakeRuntimeScript(t, "", "", "  version) echo boom >&2; exit 4\n    ;;\n")

	var out bytes.Buffer
	err := dockerRun(context.Background(), []string{"version"}, BuiltinRequest{Stdout: &out, Stderr: &out})
	if err == nil {
		t.Fatal("expected an error")
	}
	if code := ExitCode(err); code != 4 {
		t.Fatalf("exit code = %d, want 4", code)
	}
	if !strings.Contains(out.String(), "boom") {
		t.Errorf("stderr not forwarded: %q", out.String())
	}
}

func TestDockerCaptureReturnsOutput(t *testing.T) {
	fakeRuntimeScript(t, "", "", "  version) echo captured\n    ;;\n")

	got, err := dockerOutput(context.Background(), "version")
	if err != nil {
		t.Fatalf("dockerOutput: %v", err)
	}
	if strings.TrimSpace(got) != "captured" {
		t.Fatalf("output = %q, want captured", got)
	}
}

func TestDockerOutputReportsDockerError(t *testing.T) {
	fakeRuntimeScript(t, "", "", "  version) echo 'daemon not running' >&2; exit 1\n    ;;\n")

	_, err := dockerOutput(context.Background(), "version")
	if err == nil || !strings.Contains(err.Error(), "daemon not running") {
		t.Fatalf("err = %v, want docker's stderr in the message", err)
	}
}

func rmFixture(t *testing.T) string {
	t.Helper()
	return fakeRuntimeScript(t, imageList(),
		"3f2a1b0c9d8e\t/web\tnginx:1.25\tUp 2 hours\n"+
			"aabbccdd1122\tdb\tpostgres:16\tExited (0) 3 days ago", "")
}

func TestDockerRMResolvesNameAndID(t *testing.T) {
	logPath := rmFixture(t)

	// A container name and an ID prefix both reach docker as names. "web" is
	// running in the fixture, so only the stopped container is removable.
	if err := runDockerRM(context.Background(), BuiltinRequest{
		Args: []string{"db", "aabb"}, Stdout: &bytes.Buffer{},
	}); err != nil {
		t.Fatalf("rm: %v", err)
	}

	calls := calls(t, logPath)
	joined := strings.Join(calls, " | ")
	if !strings.Contains(joined, "rm db db") {
		t.Fatalf("docker calls = %q, want `rm db db`", calls)
	}
}

func TestDockerRMUnknownTargetDoesNotCallDocker(t *testing.T) {
	logPath := rmFixture(t)

	err := runDockerRM(context.Background(), BuiltinRequest{Args: []string{"nosuch"}, Stdout: &bytes.Buffer{}})
	if err == nil || !strings.Contains(err.Error(), "本地没有容器") {
		t.Fatalf("err = %v, want unknown-container error", err)
	}
	for _, call := range calls(t, logPath) {
		if strings.HasPrefix(call, "rm ") || call == "rm" {
			t.Fatalf("docker rm must not run for an unknown target, calls = %q", calls(t, logPath))
		}
	}
}

func TestDockerRMRequiresTarget(t *testing.T) {
	rmFixture(t)
	if err := runDockerRM(context.Background(), BuiltinRequest{}); err == nil ||
		!strings.Contains(err.Error(), "缺少容器") {
		t.Fatalf("err = %v, want missing-container error", err)
	}
}

func TestDockerRMIResolvesNameAndID(t *testing.T) {
	logPath := rmFixture(t)

	if err := runDockerRMI(context.Background(), BuiltinRequest{
		Args: []string{"alpine:3.20"}, Stdout: &bytes.Buffer{},
	}); err != nil {
		t.Fatalf("rmi by name: %v", err)
	}
	if err := runDockerRMI(context.Background(), BuiltinRequest{
		Args: []string{"sha256:5ca7b1d5e2fc"}, Stdout: &bytes.Buffer{},
	}); err != nil {
		t.Fatalf("rmi by sha256 ID: %v", err)
	}
	if err := runDockerRMI(context.Background(), BuiltinRequest{
		Args: []string{"1b9a2c"}, Stdout: &bytes.Buffer{},
	}); err != nil {
		t.Fatalf("rmi by ID prefix: %v", err)
	}

	joined := strings.Join(calls(t, logPath), " | ")
	for _, want := range []string{
		"rmi alpine:3.20",
		"rmi nginx:1.25.3", // 5ca7… resolves to its name
		"rmi registry.example.com:5000/team/app:1.0", // 1b9a… likewise
	} {
		if !strings.Contains(joined, want) {
			t.Errorf("docker calls missing %q: %s", want, joined)
		}
	}
}

func TestDockerRMIUntaggedUsesID(t *testing.T) {
	logPath := rmFixture(t)

	// A dangling image has no name, so it is removed by ID.
	if err := runDockerRMI(context.Background(), BuiltinRequest{
		Args: []string{"7d3e5f7a9b1c"}, Stdout: &bytes.Buffer{},
	}); err != nil {
		t.Fatalf("rmi: %v", err)
	}
	if joined := strings.Join(calls(t, logPath), " | "); !strings.Contains(joined, "rmi 7d3e5f7a9b1c") {
		t.Fatalf("docker calls = %s, want the ID", joined)
	}
}

func TestDockerRMIUnknownImageDoesNotCallDocker(t *testing.T) {
	logPath := rmFixture(t)

	err := runDockerRMI(context.Background(), BuiltinRequest{Args: []string{"busybox:1"}, Stdout: &bytes.Buffer{}})
	if err == nil || !strings.Contains(err.Error(), "本地没有镜像") {
		t.Fatalf("err = %v, want unknown-image error", err)
	}
	for _, call := range calls(t, logPath) {
		if strings.HasPrefix(call, "rmi") {
			t.Fatalf("docker rmi must not run for an unknown image, calls = %q", calls(t, logPath))
		}
	}
}

func TestRejectForce(t *testing.T) {
	msg := "no -f"
	for _, args := range [][]string{{"-f"}, {"--force"}, {"--force=true"}, {"-fv"}, {"x", "-f"}} {
		if err := rejectForce(args, msg); err == nil {
			t.Errorf("args %v: expected -f to be rejected", args)
		}
	}
	for _, args := range [][]string{{"web"}, {"--force=false"}, {"--", "-f"}} {
		if err := rejectForce(args, msg); err != nil {
			t.Errorf("args %v: unexpected error %v", args, err)
		}
	}
}

func TestDockerStopAndRestartResolveTargets(t *testing.T) {
	logPath := rmFixture(t)

	if err := runDockerStop(context.Background(), BuiltinRequest{
		Args: []string{"3f2a1b", "-t", "5"}, Stdout: &bytes.Buffer{},
	}); err != nil {
		t.Fatalf("stop: %v", err)
	}
	if err := runDockerRestart(context.Background(), BuiltinRequest{
		Args: []string{"db"}, Stdout: &bytes.Buffer{},
	}); err != nil {
		t.Fatalf("restart: %v", err)
	}

	joined := strings.Join(calls(t, logPath), " | ")
	// The ID prefix resolves to the container name; -t keeps its value.
	if !strings.Contains(joined, "stop web -t 5") {
		t.Errorf("docker calls = %s, want `stop web -t 5`", joined)
	}
	if !strings.Contains(joined, "restart db") {
		t.Errorf("docker calls = %s, want `restart db`", joined)
	}
}

func TestDockerStopUnknownContainerPointsAtPsall(t *testing.T) {
	rmFixture(t)

	err := runDockerStop(context.Background(), BuiltinRequest{Args: []string{"ghost"}, Stdout: &bytes.Buffer{}})
	if err == nil || !strings.Contains(err.Error(), "kopsc docker psall") {
		t.Fatalf("err = %v, want a hint to run `kopsc docker psall`", err)
	}
}

func TestDockerExecDefaultsToInteractive(t *testing.T) {
	logPath := rmFixture(t)

	// The command's own flags (-p, -i) belong to the command, not to docker.
	if err := runDockerExec(context.Background(), BuiltinRequest{
		Args: []string{"web", "sh", "-c", "echo hi"}, Stdout: &bytes.Buffer{},
	}); err != nil {
		t.Fatalf("exec: %v", err)
	}
	joined := strings.Join(calls(t, logPath), " | ")
	if !strings.Contains(joined, "exec -it web -- sh -c echo hi") {
		t.Fatalf("docker calls = %s, want `exec -it web -- sh -c echo hi`", joined)
	}
}

func TestDockerExecKeepsUserOptions(t *testing.T) {
	logPath := rmFixture(t)

	if err := runDockerExec(context.Background(), BuiltinRequest{
		Args: []string{"-u", "0", "-w", "/app", "web", "env"}, Stdout: &bytes.Buffer{},
	}); err != nil {
		t.Fatalf("exec: %v", err)
	}
	joined := strings.Join(calls(t, logPath), " | ")
	if !strings.Contains(joined, "exec -it -u 0 -w /app web -- env") {
		t.Fatalf("docker calls = %s, want the options kept before the container", joined)
	}
}

func TestDockerExecRejectsExplicitInteractiveFlags(t *testing.T) {
	rmFixture(t)

	for _, args := range [][]string{
		{"-it", "web", "sh"},
		{"-i", "web", "sh"},
		{"-t", "web", "sh"},
		{"--interactive", "web", "sh"},
		{"-ti", "web", "sh"},
	} {
		err := runDockerExec(context.Background(), BuiltinRequest{Args: args, Stdout: &bytes.Buffer{}})
		if err == nil || !strings.Contains(err.Error(), "已默认以交互方式") {
			t.Errorf("args %v: err = %v, want the interactive-flag rejection", args, err)
		}
	}
}

func TestDockerExecRequiresContainer(t *testing.T) {
	rmFixture(t)
	if err := runDockerExec(context.Background(), BuiltinRequest{}); err == nil ||
		!strings.Contains(err.Error(), "缺少容器") {
		t.Fatalf("err = %v, want missing-container error", err)
	}
}

func TestExecArgv(t *testing.T) {
	got := strings.Join(execArgv([]string{"-u", "0"}, "web", "sh", "-c", "id"), " ")
	if want := "exec -it -u 0 web -- sh -c id"; got != want {
		t.Fatalf("execArgv = %q, want %q", got, want)
	}
}

// logsScript makes a fake docker print three log lines for `logs`.
func logsScript(t *testing.T) string {
	t.Helper()
	return fakeRuntimeScript(t, "", "", "  logs) printf 'INFO start\\nERROR boom\\nINFO done\\n'\n    ;;\n")
}

func TestDockerLogsStreamsVerbatimWithoutKopscOptions(t *testing.T) {
	logPath := logsScript(t)

	var out bytes.Buffer
	if err := runDockerLogs(context.Background(), BuiltinRequest{
		Args: []string{"web", "-f", "--tail", "10"}, Stdout: &out,
	}); err != nil {
		t.Fatalf("logs: %v", err)
	}
	if !strings.Contains(out.String(), "ERROR boom") {
		t.Errorf("all lines should be shown, got %q", out.String())
	}
	joined := strings.Join(calls(t, logPath), " | ")
	// -f and --tail come after the container and go to docker untouched.
	if !strings.Contains(joined, "logs web -f --tail 10") {
		t.Errorf("docker calls = %s, want `logs web -f --tail 10`", joined)
	}
}

func TestDockerLogsGrepFiltersLines(t *testing.T) {
	logsScript(t)

	var out bytes.Buffer
	if err := runDockerLogs(context.Background(), BuiltinRequest{
		Args: []string{"--grep", "^ERROR", "web"}, Stdout: &out,
	}); err != nil {
		t.Fatalf("logs: %v", err)
	}
	if strings.Contains(out.String(), "INFO") {
		t.Errorf("INFO lines must be filtered out, got %q", out.String())
	}
	if !strings.Contains(out.String(), "ERROR boom") {
		t.Errorf("the matching line is missing: %q", out.String())
	}
}

func TestDockerLogsSaveWritesFile(t *testing.T) {
	logsScript(t)
	path := filepath.Join(t.TempDir(), "sub", "web.log")

	var out bytes.Buffer
	if err := runDockerLogs(context.Background(), BuiltinRequest{
		Args: []string{"--save", path, "web"}, Stdout: &out,
	}); err != nil {
		t.Fatalf("logs: %v", err)
	}
	if !strings.Contains(out.String(), "ERROR boom") {
		t.Errorf("logs should still be shown, got %q", out.String())
	}
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("log file not written: %v", err)
	}
	if string(data) != "INFO start\nERROR boom\nINFO done\n" {
		t.Errorf("log file = %q", data)
	}
}

func TestDockerLogsPassesSinceOnce(t *testing.T) {
	logPath := logsScript(t)

	if err := runDockerLogs(context.Background(), BuiltinRequest{
		Args: []string{"--since", "2h", "--grep", "INFO", "web"}, Stdout: &bytes.Buffer{},
	}); err != nil {
		t.Fatalf("logs: %v", err)
	}
	call := strings.Join(calls(t, logPath), " | ")
	if strings.Count(call, "--since") != 1 || !strings.Contains(call, "logs web --since 2h") {
		t.Errorf("docker calls = %s, want exactly one --since 2h", call)
	}
}

func TestDockerLogsRejectsBadArguments(t *testing.T) {
	logsScript(t)

	tests := []struct {
		args []string
		want string
	}{
		{nil, "缺少容器"},
		{[]string{"--grep"}, "--grep 缺少参数"},
		{[]string{"--grep", "[", "web"}, "正则表达式无效"},
	}
	for _, tc := range tests {
		err := runDockerLogs(context.Background(), BuiltinRequest{Args: tc.args, Stdout: &bytes.Buffer{}})
		if err == nil || !strings.Contains(err.Error(), tc.want) {
			t.Errorf("args %v: err = %v, want %q", tc.args, err, tc.want)
		}
	}
}

func TestParseDockerSize(t *testing.T) {
	cases := map[string]int64{
		"0B":            0,
		"512B":          512,
		"-":             0,
		"":              0,
		"1KB":           1000,
		"1.234GB":       1234000000,
		"1.5GB (25%)":   1500000000,
		"2.1GB (2.1GB)": 2100000000,
		"1KiB":          1024,
		"3 MiB":         3 << 20,
		"1024":          1024,
	}
	for in, want := range cases {
		got, err := parseDockerSize(in)
		if err != nil {
			t.Errorf("parseDockerSize(%q): %v", in, err)
			continue
		}
		if got != want {
			t.Errorf("parseDockerSize(%q) = %d, want %d", in, got, want)
		}
	}
	if _, err := parseDockerSize("lots"); err == nil {
		t.Error("parseDockerSize should reject non-numeric sizes")
	}
}

func TestDockerDFRanksBySize(t *testing.T) {
	logPath := fakeRuntimeScript(t, "", "", "  system) printf 'TYPE\\tTOTAL\\tACTIVE\\tSIZE\\tRECLAIMABLE\\nContainers\\t12\\t3\\t512MB\\t256MB\\nImages\\t30\\t5\\t3.5GB\\t1.2GB\\nLocal Volumes\\t8\\t2\\t900MB\\t900MB\\nBuild Cache\\t40\\t10\\t1.1GB\\t1.1GB\\n'\n    ;;\n")

	var out bytes.Buffer
	if err := runDockerDF(context.Background(), BuiltinRequest{Stdout: &out, Stderr: &out}); err != nil {
		t.Fatalf("df: %v", err)
	}
	lines := strings.Split(strings.TrimSpace(out.String()), "\n")
	// header + 4 rows + total + blank + hint
	if len(lines) < 6 {
		t.Fatalf("unexpected output:\n%s", out.String())
	}
	if !strings.Contains(lines[0], "类型") || !strings.Contains(lines[0], "可回收") {
		t.Errorf("header = %q", lines[0])
	}
	// Largest first: Images (3.5GB) > Build Cache (1.1GB) > Volumes (900MB) > Containers.
	wantOrder := []string{"Images", "Build Cache", "Local Volumes", "Containers"}
	for i, want := range wantOrder {
		if !strings.Contains(lines[i+1], want) {
			t.Errorf("line %d = %q, want %q first", i+1, lines[i+1], want)
		}
	}
	if !strings.Contains(lines[5], "总计") {
		t.Errorf("total line = %q", lines[5])
	}
	if !strings.Contains(out.String(), "可回收") {
		t.Errorf("missing reclaim hint:\n%s", out.String())
	}
	if !strings.Contains(strings.Join(calls(t, logPath), " "), "system df") {
		t.Error("df should call `docker system df`")
	}
}

func TestDockerDFNoReclaimHint(t *testing.T) {
	fakeRuntimeScript(t, "", "", "  system) printf 'TYPE\\tTOTAL\\tACTIVE\\tSIZE\\tRECLAIMABLE\\nImages\\t1\\t1\\t0B\\t0B\\n'\n    ;;\n")

	var out bytes.Buffer
	if err := runDockerDF(context.Background(), BuiltinRequest{Stdout: &out, Stderr: &out}); err != nil {
		t.Fatalf("df: %v", err)
	}
	if !strings.Contains(out.String(), "没有可回收的空间") {
		t.Errorf("output = %q", out.String())
	}
}

func TestDockerDFEmptyOutputIsAnError(t *testing.T) {
	fakeRuntimeScript(t, "", "", "")
	if err := runDockerDF(context.Background(), BuiltinRequest{Stdout: &bytes.Buffer{}}); err == nil ||
		!strings.Contains(err.Error(), "没有返回任何数据") {
		t.Fatalf("err = %v, want empty-output error", err)
	}
}

func statsFixture(t *testing.T) string {
	t.Helper()
	return fakeRuntimeScript(t, "",
		"3f2a1b0c9d8e\tweb\tnginx:1.25\tUp 2 hours\n"+
			"aabbccdd1122\tdb\tpostgres:16\tExited (0) 3 days ago", "")
}

func TestDockerStatsWithoutArgsUsesNoStream(t *testing.T) {
	logPath := statsFixture(t)

	if err := runDockerStats(context.Background(), BuiltinRequest{Stdout: &bytes.Buffer{}}); err != nil {
		t.Fatalf("stats: %v", err)
	}
	joined := strings.Join(calls(t, logPath), " | ")
	if !strings.Contains(joined, "stats --no-stream") {
		t.Fatalf("docker calls = %s, want `stats --no-stream`", joined)
	}
	// Without names docker reports every running container itself; kopsc must
	// not invent a container list.
	if strings.Contains(joined, " ps ") {
		t.Errorf("stats without args should not list containers first: %s", joined)
	}
}

func TestDockerStatsResolvesNamedContainer(t *testing.T) {
	logPath := statsFixture(t)

	if err := runDockerStats(context.Background(), BuiltinRequest{
		Args: []string{"3f2a1b", "--format", "{{.Name}}"}, Stdout: &bytes.Buffer{},
	}); err != nil {
		t.Fatalf("stats: %v", err)
	}
	joined := strings.Join(calls(t, logPath), " | ")
	if !strings.Contains(joined, "stats --no-stream web --format {{.Name}}") {
		t.Fatalf("docker calls = %s, want the resolved name and forwarded --format", joined)
	}
}

func TestDockerStatsRejectsStoppedContainer(t *testing.T) {
	statsFixture(t)

	err := runDockerStats(context.Background(), BuiltinRequest{Args: []string{"db"}, Stdout: &bytes.Buffer{}})
	if err == nil || !strings.Contains(err.Error(), "未在运行") {
		t.Fatalf("err = %v, want not-running error", err)
	}
}

func TestDockerStatsDoesNotDuplicateNoStream(t *testing.T) {
	logPath := statsFixture(t)

	if err := runDockerStats(context.Background(), BuiltinRequest{
		Args: []string{"--no-stream"}, Stdout: &bytes.Buffer{},
	}); err != nil {
		t.Fatalf("stats: %v", err)
	}
	call := strings.Join(calls(t, logPath), " | ")
	if strings.Count(call, "--no-stream") != 1 {
		t.Errorf("--no-stream must appear once, calls = %s", call)
	}
}

func TestDockerPreviewRMListsContainers(t *testing.T) {
	rmFixture(t)

	var out bytes.Buffer
	if err := runDockerPreviewRM(context.Background(), BuiltinRequest{
		Args: []string{"web", "aabb"}, Stdout: &out,
	}); err != nil {
		t.Fatalf("preview: %v", err)
	}
	text := out.String()
	header := strings.Split(strings.TrimSpace(text), "\n")[0]
	for _, col := range []string{"ID", "名称", "镜像", "状态"} {
		if !strings.Contains(header, col) {
			t.Errorf("header %q missing column %q", header, col)
		}
	}
	// Each target is one row: ID, name, image, status.
	for _, want := range []string{"3f2a1b0c9d8e", "web", "nginx:1.25", "Up 2 hours", "db", "postgres:16", "Exited (0) 3 days ago"} {
		if !strings.Contains(text, want) {
			t.Errorf("preview missing %q:\n%s", want, text)
		}
	}
}

func TestDockerPreviewRMRejectsUnknownBeforeYes(t *testing.T) {
	rmFixture(t)
	err := runDockerPreviewRM(context.Background(), BuiltinRequest{Args: []string{"ghost"}, Stdout: &bytes.Buffer{}})
	if err == nil || !strings.Contains(err.Error(), "本地没有容器") {
		t.Fatalf("err = %v, want unknown-container error", err)
	}
}

func TestDockerPreviewRMIUsesNameIDCreatedSize(t *testing.T) {
	rmFixture(t)

	var out bytes.Buffer
	if err := runDockerPreviewRMI(context.Background(), BuiltinRequest{
		Args: []string{"alpine:3.20", "7d3e5f7a9b1c"}, Stdout: &out,
	}); err != nil {
		t.Fatalf("preview: %v", err)
	}
	text := out.String()
	header := strings.Split(strings.TrimSpace(text), "\n")[0]
	for _, col := range []string{"镜像", "ID", "创建", "大小"} {
		if !strings.Contains(header, col) {
			t.Errorf("header %q missing column %q", header, col)
		}
	}
	if !strings.Contains(text, "alpine:3.20") || !strings.Contains(text, "9c6f07244728") {
		t.Errorf("named image row missing:\n%s", text)
	}
	// The dangling image is shown as such rather than with a blank name.
	if !strings.Contains(text, "悬空镜像") {
		t.Errorf("dangling image not marked:\n%s", text)
	}
}

func TestShortID(t *testing.T) {
	cases := map[string]string{
		"sha256:9c6f0724472892f3061e": "9c6f07244728",
		"3f2a1b0c9d8e":                "3f2a1b0c9d8e",
		"abc":                         "abc",
	}
	for in, want := range cases {
		if got := shortID(in); got != want {
			t.Errorf("shortID(%q) = %q, want %q", in, got, want)
		}
	}
}

func TestDockerRMAndRMIRejectForce(t *testing.T) {
	for _, flag := range []string{"-f", "--force", "--force=true"} {
		logPath := rmFixture(t)
		err := runDockerRM(context.Background(), BuiltinRequest{Args: []string{flag, "web"}, Stdout: &bytes.Buffer{}})
		if err == nil || !strings.Contains(err.Error(), "不再接受 -f") || !strings.Contains(err.Error(), "--yes") {
			t.Errorf("rm %s: err = %v, want -f rejection pointing at --yes", flag, err)
		}
		for _, call := range calls(t, logPath) {
			if call == "rm" || strings.HasPrefix(call, "rm ") {
				t.Errorf("rm %s must not call docker, calls = %q", flag, calls(t, logPath))
			}
		}
	}

	logPath := rmFixture(t)
	err := runDockerRMI(context.Background(), BuiltinRequest{Args: []string{"--force", "alpine:3.20"}, Stdout: &bytes.Buffer{}})
	if err == nil || !strings.Contains(err.Error(), "不再接受 -f") {
		t.Fatalf("rmi --force: err = %v, want -f rejection", err)
	}
	for _, call := range calls(t, logPath) {
		if strings.HasPrefix(call, "rmi") {
			t.Errorf("rmi must not call docker, calls = %q", calls(t, logPath))
		}
	}
}

func TestDockerRMRejectsRunningContainer(t *testing.T) {
	logPath := rmFixture(t)

	err := runDockerRM(context.Background(), BuiltinRequest{Args: []string{"web"}, Stdout: &bytes.Buffer{}})
	if err == nil || !strings.Contains(err.Error(), "正在运行") || !strings.Contains(err.Error(), "kopsc docker stop web") {
		t.Fatalf("err = %v, want a hint to stop the container first", err)
	}
	for _, call := range calls(t, logPath) {
		if call == "rm" || strings.HasPrefix(call, "rm ") {
			t.Errorf("docker rm must not run for a running container, calls = %q", calls(t, logPath))
		}
	}

	// A stopped container is removable.
	if err := runDockerRM(context.Background(), BuiltinRequest{Args: []string{"db"}, Stdout: &bytes.Buffer{}}); err != nil {
		t.Fatalf("removing a stopped container: %v", err)
	}
}

func TestDockerPreviewRMMarksRunningContainer(t *testing.T) {
	rmFixture(t)

	var out bytes.Buffer
	if err := runDockerPreviewRM(context.Background(), BuiltinRequest{Args: []string{"web"}, Stdout: &out}); err != nil {
		t.Fatalf("preview: %v", err)
	}
	if !strings.Contains(out.String(), "需先 kopsc docker stop web") {
		t.Errorf("preview should warn about the running container:\n%s", out.String())
	}
}

func TestDryRunNeverTouchesDocker(t *testing.T) {
	for _, name := range []string{"docker.rm", "docker.rmi", "docker.stop", "docker.exec", "docker.logs", "docker.logs --grep"} {
		logPath := rmFixture(t)
		builtinName, args := name, []string{"web"}
		if name == "docker.rmi" {
			args = []string{"alpine:3.20"}
		}
		if name == "docker.logs --grep" {
			builtinName, args = "docker.logs", []string{"--grep", "ERROR", "web"}
		}

		fn, ok := LookupBuiltin(builtinName)
		if !ok {
			t.Fatalf("builtin %q not registered", builtinName)
		}
		// A dry-run is handled before any builtin runs; verify the executor's
		// contract directly.
		plan := Plan{Result: resolveResultWithVariant(registryVariantBuiltinFor(builtinName)), ExtraArgs: args}
		var out bytes.Buffer
		if err := plan.Run(context.Background(), Options{DryRun: true, Stdout: &out}); err != nil {
			t.Fatalf("%s dry-run: %v", builtinName, err)
		}
		if !strings.Contains(out.String(), "[dry-run]") {
			t.Errorf("%s: dry-run output missing marker: %q", builtinName, out.String())
		}
		if got := calls(t, logPath); len(got) != 0 {
			t.Errorf("%s: dry-run must not call docker, calls = %q", builtinName, got)
		}
		_ = fn
	}
}

// registryVariantBuiltinFor builds the resolved variant kopsc would pick for
// the given builtin name, so direct Plan tests exercise the real path.
func registryVariantBuiltinFor(name string) registry.Variant {
	return registry.Variant{Builtin: name, Runtime: "docker", Match: registry.Match{OS: "linux"}}
}

func resolveResultWithVariant(v registry.Variant) resolve.Result {
	return resolve.Result{Command: &registry.Command{ID: "docker.test"}, Variant: v}
}
