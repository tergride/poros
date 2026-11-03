package profile

import (
	"errors"
	"strconv"
	"strings"
)

// parseShadowsocksLink reads SIP002 (ss://base64(method:password)@host:port/?plugin=...#name, or the userinfo as
// plain method:password) and the legacy ss://base64(method:password@host:port)#name. The port has no default.
func parseShadowsocksLink(link string) (Profile, error) {
	scheme, rest, _ := strings.Cut(link, "://")
	body, fragment, hasFragment := strings.Cut(rest, "#")
	if !strings.Contains(body, "@") {
		decoded, ok := decodeLinkBase64(body)
		if !ok {
			return Profile{}, errors.New("the link is neither ss://method:password@host:port nor valid base64")
		}
		link = scheme + "://" + string(decoded)
		if hasFragment {
			link += "#" + fragment
		}
	}
	l, err := parseShareLink(link)
	if err != nil {
		return Profile{}, err
	}
	port, err := l.serverPort(0)
	if err != nil {
		return Profile{}, err
	}

	method, password := l.user, l.password
	if password == "" {
		decoded, ok := decodeLinkBase64(l.user)
		if !ok {
			return Profile{}, errors.New("the Shadowsocks method and password are not valid base64")
		}
		// Without the colon the text may be a bare password, which would pass for the method.
		var found bool
		if method, password, found = strings.Cut(string(decoded), ":"); !found {
			return Profile{}, errors.New("the Shadowsocks userinfo must be method:password")
		}
	}

	q := l.query
	s := &Shadowsocks{
		// The core matches method names exactly; links and subscriptions often upper-case them.
		Method:    strings.ToLower(strings.TrimSpace(method)),
		Password:  password,
		Multiplex: linkMultiplex(q),
	}
	name, opts, _ := strings.Cut(q.get("plugin"), ";")
	s.Plugin = shadowsocksLinkPlugin(name)
	if s.Plugin != "" {
		s.PluginOpts = strings.TrimSpace(opts)
		if q.has("plugin-opts") {
			s.PluginOpts = q.trimmed("plugin-opts")
		}
	}
	if uot := q.trimmed("uot"); uot != "" {
		n, _ := strconv.Atoi(uot)
		s.UDPOverTCP = uot == "true" || n > 0
	}
	return Profile{Type: TypeShadowsocks, Name: l.name, Server: l.host, ServerPort: port, Shadowsocks: s}, nil
}

// shadowsocksLinkPlugin maps simple-obfs and Clash's obfs onto obfs-local, the name the core registers.
func shadowsocksLinkPlugin(raw string) string {
	switch name := strings.TrimSpace(raw); name {
	case "simple-obfs", "obfs":
		return "obfs-local"
	default:
		return name
	}
}
