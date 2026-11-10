package profile

import (
	"encoding/base64"
	"encoding/json"
	"errors"
	"reflect"
	"strings"
	"testing"
)

// The same SHA-256 as testSS2022Key, in hex with a separator, as Xray and Hysteria2 write pins.
const testPinHex = "000102030405060708090A0B0C0D0E0F:101112131415161718191a1b1c1d1e1f"

func TestSingBoxOutbound(t *testing.T) {
	tests := []struct {
		name string
		json string
		tag  string
		want Profile
	}{
		{
			name: "vless reality vision as panels export it",
			json: `{"type": "vless", "tag": " 🇩🇪 Germany ", "server": "DE1.Example.com", "server_port": 443,
				"uuid": "` + testUUID + `", "flow": "xtls-rprx-vision", "packet_encoding": "xudp",
				"tls": {"enabled": true, "server_name": "www.microsoft.com", "insecure": false,
					"utls": {"enabled": true, "fingerprint": "chrome"},
					"reality": {"enabled": true, "public_key": "` + testPBK + `=", "short_id": "6ba85179e30d4fc2"}},
				"domain_strategy": "ipv4_only", "tcp_fast_open": true}`,
			tag: "🇩🇪 Germany",
			want: Profile{Type: TypeVLESS, Server: "de1.example.com", ServerPort: 443, VLESS: &VLESS{
				UUID: testUUID, Flow: "xtls-rprx-vision", PacketEncoding: "xudp",
				TLS: TLS{Enabled: true, ServerName: "www.microsoft.com", UTLS: UTLS{Enabled: true, Fingerprint: "chrome"},
					Reality: Reality{Enabled: true, PublicKey: testPBK, ShortID: "6ba85179e30d4fc2"}},
			}},
		},
		{
			name: "vless ws tls drops vision and takes the host from the headers",
			json: `{"type": "vless", "tag": "WS", "server": "ws.example.com", "server_port": 8443, "uuid": "` + testUUID + `",
				"flow": "xtls-rprx-vision", "packet_encoding": "",
				"tls": {"enabled": true, "server_name": "ws.example.com", "alpn": "h2, http/1.1",
					"utls": {"enabled": true, "fingerprint": "Firefox"}},
				"transport": {"type": "ws", "path": "/ray?ed=2048",
					"headers": {"host": "cdn.example.com", "User-Agent": ["Mozilla/5.0", "curl/8.5"]}}}`,
			tag: "WS",
			want: Profile{Type: TypeVLESS, Server: "ws.example.com", ServerPort: 8443, VLESS: &VLESS{
				UUID: testUUID,
				TLS: TLS{Enabled: true, ServerName: "ws.example.com", ALPN: []string{"h2", "http/1.1"},
					UTLS: UTLS{Enabled: true, Fingerprint: "firefox"}},
				Transport: Transport{Type: TransportWebSocket, Host: "cdn.example.com", Path: "/ray",
					Headers: map[string]string{"User-Agent": "Mozilla/5.0"}, MaxEarlyData: 2048, EarlyDataHeaderName: "Sec-WebSocket-Protocol"},
			}},
		},
		{
			name: "vless grpc keeps the xudp default and runs utls without a fingerprint as chrome",
			json: `{"type": "vless", "tag": "gRPC", "server": "203.0.113.10", "server_port": 2053, "uuid": "` + testUUID + `",
				"tls": {"enabled": true, "server_name": "grpc.example.com", "utls": {"enabled": true}},
				"transport": {"type": "grpc", "service_name": "gun/svc", "idle_timeout": "15s", "permit_without_stream": true},
				"multiplex": {"enabled": false, "protocol": "smux"}}`,
			tag: "gRPC",
			want: Profile{Type: TypeVLESS, Server: "203.0.113.10", ServerPort: 2053, VLESS: &VLESS{
				UUID: testUUID, PacketEncoding: "xudp",
				TLS:       TLS{Enabled: true, ServerName: "grpc.example.com", UTLS: UTLS{Enabled: true, Fingerprint: "chrome"}},
				Transport: Transport{Type: TransportGRPC, ServiceName: "gun/svc"},
			}},
		},
		{
			name: "vless with tls off ignores reality and the flow",
			json: `{"type": "VLESS", "server": "v.example.com", "server_port": 80, "uuid": "` + testUUID + `",
				"flow": "xtls-rprx-vision", "packet_encoding": "packetaddr",
				"tls": {"enabled": false, "server_name": "v.example.com", "reality": {"enabled": true, "public_key": "bad"}},
				"transport": {"type": "tcp"}}`,
			want: Profile{Type: TypeVLESS, Server: "v.example.com", ServerPort: 80, VLESS: &VLESS{
				UUID: testUUID, PacketEncoding: "packetaddr",
			}},
		},
		{
			name: "vmess httpupgrade with mux has no packet encoding unless it says so",
			json: `{"type": "vmess", "tag": "🇳🇱 VMess", "server": "vm.example.com", "server_port": 443, "uuid": "` + testUUID + `",
				"security": "AES-128-GCM", "alter_id": 0, "global_padding": true, "authenticated_length": true,
				"tls": {"enabled": true, "server_name": "vm.example.com", "insecure": true, "alpn": ["h2", "", "http/1.1"],
					"utls": {"enabled": true, "fingerprint": "chrome_pq"}},
				"transport": {"type": "httpupgrade", "host": "cdn.example.com", "path": "/up?ed=2048",
					"headers": {"X-Forwarded-For": "198.51.100.1"}},
				"multiplex": {"enabled": true, "protocol": "h2mux", "max_streams": 8, "min_streams": -1, "padding": true,
					"brutal": {"enabled": true, "up_mbps": 100, "down_mbps": 100}}}`,
			tag: "🇳🇱 VMess",
			want: Profile{Type: TypeVMess, Server: "vm.example.com", ServerPort: 443, VMess: &VMess{
				UUID: testUUID, Security: "aes-128-gcm",
				TLS: TLS{Enabled: true, ServerName: "vm.example.com", Insecure: true, ALPN: []string{"h2", "http/1.1"},
					UTLS: UTLS{Enabled: true, Fingerprint: "chrome"}},
				Transport: Transport{Type: TransportHTTPUpgrade, Host: "cdn.example.com", Path: "/up",
					Headers: map[string]string{"X-Forwarded-For": "198.51.100.1"}},
				Multiplex: Multiplex{Mode: SwitchOn, Protocol: "h2mux", MaxStreams: 8, Padding: true},
			}},
		},
		{
			name: "vmess over the http transport",
			json: `{"type": "vmess", "server": "198.51.100.20", "server_port": 8080, "uuid": "` + testUUID + `",
				"security": "aes-128-cfb", "alter_id": 64, "packet_encoding": "packetaddr",
				"transport": {"type": "http", "host": ["a.example.com", " b.example.com"], "path": "/video", "method": "PUT",
					"headers": {"Accept": ["*/*"]}, "idle_timeout": "30s"}}`,
			want: Profile{Type: TypeVMess, Server: "198.51.100.20", ServerPort: 8080, VMess: &VMess{
				UUID: testUUID, AlterID: 64, Security: "auto", PacketEncoding: "packetaddr",
				Transport: Transport{Type: TransportHTTP, Host: "a.example.com,b.example.com", Path: "/video", Method: "PUT",
					Headers: map[string]string{"Accept": "*/*"}},
			}},
		},
		{
			name: "trojan over the quic transport with ech, pins and one tls version",
			json: `{"type": "trojan", "tag": "Trojan", "server": "tr.example.com", "server_port": 443, "password": "p@ss:w0rd",
				"tls": {"enabled": true, "server_name": "tr.example.com", "min_version": "1.3", "max_version": "1.3",
					"certificate_sha256": "` + testPinHex + `", "certificate_public_key_sha256": ["` + testSS2022Key + `"],
					"ech": {"enabled": true, "config": "AEX+DQBBpQAgACB", "query_server_name": "cloudflare-ech.com"},
					"certificate": "-----BEGIN CERTIFICATE-----\nMIIB\n-----END CERTIFICATE-----", "fragment": true},
				"transport": {"type": "quic"}}`,
			tag: "Trojan",
			want: Profile{Type: TypeTrojan, Server: "tr.example.com", ServerPort: 443, Trojan: &Trojan{
				Password: "p@ss:w0rd",
				TLS: TLS{Enabled: true, ServerName: "tr.example.com", MinVersion: "1.3", MaxVersion: "1.3",
					CertificateSHA256: []string{testSS2022Key}, CertificatePublicKeySHA256: []string{testSS2022Key}, Fragment: SwitchOn,
					ECH: ECH{Enabled: true, Config: []string{echPEMBegin, "AEX+DQBBpQAgACB", echPEMEnd}, QueryServerName: "cloudflare-ech.com"}},
				Transport: Transport{Type: TransportQUIC},
			}},
		},
		{
			name: "trojan ws with an ech pem block and explicit early data",
			json: `{"type": "trojan", "server": "tr2.example.com", "server_port": 443, "password": "secret",
				"tls": {"enabled": true, "server_name": "tr2.example.com",
					"ech": {"enabled": true, "config": "-----BEGIN ECH CONFIGS-----\r\nAEX+DQBBpQAgACB\r\n-----END ECH CONFIGS-----\n"}},
				"transport": {"type": "ws", "path": "/ws", "headers": {"Host": ["cdn.example.com"]},
					"max_early_data": 2048, "early_data_header_name": "Sec-WebSocket-Protocol"}}`,
			want: Profile{Type: TypeTrojan, Server: "tr2.example.com", ServerPort: 443, Trojan: &Trojan{
				Password: "secret",
				TLS: TLS{Enabled: true, ServerName: "tr2.example.com",
					ECH: ECH{Enabled: true, Config: []string{echPEMBegin, "AEX+DQBBpQAgACB", echPEMEnd}}},
				Transport: Transport{Type: TransportWebSocket, Host: "cdn.example.com", Path: "/ws",
					MaxEarlyData: 2048, EarlyDataHeaderName: "Sec-WebSocket-Protocol"},
			}},
		},
		{
			name: "trojan without tls stays plain",
			json: `{"type": "trojan", "server": "t.example.com", "server_port": 8080, "password": "secret", "multiplex": {"enabled": true}}`,
			want: Profile{Type: TypeTrojan, Server: "t.example.com", ServerPort: 8080, Trojan: &Trojan{
				Password: "secret", Multiplex: Multiplex{Mode: SwitchOn},
			}},
		},
		{
			name: "shadowsocks 2022 with udp over tcp as an object and smux",
			json: `{"type": "shadowsocks", "tag": "SS 2022", "server": "ss.example.com", "server_port": 443,
				"method": "2022-blake3-aes-256-gcm", "password": "` + testSS2022Key + `",
				"udp_over_tcp": {"enabled": true, "version": 2},
				"multiplex": {"enabled": true, "protocol": "smux", "max_connections": 4, "min_streams": 4}}`,
			tag: "SS 2022",
			want: Profile{Type: TypeShadowsocks, Server: "ss.example.com", ServerPort: 443, Shadowsocks: &Shadowsocks{
				Method: "2022-blake3-aes-256-gcm", Password: testSS2022Key, UDPOverTCP: true,
				Multiplex: Multiplex{Mode: SwitchOn, Protocol: "smux", MaxConnections: 4, MinStreams: 4},
			}},
		},
		{
			name: "shadowsocks with simple-obfs on a bracketed ipv6 address",
			json: `{"type": "shadowsocks", "server": "[2001:DB8::5]", "server_port": 8388, "method": " AES-128-GCM ", "password": "pass",
				"plugin": "simple-obfs", "plugin_opts": " obfs=http;obfs-host=www.bing.com ", "udp_over_tcp": true, "network": "tcp"}`,
			want: Profile{Type: TypeShadowsocks, Server: "2001:db8::5", ServerPort: 8388, Shadowsocks: &Shadowsocks{
				Method: "aes-128-gcm", Password: "pass", Plugin: "obfs-local", PluginOpts: "obfs=http;obfs-host=www.bing.com", UDPOverTCP: true,
			}},
		},
		{
			name: "hysteria2 port hopping with salamander and tls forced on",
			json: `{"type": "hysteria2", "tag": "HY2 hop", "server": "hy.example.com", "server_ports": ["20000:30000", "443"],
				"hop_interval": "30s", "hop_interval_max": "60s", "up_mbps": 50, "down_mbps": 200,
				"obfs": {"password": "gawr+gura"}, "password": "letmein",
				"tls": {"server_name": "hy.example.com", "insecure": true, "alpn": "h3",
					"utls": {"enabled": true, "fingerprint": "chrome"}, "certificate_sha256": "` + testSS2022Key + `"},
				"bbr_profile": "aggressive"}`,
			tag: "HY2 hop",
			want: Profile{Type: TypeHysteria2, Server: "hy.example.com", ServerPort: 20000, Hysteria2: &Hysteria2{
				Password: "letmein", ServerPorts: []string{"20000:30000", "443:443"}, HopInterval: "30s", UpMbps: 50, DownMbps: 200,
				ObfsType: "salamander", ObfsPassword: "gawr+gura",
				TLS: TLS{Enabled: true, ServerName: "hy.example.com", Insecure: true, ALPN: []string{"h3"},
					CertificateSHA256: []string{testSS2022Key}},
			}},
		},
		{
			name: "hysteria2 with its port, one dashed range and the legacy obfs password",
			json: `{"type": "hysteria2", "server": "2001:db8::2", "server_port": 443, "server_ports": "5000-6000", "hop_interval": "30",
				"obfs": {"type": "Salamander", "password": ""}, "obfsPassword": "legacy",
				"tls": {"enabled": true, "reality": {"enabled": true, "public_key": "x"}}}`,
			want: Profile{Type: TypeHysteria2, Server: "2001:db8::2", ServerPort: 443, Hysteria2: &Hysteria2{
				ServerPorts: []string{"5000:6000"}, ObfsType: "salamander", ObfsPassword: "legacy", TLS: TLS{Enabled: true},
			}},
		},
		{
			name: "tuic",
			json: `{"type": "tuic", "tag": "TUIC", "server": "tuic.example.com", "server_port": 443, "uuid": "` + testUUID + `",
				"password": "p@ss", "congestion_control": "BBR", "udp_relay_mode": "quic", "udp_over_stream": false,
				"zero_rtt_handshake": true, "heartbeat": "10s",
				"tls": {"enabled": true, "server_name": "tuic.example.com", "alpn": ["h3"], "insecure": true}}`,
			tag: "TUIC",
			want: Profile{Type: TypeTUIC, Server: "tuic.example.com", ServerPort: 443, TUIC: &TUIC{
				UUID: testUUID, Password: "p@ss", CongestionControl: "bbr", UDPRelayMode: "quic", ZeroRTT: true, Heartbeat: "10s",
				TLS: TLS{Enabled: true, ServerName: "tuic.example.com", Insecure: true, ALPN: []string{"h3"}},
			}},
		},
		{
			name: "tuic drops unknown values and malformed lists",
			json: `{"type": "tuic", "server": "198.51.100.30", "server_port": 443, "uuid": "` + testUUID + `",
				"congestion_control": "bbr2", "udp_relay_mode": "bogus", "heartbeat": "10",
				"tls": {"alpn": [3], "certificate_sha256": {"sha": 1}}}`,
			want: Profile{Type: TypeTUIC, Server: "198.51.100.30", ServerPort: 443, TUIC: &TUIC{
				UUID: testUUID, TLS: TLS{Enabled: true},
			}},
		},
		{
			// The Qt client's test: a certificate on another computer never reaches the profile.
			name: "socks keeps nothing of a tls object",
			json: `{"type": "socks", "tag": "Germany", "server": "198.51.100.1", "server_port": 1080,
				"tls": {"certificate_path": "\\\\evil\\share\\ca.pem"}}`,
			tag:  "Germany",
			want: Profile{Type: TypeSOCKS, Server: "198.51.100.1", ServerPort: 1080, SOCKS: &SOCKS{Version: "5"}},
		},
		{
			name: "socks4a",
			json: `{"type": "socks", "server": "Proxy.example.com", "server_port": 1080, "version": "4A", "username": "user1234"}`,
			want: Profile{Type: TypeSOCKS, Server: "proxy.example.com", ServerPort: 1080, SOCKS: &SOCKS{Version: "4a", Username: "user1234"}},
		},
		{
			name: "socks5 written as a number with auth",
			json: `{"type": "socks", "server": "127.0.0.1", "server_port": 1081, "version": 5, "username": "user", "password": "pass",
				"udp_over_tcp": {"enabled": true}}`,
			want: Profile{Type: TypeSOCKS, Server: "127.0.0.1", ServerPort: 1081, SOCKS: &SOCKS{Version: "5", Username: "user", Password: "pass"}},
		},
		{
			name: "http over tls with path and headers",
			json: `{"type": "http", "tag": "office", "server": "proxy.example.com", "server_port": 443, "username": "alice",
				"password": "secret", "path": "/connect", "headers": {"X-Token": "abc", "X-Id": ["7", "8"], " ": "dropped"},
				"tls": {"enabled": true, "server_name": "proxy.example.com", "utls": {"enabled": true, "fingerprint": "netscape"}},
				"version": 2}`,
			tag: "office",
			want: Profile{Type: TypeHTTP, Server: "proxy.example.com", ServerPort: 443, HTTP: &HTTP{
				Username: "alice", Password: "secret", Path: "/connect", Headers: map[string]string{"X-Token": "abc", "X-Id": "7"},
				TLS: TLS{Enabled: true, ServerName: "proxy.example.com"},
			}},
		},
		{
			name: "plain http drops headers that are not an object",
			json: `{"type": "http", "server": "203.0.113.5", "server_port": 3128, "headers": "X-Token: abc"}`,
			want: Profile{Type: TypeHTTP, Server: "203.0.113.5", ServerPort: 3128, HTTP: &HTTP{}},
		},
		{
			name: "anytls keeps what the json sets",
			json: `{"type": "anytls", "tag": "AnyTLS", "server": "at.example.com", "server_port": 8443, "password": "passw0rd",
				"idle_session_check_interval": "bad", "idle_session_timeout": "60s", "min_idle_session": 0,
				"tls": {"enabled": true, "server_name": "at.example.com", "insecure": true,
					"utls": {"enabled": true, "fingerprint": "safari"}}}`,
			tag: "AnyTLS",
			want: Profile{Type: TypeAnyTLS, Server: "at.example.com", ServerPort: 8443, AnyTLS: &AnyTLS{
				Password: "passw0rd", IdleSessionCheckInterval: "30s", IdleSessionTimeout: "60s",
				TLS: TLS{Enabled: true, ServerName: "at.example.com", Insecure: true, UTLS: UTLS{Enabled: true, Fingerprint: "safari"}},
			}},
		},
		{
			name: "anytls defaults keep tls on",
			json: `{"type": "anytls", "server": "at2.example.com", "server_port": 443, "password": "pw"}`,
			want: Profile{Type: TypeAnyTLS, Server: "at2.example.com", ServerPort: 443, AnyTLS: &AnyTLS{
				Password: "pw", IdleSessionCheckInterval: "30s", IdleSessionTimeout: "30s", MinIdleSession: 5, TLS: TLS{Enabled: true},
			}},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, tag, proxy, err := singBoxOutbound(json.RawMessage(tt.json))
			if err != nil || !proxy {
				t.Fatalf("singBoxOutbound: proxy %v, error %v", proxy, err)
			}
			if tag != tt.tag {
				t.Errorf("tag %q, want %q", tag, tt.tag)
			}
			if err = got.Validate(); err != nil {
				t.Errorf("the profile does not validate: %v", err)
			}
			if !reflect.DeepEqual(got, tt.want) {
				t.Errorf("singBoxOutbound\n got %s\nwant %s", dump(got), dump(tt.want))
			}
		})
	}
}

func TestSingBoxOutboundLeavesOutInfra(t *testing.T) {
	for _, raw := range []string{
		`{"type": "direct", "tag": "direct"}`,
		`{"type": "block", "tag": "block"}`,
		`{"type": "dns", "tag": "dns-out"}`,
		`{"type": "selector", "tag": "proxy", "outbounds": ["auto", "DE"], "default": "auto"}`,
		`{"type": "urltest", "tag": "auto", "outbounds": ["DE"], "url": "https://www.gstatic.com/generate_204"}`,
		`{"type": " Block ", "tag": "Germany"}`,
		// ShadowTLS only carries the Shadowsocks server that goes through it.
		`{"type": "shadowtls", "tag": "st", "server": "st.example.com", "server_port": 443, "version": 3}`,
		// The Qt client left out what is not an object.
		`"direct"`, `5`, `null`, `[]`,
	} {
		if _, _, proxy, err := singBoxOutbound(json.RawMessage(raw)); proxy || err != nil {
			t.Errorf("singBoxOutbound(%s): proxy %v, error %v; want it left out", raw, proxy, err)
		}
	}
}

func TestSingBoxOutboundRejects(t *testing.T) {
	const vless = `"type": "vless", "tag": "x", "server": "v.example.com", "server_port": 443, "uuid": "` + testUUID + `"`
	const reality = `"tls": {"enabled": true, "server_name": "www.microsoft.com", "utls": {"enabled": true}, "reality": {"enabled": true, `
	tests := []struct {
		name        string
		json        string
		unsupported bool
		want        string
	}{
		{"wireguard key not a key", `{"type": "wireguard", "tag": "x", "address": ["172.16.0.2/32"], "private_key": "s3cret",
			"peers": [{"address": "engage.cloudflareclient.com", "port": 2408, "public_key": "s3cret"}]}`, false, "32 bytes in base64"},
		{"hysteria v1 without speeds", `{"type": "hysteria", "tag": "x", "server": "h.example.com", "server_port": 443, "auth_str": "s3cret"}`, false, "speeds"},
		{"naive skipping the certificate check", `{"type": "naive", "tag": "x", "server": "n.example.com", "server_port": 443, "password": "s3cret",
			"tls": {"enabled": true, "insecure": true}}`, true, "certificate"},
		{"ssh key from a file", `{"type": "ssh", "tag": "x", "server": "ssh.example.com", "server_port": 22, "private_key_path": "id_s3cret"}`, false, "file path"},
		{"snell v5", `{"type": "snell", "tag": "x", "server": "sn.example.com", "server_port": 443, "version": 5, "psk": "s3cret"}`, true, "versions 4 and 6"},
		{"tor", `{"type": "tor", "tag": "x"}`, true, `type "tor"`},
		{"tailscale endpoint", `{"type": "tailscale", "tag": "x", "auth_key": "s3cret"}`, true, `type "tailscale"`},
		{"detour", `{"type": "shadowsocks", "tag": "x", "detour": "st", "method": "aes-128-gcm", "password": "s3cret"}`, true, `outbound "st"`},
		{"xhttp transport", `{` + vless + `, "transport": {"type": "xhttp", "path": "/"}}`, true, `transport "xhttp"`},
		{"gecko obfs", `{"type": "hysteria2", "tag": "x", "server": "hy.example.com", "server_port": 443,
			"obfs": {"type": "gecko", "password": "s3cret", "min_packet_size": 512}}`, true, `obfuscation "gecko"`},
		{"hysteria2 realm over http", `{"type": "hysteria2", "tag": "x", "password": "s3cret",
			"realm": {"server_url": "http://realm.example.com", "realm_id": "r", "stun_servers": "stun.example.com:3478"}}`, false, "https://"},
		{"http/3", `{"type": "http", "tag": "x", "server": "h.example.com", "server_port": 443, "version": 3}`, true, "HTTP/3"},

		{"no type", `{"tag": "x", "server": "v.example.com", "password": "s3cret"}`, false, "has no type"},
		{"type not text", `{"type": 5, "tag": "x"}`, false, "has no type"},
		{"port as text", `{"type": "trojan", "tag": "x", "server": "t.example.com", "server_port": "443", "password": "s3cret"}`, false, "server_port"},
		{"tls as a bool", `{"type": "trojan", "tag": "x", "server": "t.example.com", "server_port": 443, "password": "s3cret", "tls": true}`, false, "tls"},
		{"server name as a number", `{"type": "trojan", "tag": "x", "server": "t.example.com", "server_port": 443, "password": "s3cret",
			"tls": {"enabled": true, "server_name": 1}}`, false, "tls.server_name"},
		{"bad host", `{"type": "trojan", "tag": "x", "server": "bad host", "server_port": 443, "password": "s3cret"}`, false, "not a host name"},
		{"reality without a key", `{` + vless + `, ` + reality + `"short_id": "ab"}}}`, false, "public key"},
		{"reality key of the wrong size", `{` + vless + `, ` + reality + `"public_key": "c3Mz"}}}`, false, "32 bytes"},
		{"reality short id too long", `{` + vless + `, ` + reality + `"public_key": "` + testPBK + `", "short_id": "0123456789abcdef01"}}}`, false, "short ID"},
		{"reality short id of odd length", `{` + vless + `, ` + reality + `"public_key": "` + testPBK + `", "short_id": "abc"}}}`, false, "short ID"},
		{"reality with ech", `{` + vless + `, "tls": {"enabled": true, "reality": {"enabled": true, "public_key": "` + testPBK + `"},
			"ech": {"enabled": true}}}`, false, "ECH"},
		{"reality without sni", `{` + vless + `, "tls": {"enabled": true, "disable_sni": true,
			"reality": {"enabled": true, "public_key": "` + testPBK + `"}}}`, false, "disable_sni"},
		{"reality over the quic transport", `{` + vless + `, ` + reality + `"public_key": "` + testPBK + `"}},
			"transport": {"type": "quic"}}`, false, "QUIC transport"},
		{"quic transport without tls", `{"type": "vmess", "tag": "x", "server": "vm.example.com", "server_port": 443,
			"uuid": "` + testUUID + `", "transport": {"type": "quic"}}`, false, "needs TLS"},
		{"bad pin", `{"type": "trojan", "tag": "x", "server": "t.example.com", "server_port": 443, "password": "s3cret",
			"tls": {"enabled": true, "insecure": true, "certificate_sha256": ["deadbeef"]}}`, false, `pin "deadbeef"`},
		{"bad key pin", `{"type": "anytls", "tag": "x", "server": "a.example.com", "server_port": 443, "password": "s3cret",
			"tls": {"certificate_public_key_sha256": "abc="}}`, false, `pin "abc="`},
		{"spoof without sni", `{"type": "trojan", "tag": "x", "server": "t.example.com", "server_port": 443, "password": "s3cret",
			"tls": {"enabled": true, "disable_sni": true, "spoof": "www.example.com"}}`, false, "disable_sni"},
		{"spoof of the real name", `{"type": "trojan", "tag": "x", "server": "t.example.com", "server_port": 443, "password": "s3cret",
			"tls": {"enabled": true, "server_name": "www.example.com", "spoof": "www.example.com"}}`, false, "differ"},
		{"tls versions the wrong way round", `{"type": "trojan", "tag": "x", "server": "t.example.com", "server_port": 443,
			"password": "s3cret", "tls": {"enabled": true, "min_version": "1.3", "max_version": "1.2"}}`, false, "minimum TLS version"},
		{"hops too fast", `{"type": "hysteria2", "tag": "x", "server": "hy.example.com", "server_ports": ["20000:30000"],
			"hop_interval": "1s", "password": "s3cret"}`, false, "at least 5s"},
		{"bad port range", `{"type": "hysteria2", "tag": "x", "server": "hy.example.com", "server_ports": "20000:70000",
			"password": "s3cret"}`, false, `range "20000:70000"`},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			p, tag, proxy, err := singBoxOutbound(json.RawMessage(tt.json))
			// Some refusals are Validate's, which the document runs on every profile.
			if err == nil {
				err = p.Validate()
			}
			if err == nil {
				t.Fatalf("singBoxOutbound = %s, want an error with %q", dump(p), tt.want)
			}
			if !proxy || tag != "x" {
				t.Errorf("proxy %v, tag %q: a refused server is still reported under its tag", proxy, tag)
			}
			if !strings.Contains(err.Error(), tt.want) || errors.Is(err, ErrUnsupported) != tt.unsupported {
				t.Errorf("error %q (unsupported %v), want it to contain %q (unsupported %v)",
					err, errors.Is(err, ErrUnsupported), tt.want, tt.unsupported)
			}
			if strings.Contains(err.Error(), "s3cret") || strings.Contains(err.Error(), testUUID) || strings.Contains(err.Error(), testPBK) {
				t.Errorf("the error %q gives away a secret", err)
			}
		})
	}
}

func TestParseDocumentSingBoxConfig(t *testing.T) {
	body := `{
		"log": {"level": "warn", "timestamp": true},
		"dns": {"servers": [{"type": "https", "tag": "remote", "server": "1.1.1.1", "detour": "proxy"}], "final": "remote"},
		"inbounds": [{"type": "tun", "tag": "tun-in", "address": ["172.19.0.1/30"], "auto_route": true, "strict_route": true}],
		"outbounds": [
			{"type": "selector", "tag": "proxy", "outbounds": ["auto", "🇩🇪 DE", "🇳🇱 NL"], "default": "auto"},
			{"type": "urltest", "tag": "auto", "outbounds": ["🇩🇪 DE", "🇳🇱 NL"], "interval": "3m"},
			{"type": "vless", "tag": "🇩🇪 DE", "server": "de.example.com", "server_port": 443, "uuid": "` + testUUID + `",
				"flow": "xtls-rprx-vision", "packet_encoding": "xudp",
				"tls": {"enabled": true, "server_name": "www.microsoft.com", "utls": {"enabled": true, "fingerprint": "chrome"},
					"reality": {"enabled": true, "public_key": "` + testPBK + `", "short_id": "ab"}}},
			{"type": "hysteria2", "tag": "🇳🇱 NL", "server": "nl.example.com", "server_port": 443, "password": "s3cret",
				"up_mbps": 100, "down_mbps": 500, "tls": {"enabled": true, "server_name": "nl.example.com", "alpn": ["h3"]}},
			{"type": "shadowtls", "tag": "st", "server": "st.example.com", "server_port": 443, "version": 3, "password": "s3cret",
				"tls": {"enabled": true, "server_name": "www.apple.com"}},
			{"type": "shadowsocks", "tag": "SS over ShadowTLS", "detour": "st", "method": "2022-blake3-aes-128-gcm",
				"password": "AAECAwQFBgcICQoLDA0ODw=="},
			{"type": "vmess", "tag": "No server", "server_port": 443, "uuid": "` + testUUID + `"},
			{"type": "direct", "tag": "direct"},
			{"type": "block", "tag": "block"},
			{"type": "dns", "tag": "dns-out"}
		],
		"endpoints": [
			{"type": "wireguard", "tag": "WARP", "address": ["172.16.0.2/32", "2606:4700:110:8a36::2/128"], "private_key": "` + testWGPrivate + `",
				"mtu": 1280, "peers": [{"address": "engage.cloudflareclient.com", "port": 2408, "public_key": "` + testWGPublic + `",
				"allowed_ips": ["0.0.0.0/0"], "reserved": [1, 2, 3], "persistent_keepalive_interval": 25}]},
			{"type": "tailscale", "tag": "Tailnet", "auth_key": "s3cret"}
		],
		"route": {"final": "proxy", "auto_detect_interface": true}
	}`
	d := mustParseDocument(t, body)
	if d.Format != FormatSingBox {
		t.Fatalf("format %q", d.Format)
	}
	wantProfiles := []Profile{
		{Type: TypeVLESS, Name: "🇩🇪 DE", Server: "de.example.com", ServerPort: 443, VLESS: &VLESS{
			UUID: testUUID, Flow: "xtls-rprx-vision", PacketEncoding: "xudp",
			TLS: TLS{Enabled: true, ServerName: "www.microsoft.com", UTLS: UTLS{Enabled: true, Fingerprint: "chrome"},
				Reality: Reality{Enabled: true, PublicKey: testPBK, ShortID: "ab"}},
		}},
		{Type: TypeHysteria2, Name: "🇳🇱 NL", Server: "nl.example.com", ServerPort: 443, Hysteria2: &Hysteria2{
			Password: "s3cret", UpMbps: 100, DownMbps: 500, TLS: TLS{Enabled: true, ServerName: "nl.example.com", ALPN: []string{"h3"}},
		}},
		{Type: TypeShadowsocks, Name: "SS over ShadowTLS", Server: "st.example.com", ServerPort: 443, Shadowsocks: &Shadowsocks{
			Method: "2022-blake3-aes-128-gcm", Password: "AAECAwQFBgcICQoLDA0ODw==",
			ShadowTLS: ShadowTLS{Version: 3, Password: "s3cret", TLS: TLS{Enabled: true, ServerName: "www.apple.com"}},
		}},
		{Type: TypeWireGuard, Name: "WARP", Server: "engage.cloudflareclient.com", ServerPort: 2408, WireGuard: &WireGuard{
			PrivateKey: testWGPrivate, PeerPublicKey: testWGPublic, Address: []string{"172.16.0.2/32", "2606:4700:110:8a36::2/128"},
			Reserved: []int{1, 2, 3}, MTU: 1280, PersistentKeepalive: 25,
		}},
	}
	if !reflect.DeepEqual(d.Profiles, wantProfiles) {
		t.Errorf("profiles\n got %q\nwant %q", names(d.Profiles), names(wantProfiles))
		for i := range min(len(d.Profiles), len(wantProfiles)) {
			t.Errorf("profile %d\n got %s\nwant %s", i, dump(d.Profiles[i]), dump(wantProfiles[i]))
		}
	}
	// Endpoints count on from the outbounds.
	wantSkipped := []Skipped{
		{Index: 7, Name: "No server", Reason: "the server address is missing"},
		{Index: 12, Name: "Tailnet", Unsupported: true, Reason: `the sing-box type "tailscale" is not supported`},
	}
	if !reflect.DeepEqual(d.Skipped, wantSkipped) {
		t.Errorf("skipped\n got %+v\nwant %+v", d.Skipped, wantSkipped)
	}
}

func TestParseDocumentSingBoxShapes(t *testing.T) {
	list := `[
		{"type": "trojan", "tag": "A", "server": "a.example.com", "server_port": 443, "password": "pw",
			"tls": {"enabled": true, "server_name": "a.example.com"}},
		{"type": "direct", "tag": "direct"},
		{"type": "tuic", "tag": "TUIC v4", "server": "tuic.example.com", "server_port": 443, "uuid": "token"},
		{"type": "socks", "tag": "B", "server": "198.51.100.1", "server_port": 1080}
	]`
	tests := []struct {
		name    string
		body    string
		names   []string
		skipped []int
	}{
		{"list of outbounds", list, []string{"A", "B"}, []int{3}},
		{"list in base64", base64.StdEncoding.EncodeToString([]byte(list)), []string{"A", "B"}, []int{3}},
		// The Qt client's tests: one outbound alone is a server, a config of groups and direct holds none.
		{"one outbound", `{"type": "socks", "tag": "Germany", "server": "198.51.100.1", "server_port": 1080}`, []string{"Germany"}, nil},
		{"one direct outbound", `{"type": "direct", "tag": "Germany"}`, nil, nil},
		{"config without a server", `{"outbounds": [{"type": "direct"}, {"type": "selector", "outbounds": ["direct"]}]}`, nil, nil},
		{"endpoints only", `{"endpoints": [{"type": "wireguard", "tag": "WARP"}]}`, nil, []int{1}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			d := mustParseDocument(t, tt.body)
			var skipped []int
			for _, s := range d.Skipped {
				skipped = append(skipped, s.Index)
			}
			if d.Format != FormatSingBox || !reflect.DeepEqual(names(d.Profiles), tt.names) || !reflect.DeepEqual(skipped, tt.skipped) {
				t.Errorf("format %q, profiles %q, skipped %+v", d.Format, names(d.Profiles), d.Skipped)
			}
		})
	}
}
