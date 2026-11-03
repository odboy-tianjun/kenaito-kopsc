package executor

import (
	"context"
	"errors"
	"fmt"
	"strconv"
	"strings"
)

// gitSquashOptions are the parsed arguments of `kopsc git squash`.
type gitSquashOptions struct {
	Count   int
	Message string
}

// runGitSquash merges the last N commits into one.
func runGitSquash(ctx context.Context, req BuiltinRequest) error {
	return runGitSquashWith(ctx, req, execGitRunner{})
}

func runGitSquashWith(ctx context.Context, req BuiltinRequest, runner gitRunner) error {
	opts, confirmed, err := parseGitSquashArgs(req.Args)
	if err != nil {
		return err
	}
	confirmed = confirmed || req.Confirmed

	branch, err := currentBranch(ctx, runner)
	if err != nil {
		return err
	}

	// Get the last N commits for preview
	logOut, _, err := runner.Run(ctx, "log", "-n", strconv.Itoa(opts.Count), "--format=%h %s")
	if err != nil {
		return fmt.Errorf("获取提交历史失败: %w", err)
	}

	commits := strings.Split(strings.TrimSpace(logOut), "\n")
	if len(commits) < opts.Count {
		return fmt.Errorf("提交历史不足 %d 个（当前只有 %d 个）", opts.Count, len(commits))
	}

	// Show preview
	fmt.Fprintf(req.Stdout, "合并最近 %d 个提交为一个\n\n", opts.Count)
	fmt.Fprintf(req.Stdout, "分支: %s\n\n", branch)
	fmt.Fprintln(req.Stdout, "将被合并的提交:")
	for i, c := range commits[:opts.Count] {
		fmt.Fprintf(req.Stdout, "  %d. %s\n", i+1, c)
	}

	// Determine commit message
	msg := opts.Message
	if msg == "" {
		// Use the first (oldest) commit's message
		parts := strings.SplitN(commits[opts.Count-1], " ", 2)
		if len(parts) == 2 {
			msg = parts[1]
		}
	}
	fmt.Fprintf(req.Stdout, "\n新提交消息: %s\n", msg)

	if !confirmed {
		fmt.Fprintf(req.Stdout, "\n这是破坏性操作，会改变提交历史。加 --yes 确认执行。\n")
		return &ExitError{Code: 2, Err: errors.New("需要确认")}
	}

	// Execute squash: reset --soft HEAD~N then commit
	if _, stderr, err := runner.Run(ctx, "reset", "--soft", fmt.Sprintf("HEAD~%d", opts.Count)); err != nil {
		return fmt.Errorf("reset 失败: %w\n%s", err, stderr)
	}

	if _, stderr, err := runner.Run(ctx, "commit", "-m", msg); err != nil {
		return fmt.Errorf("提交失败: %w\n%s", err, stderr)
	}

	fmt.Fprintf(req.Stdout, "\n✓ 已将 %d 个提交合并为一个\n", opts.Count)
	return nil
}

func parseGitSquashArgs(args []string) (gitSquashOptions, bool, error) {
	var opts gitSquashOptions
	var confirmed bool
	var countStr string

	for i := 0; i < len(args); i++ {
		arg := args[i]
		switch {
		case arg == "--yes" || arg == "-y":
			confirmed = true
		case arg == "--message" || arg == "-m":
			if i+1 >= len(args) {
				return opts, false, fmt.Errorf("-m 缺少提交消息")
			}
			i++
			opts.Message = args[i]
		case strings.HasPrefix(arg, "-"):
			return opts, false, fmt.Errorf("未知选项 %q", arg)
		default:
			if countStr == "" {
				countStr = arg
			} else {
				return opts, false, fmt.Errorf("多余的参数 %q", arg)
			}
		}
	}

	if countStr == "" {
		return opts, false, fmt.Errorf("缺少数量参数；用法: kopsc git squash <数量>")
	}

	count, err := strconv.Atoi(countStr)
	if err != nil || count < 2 {
		return opts, false, fmt.Errorf("数量必须是大于 1 的整数")
	}
	opts.Count = count

	return opts, confirmed, nil
}
