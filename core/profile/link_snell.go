package profile

import (
	"strconv"
	"strings"
)

// parseSnellLink reads snell://psk@host:port?version=&obfs=&obfs-host=&mode=#name, as Throne writes it.
func parseSnellLink(link string) (Profile, error) {
	l, err := parseShareLink(link)
	if err != nil {
		return Profile{}, err
	}
	q := l.query
	port, err := l.serverPort(0)
	if err != nil {
		return Profile{}, err
	}
	s := &Snell{PSK: l.userinfo(), Version: 4, UserKey: q.get("userkey"), Mode: q.trimmed("mode")}
	if s.PSK == "" {
		s.PSK = q.get("psk")
	}
	if q.has("version") {
		if s.Version, err = strconv.Atoi(q.trimmed("version")); err != nil {
			return Profile{}, unsupported("the Snell version %q is not a number", q.trimmed("version"))
		}
	}
	if q.has("reuse") {
		value := strings.ToLower(q.trimmed("reuse"))
		s.Reuse = value != "0" && value != "false"
	}
	if obfs := strings.ToLower(q.trimmed("obfs")); obfs != "" && obfs != "none" {
		s.ObfsMode, s.ObfsHost = obfs, q.trimmed("obfs-host")
	}
	return Profile{Type: TypeSnell, Name: l.name, Server: l.host, ServerPort: port, Snell: s}, nil
}
