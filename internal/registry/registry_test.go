package registry

import (
	"reflect"
	"strings"
	"testing"
)

func TestBuiltinIsValid(t *testing.T) {
	reg := Builtin()
	if err := reg.Validate(); err != nil {
		t.Fatalf("Builtin() is invalid: %v", err)
	}
}

func TestBuiltinIDsStable(t *testing.T) {
	reg := Builtin()
	want := []string{"docker.clearsafe", "disk.usage", "net.ports", "pkg.refreshindex"}
	if got := reg.IDs(); !reflect.DeepEqual(got, want) {
		t.Fatalf("IDs() = %v, want %v", got, want)
	}
}

func TestBuiltinClearsafeIsSafe(t *testing.T) {
	reg := Builtin()
	cmd, ok := reg.Lookup("docker.clearsafe")
	if !ok {
		t.Fatal("docker.clearsafe not found")
	}
	if len(cmd.Variants) < 4 {
		t.Fatalf("expected a variant per platform, got %d", len(cmd.Variants))
	}

	for _, variant := range cmd.Variants {
		if len(variant.Argv) == 0 || (variant.Argv[0] != "docker" && variant.Argv[0] != "podman") {
			t.Errorf("unexpected program %v", variant.Argv)
		}
		if strings.Join(variant.Argv, " ") != variant.Argv[0]+" system prune -f" {
			t.Errorf("clearsafe must stay scoped to `system prune -f`, got %v", variant.Argv)
		}
		for _, forbidden := range []string{"-a", "--all", "--volumes", "-v", "--force=false"} {
			for _, arg := range variant.Argv {
				if arg == forbidden {
					t.Errorf("variant %v must not pass %q", variant.Argv, forbidden)
				}
			}
		}
	}
}

func TestLookupAlias(t *testing.T) {
	reg := Builtin()
	cmd, ok := reg.Lookup("docker.prunesafe")
	if !ok {
		t.Fatal("alias lookup failed")
	}
	if cmd.ID != "docker.clearsafe" {
		t.Fatalf("alias resolved to %q", cmd.ID)
	}
	if _, ok := reg.Lookup("does.not.exist"); ok {
		t.Fatal("unexpected lookup hit")
	}
}

func TestVariantPlatformsExplicit(t *testing.T) {
	for _, cmd := range Builtin().Commands {
		for _, variant := range cmd.Variants {
			if variant.Match.OS == "" {
				t.Errorf("command %q has a variant without an OS constraint: %v", cmd.ID, variant.Argv)
			}
		}
	}
}

func plainCommand(id string) Command {
	return Command{
		ID:       id,
		Summary:  "summary",
		Variants: []Variant{{Argv: []string{"x"}}},
	}
}

func TestValidateErrors(t *testing.T) {
	withBlankDistroLike := plainCommand("a")
	withBlankDistroLike.Variants = []Variant{{
		Match: Match{DistroLike: []string{" "}},
		Argv:  []string{"x"},
	}}

	withEmptyArgv := plainCommand("a")
	withEmptyArgv.Variants = []Variant{{}}

	noVariants := plainCommand("a")
	noVariants.Variants = nil

	noSummary := plainCommand("a")
	noSummary.Summary = ""

	noID := plainCommand("")
	noID.ID = ""

	aliased := func(first, second []string) Registry {
		a, b := plainCommand("a"), plainCommand("b")
		a.Aliases, b.Aliases = first, second
		return Registry{Commands: []Command{a, b}}
	}

	tests := []struct {
		name     string
		registry Registry
		wantErr  string
	}{
		{"empty ID", Registry{Commands: []Command{noID}}, "empty ID"},
		{"empty summary", Registry{Commands: []Command{noSummary}}, "empty summary"},
		{"duplicate ID", Registry{Commands: []Command{plainCommand("a"), plainCommand("a")}}, "duplicate ID"},
		{"duplicate alias", aliased([]string{"z"}, []string{"z"}), "already used"},
		{"no variants", Registry{Commands: []Command{noVariants}}, "no variants"},
		{"empty argv", Registry{Commands: []Command{withEmptyArgv}}, "empty argv"},
		{"blank DistroLike", Registry{Commands: []Command{withBlankDistroLike}}, "empty DistroLike"},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			err := tc.registry.Validate()
			if err == nil {
				t.Fatal("expected an error")
			}
			if !strings.Contains(err.Error(), tc.wantErr) {
				t.Fatalf("error %q does not contain %q", err, tc.wantErr)
			}
		})
	}
}

func TestValidateBuiltinVariants(t *testing.T) {
	builtinOnly := plainCommand("a")
	builtinOnly.Variants = []Variant{{Match: Match{OS: "linux"}, Builtin: "demo.action"}}

	if err := (&Registry{Commands: []Command{builtinOnly}}).Validate(); err != nil {
		t.Fatalf("builtin-only variant should be valid: %v", err)
	}

	both := plainCommand("a")
	both.Variants = []Variant{{Match: Match{OS: "linux"}, Argv: []string{"x"}, Builtin: "demo.action"}}
	err := (&Registry{Commands: []Command{both}}).Validate()
	if err == nil || !strings.Contains(err.Error(), "mutually exclusive") {
		t.Fatalf("err = %v, want mutual-exclusion error", err)
	}
}

func TestDiskUsageIsBuiltinEverywhere(t *testing.T) {
	reg := Builtin()
	cmd, ok := reg.Lookup("disk.usage")
	if !ok {
		t.Fatal("disk.usage not found")
	}
	seen := map[string]bool{}
	for _, variant := range cmd.Variants {
		if variant.Builtin != "disk.usage" {
			t.Errorf("variant for %s should use the disk.usage builtin, got %q", variant.Match.OS, variant.Builtin)
		}
		if len(variant.Argv) != 0 {
			t.Errorf("builtin variant must not carry argv: %v", variant.Argv)
		}
		seen[variant.Match.OS] = true
	}
	for _, osName := range []string{"linux", "darwin", "windows"} {
		if !seen[osName] {
			t.Errorf("disk.usage lacks a %s variant", osName)
		}
	}
}

func TestMatchIsComparable(t *testing.T) {
	a := Match{OS: "linux", Distro: "centos", DistroLike: []string{"rhel"}, Runtime: "docker"}
	b := Match{OS: "linux", Distro: "centos", DistroLike: []string{"rhel"}, Runtime: "docker"}
	if !reflect.DeepEqual(a, b) {
		t.Fatal("Match should be a comparable plain struct value")
	}
}
