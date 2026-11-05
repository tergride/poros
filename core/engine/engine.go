// Package engine is the client logic of Poros. The desktop host reaches it over JSON-RPC, a mobile build will bind it directly.
package engine

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"sync"
	"time"

	"github.com/sagernet/bbolt"
)

// Notify delivers an event to the interface.
type Notify func(method string, params any)

// Config is what the host tells the engine.
type Config struct {
	DataDir string
	// Version is the app's version, which providers see as Poros/<version>.
	Version string
	// Notify may be nil.
	Notify Notify
}

type Engine struct {
	dataDir   string
	userAgent string
	db        *bbolt.DB
	notify    Notify
	presets   []presetRecord
	now       func() time.Time
	client    *http.Client

	// Writes that announce themselves hold it, so events leave in the order of the changes.
	writeMu sync.Mutex
	// updating holds a lock per subscription, so one is not fetched twice at once.
	updatingMu sync.Mutex
	updating   map[string]*sync.Mutex

	stop    context.CancelFunc
	stopped sync.WaitGroup
}

// fetchTimeout keeps an update inside the host's 30 seconds for an answer.
const fetchTimeout = 20 * time.Second

// Open opens or creates the data in cfg.DataDir.
func Open(cfg Config) (*Engine, error) {
	notify := cfg.Notify
	if notify == nil {
		notify = func(string, any) {}
	}
	presets, err := builtinPresets()
	if err != nil {
		return nil, err
	}
	if err = os.MkdirAll(cfg.DataDir, 0o700); err != nil {
		return nil, err
	}
	// A second core on the same folder would wait here; a second of that means another Poros holds the data.
	db, err := bbolt.Open(filepath.Join(cfg.DataDir, "poros.db"), 0o600, &bbolt.Options{Timeout: time.Second})
	if err != nil {
		return nil, fmt.Errorf("open poros.db: %w", err)
	}
	if err = createBuckets(db); err != nil {
		db.Close()
		return nil, err
	}
	userAgent := "Poros"
	if cfg.Version != "" {
		userAgent += "/" + cfg.Version
	}
	// Subscriptions download directly: the system proxy may be Poros's own, which is not running yet.
	transport := http.DefaultTransport.(*http.Transport).Clone()
	transport.Proxy = nil
	return &Engine{
		dataDir:   cfg.DataDir,
		userAgent: userAgent,
		db:        db,
		notify:    notify,
		presets:   presets,
		now:       time.Now,
		client:    &http.Client{Timeout: fetchTimeout, Transport: transport},
		updating:  map[string]*sync.Mutex{},
		stop:      func() {},
	}, nil
}

// Close stops the schedule, waits for an update in flight and closes the data.
func (e *Engine) Close() error {
	e.stop()
	e.stopped.Wait()
	return e.db.Close()
}

func newID() string {
	var b [8]byte
	if _, err := rand.Read(b[:]); err != nil {
		panic(err)
	}
	return hex.EncodeToString(b[:])
}
