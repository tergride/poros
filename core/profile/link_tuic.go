package profile

import (
	"slices"
	"strings"
)

// parseTUICLink reads tuic://uuid:password@host:port?query#name, TUIC v5.
func parseTUICLink(link string) (Profile, error) {
	l, err := parseShareLink(link)
	if err != nil {
		return Profile{}, err
	}
	port, err := l.serverPort(443)
	if err != nil {
		return Profile{}, err
	}
	q := l.query
	t := &TUIC{
		UUID:              l.user,
		Password:          l.password,
		CongestionControl: tuicLinkChoice(q.get("congestion_control"), []string{"cubic", "new_reno", "bbr"}),
		// The core treats an unknown mode as native.
		UDPRelayMode: tuicLinkChoice(q.get("udp_relay_mode"), []string{"native", "quic"}),
		ZeroRTT:      q.get("zero_rtt_handshake") == "true",
		Heartbeat:    linkDuration(q, "heartbeat"),
	}
	tls, err := linkTLS(q, true)
	if err != nil {
		return Profile{}, err
	}
	t.TLS = quicLinkTLS(tls)
	return Profile{Type: TypeTUIC, Name: l.name, Server: l.host, ServerPort: port, TUIC: t}, nil
}

// tuicLinkChoice folds case and spaces, as Throne's combo labels ("BBR", "New Reno") were written; anything
// unknown is dropped.
func tuicLinkChoice(raw string, choices []string) string {
	folded := strings.ToLower(strings.TrimSpace(raw))
	for _, candidate := range []string{folded, strings.ReplaceAll(folded, " ", "_"), strings.ReplaceAll(folded, " ", "")} {
		if slices.Contains(choices, candidate) {
			return candidate
		}
	}
	return ""
}
