package diagnose

import (
	"bytes"
	"context"
	"fmt"
	"strings"
	"time"

	"gitea.odboy.cn/odboy/kenaito-kopsc/internal/platform"
)

// fakeRunner records calls and returns canned output.
type fakeRunner struct {
	available map[string]bool
	calls     []string
	respond   func(name string, args []string) (CommandResult, error)
}

func (f *fakeRunner) LookPath(name string) (string, error) {
	if f.available[name] {
		return "/usr/bin/" + name, nil
	}
	return "", fmt.Errorf("%s not found", name)
}

func (f *fakeRunner) Run(ctx context.Context, name string, args ...string) (CommandResult, error) {
	f.calls = append(f.calls, name+" "+strings.Join(args, " "))
	if f.respond == nil {
		return CommandResult{}, nil
	}
	return f.respond(name, args)
}

func (f *fakeRunner) called(substr string) bool {
	for _, call := range f.calls {
		if strings.Contains(call, substr) {
			return true
		}
	}
	return false
}

func newTestService(r Runner) (*Service, *bytes.Buffer, *bytes.Buffer) {
	out, errOut := &bytes.Buffer{}, &bytes.Buffer{}
	s := New(platform.Info{OS: "linux", Arch: "amd64"}, r, out, errOut)
	s.now = func() time.Time { return time.Date(2026, 10, 31, 12, 0, 0, 0, time.UTC) }
	return s, out, errOut
}

// sampleDocker builds an exited container.
func sampleDocker(status string, exitCode int, oom bool, restarts int) dockerContainer {
	c := dockerContainer{ID: "abc", Name: "/web", RestartCount: restarts}
	c.Config.Image = "nginx:1.25"
	c.State.Status = status
	c.State.ExitCode = exitCode
	c.State.OOMKilled = oom
	c.State.Running = status == "running"
	return c
}
