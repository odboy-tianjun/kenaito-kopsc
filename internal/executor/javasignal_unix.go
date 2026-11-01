//go:build !windows

package executor

import (
	"os"
	"os/exec"
	"os/signal"
	"syscall"
)

// forwardSignals relays interrupts to the JVM so its shutdown hooks run
// instead of the process being torn down abruptly.
//
// The JVM shares kopsc's process group, so a terminal Ctrl+C already reaches
// it; this relay also covers the case where a supervisor signals kopsc alone.
// The returned stop function must be called once the child has been waited for.
func forwardSignals(cmd *exec.Cmd) (stop func()) {
	signals := make(chan os.Signal, 1)
	signal.Notify(signals, os.Interrupt, syscall.SIGTERM)

	done := make(chan struct{})
	go func() {
		for {
			select {
			case sig := <-signals:
				if cmd.Process != nil {
					_ = cmd.Process.Signal(sig)
				}
			case <-done:
				return
			}
		}
	}()

	return func() {
		signal.Stop(signals)
		close(done)
	}
}
