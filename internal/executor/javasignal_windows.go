//go:build windows

package executor

import "os/exec"

// forwardSignals is a no-op on Windows: there is no SIGINT to relay, and
// console Ctrl+C is delivered to every attached process already.
func forwardSignals(cmd *exec.Cmd) (stop func()) {
	return func() {}
}
