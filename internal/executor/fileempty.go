package executor

import (
	"context"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"sort"
	"strings"
)

// runFileEmpty implements `kopsc file empty`: find empty files and directories.
func runFileEmpty(ctx context.Context, req BuiltinRequest) error {
	opts, err := parseFileEmptyArgs(req.Args)
	if err != nil {
		return err
	}

	root, err := filepath.Abs(opts.path)
	if err != nil {
		return fmt.Errorf("无法解析路径 %q: %w", opts.path, err)
	}

	var emptyFiles []string
	var emptyDirs []string

	err = filepath.WalkDir(root, func(path string, d fs.DirEntry, walkErr error) error {
		if walkErr != nil {
			return nil
		}
		if err := ctx.Err(); err != nil {
			return err
		}

		if d.IsDir() {
			// Check if directory is empty
			entries, err := os.ReadDir(path)
			if err != nil {
				return nil
			}
			if len(entries) == 0 && path != root {
				rel, _ := filepath.Rel(root, path)
				if rel == "" {
					rel = path
				}
				emptyDirs = append(emptyDirs, rel)
			}
			return nil
		}

		// Check if file is empty
		info, err := d.Info()
		if err != nil {
			return nil
		}
		if info.Size() == 0 {
			rel, _ := filepath.Rel(root, path)
			if rel == "" {
				rel = path
			}
			emptyFiles = append(emptyFiles, rel)
		}
		return nil
	})
	if err != nil {
		return err
	}

	sort.Strings(emptyFiles)
	sort.Strings(emptyDirs)

	if !opts.dirsOnly {
		if len(emptyFiles) == 0 {
			fmt.Fprintln(req.Stdout, "没有空文件")
		} else {
			fmt.Fprintf(req.Stdout, "=== 空文件 (%d 个) ===\n", len(emptyFiles))
			for _, f := range emptyFiles {
				fmt.Fprintln(req.Stdout, f)
			}
			fmt.Fprintln(req.Stdout)
		}
	}

	if !opts.filesOnly {
		if len(emptyDirs) == 0 {
			fmt.Fprintln(req.Stdout, "没有空目录")
		} else {
			fmt.Fprintf(req.Stdout, "=== 空目录 (%d 个) ===\n", len(emptyDirs))
			for _, d := range emptyDirs {
				fmt.Fprintln(req.Stdout, d)
			}
		}
	}

	return nil
}

type fileEmptyOptions struct {
	path      string
	filesOnly bool
	dirsOnly  bool
}

func parseFileEmptyArgs(args []string) (fileEmptyOptions, error) {
	opts := fileEmptyOptions{path: "."}
	var paths []string

	for i := 0; i < len(args); i++ {
		arg := args[i]
		switch {
		case arg == "--":
			paths = append(paths, args[i+1:]...)
			i = len(args)
		case arg == "--files-only":
			opts.filesOnly = true
		case arg == "--dirs-only":
			opts.dirsOnly = true
		case strings.HasPrefix(arg, "-") && arg != "-":
			return opts, fmt.Errorf("未知选项 %q", arg)
		default:
			paths = append(paths, arg)
		}
	}
	if len(paths) > 1 {
		return opts, fmt.Errorf("一次只支持一个路径，收到 %d 个", len(paths))
	}
	if len(paths) == 1 {
		opts.path = paths[0]
	}
	if opts.filesOnly && opts.dirsOnly {
		return opts, fmt.Errorf("--files-only 和 --dirs-only 不能同时使用")
	}
	return opts, nil
}
