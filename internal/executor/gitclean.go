package executor

import (
	"context"
	"errors"
	"fmt"
	"strings"
)

// gitCleanOptions are the parsed arguments of `kopsc git clean`.
type gitCleanOptions struct {
	DryRun bool
}

// runGitClean removes untracked files and directories.
func runGitClean(ctx context.Context, req BuiltinRequest) error {
	return runGitCleanWith(ctx, req, execGitRunner{})
}

func runGitCleanWith(ctx context.Context, req BuiltinRequest, runner gitRunner) error {
	opts, confirmed, err := parseGitCleanArgs(req.Args)
	if err != nil {
		return err
	}
	confirmed = confirmed || req.Confirmed

	// First, show what will be cleaned (dry-run)
	dryOut, _, err := runner.Run(ctx, "clean", "-fd", "--dry-run")
	if err != nil {
		return fmt.Errorf("检查未跟踪文件失败: %w", err)
	}

	files := strings.TrimSpace(dryOut)
	if files == "" {
		fmt.Fprintf(req.Stdout, "没有未跟踪的文件或目录需要清理\n")
		return nil
	}

	// Count files
	lines := strings.Split(files, "\n")
	count := 0
	for _, line := range lines {
		if strings.HasPrefix(line, "Would remove ") {
			count++
		}
	}

	fmt.Fprintf(req.Stdout, "将清理 %d 个未跟踪的文件/目录:\n\n", count)
	for _, line := range lines {
		if strings.HasPrefix(line, "Would remove ") {
			fmt.Fprintf(req.Stdout, "  %s\n", strings.TrimPrefix(line, "Would remove "))
		}
	}

	if opts.DryRun {
		fmt.Fprintf(req.Stdout, "\n（仅预览，未执行任何操作）\n")
		return nil
	}

	if !confirmed {
		fmt.Fprintf(req.Stdout, "\n加 --yes 确认删除这些文件。\n")
		return &ExitError{Code: 2, Err: errors.New("需要确认")}
	}

	// Execute clean
	if _, stderr, err := runner.Run(ctx, "clean", "-fd"); err != nil {
		return fmt.Errorf("清理失败: %w\n%s", err, stderr)
	}

	fmt.Fprintf(req.Stdout, "\n✓ 已清理 %d 个未跟踪的文件/目录\n", count)
	return nil
}

func parseGitCleanArgs(args []string) (gitCleanOptions, bool, error) {
	var opts gitCleanOptions
	var confirmed bool

	for _, arg := range args {
		switch arg {
		case "--yes", "-y":
			confirmed = true
		case "--dry-run", "-n":
			opts.DryRun = true
		default:
			if strings.HasPrefix(arg, "-") {
				return opts, false, fmt.Errorf("未知选项 %q", arg)
			}
		}
	}

	return opts, confirmed, nil
}
