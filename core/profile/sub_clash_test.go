package profile

import (
	"encoding/base64"
	"encoding/json"
	"fmt"
	"reflect"
	"strings"
	"testing"
)

func TestClashProxies(t *testing.T) {
	tests := []struct {
		name  string
		proxy string
		want  Profile
	}{
		{
			name: "vless reality vision",
			proxy: `
  - name: "🇩🇪 Germany"
    type: vless
    server: DE1.Example.com
    port: 443
    uuid: b831381d-6324-4d53-ad4f-8cda48b30811
    network: tcp
    udp: true
    tls: true
    flow: xtls-rprx-vision
    servername: www.microsoft.com
    client-fingerprint: chrome
    reality-opts:
      public-key: Z84J2IelR9ch3k8VtlVhhs5ycBUlXA7wHBWcBrjqnAw=
      short-id: 6ba85179e30d4fc2
`,
			want: Profile{Type: TypeVLESS, Name: "🇩🇪 Germany", Server: "de1.example.com", ServerPort: 443, VLESS: &VLESS{
				UUID: testUUID, Flow: "xtls-rprx-vision",
				TLS: TLS{Enabled: true, ServerName: "www.microsoft.com", UTLS: UTLS{Enabled: true, Fingerprint: "chrome"},
					Reality: Reality{Enabled: true, PublicKey: testPBK, ShortID: "6ba85179e30d4fc2"}},
			}},
		},
		{
			name: "vless ws tls with a host header and early data in the path drops vision",
			proxy: `
  - name: VLESS WS
    type: vless
    server: 203.0.113.10
    port: 8443
    uuid: b831381d-6324-4d53-ad4f-8cda48b30811
    tls: true
    servername: ws.example.com
    skip-cert-verify: true
    alpn: [h2, "http/1.1"]
    client-fingerprint: Firefox
    flow: xtls-rprx-vision
    packet-encoding: xudp
    network: ws
    ws-opts:
      path: /ray?ed=2048
      headers:
        Host: cdn.example.com
        User-Agent: Mozilla/5.0
`,
			want: Profile{Type: TypeVLESS, Name: "VLESS WS", Server: "203.0.113.10", ServerPort: 8443, VLESS: &VLESS{
				UUID: testUUID, PacketEncoding: "xudp",
				TLS: TLS{Enabled: true, ServerName: "ws.example.com", Insecure: true, ALPN: []string{"h2", "http/1.1"},
					UTLS: UTLS{Enabled: true, Fingerprint: "firefox"}},
				Transport: Transport{Type: TransportWebSocket, Host: "cdn.example.com", Path: "/ray",
					Headers: map[string]string{"User-Agent": "Mozilla/5.0"}, MaxEarlyData: 2048, EarlyDataHeaderName: "Sec-WebSocket-Protocol"},
			}},
		},
		{
			name:  "vless grpc as a flow mapping",
			proxy: "  - {name: gRPC, type: vless, server: grpc.example.com, port: 443, uuid: b831381d-6324-4d53-ad4f-8cda48b30811, tls: true, sni: grpc.example.com, network: grpc, grpc-opts: {grpc-service-name: gun/svc}, packet-encoding: packetaddr}\n",
			want: Profile{Type: TypeVLESS, Name: "gRPC", Server: "grpc.example.com", ServerPort: 443, VLESS: &VLESS{
				UUID: testUUID, PacketEncoding: "packetaddr",
				TLS:       TLS{Enabled: true, ServerName: "grpc.example.com"},
				Transport: Transport{Type: TransportGRPC, ServiceName: "gun/svc"},
			}},
		},
		{
			name: "vless xhttp reality with encryption, padding, xmux and a download leg",
			proxy: `
  - name: XHTTP
    type: vless
    server: x.example.com
    port: 443
    uuid: b831381d-6324-4d53-ad4f-8cda48b30811
    encryption: mlkem768x25519plus.native.0rtt.abc
    flow: xtls-rprx-vision
    tls: true
    servername: x.example.com
    client-fingerprint: chrome
    reality-opts:
      public-key: Z84J2IelR9ch3k8VtlVhhs5ycBUlXA7wHBWcBrjqnAw
      short-id: ab
    network: xhttp
    xhttp-opts:
      host: x.example.com
      path: /xh
      mode: packet-up
      x-padding-obfs-mode: true
      x-padding-key: x_padding
      x-padding-placement: cookie
      x-padding-method: bogus
      sc-min-posts-interval-ms: 10-50
      reuse-settings:
        max-concurrency: 16-32
        max-connections: "0"
        c-max-reuse-times: 1-x
        h-max-reusable-secs: 1800-3000
        h-keep-alive-period: 15
      download-settings:
        server: dl.example.com
        port: 8443
        path: /down
        mode: stream-down
        x-padding-key: dl_padding
        reuse-settings:
          max-concurrency: 4
`,
			want: Profile{Type: TypeVLESS, Name: "XHTTP", Server: "x.example.com", ServerPort: 443, VLESS: &VLESS{
				UUID: testUUID, Flow: "xtls-rprx-vision", Encryption: "mlkem768x25519plus.native.0rtt.abc",
				TLS: TLS{Enabled: true, ServerName: "x.example.com", UTLS: UTLS{Enabled: true, Fingerprint: "chrome"},
					Reality: Reality{Enabled: true, PublicKey: testPBK, ShortID: "ab"}},
				Transport: Transport{Type: TransportXHTTP, Host: "x.example.com", Path: "/xh", XHTTP: XHTTP{Mode: "packet-up",
					Extra: json.RawMessage(`{"downloadSettings":{"address":"dl.example.com","network":"xhttp","port":8443,` +
						`"realitySettings":{"fingerprint":"chrome","publicKey":"` + testPBK + `","serverName":"x.example.com",` +
						`"shortId":"ab","show":false},"security":"reality","xhttpSettings":{"extra":{"xPaddingKey":"dl_padding",` +
						`"xmux":{"maxConcurrency":"4"}},"host":"x.example.com","mode":"auto","path":"/down"}},` +
						`"scMinPostsIntervalMs":"10-50","xPaddingKey":"x_padding","xPaddingObfsMode":true,"xPaddingPlacement":"cookie",` +
						`"xmux":{"hKeepAlivePeriod":15,"hMaxReusableSecs":"1800-3000","maxConcurrency":"16-32","maxConnections":"0"}}`)}},
			}},
		},
		{
			name: "vless xhttp tls with a download leg of its own",
			proxy: `
  - name: XHTTP TLS
    type: vless
    server: x2.example.com
    port: 443
    uuid: b831381d-6324-4d53-ad4f-8cda48b30811
    tls: true
    servername: x2.example.com
    alpn: [h2]
    network: xhttp
    xhttp-opts:
      path: /up
      download-settings:
        server: 2001:DB8::9
        servername: dl.example.com
        client-fingerprint: none
        fingerprint: 00:01:02:03:04:05:06:07:08:09:0A:0B:0C:0D:0E:0F:10:11:12:13:14:15:16:17:18:19:1a:1b:1c:1d:1e:1f
        host: dl.example.com
`,
			want: Profile{Type: TypeVLESS, Name: "XHTTP TLS", Server: "x2.example.com", ServerPort: 443, VLESS: &VLESS{
				UUID: testUUID,
				TLS:  TLS{Enabled: true, ServerName: "x2.example.com", ALPN: []string{"h2"}},
				Transport: Transport{Type: TransportXHTTP, Path: "/up", XHTTP: XHTTP{Mode: "auto",
					Extra: json.RawMessage(`{"downloadSettings":{"address":"2001:db8::9","network":"xhttp","port":443,` +
						`"security":"tls","tlsSettings":{"alpn":["h2"],"fingerprint":"unsafe",` +
						`"pinnedPeerCertSha256":"000102030405060708090a0b0c0d0e0f101112131415161718191a1b1c1d1e1f",` +
						`"serverName":"dl.example.com"},"xhttpSettings":{"host":"dl.example.com","mode":"auto","path":"/up"}}}`)}},
			}},
		},
		{
			name:  "vless xhttp stream-one leaves the download leg out",
			proxy: "  - {name: One, type: vless, server: one.example.com, port: 443, uuid: b831381d-6324-4d53-ad4f-8cda48b30811, tls: true, network: xhttp, xhttp-opts: {mode: stream-one, path: /one, download-settings: {server: dl.example.com}}}\n",
			want: Profile{Type: TypeVLESS, Name: "One", Server: "one.example.com", ServerPort: 443, VLESS: &VLESS{
				UUID: testUUID, TLS: TLS{Enabled: true},
				Transport: Transport{Type: TransportXHTTP, Path: "/one", XHTTP: XHTTP{Mode: "stream-one"}},
			}},
		},
		{
			name: "vless httpupgrade without tls",
			proxy: `
  - name: HU
    type: vless
    server: hu.example.com
    port: 80
    uuid: b831381d-6324-4d53-ad4f-8cda48b30811
    network: ws
    ws-opts:
      path: "/up?ed=1024"
      v2ray-http-upgrade: true
      max-early-data: 2048
      headers: {host: hu-cdn.example.com}
`,
			want: Profile{Type: TypeVLESS, Name: "HU", Server: "hu.example.com", ServerPort: 80, VLESS: &VLESS{
				UUID:      testUUID,
				Transport: Transport{Type: TransportHTTPUpgrade, Host: "hu-cdn.example.com", Path: "/up"},
			}},
		},
		{
			name: "vless h2 with ech, a pin and smux",
			proxy: `
  - name: H2
    type: vless
    server: "[2001:DB8::1]"
    port: 443
    uuid: b831381d-6324-4d53-ad4f-8cda48b30811
    tls: true
    servername: h2.example.com
    fingerprint: 00:01:02:03:04:05:06:07:08:09:0a:0b:0c:0d:0e:0f:10:11:12:13:14:15:16:17:18:19:1a:1b:1c:1d:1e:1f
    ech-opts:
      enable: true
      config: AEX+DQBBpQAgACB
    network: h2
    h2-opts:
      host: [h2.example.com, h2b.example.com]
      path: /h2
    smux:
      enabled: true
      protocol: h2mux
      max-streams: 8
      min-streams: 4
      padding: true
`,
			want: Profile{Type: TypeVLESS, Name: "H2", Server: "2001:db8::1", ServerPort: 443, VLESS: &VLESS{
				UUID: testUUID,
				TLS: TLS{Enabled: true, ServerName: "h2.example.com", CertificateSHA256: []string{testSS2022Key},
					ECH: ECH{Enabled: true, Config: []string{echPEMBegin, "AEX+DQBBpQAgACB", echPEMEnd}}},
				Transport: Transport{Type: TransportHTTP, Host: "h2.example.com,h2b.example.com", Path: "/h2"},
				Multiplex: Multiplex{Enabled: true, Protocol: "h2mux", MaxStreams: 8, Padding: true},
			}},
		},
		{
			name: "vmess ws tls with numbers as strings and the server name as host",
			proxy: `
  - name: "🇳🇱 VMess WS"
    type: vmess
    server: vm.example.com
    port: "443"
    uuid: b831381d-6324-4d53-ad4f-8cda48b30811
    alterId: "0"
    cipher: AUTO
    tls: "true"
    servername: vm.example.com
    network: ws
    ws-opts:
      path: /vmess
      max-early-data: 2048
      early-data-header-name: Sec-WebSocket-Protocol
`,
			want: Profile{Type: TypeVMess, Name: "🇳🇱 VMess WS", Server: "vm.example.com", ServerPort: 443, VMess: &VMess{
				UUID: testUUID, Security: "auto",
				TLS: TLS{Enabled: true, ServerName: "vm.example.com"},
				Transport: Transport{Type: TransportWebSocket, Host: "vm.example.com", Path: "/vmess",
					MaxEarlyData: 2048, EarlyDataHeaderName: "Sec-WebSocket-Protocol"},
			}},
		},
		{
			name: "vmess http header obfuscation without tls",
			proxy: `
  - name: HTTP obfs
    type: vmess
    server: 198.51.100.20
    port: 80
    uuid: b831381d-6324-4d53-ad4f-8cda48b30811
    alterId: 64
    cipher: chacha20-poly1305
    tls: false
    servername: ignored.example.com
    network: http
    http-opts:
      method: GET
      path: [/, /video]
      headers:
        Host: [a.example.com, b.example.com]
        Connection: [keep-alive]
`,
			want: Profile{Type: TypeVMess, Name: "HTTP obfs", Server: "198.51.100.20", ServerPort: 80, VMess: &VMess{
				UUID: testUUID, AlterID: 64, Security: "chacha20-poly1305",
				Transport: Transport{HeaderType: "http", Host: "a.example.com,b.example.com", Path: "/,/video", Method: "GET"},
			}},
		},
		{
			name: "vmess with yamux and an unknown cipher",
			proxy: `
  - name: Mux
    type: vmess
    server: mux.example.com
    port: 443
    uuid: b831381d-6324-4d53-ad4f-8cda48b30811
    cipher: aes-128-cfb
    packet-encoding: none
    smux:
      enabled: true
      protocol: yamux
      max-connections: 4
      min-streams: -1
`,
			want: Profile{Type: TypeVMess, Name: "Mux", Server: "mux.example.com", ServerPort: 443, VMess: &VMess{
				UUID: testUUID, Security: "auto", Multiplex: Multiplex{Enabled: true, Protocol: "yamux", MaxConnections: 4},
			}},
		},
		{
			name: "trojan grpc with no utls and one alpn",
			proxy: `
  - name: Trojan gRPC
    type: trojan
    server: tr.example.com
    port: 443
    password: "p@ss:w0rd"
    sni: tr.example.com
    skip-cert-verify: 1
    client-fingerprint: none
    alpn: h2
    network: grpc
    grpc-opts:
      grpc-service-name: trojan-grpc
    smux: {enabled: false, protocol: smux}
`,
			want: Profile{Type: TypeTrojan, Name: "Trojan gRPC", Server: "tr.example.com", ServerPort: 443, Trojan: &Trojan{
				Password:  "p@ss:w0rd",
				TLS:       TLS{Enabled: true, ServerName: "tr.example.com", Insecure: true, ALPN: []string{"h2"}},
				Transport: Transport{Type: TransportGRPC, ServiceName: "trojan-grpc"},
			}},
		},
		{
			name:  "trojan ws takes the sni as host",
			proxy: "  - {name: Trojan WS, type: trojan, server: 192.0.2.5, port: 443, password: secret, sni: t.example.com, network: ws, ws-opts: {path: /tr}}\n",
			want: Profile{Type: TypeTrojan, Name: "Trojan WS", Server: "192.0.2.5", ServerPort: 443, Trojan: &Trojan{
				Password:  "secret",
				TLS:       TLS{Enabled: true, ServerName: "t.example.com"},
				Transport: Transport{Type: TransportWebSocket, Host: "t.example.com", Path: "/tr"},
			}},
		},
		{
			name: "shadowsocks keeps a password that looks like a number",
			proxy: `
  - name: SS one
    type: ss
    server: 192.0.2.10
    port: 8388
    cipher: AES-256-GCM
    password: 0123
    udp: true
    udp-over-tcp: true
`,
			want: Profile{Type: TypeShadowsocks, Name: "SS one", Server: "192.0.2.10", ServerPort: 8388, Shadowsocks: &Shadowsocks{
				Method: "aes-256-gcm", Password: "0123", UDPOverTCP: true,
			}},
		},
		{
			name: "shadowsocks 2022 with smux",
			proxy: `
  - name: 2022
    type: ss
    server: ss.example.com
    port: 443
    cipher: 2022-blake3-aes-256-gcm
    password: "AAECAwQFBgcICQoLDA0ODxAREhMUFRYXGBkaGxwdHh8="
    smux:
      enabled: true
      protocol: smux
      max-connections: 4
      min-streams: 4
`,
			want: Profile{Type: TypeShadowsocks, Name: "2022", Server: "ss.example.com", ServerPort: 443, Shadowsocks: &Shadowsocks{
				Method: "2022-blake3-aes-256-gcm", Password: testSS2022Key,
				Multiplex: Multiplex{Enabled: true, Protocol: "smux", MaxConnections: 4, MinStreams: 4},
			}},
		},
		{
			name: "shadowsocks obfs",
			proxy: `
  - name: obfs
    type: ss
    server: ss2.example.com
    port: 8388
    cipher: aes-128-gcm
    password: pass
    plugin: obfs
    plugin-opts:
      mode: tls
      host: www.bing.com
`,
			want: Profile{Type: TypeShadowsocks, Name: "obfs", Server: "ss2.example.com", ServerPort: 8388, Shadowsocks: &Shadowsocks{
				Method: "aes-128-gcm", Password: "pass", Plugin: "obfs-local", PluginOpts: "obfs=tls;obfs-host=www.bing.com",
			}},
		},
		{
			name: "shadowsocks v2ray-plugin on ipv6",
			proxy: `
  - name: v2ray-plugin
    type: ss
    server: 2001:db8::5
    port: 443
    cipher: chacha20-ietf-poly1305
    password: pw
    plugin: v2ray-plugin
    plugin-opts:
      mode: websocket
      tls: true
      host: cdn.example.com
      path: /ws;x
      mux: true
      skip-cert-verify: true
`,
			want: Profile{Type: TypeShadowsocks, Name: "v2ray-plugin", Server: "2001:db8::5", ServerPort: 443, Shadowsocks: &Shadowsocks{
				Method: "chacha20-ietf-poly1305", Password: "pw", Plugin: "v2ray-plugin",
				PluginOpts: `tls;host=cdn.example.com;path=/ws\;x;mode=websocket;mux`,
			}},
		},
		{
			name: "hysteria2 with port hopping and obfs",
			proxy: `
  - name: HY2 hop
    type: hysteria2
    server: hy.example.com
    ports: 20000-30000/40000
    password: letmein
    up: 50 Mbps
    down: 200
    obfs: salamander
    obfs-password: "gawr+gura"
    sni: hy.example.com
    skip-cert-verify: true
    alpn: [h3]
    client-fingerprint: chrome
    fingerprint: AAECAwQFBgcICQoLDA0ODxAREhMUFRYXGBkaGxwdHh8=
    hop-interval: 30
`,
			want: Profile{Type: TypeHysteria2, Name: "HY2 hop", Server: "hy.example.com", ServerPort: 20000, Hysteria2: &Hysteria2{
				Password: "letmein", ServerPorts: []string{"20000:30000", "40000:40000"}, HopInterval: "30s",
				UpMbps: 50, DownMbps: 200, ObfsType: "salamander", ObfsPassword: "gawr+gura",
				TLS: TLS{Enabled: true, ServerName: "hy.example.com", Insecure: true, ALPN: []string{"h3"},
					CertificateSHA256: []string{testSS2022Key}},
			}},
		},
		{
			name:  "hysteria2 plain",
			proxy: "  - {name: HY2, type: hysteria2, server: hy2.example.com, port: 443, password: pw, hop-interval: 15s}\n",
			want: Profile{Type: TypeHysteria2, Name: "HY2", Server: "hy2.example.com", ServerPort: 443, Hysteria2: &Hysteria2{
				Password: "pw", HopInterval: "15s", TLS: TLS{Enabled: true},
			}},
		},
		{
			name: "tuic dialing ip keeps the server as sni",
			proxy: `
  - name: TUIC
    type: tuic
    server: tuic.example.com
    ip: 198.51.100.30
    port: 443
    uuid: b831381d-6324-4d53-ad4f-8cda48b30811
    password: "p@ss"
    congestion-controller: BBR
    udp-relay-mode: native
    reduce-rtt: true
    heartbeat-interval: 10000
    alpn: [h3]
    request-timeout: 8000
`,
			want: Profile{Type: TypeTUIC, Name: "TUIC", Server: "198.51.100.30", ServerPort: 443, TUIC: &TUIC{
				UUID: testUUID, Password: "p@ss", CongestionControl: "bbr", UDPRelayMode: "native", ZeroRTT: true,
				Heartbeat: "10000ms", TLS: TLS{Enabled: true, ServerName: "tuic.example.com", ALPN: []string{"h3"}},
			}},
		},
		{
			name:  "socks5",
			proxy: "  - {name: local, type: socks5, server: 127.0.0.1, port: 1080, username: user, password: pass, udp: true}\n",
			want: Profile{Type: TypeSOCKS, Name: "local", Server: "127.0.0.1", ServerPort: 1080, SOCKS: &SOCKS{
				Version: "5", Username: "user", Password: "pass",
			}},
		},
		{
			name: "http over tls with headers",
			proxy: `
  - name: office
    type: http
    server: proxy.example.com
    port: 443
    username: alice
    password: secret
    tls: true
    sni: proxy.example.com
    headers:
      X-Token: abc
`,
			want: Profile{Type: TypeHTTP, Name: "office", Server: "proxy.example.com", ServerPort: 443, HTTP: &HTTP{
				Username: "alice", Password: "secret", Headers: map[string]string{"X-Token": "abc"},
				TLS: TLS{Enabled: true, ServerName: "proxy.example.com"},
			}},
		},
		{
			name: "anytls with idle settings in seconds",
			proxy: `
  - name: AnyTLS
    type: anytls
    server: at.example.com
    port: 8443
    password: passw0rd
    sni: at.example.com
    skip-cert-verify: true
    client-fingerprint: chrome
    idle-session-check-interval: 60
    idle-session-timeout: 120
    min-idle-session: 0
`,
			want: Profile{Type: TypeAnyTLS, Name: "AnyTLS", Server: "at.example.com", ServerPort: 8443, AnyTLS: &AnyTLS{
				Password: "passw0rd", IdleSessionCheckInterval: "60s", IdleSessionTimeout: "120s", MinIdleSession: 5,
				TLS: TLS{Enabled: true, ServerName: "at.example.com", Insecure: true, UTLS: UTLS{Enabled: true, Fingerprint: "chrome"}},
			}},
		},
		{
			name:  "anytls defaults",
			proxy: "  - {name: AT, type: anytls, server: at2.example.com, port: 443, password: pw, min-idle-session: 2}\n",
			want: Profile{Type: TypeAnyTLS, Name: "AT", Server: "at2.example.com", ServerPort: 443, AnyTLS: &AnyTLS{
				Password: "pw", IdleSessionCheckInterval: "30s", IdleSessionTimeout: "30s", MinIdleSession: 2,
				TLS: TLS{Enabled: true},
			}},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			d := mustParseDocument(t, "proxies:\n"+tt.proxy)
			if d.Format != FormatClash || len(d.Profiles) != 1 || len(d.Skipped) != 0 {
				t.Fatalf("format %q, %d profiles, skipped %+v", d.Format, len(d.Profiles), d.Skipped)
			}
			if got := d.Profiles[0]; !reflect.DeepEqual(got, tt.want) {
				t.Errorf("\n got %s\nwant %s", dump(got), dump(tt.want))
			}
		})
	}
}

func TestClashDocument(t *testing.T) {
	body := `mixed-port: 7890
mode: rule
dns:
  enable: true
  nameserver: [https://1.1.1.1/dns-query, "tls://8.8.8.8"]
proxies:
  - {name: DIRECT, type: direct}
  - name: Good
    type: trojan
    server: t.example.com
    port: 443
    password: secret
  - {name: SSR, type: ssr, server: s.example.com, port: 443, cipher: aes-256-cfb, password: secret, obfs: plain, protocol: origin}
  - {name: WG, type: wireguard, server: w.example.com, port: 51820, private-key: secret, public-key: key, ip: 10.0.0.2}
  - {name: HY1, type: hysteria, server: h.example.com, port: 443, auth-str: secret}
  - {name: Bad port, type: ss, server: s.example.com, port: abc, cipher: aes-128-gcm, password: secret}
  - name: List opts
    type: vmess
    server: v.example.com
    port: 443
    uuid: b831381d-6324-4d53-ad4f-8cda48b30811
    network: ws
    ws-opts: [/path]
  - {name: No type, server: n.example.com, port: 1}
  - ~
  - just text
  - {name: REALITY, type: vless, server: r.example.com, port: 443, uuid: b831381d-6324-4d53-ad4f-8cda48b30811, tls: true, reality-opts: {short-id: ab}}
  - {name: Pin, type: trojan, server: p.example.com, port: 443, password: secret, fingerprint: deadbeef}
  - {name: TUIC v4, type: tuic, server: q.example.com, port: 443, token: secret}
  - {name: TLS SOCKS, type: socks5, server: s5.example.com, port: 1080, username: u, password: secret, tls: true}
  - {name: KCP, type: vmess, server: k.example.com, port: 443, uuid: b831381d-6324-4d53-ad4f-8cda48b30811, network: kcp}
  - {name: Shadow-TLS, type: ss, server: st.example.com, port: 443, cipher: aes-128-gcm, password: secret, plugin: shadow-tls, plugin-opts: {host: cloud.tencent.com, password: secret}}
  - {name: Bad host, type: vless, server: "bad host", port: 443, uuid: secret}
  - {name: Group, type: select, proxies: [Good]}
  - name: Last
    type: ss
    server: 192.0.2.30
    port: 8388
    cipher: chacha20-ietf-poly1305
    password: secret
proxy-groups:
  - name: PROXY
    type: select
    proxies: [Good, Last]
rules:
  - MATCH,PROXY
`
	d := mustParseDocument(t, body)
	wantProfiles := []Profile{
		{Type: TypeTrojan, Name: "Good", Server: "t.example.com", ServerPort: 443, Trojan: &Trojan{Password: "secret", TLS: TLS{Enabled: true}}},
		{Type: TypeShadowsocks, Name: "Last", Server: "192.0.2.30", ServerPort: 8388, Shadowsocks: &Shadowsocks{
			Method: "chacha20-ietf-poly1305", Password: "secret"}},
	}
	if d.Format != FormatClash || !reflect.DeepEqual(d.Profiles, wantProfiles) {
		t.Fatalf("format %q, profiles %q", d.Format, names(d.Profiles))
	}
	want := []Skipped{
		{Index: 3, Name: "SSR", Unsupported: true, Reason: `Clash proxies of type "ssr" are not supported`},
		{Index: 4, Name: "WG", Unsupported: true, Reason: `Clash proxies of type "wireguard" are not supported`},
		{Index: 5, Name: "HY1", Unsupported: true, Reason: `Clash proxies of type "hysteria" are not supported`},
		{Index: 6, Name: "Bad port", Reason: "the server port 0 is not from 1 to 65535"},
		{Index: 7, Name: "List opts", Reason: "ws-opts must be a map"},
		{Index: 8, Name: "No type", Reason: "the proxy has no type"},
		{Index: 9, Reason: "the proxy is empty"},
		{Index: 10, Reason: "the proxy is not a map of settings"},
		{Index: 11, Name: "REALITY", Reason: "reality-opts needs the server's public-key"},
		{Index: 12, Name: "Pin", Reason: `the certificate pin "deadbeef" is not a SHA-256 hash in hex or base64`},
		{Index: 13, Name: "TUIC v4", Unsupported: true, Reason: "TUIC v4 servers, which take a token, are not supported; only TUIC v5 works"},
		{Index: 14, Name: "TLS SOCKS", Unsupported: true, Reason: "SOCKS5 over TLS is not supported"},
		{Index: 15, Name: "KCP", Unsupported: true, Reason: `the transport "kcp" is not supported`},
		{Index: 16, Name: "Shadow-TLS", Unsupported: true, Reason: `the Shadowsocks plugin "shadow-tls" is not supported; only obfs and v2ray-plugin work`},
		{Index: 17, Name: "Bad host", Reason: `"bad host" is not a host name`},
	}
	if !reflect.DeepEqual(d.Skipped, want) {
		t.Fatalf("skipped\n got %+v\nwant %+v", d.Skipped, want)
	}
	for _, s := range d.Skipped {
		if strings.Contains(s.Reason, "secret") {
			t.Errorf("the reason %q shows a secret", s.Reason)
		}
	}
}

func TestClashBrokenProxies(t *testing.T) {
	const vless = "{name: X, type: vless, server: x.example.com, port: 443, uuid: b831381d-6324-4d53-ad4f-8cda48b30811, "
	tests := []struct {
		name, proxy, want string
		unsupported       bool
	}{
		{"download leg without a server", vless + "network: xhttp, xhttp-opts: {download-settings: {server: ''}}}", "the XHTTP download leg has no server", false},
		{"download port", vless + "network: xhttp, xhttp-opts: {download-settings: {port: 70000}}}", "the XHTTP download port 70000 is not from 1 to 65535", false},
		{"download pin", vless + "tls: true, network: xhttp, xhttp-opts: {download-settings: {fingerprint: abc}}}", `the certificate pin "abc" is not a SHA-256 hash in hex or base64`, false},
		{"h2 hosts as a map", vless + "tls: true, network: h2, h2-opts: {host: {a: b}}}", "h2-opts.host must be a list", false},
		{"alpn as a map", vless + "tls: true, alpn: {h2: true}}", "alpn must be a list", false},
		{"bad merge", "{name: X, type: ss, <<: 1}", "the proxy cannot be read: map merge requires map or sequence of maps as the value", false},
		{"hysteria2 range", "{name: X, type: hysteria2, server: h.example.com, ports: 20000-70000, password: x}", `the port range "20000-70000" is not from 1 to 65535`, false},
		{"hysteria2 gecko", "{name: X, type: hysteria2, server: h.example.com, port: 443, obfs: gecko, obfs-password: x}", `the Hysteria2 obfuscation "gecko" is not supported`, true},
		{"tuic without uuid", "{name: X, type: tuic, server: q.example.com, port: 443, password: x}", "the TUIC UUID is missing", false},
		{"trojan-go shadowsocks", "{name: X, type: trojan, server: t.example.com, port: 443, password: x, ss-opts: {enabled: true, method: aes-128-gcm, password: y}}", "Trojan-Go's Shadowsocks layer, ss-opts, is not supported", true},
		{"vless without uuid", "{name: X, type: vless, server: x.example.com, port: 443}", "the VLESS UUID is missing", false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			d := mustParseDocument(t, "proxies:\n  - "+tt.proxy+"\n")
			want := []Skipped{{Index: 1, Name: "X", Unsupported: tt.unsupported, Reason: tt.want}}
			if len(d.Profiles) != 0 || !reflect.DeepEqual(d.Skipped, want) {
				t.Fatalf("profiles %q, skipped\n got %+v\nwant %+v", names(d.Profiles), d.Skipped, want)
			}
		})
	}
}

func TestClashDocumentForms(t *testing.T) {
	merges := `x-common: &common
  type: ss
  server: 192.0.2.1
  port: 8388
  cipher: aes-128-gcm
proxies:
  - <<: *common
    name: A
    password: a
  - name: B
    <<: *common
    password: b
    port: 8389
`
	flow := "proxies: [{name: A, type: socks5, server: 192.0.2.40, port: 1080}, {name: B, type: http, server: 192.0.2.41, port: 8080}]\n"
	for name, body := range map[string]string{
		"merge keys":  merges,
		"flow list":   flow,
		"base64":      base64.StdEncoding.EncodeToString([]byte(merges)),
		"crlf and cr": "\uFEFF" + strings.ReplaceAll(strings.ReplaceAll(flow, "\n", "\r\n"), ", {", ",\r  {"),
	} {
		t.Run(name, func(t *testing.T) {
			d := mustParseDocument(t, body)
			if got := names(d.Profiles); d.Format != FormatClash || !reflect.DeepEqual(got, []string{"A", "B"}) || len(d.Skipped) != 0 {
				t.Fatalf("format %q, profiles %q, skipped %+v", d.Format, got, d.Skipped)
			}
		})
	}
	d := mustParseDocument(t, merges)
	if d.Profiles[0].ServerPort != 8388 || d.Profiles[1].ServerPort != 8389 || d.Profiles[1].Shadowsocks.Password != "b" {
		t.Fatalf("merged %s and %s", dump(d.Profiles[0]), dump(d.Profiles[1]))
	}
}

func TestClashSanitizedBody(t *testing.T) {
	body := "\uFEFFproxies:\r\n  - name: caf\xe9\r    type: ss\r\n    server: 192.0.2.1\r\n    port: 8388\r\n" +
		"    cipher: aes-128-gcm\r\n    password: p\x00w\r\n"
	d := mustParseDocument(t, body)
	want := []Profile{{Type: TypeShadowsocks, Name: "caf\uFFFD", Server: "192.0.2.1", ServerPort: 8388, Shadowsocks: &Shadowsocks{
		Method: "aes-128-gcm", Password: "pw"}}}
	if !reflect.DeepEqual(d.Profiles, want) || len(d.Skipped) != 0 {
		t.Fatalf("profiles %+v, skipped %+v", d.Profiles, d.Skipped)
	}
}

func TestClashDocumentErrors(t *testing.T) {
	providers := `proxy-providers:
  provider1:
    type: http
    url: "https://sub.example.com/clash"
    interval: 3600
proxy-groups:
  - name: PROXY
    type: select
    use: [provider1]
    proxies: [DIRECT]
`
	for name, tt := range map[string]struct{ body, want string }{
		"empty list":     {"proxies: []\nrules: [MATCH,DIRECT]\n", "the Clash config has no proxies list"},
		"a map":          {"proxies:\n  name: a\n", "the Clash config has no proxies list"},
		"null":           {"proxies:\nrules: []\n", "the Clash config has no proxies list"},
		"root list":      {"- proxies: []\n", "the Clash config has no proxies list"},
		"only providers": {providers, "the Clash config only lists proxy-providers, which are not downloaded"},
		"bad yaml":       {"proxies:\n  - name: a\n   type: ss\n", "the Clash config cannot be read: line 2: did not find expected '-' indicator"},
		"unknown anchor": {"proxies:\n  - *nowhere\n", "the Clash config cannot be read: unknown anchor 'nowhere' referenced"},
	} {
		t.Run(name, func(t *testing.T) {
			if _, err := ParseDocument([]byte(tt.body)); err == nil || !strings.Contains(err.Error(), tt.want) {
				t.Fatalf("error %v, want %q", err, tt.want)
			}
		})
	}
}

// The Qt client's cases, and bodies that hide structure where a careless scanner would see a scalar.
func TestClashDeepNestingIsRefused(t *testing.T) {
	stairs := "proxies:\n"
	for i := range 200 {
		stairs += strings.Repeat(" ", i) + "- a:\n"
	}
	brackets := strings.Repeat("[", 100) + strings.Repeat("]", 100)
	for name, body := range map[string]string{
		"flow sequences":     "proxies: " + strings.Repeat("[", 20000) + strings.Repeat("]", 20000),
		"flow mappings":      "proxies: " + strings.Repeat("{a: ", 20000) + strings.Repeat("}", 20000),
		"dashes on a line":   "proxies:\n" + strings.Repeat("- ", 20000) + "x\n",
		"explicit keys":      "proxies:\n" + strings.Repeat("? ", 20000) + "x\n",
		"indented":           stairs,
		"behind a directive": "%foo \"\n---\nproxies: " + strings.Repeat("[", 20000),
		"after a plain scalar that goes on with a quote": "a: x\n  'hidden\nb:\n  c: " + brackets + "\n'\nproxies: []\n",
		"after a block scalar under a dash":              "proxies:\n- k: |\n  a:\n    b: " + brackets + "\n",
		"flow over lines":                                "proxies: [\n" + strings.Repeat("[\n", 100) + strings.Repeat("]\n", 100) + "]\n",
	} {
		t.Run(name, func(t *testing.T) {
			if _, err := ParseDocument([]byte(body)); err == nil || !strings.Contains(err.Error(), "too deeply") {
				t.Fatalf("error %v, want the nesting refused", err)
			}
		})
	}
}

func TestClashNestingSkipsScalarsAndComments(t *testing.T) {
	body := strings.Repeat("# a line of notes\r", 10000) +
		"proxies:\n" +
		"  - name: \"" + strings.Repeat("[", 100) + "\"\n" +
		"    type: ss\n    server: 192.0.2.1\n    port: 8388\n    cipher: aes-128-gcm\n" +
		"    password: '" + strings.Repeat("{'' ", 50) + "'\n" +
		"    extra: {a: {b: [1, [2, [3]]]}}\n" +
		"  - name: US [Premium] {1}\n" +
		"    type: ss\n    server: 192.0.2.2\n    port: 8388\n    cipher: aes-128-gcm\n" +
		"    password: p#1 # a comment after the space\n" +
		"# " + strings.Repeat("[", 100) + "\n" +
		"notes: |\n  " + strings.Repeat("[", 100) + "\n\n    " + strings.Repeat("- ", 100) + "\n" +
		"folded: >-\n  " + strings.Repeat("{", 100) + "\n" +
		"quoted: \"first line\n  " + strings.Repeat("[", 100) + "\"\n"
	d := mustParseDocument(t, body)
	if got := names(d.Profiles); !reflect.DeepEqual(got, []string{strings.Repeat("[", 100), "US [Premium] {1}"}) || len(d.Skipped) != 0 {
		t.Fatalf("profiles %q, skipped %+v", got, d.Skipped)
	}
	if password := d.Profiles[1].Shadowsocks.Password; password != "p#1" {
		t.Fatalf("password %q", password)
	}
}

func TestClashTooManyValuesAreRefused(t *testing.T) {
	// Empty items are null nodes to yaml.v2, so they count as much as written ones.
	for name, body := range map[string]string{
		"empty items": "proxies:\n" + strings.Repeat("-\n", clashMaxTokens+1),
		"flow items":  "proxies: [" + strings.Repeat("{type: ss},", clashMaxTokens/4) + "]\n",
	} {
		t.Run(name, func(t *testing.T) {
			if _, err := ParseDocument([]byte(body)); err == nil || !strings.Contains(err.Error(), "too many values") {
				t.Fatalf("error %v, want the body refused", err)
			}
		})
	}
}

// yaml.v2's own limits must hold the stack even where clashCheckSize would let a body through.
func TestClashDecoderSurvivesYAMLDepthLimits(t *testing.T) {
	deepest := "proxies:\n" + strings.Repeat("- ", 9990) + strings.Repeat("[", 9990) + "x" + strings.Repeat("]", 9990) + "\n"
	var root clashRoot
	if err := clashDecode([]byte(deepest), &root); err != nil || len(root.Proxies) != 1 || root.Proxies[0].err == nil {
		t.Fatalf("error %v, proxies %+v", err, root.Proxies)
	}
	if err := clashDecode([]byte("proxies: "+strings.Repeat("[", 10001)), &root); err == nil || !strings.Contains(err.Error(), "max depth") {
		t.Fatalf("error %v, want yaml.v2's depth limit", err)
	}
}

func TestClashAliasBombs(t *testing.T) {
	var laughs strings.Builder
	laughs.WriteString(`a: &a ["lol","lol","lol","lol","lol","lol","lol","lol","lol"]` + "\n")
	for c := 'b'; c <= 'i'; c++ {
		fmt.Fprintf(&laughs, "%c: &%c [%s*%c]\n", c, c, strings.Repeat("*"+string(c-1)+",", 8), c-1)
	}
	laughs.WriteString("proxies:\n  - *i\n  - {name: Q, type: trojan, server: q.example.com, port: 443, password: x, alpn: *i}\n")
	// The typed decoder reads one level of an alias: a list of lists is not a proxy, nor a list of ALPN names.
	d := mustParseDocument(t, laughs.String())
	wantProfiles := []Profile{{Type: TypeTrojan, Name: "Q", Server: "q.example.com", ServerPort: 443, Trojan: &Trojan{
		Password: "x", TLS: TLS{Enabled: true}}}}
	wantSkipped := []Skipped{{Index: 1, Reason: "the proxy is not a map of settings"}}
	if !reflect.DeepEqual(d.Profiles, wantProfiles) || !reflect.DeepEqual(d.Skipped, wantSkipped) {
		t.Fatalf("profiles %+v, skipped %+v", d.Profiles, d.Skipped)
	}

	// Merges expand into one map, so they meet yaml.v2's limit on alias expansion.
	var merges strings.Builder
	merges.WriteString("m0: &m0 {name: lol}\n")
	for i := 1; i <= 9; i++ {
		fmt.Fprintf(&merges, "m%d: &m%d {<<: [%s*m%d]}\n", i, i, strings.Repeat(fmt.Sprintf("*m%d,", i-1), 9), i-1)
	}
	merges.WriteString("proxies:\n  - <<: *m9\n    type: ss\n")
	if _, err := ParseDocument([]byte(merges.String())); err == nil || !strings.Contains(err.Error(), "excessive aliasing") {
		t.Fatalf("error %v, want the aliasing refused", err)
	}
}
