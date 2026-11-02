package executor

import (
	"context"
	"fmt"
	"io/fs"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"time"
)

// runFileFindBig implements `kopsc file findbig`: find files larger than a
// threshold, sorted by size descending.
func runFileFindBig(ctx context.Context, req BuiltinRequest) error {
	opts, err := parseFileFindBigArgs(req.Args)
	if err != nil {
		return err
	}

	root, err := filepath.Abs(opts.path)
	if err != nil {
		return fmt.Errorf("无法解析路径 %q: %w", opts.path, err)
	}

	type fileEntry struct {
		path string
		size int64
	}
	var results []fileEntry

	err = filepath.WalkDir(root, func(path string, d fs.DirEntry, walkErr error) error {
		if walkErr != nil {
			return nil
		}
		if err := ctx.Err(); err != nil {
			return err
		}
		if d.IsDir() {
			return nil
		}
		info, err := d.Info()
		if err != nil || !info.Mode().IsRegular() {
			return nil
		}
		if info.Size() >= opts.threshold {
			rel, _ := filepath.Rel(root, path)
			if rel == "" {
				rel = path
			}
			results = append(results, fileEntry{path: rel, size: info.Size()})
		}
		return nil
	})
	if err != nil {
		return err
	}

	sort.Slice(results, func(i, j int) bool {
		return results[i].size > results[j].size
	})

	if len(results) == 0 {
		fmt.Fprintf(req.Stdout, "没有找到大于 %s 的文件\n", humanSize(opts.threshold))
		return nil
	}

	for _, f := range results {
		fmt.Fprintf(req.Stdout, "%-8s %s\n", humanSize(f.size), f.path)
	}
	fmt.Fprintf(req.Stdout, "\n共 %d 个文件大于 %s\n", len(results), humanSize(opts.threshold))
	return nil
}

type fileFindBigOptions struct {
	path      string
	threshold int64
}

func parseFileFindBigArgs(args []string) (fileFindBigOptions, error) {
	opts := fileFindBigOptions{path: ".", threshold: 100 * 1024 * 1024}
	var paths []string

	for i := 0; i < len(args); i++ {
		arg := args[i]
		switch {
		case arg == "--":
			paths = append(paths, args[i+1:]...)
			i = len(args)
		case arg == "--threshold" || arg == "-s":
			if i+1 >= len(args) {
				return opts, fmt.Errorf("%s 缺少大小参数，例如 --threshold 50M", arg)
			}
			i++
			n, err := parseSizeArg(args[i])
			if err != nil {
				return opts, fmt.Errorf("%s: %w", arg, err)
			}
			opts.threshold = n
		case strings.HasPrefix(arg, "--threshold="):
			n, err := parseSizeArg(strings.TrimPrefix(arg, "--threshold="))
			if err != nil {
				return opts, fmt.Errorf("--threshold: %w", err)
			}
			opts.threshold = n
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
	return opts, nil
}

// parseSizeArg parses human-friendly size strings: "100M", "1G", "500K", "1024".
func parseSizeArg(s string) (int64, error) {
	s = strings.TrimSpace(s)
	if s == "" {
		return 0, fmt.Errorf("缺少大小值")
	}
	multiplier := int64(1)
	upper := strings.ToUpper(s)
	if strings.HasSuffix(upper, "TB") || strings.HasSuffix(upper, "T") {
		multiplier = 1024 * 1024 * 1024 * 1024
	} else if strings.HasSuffix(upper, "GB") || strings.HasSuffix(upper, "G") {
		multiplier = 1024 * 1024 * 1024
	} else if strings.HasSuffix(upper, "MB") || strings.HasSuffix(upper, "M") {
		multiplier = 1024 * 1024
	} else if strings.HasSuffix(upper, "KB") || strings.HasSuffix(upper, "K") {
		multiplier = 1024
	} else if strings.HasSuffix(upper, "B") {
		multiplier = 1
	}

	numStr := s
	if multiplier > 1 {
		numStr = s[:len(s)-lenSuffix(s)]
	}
	numStr = strings.TrimSpace(numStr)

	n, err := strconv.ParseFloat(numStr, 64)
	if err != nil {
		return 0, fmt.Errorf("无法解析大小 %q", s)
	}
	return int64(n * float64(multiplier)), nil
}

func lenSuffix(s string) int {
	i := len(s)
	for i > 0 && !isDigit(s[i-1]) && s[i-1] != '.' {
		i--
	}
	return len(s) - i
}

func isDigit(b byte) bool {
	return b >= '0' && b <= '9'
}

// runFileFindRecent implements `kopsc file findrecent`: find files modified
// within the last N days.
func runFileFindRecent(ctx context.Context, req BuiltinRequest) error {
	opts, err := parseFileFindRecentArgs(req.Args)
	if err != nil {
		return err
	}

	root, err := filepath.Abs(opts.path)
	if err != nil {
		return fmt.Errorf("无法解析路径 %q: %w", opts.path, err)
	}

	cutoff := time.Now().AddDate(0, 0, -opts.days)
	type fileEntry struct {
		path    string
		modTime time.Time
	}
	var results []fileEntry

	err = filepath.WalkDir(root, func(path string, d fs.DirEntry, walkErr error) error {
		if walkErr != nil {
			return nil
		}
		if err := ctx.Err(); err != nil {
			return err
		}
		if d.IsDir() {
			return nil
		}
		info, err := d.Info()
		if err != nil || !info.Mode().IsRegular() {
			return nil
		}
		if info.ModTime().After(cutoff) {
			rel, _ := filepath.Rel(root, path)
			if rel == "" {
				rel = path
			}
			results = append(results, fileEntry{path: rel, modTime: info.ModTime()})
		}
		return nil
	})
	if err != nil {
		return err
	}

	sort.Slice(results, func(i, j int) bool {
		return results[i].modTime.After(results[j].modTime)
	})

	if len(results) == 0 {
		fmt.Fprintf(req.Stdout, "最近 %d 天内没有修改过的文件\n", opts.days)
		return nil
	}

	for _, f := range results {
		fmt.Fprintf(req.Stdout, "%s  %s\n", f.modTime.Format("2006-01-02 15:04"), f.path)
	}
	fmt.Fprintf(req.Stdout, "\n共 %d 个文件在最近 %d 天内修改\n", len(results), opts.days)
	return nil
}

type fileFindRecentOptions struct {
	path string
	days int
}

func parseFileFindRecentArgs(args []string) (fileFindRecentOptions, error) {
	opts := fileFindRecentOptions{path: ".", days: 7}
	var paths []string

	for i := 0; i < len(args); i++ {
		arg := args[i]
		switch {
		case arg == "--":
			paths = append(paths, args[i+1:]...)
			i = len(args)
		case arg == "--days" || arg == "-d":
			if i+1 >= len(args) {
				return opts, fmt.Errorf("%s 缺少天数", arg)
			}
			i++
			n, err := strconv.Atoi(args[i])
			if err != nil || n < 1 {
				return opts, fmt.Errorf("--days 需要正整数，收到 %q", args[i])
			}
			opts.days = n
		case strings.HasPrefix(arg, "--days="):
			n, err := strconv.Atoi(strings.TrimPrefix(arg, "--days="))
			if err != nil || n < 1 {
				return opts, fmt.Errorf("--days 需要正整数，收到 %q", strings.TrimPrefix(arg, "--days="))
			}
			opts.days = n
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
	return opts, nil
}

// runFileFindName implements `kopsc file findname`: find files by name pattern
// (case-insensitive glob match).
func runFileFindName(ctx context.Context, req BuiltinRequest) error {
	opts, err := parseFileFindNameArgs(req.Args)
	if err != nil {
		return err
	}

	root, err := filepath.Abs(opts.path)
	if err != nil {
		return fmt.Errorf("无法解析路径 %q: %w", opts.path, err)
	}

	var results []string
	err = filepath.WalkDir(root, func(path string, d fs.DirEntry, walkErr error) error {
		if walkErr != nil {
			return nil
		}
		if err := ctx.Err(); err != nil {
			return err
		}
		if d.IsDir() {
			return nil
		}
		name := d.Name()
		if matchGlob(opts.pattern, name) {
			rel, _ := filepath.Rel(root, path)
			if rel == "" {
				rel = path
			}
			results = append(results, rel)
		}
		return nil
	})
	if err != nil {
		return err
	}

	sort.Strings(results)

	if len(results) == 0 {
		fmt.Fprintf(req.Stdout, "没有找到匹配 %q 的文件\n", opts.pattern)
		return nil
	}

	for _, f := range results {
		fmt.Fprintln(req.Stdout, f)
	}
	fmt.Fprintf(req.Stdout, "\n共 %d 个文件匹配\n", len(results))
	return nil
}

type fileFindNameOptions struct {
	path    string
	pattern string
}

func parseFileFindNameArgs(args []string) (fileFindNameOptions, error) {
	opts := fileFindNameOptions{path: "."}
	var paths []string

	for i := 0; i < len(args); i++ {
		arg := args[i]
		switch {
		case arg == "--":
			remaining := args[i+1:]
			if len(remaining) > 0 && opts.pattern == "" {
				opts.pattern = remaining[0]
				paths = append(paths, remaining[1:]...)
			} else {
				paths = append(paths, remaining...)
			}
			i = len(args)
		case arg == "--pattern" || arg == "-p":
			if i+1 >= len(args) {
				return opts, fmt.Errorf("%s 缺少模式参数", arg)
			}
			i++
			opts.pattern = args[i]
		case strings.HasPrefix(arg, "--pattern="):
			opts.pattern = strings.TrimPrefix(arg, "--pattern=")
		case strings.HasPrefix(arg, "-") && arg != "-":
			return opts, fmt.Errorf("未知选项 %q", arg)
		default:
			if opts.pattern == "" {
				opts.pattern = arg
			} else {
				paths = append(paths, arg)
			}
		}
	}
	if opts.pattern == "" {
		return opts, fmt.Errorf("必须提供文件名模式，例如 kopsc file findname \"*.log\"")
	}
	if len(paths) > 1 {
		return opts, fmt.Errorf("一次只支持一个路径，收到 %d 个", len(paths))
	}
	if len(paths) == 1 {
		opts.path = paths[0]
	}
	return opts, nil
}

// matchGlob does a case-insensitive filepath.Match.
func matchGlob(pattern, name string) bool {
	if ok, _ := filepath.Match(strings.ToLower(pattern), strings.ToLower(name)); ok {
		return true
	}
	if ok, _ := filepath.Match(pattern, name); ok {
		return true
	}
	return false
}
