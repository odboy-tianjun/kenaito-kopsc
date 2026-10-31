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
	root := a.Root()
	root.SetArgs(args)
	if err := root.Execute(); err != nil {
		fmt.Fprintln(a.Stderr, "kopsc: "+err.Error())
		return executor.ExitCode(err)
	}
	return 0
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

func (a *App) newLeaf(cmd *registry.Command, name string) *cobra.Command {
	leaf := &cobra.Command{
		Use:                name + " [--dry-run] [-h] [-- 透传参数...]",
		Short:              cmd.Summary,
		Long:               cmd.Summary + "\n\n动作: " + cmd.Action,
		DisableFlagParsing: true,
		RunE: func(c *cobra.Command, args []string) error {
			dryRun, extra, help := parseLeafArgs(args)
			if help {
				a.printLeafHelp(c.OutOrStdout(), cmd)
				return nil
			}

			result, err := resolve.Resolve(cmd, a.Info)
			if err != nil {
				return fmt.Errorf("%w（可用 `kopsc explain %s` 查看全部候选）", err, cmd.ID)
			}

			plan := executor.Plan{Result: result, ExtraArgs: extra}
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

// parseLeafArgs splits leaf arguments: kopsc-level flags before the first `--`
// are consumed, everything else (including `--` and after) is forwarded.
func parseLeafArgs(args []string) (dryRun bool, extra []string, help bool) {
	passthrough := false
	for _, arg := range args {
		if passthrough {
			extra = append(extra, arg)
			continue
		}
		switch {
		case arg == "--":
			passthrough = true
		case arg == "-n" || arg == "--dry-run" || arg == "--dry-run=true":
			dryRun = true
		case arg == "--dry-run=false":
			dryRun = false
		case arg == "-h" || arg == "--help":
			help = true
		default:
			extra = append(extra, arg)
		}
	}
	return dryRun, extra, help
}

func (a *App) printLeafHelp(w io.Writer, cmd *registry.Command) {
	invocation := "kopsc " + strings.ReplaceAll(cmd.ID, ".", " ")
	fmt.Fprintf(w, "用法: %s [--dry-run] [-h] [-- 透传参数...]\n\n", invocation)
	fmt.Fprintf(w, "%s\n\n", cmd.Summary)
	fmt.Fprintf(w, "动作: %s\n\n", cmd.Action)
	fmt.Fprintf(w, "当前平台: %s\n", resolve.DescribeInfo(a.Info))

	if result, err := resolve.Resolve(cmd, a.Info); err == nil {
		fmt.Fprintf(w, "将执行: %s\n\n", strings.Join(result.Variant.Argv, " "))
	} else {
		fmt.Fprintf(w, "当前平台不可用: %v\n\n", err)
	}

	fmt.Fprint(w, "选项:\n")
	fmt.Fprint(w, "  -n, --dry-run  仅打印将执行的命令与判定依据，不执行\n")
	fmt.Fprint(w, "  -h, --help     显示本帮助\n")
	fmt.Fprint(w, "  --             其后的参数原样透传给底层命令\n")
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
					planned = strings.Join(result.Variant.Argv, " ")
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
					result.VariantIdx, result.Score, strings.Join(result.Variant.Argv, " "))
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
					mark, c.Index, c.Score, c.Reason, strings.Join(c.Variant.Argv, " "), note)
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
