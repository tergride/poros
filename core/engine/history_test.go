package engine

import (
	"fmt"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/sagernet/bbolt"

	"PorosCore/internal/boxmain"
)

// The history counts the traffic of a connection by profile and by program, keeps it over a stop, and lets go of
// hours older than it keeps.
func TestTrafficHistory(t *testing.T) {
	e, states := openConnectionTest(t)
	page := webPage(t)
	port := freePort(t)
	server, cancel, err := boxmain.Create([]byte(fmt.Sprintf(`{"log":{"level":"warn"},`+
		`"inbounds":[{"type":"trojan","listen":"127.0.0.1","listen_port":%d,"users":[{"password":"secret"}]}],`+
		`"outbounds":[{"type":"direct","tag":"direct"}]}`, port)), nil)
	if err != nil {
		t.Fatalf("the test server: %v", err)
	}
	t.Cleanup(func() { cancel(); server.Close() })
	useServer(t, e, fmt.Sprintf("trojan://secret@127.0.0.1:%d?security=none#local", port))
	if _, err = e.TrafficHistory(TrafficQuery{Hours: 0}); reason(err) != ReasonInvalidValue {
		t.Errorf("no hours: %v", err)
	}
	connectAndFetch(t, e, states, page)
	settings, _ := e.Settings()
	history, err := e.TrafficHistory(TrafficQuery{Hours: 24})
	if err != nil || len(history.Profiles) != 1 || history.Profiles[0].Key != settings.Server || history.Profiles[0].Download == 0 || len(history.Apps) == 0 {
		t.Fatalf("the history while connected: %+v, %v", history, err)
	}
	e.StopConnection()
	if state := waitState(t, states); state.State != ConnectionIdle {
		t.Fatalf("stop: %+v", state)
	}
	if after, _ := e.TrafficHistory(TrafficQuery{Hours: 24}); after.Profiles[0].Download != history.Profiles[0].Download {
		t.Errorf("the history after the stop: %+v, before %+v", after.Profiles, history.Profiles)
	}
	if _, err = os.Stat(filepath.Join(e.dataDir, HistoryFile)); err != nil {
		t.Errorf("the history keeps a file of its own: %v", err)
	}

	// An hour older than the history keeps goes with the next record.
	db, _ := e.historyDB()
	old := hourKey(time.Now().Add(-historyKeep - time.Hour))
	db.Update(func(tx *bbolt.Tx) error { return tx.Bucket(bucketHours).Put(old, []byte(`{"a":{"old.exe":[1,1]}}`)) })
	h := newHistoryTracker("x")
	h.pair(h.apps, "new.exe").download.Store(10)
	e.recordHistory(h)
	db.View(func(tx *bbolt.Tx) error {
		if tx.Bucket(bucketHours).Get(old) != nil {
			t.Error("an hour older than 90 days stays")
		}
		return nil
	})
}
