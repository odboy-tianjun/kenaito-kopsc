package executor

import (
	"context"
	"errors"
	"fmt"
	"strings"
)

// execValueOption lists docker exec options that take a separate value, so the
// value is not mistaken for the container name or the command.
var execValueOption = map[string]bool{
	"u":           true,
	"user":        true,
	"w":           true,
	"workdir":     true,
	"e":           true,
	"env":         true,
	"env-file":    true,
	"detach-keys": true,
}

// runDockerExec implements `kopsc docker exec <容器> <命令...>`.
//
// Interactive use is the default, so `-it` is no longer typed: kopsc always
// runs docker exec with -it and separates the command with `--`, which keeps
// the command's own flags from being read as docker options. Passing -i/-t/-it
// explicitly is an error, because the flag is now implied.
//
// Arguments before the container name are docker exec options and are
// forwarded; everything after the container name is the command, verbatim.
func runDockerExec(ctx context.Context, req BuiltinRequest) error {
	args := req.Args
	if err := rejectInteractiveFlags(args); err != nil {
		return err
	}
	if len(args) == 0 {
		return errors.New("缺少容器；用法: kopsc docker exec <容器> <命令...>")
	}

	var opts []string
	target := ""
	for i := 0; i < len(args); i++ {
		arg := args[i]
		switch {
		case arg == "--":
			// An explicit separator before the container name: what follows is
			// still the container, then the command.
			if i+1 < len(args) {
				i++
				target = args[i]
				return dockerRun(ctx, execArgv(opts, target, args[i+1:]...), req)
			}
			return errors.New("`--` 之后缺少容器；用法: kopsc docker exec <容器> <命令...>")
		case !strings.HasPrefix(arg, "-") || arg == "-":
			// The first positional argument is the container; the rest is the
			// command, which may itself contain flags.
			target = arg
			return dockerRun(ctx, execArgv(opts, target, args[i+1:]...), req)
		default:
			// A docker exec option; keep its value attached to it.
			opts = append(opts, arg)
			if !strings.Contains(arg, "=") && execValueOption[strings.TrimLeft(arg, "-")] {
				if i+1 >= len(args) {
					return fmt.Errorf("选项 %s 缺少参数", arg)
				}
				i++
				opts = append(opts, args[i])
			}
		}
	}

	return errors.New("缺少容器；用法: kopsc docker exec <容器> <命令...>")
}

// execArgv builds `exec -it <已有的 docker 选项> <容器> -- <命令...>`.
func execArgv(opts []string, target string, command ...string) []string {
	argv := append([]string{"exec", "-it"}, opts...)
	argv = append(argv, target, "--")
	return append(argv, command...)
}

// rejectInteractiveFlags refuses -i/-t/-it/--interactive/--tty: kopsc always
// provides them.
func rejectInteractiveFlags(args []string) error {
	for _, arg := range args {
		if arg == "--" || !strings.HasPrefix(arg, "-") || arg == "-" {
			return nil
		}
		switch {
		case arg == "--interactive" || arg == "--tty" || arg == "-i" || arg == "-t" || arg == "-it" || arg == "-ti":
			return errors.New("kopsc docker exec 已默认以交互方式（-it）执行，无需再传 " + arg +
				"；用法: kopsc docker exec <容器> <命令...>")
		}
		if strings.HasPrefix(arg, "--") {
			continue
		}
		// Bundled short flags such as -di still carry -i or -t.
		short := strings.TrimPrefix(arg, "-")
		if strings.ContainsRune(short, 'i') || strings.ContainsRune(short, 't') {
			return errors.New("kopsc docker exec 已默认以交互方式（-it）执行，无需再传 " + arg +
				"；用法: kopsc docker exec <容器> <命令...>")
		}
	}
	return nil
}
