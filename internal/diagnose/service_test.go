package diagnose

import (
	"context"
	"fmt"
	"strings"
	"testing"
)

func TestResolveAutoDocker(t *testing.T) {
	inspectJSON := `[{"Id":"abc","Name":"/web","State":{"Status":"running"}}]`
	r := &fakeRunner{
		available: map[string]bool{"docker": true, "kubectl": true},
		respond: func(name string, args []string) (CommandResult, error) {
			switch {
			case len(args) > 0 && args[0] == "inspect":
				return CommandResult{Stdout: []byte(inspectJSON)}, nil
			case len(args) > 0 && args[0] == "logs":
				return CommandResult{Stdout: []byte("ok\n")}, nil
			case len(args) > 0 && args[0] == "events":
				return CommandResult{}, nil
			}
			return CommandResult{}, fmt.Errorf("unexpected call %s %v", name, args)
		},
	}

	s, out, _ := newTestService(r)
	if err := s.Run(context.Background(), []string{"web"}); err != nil {
		t.Fatalf("Run error: %v", err)
	}
	if !strings.Contains(out.String(), "来源:   Docker (docker)") {
		t.Errorf("expected docker source:\n%s", out.String())
	}
	if r.called("kubectl") {
		t.Error("kubectl should not be consulted when docker hits")
	}
}

func TestResolveAutoKubernetes(t *testing.T) {
	r := &fakeRunner{
		available: map[string]bool{"docker": true, "kubectl": true},
		respond: func(name string, args []string) (CommandResult, error) {
			joined := strings.Join(args, " ")
			switch {
			case len(args) > 0 && args[0] == "inspect":
				return CommandResult{Stdout: []byte("[]")}, nil
			case strings.HasPrefix(joined, "config view"):
				return CommandResult{}, nil
			case strings.HasPrefix(joined, "get pod"):
				return CommandResult{Stdout: []byte("pod/api-0\n")}, nil
			case strings.HasPrefix(joined, "logs"):
				return CommandResult{Stdout: []byte("log\n")}, nil
			case strings.HasPrefix(joined, "get events"):
				return CommandResult{}, nil
			}
			return CommandResult{}, fmt.Errorf("unexpected call %s", joined)
		},
	}

	s, out, _ := newTestService(r)
	// The fake returns name output for the first pod probe but diagnoseK8s
	// needs JSON; swap the responder after resolve would be brittle, so return
	// JSON for every get pod call instead.
	r.respond = func(name string, args []string) (CommandResult, error) {
		joined := strings.Join(args, " ")
		switch {
		case len(args) > 0 && args[0] == "inspect":
			return CommandResult{Stdout: []byte("[]")}, nil
		case strings.HasPrefix(joined, "config view"):
			return CommandResult{}, nil
		case strings.HasPrefix(joined, "get pod"):
			return CommandResult{Stdout: []byte(crashLoopPodJSON)}, nil
		case strings.HasPrefix(joined, "logs"):
			return CommandResult{Stdout: []byte("log\n")}, nil
		case strings.HasPrefix(joined, "get events"):
			return CommandResult{}, nil
		}
		return CommandResult{}, fmt.Errorf("unexpected call %s", joined)
	}

	if err := s.Run(context.Background(), []string{"api-0"}); err != nil {
		t.Fatalf("Run error: %v", err)
	}
	if !strings.Contains(out.String(), "来源:   Kubernetes") {
		t.Errorf("expected kubernetes source:\n%s", out.String())
	}
}

func TestResolveNotFound(t *testing.T) {
	r := &fakeRunner{
		available: map[string]bool{"docker": true, "kubectl": true},
		respond: func(name string, args []string) (CommandResult, error) {
			joined := strings.Join(args, " ")
			switch {
			case len(args) > 0 && args[0] == "inspect":
				return CommandResult{Stdout: []byte("[]")}, nil
			case strings.HasPrefix(joined, "config view"):
				return CommandResult{}, nil
			case strings.HasPrefix(joined, "get pod"):
				return CommandResult{Stderr: []byte("not found")}, fmt.Errorf("exit status 1")
			}
			return CommandResult{}, nil
		},
	}

	s, _, _ := newTestService(r)
	err := s.Run(context.Background(), []string{"ghost"})
	if err == nil || !strings.Contains(err.Error(), "未找到目标") {
		t.Fatalf("err = %v, want not-found error", err)
	}
}

func TestResolveNamespaceSkipsDocker(t *testing.T) {
	r := &fakeRunner{
		available: map[string]bool{"docker": true, "kubectl": true},
		respond: func(name string, args []string) (CommandResult, error) {
			joined := strings.Join(args, " ")
			switch {
			case strings.HasPrefix(joined, "get pod"):
				return CommandResult{Stdout: []byte(crashLoopPodJSON)}, nil
			case strings.HasPrefix(joined, "logs"):
				return CommandResult{}, nil
			case strings.HasPrefix(joined, "get events"):
				return CommandResult{}, nil
			}
			return CommandResult{}, fmt.Errorf("unexpected call %s", joined)
		},
	}

	s, out, _ := newTestService(r)
	if err := s.Run(context.Background(), []string{"api-0", "--namespace", "prod"}); err != nil {
		t.Fatalf("Run error: %v", err)
	}
	if r.called("inspect") {
		t.Error("namespace should skip the docker probe")
	}
	if !strings.Contains(out.String(), "namespace=prod") {
		t.Errorf("namespace missing from output:\n%s", out.String())
	}
}

func TestResolveNotFoundSuggestsSimilarContainers(t *testing.T) {
	r := &fakeRunner{
		available: map[string]bool{"docker": true, "kubectl": true},
		respond: func(name string, args []string) (CommandResult, error) {
			joined := strings.Join(args, " ")
			switch {
			case len(args) > 0 && args[0] == "inspect":
				return CommandResult{Stdout: []byte("[]")}, nil
			case strings.HasPrefix(joined, "ps -a"):
				return CommandResult{Stdout: []byte("web-1\nweb-2\ndb\nnginx-web\n")}, nil
			case strings.HasPrefix(joined, "config view"):
				return CommandResult{}, nil
			case strings.HasPrefix(joined, "get pod"):
				return CommandResult{Stderr: []byte("not found")}, fmt.Errorf("exit status 1")
			}
			return CommandResult{}, nil
		},
	}

	s, _, _ := newTestService(r)
	err := s.Run(context.Background(), []string{"web"})
	if err == nil || !strings.Contains(err.Error(), "未找到目标") {
		t.Fatalf("err = %v, want not-found error", err)
	}
	msg := err.Error()
	for _, want := range []string{"你可能要找的是", "web-1", "web-2"} {
		if !strings.Contains(msg, want) {
			t.Errorf("error missing %q:\n%s", want, msg)
		}
	}
}

func TestResolveNotFoundWithoutCandidatesKeepsOriginalText(t *testing.T) {
	r := &fakeRunner{
		available: map[string]bool{"docker": true},
		respond: func(name string, args []string) (CommandResult, error) {
			switch {
			case len(args) > 0 && args[0] == "inspect":
				return CommandResult{Stdout: []byte("[]")}, nil
			case strings.HasPrefix(strings.Join(args, " "), "ps -a"):
				return CommandResult{Stdout: []byte("alpha\nbeta\n")}, nil
			}
			return CommandResult{}, nil
		},
	}

	s, _, _ := newTestService(r)
	err := s.Run(context.Background(), []string{"ghost"})
	if err == nil || !strings.Contains(err.Error(), "未找到目标") {
		t.Fatalf("err = %v, want not-found error", err)
	}
	if strings.Contains(err.Error(), "你可能要找的是") {
		t.Errorf("unexpected candidate hint:\n%s", err.Error())
	}
	// Nothing is similar, so the existing objects are listed instead.
	for _, want := range []string{"当前存在的对象", "alpha", "beta"} {
		if !strings.Contains(err.Error(), want) {
			t.Errorf("error missing %q:\n%s", want, err.Error())
		}
	}
}

func TestMatchNamesRanking(t *testing.T) {
	names := []string{"nginx-web", "web-1", "WEB-2", "api", "myweb"}
	got := matchNames(names, "web")
	want := []string{"web-1", "WEB-2", "nginx-web", "myweb"}
	if strings.Join(got, ",") != strings.Join(want, ",") {
		t.Fatalf("matchNames() = %v, want %v (prefix hits first, case-insensitive)", got, want)
	}

	many := []string{"web-1", "web-2", "web-3", "web-4", "web-5", "web-6", "web-7"}
	if got := matchNames(many, "web"); len(got) != 5 {
		t.Fatalf("matchNames returned %d names, want the 5 closest", len(got))
	}
}
