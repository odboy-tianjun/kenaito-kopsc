package executor

import (
	"context"
	"errors"
	"fmt"
	"io"
	"math/rand"
	"strconv"
	"strings"
	"time"
)

// gitRetimeOptions are the parsed arguments of `kopsc git retime`.
type gitRetimeOptions struct {
	From       time.Time     // inclusive start of the commit range
	To         time.Time     // inclusive end of the commit range
	Window     time.Duration // window start, measured from midnight (default 21h)
	WindowEnd  time.Duration // window end, measured from midnight (default 23h)
	MinGap     time.Duration
	MaxGap     time.Duration
	Seed       int64
	SeedSet    bool
	Remote     string
	Branch     string
	NoPush     bool
	MatchEmote bool // --interval match-remote
}

// runGitRetime rewrites the timestamps of commits in [From, To] so they land
// between the window's start and end on their own calendar day, then force
// pushes the branch.
//
// Without --yes it only prints the plan (exit code 2), so the destructive path
// always needs an explicit confirmation.
func runGitRetime(ctx context.Context, req BuiltinRequest) error {
	err := runGitRetimeWith(ctx, req, execGitRunner{}, time.Now())
	if errors.Is(err, errNothingToRewrite) {
		return nil
	}
	return err
}

// runGitRetimeWith is runGitRetime with its dependencies injected for tests.
func runGitRetimeWith(ctx context.Context, req BuiltinRequest, runner gitRunner, now time.Time) error {
	opts, confirmedInArgs, err := parseGitRetimeArgs(req.Args, now)
	if err != nil {
		return err
	}
	// The CLI consumes --yes and passes it along as req.Confirmed; accept
	// either so the flag works wherever the user puts it.
	confirmed := req.Confirmed || confirmedInArgs

	branch := opts.Branch
	if branch == "" {
		if branch, err = currentBranch(ctx, runner); err != nil {
			return err
		}
	}

	remote := opts.Remote
	if remote == "" && !opts.NoPush {
		names, err := remotes(ctx, runner)
		if err != nil {
			return err
		}
		switch {
		case len(names) == 1:
			remote = names[0]
		case len(names) == 0:
			return errors.New("仓库没有配置远程；用 --no-push 只改写本地，或先 git remote add")
		default:
			return fmt.Errorf("仓库有多个远程（%s），请用 --remote 指定要强推哪个",
				strings.Join(names, ", "))
		}
	}

	plan, err := collectRetimePlan(ctx, runner, opts, branch, remote)
	if err != nil {
		return err
	}
	if len(plan.Commits) == 0 {
		fmt.Fprintf(req.Stdout, "%s 到 %s 之间没有需要改写的提交，未做任何改动\n",
			opts.From.Format("2006-01-02"), opts.To.Format("2006-01-02"))
		// Nothing to rewrite is not an error, but there is nothing to confirm
		// either: report success so the CLI skips the destructive prompt.
		return errNothingToRewrite
	}

	writeRetimePreview(req, plan, opts)

	if !confirmed {
		// Without --yes, stop after showing what would happen.
		return &ExitError{Code: 2, Err: errRetimeNeedsConfirmation}
	}

	return applyRetime(ctx, req, runner, plan, opts, now)
}

// writeRetimePreview prints the "提交 / 原时间 / 新时间" table.
//
// Columns are padded by display width, not byte length: the headings are CJK
// and text/tabwriter would misalign them.
func writeRetimePreview(req BuiltinRequest, plan retimePlan, opts gitRetimeOptions) {
	w := req.Stdout
	target := plan.Remote + "/" + plan.Branch
	if opts.NoPush {
		target = plan.Branch + "（--no-push，只改本地）"
	}
	fmt.Fprintf(w, "改写:   %d 个提交（分支 %s，远程 %s）\n", len(plan.Commits), plan.Branch, plan.Remote)
	fmt.Fprintf(w, "强推:   %s\n", pushSummary(plan, opts))
	fmt.Fprintf(w, "目标:   %s\n", target)
	fmt.Fprintf(w, "窗口:   %s-%s（保留原日期）\n\n",
		clockOf(opts.Window), clockOf(opts.WindowEnd))

	rows := [][]string{{"提交", "原时间", "新时间", "主题"}}
	for _, item := range plan.Commits {
		mark := ""
		if item.Squeezed {
			mark = "  ⚠ 已压缩间隔"
		}
		rows = append(rows, []string{
			item.Short,
			item.OldAuthor.Format("2006-01-02 15:04:05"),
			item.NewTime.Format("15:04:05") + mark,
			item.Subject,
		})
	}
	writeDisplayColumns(w, rows)
}

// pushSummary states whether the branch will be force pushed.
func pushSummary(plan retimePlan, opts gitRetimeOptions) string {
	if opts.NoPush {
		return "否（--no-push，只改本地）"
	}
	return fmt.Sprintf("是 --force-with-lease → %s/%s", plan.Remote, plan.Branch)
}

// clockOf renders a duration since midnight as HH:MM.
func clockOf(d time.Duration) string {
	return fmt.Sprintf("%02d:%02d", int(d.Hours()), int(d.Minutes())%60)
}

// writeDisplayColumns prints rows padded by terminal display width, so CJK
// headings line up (text/tabwriter counts bytes and misaligns them).
func writeDisplayColumns(w io.Writer, rows [][]string) {
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

// errRetimeNeedsConfirmation ends the run with exit code 2 after the preview.
var errRetimeNeedsConfirmation = errors.New("未执行改写：确认后请追加 --yes")

// errNothingToRewrite signals a range with no matching commits. It is not a
// failure: the command printed what it found and changed nothing.
var errNothingToRewrite = errors.New("区间内没有需要改写的提交")

// collectRetimePlan reads the range, builds the new times and returns the plan.
func collectRetimePlan(ctx context.Context, runner gitRunner, opts gitRetimeOptions, branch, remote string) (retimePlan, error) {
	commits, err := listCommits(ctx, runner, branch)
	if err != nil {
		return retimePlan{}, err
	}

	selected := selectCommitsInRange(commits, opts.From, opts.To)
	if len(selected) == 0 {
		return retimePlan{}, nil
	}

	plannerOpts := retimeOptions{
		WindowStart: opts.Window,
		WindowEnd:   opts.WindowEnd,
		MinGap:      opts.MinGap,
		MaxGap:      opts.MaxGap,
		FirstGapMax: 20 * time.Minute,
		Rand:        newRetimeRand(opts.Seed),
	}
	_ = opts.MatchEmote
	return buildPlan(selected, plannerOpts, branch, remote), nil
}

// selectCommitsInRange keeps commits whose calendar day falls inside
// [from, to], both inclusive.
func selectCommitsInRange(commits []gitCommit, from, to time.Time) []gitCommit {
	start := time.Date(from.Year(), from.Month(), from.Day(), 0, 0, 0, 0, from.Location())
	end := time.Date(to.Year(), to.Month(), to.Day(), 23, 59, 59, 0, to.Location())

	var out []gitCommit
	for _, c := range commits {
		day := c.Date()
		if (day.Equal(start) || day.After(start)) && (day.Equal(end) || day.Before(end)) {
			out = append(out, c)
		}
	}
	return out
}

// applyRetime performs the rewrite, verifies it and pushes.
func applyRetime(ctx context.Context, req BuiltinRequest, runner gitRunner, plan retimePlan, opts gitRetimeOptions, now time.Time) error {
	if err := ensureCleanWorktree(ctx, runner); err != nil {
		return err
	}

	oldTip := plan.Commits[len(plan.Commits)-1].Hash
	backupRef, err := createBackup(ctx, runner, plan.Branch, oldTip, now)
	if err != nil {
		return err
	}
	fmt.Fprintf(req.Stdout, "备份:   %s\n", backupRef)

	newTip, rebuilt, err := rewriteChain(ctx, runner, plan)
	if err != nil {
		fmt.Fprintf(req.Stderr, "%s\n", backupHint(plan.Branch, backupRef))
		return err
	}
	if err := verifyRewrite(ctx, runner, oldTip, newTip); err != nil {
		fmt.Fprintf(req.Stderr, "%s\n", backupHint(plan.Branch, backupRef))
		return err
	}

	if err := moveBranch(ctx, runner, plan.Branch, newTip); err != nil {
		fmt.Fprintf(req.Stderr, "%s\n", backupHint(plan.Branch, backupRef))
		return err
	}
	fmt.Fprintf(req.Stdout, "已改写: %d 个提交，%s -> %s\n", len(rebuilt), oldTip[:minInt(12, len(oldTip))], newTip[:minInt(12, len(newTip))])

	if opts.NoPush {
		fmt.Fprintln(req.Stdout, "未推送（--no-push）；需要时手动执行 git push --force-with-lease")
		return nil
	}
	if err := forcePush(ctx, runner, plan.Remote, plan.Branch); err != nil {
		fmt.Fprintf(req.Stderr, "%s\n", backupHint(plan.Branch, backupRef))
		return err
	}
	fmt.Fprintf(req.Stdout, "已强推: %s %s\n", plan.Remote, plan.Branch)
	fmt.Fprintf(req.Stdout, "回滚:   %s\n", backupHint(plan.Branch, backupRef))
	return nil
}

func minInt(a, b int) int {
	if a < b {
		return a
	}
	return b
}

// newRetimeRand builds the planner's random source from --seed.
func newRetimeRand(seed int64) *rand.Rand {
	return rand.New(rand.NewSource(seed))
}

// parseGitRetimeArgs parses `git retime <from> <to> [options]`.
//
// The second return value reports whether --yes was given; the CLI layer owns
// --yes for destructive commands, so this parser only needs to know whether the
// caller approved. Without it the command previews and exits with code 2.
func parseGitRetimeArgs(args []string, now time.Time) (gitRetimeOptions, bool, error) {
	opts := gitRetimeOptions{
		Window:    21 * time.Hour,
		WindowEnd: 23 * time.Hour,
		MinGap:    5 * time.Minute,
		MaxGap:    40 * time.Minute,
		Remote:    "",
	}

	var dates []string
	confirmed := false
	interval := ""

	for i := 0; i < len(args); i++ {
		arg := args[i]

		if !strings.HasPrefix(arg, "-") || arg == "-" {
			dates = append(dates, arg)
			continue
		}

		name, value := arg, ""
		if eq := strings.IndexByte(arg, '='); eq >= 0 && strings.HasPrefix(arg, "--") {
			name, value = arg[:eq], arg[eq+1:]
		}
		takeValue := func() (string, error) {
			if value != "" {
				return value, nil
			}
			if i+1 >= len(args) {
				return "", fmt.Errorf("%s 缺少参数", name)
			}
			i++
			return args[i], nil
		}

		switch name {
		case "-y", "--yes":
			confirmed = true
		case "--window", "--since", "--until":
			// --since/--until are accepted as aliases: they name the window's
			// lower/upper bound, which is what the option sets together.
			v, err := takeValue()
			if err != nil {
				return opts, false, err
			}
			if opts.Window, opts.WindowEnd, err = parseWindow(v); err != nil {
				return opts, false, err
			}
		case "--interval":
			v, err := takeValue()
			if err != nil {
				return opts, false, err
			}
			interval = v
		case "--seed":
			v, err := takeValue()
			if err != nil {
				return opts, false, err
			}
			seed, err := strconv.ParseInt(v, 10, 64)
			if err != nil {
				return opts, false, fmt.Errorf("--seed 需要整数，收到 %q", v)
			}
			opts.Seed = seed
			opts.SeedSet = true
		case "--remote":
			v, err := takeValue()
			if err != nil {
				return opts, false, err
			}
			opts.Remote = v
		case "--branch":
			v, err := takeValue()
			if err != nil {
				return opts, false, err
			}
			opts.Branch = v
		case "--no-push":
			opts.NoPush = true
		default:
			return opts, false, fmt.Errorf("未知选项 %q", arg)
		}
	}

	if len(dates) != 2 {
		return opts, false, fmt.Errorf("需要开始与结束日期，例如 kopsc git retime 2026-10-01 2026-10-15")
	}
	from, err := parseDate(dates[0])
	if err != nil {
		return opts, false, err
	}
	to, err := parseDate(dates[1])
	if err != nil {
		return opts, false, err
	}
	if to.Before(from) {
		return opts, false, fmt.Errorf("结束日期 %s 早于开始日期 %s", dates[1], dates[0])
	}
	opts.From, opts.To = from, to

	if interval != "" {
		if err := applyInterval(&opts, interval); err != nil {
			return opts, false, err
		}
	}
	if opts.MaxGap <= opts.MinGap {
		return opts, false, fmt.Errorf("间隔上限必须大于下限")
	}
	return opts, confirmed, nil
}

// parseDate accepts YYYY-MM-DD.
func parseDate(value string) (time.Time, error) {
	t, err := time.ParseInLocation("2006-01-02", strings.TrimSpace(value), time.Local)
	if err != nil {
		return time.Time{}, fmt.Errorf("无法识别日期 %q，请使用 YYYY-MM-DD（如 2026-10-01）", value)
	}
	return t, nil
}

// parseWindow parses "21:00-23:00" into offsets from midnight.
func parseWindow(value string) (time.Duration, time.Duration, error) {
	parts := strings.SplitN(value, "-", 2)
	if len(parts) != 2 {
		return 0, 0, fmt.Errorf("时间窗口格式应为 HH:MM-HH:MM，收到 %q", value)
	}
	start, err := parseClock(parts[0])
	if err != nil {
		return 0, 0, err
	}
	end, err := parseClock(parts[1])
	if err != nil {
		return 0, 0, err
	}
	if end <= start {
		return 0, 0, fmt.Errorf("时间窗口的结束必须晚于开始，收到 %q", value)
	}
	return start, end, nil
}

// parseClock parses "21:00" into an offset from midnight.
func parseClock(value string) (time.Duration, error) {
	value = strings.TrimSpace(value)
	h, m, ok := strings.Cut(value, ":")
	if !ok {
		return 0, fmt.Errorf("时间应为 HH:MM，收到 %q", value)
	}
	hour, err1 := strconv.Atoi(h)
	minute, err2 := strconv.Atoi(m)
	if err1 != nil || err2 != nil || hour < 0 || hour > 23 || minute < 0 || minute > 59 {
		return 0, fmt.Errorf("时间应为合法 HH:MM，收到 %q", value)
	}
	return time.Duration(hour)*time.Hour + time.Duration(minute)*time.Minute, nil
}

// applyInterval sets the gap range: either "5-40" (minutes) or "match-remote".
func applyInterval(opts *gitRetimeOptions, value string) error {
	if value == "match-remote" {
		opts.MatchEmote = true
		return nil
	}
	parts := strings.SplitN(value, "-", 2)
	if len(parts) != 2 {
		return fmt.Errorf("--interval 应为 下限-上限（分钟），如 5-40，或 match-remote；收到 %q", value)
	}
	lo, err1 := strconv.Atoi(strings.TrimSpace(parts[0]))
	hi, err2 := strconv.Atoi(strings.TrimSpace(parts[1]))
	if err1 != nil || err2 != nil || lo <= 0 || hi <= 0 {
		return fmt.Errorf("--interval 的分钟数必须是正整数，收到 %q", value)
	}
	if hi <= lo {
		return fmt.Errorf("--interval 上限必须大于下限，收到 %q", value)
	}
	opts.MinGap = time.Duration(lo) * time.Minute
	opts.MaxGap = time.Duration(hi) * time.Minute
	return nil
}

// runGitRetimePreview renders the plan for the confirmation prompt and for
// --dry-run: the same table the command would show, without touching anything.
//
// It never fails with the "needs --yes" exit code — the confirmation prompt is
// already the place that asks for it, so reporting it here would abort the
// prompt instead of showing the plan.
func runGitRetimePreview(ctx context.Context, req BuiltinRequest) error {
	return runGitRetimePreviewWith(ctx, req, execGitRunner{}, time.Now())
}

// runGitRetimePreviewWith is runGitRetimePreview with injected dependencies.
func runGitRetimePreviewWith(ctx context.Context, req BuiltinRequest, runner gitRunner, now time.Time) error {
	err := runGitRetimeWith(ctx, req, runner, now)
	switch {
	case err == nil, errors.Is(err, errNothingToRewrite):
		return nil
	}
	var exitErr *ExitError
	if errors.As(err, &exitErr) && exitErr.Code == 2 {
		return nil
	}
	return err
}
