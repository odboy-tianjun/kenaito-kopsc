package executor

import (
	"context"
	"errors"
	"strings"
	"testing"
)

func TestForcePushUsesLease(t *testing.T) {
	runner := &fakeGitRunner{responses: map[string]gitResponse{
		"push --force-with-lease origin main": {out: "ok"},
	}}
	if err := forcePush(context.Background(), runner, "origin", "main"); err != nil {
		t.Fatalf("forcePush: %v", err)
	}
	// Never a bare --force: the lease protects concurrent pushes.
	if len(runner.calls) != 1 || runner.calls[0] != "push --force-with-lease origin main" {
		t.Fatalf("calls = %v", runner.calls)
	}
	for _, call := range runner.calls {
		if strings.Contains(call, "--force ") || strings.HasSuffix(call, " --force") {
			t.Fatalf("unexpected bare --force: %q", call)
		}
	}
}

func TestForcePushExplainsRejectedLease(t *testing.T) {
	runner := &fakeGitRunner{responses: map[string]gitResponse{
		"push --force-with-lease origin main": {
			errOut: " ! [rejected]        main -> main (stale info)",
			err:    errors.New("exit status 1"),
		},
	}}
	err := forcePush(context.Background(), runner, "origin", "main")
	if err == nil {
		t.Fatal("expected an error")
	}
	for _, want := range []string{"强推被拒绝", "git fetch", "回滚"} {
		if !strings.Contains(err.Error(), want) {
			t.Errorf("error missing %q: %v", want, err)
		}
	}
}

func TestIsRejectedPush(t *testing.T) {
	yes := []string{
		"! [rejected] main -> main (stale info)",
		"remote: pre-receive hook declined",
		"protected branch hook declined",
	}
	for _, s := range yes {
		if !isRejectedPush(s) {
			t.Errorf("isRejectedPush(%q) = false, want true", s)
		}
	}
	if isRejectedPush("fatal: unable to access remote") {
		t.Error("network errors are not lease rejections")
	}
}

func TestForcePushReportsNetworkError(t *testing.T) {
	runner := &fakeGitRunner{responses: map[string]gitResponse{
		"push --force-with-lease origin main": {
			errOut: "fatal: unable to access remote", err: errors.New("exit 128"),
		},
	}}
	err := forcePush(context.Background(), runner, "origin", "main")
	if err == nil || !strings.Contains(err.Error(), "git push 失败") {
		t.Fatalf("err = %v, want a git push failure", err)
	}
}
