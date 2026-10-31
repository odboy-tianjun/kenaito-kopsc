package diagnose

import (
	"context"
	"fmt"
	"strings"
	"testing"
)

const crashLoopPodJSON = `{
  "metadata":{"name":"api-0","namespace":"prod"},
  "status":{
    "phase":"Running",
    "containerStatuses":[
      {"name":"app","ready":false,"restartCount":5,
       "state":{"waiting":{"reason":"CrashLoopBackOff","message":"back-off 5m0s"}},
       "lastState":{"terminated":{"reason":"Error","exitCode":1,"message":"boom"}}}
    ]
  }
}`

func waitingContainer(reason string, lastExit int) containerStatus {
	cs := containerStatus{Name: "app", RestartCount: 2}
	cs.State.Waiting = &podWaiting{Reason: reason, Message: "message"}
	if lastExit >= 0 {
		cs.LastState.Terminated = &podTerminated{Reason: "Error", ExitCode: lastExit}
	}
	return cs
}

func TestJudgeContainer(t *testing.T) {
	oom := containerStatus{Name: "app"}
	oom.State.Terminated = &podTerminated{Reason: "OOMKilled", ExitCode: 137}

	completed := containerStatus{Name: "app"}
	completed.State.Terminated = &podTerminated{Reason: "Completed", ExitCode: 0}

	notReady := containerStatus{Name: "app"}
	notReady.State.Running = &podRunning{}

	ready := containerStatus{Name: "app", Ready: true}
	ready.State.Running = &podRunning{}

	cases := []struct {
		name         string
		status       containerStatus
		wantSeverity string
		wantContains string
	}{
		{"crash loop", waitingContainer("CrashLoopBackOff", 1), "error", "CrashLoopBackOff"},
		{"image pull", waitingContainer("ImagePullBackOff", -1), "error", "镜像拉取"},
		{"err image pull", waitingContainer("ErrImagePull", -1), "error", "镜像拉取"},
		{"config error", waitingContainer("CreateContainerConfigError", -1), "error", "配置"},
		{"creating", waitingContainer("ContainerCreating", -1), "warn", "创建"},
		{"oom killed", oom, "error", "OOMKilled"},
		{"completed", completed, "ok", "正常结束"},
		{"not ready", notReady, "warn", "未就绪"},
		{"ready", ready, "ok", "已就绪"},
		{"unknown", containerStatus{Name: "app"}, "warn", "未知"},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			f := judgeContainer(tc.status)
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

func TestJudgePod(t *testing.T) {
	cases := []struct {
		phase        string
		wantSeverity string
	}{
		{"Pending", "warn"},
		{"Failed", "error"},
		{"Succeeded", "ok"},
		{"Running", "ok"},
	}
	for _, tc := range cases {
		var p pod
		p.Status.Phase = tc.phase
		if got := judgePod(p); got.Severity != tc.wantSeverity {
			t.Errorf("phase %s: severity = %q, want %q", tc.phase, got.Severity, tc.wantSeverity)
		}
	}
}

func TestDiagnoseK8sOutput(t *testing.T) {
	r := &fakeRunner{
		available: map[string]bool{"kubectl": true},
		respond: func(name string, args []string) (CommandResult, error) {
			joined := strings.Join(args, " ")
			switch {
			case strings.HasPrefix(joined, "get pod"):
				return CommandResult{Stdout: []byte(crashLoopPodJSON)}, nil
			case strings.HasPrefix(joined, "logs"):
				return CommandResult{Stdout: []byte("panic: boom\n")}, nil
			case strings.HasPrefix(joined, "get events"):
				return CommandResult{Stdout: []byte("0s Warning FailedScheduling insufficient cpu\n")}, nil
			case strings.HasPrefix(joined, "config view"):
				return CommandResult{}, nil
			}
			return CommandResult{}, fmt.Errorf("unexpected call %s", joined)
		},
	}

	s, out, errOut := newTestService(r)
	err := s.Run(context.Background(), []string{"api-0", "--kind", "k8s", "--namespace", "prod"})
	if err != nil {
		t.Fatalf("Run error: %v (stderr=%s)", err, errOut.String())
	}

	text := out.String()
	for _, want := range []string{
		"目标:   api-0",
		"来源:   Kubernetes",
		"namespace=prod",
		"phase=Running",
		"容器 app:",
		"CrashLoopBackOff",
		"lastState.exitCode=1",
		"日志 (app, 最近 50 行)",
		"panic: boom",
		"日志 (app, 上一次)",
		"FailedScheduling",
		"事件提示",
	} {
		if !strings.Contains(text, want) {
			t.Errorf("output missing %q:\n%s", want, text)
		}
	}
}

func TestDiagnoseK8sFiltersContainer(t *testing.T) {
	podJSON := `{
	  "metadata":{"name":"api-0","namespace":"prod"},
	  "status":{"phase":"Running","containerStatuses":[
	    {"name":"one","ready":true,"state":{"running":{}}},
	    {"name":"two","ready":true,"state":{"running":{}}}
	  ]}
	}`

	r := &fakeRunner{
		available: map[string]bool{"kubectl": true},
		respond: func(name string, args []string) (CommandResult, error) {
			joined := strings.Join(args, " ")
			if strings.HasPrefix(joined, "get pod") {
				return CommandResult{Stdout: []byte(podJSON)}, nil
			}
			return CommandResult{Stdout: []byte("log\n")}, nil
		},
	}

	s, out, _ := newTestService(r)
	if err := s.Run(context.Background(), []string{"api-0", "--kind", "k8s", "-c", "one"}); err != nil {
		t.Fatalf("Run error: %v", err)
	}
	text := out.String()
	if !strings.Contains(text, "容器 one:") || strings.Contains(text, "容器 two:") {
		t.Errorf("container filter not applied:\n%s", text)
	}
}
