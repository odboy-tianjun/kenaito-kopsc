// Package diagnose explains why a container or Pod is in its current state.
// It knows how to inspect Docker containers and Kubernetes Pods, and
// falls back to a built-in implementation so the output is identical on every
// platform.
package diagnose

import (
	"bytes"
	"context"
	"errors"
	"os/exec"
)

// CommandResult is the output of one external command.
type CommandResult struct {
	Stdout   []byte
	Stderr   []byte
	ExitCode int
}

// Runner executes external programs. It is an interface so diagnoses can be
// tested without docker or kubectl.
type Runner interface {
	LookPath(name string) (string, error)
	Run(ctx context.Context, name string, args ...string) (CommandResult, error)
}

// ExecRunner runs commands on the host.
type ExecRunner struct{}

// LookPath reports whether a program is available on PATH.
func (ExecRunner) LookPath(name string) (string, error) {
	return exec.LookPath(name)
}

// Run executes a command and captures its output.
func (ExecRunner) Run(ctx context.Context, name string, args ...string) (CommandResult, error) {
	cmd := exec.CommandContext(ctx, name, args...)
	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr

	err := cmd.Run()
	result := CommandResult{Stdout: stdout.Bytes(), Stderr: stderr.Bytes()}
	if err == nil {
		return result, nil
	}

	var exitErr *exec.ExitError
	if errors.As(err, &exitErr) {
		result.ExitCode = exitErr.ExitCode()
	}
	return result, err
}
