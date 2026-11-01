package executor

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

// runDockerSave implements `docker save <镜像> > <镜像名>.tar`.
//
// The archive is named after the image's repository and tag rather than its
// ID: `alpine:3.20` becomes alpine_3.20.tar, an untagged image keeps its bare
// name, and an image ID is first resolved to the name it is tagged with.
// Extra arguments such as --platform are forwarded to docker.
func runDockerSave(ctx context.Context, req BuiltinRequest) error {
	rest := req.Args
	var images []string
	var passthrough []string
	for i := 0; i < len(rest); i++ {
		arg := rest[i]
		if !strings.HasPrefix(arg, "-") || arg == "-" {
			images = append(images, arg)
			continue
		}
		passthrough = append(passthrough, arg)
		// Keep an option's value with the option: `--platform linux/arm64`
		// must not be read as a second image.
		if !strings.Contains(arg, "=") && saveValueOption[arg] {
			if i+1 < len(rest) {
				i++
				passthrough = append(passthrough, rest[i])
			}
		}
	}
	image, err := singleImage(images, "kopsc docker save <镜像名|镜像ID>")
	if err != nil {
		return err
	}

	ref, err := resolveImageRef(ctx, image)
	if err != nil {
		return err
	}
	outPath := archiveName(ref.Name)

	// Docker derives the archive's internal tags from the image itself, so the
	// caller's argument is passed through as given.
	args := append([]string{"save", "-o", outPath, image}, passthrough...)
	if err := dockerRun(ctx, args, req); err != nil {
		return err
	}

	fmt.Fprintf(req.Stdout, "已保存: %s\n", ref.Name)
	fmt.Fprintf(req.Stdout, "文件:   %s\n", outPath)
	return nil
}

// runDockerLoad implements `docker load < archive.tar`.
//
// The archive is positional, mirroring `docker load x.tar`. A lone `-i archive`
// (docker's own spelling) or an argument after `--` is accepted too; a
// `docker load -` stream still goes to stdin.
func runDockerLoad(ctx context.Context, req BuiltinRequest) error {
	rest := req.Args

	var archives []string
	var passthrough []string
	afterDashDash, stream := false, false
	for i := 0; i < len(rest); i++ {
		arg := rest[i]
		switch {
		case afterDashDash:
			archives = append(archives, arg)
		case arg == "--":
			afterDashDash = true
		case arg == "-i" || arg == "--input":
			if i+1 >= len(rest) {
				return errors.New("-i 缺少归档文件，例如 kopsc docker load alpine_3.20.tar")
			}
			i++
			archives = append(archives, rest[i])
		case strings.HasPrefix(arg, "-i="):
			archives = append(archives, strings.TrimPrefix(arg, "-i="))
		case strings.HasPrefix(arg, "--input="):
			archives = append(archives, strings.TrimPrefix(arg, "--input="))
		case arg == "-":
			stream = true
		case strings.HasPrefix(arg, "-"):
			passthrough = append(passthrough, arg)
		default:
			archives = append(archives, arg)
		}
	}
	if len(archives) == 0 {
		if !stream {
			return errors.New("缺少归档文件；用法: kopsc docker load <镜像归档.tar>")
		}
		return dockerRun(ctx, []string{"load"}, req)
	}

	var paths []string
	for _, archive := range archives {
		path, err := existingArchive(archive)
		if err != nil {
			return err
		}
		paths = append(paths, path)
	}

	args := append([]string{"load"}, passthrough...)
	for _, path := range paths {
		args = append(args, "-i", path)
	}
	return dockerRun(ctx, args, req)
}

// saveValueOption lists docker `save` options that take a separate
// value, so the value is not mistaken for an image name.
var saveValueOption = map[string]bool{
	"--platform": true,
}

func singleImage(images []string, usage string) (string, error) {
	switch len(images) {
	case 0:
		return "", fmt.Errorf("缺少镜像；用法: %s", usage)
	case 1:
		return images[0], nil
	default:
		return "", fmt.Errorf("一次只能保存一个镜像，收到 %d 个；用法: %s", len(images), usage)
	}
}

// existingArchive checks that the archive can be read and returns an absolute
// path, so the runtime resolves it independently of its working directory.
func existingArchive(archive string) (string, error) {
	info, err := os.Stat(archive)
	if err != nil {
		return "", fmt.Errorf("无法读取归档文件 %q: %w", archive, err)
	}
	if info.IsDir() {
		return "", fmt.Errorf("%q 是目录，不是镜像归档", archive)
	}
	if abs, err := filepath.Abs(archive); err == nil {
		return abs, nil
	}
	return archive, nil
}

// archiveName turns an image reference into a file name: every character that
// cannot appear in a portable file name, including the ":" between repository
// and tag, becomes "_". "alpine:3.20" -> "alpine_3.20.tar" and
// "registry.example.com:5000/team/app:1.0" -> "registry.example.com_5000_team_app_1.0.tar".
func archiveName(image string) string {
	var b strings.Builder
	for _, r := range image {
		switch r {
		case ':', '/', '\\', '@', '?', '*', '"', '<', '>', '|', '\x00':
			b.WriteByte('_')
		default:
			b.WriteRune(r)
		}
	}
	return b.String() + ".tar"
}
