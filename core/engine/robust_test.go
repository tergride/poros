package engine

import (
	"context"
	"fmt"
	"go/ast"
	"go/parser"
	"go/token"
	"net"
	"net/http"
	"os"
	"runtime"
	"runtime/pprof"
	"slices"
	"strconv"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/sagernet/bbolt"
)

// The robustness tests hit the engine with random input, many callers at once, long runs and killed writers. They
// run on loopback alone, as every check on the host: no TUN, no system proxy, nothing past this computer.
// POROS_STRESS=1 runs their long versions; POROS_ROBUST_SEED=<n> repeats a run of random input.

func robustStress() bool { return os.Getenv("POROS_STRESS") == "1" }

// robustSeed is the seed of the random input, printed so that a failure repeats.
func robustSeed(t *testing.T) uint64 {
	t.Helper()
	seed := uint64(20261113)
	if text := os.Getenv("POROS_ROBUST_SEED"); text != "" {
		parsed, err := strconv.ParseUint(text, 10, 64)
		if err != nil {
			t.Fatalf("POROS_ROBUST_SEED: %v", err)
		}
		seed = parsed
	}
	t.Logf("seed %d: POROS_ROBUST_SEED=%d repeats this run", seed, seed)
	return seed
}

// robustOnHost skips a run with administrator rights: with them a TUN or the task of the sign-in would come up for
// real.
func robustOnHost(t *testing.T) {
	t.Helper()
	if elevated() {
		t.Skip("the test runs with administrator rights; it is for the host without them")
	}
}

// closerServer is a loopback server that closes every connection at once: a server that fails fast.
func closerServer(t *testing.T) string {
	t.Helper()
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { listener.Close() })
	go func() {
		for {
			conn, err := listener.Accept()
			if err != nil {
				return
			}
			conn.Close()
		}
	}()
	return listener.Addr().String()
}

// loopbackClient downloads from the addresses given alone, so that no input reaches past this computer.
func loopbackClient(allowed ...string) *http.Client {
	dialer := &net.Dialer{Timeout: 2 * time.Second}
	transport := &http.Transport{
		Proxy: nil,
		DialContext: func(ctx context.Context, network, address string) (net.Conn, error) {
			if !slices.Contains(allowed, address) {
				return nil, fmt.Errorf("the robustness tests dial their own servers alone, not %s", address)
			}
			return dialer.DialContext(ctx, network, address)
		},
	}
	return &http.Client{Timeout: 5 * time.Second, Transport: transport}
}

func hostOf(t *testing.T, link string) string {
	t.Helper()
	rest, ok := strings.CutPrefix(link, "http://")
	if !ok {
		t.Fatalf("%s is no http:// link", link)
	}
	host, _, _ := strings.Cut(rest, "/")
	return host
}

// openRobust opens an engine for the robustness tests. A core under another account sets no system proxy, so no
// input can set one; the delay tests ask page, and downloads reach the addresses allowed alone. close may be called
// more than once.
func openRobust(t *testing.T, dir, page string, allowed ...string) (e *Engine, close func()) {
	t.Helper()
	e, err := Open(Config{DataDir: dir, MixedPort: freePort(t), ForeignUser: true})
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	// Commits do not wait for the disk: these tests are about the logic, and they make more changes so.
	e.db.NoSync = true
	e.latencyURL = page
	e.client = loopbackClient(allowed...)
	var once sync.Once
	close = func() { once.Do(func() { closeWithin(t, e, 30*time.Second) }) }
	t.Cleanup(close)
	return e, close
}

// closeWithin closes the engine and fails the test when Close hangs.
func closeWithin(t *testing.T, e *Engine, limit time.Duration) {
	t.Helper()
	done := make(chan error, 1)
	go func() { done <- e.Close() }()
	select {
	case err := <-done:
		if err != nil {
			t.Errorf("Close: %v", err)
		}
	case <-time.After(limit):
		t.Errorf("Close did not end in %s:\n%s", limit, goroutineDump())
	}
}

// waitIdle waits for the connection to settle idle with nothing running, the refusal of the kill switch included.
func waitIdle(t *testing.T, e *Engine, limit time.Duration) {
	t.Helper()
	deadline := time.Now().Add(limit)
	for {
		running := true
		if e.conn.lifecycle.TryLock() {
			running = e.conn.running != nil
			e.conn.lifecycle.Unlock()
		}
		state := e.ConnectionState()
		if state.State == ConnectionIdle && !running {
			return
		}
		if time.Now().After(deadline) {
			t.Errorf("the connection did not settle idle in %s: %+v, something running %t\n%s", limit, state, running, goroutineDump())
			return
		}
		time.Sleep(50 * time.Millisecond)
	}
}

// stopTestsWithin stops the server tests that run and fails the test when they do not end.
func stopTestsWithin(t *testing.T, e *Engine, limit time.Duration) {
	t.Helper()
	done := make(chan struct{})
	go func() {
		e.tests.stopAll()
		close(done)
	}()
	select {
	case <-done:
	case <-time.After(limit):
		t.Errorf("the server tests did not stop in %s:\n%s", limit, goroutineDump())
	}
}

// robustCheckData checks what must hold of the data whatever happened to it: the settings pass their own check,
// so the chosen server and preset exist; each group counts its servers right; no server ID repeats; no servers
// are left of a group that is gone; bbolt finds its pages whole.
func robustCheckData(t *testing.T, e *Engine) {
	t.Helper()
	settings, err := e.Settings()
	if err != nil {
		t.Errorf("the settings cannot be read: %v", err)
	} else if err = e.validateSettings(settings); err != nil {
		t.Errorf("the stored settings fail their own check: %v (server %q, preset %q)", err, settings.Server, settings.Preset)
	}
	err = e.db.View(func(tx *bbolt.Tx) error {
		for err := range tx.Check() {
			t.Errorf("bbolt: %v", err)
		}
		store, err := loadServers(tx)
		if err != nil {
			return err
		}
		seen := map[string]bool{}
		for _, group := range store.groups {
			servers := store.servers[group.ID]
			if group.Servers != len(servers) {
				t.Errorf("the group %s counts %d servers and holds %d", group.ID, group.Servers, len(servers))
			}
			for _, server := range servers {
				if seen[server.ID] {
					t.Errorf("the server ID %s repeats", server.ID)
				}
				seen[server.ID] = true
			}
		}
		groups := map[string]bool{}
		for _, group := range store.groups {
			groups[group.ID] = true
		}
		return tx.Bucket(bucketServers).ForEach(func(key, _ []byte) error {
			if !groups[string(key)] {
				t.Errorf("servers are left of the group %s, which is gone", key)
			}
			return nil
		})
	})
	if err != nil {
		t.Errorf("the servers cannot be read: %v", err)
	}
	entries, err := e.OTPCodes()
	if err != nil {
		t.Errorf("the codes of OTP cannot be read: %v", err)
	}
	ids := map[string]bool{}
	for _, entry := range entries {
		if ids[entry.ID] {
			t.Errorf("the code of OTP %s repeats", entry.ID)
		}
		ids[entry.ID] = true
	}
}

// apiConstants reads the string constants of api.go whose names start with prefix, by value. The test runs in the
// folder of the package.
func apiConstants(t *testing.T, prefix string) map[string]string {
	t.Helper()
	file, err := parser.ParseFile(token.NewFileSet(), "api.go", nil, 0)
	if err != nil {
		t.Fatalf("api.go: %v (the test runs in the folder of the package)", err)
	}
	values := map[string]string{}
	for _, decl := range file.Decls {
		gen, ok := decl.(*ast.GenDecl)
		if !ok || gen.Tok != token.CONST {
			continue
		}
		for _, spec := range gen.Specs {
			valueSpec := spec.(*ast.ValueSpec)
			for i, name := range valueSpec.Names {
				if !strings.HasPrefix(name.Name, prefix) || i >= len(valueSpec.Values) {
					continue
				}
				if literal, ok := valueSpec.Values[i].(*ast.BasicLit); ok && literal.Kind == token.STRING {
					value, _ := strconv.Unquote(literal.Value)
					values[value] = name.Name
				}
			}
		}
	}
	return values
}

func goroutineDump() string {
	buf := make([]byte, 1<<20)
	for {
		n := runtime.Stack(buf, true)
		if n < len(buf) {
			return string(buf[:n])
		}
		buf = make([]byte, 2*len(buf))
	}
}

// snapshot is what a process holds at a moment: goroutines, by where they were created and where they wait, the
// handles of Windows (-1 elsewhere) and the threads the runtime made, which hold handles of their own.
type snapshot struct {
	goroutines int
	handles    int
	threads    int
	groups     map[string]int
}

func takeSnapshot() snapshot {
	// What was dropped closes its handles in finalizers.
	runtime.GC()
	runtime.GC()
	s := snapshot{handles: processHandles(), threads: pprof.Lookup("threadcreate").Count(), groups: map[string]int{}}
	for _, block := range strings.Split(goroutineDump(), "\n\n") {
		lines := strings.Split(strings.TrimSpace(block), "\n")
		if len(lines) < 2 || !strings.HasPrefix(lines[0], "goroutine ") {
			continue
		}
		s.goroutines++
		created, top := "?", ""
		for _, line := range lines[1:] {
			switch rest, isCreator := strings.CutPrefix(line, "created by "); {
			case strings.HasPrefix(line, "\t"):
			case isCreator:
				created, _, _ = strings.Cut(rest, " in goroutine")
			case top == "" && !waitFrame(frameName(line)):
				top = frameName(line)
			}
		}
		if top == "" {
			top = frameName(lines[1])
		}
		s.groups[top+" <- "+created]++
	}
	return s
}

func frameName(line string) string {
	if i := strings.LastIndex(line, "("); i > 0 {
		return strings.TrimSpace(line[:i])
	}
	return strings.TrimSpace(line)
}

// waitFrame is a frame of the runtime or the standard library a goroutine waits in, not the code that made it wait.
func waitFrame(name string) bool {
	for _, prefix := range []string{"runtime.", "internal/", "sync.", "syscall.", "time.", "net.", "os.", "io.", "bufio."} {
		if strings.HasPrefix(name, prefix) {
			return true
		}
	}
	return false
}

// growth lists the goroutines after has more of than before, the most first.
func growth(before, after snapshot) string {
	type change struct {
		name string
		n    int
	}
	var changes []change
	for name, n := range after.groups {
		if d := n - before.groups[name]; d > 0 {
			changes = append(changes, change{name, d})
		}
	}
	slices.SortFunc(changes, func(a, b change) int { return b.n - a.n })
	var lines []string
	for _, c := range changes {
		lines = append(lines, fmt.Sprintf("  +%d %s", c.n, c.name))
	}
	return strings.Join(lines, "\n")
}

// settle waits up to limit for the goroutines and handles to come back within the tolerances of before, and gives
// the last snapshot.
func settle(before snapshot, goroutines, handles int, limit time.Duration) snapshot {
	deadline := time.Now().Add(limit)
	for {
		after := takeSnapshot()
		if after.goroutines <= before.goroutines+goroutines && after.handles <= before.handles+handles || time.Now().After(deadline) {
			return after
		}
		time.Sleep(250 * time.Millisecond)
	}
}
