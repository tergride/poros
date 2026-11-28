package engine

import (
	"context"
	"errors"
	"log"
	"regexp"
	"slices"
	"strings"

	"github.com/sagernet/bbolt"
)

// The cleanup removes what this copy of Poros left in Windows outside its folder, so that deleting the folder
// removes Poros whole. The folder itself, the program and its data, stays as it is. Each kind of trace finds and
// removes itself; those of other copies of Poros and of other programs stay.

// trace is one kind of what Poros leaves outside its folder.
type trace struct {
	kind string
	// rights is a trace only a core with administrator rights removes.
	rights bool
	// profile is a trace in the user's profile, which a core with the rights of another account does not see.
	profile bool
	// find tells how many there are.
	find   func() (int, error)
	remove func() error
}

// ScanCleanup finds the traces of this copy outside its folder.
func (e *Engine) ScanCleanup() CleanupReport {
	return CleanupReport{Items: scanTraces(e.traces(), e.foreignUser)}
}

// RunCleanup turns off what would bring the traces back at the next start, the start with Windows, stops the connection and what the kill switch holds, removes the traces it may and tells what stays.
func (e *Engine) RunCleanup() (CleanupReport, error) {
	if err := e.turnOffStart(); err != nil {
		return CleanupReport{}, err
	}
	tun, err := e.releaseAll()
	if err != nil {
		log.Printf("cleanup: stop the connection: %v", err)
	}
	// Windows forgets a network only once its adapter is gone.
	if tun && !waitTunGone(context.Background(), tunGoneTimeout) {
		log.Print("cleanup: the TUN adapter is still there")
	}
	rights := elevated()
	items := removeTraces(e.traces(), rights, e.foreignUser)
	// A core with rights sees the networks themselves: none stays, so the mark of a TUN that ran goes.
	if rights && !slices.ContainsFunc(items, func(item CleanupItem) bool { return item.Kind == CleanupNetworks }) {
		e.forgetTun()
	}
	log.Printf("cleanup: done, %d kinds of traces stay", len(items))
	return CleanupReport{Items: items}, nil
}

// turnOffStart turns the start with Windows off.
func (e *Engine) turnOffStart() error {
	e.writeMu.Lock()
	defer e.writeMu.Unlock()
	settings, err := e.Settings()
	if err != nil || !settings.Autostart {
		return err
	}
	settings.Autostart = false
	_, err = e.setSettings(settings)
	return err
}

// releaseAll stops the connection or the refusal and lets go of what the kill switch holds, in any of its modes,
// and waits for it. It tells whether a TUN ran.
func (e *Engine) releaseAll() (bool, error) {
	e.conn.mu.Lock()
	serial := e.nextRequest()
	e.conn.mu.Unlock()
	e.conn.lifecycle.Lock()
	defer e.conn.lifecycle.Unlock()
	tun := e.conn.running != nil && e.conn.running.tun
	err := e.releaseKillSwitch()
	e.conn.mu.Lock()
	if serial == e.conn.serial && e.conn.state.State != ConnectionIdle {
		e.setState(ConnectionState{State: ConnectionIdle})
	}
	e.conn.mu.Unlock()
	return tun, err
}

// keyTunRan in the meta bucket marks a TUN of this copy that came up. Windows remembers the network of the TUN after
// it goes, and only an administrator may look through the remembered networks: a core without rights goes by the mark.
var keyTunRan = []byte("tunRan")

// noteTun marks a TUN that came up.
func (e *Engine) noteTun() {
	if e.tunNoted.Load() {
		return
	}
	err := e.db.Update(func(tx *bbolt.Tx) error {
		bucket := tx.Bucket(bucketMeta)
		if bucket == nil {
			return errors.New("no meta bucket")
		}
		return bucket.Put(keyTunRan, []byte{1})
	})
	if err != nil {
		log.Printf("cleanup: the mark of the TUN: %v", err)
		return
	}
	e.tunNoted.Store(true)
}

// tunRan tells whether a TUN came up since the last removal of the traces.
func (e *Engine) tunRan() bool {
	ran := false
	_ = e.db.View(func(tx *bbolt.Tx) error {
		if bucket := tx.Bucket(bucketMeta); bucket != nil {
			ran = bucket.Get(keyTunRan) != nil
		}
		return nil
	})
	return ran
}

func (e *Engine) forgetTun() {
	err := e.db.Update(func(tx *bbolt.Tx) error {
		if bucket := tx.Bucket(bucketMeta); bucket != nil {
			return bucket.Delete(keyTunRan)
		}
		return nil
	})
	if err != nil {
		log.Printf("cleanup: the mark of the TUN: %v", err)
		return
	}
	e.tunNoted.Store(false)
}

// systemProxyTrace is the proxy of Windows a connection or a refusal points at Poros.
func (e *Engine) systemProxyTrace() trace {
	return trace{
		kind:    CleanupSystemProxy,
		profile: true,
		find: func() (int, error) {
			if e.sysProxy.Active() {
				return 1, nil
			}
			return 0, nil
		},
		remove: e.sysProxy.Clear,
	}
}

func scanTraces(traces []trace, foreign bool) []CleanupItem {
	items := []CleanupItem{}
	hidden := false
	for _, t := range traces {
		if t.profile && foreign {
			hidden = true
			continue
		}
		count, err := t.find()
		if err != nil {
			log.Printf("cleanup: look for %s: %v", t.kind, err)
			continue
		}
		if count > 0 {
			items = addTrace(items, CleanupItem{Kind: t.kind, Count: count, Rights: t.rights})
		}
	}
	if hidden {
		items = addTrace(items, CleanupItem{Kind: CleanupProfile})
	}
	return items
}

// removeTraces removes the traces it may: those that take rights only with them, those of the user's profile only
// in a core of the user's own account. It gives back the ones that stay.
func removeTraces(traces []trace, rights, foreign bool) []CleanupItem {
	items := []CleanupItem{}
	hidden := false
	for _, t := range traces {
		if t.profile && foreign {
			hidden = true
			continue
		}
		count, err := t.find()
		if err == nil && count == 0 {
			continue
		}
		if err == nil && t.rights && !rights {
			items = addTrace(items, CleanupItem{Kind: t.kind, Count: count, Rights: true})
			continue
		}
		if err == nil {
			err = t.remove()
		}
		if err == nil {
			log.Printf("cleanup: removed %s: %d", t.kind, count)
			continue
		}
		log.Printf("cleanup: %s: %v", t.kind, err)
		items = addTrace(items, CleanupItem{Kind: t.kind, Count: count, Rights: t.rights, Error: err.Error()})
	}
	if hidden {
		items = addTrace(items, CleanupItem{Kind: CleanupProfile})
	}
	return items
}

// addTrace adds an item, or adds it up with the item of the same kind: one kind may have several traces.
func addTrace(items []CleanupItem, item CleanupItem) []CleanupItem {
	for i := range items {
		if items[i].Kind != item.Kind {
			continue
		}
		items[i].Count += item.Count
		items[i].Rights = items[i].Rights || item.Rights
		items[i].Error = strings.Join(nonEmpty(items[i].Error, item.Error), "; ")
		return items
	}
	return append(items, item)
}

func nonEmpty(texts ...string) []string {
	var kept []string
	for _, text := range texts {
		if text != "" {
			kept = append(kept, text)
		}
	}
	return kept
}

// tunNetwork matches the names Windows gives the network of the TUN adapter: its name, or with a number when the
// name was taken, "poros-net 2".
func tunNetwork(name, tun string) bool {
	return regexp.MustCompile(`(?i)^` + regexp.QuoteMeta(tun) + `( \d+)?$`).MatchString(name)
}

// errStillThere is a trace that its removal left.
var errStillThere = errors.New("still there after the removal")
