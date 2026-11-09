package engine

import (
	"context"
	"errors"
	"fmt"
	"log"
	"net"
	"net/netip"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/sagernet/bbolt"
	"github.com/sagernet/sing/common/control"
	"github.com/xtls/xray-core/core"

	"PorosCore/config"
	"PorosCore/internal/boxbox"
	"PorosCore/internal/boxmain"
	"PorosCore/internal/netmon"
	"PorosCore/internal/probe"
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
	resolveTimeout = 10 * time.Second
)

// singBoxLogLevel keeps the log to warnings and errors; the debug log adds every connection.
func singBoxLogLevel(debug bool) string {
	if debug {
		return "info"
	}
	return "warn"
}

type connection struct {
	// mu guards state, serial and cancelStart. A newer start or a stop raises serial and cancels the start that
	// runs, and a request that is no longer the last one leaves the state to the one that is.
	mu          sync.Mutex
	state       ConnectionState
	serial      uint64
	cancelStart context.CancelFunc
	// lifecycle runs one start or stop at a time and guards running.
	lifecycle sync.Mutex
	running   *runningConnection
	// box is the running sing-box, for downloads through the connection that must not wait for a start.
	box atomic.Pointer[boxbox.Box]
}

type runningConnection struct {
	box    *boxbox.Box
	cancel context.CancelFunc
	xray   *core.Instance
	// iface is the interface Xray's sockets are bound to.
	iface string
	tun   bool
	// refusing is the refusal of the kill switch, which has no server.
	refusing bool
	// stopTraffic ends the speed and delay reports.
	stopTraffic context.CancelFunc
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
	serial := e.nextRequest()
	ctx, cancel := context.WithCancel(context.Background())
	e.conn.cancelStart = cancel
	e.setState(ConnectionState{State: ConnectionStarting})
	state := e.conn.state
	e.conn.mu.Unlock()
	go e.runStart(ctx, serial)
	return state, nil
}

// nextRequest makes a request the last one and cancels a start that runs; the caller holds conn.mu.
func (e *Engine) nextRequest() uint64 {
	e.conn.serial++
	if e.conn.cancelStart != nil {
		e.conn.cancelStart()
		e.conn.cancelStart = nil
	}
	return e.conn.serial
}

// StopConnection disconnects in the background; a start that runs is cancelled where it can be.
func (e *Engine) StopConnection() (ConnectionState, error) {
	e.conn.mu.Lock()
	serial := e.nextRequest()
	active := e.conn.state.State != ConnectionIdle
	if active {
		state := e.conn.state
		state.State = ConnectionStopping
		e.setState(state)
	}
	state := e.conn.state
	e.conn.mu.Unlock()
	// A Stop with nothing up changes no address: no need to ask the services again.
	go e.runStop(serial, active || e.KillSwitchState().On)
	return state, nil
}

func (e *Engine) runStart(ctx context.Context, serial uint64) {
	e.conn.lifecycle.Lock()
	defer e.conn.lifecycle.Unlock()
	if !e.isLatest(serial) {
		return
	}
	// The kill switch keeps the system proxy through a reconnect: in between, programs are refused, not let out.
	held := e.KillSwitchState().On
	settings, _ := e.Settings()
	stopErr := e.stopRunning(settings.KillSwitch == KillSwitchConnected || settings.KillSwitch == KillSwitchAlways)
	began := time.Now()
	state, err := e.startRunning(ctx)
	if err == nil && stopErr != nil {
		state.Error, state.ErrorText = ReasonStopTimeout, stopErr.Error()
	}
	took := time.Since(began).Round(time.Millisecond)
	switch {
	case err == nil:
		log.Printf("connection: running in %s", took)
	case ctx.Err() != nil:
		log.Printf("connection: the start was cancelled after %s", took)
	default:
		log.Printf("connection: the start failed after %s: %v", took, err)
	}
	if err != nil {
		state = ConnectionState{State: ConnectionIdle, Error: ReasonStartFailed, ErrorText: err.Error()}
		var failed connError
		if errors.As(err, &failed) {
			state.Error = failed.reason
		}
		e.afterFailedStart(held, ctx.Err() != nil)
	}
	e.conn.mu.Lock()
	defer e.conn.mu.Unlock()
	// A newer request is waiting for the lifecycle and will set the state; this result is already out of date.
	if serial == e.conn.serial {
		e.setState(state)
		go e.requestNetworkCheck()
	}
}

func (e *Engine) runStop(serial uint64, changesNetwork bool) {
	e.conn.lifecycle.Lock()
	defer e.conn.lifecycle.Unlock()
	if !e.isLatest(serial) {
		return
	}
	state := ConnectionState{State: ConnectionIdle}
	// In the always mode the refusal takes the connection's place; otherwise everything goes.
	var err error
	if settings, _ := e.Settings(); settings.KillSwitch == KillSwitchAlways {
		err = e.stopRunning(true)
		e.startRefusal(settings)
	} else {
		err = e.releaseKillSwitch()
	}
	if err != nil {
		state.Error, state.ErrorText = ReasonStopTimeout, err.Error()
		log.Printf("connection: stop: %v", err)
	}
	e.conn.mu.Lock()
	defer e.conn.mu.Unlock()
	if serial == e.conn.serial {
		e.setState(state)
		if changesNetwork {
			go e.requestNetworkCheck()
		}
	}
}

func (e *Engine) isLatest(serial uint64) bool {
	e.conn.mu.Lock()
	defer e.conn.mu.Unlock()
	return serial == e.conn.serial
}

// startRunning brings a connection up; the caller holds the lifecycle and nothing runs. Cancelling ctx stops it at
// the next step: sing-box opening the TUN cannot be interrupted, so what it opened is closed again.
func (e *Engine) startRunning(ctx context.Context) (ConnectionState, error) {
	settings, err := e.Settings()
	if err != nil {
		return ConnectionState{}, err
	}
	if settings.Server == "" {
		return ConnectionState{}, failure(ReasonNoServer, "no server is chosen")
	}
	server, links, found, err := e.serverWithLinks(settings.Server)
	var missing missingLink
	if errors.As(err, &missing) {
		return ConnectionState{}, failure(ReasonNoServer, "%v", err)
	}
	if err != nil {
		return ConnectionState{}, err
	}
	if !found {
		return ConnectionState{}, failure(ReasonNoServer, "the chosen server is no longer in its subscription")
	}
	preset, found, err := e.preset(settings.Preset)
	if err != nil {
		return ConnectionState{}, err
	}
	if !found {
		return ConnectionState{}, failure(ReasonStartFailed, "the preset %q is gone", settings.Preset)
	}
	if settings.Tun && !elevated() {
		return ConnectionState{}, failure(ReasonNeedsElevation, "TUN needs administrator rights")
	}
	entry := config.Entry(server, links)
	onXray := config.RunsOnXray(entry, settings.RealityOnXray)
	log.Printf("connection: starting %s on %s, TUN %t, System %t, kill switch %s, IPv6 %t, preset %s",
		server.Type, coreName(onXray), settings.Tun, settings.SystemProxy, settings.KillSwitch, settings.IPv6, settings.Preset)
	listen := "127.0.0.1"
	if settings.AllowLAN {
		listen = "0.0.0.0"
	}
	if !portFree(net.JoinHostPort(listen, strconv.Itoa(int(e.mixedPort)))) {
		return ConnectionState{}, failure(ReasonPortBusy, "port %d is taken by another program", e.mixedPort)
	}
	e.clearFailure()
	if settings.Tun && !waitTunGone(ctx, tunGoneTimeout) {
		if ctx.Err() != nil {
			return ConnectionState{}, ctx.Err()
		}
		return ConnectionState{}, failure(ReasonTunBusy, "Windows still holds the TUN adapter of an earlier start; try again, or restart Windows if it stays")
	}
	if settings.Tun {
		if removed, err := removeStaleTun(); err != nil {
			log.Printf("connection: the TUN adapter an earlier run left behind could not be removed: %v", err)
		} else if removed {
			log.Print("connection: removed the TUN adapter an earlier run left behind")
		}
	}
	// The block of Windows comes first: nothing may go out past the TUN while it comes up.
	guarded := settings.KillSwitch != KillSwitchOff
	if guarded && settings.Tun {
		if err = e.armBlock(); err != nil {
			return ConnectionState{}, err
		}
	}

	in := config.Input{
		Server:        server,
		Links:         links,
		Preset:        preset,
		Tun:           settings.Tun,
		AllowLAN:      settings.AllowLAN,
		DisableIPv6:   !settings.IPv6,
		MixedPort:     e.mixedPort,
		RealityOnXray: settings.RealityOnXray,
		LogLevel:      singBoxLogLevel(settings.DebugLog),
	}
	if onXray {
		if in.Bridge, err = newBridge(); err != nil {
			return ConnectionState{}, err
		}
		// Only the TUN would catch Xray's own connections; without it they go out as the system routes them.
		if iface := netmon.DefaultInterface(); iface != nil && settings.Tun {
			in.Interface = iface.Name
		}
		resolveCtx, cancel := context.WithTimeout(ctx, resolveTimeout)
		in.Resolved, err = resolveHosts(resolveCtx, e.systemResolver(), config.XrayHosts(entry, settings.RealityOnXray), !settings.IPv6)
		cancel()
		if ctx.Err() != nil {
			return ConnectionState{}, ctx.Err()
		}
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

	if ctx.Err() != nil {
		return ConnectionState{}, ctx.Err()
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
	counter := &trafficCounter{}
	// The counter joins before the box starts, so that it sees the first connection too.
	trackTraffic := func(box *boxbox.Box) { box.Router().AppendTracker(counter) }
	running.box, running.cancel, err = boxmain.Create(singBox, trackTraffic)
	if err != nil && settings.Tun && tunFailed(err) && ctx.Err() == nil {
		// Windows may have stalled on the adapter, as on bringing back one left behind: Wintun removes it on the
		// failure, and a second try makes a new one.
		log.Printf("connection: the TUN did not open, trying once more: %v", err)
		if waitTunGone(ctx, tunGoneTimeout) {
			running.box, running.cancel, err = boxmain.Create(singBox, trackTraffic)
		}
	}
	if err != nil {
		if running.xray != nil {
			running.xray.Close()
		}
		if ctx.Err() != nil {
			return ConnectionState{}, ctx.Err()
		}
		return ConnectionState{}, failure(ReasonStartFailed, "sing-box: %v", err)
	}
	e.conn.running = running
	e.conn.box.Store(running.box)
	e.setGuarding(guarded, settings.Tun)
	if !settings.Tun {
		// A block for the TUN of before is no longer needed: without the TUN the kill switch is the system proxy.
		e.disarmBlock()
	}
	trafficCtx, stopTraffic := context.WithCancel(context.Background())
	running.stopTraffic = stopTraffic
	go e.reportTraffic(trafficCtx, counter, settings.Server)
	go e.pingConnection(trafficCtx, running.box, settings.Server)

	// OpenVPN and OpenConnect refuse connections until their handshake is done: the connection is up only then.
	if err = probe.AwaitTunnels(ctx, running.box, config.TagProxy); err != nil && ctx.Err() == nil {
		_ = e.stopRunning(true)
		return ConnectionState{}, failure(ReasonStartFailed, "%v", err)
	}
	if ctx.Err() != nil {
		_ = e.stopRunning(true)
		return ConnectionState{}, ctx.Err()
	}
	state := ConnectionState{State: ConnectionRunning, ServerID: settings.Server, Tun: settings.Tun, Servers: e.serversThrough(settings.Server),
		Since: e.now().UnixMilli()}
	if err = e.syncProxyOf(settings); err != nil {
		state.Error, state.ErrorText = ReasonSystemProxyFailed, err.Error()
		log.Printf("connection: system proxy: %v", err)
	}
	state.SystemProxy = settings.SystemProxy && err == nil
	return state, nil
}

// syncProxyOf points the system proxy at Poros when the settings ask for it and gives it back otherwise. Apply may
// set some connections before failing on others, and Clear still sees them. The caller holds the lifecycle.
func (e *Engine) syncProxyOf(settings Settings) error {
	if settings.SystemProxy {
		return e.applySystemProxy()
	}
	return e.sysProxy.Clear()
}

// stopRunning brings the connection or the refusal down, the system proxy first: nothing may lead to a closed
// port, unless keepProxy asks for it, as the kill switch does while the next one comes. The caller holds the
// lifecycle.
func (e *Engine) stopRunning(keepProxy bool) error {
	var errs []error
	if !keepProxy {
		errs = append(errs, e.sysProxy.Clear())
	}
	running := e.conn.running
	if running == nil {
		return errors.Join(errs...)
	}
	e.conn.running = nil
	e.conn.box.Store(nil)
	running.stopTraffic()
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

// syncSystemProxy sets or clears the system proxy of a running connection or refusal after the setting changed.
func (e *Engine) syncSystemProxy() {
	e.conn.lifecycle.Lock()
	defer e.conn.lifecycle.Unlock()
	running := e.conn.running
	settings, err := e.Settings()
	if running == nil || err != nil || settings.SystemProxy == e.sysProxy.Active() {
		return
	}
	err = e.syncProxyOf(settings)
	if running.refusing {
		if err != nil {
			log.Printf("kill switch: system proxy: %v", err)
		}
		return
	}
	state := e.ConnectionState()
	state.Error, state.ErrorText = "", ""
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
	// Without a network the addresses found before are no longer the user's: they go until it comes back.
	if iface == nil {
		e.clearNetworkCheck()
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
func waitTunGone(ctx context.Context, timeout time.Duration) bool {
	deadline := time.Now().Add(timeout)
	for {
		if _, err := net.InterfaceByName(config.TunName); err != nil {
			return true
		}
		if time.Now().After(deadline) {
			return false
		}
		select {
		case <-ctx.Done():
			return false
		case <-time.After(250 * time.Millisecond):
		}
	}
}

// prepareTun waits for the TUN of an earlier start to go and removes an adapter an earlier run left behind; false
// is a TUN that cannot come up now.
func prepareTun(ctx context.Context) bool {
	if !waitTunGone(ctx, tunGoneTimeout) {
		return false
	}
	if removed, err := removeStaleTun(); err != nil {
		log.Printf("connection: the TUN adapter an earlier run left behind could not be removed: %v", err)
	} else if removed {
		log.Print("connection: removed the TUN adapter an earlier run left behind")
	}
	return true
}

// tunFailed tells a start that failed on the TUN, in the words of sing-box's tun inbound.
func tunFailed(err error) bool {
	text := strings.ToLower(err.Error())
	return strings.Contains(text, "tun interface") || strings.Contains(text, "tun stack")
}

func coreName(onXray bool) string {
	if onXray {
		return "Xray"
	}
	return "sing-box"
}

func (e *Engine) closeConnection() error {
	e.conn.mu.Lock()
	e.nextRequest()
	e.conn.mu.Unlock()
	e.conn.lifecycle.Lock()
	defer e.conn.lifecycle.Unlock()
	return e.stopRunning(e.ks.handover.Load())
}

func (e *Engine) serverProfile(id string) (p profile.Profile, found bool, err error) {
	err = e.db.View(func(tx *bbolt.Tx) error {
		place, err := findServer(tx, id)
		if place.found() {
			p, found = place.servers[place.index].Profile, true
		}
		return err
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
func resolveHosts(ctx context.Context, resolver *net.Resolver, hosts []string, ipv4Only bool) (map[string]netip.Addr, error) {
	resolved := make(map[string]netip.Addr, len(hosts))
	for _, host := range hosts {
		addr, err := lookupPast(ctx, resolver, host, ipv4Only)
		if err != nil {
			return nil, err
		}
		resolved[host] = addr
	}
	return resolved, nil
}
