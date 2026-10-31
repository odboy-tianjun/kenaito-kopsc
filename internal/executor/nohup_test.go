package executor

import (
	"bytes"
	"context"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
	"time"
)

func TestParseNohupArgsSplitsOptionsFromCommand(t *testing.T) {
	tests := []struct {
		name string
		args []string
		want nohupOptions
	}{
		{
			name: "command only",
			args: []string{"./server", "-port", "8080"},
			want: nohupOptions{command: []string{"./server", "-port", "8080"}},
		},
		{
			name: "all options",
			args: []string{"-o", "a.log", "-C", "/var/app", "-E", "K=V", "--detach", "-q", "-w", "./run.sh", "x"},
			want: nohupOptions{
				command: []string{"./run.sh", "x"},
				outPath: "a.log",
				dir:     "/var/app",
				env:     []string{"K=V"},
				detach:  true,
				quiet:   true,
				wait:    true,
			},
		},
		{
			name: "equals forms",
			args: []string{"--output=out.log", "--dir=/tmp", "--env=A=1", "--detach=false", "sleep", "1"},
			want: nohupOptions{command: []string{"sleep", "1"}, outPath: "out.log", dir: "/tmp", env: []string{"A=1"}},
		},
		{
			name: "no-detach wins when last",
			args: []string{"--detach", "--no-detach", "cmd"},
			want: nohupOptions{command: []string{"cmd"}},
		},
		{
			name: "command flags are not parsed",
			args: []string{"ffmpeg", "-o", "out.mp4", "--wait", "-i", "in.mp4"},
			want: nohupOptions{command: []string{"ffmpeg", "-o", "out.mp4", "--wait", "-i", "in.mp4"}},
		},
		{
			name: "double dash ends options",
			args: []string{"-o", "x.log", "--", "-weird-name", "--detach"},
			want: nohupOptions{outPath: "x.log", command: []string{"-weird-name", "--detach"}},
		},
	}
	for _, tc := range tests {
		got, err := parseNohupArgs(tc.args)
		if err != nil {
			t.Errorf("%s: unexpected error %v", tc.name, err)
			continue
		}
		if !reflect.DeepEqual(got, tc.want) {
			t.Errorf("%s: got %+v, want %+v", tc.name, got, tc.want)
		}
	}
}

func TestParseNohupArgsRejectsBadInput(t *testing.T) {
	tests := []struct {
		args []string
		want string
	}{
		{nil, "必须提供要后台运行的命令"},
		{[]string{"-o"}, "-o 缺少参数"},
		{[]string{"-o", "a.log"}, "必须提供要后台运行的命令"},
		{[]string{"-E", "NOEQUALS", "cmd"}, "NAME=VALUE"},
		{[]string{"--detach=maybe", "cmd"}, "需要布尔值"},
		{[]string{"--unknown", "cmd"}, "未知选项"},
	}
	for _, tc := range tests {
		_, err := parseNohupArgs(tc.args)
		if err == nil || !strings.Contains(err.Error(), tc.want) {
			t.Errorf("args %v: err = %v, want %q", tc.args, err, tc.want)
		}
	}
}

func TestNohupRunsDetachedAndAppendsLog(t *testing.T) {
	dir := t.TempDir()
	logPath := filepath.Join(dir, "output.log")

	// First launch writes the log.
	if _, err := runNohupForTest(t, dir, "-o", logPath, "true"); err != nil {
		t.Fatalf("first launch: %v", err)
	}
	if _, err := os.Stat(logPath); err != nil {
		t.Fatalf("first launch did not create %s: %v", logPath, err)
	}
	if data, _ := os.ReadFile(logPath); len(data) != 0 {
		t.Fatalf("expected empty log from true, got %q", data)
	}

	// Second launch appends instead of truncating.
	if _, err := runNohupForTest(t, dir, "-o", logPath, "echo", "second"); err != nil {
		t.Fatalf("second launch: %v", err)
	}
	waitForFile(t, logPath, "second")
	if got := readFileString(logPath); got != "second\n" {
		t.Fatalf("log = %q, want %q", got, "second\n")
	}
}

func TestNohupCapturesStderrIntoLog(t *testing.T) {
	dir := t.TempDir()
	logPath := filepath.Join(dir, "err.log")

	out, err := runNohupForTest(t, dir, "-o", logPath, "-w", "sh", "-c", "echo out; echo err >&2")
	if err != nil {
		t.Fatalf("launch: %v", err)
	}
	log := readFileString(logPath)
	if !strings.Contains(log, "out") || !strings.Contains(log, "err") {
		t.Fatalf("log = %q, want both stdout and stderr", log)
	}
	// --wait relays the same content to the caller's streams.
	if !strings.Contains(out, "out") || !strings.Contains(out, "err") {
		t.Fatalf("relayed output = %q", out)
	}
	if !strings.Contains(out, "退出") {
		t.Fatalf("summary missing wait note: %q", out)
	}
}

func TestNohupWaitPropagatesExitCode(t *testing.T) {
	dir := t.TempDir()
	logPath := filepath.Join(dir, "exit.log")

	_, err := runNohupForTest(t, dir, "-o", logPath, "-w", "sh", "-c", "exit 7")
	if err == nil {
		t.Fatal("expected an exit error from --wait")
	}
	if code := ExitCode(err); code != 7 {
		t.Fatalf("ExitCode(err) = %d, want 7", code)
	}
}

func TestNohupUsesDirEnvAndDefaultLog(t *testing.T) {
	dir := t.TempDir()

	// The default log lives in the working directory and is named after the
	// program, like nohup.out.
	if _, err := runNohupForTest(t, dir, "-C", dir, "-E", "KOPSC_TEST_VALUE=xyz", "-w",
		"sh", "-c", `printf "%s %s\n" "$(pwd)" "$KOPSC_TEST_VALUE"`); err != nil {
		t.Fatalf("launch: %v", err)
	}

	entries, err := os.ReadDir(dir)
	if err != nil {
		t.Fatal(err)
	}
	var logName string
	for _, entry := range entries {
		if strings.HasSuffix(entry.Name(), ".out") {
			logName = entry.Name()
		}
	}
	if logName != "sh.out" {
		t.Fatalf("expected default log sh.out, got %q", logName)
	}
	log := readFileString(filepath.Join(dir, logName))
	if !strings.Contains(log, "KOPSC_TEST_VALUE") || !strings.Contains(log, dir) {
		// pwd of a symlinked temp dir (macOS /var -> /private/var) differs.
		if !strings.Contains(log, "xyz") {
			t.Fatalf("log = %q, want the env value and working directory", log)
		}
	}
}

func TestNohupQuietHidesSummary(t *testing.T) {
	dir := t.TempDir()
	out, err := runNohupForTest(t, dir, "-q", "-o", filepath.Join(dir, "q.log"), "true")
	if err != nil {
		t.Fatalf("launch: %v", err)
	}
	if out != "" {
		t.Fatalf("quiet launch printed %q", out)
	}
}

func TestNohupReportsMissingProgram(t *testing.T) {
	_, err := runNohupForTest(t, t.TempDir(), "kopsc-definitely-not-a-program")
	if err == nil || !strings.Contains(err.Error(), "找不到可执行文件") {
		t.Fatalf("err = %v, want missing-program error", err)
	}
}

// runNohupForTest runs the builtin with a fresh context and returns stdout.
func runNohupForTest(t *testing.T, dir string, args ...string) (string, error) {
	t.Helper()
	var out bytes.Buffer
	// The builtin reads the *current* directory when -C is absent, and the
	// tests that check the default log name rely on that.
	old, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	if err := os.Chdir(dir); err != nil {
		t.Fatal(err)
	}
	defer func() { _ = os.Chdir(old) }()

	err = runNohup(context.Background(), BuiltinRequest{Args: args, Stdout: &out, Stderr: &out})
	return out.String(), err
}

func readFileString(path string) string {
	data, _ := os.ReadFile(path)
	return string(data)
}

// waitForFile polls until path contains want (the child writes asynchronously).
func waitForFile(t *testing.T, path, want string) {
	t.Helper()
	for i := 0; i < 200; i++ {
		if strings.Contains(readFileString(path), want) {
			return
		}
		time.Sleep(10 * time.Millisecond)
	}
	t.Fatalf("%s never contained %q (content %q)", path, want, readFileString(path))
}
