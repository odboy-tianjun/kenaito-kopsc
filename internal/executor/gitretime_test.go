package executor

import (
	"math/rand"
	"testing"
	"time"
)

var cst = time.FixedZone("CST", 8*3600)

// commitAt builds a commit whose author time is at hour:minute on date.
func commitAt(short string, date time.Time, hour, minute, second int) gitCommit {
	ts := time.Date(date.Year(), date.Month(), date.Day(), hour, minute, second, 0, cst)
	return gitCommit{Hash: short + "-full", Short: short, AuthorTime: ts, CommitTime: ts, Subject: "subject " + short}
}

func day(y int, m time.Month, d int) time.Time { return time.Date(y, m, d, 0, 0, 0, 0, cst) }

func TestPlanRetimeKeepsDateAndWindow(t *testing.T) {
	// Three commits spread over two days.
	commits := []gitCommit{
		commitAt("a1", day(2026, 10, 1), 10, 15, 0),
		commitAt("a2", day(2026, 10, 1), 18, 40, 0),
		commitAt("b1", day(2026, 10, 2), 9, 0, 0),
	}
	opts := defaultRetimeOptions(42)
	plan := planRetime(commits, opts)

	if len(plan) != 3 {
		t.Fatalf("plan has %d entries, want 3", len(plan))
	}
	for i, r := range plan {
		// The calendar day never changes.
		wantDay := commits[i].Date()
		gotDay := r.NewTime.In(cst)
		if gotDay.Year() != wantDay.Year() || gotDay.Month() != wantDay.Month() || gotDay.Day() != wantDay.Day() {
			t.Errorf("commit %s moved to another day: %v -> %v", r.Short, r.OldAuthor, r.NewTime)
		}
		// The time of day lands in [21:00, 23:00).
		hm := r.NewTime.In(cst).Hour()*60 + r.NewTime.In(cst).Minute()
		if hm < 21*60 || hm >= 23*60 {
			t.Errorf("commit %s outside 21:00-23:00: %v", r.Short, r.NewTime.In(cst))
		}
	}
}

func TestPlanRetimeIsStrictlyIncreasingWithinADay(t *testing.T) {
	commits := []gitCommit{
		commitAt("a1", day(2026, 10, 1), 8, 0, 0),
		commitAt("a2", day(2026, 10, 1), 9, 0, 0),
		commitAt("a3", day(2026, 10, 1), 10, 0, 0),
	}
	plan := planRetime(commits, defaultRetimeOptions(7))

	for i := 1; i < len(plan); i++ {
		if !plan[i].NewTime.After(plan[i-1].NewTime) {
			t.Fatalf("commit %d (%v) is not after commit %d (%v)",
				i, plan[i].NewTime, i-1, plan[i-1].NewTime)
		}
	}
}

func TestPlanRetimePreservesCommitOrder(t *testing.T) {
	commits := []gitCommit{
		commitAt("a1", day(2026, 10, 1), 23, 30, 0),
		commitAt("a2", day(2026, 10, 2), 1, 0, 0),
		commitAt("a3", day(2026, 10, 2), 2, 0, 0),
	}
	plan := planRetime(commits, defaultRetimeOptions(3))
	for i, r := range plan {
		if r.Short != commits[i].Short {
			t.Fatalf("plan[%d] = %s, want %s (order must be preserved)", i, r.Short, commits[i].Short)
		}
	}
	// Even across the midnight boundary the plan stays ascending.
	for i := 1; i < len(plan); i++ {
		if !plan[i].NewTime.After(plan[i-1].NewTime) {
			t.Fatalf("plan is not ascending at %d", i)
		}
	}
}

func TestRandomDurationBounds(t *testing.T) {
	r := newTestRand(1)
	min, max := 5*time.Minute, 40*time.Minute
	sawSecond := false
	for i := 0; i < 500; i++ {
		d := randomDuration(r, min, max)
		if d < min || d >= max {
			t.Fatalf("duration %v outside [%v, %v)", d, min, max)
		}
		if d%time.Minute != 0 {
			sawSecond = true
		}
	}
	// The gaps must not sit on minute boundaries, or the commits look machine
	// generated (every timestamp at :00 seconds).
	if !sawSecond {
		t.Error("randomDuration never produced a non-zero second component")
	}
}

func TestPlanRetimeFirstCommitStartsSoonAfterWindowStart(t *testing.T) {
	r := newTestRand(11)
	for i := 0; i < 100; i++ {
		plan := planRetime([]gitCommit{commitAt("a1", day(2026, 10, 1), 9, 0, 0)}, defaultRetimeOptions(int64(i)))
		ts := plan[0].NewTime.In(cst)
		if ts.Hour() < 21 || ts.Hour() > 21 || ts.Minute() > 20 {
			t.Fatalf("first commit at %v, want between 21:00 and 21:20", ts)
		}
	}
	_ = r
}

func TestPlanRetimeGapsStayWithinRange(t *testing.T) {
	// Six commits on one day, comfortably inside the 120-minute window.
	date := day(2026, 10, 1)
	var commits []gitCommit
	for i := 0; i < 6; i++ {
		commits = append(commits, commitAt("c"+string(rune('1'+i)), date, 9, i, 0))
	}
	plan := planRetime(commits, defaultRetimeOptions(99))
	for i := 1; i < len(plan); i++ {
		gap := plan[i].NewTime.Sub(plan[i-1].NewTime)
		if gap < 5*time.Minute || gap > 40*time.Minute {
			t.Errorf("gap %d = %v, want within [5m, 40m]", i, gap)
		}
	}
}

func TestPlanRetimeSeedIsReproducible(t *testing.T) {
	commits := []gitCommit{
		commitAt("a1", day(2026, 10, 1), 9, 0, 0),
		commitAt("a2", day(2026, 10, 1), 10, 0, 0),
	}
	first := planRetime(commits, defaultRetimeOptions(2024))
	second := planRetime(commits, defaultRetimeOptions(2024))
	for i := range first {
		if !first[i].NewTime.Equal(second[i].NewTime) {
			t.Fatalf("same seed produced different times: %v vs %v", first[i].NewTime, second[i].NewTime)
		}
	}
}

// newTestRand returns a deterministic source for repeatable tests.
func newTestRand(seed int64) *rand.Rand { return rand.New(rand.NewSource(seed)) }

func TestPlanRetimeCompressesWhenWindowIsFull(t *testing.T) {
	// 30 commits on one day cannot fit 5-minute gaps into 120 minutes
	// (29 * 5 = 145), so the day must be compressed.
	date := day(2026, 10, 1)
	var commits []gitCommit
	for i := 0; i < 30; i++ {
		commits = append(commits, commitAt("c", date, 9, 0, i))
	}

	plan := planRetime(commits, defaultRetimeOptions(5))
	for i, r := range plan {
		ts := r.NewTime.In(cst)
		if ts.Day() != 1 || ts.Hour() < 21 || ts.Hour() >= 23 {
			t.Fatalf("commit %d at %v, outside the day's 21:00-23:00 window", i, ts)
		}
		if i > 0 && !r.NewTime.After(plan[i-1].NewTime) {
			t.Fatalf("commit %d is not after the previous one", i)
		}
		// Every commit of a compressed day is flagged, so the preview can say so.
		if !r.Squeezed {
			t.Fatalf("commit %d should be marked as squeezed", i)
		}
	}
}

func TestPlanRetimeNotSqueezedWhenThereIsRoom(t *testing.T) {
	date := day(2026, 10, 1)
	commits := []gitCommit{
		commitAt("a1", date, 9, 0, 0),
		commitAt("a2", date, 10, 0, 0),
	}
	for _, r := range planRetime(commits, defaultRetimeOptions(5)) {
		if r.Squeezed {
			t.Fatalf("two commits should fit comfortably, got squeezed %v", r.NewTime)
		}
	}
}

func TestSpreadWithinCompressedStaysInsideWindow(t *testing.T) {
	offsets, squeezed := spreadWithin(day(2026, 10, 1), 40, defaultRetimeOptions(1), 2*time.Hour)
	if !squeezed {
		t.Fatal("40 commits cannot fit 5-minute gaps in 2 hours")
	}
	for i, off := range offsets {
		if off < 0 || off >= 2*time.Hour {
			t.Fatalf("offset %d = %v, outside the window", i, off)
		}
		if i > 0 && off <= offsets[i-1] {
			t.Fatalf("offsets are not increasing at %d", i)
		}
	}
}

func TestRemoteGaps(t *testing.T) {
	base := time.Date(2026, 10, 1, 9, 0, 0, 0, cst)
	remote := []time.Time{
		base,
		base.Add(12 * time.Minute),
		base.Add(12*time.Minute + 30*time.Minute),
	}
	gaps := remoteGaps(remote)
	if len(gaps) != 3 {
		t.Fatalf("gaps = %v", gaps)
	}
	if gaps[1] != 12*time.Minute || gaps[2] != 30*time.Minute {
		t.Fatalf("gaps = %v, want 12m then 30m", gaps)
	}

	// Too few commits or out-of-order history yields no gaps (random fallback).
	if got := remoteGaps(remote[:1]); got != nil {
		t.Errorf("single commit should yield nil gaps, got %v", got)
	}
	if got := remoteGaps([]time.Time{base.Add(time.Hour), base}); got != nil {
		t.Errorf("unsorted history should yield nil gaps, got %v", got)
	}
}

func TestPlanRetimeUsesSuppliedGaps(t *testing.T) {
	date := day(2026, 10, 1)
	commits := []gitCommit{
		commitAt("a1", date, 9, 0, 0),
		commitAt("a2", date, 10, 0, 0),
		commitAt("a3", date, 11, 0, 0),
	}
	opts := defaultRetimeOptions(1)
	opts.Gaps = []time.Duration{0, 12 * time.Minute, 30 * time.Minute}

	plan := planRetime(commits, opts)
	if got := plan[1].NewTime.Sub(plan[0].NewTime); got != 12*time.Minute {
		t.Errorf("gap 1 = %v, want 12m", got)
	}
	if got := plan[2].NewTime.Sub(plan[1].NewTime); got != 30*time.Minute {
		t.Errorf("gap 2 = %v, want 30m", got)
	}
}

func TestPlanRetimeFallsBackWhenSuppliedGapIsOutOfRange(t *testing.T) {
	date := day(2026, 10, 1)
	commits := []gitCommit{
		commitAt("a1", date, 9, 0, 0),
		commitAt("a2", date, 10, 0, 0),
	}
	opts := defaultRetimeOptions(1)
	// A 3-hour gap from the remote branch would leave the 21:00-23:00 window,
	// so it must be rejected in favour of a random gap.
	opts.Gaps = []time.Duration{0, 3 * time.Hour}

	gap := planRetime(commits, opts)[1].NewTime.Sub(planRetime(commits, opts)[0].NewTime)
	if gap < 5*time.Minute || gap > 40*time.Minute {
		t.Fatalf("gap = %v, want a random gap inside [5m, 40m]", gap)
	}
}

func TestPlanRetimeHonoursSeed(t *testing.T) {
	date := day(2026, 10, 1)
	commits := []gitCommit{
		commitAt("a1", date, 9, 0, 0),
		commitAt("a2", date, 10, 0, 0),
		commitAt("a3", date, 11, 0, 0),
	}
	a := planRetime(commits, defaultRetimeOptions(7))
	b := planRetime(commits, defaultRetimeOptions(7))
	c := planRetime(commits, defaultRetimeOptions(8))
	for i := range a {
		if !a[i].NewTime.Equal(b[i].NewTime) {
			t.Fatalf("seed 7 is not reproducible at %d: %v vs %v", i, a[i].NewTime, b[i].NewTime)
		}
	}
	same := true
	for i := range a {
		if !a[i].NewTime.Equal(c[i].NewTime) {
			same = false
		}
	}
	if same {
		t.Error("different seeds produced identical times")
	}
}

func TestBuildPlanOrdersOldestFirstAndRecordsSqueezedDays(t *testing.T) {
	date := day(2026, 10, 1)
	// git log hands commits newest first; buildPlan must flip them.
	newestFirst := []gitCommit{
		commitAt("c3", date, 11, 0, 0),
		commitAt("c2", date, 10, 0, 0),
		commitAt("c1", date, 9, 0, 0),
	}
	plan := buildPlan(newestFirst, defaultRetimeOptions(1), "main", "origin")

	if len(plan.Commits) != 3 {
		t.Fatalf("plan has %d commits", len(plan.Commits))
	}
	wantOrder := []string{"c1", "c2", "c3"}
	for i, want := range wantOrder {
		if plan.Commits[i].Short != want {
			t.Fatalf("plan order = %v, want oldest first", []string{
				plan.Commits[0].Short, plan.Commits[1].Short, plan.Commits[2].Short})
		}
	}
	if plan.Branch != "main" || plan.Remote != "origin" {
		t.Errorf("plan branch/remote = %q/%q", plan.Branch, plan.Remote)
	}
	// Three commits fit comfortably, so no day is flagged.
	if len(plan.Squeezed) != 0 {
		t.Errorf("unexpected squeezed days: %v", plan.Squeezed)
	}
}

func TestBuildPlanListsEachSqueezedDayOnce(t *testing.T) {
	date := day(2026, 10, 2)
	var newestFirst []gitCommit
	for i := 29; i >= 0; i-- {
		newestFirst = append(newestFirst, commitAt("c", date, 9, 0, i))
	}
	plan := buildPlan(newestFirst, defaultRetimeOptions(3), "main", "origin")
	if len(plan.Squeezed) != 1 {
		t.Fatalf("squeezed days = %v, want exactly one entry for 2026-10-02", plan.Squeezed)
	}
	if got := plan.Squeezed[0].Format("2006-01-02"); got != "2026-10-02" {
		t.Errorf("squeezed day = %q", got)
	}
}
