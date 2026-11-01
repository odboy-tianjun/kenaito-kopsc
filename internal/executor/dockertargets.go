package executor

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"os/exec"
	"strings"

	"gitea.odboy.cn/odboy/kenaito-kopsc/internal/diagnose"
)

// This file holds the shared plumbing for the docker commands kopsc implements
// itself: running docker, and resolving the image/container references a user
// types (a name, an ID, or a unique ID prefix) to the concrete object.

// dockerRun streams one docker command's output to the caller's streams.
func dockerRun(ctx context.Context, args []string, req BuiltinRequest) error {
	cmd := exec.CommandContext(ctx, "docker", args...)
	cmd.Stdin = nil
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

// dockerCapture runs one docker command and collects its output, so kopsc can
// parse it before deciding what to do.
func dockerCapture(ctx context.Context, args ...string) (diagnose.CommandResult, error) {
	return diagnose.ExecRunner{}.Run(ctx, "docker", args...)
}

// dockerOutput runs one docker command and returns its stdout, turning a
// failure into an error that carries docker's own message.
func dockerOutput(ctx context.Context, args ...string) (string, error) {
	result, err := dockerCapture(ctx, args...)
	if err != nil {
		return "", dockerError(args[0], result, err)
	}
	return string(result.Stdout), nil
}

// dockerError renders a failed docker invocation with its stderr.
func dockerError(subcommand string, result diagnose.CommandResult, err error) error {
	detail := strings.TrimSpace(string(result.Stderr))
	if detail == "" {
		detail = err.Error()
	}
	return fmt.Errorf("docker %s 失败: %s", subcommand, detail)
}

// splitLines splits docker output into non-empty, trimmed lines.
func splitLines(out string) []string {
	var lines []string
	for _, line := range strings.Split(out, "\n") {
		if line = strings.TrimSpace(line); line != "" {
			lines = append(lines, line)
		}
	}
	return lines
}

// imageRef is one local image: its repository[:tag][@digest] and its ID.
type imageRef struct {
	Name       string // repository[:tag][@digest], e.g. "alpine:3.20"; empty when untagged
	ID         string // image ID, e.g. "sha256:abc..."
	CreatedAt  string // human-readable creation time, as docker lists it
	Size       string // human-readable size, as docker lists it
	Repository string
	Tag        string
}

// imageFormat lets a single `images` call list IDs together with names
// (docker images -q prints IDs only).
const imageFormat = `{{.ID}}{{"\t"}}{{if .Repository}}{{.Repository}}{{"\t"}}{{.Tag}}{{"\t"}}{{.Digest}}{{"\t"}}{{.CreatedAt}}{{"\t"}}{{.Size}}{{end}}`

// listImages asks docker for every local image. docker images prints a header
// line, which is dropped explicitly.
func listImages(ctx context.Context) ([]imageRef, error) {
	out, err := dockerOutput(ctx, "images", "--format", imageFormat)
	if err != nil {
		return nil, err
	}

	var refs []imageRef
	for _, line := range splitLines(out) {
		if strings.HasPrefix(line, "REPOSITORY") {
			continue
		}
		fields := strings.Split(line, "\t")
		ref := imageRef{ID: strings.TrimSpace(fields[0])}
		if len(fields) >= 3 {
			repo, tag := strings.TrimSpace(fields[1]), strings.TrimSpace(fields[2])
			digest := ""
			if len(fields) >= 4 {
				digest = strings.TrimSpace(fields[3])
			}
			if len(fields) >= 5 {
				ref.CreatedAt = strings.TrimSpace(fields[4])
			}
			if len(fields) >= 6 {
				ref.Size = strings.TrimSpace(fields[5])
			}
			ref.Repository = repo
			ref.Tag = tag
			ref.Name = formatImageName(repo, tag, digest)
		}
		refs = append(refs, ref)
	}
	return refs, nil
}

// formatImageName renders repository[:tag][@digest]. "<none>" is how docker
// groups untagged images.
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

// resolveImageRef maps a user-supplied image name or ID to the local image.
//
// A name is used as given once it is confirmed locally. An image ID — with or
// without the sha256: prefix, or a unique prefix — resolves to the image that
// carries it. Names always win over IDs, so a short name such as "alpine" is
// never mistaken for an ID prefix. The returned ref always has a non-empty
// Name: every kopsc command that takes an image needs one to act on.
func resolveImageRef(ctx context.Context, image string) (imageRef, error) {
	refs, err := listImages(ctx)
	if err != nil {
		return imageRef{}, err
	}
	return matchImageRef(refs, image)
}

// matchImageRef is resolveImageRef over an already-fetched image list.
func matchImageRef(refs []imageRef, image string) (imageRef, error) {
	ref, err := lookupImageRef(refs, image)
	if err != nil {
		return imageRef{}, err
	}
	if ref.Name == "" {
		return imageRef{}, fmt.Errorf("镜像 %q 没有名称（未打标签），请先 docker tag %s <名称>:<标签>", image, ref.ID)
	}
	return ref, nil
}

// lookupImageRef is matchImageRef without the naming requirement: it may
// return a ref with an empty Name for an untagged image.
func lookupImageRef(refs []imageRef, image string) (imageRef, error) {
	for _, ref := range refs {
		if ref.Name == image {
			return ref, nil
		}
	}

	if !looksLikeImageID(image) {
		return imageRef{}, fmt.Errorf("本地没有镜像 %q，可用 kopsc docker images 查看", image)
	}

	// The same ID appears on one row per repository:tag, so group by ID.
	wanted := normalizeImageID(image)
	ids := map[string][]imageRef{}
	var order []string
	for _, ref := range refs {
		id := normalizeImageID(ref.ID)
		if !strings.HasPrefix(id, wanted) {
			continue
		}
		if _, seen := ids[id]; !seen {
			order = append(order, id)
		}
		ids[id] = append(ids[id], ref)
	}

	switch len(order) {
	case 0:
		return imageRef{}, fmt.Errorf("本地没有镜像 ID %q，可用 kopsc docker images 查看", image)
	case 1:
	default:
		return imageRef{}, fmt.Errorf("镜像 ID %q 匹配多个本地镜像（%s），请改用镜像名称",
			image, strings.Join(order, ", "))
	}

	rows := ids[order[0]]
	var names []string
	for _, row := range rows {
		if row.Name != "" {
			names = append(names, row.Name)
		}
	}
	resolved := rows[0]
	if len(names) > 0 {
		resolved.Name = names[0]
	}
	return resolved, nil
}

// containerRef is one container as docker lists it.
type containerRef struct {
	ID     string // container ID, e.g. "3f2a..."
	Names  string // comma-separated names without docker's leading "/"
	Image  string
	Status string // e.g. "Up 2 hours", "Exited (0) 3 days ago"
}

// Running reports whether the container is up, judged from its status line.
func (c containerRef) Running() bool { return strings.HasPrefix(c.Status, "Up") }

// containerFormat lists every container (including stopped ones) with the
// fields kopsc needs to identify it.
const containerFormat = `{{.ID}}{{"\t"}}{{.Names}}{{"\t"}}{{.Image}}{{"\t"}}{{.Status}}`

// listContainers asks docker for every container, running or not.
func listContainers(ctx context.Context) ([]containerRef, error) {
	out, err := dockerOutput(ctx, "ps", "-a", "--format", containerFormat)
	if err != nil {
		return nil, err
	}

	var refs []containerRef
	for _, line := range splitLines(out) {
		fields := strings.Split(line, "\t")
		if len(fields) < 4 {
			continue
		}
		refs = append(refs, containerRef{
			ID:     strings.TrimSpace(fields[0]),
			Names:  strings.TrimPrefix(strings.TrimSpace(fields[1]), "/"),
			Image:  strings.TrimSpace(fields[2]),
			Status: strings.TrimSpace(fields[3]),
		})
	}
	return refs, nil
}

// resolveContainerRef maps a user-supplied container name or ID to the local
// container, following docker's own precedence: an exact name, then an exact
// ID, then a unique ID prefix.
func resolveContainerRef(ctx context.Context, target string) (containerRef, error) {
	refs, err := listContainers(ctx)
	if err != nil {
		return containerRef{}, err
	}
	return matchContainerRef(refs, target)
}

// matchContainerRef is resolveContainerRef over an already-fetched list.
func matchContainerRef(refs []containerRef, target string) (containerRef, error) {
	for _, ref := range refs {
		if hasName(ref, target) {
			return ref, nil
		}
	}

	wanted := normalizeContainerID(target)
	if wanted != "" {
		for _, ref := range refs {
			if normalizeContainerID(ref.ID) == wanted {
				return ref, nil
			}
		}

		var byPrefix []containerRef
		for _, ref := range refs {
			if strings.HasPrefix(normalizeContainerID(ref.ID), wanted) {
				byPrefix = append(byPrefix, ref)
			}
		}
		switch len(byPrefix) {
		case 1:
			return byPrefix[0], nil
		case 0:
			// Fall through to the "not found" error below.
		default:
			return containerRef{}, fmt.Errorf("容器 ID 前缀 %q 匹配多个容器（%s），请指定更长的 ID 或容器名称",
				target, strings.Join(containerIDs(byPrefix), ", "))
		}
	}

	return containerRef{}, fmt.Errorf("本地没有容器 %q，可用 kopsc docker psall 查看", target)
}

// hasName reports whether ref carries exactly the given name (docker may list
// several aliases, comma-separated).
func hasName(ref containerRef, name string) bool {
	for _, candidate := range strings.Split(ref.Names, ",") {
		if strings.TrimSpace(candidate) == name {
			return true
		}
	}
	return false
}

func containerIDs(refs []containerRef) []string {
	ids := make([]string, len(refs))
	for i, ref := range refs {
		ids[i] = ref.ID
	}
	return ids
}

// looksLikeImageID reports whether s is an image ID (optionally
// sha256:-prefixed, lowercase hex) instead of a name.
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

// normalizeContainerID strips the separators from a container ID so prefixes
// from `docker ps` and from the user compare equal.
func normalizeContainerID(s string) string {
	return strings.ToLower(strings.TrimSpace(s))
}

// captureDocker is a small helper for builtins that need docker's output in a
// buffer while still reporting failures uniformly.
func captureDocker(ctx context.Context, args ...string) ([]byte, error) {
	result, err := dockerCapture(ctx, args...)
	if err != nil {
		return nil, dockerError(args[0], result, err)
	}
	return bytes.TrimSpace(result.Stdout), nil
}

// startDockerPipe starts a docker command whose stdout is read line by line.
// Stderr goes straight to the caller so errors such as "No such container"
// are still shown.
func startDockerPipe(ctx context.Context, args []string, stderr io.Writer) (*exec.Cmd, io.ReadCloser, error) {
	cmd := exec.CommandContext(ctx, "docker", args...)
	out, err := cmd.StdoutPipe()
	if err != nil {
		return nil, nil, fmt.Errorf("创建 docker 输出管道失败: %w", err)
	}
	cmd.Stderr = stderr
	if err := cmd.Start(); err != nil {
		return nil, nil, fmt.Errorf("启动 docker 失败: %w", err)
	}
	return cmd, out, nil
}

// waitDockerPipe waits for a command started by startDockerPipe and forwards
// its exit code, preferring a scan error over the exit status when the read
// side failed first.
func waitDockerPipe(cmd *exec.Cmd, scanErr error) error {
	err := cmd.Wait()
	if scanErr != nil {
		return fmt.Errorf("读取 docker 输出失败: %w", scanErr)
	}
	if err != nil {
		var exitErr *exec.ExitError
		if errors.As(err, &exitErr) {
			return &ExitError{Code: exitErr.ExitCode(), Err: err}
		}
		return fmt.Errorf("执行 docker 失败: %w", err)
	}
	return nil
}
