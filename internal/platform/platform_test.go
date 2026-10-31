package platform

import (
	"errors"
	"reflect"
	"strings"
	"testing"
)

const centosStream9 = `NAME="CentOS Stream"
VERSION="9"
ID="centos"
ID_LIKE="rhel fedora"
VERSION_ID="9"
PLATFORM_ID="platform:el9"
`

const ubuntu2404 = `PRETTY_NAME="Ubuntu 24.04 LTS"
NAME="Ubuntu"
VERSION_ID="24.04"
ID=ubuntu
ID_LIKE=debian
`

const alpine = `NAME="Alpine Linux"
ID=alpine
VERSION_ID=3.20.3
`

func TestParseOSRelease(t *testing.T) {
	tests := []struct {
		name       string
		input      string
		wantDistro string
		wantLike   []string
	}{
		{"centos stream 9", centosStream9, "centos", []string{"rhel", "fedora"}},
		{"ubuntu 24.04", ubuntu2404, "ubuntu", []string{"debian"}},
		{"alpine without ID_LIKE", alpine, "alpine", nil},
		{"comments and blank lines", "# a comment\n\nID=rocky\nID_LIKE='rhel centos'\n", "rocky", []string{"rhel", "centos"}},
		{"empty input", "", "", nil},
		{"malformed line", "ID\nID=debian\n", "debian", nil},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			distro, like := ParseOSRelease([]byte(tc.input))
			if distro != tc.wantDistro {
				t.Errorf("distro = %q, want %q", distro, tc.wantDistro)
			}
			if !reflect.DeepEqual(like, tc.wantLike) {
				t.Errorf("like = %v, want %v", like, tc.wantLike)
			}
		})
	}
}

func TestDetect(t *testing.T) {
	tests := []struct {
		name         string
		goos         string
		goarch       string
		osRelease    string
		osReleaseErr error
		present      []string
		want         Info
	}{
		{
			name:      "linux centos with docker",
			goos:      "linux",
			goarch:    "amd64",
			osRelease: centosStream9,
			present:   []string{"docker"},
			want: Info{
				OS: "linux", Arch: "amd64",
				Distro: "centos", DistroLike: []string{"rhel", "fedora"},
				Runtimes: []string{"docker"},
			},
		},
		{
			name:      "linux centos with podman and docker",
			goos:      "linux",
			goarch:    "arm64",
			osRelease: centosStream9,
			present:   []string{"docker", "podman"},
			want: Info{
				OS: "linux", Arch: "arm64",
				Distro: "centos", DistroLike: []string{"rhel", "fedora"},
				Runtimes: []string{"docker", "podman"},
			},
		},
		{
			name:         "linux without os-release",
			goos:         "linux",
			goarch:       "amd64",
			osReleaseErr: errors.New("no such file"),
			want:         Info{OS: "linux", Arch: "amd64"},
		},
		{
			name:    "darwin ignores os-release",
			goos:    "darwin",
			goarch:  "arm64",
			present: []string{"docker"},
			want:    Info{OS: "darwin", Arch: "arm64", Runtimes: []string{"docker"}},
		},
		{
			name:   "windows has no runtimes on PATH",
			goos:   "windows",
			goarch: "amd64",
			want:   Info{OS: "windows", Arch: "amd64"},
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			probe := Probe{
				GOOS:   tc.goos,
				GOARCH: tc.goarch,
				ReadFile: func(path string) ([]byte, error) {
					if path != OSReleasePath {
						t.Fatalf("unexpected path %q", path)
					}
					if tc.osReleaseErr != nil {
						return nil, tc.osReleaseErr
					}
					return []byte(tc.osRelease), nil
				},
				LookPath: func(name string) (string, error) {
					for _, p := range tc.present {
						if p == name {
							return "/usr/bin/" + name, nil
						}
					}
					return "", errors.New("not found")
				},
				RuntimeCandidates: []string{"docker", "podman"},
			}

			got := probe.Detect()
			if !reflect.DeepEqual(got, tc.want) {
				t.Errorf("Info = %+v, want %+v", got, tc.want)
			}
		})
	}
}

func TestInfoString(t *testing.T) {
	info := Info{OS: "linux", Arch: "amd64", Distro: "centos", DistroLike: []string{"rhel", "fedora"}, Runtimes: []string{"docker"}}
	out := info.String()
	for _, want := range []string{"linux", "amd64", "centos", "rhel fedora", "docker"} {
		if !strings.Contains(out, want) {
			t.Errorf("String() missing %q:\n%s", want, out)
		}
	}

	empty := Info{OS: "windows", Arch: "amd64"}.String()
	if !strings.Contains(empty, "distro:  -") || !strings.Contains(empty, "runtimes: -") {
		t.Errorf("String() should render placeholders for empty fields:\n%s", empty)
	}
}
