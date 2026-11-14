package engine

import (
	"context"
	"fmt"
	"net/http"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"PorosCore/config"
)

// Tolerances of the leak tests, after the settle period: a few goroutines of the runtime and the network come and
// go, and every thread the runtime makes keeps handles of its own for good.
const (
	leakGoroutines      = 5
	leakHandles         = 40
	leakHandlesByThread = 6
	leakSettle          = 15 * time.Second
)

// checkLeaks compares two snapshots and fails the test when more stays than the tolerances allow.
func checkLeaks(t *testing.T, what string, before, after snapshot) {
	t.Helper()
	handles := leakHandles + leakHandlesByThread*max(0, after.threads-before.threads)
	t.Logf("%s: goroutines %d -> %d, handles %d -> %d, threads %d -> %d", what, before.goroutines, after.goroutines,
		before.handles, after.handles, before.threads, after.threads)
	if after.goroutines > before.goroutines+leakGoroutines {
		t.Errorf("%s left %d goroutines:\n%s", what, after.goroutines-before.goroutines, growth(before, after))
	}
	if before.handles >= 0 && after.handles > before.handles+handles {
		t.Errorf("%s left %d handles, more than %d", what, after.handles-before.handles, handles)
	}
}

// checkLeaksAfter compares the goroutines before the test with those after it and all its cleanups: engines
// closed, servers down. Called first in the test, its cleanup runs last. The handles are only told: the first test
// of a run also loads what the process keeps for good.
func checkLeaksAfter(t *testing.T) {
	t.Helper()
	before := takeSnapshot()
	t.Cleanup(func() {
		after := settle(before, leakGoroutines, 1<<20, leakSettle)
		t.Logf("%s: goroutines %d -> %d, handles %d -> %d, threads %d -> %d", t.Name(), before.goroutines, after.goroutines,
			before.handles, after.handles, before.threads, after.threads)
		if after.goroutines > before.goroutines+leakGoroutines {
			t.Errorf("%s left %d goroutines:\n%s", t.Name(), after.goroutines-before.goroutines, growth(before, after))
		}
	})
}

// A delay, a speed and an IP test run at once, each with Xray on bridges for its servers. A bridge port that the
// connection of another test took in the meantime fails the start of Xray; the inbounds Xray had started before it
// go too, rather than listen until the core exits.
func TestRobustTestBoxesLetGo(t *testing.T) {
	robustOnHost(t)
	rounds := 10
	if robustStress() {
		rounds = 40
	}
	dead := closerServer(t)
	e, _ := openRobust(t, t.TempDir(), webPage(t))
	links := make([]string, 250)
	for i := range links {
		links[i] = xhttpTo(dead, fmt.Sprintf("X%d", i))
	}
	if _, err := e.AddServers(AddServers{NewGroup: "Xray", Text: strings.Join(links, "\n")}); err != nil {
		t.Fatal(err)
	}
	servers, err := e.testServersOf(ServersQuery{})
	if err != nil {
		t.Fatal(err)
	}
	settings, _ := e.Settings()
	listeners := func() int { return strings.Count(goroutineDump(), "tcp.(*Listener).keepAccepting") }
	before := listeners()
	var failed atomic.Int32
	var wg sync.WaitGroup
	for range 3 {
		wg.Go(func() {
			for range rounds {
				box, err := openTestBox(e.testInput(context.Background(), servers, settings))
				if err != nil {
					failed.Add(1)
				}
				box.close()
			}
		})
	}
	wg.Wait()
	// The bridges take ports in turn below the ranges of outgoing connections: none may be taken from under Xray.
	if n := failed.Load(); n > 0 {
		t.Errorf("%d of %d starts of Xray failed on a taken port", n, 3*rounds)
	}
	if left := listeners() - before; left > 0 {
		t.Errorf("%d inbounds of Xray still listen after the tests closed", left)
	}
}

// Starts and stops through a Trojan server on loopback leave no goroutines and no handles behind.
func TestRobustLeaksOfStartStop(t *testing.T) {
	cycles := 200
	if !robustStress() {
		t.Skip("200 starts and stops: POROS_STRESS=1")
	}
	robustOnHost(t)
	page := webPage(t)
	live, _ := startTrojanServer(t)
	provider := newProvider(t)
	provider.serve(http.StatusOK, live)
	states := make(chan ConnectionState, 64)
	e, err := Open(Config{DataDir: t.TempDir(), MixedPort: freePort(t), ForeignUser: true, Notify: func(method string, params any) {
		if method == EventConnectionChanged {
			states <- params.(ConnectionState)
		}
	}})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { closeWithin(t, e, 30*time.Second) })
	e.latencyURL = page
	e.client = loopbackClient(hostOf(t, provider.url))
	// Everything through the server, loopback included: the built-in presets send private addresses direct.
	e.presets = append(e.presets, presetRecord{ID: "through", Preset: config.Preset{Name: "Through", DefaultOutbound: config.OutboundProxy}})
	update, err := e.AddSubscription(t.Context(), AddSubscription{URL: provider.url})
	if err != nil {
		t.Fatal(err)
	}
	servers, err := e.Servers(ServersQuery{GroupID: update.Group.ID})
	if err != nil || len(servers) != 1 {
		t.Fatalf("servers: %v, %v", servers, err)
	}
	settings, _ := e.Settings()
	settings.Server, settings.Preset, settings.CheckIP = servers[0].ID, "through", false
	if _, err = e.SetSettings(settings); err != nil {
		t.Fatal(err)
	}

	cycle := func(i int) {
		t.Helper()
		e.StartConnection()
		if state := waitState(t, states); state.State != ConnectionRunning || state.Error != "" {
			t.Fatalf("start %d: %+v", i, state)
		}
		if body, err := fetchThrough(e, page); err != nil || body != "through the server" {
			t.Fatalf("start %d: fetch through Poros: %q, %v", i, body, err)
		}
		e.StopConnection()
		if state := waitState(t, states); state.State != ConnectionIdle || state.Error != "" {
			t.Fatalf("stop %d: %+v", i, state)
		}
	}
	// The first starts make what lives as long as the process: the runtime's threads, caches of the cores.
	for i := range 5 {
		cycle(i)
	}
	before := takeSnapshot()
	began := time.Now()
	for i := range cycles {
		cycle(i)
	}
	took := time.Since(began)
	after := settle(before, leakGoroutines, leakHandles, leakSettle)
	checkLeaks(t, fmt.Sprintf("%d starts and stops in %s", cycles, took.Round(time.Millisecond)), before, after)
}

// Opening and closing the engine on the same data folder leaves no goroutines and no handles behind.
func TestRobustLeaksOfOpenClose(t *testing.T) {
	opens := 50
	if !robustStress() {
		t.Skip("50 opens of the engine: POROS_STRESS=1")
	}
	robustOnHost(t)
	dir := t.TempDir()
	cycle := func(i int) {
		t.Helper()
		e, err := Open(Config{DataDir: dir, MixedPort: freePort(t), ForeignUser: true})
		if err != nil {
			t.Fatalf("open %d: %v", i, err)
		}
		settings, err := e.Settings()
		if err == nil {
			settings.LogLines = 100 + i
			_, err = e.SetSettings(settings)
		}
		if err == nil {
			_, err = e.AddGroup(AddGroup{Name: fmt.Sprintf("Group %d", i)})
		}
		if err == nil {
			_, err = e.TrafficHistory(TrafficQuery{Hours: 24})
		}
		if err == nil {
			_, err = e.StatsNow()
		}
		if err != nil {
			t.Errorf("open %d: %v", i, err)
		}
		closeWithin(t, e, 30*time.Second)
	}
	for i := range 3 {
		cycle(i)
	}
	before := takeSnapshot()
	began := time.Now()
	for i := range opens {
		cycle(i)
	}
	took := time.Since(began)
	after := settle(before, leakGoroutines, leakHandles, leakSettle)
	checkLeaks(t, fmt.Sprintf("%d opens and closes in %s", opens, took.Round(time.Millisecond)), before, after)
}
