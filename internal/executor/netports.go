package executor

import (
	"context"
	"fmt"
	"os/exec"
	"regexp"
	"runtime"
	"sort"
	"strconv"
	"strings"
	"text/tabwriter"
)

// listeningPort is one listening socket with its owner, normalised across
// platforms so the table looks the same everywhere.
type listeningPort struct {
	Proto   string // TCP or UDP
	Address string // local address as reported, e.g. "0.0.0.0", "*", "[::]"
	Port    int
	PID     string // empty when the platform does not expose the owner
	Process string // empty when unknown
}

// listenCommand describes how to list listening sockets on this platform.
type listenCommand struct {
	program string
	args    []string
	parse   func([]byte) []listeningPort
}

// runNetPorts implements `kopsc net ports [--raw]`.
//
// By default the listeners are printed as one table with the same columns on
// every platform; --raw keeps the platform command's own output for scripts
// that parse it.
func runNetPorts(ctx context.Context, req BuiltinRequest) error {
	raw := false
	for _, arg := range req.Args {
		switch arg {
		case "--raw":
			raw = true
		case "--":
		default:
			if strings.HasPrefix(arg, "-") {
				return fmt.Errorf("未知选项 %q（支持 --raw）", arg)
			}
		}
	}

	if raw {
		spec := listenCommandFor(runtime.GOOS)
		cmd := exec.CommandContext(ctx, spec.program, spec.args...)
		cmd.Stdout = req.Stdout
		cmd.Stderr = req.Stderr
		if err := cmd.Run(); err != nil {
			if exitErr, ok := err.(*exec.ExitError); ok && exitErr.ExitCode() == 1 {
				return nil // lsof: nothing is listening
			}
			return fmt.Errorf("%s 执行失败: %w", spec.program, err)
		}
		return nil
	}

	ports, err := listListeningPorts(ctx, runtime.GOOS)
	if err != nil {
		return err
	}
	renderListeningPorts(ports, req)
	return nil
}

// renderListeningPorts writes the table, sorted by port.
func renderListeningPorts(ports []listeningPort, req BuiltinRequest) {
	if len(ports) == 0 {
		if req.Stderr != nil {
			fmt.Fprintln(req.Stderr, "没有正在监听的端口")
		}
		return
	}

	tw := tabwriter.NewWriter(req.Stdout, 0, 0, 2, ' ', 0)
	fmt.Fprintln(tw, "端口\t协议\t地址\tPID\t进程")
	for _, p := range ports {
		pid, process := p.PID, p.Process
		if pid == "" {
			pid, process = "-", "-"
		}
		if process == "" {
			process = "-"
		}
		fmt.Fprintf(tw, "%d\t%s\t%s\t%s\t%s\n", p.Port, p.Proto, p.Address, pid, process)
	}
	_ = tw.Flush()
}

// listenCommandFor picks the platform's command.
func listenCommandFor(goos string) listenCommand {
	switch goos {
	case "linux":
		return listenCommand{program: "ss", args: []string{"-tulnp"}, parse: parseSSListeners}
	case "windows":
		return listenCommand{program: "netstat", args: []string{"-ano"}, parse: parseNetstatListeners}
	default:
		// macOS: lsof is part of the base system and prints the owner name.
		return listenCommand{program: "lsof", args: []string{"-iTCP", "-sTCP:LISTEN", "-nP"}, parse: parseLsofListeners}
	}
}

// listListeningPorts runs the platform command and parses its output.
func listListeningPorts(ctx context.Context, goos string) ([]listeningPort, error) {
	spec := listenCommandFor(goos)
	cmd := exec.CommandContext(ctx, spec.program, spec.args...)
	out, err := cmd.Output()
	if err != nil {
		// lsof exits 1 with no output when nothing is listening.
		if exitErr, ok := err.(*exec.ExitError); ok && exitErr.ExitCode() == 1 && len(out) == 0 {
			return nil, nil
		}
		return nil, fmt.Errorf("%s 执行失败: %w", spec.program, err)
	}
	return spec.parse(out), nil
}

// ssProcessPattern extracts the process name from `ss -p` output such as
// users:(("nginx",pid=1234,fd=6)).
var ssProcessPattern = regexp.MustCompile(`\("([^"]+)"`)

// parseSSListeners parses `ss -tulnp`:
//
//	Netid State  Recv-Q Send-Q Local Address:Port Peer Address:Port Process
//	udp   UNCONN 0      0      0.0.0.0:68         0.0.0.0:*   users:(("dhclient",pid=812,fd=6))
//	tcp   LISTEN 0      128    0.0.0.0:22         0.0.0.0:*   users:(("sshd",pid=901,fd=3))
func parseSSListeners(out []byte) []listeningPort {
	var ports []listeningPort
	for _, line := range strings.Split(string(out), "\n") {
		fields := strings.Fields(line)
		if len(fields) < 5 {
			continue
		}
		proto := strings.ToUpper(fields[0])
		if proto != "TCP" && proto != "UDP" {
			continue
		}
		// ss lists every socket; only listeners (TCP LISTEN, any bound UDP)
		// are interesting, so established connections are skipped.
		if !isListeningState(proto, fields[1]) {
			continue
		}
		addr, port, ok := splitHostPort(fields[4])
		if !ok {
			continue
		}
		entry := listeningPort{Proto: proto, Address: addr, Port: port}
		if m := ssProcessPattern.FindStringSubmatch(line); len(m) == 2 {
			entry.Process = m[1]
		}
		if m := pidPattern.FindStringSubmatch(line); len(m) == 2 {
			entry.PID = m[1]
		}
		ports = append(ports, entry)
	}
	return dedupeListeners(ports)
}

// isListeningState reports whether ss's State column means "reachable
// inbound": TCP needs LISTEN, while UDP has no handshake and a bound socket
// shows as UNCONN. Established connections are not listeners.
func isListeningState(proto, state string) bool {
	switch strings.ToUpper(proto) {
	case "TCP":
		return strings.EqualFold(state, "LISTEN")
	case "UDP":
		return strings.EqualFold(state, "UNCONN")
	}
	return false
}

// lsofListenPattern matches the tail of a lsof row:
//
//	NAME: TCP *:22 (LISTEN)
var lsofListenPattern = regexp.MustCompile(`(TCP|UDP)\s+(\S+)\s+\(LISTEN\)`)

// parseLsofListeners parses `lsof -iTCP -sTCP:LISTEN -nP`:
//
//	COMMAND  PID USER FD TYPE DEVICE SIZE/OFF NODE NAME
//	sshd     901 root 3u IPv4 ...       0t0  TCP *:22 (LISTEN)
func parseLsofListeners(out []byte) []listeningPort {
	var ports []listeningPort
	for _, line := range strings.Split(string(out), "\n") {
		fields := strings.Fields(line)
		if len(fields) < 2 || fields[0] == "COMMAND" {
			continue
		}
		m := lsofListenPattern.FindStringSubmatch(line)
		if len(m) != 3 {
			continue
		}
		addr, port, ok := splitHostPort(m[2])
		if !ok {
			continue
		}
		ports = append(ports, listeningPort{
			Proto:   strings.ToUpper(m[1]),
			Address: addr,
			Port:    port,
			PID:     fields[1],
			Process: fields[0],
		})
	}
	return dedupeListeners(ports)
}

// parseNetstatListeners parses `netstat -ano` on Windows:
//
//	Proto  Local Address     Foreign Address   State      PID
//	TCP    0.0.0.0:135       0.0.0.0:0         LISTENING  1234
//	UDP    0.0.0.0:53        *:*                            5678
func parseNetstatListeners(out []byte) []listeningPort {
	var ports []listeningPort
	for _, line := range strings.Split(string(out), "\n") {
		fields := strings.Fields(line)
		if len(fields) < 3 {
			continue
		}
		proto := strings.ToUpper(fields[0])
		if proto != "TCP" && proto != "UDP" {
			continue
		}
		// TCP rows carry a state column; UDP rows do not. Only listening TCP
		// sockets are interesting, while every bound UDP socket is.
		if proto == "TCP" && (len(fields) < 4 || !strings.EqualFold(fields[3], "LISTENING")) {
			continue
		}
		addr, port, ok := splitHostPort(fields[1])
		if !ok {
			continue
		}
		pid := fields[len(fields)-1]
		if pid == "*" {
			pid = ""
		}
		ports = append(ports, listeningPort{Proto: proto, Address: addr, Port: port, PID: pid})
	}
	return dedupeListeners(ports)
}

// splitHostPort splits "addr:port" from right, so IPv6 forms such as
// [::]:22 and *:7000 work. The brackets around IPv6 addresses are dropped.
func splitHostPort(s string) (string, int, bool) {
	i := strings.LastIndexByte(s, ':')
	if i < 0 {
		return "", 0, false
	}
	port, err := strconv.Atoi(s[i+1:])
	if err != nil || port < 0 || port > 65535 {
		return "", 0, false
	}
	host := strings.TrimSuffix(strings.TrimPrefix(s[:i], "["), "]")
	return host, port, true
}

// dedupeListeners drops rows repeating the same socket (lsof lists the same
// port once per address family) and sorts by port, then protocol.
func dedupeListeners(ports []listeningPort) []listeningPort {
	seen := map[string]bool{}
	out := ports[:0]
	for _, p := range ports {
		key := fmt.Sprintf("%s|%s|%d|%s", p.Proto, p.Address, p.Port, p.PID)
		if seen[key] {
			continue
		}
		seen[key] = true
		out = append(out, p)
	}
	sort.SliceStable(out, func(i, j int) bool {
		if out[i].Port != out[j].Port {
			return out[i].Port < out[j].Port
		}
		if out[i].Proto != out[j].Proto {
			return out[i].Proto < out[j].Proto
		}
		return out[i].Address < out[j].Address
	})
	return out
}
