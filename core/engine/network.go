package engine

import (
	"context"
	"errors"
	"fmt"
	"net"
	"net/http"
	"strings"
	"sync"
	"time"

	"github.com/sagernet/sing/common/metadata"
	N "github.com/sagernet/sing/common/network"

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

// clearNetworkCheck forgets the results once the user turns the check off.
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

	if running == nil {
		// Without a connection the way out is the system's; Poros's own proxy is down, so no proxy.
		transport := http.DefaultTransport.(*http.Transport).Clone()
		transport.Proxy = nil
		defer transport.CloseIdleConnections()
		direct, err := netcheck.Lookup(ctx, &http.Client{Transport: transport}, netcheck.Services)
		if err != nil {
			errs = append(errs, fmt.Errorf("without the VPN: %w", err))
		}
		state.Direct = ipInfo(direct)
	} else {
		// Through the outbounds of the running connection, not its routes: with TUN, plain requests would go
		// into the tunnel, and a preset may send the services' sites either way.
		directClient, closeDirect := outboundClient(running.box, config.TagDirect)
		proxyClient, closeProxy := outboundClient(running.box, config.TagProxy)
		defer closeDirect()
		defer closeProxy()
		var direct, proxy netcheck.Info
		var servers []netcheck.Info
		var directErr, proxyErr, leakErr error
		var wg sync.WaitGroup
		wg.Add(3)
		go func() { defer wg.Done(); direct, directErr = netcheck.Lookup(ctx, directClient, netcheck.Services) }()
		go func() { defer wg.Done(); proxy, proxyErr = netcheck.Lookup(ctx, proxyClient, netcheck.Services) }()
		go func() {
			defer wg.Done()
			// Names resolve the way programs resolve them: with TUN the system's queries go into the tunnel,
			// with System alone they go to the provider, which is the leak the user should see.
			servers, leakErr = netcheck.Leak(ctx, proxyClient, netcheck.LeakService, func(ctx context.Context, host string) error {
				_, err := net.DefaultResolver.LookupHost(ctx, host)
				return err
			})
		}()
		wg.Wait()
		for _, part := range []struct {
			name string
			err  error
		}{{"without the VPN", directErr}, {"through the VPN", proxyErr}, {"DNS leak test", leakErr}} {
			if part.err != nil {
				errs = append(errs, fmt.Errorf("%s: %w", part.name, part.err))
			}
		}
		state.Direct, state.Proxy = ipInfo(direct), ipInfo(proxy)
		if leakErr == nil {
			state.DNS, state.Leak = dnsServers(servers, direct)
		}
	}
	state.CheckedAt = e.now().UnixMilli()
	if len(errs) > 0 {
		state.Error, state.ErrorText = ReasonCheckFailed, errors.Join(errs...).Error()
	}
	return state
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

// outboundClient sends requests through one outbound of a running connection; names resolve on its side.
func outboundClient(box *boxbox.Box, tag string) (*http.Client, func()) {
	outbound, found := box.Outbound().Outbound(tag)
	transport := &http.Transport{
		Proxy: nil,
		DialContext: func(ctx context.Context, _, address string) (net.Conn, error) {
			if !found {
				return nil, fmt.Errorf("the connection has no %s outbound", tag)
			}
			return outbound.DialContext(ctx, N.NetworkTCP, metadata.ParseSocksaddr(address))
		},
		ForceAttemptHTTP2: true,
	}
	return &http.Client{Transport: transport}, transport.CloseIdleConnections
}
