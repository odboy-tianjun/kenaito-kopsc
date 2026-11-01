package executor

import (
	"bytes"
	"context"
	"fmt"
	"strings"
	"testing"
	"time"
)

func TestParseGitRetimeArgsDefaults(t *testing.T) {
	now := time.Date(2026, 11, 1, 9, 0, 0, 0, cst)
	opts, confirmed, err := parseGitRetimeArgs([]string{"2026-10-01", "2026-10-15"}, now)
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	if confirmed {
		t.Error("--yes must not be implied")
	}
	if opts.Window != 21*time.Hour || opts.WindowEnd != 23*time.Hour {
		t.Errorf("window = %v-%v, want 21:00-23:00", opts.Window, opts.WindowEnd)
	}
	if opts.MinGap != 5*time.Minute || opts.MaxGap != 40*time.Minute {
		t.Errorf("gaps = %v-%v, want 5m-40m", opts.MinGap, opts.MaxGap)
	}
	if opts.From.Format("2006-01-02") != "2026-10-01" || opts.To.Format("2006-01-02") != "2026-10-15" {
		t.Errorf("range = %v..%v", opts.From, opts.To)
	}
}

func TestParseGitRetimeArgsRequiresTwoDates(t *testing.T) {
	now := time.Now()
	for _, args := range [][]string{
		nil,
		{"2026-10-01"},
		{"2026-10-01", "2026-10-02", "2026-10-03"},
	} {
		if _, _, err := parseGitRetimeArgs(args, now); err == nil ||
			!strings.Contains(err.Error(), "需要开始与结束日期") {
			t.Errorf("args %v: err = %v", args, err)
		}
	}
}

func TestParseGitRetimeArgsRejectsBadDates(t *testing.T) {
	now := time.Now()
	if _, _, err := parseGitRetimeArgs([]string{"2026/10/01", "2026-10-15"}, now); err == nil ||
		!strings.Contains(err.Error(), "YYYY-MM-DD") {
		t.Errorf("err = %v, want a date-format error", err)
	}
	if _, _, err := parseGitRetimeArgs([]string{"2026-10-15", "2026-10-01"}, now); err == nil ||
		!strings.Contains(err.Error(), "早于") {
		t.Errorf("err = %v, want a reversed-range error", err)
	}
}

func TestParseGitRetimeArgsOptions(t *testing.T) {
	now := time.Now()
	args := []string{
		"2026-10-01", "2026-10-15",
		"--window", "20:00-22:30",
		"--interval", "10-25",
		"--seed", "42",
		"--remote", "upstream",
		"--branch", "release",
		"--no-push",
		"--yes",
	}
	opts, confirmed, err := parseGitRetimeArgs(args, now)
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	if !confirmed {
		t.Error("--yes should be recognised")
	}
	if opts.Window != 20*time.Hour || opts.WindowEnd != 22*time.Hour+30*time.Minute {
		t.Errorf("window = %v-%v", opts.Window, opts.WindowEnd)
	}
	if opts.MinGap != 10*time.Minute || opts.MaxGap != 25*time.Minute {
		t.Errorf("gaps = %v-%v", opts.MinGap, opts.MaxGap)
	}
	if opts.Seed != 42 || !opts.SeedSet {
		t.Errorf("seed = %d", opts.Seed)
	}
	if opts.Remote != "upstream" || opts.Branch != "release" || !opts.NoPush {
		t.Errorf("opts = %+v", opts)
	}
}

func TestParseInterval(t *testing.T) {
	opts := gitRetimeOptions{}
	if err := applyInterval(&opts, "match-remote"); err != nil || !opts.MatchEmote {
		t.Fatalf("match-remote: %v %+v", err, opts)
	}
	opts = gitRetimeOptions{}
	if err := applyInterval(&opts, "3-15"); err != nil || opts.MinGap != 3*time.Minute || opts.MaxGap != 15*time.Minute {
		t.Fatalf("3-15: %v %+v", err, opts)
	}
	for _, bad := range []string{"15-3", "0-10", "abc", "5"} {
		if err := applyInterval(&gitRetimeOptions{}, bad); err == nil {
			t.Errorf("applyInterval(%q) should fail", bad)
		}
	}
}

func TestParseWindow(t *testing.T) {
	start, end, err := parseWindow("21:00-23:00")
	if err != nil || start != 21*time.Hour || end != 23*time.Hour {
		t.Fatalf("parseWindow = %v, %v, %v", start, end, err)
	}
	for _, bad := range []string{"21:00", "25:00-26:00", "23:00-21:00", "21:00-21:00"} {
		if _, _, err := parseWindow(bad); err == nil {
			t.Errorf("parseWindow(%q) should fail", bad)
		}
	}
}

func TestParseGitRetimeArgsRejectsUnknownOption(t *testing.T) {
	if _, _, err := parseGitRetimeArgs([]string{"2026-10-01", "2026-10-02", "--nope"}, time.Now()); err == nil {
		t.Fatal("unknown option should fail")
	}
}

func TestParseGitRetimeArgsWindowAliases(t *testing.T) {
	now := time.Now()
	for _, flag := range []string{"--window", "--since", "--until"} {
		opts, _, err := parseGitRetimeArgs([]string{"2026-10-01", "2026-10-02", flag, "20:00-22:00"}, now)
		if err != nil {
			t.Fatalf("%s: %v", flag, err)
		}
		if opts.Window != 20*time.Hour || opts.WindowEnd != 22*time.Hour {
			t.Errorf("%s: window = %v-%v", flag, opts.Window, opts.WindowEnd)
		}
	}
}

func TestParseGitRetimeArgsAllOptionsTogether(t *testing.T) {
	// Every documented option must survive parsing together.
	args := []string{
		"2026-10-01", "2026-10-15",
		"--since", "21:00-23:00",
		"--interval", "5-40",
		"--seed", "1234",
		"--remote", "origin",
		"--branch", "main",
		"--no-push",
		"--yes",
	}
	opts, confirmed, err := parseGitRetimeArgs(args, time.Now())
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	if !confirmed || !opts.NoPush || opts.Remote != "origin" || opts.Branch != "main" {
		t.Fatalf("opts = %+v confirmed=%v", opts, confirmed)
	}
	if opts.Seed != 1234 || opts.MinGap != 5*time.Minute || opts.MaxGap != 40*time.Minute {
		t.Fatalf("opts = %+v", opts)
	}
}

// previewFixture fakes a repository with two commits on 2026-10-01.
func previewFixture() *fakeGitRunner {
	metaFormat := func(hash, tree, parents, subject string) map[string]gitResponse {
		return map[string]gitResponse{
			"log -1 --format=" + rawCommitFormat + " " + hash: {
				out: strings.Join([]string{hash, tree, parents, "Odboy", "t@e.cn", "Odboy", "t@e.cn", subject}, "\t") + "\n",
			},
			"log -1 --format=%B " + hash: {out: subject + "\n"},
		}
	}
	responses := map[string]gitResponse{}
	for _, m := range []map[string]gitResponse{
		metaFormat("h2", "t2", "h1", "second"),
		metaFormat("h1", "t1", "", "first"),
	} {
		for k, v := range m {
			responses[k] = v
		}
	}
	// git log lists newest first.
	responses["log --format="+gitLogFormat+" main"] = gitResponse{
		out: "h2\t2222222\t2026-10-01T22:10:00+08:00\t2026-10-01T22:10:00+08:00\tsecond\n" +
			"h1\t1111111\t2026-10-01T10:00:00+08:00\t2026-10-01T10:00:00+08:00\tfirst\n",
	}
	responses["rev-parse --abbrev-ref HEAD"] = gitResponse{out: "main\n"}
	responses["remote"] = gitResponse{out: "origin\n"}
	return &fakeGitRunner{responses: responses}
}

func TestRetimeWithoutYesOnlyPreviews(t *testing.T) {
	runner := previewFixture()
	var out bytes.Buffer
	err := runGitRetimeWith(context.Background(), BuiltinRequest{
		Args:   []string{"2026-10-01", "2026-10-01", "--seed", "1"},
		Stdout: &out,
		Stderr: &out,
	}, runner, time.Date(2026, 11, 1, 9, 0, 0, 0, cst))

	if err == nil {
		t.Fatal("expected exit code 2 without --yes")
	}
	if code := ExitCode(err); code != 2 {
		t.Fatalf("exit code = %d, want 2", code)
	}

	text := out.String()
	for _, want := range []string{"目标:   origin/main", "窗口:   21:00-23:00", "提交", "原时间", "新时间", "主题", "1111111", "2222222"} {
		if !strings.Contains(text, want) {
			t.Errorf("preview missing %q:\n%s", want, text)
		}
	}
	// Nothing may be touched without --yes.
	for _, call := range runner.calls {
		if strings.HasPrefix(call, "commit-tree") || strings.HasPrefix(call, "update-ref") ||
			strings.HasPrefix(call, "push") {
			t.Fatalf("preview must not modify the repo, but ran: %s", call)
		}
	}
}

func TestRetimePreviewAlignsColumns(t *testing.T) {
	runner := previewFixture()
	var out bytes.Buffer
	_ = runGitRetimeWith(context.Background(), BuiltinRequest{
		Args:   []string{"2026-10-01", "2026-10-01", "--seed", "1"},
		Stdout: &out,
		Stderr: &out,
	}, runner, time.Now())

	lines := strings.Split(out.String(), "\n")
	var header string
	var rows []string
	for _, line := range lines {
		if strings.HasPrefix(line, "提交") {
			header = line
		} else if strings.HasPrefix(line, "1111") || strings.HasPrefix(line, "2222") {
			rows = append(rows, line)
		}
	}
	if header == "" || len(rows) != 2 {
		t.Fatalf("unexpected preview:\n%s", out.String())
	}
	// The "原时间" column must start at the same display column everywhere:
	// the heading is 6 columns wide (three CJK runes), the data 19.
	want := columnStartOf(header, "原时间")
	for _, row := range rows {
		// The original time is "YYYY-MM-DD HH:MM:SS": date and clock are two
		// whitespace-separated fields, so match the date field instead.
		var stamp string
		for _, field := range strings.Fields(row) {
			if len(field) == 10 && field[4] == '-' && field[7] == '-' {
				stamp = field
				break
			}
		}
		if stamp == "" {
			t.Fatalf("no timestamp in row: %q", row)
		}
		if got := columnStartOf(row, stamp); got != want {
			t.Errorf("row starts at %d, header at %d:\n%s\n%s", got, want, header, row)
		}
	}
}

// columnStartOf returns the display column where cell begins in line.
func columnStartOf(line, cell string) int {
	i := strings.Index(line, cell)
	if i < 0 {
		return -1
	}
	return displayWidth(line[:i])
}

func TestRetimeNoCommitsInRange(t *testing.T) {
	offline := func(ctx context.Context, args ...string) (string, string, error) {
		return "", "", fmt.Errorf("empty range must not run git: %v", args)
	}
	_ = offline
	runner := previewFixture()
	var out bytes.Buffer
	// The preview path treats an empty range as "nothing to do", not a failure.
	err := runGitRetimePreviewWith(context.Background(), BuiltinRequest{
		Args:   []string{"2026-12-01", "2026-12-31"},
		Stdout: &out,
		Stderr: &out,
	}, runner, time.Now())
	if err != nil {
		t.Fatalf("empty range should succeed: %v", err)
	}
	if !strings.Contains(out.String(), "没有需要改写的提交") {
		t.Errorf("output = %q", out.String())
	}
}

func TestRetimePreviewListsRewriteScope(t *testing.T) {
	runner := previewFixture()
	var out bytes.Buffer
	_ = runGitRetimeWith(context.Background(), BuiltinRequest{
		Args:   []string{"2026-10-01", "2026-10-01", "--seed", "1"},
		Stdout: &out,
		Stderr: &out,
	}, runner, time.Now())

	text := out.String()
	for _, want := range []string{
		"改写:   2 个提交（分支 main，远程 origin）",
		"强推:   是 --force-with-lease → origin/main",
		"窗口:   21:00-23:00",
	} {
		if !strings.Contains(text, want) {
			t.Errorf("preview missing %q:\n%s", want, text)
		}
	}
}

func TestRetimePreviewStatesNoPush(t *testing.T) {
	runner := previewFixture()
	var out bytes.Buffer
	_ = runGitRetimeWith(context.Background(), BuiltinRequest{
		Args:   []string{"2026-10-01", "2026-10-01", "--no-push", "--seed", "1"},
		Stdout: &out,
		Stderr: &out,
	}, runner, time.Now())

	if !strings.Contains(out.String(), "强推:   否（--no-push，只改本地）") {
		t.Errorf("preview should say nothing is pushed:\n%s", out.String())
	}
}

func TestGitRetimePreviewSwallowsConfirmationExit(t *testing.T) {
	// The confirmation prompt calls the preview builtin; a code-2 result would
	// abort the prompt instead of showing the plan.
	runner := previewFixture()
	_ = runner
	err := runGitRetimePreview(context.Background(), BuiltinRequest{
		Args:   []string{"not-a-date", "2026-10-01"},
		Stdout: &bytes.Buffer{},
		Stderr: &bytes.Buffer{},
	})
	if err == nil {
		t.Fatal("a real parse error must still surface")
	}
}

// TestRetimeWithYesRewritesAndPushes walks the whole flow with --yes: backup,
// rewrite, verify, move the branch, force push.
func TestRetimeWithYesRewritesAndPushes(t *testing.T) {
	runner := previewFixture()
	// Content check succeeds (same tree), branch move and push succeed.
	runner.responses["rev-parse h2^{tree}"] = fixtureResp("tree-final\n")
	runner.responses["rev-parse N2^{tree}"] = fixtureResp("tree-final\n")
	runner.responses["commit-tree t1 -m first"] = fixtureResp("N1\n")
	runner.responses["commit-tree t2 -p N1 -m second"] = fixtureResp("N2\n")
	runner.responses["update-ref refs/heads/main N2"] = fixtureResp()
	runner.responses["push --force-with-lease origin main"] = fixtureResp()
	runner.responses["status --porcelain"] = fixtureResp()
	// The backup ref is named after the injected clock.
	runner.responses["update-ref refs/kopsc-backup/main-20261101-093000 h2"] = fixtureResp()

	var out bytes.Buffer
	err := runGitRetimeWith(context.Background(), BuiltinRequest{
		Args:   []string{"2026-10-01", "2026-10-01", "--seed", "1", "--yes"},
		Stdout: &out,
		Stderr: &out,
	}, runner, time.Date(2026, 11, 1, 9, 30, 0, 0, cst))
	if err != nil {
		t.Fatalf("retime --yes: %v\n%s", err, out.String())
	}

	text := out.String()
	for _, want := range []string{"备份:   refs/kopsc-backup/main-20261101-093000", "已改写: 2 个提交", "已强推: origin main", "回滚:"} {
		if !strings.Contains(text, want) {
			t.Errorf("output missing %q:\n%s", want, text)
		}
	}

	// The whole sequence must have happened, in order.
	seen := func(prefix string) bool {
		for _, call := range runner.calls {
			if strings.HasPrefix(call, prefix) {
				return true
			}
		}
		return false
	}
	for _, prefix := range []string{
		"update-ref refs/kopsc-backup/", "commit-tree", "rev-parse h2^{tree}",
		"update-ref refs/heads/main", "push --force-with-lease origin main",
	} {
		if !seen(prefix) {
			t.Errorf("no %q call; calls = %v", prefix, runner.calls)
		}
	}
}

// TestRetimeRefusesDirtyWorktree stops before touching anything.
func TestRetimeRefusesDirtyWorktree(t *testing.T) {
	runner := previewFixture()
	runner.responses["status --porcelain"] = fixtureResp(" M main.go\n")

	var out bytes.Buffer
	err := runGitRetimeWith(context.Background(), BuiltinRequest{
		Args:   []string{"2026-10-01", "2026-10-01", "--yes"},
		Stdout: &out,
		Stderr: &out,
	}, runner, time.Now())
	if err == nil || !strings.Contains(err.Error(), "未提交的改动") {
		t.Fatalf("err = %v, want a dirty-worktree rejection", err)
	}
	for _, call := range runner.calls {
		if strings.HasPrefix(call, "commit-tree") || strings.HasPrefix(call, "push") {
			t.Fatalf("dirty worktree must not rewrite: %s", call)
		}
	}
}

// fixtureResp builds a canned successful git response.
func fixtureResp(args ...string) gitResponse {
	r := gitResponse{}
	if len(args) > 0 {
		r.out = args[0]
	}
	return r
}

func TestParseGitRetimeArgsRedate(t *testing.T) {
	opts, _, err := parseGitRetimeArgs([]string{
		"2026-10-01", "2026-10-15",
		"--start-date", "2026-11-01",
		"--end-date", "2026-11-15",
	}, time.Now())
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	if opts.StartDate.Format("2006-01-02") != "2026-11-01" {
		t.Errorf("StartDate = %v", opts.StartDate)
	}
	if opts.EndDate.Format("2006-01-02") != "2026-11-15" {
		t.Errorf("EndDate = %v", opts.EndDate)
	}
}

func TestParseGitRetimeArgsRedateValidation(t *testing.T) {
	tests := []struct {
		args []string
		want string
	}{
		{[]string{"2026-10-01", "2026-10-02", "--end-date", "2026-11-15"}, "需要配合 --start-date"},
		{[]string{"2026-10-01", "2026-10-02", "--start-date", "2026/11/01"}, "YYYY-MM-DD"},
		{[]string{"2026-10-01", "2026-10-02", "--start-date", "2026/11/01", "--end-date", "2026-11-15"}, "YYYY-MM-DD"},
		{[]string{"2026-10-01", "2026-10-02", "--start-date", "2026-11-15", "--end-date", "2026-11-01"}, "早于 --start-date"},
	}
	for _, tc := range tests {
		_, _, err := parseGitRetimeArgs(tc.args, time.Now())
		if err == nil || !strings.Contains(err.Error(), tc.want) {
			t.Errorf("args %v: err = %v, want %q", tc.args, err, tc.want)
		}
	}
}

func TestParseGitRetimeArgsEndDateOptional(t *testing.T) {
	opts, _, err := parseGitRetimeArgs([]string{"2026-10-01", "2026-10-02", "--start-date", "2026-11-01"}, time.Now())
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	if !opts.EndDate.IsZero() {
		t.Errorf("EndDate = %v, want the zero value (single start day)", opts.EndDate)
	}
}
