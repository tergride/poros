package config

import (
	"bytes"
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
	}
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

	excluded := tunExcluded(preset)
	if slices.Contains(excluded, netip.MustParsePrefix("10.0.0.0/8")) || !slices.Contains(excluded, netip.MustParsePrefix("192.168.0.0/16")) {
		t.Errorf("a range a rule sends to the server must reach the TUN, the rest stays out: %v", excluded)
	}
	dns := configs.SingBox.DNS.Rules
	if len(dns) != 2 || dns[0].DefaultOptions.RouteOptions.Server != dnsDirect || dns[0].DefaultOptions.DomainSuffix[1] != ".xn--p1ai" ||
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
		"unknown outbound": strings.Replace(good, `"outbound":"proxy"`, `"outbound":"warp"`, 1),
		"bad cidr":         strings.Replace(good, `"domain_suffix":["example.com"]`, `"ip_cidr":["10.0.0.0/33"]`, 1),
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
	if got := len(configs.SingBox.Outbounds); got != len(in.Servers) {
		t.Errorf("an outbound per working server and direct: %d", got)
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
