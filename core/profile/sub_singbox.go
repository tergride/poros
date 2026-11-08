package profile

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"math"
	"net/url"
	"slices"
	"strconv"
	"strings"
)

// singBoxInfraTypes are the outbounds every full config carries that are not servers; they are left out silently.
var singBoxInfraTypes = []string{"direct", "block", "dns", "selector", "urltest"}

var singBoxParsers = map[string]func(json.RawMessage) (Profile, error){
	TypeVLESS:       singBoxVLESSOutbound,
	TypeVMess:       singBoxVMessOutbound,
	TypeTrojan:      singBoxTrojanOutbound,
	TypeShadowsocks: singBoxShadowsocksOutbound,
	TypeHysteria2:   singBoxHysteria2Outbound,
	TypeTUIC:        singBoxTUICOutbound,
	TypeSOCKS:       singBoxSOCKSOutbound,
	TypeHTTP:        singBoxHTTPOutbound,
	TypeAnyTLS:      singBoxAnyTLSOutbound,
	TypeWireGuard:   singBoxWireGuardOutbound,
	TypeHysteria:    singBoxHysteriaOutbound,
	TypeNaive:       singBoxNaiveOutbound,
	TypeSSH:         singBoxSSHOutbound,
	TypeSnell:       singBoxSnellOutbound,
}

// singBoxOutbound reads one outbound or endpoint of a sing-box config. What is not an object is left out, as in the
// Qt client. A plain field of the wrong type makes the entry broken, as sing-box would refuse it too; only the
// shapes sing-box itself lets vary are lenient.
func singBoxOutbound(raw json.RawMessage) (p Profile, tag string, proxy bool, err error) {
	return singBoxConfigOutbound(raw, nil)
}

// singBoxConfigOutbound reads an outbound of a whole config, whose ShadowTLS outbounds shadowTLS holds by tag. A
// ShadowTLS outbound is part of the Shadowsocks server that goes through it, not a server of its own.
func singBoxConfigOutbound(raw json.RawMessage, shadowTLS map[string]json.RawMessage) (p Profile, tag string, proxy bool, err error) {
	var head struct {
		Type any `json:"type"`
		Tag  any `json:"tag"`
	}
	if raw = bytes.TrimSpace(raw); len(raw) == 0 || raw[0] != '{' || json.Unmarshal(raw, &head) != nil {
		return Profile{}, "", false, nil
	}
	tag, _ = head.Tag.(string)
	tag = strings.TrimSpace(tag)
	kind, _ := head.Type.(string)
	// sing-box matches types exactly; a capitalised one is still clear.
	kind = strings.ToLower(strings.TrimSpace(kind))
	if slices.Contains(singBoxInfraTypes, kind) {
		return Profile{}, tag, false, nil
	}
	if kind == "" {
		return Profile{}, tag, true, errors.New("the sing-box outbound has no type")
	}
	if kind == "shadowtls" {
		return Profile{}, tag, false, nil
	}
	if kind == TypeShadowsocks {
		var detour struct {
			Detour string `json:"detour"`
		}
		_ = json.Unmarshal(raw, &detour)
		if through, ok := shadowTLS[strings.TrimSpace(detour.Detour)]; ok {
			p, err = singBoxShadowsocksOverShadowTLS(raw, through)
			return p, tag, true, err
		}
	}
	parse, ok := singBoxParsers[kind]
	if !ok {
		return Profile{}, tag, true, unsupported("the sing-box type %q is not supported", kind)
	}
	if p, err = parse(raw); err != nil {
		return Profile{}, tag, true, err
	}
	return p, tag, true, nil
}

// singBoxDecode fills one of the option structs. Its errors name the field, never the value: it may be a password.
func singBoxDecode(raw json.RawMessage, options any) error {
	err := json.Unmarshal(raw, options)
	if err == nil {
		return nil
	}
	var typeErr *json.UnmarshalTypeError
	if errors.As(err, &typeErr) && typeErr.Field != "" {
		return fmt.Errorf("the sing-box option %s has the wrong type", typeErr.Field)
	}
	return errors.New("the sing-box outbound cannot be read")
}

// The custom types below drop a value of another shape, as the Qt client did, rather than refuse the outbound.

// singBoxList is sing-box's Listable: one string or a list of them. Items that are not text are dropped.
type singBoxList []string

func (l *singBoxList) UnmarshalJSON(data []byte) error {
	var value any
	if json.Unmarshal(data, &value) != nil {
		return nil
	}
	switch value := value.(type) {
	case string:
		*l = singBoxList{value}
	case []any:
		var items singBoxList
		for _, item := range value {
			if text, ok := item.(string); ok {
				items = append(items, text)
			}
		}
		*l = items
	}
	return nil
}

// singBoxHeaders is sing-box's HTTPHeader, whose values are Listable too.
type singBoxHeaders map[string]singBoxList

func (h *singBoxHeaders) UnmarshalJSON(data []byte) error {
	var headers map[string]singBoxList
	if json.Unmarshal(data, &headers) == nil {
		*h = headers
	}
	return nil
}

// first keeps the first value of each header: the profile holds one.
func (h singBoxHeaders) first() map[string]string {
	headers := map[string]string{}
	for name, values := range h {
		if strings.TrimSpace(name) != "" && len(values) > 0 {
			headers[name] = values[0]
		}
	}
	if len(headers) == 0 {
		return nil
	}
	return headers
}

// singBoxUDPOverTCP is udp_over_tcp: a bool, or an object with enabled and version.
type singBoxUDPOverTCP bool

func (u *singBoxUDPOverTCP) UnmarshalJSON(data []byte) error {
	var enabled bool
	if json.Unmarshal(data, &enabled) == nil {
		*u = singBoxUDPOverTCP(enabled)
		return nil
	}
	var object struct {
		Enabled bool `json:"enabled"`
	}
	if json.Unmarshal(data, &object) == nil {
		*u = singBoxUDPOverTCP(object.Enabled)
	}
	return nil
}

// singBoxCount drops a negative number, which sing-box refuses.
func singBoxCount(value int) int {
	return min(max(value, 0), math.MaxInt32)
}

type singBoxServerOptions struct {
	Detour     string `json:"detour"`
	Server     string `json:"server"`
	ServerPort int    `json:"server_port"`
}

func (o singBoxServerOptions) profile(kind string) (Profile, error) {
	if detour := strings.TrimSpace(o.Detour); detour != "" {
		return Profile{}, unsupported("the server is reached through the outbound %q, and chains are not supported", detour)
	}
	host := strings.TrimSpace(o.Server)
	host = strings.TrimSuffix(strings.TrimPrefix(host, "["), "]")
	server, err := normalizeLinkHost(host)
	if err != nil {
		return Profile{}, err
	}
	return Profile{Type: kind, Server: server, ServerPort: o.ServerPort}, nil
}

type singBoxTLSOptions struct {
	Enabled                    bool                   `json:"enabled"`
	DisableSNI                 bool                   `json:"disable_sni"`
	ServerName                 string                 `json:"server_name"`
	Insecure                   bool                   `json:"insecure"`
	ALPN                       singBoxList            `json:"alpn"`
	MinVersion                 string                 `json:"min_version"`
	MaxVersion                 string                 `json:"max_version"`
	CertificateSHA256          singBoxList            `json:"certificate_sha256"`
	CertificatePublicKeySHA256 singBoxList            `json:"certificate_public_key_sha256"`
	ECH                        *singBoxECHOptions     `json:"ech"`
	UTLS                       *singBoxUTLSOptions    `json:"utls"`
	Reality                    *singBoxRealityOptions `json:"reality"`
}

type singBoxECHOptions struct {
	Enabled         bool        `json:"enabled"`
	Config          singBoxList `json:"config"`
	QueryServerName string      `json:"query_server_name"`
}

type singBoxUTLSOptions struct {
	Enabled     bool   `json:"enabled"`
	Fingerprint string `json:"fingerprint"`
}

type singBoxRealityOptions struct {
	Enabled   bool   `json:"enabled"`
	PublicKey string `json:"public_key"`
	ShortID   string `json:"short_id"`
}

// sing-box runs these, and an empty fingerprint, as chrome.
var singBoxChromeFingerprints = []string{"", "chrome_psk", "chrome_psk_shuffle", "chrome_padding_psk_shuffle", "chrome_pq", "chrome_pq_psk"}

// singBoxTLS reads a tls object. forced is for the protocols that always run over TLS, whatever enabled says.
func singBoxTLS(o *singBoxTLSOptions, forced bool) (TLS, error) {
	if o == nil {
		o = &singBoxTLSOptions{}
	}
	if !o.Enabled && !forced {
		return TLS{}, nil
	}
	t := TLS{
		Enabled:    true,
		ServerName: o.ServerName,
		DisableSNI: o.DisableSNI,
		Insecure:   o.Insecure,
		ALPN:       linkList(strings.Join(o.ALPN, ",")),
		MinVersion: singBoxTLSVersion(o.MinVersion),
		MaxVersion: singBoxTLSVersion(o.MaxVersion),
	}
	var err error
	if t.CertificateSHA256, err = linkCertificatePins(strings.Join(o.CertificateSHA256, ",")); err != nil {
		return TLS{}, err
	}
	if t.CertificatePublicKeySHA256, err = linkCertificatePins(strings.Join(o.CertificatePublicKeySHA256, ",")); err != nil {
		return TLS{}, err
	}
	if u := o.UTLS; u != nil && u.Enabled {
		fingerprint := strings.ToLower(strings.TrimSpace(u.Fingerprint))
		if slices.Contains(singBoxChromeFingerprints, fingerprint) {
			fingerprint = "chrome"
		}
		if slices.Contains(linkFingerprints, fingerprint) {
			t.UTLS = UTLS{Enabled: true, Fingerprint: fingerprint}
		}
	}
	if r := o.Reality; r != nil && r.Enabled {
		t.Reality = Reality{Enabled: true, PublicKey: linkRealityKey(r.PublicKey), ShortID: strings.TrimSpace(r.ShortID)}
	}
	if e := o.ECH; e != nil && e.Enabled {
		t.ECH = ECH{Enabled: true, Config: singBoxECHConfig(e.Config), QueryServerName: strings.TrimSpace(e.QueryServerName)}
	}
	return t, nil
}

func singBoxTLSVersion(raw string) string {
	if version := strings.TrimSpace(raw); slices.Contains([]string{"1.0", "1.1", "1.2", "1.3"}, version) {
		return version
	}
	return ""
}

// singBoxECHConfig keeps the PEM text one line per item, whether the config wrote it as one string or as a list.
func singBoxECHConfig(items []string) []string {
	var lines []string
	for _, item := range items {
		for line := range strings.SplitSeq(item, "\n") {
			if line = strings.TrimSpace(line); line != "" {
				lines = append(lines, line)
			}
		}
	}
	return linkECHConfig(lines)
}

type singBoxTransportOptions struct {
	Type                string         `json:"type"`
	Host                singBoxList    `json:"host"`
	Path                string         `json:"path"`
	Method              string         `json:"method"`
	Headers             singBoxHeaders `json:"headers"`
	MaxEarlyData        int            `json:"max_early_data"`
	EarlyDataHeaderName string         `json:"early_data_header_name"`
	ServiceName         string         `json:"service_name"`
}

func singBoxTransport(o *singBoxTransportOptions) (Transport, error) {
	if o == nil {
		return Transport{}, nil
	}
	var t Transport
	// websocket is Xray's name, which configs converted from it carry.
	switch kind := strings.ToLower(strings.TrimSpace(o.Type)); kind {
	case "", "tcp":
		return Transport{}, nil
	case "grpc":
		return Transport{Type: TransportGRPC, ServiceName: o.ServiceName}, nil
	case "quic":
		return Transport{Type: TransportQUIC}, nil
	case "ws", "websocket":
		t.Type = TransportWebSocket
	case "httpupgrade":
		t.Type = TransportHTTPUpgrade
	case "http":
		t.Type = TransportHTTP
	default:
		return Transport{}, unsupported("the transport %q is not supported", kind)
	}
	// The http transport picks one of several hosts, kept as a comma list.
	t.Host = strings.Join(linkList(strings.Join(o.Host, ",")), ",")
	t.Path = o.Path
	t.Headers = o.Headers.first()

	switch t.Type {
	case TransportWebSocket:
		// sing-box sends the Host header as the request's host, which the profile keeps apart.
		for name, value := range t.Headers {
			if strings.EqualFold(name, "Host") {
				t.Host = value
				delete(t.Headers, name)
			}
		}
		if len(t.Headers) == 0 {
			t.Headers = nil
		}
		t.MaxEarlyData, t.EarlyDataHeaderName = singBoxCount(o.MaxEarlyData), o.EarlyDataHeaderName
		if path, ed, ok := cutEarlyData(t.Path); ok {
			t.Path, t.MaxEarlyData, t.EarlyDataHeaderName = path, ed, wsEarlyDataHeader
		}
	case TransportHTTPUpgrade:
		if path, _, ok := cutEarlyData(t.Path); ok {
			t.Path = path
		}
	case TransportHTTP:
		if method := strings.TrimSpace(o.Method); method != "" && !strings.ContainsFunc(method, notHTTPTokenRune) {
			t.Method = method
		}
	}
	return t, nil
}

type singBoxMultiplexOptions struct {
	Enabled        bool   `json:"enabled"`
	Protocol       string `json:"protocol"`
	MaxConnections int    `json:"max_connections"`
	MinStreams     int    `json:"min_streams"`
	MaxStreams     int    `json:"max_streams"`
	Padding        bool   `json:"padding"`
}

func singBoxMultiplex(o *singBoxMultiplexOptions) Multiplex {
	if o == nil || !o.Enabled {
		return Multiplex{}
	}
	m := Multiplex{
		Enabled:        true,
		MaxConnections: singBoxCount(o.MaxConnections),
		MinStreams:     singBoxCount(o.MinStreams),
		MaxStreams:     singBoxCount(o.MaxStreams),
		Padding:        o.Padding,
	}
	if protocol := strings.TrimSpace(o.Protocol); slices.Contains(multiplexProtocols, protocol) {
		m.Protocol = protocol
	}
	return m
}

// singBoxStream reads the TLS and transport of the V2Ray protocols.
func singBoxStream(tlsOptions *singBoxTLSOptions, transportOptions *singBoxTransportOptions) (TLS, Transport, error) {
	tls, err := singBoxTLS(tlsOptions, false)
	if err != nil {
		return TLS{}, Transport{}, err
	}
	transport, err := singBoxTransport(transportOptions)
	if err != nil {
		return TLS{}, Transport{}, err
	}
	return tls, transport, nil
}

type singBoxVLESSOptions struct {
	singBoxServerOptions
	UUID           string                   `json:"uuid"`
	Flow           string                   `json:"flow"`
	PacketEncoding *string                  `json:"packet_encoding"`
	TLS            *singBoxTLSOptions       `json:"tls"`
	Transport      *singBoxTransportOptions `json:"transport"`
	Multiplex      *singBoxMultiplexOptions `json:"multiplex"`
}

func singBoxVLESSOutbound(raw json.RawMessage) (Profile, error) {
	var o singBoxVLESSOptions
	if err := singBoxDecode(raw, &o); err != nil {
		return Profile{}, err
	}
	p, err := o.profile(TypeVLESS)
	if err != nil {
		return Profile{}, err
	}
	// sing-box's VLESS uses xudp unless packet_encoding names another, the empty one included.
	v := &VLESS{UUID: o.UUID, Flow: vlessLinkFlow(o.Flow), PacketEncoding: "xudp", Multiplex: singBoxMultiplex(o.Multiplex)}
	if o.PacketEncoding != nil {
		v.PacketEncoding = linkPacketEncoding(*o.PacketEncoding)
	}
	if v.TLS, v.Transport, err = singBoxStream(o.TLS, o.Transport); err != nil {
		return Profile{}, err
	}
	fitVisionFlow(v)
	p.VLESS = v
	return p, nil
}

type singBoxVMessOptions struct {
	singBoxServerOptions
	UUID           string                   `json:"uuid"`
	Security       string                   `json:"security"`
	AlterID        int                      `json:"alter_id"`
	PacketEncoding string                   `json:"packet_encoding"`
	TLS            *singBoxTLSOptions       `json:"tls"`
	Transport      *singBoxTransportOptions `json:"transport"`
	Multiplex      *singBoxMultiplexOptions `json:"multiplex"`
}

func singBoxVMessOutbound(raw json.RawMessage) (Profile, error) {
	var o singBoxVMessOptions
	if err := singBoxDecode(raw, &o); err != nil {
		return Profile{}, err
	}
	p, err := o.profile(TypeVMess)
	if err != nil {
		return Profile{}, err
	}
	// Unlike VLESS, sing-box's VMess has no packet encoding unless packet_encoding names one.
	v := &VMess{
		UUID:           o.UUID,
		AlterID:        singBoxCount(o.AlterID),
		Security:       vmessLinkSecurity(o.Security),
		PacketEncoding: linkPacketEncoding(o.PacketEncoding),
		Multiplex:      singBoxMultiplex(o.Multiplex),
	}
	if v.TLS, v.Transport, err = singBoxStream(o.TLS, o.Transport); err != nil {
		return Profile{}, err
	}
	p.VMess = v
	return p, nil
}

type singBoxTrojanOptions struct {
	singBoxServerOptions
	Password  string                   `json:"password"`
	TLS       *singBoxTLSOptions       `json:"tls"`
	Transport *singBoxTransportOptions `json:"transport"`
	Multiplex *singBoxMultiplexOptions `json:"multiplex"`
}

func singBoxTrojanOutbound(raw json.RawMessage) (Profile, error) {
	var o singBoxTrojanOptions
	if err := singBoxDecode(raw, &o); err != nil {
		return Profile{}, err
	}
	p, err := o.profile(TypeTrojan)
	if err != nil {
		return Profile{}, err
	}
	t := &Trojan{Password: o.Password, Multiplex: singBoxMultiplex(o.Multiplex)}
	if t.TLS, t.Transport, err = singBoxStream(o.TLS, o.Transport); err != nil {
		return Profile{}, err
	}
	p.Trojan = t
	return p, nil
}

type singBoxShadowsocksOptions struct {
	singBoxServerOptions
	Method     string                   `json:"method"`
	Password   string                   `json:"password"`
	Plugin     string                   `json:"plugin"`
	PluginOpts string                   `json:"plugin_opts"`
	UDPOverTCP singBoxUDPOverTCP        `json:"udp_over_tcp"`
	Multiplex  *singBoxMultiplexOptions `json:"multiplex"`
}

func singBoxShadowsocksOutbound(raw json.RawMessage) (Profile, error) {
	var o singBoxShadowsocksOptions
	if err := singBoxDecode(raw, &o); err != nil {
		return Profile{}, err
	}
	p, err := o.profile(TypeShadowsocks)
	if err != nil {
		return Profile{}, err
	}
	s := &Shadowsocks{
		Method:     strings.ToLower(strings.TrimSpace(o.Method)),
		Password:   o.Password,
		Plugin:     shadowsocksLinkPlugin(o.Plugin),
		UDPOverTCP: bool(o.UDPOverTCP),
		Multiplex:  singBoxMultiplex(o.Multiplex),
	}
	if s.Plugin != "" {
		s.PluginOpts = strings.TrimSpace(o.PluginOpts)
	}
	p.Shadowsocks = s
	return p, nil
}

type singBoxHysteria2Options struct {
	singBoxServerOptions
	ServerPorts singBoxList `json:"server_ports"`
	HopInterval string      `json:"hop_interval"`
	UpMbps      int         `json:"up_mbps"`
	DownMbps    int         `json:"down_mbps"`
	Obfs        *struct {
		Type     string `json:"type"`
		Password string `json:"password"`
	} `json:"obfs"`
	// Some generators write the obfuscation password here; the Qt client read it.
	ObfsPassword string `json:"obfsPassword"`
	Password     string `json:"password"`
	Realm        *struct {
		ServerURL   string      `json:"server_url"`
		Token       string      `json:"token"`
		RealmID     string      `json:"realm_id"`
		STUNServers singBoxList `json:"stun_servers"`
	} `json:"realm"`
	TLS *singBoxTLSOptions `json:"tls"`
}

func singBoxHysteria2Outbound(raw json.RawMessage) (Profile, error) {
	var o singBoxHysteria2Options
	if err := singBoxDecode(raw, &o); err != nil {
		return Profile{}, err
	}
	var realm HysteriaRealm
	if r := o.Realm; r != nil {
		realm = HysteriaRealm{
			ServerURL:   strings.TrimSpace(r.ServerURL),
			Token:       r.Token,
			RealmID:     strings.TrimSpace(r.RealmID),
			STUNServers: linkList(strings.Join(r.STUNServers, ",")),
		}
		// The realm takes the place of the address: the profile names the rendezvous service instead.
		if u, err := url.Parse(realm.ServerURL); err == nil && u.Hostname() != "" {
			o.Server = u.Hostname()
			o.ServerPort, _ = strconv.Atoi(u.Port())
			if o.ServerPort == 0 {
				o.ServerPort = 443
			}
		}
	}
	p, err := o.profile(TypeHysteria2)
	if err != nil {
		return Profile{}, err
	}
	h := &Hysteria2{
		Password:    o.Password,
		HopInterval: goDuration(o.HopInterval),
		UpMbps:      singBoxCount(o.UpMbps),
		DownMbps:    singBoxCount(o.DownMbps),
	}
	if h.ServerPorts, err = hysteriaPortRanges(strings.Join(o.ServerPorts, ",")); err != nil {
		return Profile{}, err
	}
	// The port goes unused while hopping, but the profile still names one.
	if p.ServerPort == 0 && len(h.ServerPorts) > 0 {
		first, _, _ := strings.Cut(h.ServerPorts[0], ":")
		p.ServerPort, _ = strconv.Atoi(first)
	}

	obfsType, obfsPassword := "", o.ObfsPassword
	if o.Obfs != nil {
		obfsType = strings.ToLower(strings.TrimSpace(o.Obfs.Type))
		if o.Obfs.Password != "" {
			obfsPassword = o.Obfs.Password
		}
	}
	if obfsPassword != "" {
		if obfsType != "" && obfsType != "salamander" {
			return Profile{}, unsupported("the Hysteria2 obfuscation %q is not supported", obfsType)
		}
		h.ObfsType, h.ObfsPassword = "salamander", obfsPassword
	}

	tls, err := singBoxTLS(o.TLS, true)
	if err != nil {
		return Profile{}, err
	}
	h.TLS = quicLinkTLS(tls)
	h.Realm = realm
	p.Hysteria2 = h
	return p, nil
}

type singBoxTUICOptions struct {
	singBoxServerOptions
	UUID              string             `json:"uuid"`
	Password          string             `json:"password"`
	CongestionControl string             `json:"congestion_control"`
	UDPRelayMode      string             `json:"udp_relay_mode"`
	ZeroRTTHandshake  bool               `json:"zero_rtt_handshake"`
	Heartbeat         string             `json:"heartbeat"`
	TLS               *singBoxTLSOptions `json:"tls"`
}

func singBoxTUICOutbound(raw json.RawMessage) (Profile, error) {
	var o singBoxTUICOptions
	if err := singBoxDecode(raw, &o); err != nil {
		return Profile{}, err
	}
	p, err := o.profile(TypeTUIC)
	if err != nil {
		return Profile{}, err
	}
	t := &TUIC{
		UUID:              o.UUID,
		Password:          o.Password,
		CongestionControl: tuicLinkChoice(o.CongestionControl, tuicCongestionControls),
		UDPRelayMode:      tuicLinkChoice(o.UDPRelayMode, tuicRelayModes),
		ZeroRTT:           o.ZeroRTTHandshake,
		Heartbeat:         goDuration(o.Heartbeat),
	}
	tls, err := singBoxTLS(o.TLS, true)
	if err != nil {
		return Profile{}, err
	}
	t.TLS = quicLinkTLS(tls)
	p.TUIC = t
	return p, nil
}

type singBoxSOCKSOptions struct {
	singBoxServerOptions
	// Version is text in sing-box; a number is read too.
	Version  any    `json:"version"`
	Username string `json:"username"`
	Password string `json:"password"`
}

func singBoxSOCKSOutbound(raw json.RawMessage) (Profile, error) {
	var o singBoxSOCKSOptions
	if err := singBoxDecode(raw, &o); err != nil {
		return Profile{}, err
	}
	p, err := o.profile(TypeSOCKS)
	if err != nil {
		return Profile{}, err
	}
	s := &SOCKS{Version: "5", Username: o.Username, Password: o.Password}
	var version string
	switch value := o.Version.(type) {
	case string:
		version = strings.ToLower(strings.TrimSpace(value))
	case float64:
		version = strconv.FormatFloat(value, 'f', -1, 64)
	}
	// Anything but 4 and 4a is SOCKS5, the core's default.
	if version == "4" || version == "4a" {
		s.Version = version
	}
	p.SOCKS = s
	return p, nil
}

type singBoxHTTPOptions struct {
	singBoxServerOptions
	Username string             `json:"username"`
	Password string             `json:"password"`
	Path     string             `json:"path"`
	Headers  singBoxHeaders     `json:"headers"`
	Version  int                `json:"version"`
	TLS      *singBoxTLSOptions `json:"tls"`
}

func singBoxHTTPOutbound(raw json.RawMessage) (Profile, error) {
	var o singBoxHTTPOptions
	if err := singBoxDecode(raw, &o); err != nil {
		return Profile{}, err
	}
	// 1 and 2 are left to the core, which tries HTTP/2 and falls back to 1.1.
	if o.Version == 3 {
		return Profile{}, unsupported("HTTP/3 proxies are not supported")
	}
	p, err := o.profile(TypeHTTP)
	if err != nil {
		return Profile{}, err
	}
	h := &HTTP{Username: o.Username, Password: o.Password, Path: o.Path, Headers: o.Headers.first()}
	if h.TLS, err = singBoxTLS(o.TLS, false); err != nil {
		return Profile{}, err
	}
	p.HTTP = h
	return p, nil
}

type singBoxAnyTLSOptions struct {
	singBoxServerOptions
	Password                 string             `json:"password"`
	IdleSessionCheckInterval string             `json:"idle_session_check_interval"`
	IdleSessionTimeout       string             `json:"idle_session_timeout"`
	MinIdleSession           *int               `json:"min_idle_session"`
	TLS                      *singBoxTLSOptions `json:"tls"`
}

func singBoxAnyTLSOutbound(raw json.RawMessage) (Profile, error) {
	var o singBoxAnyTLSOptions
	if err := singBoxDecode(raw, &o); err != nil {
		return Profile{}, err
	}
	p, err := o.profile(TypeAnyTLS)
	if err != nil {
		return Profile{}, err
	}
	// The Qt client's defaults, as parseAnyTLSLink sets them.
	a := &AnyTLS{Password: o.Password, IdleSessionCheckInterval: "30s", IdleSessionTimeout: "30s", MinIdleSession: 5}
	if value := goDuration(o.IdleSessionCheckInterval); value != "" {
		a.IdleSessionCheckInterval = value
	}
	if value := goDuration(o.IdleSessionTimeout); value != "" {
		a.IdleSessionTimeout = value
	}
	if o.MinIdleSession != nil && *o.MinIdleSession >= 0 {
		a.MinIdleSession = singBoxCount(*o.MinIdleSession)
	}
	if a.TLS, err = singBoxTLS(o.TLS, true); err != nil {
		return Profile{}, err
	}
	p.AnyTLS = a
	return p, nil
}

// singBoxReserved is WireGuard's reserved bytes: a list of numbers, or their base64 as Go writes a byte slice.
type singBoxReserved []int

func (r *singBoxReserved) UnmarshalJSON(data []byte) error {
	var numbers []int
	if json.Unmarshal(data, &numbers) == nil {
		*r = numbers
		return nil
	}
	var text string
	if json.Unmarshal(data, &text) == nil {
		if decoded, ok := decodeLinkBase64(text); ok {
			for _, b := range decoded {
				*r = append(*r, int(b))
			}
		}
	}
	return nil
}

type singBoxWireGuardPeer struct {
	Address             string          `json:"address"`
	Port                int             `json:"port"`
	PublicKey           string          `json:"public_key"`
	PreSharedKey        string          `json:"pre_shared_key"`
	PersistentKeepalive int             `json:"persistent_keepalive_interval"`
	Reserved            singBoxReserved `json:"reserved"`
}

// singBoxWireGuardOptions holds the endpoint of sing-box 1.11 and later and the older outbound, which named the
// peer at the top: server, local_address, peer_public_key.
type singBoxWireGuardOptions struct {
	singBoxServerOptions
	Address       singBoxList            `json:"address"`
	LocalAddress  singBoxList            `json:"local_address"`
	PrivateKey    string                 `json:"private_key"`
	PeerPublicKey string                 `json:"peer_public_key"`
	PreSharedKey  string                 `json:"pre_shared_key"`
	Reserved      singBoxReserved        `json:"reserved"`
	MTU           int                    `json:"mtu"`
	Peers         []singBoxWireGuardPeer `json:"peers"`
}

func singBoxWireGuardOutbound(raw json.RawMessage) (Profile, error) {
	var o singBoxWireGuardOptions
	if err := singBoxDecode(raw, &o); err != nil {
		return Profile{}, err
	}
	w := &WireGuard{
		PrivateKey:    strings.TrimSpace(o.PrivateKey),
		PeerPublicKey: strings.TrimSpace(o.PeerPublicKey),
		PreSharedKey:  strings.TrimSpace(o.PreSharedKey),
		Reserved:      o.Reserved,
		MTU:           singBoxCount(o.MTU),
	}
	if len(o.Peers) > 0 {
		peer := o.Peers[0]
		o.Server, o.ServerPort = peer.Address, peer.Port
		w.PeerPublicKey = strings.TrimSpace(peer.PublicKey)
		w.PreSharedKey = strings.TrimSpace(peer.PreSharedKey)
		w.PersistentKeepalive = singBoxCount(peer.PersistentKeepalive)
		w.Reserved = peer.Reserved
	}
	p, err := o.profile(TypeWireGuard)
	if err != nil {
		return Profile{}, err
	}
	if w.Address, err = wireGuardAddresses(append(o.Address, o.LocalAddress...)); err != nil {
		return Profile{}, err
	}
	p.WireGuard = w
	return p, nil
}

type singBoxHysteriaOptions struct {
	singBoxServerOptions
	ServerPorts singBoxList        `json:"server_ports"`
	HopInterval string             `json:"hop_interval"`
	Up          string             `json:"up"`
	UpMbps      int                `json:"up_mbps"`
	Down        string             `json:"down"`
	DownMbps    int                `json:"down_mbps"`
	Obfs        string             `json:"obfs"`
	Auth        string             `json:"auth"`
	AuthString  string             `json:"auth_str"`
	TLS         *singBoxTLSOptions `json:"tls"`
}

func singBoxHysteriaOutbound(raw json.RawMessage) (Profile, error) {
	var o singBoxHysteriaOptions
	if err := singBoxDecode(raw, &o); err != nil {
		return Profile{}, err
	}
	p, err := o.profile(TypeHysteria)
	if err != nil {
		return Profile{}, err
	}
	h := &Hysteria{
		Auth:        o.AuthString,
		Obfs:        o.Obfs,
		HopInterval: goDuration(o.HopInterval),
		UpMbps:      singBoxCount(o.UpMbps),
		DownMbps:    singBoxCount(o.DownMbps),
	}
	if h.Auth == "" {
		if decoded, ok := decodeLinkBase64(o.Auth); ok {
			h.Auth = string(decoded)
		}
	}
	if mbps := hysteriaLinkMbps(o.Up); mbps > 0 {
		h.UpMbps = mbps
	}
	if mbps := hysteriaLinkMbps(o.Down); mbps > 0 {
		h.DownMbps = mbps
	}
	if h.ServerPorts, err = hysteriaPortRanges(strings.Join(o.ServerPorts, ",")); err != nil {
		return Profile{}, err
	}
	if p.ServerPort == 0 && len(h.ServerPorts) > 0 {
		first, _, _ := strings.Cut(h.ServerPorts[0], ":")
		p.ServerPort, _ = strconv.Atoi(first)
	}
	tls, err := singBoxTLS(o.TLS, true)
	if err != nil {
		return Profile{}, err
	}
	h.TLS = quicLinkTLS(tls)
	p.Hysteria = h
	return p, nil
}

type singBoxNaiveOptions struct {
	singBoxServerOptions
	Username              string             `json:"username"`
	Password              string             `json:"password"`
	InsecureConcurrency   int                `json:"insecure_concurrency"`
	ExtraHeaders          singBoxHeaders     `json:"extra_headers"`
	UDPOverTCP            singBoxUDPOverTCP  `json:"udp_over_tcp"`
	QUIC                  bool               `json:"quic"`
	QUICCongestionControl string             `json:"quic_congestion_control"`
	TLS                   *singBoxTLSOptions `json:"tls"`
}

func singBoxNaiveOutbound(raw json.RawMessage) (Profile, error) {
	var o singBoxNaiveOptions
	if err := singBoxDecode(raw, &o); err != nil {
		return Profile{}, err
	}
	p, err := o.profile(TypeNaive)
	if err != nil {
		return Profile{}, err
	}
	n := &Naive{
		Username:              o.Username,
		Password:              o.Password,
		QUIC:                  o.QUIC,
		QUICCongestionControl: strings.ToLower(strings.TrimSpace(o.QUICCongestionControl)),
		InsecureConcurrency:   singBoxCount(o.InsecureConcurrency),
		ExtraHeaders:          o.ExtraHeaders.first(),
		UDPOverTCP:            bool(o.UDPOverTCP),
	}
	if n.TLS, err = singBoxTLS(o.TLS, true); err != nil {
		return Profile{}, err
	}
	p.Naive = n
	return p, nil
}

type singBoxSSHOptions struct {
	singBoxServerOptions
	User                 string      `json:"user"`
	Password             string      `json:"password"`
	PrivateKey           singBoxList `json:"private_key"`
	PrivateKeyPath       string      `json:"private_key_path"`
	PrivateKeyPassphrase string      `json:"private_key_passphrase"`
	HostKey              singBoxList `json:"host_key"`
	HostKeyAlgorithms    singBoxList `json:"host_key_algorithms"`
	ClientVersion        string      `json:"client_version"`
}

func singBoxSSHOutbound(raw json.RawMessage) (Profile, error) {
	var o singBoxSSHOptions
	if err := singBoxDecode(raw, &o); err != nil {
		return Profile{}, err
	}
	if strings.TrimSpace(o.PrivateKeyPath) != "" && len(o.PrivateKey) == 0 {
		return Profile{}, errors.New("the SSH private key is a file path, which Poros cannot read from a config")
	}
	p, err := o.profile(TypeSSH)
	if err != nil {
		return Profile{}, err
	}
	p.SSH = &SSH{
		User:                 o.User,
		Password:             o.Password,
		PrivateKey:           strings.TrimSpace(strings.Join(o.PrivateKey, "\n")),
		PrivateKeyPassphrase: o.PrivateKeyPassphrase,
		HostKey:              linkList(strings.Join(o.HostKey, ",")),
		HostKeyAlgorithms:    linkList(strings.Join(o.HostKeyAlgorithms, ",")),
		ClientVersion:        strings.TrimSpace(o.ClientVersion),
	}
	return p, nil
}

type singBoxSnellOptions struct {
	singBoxServerOptions
	Version  int    `json:"version"`
	PSK      string `json:"psk"`
	UserKey  string `json:"userkey"`
	Reuse    bool   `json:"reuse"`
	ObfsMode string `json:"obfs_mode"`
	ObfsHost string `json:"obfs_host"`
	Mode     string `json:"mode"`
}

func singBoxSnellOutbound(raw json.RawMessage) (Profile, error) {
	var o singBoxSnellOptions
	if err := singBoxDecode(raw, &o); err != nil {
		return Profile{}, err
	}
	p, err := o.profile(TypeSnell)
	if err != nil {
		return Profile{}, err
	}
	s := &Snell{Version: o.Version, PSK: o.PSK, UserKey: o.UserKey, Reuse: o.Reuse, Mode: strings.TrimSpace(o.Mode)}
	if mode := strings.ToLower(strings.TrimSpace(o.ObfsMode)); mode != "" && mode != "none" {
		s.ObfsMode, s.ObfsHost = mode, strings.TrimSpace(o.ObfsHost)
	}
	p.Snell = s
	return p, nil
}

type singBoxShadowTLSOptions struct {
	singBoxServerOptions
	Version  int                `json:"version"`
	Password string             `json:"password"`
	TLS      *singBoxTLSOptions `json:"tls"`
}

// singBoxShadowsocksOverShadowTLS reads a Shadowsocks outbound whose detour is a ShadowTLS one: the server is the
// ShadowTLS server, as in sing-box's own example.
func singBoxShadowsocksOverShadowTLS(raw, shadowTLS json.RawMessage) (Profile, error) {
	var fields map[string]json.RawMessage
	if err := json.Unmarshal(raw, &fields); err != nil {
		return Profile{}, errors.New("the sing-box outbound cannot be read")
	}
	var o singBoxShadowTLSOptions
	if err := singBoxDecode(shadowTLS, &o); err != nil {
		return Profile{}, err
	}
	delete(fields, "detour")
	fields["server"], _ = json.Marshal(o.Server)
	fields["server_port"], _ = json.Marshal(o.ServerPort)
	plain, _ := json.Marshal(fields)
	p, err := singBoxShadowsocksOutbound(plain)
	if err != nil {
		return Profile{}, err
	}
	if o.Detour != "" {
		return Profile{}, unsupported("the server is reached through the outbound %q, and chains are not supported", o.Detour)
	}
	tls, err := singBoxTLS(o.TLS, true)
	if err != nil {
		return Profile{}, err
	}
	p.Shadowsocks.ShadowTLS = ShadowTLS{Version: max(o.Version, 1), Password: o.Password, TLS: tls}
	return p, nil
}

// singBoxShadowTLSTags maps the tags of the ShadowTLS outbounds of a config to them.
func singBoxShadowTLSTags(outbounds []json.RawMessage) map[string]json.RawMessage {
	tags := map[string]json.RawMessage{}
	for _, raw := range outbounds {
		var head struct {
			Type string `json:"type"`
			Tag  string `json:"tag"`
		}
		if json.Unmarshal(raw, &head) == nil && strings.EqualFold(strings.TrimSpace(head.Type), "shadowtls") && head.Tag != "" {
			tags[strings.TrimSpace(head.Tag)] = raw
		}
	}
	return tags
}
