package executor

import (
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
)

// nohupOptions are the options of `kopsc proc nohup`. Their names echo the
// shell idiom they replace:
//
//	nohup cmd... > output.log 2>&1 &
type nohupOptions struct {
	command []string // argv of the command to launch
	outPath string   // output file (the shell's "> output.log 2>&1")
	dir     string   // working directory of the child (-C)
	env     []string // extra NAME=VALUE entries (-E), appended to the env
	detach  bool     // start in a new session, with no controlling terminal
	quiet   bool     // do not print the launch summary
	wait    bool     // stay attached until the command exits
}

// runNohup implements `nohup <命令> > output.log 2>&1 &`: launch a command in
// the background, detached from the current terminal, with stdout and stderr
// appended to a log file.
//
// A real `nohup ... &` is a shell job: it survives the shell exiting, ignores
// SIGHUP and keeps a process group of its own. kopsc reproduces the parts that
// make that idiom useful and that are portable:
//
//   - the child's stdin is closed, so it cannot hold a pipe or terminal open;
//   - on Unix its stdout and stderr are appended to the log file, its signal
//     handler ignores SIGHUP and (with --detach, the default) it runs in a new
//     session with no controlling terminal;
//   - on Windows it is started with DETACHED_PROCESS and its inherited handles
//     are redirected to the same log file;
//   - kopsc returns as soon as the command has started, without waiting.
//
// The child is started with exec.Cmd.Start and never with a shell, so the
// argument vector is passed through verbatim on every platform.
func runNohup(ctx context.Context, req BuiltinRequest) error {
	opts, err := parseNohupArgs(req.Args)
	if err != nil {
		return err
	}
	if err := ctx.Err(); err != nil {
		return err
	}

	dir := opts.dir
	if dir == "" {
		if dir, err = os.Getwd(); err != nil {
			return fmt.Errorf("无法获取当前目录: %w", err)
		}
	}
	if resolved, err := filepath.Abs(dir); err == nil {
		dir = resolved
	}

	name, lookErr := lookPathInDir(opts.command[0], dir)
	if lookErr != nil {
		return lookErr
	}

	logPath := opts.outPath
	if logPath == "" {
		logPath = defaultLogName(opts.command[0])
	}
	if !filepath.IsAbs(logPath) {
		logPath = filepath.Join(dir, logPath)
	}

	logFile, err := openNohupLog(logPath)
	if err != nil {
		return err
	}
	// The child keeps its own duplicate of the descriptor; the parent's copy
	// is closed as soon as the process is running.
	defer logFile.Close()

	cmd := exec.Command(name, opts.command[1:]...)
	cmd.Dir = dir
	cmd.Env = append(os.Environ(), opts.env...)
	prepareNohup(cmd, logFile, opts.detach)

	if err := cmd.Start(); err != nil {
		return fmt.Errorf("启动 %q 失败: %w", opts.command[0], err)
	}

	w := req.Stdout
	if opts.quiet {
		w = io.Discard
	}
	fmt.Fprintf(w, "已在后台启动: %s\n", strings.Join(opts.command, " "))
	fmt.Fprintf(w, "PID:    %d\n", cmd.Process.Pid)
	fmt.Fprintf(w, "目录:   %s\n", dir)
	fmt.Fprintf(w, "日志:   %s\n", logPath)
	if !opts.detach {
		fmt.Fprintf(w, "方式:   后台运行，未脱离当前会话（关闭终端可能终止该命令）\n")
	}
	fmt.Fprintf(w, "查看:   %s\n", nohupFollowHint(logPath))
	if !opts.wait {
		// Release the child so the OS reaps it when it exits, independent of
		// this (short-lived) kopsc process.
		_ = cmd.Process.Release()
		return nil
	}

	fmt.Fprintf(w, "等待:   前台等待，直到该命令退出\n")
	return waitNohup(cmd, logPath, w, req)
}

// parseNohupArgs parses kopsc-level options and stops at the first argument
// that is neither a recognised option nor an option value: everything from
// there on is the command to run, verbatim (so `kopsc proc nohup ffmpeg -i a`
// passes -i to ffmpeg). An explicit `--` ends option parsing.
func parseNohupArgs(args []string) (nohupOptions, error) {
	var opts nohupOptions
	if len(args) == 0 {
		return opts, errors.New("必须提供要后台运行的命令，例如 kopsc proc nohup ./server > run.log")
	}

	i := 0
	for i < len(args) {
		arg := args[i]
		if arg == "--" {
			i++
			break
		}
		if !strings.HasPrefix(arg, "-") || arg == "-" {
			break
		}

		var name, value string
		if eq := strings.IndexByte(arg, '='); eq >= 0 && strings.HasPrefix(arg, "--") {
			name, value = arg[:eq], arg[eq+1:]
		} else {
			name = arg
		}

		// takeValue consumes the next argument as this option's value.
		takeValue := func(display string) (string, error) {
			if value != "" {
				return value, nil
			}
			if i+1 >= len(args) {
				return "", fmt.Errorf("%s 缺少参数", display)
			}
			i++
			return args[i], nil
		}

		switch name {
		case "-o", "--out", "--output":
			v, err := takeValue(name)
			if err != nil {
				return opts, err
			}
			opts.outPath = v
		case "-C", "--dir", "--chdir":
			v, err := takeValue(name)
			if err != nil {
				return opts, err
			}
			opts.dir = v
		case "-E", "--env":
			v, err := takeValue(name)
			if err != nil {
				return opts, err
			}
			if !strings.Contains(v, "=") {
				return opts, fmt.Errorf("%s 需要 NAME=VALUE 形式，收到 %q", name, v)
			}
			opts.env = append(opts.env, v)
		case "--detach":
			b, err := boolFlagValue(name, value)
			if err != nil {
				return opts, err
			}
			opts.detach = b
		case "--no-detach":
			opts.detach = false
		case "-q", "--quiet":
			opts.quiet = true
		case "-w", "--wait":
			opts.wait = true
		default:
			return opts, fmt.Errorf("未知选项 %q；命令自身的选项请写在该命令之后，或用 -- 分隔", name)
		}
		i++
	}

	if i >= len(args) {
		return opts, errors.New("必须提供要后台运行的命令，例如 kopsc proc nohup ./server")
	}
	opts.command = append([]string(nil), args[i:]...)
	return opts, nil
}

func boolFlagValue(name, value string) (bool, error) {
	if value == "" {
		return true, nil
	}
	b, err := parseBool(value)
	if err != nil {
		return false, fmt.Errorf("%s 需要布尔值，收到 %q", name, value)
	}
	return b, nil
}

func parseBool(value string) (bool, error) {
	switch strings.ToLower(value) {
	case "true", "1", "yes":
		return true, nil
	case "false", "0", "no":
		return false, nil
	}
	return false, fmt.Errorf("invalid boolean %q", value)
}

// lookPathInDir resolves the program like the shell would: a name containing a
// path separator is used as given, anything else is searched on PATH.
func lookPathInDir(program, dir string) (string, error) {
	if !strings.ContainsAny(program, `/\`) {
		name, err := exec.LookPath(program)
		if err != nil {
			return "", fmt.Errorf("找不到可执行文件 %q: %w", program, err)
		}
		return name, nil
	}
	// A path: resolve it like the shell would, relative to the child's dir.
	if filepath.IsAbs(program) {
		return program, nil
	}
	name := filepath.Join(dir, program)
	if _, err := os.Stat(name); err == nil {
		return name, nil
	}
	return program, nil
}

// defaultLogName mirrors `nohup.out`: the file `nohup` would otherwise write.
func defaultLogName(program string) string {
	return filepath.Base(program) + ".out"
}

// openNohupLog creates the log file if needed and returns it opened for
// appending, so repeated launches accumulate rather than overwrite — exactly
// like `>>`, and like the redirect idiom when the file already exists.
func openNohupLog(path string) (*os.File, error) {
	if dir := filepath.Dir(path); dir != "" {
		if err := os.MkdirAll(dir, 0o755); err != nil {
			return nil, fmt.Errorf("创建日志目录 %q 失败: %w", dir, err)
		}
	}
	f, err := os.OpenFile(path, os.O_WRONLY|os.O_CREATE|os.O_APPEND, 0o644)
	if err != nil {
		return nil, fmt.Errorf("打开日志文件 %q 失败: %w", path, err)
	}
	return f, nil
}

// nohupFollowHint renders a shell command that follows the log.
func nohupFollowHint(path string) string {
	if runtime.GOOS == "windows" {
		return "Get-Content -Wait " + quoteForShell(path, "windows")
	}
	return "tail -f " + quoteForShell(path, runtime.GOOS)
}

// waitNohup stays attached to a child started with --wait, then relays what it
// appended to the log file so an interactive run still shows the output.
func waitNohup(cmd *exec.Cmd, logPath string, w io.Writer, req BuiltinRequest) error {
	offset := fileSize(logPath)
	err := cmd.Wait()
	relayLog(logPath, offset, w, req)
	if err != nil {
		var exitErr *exec.ExitError
		if errors.As(err, &exitErr) {
			return &ExitError{Code: exitErr.ExitCode(), Err: err}
		}
		return err
	}
	return nil
}

func fileSize(path string) int64 {
	if info, err := os.Stat(path); err == nil {
		return info.Size()
	}
	return 0
}

// relayLog copies everything written to the log after offset to w and stderr.
func relayLog(path string, offset int64, w io.Writer, req BuiltinRequest) {
	if w != nil {
		copyLogTail(path, offset, w)
	}
	if req.Stderr != nil && req.Stderr != w {
		copyLogTail(path, offset, req.Stderr)
	}
}

func copyLogTail(path string, offset int64, w io.Writer) {
	f, err := os.Open(path)
	if err != nil {
		return
	}
	defer f.Close()
	if _, err := f.Seek(offset, io.SeekStart); err != nil {
		return
	}
	_, _ = io.Copy(w, f)
}
