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
)

// runArchiveCreate implements `kopsc archive create <path>`: create an archive.
func runArchiveCreate(ctx context.Context, req BuiltinRequest) error {
	opts, err := parseArchiveCreateArgs(req.Args)
	if err != nil {
		return err
	}

	src := opts.source
	if _, err := os.Stat(src); err != nil {
		return fmt.Errorf("路径 %q 不存在: %w", src, err)
	}

	// Determine output filename
	output := opts.output
	if output == "" {
		base := filepath.Base(src)
		switch opts.format {
		case "tar.gz", "tgz":
			output = base + ".tar.gz"
		case "tar":
			output = base + ".tar"
		case "zip":
			output = base + ".zip"
		}
	}

	fmt.Fprintf(req.Stdout, "创建压缩文件: %s -> %s\n", src, output)

	switch opts.format {
	case "tar.gz", "tgz":
		return createTarGz(src, output, req)
	case "tar":
		return createTar(src, output, req)
	case "zip":
		return createZip(src, output, req)
	default:
		return fmt.Errorf("不支持的格式: %q（支持 tar.gz / tgz / tar / zip）", opts.format)
	}
}

type archiveCreateOptions struct {
	source string
	output string
	format string
}

func parseArchiveCreateArgs(args []string) (archiveCreateOptions, error) {
	opts := archiveCreateOptions{
		format: "tar.gz", // default format
	}
	var positional []string

	for i := 0; i < len(args); i++ {
		arg := args[i]
		switch {
		case arg == "--":
			positional = append(positional, args[i+1:]...)
			i = len(args)
		case arg == "--output" || arg == "-o":
			if i+1 >= len(args) {
				return opts, fmt.Errorf("%s 缺少输出文件名", arg)
			}
			i++
			opts.output = args[i]
		case strings.HasPrefix(arg, "--output="):
			opts.output = strings.TrimPrefix(arg, "--output=")
		case arg == "--format" || arg == "-f":
			if i+1 >= len(args) {
				return opts, fmt.Errorf("%s 缺少格式参数", arg)
			}
			i++
			opts.format = strings.ToLower(args[i])
		case strings.HasPrefix(arg, "--format="):
			opts.format = strings.ToLower(strings.TrimPrefix(arg, "--format="))
		case strings.HasPrefix(arg, "-") && arg != "-":
			return opts, fmt.Errorf("未知选项 %q", arg)
		default:
			positional = append(positional, arg)
		}
	}

	if len(positional) == 0 {
		return opts, fmt.Errorf("缺少源路径；用法: kopsc archive create <路径>")
	}
	opts.source = positional[0]

	// Normalize format aliases
	switch opts.format {
	case "tgz":
		opts.format = "tar.gz"
	}

	return opts, nil
}

func createTarGz(src, output string, req BuiltinRequest) error {
	outFile, err := os.Create(output)
	if err != nil {
		return fmt.Errorf("创建输出文件失败: %w", err)
	}
	defer outFile.Close()

	gz := gzip.NewWriter(outFile)
	defer gz.Close()

	tw := tar.NewWriter(gz)
	defer tw.Close()

	return writeTarArchive(src, tw, req)
}

func createTar(src, output string, req BuiltinRequest) error {
	outFile, err := os.Create(output)
	if err != nil {
		return fmt.Errorf("创建输出文件失败: %w", err)
	}
	defer outFile.Close()

	tw := tar.NewWriter(outFile)
	defer tw.Close()

	return writeTarArchive(src, tw, req)
}

func writeTarArchive(src string, tw *tar.Writer, req BuiltinRequest) error {
	baseDir := filepath.Dir(src)
	baseName := filepath.Base(src)

	count := 0
	err := filepath.WalkDir(src, func(path string, d os.DirEntry, err error) error {
		if err != nil {
			return err
		}

		info, err := d.Info()
		if err != nil {
			return err
		}

		// Get relative path
		relPath, err := filepath.Rel(baseDir, path)
		if err != nil {
			return err
		}

		// For single file, just use the filename
		if !info.IsDir() && baseName == filepath.Base(path) {
			relPath = filepath.Base(path)
		}

		header, err := tar.FileInfoHeader(info, "")
		if err != nil {
			return err
		}
		header.Name = relPath

		if err := tw.WriteHeader(header); err != nil {
			return err
		}

		if !info.IsDir() {
			f, err := os.Open(path)
			if err != nil {
				return err
			}
			defer f.Close()
			if _, err := io.Copy(tw, f); err != nil {
				return err
			}
			count++
		}
		return nil
	})

	if err != nil {
		return err
	}

	fmt.Fprintf(req.Stdout, "已压缩 %d 个文件\n", count)
	return nil
}

func createZip(src, output string, req BuiltinRequest) error {
	outFile, err := os.Create(output)
	if err != nil {
		return fmt.Errorf("创建输出文件失败: %w", err)
	}
	defer outFile.Close()

	zw := zip.NewWriter(outFile)
	defer zw.Close()

	baseDir := filepath.Dir(src)
	count := 0

	err = filepath.WalkDir(src, func(path string, d os.DirEntry, err error) error {
		if err != nil {
			return err
		}

		info, err := d.Info()
		if err != nil {
			return err
		}

		// Get relative path
		relPath, err := filepath.Rel(baseDir, path)
		if err != nil {
			return err
		}

		header, err := zip.FileInfoHeader(info)
		if err != nil {
			return err
		}
		header.Name = relPath
		header.Method = zip.Deflate

		if info.IsDir() {
			header.Name += "/"
			_, err = zw.CreateHeader(header)
			return err
		}

		writer, err := zw.CreateHeader(header)
		if err != nil {
			return err
		}

		f, err := os.Open(path)
		if err != nil {
			return err
		}
		defer f.Close()

		if _, err := io.Copy(writer, f); err != nil {
			return err
		}
		count++
		return nil
	})

	if err != nil {
		return err
	}

	fmt.Fprintf(req.Stdout, "已压缩 %d 个文件\n", count)
	return nil
}
