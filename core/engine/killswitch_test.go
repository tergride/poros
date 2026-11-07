package engine

import (
	"fmt"
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
	k := ksTest{states: make(chan ConnectionState, 64), blocks: make(chan KillSwitchState, 64)}
	var guard Guard
	if g != nil {
		guard = g
	}
	e, err := Open(Config{DataDir: dir, MixedPort: freePort(t), Guard: guard, Notify: func(method string, params any) {
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
	e.presets = append(e.presets, presetRecord{ID: "through", Preset: config.Preset{Name: "Through", DefaultOutbound: config.OutboundProxy}})
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

func TestKillSwitchWhileConnected(t *testing.T) {
	g := &fakeGuard{rights: true}
	k := openKillSwitchTest(t, g, t.TempDir(), KillSwitchConnected)
	if g.on() {
		t.Fatal("the connected mode blocks nothing before a Start")
	}

	k.StartConnection()
	if state := waitState(t, k.states); state.State != ConnectionRunning {
		t.Fatalf("start: %+v", state)
	}
	if !g.on() || !k.KillSwitchState().On || g.whileRun {
		t.Fatalf("the block must be on, and on before the connection: %+v, armed while running %v", k.KillSwitchState(), g.whileRun)
	}
	// A reconnect keeps the block: nothing goes out between the two connections.
	k.StartConnection()
	waitState(t, k.states)
	if arms, disarms := g.counts(); arms != 1 || disarms != 0 {
		t.Errorf("a reconnect armed %d times, disarmed %d times", arms, disarms)
	}

	k.StopConnection()
	waitState(t, k.states)
	if g.on() || k.KillSwitchState() != (KillSwitchState{}) {
		t.Fatalf("Stop turns the block off: %+v", k.KillSwitchState())
	}

	// A Start that fails after its block came takes the block back: no connection ran for it to guard.
	k.conn.lifecycle.Lock()
	if err := k.armKillSwitch(); err != nil {
		t.Fatal(err)
	}
	k.afterFailedStart(false)
	k.conn.lifecycle.Unlock()
	if g.on() {
		t.Error("a failed start left its block on")
	}
}

func TestKillSwitchHoldsForALostConnection(t *testing.T) {
	g := &fakeGuard{rights: true}
	dir := t.TempDir()
	first := openKillSwitchTest(t, g, dir, KillSwitchConnected)
	first.StartConnection()
	waitState(t, first.states)
	// The core goes away while connected, as in a crash: the block stays with its guard.
	first.close()
	if !g.on() {
		t.Fatal("the block went with the core")
	}

	next := openKillSwitchTest(t, g, dir, KillSwitchConnected)
	next.StartKillSwitch()
	if state := next.KillSwitchState(); !state.On || next.ConnectionState().State != ConnectionIdle {
		t.Fatalf("the next core holds the block it found: %+v", state)
	}
	// A reconnect that fails keeps a block that guarded a connection.
	settings, _ := next.Settings()
	settings.Server = ""
	next.SetSettings(settings)
	next.StartConnection()
	waitState(t, next.states)
	if !g.on() {
		t.Fatal("a failed reconnect let the internet through")
	}
	next.StopConnection()
	waitState(t, next.states)
	if g.on() || next.KillSwitchState().On {
		t.Fatal("Stop turns the found block off")
	}
	if arms, _ := g.counts(); arms != 1 {
		t.Errorf("the block was armed %d times", arms)
	}
}

func TestKillSwitchAlways(t *testing.T) {
	g := &fakeGuard{rights: true}
	dir := t.TempDir()
	k := openKillSwitchTest(t, g, dir, KillSwitchAlways)
	waitBlock(t, k.Engine, true)
	k.StartConnection()
	waitState(t, k.states)
	k.StopConnection()
	waitState(t, k.states)
	if !g.on() {
		t.Fatal("Stop keeps the block in the always mode")
	}

	// Connected without a connection blocks nothing; off turns any block off.
	setMode(t, k.Engine, KillSwitchConnected)
	waitBlock(t, k.Engine, false)
	setMode(t, k.Engine, KillSwitchAlways)
	waitBlock(t, k.Engine, true)
	setMode(t, k.Engine, KillSwitchOff)
	waitBlock(t, k.Engine, false)
	if g.on() {
		t.Fatal("off left the block on")
	}

	// The next core joins a block that is on, and one that starts without a block turns it on.
	setMode(t, k.Engine, KillSwitchAlways)
	waitBlock(t, k.Engine, true)
	k.close()
	next := openKillSwitchTest(t, g, dir, KillSwitchAlways)
	next.StartKillSwitch()
	if !next.KillSwitchState().On {
		t.Fatal("the next core did not join the block")
	}
	next.close()
	g.current().Disarm()
	last := openKillSwitchTest(t, g, dir, KillSwitchAlways)
	last.StartKillSwitch()
	if !last.KillSwitchState().On {
		t.Fatal("a core in the always mode did not turn the block on at its start")
	}
	if arms, _ := g.counts(); arms != 4 {
		t.Errorf("armed %d times, want 4: a block that is on is joined, not armed again", arms)
	}
}

func TestKillSwitchConnectedWhileRunning(t *testing.T) {
	g := &fakeGuard{rights: true}
	k := openKillSwitchTest(t, g, t.TempDir(), KillSwitchOff)
	k.StartConnection()
	waitState(t, k.states)
	// The mode takes effect without a reconnect.
	setMode(t, k.Engine, KillSwitchConnected)
	waitBlock(t, k.Engine, true)
	setMode(t, k.Engine, KillSwitchOff)
	waitBlock(t, k.Engine, false)
}

func TestKillSwitchWithoutRights(t *testing.T) {
	g := &fakeGuard{}
	k := openKillSwitchTest(t, g, t.TempDir(), KillSwitchConnected)
	k.StartConnection()
	if state := waitState(t, k.states); state.State != ConnectionIdle || state.Error != ReasonKillSwitchNeedsElevation {
		t.Fatalf("no connection without the block: %+v", state)
	}
	if state := k.KillSwitchState(); state.On || state.Error != ReasonKillSwitchNeedsElevation {
		t.Fatalf("the kill switch tells why it is off: %+v", state)
	}
	k.StopConnection()
	waitState(t, k.states)
	if state := k.KillSwitchState(); state.Error != "" {
		t.Errorf("Stop takes the request back: %+v", state)
	}

	setMode(t, k.Engine, KillSwitchAlways)
	deadline := time.Now().Add(5 * time.Second)
	for k.KillSwitchState().Error != ReasonKillSwitchNeedsElevation && time.Now().Before(deadline) {
		time.Sleep(10 * time.Millisecond)
	}
	if state := k.KillSwitchState(); state.Error != ReasonKillSwitchNeedsElevation {
		t.Fatalf("the always mode asks for rights at once: %+v", state)
	}

	unsupported := openKillSwitchTest(t, nil, t.TempDir(), KillSwitchConnected)
	unsupported.StartConnection()
	if state := waitState(t, unsupported.states); state.Error != ReasonKillSwitchFailed {
		t.Errorf("without a guard the kill switch fails the start: %+v", state)
	}
}

func TestKillSwitchComesBackAfterAFailure(t *testing.T) {
	g := &fakeGuard{rights: true}
	k := openKillSwitchTest(t, g, t.TempDir(), KillSwitchAlways)
	waitBlock(t, k.Engine, true)

	g.current().fail("the rules were removed")
	failed := false
	timeout := time.After(5 * time.Second)
	for !failed || !k.KillSwitchState().On {
		select {
		case state := <-k.blocks:
			failed = failed || state.Error == ReasonKillSwitchFailed && state.ErrorText == "the rules were removed"
		case <-timeout:
			t.Fatalf("the failure was not told, or the block did not come back: %+v", k.KillSwitchState())
		}
	}
	// A second failure soon after is told, and the block stays off: it would keep failing.
	g.current().fail("again")
	state := waitBlock(t, k.Engine, false)
	time.Sleep(100 * time.Millisecond)
	if state.ErrorText != "again" || g.on() {
		t.Errorf("after a second failure: %+v, on %v", state, g.on())
	}
}
