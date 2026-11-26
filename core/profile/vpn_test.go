package profile

import (
	"errors"
	"reflect"
	"strings"
	"testing"
)

const (
	testCA        = "-----BEGIN CERTIFICATE-----\nMIIBszCCAVmgAwIBAgIUQ0E=\n-----END CERTIFICATE-----"
	testClientKey = "-----BEGIN PRIVATE KEY-----\nMIGHAgEAMBMGByqGSM49=\n-----END PRIVATE KEY-----"
	testTLSCrypt  = "-----BEGIN OpenVPN Static key V1-----\n6acef03f62675b4b1bbd03e53b187727\n-----END OpenVPN Static key V1-----"
)

func TestOpenVPNConfigFile(t *testing.T) {
	config := "# OVPN_FRIENDLY_PROFILE_NAME=Work VPN\r\nclient\r\ndev tun\r\nproto udp\r\n" +
		"remote vpn1.example.com 1194\r\nremote vpn2.example.com 443 tcp-client\r\nremote-random\r\nresolv-retry infinite\r\nnobind\r\n" +
		"remote-cert-tls server\r\nverify-x509-name \"server name\" name\r\ncipher AES-256-GCM\r\n" +
		"data-ciphers AES-256-GCM:CHACHA20-POLY1305\r\nauth SHA256\r\ncompress lz4-v2\r\ntun-mtu 1400\r\nkeepalive 10 60\r\n" +
		"auth-user-pass\r\nstatic-challenge \"Enter the code\" 1\r\nkey-direction 1 ; the client's side\r\n" +
		"<ca>\r\n" + testCA + "\r\n</ca>\r\n<tls-auth>\r\n" + testTLSCrypt + "\r\n</tls-auth>\r\n"
	d := mustParseDocument(t, config)
	want := Profile{Type: TypeOpenVPN, Name: "Work VPN", Server: "vpn1.example.com", ServerPort: 1194, OpenVPN: &OpenVPN{
		Network:      "udp",
		Remotes:      []OpenVPNRemote{{Server: "vpn2.example.com", Port: 443, Network: "tcp"}},
		RemoteRandom: true, StaticChallenge: "Enter the code", StaticChallengeEcho: true,
		CA: testCA, ControlWrap: OpenVPNControlWrap{Type: "tls_auth", Key: testTLSCrypt, Direction: "client"},
		VerifyName: "server name", VerifyNameType: "name", RemoteCertTLS: "server", Cipher: "AES-256-GCM",
		DataCiphers: []string{"AES-256-GCM", "CHACHA20-POLY1305"}, Auth: "SHA256", Compression: "lz4-v2", MTU: 1400,
		PingInterval: "10s", PingRestart: "60s",
	}}
	if d.Format != FormatOpenVPN || len(d.Profiles) != 1 || !reflect.DeepEqual(d.Profiles[0], want) {
		if len(d.Profiles) == 1 {
			t.Errorf("\n got %s\nwant %s", dump(d.Profiles[0]), dump(want))
		}
		t.Fatalf("format %q, skipped %+v", d.Format, d.Skipped)
	}

	connections := "client\ndev tun\n<connection>\nremote a.example.com 1194 udp\n</connection>\n<connection>\nremote b.example.com 443\nproto tcp\n</connection>\n" +
		"<ca>\n" + testCA + "\n</ca>\n<cert>\n" + testCA + "\n</cert>\n<key>\n" + testClientKey + "\n</key>\n"
	d = mustParseDocument(t, connections)
	if len(d.Profiles) != 1 || d.Profiles[0].Server != "a.example.com" ||
		!reflect.DeepEqual(d.Profiles[0].OpenVPN.Remotes, []OpenVPNRemote{{Server: "b.example.com", Port: 443, Network: "tcp"}}) ||
		d.Profiles[0].OpenVPN.Key != testClientKey {
		t.Errorf("connection blocks: %q, skipped %+v", names(d.Profiles), d.Skipped)
	}

	for name, tt := range map[string]struct {
		text        string
		want        string
		unsupported bool
	}{
		"a CA in a file":  {"client\nremote v.example.com\nca ca.crt\n", `file "ca.crt"`, false},
		"a TAP device":    {"client\ndev tap\nremote v.example.com\n<ca>\n" + testCA + "\n</ca>\n", "TAP", true},
		"a PKCS#12 key":   {"client\nremote v.example.com\npkcs12 me.p12\n", "PKCS#12", true},
		"a server config": {"dev tun\nmode server\nremote v.example.com\n", "server", false},
		"no CA":           {"client\nremote v.example.com\n", "CA", false},
		"an open block":   {"client\nremote v.example.com\n<ca>\n" + testCA + "\n", "no end", false},
		"static key mode": {"client\nremote v.example.com\nsecret static.key\n", "static key", true},
	} {
		d := mustParseDocument(t, tt.text)
		if len(d.Profiles) != 0 || len(d.Skipped) != 1 || !strings.Contains(d.Skipped[0].Reason, tt.want) || d.Skipped[0].Unsupported != tt.unsupported {
			t.Errorf("%s: %q, skipped %+v; want %q (unsupported %v)", name, names(d.Profiles), d.Skipped, tt.want, tt.unsupported)
		}
	}
}

// A config of OpenVPN|BY as its client writes one: the BTLS suites, belt and bash make the type, and the keys in files
// of their own are left for the user to choose, which a document cannot do.
func TestOpenVPNBYConfigFile(t *testing.T) {
	files := "ca CA_crt.pem\r\ncert client_crt.pem\r\nkey client_pr.pem\r\n"
	config := string(rune(0xFEFF)) + "# client\r\nclient\r\ndev tun\r\nproto tcp\r\nremote vpn.example.com 443\r\nresolv-retry infinite\r\nnobind\r\n" + files +
		"tls-cipher DHE-BIGN-WITH-BELT-DWP-HBELT\r\ncipher belt-cfb256\r\ndata-ciphers belt-cfb256\r\nauth bash512\r\n" +
		"tls-version-max 1.2\r\ntls-version-min 1.2\r\nauth-nocache\r\ndisable-dco\r\n"
	d := mustParseDocument(t, config)
	if len(d.Profiles) != 0 || len(d.Skipped) != 1 || !strings.Contains(d.Skipped[0].Reason, `file "CA_crt.pem"`) {
		t.Fatalf("a document has no folder to read the files from: %q, skipped %+v", names(d.Profiles), d.Skipped)
	}
	p, named, err := ReadOpenVPNConfig([]byte(config))
	want := Profile{Type: TypeOpenVPNBY, Server: "vpn.example.com", ServerPort: 443, OpenVPN: &OpenVPN{
		Network: "tcp", TLSCipher: []string{"DHE-BIGN-WITH-BELT-DWP-HBELT"}, TLSVersionMin: "1.2", TLSVersionMax: "1.2",
		Cipher: "belt-cfb256", DataCiphers: []string{"belt-cfb256"}, Auth: "bash512",
	}}
	if err != nil || !reflect.DeepEqual(p, want) || !named.CA || !named.Cert || !named.Key || named.WrapKey || named.Credentials {
		t.Fatalf("\n got %s, %+v, %v\nwant %s", dump(p), named, err, dump(want))
	}

	inline := strings.Replace(config, files, "<ca>\n"+testCA+"\n</ca>\n<cert>\n"+testCA+"\n</cert>\n<key>\n"+testClientKey+"\n</key>\n", 1)
	if d = mustParseDocument(t, inline); len(d.Profiles) != 1 || d.Profiles[0].Type != TypeOpenVPNBY || d.Profiles[0].OpenVPN.Key != testClientKey {
		t.Errorf("inline, it is a server: %q, skipped %+v", names(d.Profiles), d.Skipped)
	}
	if _, named, _ = ReadOpenVPNConfig([]byte(inline)); named.Any() {
		t.Errorf("inline, it names no files: %+v", named)
	}

	// Belt alone makes the type too, with the suites OpenVPN|BY offers.
	beltOnly := strings.Replace(inline, "tls-cipher DHE-BIGN-WITH-BELT-DWP-HBELT\r\n", "", 1)
	if p, _, err = ReadOpenVPNConfig([]byte(beltOnly)); err != nil || p.Type != TypeOpenVPNBY || !reflect.DeepEqual(p.OpenVPN.TLSCipher, btlsSuites) {
		t.Errorf("belt without tls-cipher: %s, %v", dump(p), err)
	}
	wrapped := inline + "tls-crypt-v2 client.key\r\nauth-user-pass login.txt\r\n"
	if p, named, err = ReadOpenVPNConfig([]byte(wrapped)); err != nil || !named.WrapKey || !named.Credentials || p.OpenVPN.ControlWrap.Type != "tls_crypt_v2" {
		t.Errorf("tls-crypt-v2 and auth-user-pass in files: %s, %+v, %v", dump(p), named, err)
	}
	plain := "client\nremote v.example.com\ntls-cipher TLS-ECDHE-RSA-WITH-AES-256-GCM-SHA384\n<ca>\n" + testCA + "\n</ca>\n"
	if p, _, err = ReadOpenVPNConfig([]byte(plain)); err != nil || p.Type != TypeOpenVPN || p.OpenVPN.TLSCipher != nil {
		t.Errorf("plain OpenVPN runs the suites of Go: %s, %v", dump(p), err)
	}
	if _, _, err = ReadOpenVPNConfig([]byte("vless://u@v.example.com:443")); err == nil {
		t.Error("a link is no .ovpn")
	}

	body := `{"endpoints": [{"type": "openvpn-client", "tag": "BY", "server": "b.example.com", "server_port": 443, "network": "tcp",
		"tls": {"certificate": "` + strings.ReplaceAll(testCA, "\n", `\n`) + `", "cipher": "DHE-BIGN-WITH-BELT-DWP-HBELT"},
		"data_ciphers": ["belt-cbc256"], "auth": "bash512"}]}`
	d = mustParseDocument(t, body)
	if len(d.Profiles) != 1 || d.Profiles[0].Type != TypeOpenVPNBY || !reflect.DeepEqual(d.Profiles[0].OpenVPN.TLSCipher, []string{"DHE-BIGN-WITH-BELT-DWP-HBELT"}) {
		t.Fatalf("a sing-box endpoint with BTLS: %q, skipped %+v", names(d.Profiles), d.Skipped)
	}
	if link := ShareLink(d.Profiles[0]); !reflect.DeepEqual(mustParseLink(t, link), d.Profiles[0]) {
		t.Errorf("%s does not read back", link)
	}
}

func TestReadVPNEndpoints(t *testing.T) {
	body := `{"endpoints": [
		{"type": "openvpn-client", "tag": "OVPN", "servers": [{"server": "o1.example.com", "server_port": 1194, "network": "tcp"},
			{"server": "o2.example.com", "server_port": 1195}], "username": "u", "password": "pw",
			"tls": {"certificate": ["` + strings.ReplaceAll(testCA, "\n", `", "`) + `"], "server_name": "o.example.com",
				"control_wrap": {"type": "tls_crypt", "key": "` + strings.ReplaceAll(testTLSCrypt, "\n", `\n`) + `"}},
			"data_ciphers": "AES-128-GCM", "ping_interval": "10s"},
		{"type": "openconnect", "tag": "Office", "server": "https://gw.example.com:8443/staff", "flavor": "gp", "username": "u",
			"password": "pw", "tls": {"peer_fingerprint": "pin-sha256:abc="}},
		{"type": "masque-client", "tag": "MASQUE", "server": "m.example.com", "username": "u", "password": "pw", "path": "/.well-known/masque/ip/",
			"version": 3, "tls": {"enabled": true, "server_name": "m.example.com"}},
		{"type": "openvpn-client", "tag": "Files", "server": "f.example.com", "server_port": 1194, "tls": {"certificate_path": "ca.crt"}}
	]}`
	d := mustParseDocument(t, body)
	want := []Profile{
		{Type: TypeOpenVPN, Name: "OVPN", Server: "o1.example.com", ServerPort: 1194, OpenVPN: &OpenVPN{
			Network: "tcp", Remotes: []OpenVPNRemote{{Server: "o2.example.com", Port: 1195}}, Username: "u", Password: "pw",
			CA: testCA, VerifyName: "o.example.com", ControlWrap: OpenVPNControlWrap{Type: "tls_crypt", Key: testTLSCrypt},
			DataCiphers: []string{"AES-128-GCM"}, PingInterval: "10s",
		}},
		{Type: TypeOpenConnect, Name: "Office", Server: "gw.example.com", ServerPort: 8443, OpenConnect: &OpenConnect{
			Flavor: "gp", Path: "/staff", Username: "u", Password: "pw", PeerFingerprint: []string{"pin-sha256:abc="},
		}},
		{Type: TypeMASQUE, Name: "MASQUE", Server: "m.example.com", ServerPort: 443, MASQUE: &MASQUE{
			Username: "u", Password: "pw", Path: "/.well-known/masque/ip/", Version: 3, TLS: TLS{Enabled: true, ServerName: "m.example.com"},
		}},
	}
	if !reflect.DeepEqual(d.Profiles, want) {
		for i := range min(len(d.Profiles), len(want)) {
			t.Errorf("%d\n got %s\nwant %s", i, dump(d.Profiles[i]), dump(want[i]))
		}
		t.Fatalf("profiles %q", names(d.Profiles))
	}
	if len(d.Skipped) != 1 || !strings.Contains(d.Skipped[0].Reason, "from a file") {
		t.Errorf("skipped %+v", d.Skipped)
	}
	for _, p := range want {
		if link := ShareLink(p); !strings.HasPrefix(link, "poros://add/") || !reflect.DeepEqual(mustParseLink(t, link), p) {
			t.Errorf("%s: %s, want a poros://add/ link that reads back", p.Name, link)
		}
	}
}

func TestValidateVPNProfiles(t *testing.T) {
	base := func() *OpenVPN { return &OpenVPN{CA: testCA} }
	cases := map[string]struct {
		edit        func(*OpenVPN)
		want        string
		unsupported bool
	}{
		"a certificate without its key": {func(o *OpenVPN) { o.Cert = testCA }, "go together", false},
		"a bad compression":             {func(o *OpenVPN) { o.Compression = "zstd" }, "compression", true},
		"a CA that is no PEM":           {func(o *OpenVPN) { o.CA = "abc" }, "PEM", false},
		"a wrap without a key":          {func(o *OpenVPN) { o.ControlWrap.Type = "tls_crypt" }, "key of the control", false},
		"a remote without a port":       {func(o *OpenVPN) { o.Remotes = []OpenVPNRemote{{Server: "r.example.com"}} }, "remote", false},
	}
	for name, tt := range cases {
		o := base()
		tt.edit(o)
		p := Profile{Type: TypeOpenVPN, Server: "v.example.com", ServerPort: 1194, OpenVPN: o}
		if err := p.Validate(); err == nil || !strings.Contains(err.Error(), tt.want) || errors.Is(err, ErrUnsupported) != tt.unsupported {
			t.Errorf("%s: %v, want %q", name, err, tt.want)
		}
	}
	by := func() *OpenVPN {
		return &OpenVPN{CA: testCA, TLSCipher: []string{"DHE-BIGN-WITH-BELT-DWP-HBELT"}, DataCiphers: []string{"belt-cbc256"}, Auth: "bash512"}
	}
	byCases := map[string]struct {
		edit        func(*OpenVPN)
		want        string
		unsupported bool
	}{
		"no suites":          {func(o *OpenVPN) { o.TLSCipher = nil }, "BTLS suites", false},
		"a suite of OpenSSL": {func(o *OpenVPN) { o.TLSCipher = []string{"ECDHE-RSA-AES256-GCM-SHA384"} }, "BTLS suites alone", true},
		"TLS 1.3":            {func(o *OpenVPN) { o.TLSVersionMax = "1.3" }, "TLS 1.2", true},
		"AES":                {func(o *OpenVPN) { o.DataCiphers = []string{"AES-256-GCM"} }, "belt ciphers alone", true},
		"no cipher":          {func(o *OpenVPN) { o.DataCiphers = nil }, "belt cipher", false},
		"no digest":          {func(o *OpenVPN) { o.Auth = "" }, "digest", false},
		"SHA-256":            {func(o *OpenVPN) { o.Auth = "SHA256" }, "bash", true},
	}
	for name, tt := range byCases {
		o := by()
		tt.edit(o)
		p := Profile{Type: TypeOpenVPNBY, Server: "v.example.com", ServerPort: 443, OpenVPN: o}
		if err := p.Validate(); err == nil || !strings.Contains(err.Error(), tt.want) || errors.Is(err, ErrUnsupported) != tt.unsupported {
			t.Errorf("OpenVPN|BY, %s: %v, want %q", name, err, tt.want)
		}
	}
	o := by()
	o.KeyPassword, o.Cipher, o.Auth = "pw", "BELT-CFB256", "BASH512"
	if err := (Profile{Type: TypeOpenVPNBY, Server: "v.example.com", ServerPort: 443, OpenVPN: o}).Validate(); err != nil {
		t.Errorf("OpenVPN|BY takes the names in any case and a key password: %v", err)
	}
	if err := (Profile{Type: TypeOpenVPN, Server: "v.example.com", ServerPort: 443, OpenVPN: o}).Validate(); !errors.Is(err, ErrUnsupported) {
		t.Errorf("plain OpenVPN has no suites of BTLS and no key password: %v", err)
	}
	if err := (Profile{Type: TypeOpenVPNBY, Server: "v.example.com", ServerPort: 443, OpenConnect: &OpenConnect{}}).Validate(); err == nil {
		t.Error("OpenVPN|BY lives in the part of OpenVPN")
	}
	openConnect := Profile{Type: TypeOpenConnect, Server: "gw.example.com", ServerPort: 443, OpenConnect: &OpenConnect{Flavor: "juniper"}}
	if err := openConnect.Validate(); !errors.Is(err, ErrUnsupported) {
		t.Errorf("an unknown OpenConnect gateway: %v", err)
	}
	masque := Profile{Type: TypeMASQUE, Server: "m.example.com", ServerPort: 443, MASQUE: &MASQUE{}}
	if err := masque.Validate(); err == nil || !strings.Contains(err.Error(), "TLS") {
		t.Errorf("MASQUE without TLS: %v", err)
	}
}
