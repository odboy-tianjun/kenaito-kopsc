package executor

import (
	"context"
	"fmt"
	"os/exec"
	"runtime"
	"strings"
)

// runDiskInode implements `kopsc disk inode`: show inode usage.
func runDiskInode(ctx context.Context, req BuiltinRequest) error {
	opts, err := parseDiskInodeArgs(req.Args)
	if err != nil {
		return err
	}

	path := opts.path
	if path == "" {
		path = "."
	}

	switch runtime.GOOS {
	case "linux", "darwin":
		return diskInodeUnix(ctx, req, path)
	case "windows":
		return fmt.Errorf("Windows 不支持 inode（NTFS 使用 MFT，无 inode 概念）")
	default:
		return fmt.Errorf("不支持的操作系统: %s", runtime.GOOS)
	}
}

type diskInodeOptions struct {
	path string
}

func parseDiskInodeArgs(args []string) (diskInodeOptions, error) {
	var opts diskInodeOptions
	var positional []string

	for i := 0; i < len(args); i++ {
		arg := args[i]
		switch {
		case arg == "--":
			positional = append(positional, args[i+1:]...)
			i = len(args)
		case strings.HasPrefix(arg, "-") && arg != "-":
			return opts, fmt.Errorf("未知选项 %q", arg)
		default:
			positional = append(positional, arg)
		}
	}

	if len(positional) > 0 {
		opts.path = positional[0]
	}
	return opts, nil
}

func diskInodeUnix(ctx context.Context, req BuiltinRequest, path string) error {
	// Use df -i to get inode info
	cmd := exec.CommandContext(ctx, "df", "-i", path)
	cmd.Stdout = req.Stdout
	cmd.Stderr = req.Stderr
	return cmd.Run()
}
