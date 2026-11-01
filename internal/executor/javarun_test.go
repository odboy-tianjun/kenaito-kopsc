package executor

import (
	"bytes"
	"context"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

func TestParseJavaRunArgsBasic(t *testing.T) {
	opts, err := parseJavaRunArgs([]string{"app.jar"})
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	if opts.jar != "app.jar" || len(opts.appArgs) != 0 {
		t.Fatalf("opts = %+v", opts)
	}
}

func TestParseJavaRunArgsApplicationArgsAfterDoubleDash(t *testing.T) {
	opts, err := parseJavaRunArgs([]string{
		"--jdk", "17",
		"app.jar",
		"--",
		"--spring.profiles.active=prod", "--server.port=8080", "-X", "flag",
	})
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	if opts.jar != "app.jar" {
		t.Errorf("jar = %q", opts.jar)
	}
	want := "--spring.profiles.active=prod --server.port=8080 -X flag"
	if got := strings.Join(opts.appArgs, " "); got != want {
		t.Errorf("appArgs = %q, want %q", got, want)
	}
}

func TestParseJavaRunArgsKeepsJarArgsWithoutDoubleDash(t *testing.T) {
	// Arguments after the jar belong to the application, so a leading dash
	// there must not be mistaken for a kopsc option.
	opts, err := parseJavaRunArgs([]string{"app.jar", "--spring.profiles.active=dev"})
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	if strings.Join(opts.appArgs, " ") != "--spring.profiles.active=dev" {
		t.Fatalf("appArgs = %v", opts.appArgs)
	}
}

func TestParseJavaRunArgsAllOptions(t *testing.T) {
	opts, err := parseJavaRunArgs([]string{
		"--jdk=11", "--heap", "2g", "--xms", "512m", "--env", "staging",
		"--gc-log", "/var/log/app", "--heap-dump-on-oom", "--print-flags", "-q",
		"app.jar",
	})
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	if opts.jdk != "11" || opts.heap != "2g" || opts.initial != "512m" ||
		opts.env != "staging" || opts.gcDir != "/var/log/app" ||
		!opts.heapDump || !opts.printFlags || !opts.quiet {
		t.Fatalf("opts = %+v", opts)
	}
}

func TestParseJavaRunArgsRejectsBadInput(t *testing.T) {
	tests := []struct {
		args []string
		want string
	}{
		{nil, "缺少应用 jar"},
		{[]string{"--jdk"}, "--jdk 缺少参数"},
		{[]string{"--heap", "huge", "app.jar"}, "--heap 无法识别大小"},
		{[]string{"--xms", "0", "app.jar"}, "--xms 无法识别大小"},
		{[]string{"--unknown", "app.jar"}, "未知选项"},
	}
	for _, tc := range tests {
		_, err := parseJavaRunArgs(tc.args)
		if err == nil || !strings.Contains(err.Error(), tc.want) {
			t.Errorf("args %v: err = %v, want %q", tc.args, err, tc.want)
		}
	}
}

func TestNormalizeJDKFlag(t *testing.T) {
	cases := map[string]string{
		"8":     "8",
		"1.8":   "8",
		"jdk8":  "8",
		"JDK17": "17",
		"17":    "17",
		"21.0":  "21",
	}
	for in, want := range cases {
		if got := normalizeJDKFlag(in); got != want {
			t.Errorf("normalizeJDKFlag(%q) = %q, want %q", in, got, want)
		}
	}
}

// TestJavaRunOptionsBeforeJar checks that every kopsc option is consumed
// before the jar and never leaks into the application's arguments.
func TestJavaRunOptionsBeforeJar(t *testing.T) {
	opts, err := parseJavaRunArgs([]string{
		"--jdk", "17",
		"--heap", "2g",
		"--xms", "1g",
		"--env", "prod",
		"--gc-log", "./logs",
		"--heap-dump-on-oom",
		"app.jar",
		"--",
		"--server.port=9000",
	})
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	if strings.Join(opts.appArgs, " ") != "--server.port=9000" {
		t.Fatalf("kopsc options leaked into app args: %v", opts.appArgs)
	}

	flags := BuildJavaFlags(javaTuning{
		JDK: opts.jdk, Heap: opts.heap, Initial: opts.initial,
		Env: opts.env, GCDir: opts.gcDir, HeapDump: opts.heapDump,
		Memory: 8 << 30,
	})
	for _, want := range []string{
		"-Xmx2g", "-Xms1g",
		"-Xlog:gc*,safepoint:./logs/gc.log:time,uptime:filecount=100,filesize=50M",
		"-XX:+HeapDumpOnOutOfMemoryError",
		"-Dconfig.file=application-prod.yaml",
	} {
		if indexOfPrefix(flags, want) < 0 {
			t.Errorf("flag %q missing for the parsed options: %v", want, flags)
		}
	}
}

func TestJavaRunNoGCLogByDefault(t *testing.T) {
	opts, err := parseJavaRunArgs([]string{"app.jar"})
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	if opts.gcDir != "" {
		t.Fatalf("GC logging should be off by default, got %q", opts.gcDir)
	}
	flags := BuildJavaFlags(javaTuning{JDK: "17", Memory: 2 << 30, GCDir: opts.gcDir})
	if indexOfPrefix(flags, "-Xlog:") >= 0 {
		t.Fatalf("no GC log flag expected when --gc-log is absent: %v", flags)
	}
}

func TestJavaRunRejectsUnsupportedJDK(t *testing.T) {
	// JDK 9/10 are not supported releases; kopsc must say so instead of
	// silently applying the wrong profile.
	if v := normalizeJDKFlag("9"); v != "9" {
		t.Fatalf("normalizeJDKFlag(9) = %q", v)
	}
	for _, supported := range supportedJDKs {
		if !containsString(supportedJDKs, supported) {
			t.Fatalf("%q should be supported", supported)
		}
	}
	if containsString(supportedJDKs, "9") {
		t.Fatal("JDK 9 must not be advertised as supported")
	}
}

// fakeJavaScript writes an executable shell script standing in for java.
func fakeJavaScript(t *testing.T, body string) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), "java")
	if err := os.WriteFile(path, []byte("#!/bin/sh\n"+body+"\n"), 0o755); err != nil {
		t.Fatal(err)
	}
	return path
}

func TestJavaRunForegroundWiresStdioAndExitCode(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("fake java is a shell script")
	}
	// The script echoes stdin to stdout, writes to stderr, and exits 7.
	java := fakeJavaScript(t, `cat; echo "to-stderr" >&2; echo "args: $*" >&2; exit 7`)

	jar := filepath.Join(t.TempDir(), "app.jar")
	if err := os.WriteFile(jar, []byte("PK"), 0o644); err != nil {
		t.Fatal(err)
	}

	var out, errOut bytes.Buffer
	err := runJavaRun(context.Background(), BuiltinRequest{
		Args:   []string{"--jdk", "17", "--java", java, "-q", jar, "--", "--x=1"},
		Stdin:  strings.NewReader("hello-stdin\n"),
		Stdout: &out,
		Stderr: &errOut,
	})
	if err == nil {
		t.Fatal("expected the exit code to be propagated")
	}
	if code := ExitCode(err); code != 7 {
		t.Fatalf("exit code = %d, want 7", code)
	}
	if !strings.Contains(out.String(), "hello-stdin") {
		t.Errorf("stdin was not forwarded to the JVM: stdout=%q", out.String())
	}
	if !strings.Contains(errOut.String(), "to-stderr") {
		t.Errorf("stderr was not forwarded: %q", errOut.String())
	}
	if !strings.Contains(errOut.String(), "--x=1") {
		t.Errorf("application arguments missing: %q", errOut.String())
	}
}

func TestJavaRunForegroundReturnsNilOnSuccess(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("fake java is a shell script")
	}
	java := fakeJavaScript(t, "exit 0")
	jar := filepath.Join(t.TempDir(), "app.jar")
	if err := os.WriteFile(jar, []byte("PK"), 0o644); err != nil {
		t.Fatal(err)
	}

	if err := runJavaRun(context.Background(), BuiltinRequest{
		Args:   []string{"--jdk", "21", "--java", java, "-q", jar},
		Stdout: &bytes.Buffer{},
		Stderr: &bytes.Buffer{},
	}); err != nil {
		t.Fatalf("successful run returned %v", err)
	}
}

// TestBuiltinRequestCarriesStdin pins the wiring that lets a builtin read the
// caller's stdin: Plan.Run must hand Options.Stdin to the builtin request.
func TestBuiltinRequestCarriesStdin(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("fake java is a shell script")
	}
	java := fakeJavaScript(t, "cat")
	jar := filepath.Join(t.TempDir(), "app.jar")
	if err := os.WriteFile(jar, []byte("PK"), 0o644); err != nil {
		t.Fatal(err)
	}

	plan := Plan{
		Result:    resolveResultWithVariant(registryVariantBuiltinFor("java.run")),
		ExtraArgs: []string{"--jdk", "17", "--java", java, "-q", jar},
	}
	var out bytes.Buffer
	if err := plan.Run(context.Background(), Options{
		Stdin:  strings.NewReader("from-plan-run\n"),
		Stdout: &out,
		Stderr: &out,
	}); err != nil {
		t.Fatalf("Run: %v", err)
	}
	if !strings.Contains(out.String(), "from-plan-run") {
		t.Fatalf("Options.Stdin never reached the builtin: %q", out.String())
	}
}

// TestPrintFlagsEmitsOnlyTheCommandLine keeps the output pasteable into a
// Dockerfile or systemd unit: exactly one line, starting with java.
func TestPrintFlagsEmitsOnlyTheCommandLine(t *testing.T) {
	jar := filepath.Join(t.TempDir(), "app.jar")
	if err := os.WriteFile(jar, []byte("PK"), 0o644); err != nil {
		t.Fatal(err)
	}

	var out bytes.Buffer
	if err := runJavaRun(context.Background(), BuiltinRequest{
		Args:   []string{"--jdk", "17", "--print-flags", jar, "--", "--server.port=9000"},
		Stdout: &out,
		Stderr: &bytes.Buffer{},
	}); err != nil {
		t.Fatalf("print-flags: %v", err)
	}

	text := strings.TrimRight(out.String(), "\n")
	if strings.Contains(text, "\n") {
		t.Fatalf("--print-flags must emit a single line, got:\n%s", out.String())
	}
	if !strings.HasPrefix(text, "java ") {
		t.Errorf("command should start with java: %q", text)
	}
	for _, want := range []string{"-server", "-XX:+UseG1GC", "-jar " + jar, "--server.port=9000"} {
		if !strings.Contains(text, want) {
			t.Errorf("--print-flags output missing %q: %s", want, text)
		}
	}
	// Nothing was started: no startup summary lines.
	if strings.Contains(text, "JDK:") || strings.Contains(text, "启动:") {
		t.Errorf("--print-flags must not print the startup summary: %s", text)
	}
}

func TestPreviewBuiltinEmitsCommandWithHeading(t *testing.T) {
	jar := filepath.Join(t.TempDir(), "app.jar")
	if err := os.WriteFile(jar, []byte("PK"), 0o644); err != nil {
		t.Fatal(err)
	}
	var out bytes.Buffer
	if err := runJavaPreview(context.Background(), BuiltinRequest{
		Args:   []string{"--jdk", "21", jar},
		Stdout: &out,
		Stderr: &bytes.Buffer{},
	}); err != nil {
		t.Fatalf("preview: %v", err)
	}
	text := out.String()
	for _, want := range []string{"将要启动的 java 命令:", "java -server", "-jar " + jar, "资源:"} {
		if !strings.Contains(text, want) {
			t.Errorf("preview missing %q:\n%s", want, text)
		}
	}
}
