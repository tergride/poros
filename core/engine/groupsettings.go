package engine

import (
	"cmp"
	"crypto/tls"
	"encoding/json"
	"net/http"
	"net/netip"
	"slices"
	"strconv"
	"strings"

	"github.com/sagernet/bbolt"

	"PorosCore/internal/jsonrpc"
	"PorosCore/profile"
)

// maxGroupUpdateHours bounds the interval of a group as the settings bound theirs.
const maxGroupUpdateHours = maxUpdateHours

// validateGroupSettings checks the settings of a group against the servers there are. A group of the user's takes
// only what changes how its servers connect.
func validateGroupSettings(s *serverStore, rec groupRecord, gs GroupSettings) error {
	invalid := func(format string, args ...any) error { return jsonrpc.Failed(ReasonInvalidValue, format, args...) }
	if address := strings.TrimSpace(gs.ServerAddress); address != "" {
		if _, err := netip.ParseAddr(address); err != nil && !validHostName(address) {
			return invalid("%q is no host name or IP address", address)
		}
	}
	for _, id := range []string{gs.FrontProxy, gs.LandingProxy} {
		if id == "" {
			continue
		}
		g, i := s.locate(id)
		switch {
		case g < 0:
			return serverNotFound(id)
		case s.groups[g].ID == rec.ID:
			return invalid("a front or landing proxy is a server of another group")
		}
		switch s.servers[s.groups[g].ID][i].Profile.Type {
		case profile.TypeChain, profile.TypeDirect:
			return invalid("a front or landing proxy is a server of its own, not a chain or Direct")
		}
	}
	if gs.FrontProxy != "" && gs.FrontProxy == gs.LandingProxy {
		return invalid("the front and the landing proxy are two servers")
	}
	subscriptionOnly := gs
	subscriptionOnly.ServerAddress, subscriptionOnly.FrontProxy, subscriptionOnly.LandingProxy = "", "", ""
	if rec.manual() && subscriptionOnly != (GroupSettings{}) {
		return invalid("the request and updates are settings of a subscription")
	}
	choices := []struct {
		value, what string
		allowed     []string
	}{
		{gs.TLS, "TLS version", []string{"", "auto", "1.2", "1.3"}},
		{gs.HTTP, "HTTP version", []string{"", "auto", "1.1"}},
		{gs.DeviceInfo, "device info choice", []string{"", "on", "off"}},
		{gs.Update, "update choice", []string{GroupUpdateDefault, GroupUpdateProvider, GroupUpdateHours, GroupUpdateNever}},
	}
	for _, c := range choices {
		if !slices.Contains(c.allowed, c.value) {
			return invalid("unknown %s %q", c.what, c.value)
		}
	}
	if gs.Update == GroupUpdateHours && (gs.UpdateHours < 1 || gs.UpdateHours > maxGroupUpdateHours) {
		return invalid("the update interval is from 1 to %d hours", maxGroupUpdateHours)
	}
	if gs.Update != GroupUpdateHours && gs.UpdateHours != 0 {
		return invalid("an update interval goes with updates every few hours")
	}
	if (gs.RemoveUnavailable || gs.SortByLatency) && !gs.Test {
		return invalid("removing unavailable servers and sorting by delay go by the test after the update")
	}
	return nil
}

// validHostName accepts the letters, digits, hyphens and dots of a host name.
func validHostName(host string) bool {
	if host == "" || len(host) > 253 || strings.HasPrefix(host, ".") || strings.HasSuffix(host, ".") {
		return false
	}
	for _, label := range strings.Split(host, ".") {
		if label == "" || len(label) > 63 || strings.HasPrefix(label, "-") || strings.HasSuffix(label, "-") {
			return false
		}
		for _, r := range label {
			if !(r >= 'a' && r <= 'z' || r >= 'A' && r <= 'Z' || r >= '0' && r <= '9' || r == '-') {
				return false
			}
		}
	}
	return true
}

// ConfigureGroup changes the settings of a group and the URL of a subscription.
func (e *Engine) ConfigureGroup(req ConfigureGroup) (Group, error) {
	req.Settings.ServerAddress = strings.TrimSpace(req.Settings.ServerAddress)
	url := strings.TrimSpace(req.URL)
	e.writeMu.Lock()
	var rec groupRecord
	err := e.db.Update(func(tx *bbolt.Tx) error {
		store, err := loadServers(tx)
		if err != nil {
			return err
		}
		g := slices.IndexFunc(store.groups, func(r groupRecord) bool { return r.ID == req.ID })
		if g < 0 {
			return groupNotFound(req.ID)
		}
		rec = store.groups[g]
		if err = validateGroupSettings(store, rec, req.Settings); err != nil {
			return err
		}
		if url != "" && url != rec.URL {
			if rec.manual() {
				return jsonrpc.Failed(ReasonInvalidValue, "a group of the user's has no URL")
			}
			if err = checkSubscriptionURL(url); err != nil {
				return err
			}
			if slices.ContainsFunc(store.groups, func(r groupRecord) bool { return r.URL == url }) {
				return jsonrpc.Failed(ReasonDuplicate, "this subscription is already added")
			}
			rec.URL = url
		}
		rec.Settings = req.Settings
		return putJSON(tx, bucketGroups, []byte(rec.ID), rec)
	})
	e.writeMu.Unlock()
	if err != nil {
		return Group{}, err
	}
	e.announce(rec.ID)
	return rec.view(), nil
}

// resolveServer gives what a server connects with: its profile with the address of its group, and for a chain, or
// a group with a front or landing proxy, the servers to go through in order. found is false for a missing server; a
// missing server of a chain or a proxy that is gone is a missingLink error.
func resolveServer(s *serverStore, id string) (p profile.Profile, links []profile.Profile, found bool, err error) {
	g, i := s.locate(id)
	if g < 0 {
		return profile.Profile{}, nil, false, nil
	}
	group := s.groups[g]
	p = withGroupAddress(group, s.servers[group.ID][i].Profile)
	// One server named by ID, with the address of its own group.
	server := func(id string) (profile.Profile, error) {
		g, i := s.locate(id)
		if g < 0 {
			return profile.Profile{}, missingLink(id)
		}
		return withGroupAddress(s.groups[g], s.servers[s.groups[g].ID][i].Profile), nil
	}
	if p.Type == profile.TypeChain {
		for _, linkID := range p.Chain.Servers {
			link, err := server(linkID)
			if err != nil {
				return p, nil, true, err
			}
			links = append(links, link)
		}
	}
	front, landing := group.Settings.FrontProxy, group.Settings.LandingProxy
	if front == "" && landing == "" {
		return p, links, true, nil
	}
	// The server between the proxies, as a chain of its own; a chain takes them around its links.
	middle := links
	if p.Type != profile.TypeChain {
		middle = []profile.Profile{p}
	}
	var hops []profile.Profile
	if front != "" {
		proxy, err := server(front)
		if err != nil {
			return p, nil, true, err
		}
		hops = append(hops, proxy)
	}
	hops = append(hops, middle...)
	if landing != "" {
		proxy, err := server(landing)
		if err != nil {
			return p, nil, true, err
		}
		hops = append(hops, proxy)
	}
	// The hops are already resolved: the chain only counts them.
	ids := make([]string, len(hops))
	for i := range hops {
		ids[i] = "hop-" + strconv.Itoa(i+1)
	}
	chain := profile.Profile{Type: profile.TypeChain, Name: p.Name, Chain: &profile.Chain{Servers: ids}}
	return chain, hops, true, nil
}

// withGroupAddress dials a server at the address its group gives, keeping its own name for TLS and HTTP.
func withGroupAddress(group groupRecord, p profile.Profile) profile.Profile {
	if group.Settings.ServerAddress == "" {
		return p
	}
	return p.WithAddress(group.Settings.ServerAddress)
}

// subscriptionRequest is how a subscription downloads: the User-Agent, the device headers and the client.
type subscriptionRequest struct {
	userAgent string
	device    http.Header
	tls       string
	http11    bool
}

func (e *Engine) requestOf(rec groupRecord, settings Settings) subscriptionRequest {
	gs := rec.Settings
	req := subscriptionRequest{userAgent: e.userAgent, tls: settings.SubscriptionTLS, http11: settings.SubscriptionHTTP == "1.1"}
	if agent := strings.TrimSpace(cmp.Or(gs.UserAgent, settings.SubscriptionUserAgent)); agent != "" {
		req.userAgent = agent
	}
	switch gs.TLS {
	case "auto":
		req.tls = ""
	case "1.2", "1.3":
		req.tls = gs.TLS
	}
	switch gs.HTTP {
	case "auto":
		req.http11 = false
	case "1.1":
		req.http11 = true
	}
	send := settings.SendDeviceInfo
	switch gs.DeviceInfo {
	case "on":
		send = true
	case "off":
		send = false
	}
	if send {
		req.device = e.deviceHeadersOf(gs)
	}
	return req
}

// client gives the client to download with: Poros's own, unless the request needs a TLS or HTTP version of its own.
func (r subscriptionRequest) client(base *http.Client) *http.Client {
	if r.tls == "" && !r.http11 {
		return base
	}
	transport, ok := base.Transport.(*http.Transport)
	if !ok {
		return base
	}
	transport = transport.Clone()
	config := &tls.Config{}
	if transport.TLSClientConfig != nil {
		config = transport.TLSClientConfig.Clone()
	}
	switch r.tls {
	case "1.2":
		config.MinVersion, config.MaxVersion = tls.VersionTLS12, tls.VersionTLS12
	case "1.3":
		config.MinVersion, config.MaxVersion = tls.VersionTLS13, tls.VersionTLS13
	}
	transport.TLSClientConfig = config
	if r.http11 {
		transport.ForceAttemptHTTP2 = false
		transport.TLSNextProto = map[string]func(string, *tls.Conn) http.RoundTripper{}
	}
	client := *base
	client.Transport = transport
	return &client
}

// afterDownload applies what the group does with a download before it is stored: duplicates and insecure servers
// left out.
func afterDownload(gs GroupSettings, profiles []profile.Profile) []profile.Profile {
	if !gs.RemoveDuplicates && !gs.RemoveInsecure {
		return profiles
	}
	kept := make([]profile.Profile, 0, len(profiles))
	seen := map[string]bool{}
	for _, p := range profiles {
		if gs.RemoveInsecure && p.Security() != profile.SecurityOK {
			continue
		}
		if gs.RemoveDuplicates {
			// The same server under another name is still the same server.
			unnamed := p
			unnamed.Name = ""
			key, _ := json.Marshal(unnamed)
			if seen[string(key)] {
				continue
			}
			seen[string(key)] = true
		}
		kept = append(kept, p)
	}
	return kept
}

// keepWorking adds to the servers of an update those the provider dropped whose last delay test worked.
func (e *Engine) keepWorking(gs GroupSettings, old, servers []serverRecord) []serverRecord {
	if !gs.KeepWorking {
		return servers
	}
	present := map[string]bool{}
	for _, r := range servers {
		present[r.ID] = true
	}
	for _, r := range old {
		if !present[r.ID] && e.latencyOf(r.ID) > 0 {
			servers = append(servers, r)
		}
	}
	return servers
}

// afterTest removes the servers of a group that did not answer, or puts the fastest first, by a test's results.
func (e *Engine) afterTest(groupID string, results map[string]int) {
	rec, found, err := e.group(groupID)
	if err != nil || !found || (!rec.Settings.RemoveUnavailable && !rec.Settings.SortByLatency) {
		return
	}
	_ = e.changeServers(func(s *serverStore) error {
		servers := s.servers[groupID]
		if rec.Settings.RemoveUnavailable {
			kept := servers[:0:0]
			for _, r := range servers {
				if results[r.ID] >= 0 {
					kept = append(kept, r)
				}
			}
			servers = kept
		}
		if rec.Settings.SortByLatency {
			// Measured first, fastest ahead; then the untested, then those that did not answer.
			rank := func(latency int) (int, int) {
				switch {
				case latency > 0:
					return 0, latency
				case latency == 0:
					return 1, 0
				}
				return 2, 0
			}
			slices.SortStableFunc(servers, func(a, b serverRecord) int {
				ra, la := rank(results[a.ID])
				rb, lb := rank(results[b.ID])
				return cmp.Or(cmp.Compare(ra, rb), cmp.Compare(la, lb))
			})
		}
		s.servers[groupID] = servers
		s.changed[groupID] = true
		return nil
	})
}
