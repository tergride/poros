package engine

import (
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"PorosCore/internal/boxmain"
)

func TestRenameGroup(t *testing.T) {
	e, _ := openTest(t, t.TempDir())
	p := newProvider(t)
	p.serve(http.StatusOK, vless("de.example.com", "DE"))
	p.headers["profile-title"] = "Provider"
	update, err := e.AddSubscription(t.Context(), AddSubscription{URL: p.url})
	if err != nil {
		t.Fatal(err)
	}
	if sub, err := e.RenameGroup(RenameGroup{ID: update.Group.ID, Name: " Mine "}); err != nil || sub.Name != "Mine" {
		t.Fatalf("rename: %+v, %v", sub, err)
	}
	if sub, err := e.RenameGroup(RenameGroup{ID: update.Group.ID}); err != nil || sub.Name != "Provider" {
		t.Fatalf("an empty name goes back to the provider's: %+v, %v", sub, err)
	}
	if _, err = e.RenameGroup(RenameGroup{ID: "nope", Name: "x"}); reason(err) != ReasonNotFound {
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
		if _, err = e.UpdateSubscription(t.Context(), update.Group.ID); err != nil {
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
	if chunk, err := e.Logs(LogsQuery{}); err != nil || len(chunk.Lines) != 0 {
		t.Fatalf("no log yet: %+v, %v", chunk, err)
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
	chunk, err := e.Logs(LogsQuery{})
	lines := chunk.Lines
	if err != nil || len(lines) != defaultLogLines || lines[0] != "line 200" || lines[len(lines)-1] != "line 699" || chunk.Reset {
		t.Fatalf("logs: %d lines, %q … %q, %v", len(lines), lines[0], lines[len(lines)-1], err)
	}
	// What comes after: whole lines alone, the one being written with the next query.
	log, err := os.OpenFile(filepath.Join(dir, "logs", "core.log"), os.O_APPEND|os.O_WRONLY, 0)
	if err != nil {
		t.Fatal(err)
	}
	log.WriteString("line 700\r\nline 7")
	next, err := e.Logs(LogsQuery{After: chunk.Next})
	if err != nil || len(next.Lines) != 1 || next.Lines[0] != "line 700" || next.Reset {
		t.Fatalf("the next lines: %+v, %v", next, err)
	}
	log.WriteString("01\n")
	log.Close()
	if last, err := e.Logs(LogsQuery{After: next.Next}); err != nil || len(last.Lines) != 1 || last.Lines[0] != "line 701" {
		t.Fatalf("the line written in two parts: %+v, %v", last, err)
	}
	// A log shorter than asked began anew.
	if again, err := e.Logs(LogsQuery{After: 1 << 30}); err != nil || !again.Reset || len(again.Lines) != defaultLogLines {
		t.Fatalf("a log begun anew: %d lines, reset %v, %v", len(again.Lines), again.Reset, err)
	}
	// The settings say how many.
	settings, _ := e.Settings()
	settings.LogLines = 100
	if _, err = e.SetSettings(settings); err != nil {
		t.Fatal(err)
	}
	if chunk, err = e.Logs(LogsQuery{}); err != nil || len(chunk.Lines) != 100 || chunk.Lines[0] != "line 602" {
		t.Fatalf("100 lines: %d, %v", len(chunk.Lines), err)
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
	servers, _ := e.Servers(ServersQuery{GroupID: update.Group.ID})
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
	if _, err = e.UpdateSubscription(t.Context(), update.Group.ID); err != nil {
		t.Fatalf("an update through the connection: %v", err)
	}
	// With the server gone the update fails: it went through the connection, not around it.
	stop()
	if _, err = e.UpdateSubscription(t.Context(), update.Group.ID); reason(err) != ReasonFetchFailed {
		t.Fatalf("the update went around the connection: %v", err)
	}
}
