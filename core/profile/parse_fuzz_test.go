package profile

import (
	"encoding/base64"
	"encoding/json"
	"runtime/debug"
	"strings"
	"testing"
	"time"
)

// hangLimit bounds one input. The fuzzing engine has no time limit of its own, and a parser that hangs on what a
// provider sends would hang the core.
const hangLimit = 10 * time.Second

// watchHang crashes the process when an input runs past hangLimit, which keeps the input as a crasher.
func watchHang() (stop func()) {
	timer := time.AfterFunc(hangLimit, func() {
		debug.SetTraceback("all")
		panic("the input ran longer than " + hangLimit.String())
	})
	return func() { timer.Stop() }
}

// storedForm is the JSON of a profile as Poros keeps it. JSON holds text in UTF-8, so a byte that is not UTF-8 in a
// name or a password comes back from a poros://add/ link as U+FFFD: a loss of the stored form itself, which every
// link of the profile shares.
func storedForm(t *testing.T, p Profile) string {
	t.Helper()
	data, err := json.Marshal(p)
	if err != nil {
		t.Fatalf("the profile has no JSON: %v", err)
	}
	var stored Profile
	if err = json.Unmarshal(data, &stored); err != nil {
		t.Fatalf("the JSON of the profile does not read back: %v", err)
	}
	data, _ = json.Marshal(stored)
	return string(data)
}

// checkLinksOf shares a profile Poros read and reads the links back: ShareLink and PorosLink must each give the same
// profile. The only change allowed is the host in lower case when ShareLink writes a link of the protocol; a
// poros://add/ link keeps the host as it was.
func checkLinksOf(t *testing.T, p Profile) {
	t.Helper()
	shared := ShareLink(p)
	sharedWant := p
	if host, err := normalizeLinkHost(p.Server); err == nil && !strings.HasPrefix(shared, "poros://add/") {
		sharedWant.Server = host
	}
	for _, check := range []struct {
		link string
		want string
	}{{shared, storedForm(t, sharedWant)}, {PorosLink(p), storedForm(t, p)}} {
		back, err := ParseLink(check.link)
		if err != nil {
			t.Fatalf("%s\nis shared as %q, which does not read back: %v", check.want, check.link, err)
		}
		if got := storedForm(t, back); got != check.want {
			t.Fatalf("%s\nis shared as %q, which reads as\n%s", check.want, check.link, got)
		}
	}
}

func encodeJSONLink(scheme, text string) string {
	return scheme + "://add/" + base64.RawURLEncoding.EncodeToString([]byte(text))
}

// Share links come from outside: a subscription, the clipboard, a QR code. Whatever the text, ParseLink gives a
// valid profile or an error, and the links Poros writes for a profile it read give that profile back.
func FuzzParseLink(f *testing.F) {
	for _, link := range roundTripLinks {
		f.Add(link)
	}
	b64 := func(text string) string { return base64.RawStdEncoding.EncodeToString([]byte(text)) }
	for _, link := range []string{
		"  vless://" + testUUID + "@DE1.Example.com:443?encryption=none&flow=xtls-rprx-vision&security=reality&sni=www.microsoft.com" +
			"&fp=chrome&pbk=" + testPBK + "%3D&sid=6ba85179e30d4fc2&spx=%2F&type=tcp&headerType=none#%F0%9F%87%A9%F0%9F%87%AA%20Germany\r\n",
		"vless://" + testUUID + "@x2.example.com:2053?encryption=mlkem768x25519plus.native.0rtt.abc&flow=xtls-rprx-vision&type=splithttp" +
			"&path=%2F&mode=bogus&extra=%7B%27xmux%27%3A+%7B%27maxConcurrency%27%3A+%2716-32%27%7D%2C+%27noSSEHeader%27%3A+True%7D",
		"vless://" + testUUID + "@[2001:DB8:0::1]:80?type=raw&headerType=http&host=a.example.com%2Cb.example.com&path=%2F%2C%2Fvideo" +
			"&packetEncoding=none&mux=true&mux_protocol=h2mux&mux_max_streams=8&mux_padding=true&mux_min_streams=-1&flow=bogus#v6",
		"vless://" + testUUID + "@h2.example.com:443?security=tls&sni=h2.example.com&type=h2&host=h2.example.com&path=%2Fh2&method=PUT" +
			"&ech=AEX%2BDQBBpQAgACB&tls_min_version=1.2&tls_max_version=1.4&tls_certificate_public_key_sha256=abc%3D%2C%20def%3D&disable_sni=1",
		"vmess://" + base64.RawURLEncoding.EncodeToString([]byte(`{"v":2,"ps":"","add":"203.0.113.20","port":8080,"id":"`+testUUID+
			`","aid":64,"scy":"CHACHA20-POLY1305","net":"grpc","path":"svc","tls":"","sni":"ignored.example.com",`+
			`"throneExtra":"mux=true&mux_protocol=smux&packetEncoding=none&security=tls"}`)) + "#From%20fragment",
		"vmess://" + testUUID + "@vm2.example.com?encryption=aes-128-gcm&type=grpc&serviceName=svc&security=tls&sni=vm2.example.com&alterId=2#URL",
		"trojan://p%40ss:w0rd@tr.example.com:443?security=tls&sni=tr.example.com&type=grpc&serviceName=trojan-grpc&allowInsecure=1" +
			"&ech=cloudflare-ech.com%2Bhttps%3A%2F%2F1.1.1.1%2Fdns-query#Trojan%20gRPC",
		"ss://Y2hhY2hhMjAtaWV0Zi1wb2x5MTMwNTpwYTpzc0AxOTguNTEuMTAwLjQ6ODM4OQ==#legacy%20form",
		"ss://QUVTLTEyOC1HQ006cGFzcw==@ss2.example.com:8388/?plugin=simple-obfs%3Bobfs%3Dhttp%3Bobfs-host%3Dwww.bing.com#obfs",
		"ss://Y2hhY2hhMjAtaWV0Zi1wb2x5MTMwNTpwdw@[2001:db8::5]:443?plugin=v2ray-plugin%3Bmode%3Dquic" +
			"&plugin-opts=mode%3Dwebsocket%3Btls%3Bhost%3Dcdn.example.com&mux=true#v6%20plugin",
		"hysteria2://letmein@hy.example.com:20000-30000/?sni=hy.example.com&obfs=salamander&obfs-password=gawr%2Bgura&insecure=1" +
			"&upmbps=50Mbps&downmbps=200&fp=chrome&pinSHA256=000102030405060708090A0B0C0D0E0F%3A101112131415161718191a1b1c1d1e1f#HY2",
		"hy2://hy3.example.com?obfs=salamander&upmbps=50%20MB%2Fs&downmbps=fast&hop_interval=30",
		"tuic://" + testUUID + ":pw@198.51.100.30?congestion_control=New%20Reno&udp_relay_mode=bogus&heartbeat=10",
		"socks://dXNlcjpwYXNz@198.51.100.9:1081",
		"socks4://proxy.example.com?version=6",
		"http://user:p%40ss@203.0.113.5:3128#office 100%",
		"https://alice:secret@proxy.example.com?sni=proxy.example.com&path=%2Fconnect&headers=X-Token,abc,X-Id,7",
		"http://:token@203.0.113.6:8080",
		"anytls://passw0rd@at.example.com:8443?sni=at.example.com&insecure=1&idle_session_timeout=60s&min_idle_session=2",
		"wg://" + strings.NewReplacer("+", "%2B", "/", "%2F", "=", "%3D").Replace(testWGPrivate) + "@wg.example.com:51820?publickey=" +
			testWGPublic + "&psk=" + testWGPublic + "&address=172.16.0.2,fd01::2/128&reserved=1-2-3&mtu=1280&keepalive=25#WG",
		"hysteria://hy.example.com:443?protocol=udp&auth=pw&peer=sni.example.com&upmbps=50&downmbps=200%20Mbps&obfs=xplus&obfsParam=ob",
		"naive+quic://user:pw@nv.example.com:443?sni=nv.example.com&congestion_control=bbr&extra-headers=X-Token%3A%20abc#NV",
		"ssh://ssh.example.com:2222?user=root&password=pw&private_key=" + b64(testSSHKey) + "&host_key=" + b64("ssh-ed25519 AAAAC3NzaC1lZDI1NTE5"),
		"snell://psk@sn.example.com:8443?version=4&reuse=1&obfs=http&obfs-host=bing.com#SN",
		encodeJSONLink("poros", `{"type":"trojan","name":"🇩🇪 Home","server":"DE.example.com","server_port":443,"trojan":{"password":"p",`+
			`"tls":{"enabled":true},"transport":{"type":"xhttp","xhttp":{"extra":{"a" : 1}}}}}`),
		encodeJSONLink("poros", `{"type":"chain","chain":{"servers":["a","b"]}}`),
		encodeJSONLink("throne", `{"type":"trojan","tag":"Throne server","server":"th.example.com","server_port":443,"password":"pw",`+
			`"tls":{"enabled":true,"server_name":"th.example.com"}}`),
		"vless://@v.example.com:443",
		"vless://" + testUUID + "@[2001:db8::1",
		"vless://" + testUUID + "@bad%20host:443",
		"ss://!!!#x",
		"hy2://pw@hy.example.com:20000-70000",
		"poros://add/%%%",
		"juicity://key@198.51.100.7:443",
	} {
		f.Add(link)
	}
	f.Fuzz(func(t *testing.T, link string) {
		defer watchHang()()
		p, err := ParseLink(link)
		if err != nil {
			return
		}
		if err = p.Validate(); err != nil {
			t.Fatalf("ParseLink gave a profile Validate refuses: %v\n%s", err, dump(p))
		}
		checkLinksOf(t, p)
	})
}

// A subscription body is whatever the provider sends. ParseDocument reads it or fails as a whole; every server it
// keeps is valid and shares into links that read back the same, and the skipped items are bounded.
func FuzzParseDocument(f *testing.F) {
	list := "vless://" + testUUID + "@a.example.com:443#A\nss://YWVzLTI1Ni1nY206cHc@ss.example.com:8388#B\n"
	for _, body := range []string{
		"\xef\xbb\xbf#profile-title: Provider\r\nСрок действия: до 2026-12-01\r\nvless://" + testUUID + "@a.example.com:443?security=tls#A\r\n\r\n" +
			"juicity://key@198.51.100.7:443#Juicity\r\ntrojan://@t.example.com#Broken\r\nhttps://t.me/provider\r\n" +
			"ss://YWVzLTI1Ni1nY206cHc@ss.example.com:8388#SS \r\n",
		base64.StdEncoding.EncodeToString([]byte(list)),
		"# servers\n" + base64.StdEncoding.EncodeToString([]byte(list)) + "\n" + base64.StdEncoding.EncodeToString([]byte("trojan://pw@t.example.com#C")),
		"#note\n" + base64.StdEncoding.EncodeToString([]byte("vless://"+testUUID+"@a.example.com#A\nvless://@b.example.com#Bad")),
		`{"version": 1, "servers": [{"remarks": "S1", "server": "[2001:db8::1]", "server_port": 8388, "password": "pw", "method": "AES-256-GCM",
			"plugin": "simple-obfs", "plugin_opts": "obfs=http"}, {"remarks": "S2", "server": "s2.example.com", "server_port": "8388"}]}`,
		`{"proxies": [{"name": "T", "type": "trojan", "server": "t.example.com", "port": 443, "password": "pw"}]}`,
		"mixed-port: 7890\nproxies:\n  - {name: DIRECT, type: direct}\n  - name: Good\n    type: trojan\n    server: t.example.com\n    port: 443\n" +
			"    password: secret\n  - {name: WG, type: wireguard, server: w.example.com, port: 51820, private-key: " + testWGPrivate +
			", public-key: " + testWGPublic + ", ip: 10.0.0.2}\n  - {name: Shadow-TLS, type: ss, server: st.example.com, port: 443, cipher: aes-128-gcm," +
			" password: secret, plugin: shadow-tls, plugin-opts: {host: cloud.tencent.com, password: secret}}\n" +
			"  - {name: X, type: vless, server: x.example.com, port: 443, uuid: " + testUUID + ", network: xhttp, tls: true," +
			" xhttp-opts: {path: /x, mode: stream-one}}\n  - ~\n  - just text\nproxy-groups:\n  - {name: PROXY, type: select, proxies: [Good]}\n",
		"proxies:\n  - &a {name: A, type: socks5, server: s.example.com, port: 1080}\n  - *a\n",
		`{"outbounds": [
			{"type": "selector", "tag": "proxy", "outbounds": ["DE"]},
			{"type": "vless", "tag": "DE", "server": "de.example.com", "server_port": 443, "uuid": "` + testUUID + `", "flow": "xtls-rprx-vision",
				"tls": {"enabled": true, "server_name": "www.microsoft.com", "utls": {"enabled": true, "fingerprint": "chrome"},
				"reality": {"enabled": true, "public_key": "` + testPBK + `", "short_id": "ab"}}},
			{"type": "shadowtls", "tag": "st", "server": "st.example.com", "server_port": 443, "version": 3, "password": "s3cret",
				"tls": {"enabled": true, "server_name": "www.apple.com"}},
			{"type": "shadowsocks", "tag": "SS", "detour": "st", "method": "2022-blake3-aes-128-gcm", "password": "AAECAwQFBgcICQoLDA0ODw=="},
			{"type": "hysteria", "tag": "HY", "server": "hy.example.com", "server_port": 443, "auth_str": "pw", "up": "50 Mbps", "down_mbps": 200},
			{"type": "wireguard", "tag": "WG", "server": "wg.example.com", "server_port": 51820, "local_address": ["172.16.0.2/32"],
				"private_key": "` + testWGPrivate + `", "peer_public_key": "` + testWGPublic + `", "reserved": "AQID", "mtu": 1280}],
		 "endpoints": [{"type": "openvpn-client", "tag": "OVPN", "servers": [{"server": "o1.example.com", "server_port": 1194, "network": "tcp"}],
			"tls": {"certificate": ["` + strings.ReplaceAll(testCA, "\n", `", "`) + `"], "server_name": "o.example.com"}},
			{"type": "openconnect", "tag": "Office", "server": "https://gw.example.com:8443/staff", "flavor": "gp", "username": "u", "password": "pw"},
			{"type": "masque-client", "tag": "MASQUE", "server": "m.example.com", "username": "u", "password": "pw", "version": 3,
				"tls": {"enabled": true, "server_name": "m.example.com"}}]}`,
		`{"type": "hysteria2", "tag": "Realm", "password": "pw", "tls": {"enabled": true, "server_name": "hy.example.com"},
			"realm": {"server_url": "https://realm.example.com:8443", "token": "t", "realm_id": "home"}}`,
		xrayPanelSubscription,
		`{"outbounds": [{"tag": "de", "protocol": "vless", "settings": {"address": "de.example.com", "port": 443, "id": "` + testUUID + `",
			"encryption": "none"}, "streamSettings": {"network": "xhttp", "security": "tls", "tlsSettings": {"serverName": "de.example.com"},
			"xhttpSettings": {"path": "/x", "extra": {"downloadSettings": {"address": "dl.example.com", "port": 443}}}}},
			{"tag": "ss", "protocol": "shadowsocks", "settings": {"servers": [{"address": "ss.example.com", "port": 8388, "method": "aes-128-gcm", "password": "pw"}]}},
			{"tag": "wg", "protocol": "wireguard", "settings": {"secretKey": "` + testWGPrivate + `", "address": ["10.0.0.2"],
				"peers": [{"publicKey": "` + testWGPublic + `", "endpoint": "wg.example.com:51820"}]}},
			{"tag": "direct", "protocol": "freedom"}]}`,
		`[{"protocol": "socks", "tag": "s1", "settings": {"servers": [{"address": "198.51.100.1", "port": 1080}]}},
			{"protocol": "http", "tag": "h1", "settings": {"address": "198.51.100.2", "port": 8080, "user": "u", "pass": "p"}}]`,
		"[Interface]\r\nPrivateKey = " + testWGPrivate + "\r\nAddress = 172.16.0.2/32, fd01::2/128\r\nDNS = 1.1.1.1\r\nMTU = 1280\r\n\r\n" +
			"[Peer]\r\nPublicKey = " + testWGPublic + "\r\nPresharedKey = " + testWGPublic + " # the provider's\r\n" +
			"AllowedIPs = 0.0.0.0/0, ::/0\r\nEndpoint = [2001:db8::1]:51820\r\nPersistentKeepalive = 25\r\n",
		"# OVPN_FRIENDLY_PROFILE_NAME=Work VPN\r\nclient\r\ndev tun\r\nproto udp\r\nremote vpn1.example.com 1194\r\n" +
			"remote vpn2.example.com 443 tcp-client\r\nremote-random\r\nverify-x509-name \"server name\" name\r\ncipher AES-256-GCM\r\n" +
			"data-ciphers AES-256-GCM:CHACHA20-POLY1305\r\nkeepalive 10 60\r\nauth-user-pass\r\nstatic-challenge \"Enter the code\" 1\r\n" +
			"key-direction 1 ; the client's side\r\n<ca>\r\n" + testCA + "\r\n</ca>\r\n<tls-auth>\r\n" + testTLSCrypt + "\r\n</tls-auth>\r\n",
		"client\ndev tun\n<connection>\nremote a.example.com 1194 udp\n</connection>\n<connection>\nremote b.example.com 443\nproto tcp\n</connection>\n" +
			"<ca>\n" + testCA + "\n</ca>\n<cert>\n" + testCA + "\n</cert>\n<key>\n" + testClientKey + "\n</key>\n",
		"{\n  \"outbounds\": [\n  }",
		`{"servers": {}}`,
		"[]",
	} {
		f.Add([]byte(body))
	}
	f.Fuzz(func(t *testing.T, body []byte) {
		defer watchHang()()
		d, err := ParseDocument(body)
		if err != nil {
			return
		}
		if len(d.Skipped) > maxSkipped || d.SkippedCount < len(d.Skipped) {
			t.Fatalf("%d skipped items keep %d reasons", d.SkippedCount, len(d.Skipped))
		}
		for _, p := range d.Profiles {
			if err := p.Validate(); err != nil {
				t.Fatalf("ParseDocument kept a profile Validate refuses: %v\n%s", err, dump(p))
			}
			checkLinksOf(t, p)
		}
	})
}
