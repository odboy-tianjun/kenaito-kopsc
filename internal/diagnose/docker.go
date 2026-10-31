package diagnose

import (
	"context"
	"encoding/json"
	"fmt"
	"strconv"
	"strings"
	"time"
)

type dockerHealth struct {
	Status string `json:"Status"`
}

type dockerContainer struct {
	ID           string `json:"Id"`
	Name         string `json:"Name"`
	RestartCount int    `json:"RestartCount"`
	Config       struct {
		Image string `json:"Image"`
	} `json:"Config"`
	State struct {
		Status     string        `json:"Status"`
		Running    bool          `json:"Running"`
		Paused     bool          `json:"Paused"`
		Restarting bool          `json:"Restarting"`
		OOMKilled  bool          `json:"OOMKilled"`
		Dead       bool          `json:"Dead"`
		ExitCode   int           `json:"ExitCode"`
		Error      string        `json:"Error"`
		StartedAt  string        `json:"StartedAt"`
		FinishedAt string        `json:"FinishedAt"`
		Health     *dockerHealth `json:"Health"`
	} `json:"State"`
}

func (s *Service) diagnoseDocker(ctx context.Context, src source, target string, opts Options) error {
	res, err := s.run(ctx, src.program, "inspect", "--type", "container", target)
	if err != nil {
		return err
	}

	var containers []dockerContainer
	if err := json.Unmarshal(res.Stdout, &containers); err != nil {
		return fmt.Errorf("解析 %s inspect 输出失败: %w", src.program, err)
	}
	if len(containers) == 0 {
		return fmt.Errorf("%s 中没有名为 %q 的容器", src.program, target)
	}

	c := containers[0]
	name := strings.TrimPrefix(c.Name, "/")
	if name == "" {
		name = target
	}

	fmt.Fprintf(s.out, "目标:   %s\n", name)
	fmt.Fprintf(s.out, "来源:   Docker (%s)\n", src.program)
	fmt.Fprintf(s.out, "状态:   %s  ExitCode=%d  OOMKilled=%t  RestartCount=%d\n",
		c.State.Status, c.State.ExitCode, c.State.OOMKilled, c.RestartCount)
	if c.Config.Image != "" {
		fmt.Fprintf(s.out, "镜像:   %s\n", c.Config.Image)
	}
	if c.State.Health != nil && c.State.Health.Status != "" {
		fmt.Fprintf(s.out, "健康:   %s\n", c.State.Health.Status)
	}
	if c.State.StartedAt != "" && !strings.HasPrefix(c.State.StartedAt, "0001-") {
		fmt.Fprintf(s.out, "时间:   %s", c.State.StartedAt)
		if c.State.FinishedAt != "" && !strings.HasPrefix(c.State.FinishedAt, "0001-") && c.State.FinishedAt != c.State.StartedAt {
			fmt.Fprintf(s.out, " → %s", c.State.FinishedAt)
		}
		fmt.Fprintln(s.out)
	}
	s.renderFinding("", judgeDocker(c))

	if !opts.NoLogs {
		logRes, err := s.run(ctx, src.program, "logs", "--tail", strconv.Itoa(opts.Lines), "--timestamps", target)
		if err != nil {
			fmt.Fprintf(s.errOut, "警告: 获取日志失败: %v\n", err)
		} else {
			s.writeSection(fmt.Sprintf("最近日志 (%d 行)", opts.Lines), string(logRes.Stdout))
		}
	}

	if !opts.NoEvents {
		now := s.now()
		since := now.Add(-time.Hour).UTC().Format(time.RFC3339)
		until := now.UTC().Format(time.RFC3339)
		eventRes, err := s.run(ctx, src.program, "events",
			"--since", since, "--until", until, "--filter", "container="+name)
		if err != nil {
			fmt.Fprintf(s.errOut, "警告: 获取事件失败: %v\n", err)
		} else {
			s.writeSection("最近事件 (1h)", tailLines(string(eventRes.Stdout), 20))
		}
	}

	return nil
}

// judgeDocker maps container state to a human-readable root cause.
func judgeDocker(c dockerContainer) Finding {
	st := c.State
	evidence := "State.Status=" + st.Status
	if st.Status == "exited" || st.ExitCode != 0 {
		evidence += fmt.Sprintf(", ExitCode=%d", st.ExitCode)
	}
	if st.OOMKilled {
		evidence += ", OOMKilled=true"
	}
	if c.RestartCount > 0 {
		evidence += fmt.Sprintf(", RestartCount=%d", c.RestartCount)
	}
	if st.Error != "" {
		evidence += ", Error=" + truncate(st.Error, 200)
	}

	switch {
	case st.Restarting || st.Status == "restarting":
		return Finding{
			Severity: "error",
			Verdict:  "容器处于重启循环，正在反复启动",
			Evidence: evidence,
			Advice:   "查看最近日志与上一次退出码，确认应用是否启动即崩溃或依赖不可用",
		}

	case st.Status == "exited":
		switch {
		case st.OOMKilled:
			return Finding{
				Severity: "error",
				Verdict:  "容器因内存超限被 OOM kill",
				Evidence: evidence,
				Advice:   "提高容器的内存限制，或降低应用峰值内存；可用 kopsc docker stats 观察",
			}
		case st.ExitCode == 137:
			return Finding{
				Severity: "error",
				Verdict:  "容器收到 SIGKILL（137），可能被手动 kill 或 stop 超时后强杀",
				Evidence: evidence,
				Advice:   "若并非人为操作，检查宿主机 OOM 日志与 stop 超时设置",
			}
		case st.ExitCode == 143:
			return Finding{
				Severity: "warn",
				Verdict:  "容器收到 SIGTERM（143）后退出，通常是正常停止",
				Evidence: evidence,
				Advice:   "若预期继续运行，检查是谁发送了停止信号或重启策略",
			}
		case st.ExitCode == 126:
			return Finding{
				Severity: "error",
				Verdict:  "容器内命令不可执行（126）：权限不足或可执行格式错误",
				Evidence: evidence,
				Advice:   "检查 entrypoint/cmd 是否可执行、是否缺少可执行权限",
			}
		case st.ExitCode == 127:
			return Finding{
				Severity: "error",
				Verdict:  "容器内命令不存在（127）",
				Evidence: evidence,
				Advice:   "检查 entrypoint/cmd 与镜像内实际的二进制路径",
			}
		case st.ExitCode == 0:
			if c.RestartCount > 0 {
				return Finding{
					Severity: "warn",
					Verdict:  "容器正常退出，但已多次重启，重启策略可能导致循环",
					Evidence: evidence,
					Advice:   "若是任务型容器，确认重启策略应为 no/on-failure 而不是 always",
				}
			}
			return Finding{
				Severity: "ok",
				Verdict:  "容器已正常退出（ExitCode=0）",
				Evidence: evidence,
			}
		default:
			return Finding{
				Severity: "error",
				Verdict:  "容器以非零状态退出，应用自身报错",
				Evidence: evidence,
				Advice:   "查看下方日志的末尾，定位应用错误；确认命令与配置参数",
			}
		}

	case st.Status == "running":
		if st.Paused {
			return Finding{
				Severity: "warn",
				Verdict:  "容器处于暂停（paused）状态",
				Evidence: evidence,
				Advice:   "确认是否有人执行了 docker pause；唤醒使用 docker unpause",
			}
		}
		if st.Health != nil && strings.EqualFold(st.Health.Status, "unhealthy") {
			return Finding{
				Severity: "error",
				Verdict:  "容器健康检查失败（unhealthy）",
				Evidence: evidence + ", State.Health.Status=unhealthy",
				Advice:   "查看日志与 healthcheck 命令；确认依赖服务可用、检查间隔与超时合理",
			}
		}
		return Finding{
			Severity: "ok",
			Verdict:  "容器正在运行",
			Evidence: evidence,
		}

	case st.Status == "created":
		return Finding{
			Severity: "warn",
			Verdict:  "容器已创建但尚未启动",
			Evidence: evidence,
			Advice:   "使用 kopsc docker start <容器> 启动；若启动失败查看 docker events",
		}

	case st.Status == "paused":
		return Finding{
			Severity: "warn",
			Verdict:  "容器已暂停",
			Evidence: evidence,
		}

	case st.Status == "dead":
		return Finding{
			Severity: "error",
			Verdict:  "容器处于 dead 状态，运行时未能正常清理",
			Evidence: evidence,
			Advice:   "通常需要删除后重新创建容器；检查存储驱动与宿主资源",
		}
	}

	return Finding{
		Severity: "warn",
		Verdict:  "容器状态未识别：" + st.Status,
		Evidence: evidence,
	}
}

func truncate(s string, max int) string {
	s = strings.ReplaceAll(strings.TrimSpace(s), "\n", " ")
	if len(s) <= max {
		return s
	}
	return s[:max] + "…"
}
