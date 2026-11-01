package executor

import (
	"context"
	"errors"
)

// runDockerStop and runDockerRestart implement `docker stop` / `docker
// restart` over kopsc-resolved containers: a name, ID or unique ID prefix is
// mapped to the container's name before docker runs, so a typo is reported by
// kopsc (pointing at `kopsc docker psall`) instead of by docker.
func runDockerStop(ctx context.Context, req BuiltinRequest) error {
	return runContainerLifecycle(ctx, req, "stop", "kopsc docker stop <容器>...")
}

func runDockerRestart(ctx context.Context, req BuiltinRequest) error {
	return runContainerLifecycle(ctx, req, "restart", "kopsc docker restart <容器>...")
}

func runContainerLifecycle(ctx context.Context, req BuiltinRequest, subcommand, usage string) error {
	targets, passthrough, err := splitDockerTargets(req.Args, lifecycleValueOption)
	if err != nil {
		return err
	}
	if len(targets) == 0 {
		return errors.New("缺少容器；用法: " + usage)
	}

	names := make([]string, 0, len(targets))
	for _, target := range targets {
		ref, err := resolveContainerRef(ctx, target)
		if err != nil {
			return err
		}
		names = append(names, ref.Names)
	}

	args := append([]string{subcommand}, names...)
	args = append(args, passthrough...)
	return dockerRun(ctx, args, req)
}

// lifecycleValueOption lists stop/restart options that take a separate value,
// keyed the way splitDockerTargets looks them up (short and long spellings).
var lifecycleValueOption = map[string]bool{
	"t":       true,
	"time":    true,
	"s":       true,
	"signal":  true,
	"timeout": true,
	"signals": true,
}
