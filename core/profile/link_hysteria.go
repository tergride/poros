package profile

import (
	"strconv"
	"strings"
)

// parseHysteriaLink reads Hysteria v1's hysteria://host:port?auth=&peer=&upmbps=&downmbps=&obfsParam=#name. The port
// may be a hopping list, as in Hysteria2 links, and so may mport.
func parseHysteriaLink(link string) (Profile, error) {
	l, err := parseShareLink(link)
	if err != nil {
		return Profile{}, err
	}
	q := l.query
	if protocol := strings.ToLower(q.trimmed("protocol")); protocol != "" && protocol != "udp" {
		return Profile{}, unsupported("Hysteria over %s is not supported; sing-box runs it over UDP", protocol)
	}
	h := &Hysteria{Auth: q.get("auth"), Obfs: q.get("obfsParam")}
	if h.Auth == "" {
		h.Auth = l.userinfo()
	}
	var port int
	if strings.ContainsAny(l.port, ",-") {
		if h.ServerPorts, err = hysteriaPortRanges(l.port); err != nil {
			return Profile{}, err
		}
	} else if port, err = l.serverPort(0); err != nil {
		return Profile{}, err
	}
	if q.has("mport") {
		if h.ServerPorts, err = hysteriaPortRanges(q.get("mport")); err != nil {
			return Profile{}, err
		}
	}
	if port == 0 && len(h.ServerPorts) > 0 {
		first, _, _ := strings.Cut(h.ServerPorts[0], ":")
		port, _ = strconv.Atoi(first)
	}
	if port == 0 {
		port = 443
	}
	h.UpMbps = hysteriaLinkMbps(q.get("upmbps"))
	h.DownMbps = hysteriaLinkMbps(q.get("downmbps"))
	h.HopInterval = linkDuration(q, "hop_interval")
	tls, err := linkTLS(q, true)
	if err != nil {
		return Profile{}, err
	}
	h.TLS = quicLinkTLS(tls)
	return Profile{Type: TypeHysteria, Name: l.name, Server: l.host, ServerPort: port, Hysteria: h}, nil
}
