// Package engine is the client logic of Poros. The desktop host reaches it over JSON-RPC, a mobile build will bind it directly.
package engine

import (
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"os"
	"path/filepath"
	"sync"
	"time"

	"github.com/sagernet/bbolt"
)

// Notify delivers an event to the interface.
type Notify func(method string, params any)

type Engine struct {
	dataDir string
	db      *bbolt.DB
	notify  Notify
	presets []Preset
	now     func() time.Time

	// Writes that announce themselves hold it, so events leave in the order of the changes.
	writeMu sync.Mutex
}

// Open opens or creates the data in dataDir. notify may be nil.
func Open(dataDir string, notify Notify) (*Engine, error) {
	if notify == nil {
		notify = func(string, any) {}
	}
	presets, err := builtinPresets()
	if err != nil {
		return nil, err
	}
	if err = os.MkdirAll(dataDir, 0o700); err != nil {
		return nil, err
	}
	// A second core on the same folder would wait here; a second of that means another Poros holds the data.
	db, err := bbolt.Open(filepath.Join(dataDir, "poros.db"), 0o600, &bbolt.Options{Timeout: time.Second})
	if err != nil {
		return nil, fmt.Errorf("open poros.db: %w", err)
	}
	if err = createBuckets(db); err != nil {
		db.Close()
		return nil, err
	}
	return &Engine{dataDir: dataDir, db: db, notify: notify, presets: presets, now: time.Now}, nil
}

func (e *Engine) Close() error {
	return e.db.Close()
}

func newID() string {
	var b [8]byte
	if _, err := rand.Read(b[:]); err != nil {
		panic(err)
	}
	return hex.EncodeToString(b[:])
}
