package main

import (
	"context"
	"errors"
	"fmt"
	"io"
	"log"
	"os"
	"runtime"
	runtimeDebug "runtime/debug"
	"runtime/metrics"
	"runtime/pprof"
	"time"

	"github.com/xtls/xray-core/core"

	"PorosCore/engine"
	"PorosCore/internal/boxmain"
	"PorosCore/internal/guard"
	"PorosCore/internal/ipc"
	"PorosCore/internal/jsonrpc"

	C "github.com/sagernet/sing-box/constant"

	_ "PorosCore/internal/distro/all"
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

// The host passes these in the environment, so they do not show in the process list.
const (
	envPipe    = "POROS_CORE_PIPE"
	envToken   = "POROS_CORE_TOKEN"
	envDataDir = "POROS_DATA_DIR"
)

// runCore serves the host over its channel and exits when the host goes away: the channel is the core's lifetime.
func runCore() {
	pipe, token, dataDir := os.Getenv(envPipe), os.Getenv(envToken), os.Getenv(envDataDir)
	if pipe == "" || token == "" || dataDir == "" {
		log.Fatalf("%s, %s and %s must be set", envPipe, envToken, envDataDir)
	}
	// The token is for this process only; children such as the guard must not inherit it.
	os.Unsetenv(envToken)

	boxmain.DisableColor()

	conn, err := ipc.Dial(pipe)
	if err != nil {
		log.Fatalf("connect to the host: %v", err)
	}
	peer := jsonrpc.NewConn(conn)
	eng, err := engine.Open(dataDir, peer.Notify)
	if err != nil {
		log.Fatalf("open the data folder: %v", err)
	}
	err = peer.Serve(context.Background(), token, eng.Handlers())
	eng.Close()
	if !errors.Is(err, io.EOF) {
		log.Fatalf("host connection: %v", err)
	}
}

func main() {
	// Before anything prints: stdout is the guard's protocol channel.
	if len(os.Args) > 1 && os.Args[1] == "--guard" {
		guard.Run(os.Getppid())
	}

	defer func() {
		if err := recover(); err != nil {
			// The exit code is all the GUI has to tell a panic from a clean stop.
			fmt.Fprintf(os.Stderr, "Core panicked: %v\n%s\n", err, runtimeDebug.Stack())
			os.Exit(2)
		}
	}()
	fmt.Println("sing-box:", C.Version)
	fmt.Println("Xray-core:", core.Version())
	fmt.Println()
	runtimeDebug.SetMemoryLimit(memoryLimit)
	go watchMemory()

	runCore()
}
