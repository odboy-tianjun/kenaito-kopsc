package executor

import (
	"context"
	"strings"
)

// runDockerStats implements `kopsc docker stats [容器...]`.
//
// Plain `docker stats` prints an endless live table, which is rarely what
// anyone wants from a one-shot helper, so kopsc always asks for a single
// snapshot (`--no-stream`). With no container named, every running container
// is reported; with names, those are resolved to the running containers and
// only they are shown. Options such as --format are forwarded.
func runDockerStats(ctx context.Context, req BuiltinRequest) error {
	opts, targets, passthrough, err := parseDockerStats(req.Args)
	if err != nil {
		return err
	}

	if len(targets) > 0 {
		refs, err := listContainers(ctx)
		if err != nil {
			return err
		}
		resolved := make([]string, 0, len(targets))
		for _, target := range targets {
			ref, err := matchContainerRef(refs, target)
			if err != nil {
				return err
			}
			if !ref.Running() {
				return &NotRunningError{Name: ref.Names}
			}
			resolved = append(resolved, ref.Names)
		}
		targets = resolved
	}

	// docker stats takes the containers after its options; keeping the
	// forwarded options last matches how a caller would type them.
	args := append([]string{"stats"}, opts...)
	args = append(args, targets...)
	args = append(args, passthrough...)
	return dockerRun(ctx, args, req)
}

// NotRunningError reports a container docker stats cannot sample.
type NotRunningError struct {
	Name string
}

func (e *NotRunningError) Error() string {
	return "容器 " + e.Name + " 未在运行，docker stats 只能统计运行中的容器；先启动请用 kopsc docker start " + e.Name
}

// parseDockerStats separates options from container names. --no-stream is
// added by kopsc, so it is kept as an option and never duplicated.
func parseDockerStats(args []string) (opts, targets, passthrough []string, err error) {
	opts = []string{"--no-stream"}
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
		case arg == "--no-stream":
			// Already requested by kopsc.
		default:
			passthrough = append(passthrough, arg)
			if !strings.Contains(arg, "=") && statsValueOption[strings.TrimLeft(arg, "-")] {
				if i+1 >= len(args) {
					return nil, nil, nil, &MissingOptionValueError{Option: arg}
				}
				i++
				passthrough = append(passthrough, args[i])
			}
		}
	}
	return opts, targets, passthrough, nil
}

// statsValueOption lists docker stats options that take a separate value.
var statsValueOption = map[string]bool{
	"format": true,
}

// MissingOptionValueError reports an option given without its value.
type MissingOptionValueError struct {
	Option string
}

func (e *MissingOptionValueError) Error() string {
	return e.Option + " 缺少参数"
}
