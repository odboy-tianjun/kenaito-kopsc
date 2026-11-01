package executor

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"strings"
	"time"
)

// gitCommit is one commit as kopsc needs to see it.
type gitCommit struct {
	Hash       string    // full SHA-1
	Short      string    // abbreviated SHA-1, as git prints it
	AuthorTime time.Time // author date
	CommitTime time.Time // committer date
	Subject    string
}

// Date returns the calendar day the commit belongs to, taken from the author
// date: retiming keeps commits on their own day.
func (c gitCommit) Date() time.Time {
	return time.Date(c.AuthorTime.Year(), c.AuthorTime.Month(), c.AuthorTime.Day(), 0, 0, 0, 0, c.AuthorTime.Location())
}

// gitRunner executes git commands. It is an interface so history rewriting can
// be tested against canned output.
type gitRunner interface {
	Run(ctx context.Context, args ...string) (stdout, stderr string, err error)
	// RunEnv runs git with extra environment variables, used to set the author
	// and committer dates when rebuilding commits.
	RunEnv(ctx context.Context, env []string, args ...string) (stdout, stderr string, err error)
}

// execGitRunner runs git in a directory.
type execGitRunner struct {
	dir string
}

func (r execGitRunner) Run(ctx context.Context, args ...string) (string, string, error) {
	return r.RunEnv(ctx, nil, args...)
}

// RunEnv runs git with additional environment variables.
func (r execGitRunner) RunEnv(ctx context.Context, env []string, args ...string) (string, string, error) {
	cmd := exec.CommandContext(ctx, "git", args...)
	cmd.Dir = r.dir
	if len(env) > 0 {
		cmd.Env = append(os.Environ(), env...)
	}
	var out, errOut strings.Builder
	cmd.Stdout = &out
	cmd.Stderr = &errOut
	err := cmd.Run()
	return out.String(), errOut.String(), err
}

// gitLogFormat is the record format used by listCommits: full hash, short
// hash, ISO-8601 author date, ISO-8601 committer date and subject, separated by
// tabs so a subject may contain anything else.
const gitLogFormat = "%H%x09%h%x09%aI%x09%cI%x09%s"

// listCommits returns commits reachable from base, newest first.
func listCommits(ctx context.Context, runner gitRunner, base string) ([]gitCommit, error) {
	out, stderr, err := runner.Run(ctx, "log", "--format="+gitLogFormat, base)
	if err != nil {
		return nil, gitError("git log", stderr, err)
	}
	return parseGitLog(out)
}

// parseGitLog parses the tab-separated records produced by gitLogFormat.
func parseGitLog(out string) ([]gitCommit, error) {
	var commits []gitCommit
	for _, line := range strings.Split(out, "\n") {
		line = strings.TrimRight(line, "\r")
		if strings.TrimSpace(line) == "" {
			continue
		}
		fields := strings.SplitN(line, "\t", 5)
		if len(fields) < 5 {
			return nil, fmt.Errorf("无法解析 git log 输出（字段不足）: %q", line)
		}
		author, err := time.Parse(time.RFC3339, fields[2])
		if err != nil {
			return nil, fmt.Errorf("无法解析作者时间 %q: %w", fields[2], err)
		}
		committed, err := time.Parse(time.RFC3339, fields[3])
		if err != nil {
			return nil, fmt.Errorf("无法解析提交时间 %q: %w", fields[3], err)
		}
		commits = append(commits, gitCommit{
			Hash:       fields[0],
			Short:      fields[1],
			AuthorTime: author,
			CommitTime: committed,
			Subject:    fields[4],
		})
	}
	return commits, nil
}

// gitError renders a failed git invocation with its stderr.
func gitError(command, stderr string, err error) error {
	detail := strings.TrimSpace(stderr)
	if detail == "" {
		detail = err.Error()
	}
	return fmt.Errorf("%s 失败: %s", command, detail)
}

// currentBranch returns the checked-out branch name.
func currentBranch(ctx context.Context, runner gitRunner) (string, error) {
	out, stderr, err := runner.Run(ctx, "rev-parse", "--abbrev-ref", "HEAD")
	if err != nil {
		return "", gitError("git rev-parse", stderr, err)
	}
	branch := strings.TrimSpace(out)
	if branch == "" || branch == "HEAD" {
		return "", fmt.Errorf("当前不在任何分支上（detached HEAD）；请先切换到分支再重写历史")
	}
	return branch, nil
}

// remotes lists the configured remotes.
func remotes(ctx context.Context, runner gitRunner) ([]string, error) {
	out, stderr, err := runner.Run(ctx, "remote")
	if err != nil {
		return nil, gitError("git remote", stderr, err)
	}
	return strings.Fields(out), nil
}

// isWorktreeClean reports whether there are uncommitted changes. History
// rewriting with a dirty tree risks losing work on a failed checkout.
func isWorktreeClean(ctx context.Context, runner gitRunner) (bool, error) {
	out, stderr, err := runner.Run(ctx, "status", "--porcelain")
	if err != nil {
		return false, gitError("git status", stderr, err)
	}
	return strings.TrimSpace(out) == "", nil
}

// upstreamRef returns the ref a branch tracks (e.g. refs/remotes/origin/main),
// or "" when the branch has no upstream.
func upstreamRef(ctx context.Context, runner gitRunner, branch string) (string, error) {
	out, stderr, err := runner.Run(ctx, "for-each-ref",
		"--format=%(upstream)", "refs/heads/"+branch)
	if err != nil {
		return "", gitError("git for-each-ref", stderr, err)
	}
	return strings.TrimSpace(out), nil
}
