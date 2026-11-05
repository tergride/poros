package engine

import (
	"context"
	"errors"
	"fmt"
	"net"
	"net/netip"
	"strconv"
	"sync"
	"time"

	"github.com/sagernet/bbolt"
	"github.com/sagernet/sing/common/control"
	"github.com/xtls/xray-core/core"

	"PorosCore/config"
	"PorosCore/internal/boxbox"
	"PorosCore/internal/boxmain"
	"PorosCore/internal/netmon"
	"PorosCore/internal/xray"
	"PorosCore/profile"

	// Xray's protocols and transports register themselves.
	_ "PorosCore/internal/distro/all"
)

// DefaultMixedPort is the port of the local proxy; Throne, which may run alongside, keeps 2080.
const DefaultMixedPort = 2090

const (
	// stopTimeout bounds the close of a connection; a TUN that does not let go in this time needs the core to exit.
	stopTimeout = 10 * time.Second
	// resolveTimeout bounds the lookup of the addresses Xray dials.
	resolveTimeout  = 10 * time.Second
	singBoxLogLevel = "warn"
)

type connection struct {
	// mu guards state and serial. A newer start or a stop raises serial, and a request that is no longer the last
	// one leaves the state to the one that is.
	mu     sync.Mutex
	state  ConnectionState
	serial uint64
	// lifecycle runs one start or stop at a time and guards running.
	lifecycle sync.Mutex
	running   *runningConnection
}

type runningConnection struct {
	box    *boxbox.Box
	cancel context.CancelFunc
	xray   *core.Instance
	// iface is the interface Xray's sockets are bound to.
	iface       string
	tun         bool
	systemProxy bool
}

// connError is a failed start: a reason for the interface and the details.
type connError struct {
	reason string
	text   string
}

func (e connError) Error() string { return e.text }

func failure(reason, format string, args ...any) error {
	return connError{reason: reason, text: fmt.Sprintf(format, args...)}
}

func (e *Engine) ConnectionState() ConnectionState {
	e.conn.mu.Lock()
	defer e.conn.mu.Unlock()
	return e.conn.state
}

// setState announces a state; the caller holds conn.mu, so that events leave in the order of the changes.
func (e *Engine) setState(state ConnectionState) {
	e.conn.state = state
	e.notify(EventConnectionChanged, state)
}

// StartConnection connects with the current settings in the background; a running connection is replaced.
func (e *Engine) StartConnection() (ConnectionState, error) {
	e.conn.mu.Lock()
	e.conn.serial++
	serial := e.conn.serial
	e.setState(ConnectionState{State: ConnectionStarting})
	state := e.conn.state
	e.conn.mu.Unlock()
	go e.runStart(serial)
	return state, nil
}

// StopConnection disconnects in the background.
func (e *Engine) StopConnection() (ConnectionState, error) {
	e.conn.mu.Lock()
	e.conn.serial++
	serial := e.conn.serial
	if e.conn.state.State != ConnectionIdle {
		state := e.conn.state
		state.State = ConnectionStopping
		e.setState(state)
	}
	state := e.conn.state
	e.conn.mu.Unlock()
	go e.runStop(serial)
	return state, nil
}

func (e *Engine) runStart(serial uint64) {
	e.conn.lifecycle.Lock()
	defer e.conn.lifecycle.Unlock()
	if !e.isLatest(serial) {
		return
	}
	stopErr := e.stopRunning()
	state, err := e.startRunning()
	if err == nil && stopErr != nil {
		state.Error, state.ErrorText = ReasonStopTimeout, stopErr.Error()
	}
	if err != nil {
		state = ConnectionState{State: ConnectionIdle, Error: ReasonStartFailed, ErrorText: err.Error()}
		var failed connError
		if errors.As(err, &failed) {
			state.Error = failed.reason
		}
	}
	e.conn.mu.Lock()
	defer e.conn.mu.Unlock()
	// A newer request is waiting for the lifecycle and will set the state; this result is already out of date.
	if serial == e.conn.serial {
		e.setState(state)
		go e.requestNetworkCheck()
	}
}

func (e *Engine) runStop(serial uint64) {
	e.conn.lifecycle.Lock()
	defer e.conn.lifecycle.Unlock()
	if !e.isLatest(serial) {
		return
	}
	state := ConnectionState{State: ConnectionIdle}
	if err := e.stopRunning(); err != nil {
		state.Error, state.ErrorText = ReasonStopTimeout, err.Error()
	}
	e.conn.mu.Lock()
	defer e.conn.mu.Unlock()
	if serial == e.conn.serial {
		e.setState(state)
		go e.requestNetworkCheck()
	}
}

func (e *Engine) isLatest(serial uint64) bool {
	e.conn.mu.Lock()
	defer e.conn.mu.Unlock()
	return serial == e.conn.serial
}

// startRunning brings a connection up; the caller holds the lifecycle and nothing runs.
func (e *Engine) startRunning() (ConnectionState, error) {
	settings, err := e.Settings()
	if err != nil {
		return ConnectionState{}, err
	}
	if settings.Server == "" {
		return ConnectionState{}, failure(ReasonNoServer, "no server is chosen")
	}
	server, found, err := e.serverProfile(settings.Server)
	if err != nil {
		return ConnectionState{}, err
	}
	if !found {
		return ConnectionState{}, failure(ReasonNoServer, "the chosen server is no longer in its subscription")
	}
	var preset config.Preset
	for _, record := range e.presets {
		if record.ID == settings.Preset {
			preset = record.Preset
		}
	}
	if settings.Tun && !elevated() {
		return ConnectionState{}, failure(ReasonNeedsElevation, "TUN needs administrator rights")
	}
	listen := "127.0.0.1"
	if settings.AllowLAN {
		listen = "0.0.0.0"
	}
	if !portFree(net.JoinHostPort(listen, strconv.Itoa(int(e.mixedPort)))) {
		return ConnectionState{}, failure(ReasonPortBusy, "port %d is taken by another program", e.mixedPort)
	}
	if settings.Tun && !waitTunGone(tunGoneTimeout) {
		return ConnectionState{}, failure(ReasonTunBusy, "Windows still holds the TUN adapter of an earlier start; try again, or restart Windows if it stays")
	}

	in := config.Input{
		Server:        server,
		Preset:        preset,
		Tun:           settings.Tun,
		AllowLAN:      settings.AllowLAN,
		DisableIPv6:   !settings.IPv6,
		MixedPort:     e.mixedPort,
		RealityOnXray: settings.RealityOnXray,
		LogLevel:      singBoxLogLevel,
	}
	onXray := config.RunsOnXray(server, settings.RealityOnXray)
	if onXray {
		if in.Bridge, err = newBridge(); err != nil {
			return ConnectionState{}, err
		}
		// Only the TUN would catch Xray's own connections; without it they go out as the system routes them.
		if iface := netmon.DefaultInterface(); iface != nil && settings.Tun {
			in.Interface = iface.Name
		}
		ctx, cancel := context.WithTimeout(context.Background(), resolveTimeout)
		in.Resolved, err = resolveHosts(ctx, config.XrayHosts(server, settings.RealityOnXray), !settings.IPv6)
		cancel()
		if err != nil {
			return ConnectionState{}, failure(ReasonStartFailed, "%v", err)
		}
	}
	configs, err := config.Build(in)
	if errors.Is(err, profile.ErrUnsupported) {
		return ConnectionState{}, failure(ReasonServerUnsupported, "%v", err)
	}
	if err != nil {
		return ConnectionState{}, failure(ReasonStartFailed, "%v", err)
	}
	singBox, err := configs.SingBoxJSON()
	if err != nil {
		return ConnectionState{}, err
	}

	running := &runningConnection{iface: in.Interface, tun: settings.Tun}
	// Xray listens first: sing-box dials its bridge.
	if configs.Xray != nil {
		if running.xray, err = xray.CreateXrayInstance(string(configs.Xray)); err != nil {
			return ConnectionState{}, failure(ReasonStartFailed, "Xray: %v", err)
		}
		if err = running.xray.Start(); err != nil {
			running.xray.Close()
			return ConnectionState{}, failure(ReasonStartFailed, "Xray: %v", err)
		}
	}
	if running.box, running.cancel, err = boxmain.Create(singBox, nil); err != nil {
		if running.xray != nil {
			running.xray.Close()
		}
		return ConnectionState{}, failure(ReasonStartFailed, "sing-box: %v", err)
	}
	e.conn.running = running

	state := ConnectionState{State: ConnectionRunning, ServerID: settings.Server, Tun: settings.Tun, Since: e.now().UnixMilli()}
	if settings.SystemProxy {
		if err = e.applySystemProxy(); err != nil {
			state.Error, state.ErrorText = ReasonSystemProxyFailed, err.Error()
		}
		// Apply may have set some connections before failing on others: Clear must still see them.
		running.systemProxy = true
		state.SystemProxy = err == nil
	}
	return state, nil
}

// stopRunning brings the connection down, the system proxy first: nothing may lead to a closed port. The caller
// holds the lifecycle.
func (e *Engine) stopRunning() error {
	running := e.conn.running
	if running == nil {
		return nil
	}
	e.conn.running = nil
	var errs []error
	if running.systemProxy {
		errs = append(errs, e.sysProxy.Clear())
	}
	done := make(chan struct{})
	go func() {
		running.cancel()
		running.box.Close()
		if running.xray != nil {
			running.xray.Close()
		}
		close(done)
	}()
	select {
	case <-done:
	case <-time.After(stopTimeout):
		errs = append(errs, errors.New("the connection did not close in time"))
	}
	return errors.Join(errs...)
}

// syncSystemProxy sets or clears the system proxy of a running connection after the setting changed.
func (e *Engine) syncSystemProxy() {
	e.conn.lifecycle.Lock()
	defer e.conn.lifecycle.Unlock()
	running := e.conn.running
	settings, err := e.Settings()
	if running == nil || err != nil || settings.SystemProxy == running.systemProxy {
		return
	}
	state := e.ConnectionState()
	state.Error, state.ErrorText = "", ""
	if settings.SystemProxy {
		err = e.applySystemProxy()
		running.systemProxy = true
	} else {
		err = e.sysProxy.Clear()
		running.systemProxy = false
	}
	state.SystemProxy = settings.SystemProxy && err == nil
	if err != nil {
		state.Error, state.ErrorText = ReasonSystemProxyFailed, err.Error()
	}
	e.conn.mu.Lock()
	defer e.conn.mu.Unlock()
	if e.conn.state.State == ConnectionRunning {
		e.setState(state)
	}
}

// onDefaultInterface reconnects a connection on Xray when the network changes: its sockets are bound to the old
// interface. sing-box follows the network on its own.
func (e *Engine) onDefaultInterface(iface *control.Interface, _ int) {
	if iface == nil {
		return
	}
	// The monitor waits for its callbacks, and a start may hold the lifecycle for long.
	go func() {
		e.conn.lifecycle.Lock()
		running := e.conn.running
		changed := running != nil && running.iface != "" && running.iface != iface.Name
		e.conn.lifecycle.Unlock()
		if changed && e.ConnectionState().State == ConnectionRunning {
			e.StartConnection()
			return
		}
		e.requestNetworkCheck()
	}()
}

func (e *Engine) applySystemProxy() error {
	if e.foreignUser {
		return errors.New("the core runs as another account than Poros: its system proxy would be that account's")
	}
	return e.sysProxy.Apply("127.0.0.1", e.mixedPort)
}

// tunGoneTimeout covers Windows removing the adapter of a core that was killed with the TUN up.
const tunGoneTimeout = 10 * time.Second

// waitTunGone waits until no adapter has the name of Poros's TUN: sing-box would otherwise take over an adapter
// that is about to disappear, with its routes still pointing into it.
func waitTunGone(timeout time.Duration) bool {
	deadline := time.Now().Add(timeout)
	for {
		if _, err := net.InterfaceByName(config.TunName); err != nil {
			return true
		}
		if time.Now().After(deadline) {
			return false
		}
		time.Sleep(250 * time.Millisecond)
	}
}

func (e *Engine) closeConnection() error {
	e.conn.mu.Lock()
	e.conn.serial++
	e.conn.mu.Unlock()
	e.conn.lifecycle.Lock()
	defer e.conn.lifecycle.Unlock()
	return e.stopRunning()
}

func (e *Engine) serverProfile(id string) (p profile.Profile, found bool, err error) {
	err = e.db.View(func(tx *bbolt.Tx) error {
		subs, err := records(tx)
		if err != nil {
			return err
		}
		for _, sub := range subs {
			var list []serverRecord
			if _, err = getJSON(tx, bucketServers, []byte(sub.ID), &list); err != nil {
				return err
			}
			for _, r := range list {
				if r.ID == id {
					p, found = r.Profile, true
					return nil
				}
			}
		}
		return nil
	})
	return p, found, err
}

func portFree(address string) bool {
	listener, err := net.Listen("tcp", address)
	if err != nil {
		return false
	}
	listener.Close()
	return true
}

func newBridge() (config.Bridge, error) {
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		return config.Bridge{}, err
	}
	port := listener.Addr().(*net.TCPAddr).Port
	listener.Close()
	return config.Bridge{Port: uint16(port), Username: newID(), Password: newID() + newID()}, nil
}

// resolveHosts looks up the addresses Xray dials, preferring IPv4.
func resolveHosts(ctx context.Context, hosts []string, ipv4Only bool) (map[string]netip.Addr, error) {
	network := "ip"
	if ipv4Only {
		network = "ip4"
	}
	resolved := make(map[string]netip.Addr, len(hosts))
	for _, host := range hosts {
		addrs, err := net.DefaultResolver.LookupNetIP(ctx, network, host)
		if err != nil || len(addrs) == 0 {
			return nil, fmt.Errorf("cannot resolve %s: %v", host, err)
		}
		resolved[host] = addrs[0].Unmap()
		for _, addr := range addrs {
			if addr.Unmap().Is4() {
				resolved[host] = addr.Unmap()
				break
			}
		}
	}
	return resolved, nil
}
