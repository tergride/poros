package engine

import (
	"fmt"
	"net/http"
	"net/netip"
	"sync"
	"testing"
	"time"
)

func TestServerTestsAndCleanup(t *testing.T) {
	var mu sync.Mutex
	var tested []ServerLatency
	var speeds []ServerSpeed
	latencyDone, speedDone := make(chan struct{}, 4), make(chan struct{}, 1)
	e, states := openConnectionTestWith(t, func(method string, params any) {
		mu.Lock()
		defer mu.Unlock()
		switch method {
		case EventServersTested:
			report := params.(ServersTested)
			tested = append(tested, report.Results...)
			if report.Done {
				latencyDone <- struct{}{}
			}
		case EventServersSpeedTested:
			report := params.(ServersSpeedTested)
			speeds = append(speeds, report.Results...)
			if report.Done {
				speedDone <- struct{}{}
			}
		}
	})
	wait := func(done chan struct{}, what string) {
		t.Helper()
		select {
		case <-done:
		case <-time.After(30 * time.Second):
			t.Fatalf("the %s did not finish", what)
		}
	}
	page := webPage(t)
	e.latencyURL = page
	link, _ := startTrojanServer(t)
	dead := fmt.Sprintf("trojan://secret@127.0.0.1:%d?security=none#dead", freePort(t))
	p := newProvider(t)
	p.serve(http.StatusOK, link, dead)
	update, err := e.AddSubscription(t.Context(), AddSubscription{URL: p.url})
	if err != nil {
		t.Fatal(err)
	}
	servers, _ := e.Servers(ServersQuery{GroupID: update.Group.ID})
	alive, gone := servers[0].ID, servers[1].ID

	// A test of the servers picked alone.
	if _, err = e.TestServers(ServersQuery{IDs: []string{alive}}); err != nil {
		t.Fatal(err)
	}
	wait(latencyDone, "delay test of one server")
	mu.Lock()
	if len(tested) != 1 || tested[0].ServerID != alive || tested[0].Latency <= 0 {
		t.Errorf("a test of one server: %+v", tested)
	}
	mu.Unlock()
	if _, err = e.TestServers(ServersQuery{}); err != nil {
		t.Fatal(err)
	}
	wait(latencyDone, "delay test")

	settings, _ := e.Settings()
	settings.SpeedTestMode, settings.SpeedTestURL = SpeedTestSimple, page
	if _, err = e.SetSettings(settings); err != nil {
		t.Fatal(err)
	}
	if _, err = e.SpeedTestServers(ServersQuery{IDs: []string{alive}}); err != nil {
		t.Fatal(err)
	}
	wait(speedDone, "speed test")
	if got, _ := e.Server(ByID{ID: alive}); got.Server.SpeedDown == "" || len(speeds) != 1 || speeds[0].Error != "" {
		t.Errorf("a simple speed test: %+v, reported %+v", got.Server, speeds)
	}

	removed, err := e.CleanServers(CleanServers{GroupID: update.Group.ID, Unavailable: true})
	if err != nil || len(removed.IDs) != 1 || removed.IDs[0] != gone {
		t.Fatalf("removing the server that did not answer: %+v, %v", removed, err)
	}

	settings, _ = e.Settings()
	settings.Server, settings.Preset = alive, "through"
	if _, err = e.SetSettings(settings); err != nil {
		t.Fatal(err)
	}
	connectAndFetch(t, e, states, page)
	e.StopConnection()
	waitState(t, states)
	deadline := time.Now().Add(5 * time.Second)
	for {
		if got, _ := e.Server(ByID{ID: alive}); got.Server.Download > 0 {
			break
		}
		if time.Now().After(deadline) {
			t.Fatal("the traffic of the connection was not counted to its server")
		}
		time.Sleep(100 * time.Millisecond)
	}
	if err = e.ResetTraffic(ServersQuery{IDs: []string{alive}}); err != nil {
		t.Fatal(err)
	}
	if err = e.ClearResults(ServersQuery{GroupID: update.Group.ID}); err != nil {
		t.Fatal(err)
	}
	if got, _ := e.Server(ByID{ID: alive}); got.Server.Download != 0 || got.Server.Latency != 0 || got.Server.SpeedDown != "" {
		t.Errorf("after the reset and the clearing: %+v", got.Server)
	}
}

func TestResolveServers(t *testing.T) {
	e, _ := openTest(t, t.TempDir())
	added, err := e.AddServers(AddServers{NewGroup: "Mine", Text: "trojan://pw@localhost:443?security=tls#Local\n" +
		"trojan://pw@192.0.2.1:443?security=tls#Address"})
	if err != nil {
		t.Fatal(err)
	}
	resolved, err := e.ResolveServers(ServersQuery{GroupID: added.Group.ID})
	if err != nil || resolved.Resolved != 1 {
		t.Fatalf("ResolveServers: %+v, %v", resolved, err)
	}
	local, _ := e.Server(ByID{ID: added.IDs[0]})
	if addr, err := netip.ParseAddr(local.Profile.Server); err != nil || !addr.IsLoopback() || local.Profile.Trojan.TLS.ServerName != "localhost" {
		t.Errorf("localhost resolved: %s, server name %q", local.Profile.Server, local.Profile.Trojan.TLS.ServerName)
	}
}
