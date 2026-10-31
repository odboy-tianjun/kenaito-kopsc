// Package executor turns a resolved variant into a process, with dry-run
// support and faithful stdio/exit-code passthrough.
package executor

import (
	"context"
	"errors"
	"fmt"
	"io"
	"os/exec"
	"runtime"
	"strings"
	"unicode"

	"gitea.odboy.cn/odboy/kenaito-kopsc/internal/resolve"
)

// Options controls how a Plan is executed.
type Options struct {
	DryRun bool
	Stdin  io.Reader
	Stdout io.Writer
	Stderr io.Writer
	Env    []string // optional override of the child environment
	Dir    string   // optional working directory
}

// Plan is a resolved variant plus arguments forwarded from the caller.
type Plan struct {
	Result    resolve.Result
	ExtraArgs []string
}

// Argv returns the final argument vector: the variant's argv followed by the
// forwarded arguments.
func (p Plan) Argv() []string {
	argv := make([]string, 0, len(p.Result.Variant.Argv)+len(p.ExtraArgs))
	argv = append(argv, p.Result.Variant.Argv...)
	argv = append(argv, p.ExtraArgs...)
	return argv
}

// Display renders the command line the plan would run.
func (p Plan) Display() string {
	if p.Result.Variant.Builtin != "" {
		parts := append([]string{"(内置)", p.Result.Variant.Builtin}, p.ExtraArgs...)
		return strings.Join(parts, " ")
	}
	argv := p.Argv()
	if p.Result.Variant.Shell {
		flag := "-c"
		if runtime.GOOS == "windows" {
			flag = "/c"
		}
		return shellName() + " " + flag + " " + quoteForShell(strings.Join(argv, " "), runtime.GOOS)
	}
	return strings.Join(argv, " ")
}

// Explain writes why this variant was chosen and what will run.
func (p Plan) Explain(w io.Writer) {
	if w == nil {
		return
	}
	variant := p.Result.Variant
	fmt.Fprintf(w, "将执行: %s\n", p.Display())
	fmt.Fprintf(w, "平台:   %s\n", resolve.DescribeInfo(p.Result.Info))
	switch {
	case variant.Builtin != "":
		fmt.Fprintf(w, "方式:   内置实现（%s，不启动外部进程）\n", variant.Builtin)
	case variant.Shell:
		fmt.Fprintf(w, "方式:   shell (%s)\n", shellName())
	default:
		fmt.Fprintf(w, "方式:   直接执行\n")
	}
	note := variant.Note
	if note == "" {
		note = "无备注"
	}
	fmt.Fprintf(w, "命中:   变体 #%d（%d 分）%s\n", p.Result.VariantIdx, p.Result.Score, note)
	if p.Result.VariantIdx >= 0 && p.Result.VariantIdx < len(p.Result.Candidates) {
		fmt.Fprintf(w, "依据:   %s\n", p.Result.Candidates[p.Result.VariantIdx].Reason)
	}
	if len(p.ExtraArgs) > 0 {
		fmt.Fprintf(w, "透传:   %s\n", strings.Join(p.ExtraArgs, " "))
	}
}

// Run executes the plan. With DryRun it only prints the explanation. A child
// exiting non-zero is reported as *ExitError so callers can preserve the code.
func (p Plan) Run(ctx context.Context, opts Options) error {
	if opts.Stdout == nil {
		opts.Stdout = io.Discard
	}
	if opts.Stderr == nil {
		opts.Stderr = io.Discard
	}
	if opts.DryRun {
		fmt.Fprintln(opts.Stdout, "[dry-run] 未执行任何命令")
		p.Explain(opts.Stdout)
		return nil
	}

	if variant := p.Result.Variant; variant.Builtin != "" {
		fn, ok := LookupBuiltin(variant.Builtin)
		if !ok {
			return fmt.Errorf("未注册的内置动作 %q", variant.Builtin)
		}
		return fn(ctx, BuiltinRequest{Args: p.ExtraArgs, Stdout: opts.Stdout, Stderr: opts.Stderr})
	}

	name, args := p.command()
	cmd := exec.CommandContext(ctx, name, args...)
	cmd.Stdin = opts.Stdin
	cmd.Stdout = opts.Stdout
	cmd.Stderr = opts.Stderr
	cmd.Env = opts.Env
	cmd.Dir = opts.Dir

	if err := cmd.Run(); err != nil {
		var exitErr *exec.ExitError
		if errors.As(err, &exitErr) {
			return &ExitError{Code: exitErr.ExitCode(), Err: err}
		}
		return fmt.Errorf("执行 %q 失败: %w", name, err)
	}
	return nil
}

func (p Plan) command() (string, []string) {
	argv := p.Argv()
	if !p.Result.Variant.Shell {
		return argv[0], argv[1:]
	}
	shell := shellName()
	if runtime.GOOS == "windows" {
		return shell, []string{"/c", joinForShell(argv, runtime.GOOS)}
	}
	return shell, []string{"-c", joinForShell(argv, runtime.GOOS)}
}

func shellName() string {
	if runtime.GOOS == "windows" {
		return "cmd"
	}
	return "sh"
}

// joinForShell joins argv with platform-appropriate quoting.
func joinForShell(argv []string, osName string) string {
	parts := make([]string, len(argv))
	for i, arg := range argv {
		parts[i] = quoteForShell(arg, osName)
	}
	return strings.Join(parts, " ")
}

// quoteForShell leaves safe arguments untouched and quotes the rest.
func quoteForShell(arg, osName string) string {
	if osName == "windows" {
		if arg != "" && !strings.ContainsAny(arg, " \t\"&|<>^%") {
			return arg
		}
		return `"` + strings.ReplaceAll(arg, `"`, `\"`) + `"`
	}

	if arg != "" && strings.IndexFunc(arg, unsafeShellRune) < 0 {
		return arg
	}
	return "'" + strings.ReplaceAll(arg, "'", `'\''`) + "'"
}

func unsafeShellRune(r rune) bool {
	if unicode.IsLetter(r) || unicode.IsDigit(r) {
		return false
	}
	return !strings.ContainsRune("-_.:/=@+,", r)
}

// ExitError carries the child process exit code.
type ExitError struct {
	Code int
	Err  error
}

func (e *ExitError) Error() string {
	return fmt.Sprintf("命令以状态码 %d 退出", e.Code)
}

func (e *ExitError) Unwrap() error { return e.Err }

// ExitCode maps an error to the process exit code kopsc should return.
func ExitCode(err error) int {
	if err == nil {
		return 0
	}
	var exitErr *ExitError
	if errors.As(err, &exitErr) {
		return exitErr.Code
	}
	return 1
}
