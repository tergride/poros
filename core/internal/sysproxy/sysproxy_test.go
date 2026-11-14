package sysproxy

import (
	"errors"
	"os"
	"path/filepath"
	"testing"
)

type fakeSetting struct {
	flags  uint32
	server string
}

// fakeBackend stands in for WinINet: the tests must never touch the system's own proxy.
type fakeBackend struct {
	settings  map[string]fakeSetting
	order     []string
	failSet   map[string]bool
	announced int
}

func newFake(settings map[string]fakeSetting, order ...string) *fakeBackend {
	return &fakeBackend{settings: settings, order: order, failSet: map[string]bool{}}
}

func (f *fakeBackend) connections() ([]string, error) { return f.order, nil }
func (f *fakeBackend) flags(name string) uint32       { return f.settings[name].flags }
func (f *fakeBackend) server(name string) string      { return f.settings[name].server }
func (f *fakeBackend) announce()                      { f.announced++ }

func (f *fakeBackend) set(name string, flags uint32, server string) error {
	if f.failSet[name] {
		return errors.New("refused")
	}
	f.settings[name] = fakeSetting{flags, server}
	return nil
}

func testProxy(t *testing.T, b backend) (*Proxy, string) {
	path := filepath.Join(t.TempDir(), "system-proxy.json")
	p := newProxy(path, b)
	p.serves = func(string) bool { return true }
	return p, path
}

func TestApplyAndClearGiveTheSettingsBack(t *testing.T) {
	user := map[string]fakeSetting{"": {flagDirect | flagProxy, "corp.example:3128"}, "VPN": {flagDirect, ""}}
	b := newFake(map[string]fakeSetting{"": user[""], "VPN": user["VPN"]}, "", "VPN")
	p, path := testProxy(t, b)
	if err := p.Apply("127.0.0.1", 2090); err != nil {
		t.Fatal(err)
	}
	if err := p.Apply("127.0.0.1", 2091); err != nil {
		t.Fatal(err)
	}
	for _, name := range b.order {
		if got := b.settings[name]; got != (fakeSetting{flagDirect | flagProxy, "127.0.0.1:2091"}) {
			t.Fatalf("%q after Apply: %+v", name, got)
		}
	}
	if _, err := os.Stat(path); err != nil {
		t.Fatalf("Apply must write the state for a start after a crash: %v", err)
	}
	if err := p.Clear(); err != nil {
		t.Fatal(err)
	}
	for _, name := range b.order {
		if got := b.settings[name]; got != user[name] {
			t.Errorf("%q after Clear: %+v, want the user's %+v from before the first Apply", name, got, user[name])
		}
	}
	if _, err := os.Stat(path); !errors.Is(err, os.ErrNotExist) {
		t.Errorf("Clear must remove the state: %v", err)
	}
}

func TestClearLeavesSomeoneElsesProxy(t *testing.T) {
	b := newFake(map[string]fakeSetting{"": {flagDirect, ""}}, "")
	p, _ := testProxy(t, b)
	if err := p.Apply("127.0.0.1", 2090); err != nil {
		t.Fatal(err)
	}
	b.settings[""] = fakeSetting{flagDirect | flagProxy, "127.0.0.1:2080"}
	if err := p.Clear(); err != nil {
		t.Fatal(err)
	}
	if got := b.settings[""]; got.server != "127.0.0.1:2080" {
		t.Errorf("another client's proxy must stay, got %+v", got)
	}
}

func TestRecoverAfterACrash(t *testing.T) {
	b := newFake(map[string]fakeSetting{"": {flagDirect | flagProxy, "127.0.0.1:9999"}}, "")
	crashed, path := testProxy(t, b)
	if err := crashed.Apply("127.0.0.1", 2090); err != nil {
		t.Fatal(err)
	}
	next := newProxy(path, b)
	// The proxy from before Poros is gone too: browsers must not be left on a dead port.
	next.serves = func(string) bool { return false }
	if err := next.Recover(); err != nil {
		t.Fatal(err)
	}
	if got := b.settings[""]; got != (fakeSetting{flagDirect, "127.0.0.1:9999"}) {
		t.Errorf("Recover: %+v, want the old server without the proxy flag", got)
	}
	if _, err := os.Stat(path); !errors.Is(err, os.ErrNotExist) {
		t.Errorf("Recover must remove the state: %v", err)
	}
}

func TestRecoverKeepsWhatFailed(t *testing.T) {
	b := newFake(map[string]fakeSetting{"": {flagDirect, ""}}, "")
	crashed, path := testProxy(t, b)
	if err := crashed.Apply("127.0.0.1", 2090); err != nil {
		t.Fatal(err)
	}
	b.failSet[""] = true
	next, _ := testProxy(t, b)
	next.path = path
	if err := next.Recover(); err == nil {
		t.Fatal("a failed restore must be reported")
	}
	if _, err := os.Stat(path); err != nil {
		t.Errorf("the state must stay for the next try: %v", err)
	}
}

func TestRecoverRemovesADamagedFile(t *testing.T) {
	p, path := testProxy(t, newFake(map[string]fakeSetting{}, ""))
	if err := os.WriteFile(path, []byte("{broken"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := p.Recover(); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(path); !errors.Is(err, os.ErrNotExist) {
		t.Errorf("a damaged state must go: %v", err)
	}
}

func TestStillServes(t *testing.T) {
	if !stillServes("corp.example:3128") || !stillServes("http=10.0.0.1:8080;https=10.0.0.1:8443") {
		t.Error("a proxy of another computer counts as working")
	}
	if stillServes("127.0.0.1:1") || stillServes("http=localhost:1") {
		t.Error("a loopback proxy that does not answer is gone")
	}
}

func TestPendingFollowsTheStateFile(t *testing.T) {
	b := newFake(map[string]fakeSetting{"": {flagDirect, ""}}, "")
	p, path := testProxy(t, b)
	var heard []bool
	p.OnPending(func(pending bool) { heard = append(heard, pending) })
	if err := p.Apply("127.0.0.1", 2090); err != nil {
		t.Fatal(err)
	}
	if !p.Served() {
		t.Error("a proxy with its server is not served")
	}
	if err := p.Clear(); err != nil {
		t.Fatal(err)
	}
	if len(heard) != 2 || !heard[0] || heard[1] {
		t.Errorf("heard %v", heard)
	}
	if _, err := os.Stat(path); !os.IsNotExist(err) {
		t.Error("the state file stayed")
	}
	if p.Served() {
		t.Error("no state file, yet served")
	}
}

func TestAdoptKeepsTheProxyAndWhatWasBefore(t *testing.T) {
	b := newFake(map[string]fakeSetting{"": {flagDirect, ""}}, "")
	stopped, path := testProxy(t, b)
	if err := stopped.Apply("127.0.0.1", 2090); err != nil {
		t.Fatal(err)
	}
	next := newProxy(path, b)
	next.serves = func(string) bool { return true }
	if err := next.Adopt(); err != nil {
		t.Fatal(err)
	}
	if got := b.settings[""]; got != (fakeSetting{flagDirect | flagProxy, "127.0.0.1:2090"}) || !next.Active() {
		t.Fatalf("Adopt must keep the proxy on Poros: %+v, active %v", got, next.Active())
	}
	// Applying again must not take Poros's own settings for the user's.
	if err := next.Apply("127.0.0.1", 2090); err != nil {
		t.Fatal(err)
	}
	if err := next.Clear(); err != nil {
		t.Fatal(err)
	}
	if got := b.settings[""]; got != (fakeSetting{flagDirect, ""}) || next.Active() {
		t.Errorf("Clear after Adopt: %+v, want the settings from before Poros", got)
	}
}
