package executor

import (
	"context"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	"gitea.odboy.cn/odboy/kenaito-kopsc/internal/diagnose"
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

	ref, err := resolveImageRef(ctx, diagnose.ExecRunner{}, image)
	if err != nil {
		return err
	}
	outPath := archiveName(ref)

	// Docker derives the archive's internal tags from the image itself, so the
	// caller's argument is passed through as given.
	args := append([]string{"save", "-o", outPath, image}, passthrough...)
	if err := runRuntimeCommand(ctx, args, req); err != nil {
		return err
	}

	fmt.Fprintf(req.Stdout, "已保存: %s\n", ref)
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
		return runRuntimeCommand(ctx, []string{"load"}, req)
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
	return runRuntimeCommand(ctx, args, req)
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

// imageRef is the repository and tag (or digest) of one local image.
type imageRef struct {
	Name string // repository[:tag][@digest], e.g. "alpine:3.20"
	ID   string // image ID, e.g. "sha256:abc..."
}

// customFormat lets a single `images --format` call list IDs together with
// names (docker images -q prints IDs only).
const customFormat = `{{.ID}}{{"\t"}}{{if .Repository}}{{.Repository}}{{"\t"}}{{.Tag}}{{"\t"}}{{.Digest}}{{end}}`

// resolveImageRef maps the user's argument to the image's repository[:tag].
//
// A name is used as given once it is confirmed locally. An image ID — with or
// without the sha256: prefix, or a unique prefix — resolves to the name it is
// tagged with; an untagged image is an error, because the archive would have no
// name to take its file name from. Names always win over IDs, so a short name
// such as "alpine" is never mistaken for an ID prefix.
func resolveImageRef(ctx context.Context, runner diagnose.Runner, image string) (string, error) {
	refs, err := listImages(ctx, runner)
	if err != nil {
		return "", err
	}

	for _, ref := range refs {
		if ref.Name == image {
			return image, nil
		}
	}

	if !looksLikeImageID(image) {
		return "", fmt.Errorf("本地没有镜像 %q，可用 kopsc docker images 查看", image)
	}

	// Same image ID on several rows is normal (one row per repository:tag).
	ids := map[string][]string{}
	var order []string
	for _, ref := range refs {
		id := normalizeImageID(ref.ID)
		if !strings.HasPrefix(id, normalizeImageID(image)) {
			continue
		}
		if _, seen := ids[id]; !seen {
			order = append(order, id)
		}
		ids[id] = append(ids[id], ref.Name)
	}

	switch len(order) {
	case 0:
		return "", fmt.Errorf("本地没有镜像 ID %q，可用 kopsc docker images 查看", image)
	case 1:
	default:
		return "", fmt.Errorf("镜像 ID %q 匹配多个本地镜像（%s），请改用镜像名称",
			image, strings.Join(order, ", "))
	}

	names := ids[order[0]]
	if len(names) > 1 {
		return "", fmt.Errorf("镜像 ID %q 对应多个名称（%s），请直接指定要保存的名称",
			image, strings.Join(names, ", "))
	}
	if len(names) == 0 || names[0] == "" {
		return "", fmt.Errorf("镜像 ID %q 存在但没有名称（未打标签），无法据此命名归档文件；请先 docker tag 再保存", image)
	}
	return names[0], nil
}

// listImages asks docker for every local image. docker images prints a header
// line, which is dropped explicitly.
func listImages(ctx context.Context, runner diagnose.Runner) ([]imageRef, error) {
	result, err := runner.Run(ctx, "docker", "images", "--format", customFormat)
	if err != nil {
		return nil, runtimeError("images", result, err)
	}

	var refs []imageRef
	for _, line := range strings.Split(string(result.Stdout), "\n") {
		line = strings.TrimSpace(line)
		if line == "" || strings.HasPrefix(line, "REPOSITORY") {
			continue
		}
		fields := strings.Split(line, "\t")
		ref := imageRef{ID: strings.TrimSpace(fields[0])}
		if len(fields) >= 3 {
			repo, tag, digest := strings.TrimSpace(fields[1]), strings.TrimSpace(fields[2]), ""
			if len(fields) >= 4 {
				digest = strings.TrimSpace(fields[3])
			}
			ref.Name = formatImageName(repo, tag, digest)
		}
		refs = append(refs, ref)
	}
	return refs, nil
}

// formatImageName renders repository[:tag][@digest]. "<none>" is how both
// runtimes group untagged images.
func formatImageName(repo, tag, digest string) string {
	if repo == "<none>" {
		return ""
	}
	name := repo
	if tag != "" && tag != "<none>" {
		name += ":" + tag
	}
	if digest != "" && digest != "<none>" {
		name += "@" + digest
	}
	return name
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

// looksLikeImageID reports whether s is an ID (optionally sha256:-prefixed)
// instead of a name.
func looksLikeImageID(s string) bool {
	s = normalizeImageID(s)
	if len(s) < 4 || len(s) > 64 {
		return false
	}
	for _, r := range s {
		if !(r >= '0' && r <= '9' || r >= 'a' && r <= 'f') {
			return false
		}
	}
	return true
}

func normalizeImageID(s string) string {
	return strings.ToLower(strings.TrimPrefix(strings.TrimPrefix(s, "sha256:"), "sha256"))
}

// runRuntimeCommand runs one docker command, streaming its output.
func runRuntimeCommand(ctx context.Context, args []string, req BuiltinRequest) error {
	cmd := exec.CommandContext(ctx, "docker", args...)
	cmd.Stdout = req.Stdout
	cmd.Stderr = req.Stderr
	if err := cmd.Run(); err != nil {
		var exitErr *exec.ExitError
		if errors.As(err, &exitErr) {
			return &ExitError{Code: exitErr.ExitCode(), Err: err}
		}
		return fmt.Errorf("执行 docker 失败: %w", err)
	}
	return nil
}

func runtimeError(subcommand string, result diagnose.CommandResult, err error) error {
	detail := strings.TrimSpace(string(result.Stderr))
	if detail == "" {
		detail = err.Error()
	}
	return fmt.Errorf("docker %s 失败: %s", subcommand, detail)
}
