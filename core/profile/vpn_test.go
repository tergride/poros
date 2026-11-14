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
	openConnect := Profile{Type: TypeOpenConnect, Server: "gw.example.com", ServerPort: 443, OpenConnect: &OpenConnect{Flavor: "juniper"}}
	if err := openConnect.Validate(); !errors.Is(err, ErrUnsupported) {
		t.Errorf("an unknown OpenConnect gateway: %v", err)
	}
	masque := Profile{Type: TypeMASQUE, Server: "m.example.com", ServerPort: 443, MASQUE: &MASQUE{}}
	if err := masque.Validate(); err == nil || !strings.Contains(err.Error(), "TLS") {
		t.Errorf("MASQUE without TLS: %v", err)
	}
}
