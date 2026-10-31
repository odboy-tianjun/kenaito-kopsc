package executor

import (
	"bytes"
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"gitea.odboy.cn/odboy/kenaito-kopsc/internal/registry"
)

func registryVariantBuiltin(name string) registry.Variant {
	return registry.Variant{Builtin: name, Match: registry.Match{OS: "linux"}}
}

func writeSizedFile(t *testing.T, path string, size int) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatalf("mkdir %s: %v", filepath.Dir(path), err)
	}
	if err := os.WriteFile(path, bytes.Repeat([]byte("x"), size), 0o644); err != nil {
		t.Fatalf("write %s: %v", path, err)
	}
}

// fixture builds:
//
//	root/big/big.bin        4096
//	root/direct.txt         2048
//	root/small/small.txt    1024
//	root/empty/             (no files)
func diskFixture(t *testing.T) string {
	t.Helper()
	root := t.TempDir()
	writeSizedFile(t, filepath.Join(root, "big", "big.bin"), 4096)
	writeSizedFile(t, filepath.Join(root, "direct.txt"), 2048)
	writeSizedFile(t, filepath.Join(root, "small", "small.txt"), 1024)
	if err := os.MkdirAll(filepath.Join(root, "empty"), 0o755); err != nil {
		t.Fatal(err)
	}
	return root
}

func runDisk(t *testing.T, args ...string) (string, error) {
	t.Helper()
	var out bytes.Buffer
	err := runDiskUsage(context.Background(), BuiltinRequest{Args: args, Stdout: &out})
	return out.String(), err
}

func TestDiskUsageOrdersBySizeDescending(t *testing.T) {
	root := diskFixture(t)

	out, err := runDisk(t, root)
	if err != nil {
		t.Fatalf("runDiskUsage error: %v", err)
	}
	lines := strings.Split(strings.TrimSpace(out), "\n")
	if len(lines) != 5 {
		t.Fatalf("expected 4 entries + total, got %d:\n%s", len(lines), out)
	}

	wantOrder := []struct {
		size string
		name string
	}{
		{"4.0K", "big"},
		{"2.0K", "direct.txt"},
		{"1.0K", "small"},
		{"0", "empty"},
	}
	for i, want := range wantOrder {
		if !strings.HasPrefix(lines[i], want.size) || !strings.Contains(lines[i], want.name) {
			t.Errorf("line %d = %q, want size %s and name %s", i, lines[i], want.size, want.name)
		}
	}
	if !strings.HasSuffix(lines[4], "总计 "+root) {
		t.Errorf("total line = %q, want suffix %q", lines[4], "总计 "+root)
	}
	if !strings.HasPrefix(lines[4], "7.0K") {
		t.Errorf("total line = %q, want 7.0K", lines[4])
	}
}

func TestDiskUsageAcceptsPathFlag(t *testing.T) {
	root := diskFixture(t)

	for _, args := range [][]string{
		{"--path", root},
		{"--path=" + root},
		{"--path", root, "--"},
	} {
		out, err := runDisk(t, args...)
		if err != nil {
			t.Fatalf("args %v: %v", args, err)
		}
		if !strings.Contains(out, "big") || !strings.Contains(out, "总计") {
			t.Errorf("args %v produced unexpected output:\n%s", args, out)
		}
	}
}

func TestDiskUsageRequiresPath(t *testing.T) {
	_, err := runDisk(t)
	if err == nil || !strings.Contains(err.Error(), "必须提供路径") {
		t.Fatalf("err = %v, want missing-path error", err)
	}
}

func TestDiskUsageRejectsBadArgs(t *testing.T) {
	tests := []struct {
		args []string
		want string
	}{
		{[]string{"--max-depth=2", "/var"}, "未知选项"},
		{[]string{"-h", "/var"}, "未知选项"},
		{[]string{"/var", "/tmp"}, "一次只支持一个路径"},
		{[]string{"--path"}, "--path 缺少参数"},
		{[]string{"--path="}, "--path 缺少参数"},
	}
	for _, tc := range tests {
		_, err := runDisk(t, tc.args...)
		if err == nil || !strings.Contains(err.Error(), tc.want) {
			t.Errorf("args %v: err = %v, want %q", tc.args, err, tc.want)
		}
	}
}

func TestDiskUsageRejectsFile(t *testing.T) {
	file := filepath.Join(t.TempDir(), "f.txt")
	writeSizedFile(t, file, 10)

	_, err := runDisk(t, file)
	if err == nil || !strings.Contains(err.Error(), "不是目录") {
		t.Fatalf("err = %v, want not-a-directory error", err)
	}
}

func TestHumanSize(t *testing.T) {
	cases := map[int64]string{
		0:                  "0",
		512:                "512",
		1023:               "1023",
		1024:               "1.0K",
		1536:               "1.5K",
		1024 * 1024:        "1.0M",
		1024 * 1024 * 1024: "1.0G",
		1 << 40:            "1.0T",
	}
	for in, want := range cases {
		if got := humanSize(in); got != want {
			t.Errorf("humanSize(%d) = %q, want %q", in, got, want)
		}
	}
}

func TestBuiltinIsRegistered(t *testing.T) {
	if _, ok := LookupBuiltin("disk.usage"); !ok {
		t.Fatal("disk.usage builtin not registered")
	}
	if _, ok := LookupBuiltin("missing"); ok {
		t.Fatal("unexpected builtin hit")
	}
}

func TestPlanRunsBuiltin(t *testing.T) {
	root := diskFixture(t)
	plan := samplePlan()
	plan.Result.Variant = registryVariantBuiltin("disk.usage")
	plan.ExtraArgs = []string{root}

	var out bytes.Buffer
	if err := plan.Run(context.Background(), Options{Stdout: &out, Stderr: &out}); err != nil {
		t.Fatalf("Run() error: %v", err)
	}
	if !strings.Contains(out.String(), "总计") {
		t.Errorf("builtin output missing total:\n%s", out.String())
	}
	if !strings.Contains(plan.Display(), "(内置) disk.usage") {
		t.Errorf("Display() = %q", plan.Display())
	}
}

func TestPlanBuiltinDryRunDoesNotExecute(t *testing.T) {
	plan := samplePlan()
	plan.Result.Variant = registryVariantBuiltin("disk.usage")
	plan.ExtraArgs = []string{filepath.Join(t.TempDir(), "missing")}

	var out bytes.Buffer
	if err := plan.Run(context.Background(), Options{DryRun: true, Stdout: &out}); err != nil {
		t.Fatalf("dry-run should not execute: %v", err)
	}
	if !strings.Contains(out.String(), "内置实现") {
		t.Errorf("dry-run output lacks builtin note:\n%s", out.String())
	}
}
