// Package resolve picks the most specific registry variant for a detected
// platform, and explains why each candidate did or did not match.
package resolve

import (
	"errors"
	"fmt"
	"strings"

	"gitea.odboy.cn/odboy/kenaito-kopsc/internal/platform"
	"gitea.odboy.cn/odboy/kenaito-kopsc/internal/registry"
)

// Scoring weights. A variant must match on every declared constraint; the
// score decides among the variants that do match.
const (
	scoreDistroExact = 3
	scoreDistroLike  = 2
	scoreRuntime     = 2
)

// ErrNoMatch is returned when no variant fits the environment.
var ErrNoMatch = errors.New("no matching variant")

// ErrAmbiguous is returned when two or more variants tie for best match.
var ErrAmbiguous = errors.New("ambiguous match")

// Candidate is one variant together with the evaluation outcome.
type Candidate struct {
	Index   int
	Variant registry.Variant
	Score   int
	// Matched reports whether every declared constraint was satisfied.
	Matched bool
	// Reason explains the decision in human-readable form.
	Reason string
}

// Result is a successful resolution.
type Result struct {
	Command    *registry.Command
	Info       platform.Info
	Variant    registry.Variant
	VariantIdx int
	Score      int
	Candidates []Candidate
}

// NoMatchError describes why resolution failed.
type NoMatchError struct {
	CommandID  string
	Info       platform.Info
	Candidates []Candidate
}

func (e *NoMatchError) Error() string {
	return fmt.Sprintf("命令 %q 不支持当前平台 %s", e.CommandID, DescribeInfo(e.Info))
}

// Unwrap lets callers test with errors.Is(err, ErrNoMatch).
func (e *NoMatchError) Unwrap() error { return ErrNoMatch }

// AmbiguousError reports tied candidates.
type AmbiguousError struct {
	CommandID  string
	Info       platform.Info
	Candidates []Candidate
}

func (e *AmbiguousError) Error() string {
	return fmt.Sprintf("命令 %q 在当前平台 %s 上匹配到多个同样具体的变体", e.CommandID, DescribeInfo(e.Info))
}

// Unwrap lets callers test with errors.Is(err, ErrAmbiguous).
func (e *AmbiguousError) Unwrap() error { return ErrAmbiguous }

// Resolve evaluates every variant of cmd against info and returns the single
// most specific match. The returned Result always carries all candidates so
// callers can render an explanation.
func Resolve(cmd *registry.Command, info platform.Info) (Result, error) {
	result := Result{Command: cmd, Info: info, VariantIdx: -1}
	var best []Candidate

	for i, variant := range cmd.Variants {
		score, matched, reason := Evaluate(variant.Match, info)
		candidate := Candidate{
			Index:   i,
			Variant: variant,
			Score:   score,
			Matched: matched,
			Reason:  reason,
		}
		result.Candidates = append(result.Candidates, candidate)
		if !matched {
			continue
		}
		switch {
		case len(best) == 0:
			best = []Candidate{candidate}
		case betterThan(candidate, best[0]):
			best = []Candidate{candidate}
		case ties(candidate, best[0]):
			best = append(best, candidate)
		}
	}

	switch {
	case len(best) == 0:
		return result, &NoMatchError{CommandID: cmd.ID, Info: info, Candidates: result.Candidates}
	case len(best) > 1:
		return result, &AmbiguousError{CommandID: cmd.ID, Info: info, Candidates: result.Candidates}
	}

	result.Variant = best[0].Variant
	result.VariantIdx = best[0].Index
	result.Score = best[0].Score
	return result, nil
}

// ResolveID looks a command up by ID or alias and resolves it.
func ResolveID(reg *registry.Registry, id string, info platform.Info) (Result, error) {
	cmd, ok := reg.Lookup(id)
	if !ok {
		return Result{}, fmt.Errorf("未知命令 %q", id)
	}
	return Resolve(cmd, info)
}

// Evaluate checks a single variant against the environment. It returns the
// specificity score, whether all constraints matched, and the reasoning.
func Evaluate(m registry.Match, info platform.Info) (score int, matched bool, reason string) {
	var notes []string
	rejected := ""

	reject := func(note string) {
		if rejected == "" {
			rejected = note
		}
	}
	add := func(note string) { notes = append(notes, note) }

	if m.OS != "" {
		if m.OS != info.OS {
			reject(fmt.Sprintf("os=%s 不匹配（当前 %s）", m.OS, info.OS))
		} else {
			add("os=" + m.OS)
		}
	}
	if m.Arch != "" {
		if m.Arch != info.Arch {
			reject(fmt.Sprintf("arch=%s 不匹配（当前 %s）", m.Arch, info.Arch))
		} else {
			add("arch=" + m.Arch)
		}
	}
	if m.Distro != "" {
		if m.Distro != info.Distro {
			reject(fmt.Sprintf("distro=%s 不匹配（当前 %s）", m.Distro, orUnknown(info.Distro)))
		} else {
			score += scoreDistroExact
			add(fmt.Sprintf("distro=%s (+%d)", m.Distro, scoreDistroExact))
		}
	}
	if len(m.DistroLike) > 0 {
		if !distroLikeMatches(m.DistroLike, info) {
			reject(fmt.Sprintf("ID_LIKE 要求 %s（当前 %s）", strings.Join(m.DistroLike, "|"), describeDistro(info)))
		} else {
			score += scoreDistroLike
			add(fmt.Sprintf("ID_LIKE∈{%s} (+%d)", strings.Join(m.DistroLike, ","), scoreDistroLike))
		}
	}
	if m.Runtime != "" {
		if !contains(info.Runtimes, m.Runtime) {
			reject(fmt.Sprintf("需要运行时 %s（未在 PATH 找到）", m.Runtime))
		} else {
			score += scoreRuntime
			add(fmt.Sprintf("runtime=%s (+%d)", m.Runtime, scoreRuntime))
		}
	}

	if rejected != "" {
		return 0, false, rejected
	}
	if len(notes) == 0 {
		return 0, true, "通用变体（无平台约束, 0 分）"
	}
	return score, true, "匹配 " + strings.Join(notes, ", ")
}

// runtimeRank orders runtimes by preference when two variants are otherwise
// equally specific: podman beats docker because it is the rootless default on
// CentOS-family hosts, and a variant with no runtime constraint ranks last.
func runtimeRank(runtime string) int {
	switch strings.ToLower(runtime) {
	case "podman":
		return 0
	case "docker":
		return 1
	case "":
		return 3
	default:
		return 2
	}
}

// betterThan reports whether a is a strictly better match than b: higher
// score, then the runtime preference, then the more constrained variant.
func betterThan(a, b Candidate) bool {
	if a.Score != b.Score {
		return a.Score > b.Score
	}
	if ra, rb := runtimeRank(a.Variant.Match.Runtime), runtimeRank(b.Variant.Match.Runtime); ra != rb {
		return ra < rb
	}
	return specificity(a.Variant.Match) > specificity(b.Variant.Match)
}

// ties reports whether two candidates are equally good, in which case the
// resolution is ambiguous rather than silently picking one.
func ties(a, b Candidate) bool {
	return a.Score == b.Score &&
		runtimeRank(a.Variant.Match.Runtime) == runtimeRank(b.Variant.Match.Runtime) &&
		specificity(a.Variant.Match) == specificity(b.Variant.Match)
}

// specificity counts the constraints a variant declares, so an OS-only
// variant outranks a fully generic one at equal score.
func specificity(m registry.Match) int {
	n := 0
	if m.OS != "" {
		n++
	}
	if m.Arch != "" {
		n++
	}
	if m.Distro != "" {
		n++
	}
	if len(m.DistroLike) > 0 {
		n++
	}
	if m.Runtime != "" {
		n++
	}
	return n
}

func distroLikeMatches(want []string, info platform.Info) bool {
	current := make([]string, 0, 1+len(info.DistroLike))
	if info.Distro != "" {
		current = append(current, info.Distro)
	}
	current = append(current, info.DistroLike...)
	for _, c := range current {
		for _, w := range want {
			if strings.EqualFold(c, w) {
				return true
			}
		}
	}
	return false
}

func contains(values []string, want string) bool {
	for _, v := range values {
		if strings.EqualFold(v, want) {
			return true
		}
	}
	return false
}

func orUnknown(s string) string {
	if s == "" {
		return "未知"
	}
	return s
}

func describeDistro(info platform.Info) string {
	if info.Distro == "" && len(info.DistroLike) == 0 {
		return "未知"
	}
	if len(info.DistroLike) == 0 {
		return info.Distro
	}
	return info.Distro + " (" + strings.Join(info.DistroLike, " ") + ")"
}

// DescribeInfo renders the environment compactly for error messages.
func DescribeInfo(info platform.Info) string {
	s := info.OS + "/" + info.Arch
	if info.Distro != "" {
		s += ", distro=" + info.Distro
	}
	if len(info.Runtimes) > 0 {
		s += ", runtimes=" + strings.Join(info.Runtimes, ",")
	}
	return s
}
