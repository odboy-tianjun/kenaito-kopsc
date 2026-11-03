package executor

import (
	"context"
	"fmt"
	"io"
	"io/fs"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"sort"
	"strconv"
	"strings"
)

// runDiskCheck implements `kopsc disk check`: comprehensive disk health check.
func runDiskCheck(ctx context.Context, req BuiltinRequest) error {
	opts, err := parseDiskCheckArgs(req.Args)
	if err != nil {
		return err
	}

	w := req.Stdout

	fmt.Fprintln(w, "=== 磁盘健康检查 ===")
	fmt.Fprintln(w)

	if opts.all {
		// Check all mount points
		return diskCheckAll(ctx, req)
	}

	path := opts.path
	if path == "" {
		path = "/"
	}

	return diskCheckPath(ctx, req, path)
}

type diskCheckOptions struct {
	path string
	all  bool
}

func parseDiskCheckArgs(args []string) (diskCheckOptions, error) {
	var opts diskCheckOptions
	var positional []string

	for i := 0; i < len(args); i++ {
		arg := args[i]
		switch {
		case arg == "--":
			positional = append(positional, args[i+1:]...)
			i = len(args)
		case arg == "--all" || arg == "-a":
			opts.all = true
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

func diskCheckAll(ctx context.Context, req BuiltinRequest) error {
	// Get all mount points from df
	var cmd *exec.Cmd
	if runtime.GOOS == "windows" {
		cmd = exec.CommandContext(ctx, "wmic", "logicaldisk", "get", "caption,size,freespace", "/format:list")
	} else {
		cmd = exec.CommandContext(ctx, "df", "-h")
	}

	out, err := cmd.Output()
	if err != nil {
		return fmt.Errorf("获取磁盘信息失败: %w", err)
	}

	// Parse df output to get mount points
	lines := strings.Split(strings.TrimSpace(string(out)), "\n")
	if len(lines) < 2 {
		return fmt.Errorf("无法解析磁盘信息")
	}

	// Skip header line
	for _, line := range lines[1:] {
		fields := strings.Fields(line)
		if len(fields) < 6 {
			continue
		}
		mountPoint := fields[len(fields)-1]
		if runtime.GOOS != "windows" {
			fmt.Fprintf(req.Stdout, "\n--- %s ---\n\n", mountPoint)
			if err := diskCheckPath(ctx, req, mountPoint); err != nil {
				fmt.Fprintf(req.Stderr, "检查 %s 失败: %v\n", mountPoint, err)
			}
		}
	}
	return nil
}

func diskCheckPath(ctx context.Context, req BuiltinRequest, path string) error {
	w := req.Stdout

	// 1. Space usage
	fmt.Fprintln(w, "[1/5] 空间使用")
	diskCheckSpace(ctx, w, path)
	fmt.Fprintln(w)

	// 2. Inode usage (Unix only)
	if runtime.GOOS != "windows" {
		fmt.Fprintln(w, "[2/5] inode 使用")
		diskCheckInode(ctx, w, path)
		fmt.Fprintln(w)
	} else {
		fmt.Fprintln(w, "[2/5] inode 使用")
		fmt.Fprintln(w, "  Windows 不支持 inode")
		fmt.Fprintln(w)
	}

	// 3. Large files
	fmt.Fprintln(w, "[3/5] 大文件 TOP 5（>100M）")
	diskCheckLargeFiles(ctx, w, path)
	fmt.Fprintln(w)

	// 4. Large directories
	fmt.Fprintln(w, "[4/5] 大目录 TOP 5")
	diskCheckLargeDirs(ctx, w, path)
	fmt.Fprintln(w)

	// 5. Recommendations
	fmt.Fprintln(w, "[5/5] 建议")
	diskCheckRecommendations(ctx, w, path)

	return nil
}

func diskCheckSpace(ctx context.Context, w io.Writer, path string) {
	var cmd *exec.Cmd
	if runtime.GOOS == "windows" {
		cmd = exec.CommandContext(ctx, "wmic", "logicaldisk", "get", "size,freespace,caption", "/format:list")
	} else {
		cmd = exec.CommandContext(ctx, "df", "-h", path)
	}

	out, err := cmd.Output()
	if err != nil {
		fmt.Fprintln(w, "  无法获取磁盘空间信息")
		return
	}

	lines := strings.Split(strings.TrimSpace(string(out)), "\n")
	if len(lines) < 2 {
		fmt.Fprintln(w, "  无法解析磁盘信息")
		return
	}

	// Parse and display
	for _, line := range lines[1:] {
		fields := strings.Fields(line)
		if len(fields) < 5 {
			continue
		}

		if runtime.GOOS == "windows" {
			// Parse WMIC output
			fmt.Fprintf(w, "  %s\n", line)
		} else {
			// Parse df output: Filesystem Size Used Avail Use% Mounted
			filesystem := fields[0]
			size := fields[1]
			used := fields[2]
			avail := fields[3]
			usePct := fields[4]
			mount := fields[5]

			status := "✓ 正常"
			if pct, err := strconv.Atoi(strings.TrimSuffix(usePct, "%")); err == nil {
				if pct >= 90 {
					status = "✗ 危险！空间即将耗尽"
				} else if pct >= 80 {
					status = "! 警告：空间使用率较高"
				}
			}

			fmt.Fprintf(w, "  %s  %s 总计  %s 已用  %s 可用  %s 使用率  %s\n",
				filesystem, size, used, avail, usePct, status)
			if mount != path {
				fmt.Fprintf(w, "    挂载点: %s\n", mount)
			}
		}
	}
}

func diskCheckInode(ctx context.Context, w io.Writer, path string) {
	cmd := exec.CommandContext(ctx, "df", "-i", path)
	out, err := cmd.Output()
	if err != nil {
		fmt.Fprintln(w, "  无法获取 inode 信息")
		return
	}

	lines := strings.Split(strings.TrimSpace(string(out)), "\n")
	if len(lines) < 2 {
		fmt.Fprintln(w, "  无法解析 inode 信息")
		return
	}

	for _, line := range lines[1:] {
		fields := strings.Fields(line)
		if len(fields) < 5 {
			continue
		}

		filesystem := fields[0]
		inodes := fields[1]
		used := fields[2]
		free := fields[3]
		usePct := fields[4]

		status := "✓ 正常"
		if pct, err := strconv.Atoi(strings.TrimSuffix(usePct, "%")); err == nil {
			if pct >= 90 {
				status = "✗ 危险！inode 即将耗尽"
			} else if pct >= 80 {
				status = "! 警告：inode 使用率较高"
			}
		}

		fmt.Fprintf(w, "  %s  %s 总计  %s 已用  %s 可用  %s 使用率  %s\n",
			filesystem, inodes, used, free, usePct, status)
	}
}

func diskCheckLargeFiles(ctx context.Context, w io.Writer, path string) {
	type fileEntry struct {
		path string
		size int64
	}

	var files []fileEntry
	threshold := int64(100 * 1024 * 1024) // 100MB

	err := filepath.WalkDir(path, func(p string, d fs.DirEntry, err error) error {
		if err != nil {
			return nil // Skip errors
		}
		if d.IsDir() {
			// Skip common non-user directories
			base := filepath.Base(p)
			if base == ".git" || base == "node_modules" || base == ".cache" {
				return filepath.SkipDir
			}
			return nil
		}
		if info, err := d.Info(); err == nil && info.Mode().IsRegular() {
			if info.Size() >= threshold {
				files = append(files, fileEntry{path: p, size: info.Size()})
			}
		}
		return nil
	})

	if err != nil {
		fmt.Fprintf(w, "  扫描失败: %v\n", err)
		return
	}

	if len(files) == 0 {
		fmt.Fprintln(w, "  没有发现大于 100M 的文件 ✓")
		return
	}

	// Sort by size descending
	sort.Slice(files, func(i, j int) bool {
		return files[i].size > files[j].size
	})

	// Show top 5
	limit := 5
	if len(files) < limit {
		limit = len(files)
	}

	for i := 0; i < limit; i++ {
		fmt.Fprintf(w, "  %s  %s\n", humanSize(files[i].size), files[i].path)
	}

	if len(files) > 5 {
		fmt.Fprintf(w, "  ... 还有 %d 个大文件\n", len(files)-5)
	}
}

func diskCheckLargeDirs(ctx context.Context, w io.Writer, path string) {
	type dirEntry struct {
		path string
		size int64
	}

	var dirs []dirEntry

	// Only check direct children
	entries, err := os.ReadDir(path)
	if err != nil {
		fmt.Fprintf(w, "  无法读取目录: %v\n", err)
		return
	}

	for _, entry := range entries {
		if !entry.IsDir() {
			continue
		}
		dirPath := filepath.Join(path, entry.Name())
		size, err := dirSize(ctx, dirPath, 0, false)
		if err != nil {
			continue
		}
		if size > 0 {
			dirs = append(dirs, dirEntry{path: dirPath, size: size})
		}
	}

	if len(dirs) == 0 {
		fmt.Fprintln(w, "  没有子目录")
		return
	}

	// Sort by size descending
	sort.Slice(dirs, func(i, j int) bool {
		return dirs[i].size > dirs[j].size
	})

	// Show top 5
	limit := 5
	if len(dirs) < limit {
		limit = len(dirs)
	}

	for i := 0; i < limit; i++ {
		fmt.Fprintf(w, "  %s  %s\n", humanSize(dirs[i].size), dirs[i].path)
	}
}

func diskCheckRecommendations(ctx context.Context, w io.Writer, path string) {
	var recommendations []string

	// Check disk usage percentage
	cmd := exec.CommandContext(ctx, "df", path)
	out, err := cmd.Output()
	if err == nil {
		lines := strings.Split(strings.TrimSpace(string(out)), "\n")
		if len(lines) >= 2 {
			fields := strings.Fields(lines[1])
			if len(fields) >= 5 {
				usePct := fields[4]
				if pct, err := strconv.Atoi(strings.TrimSuffix(usePct, "%")); err == nil {
					if pct >= 90 {
						recommendations = append(recommendations, "✗ 磁盘空间严重不足！建议立即清理或扩容")
					} else if pct >= 80 {
						recommendations = append(recommendations, "! 磁盘空间使用率较高，建议清理不必要的文件")
					} else {
						recommendations = append(recommendations, "✓ 磁盘空间充足")
					}
				}
			}
		}
	}

	// Check inode usage
	if runtime.GOOS != "windows" {
		cmd = exec.CommandContext(ctx, "df", "-i", path)
		out, err = cmd.Output()
		if err == nil {
			lines := strings.Split(strings.TrimSpace(string(out)), "\n")
			if len(lines) >= 2 {
				fields := strings.Fields(lines[1])
				if len(fields) >= 5 {
					usePct := fields[4]
					if pct, err := strconv.Atoi(strings.TrimSuffix(usePct, "%")); err == nil {
						if pct >= 90 {
							recommendations = append(recommendations, "✗ inode 严重不足！大量小文件可能耗尽 inode")
						} else if pct >= 80 {
							recommendations = append(recommendations, "! inode 使用率较高")
						} else {
							recommendations = append(recommendations, "✓ inode 使用正常")
						}
					}
				}
			}
		}
	}

	// Check for common cleanup targets
	commonTargets := []string{
		"/var/log",
		"/tmp",
		"/var/cache",
	}

	for _, target := range commonTargets {
		if info, err := os.Stat(target); err == nil && info.IsDir() {
			size, _ := dirSize(ctx, target, 0, false)
			if size > 1024*1024*1024 { // > 1GB
				recommendations = append(recommendations, fmt.Sprintf("! %s 占用 %s，可考虑清理", target, humanSize(size)))
			}
		}
	}

	if len(recommendations) == 0 {
		fmt.Fprintln(w, "  ✓ 一切正常")
	} else {
		for _, rec := range recommendations {
			fmt.Fprintf(w, "  %s\n", rec)
		}
	}
}
