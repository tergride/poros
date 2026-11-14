package guard

import (
	"crypto/rand"
	"encoding/hex"
	"sync"
	"testing"
	"time"

	"golang.org/x/sys/windows"
)

func TestOptions(t *testing.T) {
	want := options{hostPID: 42, logPath: `C:\Poros Data\logs\guard.log`, config: Config{AllowLAN: true, TunName: "poros-net"}}
	args := want.args()
	if args[0] != "--guard" {
		t.Fatalf("args: %q", args)
	}
	got, err := parseOptions(args[1:])
	if err != nil || got != want {
		t.Fatalf("parseOptions(%q) = %+v, %v", args, got, err)
	}
	for _, bad := range [][]string{{}, {"--host-pid", "0"}, {"--host-pid", "x"}, {"--host-pid", "1", "--other"}} {
		if _, err := parseOptions(bad); ErrorCode(err) != CodeConfig {
			t.Errorf("parseOptions(%q): %v, want a %s error", bad, err, CodeConfig)
		}
	}
	for _, name := range []string{"has space", "longer-than-fifteen"} {
		if _, err := newConfig(Config{TunName: name}); ErrorCode(err) != CodeConfig {
			t.Errorf("tun name %q: %v", name, err)
		}
	}
}

type fakeBackend struct {
	mu      sync.Mutex
	disarms int
}

func (b *fakeBackend) Arm(*config, chan<- Event) error { return nil }

func (b *fakeBackend) Disarm() {
	b.mu.Lock()
	defer b.mu.Unlock()
	b.disarms++
}

func (b *fakeBackend) count() int {
	b.mu.Lock()
	defer b.mu.Unlock()
	return b.disarms
}

type served struct {
	name    string
	backend *fakeBackend
	events  chan Event
	stop    chan string
	exit    chan int
}

// startServe runs the guard's pipe with a block that only counts how often it was turned off. The test runs
// without rights, so its pipe gives this account everything the guard's gives administrators.
func startServe(t *testing.T) served {
	t.Helper()
	b := make([]byte, 8)
	rand.Read(b)
	user, err := windows.GetCurrentProcessToken().GetTokenUser()
	if err != nil {
		t.Fatal(err)
	}
	s := served{
		name: `\\.\pipe\poros-guard-test-` + hex.EncodeToString(b), backend: &fakeBackend{},
		events: make(chan Event, 1), stop: make(chan string, 1), exit: make(chan int, 1),
	}
	listener, err := listen(s.name, "D:P(A;;GA;;;"+user.User.Sid.String()+")")
	if err != nil {
		t.Fatal(err)
	}
	go func() { s.exit <- serve(s.backend, listener, s.events, s.stop) }()
	return s
}

func (s served) join(t *testing.T) *Link {
	t.Helper()
	link, err := dial(s.name)
	if err != nil || link == nil {
		t.Fatalf("join the guard: %v, %v", link, err)
	}
	return link
}

func (s served) exitCode(t *testing.T) int {
	t.Helper()
	select {
	case code := <-s.exit:
		return code
	case <-time.After(5 * time.Second):
		t.Fatal("the guard did not exit")
		return 0
	}
}

func lost(t *testing.T, link *Link) {
	t.Helper()
	select {
	case <-link.Lost():
	case <-time.After(5 * time.Second):
		t.Fatal("the link did not see the guard go")
	}
}

// A core that goes away leaves the block on; any other core of the window turns it off.
func TestDisarmThroughThePipe(t *testing.T) {
	s := startServe(t)
	failed := s.join(t)
	failed.Close()
	select {
	case <-s.exit:
		t.Fatal("a core that let go took the block with it")
	case <-time.After(200 * time.Millisecond):
	}
	if s.backend.count() != 0 {
		t.Fatal("the block went off without being asked")
	}

	next := s.join(t)
	if err := next.Disarm(); err != nil {
		t.Fatalf("Disarm: %v", err)
	}
	if s.backend.count() != 1 {
		t.Fatal("Disarm returned before the block was off")
	}
	if code := s.exitCode(t); code != 0 {
		t.Errorf("exit code %d", code)
	}
	lost(t, next)
	if link, err := dial(s.name + "-none"); link != nil || err != nil {
		t.Errorf("a window without a guard has none to join: %v, %v", link, err)
	}
}

func TestFailureReachesTheCores(t *testing.T) {
	s := startServe(t)
	link := s.join(t)
	s.events <- Event{Fatal: true, Code: CodeTampered, Message: "the rules were removed"}
	lost(t, link)
	if link.Reason() != "the rules were removed" {
		t.Errorf("reason %q", link.Reason())
	}
	if code := s.exitCode(t); code != 3 || s.backend.count() != 1 {
		t.Errorf("exit code %d, disarms %d", code, s.backend.count())
	}
}

func TestTheWindowGoing(t *testing.T) {
	s := startServe(t)
	link := s.join(t)
	s.stop <- "the Poros window is gone"
	if code := s.exitCode(t); code != 0 || s.backend.count() != 1 {
		t.Errorf("exit code %d, disarms %d", code, s.backend.count())
	}
	lost(t, link)
	if link.Reason() != "" {
		t.Errorf("a block turned off is no failure: %q", link.Reason())
	}
}

// Nothing can open the guard's pipe before it, or next to it.
func TestThePipeIsTheGuards(t *testing.T) {
	s := startServe(t)
	if _, err := listen(s.name, ""); err == nil {
		t.Fatal("a second program opened the guard's pipe")
	}
	s.stop <- "done"
	s.exitCode(t)
}
