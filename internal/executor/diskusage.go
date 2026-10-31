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

// runDiskUsage implements `du -x --max-depth=1 -h <path> | sort -hr`.
//
// It reports every direct child of the given directory: regular files by their
// size, directories by the recursive size of their regular files. Symbolic
// links are not followed and, where the platform exposes device IDs, nested
// filesystems are not crossed (-x).
func runDiskUsage(ctx context.Context, req BuiltinRequest) error {
	paths, err := parseDiskUsageArgs(req.Args)
	if err != nil {
		return err
	}
	root := paths[0]

	rootInfo, err := os.Stat(root)
	if err != nil {
		return fmt.Errorf("无法访问 %q: %w", root, err)
	}
	if !rootInfo.IsDir() {
		return fmt.Errorf("%q 不是目录", root)
	}

	entries, err := os.ReadDir(root)
	if err != nil {
		return fmt.Errorf("读取目录 %q 失败: %w", root, err)
	}

	rootDev, checkDev := deviceOf(rootInfo)

	type entrySize struct {
		path string
		size int64
	}
	items := make([]entrySize, 0, len(entries))
	var total int64

	for _, entry := range entries {
		if err := ctx.Err(); err != nil {
			return err
		}
		path := filepath.Join(root, entry.Name())
		info, err := entry.Info()
		if err != nil {
			// Unreadable entries are skipped rather than failing the whole run.
			continue
		}

		var size int64
		switch {
		case info.IsDir():
			if checkDev {
				if dev, ok := deviceOf(info); ok && dev != rootDev {
					// -x: a different filesystem contributes nothing.
					break
				}
			}
			size, err = dirSize(ctx, path, rootDev, checkDev)
			if err != nil {
				return err
			}
		case info.Mode().IsRegular():
			size = info.Size()
		}

		items = append(items, entrySize{path: path, size: size})
		total += size
	}

	sort.SliceStable(items, func(i, j int) bool {
		if items[i].size != items[j].size {
			return items[i].size > items[j].size
		}
		return items[i].path < items[j].path
	})

	w := req.Stdout
	for _, item := range items {
		fmt.Fprintf(w, "%-8s %s\n", humanSize(item.size), item.path)
	}
	fmt.Fprintf(w, "%-8s 总计 %s\n", humanSize(total), root)
	return nil
}

// parseDiskUsageArgs accepts either a positional path or --path/--path=value.
// Exactly one path is required.
func parseDiskUsageArgs(args []string) ([]string, error) {
	var paths []string
	positionalOnly := false

	for i := 0; i < len(args); i++ {
		arg := args[i]
		switch {
		case positionalOnly:
			paths = append(paths, arg)
		case arg == "--":
			positionalOnly = true
		case arg == "--path":
			if i+1 >= len(args) {
				return nil, errors.New("--path 缺少参数，例如 --path /var")
			}
			paths = append(paths, args[i+1])
			i++
		case strings.HasPrefix(arg, "--path="):
			value := strings.TrimPrefix(arg, "--path=")
			if value == "" {
				return nil, errors.New("--path 缺少参数，例如 --path /var")
			}
			paths = append(paths, value)
		case strings.HasPrefix(arg, "-") && arg != "-":
			return nil, fmt.Errorf("未知选项 %q：深度(-d 1)、人类可读(-h)与不跨文件系统(-x)已固定", arg)
		default:
			paths = append(paths, arg)
		}
	}

	if len(paths) == 0 {
		return nil, errors.New("必须提供路径，例如 kopsc disk usage /var 或 kopsc disk usage --path /var")
	}
	if len(paths) > 1 {
		return nil, fmt.Errorf("一次只支持一个路径，收到 %d 个", len(paths))
	}
	return paths, nil
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
