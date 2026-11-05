package profile

import "strings"

const vlessFlowVision = "xtls-rprx-vision"

// parseVLESSLink reads vless://uuid@host:port?query#name. Xray's keys (encryption, xhttp, extra, spx) are read
// alongside sing-box's: the profile is one type for both cores.
func parseVLESSLink(link string) (Profile, error) {
	l, err := parseShareLink(link)
	if err != nil {
		return Profile{}, err
	}
	port, err := l.serverPort(443)
	if err != nil {
		return Profile{}, err
	}
	q := l.query
	v := &VLESS{
		UUID:       l.user,
		Flow:       vlessLinkFlow(q.get("flow")),
		Encryption: strings.TrimSpace(q.get("encryption")),
		// No packetEncoding is xudp, the Qt client's default; none is the empty one.
		PacketEncoding: "xudp",
		Multiplex:      linkMultiplex(q),
	}
	if strings.EqualFold(v.Encryption, "none") {
		v.Encryption = ""
	}
	if value := q.trimmed("packetEncoding"); value != "" {
		v.PacketEncoding = linkPacketEncoding(value)
	}
	if v.Transport, err = linkTransport(q); err != nil {
		return Profile{}, err
	}
	if v.TLS, err = linkTLS(q, false); err != nil {
		return Profile{}, err
	}
	fitVisionFlow(v)
	return Profile{Type: TypeVLESS, Name: l.name, Server: l.host, ServerPort: port, VLESS: v}, nil
}

// fitVisionFlow drops a flow the server cannot run: an unknown flow stops the core and a misplaced Vision fails
// every connection, while no flow at worst loses Vision's speed-up. VLESS encryption carries Vision over any
// transport.
func fitVisionFlow(v *VLESS) {
	if v.Flow != vlessFlowVision || v.Encryption == "" && (!v.TLS.Enabled || v.Transport.Type != TransportTCP) {
		v.Flow = ""
	}
}

func vlessLinkFlow(raw string) string {
	switch flow := strings.ToLower(strings.TrimSpace(raw)); flow {
	case "", "none":
		return ""
	case "xtls-rprx-vision-udp443":
		// Xray's client-side variant that also lets QUIC to port 443 through; the server sees plain Vision.
		return vlessFlowVision
	default:
		return flow
	}
}
