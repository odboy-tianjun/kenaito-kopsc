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

func TestDiskUsageDefaultsToCurrentDirectory(t *testing.T) {
	root := diskFixture(t)
	old, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	if err := os.Chdir(root); err != nil {
		t.Fatal(err)
	}
	defer func() { _ = os.Chdir(old) }()

	out, err := runDisk(t)
	if err != nil {
		t.Fatalf("omitting the path should analyse the current directory: %v", err)
	}
	for _, want := range []string{"big", "direct.txt", "总计 ."} {
		if !strings.Contains(out, want) {
			t.Errorf("output missing %q:\n%s", want, out)
		}
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

func TestParseDiskUsageDepth(t *testing.T) {
	tests := []struct {
		args      []string
		wantPath  string
		wantDepth int
		wantErr   string
	}{
		{[]string{"/var"}, "/var", 1, ""},
		{[]string{"--depth", "2", "/var"}, "/var", 2, ""},
		{[]string{"-d", "3", "/var"}, "/var", 3, ""},
		{[]string{"--depth=2", "/var"}, "/var", 2, ""},
		{[]string{"--depth", "2"}, ".", 2, ""},
		{[]string{"--depth"}, "", 0, "--depth 缺少层数"},
		{[]string{"--depth", "0", "/var"}, "", 0, "--depth 需要大于 0"},
		{[]string{"--depth=x", "/var"}, "", 0, "--depth 需要大于 0"},
	}
	for _, tc := range tests {
		opts, err := parseDiskUsageOptions(tc.args)
		if tc.wantErr != "" {
			if err == nil || !strings.Contains(err.Error(), tc.wantErr) {
				t.Errorf("args %v: err = %v, want %q", tc.args, err, tc.wantErr)
			}
			continue
		}
		if err != nil {
			t.Errorf("args %v: %v", tc.args, err)
			continue
		}
		if opts.path != tc.wantPath || opts.depth != tc.wantDepth {
			t.Errorf("args %v: got %+v, want path=%q depth=%d", tc.args, opts, tc.wantPath, tc.wantDepth)
		}
	}
}

func TestDiskUsageDepthTwoListsNestedFiles(t *testing.T) {
	root := t.TempDir()
	writeSizedFile(t, filepath.Join(root, "a", "deep", "x.bin"), 4096)
	writeSizedFile(t, filepath.Join(root, "a", "y.bin"), 2048)
	writeSizedFile(t, filepath.Join(root, "top.txt"), 1024)

	// depth 1: only "a" (summed) and top.txt.
	out, err := runDisk(t, root)
	if err != nil {
		t.Fatalf("depth 1: %v", err)
	}
	if !strings.Contains(out, filepath.Join("a")) || strings.Contains(out, "x.bin") {
		t.Fatalf("depth 1 should stop at direct children:\n%s", out)
	}

	// depth 2: a's children show up individually.
	out, err = runDisk(t, root, "--depth", "2")
	if err != nil {
		t.Fatalf("depth 2: %v", err)
	}
	for _, want := range []string{"y.bin", "deep"} {
		if !strings.Contains(out, want) {
			t.Errorf("depth 2 output missing %q:\n%s", want, out)
		}
	}
	if strings.Contains(out, "x.bin") {
		t.Errorf("depth 2 must not descend into deep/:\n%s", out)
	}

	// depth 3 reaches the nested file.
	out, err = runDisk(t, root, "--depth", "3")
	if err != nil {
		t.Fatalf("depth 3: %v", err)
	}
	if !strings.Contains(out, "x.bin") {
		t.Errorf("depth 3 output missing x.bin:\n%s", out)
	}
}

func TestDiskUsageDepthKeepsTotalAccurate(t *testing.T) {
	root := t.TempDir()
	writeSizedFile(t, filepath.Join(root, "a", "deep", "x.bin"), 4096)
	writeSizedFile(t, filepath.Join(root, "a", "y.bin"), 2048)
	writeSizedFile(t, filepath.Join(root, "top.txt"), 1024)

	// Every file is counted once no matter the depth, so the total is the same
	// 7168 bytes (7.0K) as depth 1 — deeper listing must not double count.
	for _, depth := range []string{"1", "2", "3"} {
		out, err := runDisk(t, root, "--depth", depth)
		if err != nil {
			t.Fatalf("depth %s: %v", depth, err)
		}
		// The total line is "7.0K     总计 <root>".
		var total string
		for _, line := range strings.Split(out, "\n") {
			if strings.Contains(line, "总计") {
				total = strings.Fields(line)[0]
			}
		}
		if total != "7.0K" {
			t.Errorf("depth %s total = %q, want 7.0K", depth, total)
		}
	}
}
