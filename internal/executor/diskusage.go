package executor

import (
	"context"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
)

// runDiskUsage implements `du -x --max-depth=N -h <path> | sort -hr`.
//
// It reports the entries down to N levels below the given directory: regular
// files by their size, directories by the recursive size of their regular
// files. Symbolic links are not followed and, where the platform exposes
// device IDs, nested filesystems are not crossed (-x).
func runDiskUsage(ctx context.Context, req BuiltinRequest) error {
	opts, err := parseDiskUsageOptions(req.Args)
	if err != nil {
		return err
	}
	root, depth := opts.path, opts.depth

	rootInfo, err := os.Stat(root)
	if err != nil {
		return fmt.Errorf("无法访问 %q: %w", root, err)
	}
	if !rootInfo.IsDir() {
		return fmt.Errorf("%q 不是目录", root)
	}

	rootDev, checkDev := deviceOf(rootInfo)

	// Each listed entry is a direct child, or with --depth a deeper one.
	// Every file's size is counted exactly once, so the total stays the real
	// directory size rather than summing overlapping subtrees.
	items := make([]entrySize, 0, 16)
	if err := collectEntries(ctx, root, root, 1, depth, rootDev, checkDev, &items); err != nil {
		return err
	}

	var total int64
	for _, item := range items {
		total += item.size
	}

	sort.SliceStable(items, func(i, j int) bool {
		if items[i].size != items[j].size {
			return items[i].size > items[j].size
		}
		return items[i].path < items[j].path
	})

	w := req.Stdout
	shownRoot := root
	for _, item := range items {
		fmt.Fprintf(w, "%-8s %s\n", humanSize(item.size), strings.TrimPrefix(item.path, "./"))
	}
	fmt.Fprintf(w, "%-8s 总计 %s\n", humanSize(total), shownRoot)
	return nil
}

// entrySize is one row of the report.
type entrySize struct {
	path string
	size int64
}

// collectEntries walks dir down to maxDepth levels. Files are added with their
// own size; a directory at the deepest level is added with its recursive size
// (the "how big is this subtree" question), and shallower directories are
// descended into. Symbolic links are skipped and other filesystems are not
// crossed (the -x behaviour).
func collectEntries(ctx context.Context, dir, root string, level, maxDepth int, rootDev uint64, checkDev bool, out *[]entrySize) error {
	entries, err := os.ReadDir(dir)
	if err != nil {
		if dir == root {
			return fmt.Errorf("读取目录 %q 失败: %w", dir, err)
		}
		return nil // an unreadable subdirectory must not abort the report
	}

	for _, entry := range entries {
		if err := ctx.Err(); err != nil {
			return err
		}
		path := filepath.Join(dir, entry.Name())
		info, err := entry.Info()
		if err != nil {
			continue // unreadable entries are skipped
		}

		switch {
		case info.IsDir():
			if checkDev {
				if dev, ok := deviceOf(info); ok && dev != rootDev {
					// -x: a different filesystem contributes nothing.
					continue
				}
			}
			if level >= maxDepth {
				size, err := dirSize(ctx, path, rootDev, checkDev)
				if err != nil {
					return err
				}
				*out = append(*out, entrySize{path: path, size: size})
				continue
			}
			if err := collectEntries(ctx, path, root, level+1, maxDepth, rootDev, checkDev, out); err != nil {
				return err
			}
		case info.Mode().IsRegular():
			*out = append(*out, entrySize{path: path, size: info.Size()})
		}
	}
	return nil
}

// diskUsageOptions are the parsed arguments of `kopsc disk usage`.
type diskUsageOptions struct {
	path  string
	depth int
}

// parseDiskUsageOptions accepts either a positional path or
// --path/--path=value, plus --depth/-d. At most one path may be given;
// without one the current directory is used.
func parseDiskUsageOptions(args []string) (diskUsageOptions, error) {
	paths, depth, err := parseDiskUsageArgs(args)
	if err != nil {
		return diskUsageOptions{}, err
	}
	return diskUsageOptions{path: paths[0], depth: depth}, nil
}

// parseDiskUsageArgs returns the path and the depth (1 by default).
func parseDiskUsageArgs(args []string) ([]string, int, error) {
	var paths []string
	depth := 1
	positionalOnly := false

	for i := 0; i < len(args); i++ {
		arg := args[i]
		switch {
		case positionalOnly:
			paths = append(paths, arg)
		case arg == "--":
			positionalOnly = true
		case arg == "--depth" || arg == "-d":
			if i+1 >= len(args) {
				return nil, 0, fmt.Errorf("%s 缺少层数，例如 --depth 2", arg)
			}
			i++
			n, err := strconv.Atoi(args[i])
			if err != nil || n < 1 {
				return nil, 0, fmt.Errorf("--depth 需要大于 0 的整数，收到 %q", args[i])
			}
			depth = n
		case strings.HasPrefix(arg, "--depth="):
			value := strings.TrimPrefix(arg, "--depth=")
			n, err := strconv.Atoi(value)
			if err != nil || n < 1 {
				return nil, 0, fmt.Errorf("--depth 需要大于 0 的整数，收到 %q", value)
			}
			depth = n
		case arg == "--path":
			if i+1 >= len(args) {
				return nil, 0, errors.New("--path 缺少参数，例如 --path /var")
			}
			paths = append(paths, args[i+1])
			i++
		case strings.HasPrefix(arg, "--path="):
			value := strings.TrimPrefix(arg, "--path=")
			if value == "" {
				return nil, 0, errors.New("--path 缺少参数，例如 --path /var")
			}
			paths = append(paths, value)
		case strings.HasPrefix(arg, "-") && arg != "-":
			return nil, 0, fmt.Errorf("未知选项 %q：人类可读(-h)与不跨文件系统(-x)已固定，深度请用 --depth N", arg)
		default:
			paths = append(paths, arg)
		}
	}

	// The most common use is "what is taking space right here", so the path
	// defaults to the current directory.
	if len(paths) == 0 {
		paths = append(paths, ".")
	}
	if len(paths) > 1 {
		return nil, 0, fmt.Errorf("一次只支持一个路径，收到 %d 个", len(paths))
	}
	return paths, depth, nil
}

// dirSize sums the sizes of regular files under dir without following symbolic
// links or crossing into another filesystem.
func dirSize(ctx context.Context, dir string, rootDev uint64, checkDev bool) (int64, error) {
	var total int64

	err := filepath.WalkDir(dir, func(path string, d fs.DirEntry, walkErr error) error {
		if walkErr != nil {
			// Permission problems on one subtree must not abort the report.
			return nil
		}
		if err := ctx.Err(); err != nil {
			return err
		}
		if d.IsDir() {
			if path != dir && checkDev {
				if dev, ok := deviceOfPath(path); ok && dev != rootDev {
					return fs.SkipDir
				}
			}
			return nil
		}
		info, err := d.Info()
		if err != nil {
			return nil
		}
		if info.Mode().IsRegular() {
			total += info.Size()
		}
		return nil
	})
	if err != nil {
		return total, err
	}
	return total, nil
}

// humanSize formats bytes the way `du -h` does: bytes below 1 KiB are printed
// bare, larger values use binary units with one decimal.
func humanSize(n int64) string {
	if n < 1024 {
		return strconv.FormatInt(n, 10)
	}
	units := []string{"K", "M", "G", "T", "P", "E"}
	value := float64(n)
	unit := -1
	for value >= 1024 && unit < len(units)-1 {
		value /= 1024
		unit++
	}
	return fmt.Sprintf("%.1f%s", value, units[unit])
}
