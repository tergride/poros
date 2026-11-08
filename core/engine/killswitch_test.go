package engine

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sync"
	"testing"
	"time"

	"PorosCore/config"
)

// fakeGuard keeps a block that is only a flag, the way the guard process keeps it for a window: a new engine on the
// same guard finds the block the previous one left.
type fakeGuard struct {
	mu       sync.Mutex
	rights   bool
	block    *fakeLink
	arms     int
	disarms  int
	whileRun bool
	engine   *Engine
}

type fakeLink struct {
	guard  *fakeGuard
	lost   chan struct{}
	once   sync.Once
	reason string
}

func (g *fakeGuard) Find() (GuardLink, error) {
	g.mu.Lock()
	defer g.mu.Unlock()
	if g.block == nil {
		return nil, nil
	}
	return g.block, nil
}

func (g *fakeGuard) Arm() (GuardLink, error) {
	g.mu.Lock()
	defer g.mu.Unlock()
	if g.block != nil {
		return g.block, nil
	}
	if !g.rights {
		return nil, errGuardRights
	}
	g.arms++
	// The block must be on before the connection: it is armed while nothing runs yet.
	if g.engine != nil && g.engine.conn.box.Load() != nil {
		g.whileRun = true
	}
	g.block = &fakeLink{guard: g, lost: make(chan struct{})}
	return g.block, nil
}

func (l *fakeLink) Lost() <-chan struct{} { return l.lost }
func (l *fakeLink) Reason() string        { return l.reason }
func (l *fakeLink) Close() error          { return nil }

func (l *fakeLink) Disarm() error {
	l.guard.mu.Lock()
	defer l.guard.mu.Unlock()
	l.guard.disarms++
	l.guard.block = nil
	l.once.Do(func() { close(l.lost) })
	return nil
}

// fail is the guard losing its rules: the block goes away on its own.
func (l *fakeLink) fail(reason string) {
	l.guard.mu.Lock()
	l.guard.block = nil
	l.reason = reason
	l.guard.mu.Unlock()
	l.once.Do(func() { close(l.lost) })
}

func (g *fakeGuard) on() bool {
	g.mu.Lock()
	defer g.mu.Unlock()
	return g.block != nil
}

func (g *fakeGuard) current() *fakeLink {
	g.mu.Lock()
	defer g.mu.Unlock()
	return g.block
}

func (g *fakeGuard) counts() (arms, disarms int) {
	g.mu.Lock()
	defer g.mu.Unlock()
	return g.arms, g.disarms
}

type ksTest struct {
	*Engine
	states chan ConnectionState
	blocks chan KillSwitchState
	close  func()
}

// openKillSwitchTest opens an engine with the guard and a server for its connection; the server is not there,
// which a start does not notice. A nil guard is a core without a kill switch.
func openKillSwitchTest(t *testing.T, g *fakeGuard, dir, mode string) ksTest {
	t.Helper()
	return openWindowTest(t, g, dir, mode, 0)
}

// openWindowTest opens the engine as a core of the Poros window with the PID window.
func openWindowTest(t *testing.T, g *fakeGuard, dir, mode string, window int) ksTest {
	t.Helper()
	k := ksTest{states: make(chan ConnectionState, 64), blocks: make(chan KillSwitchState, 64)}
	var guard Guard
	if g != nil {
		guard = g
	}
	e, err := Open(Config{DataDir: dir, MixedPort: freePort(t), Guard: guard, Window: window, Notify: func(method string, params any) {
		switch method {
		case EventConnectionChanged:
			k.states <- params.(ConnectionState)
		case EventKillSwitchChanged:
			k.blocks <- params.(KillSwitchState)
		}
	}})
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	k.Engine = e
	k.close = sync.OnceFunc(func() { e.Close() })
	t.Cleanup(k.close)
	e.presets = append(e.presets,
		presetRecord{ID: "through", Preset: config.Preset{Name: "Through", DefaultOutbound: config.OutboundProxy}},
		// split sends 127.0.0.1 directly and the rest through the server.
		presetRecord{ID: "split", Preset: config.Preset{Name: "Split", DefaultOutbound: config.OutboundProxy, Rules: []config.Rule{
			{Action: config.ActionRoute, Outbound: config.OutboundDirect, IPCIDR: []string{"127.0.0.1/32"}},
		}}})
	if g != nil {
		g.mu.Lock()
		g.engine = e
		g.mu.Unlock()
	}
	if settings, _ := e.Settings(); settings.Server == "" {
		useServer(t, e, fmt.Sprintf("trojan://secret@127.0.0.1:%d?security=none#local", freePort(t)))
	}
	setMode(t, e, mode)
	return k
}

func setMode(t *testing.T, e *Engine, mode string) {
	t.Helper()
	settings, _ := e.Settings()
	settings.KillSwitch = mode
	if _, err := e.SetSettings(settings); err != nil {
		t.Fatal(err)
	}
}

// waitBlock waits for the kill switch to say the block is on or off.
func waitBlock(t *testing.T, e *Engine, on bool) KillSwitchState {
	t.Helper()
	deadline := time.Now().Add(5 * time.Second)
	for {
		state := e.KillSwitchState()
		if state.On == on {
			return state
		}
		if time.Now().After(deadline) {
			t.Fatalf("the block did not turn on=%v: %+v", on, state)
		}
		time.Sleep(10 * time.Millisecond)
	}
}

func setSettings(t *testing.T, e *Engine, change func(*Settings)) {
	t.Helper()
	settings, _ := e.Settings()
	change(&settings)
	if _, err := e.SetSettings(settings); err != nil {
		t.Fatal(err)
	}
}

// refusing tells whether the refusal of the kill switch runs.
func refusing(e *Engine) bool {
	e.conn.lifecycle.Lock()
	defer e.conn.lifecycle.Unlock()
	return e.conn.running != nil && e.conn.running.refusing
}

// failReconnect makes the next start fail, as a lost connection does: the server is gone.
func failReconnect(t *testing.T, k ksTest) ConnectionState {
	t.Helper()
	setSettings(t, k.Engine, func(s *Settings) { s.Server = "" })
	k.StartConnection()
	return waitState(t, k.states)
}

func TestKillSwitchWithoutTunNeedsNoRights(t *testing.T) {
	g := &fakeGuard{}
	k := openKillSwitchTest(t, g, t.TempDir(), KillSwitchConnected)
	if k.KillSwitchState().On {
		t.Fatal("the connected mode holds nothing before a Start")
	}
	k.StartConnection()
	if state := waitState(t, k.states); state.State != ConnectionRunning || state.Error != "" {
		t.Fatalf("a connection without TUN needs no rights for the kill switch: %+v", state)
	}
	if state := k.KillSwitchState(); !state.On || state.Full || state.Error != "" || g.on() {
		t.Fatalf("the kill switch guards the connection without a block of Windows: %+v, block %v", state, g.on())
	}
	k.StopConnection()
	waitState(t, k.states)
	if state := k.KillSwitchState(); state != (KillSwitchState{}) || refusing(k.Engine) {
		t.Fatalf("Stop lets everything go: %+v", state)
	}
}

func TestKillSwitchRefusesTheServerAfterALostConnection(t *testing.T) {
	direct := webPage(t)
	k := openKillSwitchTest(t, &fakeGuard{}, t.TempDir(), KillSwitchConnected)
	setSettings(t, k.Engine, func(s *Settings) { s.Preset = "split" })
	k.StartConnection()
	waitState(t, k.states)

	state := failReconnect(t, k)
	if state.State != ConnectionIdle || state.Error != ReasonNoServer {
		t.Fatalf("the reconnect must fail: %+v", state)
	}
	if !refusing(k.Engine) || !k.KillSwitchState().On || k.KillSwitchState().Full {
		t.Fatalf("a lost connection brings the refusal up: %+v", k.KillSwitchState())
	}
	// The preset still works: its direct site opens, and what went through the server is refused.
	if body, err := fetchThrough(k.Engine, direct); err != nil || body != "through the server" {
		t.Errorf("the direct site of the preset: %q, %v", body, err)
	}
	if _, err := fetchThrough(k.Engine, "http://127.0.0.2:1/"); err == nil {
		t.Error("the traffic of the server went out")
	}

	k.StopConnection()
	waitState(t, k.states)
	if refusing(k.Engine) || k.KillSwitchState().On {
		t.Fatalf("Stop ends the refusal: %+v", k.KillSwitchState())
	}
	if _, err := fetchThrough(k.Engine, direct); err == nil {
		t.Error("the local proxy still answers after Stop")
	}
}

func TestKillSwitchFirstFailedStartHoldsNothing(t *testing.T) {
	k := openKillSwitchTest(t, &fakeGuard{}, t.TempDir(), KillSwitchConnected)
	failReconnect(t, k)
	if refusing(k.Engine) || k.KillSwitchState().On {
		t.Fatalf("a start that never ran has nothing to hold: %+v", k.KillSwitchState())
	}
}

func TestKillSwitchAlways(t *testing.T) {
	k := openKillSwitchTest(t, &fakeGuard{}, t.TempDir(), KillSwitchAlways)
	k.StartKillSwitch()
	if !refusing(k.Engine) || !k.KillSwitchState().On {
		t.Fatalf("the always mode refuses from the start: %+v", k.KillSwitchState())
	}
	k.StartConnection()
	if state := waitState(t, k.states); state.State != ConnectionRunning || refusing(k.Engine) {
		t.Fatalf("the connection replaces the refusal: %+v", state)
	}
	k.StopConnection()
	waitState(t, k.states)
	if !refusing(k.Engine) {
		t.Fatal("Stop brings the refusal back in the always mode")
	}

	// Connected without a lost connection holds nothing; always refuses again; off lets everything go.
	setMode(t, k.Engine, KillSwitchConnected)
	waitBlock(t, k.Engine, false)
	setMode(t, k.Engine, KillSwitchAlways)
	waitBlock(t, k.Engine, true)
	if !refusing(k.Engine) {
		t.Fatal("always without a connection refuses at once")
	}
	setMode(t, k.Engine, KillSwitchOff)
	waitBlock(t, k.Engine, false)
	if refusing(k.Engine) {
		t.Fatal("off left the refusal running")
	}
}

func TestKillSwitchModeWhileConnected(t *testing.T) {
	k := openKillSwitchTest(t, &fakeGuard{}, t.TempDir(), KillSwitchOff)
	k.StartConnection()
	waitState(t, k.states)
	if k.KillSwitchState().On {
		t.Fatal("the kill switch is off")
	}
	// The mode takes effect without a reconnect.
	setMode(t, k.Engine, KillSwitchConnected)
	waitBlock(t, k.Engine, true)
	setMode(t, k.Engine, KillSwitchOff)
	waitBlock(t, k.Engine, false)
	if k.ConnectionState().State != ConnectionRunning {
		t.Fatal("the connection must keep running")
	}
}

// Tests run without administrator rights, so the TUN never comes up: they see what a core that came back without
// rights sees.
func TestKillSwitchWithTunWithoutItsTun(t *testing.T) {
	g := &fakeGuard{rights: true}
	k := openKillSwitchTest(t, g, t.TempDir(), KillSwitchConnected)
	setSettings(t, k.Engine, func(s *Settings) { s.Tun = true })
	k.StartConnection()
	if state := waitState(t, k.states); state.Error != ReasonNeedsElevation {
		t.Fatalf("TUN without rights: %+v", state)
	}
	if g.on() || k.KillSwitchState().On {
		t.Fatal("a start that never ran left a block")
	}

	// Always with TUN holds the block of Windows while its TUN cannot come up: the whole internet waits.
	setMode(t, k.Engine, KillSwitchAlways)
	state := waitBlock(t, k.Engine, true)
	if !g.on() || !state.Full || !refusing(k.Engine) {
		t.Fatalf("always with TUN and no TUN: %+v, block %v", state, g.on())
	}
	// Without TUN in the settings the refusal comes again without the block.
	setSettings(t, k.Engine, func(s *Settings) { s.Tun = false })
	deadline := time.Now().Add(5 * time.Second)
	for (g.on() || k.KillSwitchState().Full) && time.Now().Before(deadline) {
		time.Sleep(10 * time.Millisecond)
	}
	if state := k.KillSwitchState(); g.on() || state.Full || !state.On || !refusing(k.Engine) {
		t.Fatalf("without TUN: %+v, block %v", state, g.on())
	}
}

func TestKillSwitchAlwaysWithTunAsksForRights(t *testing.T) {
	g := &fakeGuard{}
	k := openKillSwitchTest(t, g, t.TempDir(), KillSwitchOff)
	setSettings(t, k.Engine, func(s *Settings) { s.Tun = true })
	setMode(t, k.Engine, KillSwitchAlways)
	deadline := time.Now().Add(5 * time.Second)
	for !refusing(k.Engine) && time.Now().Before(deadline) {
		time.Sleep(10 * time.Millisecond)
	}
	state := k.KillSwitchState()
	if state.Error != ReasonKillSwitchNeedsElevation || !state.On || !refusing(k.Engine) {
		t.Fatalf("the refusal runs without its TUN and asks for rights: %+v", state)
	}

	unsupported := openKillSwitchTest(t, nil, t.TempDir(), KillSwitchOff)
	setSettings(t, unsupported.Engine, func(s *Settings) { s.Tun = true })
	setMode(t, unsupported.Engine, KillSwitchAlways)
	deadline = time.Now().Add(5 * time.Second)
	for unsupported.KillSwitchState().Error == "" && time.Now().Before(deadline) {
		time.Sleep(10 * time.Millisecond)
	}
	if state := unsupported.KillSwitchState(); state.Error != ReasonKillSwitchFailed {
		t.Errorf("without a guard the block fails: %+v", state)
	}
}

func TestKillSwitchGoesOnWithTheNextCoreOfTheWindow(t *testing.T) {
	g := &fakeGuard{rights: true}
	dir := t.TempDir()
	first := openWindowTest(t, g, dir, KillSwitchConnected, 42)
	first.StartConnection()
	waitState(t, first.states)
	// The shell replaces the core: what the kill switch held goes to the next one.
	first.HandOver()
	first.close()

	next := openWindowTest(t, g, dir, KillSwitchConnected, 42)
	next.StartKillSwitch()
	if !refusing(next.Engine) || !next.KillSwitchState().On {
		t.Fatalf("the next core of the window refuses: %+v", next.KillSwitchState())
	}
	// A core that exits for good takes its mark along.
	next.close()
	last := openWindowTest(t, g, dir, KillSwitchConnected, 42)
	last.StartKillSwitch()
	if refusing(last.Engine) || last.KillSwitchState().On {
		t.Fatalf("Exit let the kill switch go: %+v", last.KillSwitchState())
	}

	// The mark of another window is a Poros that is gone.
	last.StartConnection()
	waitState(t, last.states)
	last.HandOver()
	last.close()
	other := openWindowTest(t, g, dir, KillSwitchConnected, 43)
	other.StartKillSwitch()
	if refusing(other.Engine) {
		t.Fatal("a core of another window took over the refusal")
	}
	if _, err := os.Stat(filepath.Join(dir, markFile)); !errors.Is(err, os.ErrNotExist) {
		t.Errorf("the mark of another window must go: %v", err)
	}
}

func TestKillSwitchBlockComesBackAfterAFailure(t *testing.T) {
	g := &fakeGuard{rights: true}
	k := openKillSwitchTest(t, g, t.TempDir(), KillSwitchOff)
	setSettings(t, k.Engine, func(s *Settings) { s.Tun = true })
	setMode(t, k.Engine, KillSwitchAlways)
	waitBlock(t, k.Engine, true)
	for !g.on() {
		time.Sleep(10 * time.Millisecond)
	}

	g.current().fail("the rules were removed")
	failed := false
	timeout := time.After(5 * time.Second)
	for !failed || !g.on() {
		select {
		case state := <-k.blocks:
			failed = failed || state.Error == ReasonKillSwitchFailed && state.ErrorText == "the rules were removed"
		case <-timeout:
			t.Fatalf("the failure was not told, or the block did not come back: %+v", k.KillSwitchState())
		case <-time.After(10 * time.Millisecond):
		}
	}
	// A second failure soon after is told, and the block stays off: it would keep failing.
	g.current().fail("again")
	time.Sleep(100 * time.Millisecond)
	if state := k.KillSwitchState(); state.ErrorText != "again" || g.on() {
		t.Errorf("after a second failure: %+v, on %v", state, g.on())
	}
}
