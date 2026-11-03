package profile

// parseHTTPLink reads an HTTP proxy, http:// or https://user:password@host:port?query#name. https, or
// security=tls, means TLS to the proxy.
func parseHTTPLink(link string) (Profile, error) {
	l, err := parseShareLink(link)
	if err != nil {
		return Profile{}, err
	}
	q := l.query
	h := &HTTP{Path: q.get("path"), Headers: linkHeaders(q.get("headers"), ",")}
	h.Username, h.Password = linkProxyAuth(l.user, l.password)
	// Some providers set only the password, http://:token@host, and expect it as the user name too.
	if h.Username == "" && h.Password != "" {
		h.Username = h.Password
	}
	fallback := 80
	if l.scheme == "https" || q.get("security") == "tls" {
		if h.TLS, err = linkTLS(q, true); err != nil {
			return Profile{}, err
		}
		fallback = 443
	}
	port, err := l.serverPort(fallback)
	if err != nil {
		return Profile{}, err
	}
	return Profile{Type: TypeHTTP, Name: l.name, Server: l.host, ServerPort: port, HTTP: h}, nil
}
