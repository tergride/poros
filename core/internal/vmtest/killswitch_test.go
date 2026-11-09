//go:build windows

package vmtest

import (
	"encoding/json"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/url"
	"os"
	"os/exec"
	"path/filepath"
	"syscall"
	"testing"
	"time"
	"unsafe"

	"golang.org/x/sys/windows"
)

// outside is an address on the internet the test dials by number, so that a refused name lookup cannot pass for a
// block.
const outside = "1.1.1.1:80"

// reachable tells whether the test, a program other than Poros, gets out.
func reachable() bool {
	conn, err := net.DialTimeout("tcp", outside, 5*time.Second)
	if err != nil {
		return false
	}
	conn.Close()
	return true
}

// answers tells whether the outside address answers over HTTP. With the TUN up a connection alone proves nothing:
// the TUN accepts it before the server is reached.
func answers() bool {
	client := &http.Client{
		Timeout:       8 * time.Second,
		Transport:     &http.Transport{Proxy: nil, DisableKeepAlives: true},
		CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse },
	}
	response, err := client.Get("http://" + outside + "/")
	if err != nil {
		return false
	}
	response.Body.Close()
	return true
}

// waitReachable waits for the network to be as wanted; the guard takes a moment to turn the block on or off.
func waitReachable(t *testing.T, want bool, what string) {
	t.Helper()
	deadline := time.Now().Add(20 * time.Second)
	for reachable() != want {
		if time.Now().After(deadline) {
			t.Fatalf("%s: the internet is reachable=%v, want %v", what, !want, want)
		}
		time.Sleep(time.Second)
	}
}

// fetchThroughPoros opens the probe page through Poros's local proxy; the preset sends it direct, out of the core.
func fetchThroughPoros() error {
	proxy := &url.URL{Scheme: "http", Host: "127.0.0.1:2090"}
	client := &http.Client{Timeout: 15 * time.Second, Transport: &http.Transport{Proxy: http.ProxyURL(proxy), DisableKeepAlives: true}}
	response, err := client.Get(probeURL)
	if err != nil {
		return err
	}
	defer response.Body.Close()
	body, _ := io.ReadAll(response.Body)
	if string(body) != probeText {
		return fmt.Errorf("unexpected answer %q", body)
	}
	return nil
}

// limitedToken is this process's token without the administrators and their privileges, at medium integrity: the
// token of a Poros window, and of a core started without UAC.
func limitedToken(t *testing.T) windows.Token {
	t.Helper()
	var own windows.Token
	access := uint32(windows.TOKEN_DUPLICATE | windows.TOKEN_ASSIGN_PRIMARY | windows.TOKEN_QUERY | windows.TOKEN_ADJUST_DEFAULT)
	if err := windows.OpenProcessToken(windows.CurrentProcess(), access, &own); err != nil {
		t.Fatal(err)
	}
	defer own.Close()
	var disable []windows.SIDAndAttributes
	for _, kind := range []windows.WELL_KNOWN_SID_TYPE{windows.WinBuiltinAdministratorsSid, windows.WinLocalAccountAndAdministratorSid} {
		sid, err := windows.CreateWellKnownSid(kind)
		if err != nil {
			t.Fatal(err)
		}
		disable = append(disable, windows.SIDAndAttributes{Sid: sid})
	}
	const disableMaxPrivilege = 0x1
	var limited windows.Token
	r, _, err := windows.NewLazySystemDLL("advapi32.dll").NewProc("CreateRestrictedToken").Call(
		uintptr(own), disableMaxPrivilege, uintptr(len(disable)), uintptr(unsafe.Pointer(&disable[0])), 0, 0, 0, 0, uintptr(unsafe.Pointer(&limited)))
	if r == 0 {
		t.Fatalf("CreateRestrictedToken: %v", err)
	}
	t.Cleanup(func() { limited.Close() })
	medium, err := windows.CreateWellKnownSid(windows.WinMediumLabelSid)
	if err != nil {
		t.Fatal(err)
	}
	label := windows.Tokenmandatorylabel{Label: windows.SIDAndAttributes{Sid: medium, Attributes: windows.SE_GROUP_INTEGRITY}}
	if err = windows.SetTokenInformation(limited, windows.TokenIntegrityLevel, (*byte)(unsafe.Pointer(&label)), label.Size()); err != nil {
		t.Fatalf("medium integrity: %v", err)
	}
	// Both administrator groups go: with the local one left, Windows still counts the token as elevated.
	if limited.IsElevated() {
		t.Fatal("the restricted token still counts as elevated")
	}
	return limited
}

// startWindow stands for Poros.exe: a process of the user without rights, which the guard lives as long as.
func startWindow(t *testing.T, token windows.Token) *exec.Cmd {
	t.Helper()
	window := exec.Command("ping", "-n", "3600", "127.0.0.1")
	window.SysProcAttr = &syscall.SysProcAttr{Token: syscall.Token(token), HideWindow: true}
	if err := window.Start(); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { window.Process.Kill(); window.Wait() })
	return window
}

func (s *shell) killSwitch() map[string]any {
	s.t.Helper()
	var state map[string]any
	json.Unmarshal(s.call("killSwitch.state", nil), &state)
	return state
}

func (s *shell) waitKillSwitch(t *testing.T, check func(map[string]any) bool, what string) {
	t.Helper()
	deadline := time.Now().Add(30 * time.Second)
	for {
		state := s.killSwitch()
		if check(state) {
			return
		}
		if time.Now().After(deadline) {
			t.Fatalf("%s: the kill switch is %v", what, state)
		}
		time.Sleep(250 * time.Millisecond)
	}
}

func blockOn(state map[string]any) bool { return state["on"] == true }

// fullBlock is the block of Windows without Poros's TUN: everything but Poros and the local network waits.
func fullBlock(state map[string]any) bool { return state["on"] == true && state["full"] == true }

// refusedThroughPoros tells whether Poros's local proxy refuses what the preset sends to the server.
func refusedThroughPoros() bool {
	proxy := &url.URL{Scheme: "http", Host: "127.0.0.1:2090"}
	client := &http.Client{
		Timeout:       15 * time.Second,
		Transport:     &http.Transport{Proxy: http.ProxyURL(proxy), DisableKeepAlives: true},
		CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse },
	}
	response, err := client.Get("http://" + outside + "/")
	if err != nil {
		return true
	}
	response.Body.Close()
	return response.StatusCode == http.StatusBadGateway
}

// loseConnection makes the next start fail while the core lives, as a lost connection does: the server is gone.
func (s *shell) loseConnection(t *testing.T) {
	t.Helper()
	var settings map[string]any
	json.Unmarshal(s.call("settings.get", nil), &settings)
	settings["server"] = ""
	s.call("settings.set", settings)
	if state := s.start(); state["state"] != "idle" || state["error"] != "no_server" {
		t.Fatalf("the reconnect must fail: %v", state)
	}
}

// useDirectPreset picks the server and a preset that sends the probe page direct, so that it opens through
// Poros's proxy while the server is not there: the page proves that the core's own traffic gets out.
func (s *shell) useDirectPreset(providerURL, mode string) {
	s.t.Helper()
	s.useServer(providerURL)
	preset := map[string]any{"basedOn": "all", "name": "Direct probe", "rules": []map[string]any{{
		"conditions": []map[string]any{{"kind": "suffix", "values": []string{"msftconnecttest.com"}}}, "action": "direct",
	}}}
	var saved struct{ ID string }
	json.Unmarshal(s.call("presets.save", preset), &saved)
	var settings map[string]any
	json.Unmarshal(s.call("settings.get", nil), &settings)
	// Without the check of addresses: it goes through the server, which runs in this test and is blocked.
	settings["tun"], settings["preset"], settings["killSwitch"], settings["checkIp"] = false, saved.ID, mode, false
	s.call("settings.set", settings)
}

func (s *shell) exit(t *testing.T) {
	t.Helper()
	s.conn.Close()
	exited := make(chan error, 1)
	go func() { exited <- s.process.Wait() }()
	select {
	case <-exited:
	case <-time.After(25 * time.Second):
		s.process.Process.Kill()
		t.Fatal("the core did not exit after its channel closed")
	}
}

// TestKillSwitchWithoutTUN guards a connection without the TUN with no rights and no block of Windows: programs
// that do not use the proxy keep the internet throughout. After a lost connection, and after a crash of the core,
// the local proxy refuses what the preset sends to the server and still opens what it sends direct; Stop ends it,
// the always mode refuses from the start, and the window takes it away.
func TestKillSwitchWithoutTUN(t *testing.T) {
	program := os.Getenv("POROS_TEST_CORE")
	if os.Getenv("POROS_VM_TESTS") != "1" || program == "" {
		t.Skip("runs the kill switch of a Poros window; runs only in the virtual machine")
	}
	if !reachable() {
		t.Fatalf("%s is not reachable before the test", outside)
	}
	limited := limitedToken(t)
	window := startWindow(t, limited)
	dataDir := t.TempDir()
	t.Cleanup(func() {
		if !t.Failed() {
			return
		}
		for _, name := range []string{"core.1.log", "core.log"} {
			if text, err := os.ReadFile(filepath.Join(dataDir, "logs", name)); err == nil {
				t.Logf("%s:\n%s", name, text)
			}
		}
	})
	_, providerURL := startTestServer(t)

	first := startShellWith(t, shellOptions{program: program, dataDir: dataDir, hostPID: window.Process.Pid, token: limited})
	first.useDirectPreset(providerURL, "connected")
	first.connect()
	first.waitKillSwitch(t, blockOn, "after the Start")
	if fullBlock(first.killSwitch()) || !reachable() {
		t.Fatal("without the TUN the kill switch must not block Windows")
	}
	t.Log("connected without rights: the kill switch is on, other programs keep the internet")

	first.loseConnection(t)
	first.waitKillSwitch(t, blockOn, "after a lost connection")
	if err := fetchThroughPoros(); err != nil {
		t.Fatalf("the direct site of the preset must open through the refusal: %v", err)
	}
	if !refusedThroughPoros() {
		t.Fatal("the traffic of the server went out through the refusal")
	}
	if !reachable() {
		t.Fatal("a program without the proxy lost the internet")
	}
	t.Log("lost connection: the server's traffic is refused, the direct site opens, other programs are out")

	// A crash: the next core of the window brings the refusal back.
	killedPID := first.process.Process.Pid
	first.process.Process.Kill()
	first.process.Wait()
	first.conn.Close()
	second := startShellWith(t, shellOptions{program: program, dataDir: dataDir, hostPID: window.Process.Pid, waitPID: killedPID, token: limited})
	second.waitKillSwitch(t, blockOn, "the next core of the window")
	if err := fetchThroughPoros(); err != nil {
		t.Fatalf("the refusal of the next core: %v", err)
	}
	if !refusedThroughPoros() {
		t.Fatal("the traffic of the server went out after the crash")
	}
	t.Log("after a crash the next core refuses again")

	second.call("connection.stop", nil)
	second.waitKillSwitch(t, func(state map[string]any) bool { return !blockOn(state) }, "after Stop")
	if fetchThroughPoros() == nil {
		t.Fatal("the local proxy still answers after Stop")
	}

	// Always refuses from the start of a core without rights.
	var settings map[string]any
	json.Unmarshal(second.call("settings.get", nil), &settings)
	settings["killSwitch"] = "always"
	second.call("settings.set", settings)
	second.waitKillSwitch(t, blockOn, "always without rights")
	if state := second.killSwitch(); state["error"] != "" {
		t.Fatalf("always without the TUN needs no rights: %v", state)
	}
	secondPID := second.process.Process.Pid
	second.exit(t)
	third := startShellWith(t, shellOptions{program: program, dataDir: dataDir, hostPID: window.Process.Pid, waitPID: secondPID, token: limited})
	third.waitKillSwitch(t, blockOn, "always at the start of a core")
	if !refusedThroughPoros() || !reachable() {
		t.Fatal("always: the server's traffic must be refused and other programs out")
	}
	t.Log("always: refused from the start, without a connection")

	// The window goes: its core exits, and the refusal with it.
	window.Process.Kill()
	window.Wait()
	exited := make(chan error, 1)
	go func() { exited <- third.process.Wait() }()
	select {
	case <-exited:
	case <-time.After(40 * time.Second):
		third.process.Process.Kill()
		t.Fatal("the core did not exit after the window went")
	}
	if fetchThroughPoros() == nil {
		t.Fatal("the local proxy answers after the window went")
	}
	t.Log("the window went, and the refusal with it")
}

// TestKillSwitchWithTUN connects through the TUN with the kill switch: the block of Windows is on, but the TUN
// carries everything. After a lost connection the TUN stays with the refusal: what the preset sends direct opens,
// the rest does not get out. A crash takes the TUN away, and the block holds alone, the whole internet but Poros,
// for the next core, which comes without rights; its Stop gives the network back.
func TestKillSwitchWithTUN(t *testing.T) {
	program := os.Getenv("POROS_TEST_CORE")
	if os.Getenv("POROS_VM_TESTS") != "1" || program == "" {
		t.Skip("brings up a TUN and blocks the internet of the machine; runs only in the virtual machine")
	}
	limited := limitedToken(t)
	window := startWindow(t, limited)
	dataDir := t.TempDir()
	_, providerURL := startTestServer(t)
	s := startShellWith(t, shellOptions{program: program, dataDir: dataDir, hostPID: window.Process.Pid})
	s.useDirectPreset(providerURL, "connected")
	var settings map[string]any
	json.Unmarshal(s.call("settings.get", nil), &settings)
	settings["tun"] = true
	s.call("settings.set", settings)

	s.connect()
	s.waitKillSwitch(t, blockOn, "TUN with the kill switch")
	if fullBlock(s.killSwitch()) || !tunPresent() {
		t.Fatalf("the TUN must carry the traffic: %v, TUN %v", s.killSwitch(), tunPresent())
	}
	if err := fetch(30 * time.Second); err != nil {
		t.Fatalf("a page the preset sends direct must open through the TUN: %v", err)
	}
	t.Log("TUN: connected, the page opens through the TUN")

	s.loseConnection(t)
	s.waitKillSwitch(t, func(state map[string]any) bool { return blockOn(state) && !fullBlock(state) }, "the refusal on the TUN")
	if !tunPresent() {
		t.Fatal("the refusal must keep the TUN")
	}
	if err := fetch(30 * time.Second); err != nil {
		t.Fatalf("a page the preset sends direct must open through the refusal: %v", err)
	}
	if answers() {
		t.Fatalf("%s answered through the refusal", outside)
	}
	t.Log("lost connection: the TUN stays, the direct page opens, the server's traffic does not get out")

	// A crash takes the TUN; the block holds the whole internet but Poros for the next core without rights.
	killedPID := s.process.Process.Pid
	s.process.Process.Kill()
	s.process.Wait()
	s.conn.Close()
	time.Sleep(2 * time.Second)
	waitReachable(t, false, "after the core was killed")
	next := startShellWith(t, shellOptions{program: program, dataDir: dataDir, hostPID: window.Process.Pid, waitPID: killedPID, token: limited})
	next.waitKillSwitch(t, fullBlock, "the next core without rights and without the TUN")
	waitReachable(t, false, "the next core without rights")
	t.Log("after a crash the block holds the whole internet but Poros")

	next.call("connection.stop", nil)
	next.waitKillSwitch(t, func(state map[string]any) bool { return !blockOn(state) }, "after Stop")
	waitReachable(t, true, "after Stop from a core without rights")
	next.exit(t)
}
