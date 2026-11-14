package engine

import (
	"fmt"
	"io"
	"net"
	"net/http"
	"net/url"
	"strconv"
	"testing"
	"time"
)

// fetchVia asks for page through the local proxy at port, with a name and password when user has them.
func fetchVia(port uint16, user *url.Userinfo, page string) (string, error) {
	proxy := &url.URL{Scheme: "http", Host: net.JoinHostPort("127.0.0.1", strconv.Itoa(int(port))), User: user}
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

// The local proxy of the settings: its own port, and a name and password that programs must give; the system proxy
// goes to a port of its own on loopback then, which asks for nothing.
func TestLocalProxySettings(t *testing.T) {
	e, states := openConnectionTest(t)
	page := webPage(t)
	server := freePort(t)
	runServer(t, fmt.Sprintf(`{"log":{"level":"warn"},"inbounds":[{"type":"trojan","listen":"127.0.0.1","listen_port":%d,`+
		`"users":[{"password":"secret"}]}],"outbounds":[{"type":"direct","tag":"direct"}]}`, server))
	useServer(t, e, fmt.Sprintf("trojan://secret@127.0.0.1:%d?security=none#local", server))

	settings, _ := e.Settings()
	for name, change := range map[string]func(s *Settings){
		"a port":                  func(s *Settings) { s.ProxyPort = 70000 },
		"a password without name": func(s *Settings) { s.ProxyAuth, s.ProxyPassword = true, "pw" },
		"a name with a colon":     func(s *Settings) { s.ProxyAuth, s.ProxyUsername, s.ProxyPassword = true, "a:b", "pw" },
		"a test address":          func(s *Settings) { s.LatencyURL = "cp.cloudflare.com" },
		"a test without time":     func(s *Settings) { s.LatencyTimeout = 10 },
		"a test of no servers":    func(s *Settings) { s.LatencyConcurrency = 0 },
	} {
		wrong := settings
		change(&wrong)
		if _, err := e.SetSettings(wrong); reason(err) != ReasonInvalidValue {
			t.Errorf("%s: %v", name, err)
		}
	}
	port := freePort(t)
	settings.ProxyPort, settings.ProxyAuth, settings.ProxyUsername, settings.ProxyPassword = int(port), true, "me", "pw"
	if _, err := e.SetSettings(settings); err != nil {
		t.Fatal(err)
	}
	e.StartConnection()
	if state := waitState(t, states); state.State != ConnectionRunning {
		t.Fatalf("the connection did not come up: %+v", state)
	}
	if body, err := fetchVia(port, url.UserPassword("me", "pw"), page); err != nil || body != "through the server" {
		t.Errorf("with the password: %q, %v", body, err)
	}
	if _, err := fetchVia(port, nil, page); err == nil {
		t.Error("without the password the proxy must refuse")
	}
	e.conn.lifecycle.Lock()
	systemPort := e.conn.running.systemProxyPort()
	e.conn.lifecycle.Unlock()
	if systemPort == port {
		t.Fatal("the system proxy needs a port of its own")
	}
	if body, err := fetchVia(systemPort, nil, page); err != nil || body != "through the server" {
		t.Errorf("the port of the system proxy: %q, %v", body, err)
	}
}
