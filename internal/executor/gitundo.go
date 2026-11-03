package executor

import (
	"context"
	"errors"
	"fmt"
	"strings"
)

// gitUndoOptions are the parsed arguments of `kopsc git undo`.
type gitUndoOptions struct {
	Hard bool
}

// runGitUndo undoes the last commit.
func runGitUndo(ctx context.Context, req BuiltinRequest) error {
	return runGitUndoWith(ctx, req, execGitRunner{})
}

func runGitUndoWith(ctx context.Context, req BuiltinRequest, runner gitRunner) error {
	opts, confirmed, err := parseGitUndoArgs(req.Args)
	if err != nil {
		return err
	}
	confirmed = confirmed || req.Confirmed

	// Get the last commit info
	out, _, err := runner.Run(ctx, "log", "-1", "--format=%h %s")
	if err != nil {
		return fmt.Errorf("获取提交信息失败: %w", err)
	}

	lastCommit := strings.TrimSpace(out)
	if lastCommit == "" {
		return errors.New("没有提交可以撤销")
	}

	// Show preview
	fmt.Fprintf(req.Stdout, "撤销最后一次提交\n\n")
	fmt.Fprintf(req.Stdout, "提交: %s\n", lastCommit)

	if opts.Hard {
		fmt.Fprintf(req.Stdout, "模式: 完全丢弃改动（--hard）\n")
	} else {
		fmt.Fprintf(req.Stdout, "模式: 保留改动到暂存区（--soft）\n")
	}

	if !confirmed {
		fmt.Fprintf(req.Stdout, "\n")
		if opts.Hard {
			fmt.Fprintf(req.Stdout, "⚠️  这将永久丢弃最后一次提交的改动！加 --yes 确认执行。\n")
		} else {
			fmt.Fprintf(req.Stdout, "撤销后改动会保留在暂存区，加 --yes 执行。\n")
		}
		return &ExitError{Code: 2, Err: errors.New("需要确认")}
	}

	// Execute undo
	resetArg := "--soft"
	if opts.Hard {
		resetArg = "--hard"
	}

	if _, stderr, err := runner.Run(ctx, "reset", resetArg, "HEAD~1"); err != nil {
		return fmt.Errorf("撤销失败: %w\n%s", err, stderr)
	}

	if opts.Hard {
		fmt.Fprintf(req.Stdout, "\n✓ 已撤销提交并丢弃改动\n")
	} else {
		fmt.Fprintf(req.Stdout, "\n✓ 已撤销提交，改动保留在暂存区\n")
	}
	return nil
}

func parseGitUndoArgs(args []string) (gitUndoOptions, bool, error) {
	var opts gitUndoOptions
	var confirmed bool

	for _, arg := range args {
		switch arg {
		case "--yes", "-y":
			confirmed = true
		case "--hard":
			opts.Hard = true
		default:
			if strings.HasPrefix(arg, "-") {
				return opts, false, fmt.Errorf("未知选项 %q", arg)
			}
		}
	}

	return opts, confirmed, nil
}
