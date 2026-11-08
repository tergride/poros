package engine

import (
	"context"
	"net"
	"net/netip"
	"sync"
	"time"

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
	latencyURL         = "http://cp.cloudflare.com/"
	latencyTimeout     = 3 * time.Second
	latencyConcurrency = 10
	// latencyBudget bounds a whole test of many servers.
	latencyBudget = 3 * time.Minute
	// pingFirst lets a new connection settle before its delay is measured, and pingInterval spaces the measures.
	pingFirst    = 2 * time.Second
	pingInterval = 30 * time.Second
)

type latencyTest struct {
	mu      sync.Mutex
	results map[string]int
	cancel  context.CancelFunc
	running sync.WaitGroup
}

func (e *Engine) latencyOf(id string) int {
	e.latency.mu.Lock()
	defer e.latency.mu.Unlock()
	return e.latency.results[id]
}

// TestServers measures the servers of a group, or of all, in the background; a newer test replaces one that runs.
// Each result comes with servers.tested.
func (e *Engine) TestServers(query ServersQuery) (any, error) {
	type target struct {
		id     string
		server profile.Profile
	}
	var targets []target
	groups, err := e.groups()
	if err != nil {
		return nil, err
	}
	for _, group := range groups {
		if query.GroupID != "" && group.ID != query.GroupID {
			continue
		}
		list, err := e.serverRecords(group.ID)
		if err != nil {
			return nil, err
		}
		for _, r := range list {
			targets = append(targets, target{r.ID, r.Profile})
		}
	}
	settings, err := e.Settings()
	if err != nil {
		return nil, err
	}

	e.latency.mu.Lock()
	if e.latency.cancel != nil {
		e.latency.cancel()
	}
	ctx, cancel := context.WithTimeout(context.Background(), latencyBudget)
	e.latency.cancel = cancel
	e.latency.mu.Unlock()

	e.latency.running.Add(1)
	go func() {
		defer e.latency.running.Done()
		defer cancel()
		in := config.TestInput{RealityOnXray: settings.RealityOnXray}
		if iface := netmon.DefaultInterface(); iface != nil {
			in.Interface = iface.Name
		}
		var hosts []string
		for _, t := range targets {
			server := config.TestServer{Tag: t.id, Server: t.server}
			if config.RunsOnXray(t.server, settings.RealityOnXray) {
				bridge, err := newBridge()
				if err != nil {
					continue
				}
				server.Bridge = bridge
				hosts = append(hosts, config.XrayHosts(t.server, settings.RealityOnXray)...)
			} else {
				// sing-box gets the addresses too: its own lookup would go to the system's DNS, which a TUN of
				// another client may hold or answer with fake addresses.
				hosts = append(hosts, t.server.Server)
			}
			in.Servers = append(in.Servers, server)
		}
		in.Resolved = resolveEach(ctx, e.systemResolver(), hosts, !settings.IPv6)
		e.runLatencyTest(ctx, in)
	}()
	return nil, nil
}

// resolveEach looks up what it can; a server whose name does not resolve fails its own test.
func resolveEach(ctx context.Context, resolver *net.Resolver, hosts []string, ipv4Only bool) map[string]netip.Addr {
	resolved := map[string]netip.Addr{}
	var mu sync.Mutex
	var wg sync.WaitGroup
	slots := make(chan struct{}, latencyConcurrency)
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
			lookupCtx, cancel := context.WithTimeout(ctx, latencyTimeout+dohTimeout)
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

func (e *Engine) runLatencyTest(ctx context.Context, in config.TestInput) {
	report := func(results []ServerLatency, done bool) {
		e.latency.mu.Lock()
		for _, result := range results {
			e.latency.results[result.ServerID] = result.Latency
		}
		e.latency.mu.Unlock()
		e.notify(EventServersTested, ServersTested{Results: results, Done: done})
	}
	configs, err := config.BuildTest(in)
	if err != nil {
		report(failedAll(in.Servers), true)
		return
	}
	var failed []ServerLatency
	var tags []string
	for _, server := range in.Servers {
		if configs.Failed[server.Tag] != nil {
			failed = append(failed, ServerLatency{ServerID: server.Tag, Latency: -1})
		} else {
			tags = append(tags, server.Tag)
		}
	}
	if len(failed) > 0 {
		report(failed, false)
	}
	probe.LimitWireGuardWorkers(&configs.SingBox)
	singBox, err := configs.SingBoxJSON()
	if err != nil || len(tags) == 0 {
		report(failedAll(nil), true)
		return
	}
	if configs.Xray != nil {
		instance, err := xray.CreateXrayInstance(string(configs.Xray))
		if err == nil {
			err = instance.Start()
		}
		if err != nil {
			report(failedTags(tags), true)
			return
		}
		defer instance.Close()
	}
	box, cancelBox, err := boxmain.Create(singBox, nil)
	if err != nil {
		report(failedTags(tags), true)
		return
	}
	defer func() {
		cancelBox()
		box.Close()
	}()
	probe.BatchURLTestTo(ctx, box, tags, e.latencyURL, latencyConcurrency, false, latencyTimeout, func(result *probe.URLTestResult) {
		latency := -1
		if result.Error == nil {
			latency = max(1, int(result.Duration.Milliseconds()))
		}
		report([]ServerLatency{{ServerID: result.Tag, Latency: latency}}, false)
	})
	report([]ServerLatency{}, true)
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
		probe.BatchURLTestTo(ctx, box, []string{config.TagProxy}, e.latencyURL, 1, false, latencyTimeout, func(result *probe.URLTestResult) {
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
	e.latency.mu.Lock()
	if e.latency.cancel != nil {
		e.latency.cancel()
	}
	e.latency.mu.Unlock()
	e.latency.running.Wait()
}
