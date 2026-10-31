package diagnose

import (
	"context"
	"encoding/json"
	"fmt"
	"strconv"
	"strings"
)

type pod struct {
	Metadata struct {
		Name      string `json:"name"`
		Namespace string `json:"namespace"`
	} `json:"metadata"`
	Status struct {
		Phase             string            `json:"phase"`
		Reason            string            `json:"reason"`
		Message           string            `json:"message"`
		Conditions        []podCondition    `json:"conditions"`
		ContainerStatuses []containerStatus `json:"containerStatuses"`
	} `json:"status"`
}

type podCondition struct {
	Type    string `json:"type"`
	Status  string `json:"status"`
	Reason  string `json:"reason"`
	Message string `json:"message"`
}

type containerStatus struct {
	Name         string   `json:"name"`
	Ready        bool     `json:"ready"`
	RestartCount int      `json:"restartCount"`
	State        podState `json:"state"`
	LastState    podState `json:"lastState"`
}

type podState struct {
	Waiting    *podWaiting    `json:"waiting"`
	Running    *podRunning    `json:"running"`
	Terminated *podTerminated `json:"terminated"`
}

type podWaiting struct {
	Reason  string `json:"reason"`
	Message string `json:"message"`
}

type podRunning struct {
	StartedAt string `json:"startedAt"`
}

type podTerminated struct {
	Reason     string `json:"reason"`
	ExitCode   int    `json:"exitCode"`
	Message    string `json:"message"`
	StartedAt  string `json:"startedAt"`
	FinishedAt string `json:"finishedAt"`
}

func (s *Service) diagnoseK8s(ctx context.Context, src source, target string, opts Options) error {
	if _, err := s.runner.LookPath("kubectl"); err != nil {
		return fmt.Errorf("未找到 kubectl，请先安装并配置 kubeconfig")
	}

	namespace := src.namespace
	if namespace == "" {
		namespace, _ = s.kubeNamespace(ctx, src.context)
	}

	res, err := s.run(ctx, "kubectl", kubectlArgs(src.context, "get", "pod", target, "-n", namespace, "-o", "json")...)
	if err != nil {
		return err
	}

	var p pod
	if err := json.Unmarshal(res.Stdout, &p); err != nil {
		return fmt.Errorf("解析 kubectl get pod 输出失败: %w", err)
	}

	name := p.Metadata.Name
	if name == "" {
		name = target
	}
	if p.Metadata.Namespace != "" {
		namespace = p.Metadata.Namespace
	}

	contextLabel := src.context
	if contextLabel == "" {
		contextLabel = "当前上下文"
	}

	ready, total, restarts := 0, len(p.Status.ContainerStatuses), 0
	for _, cs := range p.Status.ContainerStatuses {
		if cs.Ready {
			ready++
		}
		restarts += cs.RestartCount
	}

	fmt.Fprintf(s.out, "目标:   %s\n", name)
	fmt.Fprintf(s.out, "来源:   Kubernetes  context=%s  namespace=%s\n", contextLabel, namespace)
	fmt.Fprintf(s.out, "状态:   phase=%s  ready=%d/%d  restarts=%d\n", p.Status.Phase, ready, total, restarts)
	s.renderFinding("", judgePod(p))

	selected := p.Status.ContainerStatuses
	if opts.Container != "" {
		selected = nil
		for _, cs := range p.Status.ContainerStatuses {
			if cs.Name == opts.Container {
				selected = append(selected, cs)
			}
		}
		if len(selected) == 0 {
			fmt.Fprintf(s.errOut, "警告: Pod %s 中没有名为 %q 的容器\n", name, opts.Container)
		}
	}

	for _, cs := range selected {
		fmt.Fprintf(s.out, "\n容器 %s:\n", cs.Name)
		fmt.Fprintf(s.out, "  状态:   %s  ready=%t  restarts=%d\n", containerState(cs), cs.Ready, cs.RestartCount)
		s.renderFinding("  ", judgeContainer(cs))
		if !opts.NoLogs {
			s.writeContainerLogs(ctx, src, name, namespace, cs, opts)
		}
	}

	if !opts.NoEvents {
		eventRes, err := s.run(ctx, "kubectl", kubectlArgs(src.context,
			"get", "events", "-n", namespace,
			"--field-selector", "involvedObject.name="+name,
			"--sort-by=.lastTimestamp")...)
		if err != nil {
			fmt.Fprintf(s.errOut, "警告: 获取事件失败: %v\n", err)
		} else {
			events := tailLines(string(eventRes.Stdout), 20)
			s.writeSection(fmt.Sprintf("最近事件 (namespace=%s)", namespace), events)
			if hint := eventHint(events); hint != "" {
				fmt.Fprintf(s.out, "\n事件提示: %s\n", hint)
			}
		}
	}

	return nil
}

// judgePod reports Pod-level health; per-container verdicts follow separately.
func judgePod(p pod) Finding {
	evidence := "status.phase=" + p.Status.Phase
	if p.Status.Reason != "" {
		evidence += ", reason=" + p.Status.Reason
	}

	switch p.Status.Phase {
	case "Pending":
		return Finding{
			Severity: "warn",
			Verdict:  "Pod 处于 Pending：尚未成功调度或容器还未创建",
			Evidence: evidence,
			Advice:   "查看下方事件：常见原因是资源不足、亲和性/污点不满足、PVC 未绑定或镜像拉取慢",
		}
	case "Failed":
		return Finding{
			Severity: "error",
			Verdict:  "Pod 已失败",
			Evidence: evidence,
			Advice:   "结合下方容器状态、上一次日志与事件定位失败原因",
		}
	case "Succeeded":
		return Finding{
			Severity: "ok",
			Verdict:  "Pod 已正常完成",
			Evidence: evidence,
		}
	case "Running":
		return Finding{
			Severity: "ok",
			Verdict:  "Pod 已调度并在运行（容器级状态见下）",
			Evidence: evidence,
		}
	default:
		return Finding{
			Severity: "warn",
			Verdict:  "Pod 状态未识别：" + p.Status.Phase,
			Evidence: evidence,
		}
	}
}

// judgeContainer maps containerStatus to a root cause.
func judgeContainer(cs containerStatus) Finding {
	if cs.State.Waiting != nil {
		w := cs.State.Waiting
		evidence := "state.waiting.reason=" + w.Reason
		if w.Message != "" {
			evidence += ", message=" + truncate(w.Message, 200)
		}

		switch w.Reason {
		case "CrashLoopBackOff":
			if cs.LastState.Terminated != nil {
				t := cs.LastState.Terminated
				evidence += fmt.Sprintf(", lastState.reason=%s, lastState.exitCode=%d", t.Reason, t.ExitCode)
			}
			return Finding{
				Severity: "error",
				Verdict:  "容器反复崩溃并重启（CrashLoopBackOff）",
				Evidence: evidence,
				Advice:   "查看下方“上一次”日志与退出码；检查启动命令、依赖服务与资源限制",
			}
		case "ImagePullBackOff", "ErrImagePull":
			return Finding{
				Severity: "error",
				Verdict:  "镜像拉取失败（" + w.Reason + "）",
				Evidence: evidence,
				Advice:   "检查镜像名与 tag、imagePullSecrets、仓库可达性与节点网络",
			}
		case "CreateContainerConfigError":
			return Finding{
				Severity: "error",
				Verdict:  "容器配置错误：ConfigMap/Secret 缺失或键不存在",
				Evidence: evidence,
				Advice:   "核对该 Pod 引用的 ConfigMap/Secret 是否存在，键名是否正确",
			}
		case "CreateContainerError", "RunContainerError":
			return Finding{
				Severity: "error",
				Verdict:  "容器创建/启动失败（" + w.Reason + "）",
				Evidence: evidence,
				Advice:   "检查容器运行时与事件中的具体错误（挂载、cgroup、权限）",
			}
		case "ContainerCreating":
			return Finding{
				Severity: "warn",
				Verdict:  "容器正在创建",
				Evidence: evidence,
				Advice:   "通常会自动完成；若长时间不变，查看事件中的挂载/镜像错误",
			}
		case "PodInitializing":
			return Finding{
				Severity: "warn",
				Verdict:  "初始化容器尚未完成",
				Evidence: evidence,
				Advice:   "查看 initContainer 的日志与状态",
			}
		default:
			return Finding{
				Severity: "warn",
				Verdict:  "容器处于等待状态：" + w.Reason,
				Evidence: evidence,
			}
		}
	}

	if cs.State.Terminated != nil {
		t := cs.State.Terminated
		evidence := fmt.Sprintf("state.terminated.reason=%s, exitCode=%d", t.Reason, t.ExitCode)
		if t.Message != "" {
			evidence += ", message=" + truncate(t.Message, 200)
		}

		switch t.Reason {
		case "OOMKilled":
			return Finding{
				Severity: "error",
				Verdict:  "容器因内存超限被 OOMKilled",
				Evidence: evidence,
				Advice:   "提高 resources.limits.memory，或降低应用内存占用",
			}
		case "Completed":
			return Finding{
				Severity: "ok",
				Verdict:  "容器已正常结束（Completed）",
				Evidence: evidence,
			}
		case "Error":
			return Finding{
				Severity: "error",
				Verdict:  "容器以非零状态退出（Error）",
				Evidence: evidence,
				Advice:   "查看下方日志末尾定位应用错误",
			}
		default:
			return Finding{
				Severity: "warn",
				Verdict:  "容器已终止：" + t.Reason,
				Evidence: evidence,
			}
		}
	}

	if cs.State.Running != nil {
		evidence := "state.running"
		if cs.LastState.Terminated != nil {
			t := cs.LastState.Terminated
			evidence += fmt.Sprintf(", lastState.reason=%s, lastState.exitCode=%d", t.Reason, t.ExitCode)
		}
		if !cs.Ready {
			return Finding{
				Severity: "warn",
				Verdict:  "容器运行中但未就绪（就绪探针未通过）",
				Evidence: evidence + ", ready=false",
				Advice:   "检查 readinessProbe 配置、启动时间与依赖服务",
			}
		}
		return Finding{
			Severity: "ok",
			Verdict:  "容器运行中且已就绪",
			Evidence: evidence + ", ready=true",
		}
	}

	return Finding{
		Severity: "warn",
		Verdict:  "容器状态未知（state 为空）",
		Evidence: "state 为空",
		Advice:   "查看 kubectl describe pod 与事件",
	}
}

func containerState(cs containerStatus) string {
	switch {
	case cs.State.Waiting != nil:
		return "waiting(" + cs.State.Waiting.Reason + ")"
	case cs.State.Terminated != nil:
		return "terminated(" + cs.State.Terminated.Reason + ")"
	case cs.State.Running != nil:
		return "running"
	default:
		return "unknown"
	}
}

func (s *Service) writeContainerLogs(ctx context.Context, src source, podName, namespace string, cs containerStatus, opts Options) {
	base := []string{"logs", podName, "-n", namespace, "-c", cs.Name, "--tail", strconv.Itoa(opts.Lines)}

	res, err := s.run(ctx, "kubectl", kubectlArgs(src.context, base...)...)
	if err != nil {
		fmt.Fprintf(s.errOut, "警告: 获取容器 %s 日志失败: %v\n", cs.Name, err)
	} else {
		s.writeSection(fmt.Sprintf("日志 (%s, 最近 %d 行)", cs.Name, opts.Lines), string(res.Stdout))
	}

	if cs.LastState.Terminated != nil {
		previous := append(append([]string{}, base...), "--previous")
		res, err := s.run(ctx, "kubectl", kubectlArgs(src.context, previous...)...)
		if err != nil {
			fmt.Fprintf(s.errOut, "警告: 获取容器 %s 上一次日志失败: %v\n", cs.Name, err)
		} else {
			s.writeSection(fmt.Sprintf("日志 (%s, 上一次)", cs.Name), string(res.Stdout))
		}
	}
}

// eventHint turns common event fragments into an actionable hint.
func eventHint(events string) string {
	lower := strings.ToLower(events)
	switch {
	case strings.Contains(lower, "failedscheduling"), strings.Contains(lower, "insufficient"):
		return "事件显示调度失败：检查节点资源、亲和性/反亲和性、污点与 PVC 绑定"
	case strings.Contains(lower, "failedmount"), strings.Contains(lower, "failedattachvolume"):
		return "事件显示存储挂载失败：检查 PVC/PV、StorageClass 与 ConfigMap/Secret 挂载"
	case strings.Contains(lower, "unhealthy"):
		return "事件显示探针失败：检查 liveness/readiness 探针路径、端口与启动时间"
	case strings.Contains(lower, "backoff"):
		return "事件显示退避重试：结合容器退出码与日志确认根因"
	}
	return ""
}
