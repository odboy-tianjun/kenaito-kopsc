package executor

import (
	"context"
	"errors"
	"strings"
	"testing"
)

func newStateRunner() *fakeGitRunner {
	return &fakeGitRunner{responses: map[string]gitResponse{
		"rev-parse --abbrev-ref HEAD": {out: "main\n"},
		"remote":                      {out: "origin\nupstream\n"},
		"status --porcelain":          {out: ""},
		"for-each-ref --format=%(upstream) refs/heads/main":    {out: "refs/remotes/origin/main\n"},
		"for-each-ref --format=%(upstream) refs/heads/feature": {out: "\n"},
	}}
}

func TestCurrentBranch(t *testing.T) {
	branch, err := currentBranch(context.Background(), newStateRunner())
	if err != nil || branch != "main" {
		t.Fatalf("branch = %q, err = %v", branch, err)
	}
}

func TestCurrentBranchRejectsDetachedHead(t *testing.T) {
	r := &fakeGitRunner{responses: map[string]gitResponse{
		"rev-parse --abbrev-ref HEAD": {out: "HEAD\n"},
	}}
	_, err := currentBranch(context.Background(), r)
	if err == nil || !strings.Contains(err.Error(), "detached HEAD") {
		t.Fatalf("err = %v, want a detached HEAD error", err)
	}
}

func TestRemotes(t *testing.T) {
	got, err := remotes(context.Background(), newStateRunner())
	if err != nil || strings.Join(got, ",") != "origin,upstream" {
		t.Fatalf("remotes = %v, err = %v", got, err)
	}
}

func TestIsWorktreeClean(t *testing.T) {
	clean, err := isWorktreeClean(context.Background(), newStateRunner())
	if err != nil || !clean {
		t.Fatalf("clean = %v, err = %v", clean, err)
	}
}

func TestIsWorktreeDirty(t *testing.T) {
	r := &fakeGitRunner{responses: map[string]gitResponse{
		"status --porcelain": {out: " M internal/cli/cli.go\n?? new.txt\n"},
	}}
	clean, err := isWorktreeClean(context.Background(), r)
	if err != nil || clean {
		t.Fatalf("clean = %v, err = %v", clean, err)
	}
}

func TestUpstreamRef(t *testing.T) {
	ref, err := upstreamRef(context.Background(), newStateRunner(), "main")
	if err != nil || ref != "refs/remotes/origin/main" {
		t.Fatalf("ref = %q, err = %v", ref, err)
	}
	// A branch without an upstream returns "" rather than an error.
	ref, err = upstreamRef(context.Background(), newStateRunner(), "feature")
	if err != nil || ref != "" {
		t.Fatalf("ref = %q, err = %v; want no upstream", ref, err)
	}
}

func TestGitStateHelpersPropagateErrors(t *testing.T) {
	r := &fakeGitRunner{responses: map[string]gitResponse{
		"rev-parse --abbrev-ref HEAD": {errOut: "fatal: not a repo", err: errors.New("exit 128")},
		"remote":                      {errOut: "boom", err: errors.New("exit 1")},
		"status --porcelain":          {errOut: "boom", err: errors.New("exit 1")},
	}}
	if _, err := currentBranch(context.Background(), r); err == nil || !strings.Contains(err.Error(), "not a repo") {
		t.Errorf("currentBranch err = %v", err)
	}
	if _, err := remotes(context.Background(), r); err == nil {
		t.Error("remotes should fail")
	}
	if _, err := isWorktreeClean(context.Background(), r); err == nil {
		t.Error("isWorktreeClean should fail")
	}
}
