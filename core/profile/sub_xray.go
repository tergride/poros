package profile

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"maps"
	"slices"
	"strconv"
	"strings"
)

// Xray decodes its config with encoding/json, which matches keys regardless of case and lets the last of several
// spellings win. The structs here are decoded the same way, so a config means to this parser what it means to Xray.

var xrayInfraProtocols = []string{"freedom", "direct", "blackhole", "block", "dns", "loopback"}

// xrayOutbound reads one Xray outbound. What the model has no field for is dropped, unless the server needs it to
// accept the connection.
func xrayOutbound(raw json.RawMessage) (p Profile, tag string, proxy bool, err error) {
	return xrayConfigOutbound(raw, nil)
}

// xrayDirectTags lists the tags of the plain freedom outbounds of a config. Dialing through one is 3x-ui's fragment
// feature, which only splits the TLS hello against DPI: without it the server is dialled directly. A freedom outbound
// that redirects elsewhere is not plain.
func xrayDirectTags(outbounds []json.RawMessage) []string {
	var tags []string
	for _, raw := range outbounds {
		var out struct {
			Protocol string `json:"protocol"`
			Tag      string `json:"tag"`
			Settings struct {
				Redirect string `json:"redirect"`
			} `json:"settings"`
		}
		if json.Unmarshal(raw, &out) != nil {
			continue
		}
		protocol, tag := strings.ToLower(strings.TrimSpace(out.Protocol)), strings.TrimSpace(out.Tag)
		if (protocol == "freedom" || protocol == "direct") && tag != "" && strings.TrimSpace(out.Settings.Redirect) == "" {
			tags = append(tags, tag)
		}
	}
	return tags
}

// xrayConfigOutbound reads an outbound of a whole config, whose freedom outbounds direct lists.
func xrayConfigOutbound(raw json.RawMessage, direct []string) (p Profile, tag string, proxy bool, err error) {
	// Qt skipped what is not an object, and an empty one, as it skipped direct and block.
	if len(jsonFields(raw)) == 0 {
		return Profile{}, "", false, nil
	}
	var head struct {
		Protocol any `json:"protocol"`
		Tag      any `json:"tag"`
	}
	_ = json.Unmarshal(raw, &head)
	tag, _ = head.Tag.(string)
	tag = strings.TrimSpace(tag)
	protocol, _ := head.Protocol.(string)
	// Xray lower-cases the protocol.
	protocol = strings.ToLower(strings.TrimSpace(protocol))
	if slices.Contains(xrayInfraProtocols, protocol) {
		return Profile{}, tag, false, nil
	}
	p, err = xrayServer(protocol, raw, direct)
	return p, tag, true, err
}

type xrayOutboundJSON struct {
	Settings       json.RawMessage `json:"settings"`
	StreamSettings *xrayStreamJSON `json:"streamSettings"`
	ProxySettings  *struct {
		Tag string `json:"tag"`
	} `json:"proxySettings"`
}

// xraySettingsJSON holds the settings of every supported protocol: the flat form, with the server and its account
// side by side, and the classic one, with vnext or servers.
type xraySettingsJSON struct {
	Address *string  `json:"address"`
	Port    xrayPort `json:"port"`
	xrayAccountJSON
	Vnext   []xrayServerJSON  `json:"vnext"`
	Servers []xrayServerJSON  `json:"servers"`
	Version xrayInt           `json:"version"`
	Headers map[string]string `json:"headers"`
}

type xrayServerJSON struct {
	Address *string  `json:"address"`
	Port    xrayPort `json:"port"`
	// Trojan and Shadowsocks keep the account in the server itself.
	xrayAccountJSON
	Users []xrayAccountJSON `json:"users"`
}

type xrayAccountJSON struct {
	ID         string   `json:"id"`
	Flow       string   `json:"flow"`
	Encryption string   `json:"encryption"`
	Security   string   `json:"security"`
	AlterID    xrayInt  `json:"alterId"`
	Password   string   `json:"password"`
	Method     string   `json:"method"`
	UoT        xrayFlag `json:"uot"`
	User       string   `json:"user"`
	Pass       string   `json:"pass"`
}

// endpoint picks the server as Xray does: the flat form once settings name an address, else the first entry of
// list. Xray refuses more than one server or user; Qt imported the first.
func (s *xraySettingsJSON) endpoint(list []xrayServerJSON) (*string, xrayPort, xrayAccountJSON) {
	if s.Address != nil {
		return s.Address, s.Port, s.xrayAccountJSON
	}
	if len(list) == 0 {
		return nil, xrayPort{}, xrayAccountJSON{}
	}
	server := list[0]
	account := server.xrayAccountJSON
	if len(server.Users) > 0 {
		account = server.Users[0]
	}
	return server.Address, server.Port, account
}

type xrayStreamJSON struct {
	// Method outranks network; neither means raw TCP.
	Method              *string            `json:"method"`
	Network             *string            `json:"network"`
	Security            string             `json:"security"`
	TLSSettings         *xrayTLSJSON       `json:"tlsSettings"`
	RealitySettings     *xrayRealityJSON   `json:"realitySettings"`
	RawSettings         *xrayRawJSON       `json:"rawSettings"`
	TCPSettings         *xrayRawJSON       `json:"tcpSettings"`
	WSSettings          *xrayWebSocketJSON `json:"wsSettings"`
	HTTPUpgradeSettings *xrayWebSocketJSON `json:"httpupgradeSettings"`
	GRPCSettings        *struct {
		ServiceName string `json:"serviceName"`
	} `json:"grpcSettings"`
	XHTTPSettings     json.RawMessage `json:"xhttpSettings"`
	SplitHTTPSettings json.RawMessage `json:"splithttpSettings"`
	HysteriaSettings  *struct {
		Version xrayInt `json:"version"`
		Auth    string  `json:"auth"`
	} `json:"hysteriaSettings"`
	FinalMask *xrayFinalMaskJSON `json:"finalmask"`
	Sockopt   *xraySockoptJSON   `json:"sockopt"`
}

type xrayTLSJSON struct {
	ServerName           string         `json:"serverName"`
	AllowInsecure        xrayFlag       `json:"allowInsecure"`
	ALPN                 xrayStringList `json:"alpn"`
	Fingerprint          string         `json:"fingerprint"`
	MinVersion           string         `json:"minVersion"`
	MaxVersion           string         `json:"maxVersion"`
	PinnedPeerCertSha256 string         `json:"pinnedPeerCertSha256"`
	VerifyPeerCertByName string         `json:"verifyPeerCertByName"`
	ECHConfigList        string         `json:"echConfigList"`
}

type xrayRealityJSON struct {
	ServerName  string `json:"serverName"`
	Fingerprint string `json:"fingerprint"`
	Password    string `json:"password"`
	PublicKey   string `json:"publicKey"`
	ShortID     string `json:"shortId"`
	SpiderX     string `json:"spiderX"`
}

type xrayRawJSON struct {
	Header *struct {
		Type    string `json:"type"`
		Request *struct {
			Path    xrayStringList            `json:"path"`
			Headers map[string]xrayStringList `json:"headers"`
		} `json:"request"`
	} `json:"header"`
}

type xrayWebSocketJSON struct {
	Host    string            `json:"host"`
	Path    string            `json:"path"`
	Headers map[string]string `json:"headers"`
}

type xrayFinalMaskJSON struct {
	TCP        []xrayMaskJSON `json:"tcp"`
	UDP        []xrayMaskJSON `json:"udp"`
	QuicParams *struct {
		Congestion string   `json:"congestion"`
		BrutalUp   xrayText `json:"brutalUp"`
		BrutalDown xrayText `json:"brutalDown"`
	} `json:"quicParams"`
}

type xrayMaskJSON struct {
	Type     string          `json:"type"`
	Settings json.RawMessage `json:"settings"`
}

type xraySockoptJSON struct {
	DialerProxy string `json:"dialerProxy"`
}

func xrayServer(protocol string, raw json.RawMessage, direct []string) (Profile, error) {
	switch protocol {
	case "":
		return Profile{}, errors.New("the Xray outbound names no protocol")
	case "vless", "vmess", "trojan", "shadowsocks", "socks", "http", "hysteria", "wireguard":
	default:
		return Profile{}, unsupported("Xray's %s protocol is not supported", protocol)
	}
	var out xrayOutboundJSON
	if err := json.Unmarshal(raw, &out); err != nil {
		return Profile{}, xrayReadError("", err)
	}
	stream := out.StreamSettings
	if stream == nil {
		stream = &xrayStreamJSON{}
	}
	if out.ProxySettings != nil && strings.TrimSpace(out.ProxySettings.Tag) != "" {
		return Profile{}, xrayChainError(out.ProxySettings.Tag)
	}
	if stream.Sockopt != nil {
		if through := strings.TrimSpace(stream.Sockopt.DialerProxy); through != "" && !slices.Contains(direct, through) {
			return Profile{}, xrayChainError(through)
		}
	}

	if protocol == "wireguard" {
		return xrayWireGuard(out.Settings)
	}

	var s xraySettingsJSON
	if xrayPresent(out.Settings) {
		if err := json.Unmarshal(out.Settings, &s); err != nil {
			return Profile{}, xrayReadError("settings", err)
		}
	}
	var list []xrayServerJSON
	switch protocol {
	case "vless", "vmess":
		list = s.Vnext
	case "trojan", "shadowsocks", "socks", "http":
		list = s.Servers
	}
	address, port, account := s.endpoint(list)
	if port.bad {
		return Profile{}, errors.New("the server port is not a number")
	}
	server, err := xrayHost(address)
	if err != nil {
		return Profile{}, err
	}
	p := Profile{Server: server, ServerPort: port.number}

	if protocol == "hysteria" {
		if p.Hysteria2, err = xrayHysteria2(&s, stream, server); err != nil {
			return Profile{}, err
		}
		p.Type = TypeHysteria2
		if p.ServerPort == 0 && len(p.Hysteria2.ServerPorts) > 0 {
			first, _, _ := strings.Cut(p.Hysteria2.ServerPorts[0], ":")
			p.ServerPort, _ = strconv.Atoi(first)
		}
		return p, nil
	}

	if err := xrayFinalMask(stream.FinalMask, nil); err != nil {
		return Profile{}, err
	}
	transport, err := xrayTransport(stream)
	if err != nil {
		return Profile{}, err
	}
	tls, err := xrayTLS(stream, server, false)
	if err != nil {
		return Profile{}, err
	}
	switch protocol {
	case "vless":
		v := &VLESS{
			UUID:           account.ID,
			Flow:           vlessLinkFlow(account.Flow),
			Encryption:     strings.TrimSpace(account.Encryption),
			PacketEncoding: "xudp",
			TLS:            tls,
			Transport:      transport,
		}
		// Xray demands "none"; Qt read a missing one as none too.
		if strings.EqualFold(v.Encryption, "none") {
			v.Encryption = ""
		}
		fitVisionFlow(v)
		p.Type, p.VLESS = TypeVLESS, v
	case "vmess":
		p.Type, p.VMess = TypeVMess, &VMess{
			UUID:           account.ID,
			AlterID:        int(account.AlterID),
			Security:       vmessLinkSecurity(account.Security),
			PacketEncoding: "xudp",
			TLS:            tls,
			Transport:      transport,
		}
	case "trojan":
		p.Type, p.Trojan = TypeTrojan, &Trojan{Password: account.Password, TLS: tls, Transport: transport}
	case "shadowsocks":
		if err := xrayBareStream("Shadowsocks", transport, tls, false); err != nil {
			return Profile{}, err
		}
		p.Type, p.Shadowsocks = TypeShadowsocks, &Shadowsocks{
			Method:     xrayShadowsocksMethod(account.Method),
			Password:   account.Password,
			UDPOverTCP: bool(account.UoT),
		}
	case "socks":
		if err := xrayBareStream("SOCKS", transport, tls, false); err != nil {
			return Profile{}, err
		}
		p.Type, p.SOCKS = TypeSOCKS, &SOCKS{Version: "5", Username: account.User, Password: account.Pass}
	case "http":
		if err := xrayBareStream("An HTTP proxy", transport, tls, true); err != nil {
			return Profile{}, err
		}
		h := &HTTP{Username: account.User, Password: account.Pass, TLS: tls}
		if len(s.Headers) > 0 {
			h.Headers = s.Headers
		}
		p.Type, p.HTTP = TypeHTTP, h
	}
	return p, nil
}

func xrayChainError(through string) error {
	return unsupported("the outbound connects through the outbound %q; chains are not supported", strings.TrimSpace(through))
}

// xrayReadError names the key whose value has the wrong type, never the value: it may be a password.
func xrayReadError(at string, err error) error {
	var typeErr *json.UnmarshalTypeError
	if errors.As(err, &typeErr) {
		path := strings.Trim(at+"."+typeErr.Field, ".")
		if path != "" {
			return fmt.Errorf("the Xray outbound cannot be read: %s has a value of the wrong type", path)
		}
	}
	return errors.New("the Xray outbound cannot be read")
}

// xrayPresent tells a value from a missing key or null.
func xrayPresent(raw json.RawMessage) bool {
	return len(raw) > 0 && string(raw) != "null"
}

func xrayHost(address *string) (string, error) {
	if address == nil {
		return "", nil
	}
	host := strings.TrimSpace(*address)
	host = strings.TrimSuffix(strings.TrimPrefix(host, "["), "]")
	return normalizeLinkHost(host)
}

var xrayShadowsocksAliases = map[string]string{
	"aead_aes_128_gcm":        "aes-128-gcm",
	"aead_aes_256_gcm":        "aes-256-gcm",
	"chacha20-poly1305":       "chacha20-ietf-poly1305",
	"aead_chacha20_poly1305":  "chacha20-ietf-poly1305",
	"xchacha20-poly1305":      "xchacha20-ietf-poly1305",
	"aead_xchacha20_poly1305": "xchacha20-ietf-poly1305",
}

// xrayShadowsocksMethod maps Xray's other names of the AEAD ciphers onto the ones the model stores.
func xrayShadowsocksMethod(raw string) string {
	method := strings.ToLower(strings.TrimSpace(raw))
	if alias, ok := xrayShadowsocksAliases[method]; ok {
		return alias
	}
	return method
}

// xrayBareStream refuses a transport or TLS the protocol's part cannot hold: the server expects them.
func xrayBareStream(what string, t Transport, tls TLS, tlsAllowed bool) error {
	if t.Type != TransportTCP {
		return unsupported("%s over the %s transport is not supported", what, t.Type)
	}
	if t.HeaderType != "" {
		return unsupported("%s with the %s header is not supported", what, t.HeaderType)
	}
	if tls.Enabled && !tlsAllowed {
		return unsupported("%s over TLS is not supported", what)
	}
	return nil
}

func xrayNetwork(stream *xrayStreamJSON) string {
	network := stream.Network
	if stream.Method != nil {
		network = stream.Method
	}
	if network == nil {
		return ""
	}
	return strings.ToLower(strings.TrimSpace(*network))
}

func xrayTransport(stream *xrayStreamJSON) (Transport, error) {
	switch network := xrayNetwork(stream); network {
	case "", "raw", "tcp":
		raw := stream.RawSettings
		if raw == nil {
			raw = stream.TCPSettings
		}
		return xrayRawTransport(raw)
	case "ws", "websocket":
		t := Transport{Type: TransportWebSocket}
		if c := stream.WSSettings; c != nil {
			t.Path = c.Path
			t.Host, t.Headers = xrayHostHeaders(c.Host, c.Headers)
		}
		if path, ed, ok := cutEarlyData(t.Path); ok {
			t.Path, t.MaxEarlyData, t.EarlyDataHeaderName = path, ed, wsEarlyDataHeader
		}
		return t, nil
	case "httpupgrade":
		t := Transport{Type: TransportHTTPUpgrade}
		if c := stream.HTTPUpgradeSettings; c != nil {
			t.Path = c.Path
			t.Host, t.Headers = xrayHostHeaders(c.Host, c.Headers)
		}
		// sing-box HTTPUpgrade has no early data, so a ?ed= only leaves the path.
		if path, _, ok := cutEarlyData(t.Path); ok {
			t.Path = path
		}
		return t, nil
	case "grpc":
		t := Transport{Type: TransportGRPC}
		if c := stream.GRPCSettings; c != nil {
			t.ServiceName = c.ServiceName
		}
		return t, nil
	case "xhttp", "splithttp":
		// xhttpSettings outranks Xray's older splithttpSettings.
		settings, at := stream.XHTTPSettings, "streamSettings.xhttpSettings"
		if !xrayPresent(settings) {
			settings, at = stream.SplitHTTPSettings, "streamSettings.splithttpSettings"
		}
		return xrayXHTTP(settings, at)
	default:
		return Transport{}, unsupported("the transport %q is not supported", network)
	}
}

// xrayHostHeaders moves a Host header to the host, as Xray's WebSocket does; an independent host wins.
func xrayHostHeaders(host string, headers map[string]string) (string, map[string]string) {
	var kept map[string]string
	for _, name := range slices.Sorted(maps.Keys(headers)) {
		if strings.EqualFold(name, "host") {
			if host == "" {
				host = headers[name]
			}
			continue
		}
		if strings.TrimSpace(name) == "" {
			continue
		}
		if kept == nil {
			kept = map[string]string{}
		}
		kept[name] = headers[name]
	}
	return host, kept
}

// xrayRawTransport reads Xray's HTTP header obfuscation; Host and Path keep the comma lists it picks from, as the
// links write them.
func xrayRawTransport(raw *xrayRawJSON) (Transport, error) {
	if raw == nil || raw.Header == nil {
		return Transport{}, nil
	}
	switch kind := strings.ToLower(strings.TrimSpace(raw.Header.Type)); kind {
	case "", "none":
		return Transport{}, nil
	case "http":
		t := Transport{HeaderType: "http"}
		if request := raw.Header.Request; request != nil {
			t.Path = strings.Join(linkList(strings.Join(request.Path, ",")), ",")
			for _, name := range slices.Sorted(maps.Keys(request.Headers)) {
				if strings.EqualFold(name, "host") {
					t.Host = strings.Join(linkList(strings.Join(request.Headers[name], ",")), ",")
				}
			}
		}
		return t, nil
	default:
		return Transport{}, unsupported("the raw header %q is not supported", kind)
	}
}

func xrayXHTTP(settings json.RawMessage, at string) (Transport, error) {
	t := Transport{Type: TransportXHTTP, XHTTP: XHTTP{Mode: "auto"}}
	if !xrayPresent(settings) {
		return t, nil
	}
	var c struct {
		Host  string          `json:"host"`
		Path  string          `json:"path"`
		Mode  string          `json:"mode"`
		Extra json.RawMessage `json:"extra"`
	}
	if err := json.Unmarshal(settings, &c); err != nil {
		return Transport{}, xrayReadError(at, err)
	}
	t.Host, t.Path = c.Host, c.Path
	// Xray refuses an unknown mode; Qt fell back to auto.
	if mode := strings.TrimSpace(c.Mode); slices.Contains(xhttpModes, mode) {
		t.XHTTP.Mode = mode
	}
	t.XHTTP.Extra = xrayXHTTPExtra(settings, c.Extra)
	if err := xrayDownloadChain(t.XHTTP.Extra); err != nil {
		return Transport{}, err
	}
	return t, nil
}

// xrayXHTTPExtra returns what Xray reads besides host, path and mode, so the builder can always write it as extra.
// Next to an extra object Xray reads only those three keys of xhttpSettings; without one, it reads every key there.
func xrayXHTTPExtra(settings, extra json.RawMessage) json.RawMessage {
	if extra != nil {
		if !xrayPresent(extra) {
			return nil
		}
		// Qt also took the object written as a JSON string.
		var text string
		if json.Unmarshal(extra, &text) == nil {
			return linkXHTTPExtra(text)
		}
		return linkXHTTPExtra(string(extra))
	}
	var members []string
	for _, m := range xrayMembers(settings) {
		if slices.ContainsFunc([]string{"host", "path", "mode", "extra"}, func(key string) bool { return strings.EqualFold(key, m.key) }) {
			continue
		}
		key, _ := json.Marshal(m.key)
		members = append(members, string(key)+":"+string(m.value))
	}
	return linkXHTTPExtra("{" + strings.Join(members, ",") + "}")
}

type xrayMember struct {
	key   string
	value json.RawMessage
}

// xrayMembers lists the keys of a JSON object in their order, duplicates included.
func xrayMembers(raw json.RawMessage) []xrayMember {
	decoder := json.NewDecoder(bytes.NewReader(raw))
	if token, err := decoder.Token(); err != nil || token != json.Delim('{') {
		return nil
	}
	var members []xrayMember
	for decoder.More() {
		token, err := decoder.Token()
		if err != nil {
			return nil
		}
		key, _ := token.(string)
		var value json.RawMessage
		if err := decoder.Decode(&value); err != nil {
			return nil
		}
		members = append(members, xrayMember{key, value})
	}
	return members
}

// xrayDownloadChain refuses an XHTTP download leg that dials through another outbound.
func xrayDownloadChain(extra json.RawMessage) error {
	var leg struct {
		DownloadSettings *struct {
			Sockopt *xraySockoptJSON `json:"sockopt"`
		} `json:"downloadSettings"`
	}
	if extra == nil || json.Unmarshal(extra, &leg) != nil || leg.DownloadSettings == nil {
		return nil
	}
	if sockopt := leg.DownloadSettings.Sockopt; sockopt != nil && strings.TrimSpace(sockopt.DialerProxy) != "" {
		return xrayChainError(sockopt.DialerProxy)
	}
	return nil
}

// xrayTLS reads the security block that security selects, as Xray does. forced is for a protocol that always runs
// over TLS.
func xrayTLS(stream *xrayStreamJSON, server string, forced bool) (TLS, error) {
	security := strings.ToLower(strings.TrimSpace(stream.Security))
	if forced && (security == "" || security == "none") {
		security = "tls"
	}
	switch security {
	case "", "none":
		return TLS{}, nil
	case "tls":
		return xrayTLSSettings(stream.TLSSettings, server)
	case "reality":
		return xrayReality(stream.RealitySettings)
	case "xtls":
		return TLS{}, unsupported("legacy XTLS is not supported")
	default:
		return TLS{}, fmt.Errorf("the security %q is unknown", security)
	}
}

func xrayTLSSettings(c *xrayTLSJSON, server string) (TLS, error) {
	t := TLS{Enabled: true}
	if c == nil {
		return t, nil
	}
	t.ServerName = c.ServerName
	t.Insecure = bool(c.AllowInsecure)
	// Xray takes a list or one comma-separated string; Qt split list items on commas too.
	t.ALPN = linkList(strings.Join(c.ALPN, ","))
	for _, version := range []struct {
		value  string
		target *string
	}{{c.MinVersion, &t.MinVersion}, {c.MaxVersion, &t.MaxVersion}} {
		if value := strings.TrimSpace(version.value); slices.Contains([]string{"1.0", "1.1", "1.2", "1.3"}, value) {
			*version.target = value
		}
	}
	pins, err := linkCertificatePins(c.PinnedPeerCertSha256)
	if err != nil {
		return TLS{}, err
	}
	t.CertificateSHA256 = pins
	// Xray then checks the certificate against these names instead of the server name. Without a pin, which
	// replaces the check, dropping them would refuse a server whose certificate is not for the name it is sent.
	if names := linkList(c.VerifyPeerCertByName); len(names) > 0 && len(pins) == 0 {
		name := t.ServerName
		if name == "" {
			name = server
		}
		if len(names) != 1 || !strings.EqualFold(names[0], name) {
			return TLS{}, unsupported("verifyPeerCertByName is not supported: the certificate can only be checked against the server name")
		}
	}
	if fingerprint := strings.ToLower(strings.TrimSpace(c.Fingerprint)); slices.Contains(linkFingerprints, fingerprint) {
		t.UTLS = UTLS{Enabled: true, Fingerprint: fingerprint}
	}
	t.ECH = xrayECH(c.ECHConfigList)
	return t, nil
}

// xrayECH reads echConfigList: a base64 ECHConfigList, or a DNS server to ask, "name+https://resolver" or the
// resolver alone for the server name. The resolver has no field in the profile, so only the name is kept.
func xrayECH(value string) ECH {
	value = strings.TrimSpace(value)
	if value == "" {
		return ECH{}
	}
	e := ECH{Enabled: true}
	if scheme := strings.Index(value, "://"); scheme >= 0 {
		if plus := strings.IndexByte(value, '+'); plus > 0 && plus < scheme {
			e.QueryServerName = strings.TrimSpace(value[:plus])
		}
		return e
	}
	e.Config = linkECHConfig([]string{value})
	return e
}

func xrayReality(c *xrayRealityJSON) (TLS, error) {
	if c == nil {
		c = &xrayRealityJSON{}
	}
	// Xray reads publicKey, the older name, when password is empty.
	key := c.Password
	if strings.TrimSpace(key) == "" {
		key = c.PublicKey
	}
	key = linkRealityKey(key)
	if key == "" {
		return TLS{}, errors.New("REALITY needs the server's public key, realitySettings.password")
	}
	t := TLS{Enabled: true, ServerName: c.ServerName, Reality: Reality{
		Enabled: true, PublicKey: key, ShortID: strings.TrimSpace(c.ShortID),
	}}
	if fingerprint := strings.ToLower(strings.TrimSpace(c.Fingerprint)); slices.Contains(linkFingerprints, fingerprint) {
		t.UTLS = UTLS{Enabled: true, Fingerprint: fingerprint}
	}
	// Xray refuses a spiderX that is not a path; Qt dropped it.
	if spiderX := strings.TrimSpace(c.SpiderX); spiderX == "" || strings.HasPrefix(spiderX, "/") {
		t.Reality.SpiderX = c.SpiderX
	}
	return t, nil
}

// xrayHysteria2 reads Xray's Hysteria2: version 2 in settings, the password in hysteriaSettings, and the obfuscation,
// port hopping and bandwidth in finalmask.
func xrayHysteria2(s *xraySettingsJSON, stream *xrayStreamJSON, server string) (*Hysteria2, error) {
	h := &Hysteria2{}
	versions := []xrayInt{s.Version}
	if c := stream.HysteriaSettings; c != nil {
		h.Password = c.Auth
		versions = append(versions, c.Version)
	}
	// Xray demands version 2; a missing one is taken for it.
	for _, version := range versions {
		if version != 0 && version != 2 {
			return nil, unsupported("Hysteria version %d is not supported; only Hysteria2 is", version)
		}
	}
	if network := xrayNetwork(stream); network != "" && network != "hysteria" {
		return nil, fmt.Errorf("Hysteria2 runs only over the hysteria transport, not %q", network)
	}
	if err := xrayFinalMask(stream.FinalMask, h); err != nil {
		return nil, err
	}
	tls, err := xrayTLS(stream, server, true)
	if err != nil {
		return nil, err
	}
	h.TLS = quicLinkTLS(tls)
	return h, nil
}

// xrayFinalMask checks the masks of the outbound's own transport: TCP ones, or UDP ones for Hysteria2 (h), whose
// salamander obfuscation, port hopping and bandwidth live here. A mask the client applies alone, such as fragments
// or noise, is dropped; one the server must undo would leave a server that cannot connect.
func xrayFinalMask(m *xrayFinalMaskJSON, h *Hysteria2) error {
	if m == nil {
		return nil
	}
	if h == nil {
		for _, mask := range m.TCP {
			if kind := strings.ToLower(strings.TrimSpace(mask.Type)); kind != "fragment" && kind != "filter" {
				return unsupported("the finalmask %q is not supported", kind)
			}
		}
		return nil
	}
	for _, mask := range m.UDP {
		switch kind := strings.ToLower(strings.TrimSpace(mask.Type)); kind {
		case "noise", "filter":
		case "salamander":
			var c struct {
				Password   string   `json:"password"`
				PacketSize xrayText `json:"packetSize"`
			}
			if err := json.Unmarshal(jsonOr(mask.Settings, "{}"), &c); err != nil {
				return xrayReadError("streamSettings.finalmask.udp.settings", err)
			}
			// Xray's salamander with packet sizes is gecko, which no core of Poros runs.
			if _, to, _ := xrayRange(string(c.PacketSize)); to > 0 {
				return unsupported("the Hysteria2 obfuscation %q is not supported", "gecko")
			}
			if c.Password != "" {
				h.ObfsType, h.ObfsPassword = "salamander", c.Password
			}
		case "udphop":
			var c struct {
				RemotePorts xrayText `json:"remotePorts"`
				Interval    xrayText `json:"interval"`
			}
			if err := json.Unmarshal(jsonOr(mask.Settings, "{}"), &c); err != nil {
				return xrayReadError("streamSettings.finalmask.udp.settings", err)
			}
			ports, err := hysteriaPortRanges(string(c.RemotePorts))
			if err != nil {
				return err
			}
			h.ServerPorts = ports
			// The model holds one interval; a range keeps its shortest.
			if from, _, ok := xrayRange(string(c.Interval)); ok && from > 0 {
				h.HopInterval = strconv.Itoa(from) + "s"
			}
		default:
			return unsupported("the finalmask %q is not supported", kind)
		}
	}
	// Bandwidth means Brutal, which Xray skips for an explicit bbr or reno.
	if q := m.QuicParams; q != nil {
		switch strings.ToLower(strings.TrimSpace(q.Congestion)) {
		case "", "brutal", "force-brutal":
			// Xray reads the units case-insensitively, so "MB" is megabits there too.
			h.UpMbps = hysteriaLinkMbps(strings.ToLower(string(q.BrutalUp)))
			h.DownMbps = hysteriaLinkMbps(strings.ToLower(string(q.BrutalDown)))
		}
	}
	return nil
}

// xrayRange reads Xray's integer range, "10-30" or a single number, in order.
func xrayRange(text string) (from, to int, ok bool) {
	text = strings.TrimSpace(text)
	if text == "" {
		return 0, 0, false
	}
	first, second := text, text
	// A leading '-' is a sign.
	if i := strings.IndexByte(text[1:], '-'); i >= 0 {
		first, second = text[:i+1], text[i+2:]
	}
	from, errFrom := strconv.Atoi(strings.TrimSpace(first))
	to, errTo := strconv.Atoi(strings.TrimSpace(second))
	if errFrom != nil || errTo != nil {
		return 0, 0, false
	}
	return min(from, to), max(from, to), true
}

// The types below are lenient: a value of another type is dropped rather than breaking the outbound. Their errors
// would carry no path to the key either, as encoding/json returns them unchanged.

// xrayPort reads a port that generators write as a JSON number or as a string.
type xrayPort struct {
	number int
	// bad is a port that is not a whole number, which xrayServer reports.
	bad bool
}

func (p *xrayPort) UnmarshalJSON(data []byte) error {
	*p = xrayPort{}
	var text xrayText
	_ = text.UnmarshalJSON(data)
	value := strings.TrimSpace(string(text))
	if value == "" {
		p.bad = string(data) != "null" && string(data) != `""`
		return nil
	}
	number, err := strconv.Atoi(value)
	p.number, p.bad = number, err != nil
	return nil
}

// xrayInt reads a number written as a JSON number or as a string; anything else is 0.
type xrayInt int

func (n *xrayInt) UnmarshalJSON(data []byte) error {
	var text xrayText
	_ = text.UnmarshalJSON(data)
	number, _ := strconv.Atoi(strings.TrimSpace(string(text)))
	*n = xrayInt(number)
	return nil
}

// xrayText reads a string that may also be written as a number, as Xray's ranges and port lists are.
type xrayText string

func (t *xrayText) UnmarshalJSON(data []byte) error {
	*t = ""
	var text string
	if json.Unmarshal(data, &text) == nil {
		*t = xrayText(text)
		return nil
	}
	var number json.Number
	if json.Unmarshal(data, &number) == nil {
		*t = xrayText(number.String())
	}
	return nil
}

// xrayFlag is true only for JSON true.
type xrayFlag bool

func (f *xrayFlag) UnmarshalJSON(data []byte) error {
	*f = string(bytes.TrimSpace(data)) == "true"
	return nil
}

// xrayStringList is Xray's StringList: a list of strings, or one string split on commas.
type xrayStringList []string

func (l *xrayStringList) UnmarshalJSON(data []byte) error {
	*l = nil
	var items []string
	if json.Unmarshal(data, &items) == nil {
		*l = items
		return nil
	}
	var text string
	if json.Unmarshal(data, &text) == nil {
		*l = strings.Split(text, ",")
	}
	return nil
}
