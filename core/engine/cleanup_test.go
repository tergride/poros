package engine

import (
	"errors"
	"slices"
	"testing"
	"time"

	"github.com/sagernet/bbolt"
)

// fakeTrace is a trace of a test: count of it are there until remove takes them, or fails with err.
type fakeTrace struct {
	kind            string
	rights, profile bool
	count           int
	findErr, err    error
	removed         bool
}

func (f *fakeTrace) trace() trace {
	return trace{
		kind:    f.kind,
		rights:  f.rights,
		profile: f.profile,
		find:    func() (int, error) { return f.count, f.findErr },
		remove: func() error {
			if f.err != nil {
				return f.err
			}
			f.removed, f.count = true, 0
			return nil
		},
	}
}

func tracesOf(fakes ...*fakeTrace) func() []trace {
	return func() []trace {
		var traces []trace
		for _, f := range fakes {
			traces = append(traces, f.trace())
		}
		return traces
	}
}

func TestCleanupKnowsTheNetworkOfTheTun(t *testing.T) {
	for name, ours := range map[string]bool{"poros-net": true, "Poros-Net 2": true, "poros-net 12": true, "poros-net2": false, "throne-tun": false, "Сеть 2": false, "poros-netx": false} {
		if got := tunNetwork(name, "poros-net"); got != ours {
			t.Errorf("%q is the network of the TUN: %v", name, got)
		}
	}
}

func TestCleanupScanAddsUpKindsAndHidesTheProfileOfAnotherAccount(t *testing.T) {
	shortcuts := &fakeTrace{kind: CleanupShortcuts, profile: true, count: 2}
	temp := &fakeTrace{kind: CleanupTempFiles, profile: true, count: 1}
	systemTemp := &fakeTrace{kind: CleanupTempFiles, rights: true, count: 2}
	firewall := &fakeTrace{kind: CleanupFirewall, rights: true, count: 3}
	nothing := &fakeTrace{kind: CleanupTunDevice, rights: true}
	unknown := &fakeTrace{kind: CleanupNetworks, rights: true, count: 1, findErr: errors.New("no list")}
	traces := tracesOf(shortcuts, temp, systemTemp, firewall, nothing, unknown)

	got := scanTraces(traces(), false)
	want := []CleanupItem{
		{Kind: CleanupShortcuts, Count: 2},
		{Kind: CleanupTempFiles, Count: 3, Rights: true},
		{Kind: CleanupFirewall, Count: 3, Rights: true},
	}
	if !slices.Equal(got, want) {
		t.Errorf("the scan found %+v", got)
	}

	got = scanTraces(traces(), true)
	want = []CleanupItem{
		{Kind: CleanupTempFiles, Count: 2, Rights: true},
		{Kind: CleanupFirewall, Count: 3, Rights: true},
		{Kind: CleanupProfile},
	}
	if !slices.Equal(got, want) {
		t.Errorf("a core of another account found %+v", got)
	}
	if got := scanTraces(tracesOf(nothing)(), false); got == nil || len(got) != 0 {
		t.Errorf("nothing to remove is an empty list, not %#v", got)
	}
}

func TestCleanupRemovesWhatItMay(t *testing.T) {
	shortcuts := &fakeTrace{kind: CleanupShortcuts, profile: true, count: 2}
	firewall := &fakeTrace{kind: CleanupFirewall, rights: true, count: 3}
	stuck := &fakeTrace{kind: CleanupProxyRecovery, profile: true, count: 1, err: errors.New("access denied")}
	unknown := &fakeTrace{kind: CleanupNetworks, rights: true, findErr: errors.New("no list")}
	traces := tracesOf(shortcuts, firewall, stuck, unknown)

	got := removeTraces(traces(), false, false)
	want := []CleanupItem{
		{Kind: CleanupFirewall, Count: 3, Rights: true},
		{Kind: CleanupProxyRecovery, Count: 1, Error: "access denied"},
		{Kind: CleanupNetworks, Rights: true, Error: "no list"},
	}
	if !slices.Equal(got, want) {
		t.Errorf("without rights stay %+v", got)
	}
	if !shortcuts.removed || firewall.removed {
		t.Errorf("without rights only the shortcuts go: %v, %v", shortcuts.removed, firewall.removed)
	}

	shortcuts.count = 1
	got = removeTraces(tracesOf(shortcuts, firewall)(), true, true)
	if want := []CleanupItem{{Kind: CleanupProfile}}; !slices.Equal(got, want) {
		t.Errorf("a core of another account leaves %+v", got)
	}
	if !firewall.removed || shortcuts.count != 1 {
		t.Error("a core of another account removes what takes rights and leaves the user's profile")
	}
}

func TestCleanupTurnsOffTheStartAndStopsTheKillSwitch(t *testing.T) {
	dir := tempDir(t)
	k := openKillSwitchTest(t, &fakeGuard{}, dir, KillSwitchAlways)
	shortcuts := &fakeTrace{kind: CleanupShortcuts, profile: true, count: 1}
	k.traces = tracesOf(shortcuts)
	settings, _ := k.Settings()
	settings.Autostart = true
	if err := k.db.Update(func(tx *bbolt.Tx) error { return putJSON(tx, bucketSettings, keySettings, settings) }); err != nil {
		t.Fatal(err)
	}
	// The mode the test set brings the refusal up.
	deadline := time.Now().Add(5 * time.Second)
	for !refusing(k.Engine) && time.Now().Before(deadline) {
		time.Sleep(10 * time.Millisecond)
	}
	if !refusing(k.Engine) {
		t.Fatal("the always mode refuses")
	}

	report, err := k.RunCleanup()
	if err != nil {
		t.Fatal(err)
	}
	if len(report.Items) != 0 || !shortcuts.removed {
		t.Errorf("the cleanup leaves %+v", report.Items)
	}
	if refusing(k.Engine) || k.KillSwitchState().On || k.ConnectionState().State != ConnectionIdle {
		t.Errorf("the cleanup stops the refusal: %+v, %+v", k.KillSwitchState(), k.ConnectionState())
	}
	got, _ := k.Settings()
	if got.Autostart {
		t.Errorf("the start with Windows stays on: %+v", got)
	}
	if got.KillSwitch != KillSwitchAlways {
		t.Error("the cleanup changes only the start with Windows")
	}
}

// Windows remembers the network of a TUN that ran, and a core without rights cannot see it: the mark tells it so.
func TestCleanupMarksATunThatRan(t *testing.T) {
	dir := tempDir(t)
	e, _ := openTest(t, dir)
	if e.tunRan() {
		t.Fatal("a new data folder has seen no TUN")
	}
	e.noteTun()
	e.Close()
	e, _ = openTest(t, dir)
	if !e.tunRan() {
		t.Fatal("the mark of a TUN that ran must survive a restart")
	}
	e.forgetTun()
	if e.tunRan() {
		t.Fatal("the removal of the traces forgets the mark")
	}
	e.noteTun()
	if !e.tunRan() {
		t.Fatal("a TUN after the removal marks again")
	}
}
