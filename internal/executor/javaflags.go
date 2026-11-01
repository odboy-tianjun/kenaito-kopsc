package executor

import (
	"fmt"
	"os"
	"strconv"
	"strings"
)

// javaTuning is everything BuildJavaFlags needs to describe one JVM launch.
type javaTuning struct {
	JDK      string // normalised release: "8", "11", "17", "21"
	CPUs     int    // usable CPUs, 0 when unknown
	Memory   int64  // usable memory in bytes, 0 when unknown
	Heap     string // -Xmx value; empty means "compute from Memory"
	Initial  string // -Xms value; empty means "same as -Xmx"
	Env      string // used for -Dconfig.file=application-<Env>.yaml
	GCDir    string // directory for GC logs; empty means no GC log
	HeapDump bool   // add -XX:+HeapDumpOnOutOfMemoryError
}

// javaHeapRatio is the fraction of available memory used for the heap when the
// user does not pick a size: half leaves room for metaspace, thread stacks,
// direct buffers and the OS page cache.
const javaHeapRatioPercent = 50

// compressedOopsLimit is the largest heap for which compressed oops help; JVM
// warns and effectively disables them beyond this, so kopsc stops emitting the
// flags at 30 GiB (the commonly recommended ceiling, below the 32 GiB limit).
const compressedOopsLimit = int64(30) << 30

// BuildJavaFlags renders the ordered java arguments for one launch.
//
// The order mirrors a hand-written command line so `--print-flags` output can
// be pasted into a Dockerfile or systemd unit unchanged: JVM ergonomics first,
// then heap, then GC and logging, then application properties.
func BuildJavaFlags(t javaTuning) []string {
	flags := []string{"-server"}

	// Heap: -Xms and -Xmx are equal to keep the heap from resizing at runtime.
	heapBytes, heapText := resolveHeap(t)
	initialText := t.Initial
	if initialText == "" {
		initialText = heapText
	}
	if heapText != "" {
		flags = append(flags, "-Xmx"+heapText)
	}
	if initialText != "" {
		flags = append(flags, "-Xms"+initialText)
	}

	flags = append(flags,
		"-Xss256k",
		"-XX:MaxDirectMemorySize="+directMemory(heapBytes),
	)

	// Compressed oops only make sense while the heap fits under the limit.
	if heapBytes <= 0 || heapBytes <= compressedOopsLimit {
		flags = append(flags, "-XX:+UseCompressedOops", "-XX:+UseCompressedClassPointers")
	}

	flags = append(flags,
		"-XX:+SegmentedCodeCache",
		"-XX:+UseG1GC",
		"-XX:+ExplicitGCInvokesConcurrent",
	)

	if jdkMajor(t.JDK) >= 9 {
		flags = append(flags, "-XX:+PrintCommandLineFlags")
		if t.GCDir != "" {
			flags = append(flags, "-Xlog:gc*,safepoint:"+gcLogPath(t.GCDir)+":time,uptime:filecount=100,filesize=50M")
		}
	} else {
		// JDK 8 has no -Xlog:; it uses -Xloggc plus its own rotation options.
		// Emitting -Xlog: on JDK 8 makes the JVM refuse to start.
		flags = append(flags, "-verbose:gc", "-XX:+PrintCommandLineFlags", "-XX:+PrintGCDetails", "-XX:+PrintGCDateStamps")
		if t.GCDir != "" {
			flags = append(flags,
				"-Xloggc:"+gcLogPath(t.GCDir),
				"-XX:+UseGCLogFileRotation",
				"-XX:NumberOfGCLogFiles=100",
				"-XX:GCLogFileSize=50M",
			)
		}
	}

	flags = append(flags, "-Djava.security.egd=file:/dev/./urandom")

	if t.HeapDump {
		flags = append(flags, "-XX:+HeapDumpOnOutOfMemoryError")
	}

	flags = append(flags, "-Dconfig.file="+configFile(t.Env))

	return flags
}

// configFile renders the Spring-style config property: --env wins, then the
// APP_ENV environment variable, then "prod".
func configFile(env string) string {
	if env == "" {
		env = os.Getenv("APP_ENV")
	}
	if env == "" {
		env = "prod"
	}
	return "application-" + env + ".yaml"
}

// resolveHeap decides the heap size: an explicit value wins, otherwise half of
// the available memory.
func resolveHeap(t javaTuning) (int64, string) {
	if t.Heap != "" {
		return parseJVMSize(t.Heap), t.Heap
	}
	if t.Memory <= 0 {
		return 0, ""
	}
	// Multiply first: dividing by 100 up front truncates, turning 8 GiB into
	// 3.5g because the remainder is lost before the ratio is applied.
	bytes := t.Memory * javaHeapRatioPercent / 100
	return bytes, formatJVMSize(bytes)
}

// directMemory returns MaxDirectMemorySize as a quarter of the heap (the JVM's
// own default), at least 64 MiB so small heaps still work with NIO.
func directMemory(heapBytes int64) string {
	if heapBytes <= 0 {
		return "256m"
	}
	quarter := heapBytes / 4
	if quarter < 64<<20 {
		quarter = 64 << 20
	}
	return formatJVMSize(quarter)
}

// gcLogPath joins the GC directory and the standard file name.
func gcLogPath(dir string) string {
	return strings.TrimRight(dir, `/\`) + "/gc.log"
}

// jdkMajor parses the normalised release into a number; "8" when unknown so
// the conservative JDK 8 flag set is used.
func jdkMajor(jdk string) int {
	n, err := strconv.Atoi(strings.TrimSpace(jdk))
	if err != nil {
		return 8
	}
	return n
}

// parseJVMSize converts a user-supplied size ("2g", "512m", "1024k", "1073741824")
// into bytes. An unparsable value returns 0.
func parseJVMSize(value string) int64 {
	value = strings.TrimSpace(strings.ToLower(value))
	if value == "" {
		return 0
	}
	multiplier := int64(1)
	switch value[len(value)-1] {
	case 'k':
		multiplier, value = 1<<10, value[:len(value)-1]
	case 'm':
		multiplier, value = 1<<20, value[:len(value)-1]
	case 'g':
		multiplier, value = 1<<30, value[:len(value)-1]
	case 't':
		multiplier, value = 1<<40, value[:len(value)-1]
	}
	n, err := strconv.ParseInt(strings.TrimSpace(value), 10, 64)
	if err != nil || n <= 0 {
		return 0
	}
	return n * multiplier
}

// formatJVMSize renders bytes the way -Xmx expects: "512m", "2g". Sizes are
// rounded down to whole mebibytes (or gibibytes when large enough) so the
// printed flag stays short.
func formatJVMSize(bytes int64) string {
	if bytes <= 0 {
		return ""
	}
	const (
		mib = int64(1) << 20
		gib = int64(1) << 30
	)
	if bytes >= gib {
		if bytes%gib == 0 {
			return fmt.Sprintf("%dg", bytes/gib)
		}
		// Round down to 0.5g so "6.4g" becomes "6.5g"-free output (6.4g),
		// keeping the flag short while never asking for more than available.
		halfG := int64(1) << 29
		halves := bytes / halfG
		if halves%2 == 0 {
			return fmt.Sprintf("%dg", halves/2)
		}
		return fmt.Sprintf("%.1fg", float64(halves)/2)
	}
	return fmt.Sprintf("%dm", bytes/mib)
}
