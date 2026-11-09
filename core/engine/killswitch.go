package engine

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log"
	"net"
	"os"
	"path/filepath"
	"strconv"
	"sync"
	"sync/atomic"
	"time"

	"PorosCore/config"
	"PorosCore/internal/boxmain"
	"PorosCore/internal/marker"
)

// The kill switch keeps the traffic of the server from going out directly: after a lost connection, and in the
// always mode while there is none, a refusal runs instead, the connection of the preset with a refusal in place of
// the server, so that the TUN, the system proxy and the local proxy stay up and the rest of the preset still
// works. Without the TUN it needs no rights. With the TUN a block of Windows covers the moments without Poros's
// TUN: a reconnect, a core that failed, a core that came back without rights.

// Guard keeps the block of Windows in a process of its own that lives as long as the Poros window: the block
// outlasts a core that fails or is replaced, and the next core of the window joins it.
type Guard interface {
	// Find joins the block that is on for this window; it is nil without one.
	Find() (GuardLink, error)
	// Arm turns the block on, or joins the one that is on; without administrator rights it fails with
	// errGuardRights, and with the block of another Poros on the computer with errGuardTaken.
	Arm() (GuardLink, error)
}

// GuardLink is the core's line to a block that is on.
type GuardLink interface {
	// Lost is closed when the block goes away; Reason then says why, "" when it was turned off.
	Lost() <-chan struct{}
	Reason() string
	// Disarm turns the block off.
	Disarm() error
	// Close lets go of the block and leaves it on.
	Close() error
}

var errGuardRights = errors.New("the kill switch needs administrator rights")

// errGuardTaken is a block of Windows that another Poros on the computer holds: two blocks would shut out each other's
// Poros.
var errGuardTaken = errors.New("another Poros on this computer, as of another Windows user, holds a block of Windows")

// rearmInterval spaces the attempts to bring back a block that went away, as when the firewall service restarts.
const rearmInterval = 30 * time.Second

// markFile tells the next core of the same window that the kill switch held traffic: a core that failed or was
// replaced leaves it, and the next one brings the refusal back instead of giving the system proxy back.
const markFile = "kill-switch.json"

type killSwitch struct {
	// mu guards the fields below. Arming and disarming happen under the connection's lifecycle, one at a time;
	// only a block that went away is let go without it.
	mu   sync.Mutex
	link GuardLink
	// guarding is a connection or a refusal that runs under the kill switch; tun, that it has Poros's TUN up.
	guarding, tun bool
	// failure is why the kill switch does not hold what its mode asks for, with the details in English.
	failure, failureText string
	state                KillSwitchState
	closed               bool
	rearmed              time.Time
	// window is the PID of the Poros window, 0 without one; resume is the mark of this window found at the open.
	window int
	resume bool
	// handover is a core that is being replaced: it leaves the system proxy and the mark to the next one.
	handover atomic.Bool
}

func (e *Engine) KillSwitchState() KillSwitchState {
	e.ks.mu.Lock()
	defer e.ks.mu.Unlock()
	return e.ks.state
}

// announceKillSwitch tells what the kill switch holds; the caller holds ks.mu, so that events leave in the order of
// the changes.
func (e *Engine) announceKillSwitch() {
	state := KillSwitchState{
		On:        e.ks.guarding || e.ks.link != nil,
		Full:      e.ks.link != nil && !e.ks.tun,
		Error:     e.ks.failure,
		ErrorText: e.ks.failureText,
	}
	if state == e.ks.state {
		return
	}
	e.ks.state = state
	e.notify(EventKillSwitchChanged, state)
}

// setGuarding records what runs under the kill switch and keeps the mark for the next core of the window. The
// caller holds the lifecycle.
func (e *Engine) setGuarding(on, tun bool) {
	e.ks.mu.Lock()
	defer e.ks.mu.Unlock()
	if on != e.ks.guarding && e.ks.window != 0 {
		e.writeMark(on)
	}
	e.ks.guarding, e.ks.tun = on, tun
	e.announceKillSwitch()
}

// clearFailure forgets why the kill switch failed before, as a new attempt begins.
func (e *Engine) clearFailure() {
	e.ks.mu.Lock()
	defer e.ks.mu.Unlock()
	e.ks.failure, e.ks.failureText = "", ""
	e.announceKillSwitch()
}

// failKillSwitch tells why the kill switch does not hold what its mode asks for.
func (e *Engine) failKillSwitch(reason string, err error) {
	log.Printf("kill switch: %v", err)
	e.ks.mu.Lock()
	defer e.ks.mu.Unlock()
	e.ks.failure, e.ks.failureText = reason, err.Error()
	e.announceKillSwitch()
}

func (e *Engine) writeMark(on bool) {
	path := filepath.Join(e.dataDir, markFile)
	if !on {
		if err := os.Remove(path); err != nil && !errors.Is(err, os.ErrNotExist) {
			log.Printf("kill switch: %v", err)
		}
		return
	}
	data, _ := json.Marshal(map[string]int{"window": e.ks.window})
	if err := os.WriteFile(path, data, 0o600); err != nil {
		log.Printf("kill switch: %v", err)
	}
}

// readMark tells whether the mark names this window; one of another window, a Poros that is gone, is removed.
func (e *Engine) readMark() bool {
	path := filepath.Join(e.dataDir, markFile)
	data, err := os.ReadFile(path)
	if err != nil {
		return false
	}
	var mark struct {
		Window int `json:"window"`
	}
	if json.Unmarshal(data, &mark) == nil && mark.Window != 0 && mark.Window == e.ks.window {
		return true
	}
	_ = os.Remove(path)
	return false
}

// armBlock turns the block of Windows on, or keeps it on; its error is that of a failed start. The caller holds
// the lifecycle.
func (e *Engine) armBlock() error {
	e.ks.mu.Lock()
	on, closed := e.ks.link != nil, e.ks.closed
	e.ks.mu.Unlock()
	if on {
		return nil
	}
	var link GuardLink
	err := errors.New("the kill switch works on Windows only")
	switch {
	case closed:
		err = errors.New("the core is closing")
	case e.guard != nil:
		link, err = e.guard.Arm()
	}
	if err != nil {
		reason := ReasonKillSwitchFailed
		switch {
		case errors.Is(err, errGuardRights):
			reason = ReasonKillSwitchNeedsElevation
		case errors.Is(err, errGuardTaken):
			reason = ReasonKillSwitchTaken
		}
		e.failKillSwitch(reason, err)
		return failure(reason, "%v", err)
	}
	e.ks.mu.Lock()
	defer e.ks.mu.Unlock()
	e.holdLink(link)
	return nil
}

// holdLink makes link the block of this core and follows it; the caller holds ks.mu.
func (e *Engine) holdLink(link GuardLink) {
	log.Print("kill switch: the block of Windows is on")
	e.ks.link = link
	e.ks.failure, e.ks.failureText = "", ""
	e.announceKillSwitch()
	go e.watchLink(link)
}

// watchLink follows a block until it goes away. One that was turned off or let go is no longer the core's link;
// one that failed comes back, unless it keeps failing.
func (e *Engine) watchLink(link GuardLink) {
	<-link.Lost()
	e.ks.mu.Lock()
	if e.ks.link != link {
		e.ks.mu.Unlock()
		return
	}
	e.ks.link = nil
	reason := link.Reason()
	if reason == "" {
		reason = "the kill switch process stopped"
	}
	e.ks.failure, e.ks.failureText = ReasonKillSwitchFailed, reason
	e.announceKillSwitch()
	retry := e.ks.rearmed.IsZero() || e.now().Sub(e.ks.rearmed) >= rearmInterval
	if retry {
		e.ks.rearmed = e.now()
	}
	e.ks.mu.Unlock()
	log.Printf("kill switch: the block went away: %s", reason)
	if retry {
		e.rearmBlock()
	}
}

func (e *Engine) rearmBlock() {
	e.conn.lifecycle.Lock()
	defer e.conn.lifecycle.Unlock()
	settings, err := e.Settings()
	if err != nil || settings.KillSwitch == KillSwitchOff || !settings.Tun || e.conn.running == nil {
		return
	}
	_ = e.armBlock()
}

// disarmBlock turns the block of Windows off. The caller holds the lifecycle.
func (e *Engine) disarmBlock() {
	e.ks.mu.Lock()
	link := e.ks.link
	e.ks.link = nil
	e.ks.mu.Unlock()
	if link == nil {
		return
	}
	log.Print("kill switch: turning the block of Windows off")
	err := link.Disarm()
	if err != nil {
		log.Printf("kill switch: %v", err)
	}
	e.ks.mu.Lock()
	defer e.ks.mu.Unlock()
	if err != nil {
		e.ks.failure, e.ks.failureText = ReasonKillSwitchFailed, err.Error()
	}
	e.announceKillSwitch()
}

// releaseKillSwitch lets everything go: the connection or refusal, the system proxy, the block, the failure. The
// caller holds the lifecycle.
func (e *Engine) releaseKillSwitch() error {
	err := e.stopRunning(false)
	e.disarmBlock()
	e.clearFailure()
	e.setGuarding(false, false)
	return err
}

// StartKillSwitch brings back what the previous core of this window held, the block it left on or the refusal its
// mark tells of, and starts the refusal in the always mode. A block found with the setting off goes off.
func (e *Engine) StartKillSwitch() {
	e.conn.lifecycle.Lock()
	defer e.conn.lifecycle.Unlock()
	settings, err := e.Settings()
	if err != nil {
		return
	}
	var link GuardLink
	if e.guard != nil {
		if link, err = e.guard.Find(); err != nil {
			log.Printf("kill switch: %v", err)
		}
	}
	if settings.KillSwitch == KillSwitchOff {
		if link != nil {
			if err = link.Disarm(); err != nil {
				log.Printf("kill switch: %v", err)
			}
		}
		return
	}
	if link != nil {
		e.ks.mu.Lock()
		e.holdLink(link)
		e.ks.mu.Unlock()
	}
	if settings.KillSwitch == KillSwitchAlways || e.ks.resume || link != nil {
		if e.ks.resume || link != nil {
			log.Print("kill switch: the previous core of this window held the traffic of the server")
		}
		e.startRefusal(settings)
	}
}

// syncKillSwitch applies a new mode at once, without a reconnect: connected guards a running connection, always
// the whole run of Poros.
func (e *Engine) syncKillSwitch(previous string) {
	e.conn.lifecycle.Lock()
	defer e.conn.lifecycle.Unlock()
	settings, err := e.Settings()
	if err != nil {
		return
	}
	running := e.conn.running
	switch {
	case settings.KillSwitch == KillSwitchOff:
		if running != nil && running.refusing {
			_ = e.releaseKillSwitch()
			return
		}
		e.disarmBlock()
		e.clearFailure()
		e.setGuarding(false, false)
	case running == nil && settings.KillSwitch == KillSwitchAlways:
		e.startRefusal(settings)
	case running == nil:
	case running.refusing && previous == KillSwitchAlways && settings.KillSwitch == KillSwitchConnected:
		// Without a lost connection there is nothing for the connected mode to hold.
		_ = e.releaseKillSwitch()
	default:
		if running.tun {
			_ = e.armBlock()
		}
		e.setGuarding(true, running.tun)
	}
}

// afterFailedStart decides what a failed start leaves. A kill switch that held traffic before, or holds it always,
// brings the refusal up; otherwise no connection ran for it to guard, and everything goes. A cancelled start is
// left to the request that cancelled it. The caller holds the lifecycle.
func (e *Engine) afterFailedStart(held, cancelled bool) {
	if cancelled {
		return
	}
	settings, err := e.Settings()
	if err == nil && (settings.KillSwitch == KillSwitchAlways || held && settings.KillSwitch == KillSwitchConnected) {
		e.startRefusal(settings)
		return
	}
	_ = e.releaseKillSwitch()
}

// startRefusal brings up the refusal of the kill switch on the TUN, the system proxy and the local proxy the
// settings ask for. The TUN needs rights: without them the block of Windows, if there is one, holds alone. The
// caller holds the lifecycle and nothing runs.
func (e *Engine) startRefusal(settings Settings) {
	e.ks.mu.Lock()
	closed := e.ks.closed
	e.ks.mu.Unlock()
	if closed {
		return
	}
	e.clearFailure()
	preset, found, err := e.preset(settings.Preset)
	if err != nil || !found {
		preset = config.Preset{Name: "Refusal", DefaultOutbound: config.OutboundProxy}
	}
	tun := false
	if settings.Tun {
		// The block holds while the TUN comes up, and alone if it cannot. The TUN of another Poros on the computer
		// leaves the refusal without one.
		_ = e.armBlock()
		tun = elevated() && !marker.Taken(marker.Tun) && prepareTun(context.Background())
	}
	listen := "127.0.0.1"
	if settings.AllowLAN {
		listen = "0.0.0.0"
	}
	if !portFree(net.JoinHostPort(listen, strconv.Itoa(int(e.mixedPort)))) {
		e.refusalFailed(settings, portBusy(e.mixedPort))
		return
	}
	configs, err := config.Build(config.Input{
		Refuse:      true,
		Preset:      preset,
		Tun:         tun,
		AllowLAN:    settings.AllowLAN,
		DisableIPv6: !settings.IPv6,
		MixedPort:   e.mixedPort,
		LogLevel:    singBoxLogLevel(settings.DebugLog),
	})
	var singBox []byte
	if err == nil {
		singBox, err = configs.SingBoxJSON()
	}
	running := &runningConnection{tun: tun, refusing: true}
	if err == nil {
		running.marks, err = holdMarks(e.mixedPort, tun)
	}
	if err == nil {
		if running.box, running.cancel, err = boxmain.Create(singBox, nil); err != nil {
			running.marks.release()
		}
	}
	if err != nil {
		e.refusalFailed(settings, fmt.Errorf("sing-box: %w", err))
		return
	}
	running.stopTraffic = func() {}
	e.conn.running = running
	log.Printf("kill switch: refusing the traffic of the server, TUN %t, System %t", tun, settings.SystemProxy)
	e.setGuarding(true, tun)
	if !settings.Tun {
		e.disarmBlock()
	}
	e.syncProxyOf(settings)
}

// restartRefusal brings a running refusal up again with the settings: no one is asked to apply them, as for a
// connection.
func (e *Engine) restartRefusal() {
	e.conn.lifecycle.Lock()
	defer e.conn.lifecycle.Unlock()
	settings, err := e.Settings()
	if err != nil || e.conn.running == nil || !e.conn.running.refusing {
		return
	}
	_ = e.stopRunning(true)
	e.startRefusal(settings)
}

// refusalFailed leaves what still holds: the block of Windows with the TUN on. A system proxy would point at a
// port that is not Poros's, so it goes. The caller holds the lifecycle.
func (e *Engine) refusalFailed(settings Settings, err error) {
	if clearErr := e.sysProxy.Clear(); clearErr != nil {
		log.Printf("kill switch: system proxy: %v", clearErr)
	}
	e.failKillSwitch(ReasonKillSwitchFailed, fmt.Errorf("the refusal did not start: %w", err))
	e.ks.mu.Lock()
	link := e.ks.link != nil
	e.ks.mu.Unlock()
	e.setGuarding(link && settings.Tun, false)
}

// closeKillSwitch lets go of the block as the core exits: it stays on until the window goes, or the next core
// of the window turns it off. The mark stays only for a core that takes over.
func (e *Engine) closeKillSwitch() {
	e.conn.lifecycle.Lock()
	defer e.conn.lifecycle.Unlock()
	e.ks.mu.Lock()
	defer e.ks.mu.Unlock()
	e.ks.closed = true
	if e.ks.link != nil {
		e.ks.link.Close()
		e.ks.link = nil
	}
	if !e.ks.handover.Load() && e.ks.window != 0 {
		e.writeMark(false)
	}
}

// HandOver tells the core that another core of the window replaces it: it leaves the system proxy and the mark of
// the kill switch as they are, so that nothing goes out directly in between.
func (e *Engine) HandOver() {
	log.Print("handing over to the next core")
	e.ks.handover.Store(true)
}
