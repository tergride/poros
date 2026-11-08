package engine

import (
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"reflect"
	"slices"
	"strings"
	"sync"
	"testing"
	"time"

	"PorosCore/config"
	"PorosCore/internal/boxmain"
)

func TestPresetCopies(t *testing.T) {
	e, _ := openTest(t, t.TempDir())
	draft, err := e.PresetCopy("all")
	if err != nil || draft.ID != "" || draft.BasedOn != "all" || len(draft.Inherited) != 1 || draft.Inherited[0].Action != ActionDirect {
		t.Fatalf("a new copy of all: %+v, %v", draft, err)
	}
	draft.Name = "Home"
	draft.Sites = []PresetEntry{
		{Value: "https://Video.Example.com/watch", Action: ActionProxy},
		{Value: "*.ads.example", Action: ActionBlock},
		{Value: "192.168.50.0/24", Action: ActionDirect},
		{Value: "203.0.113.7", Action: ActionDirect},
	}
	draft.Apps = []PresetEntry{{Value: "steam.exe", Action: ActionDirect}, {Value: `C:\Games\game.exe`, Action: ActionProxy}}
	draft.DNS = "https://1.1.1.1/dns-query"
	saved, err := e.SavePreset(draft)
	if err != nil || saved.ID == "" || saved.Sites != 4 || saved.Apps != 2 || saved.BasedOn != "all" {
		t.Fatalf("SavePreset: %+v, %v", saved, err)
	}

	preset, found, err := e.preset(saved.ID)
	if err != nil || !found {
		t.Fatalf("preset: %v, %v", found, err)
	}
	if preset.DNS != draft.DNS || len(preset.Rules) != 8 {
		t.Fatalf("composed: %+v", preset)
	}
	// DNS stays first, the user's entries come before the base's own rules: domains, addresses, then programs.
	first, video, ads, lan, local := preset.Rules[0], preset.Rules[1], preset.Rules[2], preset.Rules[4], preset.Rules[7]
	if first.Action != config.ActionHijackDNS || local.Name != "Local networks" ||
		!slices.Equal(video.DomainSuffix, []string{"video.example.com"}) || video.Outbound != config.OutboundProxy ||
		!slices.Equal(ads.DomainSuffix, []string{"ads.example"}) || ads.Action != config.ActionReject ||
		!slices.Equal(lan.IPCIDR, []string{"192.168.50.0/24"}) {
		t.Errorf("rules: %+v", preset.Rules)
	}
	if !slices.Equal(preset.Rules[5].ProcessPath, []string{`C:\Games\game.exe`}) || !slices.Equal(preset.Rules[6].ProcessName, []string{"steam.exe"}) {
		t.Errorf("programs: %+v", preset.Rules[5:7])
	}

	settings := DefaultSettings()
	settings.Preset = saved.ID
	if _, err = e.SetSettings(settings); err != nil {
		t.Fatalf("a copy is a preset to choose: %v", err)
	}

	link, err := e.ExportPreset(saved.ID)
	if err != nil || !strings.HasPrefix(link.Link, "poros://route/") {
		t.Fatalf("ExportPreset: %v, %v", link, err)
	}
	imported, err := e.ImportPreset(link)
	if err != nil || imported.ID == saved.ID || imported.Name != "Home 2" || imported.Sites != 4 {
		t.Fatalf("ImportPreset: %+v, %v", imported, err)
	}
	for _, bad := range []string{"https://example.com", "poros://route/!!!", "poros://route/" + strings.Repeat("A", 8)} {
		if _, err = e.ImportPreset(PresetLink{Link: bad}); reason(err) != ReasonInvalidLink {
			t.Errorf("%q: %v", bad, err)
		}
	}
	for _, bad := range []PresetEntry{{Value: "not a site", Action: ActionProxy}, {Value: "example.com", Action: "elsewhere"}} {
		broken := draft
		broken.Sites = []PresetEntry{bad}
		if _, err = e.SavePreset(broken); reason(err) != ReasonInvalidEntry {
			t.Errorf("%+v: %v", bad, err)
		}
	}

	// A connection on a removed copy goes back to the copy's base.
	if err = e.RemovePreset(saved.ID); err != nil {
		t.Fatal(err)
	}
	if settings, _ = e.Settings(); settings.Preset != "all" {
		t.Errorf("after the removal the preset is %q", settings.Preset)
	}
	presets, _ := e.Presets()
	if len(presets) != 3 || presets[2].ID != imported.ID {
		t.Errorf("presets: %+v", presets)
	}
}

// The first rule that matches decides, so a narrower entry goes first whatever the order the user added them in:
// a block of a subdomain works under a domain through the server, and so does an exception in a subnet.
func TestNarrowerEntriesFirst(t *testing.T) {
	e, _ := openTest(t, t.TempDir())
	draft, _ := e.PresetCopy("all")
	draft.Name = "Order"
	draft.Sites = []PresetEntry{
		{Value: "example.com", Action: ActionProxy},
		{Value: "10.0.0.0/8", Action: ActionDirect},
		{Value: "*.ads.example.com", Action: ActionBlock},
		{Value: "a.example", Action: ActionDirect},
		{Value: "b.example", Action: ActionProxy},
		{Value: "10.1.0.0/16", Action: ActionProxy},
		{Value: "10.1.2.3", Action: ActionBlock},
	}
	draft.Apps = []PresetEntry{{Value: "steam.exe", Action: ActionDirect}, {Value: `C:\Games\steam.exe`, Action: ActionProxy}}
	saved, err := e.SavePreset(draft)
	if err != nil {
		t.Fatal(err)
	}
	preset, _, err := e.preset(saved.ID)
	if err != nil {
		t.Fatal(err)
	}
	var order []string
	for _, rule := range preset.Rules[1:10] {
		order = append(order, rule.Name)
	}
	want := []string{"*.ads.example.com", "example.com", "a.example", "b.example", "10.1.2.3", "10.1.0.0/16", "10.0.0.0/8", `C:\Games\steam.exe`, "steam.exe"}
	if !slices.Equal(order, want) {
		t.Errorf("order:\n%q\nwant\n%q", order, want)
	}
	// The editor keeps the order the user added them in.
	if stored, _ := e.PresetCopy(saved.ID); !reflect.DeepEqual(stored.Sites, draft.Sites) {
		t.Errorf("stored: %+v", stored.Sites)
	}
}

// Built-in presets never change: the editor saves a new preset instead, and their IDs are neither saved over nor
// removed.
func TestBuiltinPresetsStay(t *testing.T) {
	e, _ := openTest(t, t.TempDir())
	before, _, _ := e.preset("all")
	draft, _ := e.PresetCopy("all")
	draft.ID, draft.Name = "all", "Changed"
	draft.Sites = []PresetEntry{{Value: "example.org", Action: ActionBlock}}
	if _, err := e.SavePreset(draft); reason(err) != ReasonNotFound {
		t.Errorf("saving over a built-in preset: %v", err)
	}
	if err := e.RemovePreset("hardened"); reason(err) != ReasonNotFound {
		t.Errorf("removing a built-in preset: %v", err)
	}
	if after, _, _ := e.preset("all"); !reflect.DeepEqual(before, after) {
		t.Errorf("the built-in preset changed: %+v", after)
	}
	if presets, _ := e.Presets(); len(presets) != 2 {
		t.Errorf("presets: %+v", presets)
	}
}

// A preset made from scratch has no base: it shows none, and runs on the standard rules of all traffic through the
// server, exactly as a copy of "all" with the same entries would.
func TestPresetFromScratch(t *testing.T) {
	e, _ := openTest(t, t.TempDir())
	draft, err := e.PresetCopy("")
	if err != nil || draft.BasedOn != "" || draft.Name != "" || len(draft.Inherited) != 1 {
		t.Fatalf("a new preset: %+v, %v", draft, err)
	}
	draft.Name = "Work"
	draft.Sites = []PresetEntry{{Value: "example.org", Action: ActionDirect}}
	saved, err := e.SavePreset(draft)
	if err != nil || saved.BasedOn != "" || saved.Sites != 1 {
		t.Fatalf("SavePreset: %+v, %v", saved, err)
	}
	copyOfAll, _ := e.PresetCopy("all")
	copyOfAll.Name, copyOfAll.Sites = "Work too", draft.Sites
	twin, err := e.SavePreset(copyOfAll)
	if err != nil {
		t.Fatal(err)
	}
	own, _, err := e.preset(saved.ID)
	if err != nil {
		t.Fatal(err)
	}
	same, _, _ := e.preset(twin.ID)
	// Two presets cannot share a name; the rest is the same.
	same.Name = own.Name
	if !reflect.DeepEqual(own, same) {
		t.Errorf("from scratch %+v, a copy of all %+v", own, same)
	}
	if edited, err := e.PresetCopy(saved.ID); err != nil || edited.BasedOn != "" || len(edited.Inherited) != 1 {
		t.Errorf("the editor: %+v, %v", edited, err)
	}

	link, _ := e.ExportPreset(saved.ID)
	if imported, err := e.ImportPreset(link); err != nil || imported.BasedOn != "" {
		t.Errorf("a link keeps no base: %+v, %v", imported, err)
	}
	settings := DefaultSettings()
	settings.Preset = saved.ID
	if _, err = e.SetSettings(settings); err != nil {
		t.Fatal(err)
	}
	if err = e.RemovePreset(saved.ID); err != nil {
		t.Fatal(err)
	}
	if settings, _ = e.Settings(); settings.Preset != "all" {
		t.Errorf("after the removal the preset is %q, want the standard one", settings.Preset)
	}
}

// The traffic no entry takes goes where the user says, and the choice travels with the preset's link.
func TestPresetDefault(t *testing.T) {
	e, _ := openTest(t, t.TempDir())
	draft, err := e.PresetCopy("all")
	if err != nil || draft.Default != ActionProxy {
		t.Fatalf("a copy of all starts through the server: %+v, %v", draft, err)
	}
	draft.Name, draft.Default = "Direct but", ActionDirect
	draft.Sites = []PresetEntry{{Value: "example.org", Action: ActionProxy}}
	saved, err := e.SavePreset(draft)
	if err != nil {
		t.Fatal(err)
	}
	if preset, _, _ := e.preset(saved.ID); preset.DefaultOutbound != config.OutboundDirect {
		t.Errorf("composed default %q", preset.DefaultOutbound)
	}
	if edited, _ := e.PresetCopy(saved.ID); edited.Default != ActionDirect {
		t.Errorf("the editor shows %q", edited.Default)
	}
	link, _ := e.ExportPreset(saved.ID)
	imported, err := e.ImportPreset(link)
	if err != nil {
		t.Fatal(err)
	}
	if preset, _, _ := e.preset(imported.ID); preset.DefaultOutbound != config.OutboundDirect {
		t.Errorf("the link lost the default: %q", preset.DefaultOutbound)
	}
	draft.Default = "elsewhere"
	if _, err = e.SavePreset(draft); reason(err) != ReasonInvalidValue {
		t.Errorf("an unknown default: %v", err)
	}
}

func TestRenameSubscription(t *testing.T) {
	e, _ := openTest(t, t.TempDir())
	p := newProvider(t)
	p.serve(http.StatusOK, vless("de.example.com", "DE"))
	p.headers["profile-title"] = "Provider"
	update, err := e.AddSubscription(t.Context(), AddSubscription{URL: p.url})
	if err != nil {
		t.Fatal(err)
	}
	if sub, err := e.RenameSubscription(RenameSubscription{ID: update.Subscription.ID, Name: " Mine "}); err != nil || sub.Name != "Mine" {
		t.Fatalf("rename: %+v, %v", sub, err)
	}
	if sub, err := e.RenameSubscription(RenameSubscription{ID: update.Subscription.ID}); err != nil || sub.Name != "Provider" {
		t.Fatalf("an empty name goes back to the provider's: %+v, %v", sub, err)
	}
	if _, err = e.RenameSubscription(RenameSubscription{ID: "nope", Name: "x"}); reason(err) != ReasonNotFound {
		t.Errorf("unknown subscription: %v", err)
	}
}

func TestDeviceInfoOnlyWhenAllowed(t *testing.T) {
	e, _ := openTest(t, t.TempDir())
	p := newProvider(t)
	p.serve(http.StatusOK, vless("de.example.com", "DE"))
	update, err := e.AddSubscription(t.Context(), AddSubscription{URL: p.url})
	if err != nil {
		t.Fatal(err)
	}
	settings := DefaultSettings()
	settings.SendDeviceInfo = true
	if _, err = e.SetSettings(settings); err != nil {
		t.Fatal(err)
	}
	for range 2 {
		if _, err = e.UpdateSubscription(t.Context(), update.Subscription.ID); err != nil {
			t.Fatal(err)
		}
	}
	p.mu.Lock()
	defer p.mu.Unlock()
	if p.hwids[0] != "" || p.hwids[1] == "" || p.hwids[1] != p.hwids[2] {
		t.Errorf("the device ID goes only with consent and stays the same: %q", p.hwids)
	}
}

func TestLogsGiveTheLastLines(t *testing.T) {
	dir := t.TempDir()
	e, _ := openTest(t, dir)
	if lines, err := e.Logs(); err != nil || len(lines) != 0 {
		t.Fatalf("no log yet: %v, %v", lines, err)
	}
	var text strings.Builder
	for i := range 700 {
		fmt.Fprintf(&text, "line %d\r\n", i)
	}
	if err := os.MkdirAll(filepath.Join(dir, "logs"), 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "logs", "core.log"), []byte(text.String()), 0o600); err != nil {
		t.Fatal(err)
	}
	lines, err := e.Logs()
	if err != nil || len(lines) != logLines || lines[0] != "line 200" || lines[len(lines)-1] != "line 699" {
		t.Fatalf("logs: %d lines, %q … %q, %v", len(lines), lines[0], lines[len(lines)-1], err)
	}
}

// startTrojanServer runs a Trojan server on loopback; stop takes it down.
func startTrojanServer(t *testing.T) (link string, stop func()) {
	t.Helper()
	port := freePort(t)
	server, cancel, err := boxmain.Create([]byte(fmt.Sprintf(`{"log":{"level":"warn"},`+
		`"inbounds":[{"type":"trojan","listen":"127.0.0.1","listen_port":%d,"users":[{"password":"secret"}]}],`+
		`"outbounds":[{"type":"direct","tag":"direct"}]}`, port)), nil)
	if err != nil {
		t.Fatalf("the test server: %v", err)
	}
	var once sync.Once
	stop = func() { once.Do(func() { cancel(); server.Close() }) }
	t.Cleanup(stop)
	return fmt.Sprintf("trojan://secret@127.0.0.1:%d?security=none#local", port), stop
}

func TestLatencyAndTraffic(t *testing.T) {
	var mu sync.Mutex
	var tested []ServerLatency
	var traffic []Traffic
	done := make(chan struct{}, 1)
	e, states := openConnectionTestWith(t, func(method string, params any) {
		mu.Lock()
		defer mu.Unlock()
		switch method {
		case EventServersTested:
			report := params.(ServersTested)
			tested = append(tested, report.Results...)
			if report.Done {
				done <- struct{}{}
			}
		case EventConnectionTraffic:
			traffic = append(traffic, params.(Traffic))
		}
	})
	page := webPage(t)
	e.latencyURL = page
	link, _ := startTrojanServer(t)
	dead := fmt.Sprintf("trojan://secret@127.0.0.1:%d?security=none#dead", freePort(t))
	p := newProvider(t)
	p.serve(http.StatusOK, link, dead)
	update, err := e.AddSubscription(t.Context(), AddSubscription{URL: p.url})
	if err != nil {
		t.Fatal(err)
	}
	if _, err = e.TestServers(ServersQuery{}); err != nil {
		t.Fatal(err)
	}
	select {
	case <-done:
	case <-time.After(30 * time.Second):
		t.Fatal("the delay test did not finish")
	}
	servers, _ := e.Servers(ServersQuery{SubscriptionID: update.Subscription.ID})
	if len(servers) != 2 || servers[0].Latency <= 0 || servers[1].Latency != -1 {
		t.Fatalf("latency: %+v, reported %+v", servers, tested)
	}

	settings := DefaultSettings()
	settings.Server, settings.Preset = servers[0].ID, "through"
	if _, err = e.SetSettings(settings); err != nil {
		t.Fatal(err)
	}
	connectAndFetch(t, e, states, page)
	deadline := time.Now().Add(5 * time.Second)
	for {
		mu.Lock()
		var total int64
		if len(traffic) > 0 {
			total = traffic[len(traffic)-1].TotalDownload
		}
		mu.Unlock()
		if total > 0 {
			break
		}
		if time.Now().After(deadline) {
			t.Fatal("no traffic through the server was counted")
		}
		time.Sleep(200 * time.Millisecond)
	}
}

func TestSubscriptionThroughTheConnection(t *testing.T) {
	e, states := openConnectionTest(t)
	link, stop := startTrojanServer(t)
	p := newProvider(t)
	p.serve(http.StatusOK, link)
	update, err := e.AddSubscription(t.Context(), AddSubscription{URL: p.url})
	if err != nil {
		t.Fatal(err)
	}
	servers, _ := e.Servers(ServersQuery{})
	settings := DefaultSettings()
	settings.Server, settings.Preset, settings.SubscriptionViaProxy = servers[0].ID, "through", true
	if _, err = e.SetSettings(settings); err != nil {
		t.Fatal(err)
	}
	connectAndFetch(t, e, states, webPage(t))
	if _, err = e.UpdateSubscription(t.Context(), update.Subscription.ID); err != nil {
		t.Fatalf("an update through the connection: %v", err)
	}
	// With the server gone the update fails: it went through the connection, not around it.
	stop()
	if _, err = e.UpdateSubscription(t.Context(), update.Subscription.ID); reason(err) != ReasonFetchFailed {
		t.Fatalf("the update went around the connection: %v", err)
	}
}
