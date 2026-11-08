package profile

import (
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"net/netip"
	"reflect"
	"slices"
	"strconv"
	"strings"
)

// ShareLink writes a profile as the share link of its protocol, the way Xray, v2rayN and Poros read it. When that link
// would lose a setting, such as an Xray extra object or a header with a '|', it writes a poros://add/ link instead:
// whatever it gives reads back into the same profile.
func ShareLink(p Profile) string {
	if link, ok := writeLink(p); ok {
		want := p
		if host, err := normalizeLinkHost(p.Server); err == nil {
			want.Server = host
		}
		if got, err := ParseLink(link); err == nil && reflect.DeepEqual(got, want) {
			return link
		}
	}
	return PorosLink(p)
}

// PorosLink writes poros://add/ with the profile's JSON in base64url: the stored form, which carries everything.
func PorosLink(p Profile) string {
	data, _ := json.Marshal(p)
	return "poros://add/" + base64.RawURLEncoding.EncodeToString(data)
}

func writeLink(p Profile) (string, bool) {
	switch {
	case p.VLESS != nil:
		return writeVLESSLink(p), true
	case p.VMess != nil:
		return writeVMessLink(p)
	case p.Trojan != nil:
		return writeTrojanLink(p), true
	case p.Shadowsocks != nil:
		return writeShadowsocksLink(p), true
	case p.Hysteria2 != nil:
		return writeHysteria2Link(p), true
	case p.TUIC != nil:
		return writeTUICLink(p), true
	case p.SOCKS != nil:
		return writeSOCKSLink(p), true
	case p.HTTP != nil:
		return writeHTTPLink(p), true
	case p.AnyTLS != nil:
		return writeAnyTLSLink(p), true
	}
	return "", false
}

// linkWriter builds a share link: scheme://userinfo@host:port?query#name, the query in the order it was set.
type linkWriter struct {
	scheme, userinfo string
	host             string
	port             int
	keys, values     []string
	name             string
}

func newLinkWriter(scheme string, p Profile) *linkWriter {
	return &linkWriter{scheme: scheme, host: p.Server, port: p.ServerPort, name: p.Name}
}

func (w *linkWriter) set(key, value string) {
	w.keys = append(w.keys, key)
	w.values = append(w.values, value)
}

// setIf sets a key whose empty value means nothing.
func (w *linkWriter) setIf(key, value string) {
	if value != "" {
		w.set(key, value)
	}
}

func (w *linkWriter) user(user, password string) {
	w.userinfo = escapeLinkText(user, "")
	if password != "" {
		w.userinfo += ":" + escapeLinkText(password, "")
	}
}

func (w *linkWriter) String() string {
	var b strings.Builder
	b.WriteString(w.scheme + "://")
	if w.userinfo != "" {
		b.WriteString(w.userinfo + "@")
	}
	if addr, err := netip.ParseAddr(w.host); err == nil && addr.Is6() {
		b.WriteString("[" + addr.String() + "]")
	} else {
		b.WriteString(escapeLinkText(w.host, ".-"))
	}
	b.WriteString(":" + strconv.Itoa(w.port))
	if query := w.query(); query != "" {
		b.WriteString("?" + query)
	}
	if w.name != "" {
		b.WriteString("#" + escapeLinkText(w.name, ""))
	}
	return b.String()
}

func (w *linkWriter) query() string {
	items := make([]string, len(w.keys))
	for i, key := range w.keys {
		items[i] = escapeLinkText(key, "") + "=" + escapeLinkText(w.values[i], "/,:@")
	}
	return strings.Join(items, "&")
}

// escapeLinkText percent-encodes everything but the unreserved characters of RFC 3986 and keep. A '+' is encoded too:
// other clients read it as a space.
func escapeLinkText(text, keep string) string {
	const hexDigits = "0123456789ABCDEF"
	var b strings.Builder
	for i := 0; i < len(text); i++ {
		c := text[i]
		if c >= 'a' && c <= 'z' || c >= 'A' && c <= 'Z' || c >= '0' && c <= '9' || strings.IndexByte("-._~", c) >= 0 ||
			strings.IndexByte(keep, c) >= 0 {
			b.WriteByte(c)
			continue
		}
		b.WriteByte('%')
		b.WriteByte(hexDigits[c>>4])
		b.WriteByte(hexDigits[c&15])
	}
	return b.String()
}

func linkBool(on bool) string {
	if on {
		return "1"
	}
	return ""
}

// writeTLS sets the TLS keys linkTLS reads. forced is a protocol whose TLS cannot be off and needs no security key;
// otherwise TLS off is security=none, which a protocol with TLS by default, such as Trojan, needs.
func (w *linkWriter) writeTLS(t TLS, forced bool) {
	if !t.Enabled {
		if !forced {
			w.set("security", "none")
		}
		return
	}
	if !forced {
		if t.Reality.Enabled {
			w.set("security", "reality")
		} else {
			w.set("security", "tls")
		}
	}
	w.setIf("sni", t.ServerName)
	w.setIf("disable_sni", linkBool(t.DisableSNI))
	w.setIf("allowInsecure", linkBool(t.Insecure))
	w.setIf("alpn", strings.Join(t.ALPN, ","))
	w.setIf("tls_min_version", t.MinVersion)
	w.setIf("tls_max_version", t.MaxVersion)
	w.setIf("tls_certificate_public_key_sha256", strings.Join(t.CertificatePublicKeySHA256, ","))
	if pins := hexPins(t.CertificateSHA256); pins != "" {
		w.set("pcs", pins)
	}
	if t.UTLS.Enabled {
		w.set("fp", t.UTLS.Fingerprint)
	}
	if t.Reality.Enabled {
		w.set("pbk", t.Reality.PublicKey)
		w.setIf("sid", t.Reality.ShortID)
		w.setIf("spx", t.Reality.SpiderX)
	}
	w.writeECH(t.ECH)
}

// hexPins writes certificate pins as Xray's pcs: hex SHA-256 hashes, comma-separated.
func hexPins(pins []string) string {
	hexes := make([]string, 0, len(pins))
	for _, pin := range pins {
		hash, err := base64.StdEncoding.DecodeString(pin)
		if err != nil {
			// linkTLS refuses the link then, and ShareLink falls back to poros://add/.
			hexes = append(hexes, pin)
			continue
		}
		hexes = append(hexes, hex.EncodeToString(hash))
	}
	return strings.Join(hexes, ",")
}

// writeECH writes Xray's ech for a config list or a name to query, and Poros's ech_* keys for anything else.
func (w *linkWriter) writeECH(e ECH) {
	if !e.Enabled {
		return
	}
	bare := len(e.Config) == 3 && e.Config[0] == echPEMBegin && e.Config[2] == echPEMEnd && !strings.ContainsAny(e.Config[1], ".")
	switch {
	case len(e.Config) == 0 && e.QueryServerName == "":
		w.set("ech", "1")
	case len(e.Config) == 0 && strings.Contains(e.QueryServerName, "."):
		w.set("ech", e.QueryServerName)
	case bare && e.QueryServerName == "":
		w.set("ech", e.Config[1])
	default:
		w.set("ech_enabled", "true")
		w.setIf("ech_config", strings.Join(e.Config, ","))
		w.setIf("ech_server_name", e.QueryServerName)
	}
}

// writeTransport sets the transport keys linkTransport reads.
func (w *linkWriter) writeTransport(t Transport) {
	switch t.Type {
	case TransportTCP:
		if t.HeaderType == "http" {
			w.set("type", "tcp")
			w.set("headerType", "http")
			w.setIf("host", t.Host)
			w.setIf("path", t.Path)
		}
		return
	case TransportGRPC:
		w.set("type", "grpc")
		w.setIf("serviceName", t.ServiceName)
		return
	case TransportQUIC:
		w.set("type", "quic")
		return
	}
	w.set("type", t.Type)
	w.setIf("host", t.Host)
	w.setIf("path", t.Path)
	if headers := linkHeaderText(t.Headers, "|"); headers != "" {
		w.set("headers", headers)
	}
	switch t.Type {
	case TransportWebSocket:
		switch {
		case t.MaxEarlyData > 0 && t.EarlyDataHeaderName == wsEarlyDataHeader:
			w.set("ed", strconv.Itoa(t.MaxEarlyData))
		case t.MaxEarlyData > 0 || t.EarlyDataHeaderName != "":
			w.set("max_early_data", strconv.Itoa(t.MaxEarlyData))
			w.setIf("early_data_header_name", t.EarlyDataHeaderName)
		}
	case TransportHTTP:
		w.setIf("method", t.Method)
	case TransportXHTTP:
		w.setIf("mode", t.XHTTP.Mode)
		if len(t.XHTTP.Extra) > 0 {
			w.set("extra", string(t.XHTTP.Extra))
		}
	}
}

// linkHeaderText writes headers as alternating names and values in the order of their names.
func linkHeaderText(headers map[string]string, separator string) string {
	names := make([]string, 0, len(headers))
	for name := range headers {
		names = append(names, name)
	}
	slices.Sort(names)
	parts := make([]string, 0, 2*len(names))
	for _, name := range names {
		parts = append(parts, name, headers[name])
	}
	return strings.Join(parts, separator)
}

func (w *linkWriter) writeMultiplex(m Multiplex) {
	if !m.Enabled {
		return
	}
	w.set("mux", "1")
	w.setIf("mux_protocol", m.Protocol)
	for _, item := range []struct {
		key   string
		value int
	}{{"mux_max_connections", m.MaxConnections}, {"mux_min_streams", m.MinStreams}, {"mux_max_streams", m.MaxStreams}} {
		if item.value != 0 {
			w.set(item.key, strconv.Itoa(item.value))
		}
	}
	w.setIf("mux_padding", linkBool(m.Padding))
}

// writePacketEncoding writes what linkPacketEncoding reads; xudp is what a link without the key gives.
func (w *linkWriter) writePacketEncoding(encoding string) {
	switch encoding {
	case "xudp":
	case "":
		w.set("packetEncoding", "none")
	default:
		w.set("packetEncoding", encoding)
	}
}

func writeVLESSLink(p Profile) string {
	v := p.VLESS
	w := newLinkWriter("vless", p)
	w.user(v.UUID, "")
	encryption := v.Encryption
	if encryption == "" {
		encryption = "none"
	}
	w.set("encryption", encryption)
	w.setIf("flow", v.Flow)
	w.writePacketEncoding(v.PacketEncoding)
	w.writeTransport(v.Transport)
	w.writeTLS(v.TLS, false)
	w.writeMultiplex(v.Multiplex)
	return w.String()
}

func writeTrojanLink(p Profile) string {
	t := p.Trojan
	w := newLinkWriter("trojan", p)
	w.user(t.Password, "")
	w.writeTLS(t.TLS, false)
	w.writeTransport(t.Transport)
	w.writeMultiplex(t.Multiplex)
	return w.String()
}

// writeVMessLink writes v2rayN's base64 JSON; what its fields cannot hold goes to throneExtra, a query the parser
// merges over them.
func writeVMessLink(p Profile) (string, bool) {
	v := p.VMess
	fields := map[string]any{
		"v": "2", "ps": p.Name, "add": p.Server, "port": strconv.Itoa(p.ServerPort), "id": v.UUID,
		"aid": strconv.Itoa(v.AlterID), "scy": v.Security, "net": "tcp", "type": "none", "host": "", "path": "", "tls": "",
	}
	extra := &linkWriter{}
	t := v.Transport
	switch t.Type {
	case TransportTCP:
		if t.HeaderType == "http" {
			fields["type"], fields["host"], fields["path"] = "http", t.Host, t.Path
		}
	case TransportGRPC:
		fields["net"], fields["path"] = "grpc", t.ServiceName
	case TransportQUIC:
		fields["net"] = "quic"
	default:
		// Other clients read host and path from the fields; throneExtra brings the transport whole and wins.
		fields["net"], fields["host"], fields["path"] = t.Type, t.Host, t.Path
		extra.writeTransport(t)
	}
	if v.TLS.Enabled {
		fields["tls"] = "tls"
		tls := v.TLS
		fields["sni"], fields["alpn"] = tls.ServerName, strings.Join(tls.ALPN, ",")
		if tls.UTLS.Enabled {
			fields["fp"] = tls.UTLS.Fingerprint
		}
		if tls.Insecure {
			fields["allowInsecure"] = "1"
		}
		rest := tls
		rest.ServerName, rest.ALPN, rest.UTLS, rest.Insecure = "", nil, UTLS{}, false
		if !reflect.DeepEqual(rest, TLS{Enabled: true}) {
			extra.writeTLS(tls, false)
		}
	}
	extra.writePacketEncoding(v.PacketEncoding)
	extra.writeMultiplex(v.Multiplex)
	if query := extra.query(); query != "" {
		fields["throneExtra"] = query
	}
	data, err := json.Marshal(fields)
	if err != nil {
		return "", false
	}
	return "vmess://" + base64.StdEncoding.EncodeToString(data), true
}

// writeShadowsocksLink writes SIP002: base64url of method:password, or for the 2022 methods, whose keys are base64
// already, method:password percent-encoded.
func writeShadowsocksLink(p Profile) string {
	s := p.Shadowsocks
	w := newLinkWriter("ss", p)
	if strings.HasPrefix(s.Method, "2022-") {
		w.user(s.Method, s.Password)
	} else {
		w.userinfo = base64.RawURLEncoding.EncodeToString([]byte(s.Method + ":" + s.Password))
	}
	if s.Plugin != "" {
		plugin := s.Plugin
		if s.PluginOpts != "" {
			plugin += ";" + s.PluginOpts
		}
		w.set("plugin", plugin)
	}
	w.setIf("uot", linkBool(s.UDPOverTCP))
	w.writeMultiplex(s.Multiplex)
	return w.String()
}

func writeHysteria2Link(p Profile) string {
	h := p.Hysteria2
	w := newLinkWriter("hysteria2", p)
	w.user(h.Password, "")
	if len(h.ServerPorts) > 0 {
		ranges := make([]string, len(h.ServerPorts))
		for i, r := range h.ServerPorts {
			ranges[i] = strings.Replace(r, ":", "-", 1)
		}
		w.set("mport", strings.Join(ranges, ","))
	}
	w.setIf("hop_interval", h.HopInterval)
	if h.UpMbps > 0 {
		w.set("upmbps", strconv.Itoa(h.UpMbps))
	}
	if h.DownMbps > 0 {
		w.set("downmbps", strconv.Itoa(h.DownMbps))
	}
	if h.ObfsPassword != "" {
		w.set("obfs", h.ObfsType)
		w.set("obfs-password", h.ObfsPassword)
	}
	w.writeTLS(h.TLS, true)
	return w.String()
}

func writeTUICLink(p Profile) string {
	t := p.TUIC
	w := newLinkWriter("tuic", p)
	w.user(t.UUID, t.Password)
	w.setIf("congestion_control", t.CongestionControl)
	w.setIf("udp_relay_mode", t.UDPRelayMode)
	if t.ZeroRTT {
		w.set("zero_rtt_handshake", "true")
	}
	w.setIf("heartbeat", t.Heartbeat)
	w.writeTLS(t.TLS, true)
	return w.String()
}

func writeSOCKSLink(p Profile) string {
	s := p.SOCKS
	scheme := "socks5"
	switch s.Version {
	case "4":
		scheme = "socks4"
	case "4a":
		scheme = "socks4a"
	}
	w := newLinkWriter(scheme, p)
	w.user(s.Username, s.Password)
	return w.String()
}

func writeHTTPLink(p Profile) string {
	h := p.HTTP
	scheme := "http"
	if h.TLS.Enabled {
		scheme = "https"
	}
	w := newLinkWriter(scheme, p)
	w.user(h.Username, h.Password)
	w.setIf("path", h.Path)
	if headers := linkHeaderText(h.Headers, ","); headers != "" {
		w.set("headers", headers)
	}
	if h.TLS.Enabled {
		w.writeTLS(h.TLS, true)
	}
	return w.String()
}

func writeAnyTLSLink(p Profile) string {
	a := p.AnyTLS
	w := newLinkWriter("anytls", p)
	w.user(a.Password, "")
	w.setIf("idle_session_check_interval", a.IdleSessionCheckInterval)
	w.setIf("idle_session_timeout", a.IdleSessionTimeout)
	w.set("min_idle_session", strconv.Itoa(a.MinIdleSession))
	w.writeTLS(a.TLS, true)
	return w.String()
}
