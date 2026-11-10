package engine

import (
	"fmt"
	"io"
	"net"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strconv"
	"testing"
	"time"

	"PorosCore/config"
	"PorosCore/internal/boxmain"
	"PorosCore/internal/marker"
	"PorosCore/internal/xray"
)

// The connection tests run real cores on loopback: a server, Poros's mixed inbound on a free port, and a web
// page behind them. TUN and the system proxy stay off, as on any check on the host.

// freePort finds a port free on loopback for TCP and UDP both: Windows keeps ranges of UDP ports for itself that
// are free for TCP, and a server on QUIC or WireGuard would fail to start on one of them. The port comes from UDP,
// since Windows hands out TCP ports in a row and twenty of them can all fall into one such range.
func freePort(t *testing.T) uint16 {
	t.Helper()
	var last error
	for range 50 {
		packet, err := net.ListenPacket("udp", "127.0.0.1:0")
		if err != nil {
			t.Fatal(err)
		}
		port := packet.LocalAddr().(*net.UDPAddr).Port
		listener, err := net.Listen("tcp", net.JoinHostPort("127.0.0.1", strconv.Itoa(port)))
		packet.Close()
		if err == nil {
			listener.Close()
			return uint16(port)
		}
		last = err
	}
	t.Fatalf("no port is free for both TCP and UDP: %v", last)
	return 0
}

func openConnectionTest(t *testing.T) (*Engine, chan ConnectionState) {
	return openConnectionTestWith(t, nil)
}

// openConnectionTestWith hands the events other than connection.changed to other.
func openConnectionTestWith(t *testing.T, other Notify) (*Engine, chan ConnectionState) {
	t.Helper()
	states := make(chan ConnectionState, 64)
	e, err := Open(Config{DataDir: t.TempDir(), MixedPort: freePort(t), Notify: func(method string, params any) {
		switch {
		case method == EventConnectionChanged:
			states <- params.(ConnectionState)
		case other != nil:
			other(method, params)
		}
	}})
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	t.Cleanup(func() { e.Close() })
	// Everything through the server, loopback included: the built-in presets send private addresses direct.
	e.presets = append(e.presets, presetRecord{ID: "through", Preset: config.Preset{Name: "Through", DefaultOutbound: config.OutboundProxy}})
	return e, states
}

// waitState waits for the connection to settle: running or idle.
func waitState(t *testing.T, states chan ConnectionState) ConnectionState {
	t.Helper()
	timeout := time.After(30 * time.Second)
	for {
		select {
		case state := <-states:
			if state.State == ConnectionRunning || state.State == ConnectionIdle {
				return state
			}
		case <-timeout:
			t.Fatal("the connection did not settle in 30 seconds")
		}
	}
}

// useServer adds a subscription with one server and picks it.
func useServer(t *testing.T, e *Engine, link string) {
	t.Helper()
	p := newProvider(t)
	p.serve(http.StatusOK, link)
	update, err := e.AddSubscription(t.Context(), AddSubscription{URL: p.url})
	if err != nil {
		t.Fatalf("AddSubscription: %v", err)
	}
	servers, err := e.Servers(ServersQuery{GroupID: update.Group.ID})
	if err != nil || len(servers) != 1 {
		t.Fatalf("servers: %v, %v", servers, err)
	}
	settings := DefaultSettings()
	settings.Server, settings.Preset = servers[0].ID, "through"
	if _, err = e.SetSettings(settings); err != nil {
		t.Fatal(err)
	}
}

func webPage(t *testing.T) string {
	t.Helper()
	page := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		io.WriteString(w, "through the server")
	}))
	t.Cleanup(page.Close)
	return page.URL
}

func fetchThrough(e *Engine, page string) (string, error) {
	proxy := &url.URL{Scheme: "http", Host: net.JoinHostPort("127.0.0.1", strconv.Itoa(int(e.mixedPort)))}
	client := &http.Client{Timeout: 10 * time.Second, Transport: &http.Transport{Proxy: http.ProxyURL(proxy), DisableKeepAlives: true}}
	response, err := client.Get(page)
	if err != nil {
		return "", err
	}
	defer response.Body.Close()
	if response.StatusCode != http.StatusOK {
		return "", fmt.Errorf("status %s", response.Status)
	}
	body, err := io.ReadAll(response.Body)
	return string(body), err
}

func connectAndFetch(t *testing.T, e *Engine, states chan ConnectionState, page string) {
	t.Helper()
	if state, _ := e.StartConnection(); state.State != ConnectionStarting {
		t.Fatalf("connection.start answers with the starting state, got %+v", state)
	}
	if state := waitState(t, states); state.State != ConnectionRunning || state.Error != "" {
		t.Fatalf("the connection did not come up: %+v", state)
	}
	if body, err := fetchThrough(e, page); err != nil || body != "through the server" {
		t.Fatalf("fetch through Poros: %q, %v", body, err)
	}
}

func TestConnectionThroughSingBox(t *testing.T) {
	e, states := openConnectionTest(t)
	page := webPage(t)
	port := freePort(t)
	server, cancel, err := boxmain.Create([]byte(fmt.Sprintf(`{"log":{"level":"warn"},`+
		`"inbounds":[{"type":"trojan","listen":"127.0.0.1","listen_port":%d,"users":[{"password":"secret"}]}],`+
		`"outbounds":[{"type":"direct","tag":"direct"}]}`, port)), nil)
	if err != nil {
		t.Fatalf("the test server: %v", err)
	}
	useServer(t, e, fmt.Sprintf("trojan://secret@127.0.0.1:%d?security=none#local", port))

	connectAndFetch(t, e, states, page)
	// A start while running reconnects.
	connectAndFetch(t, e, states, page)

	cancel()
	server.Close()
	if _, err = fetchThrough(e, page); err == nil {
		t.Fatal("with the server gone the page must not open: the traffic went around it")
	}
	e.StopConnection()
	if state := waitState(t, states); state.State != ConnectionIdle || state.Error != "" {
		t.Fatalf("stop: %+v", state)
	}
	if !portFree(net.JoinHostPort("127.0.0.1", strconv.Itoa(int(e.mixedPort)))) {
		t.Fatal("a stopped connection must free its port")
	}
}

func TestConnectionThroughXray(t *testing.T) {
	e, states := openConnectionTest(t)
	page := webPage(t)
	port := freePort(t)
	server, err := xray.CreateXrayInstance(fmt.Sprintf(`{"log":{"loglevel":"warning"},`+
		`"inbounds":[{"listen":"127.0.0.1","port":%d,"protocol":"vless","settings":{"clients":[{"id":"%s"}],"decryption":"none"},`+
		`"streamSettings":{"network":"xhttp","xhttpSettings":{"path":"/x"}}}],`+
		// Xray's freedom refuses private targets unless told otherwise; the page is on loopback.
		`"outbounds":[{"protocol":"freedom","settings":{"finalRules":[{"action":"allow","ip":["127.0.0.0/8"]}]}}]}`, port, testUUID))
	if err != nil {
		t.Fatalf("the test server: %v", err)
	}
	if err = server.Start(); err != nil {
		t.Fatalf("the test server: %v", err)
	}
	t.Cleanup(func() { server.Close() })
	useServer(t, e, fmt.Sprintf("vless://%s@127.0.0.1:%d?type=xhttp&path=%%2Fx&security=none#xhttp", testUUID, port))

	connectAndFetch(t, e, states, page)
	e.StopConnection()
	if state := waitState(t, states); state.State != ConnectionIdle {
		t.Fatalf("stop: %+v", state)
	}
}

// Stop during a start that hangs, here on the lookup of the server, ends it at once instead of after the timeout.
func TestStopCancelsAStart(t *testing.T) {
	e, states := openConnectionTest(t)
	asked := make(chan struct{}, 4)
	doh := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		asked <- struct{}{}
		<-r.Context().Done()
	}))
	defer doh.Close()
	servers, client := dohServers, dohClient
	t.Cleanup(func() { dohServers, dohClient = servers, client })
	dohServers, dohClient = []string{doh.URL}, doh.Client
	useServer(t, e, fmt.Sprintf("vless://%s@slow.poros-test.invalid:443?type=xhttp&path=%%2Fx&security=none#slow", testUUID))

	began := time.Now()
	e.StartConnection()
	select {
	case <-asked:
	case <-time.After(10 * time.Second):
		t.Fatal("the start did not get to the lookup")
	}
	e.StopConnection()
	if state := waitState(t, states); state.State != ConnectionIdle || state.Error != "" {
		t.Fatalf("a cancelled start ends idle without an error: %+v", state)
	}
	if took := time.Since(began); took > resolveTimeout {
		t.Errorf("the start was not cancelled: it took %s", took)
	}
	e.conn.lifecycle.Lock()
	running := e.conn.running
	e.conn.lifecycle.Unlock()
	if running != nil {
		t.Error("a cancelled start left a connection")
	}
}

func TestConnectionRefusals(t *testing.T) {
	e, states := openConnectionTest(t)
	e.StartConnection()
	if state := waitState(t, states); state.Error != ReasonNoServer {
		t.Errorf("a start without a server: %+v", state)
	}

	useServer(t, e, fmt.Sprintf("trojan://secret@127.0.0.1:%d?security=none#local", freePort(t)))
	busy, err := net.Listen("tcp", net.JoinHostPort("127.0.0.1", strconv.Itoa(int(e.mixedPort))))
	if err != nil {
		t.Fatal(err)
	}
	e.StartConnection()
	if state := waitState(t, states); state.Error != ReasonPortBusy {
		t.Errorf("a start on a taken port: %+v", state)
	}
	// Another Poros on the computer, as of another Windows user, holds the port with its marker.
	other, err := marker.Hold(marker.Port(e.mixedPort))
	if err != nil {
		t.Fatal(err)
	}
	e.StartConnection()
	if state := waitState(t, states); state.Error != ReasonPortPoros {
		t.Errorf("a start on the port of another Poros: %+v", state)
	}
	other.Release()
	busy.Close()

	// The TUN of another Poros is told before the rights: they would not help.
	otherTun, err := marker.Hold(marker.Tun)
	if err != nil {
		t.Fatal(err)
	}
	withTun, _ := e.Settings()
	withTun.Tun = true
	if _, err = e.SetSettings(withTun); err != nil {
		t.Fatal(err)
	}
	e.StartConnection()
	if state := waitState(t, states); state.Error != ReasonTunTaken {
		t.Errorf("a start with the TUN of another Poros on: %+v", state)
	}
	otherTun.Release()
	withTun.Tun = false
	if _, err = e.SetSettings(withTun); err != nil {
		t.Fatal(err)
	}

	// Never on the host for real: an elevated test run must not bring a TUN up.
	if elevated() {
		t.Skip("the test runs with administrator rights; TUN is checked in the virtual machine")
	}
	settings, _ := e.Settings()
	settings.Tun = true
	if _, err = e.SetSettings(settings); err != nil {
		t.Fatal(err)
	}
	e.StartConnection()
	if state := waitState(t, states); state.Error != ReasonNeedsElevation {
		t.Errorf("TUN without rights: %+v", state)
	}

	// A forged handshake takes the rights of its driver, and only for a server it can forge for.
	useServer(t, e, fmt.Sprintf("trojan://secret@127.0.0.1:%d?security=tls&sni=t.example.com#tls", freePort(t)))
	settings, _ = e.Settings()
	settings.Tun, settings.SpoofDefault, settings.SpoofSNI = false, true, "www.example.com"
	if _, err = e.SetSettings(settings); err != nil {
		t.Fatal(err)
	}
	e.StartConnection()
	if state := waitState(t, states); state.Error != ReasonSpoofNeedsElevation {
		t.Errorf("a forged handshake without rights: %+v", state)
	}
}
