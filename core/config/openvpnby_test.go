package config

import (
	"crypto/rand"
	"crypto/x509"
	"errors"
	"slices"
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

// cipher of a config is what OpenVPN 2.6 makes of it in TLS mode, where sing-box takes none: the fallback, and without
// data-ciphers one more cipher offered; OpenVPN|BY offers belt alone.
func TestOpenVPNCipherInTLSMode(t *testing.T) {
	for name, tt := range map[string]struct {
		kind             string
		cipher, fallback string
		data             []string
		want             []string
		wantFallback     string
	}{
		"cipher alone":               {profile.TypeOpenVPN, "AES-256-CBC", "", nil, []string{"AES-256-GCM", "AES-128-GCM", "CHACHA20-POLY1305", "AES-256-CBC"}, "AES-256-CBC"},
		"a default cipher":           {profile.TypeOpenVPN, "aes-256-gcm", "", nil, openVPNDefaultCiphers, "aes-256-gcm"},
		"with data-ciphers":          {profile.TypeOpenVPN, "AES-256-CBC", "", []string{"AES-128-GCM"}, []string{"AES-128-GCM"}, "AES-256-CBC"},
		"with a fallback of its own": {profile.TypeOpenVPN, "AES-256-CBC", "BF-CBC", nil, []string{"AES-256-GCM", "AES-128-GCM", "CHACHA20-POLY1305", "AES-256-CBC"}, "BF-CBC"},
		"no cipher":                  {profile.TypeOpenVPN, "", "", []string{"AES-128-GCM"}, []string{"AES-128-GCM"}, ""},
		"OpenVPN|BY":                 {profile.TypeOpenVPNBY, "belt-cfb256", "", nil, []string{"belt-cfb256"}, "belt-cfb256"},
	} {
		p := profile.Profile{Type: tt.kind, OpenVPN: &profile.OpenVPN{Cipher: tt.cipher, DataCiphersFallback: tt.fallback, DataCiphers: tt.data}}
		if got, fallback := openVPNDataCiphers(p); !slices.Equal(got, tt.want) || fallback != tt.wantFallback {
			t.Errorf("%s: %v, %q", name, got, fallback)
		}
	}
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
