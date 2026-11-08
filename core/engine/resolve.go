package engine

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/netip"
	"time"

	"github.com/sagernet/sing/common/control"

	"PorosCore/internal/netmon"
)

// fakeAddresses are where the DNS of a client with FakeIP, such as Throne, points every name: such an address leads
// only into that client's TUN, and Poros dials its servers past it.
var fakeAddresses = []netip.Prefix{netip.MustParsePrefix("198.18.0.0/15"), netip.MustParsePrefix("fc00::/18")}

// dohServers answer by address, so that reaching them needs no lookup of their own; both speak the JSON form.
var dohServers = []string{"https://1.1.1.1/dns-query", "https://8.8.8.8/resolve"}

const dohTimeout = 4 * time.Second

// systemLookupTimeout bounds the system's answer, so that DoH keeps its own time when the system's DNS hangs; a
// test shortens it.
var systemLookupTimeout = 3 * time.Second

// processResolver asks the system's DNS servers from the core itself. Behind the kill switch the DNS client of
// Windows, a service of its own, cannot reach them, while the block lets Poros through.
var processResolver = &net.Resolver{PreferGo: true}

// systemResolver is how the core asks the system's DNS: through Windows, or by itself while the kill switch blocks.
func (e *Engine) systemResolver() *net.Resolver {
	if e.KillSwitchState().On {
		return processResolver
	}
	return net.DefaultResolver
}

// dohClient makes the client of the DoH queries; a test replaces it.
var dohClient = adapterClient

func isFake(addr netip.Addr) bool {
	for _, prefix := range fakeAddresses {
		if prefix.Contains(addr) {
			return true
		}
	}
	return false
}

// lookupPast finds the address of a server the way Poros dials it: out of the network adapter, past the TUN of
// another client. The system's answer serves unless it fails or is fake; another client's TUN may hold the system's
// DNS and let no plain DNS past it, so then DoH over the adapter answers.
func lookupPast(ctx context.Context, resolver *net.Resolver, host string, ipv4Only bool) (netip.Addr, error) {
	if addr, err := netip.ParseAddr(host); err == nil {
		return addr.Unmap(), nil
	}
	network := "ip"
	if ipv4Only {
		network = "ip4"
	}
	systemCtx, cancel := context.WithTimeout(ctx, systemLookupTimeout)
	addrs, systemErr := resolver.LookupNetIP(systemCtx, network, host)
	cancel()
	if addr, ok := preferIPv4(addrs); ok && !isFake(addr) {
		return addr, nil
	}
	addr, err := lookupDoH(ctx, host, ipv4Only)
	if err != nil {
		return netip.Addr{}, fmt.Errorf("cannot resolve %s: %w", host, errors.Join(systemErr, err))
	}
	return addr, nil
}

func preferIPv4(addrs []netip.Addr) (netip.Addr, bool) {
	if len(addrs) == 0 {
		return netip.Addr{}, false
	}
	for _, addr := range addrs {
		if addr.Unmap().Is4() {
			return addr.Unmap(), true
		}
	}
	return addrs[0].Unmap(), true
}

// lookupDoH asks the DoH servers at once over the network adapter and takes the first real answer: A, then AAAA
// when IPv6 is allowed.
func lookupDoH(ctx context.Context, host string, ipv4Only bool) (netip.Addr, error) {
	ctx, cancel := context.WithTimeout(ctx, dohTimeout)
	defer cancel()
	client := dohClient()
	types := []int{1}
	if !ipv4Only {
		types = append(types, 28)
	}
	var errs []error
	for _, kind := range types {
		answers := make(chan dohAnswer, len(dohServers))
		for _, server := range dohServers {
			go func() { answers <- askDoH(ctx, client, server, host, kind) }()
		}
		for range dohServers {
			answer := <-answers
			if answer.err != nil {
				errs = append(errs, answer.err)
				continue
			}
			if !isFake(answer.addr) {
				return answer.addr, nil
			}
		}
	}
	return netip.Addr{}, errors.Join(append(errs, fmt.Errorf("no real address for %s", host))...)
}

type dohAnswer struct {
	addr netip.Addr
	err  error
}

func askDoH(ctx context.Context, client *http.Client, server, host string, kind int) dohAnswer {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, fmt.Sprintf("%s?name=%s&type=%d", server, host, kind), nil)
	if err != nil {
		return dohAnswer{err: err}
	}
	req.Header.Set("Accept", "application/dns-json")
	resp, err := client.Do(req)
	if err != nil {
		return dohAnswer{err: err}
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return dohAnswer{err: fmt.Errorf("%s answered %s", server, resp.Status)}
	}
	var reply struct {
		Answer []struct {
			Type int    `json:"type"`
			Data string `json:"data"`
		}
	}
	if err = json.NewDecoder(io.LimitReader(resp.Body, 64<<10)).Decode(&reply); err != nil {
		return dohAnswer{err: err}
	}
	for _, answer := range reply.Answer {
		if answer.Type != kind {
			continue
		}
		if addr, err := netip.ParseAddr(answer.Data); err == nil {
			return dohAnswer{addr: addr.Unmap()}
		}
	}
	return dohAnswer{err: fmt.Errorf("%s has no address for %s", server, host)}
}

// dialDirect dials past any proxy, asking the system's DNS the way the kill switch lets the core.
func (e *Engine) dialDirect(ctx context.Context, network, address string) (net.Conn, error) {
	dialer := net.Dialer{Timeout: 30 * time.Second, KeepAlive: 30 * time.Second, Resolver: e.systemResolver()}
	return dialer.DialContext(ctx, network, address)
}

// adapterClient dials out of the default network adapter, so that no TUN, Poros's own or another client's, takes
// the query.
func adapterClient() *http.Client {
	dialer := &net.Dialer{Timeout: dohTimeout}
	if iface := netmon.DefaultInterface(); iface != nil && netmon.Finder() != nil {
		dialer.Control = control.BindToInterface(netmon.Finder(), iface.Name, iface.Index)
	}
	return &http.Client{
		Timeout:   dohTimeout,
		Transport: &http.Transport{DialContext: dialer.DialContext, Proxy: nil, ForceAttemptHTTP2: true},
	}
}
