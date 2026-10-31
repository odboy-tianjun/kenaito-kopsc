package diagnose

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"strings"
	"time"

	"gitea.odboy.cn/odboy/kenaito-kopsc/internal/platform"
)

// Finding is one judgement about a container.
type Finding struct {
	Severity string // "ok", "warn" or "error"
	Verdict  string
	Evidence string
	Advice   string
}

// Service collects and renders diagnoses.
type Service struct {
	info   platform.Info
	runner Runner
	out    io.Writer
	errOut io.Writer
	now    func() time.Time
}

// New builds a Service. runner is usually ExecRunner; tests inject a fake.
func New(info platform.Info, runner Runner, out, errOut io.Writer) *Service {
	if runner == nil {
		runner = ExecRunner{}
	}
	if out == nil {
		out = io.Discard
	}
	if errOut == nil {
		errOut = io.Discard
	}
	return &Service{info: info, runner: runner, out: out, errOut: errOut, now: time.Now}
}

// Run parses the arguments forwarded by kopsc and diagnoses the target.
func (s *Service) Run(ctx context.Context, args []string) error {
	target, opts, err := ParseArgs(args)
	if err != nil {
		return err
	}
	return s.diagnose(ctx, target, opts)
}

type source struct {
	kind      string // "docker" or "k8s"
	program   string // docker or podman
	namespace string
	context   string
}

func (s *Service) diagnose(ctx context.Context, target string, opts Options) error {
	src, err := s.resolve(ctx, target, opts)
	if err != nil {
		return err
	}
	switch src.kind {
	case "docker":
		return s.diagnoseDocker(ctx, src, target, opts)
	case "k8s":
		return s.diagnoseK8s(ctx, src, target, opts)
	default:
		return fmt.Errorf("未知诊断来源 %q", src.kind)
	}
}

// resolve decides whether the target is a container or a Pod.
func (s *Service) resolve(ctx context.Context, target string, opts Options) (source, error) {
	switch opts.Kind {
	case "docker":
		program, err := s.dockerProgram()
		if err != nil {
			return source{}, err
		}
		return source{kind: "docker", program: program}, nil

	case "k8s":
		ns := opts.Namespace
		if ns == "" {
			ns, _ = s.kubeNamespace(ctx, opts.Context)
		}
		return source{kind: "k8s", namespace: ns, context: opts.Context}, nil
	}

	// Providing namespace/context is an explicit signal for Kubernetes.
	if opts.Namespace != "" || opts.Context != "" {
		ns := opts.Namespace
		if ns == "" {
			ns, _ = s.kubeNamespace(ctx, opts.Context)
		}
		return source{kind: "k8s", namespace: ns, context: opts.Context}, nil
	}

	var dockerErr error
	if program, err := s.dockerProgram(); err == nil {
		if res, err := s.run(ctx, program, "inspect", "--type", "container", target); err == nil {
			var items []json.RawMessage
			if json.Unmarshal(res.Stdout, &items) == nil && len(items) > 0 {
				return source{kind: "docker", program: program}, nil
			}
			dockerErr = fmt.Errorf("Docker 中没有名为 %q 的容器", target)
		} else {
			dockerErr = err
		}
	} else {
		dockerErr = err
	}

	if _, err := s.runner.LookPath("kubectl"); err == nil {
		ns, _ := s.kubeNamespace(ctx, opts.Context)
		if res, err := s.run(ctx, "kubectl", kubectlArgs(opts.Context, "get", "pod", target, "-n", ns, "-o", "name")...); err == nil &&
			strings.TrimSpace(string(res.Stdout)) != "" {
			return source{kind: "k8s", namespace: ns, context: opts.Context}, nil
		}
		return source{}, fmt.Errorf("未找到目标 %q：%v；Kubernetes 命名空间 %s 中也没有该 Pod", target, dockerErr, ns)
	}

	return source{}, fmt.Errorf("未找到目标 %q：%v；同时未安装 kubectl", target, dockerErr)
}

func (s *Service) dockerProgram() (string, error) {
	for _, name := range []string{"docker", "podman"} {
		if _, err := s.runner.LookPath(name); err == nil {
			return name, nil
		}
	}
	for _, name := range s.info.Runtimes {
		if name == "docker" || name == "podman" {
			return name, nil
		}
	}
	return "", fmt.Errorf("未找到 docker 或 podman")
}

// run executes a command with a per-command timeout.
func (s *Service) run(ctx context.Context, name string, args ...string) (CommandResult, error) {
	callCtx, cancel := context.WithTimeout(ctx, 10*time.Second)
	defer cancel()

	res, err := s.runner.Run(callCtx, name, args...)
	if err != nil {
		message := summarizeStderr(string(res.Stderr))
		if message == "" {
			message = err.Error()
		}
		return res, fmt.Errorf("%s %s 失败: %s", name, strings.Join(args, " "), message)
	}
	return res, nil
}

// summarizeStderr keeps the actionable last line of a multi-line error (kubectl
// prints its connection error last) and trims the rest.
func summarizeStderr(raw string) string {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return ""
	}
	lines := strings.Split(raw, "\n")
	return truncate(strings.TrimSpace(lines[len(lines)-1]), 300)
}

func kubectlArgs(context string, args ...string) []string {
	if context == "" {
		return args
	}
	out := make([]string, 0, 2+len(args))
	out = append(out, "--context", context)
	return append(out, args...)
}

// kubeNamespace resolves the effective namespace: --namespace, then the
// context's kubeconfig namespace, then "default".
func (s *Service) kubeNamespace(ctx context.Context, context string) (string, error) {
	res, err := s.run(ctx, "kubectl", kubectlArgs(context, "config", "view", "--minify", "-o", "jsonpath={..namespace}")...)
	if err != nil {
		return "default", err
	}
	ns := strings.TrimSpace(string(res.Stdout))
	if ns == "" {
		return "default", nil
	}
	return ns, nil
}

func (s *Service) renderFinding(indent string, f Finding) {
	icon := "✅"
	switch f.Severity {
	case "error":
		icon = "❌"
	case "warn":
		icon = "⚠️"
	}
	fmt.Fprintf(s.out, "%s判定:   %s %s\n", indent, icon, f.Verdict)
	if f.Evidence != "" {
		fmt.Fprintf(s.out, "%s依据:   %s\n", indent, f.Evidence)
	}
	if f.Advice != "" {
		fmt.Fprintf(s.out, "%s建议:   %s\n", indent, f.Advice)
	}
}

func (s *Service) writeSection(title, content string) {
	fmt.Fprintf(s.out, "\n%s:\n", title)
	if strings.TrimSpace(content) == "" {
		fmt.Fprintln(s.out, "(无)")
		return
	}
	fmt.Fprintln(s.out, content)
}

func tailLines(text string, n int) string {
	text = strings.TrimRight(text, "\n")
	if strings.TrimSpace(text) == "" {
		return ""
	}
	lines := strings.Split(text, "\n")
	if n > 0 && len(lines) > n {
		lines = lines[len(lines)-n:]
	}
	return strings.Join(lines, "\n")
}
