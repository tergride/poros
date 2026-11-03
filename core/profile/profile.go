// Package profile is the server model of Poros: one structure per protocol, with shared TLS, transport and
// multiplex parts. Share links, subscriptions and, later, the profile editors fill it; the config builder turns
// it into sing-box or Xray outbounds. It is stored as JSON, so field names and type values never change meaning.
package profile

import "encoding/json"

// Types of servers, as stored.
const (
	TypeVLESS       = "vless"
	TypeVMess       = "vmess"
	TypeTrojan      = "trojan"
	TypeShadowsocks = "shadowsocks"
	TypeHysteria2   = "hysteria2"
	TypeTUIC        = "tuic"
	TypeSOCKS       = "socks"
	TypeHTTP        = "http"
	TypeAnyTLS      = "anytls"
)

// Profile is one server. Exactly the part named by Type is set.
type Profile struct {
	Type       string `json:"type"`
	Name       string `json:"name,omitempty"`
	Server     string `json:"server"`
	ServerPort int    `json:"server_port"`

	VLESS       *VLESS       `json:"vless,omitempty"`
	VMess       *VMess       `json:"vmess,omitempty"`
	Trojan      *Trojan      `json:"trojan,omitempty"`
	Shadowsocks *Shadowsocks `json:"shadowsocks,omitempty"`
	Hysteria2   *Hysteria2   `json:"hysteria2,omitempty"`
	TUIC        *TUIC        `json:"tuic,omitempty"`
	SOCKS       *SOCKS       `json:"socks,omitempty"`
	HTTP        *HTTP        `json:"http,omitempty"`
	AnyTLS      *AnyTLS      `json:"anytls,omitempty"`
}

type VLESS struct {
	UUID string `json:"uuid"`
	// Flow is "" or "xtls-rprx-vision"; Vision needs TLS or REALITY directly over TCP.
	Flow string `json:"flow,omitempty"`
	// Encryption is Xray's VLESS encryption; "" is none. Anything else runs on Xray.
	Encryption string `json:"encryption,omitempty"`
	// PacketEncoding is "", "packetaddr" or "xudp".
	PacketEncoding string    `json:"packet_encoding,omitempty"`
	TLS            TLS       `json:"tls,omitzero"`
	Transport      Transport `json:"transport,omitzero"`
	Multiplex      Multiplex `json:"multiplex,omitzero"`
}

type VMess struct {
	UUID    string `json:"uuid"`
	AlterID int    `json:"alter_id,omitempty"`
	// Security is the VMess cipher: "auto", "aes-128-gcm", "chacha20-poly1305", "none" or "zero".
	Security       string    `json:"security,omitempty"`
	PacketEncoding string    `json:"packet_encoding,omitempty"`
	TLS            TLS       `json:"tls,omitzero"`
	Transport      Transport `json:"transport,omitzero"`
	Multiplex      Multiplex `json:"multiplex,omitzero"`
}

type Trojan struct {
	Password  string    `json:"password"`
	TLS       TLS       `json:"tls,omitzero"`
	Transport Transport `json:"transport,omitzero"`
	Multiplex Multiplex `json:"multiplex,omitzero"`
}

type Shadowsocks struct {
	Method   string `json:"method"`
	Password string `json:"password"`
	// Plugin is a SIP003 plugin such as "obfs-local" or "v2ray-plugin", with its options as one string.
	Plugin     string    `json:"plugin,omitempty"`
	PluginOpts string    `json:"plugin_opts,omitempty"`
	UDPOverTCP bool      `json:"udp_over_tcp,omitempty"`
	Multiplex  Multiplex `json:"multiplex,omitzero"`
}

type Hysteria2 struct {
	Password string `json:"password,omitempty"`
	// ServerPorts are port ranges for port hopping, "20000:30000"; ServerPort is still the first port.
	ServerPorts []string `json:"server_ports,omitempty"`
	HopInterval string   `json:"hop_interval,omitempty"`
	UpMbps      int      `json:"up_mbps,omitempty"`
	DownMbps    int      `json:"down_mbps,omitempty"`
	// ObfsType is "" or "salamander".
	ObfsType     string `json:"obfs_type,omitempty"`
	ObfsPassword string `json:"obfs_password,omitempty"`
	TLS          TLS    `json:"tls,omitzero"`
}

type TUIC struct {
	UUID     string `json:"uuid"`
	Password string `json:"password,omitempty"`
	// CongestionControl is "cubic", "new_reno" or "bbr".
	CongestionControl string `json:"congestion_control,omitempty"`
	// UDPRelayMode is "native" or "quic".
	UDPRelayMode string `json:"udp_relay_mode,omitempty"`
	ZeroRTT      bool   `json:"zero_rtt_handshake,omitempty"`
	Heartbeat    string `json:"heartbeat,omitempty"`
	TLS          TLS    `json:"tls,omitzero"`
}

type SOCKS struct {
	// Version is "5", "4" or "4a".
	Version  string `json:"version,omitempty"`
	Username string `json:"username,omitempty"`
	Password string `json:"password,omitempty"`
}

type HTTP struct {
	Username string            `json:"username,omitempty"`
	Password string            `json:"password,omitempty"`
	Path     string            `json:"path,omitempty"`
	Headers  map[string]string `json:"headers,omitempty"`
	TLS      TLS               `json:"tls,omitzero"`
}

type AnyTLS struct {
	Password                 string `json:"password"`
	IdleSessionCheckInterval string `json:"idle_session_check_interval,omitempty"`
	IdleSessionTimeout       string `json:"idle_session_timeout,omitempty"`
	MinIdleSession           int    `json:"min_idle_session,omitempty"`
	TLS                      TLS    `json:"tls,omitzero"`
}

type TLS struct {
	Enabled    bool     `json:"enabled,omitempty"`
	ServerName string   `json:"server_name,omitempty"`
	DisableSNI bool     `json:"disable_sni,omitempty"`
	Insecure   bool     `json:"insecure,omitempty"`
	ALPN       []string `json:"alpn,omitempty"`
	// MinVersion and MaxVersion are "1.0" to "1.3".
	MinVersion string `json:"min_version,omitempty"`
	MaxVersion string `json:"max_version,omitempty"`
	// CertificateSHA256 pins the server certificate, base64 SHA-256 of its DER form. A pin replaces the CA check,
	// so it holds with Insecure too.
	CertificateSHA256 []string `json:"certificate_sha256,omitempty"`
	// CertificatePublicKeySHA256 pins the server key, base64 SHA-256 of the SubjectPublicKeyInfo.
	CertificatePublicKeySHA256 []string `json:"certificate_public_key_sha256,omitempty"`
	UTLS                       UTLS     `json:"utls,omitzero"`
	Reality                    Reality  `json:"reality,omitzero"`
	ECH                        ECH      `json:"ech,omitzero"`
}

type UTLS struct {
	Enabled bool `json:"enabled,omitempty"`
	// Fingerprint is "chrome", "firefox", "edge", "safari", "360", "qq", "ios", "android", "random" or "randomized".
	Fingerprint string `json:"fingerprint,omitempty"`
}

type Reality struct {
	Enabled   bool   `json:"enabled,omitempty"`
	PublicKey string `json:"public_key,omitempty"`
	ShortID   string `json:"short_id,omitempty"`
	// SpiderX is the path Xray's REALITY client crawls; sing-box ignores it.
	SpiderX string `json:"spider_x,omitempty"`
}

type ECH struct {
	Enabled bool `json:"enabled,omitempty"`
	// Config is the ECHConfigList in PEM lines; empty asks DNS for it.
	Config          []string `json:"config,omitempty"`
	QueryServerName string   `json:"query_server_name,omitempty"`
}

// Transport types, as stored. "" is plain TCP.
const (
	TransportTCP         = ""
	TransportWebSocket   = "ws"
	TransportGRPC        = "grpc"
	TransportHTTP        = "http"
	TransportHTTPUpgrade = "httpupgrade"
	TransportXHTTP       = "xhttp"
	TransportQUIC        = "quic"
)

type Transport struct {
	Type        string            `json:"type,omitempty"`
	Host        string            `json:"host,omitempty"`
	Path        string            `json:"path,omitempty"`
	ServiceName string            `json:"service_name,omitempty"`
	Method      string            `json:"method,omitempty"`
	Headers     map[string]string `json:"headers,omitempty"`
	// MaxEarlyData and EarlyDataHeaderName are WebSocket early data, the ?ed= of a link.
	MaxEarlyData        int    `json:"max_early_data,omitempty"`
	EarlyDataHeaderName string `json:"early_data_header_name,omitempty"`
	// HeaderType "http" is Xray's HTTP header obfuscation over raw TCP.
	HeaderType string `json:"header_type,omitempty"`
	XHTTP      XHTTP  `json:"xhttp,omitzero"`
}

// XHTTP is Xray's transport; a profile with it runs on Xray.
type XHTTP struct {
	// Mode is "auto", "packet-up", "stream-up" or "stream-one".
	Mode string `json:"mode,omitempty"`
	// Extra is Xray's "extra" object as the link carried it: padding, XMUX, a separate download leg.
	Extra json.RawMessage `json:"extra,omitempty"`
}

type Multiplex struct {
	Enabled bool `json:"enabled,omitempty"`
	// Protocol is "smux", "yamux" or "h2mux".
	Protocol       string `json:"protocol,omitempty"`
	MaxConnections int    `json:"max_connections,omitempty"`
	MinStreams     int    `json:"min_streams,omitempty"`
	MaxStreams     int    `json:"max_streams,omitempty"`
	Padding        bool   `json:"padding,omitempty"`
}
