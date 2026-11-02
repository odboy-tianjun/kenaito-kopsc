package executor

import (
	"context"
	"fmt"
	"strings"
	"time"
)

// commitMeta is everything needed to rebuild a commit with a new time.
type commitMeta struct {
	Hash        string
	Tree        string
	Parents     []string
	AuthorName  string
	AuthorEmail string
	CommitName  string
	CommitEmail string
	Subject     string
	Body        string
}

// rawCommitFormat lists the fields kopsc rebuilds. The body (if any) is fetched
// separately so it can keep its newlines.
const rawCommitFormat = "%H%x09%T%x09%P%x09%an%x09%ae%x09%cn%x09%ce%x09%s"

// commitMetadata reads the tree, parents, identities and message of one commit.
func commitMetadata(ctx context.Context, runner gitRunner, hash string) (commitMeta, error) {
	out, stderr, err := runner.Run(ctx, "log", "-1", "--format="+rawCommitFormat, hash)
	if err != nil {
		return commitMeta{}, gitError("git log", stderr, err)
	}
	out = strings.TrimRight(out, "\n")
	fields := strings.SplitN(out, "\t", 8)
	if len(fields) < 8 {
		return commitMeta{}, fmt.Errorf("无法解析提交 %s 的元数据: %q", hash, out)
	}

	meta := commitMeta{
		Hash:        fields[0],
		Tree:        fields[1],
		AuthorName:  fields[3],
		AuthorEmail: fields[4],
		CommitName:  fields[5],
		CommitEmail: fields[6],
		Subject:     fields[7],
	}
	if fields[2] != "" {
		meta.Parents = strings.Fields(fields[2])
	}

	// The full message keeps the body and the exact formatting.
	body, bodyStderr, err := runner.Run(ctx, "log", "-1", "--format=%B", hash)
	if err != nil {
		return commitMeta{}, gitError("git log", bodyStderr, err)
	}
	meta.Body = strings.TrimRight(body, "\n")
	return meta, nil
}

// rebuildCommit writes a new commit object with the same tree, parents and
// identities but the given author and committer times, and returns its hash.
//
// git commits are immutable, so rewriting history means creating a parallel
// commit for every original one and moving the branch on top of the new chain.
func rebuildCommit(ctx context.Context, runner gitRunner, meta commitMeta, when time.Time) (string, error) {
	args := []string{"commit-tree", meta.Tree}
	for _, parent := range meta.Parents {
		args = append(args, "-p", parent)
	}
	args = append(args,
		"-m", meta.Body,
	)

	env := []string{
		// Keep the original identities: only the timestamps change.
		"GIT_AUTHOR_NAME=" + meta.AuthorName,
		"GIT_AUTHOR_EMAIL=" + meta.AuthorEmail,
		"GIT_COMMITTER_NAME=" + meta.CommitName,
		"GIT_COMMITTER_EMAIL=" + meta.CommitEmail,
		// Both dates move together; ISO format keeps the seconds.
		"GIT_AUTHOR_DATE=" + when.Format(time.RFC3339),
		"GIT_COMMITTER_DATE=" + when.Format(time.RFC3339),
	}

	out, stderr, err := runner.RunEnv(ctx, env, args...)
	if err != nil {
		return "", gitError("git commit-tree", stderr, err)
	}
	hash := strings.TrimSpace(out)
	if hash == "" {
		return "", fmt.Errorf("git commit-tree 未返回提交哈希")
	}
	return hash, nil
}

// rebuiltCommit records the replacement of one commit.
type rebuiltCommit struct {
	Old string
	New string
}

// rewriteChain rebuilds every commit in plan (oldest first) on top of the
// parent of the oldest one, returning the new tip and the hash mapping.
//
// tree, parents and identities are preserved; only the timestamps change. The
// branch is not moved here — that is a separate, explicit step so a failure
// halfway leaves the repository untouched.
func rewriteChain(ctx context.Context, runner gitRunner, plan retimePlan) (string, []rebuiltCommit, error) {
	if len(plan.Commits) == 0 {
		return "", nil, fmt.Errorf("没有需要改写的提交")
	}

	// The new parent starts as the parent of the oldest rewritten commit, so
	// everything before the range keeps pointing at the untouched history.
	newParent := ""
	first, err := commitMetadata(ctx, runner, plan.Commits[0].Hash)
	if err != nil {
		return "", nil, err
	}
	if len(first.Parents) > 0 {
		newParent = first.Parents[0]
	}

	rebuilt := make([]rebuiltCommit, 0, len(plan.Commits))
	for i, item := range plan.Commits {
		meta := first
		if i > 0 {
			if meta, err = commitMetadata(ctx, runner, item.Hash); err != nil {
				return "", rebuilt, err
			}
		}
		// Point the commit at the rewritten parent so the chain stays linear.
		meta.Parents = nil
		if newParent != "" {
			meta.Parents = []string{newParent}
		}

		newHash, err := rebuildCommit(ctx, runner, meta, item.NewTime)
		if err != nil {
			return "", rebuilt, err
		}
		rebuilt = append(rebuilt, rebuiltCommit{Old: item.Hash, New: newHash})
		newParent = newHash
	}
	return newParent, rebuilt, nil
}

// moveBranch points branch at newTip. update-ref is atomic per ref, so a crash
// either leaves the old history or the new one.
func moveBranch(ctx context.Context, runner gitRunner, branch, newTip string) error {
	_, stderr, err := runner.Run(ctx, "update-ref", "refs/heads/"+branch, newTip)
	if err != nil {
		return gitError("git update-ref", stderr, err)
	}
	return nil
}

// treeOf returns the tree object a commit points at.
func treeOf(ctx context.Context, runner gitRunner, rev string) (string, error) {
	out, stderr, err := runner.Run(ctx, "rev-parse", rev+"^{tree}")
	if err != nil {
		return "", gitError("git rev-parse", stderr, err)
	}
	tree := strings.TrimSpace(out)
	if tree == "" {
		return "", fmt.Errorf("无法读取 %s 的树对象", rev)
	}
	return tree, nil
}

// treesMatch reports whether two commits carry the same content.
//
// Comparing tree hashes is exact and cannot be confused with a command failure
// the way `git diff --quiet`'s exit status can.
func treesMatch(ctx context.Context, runner gitRunner, a, b string) (bool, error) {
	treeA, err := treeOf(ctx, runner, a)
	if err != nil {
		return false, err
	}
	treeB, err := treeOf(ctx, runner, b)
	if err != nil {
		return false, err
	}
	return treeA == treeB, nil
}

// verifyRewrite checks that the rewritten tip holds exactly the same content as
// the original one before anything is pushed.
func verifyRewrite(ctx context.Context, runner gitRunner, oldTip, newTip string) error {
	same, err := treesMatch(ctx, runner, oldTip, newTip)
	if err != nil {
		return err
	}
	if !same {
		return fmt.Errorf("改写后的内容与原提交不一致（%s vs %s），已中止推送；"+
			"可用备份引用回滚", oldTip, newTip)
	}
	return nil
}

// backupRefPrefix namespaces the safety refs kopsc writes before rewriting.
const backupRefPrefix = "refs/kopsc-backup/"

// backupRefName builds the safety ref for a branch at a given moment.
func backupRefName(branch string, now time.Time) string {
	return backupRefPrefix + branch + "-" + now.Format("20060102-150405")
}

// ensureCleanWorktree refuses to rewrite history when there are uncommitted
// changes: a failed checkout halfway would mix them into the new history.
func ensureCleanWorktree(ctx context.Context, runner gitRunner) error {
	clean, err := isWorktreeClean(ctx, runner)
	if err != nil {
		return err
	}
	if !clean {
		return fmt.Errorf("工作区有未提交的改动，已中止；请先提交或 stash 后再重写历史（git stash）")
	}
	return nil
}

// createBackup points a safety ref at the current tip, so a rewritten branch
// can be restored even after a failed push.
func createBackup(ctx context.Context, runner gitRunner, branch, tip string, now time.Time) (string, error) {
	ref := backupRefName(branch, now)
	_, stderr, err := runner.Run(ctx, "update-ref", ref, tip)
	if err != nil {
		return "", gitError("git update-ref", stderr, err)
	}
	return ref, nil
}

// backupHint renders the command that undoes a rewrite.
func backupHint(branch, ref string) string {
	return "回滚： git update-ref refs/heads/" + branch + " " + ref + "  （或 git reset --hard " + ref + "）"
}

// forcePush replaces the remote branch with the rewritten history.
//
// --force-with-lease is deliberate: it refuses to overwrite commits the local
// repository has not seen, so a colleague's push is never silently discarded.
//
// git writes a lot to stderr on a failed push, and only some of it explains the
// failure: git-lfs prints advisory notes ("Locking support detected…") that used
// to be shown as if they were the cause. The error below classifies what
// actually went wrong and tells the user what to run next.
func forcePush(ctx context.Context, runner gitRunner, remote, branch string) error {
	_, stderr, err := runner.Run(ctx, "push", "--force-with-lease", remote, branch)
	if err == nil {
		return nil
	}
	return classifyPushError(remote, branch, stderr, err)
}

// classifyPushError turns git's stderr into one actionable message.
func classifyPushError(remote, branch, stderr string, err error) error {
	detail := pushDetail(stderr)

	switch {
	case isLFSIncomplete(stderr):
		return fmt.Errorf("强推被 LFS 完整性检查拦下：本地缺少远程要接收的 LFS 对象（%s）。\n"+
			"如果当前提交用到的对象都已存在，可以跳过检查：\n"+
			"  git -c lfs.allowincompletepush=true push --force-with-lease %s %s",
			firstMissingObject(stderr), remote, branch)

	case isLeaseRejected(stderr):
		return fmt.Errorf("强推被拒绝：远程分支 %s/%s 有本地未知的新提交（租约校验失败）。\n"+
			"确认远端确实该被覆盖后，显式指定远端当前 SHA 再推：\n"+
			"  git push --force-with-lease=%s:<远端SHA> %s %s\n"+
			"（本地历史已改写，如需撤销可用备份引用回滚）\n%s",
			remote, branch, branch, remote, branch, detail)

	case isProtected(stderr):
		return fmt.Errorf("强推被拒绝：%s/%s 受分支保护，服务器不允许改写历史。\n"+
			"请先在仓库设置里临时放开保护，或改推到另一个分支后再合并。\n%s",
			remote, branch, detail)

	default:
		return gitError("git push", stderr, err)
	}
}

// pushDetail keeps git's own output for the message, dropping the advisory
// lines that only repeat what kopsc already says.
func pushDetail(stderr string) string {
	var kept []string
	for _, line := range strings.Split(stderr, "\n") {
		trimmed := strings.TrimSpace(line)
		if trimmed == "" || isLFSAdvisory(trimmed) {
			continue
		}
		kept = append(kept, trimmed)
	}
	return strings.Join(kept, "\n")
}

// isLFSAdvisory reports whether a line is git-lfs advice rather than a failure.
func isLFSAdvisory(line string) bool {
	return strings.HasPrefix(line, "Locking support detected") ||
		strings.HasPrefix(line, "$ git config lfs.") ||
		strings.HasPrefix(line, "hint: You can disable this check")
}

// isLeaseRejected reports whether git refused because the remote moved.
func isLeaseRejected(stderr string) bool {
	return strings.Contains(stderr, "stale info") ||
		strings.Contains(stderr, "[rejected]") ||
		(strings.Contains(stderr, "rejected") && !isLFSIncomplete(stderr))
}

// isProtected reports a branch-protection refusal.
func isProtected(stderr string) bool {
	return strings.Contains(stderr, "protected branch") ||
		strings.Contains(stderr, "pre-receive hook declined") ||
		strings.Contains(stderr, "hook declined")
}

// isLFSIncomplete reports that the push was stopped by git-lfs upload checks.
func isLFSIncomplete(stderr string) bool {
	return strings.Contains(stderr, "LFS upload failed") ||
		strings.Contains(stderr, "LFS upload missing objects") ||
		strings.Contains(stderr, "(missing)")
}

// isRejectedPush reports whether git refused the push for a remote-side reason,
// as opposed to a local failure such as LFS objects.
func isRejectedPush(stderr string) bool {
	return isLeaseRejected(stderr) || isProtected(stderr)
}

// firstMissingObject extracts the first "(missing) <path> (<oid>)" entry.
func firstMissingObject(stderr string) string {
	for _, line := range strings.Split(stderr, "\n") {
		line = strings.TrimSpace(line)
		if strings.HasPrefix(line, "(missing)") {
			fields := strings.Fields(strings.TrimPrefix(line, "(missing)"))
			if len(fields) > 0 {
				return fields[0]
			}
		}
	}
	return "未知对象"
}
