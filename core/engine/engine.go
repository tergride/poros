// Package engine is the client logic of Poros. The desktop host reaches it over JSON-RPC, a mobile build will bind it directly.
package engine

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"errors"
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"sync"
	"time"

	"github.com/sagernet/bbolt"
	tun "github.com/sagernet/sing-tun"
	"github.com/sagernet/sing/common/x/list"

	"PorosCore/internal/netmon"
	"PorosCore/internal/sysproxy"
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
	// MixedPort is the port of the local proxy; 0 is DefaultMixedPort.
	MixedPort uint16
	// ForeignUser is a core elevated under another account than the shell's: the system proxy it would set is
	// that account's, so it sets none.
	ForeignUser bool
	// Guard holds the block of the kill switch; nil has no kill switch.
	Guard Guard
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

	conn    connection
	ks      killSwitch
	guard   Guard
	net     networkCheck
	latency latencyTest
	// latencyURL is what a delay test requests through each server; tests point it at a page of their own.
	latencyURL  string
	mixedPort   uint16
	sysProxy    *sysproxy.Proxy
	foreignUser bool
	// netCallback follows the default interface for connections on Xray.
	netCallback *list.Element[tun.DefaultInterfaceUpdateCallback]
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
	mixedPort := cfg.MixedPort
	if mixedPort == 0 {
		mixedPort = DefaultMixedPort
	}
	e := &Engine{
		dataDir:     cfg.DataDir,
		userAgent:   userAgent,
		db:          db,
		notify:      notify,
		presets:     presets,
		now:         time.Now,
		client:      &http.Client{Timeout: fetchTimeout, Transport: transport},
		updating:    map[string]*sync.Mutex{},
		stop:        func() {},
		mixedPort:   mixedPort,
		foreignUser: cfg.ForeignUser,
		guard:       cfg.Guard,
		sysProxy:    sysproxy.New(filepath.Join(cfg.DataDir, "system-proxy.json")),
	}
	e.conn.state = ConnectionState{State: ConnectionIdle}
	e.latency.results = map[string]int{}
	e.latencyURL = latencyURL
	e.net.state = NetworkState{DNS: []DNSServer{}}
	// Only now, with the data folder locked: the proxy of another core on the folder is not this one's to clear.
	if err = e.sysProxy.Recover(); err != nil {
		e.conn.state.Error, e.conn.state.ErrorText = ReasonSystemProxyFailed, err.Error()
	}
	if monitor := netmon.Monitor(); monitor != nil {
		e.netCallback = monitor.RegisterCallback(e.onDefaultInterface)
	}
	return e, nil
}

// Close stops the schedule and the connection, gives the system proxy back, waits for an update in flight and
// closes the data.
func (e *Engine) Close() error {
	if e.netCallback != nil {
		netmon.Monitor().UnregisterCallback(e.netCallback)
	}
	e.stop()
	e.stopped.Wait()
	e.closeNetworkChecks()
	e.closeLatencyTests()
	err := e.closeConnection()
	e.closeKillSwitch()
	return errors.Join(err, e.db.Close())
}

func newID() string {
	var b [8]byte
	if _, err := rand.Read(b[:]); err != nil {
		panic(err)
	}
	return hex.EncodeToString(b[:])
}
