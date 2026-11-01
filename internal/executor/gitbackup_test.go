package executor

import (
	"context"
	"strings"
	"testing"
	"time"
)

func TestBackupRefName(t *testing.T) {
	now := time.Date(2026, 11, 1, 9, 30, 15, 0, cst)
	got := backupRefName("main", now)
	if got != "refs/kopsc-backup/main-20261101-093015" {
		t.Fatalf("backupRefName = %q", got)
	}
	if !strings.HasPrefix(got, backupRefPrefix) {
		t.Errorf("ref must live under %s: %q", backupRefPrefix, got)
	}
}

func TestCreateBackupWritesRef(t *testing.T) {
	runner := &fakeGitRunner{responses: map[string]gitResponse{
		"update-ref refs/kopsc-backup/main-20261101-093015 abc123": {out: ""},
	}}
	now := time.Date(2026, 11, 1, 9, 30, 15, 0, cst)

	ref, err := createBackup(context.Background(), runner, "main", "abc123", now)
	if err != nil {
		t.Fatalf("createBackup: %v", err)
	}
	if ref != "refs/kopsc-backup/main-20261101-093015" {
		t.Fatalf("ref = %q", ref)
	}
	if len(runner.calls) != 1 || !strings.Contains(runner.calls[0], "update-ref "+ref+" abc123") {
		t.Fatalf("calls = %v", runner.calls)
	}
}

func TestEnsureCleanWorktreeRejectsDirtyTree(t *testing.T) {
	runner := &fakeGitRunner{responses: map[string]gitResponse{
		"status --porcelain": {out: " M main.go\n"},
	}}
	err := ensureCleanWorktree(context.Background(), runner)
	if err == nil || !strings.Contains(err.Error(), "未提交的改动") {
		t.Fatalf("err = %v, want a dirty-worktree rejection", err)
	}
	if !strings.Contains(err.Error(), "stash") {
		t.Errorf("error should tell the user how to proceed: %v", err)
	}
}

func TestEnsureCleanWorktreeAcceptsCleanTree(t *testing.T) {
	runner := &fakeGitRunner{responses: map[string]gitResponse{
		"status --porcelain": {out: ""},
	}}
	if err := ensureCleanWorktree(context.Background(), runner); err != nil {
		t.Fatalf("clean tree should pass: %v", err)
	}
}

func TestBackupHint(t *testing.T) {
	hint := backupHint("main", "refs/kopsc-backup/main-20261101-093015")
	if !strings.Contains(hint, "git update-ref refs/heads/main refs/kopsc-backup/main-20261101-093015") {
		t.Errorf("hint = %q", hint)
	}
	if !strings.Contains(hint, "git reset --hard") {
		t.Errorf("hint should mention reset --hard: %q", hint)
	}
}
