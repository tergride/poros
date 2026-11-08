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
	TypeWireGuard   = "wireguard"
	TypeHysteria    = "hysteria"
	TypeNaive       = "naive"
	TypeSSH         = "ssh"
	TypeSnell       = "snell"
	TypeOpenVPN     = "openvpn"
	TypeOpenConnect = "openconnect"
	TypeMASQUE      = "masque"
	TypeChain       = "chain"
	TypeCustom      = "custom"
	TypeDirect      = "direct"
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
	WireGuard   *WireGuard   `json:"wireguard,omitempty"`
	Hysteria    *Hysteria    `json:"hysteria,omitempty"`
	Naive       *Naive       `json:"naive,omitempty"`
	SSH         *SSH         `json:"ssh,omitempty"`
	Snell       *Snell       `json:"snell,omitempty"`
	OpenVPN     *OpenVPN     `json:"openvpn,omitempty"`
	OpenConnect *OpenConnect `json:"openconnect,omitempty"`
	MASQUE      *MASQUE      `json:"masque,omitempty"`
	Chain       *Chain       `json:"chain,omitempty"`
	Custom      *Custom      `json:"custom,omitempty"`
	Direct      *Direct      `json:"direct,omitempty"`
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
	// ShadowTLS carries the Shadowsocks connection: the profile's server is the ShadowTLS server then.
	ShadowTLS ShadowTLS `json:"shadowtls,omitzero"`
}

// ShadowTLS hides a connection behind a TLS handshake with a real site, as in Clash and sing-box.
type ShadowTLS struct {
	// Version is 1, 2 or 3; 0 is no ShadowTLS.
	Version  int    `json:"version,omitempty"`
	Password string `json:"password,omitempty"`
	// TLS names the site of the handshake; it is always on.
	TLS TLS `json:"tls,omitzero"`
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
	// Realm finds the server through a rendezvous service instead of its address; the profile's server and port are
	// those of the service then.
	Realm HysteriaRealm `json:"realm,omitzero"`
}

type HysteriaRealm struct {
	// ServerURL is the rendezvous service, https://realm.example.com.
	ServerURL   string   `json:"server_url,omitempty"`
	Token       string   `json:"token,omitempty"`
	RealmID     string   `json:"realm_id,omitempty"`
	STUNServers []string `json:"stun_servers,omitempty"`
}

// Hysteria is Hysteria v1.
type Hysteria struct {
	// Auth is the password, auth_str.
	Auth        string   `json:"auth,omitempty"`
	ServerPorts []string `json:"server_ports,omitempty"`
	HopInterval string   `json:"hop_interval,omitempty"`
	// UpMbps and DownMbps are required: the protocol paces itself by them.
	UpMbps   int    `json:"up_mbps,omitempty"`
	DownMbps int    `json:"down_mbps,omitempty"`
	Obfs     string `json:"obfs,omitempty"`
	TLS      TLS    `json:"tls,omitzero"`
}

// WireGuard is a WireGuard peer: the profile's server is the peer's endpoint.
type WireGuard struct {
	PrivateKey    string `json:"private_key"`
	PeerPublicKey string `json:"peer_public_key"`
	PreSharedKey  string `json:"pre_shared_key,omitempty"`
	// Address are the addresses of this end of the tunnel with their prefixes, 172.16.0.2/32 and fd01::2/128.
	Address []string `json:"address"`
	// Reserved are the three bytes Cloudflare WARP expects in each packet.
	Reserved            []int `json:"reserved,omitempty"`
	MTU                 int   `json:"mtu,omitempty"`
	PersistentKeepalive int   `json:"persistent_keepalive,omitempty"`
}

// Naive is NaiveProxy, Chromium's network stack over HTTP/2 or HTTP/3; it always runs over TLS.
type Naive struct {
	Username string `json:"username,omitempty"`
	Password string `json:"password,omitempty"`
	// QUIC runs over HTTP/3.
	QUIC bool `json:"quic,omitempty"`
	// QUICCongestionControl is "", "bbr", "bbr2", "cubic" or "reno".
	QUICCongestionControl string            `json:"quic_congestion_control,omitempty"`
	InsecureConcurrency   int               `json:"insecure_concurrency,omitempty"`
	ExtraHeaders          map[string]string `json:"extra_headers,omitempty"`
	UDPOverTCP            bool              `json:"udp_over_tcp,omitempty"`
	TLS                   TLS               `json:"tls,omitzero"`
}

type SSH struct {
	User     string `json:"user,omitempty"`
	Password string `json:"password,omitempty"`
	// PrivateKey is the key in PEM, from -----BEGIN to PRIVATE KEY-----.
	PrivateKey           string `json:"private_key,omitempty"`
	PrivateKeyPassphrase string `json:"private_key_passphrase,omitempty"`
	// HostKey are the server keys accepted, "ssh-ed25519 AAAA..."; none accepts any.
	HostKey           []string `json:"host_key,omitempty"`
	HostKeyAlgorithms []string `json:"host_key_algorithms,omitempty"`
	ClientVersion     string   `json:"client_version,omitempty"`
}

type Snell struct {
	// Version is 4 or 6, the versions sing-box runs.
	Version int    `json:"version"`
	PSK     string `json:"psk"`
	UserKey string `json:"userkey,omitempty"`
	Reuse   bool   `json:"reuse,omitempty"`
	// ObfsMode is "", "http" or "tls", and ObfsHost its host: version 4 only.
	ObfsMode string `json:"obfs_mode,omitempty"`
	ObfsHost string `json:"obfs_host,omitempty"`
	// Mode is "", "default", "unshaped" or "unsafe-raw": version 6 only.
	Mode string `json:"mode,omitempty"`
}

// OpenVPN is an OpenVPN client as a .ovpn file describes it; the profile's server is its first remote.
type OpenVPN struct {
	// Network is "udp" or "tcp", with 4 or 6 for one address family; "" is udp.
	Network string `json:"network,omitempty"`
	// Remotes are further servers, tried in turn after the first.
	Remotes             []OpenVPNRemote `json:"remotes,omitempty"`
	RemoteRandom        bool            `json:"remote_random,omitempty"`
	Username            string          `json:"username,omitempty"`
	Password            string          `json:"password,omitempty"`
	StaticChallenge     string          `json:"static_challenge,omitempty"`
	StaticChallengeEcho bool            `json:"static_challenge_echo,omitempty"`
	// CA, Cert and Key are PEM blocks: the authority of the server's certificate, and the client's own pair.
	CA              string             `json:"ca,omitempty"`
	Cert            string             `json:"cert,omitempty"`
	Key             string             `json:"key,omitempty"`
	PeerFingerprint []string           `json:"peer_fingerprint,omitempty"`
	ControlWrap     OpenVPNControlWrap `json:"control_wrap,omitzero"`
	// VerifyName is verify-x509-name; VerifyNameType is "", "subject", "name" or "name-prefix".
	VerifyName          string   `json:"verify_name,omitempty"`
	VerifyNameType      string   `json:"verify_name_type,omitempty"`
	RemoteCertTLS       string   `json:"remote_cert_tls,omitempty"`
	TLSVersionMin       string   `json:"tls_version_min,omitempty"`
	TLSVersionMax       string   `json:"tls_version_max,omitempty"`
	Cipher              string   `json:"cipher,omitempty"`
	DataCiphers         []string `json:"data_ciphers,omitempty"`
	DataCiphersFallback string   `json:"data_ciphers_fallback,omitempty"`
	Auth                string   `json:"auth,omitempty"`
	// Compression is compress, CompressionLZO comp-lzo, AllowCompression allow-compression.
	Compression         string `json:"compression,omitempty"`
	CompressionLZO      string `json:"compression_lzo,omitempty"`
	AllowCompression    string `json:"allow_compression,omitempty"`
	MTU                 int    `json:"mtu,omitempty"`
	MSSFix              int    `json:"mss_fix,omitempty"`
	Fragment            int    `json:"fragment,omitempty"`
	PingInterval        string `json:"ping_interval,omitempty"`
	PingRestart         string `json:"ping_restart,omitempty"`
	RenegotiateInterval string `json:"renegotiate_interval,omitempty"`
}

type OpenVPNRemote struct {
	Server string `json:"server"`
	Port   int    `json:"port"`
	// Network is the remote's own, "" for the profile's.
	Network string `json:"network,omitempty"`
}

// OpenVPNControlWrap protects the control channel: tls-auth, tls-crypt or tls-crypt-v2.
type OpenVPNControlWrap struct {
	// Type is "tls_auth", "tls_crypt" or "tls_crypt_v2".
	Type string `json:"type,omitempty"`
	// Key is the PEM block of the key.
	Key string `json:"key,omitempty"`
	// Direction is "", "server" or "client", of tls-auth.
	Direction string `json:"direction,omitempty"`
}

// OpenConnect is an SSL VPN client: Cisco AnyConnect and the gateways openconnect speaks to.
type OpenConnect struct {
	// Flavor is "", "anyconnect", "gp", "fortinet", "f5", "pulse" or "nc"; "" is AnyConnect.
	Flavor string `json:"flavor,omitempty"`
	// Path follows the host in the gateway's address, such as a group: /employees.
	Path      string `json:"path,omitempty"`
	Username  string `json:"username,omitempty"`
	Password  string `json:"password,omitempty"`
	AuthGroup string `json:"auth_group,omitempty"`
	// PeerFingerprint pins the gateway's key, "pin-sha256:..." as openconnect --servercert writes it.
	PeerFingerprint []string `json:"peer_fingerprint,omitempty"`
	Insecure        bool     `json:"insecure,omitempty"`
	// CA, Cert and Key are PEM blocks; KeyPassword opens an encrypted key.
	CA          string `json:"ca,omitempty"`
	Cert        string `json:"cert,omitempty"`
	Key         string `json:"key,omitempty"`
	KeyPassword string `json:"key_password,omitempty"`
	NoUDP       bool   `json:"no_udp,omitempty"`
	MTU         int    `json:"mtu,omitempty"`
}

// MASQUE tunnels IP over HTTP/3 or HTTP/2, as RFC 9484's CONNECT-IP; it always runs over TLS.
type MASQUE struct {
	Username string            `json:"username,omitempty"`
	Password string            `json:"password,omitempty"`
	Path     string            `json:"path,omitempty"`
	Headers  map[string]string `json:"headers,omitempty"`
	// Version is 0, HTTP/3 falling back to HTTP/2, or 1, 2 or 3 for that HTTP version alone.
	Version int `json:"version,omitempty"`
	MTU     int `json:"mtu,omitempty"`
	TLS     TLS `json:"tls,omitzero"`
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

// Chain dials servers one through another. It has no address of its own and names servers of this Poros, so it
// travels in no link.
type Chain struct {
	// Servers are IDs: the first is dialled directly, each next one through the one before, and the last is where
	// the traffic leaves.
	Servers []string `json:"servers"`
}

// Custom is one sing-box outbound or endpoint as JSON, for what the model has no fields for.
type Custom struct {
	JSON json.RawMessage `json:"json"`
}

// Direct connects without a server, with the rest of what Poros does: the preset, TUN, the kill switch.
type Direct struct{}
