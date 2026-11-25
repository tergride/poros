package btls

import (
	"crypto/x509"
	"errors"
	"os"
	"path/filepath"
	"testing"
	"time"

	"PorosCore/internal/stb/bign"
	"PorosCore/internal/stb/stbtest"
)

func TestVerifyChain(t *testing.T) {
	ca := issue(t, stbtest.Options{Name: "root", Curve: bign.Curve512, IsCA: true, KeyUsage: x509.KeyUsageCertSign}, nil)
	mid := issue(t, stbtest.Options{Name: "intermediate", Curve: bign.Curve384, IsCA: true, ZeroPathLen: true}, ca)
	leaf := issue(t, stbtest.Options{Name: "leaf"}, mid)
	now := time.Now()
	roots := []*x509.Certificate{ca.Cert}

	chain, err := VerifyChain([]*x509.Certificate{leaf.Cert, mid.Cert}, roots, now)
	if err != nil || len(chain) != 3 || chain[2] != ca.Cert {
		t.Fatalf("chain %v, %v", chain, err)
	}
	if _, err := VerifyChain([]*x509.Certificate{leaf.Cert}, roots, now); !errors.Is(err, ErrUnknownAuthority) {
		t.Fatalf("without the intermediate: %v", err)
	}
	if _, err := VerifyChain([]*x509.Certificate{leaf.Cert, mid.Cert}, roots, now.Add(48*time.Hour)); err == nil {
		t.Fatal("an expired chain verified")
	}
	if chain, err := VerifyChain([]*x509.Certificate{mid.Cert}, []*x509.Certificate{mid.Cert}, now); err != nil || len(chain) != 1 {
		t.Fatalf("a trusted certificate itself: %v", err)
	}

	// A forged intermediate with the same name but its own key does not sign the leaf.
	forged := issue(t, stbtest.Options{Name: "intermediate", Curve: bign.Curve384, IsCA: true}, ca)
	if _, err := VerifyChain([]*x509.Certificate{leaf.Cert, forged.Cert}, roots, now); err == nil {
		t.Fatal("a leaf verified under a forged intermediate")
	}
	notCA := issue(t, stbtest.Options{Name: "not ca"}, ca)
	below := issue(t, stbtest.Options{Name: "below"}, notCA)
	if _, err := VerifyChain([]*x509.Certificate{below.Cert, notCA.Cert}, roots, now); err == nil {
		t.Fatal("a certificate that is no authority issued one")
	}
	noSign := issue(t, stbtest.Options{Name: "no sign", IsCA: true, KeyUsage: x509.KeyUsageDigitalSignature}, ca)
	under := issue(t, stbtest.Options{Name: "under"}, noSign)
	if _, err := VerifyChain([]*x509.Certificate{under.Cert, noSign.Cert}, roots, now); err == nil {
		t.Fatal("an authority without keyCertSign issued one")
	}
	// The intermediate allows no other authority below it.
	sub := issue(t, stbtest.Options{Name: "sub", IsCA: true}, mid)
	deep := issue(t, stbtest.Options{Name: "deep"}, sub)
	if _, err := VerifyChain([]*x509.Certificate{deep.Cert, sub.Cert, mid.Cert}, roots, now); err == nil {
		t.Fatal("the path length was ignored")
	}
}

// TestFixtures runs handshakes with the keys and certificates of the BTLS test server of the standard when
// POROS_BTLS_FIXTURES points at its folder server; OpenSSL with bee2evp made them, and they are not in the repository.
func TestFixtures(t *testing.T) {
	dir := os.Getenv("POROS_BTLS_FIXTURES")
	if dir == "" {
		t.Skip("POROS_BTLS_FIXTURES is not set")
	}
	for _, level := range []string{"256", "384", "512"} {
		certPEM, err := os.ReadFile(filepath.Join(dir, "btls"+level, "cert"+level+".pem"))
		if err != nil {
			t.Fatal(err)
		}
		keyPEM, err := os.ReadFile(filepath.Join(dir, "btls"+level, "priv"+level+".key"))
		if err != nil {
			t.Fatal(err)
		}
		cert, err := X509KeyPair(certPEM, keyPEM, nil)
		if err != nil {
			t.Fatalf("cert%s: %v", level, err)
		}
		if string(cert.Leaf.RawIssuer) == string(cert.Leaf.RawSubject) {
			if err := CheckSignature(cert.Leaf, cert.Leaf); err != nil {
				t.Fatalf("cert%s: %v", level, err)
			}
			t.Logf("cert%s: the self-signature checks out", level)
		}
		for _, suite := range []uint16{TLS_DHE_BIGN_WITH_BELT_DWP_HBELT, TLS_DHT_BIGN_WITH_BELT_CTR_MAC_HBELT} {
			clientCfg := &Config{InsecureSkipVerify: true, CipherSuites: []uint16{suite}}
			serverCfg := &Config{Certificates: []Certificate{cert}}
			r := handshake(t, clientCfg, serverCfg, nil, nil)
			if r.clientErr != nil || r.serverErr != nil {
				t.Fatalf("cert%s, %s: client %v, server %v", level, CipherSuiteName(suite), r.clientErr, r.serverErr)
			}
			r.client.Close()
			r.server.Close()
		}
	}
}
