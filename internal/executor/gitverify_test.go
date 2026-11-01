package executor

import (
	"context"
	"strings"
	"testing"
)

func TestTreesMatchComparesTreeHashes(t *testing.T) {
	runner := &fakeGitRunner{responses: map[string]gitResponse{
		"rev-parse old^{tree}":   {out: "treeA\n"},
		"rev-parse same^{tree}":  {out: "treeA\n"},
		"rev-parse other^{tree}": {out: "treeB\n"},
	}}
	ctx := context.Background()

	same, err := treesMatch(ctx, runner, "old", "same")
	if err != nil || !same {
		t.Fatalf("same trees: match=%v err=%v", same, err)
	}
	same, err = treesMatch(ctx, runner, "old", "other")
	if err != nil || same {
		t.Fatalf("different trees: match=%v err=%v", same, err)
	}
}

func TestTreesMatchReportsRevParseFailure(t *testing.T) {
	runner := &fakeGitRunner{responses: map[string]gitResponse{}}
	_, err := treesMatch(context.Background(), runner, "missing", "other")
	if err == nil {
		t.Fatal("expected an error when the commit cannot be read")
	}
}

func TestVerifyRewritePassesOnIdenticalContent(t *testing.T) {
	runner := &fakeGitRunner{responses: map[string]gitResponse{
		"rev-parse oldtip^{tree}": {out: "treeX\n"},
		"rev-parse newtip^{tree}": {out: "treeX\n"},
	}}
	if err := verifyRewrite(context.Background(), runner, "oldtip", "newtip"); err != nil {
		t.Fatalf("identical content should pass: %v", err)
	}
}

func TestVerifyRewriteStopsOnContentChange(t *testing.T) {
	runner := &fakeGitRunner{responses: map[string]gitResponse{
		"rev-parse oldtip^{tree}": {out: "treeX\n"},
		"rev-parse newtip^{tree}": {out: "treeY\n"},
	}}
	err := verifyRewrite(context.Background(), runner, "oldtip", "newtip")
	if err == nil || !strings.Contains(err.Error(), "内容与原提交不一致") {
		t.Fatalf("err = %v, want a content-mismatch error", err)
	}
	if !strings.Contains(err.Error(), "回滚") {
		t.Errorf("error should point at the backup: %v", err)
	}
}
