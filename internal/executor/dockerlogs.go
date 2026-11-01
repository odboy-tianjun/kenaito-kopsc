package executor

import (
	"bufio"
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strings"
)

// runDockerLogs implements `kopsc docker logs <容器> [docker 选项...]`.
//
// `--since`, `--grep` and `--save` are kopsc's own options and must appear
// before the container name; everything from the container name on is passed
// to docker verbatim, so `-f`, `--tail` and friends keep working. When no
// kopsc-only option is used the logs stream straight through; with --grep or
// --save they are filtered and/or written to a file while still being shown.
func runDockerLogs(ctx context.Context, req BuiltinRequest) error {
	opts, target, rest, err := parseDockerLogs(req.Args)
	if err != nil {
		return err
	}
	if target == "" {
		return errors.New("缺少容器；用法: kopsc docker logs <容器> [选项...]")
	}

	args := append([]string{"logs", target}, rest...)
	if opts.grep == nil && opts.save == "" {
		return dockerRun(ctx, args, req)
	}
	return streamLogs(ctx, args, opts, req)
}

// logsOptions are kopsc's private options for `docker logs`.
type logsOptions struct {
	grep *regexp.Regexp // keep only lines matching this pattern
	save string         // append the displayed log to this file
}

// parseDockerLogs splits kopsc's options (before the container) from the
// container name and the arguments forwarded to docker (after it).
//
// --since is accepted on either side of the container name; kopsc turns it
// into docker's own --since, so it reaches docker exactly once.
func parseDockerLogs(args []string) (opts logsOptions, target string, rest []string, err error) {
	i := 0
	for ; i < len(args); i++ {
		arg := args[i]

		if arg == "--" {
			i++
			break
		}
		if !strings.HasPrefix(arg, "-") || arg == "-" {
			break
		}

		name, value := arg, ""
		if eq := strings.IndexByte(arg, '='); eq >= 0 && strings.HasPrefix(arg, "--") {
			name, value = arg[:eq], arg[eq+1:]
		}

		takeValue := func() (string, error) {
			if value != "" {
				return value, nil
			}
			if i+1 >= len(args) {
				return "", fmt.Errorf("%s 缺少参数", name)
			}
			i++
			return args[i], nil
		}

		switch name {
		case "--grep":
			pattern, err := takeValue()
			if err != nil {
				return opts, "", nil, err
			}
			re, err := regexp.Compile(pattern)
			if err != nil {
				return opts, "", nil, fmt.Errorf("--grep 的正则表达式无效: %w", err)
			}
			opts.grep = re
		case "--save":
			save, err := takeValue()
			if err != nil {
				return opts, "", nil, err
			}
			opts.save = save
		case "--since":
			since, err := takeValue()
			if err != nil {
				return opts, "", nil, err
			}
			rest = append(rest, "--since", since)
		default:
			// A docker option typed before the container name: hand it to
			// docker, keeping any separate value attached to it.
			rest = append(rest, arg)
			if value == "" && dockerOptionTakesValue(name) {
				next, err := takeValue()
				if err != nil {
					return opts, "", nil, err
				}
				rest = append(rest, next)
			}
		}
	}

	if i >= len(args) {
		return opts, "", rest, nil
	}
	return opts, args[i], append(rest, args[i+1:]...), nil
}

// dockerOptionTakesValue lists docker logs options that take a separate
// value, so the value is not mistaken for the container name.
func dockerOptionTakesValue(arg string) bool {
	switch arg {
	case "--tail", "-n", "--since", "-s", "--until", "--details", "--timestamp":
		return true
	}
	return false
}

// streamLogs runs docker logs, relaying it line by line so --grep can filter
// and --save can capture the log while it is still displayed.
func streamLogs(ctx context.Context, args []string, opts logsOptions, req BuiltinRequest) error {
	cmd, out, err := startDockerPipe(ctx, args, req.Stderr)
	if err != nil {
		return err
	}

	var file *os.File
	if opts.save != "" {
		if file, err = openLogSink(opts.save); err != nil {
			_ = cmd.Process.Kill()
			return err
		}
		defer file.Close()
	}

	scanner := bufio.NewScanner(out)
	scanner.Buffer(make([]byte, 0, 64*1024), 4*1024*1024)
	for scanner.Scan() {
		line := scanner.Text()
		if opts.grep != nil && !opts.grep.MatchString(line) {
			continue
		}
		fmt.Fprintln(req.Stdout, line)
		if file != nil {
			if _, err := fmt.Fprintln(file, line); err != nil {
				return fmt.Errorf("写入 %q 失败: %w", opts.save, err)
			}
		}
	}

	return waitDockerPipe(cmd, scanner.Err())
}

// openLogSink opens the --save file for appending, creating its directory.
func openLogSink(path string) (*os.File, error) {
	if dir := filepath.Dir(path); dir != "" && dir != "." {
		if err := os.MkdirAll(dir, 0o755); err != nil {
			return nil, fmt.Errorf("创建目录 %q 失败: %w", dir, err)
		}
	}
	f, err := os.OpenFile(path, os.O_WRONLY|os.O_CREATE|os.O_APPEND, 0o644)
	if err != nil {
		return nil, fmt.Errorf("打开日志文件 %q 失败: %w", path, err)
	}
	return f, nil
}
