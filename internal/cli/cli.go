// Package cli builds the kopsc command tree from the registry.
package cli

import (
	"context"
	"fmt"
	"io"
	"os"
	"strings"
	"text/tabwriter"

	"github.com/spf13/cobra"

	"gitea.odboy.cn/odboy/kenaito-kopsc/internal/executor"
	"gitea.odboy.cn/odboy/kenaito-kopsc/internal/platform"
	"gitea.odboy.cn/odboy/kenaito-kopsc/internal/registry"
	"gitea.odboy.cn/odboy/kenaito-kopsc/internal/resolve"
)

// App wires a registry and a detected environment to a cobra command tree.
type App struct {
	Registry *registry.Registry
	Info     platform.Info
	Version  string

	Stdin  io.Reader
	Stdout io.Writer
	Stderr io.Writer

	// exitCode is set by commands that intentionally exit non-zero without an
	// error (for example a destructive command awaiting --yes).
	exitCode int
}

// New returns an App for the running process.
func New(version string) *App {
	return &App{
		Registry: registry.Builtin(),
		Info:     platform.System().Detect(),
		Version:  version,
		Stdin:    os.Stdin,
		Stdout:   os.Stdout,
		Stderr:   os.Stderr,
	}
}

// Execute runs the command tree and returns the process exit code.
func (a *App) Execute(args []string) int {
	a.exitCode = 0
	root := a.Root()
	root.SetArgs(args)
	if err := root.Execute(); err != nil {
		fmt.Fprintln(a.Stderr, "kopsc: "+err.Error())
		return executor.ExitCode(err)
	}
	return a.exitCode
}

// Root builds the full cobra command tree.
func (a *App) Root() *cobra.Command {
	root := &cobra.Command{
		Use:           "kopsc",
		Short:         "跨平台命令助手：用统一的意图命令在各平台执行对应命令",
		Long:          "kopsc 用平台无关的意图命令（如 docker clearsafe），根据操作系统、发行版与已安装的运行时，选择并执行对应的底层命令。",
		Version:       a.Version,
		SilenceUsage:  true,
		SilenceErrors: true,
		RunE: func(cmd *cobra.Command, args []string) error {
			return cmd.Help()
		},
	}
	root.SetOut(a.Stdout)
	root.SetErr(a.Stderr)
	root.SetIn(a.Stdin)

	for i := range a.Registry.Commands {
		a.addRegistryCommand(root, &a.Registry.Commands[i])
	}

	root.AddCommand(a.newListCmd(), a.newExplainCmd(), a.newPlatformCmd(), a.newVersionCmd())
	return root
}

// addRegistryCommand turns "docker.clearsafe" into `kopsc docker clearsafe`,
// reusing intermediate commands for shared ID prefixes.
func (a *App) addRegistryCommand(root *cobra.Command, cmd *registry.Command) {
	parts := strings.Split(cmd.ID, ".")
	parent := root

	for _, group := range parts[:len(parts)-1] {
		child := findChild(parent, group)
		if child == nil {
			groupName := group
			child = &cobra.Command{
				Use:   groupName,
				Short: groupName + " 相关命令",
				RunE: func(c *cobra.Command, args []string) error {
					return c.Help()
				},
			}
			parent.AddCommand(child)
		}
		parent = child
	}

	leaf := a.newLeaf(cmd, parts[len(parts)-1])
	parent.AddCommand(leaf)
}

func findChild(parent *cobra.Command, name string) *cobra.Command {
	for _, child := range parent.Commands() {
		if child.Name() == name {
			return child
		}
	}
	return nil
}

// variantDisplay renders how a variant is executed, for list/help/explain.
func variantDisplay(v registry.Variant) string {
	if v.Builtin != "" {
		return "(内置) " + v.Builtin
	}
	return strings.Join(v.Argv, " ")
}

func (a *App) newLeaf(cmd *registry.Command, name string) *cobra.Command {
	leaf := &cobra.Command{
		Use:                name + " [--dry-run] [-h] [-- 透传参数...]",
		Short:              cmd.Summary,
		Long:               cmd.Summary + "\n\n动作: " + cmd.Action,
		DisableFlagParsing: true,
		RunE: func(c *cobra.Command, args []string) error {
			dryRun, yes, extra, help, err := parseLeafArgs(cmd, args)
			if err != nil {
				return err
			}
			if help {
				a.printLeafHelp(c.OutOrStdout(), cmd)
				return nil
			}

			if err := cmd.ValidateArgs(extra); err != nil {
				return err
			}

			result, err := resolve.Resolve(cmd, a.Info)
			if err != nil {
				return fmt.Errorf("%w（可用 `kopsc explain %s` 查看全部候选）", err, cmd.ID)
			}

			plan := executor.Plan{Result: result, ExtraArgs: extra}

			if cmd.RequiresYes && !yes && !dryRun {
				return a.printConfirmation(cmd, plan)
			}

			return plan.Run(context.Background(), executor.Options{
				DryRun: dryRun,
				Stdin:  a.Stdin,
				Stdout: a.Stdout,
				Stderr: a.Stderr,
			})
		},
	}
	return leaf
}

// printConfirmation explains a destructive command and how to approve it. If
// the command has a preview builtin, the objects it would touch are listed
// first (and an unknown target is reported before asking for --yes).
func (a *App) printConfirmation(cmd *registry.Command, plan executor.Plan) error {
	if err := executor.Preview(context.Background(), plan, a.Stdout); err != nil {
		return err
	}

	w := a.Stdout
	fmt.Fprintln(w, "这是破坏性命令，未执行。")
	fmt.Fprintf(w, "命令:   %s\n", cmd.Invocation())
	fmt.Fprintf(w, "将执行: %s\n", plan.Display())
	fmt.Fprintf(w, "影响:   %s\n\n", cmd.Action)
	// Repeat the caller's own arguments so the suggestion is copy-pasteable.
	suggested := cmd.Invocation() + " --yes"
	if len(plan.ExtraArgs) > 0 {
		suggested += " " + strings.Join(plan.ExtraArgs, " ")
	}
	fmt.Fprintf(w, "确认执行请追加 --yes：\n  %s\n", suggested)
	a.exitCode = 2
	return nil
}

// parseLeafArgs splits leaf arguments: kopsc-level flags before the first `--`
// are consumed, everything else (including `--` and after) is forwarded.
//
// For commands marked Namespaced (Kubernetes-aware), -n/-N mean --namespace
// (kubectl style) and are normalised to --namespace; --dry-run remains the
// only preview flag. On every other command -n keeps its --dry-run meaning.
func parseLeafArgs(cmd *registry.Command, args []string) (dryRun, yes bool, extra []string, help bool, err error) {
	passthrough := false
	for i := 0; i < len(args); i++ {
		arg := args[i]

		if passthrough {
			extra = append(extra, arg)
			continue
		}

		switch {
		case arg == "--":
			passthrough = true

		case cmd.Namespaced && (arg == "-n" || arg == "-N" || arg == "--namespace"):
			if i+1 >= len(args) {
				return false, false, nil, false,
					fmt.Errorf("%s: %s 需要命名空间参数，例如 %s prod", cmd.Invocation(), arg, arg)
			}
			i++
			extra = append(extra, "--namespace", args[i])

		case cmd.Namespaced && strings.HasPrefix(arg, "-n="):
			extra = append(extra, "--namespace="+strings.TrimPrefix(arg, "-n="))

		case cmd.Namespaced && strings.HasPrefix(arg, "-N="):
			extra = append(extra, "--namespace="+strings.TrimPrefix(arg, "-N="))

		case cmd.Namespaced && strings.HasPrefix(arg, "--namespace="):
			extra = append(extra, arg)

		case arg == "-n" || arg == "--dry-run" || arg == "--dry-run=true":
			dryRun = true

		case arg == "--dry-run=false":
			dryRun = false

		case arg == "-y" || arg == "--yes":
			yes = true

		case arg == "-h" || arg == "--help":
			help = true

		default:
			extra = append(extra, arg)
		}
	}
	return dryRun, yes, extra, help, nil
}

func (a *App) printLeafHelp(w io.Writer, cmd *registry.Command) {
	invocation := cmd.Invocation()
	argsHint := ""
	if cmd.Usage != "" {
		argsHint = " " + cmd.Usage
	}
	fmt.Fprintf(w, "用法: %s [--dry-run] [-h] [--yes]%s [-- 透传参数...]\n\n", invocation, argsHint)
	fmt.Fprintf(w, "%s\n\n", cmd.Summary)
	fmt.Fprintf(w, "动作: %s\n\n", cmd.Action)
	fmt.Fprintf(w, "当前平台: %s\n", resolve.DescribeInfo(a.Info))

	if result, err := resolve.Resolve(cmd, a.Info); err == nil {
		fmt.Fprintf(w, "将执行: %s\n\n", variantDisplay(result.Variant))
	} else {
		fmt.Fprintf(w, "当前平台不可用: %v\n\n", err)
	}

	fmt.Fprint(w, "选项:\n")
	fmt.Fprint(w, "  -n, --dry-run  仅打印将执行的命令与判定依据，不执行\n")
	fmt.Fprint(w, "  -h, --help     显示本帮助\n")
	if cmd.RequiresYes {
		fmt.Fprint(w, "  -y, --yes      确认执行破坏性命令（否则只打印影响并以状态码 2 退出）\n")
	}
	fmt.Fprint(w, "  --             其后的参数原样透传给底层命令\n")
	if cmd.Usage != "" {
		fmt.Fprintf(w, "\n参数: %s（至少 %d 个）\n", cmd.Usage, cmd.MinArgs)
	}
}

func (a *App) newListCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "list",
		Short: "列出全部命令及其在当前平台的可用性",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			tw := tabwriter.NewWriter(a.Stdout, 0, 0, 2, ' ', 0)
			fmt.Fprintln(tw, "命令\t状态\t将执行")
			for i := range a.Registry.Commands {
				entry := &a.Registry.Commands[i]
				status, planned := "可用", "-"
				if result, err := resolve.Resolve(entry, a.Info); err == nil {
					planned = variantDisplay(result.Variant)
				} else {
					status = "不可用"
				}
				fmt.Fprintf(tw, "%s\t%s\t%s\n", entry.ID, status, planned)
			}
			return tw.Flush()
		},
	}
}

func (a *App) newExplainCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "explain <命令>",
		Short: "解释某条命令在当前平台的匹配过程",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			entry, ok := a.Registry.Lookup(args[0])
			if !ok {
				return fmt.Errorf("未知命令 %q", args[0])
			}

			w := a.Stdout
			fmt.Fprintf(w, "命令: %s\n", entry.ID)
			fmt.Fprintf(w, "说明: %s\n", entry.Summary)
			fmt.Fprintf(w, "动作: %s\n", entry.Action)
			fmt.Fprintf(w, "平台: %s\n\n", resolve.DescribeInfo(a.Info))

			result, err := resolve.Resolve(entry, a.Info)
			if err != nil {
				fmt.Fprintf(w, "结果: 当前平台不可用 — %v\n\n", err)
			} else {
				fmt.Fprintf(w, "结果: 命中变体 #%d（%d 分）→ %s\n\n",
					result.VariantIdx, result.Score, variantDisplay(result.Variant))
			}

			fmt.Fprintln(w, "候选:")
			for _, c := range result.Candidates {
				mark := "✗"
				if c.Matched {
					mark = "✓"
				}
				note := ""
				if c.Variant.Note != "" {
					note = "  # " + c.Variant.Note
				}
				fmt.Fprintf(w, "  %s #%d %d 分  %s -> %s%s\n",
					mark, c.Index, c.Score, c.Reason, variantDisplay(c.Variant), note)
			}
			return nil
		},
	}
}

func (a *App) newPlatformCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "platform",
		Short: "显示探测到的当前平台信息",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			_, err := io.WriteString(a.Stdout, a.Info.String())
			return err
		},
	}
}

func (a *App) newVersionCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "version",
		Short: "显示版本与平台",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			_, err := fmt.Fprintf(a.Stdout, "kopsc %s %s/%s\n", a.Version, a.Info.OS, a.Info.Arch)
			return err
		},
	}
}
