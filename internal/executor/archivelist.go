package executor

import (
	"archive/tar"
	"archive/zip"
	"compress/gzip"
	"context"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"time"
)

// runArchiveList implements `kopsc archive list <file>`: list archive contents.
func runArchiveList(ctx context.Context, req BuiltinRequest) error {
	opts, err := parseArchiveListArgs(req.Args)
	if err != nil {
		return err
	}

	src := opts.file
	if _, err := os.Stat(src); err != nil {
		return fmt.Errorf("文件 %q 不存在: %w", src, err)
	}

	lower := strings.ToLower(src)
	switch {
	case strings.HasSuffix(lower, ".tar.gz") || strings.HasSuffix(lower, ".tgz"):
		return listTarGz(src, req)
	case strings.HasSuffix(lower, ".tar"):
		return listTar(src, req)
	case strings.HasSuffix(lower, ".zip") || strings.HasSuffix(lower, ".jar") || strings.HasSuffix(lower, ".war"):
		return listZip(src, req)
	default:
		return fmt.Errorf("无法识别的压缩格式: %q（支持 .tar.gz .tgz .tar .zip）", src)
	}
}

type archiveListOptions struct {
	file string
}

func parseArchiveListArgs(args []string) (archiveListOptions, error) {
	var opts archiveListOptions
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

	if len(positional) == 0 {
		return opts, fmt.Errorf("缺少压缩文件路径；用法: kopsc archive list <文件>")
	}
	opts.file = positional[0]
	return opts, nil
}

func listTarGz(src string, req BuiltinRequest) error {
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

	return listTarContents(tar.NewReader(gz), req)
}

func listTar(src string, req BuiltinRequest) error {
	f, err := os.Open(src)
	if err != nil {
		return err
	}
	defer f.Close()

	return listTarContents(tar.NewReader(f), req)
}

func listTarContents(tr *tar.Reader, req BuiltinRequest) error {
	fmt.Fprintln(req.Stdout, "权限        大小       修改时间               名称")
	fmt.Fprintln(req.Stdout, "------------------------------------------------------------")

	count := 0
	var totalSize int64

	for {
		header, err := tr.Next()
		if err == io.EOF {
			break
		}
		if err != nil {
			return fmt.Errorf("读取 tar 失败: %w", err)
		}

		mode := formatTarMode(header)
		size := header.Size
		mtime := header.ModTime.Format("2006-01-02 15:04:05")
		name := header.Name

		fmt.Fprintf(req.Stdout, "%-10s %-9d %s  %s\n", mode, size, mtime, name)
		count++
		totalSize += size
	}

	fmt.Fprintln(req.Stdout, "------------------------------------------------------------")
	fmt.Fprintf(req.Stdout, "共 %d 项，总大小 %s\n", count, humanSize(totalSize))
	return nil
}

func formatTarMode(header *tar.Header) string {
	var mode rune
	switch header.Typeflag {
	case tar.TypeDir:
		mode = 'd'
	case tar.TypeSymlink:
		mode = 'l'
	case tar.TypeReg, tar.TypeRegA:
		mode = '-'
	default:
		mode = '?'
	}

	perm := fmt.Sprintf("%04o", header.Mode&0o7777)
	return string(mode) + perm
}

func listZip(src string, req BuiltinRequest) error {
	r, err := zip.OpenReader(src)
	if err != nil {
		return fmt.Errorf("打开 zip 失败: %w", err)
	}
	defer r.Close()

	fmt.Fprintln(req.Stdout, "权限        大小       修改时间               名称")
	fmt.Fprintln(req.Stdout, "------------------------------------------------------------")

	count := 0
	var totalSize int64

	for _, f := range r.File {
		mode := formatZipMode(f)
		size := int64(f.UncompressedSize64)
		mtime := f.Modified.Format("2006-01-02 15:04:05")
		name := f.Name

		fmt.Fprintf(req.Stdout, "%-10s %-9d %s  %s\n", mode, size, mtime, name)
		count++
		totalSize += size
	}

	fmt.Fprintln(req.Stdout, "------------------------------------------------------------")
	fmt.Fprintf(req.Stdout, "共 %d 项，总大小 %s\n", count, humanSize(totalSize))
	return nil
}

func formatZipMode(f *zip.File) string {
	if f.FileInfo().IsDir() {
		return "d0000"
	}
	return "-" + fmt.Sprintf("%04o", f.Mode()&0o7777)
}

// humanSizeTime is a helper for formatting (not used, kept for reference)
func humanSizeTime(t time.Time) string {
	return t.Format("2006-01-02 15:04:05")
}

// archiveListName extracts just the filename for display (helper)
func archiveListName(path string) string {
	return filepath.Base(path)
}
