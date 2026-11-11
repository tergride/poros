package engine

import (
	"context"
	"net"
	"net/netip"
	"sync"
	"time"

	"github.com/sagernet/bbolt"

	"PorosCore/config"
	"PorosCore/internal/boxbox"
	"PorosCore/internal/boxmain"
	"PorosCore/internal/netmon"
	"PorosCore/internal/probe"
	"PorosCore/internal/xray"
	"PorosCore/profile"
)

const (
	// latencyURL answers quickly from everywhere; a server's delay is how long a request through it takes.
	latencyURL                = "http://cp.cloudflare.com/"
	defaultLatencyTimeout     = 3 * time.Second
	defaultLatencyConcurrency = 10
	// latencyBudget bounds a whole test of many servers.
	latencyBudget = 3 * time.Minute
	// pingFirst lets a new connection settle before its delay is measured, and pingInterval spaces the measures.
	pingFirst    = 2 * time.Second
	pingInterval = 30 * time.Second
)

type latencyTest struct {
	mu      sync.Mutex
	results map[string]int
}

// Kinds of tests: a newer test of a kind replaces one that runs.
const (
	testLatency = iota
	testSpeed
	testIP
	testKinds
)

// testRuns holds the tests that run, one of each kind.
type testRuns struct {
	mu      sync.Mutex
	cancel  [testKinds]context.CancelFunc
	running sync.WaitGroup
}

// start cancels the test of the kind that runs and gives the context of the new one, which done ends.
func (t *testRuns) start(kind int, budget time.Duration) (context.Context, func()) {
	t.mu.Lock()
	defer t.mu.Unlock()
	if t.cancel[kind] != nil {
		t.cancel[kind]()
	}
	ctx, cancel := context.WithTimeout(context.Background(), budget)
	t.cancel[kind] = cancel
	t.running.Add(1)
	return ctx, func() {
		cancel()
		t.running.Done()
	}
}

// stopAll cancels every test and waits for them to end.
func (t *testRuns) stopAll() {
	t.mu.Lock()
	for _, cancel := range t.cancel {
		if cancel != nil {
			cancel()
		}
	}
	t.mu.Unlock()
	t.running.Wait()
}

func (e *Engine) latencyOf(id string) int {
	e.latency.mu.Lock()
	defer e.latency.mu.Unlock()
	return e.latency.results[id]
}

// TestServers measures the servers asked for in the background; a newer test replaces one that runs. Each result
// comes with servers.tested.
func (e *Engine) TestServers(query ServersQuery) (any, error) {
	return e.testServers(query, nil)
}

// testServers starts a test; onDone, unless nil, gets every result of a test that ran to its end.
func (e *Engine) testServers(query ServersQuery, onDone func(results map[string]int)) (any, error) {
	servers, err := e.testServersOf(query)
	if err != nil {
		return nil, err
	}
	settings, err := e.Settings()
	if err != nil {
		return nil, err
	}
	ctx, done := e.tests.start(testLatency, latencyBudget)
	go func() {
		defer done()
		in := e.testInput(ctx, servers, settings)
		results := e.runLatencyTest(ctx, in)
		if onDone != nil && ctx.Err() == nil {
			onDone(results)
		}
	}()
	return nil, nil
}

// testServersOf picks the servers of a query with what they connect with, as resolveServer gives it.
func (e *Engine) testServersOf(query ServersQuery) ([]config.TestServer, error) {
	var servers []config.TestServer
	err := e.db.View(func(tx *bbolt.Tx) error {
		store, err := loadServers(tx)
		if err != nil {
			return err
		}
		entries, err := loadOTP(tx)
		if err != nil {
			return err
		}
		codes := &otpCodes{entries: entries, now: e.now(), test: true}
		for _, id := range queryIDs(store, query) {
			p, links, _, err := resolveServer(store, id)
			if err == nil {
				p, links, err = codes.fillServer(p, links)
			}
			if err != nil {
				// A server of a chain, or a proxy of the group, is gone, or a code of OTP: an empty profile fails
				// the test, as the start would fail.
				p, links = profile.Profile{}, nil
			}
			servers = append(servers, config.TestServer{Tag: id, Server: p, Links: links})
		}
		return nil
	})
	return servers, err
}

// queryIDs lists the servers a query names: IDs that exist, else a group's, else all, in the order of the groups.
func queryIDs(store *serverStore, query ServersQuery) []string {
	var ids []string
	if len(query.IDs) > 0 {
		for _, id := range query.IDs {
			if g, _ := store.locate(id); g >= 0 {
				ids = append(ids, id)
			}
		}
		return ids
	}
	for _, group := range store.groups {
		if query.GroupID != "" && group.ID != query.GroupID {
			continue
		}
		for _, r := range store.servers[group.ID] {
			ids = append(ids, r.ID)
		}
	}
	return ids
}

// testInput gives the servers of a test the bridges to Xray they need and the addresses of their names.
func (e *Engine) testInput(ctx context.Context, servers []config.TestServer, settings Settings) config.TestInput {
	in := config.TestInput{RealityOnXray: settings.RealityOnXray, Tuning: tuningOf(settings)}
	if iface := netmon.DefaultInterface(); iface != nil {
		in.Interface = iface.Name
	}
	var hosts []string
	for _, server := range servers {
		entry := config.Entry(server.Server, server.Links)
		if config.RunsOnXray(entry, settings.RealityOnXray) {
			bridge, err := newBridge()
			if err != nil {
				continue
			}
			server.Bridge = bridge
			hosts = append(hosts, config.XrayHosts(entry, settings.RealityOnXray)...)
		} else if entry.Server != "" {
			// sing-box gets the addresses too: its own lookup would go to the system's DNS, which a TUN of
			// another client may hold or answer with fake addresses. The links after the first are dialled
			// through it, which resolves their names.
			hosts = append(hosts, entry.Server)
		}
		in.Servers = append(in.Servers, server)
	}
	in.Resolved = resolveEach(ctx, e.systemResolver(), hosts, !settings.IPv6)
	return in
}

// resolveEach looks up what it can; a server whose name does not resolve fails its own test.
func resolveEach(ctx context.Context, resolver *net.Resolver, hosts []string, ipv4Only bool) map[string]netip.Addr {
	resolved := map[string]netip.Addr{}
	var mu sync.Mutex
	var wg sync.WaitGroup
	slots := make(chan struct{}, defaultLatencyConcurrency)
	seen := map[string]bool{}
	for _, host := range hosts {
		if seen[host] {
			continue
		}
		seen[host] = true
		wg.Add(1)
		go func() {
			defer wg.Done()
			slots <- struct{}{}
			defer func() { <-slots }()
			lookupCtx, cancel := context.WithTimeout(ctx, defaultLatencyTimeout+dohTimeout)
			defer cancel()
			if addr, err := lookupPast(lookupCtx, resolver, host, ipv4Only); err == nil {
				mu.Lock()
				resolved[host] = addr
				mu.Unlock()
			}
		}()
	}
	wg.Wait()
	return resolved
}

// testBox runs the servers of a test: sing-box and, for the servers on it, Xray. tags are the servers it runs,
// failed those that could not be built.
type testBox struct {
	box    *boxbox.Box
	tags   []string
	failed []string
	close  func()
}

// openTestBox starts the cores of a test. With an error, every server failed; failed alone names the servers that
// did, the rest run.
func openTestBox(in config.TestInput) (*testBox, error) {
	t := &testBox{close: func() {}}
	configs, err := config.BuildTest(in)
	if err != nil {
		return t, err
	}
	for _, server := range in.Servers {
		if configs.Failed[server.Tag] != nil {
			t.failed = append(t.failed, server.Tag)
		} else {
			t.tags = append(t.tags, server.Tag)
		}
	}
	if len(t.tags) == 0 {
		return t, nil
	}
	probe.LimitWireGuardWorkers(&configs.SingBox)
	singBox, err := configs.SingBoxJSON()
	if err != nil {
		return t, err
	}
	var instance interface{ Close() error }
	if configs.Xray != nil {
		created, err := xray.CreateXrayInstance(string(configs.Xray))
		if err == nil {
			err = created.Start()
		}
		if err != nil {
			return t, err
		}
		instance = created
	}
	box, cancelBox, err := boxmain.Create(singBox, nil)
	if err != nil {
		if instance != nil {
			instance.Close()
		}
		return t, err
	}
	t.box = box
	t.close = func() {
		cancelBox()
		box.Close()
		if instance != nil {
			instance.Close()
		}
	}
	return t, nil
}

func (e *Engine) runLatencyTest(ctx context.Context, in config.TestInput) map[string]int {
	all := map[string]int{}
	report := func(results []ServerLatency, done bool) {
		e.latency.mu.Lock()
		for _, result := range results {
			e.latency.results[result.ServerID] = result.Latency
			all[result.ServerID] = result.Latency
		}
		e.latency.mu.Unlock()
		e.notify(EventServersTested, ServersTested{Results: results, Done: done})
	}
	t, err := openTestBox(in)
	defer t.close()
	if err != nil {
		report(failedAll(in.Servers), true)
		return all
	}
	if len(t.failed) > 0 {
		report(failedTags(t.failed), false)
	}
	if t.box == nil {
		report([]ServerLatency{}, true)
		return all
	}
	url, timeout, concurrency := e.latencyTest()
	probe.BatchURLTestTo(ctx, t.box, t.tags, url, concurrency, false, timeout, func(result *probe.URLTestResult) {
		latency := -1
		if result.Error == nil {
			latency = max(1, int(result.Duration.Milliseconds()))
		}
		report([]ServerLatency{{ServerID: result.Tag, Latency: latency}}, false)
	})
	report([]ServerLatency{}, true)
	return all
}

// pingConnection keeps the delay of the server in use current: now and then it measures through the running
// connection, the way a delay test measures through each server, until ctx ends with the connection.
func (e *Engine) pingConnection(ctx context.Context, box *boxbox.Box, serverID string) {
	timer := time.NewTimer(pingFirst)
	defer timer.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-timer.C:
		}
		latency := -1
		url, timeout, _ := e.latencyTest()
		probe.BatchURLTestTo(ctx, box, []string{config.TagProxy}, url, 1, false, timeout, func(result *probe.URLTestResult) {
			if result.Error == nil {
				latency = max(1, int(result.Duration.Milliseconds()))
			}
		})
		if ctx.Err() != nil {
			return
		}
		e.latency.mu.Lock()
		e.latency.results[serverID] = latency
		e.latency.mu.Unlock()
		e.notify(EventServersTested, ServersTested{Results: []ServerLatency{{ServerID: serverID, Latency: latency}}})
		timer.Reset(pingInterval)
	}
}

func failedTags(tags []string) []ServerLatency {
	results := make([]ServerLatency, len(tags))
	for i, tag := range tags {
		results[i] = ServerLatency{ServerID: tag, Latency: -1}
	}
	return results
}

func failedAll(servers []config.TestServer) []ServerLatency {
	results := []ServerLatency{}
	for _, server := range servers {
		results = append(results, ServerLatency{ServerID: server.Tag, Latency: -1})
	}
	return results
}

func (e *Engine) closeLatencyTests() {
	e.tests.stopAll()
}
