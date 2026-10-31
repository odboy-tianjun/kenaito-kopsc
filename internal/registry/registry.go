// Package registry holds kopsc's built-in command registry: platform-neutral
// intents, each with one or more platform-specific variants.
package registry

import (
	"fmt"
	"strings"
)

// Match constrains a Variant to environments. An empty field means "any".
// A Variant with a completely empty Match is a generic fallback.
type Match struct {
	OS         string   // runtime.GOOS
	Arch       string   // runtime.GOARCH
	Distro     string   // exact os-release ID, e.g. "centos"
	DistroLike []string // os-release ID_LIKE ancestry, e.g. "rhel"
	Runtime    string   // container runtime required on PATH, e.g. "podman"
}

// Variant is one concrete implementation of a command.
type Variant struct {
	Match Match
	Argv  []string // program followed by arguments
	Shell bool     // run through the platform shell instead of exec'ing Argv[0]
	Note  string   // human-readable rationale shown by `kopsc explain`
}

// Command is a platform-neutral intent, e.g. "docker.clearsafe".
type Command struct {
	ID       string // hierarchical, dot-separated: "docker.clearsafe"
	Aliases  []string
	Summary  string
	Group    string // CLI grouping, e.g. "docker"
	Action   string // what the command does, including its safety contract
	Variants []Variant
}

// Registry is an ordered set of commands.
type Registry struct {
	Commands []Command
}

// Validate reports structural problems: empty or duplicate identifiers,
// empty aliases, commands without variants and variants without an argv.
func (r *Registry) Validate() error {
	seenIDs := make(map[string]bool)
	seenAliases := make(map[string]string)

	for i, cmd := range r.Commands {
		if strings.TrimSpace(cmd.ID) == "" {
			return fmt.Errorf("command #%d: empty ID", i)
		}
		if seenIDs[cmd.ID] {
			return fmt.Errorf("command #%d: duplicate ID %q", i, cmd.ID)
		}
		seenIDs[cmd.ID] = true

		if strings.TrimSpace(cmd.Summary) == "" {
			return fmt.Errorf("command %q: empty summary", cmd.ID)
		}
		if len(cmd.Variants) == 0 {
			return fmt.Errorf("command %q: no variants", cmd.ID)
		}

		for _, alias := range cmd.Aliases {
			if strings.TrimSpace(alias) == "" {
				return fmt.Errorf("command %q: empty alias", cmd.ID)
			}
			if owner, ok := seenAliases[alias]; ok {
				return fmt.Errorf("command %q: alias %q already used by %q", cmd.ID, alias, owner)
			}
			seenAliases[alias] = cmd.ID
		}

		for j, variant := range cmd.Variants {
			if len(variant.Argv) == 0 || strings.TrimSpace(variant.Argv[0]) == "" {
				return fmt.Errorf("command %q variant #%d: empty argv", cmd.ID, j)
			}
			if err := variant.Match.validate(); err != nil {
				return fmt.Errorf("command %q variant #%d: %w", cmd.ID, j, err)
			}
		}
	}

	return nil
}

func (m Match) validate() error {
	for _, field := range m.DistroLike {
		if strings.TrimSpace(field) == "" {
			return fmt.Errorf("empty DistroLike entry")
		}
	}
	return nil
}

// Lookup finds a command by ID or alias.
func (r *Registry) Lookup(idOrAlias string) (*Command, bool) {
	for i := range r.Commands {
		cmd := &r.Commands[i]
		if cmd.ID == idOrAlias {
			return cmd, true
		}
		for _, alias := range cmd.Aliases {
			if alias == idOrAlias {
				return cmd, true
			}
		}
	}
	return nil, false
}

// IDs returns every command ID in registry order.
func (r *Registry) IDs() []string {
	ids := make([]string, 0, len(r.Commands))
	for _, cmd := range r.Commands {
		ids = append(ids, cmd.ID)
	}
	return ids
}
