package config

import (
	"bytes"
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/x509"
	"encoding/base64"
	"encoding/json"
	"errors"
	"flag"
	"net/netip"
	"net/url"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"
	"time"

	C "github.com/sagernet/sing-box/constant"
	"github.com/sagernet/sing-box/option"
	"github.com/sagernet/sing/common/json/badoption"

	"PorosCore/internal/boxmain"
	"PorosCore/internal/xray"
	"PorosCore/profile"
)

var update = flag.Bool("update", false, "rewrite the golden configs in testdata")

const (
	testUUID = "b831381d-6324-4d53-ad4f-8cda48b30811"
	testPBK  = "jNXHt1yRo0vDuchQlIP6Z0ZvjT3KtzVI-T4E7RoLJS0"
	testPin  = "6d3c6a1f0e9b7f2a4c8d5e1b3a7f9c2d4e6b8a1c3d5f7e9a2b4c6d8e0f1a3b5c"
)

var testBridge = Bridge{Port: 23456, Username: "bridge-user", Password: "bridge-pass"}

func mustParse(t *testing.T, link string) profile.Profile {
	t.Helper()
	p, err := profile.ParseLink(link)
	if err != nil {
		t.Fatalf("ParseLink(%q): %v", link, err)
	}
	return p
}

func vmessLink(fields string) string {
	return "vmess://" + base64.StdEncoding.EncodeToString([]byte(`{"v":"2","id":"`+testUUID+`",`+fields+`}`))
}

func xhttpExtra(t *testing.T, extra map[string]any) string {
	t.Helper()
	data, err := json.Marshal(extra)
	if err != nil {
		t.Fatal(err)
	}
	return url.QueryEscape(string(data))
}

var allPreset = Preset{
	Name:            "All traffic",
	DefaultOutbound: OutboundProxy,
	Rules: []Rule{
		{Name: "Route DNS", Action: ActionHijackDNS, Protocol: []string{"dns"}},
		{Name: "Local networks", Action: ActionRoute, Outbound: OutboundDirect, IPIsPrivate: true},
	},
}

// sampleLinks covers every protocol and transport the builder writes; each must build and pass the core's check.
func sampleLinks(t *testing.T) map[string]string {
	ca, staticKey := testCA(t), testStaticKey(t)
	leg := xhttpExtra(t, map[string]any{
		"xPaddingBytes": "100-1000",
		"downloadSettings": map[string]any{
			"address": "dl.example.com", "port": 443, "network": "xhttp", "security": "tls",
			"tlsSettings":   map[string]any{"serverName": "dl.example.com", "allowInsecure": false, "masterKeyLog": "keys.txt"},
			"xhttpSettings": map[string]any{"path": "/down"},
			"sockopt":       map[string]any{"dialerProxy": "elsewhere", "domainStrategy": "UseIPv4"},
		},
	})
	return map[string]string{
		"vless reality vision": "vless://" + testUUID + "@r.example.com:443?encryption=none&flow=xtls-rprx-vision&security=reality" +
			"&sni=www.microsoft.com&fp=chrome&pbk=" + testPBK + "&sid=6ba85179e30d4fc2&type=tcp&spx=%2Fpath#reality",
		"vless reality grpc": "vless://" + testUUID + "@rg.example.com:443?security=reality&sni=www.microsoft.com&pbk=" + testPBK +
			"&sid=ab&type=grpc&serviceName=gun#reality-grpc",
		"vless ws tls": "vless://" + testUUID + "@ws.example.com:443?encryption=none&security=tls&sni=ws.example.com&fp=firefox" +
			"&alpn=h2%2Chttp%2F1.1&type=ws&host=ws.example.com&path=%2Fws%3Fed%3D2048#ws",
		"vless grpc tls": "vless://" + testUUID + "@g.example.com:443?security=tls&sni=g.example.com&type=grpc&serviceName=gun#grpc",
		"vless httpupgrade": "vless://" + testUUID + "@hu.example.com:443?security=tls&sni=hu.example.com&type=httpupgrade" +
			"&host=hu.example.com&path=%2Fup#hu",
		"vless h2":   "vless://" + testUUID + "@h2.example.com:443?security=tls&sni=h2.example.com&type=h2&host=h2.example.com&path=%2Fh2#h2",
		"vless quic": "vless://" + testUUID + "@q.example.com:443?security=tls&sni=q.example.com&type=quic#quic",
		"vless raw http header": "vless://" + testUUID + "@203.0.113.5:80?type=raw&headerType=http&host=a.example.com%2Cb.example.com" +
			"&path=%2Fa%2C%2Fb#header",
		"vless raw http header tls": "vless://" + testUUID + "@hh.example.com:443?type=raw&headerType=http&host=a.example.com" +
			"&security=tls&sni=hh.example.com&pcs=" + testPin + "#header-tls",
		"vless xhttp reality": "vless://" + testUUID + "@x.example.com:443?encryption=none&security=reality&sni=x.example.com&fp=chrome" +
			"&pbk=" + testPBK + "&sid=ab&type=xhttp&path=%2Fx&mode=auto#xhttp",
		"vless xhttp download leg": "vless://" + testUUID + "@up.example.com:443?security=tls&sni=up.example.com&type=xhttp" +
			"&path=%2Fup&mode=packet-up&extra=" + leg + "#xhttp-leg",
		"vmess ws tls": vmessLink(`"add":"vm.example.com","port":"443","aid":"0","scy":"auto","net":"ws","host":"vm.example.com",` +
			`"path":"/vm","tls":"tls","sni":"vm.example.com","ps":"vmess"`),
		"vmess grpc": "vmess://" + testUUID + "@vm2.example.com?encryption=aes-128-gcm&type=grpc&serviceName=svc&security=tls" +
			"&sni=vm2.example.com#vmess-grpc",
		"trojan grpc": "trojan://secret@tr.example.com:443?security=tls&sni=tr.example.com&type=grpc&serviceName=trojan#trojan-grpc",
		"trojan tcp":  "trojan://secret@t.example.com:443?sni=t.example.com&allowInsecure=1#trojan",
		"ss aead":     "ss://YWVzLTI1Ni1nY206dGVzdA@192.0.2.10:8388#SS",
		"ss 2022 uot": "ss://2022-blake3-aes-256-gcm:" + url.QueryEscape("Kq7+pXcDdRsQ0zYGAQIDBAUGBwgJCgsMDQ4PEBESExQ=") +
			"@ss.example.com:443?uot=1#2022",
		"ss obfs": "ss://QUVTLTEyOC1HQ006cGFzcw==@ss2.example.com:8388/?plugin=obfs-local%3Bobfs%3Dhttp%3Bobfs-host%3Dwww.bing.com#obfs",
		"hysteria2": "hysteria2://letmein@hy.example.com:443/?sni=hy.example.com&obfs=salamander&obfs-password=gura" +
			"&mport=20000-30000&hop_interval=30s&upmbps=50&downmbps=200&pinSHA256=" + testPin + "#hy2",
		"tuic": "tuic://" + testUUID + ":pw@tuic.example.com:443?congestion_control=bbr&udp_relay_mode=native&alpn=h3" +
			"&sni=tuic.example.com&heartbeat=10s#tuic",
		"socks":  "socks5://user:pass@198.51.100.9:1080#socks",
		"http":   "https://user:pass@proxy.example.com:8443#http",
		"anytls": "anytls://passw0rd@at.example.com:8443?sni=at.example.com&idle_session_timeout=60s#anytls",
		"wireguard": "wireguard://" + url.QueryEscape(testWGPrivate) + "@wg.example.com:51820?publickey=" + url.QueryEscape(testWGPublic) +
			"&address=172.16.0.2/32,fd01::2/128&reserved=1,2,3&mtu=1280&keepalive=25#wg",
		"hysteria v1": "hysteria://hy1.example.com:443?auth=pw&upmbps=50&downmbps=200&obfsParam=ob&mport=20000-30000" +
			"&sni=hy1.example.com#hy1",
		"naive":      "naive+https://user:pw@nv.example.com:443?sni=nv.example.com&extra-headers=X-A%3A%201#naive",
		"naive quic": "naive+quic://user:pw@nq.example.com:443?congestion_control=bbr&uot=1#naive-quic",
		"ssh":        "ssh://root:pw@ssh.example.com:22?client_version=SSH-2.0-OpenSSH_9.6#ssh",
		"snell v4":   "snell://psk@sn.example.com:8443?version=4&obfs=http&obfs-host=bing.com#snell4",
		"snell v6":   "snell://psk@sn6.example.com:8443?version=6&mode=unshaped&reuse=1#snell6",
		"ss over shadowtls": porosLink(`{"type":"shadowsocks","name":"stls","server":"st.example.com","server_port":443,` +
			`"shadowsocks":{"method":"2022-blake3-aes-128-gcm","password":"AAECAwQFBgcICQoLDA0ODw==",` +
			`"shadowtls":{"version":3,"password":"pw","tls":{"enabled":true,"server_name":"www.apple.com","utls":{"enabled":true,"fingerprint":"chrome"}}}}}`),
		"openvpn": porosLink(`{"type":"openvpn","name":"ovpn","server":"o1.example.com","server_port":1194,"openvpn":{"network":"udp",` +
			`"remotes":[{"server":"o2.example.com","port":443,"network":"tcp"}],"username":"u","password":"pw","ca":` + jsonString(t, ca) +
			`,"control_wrap":{"type":"tls_crypt","key":` + jsonString(t, staticKey) + `},"verify_name":"o.example.com",` +
			`"remote_cert_tls":"server","data_ciphers":["AES-256-GCM"],"compression":"lz4-v2","ping_interval":"10s","ping_restart":"60s"}}`),
		"openconnect": porosLink(`{"type":"openconnect","name":"office","server":"gw.example.com","server_port":8443,` +
			`"openconnect":{"flavor":"anyconnect","path":"/staff","username":"u","password":"pw","ca":` + jsonString(t, ca) + `}}`),
		"masque": porosLink(`{"type":"masque","name":"masque","server":"m.example.com","server_port":443,"masque":{"username":"u",` +
			`"password":"pw","path":"/.well-known/masque/ip/","tls":{"enabled":true,"server_name":"m.example.com"}}}`),
		"custom outbound": porosLink(`{"type":"custom","name":"custom","custom":{"json":{"type":"shadowsocks","tag":"mine",` +
			`"server":"cs.example.com","server_port":8388,"method":"aes-128-gcm","password":"pw","multiplex":{"enabled":true}}}}`),
		"custom endpoint": porosLink(`{"type":"custom","name":"custom-wg","custom":{"json":{"type":"wireguard","address":["172.16.0.2/32"],` +
			`"private_key":"` + testWGPrivate + `","peers":[{"address":"cw.example.com","port":51820,"public_key":"` + testWGPublic +
			`","allowed_ips":["0.0.0.0/0"]}]}}}`),
		"direct server": porosLink(`{"type":"direct","name":"direct","direct":{}}`),
		"hysteria2 realm": porosLink(`{"type":"hysteria2","name":"realm","server":"realm.example.com","server_port":443,` +
			`"hysteria2":{"password":"pw","tls":{"enabled":true,"server_name":"hy.example.com"},` +
			`"realm":{"server_url":"https://realm.example.com","realm_id":"home","stun_servers":["stun.example.com:3478"]}}}`),
	}
}

// WireGuard keys from its documentation.
const (
	testWGPrivate = "yAnz5TF+lXXJte14tji3zlMNq+hd2rYUIgJBgB3fBmk="
	testWGPublic  = "xTIBA5rboUvnH4htodjb6e697QjLERt1NAB4mZqp8Dg="
)

func porosLink(profileJSON string) string {
	return "poros://add/" + base64.RawURLEncoding.EncodeToString([]byte(profileJSON))
}

func TestBuildPassesCoreChecks(t *testing.T) {
	// Without this the test would prove nothing: the checks must catch a broken config.
	if boxmain.Check([]byte(`{"outbounds":[{"type":"vless","tag":"proxy","flow2":""}]}`)) == nil ||
		xray.CheckXrayConfig(`{"outbounds":[{"protocol":"vless","settings":{"address":"a.example","port":1,"id":"x"},`+
			`"streamSettings":{"security":"tls","tlsSettings":{"allowInsecure":true}}}]}`) == nil {
		t.Fatal("the core checks let a broken config through")
	}
	for name, link := range sampleLinks(t) {
		server := mustParse(t, link)
		for _, realityOnXray := range []bool{true, false} {
			for _, preset := range []Preset{allPreset, hardened(allPreset)} {
				configs, err := Build(Input{
					Server: server, Preset: preset, Tun: true, MixedPort: 2090, RealityOnXray: realityOnXray,
					Bridge: testBridge, Interface: "Ethernet", Resolved: map[string]netip.Addr{},
				})
				if err != nil {
					t.Errorf("%s: Build: %v", name, err)
					continue
				}
				data, err := configs.SingBoxJSON()
				if err != nil {
					t.Errorf("%s: marshal: %v", name, err)
					continue
				}
				if err = boxmain.Check(data); err != nil {
					t.Errorf("%s (reality on Xray %v): sing-box refuses the config: %v\n%s", name, realityOnXray, err, data)
				}
				if (configs.Xray != nil) != RunsOnXray(server, realityOnXray) {
					t.Errorf("%s: an Xray config must come exactly with a server on Xray", name)
				}
				if configs.Xray != nil {
					if err = xray.CheckXrayConfig(string(configs.Xray)); err != nil {
						t.Errorf("%s: Xray refuses the config: %v\n%s", name, err, configs.Xray)
					}
				}
			}
		}
	}
}

func hardened(p Preset) Preset {
	p.Name, p.Hardened = "Hardened", true
	return p
}

func TestRunsOnXray(t *testing.T) {
	links := sampleLinks(t)
	cases := []struct {
		name          string
		realityOnXray bool
		want          bool
	}{
		{"vless reality vision", true, true},
		{"vless reality vision", false, false},
		{"vless reality grpc", true, true},
		{"vless xhttp reality", false, true},
		{"vless xhttp download leg", false, true},
		{"vless raw http header tls", false, true},
		{"vless raw http header", true, false},
		{"vless ws tls", true, false},
		{"trojan grpc", true, false},
	}
	for _, c := range cases {
		if got := RunsOnXray(mustParse(t, links[c.name]), c.realityOnXray); got != c.want {
			t.Errorf("%s with REALITY on Xray %v: RunsOnXray = %v", c.name, c.realityOnXray, got)
		}
	}
	encrypted := mustParse(t, "vless://"+testUUID+"@e.example.com:443?encryption=mlkem768x25519plus.native.0rtt.key&type=ws")
	if !RunsOnXray(encrypted, false) {
		t.Error("VLESS encryption runs only on Xray")
	}
}

func TestXrayResolvedServer(t *testing.T) {
	server := mustParse(t, sampleLinks(t)["vless xhttp download leg"])
	if hosts := XrayHosts(server, false); !slices.Equal(hosts, []string{"up.example.com", "dl.example.com"}) {
		t.Fatalf("XrayHosts = %v", hosts)
	}
	configs, err := Build(Input{
		Server: server, Preset: allPreset, MixedPort: 2090, Bridge: testBridge, Interface: "Wi-Fi",
		Resolved: map[string]netip.Addr{
			"up.example.com": netip.MustParseAddr("203.0.113.1"),
			"dl.example.com": netip.MustParseAddr("2001:db8::7"),
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	golden(t, "xray-xhttp-leg.json", configs.Xray)
	text := string(configs.Xray)
	for _, gone := range []string{"elsewhere", "UseIPv4", "keys.txt", "allowInsecure"} {
		if strings.Contains(text, gone) {
			t.Errorf("the download leg keeps %q", gone)
		}
	}
}

func TestXrayDownloadLeg(t *testing.T) {
	base := map[string]any{"address": "dl.example.com", "port": 443, "network": "xhttp"}
	broken := []map[string]any{
		{"port": 443, "network": "xhttp"},
		{"address": "dl.example.com", "port": 0, "network": "xhttp"},
		{"address": "dl.example.com", "port": 443, "network": "ws"},
	}
	for _, leg := range broken {
		raw, _ := json.Marshal(map[string]any{"downloadSettings": leg})
		if _, err := xrayExtra(raw, "auto", "", nil); err == nil {
			t.Errorf("leg %v must be refused: Xray crashes on it", leg)
		}
	}
	insecure := map[string]any{"tlsSettings": map[string]any{"allowInsecure": true}}
	for key, value := range base {
		insecure[key] = value
	}
	raw, _ := json.Marshal(map[string]any{"downloadSettings": insecure})
	if _, err := xrayExtra(raw, "auto", "", nil); !errors.Is(err, profile.ErrUnsupported) {
		t.Errorf("a leg that skips the certificate check must be unsupported, got %v", err)
	}
	raw, _ = json.Marshal(map[string]any{"downloadSettings": base, "noSSEHeader": true})
	extra, err := xrayExtra(raw, "stream-one", "", nil)
	if err != nil || len(extra) != 1 || extra["noSSEHeader"] != true {
		t.Errorf("stream-one drops the download leg and keeps the rest, got %v, %v", extra, err)
	}
}

func TestPresetRules(t *testing.T) {
	preset := Preset{
		Name:            "Mine",
		DefaultOutbound: OutboundProxy,
		// As the preset editor puts them: the DNS rule first, then the user's, then the rest of the preset.
		Rules: []Rule{
			allPreset.Rules[0],
			{Name: "Bank", Action: ActionRoute, Outbound: OutboundDirect, DomainSuffix: []string{"bank.example", ".рф"}},
			{Name: "Office", Action: ActionRoute, Outbound: OutboundProxy, IPCIDR: []string{"10.20.0.0/16", "192.0.2.1"}},
			{Name: "Games", Action: ActionRoute, Outbound: OutboundDirect, ProcessName: []string{"steam.exe"}},
			{Name: "Ads", Action: ActionReject, Domain: []string{"ads.example"}},
			allPreset.Rules[1],
		},
	}
	configs, err := Build(Input{
		Server: mustParse(t, sampleLinks(t)["trojan tcp"]), Preset: hardened(preset), Tun: true, AllowLAN: true, DisableIPv6: true, MixedPort: 2090,
	})
	if err != nil {
		t.Fatal(err)
	}
	data, err := configs.SingBoxJSON()
	if err != nil {
		t.Fatal(err)
	}
	if err = boxmain.Check(data); err != nil {
		t.Fatalf("sing-box refuses the config: %v", err)
	}
	golden(t, "singbox-preset-tun-lan.json", data)

	if rule := configs.SingBox.Route.Rules[2]; rule.DefaultOptions.Action != C.RuleActionTypeResolve ||
		rule.DefaultOptions.ResolveOptions.Strategy != option.DomainStrategy(C.DomainStrategyIPv4Only) {
		t.Errorf("without IPv6 the names of connections resolve to IPv4 here, so that the server does not pick IPv6: %+v", rule)
	}

	excluded := tunExcluded(preset)
	if slices.Contains(excluded, netip.MustParsePrefix("10.0.0.0/8")) || !slices.Contains(excluded, netip.MustParsePrefix("192.168.0.0/16")) {
		t.Errorf("a range a rule sends to the server must reach the TUN, the rest stays out: %v", excluded)
	}
	dns := configs.SingBox.DNS.Rules
	if len(dns) != 2 || dns[0].DefaultOptions.RouteOptions.Server != TagDNSDirect || dns[0].DefaultOptions.DomainSuffix[1] != ".xn--p1ai" ||
		dns[1].DefaultOptions.ProcessName[0] != "steam.exe" {
		t.Errorf("direct names and programs resolve with the system DNS, in rule order: %+v", dns)
	}
}

func TestSingBoxConfig(t *testing.T) {
	configs, err := Build(Input{Server: mustParse(t, sampleLinks(t)["vless ws tls"]), Preset: allPreset, MixedPort: 2090})
	if err != nil {
		t.Fatal(err)
	}
	data, err := configs.SingBoxJSON()
	if err != nil {
		t.Fatal(err)
	}
	golden(t, "singbox-vless-ws.json", data)
}

func TestParsePreset(t *testing.T) {
	good := `{"kind":"poros-route-profile","v":1,"name":"P","default_outbound":"direct",` +
		`"rules":[{"action":"route","outbound":"proxy","domain_suffix":["example.com"]}]}`
	if _, err := ParsePreset([]byte(good)); err != nil {
		t.Fatalf("ParsePreset: %v", err)
	}
	bad := map[string]string{
		"unknown key":      strings.Replace(good, `"domain_suffix"`, `"geosite"`, 1),
		"no condition":     strings.Replace(good, `,"domain_suffix":["example.com"]`, ``, 1),
		"unknown outbound": strings.Replace(good, `"outbound":"proxy"`, `"outbound":"tor"`, 1),
		"bad cidr":         strings.Replace(good, `"domain_suffix":["example.com"]`, `"ip_cidr":["10.0.0.0/33"]`, 1),
		"bad port range":   strings.Replace(good, `"domain_suffix":["example.com"]`, `"port_range":["9000:80"]`, 1),
		"bad regex":        strings.Replace(good, `"domain_suffix":["example.com"]`, `"domain_regex":["(ads"]`, 1),
		"bad network":      strings.Replace(good, `"domain_suffix":["example.com"]`, `"network":["icmp"]`, 1),
		"bad direct dns":   strings.Replace(good, `"default_outbound":"direct"`, `"default_outbound":"direct","direct_dns":"dns.google"`, 1),
		"other kind":       strings.Replace(good, `poros-route-profile`, `throne-route-profile`, 1),
	}
	for name, text := range bad {
		if _, err := ParsePreset([]byte(text)); err == nil {
			t.Errorf("%s: ParsePreset must refuse it", name)
		}
	}
}

func golden(t *testing.T, name string, data []byte) {
	t.Helper()
	var pretty bytes.Buffer
	if err := json.Indent(&pretty, data, "", "  "); err != nil {
		t.Fatal(err)
	}
	pretty.WriteByte('\n')
	path := filepath.Join("testdata", name)
	if *update {
		if err := os.MkdirAll("testdata", 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(path, pretty.Bytes(), 0o644); err != nil {
			t.Fatal(err)
		}
		return
	}
	want, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("%v; run the test with -update", err)
	}
	if !bytes.Equal(pretty.Bytes(), want) {
		t.Errorf("%s differs from the golden file; if the change is right, run the test with -update\n%s", name, pretty.Bytes())
	}
}

func TestBuildTestMeasuresEveryServer(t *testing.T) {
	// The engine looks the names up past any TUN; sing-box answers them from that list.
	in := TestInput{RealityOnXray: true, Interface: "Ethernet", Resolved: map[string]netip.Addr{
		"example.com": netip.MustParseAddr("192.0.2.1"), "server.example": netip.MustParseAddr("192.0.2.2"),
	}}
	port := uint16(30000)
	for name, link := range sampleLinks(t) {
		port++
		in.Servers = append(in.Servers, TestServer{
			Tag: name, Server: mustParse(t, link), Bridge: Bridge{Port: port, Username: "u", Password: "p"},
		})
	}
	broken := mustParse(t, sampleLinks(t)["trojan tcp"])
	broken.Trojan.Password = ""
	in.Servers = append(in.Servers, TestServer{Tag: "broken", Server: broken})
	configs, err := BuildTest(in)
	if err != nil {
		t.Fatal(err)
	}
	if len(configs.Failed) != 1 || configs.Failed["broken"] == nil {
		t.Errorf("only the broken server fails: %v", configs.Failed)
	}
	tags := map[string]bool{}
	for _, outbound := range configs.SingBox.Outbounds {
		tags[outbound.Tag] = true
	}
	for _, endpoint := range configs.SingBox.Endpoints {
		tags[endpoint.Tag] = true
	}
	for _, server := range in.Servers {
		if !tags[server.Tag] != (server.Tag == "broken") {
			t.Errorf("a working server, and only one, is an outbound or endpoint under its tag: %s", server.Tag)
		}
	}
	data, err := configs.SingBoxJSON()
	if err != nil {
		t.Fatal(err)
	}
	if err = boxmain.Check(data); err != nil {
		t.Fatalf("sing-box refuses the test config: %v", err)
	}
	if resolver := configs.SingBox.Route.DefaultDomainResolver; resolver == nil || resolver.Server != dnsTestHosts {
		t.Errorf("the servers' names must come from the engine's lookups: %+v", resolver)
	}
	if !strings.Contains(string(data), `"server.example":"192.0.2.2"`) && !strings.Contains(string(data), `"server.example":["192.0.2.2"]`) {
		t.Error("the looked-up addresses must be in the config")
	}
	if configs.Xray == nil {
		t.Fatal("the servers on Xray need an Xray config")
	}
	if err = xray.CheckXrayConfig(string(configs.Xray)); err != nil {
		t.Fatalf("Xray refuses the test config: %v", err)
	}
	if !strings.Contains(string(configs.Xray), `"inboundTag":["bridge-vless reality vision"]`) {
		t.Error("each bridge must route to its own server")
	}
}

func TestPresetDNS(t *testing.T) {
	server := mustParse(t, sampleLinks(t)["trojan tcp"])
	for _, dns := range []string{"https://1.1.1.1/dns-query", "https://dns.quad9.net", "tls://9.9.9.9", "tls://dns.google:853", "8.8.4.4"} {
		preset := allPreset
		preset.DNS = dns
		configs, err := Build(Input{Server: server, Preset: preset, MixedPort: 2090})
		if err != nil {
			t.Errorf("%s: %v", dns, err)
			continue
		}
		data, _ := configs.SingBoxJSON()
		if err = boxmain.Check(data); err != nil {
			t.Errorf("%s: sing-box refuses it: %v", dns, err)
		}
	}
	for _, dns := range []string{"ftp://1.1.1.1", "dns.google", "https://", "https://user@1.1.1.1/q", "tls://1.1.1.1/path"} {
		preset := allPreset
		preset.DNS = dns
		if _, err := Build(Input{Server: server, Preset: preset, MixedPort: 2090}); err == nil {
			t.Errorf("%q must be refused", dns)
		}
	}
}

func TestRefusalNeedsNoServer(t *testing.T) {
	configs, err := Build(Input{Refuse: true, Preset: allPreset, Tun: true, MixedPort: 2090, RealityOnXray: true})
	if err != nil {
		t.Fatal(err)
	}
	if configs.Xray != nil {
		t.Error("a refusal runs no Xray")
	}
	proxy := configs.SingBox.Outbounds[0]
	if proxy.Tag != TagProxy || proxy.Type != C.TypeBlock {
		t.Errorf("the server must be a refusal: %s %s", proxy.Type, proxy.Tag)
	}
	data, _ := configs.SingBoxJSON()
	if err = boxmain.Check(data); err != nil {
		t.Errorf("sing-box refuses it: %v", err)
	}
}

// Rules of an advanced preset: every condition must match, so a rule on two kinds of destination is built of rules
// apart; an inverted rule takes the rest, and the names follow the rules they can: those on names and programs.
func TestAdvancedRules(t *testing.T) {
	preset := Preset{
		Name: "Advanced", DefaultOutbound: OutboundProxy, DirectDNS: "tls://dns.example:853",
		Rules: []Rule{
			allPreset.Rules[0],
			{Name: "Video words", Action: ActionRoute, Outbound: OutboundDirect, DomainSuffix: []string{"example.com"}, DomainKeyword: []string{"video"}},
			{Name: "Web of a site", Action: ActionRoute, Outbound: OutboundDirect, DomainSuffix: []string{"example.org"}, Port: []uint16{443}, PortRange: []string{"8000:8080"}},
			{Name: "Not mine", Action: ActionRoute, Outbound: OutboundProxy, DomainSuffix: []string{"mine.example"}, Invert: true},
			{Name: "Games", Action: ActionRoute, Outbound: OutboundDirect, ProcessPathRegex: []string{`(?i)\\games\\`}},
			{Name: "QUIC", Action: ActionReject, Network: []string{"udp"}, Protocol: []string{"quic"}},
			{Name: "IPv6 of the LAN", Action: ActionRoute, Outbound: OutboundDirect, IPVersion: 6, SourceIPCIDR: []string{"192.168.1.0/24"}},
			allPreset.Rules[1],
		},
	}
	configs, err := Build(Input{Server: mustParse(t, sampleLinks(t)["trojan tcp"]), Preset: preset, Tun: true, MixedPort: 2090})
	if err != nil {
		t.Fatal(err)
	}
	data, err := configs.SingBoxJSON()
	if err != nil {
		t.Fatal(err)
	}
	if err = boxmain.Check(data); err != nil {
		t.Fatalf("sing-box refuses the config: %v", err)
	}
	route := configs.SingBox.Route.Rules
	// sniff, the DNS rule, then the user's.
	words, web, notMine := route[2], route[3], route[4]
	if words.Type != C.RuleTypeLogical || words.LogicalOptions.Mode != C.LogicalTypeAnd || len(words.LogicalOptions.Rules) != 2 ||
		words.LogicalOptions.RouteOptions.Outbound != TagDirect {
		t.Errorf("a site and a word must both match: %+v", words)
	}
	if web.Type != C.RuleTypeDefault || len(web.DefaultOptions.Port) != 1 || web.DefaultOptions.PortRange[0] != "8000:8080" {
		t.Errorf("a site and its ports are one rule: %+v", web)
	}
	if !notMine.DefaultOptions.Invert {
		t.Errorf("inverted: %+v", notMine)
	}
	dns := configs.SingBox.DNS.Rules
	if len(dns) != 3 || dns[0].Type != C.RuleTypeLogical || !dns[1].DefaultOptions.Invert || dns[1].DefaultOptions.RouteOptions.Server != TagDNSRemote ||
		dns[2].DefaultOptions.ProcessPathRegex[0] != `(?i)\\games\\` {
		t.Errorf("names follow the rules on names and programs only: %+v", dns)
	}
	servers := configs.SingBox.DNS.Servers
	if len(servers) != 3 || servers[1].Tag != TagDNSDirect || servers[1].Type != C.DNSTypeTLS || servers[2].Tag != tagDNSSystem {
		t.Fatalf("the direct DNS of the preset, its name from the system's: %+v", servers)
	}
	if tls := servers[1].Options.(*option.RemoteTLSDNSServerOptions); tls.Detour != "" || tls.DomainResolver.Server != tagDNSSystem {
		t.Errorf("the direct DNS goes past the server: %+v", tls)
	}

	inverted := Preset{Name: "Not the office", DefaultOutbound: OutboundDirect, Rules: []Rule{
		{Action: ActionRoute, Outbound: OutboundProxy, IPCIDR: []string{"10.20.0.0/16"}, Invert: true},
	}}
	if excluded := tunExcluded(inverted); len(excluded) != 1 {
		t.Errorf("an inverted rule on addresses takes the private ranges too: %v", excluded)
	}
}

// Endpoints connect alongside the server: the networks they announce go through them before the user's rules, the
// names of their domains to the DNS they were given, and a rule sends them more, its names to their DNS too. A
// refusing connection has none, and refuses what the preset sends to them.
func TestEndpoints(t *testing.T) {
	links := sampleLinks(t)
	preset := Preset{
		Name: "Office", DefaultOutbound: OutboundProxy, Endpoints: []string{"ovpn", "oc"},
		Rules: []Rule{
			allPreset.Rules[0],
			{Name: "Wiki", Action: ActionRoute, Outbound: EndpointTag(1), DomainSuffix: []string{"wiki.example"}},
			{Name: "Lab", Action: ActionRoute, Outbound: EndpointTag(0), IPCIDR: []string{"203.0.113.0/24"}},
			allPreset.Rules[1],
		},
	}
	in := Input{
		Server: mustParse(t, links["trojan tcp"]), Preset: preset, Tun: true, MixedPort: 2090,
		Endpoints: []profile.Profile{mustParse(t, links["openvpn"]), mustParse(t, links["openconnect"])},
	}
	configs, err := Build(in)
	if err != nil {
		t.Fatal(err)
	}
	data, err := configs.SingBoxJSON()
	if err != nil {
		t.Fatal(err)
	}
	if err = boxmain.Check(data); err != nil {
		t.Fatalf("sing-box refuses the config: %v\n%s", err, data)
	}
	endpoints := configs.SingBox.Endpoints
	if len(endpoints) != 2 || endpoints[0].Type != C.TypeOpenVPNClient || endpoints[0].Tag != "endpoint-1" ||
		endpoints[1].Type != C.TypeOpenConnect || endpoints[1].Tag != "endpoint-2" {
		t.Fatalf("an endpoint per profile: %+v", endpoints)
	}
	route := configs.SingBox.Route.Rules
	// sniff, the DNS rule, the networks of each endpoint, then the user's.
	for i, tag := range []string{"endpoint-1", "endpoint-2"} {
		rule := route[2+i].DefaultOptions
		if len(rule.PreferredBy) != 1 || rule.PreferredBy[0] != tag || rule.RouteOptions.Outbound != tag {
			t.Errorf("the networks of %s go through it: %+v", tag, rule)
		}
	}
	if wiki := route[4].DefaultOptions; wiki.RouteOptions.Outbound != "endpoint-2" || wiki.DomainSuffix[0] != "wiki.example" {
		t.Errorf("a rule sends a site to an endpoint: %+v", wiki)
	}
	dns := configs.SingBox.DNS
	if len(dns.Servers) != 4 || dns.Servers[2].Type != C.DNSTypeOpenVPN || dns.Servers[3].Type != C.DNSTypeOpenConnect {
		t.Fatalf("each endpoint has its DNS: %+v", dns.Servers)
	}
	if ovpn := dns.Servers[2].Options.(*option.OpenVPNDNSServerOptions); ovpn.Endpoint != "endpoint-1" || !ovpn.AcceptDefaultResolvers {
		t.Errorf("the DNS the OpenVPN server gave: %+v", ovpn)
	}
	if len(dns.Rules) != 3 || dns.Rules[0].DefaultOptions.PreferredBy[0] != "dns-endpoint-1" || dns.Rules[1].DefaultOptions.PreferredBy[0] != "dns-endpoint-2" ||
		dns.Rules[2].DefaultOptions.RouteOptions.Server != "dns-endpoint-2" {
		t.Errorf("the domains of an endpoint, then the names a rule sends it, go to its DNS: %+v", dns.Rules)
	}
	// The private networks behind an endpoint must reach the rules.
	for _, prefix := range configs.SingBox.Inbounds[1].Options.(*option.TunInboundOptions).RouteExcludeAddress {
		if prefix.Contains(netip.MustParseAddr("10.8.0.1")) || prefix.Contains(netip.MustParseAddr("192.168.10.1")) {
			t.Errorf("with endpoints, %s must stay in the TUN", prefix)
		}
	}

	in.Refuse, in.Server = true, profile.Profile{}
	refusing, err := Build(in)
	if err != nil {
		t.Fatal(err)
	}
	if len(refusing.SingBox.Endpoints) != 0 || len(refusing.SingBox.DNS.Servers) != 2 {
		t.Errorf("a refusal has no endpoints: %+v", refusing.SingBox.Endpoints)
	}
	if wiki := refusing.SingBox.Route.Rules[2].DefaultOptions; wiki.RouteOptions.Outbound != TagProxy {
		t.Errorf("a refusal refuses what goes to an endpoint: %+v", wiki)
	}
	data, _ = refusing.SingBoxJSON()
	if err = boxmain.Check(data); err != nil {
		t.Errorf("sing-box refuses the refusal: %v", err)
	}

	in.Refuse, in.Server = false, mustParse(t, links["trojan tcp"])
	for name, wrong := range map[string]Input{
		"a missing profile": {Server: in.Server, Preset: preset, MixedPort: 2090, Endpoints: in.Endpoints[:1]},
		"no VPN":            {Server: in.Server, Preset: preset, MixedPort: 2090, Endpoints: []profile.Profile{in.Server, in.Endpoints[1]}},
		"no such endpoint": {Server: in.Server, MixedPort: 2090, Preset: Preset{Name: "x", DefaultOutbound: OutboundProxy,
			Rules: []Rule{{Action: ActionRoute, Outbound: EndpointTag(0), DomainSuffix: []string{"example.com"}}}}},
	} {
		if _, err := Build(wrong); err == nil {
			t.Errorf("%s must be refused", name)
		}
	}
}

// The DNS page of the settings: ready answers, the hosts file and FakeIP before the rules of the preset, the cache
// and the family first; all names through the server or past it without rules; an own dns object instead of it all.
func TestDNSSettings(t *testing.T) {
	answers, err := ParseAnswers("# home\nNAS.home 192.168.1.20 fd00::20\n\nprinter.lan 192.168.1.30")
	if err != nil || len(answers) != 2 || answers[0].Name != "nas.home" || len(answers[0].Addresses) != 2 {
		t.Fatalf("ParseAnswers: %+v, %v", answers, err)
	}
	for _, wrong := range []string{"nas.home", "nas.home 192.168.1.300", "a.home 10.0.0.1\nA.home 10.0.0.2"} {
		if _, err := ParseAnswers(wrong); err == nil {
			t.Errorf("%q must be refused", wrong)
		}
	}
	preset := Preset{Name: "Mine", DefaultOutbound: OutboundProxy, Rules: []Rule{
		allPreset.Rules[0],
		{Name: "Bank", Action: ActionRoute, Outbound: OutboundDirect, DomainSuffix: []string{"bank.example"}},
		allPreset.Rules[1],
	}}
	server := mustParse(t, sampleLinks(t)["trojan tcp"])
	in := Input{Server: server, Preset: preset, Tun: true, MixedPort: 2090, DNS: DNS{
		FakeIP: true, Hosts: true, Answers: answers, CacheSize: 4096, IgnoreTTL: true, Optimistic: true, Timeout: 5 * time.Second,
		ClientSubnet: netip.MustParsePrefix("203.0.113.0/24"), AddressFirst: AddressIPv6,
	}}
	build := func(in Input) option.Options {
		t.Helper()
		configs, err := Build(in)
		if err != nil {
			t.Fatal(err)
		}
		data, _ := configs.SingBoxJSON()
		if err = boxmain.Check(data); err != nil {
			t.Fatalf("sing-box refuses the config: %v\n%s", err, data)
		}
		return configs.SingBox
	}
	options := build(in)
	dns := options.DNS
	// Two answers of two families, the hosts file, FakeIP, then the rule of the preset.
	if len(dns.Rules) != 7 || dns.Rules[0].DefaultOptions.Action != C.RuleActionTypePredefined || dns.Rules[0].DefaultOptions.Domain[0] != "nas.home" ||
		dns.Rules[4].DefaultOptions.RouteOptions.Server != tagDNSHosts || dns.Rules[5].DefaultOptions.RouteOptions.Server != tagDNSFake ||
		dns.Rules[6].DefaultOptions.RouteOptions.Server != TagDNSDirect {
		t.Fatalf("answers, hosts and FakeIP come before the rules: %+v", dns.Rules)
	}
	if answer := dns.Rules[1].DefaultOptions.PredefinedOptions.Answer; len(answer) != 1 || !strings.Contains(answer[0].String(), "fd00::20") {
		t.Errorf("an AAAA query gets the IPv6 address alone: %+v", answer)
	}
	fake := dns.Servers[len(dns.Servers)-1].Options.(*option.FakeIPDNSServerOptions)
	if fake.Inet4Range.Build(netip.Prefix{}) != fakeIPRange4 || fake.Inet6Range == nil {
		t.Errorf("FakeIP ranges: %+v", fake)
	}
	client := dns.DNSClientOptions
	// An optimistic cache needs answers that get outdated.
	if client.CacheCapacity != 4096 || !client.DisableExpire || client.Optimistic != nil || client.Timeout != badoption.Duration(5*time.Second) ||
		client.ClientSubnet == nil || client.Strategy != option.DomainStrategy(C.DomainStrategyPreferIPv6) {
		t.Errorf("the cache, the wait, ECS and IPv6 first: %+v", client)
	}
	in.DNS.IgnoreTTL = false
	if client := build(in).DNS.DNSClientOptions; client.Optimistic == nil || !client.Optimistic.Enabled {
		t.Errorf("an optimistic cache: %+v", client)
	}
	if options.Route.DefaultDomainResolver.Strategy != option.DomainStrategy(C.DomainStrategyPreferIPv6) {
		t.Errorf("connections take IPv6 first too: %+v", options.Route.DefaultDomainResolver)
	}

	// FakeIP needs the TUN; without IPv6 it has no IPv6 range, and IPv4 is all there is.
	in.Tun, in.DNS.Hosts, in.DNS.Answers = false, false, nil
	if servers := build(in).DNS.Servers; len(servers) != 2 {
		t.Errorf("FakeIP without the TUN: %+v", servers)
	}
	in.Tun, in.DisableIPv6 = true, true
	options = build(in)
	if fake := options.DNS.Servers[2].Options.(*option.FakeIPDNSServerOptions); fake.Inet6Range != nil ||
		options.DNS.Strategy != option.DomainStrategy(C.DomainStrategyIPv4Only) {
		t.Errorf("without IPv6: %+v, %v", fake, options.DNS.Strategy)
	}

	// All names one way: no rules of the preset. Through the server, the local proxy's direct names go there too;
	// a refusal resolves as the traffic goes.
	in.DNS = DNS{Routing: RoutingProxy}
	options = build(in)
	if len(options.DNS.Rules) != 0 || options.DNS.Final != TagDNSRemote {
		t.Errorf("all through the server: %+v", options.DNS)
	}
	if direct := options.Outbounds[len(options.Outbounds)-1].Options.(*option.DirectOutboundOptions); direct.DomainResolver == nil ||
		direct.DomainResolver.Server != TagDNSRemote {
		t.Errorf("direct names through the server: %+v", direct)
	}
	in.DNS.Routing = RoutingDirect
	if options = build(in); len(options.DNS.Rules) != 0 || options.DNS.Final != TagDNSDirect {
		t.Errorf("all past the server: %+v", options.DNS)
	}
	refusal := in
	refusal.Refuse, refusal.Server, refusal.DNS.Routing = true, profile.Profile{}, RoutingProxy
	if options = build(refusal); len(options.DNS.Rules) != 1 {
		t.Errorf("a refusal resolves as the traffic goes: %+v", options.DNS.Rules)
	}

	// An own dns object replaces it all; the addresses of servers resolve with the system's DNS.
	in.DNS = DNS{Object: `{"servers":[{"type":"https","tag":"remote","server":"1.1.1.1","detour":"proxy"},{"type":"local","tag":"local"}],` +
		`"rules":[{"domain_suffix":["lan"],"server":"local"}],"final":"remote"}`}
	options = build(in)
	if len(options.DNS.Servers) != 3 || options.DNS.Servers[2].Tag != TagDNSDirect || options.DNS.Final != "remote" {
		t.Errorf("the own object: %+v", options.DNS)
	}
	for _, wrong := range []string{
		`{"servers":[{"type":"https","tag":"remote","server":"1.1.1.1","oops":1}]}`,
		`{"servers":[{"type":"https","tag":"remote","server":"1.1.1.1","detour":"proxi"}]}`,
		`{"servers":[{"type":"https","tag":"remote","server":"1.1.1.1"}],"final":"nowhere"}`,
		`{"servers":[{"type":"local","tag":"local"}],"rules":[{"type":"logical","mode":"and","rules":[{"domain":["a.lan"]},{"query_type":["A"]}],"server":"lan"}]}`,
		`not json`,
	} {
		in.DNS.Object = wrong
		if _, err := Build(in); err == nil {
			t.Errorf("%s must be refused", wrong)
		}
	}
}

// WARP is the exit of the rules to it, behind the server: WireGuard with its reserved bytes, or MASQUE that shows
// Cloudflare a certificate of its key and trusts Cloudflare's key alone. Off, or in a refusal, the rules to it go
// to the server.
func TestWARP(t *testing.T) {
	preset := Preset{Name: "WARP", DefaultOutbound: OutboundProxy, Rules: []Rule{
		allPreset.Rules[0],
		{Name: "AI", Action: ActionRoute, Outbound: OutboundWARP, DomainSuffix: []string{"chatgpt.com"}},
		allPreset.Rules[1],
	}}
	in := Input{Server: mustParse(t, sampleLinks(t)["trojan tcp"]), Preset: preset, Tun: true, MixedPort: 2090}
	wireGuard, err := ParseWARP(WARPWireGuard, WARPDevice{
		Endpoint: "162.159.192.1:2408", PrivateKey: testWGPrivate, PeerKey: testWGPublic,
		Addresses: "172.16.0.2/32, 2606:4700:110:8a2c::2/128", Reserved: "12, 85, 201",
	}, true)
	if err != nil {
		t.Fatal(err)
	}
	build := func(in Input) option.Options {
		t.Helper()
		configs, err := Build(in)
		if err != nil {
			t.Fatal(err)
		}
		data, _ := configs.SingBoxJSON()
		if err = boxmain.Check(data); err != nil {
			t.Fatalf("sing-box refuses the config: %v\n%s", err, data)
		}
		return configs.SingBox
	}
	in.WARP = wireGuard
	options := build(in)
	warp := options.Endpoints[len(options.Endpoints)-1]
	wg, ok := warp.Options.(*option.WireGuardEndpointOptions)
	if warp.Tag != TagWARP || !ok || wg.Detour != TagProxy || len(wg.Peers) != 1 || !slices.Equal(wg.Peers[0].Reserved, []uint8{12, 85, 201}) || wg.MTU != 1280 {
		t.Fatalf("WireGuard behind the server: %+v", warp)
	}
	if rule := options.Route.Rules[2].DefaultOptions; rule.RouteOptions.Outbound != TagWARP {
		t.Errorf("a rule sends a site through WARP: %+v", rule)
	}

	private, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	server, _ := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	keyDER, _ := x509.MarshalECPrivateKey(private)
	peerDER, _ := x509.MarshalPKIXPublicKey(&server.PublicKey)
	in.WARP, err = ParseWARP(WARPMASQUE, WARPDevice{
		Endpoint: "162.159.198.2:443", PrivateKey: base64.StdEncoding.EncodeToString(keyDER), PeerKey: base64.StdEncoding.EncodeToString(peerDER),
		Addresses: "172.16.0.2/32", HTTP: WARPHTTP3,
	}, true)
	if err != nil {
		t.Fatal(err)
	}
	options = build(in)
	masque, ok := options.Endpoints[len(options.Endpoints)-1].Options.(*option.MASQUEClientEndpointOptions)
	if !ok || masque.Detour != TagProxy || masque.TLS.ServerName != warpServerName || len(masque.TLS.ClientCertificate) != 1 ||
		len(masque.TLS.CertificatePublicKeySHA256) != 1 || masque.Version != 3 || !masque.DisableVersionFallback {
		t.Fatalf("MASQUE behind the server: %+v", masque)
	}

	// Off, and in a refusal, WARP's rules go to the server.
	in.WARP = nil
	if options = build(in); options.Route.Rules[2].DefaultOptions.RouteOptions.Outbound != TagProxy || len(options.Endpoints) != 0 {
		t.Errorf("without WARP: %+v", options.Route.Rules[2])
	}
	in.WARP, in.Refuse, in.Server = wireGuard, true, profile.Profile{}
	if options = build(in); len(options.Endpoints) != 0 {
		t.Errorf("a refusal has no WARP: %+v", options.Endpoints)
	}

	for name, device := range map[string]WARPDevice{
		"no port":          {Endpoint: "162.159.192.1"},
		"no key":           {PrivateKey: "short"},
		"two reserved":     {Reserved: "1, 2"},
		"a reserved byte":  {Reserved: "1, 2, 300"},
		"no address":       {Addresses: "172.16.0.300/32"},
		"missing in whole": {Endpoint: "162.159.192.1:2408"},
	} {
		if _, err := ParseWARP(WARPWireGuard, device, name == "missing in whole"); err == nil {
			t.Errorf("%s must be refused", name)
		}
	}
	if _, err := ParseWARP(WARPMASQUE, WARPDevice{PrivateKey: testWGPrivate}, false); err == nil {
		t.Error("MASQUE takes an ECDSA key")
	}
}
