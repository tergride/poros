package main

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"io"
	"log"
	"os"
	"path/filepath"
	"runtime"
	runtimeDebug "runtime/debug"
	"runtime/metrics"
	"runtime/pprof"
	"time"

	"github.com/xtls/xray-core/core"

	"PorosCore/engine"
	"PorosCore/internal/boxmain"
	"PorosCore/internal/guard"
	"PorosCore/internal/host"
	"PorosCore/internal/ipc"
	"PorosCore/internal/jsonrpc"

	C "github.com/sagernet/sing-box/constant"
)

const (
	// memoryPanicThreshold stays under memoryLimit: that much live under the soft limit means the GC is thrashing.
	memoryLimit           = 2 * 1024 * 1024 * 1024
	memoryPanicThreshold  = 1536 * 1024 * 1024
	memoryCheckInterval   = 2 * time.Second
	memoryForcedGCBackoff = 30 * time.Second
)

// Not HeapAlloc: it counts unswept garbage and sawtooths up to the GC target, so a bare threshold on it fires on a healthy heap.
func liveHeap() uint64 {
	sample := []metrics.Sample{{Name: "/gc/heap/live:bytes"}}
	metrics.Read(sample)
	return sample[0].Value.Uint64()
}

func watchMemory() {
	for {
		time.Sleep(memoryCheckInterval)

		if liveHeap() < memoryPanicThreshold {
			continue
		}

		// The metric is only as fresh as the last cycle, which during a burst can mark short-lived objects live.
		runtimeDebug.FreeOSMemory()
		live := liveHeap()
		if live < memoryPanicThreshold {
			// FreeOSMemory is stop-the-world; do not repeat it every tick while a busy core sits near the threshold.
			time.Sleep(memoryForcedGCBackoff)
			continue
		}

		log.Printf("memory watchdog: %d MiB live after a forced GC, %d goroutines",
			live>>20, runtime.NumGoroutine())
		if path, err := writeHeapProfile(); err != nil {
			log.Printf("memory watchdog: could not write heap profile: %v", err)
		} else {
			log.Printf("memory watchdog: heap profile written to %s", path)
		}
		panic(fmt.Sprintf("Live heap reached %d MiB after a forced GC, this is not normal", live>>20))
	}
}

func writeHeapProfile() (string, error) {
	// Core runs privileged: a guessable clock-derived name lets a planted symlink turn this into a root-owned write anywhere.
	f, err := os.CreateTemp("", "poros-core-heap-*.pprof")
	if err != nil {
		return "", err
	}
	defer f.Close()
	if err = pprof.WriteHeapProfile(f); err != nil {
		return "", err
	}
	return f.Name(), f.Sync()
}

// options are what the shell passes: its channel and the token it will greet the core with, the data folder, the
// app's version, its own process, the core this one replaces, and another release feed for tests. An elevated
// core gets no environment from its shell, so all of it comes as arguments.
type options struct {
	pipe       string
	token      string
	dataDir    string
	version    string
	hostPID    int
	waitPID    int
	updateFeed string
}

func parseOptions(args []string) (options, error) {
	var opts options
	flags := flag.NewFlagSet("PorosCore", flag.ContinueOnError)
	flags.SetOutput(io.Discard)
	flags.StringVar(&opts.pipe, "pipe", "", "")
	flags.StringVar(&opts.token, "token", "", "")
	flags.StringVar(&opts.dataDir, "data-dir", "", "")
	flags.StringVar(&opts.version, "version", "", "")
	flags.IntVar(&opts.hostPID, "host-pid", 0, "")
	flags.IntVar(&opts.waitPID, "wait-pid", 0, "")
	flags.StringVar(&opts.updateFeed, "update-feed", "", "")
	if err := flags.Parse(args); err != nil {
		return opts, err
	}
	if opts.pipe == "" || opts.token == "" || opts.dataDir == "" {
		return opts, errors.New("--pipe, --token and --data-dir are required")
	}
	return opts, nil
}

const (
	// waitPrevious is how long a core waits for the one it replaces: that one closes its connection first.
	waitPrevious = 20 * time.Second
	// exitDeadline bounds the way out once the shell is gone: a connection that does not close goes with the process.
	exitDeadline = 15 * time.Second
	logFile      = "core.log"
	oldLogFile   = "core.1.log"
	// exitDataDamaged tells the shell that the data file cannot be read: no restart helps, the user decides.
	exitDataDamaged = 4
)

// openLog keeps this run's log and the previous one in the logs folder of the data folder.
func openLog(dataDir string) (*os.File, error) {
	dir := filepath.Join(dataDir, "logs")
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return nil, err
	}
	_ = os.Rename(filepath.Join(dir, logFile), filepath.Join(dir, oldLogFile))
	return os.OpenFile(filepath.Join(dir, logFile), os.O_CREATE|os.O_WRONLY|os.O_TRUNC, 0o600)
}

// runCore serves the shell over its channel and exits when the shell goes away: the channel is the core's lifetime.
func runCore() {
	opts, err := parseOptions(os.Args[1:])
	if err != nil {
		log.Fatalf("arguments: %v", err)
	}
	if opts.waitPID != 0 {
		if err = host.WaitExit(opts.waitPID, waitPrevious); err != nil {
			log.Fatalf("%v", err)
		}
	}
	if logOutput, err := openLog(opts.dataDir); err == nil {
		if err = host.RedirectOutput(logOutput); err != nil {
			log.Printf("log to %s: %v", logOutput.Name(), err)
		}
		log.SetOutput(logOutput)
		_ = runtimeDebug.SetCrashOutput(logOutput, runtimeDebug.CrashOptions{})
	}
	log.Printf("sing-box %s, Xray %s", C.Version, core.Version())

	boxmain.DisableColor()

	conn, err := ipc.Dial(opts.pipe)
	if err != nil {
		log.Fatalf("connect to the shell: %v", err)
	}
	foreignUser := false
	if opts.hostPID != 0 {
		same, err := host.SameUser(opts.hostPID)
		foreignUser = err == nil && !same
	}
	peer := jsonrpc.NewConn(conn)
	eng, err := engine.Open(engine.Config{
		DataDir: opts.dataDir, Version: opts.version, Notify: peer.Notify, ForeignUser: foreignUser,
		Guard: engine.NewGuard(opts.hostPID, opts.dataDir), UpdateFeed: opts.updateFeed,
	})
	if err != nil {
		log.Printf("open the data folder: %v", err)
		if errors.Is(err, engine.ErrDataDamaged) {
			os.Exit(exitDataDamaged)
		}
		os.Exit(1)
	}
	eng.StartSchedule()
	eng.StartNetworkChecks()
	eng.StartKillSwitch()
	if opts.hostPID != 0 {
		// A shell that is killed may leave the channel open for a while; its process is the surer sign.
		gone := host.Watch(opts.hostPID)
		go func() {
			<-gone
			log.Print("the shell is gone")
			time.AfterFunc(exitDeadline+5*time.Second, func() { os.Exit(3) })
			conn.Close()
		}()
	}
	err = peer.Serve(context.Background(), opts.token, eng.Handlers())
	if err != nil && !errors.Is(err, io.EOF) {
		log.Printf("shell connection: %v", err)
	}
	time.AfterFunc(exitDeadline, func() {
		log.Print("the connection did not close in time")
		os.Exit(3)
	})
	if err = eng.Close(); err != nil {
		log.Printf("close: %v", err)
	}
	os.Exit(0)
}

func main() {
	// Before anything prints: stdout is the guard's protocol channel.
	if len(os.Args) > 1 && os.Args[1] == "--guard" {
		guard.Run(os.Args[2:])
	}

	defer func() {
		if err := recover(); err != nil {
			// The exit code is all the GUI has to tell a panic from a clean stop.
			fmt.Fprintf(os.Stderr, "Core panicked: %v\n%s\n", err, runtimeDebug.Stack())
			os.Exit(2)
		}
	}()
	runtimeDebug.SetMemoryLimit(memoryLimit)
	go watchMemory()

	runCore()
}
