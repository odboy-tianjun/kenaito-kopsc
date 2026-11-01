package executor

import (
	"bytes"
	"context"
	"errors"
	"strings"
	"sync/atomic"
	"testing"
	"time"
)

// fakeProcesses swaps the process lister for the duration of a test. Each call
// to the lister takes the next snapshot; the last one repeats.
func fakeProcesses(t *testing.T, snapshots ...[]processInfo) *int32 {
	t.Helper()
	var calls int32
	processLister = func(ctx context.Context) ([]processInfo, error) {
		n := int(atomic.AddInt32(&calls, 1)) - 1
		if n >= len(snapshots) {
			n = len(snapshots) - 1
		}
		return snapshots[n], nil
	}
	t.Cleanup(func() { processLister = listProcesses })
	return &calls
}

func TestParseProcWaitArgs(t *testing.T) {
	opts, err := parseProcWaitArgs([]string{"nginx", "worker", "--pid", "1234,5678", "--timeout", "30", "--interval", "200ms", "--once", "-q"})
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	if strings.Join(opts.keywords, ",") != "nginx,worker" {
		t.Errorf("keywords = %v", opts.keywords)
	}
	if len(opts.pids) != 2 || opts.pids[0] != 1234 || opts.pids[1] != 5678 {
		t.Errorf("pids = %v", opts.pids)
	}
	if opts.timeout != 30*time.Second || opts.interval != 200*time.Millisecond {
		t.Errorf("timeout=%v interval=%v", opts.timeout, opts.interval)
	}
	if !opts.once || !opts.quiet {
		t.Errorf("once=%v quiet=%v", opts.once, opts.quiet)
	}

	// Keywords may start with a dash once "--" is seen.
	opts, err = parseProcWaitArgs([]string{"--", "-weird"})
	if err != nil || len(opts.keywords) != 1 || opts.keywords[0] != "-weird" {
		t.Fatalf("keywords after --: %v err=%v", opts.keywords, err)
	}
}

func TestParseProcWaitArgsRejectsBadInput(t *testing.T) {
	tests := []struct {
		args []string
		want string
	}{
		{[]string{"--pid"}, "--pid 缺少参数"},
		{[]string{"--pid", "abc"}, "--pid 需要正整数"},
		{[]string{"--pid", "0"}, "--pid 需要正整数"},
		{[]string{"--timeout", "abc"}, "--timeout"},
		{[]string{"--interval", "0"}, "--interval 必须大于 0"},
		{[]string{"--nope", "x"}, "未知选项"},
	}
	for _, tc := range tests {
		_, err := parseProcWaitArgs(tc.args)
		if err == nil || !strings.Contains(err.Error(), tc.want) {
			t.Errorf("args %v: err = %v, want %q", tc.args, err, tc.want)
		}
	}
}

func TestParseWaitDuration(t *testing.T) {
	cases := map[string]time.Duration{
		"30":    30 * time.Second,
		"30s":   30 * time.Second,
		"500ms": 500 * time.Millisecond,
		"2m":    2 * time.Minute,
	}
	for in, want := range cases {
		got, err := parseWaitDuration(in)
		if err != nil || got != want {
			t.Errorf("parseWaitDuration(%q) = %v, %v; want %v", in, got, err, want)
		}
	}
	for _, in := range []string{"", "-5", "abc"} {
		if _, err := parseWaitDuration(in); err == nil {
			t.Errorf("parseWaitDuration(%q) should fail", in)
		}
	}
}

func TestProcessInfoMatchesAllKeywords(t *testing.T) {
	p := processInfo{PID: 42, Name: "nginx", Command: "/usr/sbin/nginx -g daemon off;"}
	cases := []struct {
		keywords []string
		want     bool
	}{
		{[]string{"nginx"}, true},
		{[]string{"NGINX"}, true},           // case-insensitive
		{[]string{"daemon"}, true},          // matches the command line
		{[]string{"nginx", "daemon"}, true}, // logical AND
		{[]string{"nginx", "redis"}, false},
		{nil, true},
	}
	for _, tc := range cases {
		if got := p.Matches(tc.keywords); got != tc.want {
			t.Errorf("Matches(%v) = %v, want %v", tc.keywords, got, tc.want)
		}
	}
}

func TestProcWaitReturnsWhenProcessIsGone(t *testing.T) {
	running := []processInfo{{PID: 42, Name: "nginx", Command: "nginx -g daemon off;"}}
	gone := []processInfo{{PID: 1, Name: "init", Command: "init"}}
	calls := fakeProcesses(t, running, running, gone)

	var out bytes.Buffer
	start := time.Now()
	err := runProcWait(context.Background(), BuiltinRequest{
		Args:   []string{"nginx", "--interval", "10ms"},
		Stdout: &out,
	})
	if err != nil {
		t.Fatalf("wait: %v", err)
	}
	if time.Since(start) > 2*time.Second {
		t.Errorf("wait took too long: %v", time.Since(start))
	}
	if atomic.LoadInt32(calls) < 3 {
		t.Errorf("expected at least 3 polls, got %d", *calls)
	}
	for _, want := range []string{"等待 进程 nginx 退出", "共 1 个进程", "42(nginx)", "已退出"} {
		if !strings.Contains(out.String(), want) {
			t.Errorf("output missing %q:\n%s", want, out.String())
		}
	}
}

func TestProcWaitAlreadyGoneIsImmediate(t *testing.T) {
	fakeProcesses(t, []processInfo{{PID: 1, Name: "init"}})

	var out bytes.Buffer
	if err := runProcWait(context.Background(), BuiltinRequest{Args: []string{"nginx"}, Stdout: &out}); err != nil {
		t.Fatalf("wait: %v", err)
	}
	if !strings.Contains(out.String(), "当前没有运行") {
		t.Errorf("output = %q", out.String())
	}
}

func TestProcWaitTimeoutExitsWithCodeTwo(t *testing.T) {
	fakeProcesses(t, []processInfo{{PID: 42, Name: "nginx"}})

	err := runProcWait(context.Background(), BuiltinRequest{
		Args:   []string{"nginx", "--timeout", "60ms", "--interval", "10ms"},
		Stdout: &bytes.Buffer{},
	})
	if err == nil {
		t.Fatal("expected a timeout error")
	}
	if code := ExitCode(err); code != 2 {
		t.Fatalf("exit code = %d, want 2", code)
	}
	if !strings.Contains(err.Error(), "等待超时") || !strings.Contains(err.Error(), "nginx") {
		t.Errorf("err = %v", err)
	}
}

func TestProcWaitByPID(t *testing.T) {
	// PID 42 exists in the first snapshot only; the keyword never matches.
	withPID := []processInfo{{PID: 42, Name: "some-proc"}, {PID: 7, Name: "other"}}
	without := []processInfo{{PID: 7, Name: "other"}}
	fakeProcesses(t, withPID, without)

	var out bytes.Buffer
	if err := runProcWait(context.Background(), BuiltinRequest{
		Args:   []string{"--pid", "42", "--interval", "10ms"},
		Stdout: &out,
	}); err != nil {
		t.Fatalf("wait: %v", err)
	}
	if !strings.Contains(out.String(), "PID 42") {
		t.Errorf("output = %q", out.String())
	}
}

func TestProcWaitOnceReportsDoNotWait(t *testing.T) {
	calls := fakeProcesses(t, []processInfo{{PID: 42, Name: "nginx"}})

	var out bytes.Buffer
	if err := runProcWait(context.Background(), BuiltinRequest{Args: []string{"nginx", "--once"}, Stdout: &out}); err != nil {
		t.Fatalf("wait: %v", err)
	}
	if atomic.LoadInt32(calls) != 1 {
		t.Errorf("--once should poll exactly once, polled %d times", *calls)
	}
	if !strings.Contains(out.String(), "42(nginx)") {
		t.Errorf("output = %q", out.String())
	}
}

func TestProcWaitRequiresTarget(t *testing.T) {
	fakeProcesses(t, nil)
	err := runProcWait(context.Background(), BuiltinRequest{Stdout: &bytes.Buffer{}})
	if err == nil || !strings.Contains(err.Error(), "缺少要等待的进程") {
		t.Fatalf("err = %v, want a missing-target error", err)
	}
}

func TestParsePS(t *testing.T) {
	out := "  1234 /usr/sbin/nginx -g daemon off;\n" +
		"     1 /sbin/init\n" +
		"  bad line\n"
	procs := parsePS(out)
	if len(procs) != 2 {
		t.Fatalf("parsed %d processes, want 2: %+v", len(procs), procs)
	}
	if procs[0].PID != 1234 || procs[0].Name != "nginx" {
		t.Errorf("first = %+v", procs[0])
	}
	if !strings.Contains(procs[0].Command, "daemon off") {
		t.Errorf("command not captured: %+v", procs[0])
	}
	if procs[1].Name != "init" {
		t.Errorf("second = %+v", procs[1])
	}
}

func TestParseTasklist(t *testing.T) {
	out := `"nginx.exe","1234","Console","1","12,345 K"` + "\n" +
		`"postgres.exe","5678","Services","0","80,000 K"` + "\n"
	procs := parseTasklist(out)
	if len(procs) != 2 {
		t.Fatalf("parsed %d, want 2: %+v", len(procs), procs)
	}
	if procs[0].PID != 1234 || procs[0].Name != "nginx.exe" {
		t.Errorf("first = %+v", procs[0])
	}
}

func TestProcWaitPropagatesListerError(t *testing.T) {
	processLister = func(ctx context.Context) ([]processInfo, error) {
		return nil, errors.New("ps 不可用")
	}
	t.Cleanup(func() { processLister = listProcesses })

	err := runProcWait(context.Background(), BuiltinRequest{Args: []string{"nginx"}, Stdout: &bytes.Buffer{}})
	if err == nil || !strings.Contains(err.Error(), "ps 不可用") {
		t.Fatalf("err = %v", err)
	}
}
