package executor

import (
	"bytes"
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// linuxSS is `ss -tulnp` output in the shape iproute2 prints it.
const linuxSS = `Netid State  Recv-Q Send-Q Local Address:Port Peer Address:Port Process
udp   UNCONN 0      0      0.0.0.0:68          0.0.0.0:*     users:(("dhclient",pid=812,fd=6))
tcp   LISTEN 0      128    0.0.0.0:22          0.0.0.0:*     users:(("sshd",pid=901,fd=3))
tcp   LISTEN 0      511    [::]:80             [::]:*        users:(("nginx",pid=1200,fd=6))
tcp   LISTEN 0      4096   127.0.0.1:6379      0.0.0.0:*     users:(("redis-server",pid=1500,fd=6))
tcp   ESTAB  0      0      10.0.0.5:54321      10.0.0.9:443
`

// macOSLsof is `lsof -iTCP -sTCP:LISTEN -nP` output.
const macOSLsof = `COMMAND     PID USER   FD   TYPE             DEVICE SIZE/OFF NODE NAME
ControlCe   446  mac    8u  IPv4 0xc6e1f3949f65dbaf      0t0  TCP *:7000 (LISTEN)
ControlCe   446  mac    9u  IPv6 0xc1e379d06e632fc8      0t0  TCP *:7000 (LISTEN)
node       6200  mac   43u  IPv6  0x1228305ce20b957      0t0  TCP [::1]:5173 (LISTEN)
`

// windowsNetstat is `netstat -ano` output.
const windowsNetstat = `Active Connections

  Proto  Local Address          Foreign Address        State           PID
  TCP    0.0.0.0:135            0.0.0.0:0              LISTENING       1234
  TCP    127.0.0.1:5432         0.0.0.0:0              LISTENING       5678
  TCP    10.0.0.5:49234         10.0.0.9:443           ESTABLISHED     5678
  UDP    0.0.0.0:53             *:*                                    4321
`

func TestParseSSListeners(t *testing.T) {
	got := parseSSListeners([]byte(linuxSS))
	if len(got) != 4 {
		t.Fatalf("parsed %d listeners, want 4 (the ESTAB row is not listening):\n%+v", len(got), got)
	}

	// Sorted by port.
	wantPorts := []int{22, 68, 80, 6379}
	for i, want := range wantPorts {
		if got[i].Port != want {
			t.Fatalf("row %d port = %d, want %d (%+v)", i, got[i].Port, want, got)
		}
	}

	byPort := map[int]listeningPort{}
	for _, p := range got {
		byPort[p.Port] = p
	}
	if p := byPort[22]; p.Proto != "TCP" || p.Process != "sshd" || p.PID != "901" || p.Address != "0.0.0.0" {
		t.Errorf("port 22 = %+v", p)
	}
	if p := byPort[80]; p.Address != "::" {
		t.Errorf("IPv6 bracket not stripped: %+v", p)
	}
	if p := byPort[68]; p.Proto != "UDP" || p.Process != "dhclient" {
		t.Errorf("udp row = %+v", p)
	}
}

func TestParseLsofListeners(t *testing.T) {
	got := parseLsofListeners([]byte(macOSLsof))
	// The two ControlCe rows are the same socket in two families and collapse.
	if len(got) != 2 {
		t.Fatalf("parsed %d listeners, want 2:\n%+v", len(got), got)
	}
	if got[0].Port != 5173 || got[0].Address != "::1" || got[0].Process != "node" || got[0].PID != "6200" {
		t.Errorf("first row = %+v", got[0])
	}
	if got[1].Port != 7000 || got[1].Process != "ControlCe" || got[1].PID != "446" {
		t.Errorf("second row = %+v", got[1])
	}
}

func TestParseNetstatListeners(t *testing.T) {
	got := parseNetstatListeners([]byte(windowsNetstat))
	// ESTABLISHED TCP rows are dropped; the bound UDP socket is kept.
	if len(got) != 3 {
		t.Fatalf("parsed %d listeners, want 3:\n%+v", len(got), got)
	}
	if got[0].Port != 53 || got[0].Proto != "UDP" || got[0].PID != "4321" {
		t.Errorf("udp row = %+v", got[0])
	}
	if got[1].Port != 135 || got[1].PID != "1234" {
		t.Errorf("port 135 = %+v", got[1])
	}
	if got[2].Port != 5432 || got[2].Address != "127.0.0.1" {
		t.Errorf("port 5432 = %+v", got[2])
	}
}

func TestListenCommandFor(t *testing.T) {
	cases := map[string][]string{
		"linux":   {"ss", "-tulnp"},
		"darwin":  {"lsof", "-iTCP", "-sTCP:LISTEN", "-nP"},
		"windows": {"netstat", "-ano"},
	}
	for goos, want := range cases {
		spec := listenCommandFor(goos)
		got := append([]string{spec.program}, spec.args...)
		if strings.Join(got, " ") != strings.Join(want, " ") {
			t.Errorf("%s: command = %v, want %v", goos, got, want)
		}
		if spec.parse == nil {
			t.Errorf("%s: no parser", goos)
		}
	}
}

func TestSplitHostPort(t *testing.T) {
	cases := []struct {
		in   string
		addr string
		port int
		ok   bool
	}{
		{"0.0.0.0:22", "0.0.0.0", 22, true},
		{"*:7000", "*", 7000, true},
		{"[::]:80", "::", 80, true},
		{"[::1]:5173", "::1", 5173, true},
		{"::1:5432", "::1", 5432, true},
		{"nope", "", 0, false},
		{"host:99999", "", 0, false},
	}
	for _, tc := range cases {
		addr, port, ok := splitHostPort(tc.in)
		if ok != tc.ok || addr != tc.addr || port != tc.port {
			t.Errorf("splitHostPort(%q) = (%q, %d, %v), want (%q, %d, %v)",
				tc.in, addr, port, ok, tc.addr, tc.port, tc.ok)
		}
	}
}

func TestRenderListeningPortsTable(t *testing.T) {
	ports := dedupeListeners(parseSSListeners([]byte(linuxSS)))

	var out bytes.Buffer
	renderListeningPorts(ports, BuiltinRequest{Stdout: &out, Stderr: &out})

	lines := strings.Split(strings.TrimSpace(out.String()), "\n")
	header := lines[0]
	for _, col := range []string{"端口", "协议", "地址", "PID", "进程"} {
		if !strings.Contains(header, col) {
			t.Errorf("header %q missing column %q", header, col)
		}
	}
	if len(lines) != 5 {
		t.Fatalf("expected header + 4 rows, got %d lines:\n%s", len(lines), out.String())
	}
	// Rows are ordered by port, with the owner columns filled in.
	if !strings.HasPrefix(strings.TrimSpace(lines[1]), "22") || !strings.Contains(lines[1], "sshd") {
		t.Errorf("first row = %q", lines[1])
	}
	if !strings.Contains(out.String(), "redis-server") {
		t.Errorf("process column missing redis-server:\n%s", out.String())
	}
}

func TestRenderListeningPortsEmpty(t *testing.T) {
	var out, errOut bytes.Buffer
	renderListeningPorts(nil, BuiltinRequest{Stdout: &out, Stderr: &errOut})
	if out.Len() != 0 {
		t.Errorf("stdout = %q, want empty", out.String())
	}
	if !strings.Contains(errOut.String(), "没有正在监听的端口") {
		t.Errorf("stderr = %q, want a no-listener note", errOut.String())
	}
}

func TestRenderListeningPortsUnknownOwner(t *testing.T) {
	ports := []listeningPort{{Proto: "UDP", Address: "0.0.0.0", Port: 53}}
	var out bytes.Buffer
	renderListeningPorts(ports, BuiltinRequest{Stdout: &out, Stderr: &out})
	if !strings.Contains(out.String(), "53") || strings.Count(out.String(), "-") < 2 {
		t.Errorf("unknown owner should print placeholders:\n%s", out.String())
	}
}

func TestNetPortsRejectsUnknownOption(t *testing.T) {
	err := runNetPorts(context.Background(), BuiltinRequest{Args: []string{"--nope"}, Stdout: &bytes.Buffer{}})
	if err == nil || !strings.Contains(err.Error(), "未知选项") {
		t.Fatalf("err = %v, want an unknown-option error", err)
	}
}

// TestNetPortsRawKeepsCommandOutput feeds a fake `ss` and checks that --raw
// prints exactly what the platform command printed.
func TestNetPortsRawKeepsCommandOutput(t *testing.T) {
	dir := t.TempDir()
	script := filepath.Join(dir, "ss")
	payload := "tcp LISTEN 0 128 0.0.0.0:22 0.0.0.0:* users:((\"sshd\",pid=901,fd=3))\n"
	body := "#!/bin/sh\nprintf '%s' " + strconvQuote(payload) + "\n"
	if err := os.WriteFile(script, []byte(body), 0o755); err != nil {
		t.Fatal(err)
	}
	// darwin resolves to lsof, so the fake answers under that name too.
	if err := os.WriteFile(filepath.Join(dir, "lsof"), []byte(body), 0o755); err != nil {
		t.Fatal(err)
	}
	t.Setenv("PATH", dir+string(os.PathListSeparator)+os.Getenv("PATH"))

	var out bytes.Buffer
	if err := runNetPorts(context.Background(), BuiltinRequest{Args: []string{"--raw"}, Stdout: &out, Stderr: &out}); err != nil {
		t.Fatalf("runNetPorts --raw: %v", err)
	}
	if strings.Contains(out.String(), "端口") {
		t.Errorf("--raw must not render the table:\n%s", out.String())
	}
}

// strconvQuote single-quotes a shell payload, escaping embedded quotes.
func strconvQuote(s string) string {
	return "'" + strings.ReplaceAll(s, "'", `'\''`) + "'"
}

func TestDisplayWidth(t *testing.T) {
	cases := map[string]int{
		"端口":        4, // each CJK rune is two columns
		"PID":       3,
		"127.0.0.1": 9,
		"进程":        4,
		"":          0,
	}
	for in, want := range cases {
		if got := displayWidth(in); got != want {
			t.Errorf("displayWidth(%q) = %d, want %d", in, got, want)
		}
	}
}

// TestRenderListeningPortsAlignsCJKHeader guards the alignment bug: with the
// old byte-based tabwriter the CJK headings pushed every data column, because
// each heading rune counts as three bytes but only two columns.
func TestRenderListeningPortsAlignsCJKHeader(t *testing.T) {
	ports := []listeningPort{
		{Proto: "TCP", Address: "127.0.0.1", Port: 22, PID: "901", Process: "sshd"},
		{Proto: "TCP", Address: "0.0.0.0", Port: 3306, PID: "2442", Process: "docker"},
	}
	var out bytes.Buffer
	renderListeningPorts(ports, BuiltinRequest{Stdout: &out, Stderr: &out})

	lines := strings.Split(strings.TrimRight(out.String(), "\n"), "\n")
	if len(lines) != 3 {
		t.Fatalf("expected header + 2 rows, got %d:\n%s", len(lines), out.String())
	}

	// "协议" is the heading above the "TCP" column, so that cell must start at
	// the same display column on every line — the bug put it further right.
	want := columnStart(lines[0], "协议")
	for i, line := range lines[1:] {
		if got := columnStart(line, "TCP"); got != want {
			t.Errorf("row %d: TCP starts at display column %d, header 协议 at %d:\n%s",
				i+1, got, want, out.String())
		}
	}

	for _, line := range lines {
		if line != strings.TrimRight(line, " ") {
			t.Errorf("trailing spaces in %q", line)
		}
	}
}

// columnStart returns the display column at which cell begins in line.
func columnStart(line, cell string) int {
	i := strings.Index(line, cell)
	if i < 0 {
		return -1
	}
	return displayWidth(line[:i])
}

func TestDisplayAddress(t *testing.T) {
	cases := map[string]string{
		"0.0.0.0":   "*",
		"::":        "*",
		"":          "*",
		"127.0.0.1": "127.0.0.1",
		"::1":       "::1",
	}
	for in, want := range cases {
		if got := displayAddress(in); got != want {
			t.Errorf("displayAddress(%q) = %q, want %q", in, got, want)
		}
	}
}
