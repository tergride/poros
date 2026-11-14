package engine

import (
	"context"
	"errors"
	"fmt"
	"log"
	"maps"
	"math/rand/v2"
	"net"
	"net/netip"
	"slices"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/sagernet/bbolt"
	"github.com/sagernet/sing-box/common/trafficcontrol"
	"github.com/sagernet/sing/common/control"
	"github.com/xtls/xray-core/core"

	"PorosCore/config"
	"PorosCore/internal/boxbox"
	"PorosCore/internal/boxmain"
	"PorosCore/internal/marker"
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
	// tracker follows its connections while the statistics are on, for lists that must not wait for a start either.
	tracker atomic.Pointer[trafficcontrol.Manager]
	// onXray tells that Xray runs the server of the connection.
	onXray atomic.Bool
	// history counts the traffic of the connection for the history while it is kept.
	history atomic.Pointer[historyTracker]
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
	// marks tell another Poros on the computer that the port of the local proxy, and the TUN, are taken.
	marks marks
	// port is that of the local proxy, systemPort that of the system proxy while the local one asks for a
	// password, else 0.
	port, systemPort uint16
}

// systemProxyPort is the port the system proxy of Windows points at.
func (r *runningConnection) systemProxyPort() uint16 {
	if r.systemPort != 0 {
		return r.systemPort
	}
	return r.port
}

type marks []*marker.Marker

func (m marks) release() {
	for _, held := range m {
		held.Release()
	}
}

// holdMarks takes the markers of what a sing-box holds for the whole computer: the port of the local proxy and the TUN.
func holdMarks(port uint16, tun bool) (marks, error) {
	held, err := marker.Hold(marker.Port(port))
	if errors.Is(err, marker.ErrTaken) {
		return nil, failure(ReasonPortPoros, "port %d is taken by another Poros on this computer", port)
	}
	if err != nil {
		return nil, err
	}
	m := marks{held}
	if tun {
		if held, err = marker.Hold(marker.Tun); err != nil {
			m.release()
			if errors.Is(err, marker.ErrTaken) {
				return nil, tunTaken()
			}
			return nil, err
		}
		m = append(m, held)
	}
	return m, nil
}

func tunTaken() error {
	return failure(ReasonTunTaken, "another Poros on this computer, as of another Windows user, has its TUN on")
}

// portBusy tells another Poros from another program on the port of the local proxy.
func portBusy(port uint16) error {
	if marker.Taken(marker.Port(port)) {
		return failure(ReasonPortPoros, "port %d is taken by another Poros on this computer", port)
	}
	return failure(ReasonPortBusy, "port %d is taken by another program", port)
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
	endpoints, endpointNames, err := e.presetEndpoints(preset, server, settings.Server)
	if err != nil {
		return ConnectionState{}, err
	}
	// Before the rights: they would not help, and the prompt for them would come for nothing. This core's own TUN is
	// already down: the earlier connection stopped before this start.
	if settings.Tun && marker.Taken(marker.Tun) {
		return ConnectionState{}, tunTaken()
	}
	if settings.Tun && !elevated() {
		return ConnectionState{}, failure(ReasonNeedsElevation, "TUN needs administrator rights")
	}
	tune := tuningOf(settings)
	entry := config.Entry(server, links)
	if !elevated() && tune.Spoofs(entry, settings.RealityOnXray) {
		return ConnectionState{}, failure(ReasonSpoofNeedsElevation, "a forged TLS handshake needs administrator rights")
	}
	onXray := config.RunsOnXray(entry, settings.RealityOnXray)
	log.Printf("connection: starting %s on %s, TUN %t, System %t, kill switch %s, IPv6 %t, preset %s, %d endpoints",
		server.Type, coreName(onXray), settings.Tun, settings.SystemProxy, settings.KillSwitch, settings.IPv6, settings.Preset, len(endpoints))
	listen := "127.0.0.1"
	if settings.AllowLAN {
		listen = "0.0.0.0"
	}
	port := e.proxyPort(settings)
	if !portFree(net.JoinHostPort(listen, strconv.Itoa(int(port)))) {
		return ConnectionState{}, portBusy(port)
	}
	systemPort, err := e.systemPortOf(settings)
	if err != nil {
		return ConnectionState{}, err
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

	// The codes of OTP go in last: a HOTP code spent on a start that fails before would be lost.
	signIns := []profile.Profile{server}
	if err = e.fillOTP(signIns, links, endpoints); err != nil {
		return ConnectionState{}, err
	}
	server = signIns[0]

	in := config.Input{
		Server:        server,
		Links:         links,
		Preset:        withDefaultDNS(preset, settings),
		DNS:           dnsOf(settings),
		WARP:          warpOf(settings),
		Endpoints:     endpoints,
		Tun:           settings.Tun,
		TunOptions:    tunOf(settings),
		AllowLAN:      settings.AllowLAN,
		Auth:          proxyAuthOf(settings),
		SystemPort:    systemPort,
		DisableIPv6:   !settings.IPv6,
		MixedPort:     port,
		RealityOnXray: settings.RealityOnXray,
		LogLevel:      singBoxLogLevel(settings.DebugLog),
		FindProcess:   settings.Stats,
		Tuning:        tune,
		NTP:           ntpOf(settings),
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
	// With the TUN sing-box dials out of the network adapter, past the TUN of another client, whose DNS may answer the
	// servers' names with fake addresses that lead only into that TUN.
	if settings.Tun {
		resolveCtx, cancel := context.WithTimeout(ctx, resolveTimeout)
		pinned := pastFakes(resolveCtx, e.systemResolver(), config.ServerHosts(entry, endpoints, settings.RealityOnXray), !settings.IPv6)
		cancel()
		if ctx.Err() != nil {
			return ConnectionState{}, ctx.Err()
		}
		if len(pinned) > 0 {
			log.Printf("connection: the system's DNS has no real address for %d servers, DoH found them", len(pinned))
			if in.Resolved == nil {
				in.Resolved = map[string]netip.Addr{}
			}
			maps.Copy(in.Resolved, pinned)
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
	running := &runningConnection{iface: in.Interface, tun: settings.Tun, port: port, systemPort: systemPort}
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
	if running.marks, err = holdMarks(port, settings.Tun); err != nil {
		if running.xray != nil {
			running.xray.Close()
		}
		return ConnectionState{}, err
	}
	// The counters join before the box starts, so that they see the first connection too. Without the statistics
	// there are none: no speed, no traffic of the server and no list of connections.
	counter := &trafficCounter{}
	var tracker *trafficcontrol.Manager
	var history *historyTracker
	trackTraffic := func(box *boxbox.Box) {
		if settings.Stats {
			tracker = trafficcontrol.NewManager()
			box.Router().AppendTracker(counter)
			box.Router().AppendTracker(tracker)
		}
		if historyOn(settings) {
			history = newHistoryTracker(settings.Server)
			box.Router().AppendTracker(history)
		}
	}
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
		running.marks.release()
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
	e.conn.tracker.Store(tracker)
	e.conn.history.Store(history)
	e.conn.onXray.Store(configs.Xray != nil)
	e.setGuarding(guarded, settings.Tun)
	if !settings.Tun {
		// A block for the TUN of before is no longer needed: without the TUN the kill switch is the system proxy.
		e.disarmBlock()
	}
	trafficCtx, stopTraffic := context.WithCancel(context.Background())
	running.stopTraffic = stopTraffic
	if settings.Stats {
		go e.reportTraffic(trafficCtx, counter, settings.Server)
	}
	if history != nil {
		go e.keepHistory(trafficCtx, history)
	}
	go e.pingConnection(trafficCtx, running.box, settings.Server)

	// OpenVPN and OpenConnect refuse connections until their handshake is done: the connection is up only then.
	if err = probe.AwaitTunnels(ctx, running.box, config.TagProxy); err != nil && ctx.Err() == nil {
		_ = e.stopRunning(true)
		return ConnectionState{}, failure(ReasonStartFailed, "%v", err)
	}
	// So do the endpoints of the preset, and the start fails with the one that did not come up.
	for i, name := range endpointNames {
		if err = probe.AwaitTunnels(ctx, running.box, config.EndpointTag(i)); err != nil && ctx.Err() == nil {
			_ = e.stopRunning(true)
			return ConnectionState{}, failure(ReasonEndpointFailed, "%s: %v", name, err)
		}
	}
	if ctx.Err() != nil {
		_ = e.stopRunning(true)
		return ConnectionState{}, ctx.Err()
	}
	state := ConnectionState{State: ConnectionRunning, ServerID: settings.Server, Tun: settings.Tun, Servers: append(e.serversThrough(settings.Server), preset.Endpoints...),
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
	e.conn.tracker.Store(nil)
	running.stopTraffic()
	e.recordHistory(e.conn.history.Swap(nil))
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
	// Even a box that has not closed lets go: the next start of this core must not take its own marks for another
	// Poros's, and a TUN still there is waited for as an earlier one.
	running.marks.release()
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
	port := e.mixedPort
	if running := e.conn.running; running != nil {
		port = running.systemProxyPort()
	}
	return e.sysProxy.Apply("127.0.0.1", port)
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

// presetEndpoints gives the profiles of the endpoints of a preset and their names. One whose profile is gone or of
// another type now fails the start, and so does one that is the server of the connection or a server of its chain.
func (e *Engine) presetEndpoints(preset config.Preset, server profile.Profile, serverID string) ([]profile.Profile, []string, error) {
	var profiles []profile.Profile
	var names []string
	for _, id := range preset.Endpoints {
		p, found, err := e.serverProfile(id)
		if err != nil {
			return nil, nil, err
		}
		if !found {
			return nil, nil, failure(ReasonEndpointFailed, "a profile of the preset's endpoints is gone")
		}
		name := serverRecord{Profile: p}.view("").Name
		switch {
		case p.Type != profile.TypeOpenVPN && p.Type != profile.TypeOpenConnect:
			return nil, nil, failure(ReasonEndpointFailed, "%s is an endpoint of the preset and no OpenVPN or OpenConnect profile now", name)
		case id == serverID || (server.Chain != nil && slices.Contains(server.Chain.Servers, id)):
			return nil, nil, failure(ReasonEndpointFailed, "%s is the server of the connection and an endpoint of its preset at once", name)
		}
		profiles, names = append(profiles, p), append(names, name)
	}
	return profiles, names, nil
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

// The ports of the bridge to Xray: below the ranges outgoing connections take their ports from (49152 and up on
// Windows, 32768 and up on Linux), so that another core's connection cannot take the port between this check and
// Xray's start. They are handed out in turn, from a random place: a test of many servers takes hundreds at once.
const (
	bridgePortLow  = 20000
	bridgePortHigh = 32000
)

var nextBridgePort = func() *atomic.Uint32 {
	var next atomic.Uint32
	next.Store(rand.Uint32N(bridgePortHigh - bridgePortLow))
	return &next
}()

func newBridge() (config.Bridge, error) {
	for range 50 {
		port := bridgePortLow + int(nextBridgePort.Add(1)%(bridgePortHigh-bridgePortLow))
		listener, err := net.Listen("tcp", net.JoinHostPort("127.0.0.1", strconv.Itoa(port)))
		if err != nil {
			continue
		}
		listener.Close()
		return config.Bridge{Port: uint16(port), Username: newID(), Password: newID() + newID()}, nil
	}
	return config.Bridge{}, errors.New("no free port for the bridge to Xray")
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
