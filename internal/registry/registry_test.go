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
	want := []string{
		"docker.ps", "docker.psall", "docker.images", "docker.stats", "docker.df",
		"docker.info", "docker.version", "docker.logs", "docker.inspect",
		"docker.networks", "docker.volumes",
		"docker.start", "docker.stop", "docker.restart", "docker.rm", "docker.rmi",
		"docker.exec", "docker.pull", "docker.tag", "docker.save", "docker.load", "docker.cp",
		"docker.clearsafe", "docker.pruneimages", "docker.prunecontainers", "docker.prunenets",
		"docker.prunebuilder", "docker.prunevolumes", "docker.clearall",
		"docker.compose.up", "docker.compose.down", "docker.compose.ps", "docker.compose.logs",
		"diagnose",
		"disk.usage", "net.pids", "net.ports", "proc.nohup", "proc.wait", "java.run", "git.retime", "pkg.refreshindex",
		"user.grant", "user.revoke",
	}
	if got := reg.IDs(); !reflect.DeepEqual(got, want) {
		t.Fatalf("IDs() =\n%v\nwant\n%v", got, want)
	}
}

func TestBuiltinClearsafeIsSafe(t *testing.T) {
	reg := Builtin()
	cmd, ok := reg.Lookup("docker.clearsafe")
	if !ok {
		t.Fatal("docker.clearsafe not found")
	}
	if len(cmd.Variants) < 3 {
		t.Fatalf("expected a variant per platform, got %d", len(cmd.Variants))
	}

	for _, variant := range cmd.Variants {
		if len(variant.Argv) == 0 || variant.Argv[0] != "docker" {
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

func TestProcNohupIsBuiltinEverywhere(t *testing.T) {
	reg := Builtin()
	cmd, ok := reg.Lookup("proc.nohup")
	if !ok {
		t.Fatal("proc.nohup not found")
	}
	if cmd.RequiresYes {
		t.Error("proc.nohup is not destructive and must not require --yes")
	}
	if cmd.MinArgs != 1 || cmd.Usage == "" {
		t.Errorf("proc.nohup must require the command to run, got MinArgs=%d Usage=%q", cmd.MinArgs, cmd.Usage)
	}
	seen := map[string]bool{}
	for _, variant := range cmd.Variants {
		if variant.Builtin != "proc.nohup" || len(variant.Argv) != 0 || variant.Shell {
			t.Errorf("variant for %s must be the proc.nohup builtin, got %+v", variant.Match.OS, variant)
		}
		seen[variant.Match.OS] = true
	}
	for _, osName := range []string{"linux", "darwin", "windows"} {
		if !seen[osName] {
			t.Errorf("proc.nohup lacks a %s variant", osName)
		}
	}
}

func TestDockerCommandsWellFormed(t *testing.T) {
	reg := Builtin()
	destructive := map[string]bool{
		"docker.clearall":     true,
		"docker.prunevolumes": true,
		// rm/rmi now list their targets and require an explicit --yes.
		"docker.rm":  true,
		"docker.rmi": true,
	}
	needsArgs := map[string]int{
		"docker.logs":    1,
		"docker.inspect": 1,
		"docker.start":   1,
		"docker.stop":    1,
		"docker.restart": 1,
		"docker.rm":      1,
		"docker.rmi":     1,
		"docker.exec":    1,
		"docker.pull":    1,
		"docker.tag":     2,
		"docker.save":    1,
		"docker.load":    1,
		"docker.cp":      2,
	}
	// Built from kopsc's own implementation rather than a fixed argv.
	builtin := map[string]string{
		"docker.df":      "docker.df",
		"docker.exec":    "docker.exec",
		"docker.logs":    "docker.logs",
		"docker.stats":   "docker.stats",
		"docker.load":    "docker.load",
		"docker.restart": "docker.restart",
		"docker.rm":      "docker.rm",
		"docker.rmi":     "docker.rmi",
		"docker.save":    "docker.save",
		"docker.stop":    "docker.stop",
	}

	for i := range reg.Commands {
		cmd := &reg.Commands[i]
		if cmd.Group != "docker" {
			continue
		}

		if destructive[cmd.ID] != cmd.RequiresYes {
			t.Errorf("%s: RequiresYes = %v, want %v", cmd.ID, cmd.RequiresYes, destructive[cmd.ID])
		}
		if want, ok := needsArgs[cmd.ID]; ok && cmd.MinArgs != want {
			t.Errorf("%s: MinArgs = %d, want %d", cmd.ID, cmd.MinArgs, want)
		}
		if cmd.MinArgs > 0 && strings.TrimSpace(cmd.Usage) == "" {
			t.Errorf("%s: missing Usage for MinArgs=%d", cmd.ID, cmd.MinArgs)
		}

		platforms := map[string]bool{}
		for _, v := range cmd.Variants {
			if want, ok := builtin[cmd.ID]; ok {
				if v.Builtin != want || len(v.Argv) != 0 {
					t.Errorf("%s: builtin variant = %q with argv %v, want builtin %q",
						cmd.ID, v.Builtin, v.Argv, want)
					continue
				}
				if v.Runtime != v.Match.Runtime || v.Runtime != "docker" {
					t.Errorf("%s: variant runtime %q / match %q, want docker", cmd.ID, v.Runtime, v.Match.Runtime)
				}
				platforms[v.Match.OS] = true
				continue
			}
			if len(v.Argv) == 0 {
				t.Errorf("%s: docker command must have an argv variant", cmd.ID)
				continue
			}
			if v.Argv[0] != "docker" {
				t.Errorf("%s: unexpected program %q", cmd.ID, v.Argv[0])
			}
			if v.Match.Runtime != "docker" {
				t.Errorf("%s: runtime %q, want docker", cmd.ID, v.Match.Runtime)
			}
			platforms[v.Match.OS] = true
		}
		for _, osName := range []string{"linux", "darwin", "windows"} {
			if !platforms[osName] {
				t.Errorf("%s: missing %s variant", cmd.ID, osName)
			}
		}
	}
}

func TestValidateArgs(t *testing.T) {
	logs := &Command{ID: "docker.logs", Usage: "<容器>", MinArgs: 1}
	tag := &Command{ID: "docker.tag", Usage: "<源> <目标>", MinArgs: 2}

	cases := []struct {
		name    string
		cmd     *Command
		args    []string
		wantErr bool
	}{
		{"no args", logs, nil, true},
		{"only option", logs, []string{"-f"}, true},
		{"positional", logs, []string{"web"}, false},
		{"option then positional", logs, []string{"-f", "web"}, false},
		{"option value counted loosely", logs, []string{"--tail", "100"}, false},
		{"interactive option then args", logs, []string{"-it", "web", "sh"}, false},
		{"path flag", &Command{ID: "disk.usage", Usage: "<路径>", MinArgs: 1}, []string{"--path", "/var"}, false},
		{"after double dash", logs, []string{"--", "-weird-name"}, false},
		{"two required", tag, []string{"old:1", "new:1"}, false},
		{"two required short", tag, []string{"old:1"}, true},
		{"option does not satisfy requirement", tag, []string{"-f", "old:1"}, true},
		{"no requirement", &Command{ID: "docker.ps"}, nil, false},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			err := tc.cmd.ValidateArgs(tc.args)
			if tc.wantErr && err == nil {
				t.Fatalf("expected an error for %v", tc.args)
			}
			if !tc.wantErr && err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if err != nil {
				msg := err.Error()
				if !strings.Contains(msg, "kopsc "+strings.ReplaceAll(tc.cmd.ID, ".", " ")) {
					t.Errorf("error lacks usage hint: %q", msg)
				}
			}
		})
	}
}

func TestMatchIsComparable(t *testing.T) {
	a := Match{OS: "linux", Distro: "centos", DistroLike: []string{"rhel"}, Runtime: "docker"}
	b := Match{OS: "linux", Distro: "centos", DistroLike: []string{"rhel"}, Runtime: "docker"}
	if !reflect.DeepEqual(a, b) {
		t.Fatal("Match should be a comparable plain struct value")
	}
}

func TestProcWaitIsBuiltinEverywhere(t *testing.T) {
	reg := Builtin()
	cmd, ok := reg.Lookup("proc.wait")
	if !ok {
		t.Fatal("proc.wait not found")
	}
	// The command is read-only: it must not demand --yes.
	if cmd.RequiresYes {
		t.Error("proc.wait only observes processes and must not require --yes")
	}
	if cmd.Usage == "" {
		t.Error("proc.wait should document its arguments in Usage")
	}
	seen := map[string]bool{}
	for _, v := range cmd.Variants {
		if v.Builtin != "proc.wait" || len(v.Argv) != 0 {
			t.Errorf("variant for %s should be the proc.wait builtin, got %+v", v.Match.OS, v)
		}
		seen[v.Match.OS] = true
	}
	for _, osName := range []string{"linux", "darwin", "windows"} {
		if !seen[osName] {
			t.Errorf("proc.wait lacks a %s variant", osName)
		}
	}
}

func TestJavaRunIsBuiltinEverywhere(t *testing.T) {
	reg := Builtin()
	cmd, ok := reg.Lookup("java.run")
	if !ok {
		t.Fatal("java.run not found")
	}
	if cmd.RequiresYes {
		t.Error("java.run only starts an application; it must not require --yes")
	}
	if cmd.MinArgs != 1 || !strings.Contains(cmd.Usage, ".jar") {
		t.Errorf("java.run must require the jar argument, got MinArgs=%d Usage=%q", cmd.MinArgs, cmd.Usage)
	}
	for _, want := range []string{"JDK", "内存"} {
		if !strings.Contains(cmd.Action, want) {
			t.Errorf("java.run Action should mention %q:\n%s", want, cmd.Action)
		}
	}
	seen := map[string]bool{}
	for _, v := range cmd.Variants {
		if v.Builtin != "java.run" || len(v.Argv) != 0 {
			t.Errorf("variant for %s should be the java.run builtin, got %+v", v.Match.OS, v)
		}
		seen[v.Match.OS] = true
	}
	for _, osName := range []string{"linux", "darwin", "windows"} {
		if !seen[osName] {
			t.Errorf("java.run lacks a %s variant", osName)
		}
	}
}
