package engine

import (
	"encoding/json"
	"fmt"
	"maps"
	"math/rand/v2"
	"net"
	"net/http"
	"slices"
	"strconv"
	"strings"
	"sync"
	"testing"
	"time"
)

// concurrencyTally counts what the callers got.
type concurrencyTally struct {
	mu       sync.Mutex
	calls    map[string]int
	reasons  map[string]int
	bare     map[string][]string
	problems []string
}

func (c *concurrencyTally) add(method string, raw []byte, out outcome, reasons map[string]string) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.calls[method]++
	switch {
	case out.hung:
		c.problems = append(c.problems, fmt.Sprintf("%s did not answer in time to %s\n%s", method, shown(raw), goroutineDump()))
	case out.panicked != nil:
		c.problems = append(c.problems, fmt.Sprintf("%s panicked on %s: %v\n%s", method, shown(raw), out.panicked, out.stack))
	case out.err != nil:
		switch class, detail := classify(out.err, reasons); class {
		case errReason:
			c.reasons[method+" "+detail]++
		case errBare, errParams:
			if len(c.bare[method]) < 3 {
				c.bare[method] = append(c.bare[method], fmt.Sprintf("%.200s", detail))
			}
		case errUnknown:
			c.problems = append(c.problems, fmt.Sprintf("%s failed with the reason %q, which api.go does not have", method, detail))
		}
	}
}

// A change of the settings that races the removal of the server it chooses and of its preset never leaves the
// settings naming what is gone: every later settings.set of the interface, which sends the settings whole, would fail
// on them.
func TestRobustSettingsRaceRemovals(t *testing.T) {
	robustOnHost(t)
	rounds := 300
	if robustStress() {
		rounds = 3000
	}
	e, _ := openRobust(t, t.TempDir(), webPage(t))
	group, err := e.AddGroup(AddGroup{Name: "Race"})
	if err != nil {
		t.Fatal(err)
	}
	dangling := 0
	var first string
	for i := range rounds {
		added, err := e.AddServers(AddServers{GroupID: group.ID, Text: trojanTo("127.0.0.1:9", "Race")})
		if err != nil {
			t.Fatal(err)
		}
		preset, err := e.SavePreset(PresetCopy{BasedOn: standardPreset, Name: fmt.Sprintf("Race %d", i)})
		if err != nil {
			t.Fatal(err)
		}
		settings, _ := e.Settings()
		settings.Server, settings.Preset = added.IDs[0], preset.ID
		var wg sync.WaitGroup
		wg.Go(func() { e.SetSettings(settings) })
		wg.Go(func() { e.RemoveServers(ServerIDs{IDs: added.IDs}) })
		wg.Go(func() { e.RemovePreset(preset.ID) })
		wg.Wait()
		stored, err := e.Settings()
		if err != nil {
			t.Fatal(err)
		}
		if err = e.validateSettings(stored); err != nil {
			dangling++
			if first == "" {
				first = err.Error()
			}
			// The interface would choose again.
			stored.Server, stored.Preset = "", standardPreset
			if _, err = e.SetSettings(stored); err != nil {
				t.Fatal(err)
			}
		}
	}
	if dangling > 0 {
		t.Errorf("%d of %d races left the settings naming what is gone, first: %s", dangling, rounds, first)
	}
}

// Dozens of callers change the settings, groups, servers and presets, start and stop the connection, test servers,
// make and restore backups and update a subscription, all at once, as a busy interface and the schedule might.
// Afterwards nothing hangs, the settings pass their own check (the chosen server and preset exist), the groups
// count their servers, the connection settles idle once stopped, and the data opens again.
func TestRobustConcurrency(t *testing.T) {
	robustOnHost(t)
	checkLeaksAfter(t)
	seed := robustSeed(t)
	duration := 3 * time.Second
	if robustStress() {
		duration = 30 * time.Second
	}
	reasons := apiConstants(t, "Reason")

	page := webPage(t)
	live, _ := startTrojanServer(t)
	dead := closerServer(t)
	provider := newProvider(t)
	provider.serve(http.StatusOK, trojanTo(dead, "Sub%20A"), trojanTo(dead, "Sub%20B"))
	dir := t.TempDir()
	e, closeEngine := openRobust(t, dir, page, hostOf(t, provider.url))
	if _, err := e.AddSubscription(t.Context(), AddSubscription{URL: provider.url}); err != nil {
		t.Fatal(err)
	}
	links := strings.Join([]string{live, trojanTo(dead, "Dead"), xhttpTo(dead, "XHTTP")}, "\n")
	mine, err := e.AddServers(AddServers{NewGroup: "Mine", Text: links})
	if err != nil {
		t.Fatal(err)
	}
	for i := range 3 {
		if _, err = e.SavePreset(PresetCopy{BasedOn: standardPreset, Name: fmt.Sprintf("Seed %d", i),
			Rules: []PresetRule{rule(CondSuffix, fmt.Sprintf("seed%d.example", i), ActionDirect)}}); err != nil {
			t.Fatal(err)
		}
	}
	if _, err = e.SaveOTP(totp("GitHub", "JBSWY3DPEHPK3PXP")); err != nil {
		t.Fatal(err)
	}
	settings, _ := e.Settings()
	settings.Server, settings.CheckIP = mine.IDs[0], false
	if _, err = e.SetSettings(settings); err != nil {
		t.Fatal(err)
	}

	tally := &concurrencyTally{calls: map[string]int{}, reasons: map[string]int{}, bare: map[string][]string{}}
	handlers := e.Handlers()
	call := func(method string, params any) (any, error) {
		var raw []byte
		if params != nil {
			raw, _ = json.Marshal(params)
		}
		out := callHandler(handlers[method], raw, 45*time.Second)
		tally.add(method, raw, out, reasons)
		return out.result, out.err
	}
	serverIDs := func() []string {
		servers, _ := e.Servers(ServersQuery{})
		ids := make([]string, len(servers))
		for i, s := range servers {
			ids[i] = s.ID
		}
		return ids
	}
	groupIDs := func(kind string) []string {
		groups, _ := e.Groups()
		var ids []string
		for _, g := range groups {
			if kind == "" || g.Kind == kind {
				ids = append(ids, g.ID)
			}
		}
		return ids
	}
	presetIDs := func(copies bool) []string {
		presets, _ := e.Presets()
		var ids []string
		for _, p := range presets {
			if !copies || !p.Builtin {
				ids = append(ids, p.ID)
			}
		}
		return ids
	}
	pick := func(r *rand.Rand, ids []string) string {
		if len(ids) == 0 {
			return "gone"
		}
		return ids[r.IntN(len(ids))]
	}
	var backupsMu sync.Mutex
	var backups []string

	workers := []struct {
		name string
		n    int
		step func(r *rand.Rand)
	}{
		{"settings", 4, func(r *rand.Rand) {
			s, err := e.Settings()
			if err != nil {
				return
			}
			// TUN, System and the local network stay off: the kill switch runs its refusal on loopback alone.
			switch r.IntN(8) {
			case 0:
				s.Server = pick(r, serverIDs())
			case 1:
				s.Server = ""
			case 2:
				s.Preset = pick(r, presetIDs(false))
			case 3:
				s.KillSwitch = []string{KillSwitchOff, KillSwitchConnected, KillSwitchAlways}[r.IntN(3)]
			case 4:
				s.LogLines = 100 + r.IntN(1000)
			case 5:
				s.DNSCache = !s.DNSCache
			case 6:
				s.IPv6 = !s.IPv6
			case 7:
				s.UIMode = []string{"simple", "advanced"}[r.IntN(2)]
			}
			call(MethodSettingsSet, s)
		}},
		{"groups", 2, func(r *rand.Rand) {
			switch r.IntN(6) {
			case 0:
				call(MethodGroupsAdd, AddGroup{Name: fmt.Sprintf("Group %d", r.IntN(100))})
			case 1:
				call(MethodGroupsRename, RenameGroup{ID: pick(r, groupIDs("")), Name: fmt.Sprintf("Renamed %d", r.IntN(100))})
			case 2:
				call(MethodGroupsRemove, ByID{ID: pick(r, groupIDs(GroupManual))})
			case 3:
				front := ""
				if r.IntN(2) == 0 {
					front = pick(r, serverIDs())
				}
				call(MethodGroupsConfigure, ConfigureGroup{ID: pick(r, groupIDs("")), Settings: GroupSettings{FrontProxy: front}})
			default:
				call(MethodServersAdd, AddServers{NewGroup: "More", Text: links})
			}
		}},
		{"servers", 3, func(r *rand.Rand) {
			switch r.IntN(6) {
			case 0:
				call(MethodServersAdd, AddServers{GroupID: pick(r, groupIDs(GroupManual)), Text: links})
			case 1:
				call(MethodServersRemove, ServerIDs{IDs: []string{pick(r, serverIDs())}})
			case 2:
				call(MethodServersClone, ServerIDs{IDs: []string{pick(r, serverIDs())}})
			case 3:
				call(MethodServersMove, MoveServers{IDs: []string{pick(r, serverIDs())}, GroupID: pick(r, groupIDs(GroupManual))})
			case 4:
				id := pick(r, serverIDs())
				if server, err := e.Server(ByID{ID: id}); err == nil {
					server.Profile.Name = fmt.Sprintf("Edited %d", r.IntN(100))
					call(MethodServersSave, SaveServer{ID: id, Profile: server.Profile})
				}
			default:
				call(MethodServersClean, CleanServers{Duplicates: true})
			}
		}},
		{"presets", 2, func(r *rand.Rand) {
			switch r.IntN(4) {
			case 0:
				call(MethodPresetsSave, PresetCopy{BasedOn: standardPreset, Name: fmt.Sprintf("Copy %d", r.IntN(1000)),
					Rules: []PresetRule{rule(CondSuffix, fmt.Sprintf("s%d.example", r.IntN(100)), ActionDirect)}})
			case 1:
				if c, err := e.PresetCopy(pick(r, presetIDs(true))); err == nil && !c.Builtin {
					c.Rules = append(c.Rules, rule(CondSuffix, fmt.Sprintf("r%d.example", r.IntN(1000)), ActionBlock))
					call(MethodPresetsSave, c)
				}
			case 2:
				call(MethodPresetsRemove, ByID{ID: pick(r, presetIDs(true))})
			default:
				if link, err := e.ExportPreset(pick(r, presetIDs(true))); err == nil {
					call(MethodPresetsImport, link)
				}
			}
		}},
		{"connection", 2, func(r *rand.Rand) {
			if r.IntN(2) == 0 {
				call(MethodConnectionStart, nil)
			} else {
				call(MethodConnectionStop, nil)
			}
			time.Sleep(time.Duration(r.IntN(80)) * time.Millisecond)
		}},
		{"tests", 1, func(r *rand.Rand) {
			switch r.IntN(4) {
			case 0, 1:
				call(MethodServersTest, ServersQuery{GroupID: pick(r, groupIDs(""))})
			case 2:
				call(MethodServersStopTests, nil)
			default:
				call(MethodServersClearResults, ServersQuery{})
			}
			time.Sleep(50 * time.Millisecond)
		}},
		{"backup", 1, func(r *rand.Rand) {
			if result, err := call(MethodBackupCreate, BackupParts{Parts: backupParts}); err == nil {
				backupsMu.Lock()
				backups = append(backups[max(0, len(backups)-2):], result.(BackupFile).Text)
				backupsMu.Unlock()
			}
			backupsMu.Lock()
			text := ""
			if len(backups) > 0 {
				text = backups[r.IntN(len(backups))]
			}
			backupsMu.Unlock()
			var parts []string
			for _, part := range backupParts {
				if r.IntN(2) == 0 {
					parts = append(parts, part)
				}
			}
			if text != "" {
				call(MethodBackupRestore, BackupRestore{Text: text, Parts: parts})
			}
			time.Sleep(30 * time.Millisecond)
		}},
		{"subscription", 1, func(r *rand.Rand) {
			if subs := groupIDs(GroupSubscription); len(subs) > 0 {
				call(MethodSubscriptionsUpdate, ByID{ID: subs[0]})
			} else {
				call(MethodSubscriptionsAdd, AddSubscription{URL: provider.url})
			}
			time.Sleep(30 * time.Millisecond)
		}},
		{"otp", 1, func(r *rand.Rand) {
			codes, _ := e.OTPCodes()
			ids := make([]string, len(codes))
			for i, c := range codes {
				ids[i] = c.ID
			}
			switch r.IntN(4) {
			case 0:
				call(MethodOTPSave, totp(fmt.Sprintf("Code %d", r.IntN(100)), "JBSWY3DPEHPK3PXP"))
			case 1:
				call(MethodOTPCode, ByID{ID: pick(r, ids)})
			case 2:
				call(MethodOTPMove, OTPMove{ID: pick(r, ids), Offset: 1})
			default:
				if len(ids) > 3 {
					call(MethodOTPRemove, ByID{ID: pick(r, ids)})
				}
			}
		}},
		{"readers", 2, func(r *rand.Rand) {
			for _, method := range []string{MethodSettingsGet, MethodGroupsList, MethodPresetsList, MethodConnectionState,
				MethodStatsNow, MethodConnectionsList, MethodKillSwitchState, MethodOTPList, MethodAppDebugInfo} {
				call(method, nil)
			}
			call(MethodServersList, ServersQuery{})
		}},
		{"traffic", 1, func(r *rand.Rand) {
			_, _ = fetchThrough(e, page)
			time.Sleep(20 * time.Millisecond)
		}},
	}

	deadline := time.Now().Add(duration)
	var wg sync.WaitGroup
	callers := 0
	for _, w := range workers {
		for i := range w.n {
			callers++
			stream := uint64(i)
			for _, c := range w.name {
				stream = stream*31 + uint64(c)
			}
			r := rand.New(rand.NewPCG(seed, stream))
			wg.Go(func() {
				for time.Now().Before(deadline) {
					w.step(r)
				}
			})
		}
	}
	done := make(chan struct{})
	go func() {
		wg.Wait()
		close(done)
	}()
	select {
	case <-done:
	case <-time.After(duration + 90*time.Second):
		t.Fatalf("the callers did not finish 90 s after their time: a deadlock?\n%s", goroutineDump())
	}

	tally.mu.Lock()
	total := 0
	for _, n := range tally.calls {
		total += n
	}
	var counts []string
	for _, method := range slices.Sorted(maps.Keys(tally.calls)) {
		counts = append(counts, method+" "+strconv.Itoa(tally.calls[method]))
	}
	var reasonCounts []string
	for _, key := range slices.Sorted(maps.Keys(tally.reasons)) {
		reasonCounts = append(reasonCounts, fmt.Sprintf("%s %d", key, tally.reasons[key]))
	}
	t.Logf("%d callers, %d calls in %s: %s", callers, total, duration, strings.Join(counts, ", "))
	t.Logf("failures with a reason: %s", strings.Join(reasonCounts, ", "))
	for _, method := range slices.Sorted(maps.Keys(tally.bare)) {
		t.Logf("%s failed without a reason: %s", method, strings.Join(tally.bare[method], " | "))
	}
	for _, problem := range tally.problems {
		t.Error(problem)
	}
	tally.mu.Unlock()

	// The kill switch goes off and the connection stops: nothing may stay up.
	settings, err = e.Settings()
	if err != nil {
		t.Fatal(err)
	}
	settings.KillSwitch = KillSwitchOff
	if _, err = e.SetSettings(settings); err != nil {
		t.Errorf("the kill switch off: %v", err)
	}
	e.StopConnection()
	waitIdle(t, e, 30*time.Second)
	stopTestsWithin(t, e, 30*time.Second)
	if state := e.KillSwitchState(); state.On {
		t.Errorf("the kill switch still holds after the stop: %+v", state)
	}
	if !portFree(net.JoinHostPort("127.0.0.1", strconv.Itoa(int(e.mixedPort)))) {
		t.Error("the port of the local proxy is still taken after the stop")
	}
	robustCheckData(t, e)
	closeEngine()
	reopened, closeReopened := openRobust(t, dir, page)
	robustCheckData(t, reopened)
	closeReopened()
}
