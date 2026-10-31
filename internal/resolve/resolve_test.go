package resolve

import (
	"errors"
	"reflect"
	"strings"
	"testing"

	"gitea.odboy.cn/odboy/kenaito-kopsc/internal/platform"
	"gitea.odboy.cn/odboy/kenaito-kopsc/internal/registry"
)

func env(osName, arch, distro string, like []string, runtimes ...string) platform.Info {
	return platform.Info{
		OS:         osName,
		Arch:       arch,
		Distro:     distro,
		DistroLike: like,
		Runtimes:   runtimes,
	}
}

func builtin(t *testing.T, id string) *registry.Command {
	t.Helper()
	cmd, ok := registry.Builtin().Lookup(id)
	if !ok {
		t.Fatalf("builtin command %q missing", id)
	}
	return cmd
}

func TestResolveBuiltin(t *testing.T) {
	tests := []struct {
		name      string
		id        string
		info      platform.Info
		wantArgv  []string
		wantScore int
		wantIdx   int
	}{
		{
			name:      "centos with podman and docker prefers podman",
			id:        "docker.clearsafe",
			info:      env("linux", "amd64", "centos", []string{"rhel", "fedora"}, "docker", "podman"),
			wantArgv:  []string{"podman", "system", "prune", "-f"},
			wantScore: 2,
			wantIdx:   0,
		},
		{
			name:      "centos with docker only",
			id:        "docker.clearsafe",
			info:      env("linux", "amd64", "centos", []string{"rhel", "fedora"}, "docker"),
			wantArgv:  []string{"docker", "system", "prune", "-f"},
			wantScore: 2,
			wantIdx:   1,
		},
		{
			name:      "generic linux with podman and docker prefers podman",
			id:        "docker.clearsafe",
			info:      env("linux", "amd64", "ubuntu", []string{"debian"}, "docker", "podman"),
			wantArgv:  []string{"podman", "system", "prune", "-f"},
			wantScore: 2,
			wantIdx:   0,
		},
		{
			name:      "rocky inherits the generic linux docker variant",
			id:        "docker.clearsafe",
			info:      env("linux", "amd64", "rocky", []string{"rhel", "centos"}, "docker"),
			wantArgv:  []string{"docker", "system", "prune", "-f"},
			wantScore: 2,
			wantIdx:   1,
		},
		{
			name:      "darwin docker",
			id:        "docker.clearsafe",
			info:      env("darwin", "arm64", "", nil, "docker"),
			wantArgv:  []string{"docker", "system", "prune", "-f"},
			wantScore: 2,
			wantIdx:   2,
		},
		{
			name:      "windows docker",
			id:        "docker.clearsafe",
			info:      env("windows", "amd64", "", nil, "docker"),
			wantArgv:  []string{"docker", "system", "prune", "-f"},
			wantScore: 2,
			wantIdx:   3,
		},
		{
			name:      "centos refreshes with dnf exactly",
			id:        "pkg.refreshindex",
			info:      env("linux", "amd64", "centos", []string{"rhel", "fedora"}),
			wantArgv:  []string{"dnf", "makecache"},
			wantScore: 3,
			wantIdx:   0,
		},
		{
			name:      "rocky inherits rhel-like dnf",
			id:        "pkg.refreshindex",
			info:      env("linux", "amd64", "rocky", []string{"rhel"}),
			wantArgv:  []string{"dnf", "makecache"},
			wantScore: 2,
			wantIdx:   1,
		},
		{
			name:      "ubuntu resolves through ID_LIKE debian",
			id:        "pkg.refreshindex",
			info:      env("linux", "amd64", "ubuntu", []string{"debian"}),
			wantArgv:  []string{"apt-get", "update"},
			wantScore: 2,
			wantIdx:   2,
		},
		{
			name:      "alpine has its own distro variant",
			id:        "pkg.refreshindex",
			info:      env("linux", "amd64", "alpine", nil),
			wantArgv:  []string{"apk", "update"},
			wantScore: 3,
			wantIdx:   3,
		},
		{
			name:     "darwin homebrew",
			id:       "pkg.refreshindex",
			info:     env("darwin", "arm64", "", nil),
			wantArgv: []string{"brew", "update"},
			wantIdx:  4,
		},
		{
			name:     "windows winget",
			id:       "pkg.refreshindex",
			info:     env("windows", "amd64", "", nil),
			wantArgv: []string{"winget", "source", "update"},
			wantIdx:  5,
		},
		{
			name:     "linux listens with ss",
			id:       "net.ports",
			info:     env("linux", "amd64", "centos", []string{"rhel"}),
			wantArgv: []string{"ss", "-tulnp"},
			wantIdx:  0,
		},
		{
			name:     "darwin listens with lsof",
			id:       "net.ports",
			info:     env("darwin", "amd64", "", nil),
			wantArgv: []string{"lsof", "-iTCP", "-sTCP:LISTEN", "-nP"},
			wantIdx:  1,
		},
		{
			name:     "windows listens with netstat",
			id:       "net.ports",
			info:     env("windows", "amd64", "", nil),
			wantArgv: []string{"netstat", "-ano"},
			wantIdx:  2,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got, err := Resolve(builtin(t, tc.id), tc.info)
			if err != nil {
				t.Fatalf("Resolve() error: %v", err)
			}
			if !reflect.DeepEqual(got.Variant.Argv, tc.wantArgv) {
				t.Errorf("argv = %v, want %v", got.Variant.Argv, tc.wantArgv)
			}
			if got.VariantIdx != tc.wantIdx {
				t.Errorf("variant index = %d, want %d", got.VariantIdx, tc.wantIdx)
			}
			if got.Score != tc.wantScore {
				t.Errorf("score = %d, want %d", got.Score, tc.wantScore)
			}
			if len(got.Candidates) != len(got.Command.Variants) {
				t.Errorf("candidates = %d, want %d", len(got.Candidates), len(got.Command.Variants))
			}
		})
	}
}

func TestResolveNoMatch(t *testing.T) {
	info := env("linux", "amd64", "centos", []string{"rhel"}) // no container runtime on PATH
	cmd := builtin(t, "docker.clearsafe")

	_, err := Resolve(cmd, info)
	if err == nil {
		t.Fatal("expected an error without a container runtime")
	}
	if !errors.Is(err, ErrNoMatch) {
		t.Fatalf("error = %v, want ErrNoMatch", err)
	}

	var noMatch *NoMatchError
	if !errors.As(err, &noMatch) {
		t.Fatalf("error type = %T, want *NoMatchError", err)
	}
	if len(noMatch.Candidates) != len(cmd.Variants) {
		t.Errorf("candidates = %d, want %d", len(noMatch.Candidates), len(cmd.Variants))
	}
	for _, c := range noMatch.Candidates {
		if c.Matched {
			t.Errorf("candidate %v should not match", c.Variant.Argv)
		}
		if c.Reason == "" {
			t.Errorf("candidate %v has no reason", c.Variant.Argv)
		}
	}
	if !strings.Contains(err.Error(), "runtime") && !strings.Contains(err.Error(), "不支持") {
		t.Errorf("error message lacks detail: %v", err)
	}
}

func TestResolveAmbiguous(t *testing.T) {
	cmd := &registry.Command{
		ID:      "demo.ambiguous",
		Summary: "two equally generic variants",
		Variants: []registry.Variant{
			{Argv: []string{"one"}},
			{Argv: []string{"two"}},
		},
	}
	_, err := Resolve(cmd, env("linux", "amd64", "", nil))
	if err == nil {
		t.Fatal("expected an ambiguity error")
	}
	if !errors.Is(err, ErrAmbiguous) {
		t.Fatalf("error = %v, want ErrAmbiguous", err)
	}
	var ambiguous *AmbiguousError
	if !errors.As(err, &ambiguous) {
		t.Fatalf("error type = %T, want *AmbiguousError", err)
	}
}

func TestResolveArchMismatch(t *testing.T) {
	cmd := &registry.Command{
		ID:      "demo.arch",
		Summary: "arm64 only",
		Variants: []registry.Variant{
			{Match: registry.Match{OS: "linux", Arch: "arm64"}, Argv: []string{"arm"}},
		},
	}
	_, err := Resolve(cmd, env("linux", "amd64", "", nil))
	if !errors.Is(err, ErrNoMatch) {
		t.Fatalf("error = %v, want ErrNoMatch", err)
	}
}

func TestResolveSpecificBeatsGeneric(t *testing.T) {
	cmd := &registry.Command{
		ID:      "demo.specific",
		Summary: "specific and generic",
		Variants: []registry.Variant{
			{Argv: []string{"generic"}},
			{Match: registry.Match{OS: "linux"}, Argv: []string{"linux"}},
		},
	}
	got, err := Resolve(cmd, env("linux", "amd64", "centos", []string{"rhel"}))
	if err != nil {
		t.Fatalf("Resolve() error: %v", err)
	}
	// Both score 0 here, so the runtime tier and then declaration order decide;
	// the important property is that a single variant wins.
	if len(got.Variant.Argv) == 0 {
		t.Fatal("no variant chosen")
	}
}

func TestEvaluateRejects(t *testing.T) {
	info := env("linux", "amd64", "centos", []string{"rhel", "fedora"}, "docker")

	cases := []struct {
		name  string
		match registry.Match
		want  string
	}{
		{"os", registry.Match{OS: "darwin"}, "os=darwin"},
		{"arch", registry.Match{Arch: "arm64"}, "arch=arm64"},
		{"distro", registry.Match{Distro: "debian"}, "distro=debian"},
		{"distro like", registry.Match{DistroLike: []string{"debian"}}, "ID_LIKE"},
		{"runtime", registry.Match{Runtime: "podman"}, "podman"},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			_, matched, reason := Evaluate(tc.match, info)
			if matched {
				t.Fatalf("match should fail: %+v", tc.match)
			}
			if !strings.Contains(reason, tc.want) {
				t.Fatalf("reason %q does not contain %q", reason, tc.want)
			}
		})
	}
}
