package executor

import (
	"context"
	"fmt"
	"os/exec"
	"runtime"
)
// runProcTree implements `kopsc proc tree`: show process tree.
func runProcTree(ctx context.Context, req BuiltinRequest) error {
	switch runtime.GOOS {
	case "linux", "darwin":
		return procTreeUnix(ctx, req)
	case "windows":
		return procTreeWindows(ctx, req)
	default:
		return fmt.Errorf("不支持的操作系统: %s", runtime.GOOS)
	}
}

func procTreeUnix(ctx context.Context, req BuiltinRequest) error {
	// Try pstree first (best output)
	if path, err := exec.LookPath("pstree"); err == nil {
		cmd := exec.CommandContext(ctx, path, "-p")
		cmd.Stdout = req.Stdout
		cmd.Stderr = req.Stderr
		return cmd.Run()
	}

	// Fallback: use ps with PPID and format a simple tree
	// macOS doesn't support -H, so we use a different approach
	if runtime.GOOS == "darwin" {
		cmd := exec.CommandContext(ctx, "ps", "-eo", "pid,ppid,comm")
		cmd.Stdout = req.Stdout
		cmd.Stderr = req.Stderr
		return cmd.Run()
	}

	// Linux fallback: ps with hierarchical output
	cmd := exec.CommandContext(ctx, "ps", "-ejH")
	cmd.Stdout = req.Stdout
	cmd.Stderr = req.Stderr
	return cmd.Run()
}

func procTreeWindows(ctx context.Context, req BuiltinRequest) error {
	cmd := exec.CommandContext(ctx, "tasklist", "/tree")
	cmd.Stdout = req.Stdout
	cmd.Stderr = req.Stderr
	return cmd.Run()
}
