package executor

import (
	"strings"
	"testing"
)

const gib = int64(1) << 30

func listFlags(flags []string) string { return strings.Join(flags, " ") }

func TestBuildJavaFlagsJDK21(t *testing.T) {
	flags := BuildJavaFlags(javaTuning{
		JDK:    "21",
		Memory: 8 * gib,
		Env:    "prod",
		GCDir:  "/boot/logs",
	})
	got := listFlags(flags)

	for _, want := range []string{
		"-server",
		"-Xmx4g", "-Xms4g", // 50% of 8 GiB
		"-Xss256k",
		"-XX:MaxDirectMemorySize=1g", // a quarter of the heap
		"-XX:+UseCompressedOops", "-XX:+UseCompressedClassPointers",
		"-XX:+SegmentedCodeCache",
		"-XX:+UseG1GC",
		"-XX:+ExplicitGCInvokesConcurrent",
		"-Xlog:gc*,safepoint:/boot/logs/gc.log:time,uptime:filecount=100,filesize=50M",
		"-Djava.security.egd=file:/dev/./urandom",
		"-Dconfig.file=application-prod.yaml",
	} {
		if !strings.Contains(got, want) {
			t.Errorf("JDK 21 flags missing %q:\n%s", want, got)
		}
	}
	// JDK 8-only spellings must not appear on modern JVMs.
	for _, unwanted := range []string{"-Xloggc:", "-XX:+PrintGCDetails", "-verbose:gc"} {
		if strings.Contains(got, unwanted) {
			t.Errorf("JDK 21 flags must not contain %q:\n%s", unwanted, got)
		}
	}
}

func TestBuildJavaFlagsJDK8UsesXloggc(t *testing.T) {
	flags := BuildJavaFlags(javaTuning{JDK: "8", Memory: 4 * gib, GCDir: "/var/log/app"})
	got := listFlags(flags)

	for _, want := range []string{
		"-Xmx2g", "-Xms2g",
		"-XX:+UseG1GC",
		"-verbose:gc",
		"-Xloggc:/var/log/app/gc.log",
		"-XX:+PrintGCDetails",
		"-XX:+PrintGCDateStamps",
		"-XX:+UseGCLogFileRotation",
		"-XX:NumberOfGCLogFiles=100",
		"-XX:GCLogFileSize=50M",
	} {
		if !strings.Contains(got, want) {
			t.Errorf("JDK 8 flags missing %q:\n%s", want, got)
		}
	}
	// The decisive compatibility rule: -Xlog: makes JDK 8 refuse to start.
	if strings.Contains(got, "-Xlog:") {
		t.Fatalf("JDK 8 must not receive -Xlog: (it cannot parse it):\n%s", got)
	}
}

func TestBuildJavaFlagsJDK11And17UseXLlog(t *testing.T) {
	for _, jdk := range []string{"11", "17"} {
		got := listFlags(BuildJavaFlags(javaTuning{JDK: jdk, Memory: 4 * gib, GCDir: "./logs"}))
		if !strings.Contains(got, "-Xlog:gc*,safepoint:./logs/gc.log:time,uptime:filecount=100,filesize=50M") {
			t.Errorf("JDK %s should use -Xlog:, got:\n%s", jdk, got)
		}
		if strings.Contains(got, "-Xloggc:") {
			t.Errorf("JDK %s must not use -Xloggc:\n%s", jdk, got)
		}
	}
}

func TestBuildJavaFlagsOrder(t *testing.T) {
	flags := BuildJavaFlags(javaTuning{JDK: "17", Memory: 2 * gib, GCDir: "/tmp"})
	// The command line must start with the executable-adjacent flags so it can
	// be pasted into a Dockerfile unchanged.
	if flags[0] != "-server" {
		t.Fatalf("first flag = %q, want -server", flags[0])
	}
	xmx, xms := indexOf(flags, "-Xmx1g"), indexOf(flags, "-Xms1g")
	if xmx < 0 || xms < 0 || xmx > xms {
		t.Errorf("-Xmx should come before -Xms (%d, %d): %v", xmx, xms, flags)
	}
	logIdx := indexOfPrefix(flags, "-Xlog:")
	propertyIdx := indexOfPrefix(flags, "-Dconfig.file=")
	if logIdx < 0 || propertyIdx < 0 || logIdx > propertyIdx {
		t.Errorf("-Dconfig.file should come after the GC flags: %v", flags)
	}
}

func indexOf(values []string, want string) int {
	for i, v := range values {
		if v == want {
			return i
		}
	}
	return -1
}

func indexOfPrefix(values []string, prefix string) int {
	for i, v := range values {
		if strings.HasPrefix(v, prefix) {
			return i
		}
	}
	return -1
}

func TestBuildJavaFlagsCommonTuning(t *testing.T) {
	got := listFlags(BuildJavaFlags(javaTuning{JDK: "17", Memory: 4 * gib, GCDir: "/tmp"}))
	for _, want := range []string{
		"-server",
		"-Xss256k",
		"-XX:MaxDirectMemorySize=512m", // a quarter of the 2g heap
		"-XX:+SegmentedCodeCache",
		"-XX:+ExplicitGCInvokesConcurrent",
		"-Djava.security.egd=file:/dev/./urandom",
	} {
		if !strings.Contains(got, want) {
			t.Errorf("common flag %q missing:\n%s", want, got)
		}
	}
}

func TestBuildJavaFlagsDirectMemoryMinimum(t *testing.T) {
	// A tiny heap must still leave 64 MiB for direct buffers (NIO).
	got := listFlags(BuildJavaFlags(javaTuning{JDK: "17", Memory: 128 << 20}))
	if !strings.Contains(got, "-XX:MaxDirectMemorySize=64m") {
		t.Fatalf("direct memory floor missing:\n%s", got)
	}
}

func TestBuildJavaFlagsHeapDefaultsToHalf(t *testing.T) {
	cases := []struct {
		memory  int64
		wantXmx string
		wantXms string
	}{
		{16 * gib, "-Xmx8g", "-Xms8g"},
		{1 * gib, "-Xmx512m", "-Xms512m"},
		{512 << 20, "-Xmx256m", "-Xms256m"},
	}
	for _, tc := range cases {
		flags := BuildJavaFlags(javaTuning{JDK: "21", Memory: tc.memory})
		if got := indexOf(flags, tc.wantXmx); got < 0 {
			t.Errorf("memory %d: want %s in %v", tc.memory, tc.wantXmx, flags)
		}
		if got := indexOf(flags, tc.wantXms); got < 0 {
			t.Errorf("memory %d: -Xms must equal -Xmx (%s), got %v", tc.memory, tc.wantXms, flags)
		}
	}
}

func TestBuildJavaFlagsHeapOverrides(t *testing.T) {
	// --heap / --xmx wins over the 50% default.
	flags := BuildJavaFlags(javaTuning{JDK: "17", Memory: 16 * gib, Heap: "4g"})
	if indexOf(flags, "-Xmx4g") < 0 || indexOf(flags, "-Xms4g") < 0 {
		t.Fatalf("explicit heap not applied: %v", flags)
	}
	if indexOf(flags, "-Xmx8g") >= 0 {
		t.Fatalf("default heap leaked alongside the override: %v", flags)
	}

	// --xms makes the initial heap differ, which is the only way to split them.
	flags = BuildJavaFlags(javaTuning{JDK: "17", Memory: 16 * gib, Heap: "4g", Initial: "1g"})
	if indexOf(flags, "-Xmx4g") < 0 || indexOf(flags, "-Xms1g") < 0 {
		t.Fatalf("initial heap override not applied: %v", flags)
	}
}

func TestBuildJavaFlagsNoMemoryMeansNoHeapFlags(t *testing.T) {
	flags := BuildJavaFlags(javaTuning{JDK: "17"})
	if indexOfPrefix(flags, "-Xmx") >= 0 || indexOfPrefix(flags, "-Xms") >= 0 {
		t.Fatalf("unknown memory must leave the heap to JVM ergonomics: %v", flags)
	}
}

func TestConfigFileResolution(t *testing.T) {
	// --env wins.
	if got := configFile("staging"); got != "application-staging.yaml" {
		t.Errorf("configFile(staging) = %q", got)
	}
	// APP_ENV is the fallback.
	t.Setenv("APP_ENV", "uat")
	if got := configFile(""); got != "application-uat.yaml" {
		t.Errorf("configFile(\"\") with APP_ENV=uat = %q", got)
	}
	// prod is the default when neither is set.
	t.Setenv("APP_ENV", "")
	if got := configFile(""); got != "application-prod.yaml" {
		t.Errorf("configFile(\"\") default = %q", got)
	}
}

func TestBuildJavaFlagsEnvFlagWinsOverAppEnv(t *testing.T) {
	t.Setenv("APP_ENV", "uat")
	flags := BuildJavaFlags(javaTuning{JDK: "17", Memory: 2 * gib, Env: "staging"})
	if indexOf(flags, "-Dconfig.file=application-staging.yaml") < 0 {
		t.Fatalf("--env must win over APP_ENV: %v", flags)
	}
}

func TestCompressedOopsDroppedForLargeHeaps(t *testing.T) {
	// Above 30g the JVM cannot use compressed oops, so kopsc stops emitting
	// the flags instead of leaving a warning in the log.
	big := BuildJavaFlags(javaTuning{JDK: "17", Heap: "32g"})
	for _, flag := range big {
		if flag == "-XX:+UseCompressedOops" || flag == "-XX:+UseCompressedClassPointers" {
			t.Fatalf("heap 32g must drop compressed oops flags, got: %v", big)
		}
	}

	// 30g is still inside the usable range.
	ok := BuildJavaFlags(javaTuning{JDK: "17", Heap: "30g"})
	if indexOf(ok, "-XX:+UseCompressedOops") < 0 {
		t.Fatalf("heap 30g should keep compressed oops: %v", ok)
	}
}

func TestCompressedOopsDroppedWhenDerivedHeapIsLarge(t *testing.T) {
	// 128 GiB of memory yields a 64g heap by default, which is also too large.
	flags := BuildJavaFlags(javaTuning{JDK: "21", Memory: 128 * gib})
	if indexOf(flags, "-XX:+UseCompressedOops") >= 0 {
		t.Fatalf("derived 64g heap must drop compressed oops: %v", flags)
	}
}
