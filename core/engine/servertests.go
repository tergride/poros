package engine

import (
	"context"
	"net/netip"
	"sync"
	"time"

	"github.com/sagernet/bbolt"

	"PorosCore/internal/probe"
	"PorosCore/profile"
)

const (
	// speedBudget bounds a speed test of many servers, which measures them one by one.
	speedBudget  = 30 * time.Minute
	speedTimeout = 15 * time.Second
	ipBudget     = 3 * time.Minute
)

// serverResults holds the speed and IP tests of the servers, by ID, until they are cleared or Poros closes.
type serverResults struct {
	mu    sync.Mutex
	speed map[string]ServerSpeed
	ip    map[string]ServerIP
}

func (r *serverResults) speedOf(id string) ServerSpeed {
	r.mu.Lock()
	defer r.mu.Unlock()
	return r.speed[id]
}

func (r *serverResults) ipOf(id string) ServerIP {
	r.mu.Lock()
	defer r.mu.Unlock()
	return r.ip[id]
}

// errorText is the words of an error, "" for none.
func errorText(err error) string {
	if err == nil {
		return ""
	}
	return err.Error()
}

// SpeedTestServers measures the speed of servers one by one in the background, as the settings say; a newer speed
// test replaces one that runs.
func (e *Engine) SpeedTestServers(query ServersQuery) (any, error) {
	servers, err := e.testServersOf(query)
	if err != nil {
		return nil, err
	}
	settings, err := e.Settings()
	if err != nil {
		return nil, err
	}
	ctx, done := e.tests.start(testSpeed, speedBudget)
	go func() {
		defer done()
		report := func(results []ServerSpeed, last bool) {
			e.results.mu.Lock()
			for _, result := range results {
				e.results.speed[result.ServerID] = result
			}
			e.results.mu.Unlock()
			e.notify(EventServersSpeedTested, ServersSpeedTested{Results: results, Done: last})
		}
		in := e.testInput(ctx, servers, settings)
		t, err := openTestBox(in)
		defer t.close()
		var failed []ServerSpeed
		for _, tag := range t.failed {
			failed = append(failed, ServerSpeed{ServerID: tag, Error: "the server cannot be built"})
		}
		if err != nil {
			for _, tag := range t.tags {
				failed = append(failed, ServerSpeed{ServerID: tag, Error: err.Error()})
			}
		}
		if len(failed) > 0 {
			report(failed, false)
		}
		if err != nil || t.box == nil {
			report([]ServerSpeed{}, true)
			return
		}
		mode := settings.SpeedTestMode
		download := mode == SpeedTestFull || mode == SpeedTestDownload
		upload := mode == SpeedTestFull || mode == SpeedTestUpload
		for _, tag := range t.tags {
			if ctx.Err() != nil {
				break
			}
			results := probe.BatchSpeedTest(ctx, t.box, []string{tag}, download, upload, mode == SpeedTestSimple,
				settings.SpeedTestURL, speedTimeout, mode == SpeedTestCountry, 1)
			for _, result := range results {
				if result.Error == nil && ctx.Err() != nil {
					continue
				}
				report([]ServerSpeed{{
					ServerID: result.Tag, Download: result.DlSpeed, Upload: result.UlSpeed, Country: result.ServerCountry,
					Error: errorText(result.Error),
				}}, false)
				e.addTraffic(result.Tag, result.UlBytes, result.DlBytes)
			}
		}
		report([]ServerSpeed{}, true)
	}()
	return nil, nil
}

// IPTestServers finds the address sites see each server's traffic come from, in the background.
func (e *Engine) IPTestServers(query ServersQuery) (any, error) {
	servers, err := e.testServersOf(query)
	if err != nil {
		return nil, err
	}
	settings, err := e.Settings()
	if err != nil {
		return nil, err
	}
	ctx, done := e.tests.start(testIP, ipBudget)
	go func() {
		defer done()
		in := e.testInput(ctx, servers, settings)
		t, err := openTestBox(in)
		defer t.close()
		results := []ServerIP{}
		for _, tag := range t.failed {
			results = append(results, ServerIP{ServerID: tag, Error: "the server cannot be built"})
		}
		switch {
		case err != nil:
			for _, tag := range t.tags {
				results = append(results, ServerIP{ServerID: tag, Error: err.Error()})
			}
		case t.box != nil:
			for _, result := range probe.BatchIPTest(ctx, t.box, t.tags, latencyConcurrency, true, 0) {
				results = append(results, ServerIP{
					ServerID: result.Tag, IP: result.Result.IP, Country: result.Result.CountryCode, Error: errorText(result.Error),
				})
			}
		}
		if ctx.Err() != nil {
			return
		}
		e.results.mu.Lock()
		for _, result := range results {
			e.results.ip[result.ServerID] = result
		}
		e.results.mu.Unlock()
		e.notify(EventServersIPTested, ServersIPTested{Results: results, Done: true})
	}()
	return nil, nil
}

// ResolveServers replaces the host names of servers by the addresses they resolve to now, looked up past any TUN.
// The servers keep their names for TLS and HTTP. A chain, Direct and a custom outbound have no host to resolve.
func (e *Engine) ResolveServers(query ServersQuery) (ServersResolved, error) {
	settings, err := e.Settings()
	if err != nil {
		return ServersResolved{}, err
	}
	var hosts []string
	if err = e.db.View(func(tx *bbolt.Tx) error {
		store, err := loadServers(tx)
		if err != nil {
			return err
		}
		for _, id := range queryIDs(store, query) {
			g, i := store.locate(id)
			if host := store.servers[store.groups[g].ID][i].Profile.Server; resolvable(host) {
				hosts = append(hosts, host)
			}
		}
		return nil
	}); err != nil {
		return ServersResolved{}, err
	}
	ctx, cancel := context.WithTimeout(context.Background(), ipBudget)
	defer cancel()
	addresses := resolveEach(ctx, e.systemResolver(), hosts, !settings.IPv6)
	var resolved ServersResolved
	err = e.changeServers(func(s *serverStore) error {
		for _, id := range queryIDs(s, query) {
			g, i := s.locate(id)
			groupID := s.groups[g].ID
			r := &s.servers[groupID][i]
			if addr, ok := addresses[r.Profile.Server]; ok {
				r.Profile = r.Profile.WithAddress(addr.String())
				s.changed[groupID] = true
				resolved.Resolved++
			}
		}
		return nil
	})
	return resolved, err
}

// resolvable is a host name, as opposed to an address or nothing.
func resolvable(host string) bool {
	if host == "" {
		return false
	}
	_, err := netip.ParseAddr(host)
	return err != nil
}

// ClearResults forgets the delay, speed and IP tests of servers.
func (e *Engine) ClearResults(query ServersQuery) error {
	groups, err := e.queryGroups(query)
	if err != nil {
		return err
	}
	e.latency.mu.Lock()
	e.results.mu.Lock()
	for _, ids := range groups {
		for _, id := range ids {
			delete(e.latency.results, id)
			delete(e.results.speed, id)
			delete(e.results.ip, id)
		}
	}
	e.results.mu.Unlock()
	e.latency.mu.Unlock()
	for groupID := range groups {
		e.notify(EventServersChanged, ServersChanged{GroupID: groupID})
	}
	return nil
}

// queryGroups lists the servers a query names by their groups.
func (e *Engine) queryGroups(query ServersQuery) (map[string][]string, error) {
	groups := map[string][]string{}
	err := e.db.View(func(tx *bbolt.Tx) error {
		store, err := loadServers(tx)
		if err != nil {
			return err
		}
		for _, id := range queryIDs(store, query) {
			g, _ := store.locate(id)
			groups[store.groups[g].ID] = append(groups[store.groups[g].ID], id)
		}
		return nil
	})
	return groups, err
}

// CleanServers removes the servers of a group, or of all, that did not answer their last delay test, came twice,
// or protect the traffic poorly.
func (e *Engine) CleanServers(req CleanServers) (ServerIDs, error) {
	removed := ServerIDs{IDs: []string{}}
	err := e.changeServers(func(s *serverStore) error {
		for _, group := range s.groups {
			if req.GroupID != "" && group.ID != req.GroupID {
				continue
			}
			seen := map[string]bool{}
			kept := s.servers[group.ID][:0:0]
			for _, r := range s.servers[group.ID] {
				drop := (req.Unavailable && e.latencyOf(r.ID) < 0) || (req.Insecure && r.Profile.Security() != profile.SecurityOK)
				if req.Duplicates && !drop {
					key := sameServerKey(r.Profile)
					drop = seen[key]
					seen[key] = true
				}
				if drop {
					removed.IDs = append(removed.IDs, r.ID)
					continue
				}
				kept = append(kept, r)
			}
			if len(kept) != len(s.servers[group.ID]) {
				s.servers[group.ID] = kept
				s.changed[group.ID] = true
			}
		}
		return nil
	})
	return removed, err
}
