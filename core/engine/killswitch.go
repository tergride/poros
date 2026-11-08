package engine

import (
	"errors"
	"log"
	"sync"
	"time"
)

// Guard keeps the block of the kill switch in a process of its own that lives as long as the Poros window: the
// block outlasts a core that fails or is replaced, and the next core of the window joins it.
type Guard interface {
	// Find joins the block that is on for this window; it is nil without one.
	Find() (GuardLink, error)
	// Arm turns the block on, or joins the one that is on; without administrator rights it fails with
	// errGuardRights.
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

// rearmInterval spaces the attempts to bring back a block that went away, as when the firewall service restarts.
const rearmInterval = 30 * time.Second

type killSwitch struct {
	// mu guards the fields below. Arming and disarming happen under the connection's lifecycle, one at a time;
	// only a block that went away is let go without it.
	mu      sync.Mutex
	link    GuardLink
	state   KillSwitchState
	closed  bool
	rearmed time.Time
}

func (e *Engine) KillSwitchState() KillSwitchState {
	e.ks.mu.Lock()
	defer e.ks.mu.Unlock()
	return e.ks.state
}

// setKillSwitch announces the state; the caller holds ks.mu, so that events leave in the order of the changes.
func (e *Engine) setKillSwitch(state KillSwitchState) {
	if state == e.ks.state {
		return
	}
	e.ks.state = state
	e.notify(EventKillSwitchChanged, state)
}

// armKillSwitch turns the block on, or keeps it on; its error is that of a failed start. The caller holds the
// lifecycle.
func (e *Engine) armKillSwitch() error {
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
	e.ks.mu.Lock()
	defer e.ks.mu.Unlock()
	if err != nil {
		reason := ReasonKillSwitchFailed
		if errors.Is(err, errGuardRights) {
			reason = ReasonKillSwitchNeedsElevation
		}
		log.Printf("kill switch: %v", err)
		e.setKillSwitch(KillSwitchState{Error: reason, ErrorText: err.Error()})
		return failure(reason, "%v", err)
	}
	e.holdLink(link)
	return nil
}

// holdLink makes link the block of this core and follows it; the caller holds ks.mu.
func (e *Engine) holdLink(link GuardLink) {
	log.Print("kill switch: the block is on")
	e.ks.link = link
	e.setKillSwitch(KillSwitchState{On: true})
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
	e.setKillSwitch(KillSwitchState{Error: ReasonKillSwitchFailed, ErrorText: reason})
	retry := e.ks.rearmed.IsZero() || e.now().Sub(e.ks.rearmed) >= rearmInterval
	if retry {
		e.ks.rearmed = e.now()
	}
	e.ks.mu.Unlock()
	log.Printf("kill switch: the block went away: %s", reason)
	if retry {
		e.rearmKillSwitch()
	}
}

func (e *Engine) rearmKillSwitch() {
	e.conn.lifecycle.Lock()
	defer e.conn.lifecycle.Unlock()
	settings, err := e.Settings()
	if err != nil || settings.KillSwitch == KillSwitchOff {
		return
	}
	_ = e.armKillSwitch()
}

// disarmKillSwitch turns the block off and clears an error. The caller holds the lifecycle.
func (e *Engine) disarmKillSwitch() {
	e.ks.mu.Lock()
	link := e.ks.link
	e.ks.link = nil
	e.ks.mu.Unlock()
	state := KillSwitchState{}
	if link != nil {
		log.Print("kill switch: turning the block off")
		if err := link.Disarm(); err != nil {
			log.Printf("kill switch: %v", err)
			state = KillSwitchState{Error: ReasonKillSwitchFailed, ErrorText: err.Error()}
		}
	}
	e.ks.mu.Lock()
	defer e.ks.mu.Unlock()
	e.setKillSwitch(state)
}

// StartKillSwitch joins the block a previous core of this window left on, or turns the block on when the setting
// is always. A block found with the setting off goes off.
func (e *Engine) StartKillSwitch() {
	e.conn.lifecycle.Lock()
	defer e.conn.lifecycle.Unlock()
	settings, err := e.Settings()
	if err != nil {
		return
	}
	if e.guard != nil {
		link, err := e.guard.Find()
		if err != nil {
			log.Printf("kill switch: %v", err)
		}
		if link != nil && settings.KillSwitch == KillSwitchOff {
			if err = link.Disarm(); err != nil {
				log.Printf("kill switch: %v", err)
			}
			return
		}
		if link != nil {
			e.ks.mu.Lock()
			defer e.ks.mu.Unlock()
			e.holdLink(link)
			return
		}
	}
	if settings.KillSwitch == KillSwitchAlways {
		_ = e.armKillSwitch()
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
	switch settings.KillSwitch {
	case KillSwitchOff:
		e.disarmKillSwitch()
	case KillSwitchAlways:
		_ = e.armKillSwitch()
	case KillSwitchConnected:
		if e.conn.running != nil {
			_ = e.armKillSwitch()
		} else if previous == KillSwitchAlways {
			e.disarmKillSwitch()
		}
	}
}

// afterFailedStart takes back a block that came with a Start that failed: no connection ran for it to guard. A
// block that guarded an earlier connection holds, as after any loss of the connection. The caller holds the
// lifecycle.
func (e *Engine) afterFailedStart(wasOn bool) {
	if wasOn || !e.KillSwitchState().On {
		return
	}
	if settings, err := e.Settings(); err == nil && settings.KillSwitch == KillSwitchConnected {
		e.disarmKillSwitch()
	}
}

// afterStop turns the block off with the connection, except in the always mode. The caller holds the lifecycle.
func (e *Engine) afterStop() {
	if settings, err := e.Settings(); err == nil && settings.KillSwitch != KillSwitchAlways {
		e.disarmKillSwitch()
	}
}

// closeKillSwitch lets go of the block as the core exits: it stays on until the window goes, or the next core
// of the window turns it off.
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
}
