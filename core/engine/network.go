package engine

import (
	"context"
	"errors"
	"fmt"
	"log"
	"net"
	"net/http"
	"net/netip"
	"strings"
	"sync"
	"time"

	"github.com/sagernet/sing-box/adapter"
	C "github.com/sagernet/sing-box/constant"
	"github.com/sagernet/sing/common/metadata"
	N "github.com/sagernet/sing/common/network"
	"github.com/sagernet/sing/service"

	"PorosCore/config"
	"PorosCore/internal/boxbox"
	"PorosCore/internal/jsonrpc"
	"PorosCore/internal/netcheck"
)

// networkCheckTimeout bounds a whole check: the services and the leak test run side by side.
const networkCheckTimeout = 40 * time.Second

type networkCheck struct {
	mu    sync.Mutex
	state NetworkState
	// enabled is set by StartNetworkChecks: until then nothing is sent to outside services, as in tests.
	enabled bool
	serial  uint64
	cancel  context.CancelFunc
	running sync.WaitGroup
}

// StartNetworkChecks lets the engine check the addresses on its own after each start and stop of the connection
// and after network changes, while the user keeps the check on. The first check is the interface's to ask for,
// when it shows the address: the core sends nothing out just for starting.
func (e *Engine) StartNetworkChecks() {
	e.net.mu.Lock()
	e.net.enabled = true
	e.net.mu.Unlock()
}

func (e *Engine) NetworkState() NetworkState {
	e.net.mu.Lock()
	defer e.net.mu.Unlock()
	return e.net.state
}

// CheckNetwork checks now, at the user's request; the result comes with network.changed.
func (e *Engine) CheckNetwork() (NetworkState, error) {
	settings, err := e.Settings()
	if err != nil {
		return NetworkState{}, err
	}
	if !settings.CheckIP {
		return NetworkState{}, jsonrpc.Failed(ReasonCheckOff, "the IP and DNS check is turned off")
	}
	return e.startNetworkCheck(), nil
}

// requestNetworkCheck checks when the engine may and the user wants it.
func (e *Engine) requestNetworkCheck() {
	e.net.mu.Lock()
	enabled := e.net.enabled
	e.net.mu.Unlock()
	if !enabled {
		return
	}
	if settings, err := e.Settings(); err == nil && settings.CheckIP {
		e.startNetworkCheck()
	}
}

// clearNetworkCheck forgets the results: the user turned the check off, or the network went away.
func (e *Engine) clearNetworkCheck() {
	e.net.mu.Lock()
	defer e.net.mu.Unlock()
	if e.net.cancel != nil {
		e.net.cancel()
	}
	e.net.serial++
	e.setNetworkState(NetworkState{})
}

// setNetworkState announces a state; the caller holds net.mu.
func (e *Engine) setNetworkState(state NetworkState) {
	if state.DNS == nil {
		state.DNS = []DNSServer{}
	}
	e.net.state = state
	e.notify(EventNetworkChanged, state)
}

// startNetworkCheck runs a check in the background; a newer one cancels the one before.
func (e *Engine) startNetworkCheck() NetworkState {
	e.net.mu.Lock()
	defer e.net.mu.Unlock()
	if e.net.cancel != nil {
		e.net.cancel()
	}
	ctx, cancel := context.WithTimeout(context.Background(), networkCheckTimeout)
	e.net.cancel = cancel
	e.net.serial++
	serial := e.net.serial
	state := e.net.state
	state.Checking = true
	e.setNetworkState(state)
	e.net.running.Add(1)
	go func() {
		defer e.net.running.Done()
		defer cancel()
		result := e.runNetworkCheck(ctx)
		e.net.mu.Lock()
		defer e.net.mu.Unlock()
		if serial == e.net.serial {
			e.setNetworkState(result)
		}
	}()
	return state
}

func (e *Engine) closeNetworkChecks() {
	e.net.mu.Lock()
	e.net.enabled = false
	if e.net.cancel != nil {
		e.net.cancel()
	}
	e.net.mu.Unlock()
	e.net.running.Wait()
}

func (e *Engine) runNetworkCheck(ctx context.Context) NetworkState {
	state := NetworkState{}
	var errs []error
	e.conn.lifecycle.Lock()
	running := e.conn.running
	e.conn.lifecycle.Unlock()
	settings, err := e.Settings()
	if err != nil {
		settings = DefaultSettings()
	}

	switch {
	case running != nil && running.refusing:
		// The refusal has no way through the server; the way out is the preset's direct one, past its TUN.
		directClient := func(family int) (*http.Client, func()) {
			return outboundClient(running.box, config.TagDirect, config.TagDNSDirect, family)
		}
		direct, _, err := lookupAddresses(ctx, directClient, true)
		if err != nil {
			errs = append(errs, fmt.Errorf("without the VPN: %w", err))
		}
		state.Direct = direct
	case running == nil:
		// Without a connection the way out is the system's; Poros's own proxy is down, so no proxy.
		direct, _, err := lookupAddresses(ctx, e.plainClient, true)
		if err != nil {
			errs = append(errs, fmt.Errorf("without the VPN: %w", err))
		}
		state.Direct = direct
	default:
		// Through the outbounds of the running connection, not its routes: with TUN, plain requests would go
		// into the tunnel, and a preset may send the services' sites either way.
		directClient := func(family int) (*http.Client, func()) {
			return outboundClient(running.box, config.TagDirect, config.TagDNSDirect, family)
		}
		proxyClient := func(family int) (*http.Client, func()) {
			return outboundClient(running.box, config.TagProxy, config.TagDNSRemote, family)
		}
		leakClient, closeLeak := outboundClient(running.box, config.TagProxy, "", 0)
		defer closeLeak()
		var direct, proxy IPInfo
		var home netcheck.Info
		var servers []netcheck.Info
		var directErr, proxyErr, leakErr error
		var wg sync.WaitGroup
		wg.Go(func() { direct, home, directErr = lookupAddresses(ctx, directClient, true) })
		// Without IPv6 the connection reaches sites over IPv4 only, and so does the check.
		wg.Go(func() { proxy, _, proxyErr = lookupAddresses(ctx, proxyClient, settings.IPv6) })
		wg.Go(func() {
			// Names resolve the way programs resolve them: with TUN the system's queries go into the tunnel,
			// with System alone they go to the provider, which is the leak the user should see.
			servers, leakErr = netcheck.Leak(ctx, leakClient, netcheck.LeakService, func(ctx context.Context, host string) error {
				_, err := net.DefaultResolver.LookupHost(ctx, host)
				return err
			})
		})
		wg.Wait()
		for _, part := range []struct {
			name string
			err  error
		}{{"without the VPN", directErr}, {"through the VPN", proxyErr}, {"DNS leak test", leakErr}} {
			if part.err != nil {
				errs = append(errs, fmt.Errorf("%s: %w", part.name, part.err))
			}
		}
		state.Direct, state.Proxy = direct, proxy
		if leakErr == nil {
			state.DNS, state.Leak = dnsServers(servers, home)
		}
	}
	state.CheckedAt = e.now().UnixMilli()
	if len(errs) > 0 {
		state.Error, state.ErrorText = ReasonCheckFailed, errors.Join(errs...).Error()
		log.Printf("network check: %s", state.ErrorText)
	}
	return state
}

// ipv6CheckTimeout bounds the IPv6 part of a check: a network without IPv6 may let its requests hang rather than
// refuse them, and the IPv4 answer should not wait for that.
const ipv6CheckTimeout = 10 * time.Second

// familyClient makes a client whose requests go over one IP family, 4 or 6; the function closes its connections.
type familyClient func(family int) (*http.Client, func())

// lookupAddresses asks the services over IPv4 and, with ipv6, over IPv6, so that both addresses show. The place
// and the provider are those of the IPv4 answer when there is one. A family without an answer is no error, as long
// as the other has one: a network may have only one of them.
func lookupAddresses(ctx context.Context, client familyClient, ipv6 bool) (IPInfo, netcheck.Info, error) {
	var v4, v6 netcheck.Info
	var err4, err6 error
	var wg sync.WaitGroup
	wg.Go(func() {
		c, closeIdle := client(4)
		defer closeIdle()
		v4, err4 = netcheck.Lookup(ctx, c, netcheck.Services)
	})
	if ipv6 {
		wg.Go(func() {
			c, closeIdle := client(6)
			defer closeIdle()
			ctx, cancel := context.WithTimeout(ctx, ipv6CheckTimeout)
			defer cancel()
			v6, err6 = netcheck.Lookup(ctx, c, netcheck.Services)
		})
	}
	wg.Wait()
	switch {
	case err4 == nil && err6 == nil && ipv6:
		info := ipInfo(v4)
		info.IPv6 = v6.IP
		return info, v4, nil
	case err4 == nil:
		return ipInfo(v4), v4, nil
	case ipv6 && err6 == nil:
		info := ipInfo(v6)
		info.IP, info.IPv6 = "", v6.IP
		return info, v6, nil
	}
	return IPInfo{}, netcheck.Info{}, errors.Join(err4, err6)
}

// dnsServers marks the servers of the user's own provider and tells a leak: a name of the test resolved there went
// past the VPN. Without the home provider known there is no telling.
func dnsServers(servers []netcheck.Info, home netcheck.Info) ([]DNSServer, string) {
	result := make([]DNSServer, len(servers))
	leak := LeakNone
	if home.ASN == 0 && home.ISP == "" {
		leak = LeakUnknown
	}
	for i, server := range servers {
		own := (home.ASN != 0 && server.ASN == home.ASN) ||
			(home.ASN == 0 && home.ISP != "" && strings.EqualFold(server.ISP, home.ISP))
		result[i] = DNSServer{IP: server.IP, CountryCode: server.CountryCode, City: server.City, ISP: server.ISP, ASN: server.ASN, Home: own}
		if own {
			leak = LeakFound
		}
	}
	return result, leak
}

func ipInfo(info netcheck.Info) IPInfo {
	return IPInfo{IP: info.IP, CountryCode: info.CountryCode, City: info.City, ISP: info.ISP, ASN: info.ASN}
}

// plainClient sends requests past any proxy over one IP family, asking the system's DNS the way the kill switch
// lets the core.
func (e *Engine) plainClient(family int) (*http.Client, func()) {
	network := "tcp4"
	if family == 6 {
		network = "tcp6"
	}
	transport := &http.Transport{
		Proxy: nil,
		DialContext: func(ctx context.Context, _, address string) (net.Conn, error) {
			dialer := net.Dialer{Timeout: 30 * time.Second, Resolver: e.systemResolver()}
			return dialer.DialContext(ctx, network, address)
		},
		ForceAttemptHTTP2: true,
	}
	return &http.Client{Transport: transport}, transport.CloseIdleConnections
}

// outboundClient sends requests through one outbound of a running connection. With family 0 the name goes along
// and resolves on the outbound's side; with 4 or 6 the DNS server dnsTag of the connection finds an address of that
// family, so that the service sees the address of that family, or none when the way out has none.
func outboundClient(box *boxbox.Box, tag, dnsTag string, family int) (*http.Client, func()) {
	outbound, found := box.Outbound().Outbound(tag)
	transport := &http.Transport{
		Proxy: nil,
		DialContext: func(ctx context.Context, _, address string) (net.Conn, error) {
			if !found {
				return nil, fmt.Errorf("the connection has no %s outbound", tag)
			}
			destination := metadata.ParseSocksaddr(address)
			if family != 0 && destination.IsFqdn() {
				addr, err := lookupFamily(ctx, box, dnsTag, destination.Fqdn, family)
				if err != nil {
					return nil, err
				}
				destination = metadata.SocksaddrFrom(addr, destination.Port)
			}
			return outbound.DialContext(ctx, N.NetworkTCP, destination)
		},
		ForceAttemptHTTP2: true,
	}
	return &http.Client{Transport: transport}, transport.CloseIdleConnections
}

// lookupFamily finds an address of one family with a DNS server of the running connection.
func lookupFamily(ctx context.Context, box *boxbox.Box, dnsTag, host string, family int) (netip.Addr, error) {
	router := service.FromContext[adapter.DNSRouter](box.Context())
	transports := service.FromContext[adapter.DNSTransportManager](box.Context())
	if router == nil || transports == nil {
		return netip.Addr{}, errors.New("the connection has no DNS")
	}
	transport, found := transports.Transport(dnsTag)
	if !found {
		return netip.Addr{}, fmt.Errorf("the connection has no DNS server %s", dnsTag)
	}
	strategy := C.DomainStrategyIPv4Only
	if family == 6 {
		strategy = C.DomainStrategyIPv6Only
	}
	addrs, err := router.Lookup(ctx, host, adapter.DNSQueryOptions{Transport: transport, Strategy: strategy})
	if err != nil {
		return netip.Addr{}, err
	}
	for _, addr := range addrs {
		if addr = addr.Unmap(); addr.Is6() == (family == 6) {
			return addr, nil
		}
	}
	return netip.Addr{}, fmt.Errorf("%s has no IPv%d address", host, family)
}
