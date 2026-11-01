package executor

import (
	"bytes"
	"context"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

func TestParsePortArgs(t *testing.T) {
	cases := []struct {
		name    string
		args    []string
		want    int
		wantErr string
	}{
		{"positional", []string{"8080"}, 8080, ""},
		{"port flag", []string{"--port", "443"}, 443, ""},
		{"port flag equals", []string{"--port=443"}, 443, ""},
		{"missing", nil, 0, "必须提供端口"},
		{"zero", []string{"0"}, 0, "1-65535"},
		{"too large", []string{"65536"}, 0, "1-65535"},
		{"not a number", []string{"abc"}, 0, "1-65535"},
		{"two ports", []string{"80", "81"}, 0, "一次只支持一个端口"},
		{"unknown option", []string{"--wat"}, 0, "未知选项"},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			port, err := parsePortArgs(tc.args)
			if tc.wantErr != "" {
				if err == nil || !strings.Contains(err.Error(), tc.wantErr) {
					t.Fatalf("err = %v, want %q", err, tc.wantErr)
				}
				return
			}
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if port != tc.want {
				t.Errorf("port = %d, want %d", port, tc.want)
			}
		})
	}
}

func TestParseNetstatPIDs(t *testing.T) {
	output := `Active Connections

  Proto  Local Address          Foreign Address        State           PID
  TCP    0.0.0.0:8080           0.0.0.0:0              LISTENING       1234
  TCP    [::]:8080              [::]:0                 LISTENING       1234
  TCP    127.0.0.1:8080         127.0.0.1:51515        ESTABLISHED     5678
  TCP    0.0.0.0:8081           0.0.0.0:0              LISTENING       9999
  UDP    0.0.0.0:8080           *:*                                    4321
`

	got := parseNetstatPIDs(output, 8080)
	want := []string{"1234", "4321", "5678"}
	if strings.Join(got, ",") != strings.Join(want, ",") {
		t.Fatalf("pids = %v, want %v", got, want)
	}

	if got := parseNetstatPIDs(output, 9999); len(got) != 0 {
		t.Errorf("port 9999 should have no match, got %v", got)
	}
}

func TestNetPIDsUnixWithFakeLsof(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("lsof fixture is not executable on Windows")
	}

	dir := t.TempDir()
	script := filepath.Join(dir, "lsof")
	if err := os.WriteFile(script, []byte("#!/bin/sh\necho 111\necho 222\n"), 0o755); err != nil {
		t.Fatal(err)
	}
	t.Setenv("PATH", dir+string(os.PathListSeparator)+os.Getenv("PATH"))

	var out bytes.Buffer
	if err := runNetPIDs(context.Background(), BuiltinRequest{Args: []string{"8080"}, Stdout: &out}); err != nil {
		t.Fatalf("runNetPIDs error: %v", err)
	}
	if out.String() != "111\n222\n" {
		t.Fatalf("output = %q", out.String())
	}
}

func TestNetPIDsUnixNoMatchIsNotAnError(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("lsof fixture is not executable on Windows")
	}

	dir := t.TempDir()
	script := filepath.Join(dir, "lsof")
	if err := os.WriteFile(script, []byte("#!/bin/sh\nexit 1\n"), 0o755); err != nil {
		t.Fatal(err)
	}
	t.Setenv("PATH", dir+string(os.PathListSeparator)+os.Getenv("PATH"))

	var out, errOut bytes.Buffer
	if err := runNetPIDs(context.Background(), BuiltinRequest{Args: []string{"8080"}, Stdout: &out, Stderr: &errOut}); err != nil {
		t.Fatalf("no-match should not fail: %v", err)
	}
	if out.Len() != 0 {
		t.Fatalf("expected empty stdout, got %q", out.String())
	}
	// The note goes to stderr so scripts parsing stdout are unaffected.
	if !strings.Contains(errOut.String(), "没有进程占用端口 8080") {
		t.Fatalf("stderr = %q, want a no-match note", errOut.String())
	}
}

func TestReportPIDsSilentWhenSomethingHoldsThePort(t *testing.T) {
	var out, errOut bytes.Buffer
	if err := reportPIDs(netPIDsOptions{port: 8080}, []string{"123", "456"}, BuiltinRequest{Stdout: &out, Stderr: &errOut}); err != nil {
		t.Fatalf("reportPIDs: %v", err)
	}
	if out.String() != "123\n456\n" {
		t.Errorf("stdout = %q", out.String())
	}
	if errOut.Len() != 0 {
		t.Errorf("stderr should stay empty, got %q", errOut.String())
	}
}

func TestNetPIDsRejectsInvalidPortBeforeRunning(t *testing.T) {
	var out bytes.Buffer
	err := runNetPIDs(context.Background(), BuiltinRequest{Args: []string{"70000"}, Stdout: &out})
	if err == nil || !strings.Contains(err.Error(), "1-65535") {
		t.Fatalf("err = %v", err)
	}
}

func TestParseSSPIDs(t *testing.T) {
	output := `tcp ESTAB 0 0 127.0.0.1:8080 127.0.0.1:51515 users:(("app",pid=4321,fd=6))
udp UNCONN 0 0 0.0.0.0:8080 0.0.0.0:* users:(("dns",pid=123,fd=5))
tcp LISTEN 0 128 0.0.0.0:8081 0.0.0.0:* users:(("other",pid=9999,fd=3))`

	got := parseSSPIDs(output, 8080)
	want := []string{"123", "4321"}
	if strings.Join(got, ",") != strings.Join(want, ",") {
		t.Fatalf("pids = %v, want %v", got, want)
	}
}

func TestNetPIDsUnixFallsBackToSS(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("shell fixtures are not executable on Windows")
	}

	dir := t.TempDir()
	body := "#!/bin/sh\n" +
		"echo 'tcp ESTAB 0 0 127.0.0.1:8080 127.0.0.1:51515 users:((\"app\",pid=4321,fd=6))'\n" +
		"echo 'tcp LISTEN 0 128 0.0.0.0:8081 0.0.0.0:* users:((\"other\",pid=9999,fd=3))'\n"
	if err := os.WriteFile(filepath.Join(dir, "ss"), []byte(body), 0o755); err != nil {
		t.Fatal(err)
	}
	// Only the fixture directory: there is no lsof, so ss must be used.
	t.Setenv("PATH", dir)

	var out bytes.Buffer
	if err := runNetPIDs(context.Background(), BuiltinRequest{Args: []string{"8080"}, Stdout: &out}); err != nil {
		t.Fatalf("runNetPIDs error: %v", err)
	}
	if out.String() != "4321\n" {
		t.Fatalf("output = %q, want %q", out.String(), "4321\n")
	}
}

func TestProcPIDs(t *testing.T) {
	if runtime.GOOS != "linux" {
		t.Skip("procPIDs is Linux-only")
	}

	root := t.TempDir()
	if err := os.MkdirAll(filepath.Join(root, "net"), 0o755); err != nil {
		t.Fatal(err)
	}
	tcp := "  sl  local_address rem_address   st tx_queue rx_queue tr tm->when retrnsmt   uid  timeout inode\n" +
		"   0: 0100007F:1F90 00000000:0000 0A 00000000:00000000 00:00000000 00000000  1000 0 12345 1\n"
	if err := os.WriteFile(filepath.Join(root, "net", "tcp"), []byte(tcp), 0o644); err != nil {
		t.Fatal(err)
	}

	fdDir := filepath.Join(root, "4242", "fd")
	if err := os.MkdirAll(fdDir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink("socket:[12345]", filepath.Join(fdDir, "3")); err != nil {
		t.Fatal(err)
	}

	got, err := procPIDs(8080, root) // 0x1F90 == 8080
	if err != nil {
		t.Fatalf("procPIDs error: %v", err)
	}
	if strings.Join(got, ",") != "4242" {
		t.Fatalf("pids = %v, want [4242]", got)
	}
}

func TestWritePIDFile(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "sub", "port.pid")

	if err := writePIDFile(path, []string{"123", "456"}); err != nil {
		t.Fatalf("writePIDFile: %v", err)
	}
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	// Comma-separated on one line so `kill $(cat file)` works.
	if string(data) != "123,456\n" {
		t.Fatalf("file = %q, want \"123,456\\n\"", data)
	}

	// No holder: the file exists but is empty.
	if err := writePIDFile(path, nil); err != nil {
		t.Fatalf("writePIDFile(nil): %v", err)
	}
	if data, _ := os.ReadFile(path); len(data) != 0 {
		t.Fatalf("file = %q, want empty", data)
	}
}

func TestParseNetPIDsArgsPidFile(t *testing.T) {
	opts, err := parseNetPIDsArgs([]string{"8080", "--pid-file", "/tmp/p.pid"})
	if err != nil || opts.port != 8080 || opts.pidFile != "/tmp/p.pid" {
		t.Fatalf("opts = %+v, err = %v", opts, err)
	}
	opts, err = parseNetPIDsArgs([]string{"--pid-file=/tmp/q.pid", "8080"})
	if err != nil || opts.pidFile != "/tmp/q.pid" {
		t.Fatalf("opts = %+v, err = %v", opts, err)
	}
	if _, err := parseNetPIDsArgs([]string{"8080", "--pid-file"}); err == nil ||
		!strings.Contains(err.Error(), "--pid-file 缺少文件路径") {
		t.Fatalf("err = %v, want a missing-path error", err)
	}
}

func TestNetPIDsWritesPIDFileThroughCLIPath(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("lsof fixture is not executable on Windows")
	}
	dir := t.TempDir()
	script := filepath.Join(dir, "lsof")
	if err := os.WriteFile(script, []byte("#!/bin/sh\nprintf '4242\\n'\\n"), 0o755); err != nil {
		t.Fatal(err)
	}
	t.Setenv("PATH", dir+string(os.PathListSeparator)+os.Getenv("PATH"))

	pidFile := filepath.Join(t.TempDir(), "web.pid")
	var out, errOut bytes.Buffer
	if err := runNetPIDs(context.Background(), BuiltinRequest{
		Args:   []string{"8080", "--pid-file", pidFile},
		Stdout: &out,
		Stderr: &errOut,
	}); err != nil {
		t.Fatalf("runNetPIDs: %v", err)
	}
	if strings.TrimSpace(out.String()) != "4242" {
		t.Errorf("stdout = %q, want the PID", out.String())
	}
	data, err := os.ReadFile(pidFile)
	if err != nil {
		t.Fatalf("pid file not written: %v", err)
	}
	if strings.TrimSpace(string(data)) != "4242" {
		t.Errorf("pid file = %q", data)
	}
	if !strings.Contains(errOut.String(), "PID 已写入") {
		t.Errorf("stderr = %q, want the write note", errOut.String())
	}
}
