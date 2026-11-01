package executor

import (
	"context"
	"strings"
	"testing"
	"time"
)

// rewriteFixture builds a fake git whose two commits c1 -> c2 live on main.
func rewriteFixture() *fakeGitRunner {
	meta := func(hash, tree, parents string) map[string]gitResponse {
		return map[string]gitResponse{
			"log -1 --format=" + rawCommitFormat + " " + hash: {
				out: strings.Join([]string{hash, tree, parents, "Odboy", "tianjun@odboy.cn", "Odboy", "tianjun@odboy.cn", "subject of " + hash}, "\t") + "\n",
			},
			"log -1 --format=%B " + hash: {out: "subject of " + hash + "\n\nbody line\n"},
		}
	}

	responses := map[string]gitResponse{}
	for _, entry := range []map[string]gitResponse{meta("c1", "tree1", "base"), meta("c2", "tree2", "c1")} {
		for k, v := range entry {
			responses[k] = v
		}
	}
	// New commits are named deterministically by the tree they carry.
	responses["commit-tree tree1 -p base -m subject of c1\n\nbody line"] = gitResponse{out: "n1\n"}
	responses["commit-tree tree2 -p n1 -m subject of c2\n\nbody line"] = gitResponse{out: "n2\n"}
	return &fakeGitRunner{responses: responses}
}

func TestRewriteChainRebuildsInOrder(t *testing.T) {
	runner := rewriteFixture()
	plan := retimePlan{
		Commits: []retime{
			{Hash: "c1", Short: "c1", NewTime: time.Date(2026, 10, 1, 21, 7, 43, 0, cst)},
			{Hash: "c2", Short: "c2", NewTime: time.Date(2026, 10, 1, 21, 32, 11, 0, cst)},
		},
		Branch: "main",
		Remote: "origin",
	}

	tip, rebuilt, err := rewriteChain(context.Background(), runner, plan)
	if err != nil {
		t.Fatalf("rewriteChain: %v", err)
	}
	if tip != "n2" {
		t.Errorf("tip = %q, want n2", tip)
	}
	if len(rebuilt) != 2 || rebuilt[0].New != "n1" || rebuilt[1].New != "n2" {
		t.Fatalf("rebuilt = %+v", rebuilt)
	}

	// The dates must be passed to git exactly as planned, with the original
	// identities kept.
	if len(runner.envs) != 2 {
		t.Fatalf("expected env for two commits, got %d", len(runner.envs))
	}
	joined := strings.Join(runner.envs[0], " ")
	for _, want := range []string{
		"GIT_AUTHOR_NAME=Odboy",
		"GIT_AUTHOR_EMAIL=tianjun@odboy.cn",
		"GIT_AUTHOR_DATE=2026-10-01T21:07:43+08:00",
		"GIT_COMMITTER_DATE=2026-10-01T21:07:43+08:00",
	} {
		if !strings.Contains(joined, want) {
			t.Errorf("first commit env missing %q: %v", want, runner.envs[0])
		}
	}
	if !strings.Contains(strings.Join(runner.envs[1], " "), "GIT_AUTHOR_DATE=2026-10-01T21:32:11+08:00") {
		t.Errorf("second commit env = %v", runner.envs[1])
	}
}

func TestRebuildCommitKeepsTreeParentsAndMessage(t *testing.T) {
	var gotArgs []string
	var gotEnv []string
	runner := &fakeGitRunner{
		responses: map[string]gitResponse{
			"commit-tree T -p P1 -p P2 -m msg line 1\nline 2": {out: "newhash\n"},
		},
	}
	orig := runner.RunEnv
	_ = orig

	meta := commitMeta{
		Tree: "T", Parents: []string{"P1", "P2"},
		AuthorName: "A", AuthorEmail: "a@e", CommitName: "C", CommitEmail: "c@e",
		Body: "msg line 1\nline 2",
	}
	hash, err := rebuildCommit(context.Background(), runner, meta, time.Date(2026, 10, 1, 21, 0, 0, 0, cst))
	if err != nil {
		t.Fatalf("rebuildCommit: %v", err)
	}
	if hash != "newhash" {
		t.Errorf("hash = %q", hash)
	}
	gotArgs = append(gotArgs, runner.calls...)
	gotEnv = append(gotEnv, strings.Join(runner.envs[0], " "))
	if !strings.Contains(gotEnv[0], "GIT_COMMITTER_EMAIL=c@e") {
		t.Errorf("env = %v", gotEnv)
	}
}

func TestMoveBranch(t *testing.T) {
	runner := &fakeGitRunner{responses: map[string]gitResponse{
		"update-ref refs/heads/main n2": {out: ""},
	}}
	if err := moveBranch(context.Background(), runner, "main", "n2"); err != nil {
		t.Fatalf("moveBranch: %v", err)
	}
	if len(runner.calls) != 1 || runner.calls[0] != "update-ref refs/heads/main n2" {
		t.Fatalf("calls = %v", runner.calls)
	}
}

func TestRewriteChainEmptyPlan(t *testing.T) {
	if _, _, err := rewriteChain(context.Background(), rewriteFixture(), retimePlan{}); err == nil {
		t.Fatal("expected an error for an empty plan")
	}
}
