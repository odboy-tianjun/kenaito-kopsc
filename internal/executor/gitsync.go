package executor

import (
	"context"
	"errors"
	"fmt"
	"strings"
)

// runGitSync syncs the current branch with its remote tracking branch.
func runGitSync(ctx context.Context, req BuiltinRequest) error {
	return runGitSyncWith(ctx, req, execGitRunner{})
}

func runGitSyncWith(ctx context.Context, req BuiltinRequest, runner gitRunner) error {
	branch, err := currentBranch(ctx, runner)
	if err != nil {
		return err
	}

	// Detect remote branch name (main or master)
	remoteBranch, err := detectRemoteBranch(ctx, runner, branch)
	if err != nil {
		return err
	}

	fmt.Fprintf(req.Stdout, "同步分支: %s -> %s\n\n", branch, remoteBranch)

	// Fetch first
	fmt.Fprintf(req.Stdout, "正在 fetch...\n")
	if _, stderr, err := runner.Run(ctx, "fetch", "origin"); err != nil {
		return fmt.Errorf("fetch 失败: %w\n%s", err, stderr)
	}

	// Check if there are changes to rebase
	localOut, _, _ := runner.Run(ctx, "rev-parse", "HEAD")
	remoteOut, _, _ := runner.Run(ctx, "rev-parse", remoteBranch)

	local := strings.TrimSpace(localOut)
	remote := strings.TrimSpace(remoteOut)

	if local == remote {
		fmt.Fprintf(req.Stdout, "\n✓ 已经是最新，无需同步\n")
		return nil
	}

	// Check if we can fast-forward
	baseOut, _, _ := runner.Run(ctx, "merge-base", "HEAD", remoteBranch)
	base := strings.TrimSpace(baseOut)

	if base == local {
		// Can fast-forward
		fmt.Fprintf(req.Stdout, "正在 fast-forward 到 %s...\n", remoteBranch)
		if _, stderr, err := runner.Run(ctx, "merge", "--ff-only", remoteBranch); err != nil {
			return fmt.Errorf("merge 失败: %w\n%s", err, stderr)
		}
		fmt.Fprintf(req.Stdout, "\n✓ 已同步到远程\n")
	} else {
		// Need rebase
		fmt.Fprintf(req.Stdout, "正在 rebase 到 %s...\n", remoteBranch)
		if _, stderr, err := runner.Run(ctx, "rebase", remoteBranch); err != nil {
			return fmt.Errorf("rebase 失败: %w\n%s\n\n提示: 如有冲突，请手动解决后执行 git rebase --continue", err, stderr)
		}
		fmt.Fprintf(req.Stdout, "\n✓ 已 rebase 到远程\n")
	}

	return nil
}

// runGitDiscard discards all local changes and resets to remote.
func runGitDiscard(ctx context.Context, req BuiltinRequest) error {
	return runGitDiscardWith(ctx, req, execGitRunner{})
}

func runGitDiscardWith(ctx context.Context, req BuiltinRequest, runner gitRunner) error {
	confirmed := false
	for _, arg := range req.Args {
		if arg == "--yes" || arg == "-y" {
			confirmed = true
		}
	}
	confirmed = confirmed || req.Confirmed

	branch, err := currentBranch(ctx, runner)
	if err != nil {
		return err
	}

	// Detect remote branch name
	remoteBranch, err := detectRemoteBranch(ctx, runner, branch)
	if err != nil {
		return err
	}

	// Check for local changes
	statusOut, _, _ := runner.Run(ctx, "status", "--porcelain")
	hasChanges := strings.TrimSpace(statusOut) != ""

	// Show preview
	fmt.Fprintf(req.Stdout, "放弃本地所有改动，以远程为主\n\n")
	fmt.Fprintf(req.Stdout, "分支: %s -> %s\n", branch, remoteBranch)

	if hasChanges {
		fmt.Fprintf(req.Stdout, "\n⚠️  以下本地改动将被丢弃:\n")
		lines := strings.Split(strings.TrimSpace(statusOut), "\n")
		count := 0
		for _, line := range lines {
			if count < 10 {
				fmt.Fprintf(req.Stdout, "  %s\n", line)
			}
			count++
		}
		if count > 10 {
			fmt.Fprintf(req.Stdout, "  ... 还有 %d 个文件\n", count-10)
		}
	} else {
		fmt.Fprintf(req.Stdout, "\n工作区干净，将重置到远程分支\n")
	}

	if !confirmed {
		fmt.Fprintf(req.Stdout, "\n⚠️  这是破坏性操作，将永久丢弃所有本地改动！加 --yes 确认执行。\n")
		return &ExitError{Code: 2, Err: errors.New("需要确认")}
	}

	// Fetch first
	fmt.Fprintf(req.Stdout, "\n正在 fetch...\n")
	if _, stderr, err := runner.Run(ctx, "fetch", "origin"); err != nil {
		return fmt.Errorf("fetch 失败: %w\n%s", err, stderr)
	}

	// Reset hard to remote
	if _, stderr, err := runner.Run(ctx, "reset", "--hard", remoteBranch); err != nil {
		return fmt.Errorf("reset 失败: %w\n%s", err, stderr)
	}

	// Clean untracked files
	runner.Run(ctx, "clean", "-fd")

	fmt.Fprintf(req.Stdout, "\n✓ 已重置到 %s，所有本地改动已丢弃\n", remoteBranch)
	return nil
}

// detectRemoteBranch detects whether the remote tracking branch is main or master.
func detectRemoteBranch(ctx context.Context, runner gitRunner, localBranch string) (string, error) {
	// Try main first
	if _, _, err := runner.Run(ctx, "rev-parse", "origin/main"); err == nil {
		return "origin/main", nil
	}
	// Try master
	if _, _, err := runner.Run(ctx, "rev-parse", "origin/master"); err == nil {
		return "origin/master", nil
	}
	// Try the same branch name on remote
	if _, _, err := runner.Run(ctx, "rev-parse", fmt.Sprintf("origin/%s", localBranch)); err == nil {
		return fmt.Sprintf("origin/%s", localBranch), nil
	}
	return "", fmt.Errorf("无法检测到远程分支（尝试了 origin/main、origin/master、origin/%s）", localBranch)
}
