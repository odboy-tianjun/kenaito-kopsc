package executor

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"
)

// gitResponse is one canned git result.
type gitResponse struct {
	out, errOut string
	err         error
}

// fakeGitRunner answers git invocations from a table keyed by the joined args.
type fakeGitRunner struct {
	responses map[string]gitResponse
	calls     []string
	envs      []([]string)
}

func (f *fakeGitRunner) Run(ctx context.Context, args ...string) (string, string, error) {
	return f.RunEnv(ctx, nil, args...)
}

// RunEnv records the environment it was given so tests can assert on the
// author/committer dates passed to git commit-tree.
func (f *fakeGitRunner) RunEnv(ctx context.Context, env []string, args ...string) (string, string, error) {
	key := strings.Join(args, " ")
	f.calls = append(f.calls, key)
	if len(env) > 0 {
		f.envs = append(f.envs, env)
	}
	if r, ok := f.responses[key]; ok {
		return r.out, r.errOut, r.err
	}
	return "", "unexpected git call: " + key, errors.New("exit status 1")
}

func TestParseGitLog(t *testing.T) {
	out := "aaaa1111\t4f2a1b0\t2026-10-01T21:07:43+08:00\t2026-10-01T21:07:43+08:00\tfeat: 初始提交\n" +
		"bbbb2222\t9c6f072\t2026-10-02T22:32:11+08:00\t2026-10-02T22:33:00+08:00\tdocs: 补充说明\n"

	commits, err := parseGitLog(out)
	if err != nil {
		t.Fatalf("parseGitLog: %v", err)
	}
	if len(commits) != 2 {
		t.Fatalf("parsed %d commits, want 2", len(commits))
	}

	first := commits[0]
	if first.Hash != "aaaa1111" || first.Short != "4f2a1b0" || first.Subject != "feat: 初始提交" {
		t.Errorf("first = %+v", first)
	}
	// Times keep their seconds, which the retiming relies on.
	if first.AuthorTime.Second() != 43 || first.AuthorTime.Hour() != 21 || first.AuthorTime.Minute() != 7 {
		t.Errorf("author time lost precision: %v", first.AuthorTime)
	}
	if commits[1].CommitTime.After(commits[1].AuthorTime) != true {
		t.Errorf("committer time not parsed: %v", commits[1].CommitTime)
	}
}

func TestParseGitLogSkipsTrailingBlankLine(t *testing.T) {
	out := "aaaa\t4f2\t2026-10-01T21:07:43+08:00\t2026-10-01T21:07:43+08:00\tsubject\n\n"
	commits, err := parseGitLog(out)
	if err != nil || len(commits) != 1 {
		t.Fatalf("commits = %v, err = %v", commits, err)
	}
}

func TestParseGitLogKeepsTabsInSubject(t *testing.T) {
	// SplitN with limit 5 keeps anything the subject contains, tabs included.
	out := "aaaa\t4f2\t2026-10-01T21:00:00+08:00\t2026-10-01T21:00:00+08:00\tsub\twith\ttabs\n"
	commits, err := parseGitLog(out)
	if err != nil {
		t.Fatal(err)
	}
	if commits[0].Subject != "sub\twith\ttabs" {
		t.Errorf("subject = %q", commits[0].Subject)
	}
}

func TestParseGitLogRejectsBrokenInput(t *testing.T) {
	if _, err := parseGitLog("not-enough-fields\n"); err == nil {
		t.Error("expected an error for a short record")
	}
	bad := "a\tb\tnot-a-time\t2026-10-01T21:00:00+08:00\ts\n"
	if _, err := parseGitLog(bad); err == nil || !strings.Contains(err.Error(), "作者时间") {
		t.Errorf("err = %v, want an author-time error", err)
	}
}

func TestGitCommitDate(t *testing.T) {
	loc := time.FixedZone("CST", 8*3600)
	c := gitCommit{AuthorTime: time.Date(2026, 10, 5, 21, 7, 43, 0, loc)}
	d := c.Date()
	if d.Hour() != 0 || d.Minute() != 0 || d.Day() != 5 || d.Month() != time.October || d.Year() != 2026 {
		t.Fatalf("Date() = %v", d)
	}
	if d.Location() != loc {
		t.Errorf("Date() lost the original time zone: %v", d.Location())
	}
}

func TestListCommitsRunsGitLog(t *testing.T) {
	runner := &fakeGitRunner{responses: map[string]gitResponse{
		"log --format=" + gitLogFormat + " main": {
			out: "aaaa\t4f2\t2026-10-01T21:00:00+08:00\t2026-10-01T21:00:00+08:00\tsubject\n",
		},
	}}
	commits, err := listCommits(context.Background(), runner, "main")
	if err != nil {
		t.Fatalf("listCommits: %v", err)
	}
	if len(commits) != 1 || commits[0].Short != "4f2" {
		t.Fatalf("commits = %+v", commits)
	}
}

func TestListCommitsReportsGitFailure(t *testing.T) {
	runner := &fakeGitRunner{responses: map[string]gitResponse{
		"log --format=" + gitLogFormat + " main": {
			errOut: "fatal: not a git repository", err: errors.New("exit status 128"),
		},
	}}
	_, err := listCommits(context.Background(), runner, "main")
	if err == nil || !strings.Contains(err.Error(), "not a git repository") {
		t.Fatalf("err = %v, want git's stderr", err)
	}
}
