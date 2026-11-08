package profile

import (
	"encoding/base64"
	"errors"
	"net/url"
	"reflect"
	"strings"
	"testing"
)

func mustParseLink(t *testing.T, link string) Profile {
	t.Helper()
	p, err := ParseLink(link)
	if err != nil {
		t.Fatalf("ParseLink(%q): %v", link, err)
	}
	return p
}

// Share links of every protocol, transport and TLS variant Poros reads: each must come back as a link of its own
// protocol that reads into the same profile.
var roundTripLinks = []string{
	"vless://" + testUUID + "@de1.example.com:443?encryption=none&flow=xtls-rprx-vision&security=reality&sni=www.microsoft.com" +
		"&fp=chrome&pbk=" + testPBK + "&sid=6ba85179e30d4fc2&spx=%2F&type=tcp#%F0%9F%87%A9%F0%9F%87%AA%20Germany",
	"vless://" + testUUID + "@ws.example.com:8443?encryption=none&security=tls&sni=ws.example.com&alpn=h2,http/1.1&fp=firefox" +
		"&type=ws&host=cdn.example.com&path=%2Fray%3Fed%3D2048#WS",
	"vless://" + testUUID + "@grpc.example.com:443?type=grpc&serviceName=gun%2Fsvc&security=tls&sni=grpc.example.com",
	"vless://" + testUUID + "@x.example.com:443?type=xhttp&mode=packet-up&path=%2Fx&host=x.example.com" +
		"&extra=%7B%22xPaddingBytes%22%3A%22100-1000%22%7D&security=reality&sni=x.example.com&fp=chrome&pbk=" + testPBK,
	"vless://" + testUUID + "@hu.example.com:80?type=httpupgrade&host=hu.example.com&path=%2Fup&security=none" +
		"&packetEncoding=none&mux=1&mux_protocol=h2mux&mux_max_streams=8&mux_padding=1",
	"vless://" + testUUID + "@enc.example.com:443?encryption=mlkem768x25519plus.native.0rtt.abcdef&flow=xtls-rprx-vision&type=tcp",
	"vless://" + testUUID + "@ech.example.com:443?security=tls&sni=ech.example.com&ech=AEn-DQBFKwAgACABWIHUGj4u8nwPeIx2E6JW",
	"vless://" + testUUID + "@ech2.example.com:443?security=tls&sni=ech2.example.com&ech=cloudflare-ech.com",
	"vless://" + testUUID + "@pin.example.com:443?security=tls&allowInsecure=1&pcs=" + strings.Repeat("ab", 32),
	"vless://" + testUUID + "@obf.example.com:80?type=tcp&headerType=http&host=a.example.com,b.example.com&path=%2Fa,%2Fb",
	"vless://" + testUUID + "@h2.example.com:443?type=http&host=h2.example.com&path=%2Fh2&method=PUT&security=tls" +
		"&headers=X-Test|1&tls_min_version=1.2&tls_max_version=1.3&disable_sni=1",
	vmessJSONLink(`{"v":"2","ps":"VM","add":"vm.example.com","port":"443","id":"` + testUUID + `","aid":"0","scy":"auto",` +
		`"net":"ws","type":"none","host":"cdn.example.com","path":"/vm","tls":"tls","sni":"vm.example.com","alpn":"h2","fp":"chrome"}`),
	vmessJSONLink(`{"v":"2","ps":"gRPC","add":"vg.example.com","port":443,"id":"` + testUUID + `","aid":0,"scy":"aes-128-gcm",` +
		`"net":"grpc","path":"svc","tls":""}`),
	vmessJSONLink(`{"v":"2","ps":"Extra","add":"vx.example.com","port":"8443","id":"` + testUUID + `","aid":"0","scy":"auto",` +
		`"net":"ws","path":"/ws","tls":"tls","sni":"vx.example.com","throneExtra":"packetEncoding=packetaddr&mux=1&ed=2048&pcs=` +
		strings.Repeat("cd", 32) + `"}`),
	"trojan://secret@tr.example.com:443?security=tls&sni=tr.example.com&type=ws&host=tr.example.com&path=%2Ftr#Trojan",
	"trojan://secret@trr.example.com:443?security=reality&sni=www.example.com&fp=safari&pbk=" + testPBK + "&sid=ab&type=grpc&serviceName=g",
	"trojan://p%40ss%3Aword@trn.example.com:8080?security=none",
	"trojan://p@[2001:db8::1]:443?sni=v6.example.com",
	"ss://" + base64.RawURLEncoding.EncodeToString([]byte("aes-256-gcm:pa:ss")) + "@ss.example.com:8388#SS",
	"ss://2022-blake3-aes-256-gcm:" + url.QueryEscape(testSS2022Key) + "@ss2.example.com:8443#SS2022",
	"ss://" + base64.RawURLEncoding.EncodeToString([]byte("chacha20-ietf-poly1305:pw")) + "@obfs.example.com:443" +
		"?plugin=obfs-local%3Bobfs%3Dhttp%3Bobfs-host%3Dexample.com&uot=1",
	"hysteria2://pass%3Aword@hy.example.com:443?sni=hy.example.com&insecure=1&obfs=salamander&obfs-password=ob" +
		"&mport=20000-30000,40000-40100&upmbps=50&downmbps=200&hop_interval=30s&alpn=h3#HY2",
	"tuic://" + testUUID + ":pw@tu.example.com:443?congestion_control=bbr&udp_relay_mode=native&zero_rtt_handshake=true" +
		"&heartbeat=10s&sni=tu.example.com&alpn=h3",
	"socks5://user:pass@127.0.0.1:1080#S5",
	"socks4a://198.51.100.7:1080",
	"http://u:p@px.example.com:8080?path=%2Fp#H",
	"https://u:p@px.example.com:443?sni=px.example.com#HS",
	"anytls://pw@at.example.com:443?sni=at.example.com&idle_session_check_interval=20s&idle_session_timeout=40s&min_idle_session=2",
	"wireguard://" + url.QueryEscape(testWGPrivate) + "@wg.example.com:51820?publickey=" + url.QueryEscape(testWGPublic) +
		"&address=172.16.0.2/32,fd01::2/128&reserved=1,2,3&mtu=1280&keepalive=25#WG",
	"hysteria://hy.example.com:443?auth=pw&upmbps=50&downmbps=200&obfs=xplus&obfsParam=ob&mport=20000-30000&hop_interval=30s" +
		"&sni=hy.example.com&alpn=hysteria#HY1",
	"naive+https://user:pw@nv.example.com:443?sni=nv.example.com&extra-headers=X-A%3A%201%0D%0AX-B%3A%202&uot=1#NV",
	"naive+quic://user:pw@nq.example.com:443?congestion_control=bbr2&insecure-concurrency=3&ech=1",
	"ssh://root:pw@ssh.example.com:22?private_key=" + base64.RawStdEncoding.EncodeToString([]byte(testSSHKey)) +
		"&host_key=" + base64.RawStdEncoding.EncodeToString([]byte("ssh-ed25519 AAAAC3NzaC1lZDI1NTE5")) + "&client_version=SSH-2.0-OpenSSH_9.6#SSH",
	"snell://psk@sn.example.com:8443?version=4&obfs=tls&obfs-host=bing.com#SN4",
	"snell://psk@sn6.example.com:8443?version=6&mode=unshaped&reuse=1",
}

func TestShareLinkRoundTrip(t *testing.T) {
	for _, link := range roundTripLinks {
		p := mustParseLink(t, link)
		shared := ShareLink(p)
		if strings.HasPrefix(shared, "poros://") {
			t.Errorf("%s fell back to %s", dump(p), shared)
			continue
		}
		scheme, _, _ := strings.Cut(link, "://")
		if got, _, _ := strings.Cut(shared, "://"); got != scheme && !(scheme == "socks5" && got == "socks5") {
			t.Errorf("%q came back as %q", link, shared)
		}
		if got := mustParseLink(t, shared); !reflect.DeepEqual(got, p) {
			t.Errorf("%s\nshared as %s\nreads as %s", dump(p), shared, dump(got))
		}
	}
}

func TestShareLinkReadsLikeOtherClients(t *testing.T) {
	p := mustParseLink(t, "trojan://secret@tr.example.com:443?security=tls&sni=tr.example.com&type=ws&host=tr.example.com&path=%2Ftr#My Trojan")
	want := "trojan://secret@tr.example.com:443?security=tls&sni=tr.example.com&type=ws&host=tr.example.com&path=/tr#My%20Trojan"
	if got := ShareLink(p); got != want {
		t.Errorf("ShareLink = %s, want %s", got, want)
	}
	ss := mustParseLink(t, "ss://"+base64.RawURLEncoding.EncodeToString([]byte("aes-128-gcm:pw"))+"@ss.example.com:8388")
	if got := ShareLink(ss); got != "ss://YWVzLTEyOC1nY206cHc@ss.example.com:8388" {
		t.Errorf("SIP002 link: %s", got)
	}
}

func TestShareLinkFallsBackToPorosLink(t *testing.T) {
	base := mustParseLink(t, "vless://"+testUUID+"@fb.example.com:443?security=tls&sni=fb.example.com&type=ws&path=%2Fws")
	headers := base
	vless := *base.VLESS
	vless.Transport.Headers = map[string]string{"X-Pipe": "a|b"}
	headers.VLESS = &vless

	anytls := mustParseLink(t, "anytls://pw@at.example.com:443")
	anyPart := *anytls.AnyTLS
	anyPart.IdleSessionCheckInterval, anyPart.IdleSessionTimeout = "", ""
	anytls.AnyTLS = &anyPart

	shadowTLS := mustParseLink(t, "ss://"+base64.RawURLEncoding.EncodeToString([]byte("aes-128-gcm:pw"))+"@st.example.com:443")
	ssPart := *shadowTLS.Shadowsocks
	ssPart.ShadowTLS = ShadowTLS{Version: 3, Password: "pw", TLS: TLS{Enabled: true, ServerName: "www.apple.com"}}
	shadowTLS.Shadowsocks = &ssPart

	for name, p := range map[string]Profile{"a header with a pipe": headers, "settings no link key holds": anytls, "ShadowTLS": shadowTLS} {
		shared := ShareLink(p)
		if !strings.HasPrefix(shared, "poros://add/") {
			t.Errorf("%s: %s, want a poros://add/ link", name, shared)
			continue
		}
		if got := mustParseLink(t, shared); !reflect.DeepEqual(got, p) {
			t.Errorf("%s: the poros://add/ link reads as %s, want %s", name, dump(got), dump(p))
		}
	}
}

func TestShareLinkLowerCasesTheHost(t *testing.T) {
	p := mustParseLink(t, "trojan://secret@tr.example.com:443?sni=tr.example.com")
	p.Server = "TR.Example.com"
	shared := ShareLink(p)
	if got := mustParseLink(t, shared); strings.HasPrefix(shared, "poros://") || got.Server != "tr.example.com" {
		t.Errorf("a host in capitals: %s", shared)
	}
}

func TestThroneLink(t *testing.T) {
	outbound := `{"type":"trojan","tag":"Throne server","server":"th.example.com","server_port":443,"password":"pw",` +
		`"tls":{"enabled":true,"server_name":"th.example.com"}}`
	p, err := ParseLink("throne://add/" + base64.RawURLEncoding.EncodeToString([]byte(outbound)))
	if err != nil || p.Type != TypeTrojan || p.Name != "Throne server" || p.Server != "th.example.com" || p.Trojan.Password != "pw" {
		t.Fatalf("throne://add/: %s, %v", dump(p), err)
	}
	for _, link := range []string{"throne://add/%%%", "throne://add/" + base64.RawURLEncoding.EncodeToString([]byte(`{"type":"direct"}`))} {
		if _, err := ParseLink(link); err == nil || errors.Is(err, ErrNotLink) {
			t.Errorf("ParseLink(%q) error %v, want a damaged link", link, err)
		}
	}
	if _, err := ParseLink("throne://addsub/aHR0cHM6Ly9leGFtcGxlLmNvbQ"); !errors.Is(err, ErrNotLink) {
		t.Errorf("a Throne subscription link is no server: %v", err)
	}
}
