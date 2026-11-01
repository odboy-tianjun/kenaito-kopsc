package executor

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"text/tabwriter"
)

// runDockerRM implements `docker rm` over kopsc-resolved targets: containers
// are named by name or ID (or a unique ID prefix), and are resolved before
// docker is invoked so a typo fails with kopsc's own message instead of
// docker's.
//
// The `${targets}` placeholder in the command's BaseArgs is replaced with the
// resolved names; everything the caller typed after the targets is forwarded.
func runDockerRM(ctx context.Context, req BuiltinRequest) error {
	targets, passthrough, err := splitDockerTargets(req.Args, rmValueOption)
	if err != nil {
		return err
	}
	if len(targets) == 0 {
		return errors.New("缺少容器；用法: kopsc docker rm <容器>...")
	}

	names := make([]string, 0, len(targets))
	for _, target := range targets {
		ref, err := resolveContainerRef(ctx, target)
		if err != nil {
			return err
		}
		if ref.Running() {
			return &ContainerRunningError{Name: ref.Names}
		}
		names = append(names, ref.Names)
	}

	args := append([]string{"rm"}, names...)
	args = append(args, passthrough...)
	return dockerRun(ctx, args, req)
}

// runDockerRMI implements `docker rmi` over kopsc-resolved targets: images are
// named by name, ID, sha256:-prefixed ID or unique ID prefix. An image that is
// only known by ID without a tag (a dangling image) is still removed — by its
// ID, since it has no name.
func runDockerRMI(ctx context.Context, req BuiltinRequest) error {
	targets, passthrough, err := splitDockerTargets(req.Args, rmiValueOption)
	if err != nil {
		return err
	}
	if len(targets) == 0 {
		return errors.New("缺少镜像；用法: kopsc docker rmi <镜像名|镜像ID>...")
	}

	refs, err := listImages(ctx)
	if err != nil {
		return err
	}
	names := make([]string, 0, len(targets))
	for _, target := range targets {
		ref, err := lookupImageRef(refs, target)
		if err != nil {
			return err
		}
		if ref.Name != "" {
			names = append(names, ref.Name)
		} else {
			names = append(names, ref.ID)
		}
	}

	args := append([]string{"rmi"}, names...)
	args = append(args, passthrough...)
	return dockerRun(ctx, args, req)
}

// ContainerRunningError reports a container that must be stopped before it
// can be removed. docker rm refuses running containers anyway; reporting it
// here gives the user the exact next command.
type ContainerRunningError struct {
	Name string
}

func (e *ContainerRunningError) Error() string {
	return "容器 " + e.Name + " 正在运行，docker rm 不会强制删除；请先执行 kopsc docker stop " + e.Name
}

// rmValueOption and rmiValueOption list options that take a separate value, so
// the value is not mistaken for a target.
var (
	rmValueOption  = map[string]bool{}
	rmiValueOption = map[string]bool{"platform": true}
)

// splitDockerTargets separates positional targets from options. Options are
// collected in passthrough; an option that takes a value keeps its value with
// it. A `--` ends option parsing, so a target may not start with "-".
func splitDockerTargets(args []string, valueOptions map[string]bool) (targets, passthrough []string, err error) {
	if err := rejectForce(args, "不再接受 -f；需要强制删除请改用 --yes（kopsc 会在执行前列出将删除的对象）"); err != nil {
		return nil, nil, err
	}

	afterDashDash := false
	for i := 0; i < len(args); i++ {
		arg := args[i]
		switch {
		case afterDashDash:
			targets = append(targets, arg)
		case arg == "--":
			afterDashDash = true
		case !strings.HasPrefix(arg, "-") || arg == "-":
			targets = append(targets, arg)
		default:
			passthrough = append(passthrough, arg)
			if !strings.Contains(arg, "=") && valueOptions[strings.TrimLeft(arg, "-")] {
				if i+1 < len(args) {
					i++
					passthrough = append(passthrough, args[i])
				}
			}
		}
	}
	return targets, passthrough, nil
}

// rejectForce refuses docker's -f/--force up to the first `--`: kopsc shows
// what will be deleted and takes --yes as the confirmation, so a force flag
// would only duplicate (or, with -f, silently bypass) that contract.
func rejectForce(args []string, message string) error {
	for _, arg := range args {
		if arg == "--" {
			return nil
		}
		// Bundled short flags such as -fv still carry the force flag.
		if arg == "--force" || arg == "--force=true" || arg == "-f" ||
			(strings.HasPrefix(arg, "-") && !strings.HasPrefix(arg, "--") && strings.Contains(arg, "f")) {
			return errors.New(message)
		}
	}
	return nil
}

// runDockerPreviewRM lists the containers `docker rm` would delete: ID, name,
// image and status, one per line. It never touches the containers, and it
// reports an unknown target before the user is asked for --yes.
func runDockerPreviewRM(ctx context.Context, req BuiltinRequest) error {
	targets, _, err := splitDockerTargets(req.Args, rmValueOption)
	if err != nil {
		return err
	}
	if len(targets) == 0 {
		return errors.New("缺少容器；用法: kopsc docker rm <容器>...")
	}

	refs, err := listContainers(ctx)
	if err != nil {
		return err
	}
	tw := tabwriter.NewWriter(req.Stdout, 0, 0, 2, ' ', 0)
	fmt.Fprintln(tw, "ID\t名称\t镜像\t状态")
	for _, target := range targets {
		ref, err := matchContainerRef(refs, target)
		if err != nil {
			return err
		}
		fmt.Fprintf(tw, "%s\t%s\t%s\t%s\n", shortID(ref.ID), ref.Names, ref.Image, ref.Status)
		if ref.Running() {
			fmt.Fprintf(tw, "\t\t\t(运行中：需先 kopsc docker stop %s)\n", ref.Names)
		}
	}
	return tw.Flush()
}

// runDockerPreviewRMI lists the images `docker rmi` would delete: name, ID,
// creation time and size.
func runDockerPreviewRMI(ctx context.Context, req BuiltinRequest) error {
	targets, _, err := splitDockerTargets(req.Args, rmiValueOption)
	if err != nil {
		return err
	}
	if len(targets) == 0 {
		return errors.New("缺少镜像；用法: kopsc docker rmi <镜像名|镜像ID>...")
	}

	refs, err := listImages(ctx)
	if err != nil {
		return err
	}
	tw := tabwriter.NewWriter(req.Stdout, 0, 0, 2, ' ', 0)
	fmt.Fprintln(tw, "镜像\tID\t创建\t大小")
	for _, target := range targets {
		ref, err := lookupImageRef(refs, target)
		if err != nil {
			return err
		}
		name := ref.Name
		if name == "" {
			name = "<none>（悬空镜像）"
		}
		fmt.Fprintf(tw, "%s\t%s\t%s\t%s\n", name, shortID(ref.ID), formatCreated(ref.CreatedAt), ref.Size)
	}
	return tw.Flush()
}

// shortID trims docker's 12-character ID for display, keeping shorter IDs.
func shortID(id string) string {
	id = strings.TrimPrefix(id, "sha256:")
	if len(id) > 12 {
		return id[:12]
	}
	return id
}

// formatCreated renders docker's creation timestamp compactly.
func formatCreated(created string) string {
	created = strings.TrimSpace(created)
	if i := strings.IndexByte(created, '.'); i > 0 && strings.Contains(created[:i], "T") == false {
		return created
	}
	// Docker prints "2024-10-01 12:34:56 +0800 CST"; drop the timezone suffix.
	if i := strings.IndexByte(created, ' '); i > 0 {
		rest := created[i+1:]
		if j := strings.IndexByte(rest, ' '); j > 0 && (strings.Contains(rest, "+") || strings.Contains(rest, "-")) {
			return created[:i+1+j]
		}
	}
	return created
}
