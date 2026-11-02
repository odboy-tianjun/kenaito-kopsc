// Package cli builds the kopsc command tree from the registry.
package cli

import (
	"context"
	"fmt"
	"io"
	"os"
	"strings"

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
		Use:   "kopsc",
		Short: "跨平台命令助手：用统一的意图命令在各平台执行对应命令",
		Long: `kopsc 用平台无关的意图命令（如 docker clearsafe），根据操作系统、发行版与已安装的运行时，
自动选择并执行对应的底层命令。你只需记住要做什么，不必记住每个平台的写法。

常用示例：
  kopsc docker clearsafe          安全清理未使用的容器、网络与悬空镜像
  kopsc diagnose web              诊断容器或 Pod 为什么起不来（自动识别 Docker / K8s）
  kopsc net ports                 列出本机正在监听的端口
  kopsc disk usage                看当前目录里谁最占空间
  kopsc proc nohup ./server       后台运行并写日志（等价 nohup ./server > server.out 2>&1 &）
  kopsc list                      列出全部命令及在本平台实际会执行的命令`,
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
	localizeCobra(root)

	for i := range a.Registry.Commands {
		a.addRegistryCommand(root, &a.Registry.Commands[i])
	}

	root.AddCommand(a.newListCmd(), a.newExplainCmd(), a.newPlatformCmd(), a.newVersionCmd())
	localizeBuiltins(root)
	localizeHelpFlags(root)
	return root
}

// localizeBuiltins gives cobra's generated commands Chinese descriptions.
// cobra creates completion and help lazily, so they are initialised here
// explicitly and then renamed.
func localizeBuiltins(root *cobra.Command) {
	root.InitDefaultCompletionCmd()
	root.InitDefaultHelpCmd()

	if cmd := findChild(root, "completion"); cmd != nil {
		cmd.Short = "生成 shell 补全脚本（bash / zsh / fish / powershell）"
		cmd.Long = "为 bash、zsh、fish 或 powershell 生成 kopsc 的命令补全脚本。"
	}
	if cmd := findChild(root, "help"); cmd != nil {
		cmd.Short = "查看任意命令的帮助"
		cmd.Long = "查看任意命令的帮助，例如 kopsc help disk，效果等同 kopsc disk --help。"
	}
}

// addRegistryCommand turns "docker.clearsafe" into `kopsc docker clearsafe`,
// reusing intermediate commands for shared ID prefixes.
func (a *App) addRegistryCommand(root *cobra.Command, cmd *registry.Command) {
	parts := strings.Split(cmd.ID, ".")
	parent := root

	for _, group := range parts[:len(parts)-1] {
		child := findChild(parent, group)
		if child == nil {
			child = newGroup(group)
			parent.AddCommand(child)
		}
		parent = child
	}

	leaf := a.newLeaf(cmd, parts[len(parts)-1])
	parent.AddCommand(leaf)
}

// localizeCobra replaces cobra's built-in English strings with Chinese ones.
//
// cobra renders the text itself (its templates use cobra's own functions such
// as rpad), so kopsc swaps the templates for Chinese headings and pre-registers
// the help and version flags: cobra only generates its English descriptions
// ("help for kopsc") for flags it has not seen yet.
func localizeCobra(root *cobra.Command) {
	cobra.AddTemplateFunc("kopscOptions", func(line string) string {
		return strings.ReplaceAll(line, "[flags]", "[选项]")
	})

	root.Use = "kopsc"
	root.SetUsageTemplate(usageTemplate)
	root.SetHelpTemplate(helpTemplate)

	root.InitDefaultHelpFlag()
	root.InitDefaultVersionFlag()
	root.Flags().Lookup("help").Usage = "显示帮助"
	if flag := root.Flags().Lookup("version"); flag != nil {
		flag.Usage = "显示版本"
	}

}

// localizeHelpFlags pre-registers cobra's help flag on every command with a
// Chinese description. It must run after the command tree is complete, since
// cobra only generates the English default for commands it has not seen.
func localizeHelpFlags(root *cobra.Command) {
	queue := []*cobra.Command{root}
	for len(queue) > 0 {
		c := queue[0]
		queue = queue[1:]
		c.InitDefaultHelpFlag()
		if flag := c.Flags().Lookup("help"); flag != nil {
			flag.Usage = "显示帮助"
		}
		queue = append(queue, c.Commands()...)
	}
}

// usageTemplate is cobra's default usage layout with Chinese headings.
const usageTemplate = `用法:{{if .Runnable}}
  {{.UseLine | kopscOptions}}{{end}}{{if .HasAvailableSubCommands}}
  {{.CommandPath}} [命令]{{end}}{{if gt (len .Aliases) 0}}

别名:
  {{.NameAndAliases}}{{end}}{{if .HasExample}}

示例:
{{.Example}}{{end}}{{if .HasAvailableSubCommands}}

可用的子命令:{{range .Commands}}{{if (or .IsAvailableCommand (eq .Name "help"))}}
  {{rpad .Name .NamePadding }} {{.Short}}{{end}}{{end}}{{end}}{{if .HasAvailableLocalFlags}}

选项:
{{.LocalFlags.FlagUsages | trimTrailingWhitespaces}}{{end}}{{if .HasAvailableInheritedFlags}}

全局选项:
{{.InheritedFlags.FlagUsages | trimTrailingWhitespaces}}{{end}}{{if .HasHelpSubCommands}}

更多帮助:
  {{.CommandPath}} [命令] --help{{end}}
`

// helpTemplate is cobra's default help layout.
const helpTemplate = `{{with (or .Long .Short)}}{{. | trimTrailingWhitespaces}}

{{end}}{{if or .Runnable .HasSubCommands}}{{.UsageString}}{{end}}`

// helpTextReplacements rewrites the few English fragments cobra still emits.
var helpTextReplacements = []struct{ old, new string }{
	{"[flags]", "[选项]"},
	{"help for ", "查看 "},
	{"Generate the autocompletion script for the specified shell", "生成 shell 补全脚本（bash / zsh / fish / powershell）"},
	{"Help about any command", "查看任意命令的帮助"},
}

// translateHelp applies helpTextReplacements.
func translateHelp(text string) string {
	for _, r := range helpTextReplacements {
		text = strings.ReplaceAll(text, r.old, r.new)
	}
	return text
}

// groupDescriptions explains what each command group is for. Without them
// cobra would show "disk 相关命令", which tells a user nothing.
var groupDescriptions = map[string]string{
	"docker": "Docker 容器与镜像管理（查看、启停、日志、清理、导出/导入、Compose）",
	"disk":   "磁盘与文件占用：查看哪个目录、哪个文件最占空间",
	"net":    "网络与端口：端口被谁占用、本机监听了哪些端口",
	"pkg":    "系统软件包：刷新包管理器（dnf/apt/apk/brew/winget）的软件源索引",
	"proc":   "本机进程：在后台运行命令并写日志，关掉终端也不退出",
	"java":   "Java 应用：按 JDK 版本自动套用调优参数并前台运行 jar",
	"git":    "Git 历史：把指定日期范围的提交时间改到 21:00-23:00 并强推",
	"user":   "用户管理：授予或移除用户的 root/sudo 权限",
}

// newGroup builds an intermediate command such as `kopsc disk`.
func newGroup(name string) *cobra.Command {
	short, ok := groupDescriptions[name]
	if !ok {
		short = name + " 相关命令"
	}
	return &cobra.Command{
		Use:   name,
		Short: short,
		Long:  short + "。\n\n运行 " + "kopsc " + name + " --help 查看该组的全部子命令。",
		RunE: func(c *cobra.Command, args []string) error {
			return c.Help()
		},
	}
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

// leafUsage renders the cobra usage line for a leaf command. Commands that
// already spell out their own arguments (such as `java run <app.jar> [-- 应用参数...]`)
// keep it as-is; the generic "[-- 透传参数...]" tail would only repeat it.
func leafUsage(name, usage string) string {
	if strings.TrimSpace(usage) != "" {
		return name + " " + usage
	}
	return name + " [--dry-run] [-h] [-- 透传参数...]"
}

func (a *App) newLeaf(cmd *registry.Command, name string) *cobra.Command {
	leaf := &cobra.Command{
		Use:                leafUsage(name, cmd.Usage),
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

			if dryRun {
				// A command with a preview builtin can show exactly what it
				// would do (for example the final java command line), so
				// render that instead of only the builtin's name.
				if err := executor.Preview(context.Background(), plan, a.Stdout); err != nil {
					return err
				}
			}

			return plan.Run(context.Background(), executor.Options{
				DryRun:    dryRun,
				Confirmed: yes,
				Stdin:     a.Stdin,
				Stdout:    a.Stdout,
				Stderr:    a.Stderr,
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
	// A command that documents its own arguments already ends with them, so
	// the generic passthrough tail is only added when there is no Usage.
	if cmd.Usage != "" {
		fmt.Fprintf(w, "用法: %s [--dry-run] [-h] [--yes] %s\n\n", invocation, cmd.Usage)
	} else {
		fmt.Fprintf(w, "用法: %s [--dry-run] [-h] [--yes] [-- 透传参数...]\n\n", invocation)
	}
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
			rows := [][]string{{"命令", "状态", "将执行"}}
			for i := range a.Registry.Commands {
				entry := &a.Registry.Commands[i]
				status, planned := "可用", "-"
				if result, err := resolve.Resolve(entry, a.Info); err == nil {
					planned = variantDisplay(result.Variant)
				} else {
					status = "不可用"
				}
				rows = append(rows, []string{entry.ID, status, planned})
			}
			writeColumns(a.Stdout, rows)
			return nil
		},
	}
}

func (a *App) newExplainCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "explain <命令>",
		Short: "解释某条命令在当前平台的匹配过程",
		Long: `解释 kopsc 为什么在当前平台选择了某个底层命令。

kopsc 的每条命令都由若干「变体」组成，每个变体声明它适用的环境
（操作系统、发行版、CPU 架构、容器运行时）。explain 会打印当前探测到的
平台信息、命中的变体及其得分，并列出全部候选以及每个候选被选中或被排除的原因，
方便排查“为什么执行的是这条命令”“为什么本平台不可用”。

命令名有两种写法，等价：
  kopsc explain docker.clearsafe   点号分隔的完整 ID
  kopsc explain docker clearsafe   命令行中的写法

也可以使用别名（如 docker.prunesafe），或只写分组名查看该组内匹配较好的命令
（如 kopsc explain docker）。

输出各项含义：
  命令 / 说明 / 动作   该命令的用途与安全约定
  平台                当前探测结果：系统/架构、发行版、已安装的运行时
  结果                命中的变体序号、得分与最终要执行的命令
  候选                全部变体：✓ 为满足约束并参与比较，✗ 为被排除
  得分                发行版精确命中 +3、ID_LIKE 命中 +2、运行时命中 +2，
                      通用变体 0 分；同分时按运行时偏好与约束数量取舍

退出码：找到命令即返回 0（即使本平台不可用，也会说明原因）；命令名不存在返回 1。`,
		Example: `  kopsc explain docker.clearsafe      查看清理命令在本平台为什么这样执行
  kopsc explain docker clearsafe      同上，与命令行写法一致
  kopsc explain disk.usage            查看磁盘占用命令的候选
  kopsc explain docker                查看 docker 分组内各命令的匹配情况`,
		Args: a.explainArgs,
		RunE: a.runExplain,
	}
}

// explainArgs accepts the command name in either its dotted (docker.clearsafe)
// or spaced (docker clearsafe) form.
func (a *App) explainArgs(cmd *cobra.Command, args []string) error {
	if len(args) == 0 {
		return fmt.Errorf("缺少命令名；用法: %s <命令>，例如 %s docker.clearsafe（可先用 kopsc list 查看全部命令）",
			cmd.CommandPath(), cmd.CommandPath())
	}
	return nil
}

// resolveExplainTarget maps the arguments to a registry entry. A full dotted or
// spaced name wins; otherwise the arguments are treated as a command prefix, so
// `kopsc explain docker` explains every command in that group.
func (a *App) resolveExplainTarget(args []string) (*registry.Command, string, bool) {
	joined := strings.Join(args, " ")
	for _, name := range []string{joined, strings.Join(args, ".")} {
		if cmd, ok := a.Registry.Lookup(name); ok {
			return cmd, "", true
		}
	}
	return nil, joined, false
}

// explainGroup returns the commands whose ID starts with the given prefix, for
// `kopsc explain <分组名>`.
func (a *App) explainGroup(prefix string) []*registry.Command {
	want := strings.TrimSuffix(prefix, ".") + "."
	var out []*registry.Command
	for i := range a.Registry.Commands {
		if strings.HasPrefix(a.Registry.Commands[i].ID, want) {
			out = append(out, &a.Registry.Commands[i])
		}
	}
	return out
}

// explainCommands summarises a group: one line per command with the variant it
// would run on this platform.
func (a *App) explainCommands(cmds []*registry.Command) error {
	w := a.Stdout
	fmt.Fprintf(w, "分组 %q 下共 %d 条命令，平台: %s\n\n", strings.Split(cmds[0].ID, ".")[0], len(cmds), resolve.DescribeInfo(a.Info))

	rows := [][]string{{"命令", "状态", "将执行", "说明"}}
	for _, cmd := range cmds {
		status, planned := "可用", "-"
		if result, err := resolve.Resolve(cmd, a.Info); err == nil {
			planned = variantDisplay(result.Variant)
		} else {
			status = "不可用"
		}
		rows = append(rows, []string{cmd.ID, status, planned, cmd.Summary})
	}
	writeColumns(w, rows)
	fmt.Fprintf(w, "\n查看单条命令的匹配过程：kopsc explain %s\n", cmds[0].ID)
	return nil
}

func (a *App) runExplain(cmd *cobra.Command, args []string) error {
	w := a.Stdout
	entry, joined, ok := a.resolveExplainTarget(args)
	if !ok {
		if group := a.explainGroup(joined); len(group) > 0 {
			return a.explainCommands(group)
		}
		ids := a.Registry.IDs()
		fmt.Fprintf(w, "没有名为 %q 的命令。\n\n", joined)
		fmt.Fprintln(w, "可用的命令:")
		for _, id := range ids {
			fmt.Fprintf(w, "  %s\n", id)
		}
		fmt.Fprintln(w, "\n提示：命令名可写点号形式（docker.clearsafe）或空格形式（docker clearsafe），可用 kopsc list 查看。")
		a.exitCode = 1
		return nil
	}

	fmt.Fprintf(w, "命令: %s\n", entry.ID)
	if len(entry.Aliases) > 0 {
		fmt.Fprintf(w, "别名: %s\n", strings.Join(entry.Aliases, ", "))
	}
	fmt.Fprintf(w, "说明: %s\n", entry.Summary)
	fmt.Fprintf(w, "动作: %s\n", entry.Action)
	if entry.Usage != "" {
		fmt.Fprintf(w, "用法: %s %s\n", entry.Invocation(), entry.Usage)
	}
	if entry.RequiresYes {
		fmt.Fprintln(w, "确认: 破坏性命令，未加 --yes 时只列出将影响的对象并以退出码 2 结束")
	}
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
		fmt.Fprintf(w, "  %s #%d %d 分  %s\n", mark, c.Index, c.Score, c.Reason)
		fmt.Fprintf(w, "      执行: %s\n", variantDisplay(c.Variant))
		if c.Variant.Note != "" {
			fmt.Fprintf(w, "      说明: %s\n", c.Variant.Note)
		}
	}
	return nil
}

// writeColumns prints rows as a table, padding by terminal display width so
// CJK headings line up (text/tabwriter counts bytes and would misalign them).
func writeColumns(w io.Writer, rows [][]string) {
	const gap = 2
	widths := make([]int, len(rows[0]))
	for _, row := range rows {
		for i, cell := range row {
			if n := displayWidth(cell); n > widths[i] {
				widths[i] = n
			}
		}
	}

	var b strings.Builder
	for _, row := range rows {
		b.Reset()
		for i, cell := range row {
			b.WriteString(cell)
			if i < len(row)-1 {
				b.WriteString(strings.Repeat(" ", widths[i]-displayWidth(cell)+gap))
			}
		}
		fmt.Fprintln(w, strings.TrimRight(b.String(), " "))
	}
}

// displayWidth counts the columns s occupies: two for wide (CJK) runes.
func displayWidth(s string) int {
	width := 0
	for _, r := range s {
		if r >= 0x1100 && (r <= 0x115F ||
			r >= 0x2E80 && r <= 0xA4CF ||
			r >= 0xAC00 && r <= 0xD7A3 ||
			r >= 0xF900 && r <= 0xFAFF ||
			r >= 0xFE30 && r <= 0xFE6F ||
			r >= 0xFF00 && r <= 0xFF60 ||
			r >= 0xFFE0 && r <= 0xFFE6 ||
			r >= 0x1F300 && r <= 0x1F64F ||
			r >= 0x20000 && r <= 0x3FFFD) {
			width += 2
			continue
		}
		width++
	}
	return width
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
