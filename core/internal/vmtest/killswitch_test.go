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

// useDirectPreset picks the server and a preset that sends the probe page direct, so that it opens through
// Poros's proxy while the server is not there: the page proves that the core's own traffic gets out.
func (s *shell) useDirectPreset(providerURL, mode string) {
	s.t.Helper()
	s.useServer(providerURL)
	var preset map[string]any
	json.Unmarshal(s.call("presets.get", map[string]string{"id": "all"}), &preset)
	preset["name"] = "Direct probe"
	preset["sites"] = []map[string]string{{"value": "msftconnecttest.com", "action": "direct"}}
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

// TestKillSwitchOutlivesTheCore blocks the internet for every program but Poros from a Start, keeps the block
// through a crash of the core, lets a core without rights turn it off with Stop, turns it on at the start of a core
// in the always mode, and takes it away with the Poros window.
func TestKillSwitchOutlivesTheCore(t *testing.T) {
	program := os.Getenv("POROS_TEST_CORE")
	if os.Getenv("POROS_VM_TESTS") != "1" || program == "" {
		t.Skip("blocks the internet of the machine; runs only in the virtual machine")
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
		for _, name := range []string{"guard.1.log", "guard.log", "core.1.log", "core.log"} {
			if text, err := os.ReadFile(filepath.Join(dataDir, "logs", name)); err == nil {
				t.Logf("%s:\n%s", name, text)
			}
		}
	})
	_, providerURL := startTestServer(t)

	first := startShellWith(t, shellOptions{program: program, dataDir: dataDir, hostPID: window.Process.Pid})
	first.useDirectPreset(providerURL, "connected")
	if blockOn(first.killSwitch()) {
		t.Fatal("the connected mode blocks before a Start")
	}
	first.connect()
	first.waitKillSwitch(t, blockOn, "after the Start")
	waitReachable(t, false, "connected")
	if err := fetchThroughPoros(); err != nil {
		t.Fatalf("Poros's own traffic must pass the block: %v", err)
	}
	t.Log("connected: other programs are blocked, Poros gets out")

	// A crash: the core goes, its guard stays.
	killedPID := first.process.Process.Pid
	first.process.Process.Kill()
	first.process.Wait()
	first.conn.Close()
	time.Sleep(2 * time.Second)
	waitReachable(t, false, "after the core was killed")
	t.Log("the block outlived the killed core")

	// The next core comes up without rights, as after a crash, finds the block and turns it off with Stop.
	second := startShellWith(t, shellOptions{program: program, dataDir: dataDir, hostPID: window.Process.Pid, waitPID: killedPID, token: limited})
	var info struct{ Elevated bool }
	json.Unmarshal(second.call("app.info", nil), &info)
	if info.Elevated {
		t.Fatal("the second core must run without rights")
	}
	second.waitKillSwitch(t, blockOn, "the core without rights joins the block")
	second.call("connection.stop", nil)
	second.waitKillSwitch(t, func(state map[string]any) bool { return !blockOn(state) }, "after Stop")
	waitReachable(t, true, "after Stop from a core without rights")
	t.Log("a core without rights turned the block off")

	// Always: a core without rights cannot turn the block on and says so; one with rights does at its start.
	var settings map[string]any
	json.Unmarshal(second.call("settings.get", nil), &settings)
	settings["killSwitch"] = "always"
	second.call("settings.set", settings)
	second.waitKillSwitch(t, func(state map[string]any) bool { return state["error"] == "kill_switch_needs_elevation" }, "always without rights")
	secondPID := second.process.Process.Pid
	second.exit(t)
	third := startShellWith(t, shellOptions{program: program, dataDir: dataDir, hostPID: window.Process.Pid, waitPID: secondPID})
	third.waitKillSwitch(t, blockOn, "always at the start of a core with rights")
	waitReachable(t, false, "always, without a connection")
	t.Log("always: the block is on at the start, without a connection")

	// The window goes: its core exits, and the guard takes the block away.
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
	waitReachable(t, true, "after the window went")
	out, _ := exec.Command("tasklist", "/fi", "imagename eq PorosCore.exe", "/fo", "csv", "/nh").Output()
	if len(out) > 0 && out[0] == '"' {
		t.Errorf("a PorosCore process stayed after the window went: %s", out)
	}
	t.Log("the window went, and the block with it")
}

// TestKillSwitchWithTUN connects through the TUN with the block on: what the preset sends direct opens through the
// TUN, what it sends to the server does not get past the block, and Stop gives the network back.
func TestKillSwitchWithTUN(t *testing.T) {
	program := os.Getenv("POROS_TEST_CORE")
	if os.Getenv("POROS_VM_TESTS") != "1" || program == "" {
		t.Skip("brings up a TUN and blocks the internet of the machine; runs only in the virtual machine")
	}
	window := startWindow(t, limitedToken(t))
	_, providerURL := startTestServer(t)
	s := startShellWith(t, shellOptions{program: program, dataDir: t.TempDir(), hostPID: window.Process.Pid})
	s.useDirectPreset(providerURL, "connected")
	var settings map[string]any
	json.Unmarshal(s.call("settings.get", nil), &settings)
	settings["tun"] = true
	s.call("settings.set", settings)

	s.connect()
	s.waitKillSwitch(t, blockOn, "TUN with the kill switch")
	if !tunPresent() {
		t.Fatal("the TUN adapter is missing while the connection runs")
	}
	if err := fetch(30 * time.Second); err != nil {
		t.Fatalf("a page the preset sends direct must open through the TUN: %v", err)
	}
	// The server runs in this test, which the block stops: the traffic for it must not find another way out.
	if answers() {
		t.Fatalf("%s answered past the block", outside)
	}
	t.Log("TUN: the page opens through the TUN, nothing passes the block")

	s.call("connection.stop", nil)
	s.waitKillSwitch(t, func(state map[string]any) bool { return !blockOn(state) }, "after Stop")
	waitReachable(t, true, "after Stop with TUN")
	s.exit(t)
}
