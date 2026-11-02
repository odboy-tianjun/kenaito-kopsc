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

// lfsAdvisory is the git-lfs note that used to be mistaken for the cause.
const lfsAdvisory = `Locking support detected on remote "origin". Consider enabling it with:
  $ git config lfs.https://gitea.example.com/a/b.git/info/lfs.locksverify true
`

func TestForcePushExplainsLeaseRejection(t *testing.T) {
	runner := &fakeGitRunner{responses: map[string]gitResponse{
		"push --force-with-lease origin main": {
			errOut: lfsAdvisory + " ! [rejected]        main -> main (stale info)\n",
			err:    errors.New("exit status 1"),
		},
	}}
	err := forcePush(context.Background(), runner, "origin", "main")
	if err == nil {
		t.Fatal("expected an error")
	}
	msg := err.Error()
	for _, want := range []string{"租约校验失败", "--force-with-lease=main:", "回滚"} {
		if !strings.Contains(msg, want) {
			t.Errorf("message missing %q:\n%s", want, msg)
		}
	}
	// The git-lfs advice is noise here and must not lead the message.
	if strings.Contains(msg, "Locking support detected") {
		t.Errorf("LFS advisory leaked into the lease message:\n%s", msg)
	}
}

func TestForcePushExplainsLFSMissing(t *testing.T) {
	stderr := lfsAdvisory +
		"Git LFS upload failed: 0% (0/1)\n" +
		"  (missing) doc/ffmpeg.zip (6080e34165fcb0261f979bb1cef26c5fce616e2a416384b50bcce956aa265018)\n" +
		"hint: Your push was rejected due to missing or corrupt local objects.\n" +
		"hint: You can disable this check with: `git config lfs.allowincompletepush true`\n"
	runner := &fakeGitRunner{responses: map[string]gitResponse{
		"push --force-with-lease origin main": {errOut: stderr, err: errors.New("exit status 1")},
	}}
	err := forcePush(context.Background(), runner, "origin", "main")
	if err == nil {
		t.Fatal("expected an error")
	}
	msg := err.Error()
	for _, want := range []string{
		"LFS 完整性检查",
		"doc/ffmpeg.zip",
		"lfs.allowincompletepush=true",
	} {
		if !strings.Contains(msg, want) {
			t.Errorf("message missing %q:\n%s", want, msg)
		}
	}
	// The lease wording would mislead: LFS, not the remote, blocked this push.
	if strings.Contains(msg, "租约") {
		t.Errorf("LFS failure was reported as a lease problem:\n%s", msg)
	}
	if strings.Contains(msg, "Locking support detected") {
		t.Errorf("LFS advisory leaked into the message:\n%s", msg)
	}
}

func TestForcePushExplainsProtectedBranch(t *testing.T) {
	runner := &fakeGitRunner{responses: map[string]gitResponse{
		"push --force-with-lease origin main": {
			errOut: "remote: error: GH006: protected branch update failed\nremote: pre-receive hook declined\n",
			err:    errors.New("exit status 1"),
		},
	}}
	err := forcePush(context.Background(), runner, "origin", "main")
	if err == nil || !strings.Contains(err.Error(), "分支保护") {
		t.Fatalf("err = %v, want a branch-protection message", err)
	}
	if strings.Contains(err.Error(), "租约") {
		t.Errorf("protected branch was reported as a lease problem: %v", err)
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
	// An LFS failure is not a remote-side rejection.
	if isRejectedPush("Git LFS upload failed:\n  (missing) a.zip (abc)") {
		t.Error("LFS failures must not be classified as lease rejections")
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

func TestFirstMissingObject(t *testing.T) {
	stderr := "Git LFS upload failed:\n  (missing) doc/a.zip (abc123)\n  (missing) upx.exe (def456)"
	if got := firstMissingObject(stderr); got != "doc/a.zip" {
		t.Errorf("firstMissingObject = %q", got)
	}
	if got := firstMissingObject("no entries"); got != "未知对象" {
		t.Errorf("firstMissingObject = %q, want a placeholder", got)
	}
}
