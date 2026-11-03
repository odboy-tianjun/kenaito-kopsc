package executor

import (
	"bytes"
	"context"
	"fmt"
	"io"

	"gitea.odboy.cn/odboy/kenaito-kopsc/internal/diagnose"
	"gitea.odboy.cn/odboy/kenaito-kopsc/internal/platform"
)

// BuiltinRequest carries the caller's arguments and output streams to an
// in-process command implementation.
type BuiltinRequest struct {
	Args   []string
	Stdin  io.Reader
	Stdout io.Writer
	Stderr io.Writer
	Info   platform.Info
	// Confirmed reports that the user passed --yes; destructive builtins use
	// it instead of asking for confirmation a second time.
	Confirmed bool
	// Plan is the resolved variant and forwarded arguments. Builtins that
	// drive an external runtime (for example docker save) read the runtime's
	// program name and the passthrough arguments from it.
	Plan Plan
}

// Builtin executes a command inside kopsc instead of spawning a child. It is
// used where no portable system utility exists (for example du on Windows).
type Builtin func(ctx context.Context, req BuiltinRequest) error

var builtins = map[string]Builtin{
	"archive.create":   runArchiveCreate,
	"archive.extract":  runArchiveExtract,
	"archive.list":     runArchiveList,
	"disk.check":       runDiskCheck,
	"disk.inode":       runDiskInode,
	"disk.usage":       runDiskUsage,
	"diagnose":         runDiagnose,
	"docker.df":        runDockerDF,
	"docker.exec":      runDockerExec,
	"docker.load":      runDockerLoad,
	"docker.logs":      runDockerLogs,
	"docker.restart":   runDockerRestart,
	"docker.rm":        runDockerRM,
	"docker.rm.preview":  runDockerPreviewRM,
	"docker.rmi":       runDockerRMI,
	"docker.rmi.preview": runDockerPreviewRMI,
	"docker.save":      runDockerSave,
	"docker.stats":     runDockerStats,
	"docker.stop":      runDockerStop,
	"file.empty":       runFileEmpty,
	"file.findbig":     runFileFindBig,
	"file.findname":    runFileFindName,
	"file.findrecent":  runFileFindRecent,
	"fw.allow":         runFwAllow,
	"fw.deny":          runFwDeny,
	"fw.status":        runFwStatus,
	"git.retime":       runGitRetime,
	"git.retime.preview": runGitRetimePreview,
	"git.time":         runGitTime,
	"git.squash":       runGitSquash,
	"git.undo":         runGitUndo,
	"git.clean":        runGitClean,
	"git.sync":         runGitSync,
	"git.discard":      runGitDiscard,
	"java.run":         runJavaRun,
	"java.run.preview": runJavaPreview,
	"log.last":         runLogLast,
	"log.list":         runLogList,
	"log.search":       runLogSearch,
	"net.conn":         runNetConn,
	"net.curl":         runNetCurl,
	"net.dns":          runNetDNS,
	"net.http":         runNetHTTP,
	"net.iface":        runNetIface,
	"net.ping":         runNetPing,
	"net.pids":         runNetPIDs,
	"net.ports":        runNetPorts,
	"net.speed":        runNetSpeed,
	"net.trace":        runNetTrace,
	"pkg.info":         runPkgInfo,
	"pkg.install":      runPkgInstall,
	"pkg.list":         runPkgList,
	"pkg.remove":       runPkgRemove,
	"pkg.search":       runPkgSearch,
	"proc.killname":    runProcKillName,
	"proc.list":        runProcList,
	"proc.nohup":       runNohup,
	"proc.topcpu":      runProcTopCPU,
	"proc.topmem":      runProcTopMem,
	"proc.tree":        runProcTree,
	"proc.wait":        runProcWait,
	"svc.disable":      runSvcDisable,
	"svc.enable":       runSvcEnable,
	"svc.list":         runSvcList,
	"svc.logs":         runSvcLogs,
	"svc.status":       runSvcStatus,
	"sysinfo":          runSysInfo,
	"user.grant":       runUserGrant,
	"user.info":        runUserInfo,
	"user.list":        runUserList,
	"user.revoke":      runUserRevoke,
}

// runDiagnose bridges the registry builtin to the diagnose package.
func runDiagnose(ctx context.Context, req BuiltinRequest) error {
	service := diagnose.New(req.Info, diagnose.ExecRunner{}, req.Stdout, req.Stderr)
	return service.Run(ctx, req.Args)
}

// Preview runs a command's ConfirmBuiltin (if any) and writes the list of
// objects the command would act on. It is used for the confirmation prompt and
// for --dry-run, so neither path needs to touch the objects themselves.
//
// The list is built from the container/image references kopsc resolves, which
// means every target is validated first: an unknown target is reported here
// rather than after the user typed --yes. Commands without a ConfirmBuiltin
// print nothing.
func Preview(ctx context.Context, plan Plan, w io.Writer) error {
	name := plan.PreviewBuiltin()
	if name == "" {
		return nil
	}
	fn, ok := LookupBuiltin(name)
	if !ok {
		return fmt.Errorf("未注册的确认动作 %q", name)
	}
	// Build the list in memory first: a target that fails to resolve must not
	// leave a dangling heading behind.
	var buf bytes.Buffer
	if err := fn(ctx, BuiltinRequest{
		Args:   plan.ExtraArgs,
		Stdout: &buf,
		Stderr: io.Discard,
		Info:   plan.Result.Info,
		Plan:   plan,
	}); err != nil {
		return err
	}
	// Only destructive commands act on a list of objects; other previews (such
	// as the java command line) already carry their own heading.
	if plan.Result.Command != nil && plan.Result.Command.RequiresYes {
		fmt.Fprintln(w, "将影响:")
	}
	_, err := w.Write(buf.Bytes())
	return err
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
