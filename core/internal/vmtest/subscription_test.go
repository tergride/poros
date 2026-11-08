//go:build windows

package vmtest

import (
	"encoding/json"
	"os"
	"os/exec"
	"testing"
	"time"
)

// TestTestSubscriptionThroughTUN connects to every server of the test subscription on real servers with the TUN
// up. VLESS with REALITY runs on Xray with its sockets bound to the network adapter, the rest on sing-box. The page
// opens without a proxy, so through the TUN, and the download counter of the connection shows it went through the
// server. The subscription link gives access to the servers: neither it nor the servers' addresses are printed.
func TestTestSubscriptionThroughTUN(t *testing.T) {
	program, link := os.Getenv("POROS_TEST_CORE"), os.Getenv("POROS_TEST_SUBSCRIPTION")
	if os.Getenv("POROS_VM_TESTS") != "1" || program == "" || link == "" {
		t.Skip("brings up a TUN to the servers of the test subscription; runs only in the virtual machine")
	}
	s := startShell(t, program, t.TempDir(), 0)
	defer s.conn.Close()

	for i, server := range s.addTestSubscription(link) {
		var settings map[string]any
		json.Unmarshal(s.call("settings.get", nil), &settings)
		settings["server"], settings["tun"], settings["preset"] = server.ID, true, "all"
		s.call("settings.set", settings)

		// Each connection counts from zero.
		s.downloaded.Store(0)
		started := time.Now()
		if state := s.start(); state["state"] != "running" {
			t.Fatalf("server %d (%s): the connection did not come up: %v", i+1, server.Protocol, state["error"])
		}
		took := time.Since(started)
		before := s.downloaded.Load()
		if err := fetch(30 * time.Second); err != nil {
			t.Fatalf("server %d (%s): the page does not open through the TUN", i+1, server.Protocol)
		}
		// Traffic comes once a second.
		deadline := time.Now().Add(5 * time.Second)
		for s.downloaded.Load() <= before && time.Now().Before(deadline) {
			time.Sleep(250 * time.Millisecond)
		}
		if s.downloaded.Load() <= before {
			t.Fatalf("server %d (%s): the page opened, but nothing came through the server", i+1, server.Protocol)
		}
		t.Logf("server %d (%s): connected in %v, the page opened through the TUN and the server", i+1, server.Protocol, took)

		s.call("connection.stop", nil)
		s.waitIdle()
	}
}

// TestKillSwitchWithTheTestSubscription connects to every server of the test subscription with the kill switch on,
// through the TUN and through the proxy alone. With the TUN the block of Windows keeps the DNS client of Windows in,
// so the core must find the servers' addresses by itself; the cache of Windows is emptied before each start, so that
// no answer from before the block helps. Neither the link nor the servers' addresses are printed.
func TestKillSwitchWithTheTestSubscription(t *testing.T) {
	program, link := os.Getenv("POROS_TEST_CORE"), os.Getenv("POROS_TEST_SUBSCRIPTION")
	if os.Getenv("POROS_VM_TESTS") != "1" || program == "" || link == "" {
		t.Skip("blocks the internet and connects to the test subscription; runs only in the virtual machine")
	}
	window := startWindow(t, limitedToken(t))
	s := startShellWith(t, shellOptions{program: program, dataDir: t.TempDir(), hostPID: window.Process.Pid})

	for i, server := range s.addTestSubscription(link) {
		for _, tun := range []bool{true, false} {
			var settings map[string]any
			json.Unmarshal(s.call("settings.get", nil), &settings)
			settings["server"], settings["tun"], settings["preset"] = server.ID, tun, "all"
			settings["killSwitch"], settings["checkIp"] = "connected", false
			s.call("settings.set", settings)
			if out, err := exec.Command("ipconfig", "/flushdns").CombinedOutput(); err != nil {
				t.Fatalf("flush the DNS cache: %v %s", err, out)
			}

			started := time.Now()
			// The reason only: the words of an error may name the server.
			if state := s.start(); state["state"] != "running" {
				t.Fatalf("server %d (%s), TUN %t: with the kill switch the connection did not come up: %v", i+1, server.Protocol, tun, state["error"])
			}
			took := time.Since(started)
			s.waitKillSwitch(t, blockOn, "the kill switch with the test subscription")
			var err error
			if tun {
				err = fetch(30 * time.Second)
			} else {
				err = fetchThroughPoros()
			}
			if err != nil {
				t.Fatalf("server %d (%s), TUN %t: the page does not open through the connection", i+1, server.Protocol, tun)
			}
			t.Logf("server %d (%s), TUN %t: connected behind the kill switch in %v, the page opened", i+1, server.Protocol, tun, took)

			s.call("connection.stop", nil)
			s.waitIdle()
			s.waitKillSwitch(t, func(state map[string]any) bool { return !blockOn(state) }, "after Stop")
		}
	}
	s.exit(t)
}

// addTestSubscription adds the test subscription and gives its servers.
func (s *shell) addTestSubscription(link string) []testServer {
	s.t.Helper()
	result, failure := s.request("subscriptions.add", map[string]string{"url": link, "name": ""})
	if failure != "" {
		s.t.Fatal("the test subscription did not load")
	}
	var update struct {
		Subscription struct{ ID string } `json:"subscription"`
	}
	json.Unmarshal(result, &update)
	var servers []testServer
	json.Unmarshal(s.call("servers.list", map[string]string{"subscriptionId": update.Subscription.ID}), &servers)
	if len(servers) == 0 {
		s.t.Fatal("the test subscription has no servers")
	}
	return servers
}

type testServer struct{ ID, Protocol string }

// waitIdle waits until a stopped connection is down, so that the next start does not see its last state.
func (s *shell) waitIdle() {
	s.t.Helper()
	deadline := time.After(30 * time.Second)
	for {
		select {
		case state := <-s.states:
			if state["state"] == "idle" {
				return
			}
		case <-deadline:
			s.t.Fatal("the connection did not stop in 30 seconds")
		}
	}
}
