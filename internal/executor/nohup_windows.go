//go:build windows

package executor

import (
	"os"
	"os/exec"
	"syscall"
)

// prepareNohup applies the Windows half of `nohup ... &`: the child's stdin is
// connected to NUL and its stdout and stderr are appended to logFile
// ("> output.log 2>&1").
//
// Windows has no SIGHUP; what the shell's `nohup ... &` buys is independence
// from the launching console, which DETACHED_PROCESS provides: the child gets
// no console at all, so closing the terminal kopsc ran in does not close the
// child either. With detach disabled the child shares kopsc's console.
func prepareNohup(cmd *exec.Cmd, logFile *os.File, detach bool) {
	cmd.Stdout = logFile
	cmd.Stderr = logFile

	if detach {
		// DETACHED_PROCESS: start without a console, and do not inherit one.
		cmd.SysProcAttr = &syscall.SysProcAttr{
			CreationFlags: syscall.CREATE_NEW_PROCESS_GROUP | detachedProcess,
		}
	}
}

// DETACHED_PROCESS (0x00000008) is not exported by package syscall.
const detachedProcess = 0x00000008
