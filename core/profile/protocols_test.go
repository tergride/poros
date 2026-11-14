package profile

import (
	"encoding/base64"
	"errors"
	"reflect"
	"strings"
	"testing"
)

const testSSHKey = "-----BEGIN OPENSSH PRIVATE KEY-----\nb3BlbnNzaC1rZXktdjEAAAAA\n-----END OPENSSH PRIVATE KEY-----"

// The protocols of the main sing-box beyond the first nine: each link, Clash proxy and sing-box outbound reads into
// the same profile.
func TestReadMoreProtocols(t *testing.T) {
	wireGuard := Profile{Type: TypeWireGuard, Name: "WG", Server: "wg.example.com", ServerPort: 51820, WireGuard: &WireGuard{
		PrivateKey: testWGPrivate, PeerPublicKey: testWGPublic, PreSharedKey: testWGPublic,
		Address: []string{"172.16.0.2/32", "fd01::2/128"}, Reserved: []int{1, 2, 3}, MTU: 1280, PersistentKeepalive: 25,
	}}
	hysteria := Profile{Type: TypeHysteria, Name: "HY", Server: "hy.example.com", ServerPort: 443, Hysteria: &Hysteria{
		Auth: "pw", UpMbps: 50, DownMbps: 200, Obfs: "ob", TLS: TLS{Enabled: true, ServerName: "sni.example.com", ALPN: []string{"hysteria"}},
	}}
	naive := Profile{Type: TypeNaive, Name: "NV", Server: "nv.example.com", ServerPort: 443, Naive: &Naive{
		Username: "user", Password: "pw", QUIC: true, QUICCongestionControl: "bbr", InsecureConcurrency: 2,
		ExtraHeaders: map[string]string{"X-Token": "abc"}, TLS: TLS{Enabled: true, ServerName: "nv.example.com"},
	}}
	ssh := Profile{Type: TypeSSH, Name: "SSH", Server: "ssh.example.com", ServerPort: 2222, SSH: &SSH{
		User: "root", Password: "pw", PrivateKey: testSSHKey, HostKey: []string{"ssh-ed25519 AAAAC3NzaC1lZDI1NTE5"},
	}}
	snell := Profile{Type: TypeSnell, Name: "SN", Server: "sn.example.com", ServerPort: 8443, Snell: &Snell{
		Version: 4, PSK: "psk", Reuse: true, ObfsMode: "http", ObfsHost: "bing.com",
	}}
	b64 := func(text string) string { return base64.RawStdEncoding.EncodeToString([]byte(text)) }

	links := map[string]Profile{
		"wg://" + strings.NewReplacer("+", "%2B", "/", "%2F", "=", "%3D").Replace(testWGPrivate) + "@wg.example.com:51820" +
			"?publickey=" + testWGPublic + "&psk=" + testWGPublic + "&address=172.16.0.2,fd01::2/128&reserved=1-2-3&mtu=1280&keepalive=25#WG": wireGuard,
		"hysteria://hy.example.com:443?protocol=udp&auth=pw&peer=sni.example.com&upmbps=50&downmbps=200%20Mbps&obfs=xplus&obfsParam=ob&alpn=hysteria#HY":    hysteria,
		"naive+quic://user:pw@nv.example.com:443?sni=nv.example.com&congestion_control=bbr&insecure-concurrency=2&extra-headers=X-Token%3A%20abc#NV":        naive,
		"ssh://ssh.example.com:2222?user=root&password=pw&private_key=" + b64(testSSHKey) + "&host_key=" + b64("ssh-ed25519 AAAAC3NzaC1lZDI1NTE5") + "#SSH": ssh,
		"snell://psk@sn.example.com:8443?version=4&reuse=1&obfs=http&obfs-host=bing.com#SN":                                                                 snell,
	}
	for link, want := range links {
		if got := mustParseLink(t, link); !reflect.DeepEqual(got, want) {
			t.Errorf("%s\n got %s\nwant %s", link, dump(got), dump(want))
		}
	}

	clash := `proxies:
  - {name: WG, type: wireguard, server: wg.example.com, port: 51820, private-key: ` + testWGPrivate + `, ip: 172.16.0.2, ipv6: fd01::2,
     mtu: 1280, persistent-keepalive: 25, peers: [{server: wg.example.com, port: 51820, public-key: ` + testWGPublic + `,
     pre-shared-key: ` + testWGPublic + `, reserved: [1, 2, 3]}]}
  - {name: HY, type: hysteria, server: hy.example.com, port: 443, auth-str: pw, up: 50 Mbps, down: "200", obfs: ob,
     sni: sni.example.com, alpn: [hysteria]}
  - {name: SSH, type: ssh, server: ssh.example.com, port: 2222, username: root, password: pw, private-key: "` +
		strings.ReplaceAll(testSSHKey, "\n", `\n`) + `", host-key: [ssh-ed25519 AAAAC3NzaC1lZDI1NTE5]}
  - {name: SN, type: snell, server: sn.example.com, port: 8443, psk: psk, version: 3}
  - {name: AWG, type: wireguard, server: a.example.com, port: 51820, private-key: x, amnezia-wg-option: {jc: 4}}
`
	d := mustParseDocument(t, clash)
	if want := []Profile{wireGuard, hysteria, ssh}; !reflect.DeepEqual(d.Profiles, want) {
		for i := range min(len(d.Profiles), len(want)) {
			t.Errorf("Clash %d\n got %s\nwant %s", i, dump(d.Profiles[i]), dump(want[i]))
		}
		t.Errorf("Clash profiles %q", names(d.Profiles))
	}
	if len(d.Skipped) != 2 || !d.Skipped[0].Unsupported || !strings.Contains(d.Skipped[0].Reason, "versions 4 and 6") ||
		!d.Skipped[1].Unsupported || !strings.Contains(d.Skipped[1].Reason, "AmneziaWG") {
		t.Errorf("Clash skipped %+v", d.Skipped)
	}

	box := `{"outbounds": [
		{"type": "hysteria", "tag": "HY", "server": "hy.example.com", "server_port": 443, "auth": "` + base64.StdEncoding.EncodeToString([]byte("pw")) + `",
			"up": "50 Mbps", "down_mbps": 200, "obfs": "ob", "tls": {"enabled": true, "server_name": "sni.example.com", "alpn": "hysteria"}},
		{"type": "naive", "tag": "NV", "server": "nv.example.com", "server_port": 443, "username": "user", "password": "pw", "quic": true,
			"quic_congestion_control": "bbr", "insecure_concurrency": 2, "extra_headers": {"X-Token": ["abc"]},
			"tls": {"enabled": true, "server_name": "nv.example.com"}},
		{"type": "ssh", "tag": "SSH", "server": "ssh.example.com", "server_port": 2222, "user": "root", "password": "pw",
			"private_key": ["` + strings.ReplaceAll(testSSHKey, "\n", `", "`) + `"], "host_key": "ssh-ed25519 AAAAC3NzaC1lZDI1NTE5"},
		{"type": "snell", "tag": "SN", "server": "sn.example.com", "server_port": 8443, "version": 4, "psk": "psk", "reuse": true,
			"obfs_mode": "http", "obfs_host": "bing.com"},
		{"type": "wireguard", "tag": "WG", "server": "wg.example.com", "server_port": 51820, "local_address": ["172.16.0.2/32", "fd01::2/128"],
			"private_key": "` + testWGPrivate + `", "peer_public_key": "` + testWGPublic + `", "pre_shared_key": "` + testWGPublic + `",
			"reserved": "AQID", "mtu": 1280}
	]}`
	legacyWireGuard := wireGuard
	part := *wireGuard.WireGuard
	part.PersistentKeepalive = 0
	legacyWireGuard.WireGuard = &part
	d = mustParseDocument(t, box)
	if want := []Profile{hysteria, naive, ssh, snell, legacyWireGuard}; !reflect.DeepEqual(d.Profiles, want) || len(d.Skipped) > 0 {
		for i := range min(len(d.Profiles), len(want)) {
			t.Errorf("sing-box %d\n got %s\nwant %s", i, dump(d.Profiles[i]), dump(want[i]))
		}
		t.Errorf("sing-box profiles %q, skipped %+v", names(d.Profiles), d.Skipped)
	}
}

func TestHysteria2Realm(t *testing.T) {
	d := mustParseDocument(t, `{"type": "hysteria2", "tag": "Realm", "password": "pw", "tls": {"enabled": true, "server_name": "hy.example.com"},
		"realm": {"server_url": "https://realm.example.com:8443", "token": "t", "realm_id": "home", "stun_servers": ["stun.example.com:3478"]}}`)
	want := Profile{Type: TypeHysteria2, Name: "Realm", Server: "realm.example.com", ServerPort: 8443, Hysteria2: &Hysteria2{
		Password: "pw", TLS: TLS{Enabled: true, ServerName: "hy.example.com"},
		Realm: HysteriaRealm{ServerURL: "https://realm.example.com:8443", Token: "t", RealmID: "home", STUNServers: []string{"stun.example.com:3478"}},
	}}
	if len(d.Profiles) != 1 || !reflect.DeepEqual(d.Profiles[0], want) {
		t.Fatalf("realm: %q, skipped %+v", names(d.Profiles), d.Skipped)
	}
	hopping := want
	part := *want.Hysteria2
	part.ServerPorts = []string{"20000:30000"}
	hopping.Hysteria2 = &part
	if err := hopping.Validate(); err == nil || !strings.Contains(err.Error(), "port hopping") {
		t.Errorf("a realm with port hopping: %v", err)
	}
	if link := ShareLink(want); !strings.HasPrefix(link, "poros://add/") {
		t.Errorf("no share link carries a realm: %s", link)
	}
}

func TestWireGuardConfigFile(t *testing.T) {
	config := "[Interface]\r\nPrivateKey = " + testWGPrivate + "\r\nAddress = 172.16.0.2/32, fd01::2/128\r\nDNS = 1.1.1.1\r\nMTU = 1280\r\n\r\n" +
		"[Peer]\r\nPublicKey = " + testWGPublic + "\r\nPresharedKey = " + testWGPublic + " # the provider's\r\n" +
		"AllowedIPs = 0.0.0.0/0, ::/0\r\nEndpoint = [2001:db8::1]:51820\r\nPersistentKeepalive = 25\r\n"
	d := mustParseDocument(t, config)
	want := Profile{Type: TypeWireGuard, Server: "2001:db8::1", ServerPort: 51820, WireGuard: &WireGuard{
		PrivateKey: testWGPrivate, PeerPublicKey: testWGPublic, PreSharedKey: testWGPublic,
		Address: []string{"172.16.0.2/32", "fd01::2/128"}, MTU: 1280, PersistentKeepalive: 25,
	}}
	if d.Format != FormatWireGuard || len(d.Profiles) != 1 || !reflect.DeepEqual(d.Profiles[0], want) {
		t.Fatalf("format %q, profiles %q, skipped %+v", d.Format, names(d.Profiles), d.Skipped)
	}
	amnezia := mustParseDocument(t, strings.Replace(config, "MTU = 1280", "MTU = 1280\r\nJc = 4", 1))
	if len(amnezia.Profiles) != 0 || len(amnezia.Skipped) != 1 || !amnezia.Skipped[0].Unsupported {
		t.Errorf("an AmneziaWG config: %q, skipped %+v", names(amnezia.Profiles), amnezia.Skipped)
	}
}

func TestValidateMoreProtocols(t *testing.T) {
	base := func(kind string) Profile { return Profile{Type: kind, Server: "x.example.com", ServerPort: 443} }
	shadowTLS := base(TypeShadowsocks)
	shadowTLS.Shadowsocks = &Shadowsocks{Method: "aes-128-gcm", Password: "pw", Plugin: "obfs-local",
		ShadowTLS: ShadowTLS{Version: 3, Password: "pw", TLS: TLS{Enabled: true, ServerName: "www.apple.com"}}}
	noHandshakeSite := base(TypeShadowsocks)
	noHandshakeSite.Shadowsocks = &Shadowsocks{Method: "aes-128-gcm", Password: "pw", ShadowTLS: ShadowTLS{Version: 3, Password: "pw"}}
	wireGuard := base(TypeWireGuard)
	wireGuard.WireGuard = &WireGuard{PrivateKey: testWGPrivate, PeerPublicKey: testWGPublic, Address: []string{"172.16.0.2/32"}, Reserved: []int{1, 2}}
	naivePin := base(TypeNaive)
	naivePin.Naive = &Naive{TLS: TLS{Enabled: true, CertificateSHA256: []string{"pin"}}}
	sshPublicKey := base(TypeSSH)
	sshPublicKey.SSH = &SSH{PrivateKey: "ssh-ed25519 AAAAC3NzaC1lZDI1NTE5"}
	snellMode := base(TypeSnell)
	snellMode.Snell = &Snell{Version: 4, PSK: "psk", Mode: "unshaped"}
	for name, tt := range map[string]struct {
		p           Profile
		want        string
		unsupported bool
	}{
		"ShadowTLS with a plugin":  {shadowTLS, "plugin", false},
		"ShadowTLS without a site": {noHandshakeSite, "server name", false},
		"two reserved bytes":       {wireGuard, "three numbers", false},
		"a pin over NaiveProxy":    {naivePin, "pin a certificate", true},
		"a public key for SSH":     {sshPublicKey, "PRIVATE KEY", false},
		"a version 6 mode for v4":  {snellMode, "version 6", false},
	} {
		err := tt.p.Validate()
		if err == nil || !strings.Contains(err.Error(), tt.want) || errors.Is(err, ErrUnsupported) != tt.unsupported {
			t.Errorf("%s: %v, want %q (unsupported %v)", name, err, tt.want, tt.unsupported)
		}
	}
}
