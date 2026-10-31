//go:build !windows

package executor

import (
	"os"
	"os/exec"
	"syscall"
)

// prepareNohup applies the Unix half of `nohup ... &`: the child's stdin is
// closed, stdout and stderr are appended to logFile ("> output.log 2>&1") and,
// with detach, the child becomes a session leader with no controlling terminal
// — which also shields it from terminal-generated SIGHUP/SIGINT/SIGQUIT.
//
// Go's os/exec cannot reset a child's signal dispositions before exec (there is
// no hook between fork and exec), so "ignore SIGHUP" itself is only provided by
// the detached session, not by a signal handler.
func prepareNohup(cmd *exec.Cmd, logFile *os.File, detach bool) {
	// A nil stdin hands the child /dev/null instead of the terminal.
	cmd.Stdout = logFile
	cmd.Stderr = logFile

	if detach {
		cmd.SysProcAttr = &syscall.SysProcAttr{Setsid: true}
	}
}
