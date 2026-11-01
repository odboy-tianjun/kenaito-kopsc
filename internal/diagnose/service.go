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
	program   string // docker
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
		base := fmt.Errorf("未找到目标 %q：%v；Kubernetes 命名空间 %s 中也没有该 Pod", target, dockerErr, ns)
		return source{}, s.withCandidates(ctx, base, target, opts)
	}

	base := fmt.Errorf("未找到目标 %q：%v；同时未安装 kubectl", target, dockerErr)
	return source{}, s.withCandidates(ctx, base, target, opts)
}

// withCandidates appends similar names to a "target not found" error, so the
// user does not have to run docker ps / kubectl get pods to find out that the
// container is called web-1.
//
// When nothing is similar, the few objects that do exist are listed instead —
// knowing that the only containers are "redis" and "db" is usually enough to
// spot the typo. With no objects at all the error keeps its original text.
func (s *Service) withCandidates(ctx context.Context, base error, target string, opts Options) error {
	all := s.candidateNames(ctx, target, opts)

	var hits, existing []string
	for _, name := range all {
		if name.matched {
			hits = append(hits, name.name)
		}
		existing = append(existing, name.name)
	}
	if len(hits) > 0 {
		return fmt.Errorf("%w\n你可能要找的是: %s\n（设置 --kind docker 或 --kind k8s 可缩小查找范围）",
			base, strings.Join(hits, ", "))
	}
	if len(existing) == 0 {
		return base
	}
	return fmt.Errorf("%w\n当前存在的对象: %s\n（可用 kopsc docker psall 或 kubectl get pods 查看详情）",
		base, strings.Join(existing, ", "))
}

// nameCandidate is one existing name and whether it resembles the target.
type nameCandidate struct {
	name    string
	matched bool
}

// candidateNames collects the names of existing Docker containers and Pods,
// marking those that resemble the target (prefix or substring, ignoring case)
// and ordering the similar ones first.
func (s *Service) candidateNames(ctx context.Context, target string, opts Options) []nameCandidate {
	if opts.Kind == "k8s" {
		return rankNames(s.podNames(ctx, opts), target)
	}

	var names []string
	if program, err := s.dockerProgram(); err == nil {
		if res, err := s.run(ctx, program, "ps", "-a", "--format", "{{.Names}}"); err == nil {
			names = append(names, strings.Fields(string(res.Stdout))...)
		}
	}
	if opts.Kind != "docker" {
		names = append(names, s.podNames(ctx, opts)...)
	}
	return rankNames(names, target)
}

// podNames lists the Pods in the effective namespace.
func (s *Service) podNames(ctx context.Context, opts Options) []string {
	if _, err := s.runner.LookPath("kubectl"); err != nil {
		return nil
	}
	ns, _ := s.kubeNamespace(ctx, opts.Context)
	res, err := s.run(ctx, "kubectl", kubectlArgs(opts.Context, "get", "pods", "-n", ns, "-o", "name")...)
	if err != nil {
		return nil
	}
	var names []string
	for _, field := range strings.Fields(string(res.Stdout)) {
		names = append(names, strings.TrimPrefix(field, "pod/"))
	}
	return names
}

// rankNames marks the names that resemble target — prefix hits before
// substring hits, comparison ignoring case — and returns at most maxNames of
// them, followed by the remaining names so callers can also show what exists
// when nothing is similar.
func rankNames(names []string, target string) []nameCandidate {
	const maxNames = 5
	lower := strings.ToLower(target)

	var prefixHits, containsHits, others []nameCandidate
	for _, name := range names {
		n := strings.ToLower(name)
		switch {
		case strings.HasPrefix(n, lower), strings.HasPrefix(lower, n):
			prefixHits = append(prefixHits, nameCandidate{name: name, matched: true})
		case strings.Contains(n, lower):
			containsHits = append(containsHits, nameCandidate{name: name, matched: true})
		default:
			others = append(others, nameCandidate{name: name})
		}
	}

	hits := append(prefixHits, containsHits...)
	if len(hits) > maxNames {
		hits = hits[:maxNames]
	}
	if len(others) > maxNames {
		others = others[:maxNames]
	}
	return append(hits, others...)
}

// matchNames is rankNames reduced to the similar names only.
func matchNames(names []string, target string) []string {
	var hits []string
	for _, c := range rankNames(names, target) {
		if c.matched {
			hits = append(hits, c.name)
		}
	}
	return hits
}

func (s *Service) dockerProgram() (string, error) {
	if _, err := s.runner.LookPath("docker"); err == nil {
		return "docker", nil
	}
	for _, name := range s.info.Runtimes {
		if name == "docker" {
			return "docker", nil
		}
	}
	return "", fmt.Errorf("未找到 docker")
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
