//go:build !windows

package executor

import (
	"bytes"
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestArchiveName(t *testing.T) {
	cases := map[string]string{
		"alpine:3.20":                       "alpine_3.20.tar",
		"alpine":                            "alpine.tar",
		"nginx:1.25.3":                      "nginx_1.25.3.tar",
		"registry.example.com:5000/app:1.0": "registry.example.com_5000_app_1.0.tar",
		"team/app:latest":                   "team_app_latest.tar",
		"alpine@sha256:abc":                 "alpine_sha256_abc.tar",
	}
	for in, want := range cases {
		if got := archiveName(in); got != want {
			t.Errorf("archiveName(%q) = %q, want %q", in, got, want)
		}
	}
}

func TestLooksLikeImageID(t *testing.T) {
	yes := []string{
		"sha256:9c6f07244728", "9c6f07244728", "9C6F07244728",
		"sha256:9c6f0724472892f3061e",
	}
	for _, in := range yes {
		if !looksLikeImageID(in) {
			t.Errorf("looksLikeImageID(%q) = false, want true", in)
		}
	}
	no := []string{"alpine", "alpine:3.20", "nginx:1.25", "", "abc"}
	for _, in := range no {
		if looksLikeImageID(in) {
			t.Errorf("looksLikeImageID(%q) = true, want false", in)
		}
	}
}

// fakeRuntime installs a script called docker on PATH. images is the
// `images --format` payload; every invocation is appended to the returned log
// file as one line.
func fakeRuntime(t *testing.T, images string) (logPath string) {
	t.Helper()
	dir := t.TempDir()
	logPath = filepath.Join(dir, "calls.log")

	script := "#!/bin/sh\n" +
		"printf '%s' \"$0\" >> " + logPath + ".name\n" +
		"echo \"$*\" >> " + logPath + "\n" +
		"case \"$1\" in\n" +
		"  images) cat <<'EOF'\n" + images + "\nEOF\n" +
		"    ;;\n" +
		"esac\n"
	if err := os.WriteFile(filepath.Join(dir, "docker"), []byte(script), 0o755); err != nil {
		t.Fatal(err)
	}
	t.Setenv("PATH", dir+string(os.PathListSeparator)+os.Getenv("PATH"))
	return logPath
}

func imageList() string {
	return strings.Join([]string{
		"IMAGE ID\tREPOSITORY\tTAG\tDIGEST",
		"9c6f07244728\talpine\t3.20\t",
		"9c6f07244728\talpine\tlatest\t",
		"5ca7b1d5e2fc\tnginx\t1.25.3\t",
		"1b9a2c3d4e5f\tregistry.example.com:5000/team/app\t1.0\t",
		"7d3e5f7a9b1c\t<none>\t<none>\t",
	}, "\n")
}

func calls(t *testing.T, logPath string) []string {
	t.Helper()
	data, err := os.ReadFile(logPath)
	if err != nil {
		return nil
	}
	var out []string
	for _, line := range strings.Split(string(data), "\n") {
		if line = strings.TrimSpace(line); line != "" {
			out = append(out, line)
		}
	}
	return out
}

func TestDockerSaveNamesArchiveAfterTag(t *testing.T) {
	logPath := fakeRuntime(t, imageList())

	var out bytes.Buffer
	if err := runDockerSave(context.Background(), BuiltinRequest{
		Args:   []string{"alpine:3.20"},
		Stdout: &out,
	}); err != nil {
		t.Fatalf("save: %v", err)
	}

	if !strings.Contains(out.String(), "alpine:3.20") || !strings.Contains(out.String(), "alpine_3.20.tar") {
		t.Errorf("summary = %q", out.String())
	}
	got := calls(t, logPath)
	want := "save -o alpine_3.20.tar alpine:3.20"
	if len(got) < 2 || got[1] != want {
		t.Fatalf("calls = %q, want a %q invocation", got, want)
	}
}

func TestDockerSaveResolvesImageIDToName(t *testing.T) {
	for _, id := range []string{"5ca7b1d5e2fc", "sha256:5ca7b1d5e2fc", "5ca7"} {
		logPath := fakeRuntime(t, imageList())
		var out bytes.Buffer
		if err := runDockerSave(context.Background(), BuiltinRequest{Args: []string{id}, Stdout: &out}); err != nil {
			t.Fatalf("save %q: %v", id, err)
		}
		got := calls(t, logPath)
		want := "save -o nginx_1.25.3.tar " + id
		if len(got) < 2 || got[1] != want {
			t.Errorf("image %q: calls = %q, want %q", id, got, want)
		}
		if !strings.Contains(out.String(), "nginx_1.25.3.tar") {
			t.Errorf("image %q: summary = %q", id, out.String())
		}
	}
}

func TestDockerSaveRegistryWithPortKeepsName(t *testing.T) {
	fakeRuntime(t, imageList())
	var out bytes.Buffer
	if err := runDockerSave(context.Background(), BuiltinRequest{
		Args:   []string{"registry.example.com:5000/team/app:1.0"},
		Stdout: &out,
	}); err != nil {
		t.Fatalf("save: %v", err)
	}
	if !strings.Contains(out.String(), "registry.example.com_5000_team_app_1.0.tar") {
		t.Fatalf("summary = %q", out.String())
	}
}

func TestDockerSaveRejectsUntaggedImage(t *testing.T) {
	fakeRuntime(t, imageList())
	err := runDockerSave(context.Background(), BuiltinRequest{Args: []string{"7d3e5f7a9b1c"}})
	if err == nil || !strings.Contains(err.Error(), "没有名称") {
		t.Fatalf("err = %v, want untagged-image error", err)
	}
}

func TestDockerSaveRejectsUnknownAndExtraImages(t *testing.T) {
	fakeRuntime(t, imageList())
	err := runDockerSave(context.Background(), BuiltinRequest{Args: []string{"busybox:1"}})
	if err == nil || !strings.Contains(err.Error(), "本地没有镜像") {
		t.Fatalf("err = %v, want unknown-image error", err)
	}

	err = runDockerSave(context.Background(), BuiltinRequest{Args: []string{"alpine:3.20", "nginx:1.25.3"}})
	if err == nil || !strings.Contains(err.Error(), "一次只能保存一个镜像") {
		t.Fatalf("err = %v, want single-image error", err)
	}

	err = runDockerSave(context.Background(), BuiltinRequest{})
	if err == nil || !strings.Contains(err.Error(), "缺少镜像") {
		t.Fatalf("err = %v, want missing-image error", err)
	}
}

func TestDockerSaveForwardsPlatformFlag(t *testing.T) {
	logPath := fakeRuntime(t, imageList())
	var out bytes.Buffer
	if err := runDockerSave(context.Background(), BuiltinRequest{
		Args:   []string{"--platform", "linux/arm64", "alpine:3.20"},
		Stdout: &out,
	}); err != nil {
		t.Fatalf("save: %v", err)
	}
	got := calls(t, logPath)
	want := "save -o alpine_3.20.tar alpine:3.20 --platform linux/arm64"
	if len(got) < 2 || got[1] != want {
		t.Fatalf("calls = %q, want %q", got, want)
	}
}

func TestDockerLoadTakesArchiveAsPositional(t *testing.T) {
	dir := t.TempDir()
	archive := filepath.Join(dir, "alpine_3.20.tar")
	if err := os.WriteFile(archive, []byte("x"), 0o644); err != nil {
		t.Fatal(err)
	}

	for _, args := range [][]string{
		{archive},
		{"-i", archive},
		{"--input=" + archive},
		{"--", archive},
		{archive, "-q"},
	} {
		logPath := fakeRuntime(t, "")
		if err := runDockerLoad(context.Background(), BuiltinRequest{Args: args}); err != nil {
			t.Fatalf("load %v: %v", args, err)
		}
		got := calls(t, logPath)
		joined := strings.Join(got, "|")
		if !strings.Contains(joined, "load") || !strings.Contains(joined, "-i "+archive) {
			t.Errorf("load %v produced calls %q", args, got)
		}
		if strings.Contains(joined, "<") {
			t.Errorf("load %v must not rely on a shell redirect: %q", args, got)
		}
	}
}

func TestDockerLoadRejectsMissingArchive(t *testing.T) {
	fakeRuntime(t, "")
	for _, args := range [][]string{{}, {"-i"}, {filepath.Join(t.TempDir(), "nope.tar")}, {t.TempDir()}} {
		err := runDockerLoad(context.Background(), BuiltinRequest{Args: args})
		if err == nil {
			t.Errorf("load %v: expected an error", args)
		}
	}
	if err := runDockerLoad(context.Background(), BuiltinRequest{Args: []string{"missing.tar"}}); err == nil ||
		!strings.Contains(err.Error(), "无法读取归档文件") {
		t.Errorf("missing archive err = %v", err)
	}
}

func TestDockerLoadFromStdin(t *testing.T) {
	logPath := fakeRuntime(t, "")
	if err := runDockerLoad(context.Background(), BuiltinRequest{Args: []string{"-"}}); err != nil {
		t.Fatalf("load -: %v", err)
	}
	got := calls(t, logPath)
	if len(got) != 1 || got[0] != "load" {
		t.Fatalf("calls = %q, want a single bare `load`", got)
	}
}
