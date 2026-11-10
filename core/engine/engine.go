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
	"strings"
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
	// Window is the PID of the Poros window the core serves, 0 without one: what the kill switch held goes on with
	// the next core of the same window.
	Window int
	// UpdateFeed is latest.json of the releases, "" for those of tergride/poros, and UpdateReleases the Atom list
	// of all of them, pre-releases too, that beta versions come from; ProgramDir is the folder with updater.exe, ""
	// for the folder of this program.
	UpdateFeed     string
	UpdateReleases string
	ProgramDir     string
}

type Engine struct {
	dataDir string
	// started is when the engine opened, for the time the core works.
	started   time.Time
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
	upd     updates
	ks      killSwitch
	guard   Guard
	net     networkCheck
	latency latencyTest
	tests   testRuns
	results serverResults
	traffic serverTraffic
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
// DataFile holds the settings, subscriptions and presets in the data folder.
const DataFile = "poros.db"

// ErrDataDamaged is a data file Poros cannot read, as a power cut may leave it: starting again changes nothing, so the
// shell asks the user whether to start with clean data instead.
var ErrDataDamaged = errors.New("the data file is damaged")

func damaged(err error) bool {
	return errors.Is(err, bbolt.ErrInvalid) || errors.Is(err, bbolt.ErrChecksum) ||
		errors.Is(err, bbolt.ErrVersionMismatch) || strings.Contains(err.Error(), "file size too small")
}

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
	db, err := bbolt.Open(filepath.Join(cfg.DataDir, DataFile), 0o600, &bbolt.Options{Timeout: time.Second})
	if err != nil {
		if damaged(err) {
			err = fmt.Errorf("%w: %w", ErrDataDamaged, err)
		}
		return nil, fmt.Errorf("open %s: %w", DataFile, err)
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
		started:     time.Now(),
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
	transport.DialContext = e.dialDirect
	e.conn.state = ConnectionState{State: ConnectionIdle}
	e.openUpdates(cfg)
	e.latency.results = map[string]int{}
	e.results.speed, e.results.ip = map[string]ServerSpeed{}, map[string]ServerIP{}
	if err = e.loadTraffic(); err != nil {
		db.Close()
		return nil, err
	}
	e.latencyURL = latencyURL
	e.net.state = NetworkState{DNS: []DNSServer{}}
	// A core under another account sets no proxy of Windows, so it keeps no entry for the next sign-in either.
	if hook := ProxyRecovery(cfg.DataDir); hook != nil && !cfg.ForeignUser {
		e.sysProxy.OnPending(hook)
	}
	// Only now, with the data folder locked: the proxy of another core on the folder is not this one's to clear.
	// A core of the same window that the kill switch held traffic in left it for this one.
	e.ks.window = cfg.Window
	if settings, _ := e.Settings(); e.readMark() && settings.KillSwitch != KillSwitchOff {
		e.ks.resume = true
		err = e.sysProxy.Adopt()
	} else {
		e.writeMark(false)
		err = e.sysProxy.Recover()
	}
	if err != nil {
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
	e.closeUpdates()
	e.closeNetworkChecks()
	e.closeLatencyTests()
	err := e.closeConnection()
	e.closeKillSwitch()
	e.saveTraffic()
	return errors.Join(err, e.db.Close())
}

func newID() string {
	var b [8]byte
	if _, err := rand.Read(b[:]); err != nil {
		panic(err)
	}
	return hex.EncodeToString(b[:])
}
