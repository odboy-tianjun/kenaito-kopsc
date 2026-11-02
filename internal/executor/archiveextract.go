package executor

import (
	"archive/tar"
	"archive/zip"
	"compress/gzip"
	"context"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
)

// runArchiveExtract implements `kopsc archive extract <file>`: auto-detect
// archive format and extract to current directory (or --dir).
func runArchiveExtract(ctx context.Context, req BuiltinRequest) error {
	opts, err := parseArchiveExtractArgs(req.Args)
	if err != nil {
		return err
	}

	src := opts.file
	if _, err := os.Stat(src); err != nil {
		return fmt.Errorf("文件 %q 不存在: %w", src, err)
	}

	dest := opts.dest
	if dest == "" {
		dest = "."
	}
	if err := os.MkdirAll(dest, 0o755); err != nil {
		return fmt.Errorf("创建目标目录 %q 失败: %w", dest, err)
	}

	lower := strings.ToLower(src)
	switch {
	case strings.HasSuffix(lower, ".tar.gz") || strings.HasSuffix(lower, ".tgz"):
		return extractTarGz(src, dest, req)
	case strings.HasSuffix(lower, ".tar.bz2") || strings.HasSuffix(lower, ".tbz2"):
		return extractTarViaPipe(ctx, "bzip2", "-dc", src, dest, req)
	case strings.HasSuffix(lower, ".tar.xz") || strings.HasSuffix(lower, ".txz"):
		return extractTarViaPipe(ctx, "xz", "-dc", src, dest, req)
	case strings.HasSuffix(lower, ".tar"):
		return extractTarFile(src, dest, req)
	case strings.HasSuffix(lower, ".zip") || strings.HasSuffix(lower, ".jar") || strings.HasSuffix(lower, ".war"):
		return extractZip(src, dest, req)
	case strings.HasSuffix(lower, ".gz"):
		return extractGz(src, dest, req)
	default:
		return fmt.Errorf("无法识别的压缩格式: %q（支持 .tar.gz .tgz .tar.bz2 .tar.xz .tar .zip .gz）", src)
	}
}

type archiveExtractOptions struct {
	file string
	dest string
}

func parseArchiveExtractArgs(args []string) (archiveExtractOptions, error) {
	var opts archiveExtractOptions
	var positional []string

	for i := 0; i < len(args); i++ {
		arg := args[i]
		switch {
		case arg == "--":
			positional = append(positional, args[i+1:]...)
			i = len(args)
		case arg == "--dir" || arg == "-d" || arg == "-C":
			if i+1 >= len(args) {
				return opts, fmt.Errorf("%s 缺少目标目录", arg)
			}
			i++
			opts.dest = args[i]
		case strings.HasPrefix(arg, "--dir="):
			opts.dest = strings.TrimPrefix(arg, "--dir=")
		case strings.HasPrefix(arg, "-") && arg != "-":
			return opts, fmt.Errorf("未知选项 %q", arg)
		default:
			positional = append(positional, arg)
		}
	}
	if len(positional) == 0 {
		return opts, fmt.Errorf("缺少压缩文件路径；用法: kopsc archive extract <文件>")
	}
	opts.file = positional[0]
	return opts, nil
}

func extractTarGz(src, dest string, req BuiltinRequest) error {
	f, err := os.Open(src)
	if err != nil {
		return err
	}
	defer f.Close()
	gz, err := gzip.NewReader(f)
	if err != nil {
		return fmt.Errorf("打开 gzip 失败: %w", err)
	}
	defer gz.Close()
	fmt.Fprintf(req.Stdout, "解压 tar.gz: %s -> %s\n", src, dest)
	return writeTar(tar.NewReader(gz), dest, req)
}

func extractTarFile(src, dest string, req BuiltinRequest) error {
	f, err := os.Open(src)
	if err != nil {
		return err
	}
	defer f.Close()
	fmt.Fprintf(req.Stdout, "解压 tar: %s -> %s\n", src, dest)
	return writeTar(tar.NewReader(f), dest, req)
}

func extractTarViaPipe(ctx context.Context, decompressor, flag, src, dest string, req BuiltinRequest) error {
	if _, err := exec.LookPath(decompressor); err != nil {
		return fmt.Errorf("解压 %s 需要 %s，请先安装", filepath.Ext(src), decompressor)
	}
	fmt.Fprintf(req.Stdout, "解压 %s: %s -> %s\n", decompressor, src, dest)
	cmd := exec.CommandContext(ctx, decompressor, flag, src)
	stdoutPipe, err := cmd.StdoutPipe()
	if err != nil {
		return err
	}
	cmd.Stderr = req.Stderr
	if err := cmd.Start(); err != nil {
		return fmt.Errorf("启动 %s 失败: %w", decompressor, err)
	}
	err = writeTar(tar.NewReader(stdoutPipe), dest, req)
	if waitErr := cmd.Wait(); waitErr != nil && err == nil {
		err = waitErr
	}
	return err
}

func writeTar(tr *tar.Reader, dest string, req BuiltinRequest) error {
	count := 0
	for {
		header, err := tr.Next()
		if err == io.EOF {
			break
		}
		if err != nil {
			return fmt.Errorf("读取 tar 失败: %w", err)
		}
		target := filepath.Join(dest, header.Name)
		if !isWithinDir(target, dest) {
			return fmt.Errorf("tar 条目 %q 试图逃逸出目标目录", header.Name)
		}
		switch header.Typeflag {
		case tar.TypeDir:
			if err := os.MkdirAll(target, os.FileMode(header.Mode)); err != nil {
				return err
			}
		case tar.TypeReg:
			if dir := filepath.Dir(target); dir != "" {
				if err := os.MkdirAll(dir, 0o755); err != nil {
					return err
				}
			}
			f, err := os.OpenFile(target, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, os.FileMode(header.Mode))
			if err != nil {
				return err
			}
			if _, err := io.Copy(f, tr); err != nil {
				f.Close()
				return err
			}
			f.Close()
			count++
		case tar.TypeSymlink:
			if dir := filepath.Dir(target); dir != "" {
				if err := os.MkdirAll(dir, 0o755); err != nil {
					return err
				}
			}
			os.Remove(target)
			if err := os.Symlink(header.Linkname, target); err != nil {
				return err
			}
		}
	}
	fmt.Fprintf(req.Stdout, "已解压 %d 个文件\n", count)
	return nil
}

func extractZip(src, dest string, req BuiltinRequest) error {
	fmt.Fprintf(req.Stdout, "解压 zip: %s -> %s\n", src, dest)
	r, err := zip.OpenReader(src)
	if err != nil {
		return fmt.Errorf("打开 zip 失败: %w", err)
	}
	defer r.Close()

	count := 0
	for _, f := range r.File {
		target := filepath.Join(dest, f.Name)
		if !isWithinDir(target, dest) {
			return fmt.Errorf("zip 条目 %q 试图逃逸出目标目录", f.Name)
		}
		if f.FileInfo().IsDir() {
			if err := os.MkdirAll(target, 0o755); err != nil {
				return err
			}
			continue
		}
		if dir := filepath.Dir(target); dir != "" {
			if err := os.MkdirAll(dir, 0o755); err != nil {
				return err
			}
		}
		rc, err := f.Open()
		if err != nil {
			return err
		}
		out, err := os.OpenFile(target, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, f.Mode())
		if err != nil {
			rc.Close()
			return err
		}
		_, err = io.Copy(out, rc)
		rc.Close()
		out.Close()
		if err != nil {
			return err
		}
		count++
	}
	fmt.Fprintf(req.Stdout, "已解压 %d 个文件\n", count)
	return nil
}

func extractGz(src, dest string, req BuiltinRequest) error {
	fmt.Fprintf(req.Stdout, "解压 gz: %s -> %s\n", src, dest)
	f, err := os.Open(src)
	if err != nil {
		return err
	}
	defer f.Close()
	gz, err := gzip.NewReader(f)
	if err != nil {
		return fmt.Errorf("打开 gzip 失败: %w", err)
	}
	defer gz.Close()

	outName := strings.TrimSuffix(filepath.Base(src), ".gz")
	if outName == "" {
		outName = "output"
	}
	target := filepath.Join(dest, outName)
	out, err := os.Create(target)
	if err != nil {
		return err
	}
	defer out.Close()
	if _, err := io.Copy(out, gz); err != nil {
		return err
	}
	fmt.Fprintf(req.Stdout, "已解压: %s\n", target)
	return nil
}

// isWithinDir checks that target is inside dest (prevents zip-slip attacks).
func isWithinDir(target, dest string) bool {
	rel, err := filepath.Rel(dest, target)
	if err != nil {
		return false
	}
	return !strings.HasPrefix(rel, ".."+string(filepath.Separator)) && rel != ".."
}
