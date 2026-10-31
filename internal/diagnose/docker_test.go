package diagnose

import (
	"context"
	"fmt"
	"strings"
	"testing"
)

func TestJudgeDocker(t *testing.T) {
	unhealthy := sampleDocker("running", 0, false, 0)
	unhealthy.State.Health = &dockerHealth{Status: "unhealthy"}

	cases := []struct {
		name         string
		container    dockerContainer
		wantSeverity string
		wantContains string
	}{
		{"oom killed", sampleDocker("exited", 137, true, 3), "error", "OOM"},
		{"sigkill", sampleDocker("exited", 137, false, 0), "error", "SIGKILL"},
		{"sigterm", sampleDocker("exited", 143, false, 0), "warn", "SIGTERM"},
		{"normal exit", sampleDocker("exited", 0, false, 0), "ok", "正常退出"},
		{"normal exit with restarts", sampleDocker("exited", 0, false, 2), "warn", "重启"},
		{"application error", sampleDocker("exited", 1, false, 0), "error", "非零"},
		{"not executable", sampleDocker("exited", 126, false, 0), "error", "126"},
		{"missing command", sampleDocker("exited", 127, false, 0), "error", "127"},
		{"running", sampleDocker("running", 0, false, 0), "ok", "运行"},
		{"unhealthy", unhealthy, "error", "健康检查失败"},
		{"restarting", sampleDocker("restarting", 0, false, 5), "error", "重启循环"},
		{"created", sampleDocker("created", 0, false, 0), "warn", "未启动"},
		{"dead", sampleDocker("dead", 0, false, 0), "error", "dead"},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			f := judgeDocker(tc.container)
			if f.Severity != tc.wantSeverity {
				t.Errorf("severity = %q, want %q (verdict %q)", f.Severity, tc.wantSeverity, f.Verdict)
			}
			if !strings.Contains(f.Verdict, tc.wantContains) {
				t.Errorf("verdict %q does not contain %q", f.Verdict, tc.wantContains)
			}
			if f.Evidence == "" {
				t.Error("evidence must not be empty")
			}
		})
	}
}

func TestDiagnoseDockerOutput(t *testing.T) {
	inspectJSON := `[{"Id":"abc","Name":"/web","RestartCount":3,"Config":{"Image":"nginx:1.25"},"State":{"Status":"exited","ExitCode":137,"OOMKilled":true,"StartedAt":"2026-10-31T10:00:00Z","FinishedAt":"2026-10-31T10:05:00Z"}}]`

	r := &fakeRunner{
		available: map[string]bool{"docker": true},
		respond: func(name string, args []string) (CommandResult, error) {
			switch {
			case len(args) > 0 && args[0] == "inspect":
				return CommandResult{Stdout: []byte(inspectJSON)}, nil
			case len(args) > 0 && args[0] == "logs":
				return CommandResult{Stdout: []byte("line1\nline2\n")}, nil
			case len(args) > 0 && args[0] == "events":
				return CommandResult{Stdout: []byte("die event\n")}, nil
			}
			return CommandResult{}, fmt.Errorf("unexpected call %s %v", name, args)
		},
	}

	s, out, errOut := newTestService(r)
	if err := s.Run(context.Background(), []string{"web"}); err != nil {
		t.Fatalf("Run error: %v (stderr=%s)", err, errOut.String())
	}

	text := out.String()
	for _, want := range []string{
		"目标:   web",
		"来源:   Docker (docker)",
		"ExitCode=137",
		"OOMKilled=true",
		"OOM",
		"依据:",
		"建议:",
		"最近日志 (50 行)",
		"line1",
		"最近事件 (1h)",
		"die event",
	} {
		if !strings.Contains(text, want) {
			t.Errorf("output missing %q:\n%s", want, text)
		}
	}
}

func TestDiagnoseDockerHonoursNoLogsNoEvents(t *testing.T) {
	inspectJSON := `[{"Id":"abc","Name":"/web","State":{"Status":"running"}}]`
	r := &fakeRunner{
		available: map[string]bool{"docker": true},
		respond: func(name string, args []string) (CommandResult, error) {
			return CommandResult{Stdout: []byte(inspectJSON)}, nil
		},
	}

	s, _, _ := newTestService(r)
	if err := s.Run(context.Background(), []string{"web", "--no-logs", "--no-events"}); err != nil {
		t.Fatalf("Run error: %v", err)
	}
	if r.called("logs") {
		t.Error("logs should not be collected")
	}
	if r.called("events") {
		t.Error("events should not be collected")
	}
}
