package profile

import (
	"encoding/base64"
	"encoding/json"
	"errors"
	"reflect"
	"strings"
	"testing"
)

const (
	testUUID = "b831381d-6324-4d53-ad4f-8cda48b30811"
	testPBK  = "Z84J2IelR9ch3k8VtlVhhs5ycBUlXA7wHBWcBrjqnAw"
	// A 2022-blake3-aes-256-gcm key: 32 bytes in padded base64.
	testSS2022Key = "AAECAwQFBgcICQoLDA0ODxAREhMUFRYXGBkaGxwdHh8="
)

func vmessJSONLink(json string) string {
	return "vmess://" + base64.StdEncoding.EncodeToString([]byte(json))
}

func dump(p Profile) string {
	text, _ := json.Marshal(p)
	return string(text)
}

func TestParseLink(t *testing.T) {
	tests := []struct {
		name string
		link string
		want Profile
	}{
		{
			name: "vless reality vision",
			link: "  vless://" + testUUID + "@DE1.Example.com:443?encryption=none&flow=xtls-rprx-vision&security=reality" +
				"&sni=www.microsoft.com&fp=chrome&pbk=" + testPBK + "%3D&sid=6ba85179e30d4fc2&spx=%2F&type=tcp&headerType=none" +
				"#%F0%9F%87%A9%F0%9F%87%AA%20Germany\r\n",
			want: Profile{Type: TypeVLESS, Name: "🇩🇪 Germany", Server: "de1.example.com", ServerPort: 443, VLESS: &VLESS{
				UUID: testUUID, Flow: "xtls-rprx-vision", PacketEncoding: "xudp",
				TLS: TLS{Enabled: true, ServerName: "www.microsoft.com", UTLS: UTLS{Enabled: true, Fingerprint: "chrome"},
					Reality: Reality{Enabled: true, PublicKey: testPBK, ShortID: "6ba85179e30d4fc2", SpiderX: "/"}},
			}},
		},
		{
			name: "vless ws tls with early data in the path drops vision",
			link: "vless://" + testUUID + "@ws.example.com:8443?encryption=none&security=tls&sni=ws.example.com" +
				"&alpn=h2%2C%20http%2F1.1%2C&fp=Firefox&allowInsecure=0&type=ws&host=cdn.example.com&path=%2Fray%3Fed%3D2048" +
				"&flow=xtls-rprx-vision-udp443#WS TLS",
			want: Profile{Type: TypeVLESS, Name: "WS TLS", Server: "ws.example.com", ServerPort: 8443, VLESS: &VLESS{
				UUID: testUUID, PacketEncoding: "xudp",
				TLS: TLS{Enabled: true, ServerName: "ws.example.com", ALPN: []string{"h2", "http/1.1"},
					UTLS: UTLS{Enabled: true, Fingerprint: "firefox"}},
				Transport: Transport{Type: TransportWebSocket, Host: "cdn.example.com", Path: "/ray",
					MaxEarlyData: 2048, EarlyDataHeaderName: "Sec-WebSocket-Protocol"},
			}},
		},
		{
			name: "vless grpc",
			link: "vless://" + testUUID + "@grpc.example.com:443?type=grpc&serviceName=gun%2Fsvc&mode=multi&security=tls" +
				"&sni=grpc.example.com&packetEncoding=packetaddr#gRPC",
			want: Profile{Type: TypeVLESS, Name: "gRPC", Server: "grpc.example.com", ServerPort: 443, VLESS: &VLESS{
				UUID: testUUID, PacketEncoding: "packetaddr",
				TLS:       TLS{Enabled: true, ServerName: "grpc.example.com"},
				Transport: Transport{Type: TransportGRPC, ServiceName: "gun/svc"},
			}},
		},
		{
			name: "vless httpupgrade without tls or port",
			link: "vless://" + testUUID + "@hu.example.com?type=httpupgrade&host=hu.example.com&path=%2Fup%3Fed%3D1024" +
				"&flow=xtls-rprx-vision&headers=User-Agent%7Ccurl",
			want: Profile{Type: TypeVLESS, Server: "hu.example.com", ServerPort: 443, VLESS: &VLESS{
				UUID: testUUID, PacketEncoding: "xudp",
				Transport: Transport{Type: TransportHTTPUpgrade, Host: "hu.example.com", Path: "/up",
					Headers: map[string]string{"User-Agent": "curl"}},
			}},
		},
		{
			name: "vless xhttp reality with mode and extra",
			link: "vless://" + testUUID + "@x.example.com:443?encryption=none&security=reality&sni=x.example.com&fp=chrome" +
				"&pbk=" + testPBK + "&sid=ab&type=xhttp&host=x.example.com&path=%2Fxh&mode=stream-one" +
				"&extra=%7B%22xPaddingBytes%22%3A%20%22100-1000%22%2C%22noGRPCHeader%22%3Atrue%7D#XHTTP",
			want: Profile{Type: TypeVLESS, Name: "XHTTP", Server: "x.example.com", ServerPort: 443, VLESS: &VLESS{
				UUID: testUUID, PacketEncoding: "xudp",
				TLS: TLS{Enabled: true, ServerName: "x.example.com", UTLS: UTLS{Enabled: true, Fingerprint: "chrome"},
					Reality: Reality{Enabled: true, PublicKey: testPBK, ShortID: "ab"}},
				Transport: Transport{Type: TransportXHTTP, Host: "x.example.com", Path: "/xh", XHTTP: XHTTP{
					Mode: "stream-one", Extra: json.RawMessage(`{"xPaddingBytes":"100-1000","noGRPCHeader":true}`)}},
			}},
		},
		{
			name: "vless xhttp with a form-encoded python extra keeps vision under encryption",
			link: "vless://" + testUUID + "@x2.example.com:2053?encryption=mlkem768x25519plus.native.0rtt.abc" +
				"&flow=xtls-rprx-vision&type=splithttp&path=%2F&mode=bogus" +
				"&extra=%7B%27xmux%27%3A+%7B%27maxConcurrency%27%3A+%2716-32%27%7D%2C+%27noSSEHeader%27%3A+True%7D",
			want: Profile{Type: TypeVLESS, Server: "x2.example.com", ServerPort: 2053, VLESS: &VLESS{
				UUID: testUUID, Flow: "xtls-rprx-vision", Encryption: "mlkem768x25519plus.native.0rtt.abc",
				PacketEncoding: "xudp",
				Transport: Transport{Type: TransportXHTTP, Path: "/", XHTTP: XHTTP{
					Mode: "auto", Extra: json.RawMessage(`{"xmux":{"maxConcurrency":"16-32"},"noSSEHeader":true}`)}},
			}},
		},
		{
			name: "vless ipv6 raw http header with mux",
			link: "vless://" + testUUID + "@[2001:DB8:0::1]:80?type=raw&headerType=http&host=a.example.com%2Cb.example.com" +
				"&path=%2F%2C%2Fvideo&packetEncoding=none&mux=true&mux_protocol=h2mux&mux_max_streams=8&mux_padding=true" +
				"&mux_min_streams=-1&flow=bogus#v6",
			want: Profile{Type: TypeVLESS, Name: "v6", Server: "2001:db8::1", ServerPort: 80, VLESS: &VLESS{
				UUID:      testUUID,
				Transport: Transport{HeaderType: "http", Host: "a.example.com,b.example.com", Path: "/,/video"},
				Multiplex: Multiplex{Enabled: true, Protocol: "h2mux", MaxStreams: 8, Padding: true},
			}},
		},
		{
			name: "vless h2 with ech, pins and tls versions",
			link: "vless://" + testUUID + "@h2.example.com:443?security=tls&sni=h2.example.com&type=h2&host=h2.example.com" +
				"&path=%2Fh2&method=PUT&ech=AEX%2BDQBBpQAgACB&tls_min_version=1.2&tls_max_version=1.4" +
				"&tls_certificate_public_key_sha256=abc%3D%2C%20def%3D&disable_sni=1",
			want: Profile{Type: TypeVLESS, Server: "h2.example.com", ServerPort: 443, VLESS: &VLESS{
				UUID: testUUID, PacketEncoding: "xudp",
				TLS: TLS{Enabled: true, ServerName: "h2.example.com", DisableSNI: true, MinVersion: "1.2",
					CertificatePublicKeySHA256: []string{"abc=", "def="}, ECH: ECH{Enabled: true, Config: []string{
						echPEMBegin, "AEX+DQBBpQAgACB", echPEMEnd}}},
				Transport: Transport{Type: TransportHTTP, Host: "h2.example.com", Path: "/h2", Method: "PUT"},
			}},
		},
		{
			name: "vless with an unknown security has no tls",
			link: "VLESS://" + testUUID + "@v.example.com:443?security=xtls&security=tls&pbk=" + testPBK + "&fp=chrome",
			want: Profile{Type: TypeVLESS, Server: "v.example.com", ServerPort: 443, VLESS: &VLESS{
				UUID: testUUID, PacketEncoding: "xudp",
			}},
		},
		{
			name: "vmess v2rayn json with ws and tls",
			link: vmessJSONLink(`{"v":"2","ps":"🇳🇱 VMess WS","add":"vm.example.com","port":"443","id":"` + testUUID +
				`","aid":"0","scy":"auto","net":"ws","type":"none","host":"cdn.example.com","path":"/vmess?ed=2048",` +
				`"tls":"tls","sni":"vm.example.com","alpn":"h2,http/1.1","fp":"chrome","allowInsecure":"1"}`),
			want: Profile{Type: TypeVMess, Name: "🇳🇱 VMess WS", Server: "vm.example.com", ServerPort: 443, VMess: &VMess{
				UUID: testUUID, Security: "auto", PacketEncoding: "xudp",
				TLS: TLS{Enabled: true, ServerName: "vm.example.com", Insecure: true, ALPN: []string{"h2", "http/1.1"},
					UTLS: UTLS{Enabled: true, Fingerprint: "chrome"}},
				Transport: Transport{Type: TransportWebSocket, Host: "cdn.example.com", Path: "/vmess",
					MaxEarlyData: 2048, EarlyDataHeaderName: "Sec-WebSocket-Protocol"},
			}},
		},
		{
			name: "vmess json with numbers, url-safe base64, grpc and throneExtra",
			link: "vmess://" + base64.RawURLEncoding.EncodeToString([]byte(`{"v":2,"ps":"","add":"203.0.113.20","port":8080,`+
				`"id":"`+testUUID+`","aid":64,"scy":"CHACHA20-POLY1305","net":"grpc","path":"svc","tls":"",`+
				`"sni":"ignored.example.com","throneExtra":"mux=true&mux_protocol=smux&packetEncoding=none&security=tls"}`)) +
				"#From%20fragment",
			want: Profile{Type: TypeVMess, Name: "From fragment", Server: "203.0.113.20", ServerPort: 8080, VMess: &VMess{
				UUID: testUUID, AlterID: 64, Security: "chacha20-poly1305",
				Transport: Transport{Type: TransportGRPC, ServiceName: "svc"},
				Multiplex: Multiplex{Enabled: true, Protocol: "smux"},
			}},
		},
		{
			name: "vmess json with the raw http header",
			link: vmessJSONLink(`{"add":"H.example.com","port":80,"id":"` + testUUID + `","net":"tcp","type":"http",` +
				`"host":"a.example.com","path":"/a,/b","tls":"none","scy":"aes-128-cfb"}`),
			want: Profile{Type: TypeVMess, Server: "h.example.com", ServerPort: 80, VMess: &VMess{
				UUID: testUUID, Security: "auto", PacketEncoding: "xudp",
				Transport: Transport{HeaderType: "http", Host: "a.example.com", Path: "/a,/b"},
			}},
		},
		{
			name: "vmess url form",
			link: "vmess://" + testUUID + "@vm2.example.com?encryption=aes-128-gcm&type=grpc&serviceName=svc&security=tls" +
				"&sni=vm2.example.com&alterId=2#URL%20form",
			want: Profile{Type: TypeVMess, Name: "URL form", Server: "vm2.example.com", ServerPort: 443, VMess: &VMess{
				UUID: testUUID, AlterID: 2, Security: "aes-128-gcm", PacketEncoding: "xudp",
				TLS:       TLS{Enabled: true, ServerName: "vm2.example.com"},
				Transport: Transport{Type: TransportGRPC, ServiceName: "svc"},
			}},
		},
		{
			name: "trojan grpc with an ech query name",
			link: "trojan://p%40ss:w0rd@tr.example.com:443?security=tls&sni=tr.example.com&type=grpc&serviceName=trojan-grpc" +
				"&allowInsecure=1&ech=cloudflare-ech.com%2Bhttps%3A%2F%2F1.1.1.1%2Fdns-query#Trojan%20gRPC",
			want: Profile{Type: TypeTrojan, Name: "Trojan gRPC", Server: "tr.example.com", ServerPort: 443, Trojan: &Trojan{
				Password: "p@ss:w0rd",
				TLS: TLS{Enabled: true, ServerName: "tr.example.com", Insecure: true,
					ECH: ECH{Enabled: true, QueryServerName: "cloudflare-ech.com"}},
				Transport: Transport{Type: TransportGRPC, ServiceName: "trojan-grpc"},
			}},
		},
		{
			name: "trojan without security means tls",
			link: "trojan://secret@t.example.com",
			want: Profile{Type: TypeTrojan, Server: "t.example.com", ServerPort: 443, Trojan: &Trojan{
				Password: "secret", TLS: TLS{Enabled: true},
			}},
		},
		{
			name: "trojan with security none",
			link: "trojan://secret@t.example.com:8080?security=none&sni=t.example.com&mux=true",
			want: Profile{Type: TypeTrojan, Server: "t.example.com", ServerPort: 8080, Trojan: &Trojan{
				Password: "secret", Multiplex: Multiplex{Enabled: true},
			}},
		},
		{
			name: "shadowsocks sip002 with base64 userinfo",
			link: "ss://YWVzLTI1Ni1nY206dGVzdA@192.0.2.10:8388#SS%20one",
			want: Profile{Type: TypeShadowsocks, Name: "SS one", Server: "192.0.2.10", ServerPort: 8388, Shadowsocks: &Shadowsocks{
				Method: "aes-256-gcm", Password: "test",
			}},
		},
		{
			name: "shadowsocks sip002 with a plain 2022 userinfo",
			link: "ss://2022-blake3-aes-256-gcm:" + strings.ReplaceAll(testSS2022Key, "=", "%3D") + "@ss.example.com:443?uot=1#2022",
			want: Profile{Type: TypeShadowsocks, Name: "2022", Server: "ss.example.com", ServerPort: 443, Shadowsocks: &Shadowsocks{
				Method: "2022-blake3-aes-256-gcm", Password: testSS2022Key, UDPOverTCP: true,
			}},
		},
		{
			name: "shadowsocks sip002 with an upper-case method and simple-obfs",
			link: "ss://QUVTLTEyOC1HQ006cGFzcw==@ss2.example.com:8388/?plugin=simple-obfs%3Bobfs%3Dhttp%3Bobfs-host%3Dwww.bing.com#obfs",
			want: Profile{Type: TypeShadowsocks, Name: "obfs", Server: "ss2.example.com", ServerPort: 8388, Shadowsocks: &Shadowsocks{
				Method: "aes-128-gcm", Password: "pass", Plugin: "obfs-local", PluginOpts: "obfs=http;obfs-host=www.bing.com",
			}},
		},
		{
			name: "shadowsocks legacy base64 of the whole address",
			link: "ss://Y2hhY2hhMjAtaWV0Zi1wb2x5MTMwNTpwYTpzc0AxOTguNTEuMTAwLjQ6ODM4OQ==#legacy%20form",
			want: Profile{Type: TypeShadowsocks, Name: "legacy form", Server: "198.51.100.4", ServerPort: 8389, Shadowsocks: &Shadowsocks{
				Method: "chacha20-ietf-poly1305", Password: "pa:ss",
			}},
		},
		{
			name: "shadowsocks ipv6 with v2ray-plugin and plugin-opts",
			link: "ss://Y2hhY2hhMjAtaWV0Zi1wb2x5MTMwNTpwdw@[2001:db8::5]:443?plugin=v2ray-plugin%3Bmode%3Dquic" +
				"&plugin-opts=mode%3Dwebsocket%3Btls%3Bhost%3Dcdn.example.com&mux=true#v6%20plugin",
			want: Profile{Type: TypeShadowsocks, Name: "v6 plugin", Server: "2001:db8::5", ServerPort: 443, Shadowsocks: &Shadowsocks{
				Method: "chacha20-ietf-poly1305", Password: "pw", Plugin: "v2ray-plugin",
				PluginOpts: "mode=websocket;tls;host=cdn.example.com", Multiplex: Multiplex{Enabled: true},
			}},
		},
		{
			name: "hysteria2 port range with obfs",
			link: "hysteria2://letmein@hy.example.com:20000-30000/?sni=hy.example.com&obfs=salamander&obfs-password=gawr%2Bgura" +
				"&insecure=1&upmbps=50Mbps&downmbps=200&fp=chrome&pbk=" + testPBK +
				// One pin in hex with a separator and in base64: both forms of the same hash.
				"&pinSHA256=000102030405060708090A0B0C0D0E0F%3A101112131415161718191a1b1c1d1e1f," + testSS2022Key + "#HY2%20hop",
			want: Profile{Type: TypeHysteria2, Name: "HY2 hop", Server: "hy.example.com", ServerPort: 20000, Hysteria2: &Hysteria2{
				Password: "letmein", ServerPorts: []string{"20000:30000"}, UpMbps: 50, DownMbps: 200,
				ObfsType: "salamander", ObfsPassword: "gawr+gura",
				TLS: TLS{Enabled: true, ServerName: "hy.example.com", Insecure: true, CertificateSHA256: []string{testSS2022Key}},
			}},
		},
		{
			name: "hy2 ipv6 with mport and hop interval",
			link: "hy2://user:pass@[2001:db8::2]:443?mport=443,5000-6000&hop_interval=30s&alpn=h3#v6",
			want: Profile{Type: TypeHysteria2, Name: "v6", Server: "2001:db8::2", ServerPort: 443, Hysteria2: &Hysteria2{
				Password: "user:pass", ServerPorts: []string{"443:443", "5000:6000"}, HopInterval: "30s",
				TLS: TLS{Enabled: true, ALPN: []string{"h3"}},
			}},
		},
		{
			name: "hy2 without password, port or obfs password",
			link: "hy2://hy3.example.com?obfs=salamander&upmbps=50%20MB%2Fs&downmbps=fast&hop_interval=30",
			want: Profile{Type: TypeHysteria2, Server: "hy3.example.com", ServerPort: 443, Hysteria2: &Hysteria2{
				UpMbps: 400, TLS: TLS{Enabled: true},
			}},
		},
		{
			name: "tuic",
			link: "tuic://" + testUUID + ":p%40ss@tuic.example.com:443?congestion_control=BBR&udp_relay_mode=native&alpn=h3" +
				"&sni=tuic.example.com&allow_insecure=1&zero_rtt_handshake=true&heartbeat=10s#TUIC",
			want: Profile{Type: TypeTUIC, Name: "TUIC", Server: "tuic.example.com", ServerPort: 443, TUIC: &TUIC{
				UUID: testUUID, Password: "p@ss", CongestionControl: "bbr", UDPRelayMode: "native", ZeroRTT: true,
				Heartbeat: "10s", TLS: TLS{Enabled: true, ServerName: "tuic.example.com", Insecure: true, ALPN: []string{"h3"}},
			}},
		},
		{
			name: "tuic drops unknown values",
			link: "tuic://" + testUUID + ":pw@198.51.100.30?congestion_control=New%20Reno&udp_relay_mode=bogus&heartbeat=10",
			want: Profile{Type: TypeTUIC, Server: "198.51.100.30", ServerPort: 443, TUIC: &TUIC{
				UUID: testUUID, Password: "pw", CongestionControl: "new_reno", TLS: TLS{Enabled: true},
			}},
		},
		{
			name: "socks5 with auth",
			link: "socks5://user:pass@127.0.0.1:1080#local",
			want: Profile{Type: TypeSOCKS, Name: "local", Server: "127.0.0.1", ServerPort: 1080, SOCKS: &SOCKS{
				Version: "5", Username: "user", Password: "pass",
			}},
		},
		{
			name: "socks without auth or port",
			link: "socks://198.51.100.9",
			want: Profile{Type: TypeSOCKS, Server: "198.51.100.9", ServerPort: 1080, SOCKS: &SOCKS{Version: "5"}},
		},
		{
			name: "socks with v2rayn base64 auth",
			link: "socks://dXNlcjpwYXNz@198.51.100.9:1081",
			want: Profile{Type: TypeSOCKS, Server: "198.51.100.9", ServerPort: 1081, SOCKS: &SOCKS{
				Version: "5", Username: "user", Password: "pass",
			}},
		},
		{
			name: "socks4 with a plain user id",
			link: "socks4://user1234@198.51.100.9:1082",
			want: Profile{Type: TypeSOCKS, Server: "198.51.100.9", ServerPort: 1082, SOCKS: &SOCKS{
				Version: "4", Username: "user1234",
			}},
		},
		{
			name: "socks4a",
			link: "socks4a://proxy.example.com:1080",
			want: Profile{Type: TypeSOCKS, Server: "proxy.example.com", ServerPort: 1080, SOCKS: &SOCKS{Version: "4a"}},
		},
		{
			name: "socks version from the query",
			link: "socks4://proxy.example.com?version=6",
			want: Profile{Type: TypeSOCKS, Server: "proxy.example.com", ServerPort: 1080, SOCKS: &SOCKS{Version: "5"}},
		},
		{
			name: "http with auth",
			link: "http://user:p%40ss@203.0.113.5:3128#office 100%",
			want: Profile{Type: TypeHTTP, Name: "office 100%", Server: "203.0.113.5", ServerPort: 3128, HTTP: &HTTP{
				Username: "user", Password: "p@ss",
			}},
		},
		{
			name: "https with auth, path and headers",
			link: "https://alice:secret@proxy.example.com?sni=proxy.example.com&path=%2Fconnect&headers=X-Token,abc,X-Id,7",
			want: Profile{Type: TypeHTTP, Server: "proxy.example.com", ServerPort: 443, HTTP: &HTTP{
				Username: "alice", Password: "secret", Path: "/connect",
				Headers: map[string]string{"X-Token": "abc", "X-Id": "7"},
				TLS:     TLS{Enabled: true, ServerName: "proxy.example.com"},
			}},
		},
		{
			name: "http with only a token",
			link: "http://:token@203.0.113.6:8080",
			want: Profile{Type: TypeHTTP, Server: "203.0.113.6", ServerPort: 8080, HTTP: &HTTP{
				Username: "token", Password: "token",
			}},
		},
		{
			name: "http with security tls",
			link: "http://203.0.113.7?security=tls",
			want: Profile{Type: TypeHTTP, Server: "203.0.113.7", ServerPort: 443, HTTP: &HTTP{TLS: TLS{Enabled: true}}},
		},
		{
			name: "http without port",
			link: "http://203.0.113.7",
			want: Profile{Type: TypeHTTP, Server: "203.0.113.7", ServerPort: 80, HTTP: &HTTP{}},
		},
		{
			name: "anytls",
			link: "anytls://passw0rd@at.example.com:8443?sni=at.example.com&insecure=1&idle_session_timeout=60s" +
				"&min_idle_session=2&idle_session_check_interval=bad#AnyTLS",
			want: Profile{Type: TypeAnyTLS, Name: "AnyTLS", Server: "at.example.com", ServerPort: 8443, AnyTLS: &AnyTLS{
				Password: "passw0rd", IdleSessionCheckInterval: "30s", IdleSessionTimeout: "60s", MinIdleSession: 2,
				TLS: TLS{Enabled: true, ServerName: "at.example.com", Insecure: true},
			}},
		},
		{
			name: "anytls defaults keep tls on",
			link: "anytls://pw@at2.example.com?security=none",
			want: Profile{Type: TypeAnyTLS, Server: "at2.example.com", ServerPort: 443, AnyTLS: &AnyTLS{
				Password: "pw", IdleSessionCheckInterval: "30s", IdleSessionTimeout: "30s", MinIdleSession: 5,
				TLS: TLS{Enabled: true},
			}},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := ParseLink(tt.link)
			if err != nil {
				t.Fatalf("ParseLink(%q): %v", tt.link, err)
			}
			if !reflect.DeepEqual(got, tt.want) {
				t.Errorf("ParseLink(%q)\n got %s\nwant %s", tt.link, dump(got), dump(tt.want))
			}
		})
	}
}

func TestParseLinkRejects(t *testing.T) {
	tests := []struct {
		name string
		link string
		want string
	}{
		{"vless without uuid", "vless://@v.example.com:443", "UUID is missing"},
		{"port out of range", "vless://" + testUUID + "@v.example.com:99999", "not a number from 1 to 65535"},
		{"port not a number", "vless://" + testUUID + "@v.example.com:abc", "not a number from 1 to 65535"},
		{"unsupported transport", "vless://" + testUUID + "@v.example.com?type=kcp", `transport "kcp"`},
		{"reality without a key", "vless://" + testUUID + "@v.example.com?security=reality&sni=a.example.com", "pbk"},
		// sing-box panics on a short ID longer than 8 bytes.
		{"reality short id too long", "vless://" + testUUID + "@v.example.com?security=reality&pbk=" + testPBK + "&sid=0123456789abcdef01", "short ID"},
		{"unclosed ipv6", "vless://" + testUUID + "@[2001:db8::1", "closing bracket"},
		{"ipv6 without brackets", "vless://" + testUUID + "@2001:db8::1:443", "in brackets"},
		{"host with a space", "vless://" + testUUID + "@bad%20host:443", "not a host name"},
		{"trojan without password", "trojan://@t.example.com", "Trojan password is missing"},
		{"shadowsocks neither form", "ss://!!!#x", "neither"},
		{"shadowsocks userinfo not base64", "ss://!!!@ss.example.com:8388", "not valid base64"},
		{"shadowsocks userinfo without colon", "ss://YWVzLTI1Ni1nY20tbm9jb2xvbg@ss.example.com:8388", "method:password"},
		{"shadowsocks without port", "ss://YWVzLTI1Ni1nY206cHc@ss.example.com", "server port 0"},
		{"shadowsocks unknown method", "ss://cm90MTM6cHc@ss.example.com:8388", `method "rot13"`},
		{"shadowsocks bad 2022 key", "ss://2022-blake3-aes-128-gcm:short@ss.example.com:8388", "16 bytes"},
		{"shadowsocks unknown plugin", "ss://YWVzLTI1Ni1nY206cHc@ss.example.com:8388?plugin=kcptun", `plugin "kcptun"`},
		{"hysteria2 bad range", "hy2://pw@hy.example.com:20000-70000", `range "20000-70000"`},
		{"hysteria2 short pin", "hy2://pw@hy.example.com?insecure=1&pinSHA256=deadbeef", `pin "deadbeef"`},
		{"vless bad pcs", "vless://" + testUUID + "@v.example.com?security=tls&pcs=not-a-hash", `pin "not-a-hash"`},
		{"hysteria2 gecko", "hy2://pw@hy.example.com?obfs=gecko&obfs-password=x", `obfuscation "gecko"`},
		{"tuic v4 token", "tuic://token@tuic.example.com", "not a UUID"},
		{"vmess reality in json", vmessJSONLink(`{"add":"vm.example.com","port":443,"id":"` + testUUID + `","tls":"reality"}`), "only tls or none"},
		{"vmess json port 0", vmessJSONLink(`{"add":"vm.example.com","port":"0","id":"` + testUUID + `"}`), "server port 0"},
		{"vmess json without server", vmessJSONLink(`{"port":443,"id":"` + testUUID + `"}`), "server address is missing"},
		{"anytls without password", "anytls://@at.example.com", "AnyTLS password is missing"},
		// sing-box runs VMess and Trojan, and has no XHTTP.
		{"vmess xhttp", "vmess://" + testUUID + "@vm.example.com?type=xhttp&security=tls", "only with VLESS"},
		{"trojan http header over tls", "trojan://pw@t.example.com?type=raw&headerType=http&security=tls", "only with VLESS"},
		// Only Xray runs XHTTP, and it refuses allowInsecure.
		{"xhttp insecure without pin", "vless://" + testUUID + "@x.example.com?type=xhttp&security=tls&allowInsecure=1", "certificate pin"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			p, err := ParseLink(tt.link)
			if err == nil {
				t.Fatalf("ParseLink(%q) = %s, want an error with %q", tt.link, dump(p), tt.want)
			}
			if !strings.Contains(err.Error(), tt.want) {
				t.Errorf("ParseLink(%q) error %q, want it to contain %q", tt.link, err, tt.want)
			}
		})
	}
}

func TestPorosLink(t *testing.T) {
	want, err := ParseLink("trojan://secret@de.example.com:443?sni=cdn.example.com&type=ws&path=%2Fws#Germany")
	if err != nil {
		t.Fatal(err)
	}
	data, _ := json.Marshal(want)
	for _, link := range []string{
		"poros://add/" + base64.RawURLEncoding.EncodeToString(data),
		"POROS://Add/" + base64.StdEncoding.EncodeToString(data),
	} {
		if got, err := ParseLink(link); err != nil || !reflect.DeepEqual(got, want) {
			t.Errorf("ParseLink(%.30q) = %+v, %v", link, got, err)
		}
	}
	for _, link := range []string{"poros://add/%%%", "poros://add/" + base64.RawURLEncoding.EncodeToString([]byte(`{"type":"trojan"}`))} {
		if _, err := ParseLink(link); err == nil || errors.Is(err, ErrNotLink) {
			t.Errorf("ParseLink(%q) error %v, want a damaged link", link, err)
		}
	}
}

func TestParseLinkNotALink(t *testing.T) {
	for _, text := range []string{"", "just text", "foo://bar", "hysteria://h.example.com:443", "wg://key@198.51.100.7:51820",
		"poros://addsub/aHR0cHM6Ly9leGFtcGxlLmNvbQ", "poros://route/x"} {
		if _, err := ParseLink(text); !errors.Is(err, ErrNotLink) {
			t.Errorf("ParseLink(%q) error %v, want ErrNotLink", text, err)
		}
	}
}

func TestValidateParts(t *testing.T) {
	valid := Profile{Type: TypeTrojan, Server: "t.example.com", ServerPort: 443, Trojan: &Trojan{Password: "pw"}}
	if err := valid.Validate(); err != nil {
		t.Fatalf("a complete Trojan profile must pass: %v", err)
	}
	tests := []struct {
		name string
		p    Profile
		want string
	}{
		{"unknown type", Profile{Type: "wireguard", Server: "w.example.com", ServerPort: 51820}, `unknown server type "wireguard"`},
		{"missing part", Profile{Type: TypeVLESS, Server: "v.example.com", ServerPort: 443}, "vless settings are missing"},
		{"second part", Profile{Type: TypeTrojan, Server: "t.example.com", ServerPort: 443, Trojan: &Trojan{Password: "pw"},
			VLESS: &VLESS{UUID: testUUID}}, "carries vless settings"},
		{"blank server", Profile{Type: TypeSOCKS, Server: " ", ServerPort: 1080, SOCKS: &SOCKS{Version: "5"}}, "server address is missing"},
		{"hysteria2 range with a dash", Profile{Type: TypeHysteria2, Server: "h.example.com", ServerPort: 20000,
			Hysteria2: &Hysteria2{ServerPorts: []string{"20000-30000"}}}, "20000:30000"},
		{"chacha 2022 with two keys", Profile{Type: TypeShadowsocks, Server: "s.example.com", ServerPort: 8388,
			Shadowsocks: &Shadowsocks{Method: "2022-blake3-chacha20-poly1305", Password: testSS2022Key + ":" + testSS2022Key}},
			"single key"},
		{"shadowsocks without password", Profile{Type: TypeShadowsocks, Server: "s.example.com", ServerPort: 8388,
			Shadowsocks: &Shadowsocks{Method: "aes-256-gcm"}}, "password is missing"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := tt.p.Validate()
			if err == nil || !strings.Contains(err.Error(), tt.want) {
				t.Errorf("Validate() = %v, want an error with %q", err, tt.want)
			}
		})
	}
}
