package profile

import (
	"encoding/json"
	"errors"
	"reflect"
	"strings"
	"testing"
)

func TestXrayOutbound(t *testing.T) {
	tests := []struct {
		name string
		json string
		tag  string
		want Profile
	}{
		{
			name: "marzban vless reality vision in vnext",
			json: `{"tag": "proxy", "protocol": "vless",
				"settings": {"vnext": [{"address": "DE1.Example.com", "port": 443, "users": [{"id": "` + testUUID + `",
					"email": "user@example.com", "security": "auto", "encryption": "none", "flow": "xtls-rprx-vision"}]}]},
				"streamSettings": {"network": "tcp", "tcpSettings": {"header": {"type": "none"}}, "security": "reality",
					"realitySettings": {"serverName": "www.microsoft.com", "fingerprint": "chrome", "show": false,
						"publicKey": "` + testPBK + `=", "shortId": "6ba85179e30d4fc2", "spiderX": "", "mldsa65Verify": ""}},
				"mux": {"enabled": false, "concurrency": -1}}`,
			tag: "proxy",
			want: Profile{Type: TypeVLESS, Server: "de1.example.com", ServerPort: 443, VLESS: &VLESS{
				UUID: testUUID, Flow: "xtls-rprx-vision", PacketEncoding: "xudp",
				TLS: TLS{Enabled: true, ServerName: "www.microsoft.com", UTLS: UTLS{Enabled: true, Fingerprint: "chrome"},
					Reality: Reality{Enabled: true, PublicKey: testPBK, ShortID: "6ba85179e30d4fc2"}},
			}},
		},
		{
			name: "flat vless xhttp reality with a string port moves the top-level keys into extra and drops vision",
			json: `{"tag": "proxy", "protocol": "vless",
				"settings": {"address": "203.0.113.10", "port": "8443", "id": "` + testUUID + `", "encryption": "none",
					"flow": "xtls-rprx-vision", "level": 0},
				"streamSettings": {"network": "xhttp", "security": "reality",
					"realitySettings": {"serverName": "x.example.com", "fingerprint": "Firefox", "password": "` + testPBK + `",
						"publicKey": "ignored", "shortId": " ab ", "spiderX": "/search?q=1"},
					"xhttpSettings": {"host": "x.example.com", "path": "/xh", "mode": "stream-up", "xPaddingBytes": "100-1000",
						"noGRPCHeader": false, "xmux": {"maxConcurrency": "16-32", "cMaxReuseTimes": 0}, "scMaxEachPostBytes": 1000000}}}`,
			tag: "proxy",
			want: Profile{Type: TypeVLESS, Server: "203.0.113.10", ServerPort: 8443, VLESS: &VLESS{
				UUID: testUUID, PacketEncoding: "xudp",
				TLS: TLS{Enabled: true, ServerName: "x.example.com", UTLS: UTLS{Enabled: true, Fingerprint: "firefox"},
					Reality: Reality{Enabled: true, PublicKey: testPBK, ShortID: "ab", SpiderX: "/search?q=1"}},
				Transport: Transport{Type: TransportXHTTP, Host: "x.example.com", Path: "/xh", XHTTP: XHTTP{Mode: "stream-up",
					Extra: json.RawMessage(`{"xPaddingBytes":"100-1000","noGRPCHeader":false,` +
						`"xmux":{"maxConcurrency":"16-32","cMaxReuseTimes":0},"scMaxEachPostBytes":1000000}`)}},
			}},
		},
		{
			name: "splithttp extra outranks the top-level keys and vision stays under encryption",
			json: `{"protocol": "VLESS", "tag": " x2 ",
				"settings": {"vnext": [{"address": "x2.example.com", "port": 2053, "users": [{"id": "` + testUUID + `",
					"encryption": "mlkem768x25519plus.native.0rtt.abc", "flow": "xtls-rprx-vision-udp443"}]}]},
				"streamSettings": {"network": "splithttp", "splithttpSettings": {"path": "/", "mode": "bogus",
					"xPaddingBytes": "1-2", "extra": {"noSSEHeader": true, "xmux": {"maxConcurrency": "16-32"}}}}}`,
			tag: "x2",
			want: Profile{Type: TypeVLESS, Server: "x2.example.com", ServerPort: 2053, VLESS: &VLESS{
				UUID: testUUID, Flow: "xtls-rprx-vision", Encryption: "mlkem768x25519plus.native.0rtt.abc", PacketEncoding: "xudp",
				Transport: Transport{Type: TransportXHTTP, Path: "/", XHTTP: XHTTP{Mode: "auto",
					Extra: json.RawMessage(`{"noSSEHeader":true,"xmux":{"maxConcurrency":"16-32"}}`)}},
			}},
		},
		{
			name: "xhttp extra written as a JSON string, and an alpn of the wrong type dropped",
			json: `{"protocol": "vless", "settings": {"address": "x3.example.com", "port": 443, "id": "` + testUUID + `"},
				"streamSettings": {"network": "xhttp", "security": "tls", "tlsSettings": {"alpn": 5},
					"xhttpSettings": {"path": "/x", "extra": "{\"xPaddingBytes\": \"100-1000\"}"}}}`,
			want: Profile{Type: TypeVLESS, Server: "x3.example.com", ServerPort: 443, VLESS: &VLESS{
				UUID: testUUID, PacketEncoding: "xudp", TLS: TLS{Enabled: true},
				Transport: Transport{Type: TransportXHTTP, Path: "/x", XHTTP: XHTTP{Mode: "auto",
					Extra: json.RawMessage(`{"xPaddingBytes":"100-1000"}`)}},
			}},
		},
		{
			name: "vless ws tls with early data, a host header, pins, ech and dropped sockopt and fragments",
			json: `{"tag": "proxy", "protocol": "vless",
				"settings": {"vnext": [{"address": "ws.example.com", "port": 443, "users": [{"id": "` + testUUID + `",
					"encryption": "", "flow": "xtls-rprx-vision"}]}]},
				"streamSettings": {"network": "ws", "security": "tls",
					"tlsSettings": {"serverName": "ws.example.com", "alpn": "h2, http/1.1,", "fingerprint": "CHROME",
						"allowInsecure": true, "minVersion": "1.2", "maxVersion": "1.4", "echConfigList": "AEX+DQBBpQAgACB",
						"pinnedPeerCertSha256": "00:01:02:03:04:05:06:07:08:09:0A:0B:0C:0D:0E:0F:10:11:12:13:14:15:16:17:18:19:1a:1b:1c:1d:1e:1f",
						"verifyPeerCertByName": "other.example.com", "masterKeyLog": "C:/keys.log"},
					"wsSettings": {"path": "/ray?ed=2048", "host": "", "headers": {"Host": "cdn.example.com", "User-Agent": "curl"},
						"heartbeatPeriod": 30},
					"sockopt": {"tcpFastOpen": true, "mark": 255, "domainStrategy": "UseIPv4"},
					"finalmask": {"tcp": [{"type": "fragment", "settings": {"packets": "tlshello", "length": "100-200"}}],
						"udp": [{"type": "salamander", "settings": {"password": "unused"}}]}}}`,
			tag: "proxy",
			want: Profile{Type: TypeVLESS, Server: "ws.example.com", ServerPort: 443, VLESS: &VLESS{
				UUID: testUUID, PacketEncoding: "xudp",
				TLS: TLS{Enabled: true, ServerName: "ws.example.com", Insecure: true, ALPN: []string{"h2", "http/1.1"},
					MinVersion: "1.2", CertificateSHA256: []string{testSS2022Key}, UTLS: UTLS{Enabled: true, Fingerprint: "chrome"},
					ECH: ECH{Enabled: true, Config: []string{echPEMBegin, "AEX+DQBBpQAgACB", echPEMEnd}}},
				Transport: Transport{Type: TransportWebSocket, Host: "cdn.example.com", Path: "/ray",
					Headers: map[string]string{"User-Agent": "curl"}, MaxEarlyData: 2048, EarlyDataHeaderName: "Sec-WebSocket-Protocol"},
			}},
		},
		{
			name: "vless grpc drops multiMode and authority",
			json: `{"protocol": "vless", "settings": {"vnext": [{"address": "grpc.example.com", "port": 443,
					"users": [{"id": "` + testUUID + `", "encryption": "none"}]}]},
				"streamSettings": {"network": "grpc", "security": "tls", "tlsSettings": {"serverName": "grpc.example.com",
					"verifyPeerCertByName": "GRPC.example.com"},
					"grpcSettings": {"serviceName": "gun/svc", "multiMode": true, "authority": "a.example.com", "user_agent": "x"}}}`,
			want: Profile{Type: TypeVLESS, Server: "grpc.example.com", ServerPort: 443, VLESS: &VLESS{
				UUID: testUUID, PacketEncoding: "xudp",
				TLS:       TLS{Enabled: true, ServerName: "grpc.example.com"},
				Transport: Transport{Type: TransportGRPC, ServiceName: "gun/svc"},
			}},
		},
		{
			name: "method outranks network and httpupgrade loses its early data",
			json: `{"protocol": "vless", "tag": "hu", "settings": {"address": "hu.example.com", "port": 80, "id": "` + testUUID + `",
					"encryption": "none"},
				"streamSettings": {"method": "httpupgrade", "network": "ws", "wsSettings": {"path": "/ws"},
					"httpupgradeSettings": {"path": "/up?ed=1024", "host": "hu.example.com", "headers": {"X-Forwarded-For": "192.0.2.1"}}}}`,
			tag: "hu",
			want: Profile{Type: TypeVLESS, Server: "hu.example.com", ServerPort: 80, VLESS: &VLESS{
				UUID: testUUID, PacketEncoding: "xudp",
				Transport: Transport{Type: TransportHTTPUpgrade, Host: "hu.example.com", Path: "/up",
					Headers: map[string]string{"X-Forwarded-For": "192.0.2.1"}},
			}},
		},
		{
			name: "vless reality over grpc drops the unsafe fingerprint and a spiderX that is no path",
			json: `{"protocol": "vless", "settings": {"address": "r.example.com", "port": 443, "id": "` + testUUID + `",
					"encryption": "none", "flow": "xtls-rprx-vision"},
				"streamSettings": {"network": "grpc", "security": "Reality", "grpcSettings": {"serviceName": "svc"},
					"realitySettings": {"serverName": "r.example.com", "fingerprint": "unsafe", "publicKey": "` + testPBK + `",
						"spiderX": "abc", "mldsa65Verify": "verify-key"}}}`,
			want: Profile{Type: TypeVLESS, Server: "r.example.com", ServerPort: 443, VLESS: &VLESS{
				UUID: testUUID, PacketEncoding: "xudp",
				TLS: TLS{Enabled: true, ServerName: "r.example.com",
					Reality: Reality{Enabled: true, PublicKey: testPBK}},
				Transport: Transport{Type: TransportGRPC, ServiceName: "svc"},
			}},
		},
		{
			name: "marzban vmess ws drops mux",
			json: `{"tag": "proxy", "protocol": "vmess",
				"settings": {"vnext": [{"address": "vm.example.com", "port": 443, "users": [{"id": "` + testUUID + `",
					"alterId": 0, "email": "u@example.com", "security": "auto", "level": 0}]}]},
				"streamSettings": {"network": "ws", "security": "tls",
					"tlsSettings": {"serverName": "vm.example.com", "allowInsecure": false, "fingerprint": ""},
					"wsSettings": {"path": "/vmess", "headers": {"Host": "vm.example.com"}}},
				"mux": {"enabled": true, "concurrency": 8}}`,
			tag: "proxy",
			want: Profile{Type: TypeVMess, Server: "vm.example.com", ServerPort: 443, VMess: &VMess{
				UUID: testUUID, Security: "auto", PacketEncoding: "xudp",
				TLS:       TLS{Enabled: true, ServerName: "vm.example.com"},
				Transport: Transport{Type: TransportWebSocket, Host: "vm.example.com", Path: "/vmess"},
			}},
		},
		{
			name: "flat vmess with a string alterId and the raw http header",
			json: `{"protocol": "vmess", "settings": {"address": "2001:DB8::1", "port": 80, "id": "` + testUUID + `",
					"alterId": "64", "security": "AES-128-GCM"},
				"streamSettings": {"network": "raw",
					"rawSettings": {"header": {"type": "HTTP", "request": {"version": "1.1", "method": "GET",
						"path": ["/", " /video "], "headers": {"host": ["a.example.com", "b.example.com"], "User-Agent": ["Mozilla/5.0"]}}}},
					"tcpSettings": {"header": {"type": "none"}}}}`,
			want: Profile{Type: TypeVMess, Server: "2001:db8::1", ServerPort: 80, VMess: &VMess{
				UUID: testUUID, AlterID: 64, Security: "aes-128-gcm", PacketEncoding: "xudp",
				Transport: Transport{HeaderType: "http", Host: "a.example.com,b.example.com", Path: "/,/video"},
			}},
		},
		{
			name: "3x-ui trojan grpc with an ech query name",
			json: `{"tag": "proxy", "protocol": "trojan",
				"settings": {"servers": [{"address": "tr.example.com", "port": 443, "password": "p@ss:w0rd", "level": 0,
					"email": "x@example.com", "flow": ""}]},
				"streamSettings": {"network": "grpc", "security": "tls",
					"tlsSettings": {"serverName": "tr.example.com", "alpn": ["h2"],
						"echConfigList": "cloudflare-ech.com+https://1.1.1.1/dns-query"},
					"grpcSettings": {"serviceName": "trojan-grpc"}}}`,
			tag: "proxy",
			want: Profile{Type: TypeTrojan, Server: "tr.example.com", ServerPort: 443, Trojan: &Trojan{
				Password: "p@ss:w0rd",
				TLS: TLS{Enabled: true, ServerName: "tr.example.com", ALPN: []string{"h2"},
					ECH: ECH{Enabled: true, QueryServerName: "cloudflare-ech.com"}},
				Transport: Transport{Type: TransportGRPC, ServiceName: "trojan-grpc"},
			}},
		},
		{
			name: "flat trojan without security stays plain",
			json: `{"protocol": "trojan", "settings": {"address": "192.168.1.2", "port": 8080, "password": "secret"}}`,
			want: Profile{Type: TypeTrojan, Server: "192.168.1.2", ServerPort: 8080, Trojan: &Trojan{Password: "secret"}},
		},
		{
			name: "shadowsocks in servers with an Xray cipher name and uot",
			json: `{"tag": "ss", "protocol": "shadowsocks",
				"settings": {"servers": [{"address": "ss.example.com", "port": 8388, "method": "CHACHA20-POLY1305",
					"password": "pw", "uot": true, "UoTVersion": 2}]},
				"streamSettings": {"network": "tcp", "sockopt": {"interface": "eth0"}}}`,
			tag: "ss",
			want: Profile{Type: TypeShadowsocks, Server: "ss.example.com", ServerPort: 8388, Shadowsocks: &Shadowsocks{
				Method: "chacha20-ietf-poly1305", Password: "pw", UDPOverTCP: true,
			}},
		},
		{
			name: "flat shadowsocks 2022",
			json: `{"protocol": "shadowsocks", "settings": {"address": "198.51.100.4", "port": "443",
				"method": "2022-blake3-aes-256-gcm", "password": "` + testSS2022Key + `"}}`,
			want: Profile{Type: TypeShadowsocks, Server: "198.51.100.4", ServerPort: 443, Shadowsocks: &Shadowsocks{
				Method: "2022-blake3-aes-256-gcm", Password: testSS2022Key,
			}},
		},
		{
			name: "socks in servers with a user and a proxySettings without a tag",
			json: `{"protocol": "socks", "tag": "socks-out",
				"settings": {"servers": [{"address": "proxy.example.com", "port": 1080,
					"users": [{"user": "alice", "pass": "secret", "level": 0}]}]},
				"proxySettings": {"transportLayer": true}}`,
			tag: "socks-out",
			want: Profile{Type: TypeSOCKS, Server: "proxy.example.com", ServerPort: 1080, SOCKS: &SOCKS{
				Version: "5", Username: "alice", Password: "secret",
			}},
		},
		{
			name: "flat socks with an ipv6 address in brackets",
			json: `{"protocol": "socks", "settings": {"address": "[2001:db8::7]", "port": 1081}}`,
			want: Profile{Type: TypeSOCKS, Server: "2001:db8::7", ServerPort: 1081, SOCKS: &SOCKS{Version: "5"}},
		},
		{
			name: "http proxy over tls with headers",
			json: `{"protocol": "http", "settings": {"servers": [{"address": "proxy.example.com", "port": 443,
					"users": [{"user": "bob", "pass": "pw"}]}], "headers": {"X-Token": "abc"}},
				"streamSettings": {"security": "tls", "tlsSettings": {"serverName": "proxy.example.com"}}}`,
			want: Profile{Type: TypeHTTP, Server: "proxy.example.com", ServerPort: 443, HTTP: &HTTP{
				Username: "bob", Password: "pw", Headers: map[string]string{"X-Token": "abc"},
				TLS: TLS{Enabled: true, ServerName: "proxy.example.com"},
			}},
		},
		{
			name: "hysteria2 with salamander, port hopping and brutal bandwidth",
			json: `{"tag": "hy2", "protocol": "hysteria", "settings": {"version": 2, "address": "hy.example.com", "port": 0},
				"streamSettings": {"network": "hysteria", "hysteriaSettings": {"version": 2, "auth": "letmein", "udpIdleTimeout": 60},
					"security": "tls", "tlsSettings": {"serverName": "hy.example.com", "alpn": ["h3"], "fingerprint": "chrome",
						"pinnedPeerCertSha256": "000102030405060708090a0b0c0d0e0f101112131415161718191a1b1c1d1e1f"},
					"finalmask": {
						"udp": [{"type": "salamander", "settings": {"password": "gawr+gura"}},
							{"type": "udphop", "settings": {"remotePorts": "20000-30000,40000", "interval": "10-30", "mode": "intervalLocal"}},
							{"type": "noise", "settings": {"noise": []}}],
						"tcp": [{"type": "header-custom"}],
						"quicParams": {"congestion": "brutal", "brutalUp": "50 mbps", "brutalDown": "200 MB"}}}}`,
			tag: "hy2",
			want: Profile{Type: TypeHysteria2, Server: "hy.example.com", ServerPort: 20000, Hysteria2: &Hysteria2{
				Password: "letmein", ServerPorts: []string{"20000:30000", "40000:40000"}, HopInterval: "10s",
				UpMbps: 50, DownMbps: 200, ObfsType: "salamander", ObfsPassword: "gawr+gura",
				TLS: TLS{Enabled: true, ServerName: "hy.example.com", ALPN: []string{"h3"}, CertificateSHA256: []string{testSS2022Key}},
			}},
		},
		{
			name: "hysteria2 without security or versions still runs over tls, and bbr leaves out the bandwidth",
			json: `{"protocol": "hysteria", "settings": {"address": "hy2.example.com", "port": 443},
				"streamSettings": {"hysteriaSettings": {"auth": "pw"},
					"finalmask": {"quicParams": {"congestion": "bbr", "brutalUp": "100 mbps"}}}}`,
			want: Profile{Type: TypeHysteria2, Server: "hy2.example.com", ServerPort: 443, Hysteria2: &Hysteria2{
				Password: "pw", TLS: TLS{Enabled: true},
			}},
		},
		{
			name: "keys in other cases, as Xray reads them",
			json: `{"Protocol": "trojan", "TAG": "Cased",
				"Settings": {"Servers": [{"Address": "C.Example.com", "PORT": 443, "Password": "pw"}]},
				"StreamSettings": {"NETWORK": "WS", "Security": "TLS", "TLSSettings": {"ServerName": "c.example.com"},
					"WsSettings": {"Path": "/c"}}}`,
			tag: "Cased",
			want: Profile{Type: TypeTrojan, Server: "c.example.com", ServerPort: 443, Trojan: &Trojan{
				Password:  "pw",
				TLS:       TLS{Enabled: true, ServerName: "c.example.com"},
				Transport: Transport{Type: TransportWebSocket, Path: "/c"},
			}},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, tag, proxy, err := xrayOutbound(json.RawMessage(tt.json))
			if err != nil || !proxy {
				t.Fatalf("xrayOutbound: proxy %v, error %v", proxy, err)
			}
			if err := got.Validate(); err != nil {
				t.Fatalf("the profile does not validate: %v", err)
			}
			if tag != tt.tag {
				t.Errorf("tag %q, want %q", tag, tt.tag)
			}
			if !reflect.DeepEqual(got, tt.want) {
				t.Errorf("\n got %s\nwant %s", dump(got), dump(tt.want))
			}
		})
	}
}

func TestXrayOutboundLeavesOutInfrastructure(t *testing.T) {
	for _, raw := range []string{
		`{"protocol": "freedom", "tag": "direct", "settings": {"domainStrategy": "UseIP"}}`,
		// Qt's case: Xray lower-cases the protocol.
		`{"protocol": "Freedom", "tag": "Germany"}`,
		`{"protocol": "DIRECT", "tag": "Germany"}`,
		`{"protocol": "blackhole", "tag": "block", "settings": {"response": {"type": "http"}}}`,
		`{"protocol": "block", "tag": "Germany"}`,
		`{"protocol": "dns", "tag": "Germany"}`,
		`{"protocol": "loopback", "tag": "Germany", "settings": {"inboundTag": "socks"}}`,
		`{}`, `"text"`, `[1]`, `null`,
	} {
		p, tag, proxy, err := xrayOutbound(json.RawMessage(raw))
		if proxy || err != nil || !reflect.DeepEqual(p, Profile{}) {
			t.Errorf("%s: proxy %v, error %v, profile %s", raw, proxy, err, dump(p))
		}
		if strings.Contains(raw, "Germany") && tag != "Germany" {
			t.Errorf("%s: tag %q", raw, tag)
		}
	}
}

func TestXrayOutboundRejects(t *testing.T) {
	vless := func(stream string) string {
		return `{"protocol": "vless", "tag": "t", "settings": {"address": "v.example.com", "port": 443, "id": "` + testUUID +
			`", "encryption": "none"}, "streamSettings": ` + stream + `}`
	}
	tests := []struct {
		name        string
		json        string
		want        string
		unsupported bool
	}{
		{"wireguard without a peer", `{"protocol": "wireguard", "tag": "t", "settings": {"secretKey": "c2VjcmV0"}}`, "has no peer", false},
		{"dokodemo-door", `{"protocol": "dokodemo-door", "tag": "t"}`, "Xray's dokodemo-door protocol is not supported", true},
		{"no protocol", `{"tag": "t", "settings": {"address": "v.example.com"}}`, "names no protocol", false},
		{"kcp", vless(`{"network": "kcp"}`), `transport "kcp"`, true},
		{"h2, which Xray removed", vless(`{"network": "h2", "security": "tls"}`), `transport "h2"`, true},
		{"legacy xtls", vless(`{"security": "xtls"}`), "legacy XTLS", true},
		{"unknown security", vless(`{"security": "tls1.3"}`), `security "tls1.3" is unknown`, false},
		{"reality without a key", vless(`{"security": "reality", "realitySettings": {"serverName": "a.example.com", "password": " "}}`),
			"public key", false},
		{"proxySettings chain", `{"protocol": "trojan", "tag": "t", "settings": {"address": "t.example.com", "port": 443,
			"password": "secret"}, "proxySettings": {"tag": "entry"}}`, `through the outbound "entry"`, true},
		{"dialerProxy chain", vless(`{"security": "tls", "sockopt": {"dialerProxy": "fragment"}}`), `through the outbound "fragment"`, true},
		{"download leg chain", vless(`{"network": "xhttp", "security": "tls", "xhttpSettings": {"extra": {"downloadSettings":
			{"address": "dl.example.com", "port": 443, "network": "xhttp", "sockopt": {"dialerProxy": "relay"}}}}}`),
			`through the outbound "relay"`, true},
		{"bad pin", vless(`{"security": "tls", "tlsSettings": {"allowInsecure": true, "pinnedPeerCertSha256": "deadbeef"}}`),
			`pin "deadbeef"`, false},
		{"verifyPeerCertByName without a pin", vless(`{"security": "tls", "tlsSettings": {"serverName": "fake.example.com",
			"verifyPeerCertByName": "real.example.com"}}`), "verifyPeerCertByName", true},
		{"shadowsocks over ws", `{"protocol": "shadowsocks", "tag": "t", "settings": {"address": "s.example.com", "port": 443,
			"method": "aes-256-gcm", "password": "secret"}, "streamSettings": {"network": "ws"}}`, "Shadowsocks over the ws transport", true},
		{"shadowsocks with the http header", `{"protocol": "shadowsocks", "tag": "t", "settings": {"address": "s.example.com",
			"port": 80, "method": "aes-256-gcm", "password": "secret"}, "streamSettings": {"tcpSettings": {"header": {"type": "http"}}}}`,
			"Shadowsocks with the http header", true},
		{"socks over tls", `{"protocol": "socks", "tag": "t", "settings": {"address": "s.example.com", "port": 1080},
			"streamSettings": {"security": "tls"}}`, "SOCKS over TLS", true},
		{"http over grpc", `{"protocol": "http", "tag": "t", "settings": {"address": "h.example.com", "port": 443},
			"streamSettings": {"network": "grpc"}}`, "An HTTP proxy over the grpc transport", true},
		{"a tcp mask the server undoes", vless(`{"security": "tls", "finalmask": {"tcp": [{"type": "fragment"}, {"type": "Header-Custom"}]}}`),
			`finalmask "header-custom"`, true},
		{"unknown raw header", vless(`{"security": "tls", "tcpSettings": {"header": {"type": "srtp"}}}`), `raw header "srtp"`, true},
		{"gecko", `{"protocol": "hysteria", "tag": "t", "settings": {"version": 2, "address": "hy.example.com", "port": 443},
			"streamSettings": {"network": "hysteria", "finalmask": {"udp": [{"type": "salamander",
			"settings": {"password": "secret", "packetSize": "100-200"}}]}}}`, `obfuscation "gecko"`, true},
		{"a udp mask the server undoes", `{"protocol": "hysteria", "tag": "t", "settings": {"address": "hy.example.com", "port": 443},
			"streamSettings": {"finalmask": {"udp": [{"type": "xdns"}]}}}`, `finalmask "xdns"`, true},
		{"hysteria 1", `{"protocol": "hysteria", "tag": "t", "settings": {"version": 1, "address": "hy.example.com", "port": 443}}`,
			"Hysteria version 1", true},
		{"hysteria over raw", `{"protocol": "hysteria", "tag": "t", "settings": {"address": "hy.example.com", "port": 443},
			"streamSettings": {"network": "tcp"}}`, `only over the hysteria transport, not "tcp"`, false},
		{"bad port hopping range", `{"protocol": "hysteria", "tag": "t", "settings": {"address": "hy.example.com", "port": 443},
			"streamSettings": {"finalmask": {"udp": [{"type": "udphop", "settings": {"remotePorts": "70000-80000"}}]}}}`,
			`range "70000-80000"`, false},
		{"port not a number", `{"protocol": "vless", "tag": "t", "settings": {"vnext": [{"address": "v.example.com", "port": "https",
			"users": [{"id": "` + testUUID + `"}]}]}}`, "the server port is not a number", false},
		{"tls settings of the wrong type", vless(`{"security": "tls", "tlsSettings": "secret"}`),
			"streamSettings.tlsSettings has a value of the wrong type", false},
		{"a password of the wrong type is not shown", `{"protocol": "trojan", "tag": "t", "settings": {"servers": [{"address": "t.example.com",
			"port": 443, "password": 123456}]}}`, "settings.servers.0.password has a value of the wrong type", false},
		{"xhttp settings of the wrong type", vless(`{"network": "xhttp", "xhttpSettings": "secret"}`),
			"streamSettings.xhttpSettings has a value of the wrong type", false},
		{"bad host", `{"protocol": "socks", "tag": "t", "settings": {"address": "bad host", "port": 1080}}`, "not a host name", false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			p, tag, proxy, err := xrayOutbound(json.RawMessage(tt.json))
			if err == nil || !proxy {
				t.Fatalf("proxy %v, error %v, profile %s; want an error with %q", proxy, err, dump(p), tt.want)
			}
			if tag != "t" {
				t.Errorf("tag %q next to the error, want t", tag)
			}
			if !strings.Contains(err.Error(), tt.want) {
				t.Errorf("error %q, want it to contain %q", err, tt.want)
			}
			if errors.Is(err, ErrUnsupported) != tt.unsupported {
				t.Errorf("error %q: unsupported %v, want %v", err, !tt.unsupported, tt.unsupported)
			}
			for _, secret := range []string{testUUID, "secret", "123456", "c2VjcmV0"} {
				if strings.Contains(err.Error(), secret) {
					t.Errorf("error %q shows %q", err, secret)
				}
			}
		})
	}
}

// A list of whole configs, one server each, as Marzban, 3x-ui and Remnawave serve them.
const xrayPanelSubscription = `[
  {
    "remarks": "🇩🇪 Germany",
    "log": {"loglevel": "warning"},
    "dns": {"servers": ["1.1.1.1", "8.8.8.8"]},
    "inbounds": [
      {"tag": "socks", "port": 10808, "listen": "127.0.0.1", "protocol": "socks", "settings": {"auth": "noauth", "udp": true},
       "sniffing": {"enabled": true, "destOverride": ["http", "tls", "quic"]}},
      {"tag": "http", "port": 10809, "listen": "127.0.0.1", "protocol": "http", "settings": {"allowTransparent": false}}
    ],
    "outbounds": [
      {"tag": "proxy", "protocol": "vless",
       "settings": {"vnext": [{"address": "de.example.com", "port": 443, "users": [{"id": "` + testUUID + `",
         "email": "user@example.com", "security": "auto", "encryption": "none", "flow": "xtls-rprx-vision"}]}]},
       "streamSettings": {"network": "tcp", "security": "reality",
         "realitySettings": {"serverName": "www.microsoft.com", "fingerprint": "chrome", "show": false,
           "publicKey": "` + testPBK + `", "shortId": "6ba85179e30d4fc2", "spiderX": "/"}},
       "mux": {"enabled": false, "concurrency": -1}},
      {"tag": "direct", "protocol": "freedom", "settings": {}},
      {"tag": "block", "protocol": "blackhole", "settings": {"response": {"type": "http"}}}
    ],
    "routing": {"domainStrategy": "AsIs", "rules": [
      {"type": "field", "inboundTag": ["api"], "outboundTag": "api"},
      {"type": "field", "ip": ["geoip:private"], "outboundTag": "direct"},
      {"type": "field", "protocol": ["bittorrent"], "outboundTag": "block"}
    ]}
  },
  {
    "remarks": "🇳🇱 Netherlands",
    "outbounds": [
      {"tag": "proxy", "protocol": "vmess",
       "settings": {"vnext": [{"address": "nl.example.com", "port": 8443, "users": [{"id": "` + testUUID + `", "alterId": 0,
         "security": "auto"}]}]},
       "streamSettings": {"network": "ws", "security": "tls", "tlsSettings": {"serverName": "nl.example.com"},
         "wsSettings": {"path": "/vm", "headers": {"Host": "nl.example.com"}}}},
      {"tag": "direct", "protocol": "freedom"},
      {"tag": "block", "protocol": "blackhole"}
    ]
  },
  {
    "remarks": "Balancer",
    "outbounds": [
      {"tag": "a", "protocol": "trojan", "settings": {"servers": [{"address": "a.example.com", "port": 443, "password": "pw"}]},
       "streamSettings": {"security": "tls"}},
      {"tag": "b", "protocol": "trojan", "settings": {"servers": [{"address": "b.example.com", "port": 443, "password": "pw"}]},
       "streamSettings": {"security": "tls"}},
      {"tag": "direct", "protocol": "freedom"}
    ],
    "routing": {"balancers": [{"tag": "auto", "selector": ["a", "b"]}],
      "rules": [{"type": "field", "network": "tcp,udp", "balancerTag": "auto"}]}
  },
  {
    "remarks": "WireGuard",
    "outbounds": [
      {"tag": "proxy", "protocol": "wireguard", "settings": {"secretKey": "` + testWGPrivate + `", "address": ["10.0.0.2"],
       "peers": [{"publicKey": "` + testWGPublic + `", "endpoint": "198.51.100.7:51820", "keepAlive": 25}], "reserved": [1, 2, 3]}},
      {"tag": "direct", "protocol": "freedom"}
    ]
  },
  {
    "remarks": "Broken",
    "outbounds": [
      {"tag": "proxy", "protocol": "vless", "settings": {"vnext": []}},
      {"tag": "direct", "protocol": "freedom"}
    ]
  },
  {
    "remarks": "Fragmented",
    "outbounds": [
      {"tag": "proxy", "protocol": "vless",
       "settings": {"vnext": [{"address": "fr.example.com", "port": 443, "users": [{"id": "` + testUUID + `", "encryption": "none"}]}]},
       "streamSettings": {"security": "tls", "sockopt": {"dialerProxy": "fragment", "tcpKeepAliveIdle": 100}}},
      {"tag": "fragment", "protocol": "freedom", "settings": {"fragment": {"packets": "tlshello", "length": "100-200",
        "interval": "10-20"}}, "streamSettings": {"sockopt": {"tcpNoDelay": true}}}
    ]
  }
]`

func TestParseDocumentXrayPanelConfigs(t *testing.T) {
	d := mustParseDocument(t, xrayPanelSubscription)
	if d.Format != FormatXray {
		t.Fatalf("format %q", d.Format)
	}
	// 3x-ui's fragment is a dial through a freedom outbound of the same config: the server is dialled directly.
	if got, want := names(d.Profiles), []string{"🇩🇪 Germany", "🇳🇱 Netherlands", "Balancer · a", "Balancer · b", "WireGuard", "Fragmented"}; !reflect.DeepEqual(got, want) {
		t.Fatalf("names %q, want %q", got, want)
	}
	germany := Profile{Type: TypeVLESS, Name: "🇩🇪 Germany", Server: "de.example.com", ServerPort: 443, VLESS: &VLESS{
		UUID: testUUID, Flow: "xtls-rprx-vision", PacketEncoding: "xudp",
		TLS: TLS{Enabled: true, ServerName: "www.microsoft.com", UTLS: UTLS{Enabled: true, Fingerprint: "chrome"},
			Reality: Reality{Enabled: true, PublicKey: testPBK, ShortID: "6ba85179e30d4fc2", SpiderX: "/"}},
	}}
	if !reflect.DeepEqual(d.Profiles[0], germany) {
		t.Errorf("\n got %s\nwant %s", dump(d.Profiles[0]), dump(germany))
	}
	wireGuard := Profile{Type: TypeWireGuard, Name: "WireGuard", Server: "198.51.100.7", ServerPort: 51820, WireGuard: &WireGuard{
		PrivateKey: testWGPrivate, PeerPublicKey: testWGPublic, Address: []string{"10.0.0.2/32"}, Reserved: []int{1, 2, 3},
		PersistentKeepalive: 25,
	}}
	if !reflect.DeepEqual(d.Profiles[4], wireGuard) {
		t.Errorf("Xray's WireGuard\n got %s\nwant %s", dump(d.Profiles[4]), dump(wireGuard))
	}
	want := []Skipped{
		{Index: 5, Name: "Broken", Reason: "the server address is missing"},
	}
	if !reflect.DeepEqual(d.Skipped, want) {
		t.Fatalf("skipped\n got %+v\nwant %+v", d.Skipped, want)
	}
}

func TestParseDocumentXrayConfig(t *testing.T) {
	body := `{
	  "log": {"loglevel": "warning"},
	  "outbounds": [
	    {"tag": "dns-out", "protocol": "dns"},
	    {"tag": "de", "protocol": "vless", "settings": {"address": "de.example.com", "port": 443, "id": "` + testUUID + `",
	      "encryption": "none"}, "streamSettings": {"security": "tls", "tlsSettings": {"serverName": "de.example.com"}}},
	    {"tag": "ss", "protocol": "shadowsocks", "settings": {"servers": [{"address": "ss.example.com", "port": 8388,
	      "method": "rot13", "password": "pw"}]}},
	    {"tag": "no-user", "protocol": "vless", "settings": {"vnext": [{"address": "nu.example.com", "port": 443, "users": []}]}},
	    {"tag": "direct", "protocol": "freedom"}
	  ],
	  "routing": {"rules": [{"type": "field", "port": 53, "outboundTag": "dns-out"}]}
	}`
	d := mustParseDocument(t, body)
	if d.Format != FormatXray || !reflect.DeepEqual(names(d.Profiles), []string{"de"}) {
		t.Fatalf("format %q, names %q", d.Format, names(d.Profiles))
	}
	want := []Skipped{
		{Index: 3, Name: "ss", Unsupported: true, Reason: `unknown Shadowsocks method "rot13"`},
		{Index: 4, Name: "no-user", Reason: "the VLESS UUID is missing"},
	}
	if !reflect.DeepEqual(d.Skipped, want) {
		t.Fatalf("skipped\n got %+v\nwant %+v", d.Skipped, want)
	}
}

func TestParseDocumentXrayOutbounds(t *testing.T) {
	list := `[
	  {"protocol": "socks", "tag": "s1", "settings": {"servers": [{"address": "198.51.100.1", "port": 1080}]}},
	  {"protocol": "freedom", "tag": "direct"},
	  {"protocol": "http", "tag": "h1", "settings": {"address": "198.51.100.2", "port": 8080, "user": "u", "pass": "p"}}
	]`
	d := mustParseDocument(t, list)
	wantProfiles := []Profile{
		{Type: TypeSOCKS, Name: "s1", Server: "198.51.100.1", ServerPort: 1080, SOCKS: &SOCKS{Version: "5"}},
		{Type: TypeHTTP, Name: "h1", Server: "198.51.100.2", ServerPort: 8080, HTTP: &HTTP{Username: "u", Password: "p"}},
	}
	if d.Format != FormatXray || !reflect.DeepEqual(d.Profiles, wantProfiles) || len(d.Skipped) != 0 {
		t.Fatalf("format %q, profiles %q, skipped %+v", d.Format, names(d.Profiles), d.Skipped)
	}

	d = mustParseDocument(t, `{"protocol": "Freedom", "tag": "Germany"}`)
	if d.Format != FormatXray || len(d.Profiles)+len(d.Skipped) != 0 {
		t.Fatalf("a lone freedom outbound is no server: %+v", d)
	}
	d = mustParseDocument(t, `{"protocol": "trojan", "tag": "Lone", "settings": {"address": "t.example.com", "port": 443,
		"password": "pw"}, "streamSettings": {"security": "tls"}}`)
	if !reflect.DeepEqual(names(d.Profiles), []string{"Lone"}) || len(d.Skipped) != 0 {
		t.Fatalf("names %q, skipped %+v", names(d.Profiles), d.Skipped)
	}
}
