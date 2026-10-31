package diagnose

import (
	"fmt"
	"strconv"
	"strings"
)

// Options controls what a diagnosis collects.
type Options struct {
	Kind      string // "", "docker" or "k8s"
	Namespace string
	Context   string
	Lines     int
	NoLogs    bool
	NoEvents  bool
	Container string
}

// DefaultLines is the number of log lines collected by default.
const DefaultLines = 50

// ParseArgs parses the arguments forwarded by the CLI. It returns the target
// (container or Pod name) and the collection options.
func ParseArgs(args []string) (string, Options, error) {
	opts := Options{Lines: DefaultLines}
	var targets []string
	positionalOnly := false

	for i := 0; i < len(args); i++ {
		arg := args[i]
		switch {
		case positionalOnly:
			targets = append(targets, arg)

		case arg == "--":
			positionalOnly = true

		case arg == "--kind" || arg == "--kind=":
			value, err := valueFor(arg, args, &i, "--kind")
			if err != nil {
				return "", opts, err
			}
			opts.Kind = strings.ToLower(value)
			if opts.Kind != "docker" && opts.Kind != "k8s" {
				return "", opts, fmt.Errorf("--kind 只支持 docker 或 k8s，收到 %q", value)
			}

		case strings.HasPrefix(arg, "--kind="):
			opts.Kind = strings.ToLower(strings.TrimPrefix(arg, "--kind="))
			if opts.Kind != "docker" && opts.Kind != "k8s" {
				return "", opts, fmt.Errorf("--kind 只支持 docker 或 k8s，收到 %q", opts.Kind)
			}

		case arg == "--namespace" || arg == "-N":
			value, err := valueFor(arg, args, &i, arg)
			if err != nil {
				return "", opts, err
			}
			opts.Namespace = value

		case strings.HasPrefix(arg, "--namespace="):
			opts.Namespace = strings.TrimPrefix(arg, "--namespace=")

		case arg == "--context":
			value, err := valueFor(arg, args, &i, arg)
			if err != nil {
				return "", opts, err
			}
			opts.Context = value

		case strings.HasPrefix(arg, "--context="):
			opts.Context = strings.TrimPrefix(arg, "--context=")

		case arg == "--lines":
			value, err := valueFor(arg, args, &i, arg)
			if err != nil {
				return "", opts, err
			}
			n, err := strconv.Atoi(value)
			if err != nil || n <= 0 {
				return "", opts, fmt.Errorf("--lines 需要正整数，收到 %q", value)
			}
			opts.Lines = n

		case strings.HasPrefix(arg, "--lines="):
			n, err := strconv.Atoi(strings.TrimPrefix(arg, "--lines="))
			if err != nil || n <= 0 {
				return "", opts, fmt.Errorf("--lines 需要正整数，收到 %q", strings.TrimPrefix(arg, "--lines="))
			}
			opts.Lines = n

		case arg == "--no-logs":
			opts.NoLogs = true

		case arg == "--no-events":
			opts.NoEvents = true

		case arg == "--container" || arg == "-c":
			value, err := valueFor(arg, args, &i, arg)
			if err != nil {
				return "", opts, err
			}
			opts.Container = value

		case strings.HasPrefix(arg, "--container="):
			opts.Container = strings.TrimPrefix(arg, "--container=")

		case strings.HasPrefix(arg, "-") && arg != "-":
			return "", opts, fmt.Errorf("未知选项 %q", arg)

		default:
			targets = append(targets, arg)
		}
	}

	if len(targets) == 0 {
		return "", opts, fmt.Errorf("必须提供容器名或 Pod 名，例如 kopsc diagnose web")
	}
	if len(targets) > 1 {
		return "", opts, fmt.Errorf("一次只能诊断一个目标，收到 %d 个：%s", len(targets), strings.Join(targets, ", "))
	}

	if opts.Kind == "docker" && (opts.Namespace != "" || opts.Context != "") {
		return "", opts, fmt.Errorf("Docker 诊断不支持 --namespace/--context；请改用 --kind k8s")
	}

	return targets[0], opts, nil
}

// valueFor reads the value of a flag written as "--flag value" or "--flag=".
func valueFor(flag string, args []string, index *int, name string) (string, error) {
	if strings.HasSuffix(flag, "=") {
		return "", fmt.Errorf("%s 缺少参数", name)
	}
	if *index+1 >= len(args) {
		return "", fmt.Errorf("%s 缺少参数，例如 %s value", name, name)
	}
	*index = *index + 1
	return args[*index], nil
}
