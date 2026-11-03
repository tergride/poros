package profile

import "math"

// parseAnyTLSLink reads anytls://password@host:port?query#name. AnyTLS always runs over TLS.
func parseAnyTLSLink(link string) (Profile, error) {
	l, err := parseShareLink(link)
	if err != nil {
		return Profile{}, err
	}
	port, err := l.serverPort(443)
	if err != nil {
		return Profile{}, err
	}
	q := l.query
	// The Qt client's defaults.
	a := &AnyTLS{Password: l.userinfo(), IdleSessionCheckInterval: "30s", IdleSessionTimeout: "30s", MinIdleSession: 5}
	if value := linkDuration(q, "idle_session_check_interval"); value != "" {
		a.IdleSessionCheckInterval = value
	}
	if value := linkDuration(q, "idle_session_timeout"); value != "" {
		a.IdleSessionTimeout = value
	}
	if value, ok := linkInt(q, "min_idle_session", 0, math.MaxInt32); ok {
		a.MinIdleSession = value
	}
	if a.TLS, err = linkTLS(q, true); err != nil {
		return Profile{}, err
	}
	return Profile{Type: TypeAnyTLS, Name: l.name, Server: l.host, ServerPort: port, AnyTLS: a}, nil
}
