package profile

// parseTrojanLink reads trojan://password@host:port?query#name.
func parseTrojanLink(link string) (Profile, error) {
	l, err := parseShareLink(link)
	if err != nil {
		return Profile{}, err
	}
	port, err := l.serverPort(443)
	if err != nil {
		return Profile{}, err
	}
	q := l.query
	// A password with an unescaped ':' still counts whole.
	t := &Trojan{Password: l.userinfo(), Multiplex: linkMultiplex(q)}
	// Trojan means TLS: without it the password and every destination go in the clear. Only an explicit
	// security turns it off.
	if t.TLS, err = linkTLS(q, q.trimmed("security") == ""); err != nil {
		return Profile{}, err
	}
	if t.Transport, err = linkTransport(q); err != nil {
		return Profile{}, err
	}
	return Profile{Type: TypeTrojan, Name: l.name, Server: l.host, ServerPort: port, Trojan: t}, nil
}
