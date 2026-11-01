package executor

import (
	"context"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"

	"gitea.odboy.cn/odboy/kenaito-kopsc/internal/platform"
)

// javaRunOptions are the parsed arguments of `kopsc java run`.
type javaRunOptions struct {
	jar        string   // the application jar
	appArgs    []string // everything after "--", passed to the application
	jdk        string   // --jdk: 8 / 11 / 17 / 21
	javaBin    string   // --java: an explicit java executable
	heap       string   // --heap / --xmx
	initial    string   // --xms
	env        string   // --env
	gcDir      string   // --gc-log directory
	heapDump   bool     // --heap-dump-on-oom
	printFlags bool     // --print-flags
	quiet      bool     // -q / --quiet
}

// supportedJDKs are the releases kopsc has a tuning profile for.
var supportedJDKs = []string{"8", "11", "17", "21"}

// runJavaRun launches a Spring-style Java application with kopsc's tuned JVM
// flags, in the foreground, so its logs and Ctrl+C behave like a plain
// `java -jar`.
func runJavaRun(ctx context.Context, req BuiltinRequest) error {
	opts, err := parseJavaRunArgs(req.Args)
	if err != nil {
		return err
	}

	jar, err := resolveJarPath(opts.jar)
	if err != nil {
		return err
	}

	jdk, javaBin, err := resolveJDK(opts)
	if err != nil {
		return err
	}

	res := platform.DetectResources()
	tuning := javaTuning{
		JDK:      jdk,
		CPUs:     res.CPUs,
		Memory:   res.Memory,
		Heap:     opts.heap,
		Initial:  opts.initial,
		Env:      opts.env,
		GCDir:    opts.gcDir,
		HeapDump: opts.heapDump,
	}
	flags := BuildJavaFlags(tuning)
	argv := append([]string{javaBin}, flags...)
	argv = append(argv, "-jar", jar)
	argv = append(argv, opts.appArgs...)

	if opts.printFlags {
		fmt.Fprintln(req.Stdout, strings.Join(argv, " "))
		return nil
	}

	if !opts.quiet {
		fmt.Fprintf(req.Stdout, "JDK:    %s（探测到 %s）\n", jdk, describeJava(javaBin))
		fmt.Fprintf(req.Stdout, "资源:   %d 核, 内存 %s（%s）\n", res.CPUs, formatBytes(res.Memory), res.Detected)
		fmt.Fprintf(req.Stdout, "堆:     %s\n", heapSummary(flags))
		if opts.gcDir != "" {
			fmt.Fprintf(req.Stdout, "GC 日志: %s\n", gcLogPath(opts.gcDir))
		}
		fmt.Fprintf(req.Stdout, "启动:   %s\n\n", strings.Join(argv, " "))
	}

	if opts.gcDir != "" {
		if err := os.MkdirAll(opts.gcDir, 0o755); err != nil {
			return fmt.Errorf("创建 GC 日志目录 %q 失败: %w", opts.gcDir, err)
		}
	}

	return runJavaForeground(ctx, argv, req)
}

// runJavaForeground runs the JVM attached to the caller's terminal. The exit
// code is passed through so supervisors see the application's own status.
//
// Ctrl+C is forwarded to the JVM instead of killing kopsc outright, so the
// application's shutdown hooks run and Spring can close its context cleanly.
func runJavaForeground(ctx context.Context, argv []string, req BuiltinRequest) error {
	cmd := exec.Command(argv[0], argv[1:]...)
	cmd.Stdin = req.Stdin
	cmd.Stdout = req.Stdout
	cmd.Stderr = req.Stderr
	// Relay Ctrl+C/SIGTERM to the JVM so its shutdown hooks run.
	stopSignals := forwardSignals(cmd)
	defer stopSignals()

	if err := cmd.Start(); err != nil {
		return fmt.Errorf("启动 java 失败: %w", err)
	}

	// Killing on context cancellation keeps kopsc from outliving its parent
	// while the application itself is still shutting down gracefully.
	done := make(chan struct{})
	go func() {
		select {
		case <-ctx.Done():
			_ = cmd.Process.Kill()
		case <-done:
		}
	}()

	err := cmd.Wait()
	close(done)
	if err != nil {
		var exitErr *exec.ExitError
		if errors.As(err, &exitErr) {
			return &ExitError{Code: exitErr.ExitCode(), Err: err}
		}
		return fmt.Errorf("等待 java 退出失败: %w", err)
	}
	return nil
}

// resolveJarPath checks that the jar exists and returns an absolute path so the
// JVM resolves it independently of its working directory.
func resolveJarPath(jar string) (string, error) {
	info, err := os.Stat(jar)
	if err != nil {
		return "", fmt.Errorf("找不到应用 jar %q: %w；用法: kopsc java run <app.jar> [-- 应用参数...]", jar, err)
	}
	if info.IsDir() {
		return "", fmt.Errorf("%q 是目录，不是 jar 文件", jar)
	}
	abs, err := filepath.Abs(jar)
	if err != nil {
		return jar, nil
	}
	return abs, nil
}

// resolveJDK decides which JDK to tune for and which java executable to run.
func resolveJDK(opts javaRunOptions) (version, javaBin string, err error) {
	if opts.jdk != "" {
		version = normalizeJDKFlag(opts.jdk)
		if !containsString(supportedJDKs, version) {
			return "", "", fmt.Errorf("--jdk 只支持 %s，收到 %q",
				strings.Join(supportedJDKs, "/"), opts.jdk)
		}
		return version, javaExecutable(opts.javaBin), nil
	}

	var info platform.Java
	if opts.javaBin != "" {
		info = platform.DetectJavaAt(opts.javaBin)
	} else {
		info = platform.DetectJava()
	}
	if info.Version == "" {
		return "", "", fmt.Errorf("无法自动识别 JDK 版本（%s）；请用 --jdk 指定：%s",
			describeJavaDetection(info), strings.Join(supportedJDKs, " / "))
	}
	if !containsString(supportedJDKs, info.Version) {
		return "", "", fmt.Errorf("检测到 JDK %s，kopsc 暂无该版本的调优方案；"+
			"可用 --jdk 指定按 %s 的参数运行", info.Raw, strings.Join(supportedJDKs, " / "))
	}
	return info.Version, firstNonEmptyString(info.Path, javaExecutable(opts.javaBin)), nil
}

// javaExecutable returns the java binary to run: --java, or "java" on PATH.
func javaExecutable(explicit string) string {
	if explicit != "" {
		return explicit
	}
	return "java"
}

// normalizeJDKFlag accepts "8", "1.8", "jdk8", "JDK 17" and similar spellings.
func normalizeJDKFlag(value string) string {
	v := strings.ToLower(strings.TrimSpace(value))
	v = strings.TrimPrefix(v, "jdk")
	v = strings.TrimSpace(v)
	if strings.HasPrefix(v, "1.") {
		v = strings.TrimPrefix(v, "1.")
	}
	if i := strings.IndexAny(v, "._"); i > 0 {
		v = v[:i]
	}
	if _, err := strconv.Atoi(v); err != nil {
		return value
	}
	return v
}

// describeJava renders the java executable for the startup summary.
func describeJava(path string) string {
	if path == "" {
		return "java（PATH 中）"
	}
	return path
}

func describeJavaDetection(info platform.Java) string {
	if info.Path == "" {
		return "PATH 中没有 java"
	}
	return "java 存在但版本无法解析"
}

// heapSummary extracts the -Xmx value for the startup summary.
func heapSummary(flags []string) string {
	for _, f := range flags {
		if strings.HasPrefix(f, "-Xmx") {
			return strings.TrimPrefix(f, "-Xmx") + "（-Xms 相同或由 --xms 指定）"
		}
	}
	return "交由 JVM 自行决定（未识别到内存）"
}

// formatBytes renders a byte count for humans; "未知" when it is 0.
func formatBytes(n int64) string {
	if n <= 0 {
		return "未知"
	}
	return humanSize(n)
}

func containsString(values []string, want string) bool {
	for _, v := range values {
		if v == want {
			return true
		}
	}
	return false
}

func firstNonEmptyString(values ...string) string {
	for _, v := range values {
		if v != "" {
			return v
		}
	}
	return ""
}

// parseJavaRunArgs parses kopsc's options, which must come before the first
// positional argument (the jar). Everything after "--" is handed to the
// application untouched.
func parseJavaRunArgs(args []string) (javaRunOptions, error) {
	var opts javaRunOptions
	i := 0
	for ; i < len(args); i++ {
		arg := args[i]

		if arg == "--" {
			// `-- jar args` : the jar follows the separator.
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
		case "--jdk":
			v, err := takeValue()
			if err != nil {
				return opts, err
			}
			opts.jdk = v
		case "--java":
			v, err := takeValue()
			if err != nil {
				return opts, err
			}
			opts.javaBin = v
		case "--heap", "--xmx":
			v, err := takeValue()
			if err != nil {
				return opts, err
			}
			if parseJVMSize(v) <= 0 {
				return opts, fmt.Errorf("--heap 无法识别大小 %q，可用 2g、512m 或字节数", v)
			}
			opts.heap = v
		case "--xms":
			v, err := takeValue()
			if err != nil {
				return opts, err
			}
			if parseJVMSize(v) <= 0 {
				return opts, fmt.Errorf("--xms 无法识别大小 %q，可用 2g、512m 或字节数", v)
			}
			opts.initial = v
		case "--env":
			v, err := takeValue()
			if err != nil {
				return opts, err
			}
			opts.env = v
		case "--gc-log":
			v, err := takeValue()
			if err != nil {
				return opts, err
			}
			opts.gcDir = v
		case "--no-gc-log":
			opts.gcDir = "" // explicit, and the default is off
		case "--heap-dump-on-oom":
			opts.heapDump = true
		case "--print-flags":
			opts.printFlags = true
		case "-q", "--quiet":
			opts.quiet = true
		default:
			return opts, fmt.Errorf("未知选项 %q；应用自己的参数请写在 jar 之后，或用 -- 分隔", name)
		}
	}

	if i >= len(args) {
		return opts, errors.New("缺少应用 jar；用法: kopsc java run <app.jar> [-- 应用参数...]")
	}
	opts.jar = args[i]
	rest := args[i+1:]
	// A separator after the jar is not an application argument.
	if len(rest) > 0 && rest[0] == "--" {
		rest = rest[1:]
	}
	opts.appArgs = append(opts.appArgs, rest...)
	return opts, nil
}

// runJavaPreview renders the command `kopsc java run` would execute without
// starting anything. It backs both --dry-run and --print-flags, so the same
// resolution code path is used; only the output differs.
func runJavaPreview(ctx context.Context, req BuiltinRequest) error {
	opts, err := parseJavaRunArgs(req.Args)
	if err != nil {
		return err
	}
	// The jar may not exist yet in a dry run; report it rather than failing so
	// the flags can still be inspected.
	if _, statErr := os.Stat(opts.jar); statErr != nil {
		fmt.Fprintf(req.Stderr, "注意: %v\n", statErr)
	}

	jdk, javaBin, err := resolveJDK(opts)
	if err != nil {
		return err
	}

	res := platform.DetectResources()
	flags := BuildJavaFlags(javaTuning{
		JDK:      jdk,
		CPUs:     res.CPUs,
		Memory:   res.Memory,
		Heap:     opts.heap,
		Initial:  opts.initial,
		Env:      opts.env,
		GCDir:    opts.gcDir,
		HeapDump: opts.heapDump,
	})
	argv := append([]string{javaBin}, flags...)
	argv = append(argv, "-jar", opts.jar)
	argv = append(argv, opts.appArgs...)

	fmt.Fprintln(req.Stdout, "将要启动的 java 命令:")
	fmt.Fprintln(req.Stdout, strings.Join(argv, " "))
	fmt.Fprintf(req.Stdout, "\n资源:   %d 核, 内存 %s（%s）\n", res.CPUs, formatBytes(res.Memory), res.Detected)
	return nil
}
