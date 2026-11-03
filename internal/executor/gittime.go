package executor

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"
)

// gitTimeOptions are the parsed arguments of `kopsc git time`.
type gitTimeOptions struct {
	Time    time.Time
	Remote  string
	Branch  string
	NoPush  bool
}

// runGitTime modifies the timestamp of the last commit to the specified time
// and force pushes the branch.
//
// Without --yes it only shows what would happen (exit code 2), so the destructive
// path always needs an explicit confirmation.
func runGitTime(ctx context.Context, req BuiltinRequest) error {
	return runGitTimeWith(ctx, req, execGitRunner{})
}

// runGitTimeWith is runGitTime with its dependencies injected for tests.
func runGitTimeWith(ctx context.Context, req BuiltinRequest, runner gitRunner) error {
	opts, confirmedInArgs, err := parseGitTimeArgs(req.Args)
	if err != nil {
		return err
	}
	confirmed := req.Confirmed || confirmedInArgs

	branch := opts.Branch
	if branch == "" {
		if branch, err = currentBranch(ctx, runner); err != nil {
			return err
		}
	}

	remote := opts.Remote
	if remote == "" && !opts.NoPush {
		names, err := remotes(ctx, runner)
		if err != nil {
			return err
		}
		switch {
		case len(names) == 1:
			remote = names[0]
		case len(names) == 0:
			return errors.New("仓库没有配置远程；用 --no-push 只改写本地，或先 git remote add")
		default:
			return fmt.Errorf("仓库有多个远程（%s），请用 --remote 指定要强推哪个",
				strings.Join(names, ", "))
		}
	}

	// Get the last commit info
	lastCommit, err := getLastCommit(ctx, runner)
	if err != nil {
		return err
	}

	// Show preview
	fmt.Fprintf(req.Stdout, "修改最后一次提交的时间\n\n")
	fmt.Fprintf(req.Stdout, "提交: %s %s\n", lastCommit.Short, lastCommit.Subject)
	fmt.Fprintf(req.Stdout, "当前时间: %s\n", lastCommit.AuthorTime.Format("2006-01-02 15:04:05"))
	fmt.Fprintf(req.Stdout, "修改为:   %s\n", opts.Time.Format("2006-01-02 15:04:05"))
	fmt.Fprintf(req.Stdout, "\n")

	if !opts.NoPush {
		fmt.Fprintf(req.Stdout, "将强推到 %s/%s\n", remote, branch)
	}

	if !confirmed {
		fmt.Fprintf(req.Stdout, "\n这是破坏性操作，会改变提交历史。加 --yes 确认执行。\n")
		return &ExitError{Code: 2, Err: errors.New("需要确认")}
	}

	// Execute the time change
	// Must set both GIT_COMMITTER_DATE and --date to change both timestamps
	timeStr := opts.Time.Format("2006-01-02 15:04:05")
	env := []string{"GIT_COMMITTER_DATE=" + timeStr}
	
	if _, stderr, err := runner.RunEnv(ctx, env, "commit", "--amend", "--date="+timeStr, "--no-edit"); err != nil {
		return fmt.Errorf("修改提交时间失败: %w\n%s", err, stderr)
	}

	fmt.Fprintf(req.Stdout, "✓ 已修改最后一次提交的时间\n")

	// Force push if not --no-push
	if !opts.NoPush {
		fmt.Fprintf(req.Stdout, "强推到 %s/%s...\n", remote, branch)
		
		if _, stderr, err := runner.Run(ctx, "push", "--force", remote, branch); err != nil {
			return fmt.Errorf("强推失败: %w\n%s", err, stderr)
		}
		
		fmt.Fprintf(req.Stdout, "✓ 已强推\n")
	}

	return nil
}

// parseGitTimeArgs parses the arguments for `kopsc git time`.
func parseGitTimeArgs(args []string) (gitTimeOptions, bool, error) {
	var opts gitTimeOptions
	var confirmed bool
	var timeStr string

	for i := 0; i < len(args); i++ {
		arg := args[i]
		switch {
		case arg == "--yes" || arg == "-y":
			confirmed = true
		case arg == "--no-push":
			opts.NoPush = true
		case arg == "--remote":
			if i+1 >= len(args) {
				return opts, false, fmt.Errorf("--remote 缺少远程名称")
			}
			i++
			opts.Remote = args[i]
		case strings.HasPrefix(arg, "--remote="):
			opts.Remote = strings.TrimPrefix(arg, "--remote=")
		case arg == "--branch":
			if i+1 >= len(args) {
				return opts, false, fmt.Errorf("--branch 缺少分支名称")
			}
			i++
			opts.Branch = args[i]
		case strings.HasPrefix(arg, "--branch="):
			opts.Branch = strings.TrimPrefix(arg, "--branch=")
		case strings.HasPrefix(arg, "-"):
			return opts, false, fmt.Errorf("未知选项 %q", arg)
		default:
			if timeStr == "" {
				timeStr = arg
			} else {
				return opts, false, fmt.Errorf("多余的参数 %q", arg)
			}
		}
	}

	if timeStr == "" {
		return opts, false, fmt.Errorf("缺少时间参数；用法: kopsc git time <时间>")
	}

	// Parse and validate time format: yyyy-MM-dd HH:mm:ss
	t, err := time.ParseInLocation("2006-01-02 15:04:05", timeStr, time.Local)
	if err != nil {
		return opts, false, fmt.Errorf("时间格式错误: %q，必须是 yyyy-MM-dd HH:mm:ss（如 2026-11-05 22:30:00）", timeStr)
	}
	opts.Time = t

	return opts, confirmed, nil
}

// gitCommitInfo holds basic info about a commit.
type gitCommitInfo struct {
	Hash       string
	Short      string
	Subject    string
	AuthorTime time.Time
}

// getLastCommit returns info about the last commit.
func getLastCommit(ctx context.Context, runner gitRunner) (gitCommitInfo, error) {
	// Get commit hash and subject
	out, stderr, err := runner.Run(ctx, "log", "-1", "--format=%H|%h|%s")
	if err != nil {
		return gitCommitInfo{}, fmt.Errorf("获取提交信息失败: %w\n%s", err, stderr)
	}
	
	parts := strings.SplitN(strings.TrimSpace(out), "|", 3)
	if len(parts) != 3 {
		return gitCommitInfo{}, fmt.Errorf("解析提交信息失败: %s", out)
	}

	// Get author time
	timeOut, stderr, err := runner.Run(ctx, "log", "-1", "--format=%ai")
	if err != nil {
		return gitCommitInfo{}, fmt.Errorf("获取提交时间失败: %w\n%s", err, stderr)
	}

	authorTime, err := time.Parse("2006-01-02 15:04:05 -0700", strings.TrimSpace(timeOut))
	if err != nil {
		return gitCommitInfo{}, fmt.Errorf("解析提交时间失败: %w", err)
	}

	return gitCommitInfo{
		Hash:       parts[0],
		Short:      parts[1],
		Subject:    parts[2],
		AuthorTime: authorTime,
	}, nil
}
