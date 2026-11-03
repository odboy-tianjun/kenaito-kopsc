package executor

import (
	"context"
	"fmt"
	"os/exec"
	"runtime"
)

// runProcList implements `kopsc proc list`: list all processes.
func runProcList(ctx context.Context, req BuiltinRequest) error {
	switch runtime.GOOS {
	case "linux", "darwin":
		cmd := exec.CommandContext(ctx, "ps", "aux")
		cmd.Stdout = req.Stdout
		cmd.Stderr = req.Stderr
		return cmd.Run()
	case "windows":
		cmd := exec.CommandContext(ctx, "tasklist")
		cmd.Stdout = req.Stdout
		cmd.Stderr = req.Stderr
		return cmd.Run()
	default:
		return fmt.Errorf("不支持的操作系统: %s", runtime.GOOS)
	}
}
