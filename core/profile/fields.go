package profile

// The editor of the advanced mode draws one form from these descriptions: each field names where it lives in the
// stored profile, how it is edited, on which tab, and what it depends on. The interface names a field by its label,
// which its languages translate.

// Kinds of fields.
const (
	FieldText   = "text"
	FieldSecret = "secret"
	FieldNumber = "number"
	FieldBool   = "bool"
	FieldSelect = "select"
	// FieldList is a list of strings, written split by commas.
	FieldList = "list"
	// FieldLines is text of several lines, such as a key in PEM.
	FieldLines = "lines"
	// FieldJSON is a JSON value kept as it is.
	FieldJSON = "json"
	// FieldHeaders is a map of HTTP headers, written one "Name: value" per line.
	FieldHeaders = "headers"
	// FieldBytes is a list of numbers from 0 to 255, such as WireGuard's reserved bytes.
	FieldBytes = "bytes"
	// FieldUTLS is the uTLS object at Path as a choice of its fingerprint: "" for none.
	FieldUTLS = "utls"
	// FieldSecurity is the TLS object at Path as one choice: "" for none, "tls", or "reality".
	FieldSecurity = "security"
	// FieldServers is a list of IDs of other servers, in order.
	FieldServers = "servers"
)

// Tabs of the editor, in their order.
const (
	TabCommon    = "common"
	TabTransport = "transport"
	TabSecurity  = "security"
	TabMux       = "mux"
	TabAdvanced  = "advanced"
)

// Field describes one setting of a profile for the editor.
type Field struct {
	// Path is where the value lives in the profile's JSON, its keys joined by dots: "vless.tls.server_name".
	Path string `json:"path"`
	// Label names the field for the interface, which translates it: "tls.serverName".
	Label string `json:"label"`
	Kind  string `json:"kind"`
	Tab   string `json:"tab"`
	// Options are the values of a select or a uTLS choice, "" among them for none or the core's default.
	Options []string `json:"options,omitempty"`
	// Numeric stores the options of a select as numbers.
	Numeric bool `json:"numeric,omitempty"`
	// When are the conditions the field means something under, all of them; otherwise it is inactive and its value
	// is not used. A field that a choice of another field rules out is not shown at all.
	When []FieldCondition `json:"when,omitempty"`
}

// FieldCondition makes a field depend on another one: the value at Path is one of In, or, with no In, set at all;
// Not turns that around. Values compare as text, and a missing value is "".
type FieldCondition struct {
	Path string   `json:"path"`
	In   []string `json:"in,omitempty"`
	Not  bool     `json:"not,omitempty"`
}

// ProfileForm is the form of one type of profile.
type ProfileForm struct {
	Type   string  `json:"type"`
	Fields []Field `json:"fields"`
}

// NewProfile is a choice of «New profile»: the profile it starts from.
type NewProfile struct {
	// ID names the choice: the type, or "shadowtls" for Shadowsocks over ShadowTLS.
	ID string `json:"id"`
	// Kind is "proxy", "quic", "vpn" or "compound".
	Kind    string  `json:"kind"`
	Profile Profile `json:"profile"`
}

func field(path, label, kind, tab string) Field {
	return Field{Path: path, Label: label, Kind: kind, Tab: tab}
}

func (f Field) options(values ...string) Field {
	f.Options = values
	return f
}

func (f Field) numeric(values ...string) Field {
	f.Options, f.Numeric = values, true
	return f
}

func (f Field) when(path string, in ...string) Field {
	f.When = append(f.When, FieldCondition{Path: path, In: in})
	return f
}

func (f Field) unless(path string, in ...string) Field {
	f.When = append(f.When, FieldCondition{Path: path, In: in, Not: true})
	return f
}

func join(parts ...[]Field) []Field {
	var all []Field
	for _, part := range parts {
		all = append(all, part...)
	}
	return all
}

func addressFields() []Field {
	return []Field{
		field("name", "name", FieldText, TabCommon),
		field("server", "server", FieldText, TabCommon),
		field("server_port", "port", FieldNumber, TabCommon),
	}
}

var fingerprints = []string{"", "chrome", "firefox", "edge", "safari", "360", "qq", "ios", "android", "random", "randomized"}

// How a protocol runs TLS.
type tlsMode int

const (
	// tlsOptional may run without TLS: one choice turns it on, and REALITY where the protocol takes it.
	tlsOptional tlsMode = iota
	// tlsAlways always runs TLS over TCP.
	tlsAlways
	// tlsQUIC runs TLS inside QUIC, without uTLS.
	tlsQUIC
)

// tlsFields are the settings of the TLS object at prefix. With REALITY on, what it replaces is inactive: the
// certificate check, ECH, and leaving out the server name.
func tlsFields(prefix string, mode tlsMode, reality bool) []Field {
	on := func(f Field) Field {
		if mode == tlsOptional {
			return f.when(prefix + ".enabled")
		}
		return f
	}
	plain := func(f Field) Field {
		if reality {
			return f.unless(prefix + ".reality.enabled")
		}
		return f
	}
	var fields []Field
	if mode == tlsOptional {
		choices := []string{"", "tls"}
		if reality {
			choices = append(choices, "reality")
		}
		fields = append(fields, field(prefix, "tls.security", FieldSecurity, TabSecurity).options(choices...))
	}
	fields = append(fields, on(field(prefix+".server_name", "tls.serverName", FieldText, TabSecurity)))
	if mode != tlsQUIC {
		fields = append(fields, on(field(prefix+".utls", "tls.fingerprint", FieldUTLS, TabSecurity).options(fingerprints...)))
	}
	if reality {
		for _, f := range []Field{
			field(prefix+".reality.public_key", "tls.realityPublicKey", FieldText, TabSecurity),
			field(prefix+".reality.short_id", "tls.realityShortId", FieldText, TabSecurity),
			field(prefix+".reality.spider_x", "tls.realitySpiderX", FieldText, TabSecurity),
		} {
			fields = append(fields, f.when(prefix+".reality.enabled"))
		}
	}
	return append(fields,
		on(field(prefix+".alpn", "tls.alpn", FieldList, TabSecurity)),
		plain(on(field(prefix+".insecure", "tls.insecure", FieldBool, TabSecurity))),
		plain(on(field(prefix+".certificate_sha256", "tls.certificatePin", FieldList, TabSecurity))),
		plain(on(field(prefix+".certificate_public_key_sha256", "tls.publicKeyPin", FieldList, TabSecurity))),
		plain(on(field(prefix+".ech.enabled", "tls.ech", FieldBool, TabSecurity))),
		plain(on(field(prefix+".ech.config", "tls.echConfig", FieldList, TabSecurity).when(prefix+".ech.enabled"))),
		plain(on(field(prefix+".ech.query_server_name", "tls.echServerName", FieldText, TabSecurity).when(prefix+".ech.enabled"))),
		plain(on(field(prefix+".disable_sni", "tls.disableSni", FieldBool, TabAdvanced))),
		on(field(prefix+".min_version", "tls.minVersion", FieldSelect, TabAdvanced).options("", "1.0", "1.1", "1.2", "1.3")),
		on(field(prefix+".max_version", "tls.maxVersion", FieldSelect, TabAdvanced).options("", "1.0", "1.1", "1.2", "1.3")),
	)
}

// transportFields are the V2Ray transports at prefix; xhttp adds Xray's, which only VLESS runs.
func transportFields(prefix string, xhttp bool) []Field {
	kind := prefix + ".type"
	types := []string{TransportTCP, TransportWebSocket, TransportGRPC, TransportHTTP, TransportHTTPUpgrade, TransportQUIC}
	withHost := []string{TransportWebSocket, TransportHTTP, TransportHTTPUpgrade}
	if xhttp {
		types = append(types, TransportXHTTP)
		withHost = append(withHost, TransportXHTTP)
	}
	return []Field{
		field(kind, "transport.type", FieldSelect, TabTransport).options(types...),
		field(prefix+".header_type", "transport.headerType", FieldSelect, TabTransport).options("", "http").when(kind, TransportTCP),
		// The host and the path of raw TCP are those of its HTTP header.
		field(prefix+".host", "transport.host", FieldText, TabTransport).when(kind, TransportTCP).when(prefix+".header_type", "http"),
		field(prefix+".path", "transport.path", FieldText, TabTransport).when(kind, TransportTCP).when(prefix+".header_type", "http"),
		field(prefix+".host", "transport.host", FieldText, TabTransport).when(kind, withHost...),
		field(prefix+".path", "transport.path", FieldText, TabTransport).when(kind, withHost...),
		field(prefix+".service_name", "transport.serviceName", FieldText, TabTransport).when(kind, TransportGRPC),
		field(prefix+".method", "transport.method", FieldText, TabTransport).when(kind, TransportHTTP),
		field(prefix+".headers", "transport.headers", FieldHeaders, TabTransport).when(kind, TransportWebSocket, TransportHTTP, TransportHTTPUpgrade),
		field(prefix+".max_early_data", "transport.earlyData", FieldNumber, TabTransport).when(kind, TransportWebSocket),
		field(prefix+".early_data_header_name", "transport.earlyDataHeader", FieldText, TabTransport).when(kind, TransportWebSocket),
		field(prefix+".xhttp.mode", "transport.xhttpMode", FieldSelect, TabTransport).options("", "auto", "packet-up", "stream-up", "stream-one").when(kind, TransportXHTTP),
		field(prefix+".xhttp.extra", "transport.xhttpExtra", FieldJSON, TabTransport).when(kind, TransportXHTTP),
	}
}

func muxFields(prefix string) []Field {
	fields := []Field{field(prefix+".enabled", "mux.enabled", FieldBool, TabMux)}
	for _, f := range []Field{
		field(prefix+".protocol", "mux.protocol", FieldSelect, TabMux).options("", "smux", "yamux", "h2mux"),
		field(prefix+".max_connections", "mux.maxConnections", FieldNumber, TabMux),
		field(prefix+".min_streams", "mux.minStreams", FieldNumber, TabMux),
		field(prefix+".max_streams", "mux.maxStreams", FieldNumber, TabMux),
		field(prefix+".padding", "mux.padding", FieldBool, TabMux),
	} {
		fields = append(fields, f.when(prefix+".enabled"))
	}
	return fields
}

// hysteriaFields are what both versions of Hysteria share: port hopping and the speeds. Port hopping and a realm
// rule each other out.
func hysteriaFields(prefix string, realm bool) []Field {
	ports := field(prefix+".server_ports", "hysteria.ports", FieldList, TabCommon)
	if realm {
		ports = ports.unless(prefix + ".realm.server_url")
	}
	return []Field{
		ports,
		field(prefix+".hop_interval", "hysteria.hopInterval", FieldText, TabCommon).when(prefix + ".server_ports"),
		field(prefix+".up_mbps", "hysteria.up", FieldNumber, TabCommon),
		field(prefix+".down_mbps", "hysteria.down", FieldNumber, TabCommon),
	}
}

// pemFields are the authority of the server's certificate and the client's own certificate and key.
func pemFields(prefix string) []Field {
	return []Field{
		field(prefix+".ca", "pem.ca", FieldLines, TabSecurity),
		field(prefix+".cert", "pem.cert", FieldLines, TabSecurity),
		field(prefix+".key", "pem.key", FieldLines, TabSecurity),
	}
}

// Forms gives the form of every type of profile: those «New profile» makes, and Hysteria v1, which comes only from
// links and subscriptions.
func Forms() []ProfileForm {
	shadowTLS := "shadowsocks.shadowtls.version"
	return []ProfileForm{
		{Type: TypeVLESS, Fields: join(addressFields(), []Field{
			field("vless.uuid", "uuid", FieldText, TabCommon),
			field("vless.flow", "vless.flow", FieldSelect, TabCommon).options("", "xtls-rprx-vision").when("vless.transport.type", TransportTCP),
			field("vless.packet_encoding", "packetEncoding", FieldSelect, TabAdvanced).options("", "packetaddr", "xudp"),
			field("vless.encryption", "vless.encryption", FieldText, TabAdvanced),
		}, transportFields("vless.transport", true), tlsFields("vless.tls", tlsOptional, true), muxFields("vless.multiplex"))},
		{Type: TypeVMess, Fields: join(addressFields(), []Field{
			field("vmess.uuid", "uuid", FieldText, TabCommon),
			field("vmess.security", "vmess.security", FieldSelect, TabCommon).options("auto", "aes-128-gcm", "chacha20-poly1305", "none", "zero"),
			field("vmess.alter_id", "vmess.alterId", FieldNumber, TabAdvanced),
			field("vmess.packet_encoding", "packetEncoding", FieldSelect, TabAdvanced).options("", "packetaddr", "xudp"),
		}, transportFields("vmess.transport", false), tlsFields("vmess.tls", tlsOptional, false), muxFields("vmess.multiplex"))},
		{Type: TypeTrojan, Fields: join(addressFields(), []Field{
			field("trojan.password", "password", FieldSecret, TabCommon),
		}, transportFields("trojan.transport", false), tlsFields("trojan.tls", tlsOptional, true), muxFields("trojan.multiplex"))},
		{Type: TypeShadowsocks, Fields: join(addressFields(), []Field{
			field("shadowsocks.method", "shadowsocks.method", FieldSelect, TabCommon).options(shadowsocksMethods...),
			field("shadowsocks.password", "password", FieldSecret, TabCommon).unless("shadowsocks.method", "none"),
			field("shadowsocks.udp_over_tcp", "udpOverTcp", FieldBool, TabCommon),
			// A plugin and ShadowTLS both carry the connection: one of them at a time.
			field("shadowsocks.plugin", "shadowsocks.plugin", FieldSelect, TabTransport).options(shadowsocksPlugins...).unless(shadowTLS),
			field("shadowsocks.plugin_opts", "shadowsocks.pluginOpts", FieldText, TabTransport).when("shadowsocks.plugin").unless(shadowTLS),
			field(shadowTLS, "shadowtls.version", FieldSelect, TabSecurity).numeric("0", "1", "2", "3").unless("shadowsocks.plugin"),
			field("shadowsocks.shadowtls.password", "shadowtls.password", FieldSecret, TabSecurity).when(shadowTLS, "2", "3"),
			field("shadowsocks.shadowtls.tls.server_name", "shadowtls.site", FieldText, TabSecurity).when(shadowTLS),
			field("shadowsocks.shadowtls.tls.utls", "tls.fingerprint", FieldUTLS, TabSecurity).options(fingerprints...).when(shadowTLS),
			field("shadowsocks.shadowtls.tls.alpn", "tls.alpn", FieldList, TabSecurity).when(shadowTLS),
			field("shadowsocks.shadowtls.tls.insecure", "tls.insecure", FieldBool, TabSecurity).when(shadowTLS),
		}, muxFields("shadowsocks.multiplex"))},
		{Type: TypeAnyTLS, Fields: join(addressFields(), []Field{
			field("anytls.password", "password", FieldSecret, TabCommon),
			field("anytls.idle_session_check_interval", "anytls.checkInterval", FieldText, TabAdvanced),
			field("anytls.idle_session_timeout", "anytls.idleTimeout", FieldText, TabAdvanced),
			field("anytls.min_idle_session", "anytls.minIdle", FieldNumber, TabAdvanced),
		}, tlsFields("anytls.tls", tlsAlways, false))},
		{Type: TypeSOCKS, Fields: join(addressFields(), []Field{
			field("socks.version", "socks.version", FieldSelect, TabCommon).options("5", "4", "4a"),
			field("socks.username", "username", FieldText, TabCommon),
			field("socks.password", "password", FieldSecret, TabCommon).unless("socks.version", "4", "4a"),
		})},
		{Type: TypeHTTP, Fields: join(addressFields(), []Field{
			field("http.username", "username", FieldText, TabCommon),
			field("http.password", "password", FieldSecret, TabCommon),
			field("http.path", "transport.path", FieldText, TabAdvanced),
			field("http.headers", "transport.headers", FieldHeaders, TabAdvanced),
		}, tlsFields("http.tls", tlsOptional, false))},
		{Type: TypeNaive, Fields: join(addressFields(), []Field{
			field("naive.username", "username", FieldText, TabCommon),
			field("naive.password", "password", FieldSecret, TabCommon),
			field("naive.quic", "naive.quic", FieldBool, TabCommon),
			field("naive.quic_congestion_control", "congestion", FieldSelect, TabAdvanced).options(naiveCongestionControls...).when("naive.quic"),
			field("naive.insecure_concurrency", "naive.concurrency", FieldNumber, TabAdvanced),
			field("naive.extra_headers", "transport.headers", FieldHeaders, TabAdvanced),
			field("naive.udp_over_tcp", "udpOverTcp", FieldBool, TabAdvanced),
			// Chromium checks the certificate as a browser does: of TLS, only the name and ECH are its own.
			field("naive.tls.server_name", "tls.serverName", FieldText, TabSecurity),
			field("naive.tls.ech.enabled", "tls.ech", FieldBool, TabSecurity),
			field("naive.tls.ech.config", "tls.echConfig", FieldList, TabSecurity).when("naive.tls.ech.enabled"),
			field("naive.tls.ech.query_server_name", "tls.echServerName", FieldText, TabSecurity).when("naive.tls.ech.enabled"),
		})},
		{Type: TypeSSH, Fields: join(addressFields(), []Field{
			field("ssh.user", "username", FieldText, TabCommon),
			field("ssh.password", "password", FieldSecret, TabCommon),
			field("ssh.private_key", "ssh.privateKey", FieldLines, TabSecurity),
			field("ssh.private_key_passphrase", "ssh.passphrase", FieldSecret, TabSecurity).when("ssh.private_key"),
			field("ssh.host_key", "ssh.hostKey", FieldList, TabSecurity),
			field("ssh.host_key_algorithms", "ssh.hostKeyAlgorithms", FieldList, TabAdvanced),
			field("ssh.client_version", "ssh.clientVersion", FieldText, TabAdvanced),
		})},
		{Type: TypeSnell, Fields: join(addressFields(), []Field{
			field("snell.version", "snell.version", FieldSelect, TabCommon).numeric("4", "6"),
			field("snell.psk", "snell.psk", FieldSecret, TabCommon),
			field("snell.userkey", "snell.userKey", FieldSecret, TabAdvanced),
			field("snell.reuse", "snell.reuse", FieldBool, TabAdvanced),
			field("snell.obfs_mode", "snell.obfs", FieldSelect, TabTransport).options("", "http", "tls").when("snell.version", "4"),
			field("snell.obfs_host", "snell.obfsHost", FieldText, TabTransport).when("snell.version", "4").when("snell.obfs_mode", "http", "tls"),
			field("snell.mode", "snell.mode", FieldSelect, TabTransport).options("", "default", "unshaped", "unsafe-raw").when("snell.version", "6"),
		})},
		{Type: TypeHysteria2, Fields: join(addressFields(), []Field{
			field("hysteria2.password", "password", FieldSecret, TabCommon),
		}, hysteriaFields("hysteria2", true), []Field{
			field("hysteria2.obfs_password", "hysteria.obfs", FieldSecret, TabSecurity),
			field("hysteria2.realm.server_url", "realm.server", FieldText, TabAdvanced).unless("hysteria2.server_ports"),
			field("hysteria2.realm.realm_id", "realm.id", FieldText, TabAdvanced).when("hysteria2.realm.server_url"),
			field("hysteria2.realm.token", "realm.token", FieldSecret, TabAdvanced).when("hysteria2.realm.server_url"),
			field("hysteria2.realm.stun_servers", "realm.stun", FieldList, TabAdvanced).when("hysteria2.realm.server_url"),
		}, tlsFields("hysteria2.tls", tlsQUIC, false))},
		{Type: TypeHysteria, Fields: join(addressFields(), []Field{
			field("hysteria.auth", "password", FieldSecret, TabCommon),
		}, hysteriaFields("hysteria", false), []Field{
			field("hysteria.obfs", "hysteria.obfs", FieldSecret, TabSecurity),
		}, tlsFields("hysteria.tls", tlsQUIC, false))},
		{Type: TypeTUIC, Fields: join(addressFields(), []Field{
			field("tuic.uuid", "uuid", FieldText, TabCommon),
			field("tuic.password", "password", FieldSecret, TabCommon),
			field("tuic.congestion_control", "congestion", FieldSelect, TabCommon).options("", "cubic", "new_reno", "bbr"),
			field("tuic.udp_relay_mode", "tuic.relay", FieldSelect, TabAdvanced).options("", "native", "quic"),
			field("tuic.zero_rtt_handshake", "tuic.zeroRtt", FieldBool, TabAdvanced),
			field("tuic.heartbeat", "tuic.heartbeat", FieldText, TabAdvanced),
		}, tlsFields("tuic.tls", tlsQUIC, false))},
		{Type: TypeMASQUE, Fields: join(addressFields(), []Field{
			field("masque.username", "username", FieldText, TabCommon),
			field("masque.password", "password", FieldSecret, TabCommon),
			field("masque.path", "transport.path", FieldText, TabCommon),
			field("masque.version", "masque.version", FieldSelect, TabCommon).numeric("0", "3", "2", "1"),
			field("masque.headers", "transport.headers", FieldHeaders, TabAdvanced),
			field("masque.mtu", "mtu", FieldNumber, TabAdvanced),
		}, tlsFields("masque.tls", tlsQUIC, false))},
		{Type: TypeWireGuard, Fields: join(addressFields(), []Field{
			field("wireguard.private_key", "wireguard.privateKey", FieldSecret, TabCommon),
			field("wireguard.peer_public_key", "wireguard.peerPublicKey", FieldText, TabCommon),
			field("wireguard.address", "wireguard.address", FieldList, TabCommon),
			field("wireguard.pre_shared_key", "wireguard.preSharedKey", FieldSecret, TabSecurity),
			field("wireguard.reserved", "wireguard.reserved", FieldBytes, TabAdvanced),
			field("wireguard.mtu", "mtu", FieldNumber, TabAdvanced),
			field("wireguard.persistent_keepalive", "wireguard.keepalive", FieldNumber, TabAdvanced),
		})},
		{Type: TypeOpenVPN, Fields: join(addressFields(), []Field{
			field("openvpn.network", "openvpn.network", FieldSelect, TabCommon).options(openVPNNetworks...),
			field("openvpn.username", "username", FieldText, TabCommon),
			field("openvpn.password", "password", FieldSecret, TabCommon),
		}, pemFields("openvpn"), []Field{
			field("openvpn.peer_fingerprint", "openvpn.fingerprint", FieldList, TabSecurity),
			field("openvpn.control_wrap.type", "openvpn.wrap", FieldSelect, TabSecurity).options("", "tls_auth", "tls_crypt", "tls_crypt_v2"),
			field("openvpn.control_wrap.key", "openvpn.wrapKey", FieldLines, TabSecurity).when("openvpn.control_wrap.type"),
			field("openvpn.control_wrap.direction", "openvpn.wrapDirection", FieldSelect, TabSecurity).options("", "server", "client").when("openvpn.control_wrap.type", "tls_auth"),
			field("openvpn.verify_name", "openvpn.verifyName", FieldText, TabSecurity),
			field("openvpn.verify_name_type", "openvpn.verifyNameType", FieldSelect, TabSecurity).options("", "subject", "name", "name-prefix").when("openvpn.verify_name"),
			field("openvpn.remote_cert_tls", "openvpn.remoteCertTls", FieldSelect, TabSecurity).options("", "server", "client", "none"),
			field("openvpn.cipher", "openvpn.cipher", FieldText, TabAdvanced),
			field("openvpn.data_ciphers", "openvpn.dataCiphers", FieldList, TabAdvanced),
			field("openvpn.auth", "openvpn.auth", FieldText, TabAdvanced),
			field("openvpn.compression", "openvpn.compression", FieldSelect, TabAdvanced).options("", "none", "no", "lz4", "lz4-v2", "stub", "stub-v2", "disabled", "off"),
			field("openvpn.compression_lzo", "openvpn.lzo", FieldSelect, TabAdvanced).options("", "none", "no", "yes", "adaptive", "asym", "disabled", "off"),
			field("openvpn.mtu", "mtu", FieldNumber, TabAdvanced),
			field("openvpn.mss_fix", "openvpn.mssFix", FieldNumber, TabAdvanced),
			field("openvpn.fragment", "openvpn.fragment", FieldNumber, TabAdvanced),
			field("openvpn.ping_interval", "openvpn.ping", FieldText, TabAdvanced),
			field("openvpn.ping_restart", "openvpn.pingRestart", FieldText, TabAdvanced),
			field("openvpn.renegotiate_interval", "openvpn.renegotiate", FieldText, TabAdvanced),
			field("openvpn.static_challenge", "openvpn.challenge", FieldText, TabAdvanced),
		})},
		{Type: TypeOpenConnect, Fields: join(addressFields(), []Field{
			field("openconnect.flavor", "openconnect.flavor", FieldSelect, TabCommon).options(openConnectFlavors...),
			field("openconnect.path", "transport.path", FieldText, TabCommon),
			field("openconnect.username", "username", FieldText, TabCommon),
			field("openconnect.password", "password", FieldSecret, TabCommon),
			field("openconnect.auth_group", "openconnect.group", FieldText, TabCommon),
			field("openconnect.peer_fingerprint", "openconnect.fingerprint", FieldList, TabSecurity),
			field("openconnect.insecure", "tls.insecure", FieldBool, TabSecurity).unless("openconnect.peer_fingerprint"),
		}, pemFields("openconnect"), []Field{
			field("openconnect.key_password", "openconnect.keyPassword", FieldSecret, TabSecurity).when("openconnect.key"),
			field("openconnect.no_udp", "openconnect.noUdp", FieldBool, TabAdvanced),
			field("openconnect.mtu", "mtu", FieldNumber, TabAdvanced),
		})},
		{Type: TypeChain, Fields: []Field{
			field("name", "name", FieldText, TabCommon),
			field("chain.servers", "chain.servers", FieldServers, TabCommon),
		}},
		{Type: TypeCustom, Fields: []Field{
			field("name", "name", FieldText, TabCommon),
			field("custom.json", "custom.json", FieldJSON, TabCommon),
		}},
		{Type: TypeDirect, Fields: []Field{field("name", "name", FieldText, TabCommon)}},
	}
}

// NewProfiles gives the choices of «New profile»: only what the main sing-box runs, each with the settings a new
// server of it starts with.
func NewProfiles() []NewProfile {
	tls := TLS{Enabled: true}
	at := func(t string, port int) Profile { return Profile{Type: t, ServerPort: port} }
	vless := at(TypeVLESS, 443)
	vless.VLESS = &VLESS{TLS: tls}
	vmess := at(TypeVMess, 443)
	vmess.VMess = &VMess{Security: "auto"}
	trojan := at(TypeTrojan, 443)
	trojan.Trojan = &Trojan{TLS: tls}
	shadowsocks := at(TypeShadowsocks, 8388)
	shadowsocks.Shadowsocks = &Shadowsocks{Method: "2022-blake3-aes-128-gcm"}
	shadowTLS := at(TypeShadowsocks, 443)
	shadowTLS.Shadowsocks = &Shadowsocks{Method: "2022-blake3-aes-128-gcm", ShadowTLS: ShadowTLS{Version: 3, TLS: tls}}
	anyTLS := at(TypeAnyTLS, 443)
	anyTLS.AnyTLS = &AnyTLS{TLS: tls}
	socks := at(TypeSOCKS, 1080)
	socks.SOCKS = &SOCKS{Version: "5"}
	http := at(TypeHTTP, 8080)
	http.HTTP = &HTTP{}
	naive := at(TypeNaive, 443)
	naive.Naive = &Naive{TLS: tls}
	ssh := at(TypeSSH, 22)
	ssh.SSH = &SSH{User: "root"}
	snell := at(TypeSnell, 443)
	snell.Snell = &Snell{Version: 4}
	hysteria2 := at(TypeHysteria2, 443)
	hysteria2.Hysteria2 = &Hysteria2{TLS: tls}
	tuic := at(TypeTUIC, 443)
	tuic.TUIC = &TUIC{TLS: tls}
	masque := at(TypeMASQUE, 443)
	masque.MASQUE = &MASQUE{TLS: tls}
	wireGuard := at(TypeWireGuard, 51820)
	wireGuard.WireGuard = &WireGuard{Address: []string{}}
	openVPN := at(TypeOpenVPN, 1194)
	openVPN.OpenVPN = &OpenVPN{}
	openConnect := at(TypeOpenConnect, 443)
	openConnect.OpenConnect = &OpenConnect{}
	return []NewProfile{
		{TypeVLESS, "proxy", vless}, {TypeVMess, "proxy", vmess}, {TypeTrojan, "proxy", trojan},
		{TypeShadowsocks, "proxy", shadowsocks}, {"shadowtls", "proxy", shadowTLS}, {TypeAnyTLS, "proxy", anyTLS},
		{TypeSOCKS, "proxy", socks}, {TypeHTTP, "proxy", http}, {TypeNaive, "proxy", naive}, {TypeSSH, "proxy", ssh},
		{TypeSnell, "proxy", snell},
		{TypeHysteria2, "quic", hysteria2}, {TypeTUIC, "quic", tuic}, {TypeMASQUE, "quic", masque},
		{TypeWireGuard, "vpn", wireGuard}, {TypeOpenVPN, "vpn", openVPN}, {TypeOpenConnect, "vpn", openConnect},
		{TypeChain, "compound", Profile{Type: TypeChain, Chain: &Chain{Servers: []string{}}}},
		{TypeCustom, "compound", Profile{Type: TypeCustom, Custom: &Custom{JSON: []byte(`{"type":"","server":"","server_port":443}`)}}},
		{TypeDirect, "compound", Profile{Type: TypeDirect, Direct: &Direct{}}},
	}
}
