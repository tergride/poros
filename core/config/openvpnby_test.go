package config

import (
	"crypto/rand"
	"crypto/x509"
	"errors"
	"strings"
	"testing"

	"github.com/sagernet/sing-box/option"

	"PorosCore/internal/stb/stbkey"
	"PorosCore/internal/stb/stbtest"
	"PorosCore/profile"
)

// testBignClient is a bign CA and a client pair it issued, in PEM, with the key under password as the containers of
// OpenVPN|BY keep it.
func testBignClient(t *testing.T, password string) (ca, cert, key string, plain *stbkey.PrivateKey) {
	t.Helper()
	authority, err := stbtest.Issue(stbtest.Options{Name: "Poros test CA", IsCA: true, KeyUsage: x509.KeyUsageCertSign}, nil)
	if err != nil {
		t.Fatal(err)
	}
	client, err := stbtest.Issue(stbtest.Options{Name: "client", KeyUsage: x509.KeyUsageDigitalSignature, ExtKeyUsage: []x509.ExtKeyUsage{x509.ExtKeyUsageClientAuth}}, authority)
	if err != nil {
		t.Fatal(err)
	}
	locked, err := stbkey.Encrypt(client.Key, []byte(password), rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	return string(stbtest.PEM(authority.Cert)), string(stbtest.PEM(client.Cert)), string(locked), client.Key
}

// The key of OpenVPN|BY reaches sing-box open, with BTLS over TLS 1.2 for the control channel; a password that does
// not open it stops the build with the name of the server.
func TestOpenVPNBYKey(t *testing.T) {
	ca, cert, key, plain := testBignClient(t, "secret")
	p := profile.Profile{Type: profile.TypeOpenVPNBY, Name: "Office", Server: "vpn.example.com", ServerPort: 443, OpenVPN: &profile.OpenVPN{
		Network: "tcp", CA: ca, Cert: cert, Key: key, KeyPassword: "secret", TLSCipher: []string{"DHE-BIGN-WITH-BELT-DWP-HBELT", "DHT-BIGN-WITH-BELT-DWP-HBELT"},
		DataCiphers: []string{"belt-cbc256"}, Auth: "bash512",
	}}
	if err := p.Validate(); err != nil {
		t.Fatal(err)
	}
	endpoint, err := openVPNEndpoint(p, "proxy")
	if err != nil {
		t.Fatal(err)
	}
	tls := endpoint.Options.(*option.OpenVPNClientEndpointOptions).TLS
	if tls.Cipher != "DHE-BIGN-WITH-BELT-DWP-HBELT:DHT-BIGN-WITH-BELT-DWP-HBELT" || tls.VersionMin != "1.2" || tls.VersionMax != "1.2" || len(tls.ClientKey) != 1 {
		t.Fatalf("the TLS of OpenVPN|BY: %+v", tls)
	}
	open := tls.ClientKey[0]
	got, err := stbkey.ParsePrivateKey([]byte(open), nil)
	if err != nil || stbkey.IsEncrypted([]byte(open)) || strings.Contains(open, "ENCRYPTED") {
		t.Fatalf("sing-box gets the key open: %v", err)
	}
	want, _ := stbkey.MarshalPKCS8(plain)
	if der, err := stbkey.MarshalPKCS8(got); err != nil || string(der) != string(want) {
		t.Errorf("the open key is another one: %v", err)
	}

	for name, password := range map[string]string{"a wrong password": "wrong", "no password": ""} {
		p.OpenVPN.KeyPassword = password
		_, err := Build(Input{Server: p, Preset: allPreset, MixedPort: 2090})
		var locked *KeyPasswordError
		if !errors.As(err, &locked) || locked.Server != "Office" {
			t.Errorf("%s: %v", name, err)
		}
	}
	p.OpenVPN.KeyPassword = "wrong"
	trojan := profile.Profile{Type: profile.TypeTrojan, Server: "t.example.com", ServerPort: 443, Trojan: &profile.Trojan{Password: "pw"}}
	_, err = Build(Input{Server: trojan, Preset: Preset{Name: "Office", DefaultOutbound: OutboundProxy, Endpoints: []string{"by"}}, MixedPort: 2090,
		Endpoints: []profile.Profile{p}})
	if locked := (*KeyPasswordError)(nil); !errors.As(err, &locked) || !errors.Is(err, stbkey.ErrBadPassword) {
		t.Errorf("an endpoint whose key stays shut: %v", err)
	}
}
