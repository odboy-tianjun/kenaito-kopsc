package executor

import (
	"fmt"
	"math/rand"
	"sort"
	"time"
)

// retime is one planned change: a commit and the time it should carry.
type retime struct {
	Hash      string
	Short     string
	Subject   string // commit subject, shown in the preview
	OldAuthor time.Time
	OldCommit time.Time
	NewTime   time.Time
	Squeezed  bool // the day's window could not fit the random gaps
}

// retimeOptions controls how new times are chosen.
type retimeOptions struct {
	WindowStart time.Duration // e.g. 21h, measured from midnight
	WindowEnd   time.Duration // e.g. 23h; commits stay strictly before this
	MinGap      time.Duration // shortest gap between two commits on one day
	MaxGap      time.Duration // longest gap between two commits on one day
	FirstGapMax time.Duration // latest start offset of the first commit in a day
	Rand        *rand.Rand

	// Gaps, when non-empty, supplies the gap before each commit (index i > 0)
	// instead of a random value. It is how --interval match-remote mirrors the
	// spacing found on the remote branch. A zero (or out-of-range) entry falls
	// back to a random gap.
	Gaps []time.Duration
}

// defaultRetimeOptions matches the command's documented behaviour: commits land
// between 21:00 and 23:00, 5–40 minutes apart, the first one no later than 20
// minutes after 21:00.
func defaultRetimeOptions(seed int64) retimeOptions {
	return retimeOptions{
		WindowStart: 21 * time.Hour,
		WindowEnd:   23 * time.Hour,
		MinGap:      5 * time.Minute,
		MaxGap:      40 * time.Minute,
		FirstGapMax: 20 * time.Minute,
		Rand:        rand.New(rand.NewSource(seed)),
	}
}

// planRetime assigns a new time to every commit in commits (which must be in
// chronological order, oldest first).
//
// Each commit keeps its own calendar day; only the time of day changes. Within
// a day the commits stay strictly increasing, separated by a random gap in
// [MinGap, MaxGap] with a random second component, so the result looks like
// ordinary hand-made commits rather than a machine grid.
func planRetime(commits []gitCommit, opts retimeOptions) []retime {
	window := opts.WindowEnd - opts.WindowStart
	out := make([]retime, 0, len(commits))

	// Group by calendar day, preserving order.
	var dayCommits []gitCommit
	currentDay := time.Time{}
	flush := func() {
		if len(dayCommits) == 0 {
			return
		}
		out = append(out, planDay(dayCommits, opts, window)...)
		dayCommits = nil
	}

	for _, c := range commits {
		day := c.Date()
		if !currentDay.IsZero() && !day.Equal(currentDay) {
			flush()
		}
		currentDay = day
		dayCommits = append(dayCommits, c)
	}
	flush()
	return out
}

// planDay spreads one day's commits across the window.
func planDay(commits []gitCommit, opts retimeOptions, window time.Duration) []retime {
	out := make([]retime, 0, len(commits))
	day := commits[0].Date()

	offsets, squeezed := spreadWithin(day, len(commits), opts, window)

	for i, c := range commits {
		// Offsets are measured from the window start (21:00), not midnight.
		newTime := withOffset(day, opts.WindowStart+offsets[i])
		out = append(out, retime{
			Hash:      c.Hash,
			Short:     c.Short,
			Subject:   c.Subject,
			OldAuthor: c.AuthorTime,
			OldCommit: c.CommitTime,
			NewTime:   newTime,
			Squeezed:  squeezed,
		})
	}
	return out
}

// spreadWithin returns the time-of-day offset (with sub-minute precision) for
// each commit of a day.
//
// The first commit starts within [0, FirstGapMax]; each following commit moves
// forward by a random gap in [MinGap, MaxGap] plus random seconds. If that
// would leave the window, the day is compressed: the commits are spread evenly
// across the window instead, and flagged as squeezed.
// It also reports whether the day had to be compressed.
func spreadWithin(day time.Time, count int, opts retimeOptions, window time.Duration) ([]time.Duration, bool) {
	offsets := make([]time.Duration, count)
	if count == 0 {
		return offsets, false
	}

	first := randomDuration(opts.Rand, 0, opts.FirstGapMax)
	offsets[0] = first
	for i := 1; i < count; i++ {
		offsets[i] = offsets[i-1] + gapFor(i, opts)
	}

	// Everything must stay strictly before WindowEnd, so a day the window
	// cannot hold falls back to an even spread across it.
	if offsets[count-1] < window {
		return offsets, false
	}

	// Even spread: count slots of window/count, starting at the window start
	// so no commit drifts past the end.
	step := window / time.Duration(count)
	if step <= 0 {
		step = time.Second
	}
	for i := range offsets {
		// Keep a random second component so a compressed day still does not
		// look like a perfect grid.
		jitter := time.Duration(opts.Rand.Int63n(int64(step / 2)))
		offsets[i] = time.Duration(i)*step + jitter
	}
	return offsets, true
}

// gapFor returns the gap before the i-th commit of a day. A supplied gap is
// used when it lands inside [MinGap, MaxGap]; otherwise the gap is random, so a
// remote branch whose intervals are wildly different cannot push commits out
// of the day's window.
func gapFor(i int, opts retimeOptions) time.Duration {
	if i < len(opts.Gaps) {
		if g := opts.Gaps[i]; g >= opts.MinGap && g <= opts.MaxGap {
			return g
		}
	}
	return randomDuration(opts.Rand, opts.MinGap, opts.MaxGap)
}

// remoteGaps converts the spacing between consecutive commits on the remote
// branch into the per-commit gap list consumed by retimeOptions.Gaps.
//
// remote is expected in the same order as commits (oldest first); when it
// cannot be paired up, an empty list is returned and the plan falls back to
// random gaps.
func remoteGaps(remote []time.Time) []time.Duration {
	if len(remote) < 2 {
		return nil
	}
	gaps := make([]time.Duration, len(remote))
	for i := 1; i < len(remote); i++ {
		gap := remote[i].Sub(remote[i-1])
		if gap <= 0 {
			return nil // the remote history is not ordered; do not use it
		}
		gaps[i] = gap
	}
	return gaps
}

// randomDuration returns a random duration in [min, max] with second precision.
func randomDuration(r *rand.Rand, min, max time.Duration) time.Duration {
	if max <= min {
		return min
	}
	span := int64(max - min)
	// Random second component, then whole milliseconds within the span.
	offset := time.Duration(r.Int63n(span)) + time.Duration(r.Int63n(int64(time.Second)))
	if offset >= max-min {
		offset = max - min - time.Nanosecond
	}
	// Truncate to whole seconds: git stores "minutes + seconds" of wall time.
	return min + offset.Truncate(time.Second)
}

// withOffset returns midnight of day plus offset.
func withOffset(day time.Time, offset time.Duration) time.Time {
	return day.Add(offset)
}

// sortRetimesOldestFirst sorts a plan in commit order (oldest first), keeping
// the relationship the rewrite step needs.
func sortRetimesOldestFirst(plans []retime) {
	sort.SliceStable(plans, func(i, j int) bool {
		return plans[i].OldAuthor.Before(plans[j].OldAuthor)
	})
}

// summarizeRetime renders a plan as "21:07:43 -> 21:32:11".
func summarizeRetime(r retime) string {
	return fmt.Sprintf("%s -> %s", r.OldAuthor.Format("15:04:05"), r.NewTime.Format("15:04:05"))
}

// retimeWindow is the longest gap between two commits kopsc will produce.
// Kept here so preview, validation and the rewrite all describe the same thing.
type retimePlan struct {
	Commits  []retime    // oldest first
	Branch   string      // branch being rewritten
	Remote   string      // remote to force-push to
	Squeezed []time.Time // days whose window could not fit the gaps
}

// buildPlan collects one plan used by both the preview and the rewrite, so the
// times shown to the user are exactly the times that get committed.
func buildPlan(commits []gitCommit, opts retimeOptions, branch, remote string) retimePlan {
	// git log lists newest first; the planner works oldest first.
	ordered := make([]gitCommit, len(commits))
	for i, c := range commits {
		ordered[len(commits)-1-i] = c
	}

	plan := retimePlan{Commits: planRetime(ordered, opts), Branch: branch, Remote: remote}

	seen := map[string]bool{}
	for _, r := range plan.Commits {
		if !r.Squeezed {
			continue
		}
		day := r.NewTime.Format("2006-01-02")
		if !seen[day] {
			seen[day] = true
			plan.Squeezed = append(plan.Squeezed, r.NewTime)
		}
	}
	return plan
}
