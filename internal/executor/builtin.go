package executor

import (
	"context"
	"io"
)

// BuiltinRequest carries the caller's arguments and output streams to an
// in-process command implementation.
type BuiltinRequest struct {
	Args   []string
	Stdout io.Writer
	Stderr io.Writer
}

// Builtin executes a command inside kopsc instead of spawning a child. It is
// used where no portable system utility exists (for example du on Windows).
type Builtin func(ctx context.Context, req BuiltinRequest) error

var builtins = map[string]Builtin{
	"disk.usage": runDiskUsage,
}

// LookupBuiltin returns the implementation registered under name.
func LookupBuiltin(name string) (Builtin, bool) {
	fn, ok := builtins[name]
	return fn, ok
}

// BuiltinNames lists the registered builtin implementations.
func BuiltinNames() []string {
	names := make([]string, 0, len(builtins))
	for name := range builtins {
		names = append(names, name)
	}
	return names
}
