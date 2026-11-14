package profile

import "strings"

// parseNaiveLink reads naive+https:// and naive+quic://user:password@host:port?sni=&extra-headers=#name, as
// NaiveProxy and Throne write them. extra-headers holds "Name: value" lines.
func parseNaiveLink(link string) (Profile, error) {
	l, err := parseShareLink(link)
	if err != nil {
		return Profile{}, err
	}
	q := l.query
	port, err := l.serverPort(443)
	if err != nil {
		return Profile{}, err
	}
	n := &Naive{
		Username:   l.user,
		Password:   l.password,
		QUIC:       l.scheme == "naive+quic",
		UDPOverTCP: linkFlag(q.trimmed("uot")),
		TLS:        TLS{Enabled: true, ServerName: q.get("sni")},
	}
	if n.QUIC {
		if control := strings.ToLower(q.trimmed("congestion_control")); control != "" {
			n.QUICCongestionControl = control
		}
	}
	if concurrency, ok := linkInt(q, "insecure-concurrency", 0, 1<<20); ok {
		n.InsecureConcurrency = concurrency
	}
	n.ExtraHeaders = naiveLinkHeaders(q.formValue("extra-headers"))
	n.TLS.ECH = linkECH(q)
	return Profile{Type: TypeNaive, Name: l.name, Server: l.host, ServerPort: port, Naive: n}, nil
}

func naiveLinkHeaders(text string) map[string]string {
	headers := map[string]string{}
	for line := range strings.SplitSeq(text, "\n") {
		name, value, found := strings.Cut(strings.TrimSuffix(line, "\r"), ":")
		if name = strings.TrimSpace(name); found && name != "" {
			headers[name] = strings.TrimSpace(value)
		}
	}
	if len(headers) == 0 {
		return nil
	}
	return headers
}
