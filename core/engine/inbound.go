package engine

import (
	"cmp"
	"net"
	"strings"
	"time"

	"PorosCore/config"
	"PorosCore/internal/jsonrpc"
)

// The bounds of a delay test of the settings.
const (
	minLatencyTimeout     = 500 * time.Millisecond
	maxLatencyTimeout     = 30 * time.Second
	maxLatencyConcurrency = 64
)

// validateInbound checks the local proxy and the delay test of the settings.
func validateInbound(s Settings) error {
	if s.ProxyPort < 0 || s.ProxyPort > 65535 {
		return jsonrpc.Failed(ReasonInvalidValue, "proxyPort must be from 1 to 65535, not %d", s.ProxyPort)
	}
	if s.ProxyAuth && (strings.TrimSpace(s.ProxyUsername) == "" || s.ProxyPassword == "") {
		return jsonrpc.Failed(ReasonInvalidValue, "a local proxy with authorization needs a username and a password")
	}
	// HTTP takes the username and password as one, split at the first colon.
	if strings.Contains(s.ProxyUsername, ":") {
		return jsonrpc.Failed(ReasonInvalidValue, "the username of the local proxy cannot hold a colon")
	}
	if s.LatencyURL != "" && !strings.HasPrefix(s.LatencyURL, "http://") && !strings.HasPrefix(s.LatencyURL, "https://") {
		return jsonrpc.Failed(ReasonInvalidValue, "the delay test asks for an http:// or https:// address, not %q", s.LatencyURL)
	}
	timeout := time.Duration(s.LatencyTimeout) * time.Millisecond
	if timeout < minLatencyTimeout || timeout > maxLatencyTimeout {
		return jsonrpc.Failed(ReasonInvalidValue, "latencyTimeout must be from %d to %d ms, not %d",
			minLatencyTimeout.Milliseconds(), maxLatencyTimeout.Milliseconds(), s.LatencyTimeout)
	}
	if s.LatencyConcurrency < 1 || s.LatencyConcurrency > maxLatencyConcurrency {
		return jsonrpc.Failed(ReasonInvalidValue, "latencyConcurrency must be from 1 to %d, not %d", maxLatencyConcurrency, s.LatencyConcurrency)
	}
	return nil
}

// proxyPort is the port of the local proxy by the settings.
func (e *Engine) proxyPort(s Settings) uint16 {
	if s.ProxyPort != 0 {
		return uint16(s.ProxyPort)
	}
	return e.mixedPort
}

func proxyAuthOf(s Settings) *config.ProxyAuth {
	if !s.ProxyAuth {
		return nil
	}
	return &config.ProxyAuth{Username: strings.TrimSpace(s.ProxyUsername), Password: s.ProxyPassword}
}

// systemPortOf finds a free port on loopback for the system proxy while the local proxy asks for a password; 0
// without one.
func (e *Engine) systemPortOf(s Settings) (uint16, error) {
	if !s.ProxyAuth {
		return 0, nil
	}
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		return 0, err
	}
	defer listener.Close()
	return uint16(listener.Addr().(*net.TCPAddr).Port), nil
}

// proxyChanged tells whether two settings make the local proxy differently.
func proxyChanged(a, b Settings) bool {
	return a.ProxyPort != b.ProxyPort || a.ProxyAuth != b.ProxyAuth || a.ProxyUsername != b.ProxyUsername || a.ProxyPassword != b.ProxyPassword
}

// latencyTest is what a delay test asks for, how long it waits and how many servers it tests at once.
func (e *Engine) latencyTest() (url string, timeout time.Duration, concurrency int) {
	s, err := e.Settings()
	if err != nil {
		s = DefaultSettings()
	}
	timeout = time.Duration(s.LatencyTimeout) * time.Millisecond
	if timeout <= 0 {
		timeout = defaultLatencyTimeout
	}
	return cmp.Or(s.LatencyURL, e.latencyURL), timeout, cmp.Or(s.LatencyConcurrency, defaultLatencyConcurrency)
}
