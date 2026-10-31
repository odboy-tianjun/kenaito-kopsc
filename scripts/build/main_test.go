package main

import (
	"reflect"
	"strings"
	"testing"
)

func TestParseTargetsDefault(t *testing.T) {
	got, err := parseTargets("")
	if err != nil {
		t.Fatalf("parseTargets(\"\") error: %v", err)
	}
	if !reflect.DeepEqual(got, defaultTargets) {
		t.Fatalf("got %v, want default matrix %v", got, defaultTargets)
	}
}

func TestParseTargetsCustom(t *testing.T) {
	got, err := parseTargets(" linux/amd64 , windows/arm64 ,linux/amd64,")
	if err != nil {
		t.Fatalf("parseTargets error: %v", err)
	}
	want := []target{{"linux", "amd64"}, {"windows", "arm64"}}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("got %v, want %v", got, want)
	}
}

func TestParseTargetsInvalid(t *testing.T) {
	for _, spec := range []string{"linux", "/amd64", "linux/", " , "} {
		if _, err := parseTargets(spec); err == nil {
			t.Errorf("parseTargets(%q) should fail", spec)
		}
	}
}

func TestArtifactName(t *testing.T) {
	cases := []struct {
		version string
		t       target
		want    string
	}{
		{"1.2.3", target{"linux", "amd64"}, "kopsc-1.2.3-linux-amd64"},
		{"v1.2.3-2-gabc-dirty", target{"darwin", "arm64"}, "kopsc-v1.2.3-2-gabc-dirty-darwin-arm64"},
		{"dev", target{"windows", "amd64"}, "kopsc-dev-windows-amd64.exe"},
	}
	for _, tc := range cases {
		if got := artifactName(tc.version, tc.t); got != tc.want {
			t.Errorf("artifactName(%q, %v) = %q, want %q", tc.version, tc.t, got, tc.want)
		}
	}
}

func TestRunRejectsConflictingFlags(t *testing.T) {
	err := run([]string{"-host", "-targets", "linux/amd64"})
	if err == nil || !strings.Contains(err.Error(), "cannot be used together") {
		t.Fatalf("err = %v, want conflict error", err)
	}
}
