package btls

import (
	"bytes"
	"context"
	"crypto/rand"
	"crypto/tls"
	"crypto/x509"
	"encoding/hex"
	"errors"
	"io"
	"net"
	"strings"
	"sync"
	"testing"
	"time"

	"PorosCore/internal/stb/belt"
	"PorosCore/internal/stb/bign"
	"PorosCore/internal/stb/stbtest"
)

// TestPHash checks P_hash against brng-hmac of STB 34.101.47 (Б.4), which is the same generator.
func TestPHash(t *testing.T) {
	out := make([]byte, 96)
	pHash(out, belt.H[128:160], belt.H[192:224])
	want := "AF907A0E470A3A1B268ECCCCC0B90F239FE94A2DC6E014179FC789CB3C3887E4" +
		"695C6B96B84948F8D76924E22260859DB9B5FE757BEDA2E17103EE44655A9FEF" +
		"648077CCC5002E0561C6EF512C513B8C24B4F3A157221CFBC1597E969778C1E4"
	if got := strings.ToUpper(hex.EncodeToString(out)); got != want {
		t.Fatalf("P_hash:\n got %s\nwant %s", got, want)
	}
	short := make([]byte, 2)
	pHash(short, belt.H[128:129], belt.H[192:193])
	if got := strings.ToUpper(hex.EncodeToString(short)); got != "42B1" {
		t.Fatalf("P_hash with a short key: got %s", got)
	}
}

type pki struct {
	ca, server, client *stbtest.Identity
}

func issue(t testing.TB, opts stbtest.Options, issuer *stbtest.Identity) *stbtest.Identity {
	t.Helper()
	id, err := stbtest.Issue(opts, issuer)
	if err != nil {
		t.Fatal(err)
	}
	return id
}

func newPKI(t testing.TB, curve *bign.Curve) pki {
	ca := issue(t, stbtest.Options{Name: "Test CA", Curve: curve, IsCA: true, KeyUsage: x509.KeyUsageCertSign | x509.KeyUsageCRLSign}, nil)
	leaf := func(name string, usage x509.ExtKeyUsage) *stbtest.Identity {
		return issue(t, stbtest.Options{Name: name, Curve: curve,
			KeyUsage:    x509.KeyUsageDigitalSignature | x509.KeyUsageKeyEncipherment,
			ExtKeyUsage: []x509.ExtKeyUsage{usage}}, ca)
	}
	return pki{ca: ca, server: leaf("server", x509.ExtKeyUsageServerAuth), client: leaf("client", x509.ExtKeyUsageClientAuth)}
}

func certificateOf(id *stbtest.Identity) Certificate {
	return Certificate{Chain: [][]byte{id.Cert.Raw}, PrivateKey: id.Key, Leaf: id.Cert}
}

func (p pki) configs(suites ...uint16) (client, server *Config) {
	client = &Config{
		Certificates: []Certificate{certificateOf(p.client)},
		RootCAs:      []*x509.Certificate{p.ca.Cert},
		CipherSuites: suites,
	}
	server = &Config{
		Certificates: []Certificate{certificateOf(p.server)},
		ClientCAs:    []*x509.Certificate{p.ca.Cert},
		ClientAuth:   tls.RequireAndVerifyClientCert,
		CipherSuites: suites,
	}
	return client, server
}

// tcpPair connects two ends over loopback TCP; a deadline keeps a broken test from hanging.
func tcpPair(t testing.TB) (net.Conn, net.Conn) {
	t.Helper()
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	defer ln.Close()
	accepted := make(chan net.Conn, 1)
	go func() {
		c, _ := ln.Accept()
		accepted <- c
	}()
	client, err := net.Dial("tcp", ln.Addr().String())
	if err != nil {
		t.Fatal(err)
	}
	server := <-accepted
	if server == nil {
		t.Fatal("accept failed")
	}
	deadline := time.Now().Add(time.Minute)
	client.SetDeadline(deadline)
	server.SetDeadline(deadline)
	t.Cleanup(func() {
		client.Close()
		server.Close()
	})
	return client, server
}

type handshakeResult struct {
	client, server       *Conn
	clientErr, serverErr error
}

func handshake(t testing.TB, clientCfg, serverCfg *Config, wrapClient, wrapServer func(net.Conn) net.Conn) handshakeResult {
	t.Helper()
	cc, sc := tcpPair(t)
	if wrapClient != nil {
		cc = wrapClient(cc)
	}
	if wrapServer != nil {
		sc = wrapServer(sc)
	}
	r := handshakeResult{client: Client(cc, clientCfg), server: Server(sc, serverCfg)}
	var wg sync.WaitGroup
	wg.Add(1)
	go func() {
		defer wg.Done()
		r.serverErr = r.server.Handshake()
		if r.serverErr != nil {
			r.server.NetConn().Close()
		}
	}()
	r.clientErr = r.client.Handshake()
	if r.clientErr != nil {
		r.client.NetConn().Close()
	}
	wg.Wait()
	return r
}

func TestLoopback(t *testing.T) {
	cases := []struct {
		curve  *bign.Curve
		suites []uint16
	}{
		{bign.Curve256, DefaultCipherSuites()},
		{bign.Curve384, []uint16{TLS_DHE_BIGN_WITH_BELT_DWP_HBELT, TLS_DHT_BIGN_WITH_BELT_CTR_MAC_HBELT}},
		{bign.Curve512, []uint16{TLS_DHT_BIGN_WITH_BELT_DWP_HBELT, TLS_DHE_BIGN_WITH_BELT_CTR_MAC_HBELT}},
	}
	for _, tc := range cases {
		p := newPKI(t, tc.curve)
		for _, suite := range tc.suites {
			t.Run(tc.curve.Name+"/"+CipherSuiteName(suite), func(t *testing.T) {
				clientCfg, serverCfg := p.configs(suite)
				r := handshake(t, clientCfg, serverCfg, nil, nil)
				if r.clientErr != nil || r.serverErr != nil {
					t.Fatalf("handshake: client %v, server %v", r.clientErr, r.serverErr)
				}
				checkSession(t, r, suite)
				exchange(t, r.client, r.server)
			})
		}
	}
}

func checkSession(t *testing.T, r handshakeResult, suite uint16) {
	t.Helper()
	cs, ss := r.client.ConnectionState(), r.server.ConnectionState()
	if cs.CipherSuite != suite || ss.CipherSuite != suite || !cs.ExtendedMasterSecret || !ss.ExtendedMasterSecret {
		t.Fatalf("state: client %+v, server %+v", cs, ss)
	}
	if len(cs.PeerCertificates) != 1 || cs.PeerCertificates[0].Subject.CommonName != "server" || len(cs.VerifiedChains) != 1 {
		t.Fatalf("client sees %v", cs.PeerCertificates)
	}
	if len(ss.PeerCertificates) != 1 || ss.PeerCertificates[0].Subject.CommonName != "client" || len(ss.VerifiedChains) != 1 {
		t.Fatalf("server sees %v", ss.PeerCertificates)
	}
	for _, context := range [][]byte{nil, {}, []byte("context")} {
		a, err := r.client.ExportKeyingMaterial("EXPORTER-OpenVPN-datakeys", context, 256)
		if err != nil {
			t.Fatal(err)
		}
		b, _ := r.server.ExportKeyingMaterial("EXPORTER-OpenVPN-datakeys", context, 256)
		if !bytes.Equal(a, b) || len(a) != 256 {
			t.Fatalf("exported keys differ with context %q", context)
		}
	}
	a, _ := r.client.ExportKeyingMaterial("EXPORTER-OpenVPN-datakeys", nil, 32)
	b, _ := r.client.ExportKeyingMaterial("EXPORTER-OpenVPN-datakeys", []byte{}, 32)
	c, _ := r.client.ExportKeyingMaterial("EXPORTER-OpenVPN-p2p-peerid", nil, 32)
	if bytes.Equal(a, b) || bytes.Equal(a, c) {
		t.Fatal("the exporter ignores the context or the label")
	}
	if _, err := r.client.ExportKeyingMaterial(labelKeyExpansion, nil, 32); err == nil {
		t.Fatal("a reserved label was accepted")
	}
}

// exchange sends data larger than a record both ways at once and closes with close_notify.
func exchange(t *testing.T, client, server *Conn) {
	t.Helper()
	up := make([]byte, 3*maxPlaintext+123)
	down := make([]byte, 2*maxPlaintext+7)
	rand.Read(up)
	rand.Read(down)
	var wg sync.WaitGroup
	var serverGot []byte
	var serverErr error
	wg.Add(2)
	go func() {
		defer wg.Done()
		serverGot, serverErr = io.ReadAll(server)
	}()
	go func() {
		defer wg.Done()
		if _, err := server.Write(down); err != nil {
			t.Error(err)
		}
		if err := server.CloseWrite(); err != nil {
			t.Error(err)
		}
	}()
	if _, err := client.Write(up); err != nil {
		t.Fatal(err)
	}
	if err := client.CloseWrite(); err != nil {
		t.Fatal(err)
	}
	clientGot, err := io.ReadAll(client)
	wg.Wait()
	if err != nil || serverErr != nil {
		t.Fatalf("read: client %v, server %v", err, serverErr)
	}
	if !bytes.Equal(clientGot, down) || !bytes.Equal(serverGot, up) {
		t.Fatal("data changed on the way")
	}
	if _, err := client.Write([]byte("late")); !errors.Is(err, errShutdown) {
		t.Fatalf("write after close_notify: %v", err)
	}
	client.Close()
	server.Close()
}

func TestNoClientCertificate(t *testing.T) {
	p := newPKI(t, bign.Curve256)
	clientCfg, serverCfg := p.configs(TLS_DHE_BIGN_WITH_BELT_DWP_HBELT)
	clientCfg.Certificates = nil
	serverCfg.ClientAuth = tls.NoClientCert
	r := handshake(t, clientCfg, serverCfg, nil, nil)
	if r.clientErr != nil || r.serverErr != nil {
		t.Fatalf("handshake: client %v, server %v", r.clientErr, r.serverErr)
	}
	if len(r.server.ConnectionState().PeerCertificates) != 0 {
		t.Fatal("the server got a certificate it did not ask for")
	}
	serverCfg.ClientAuth = tls.RequireAndVerifyClientCert
	r = handshake(t, clientCfg, serverCfg, nil, nil)
	if r.serverErr == nil || !strings.Contains(r.serverErr.Error(), "did not provide a certificate") || r.clientErr == nil {
		t.Fatalf("a required certificate was missing: client %v, server %v", r.clientErr, r.serverErr)
	}
}

func TestUntrustedServer(t *testing.T) {
	p := newPKI(t, bign.Curve256)
	other := newPKI(t, bign.Curve256)
	clientCfg, serverCfg := p.configs(nil...)
	// The other authority has the same name, so the signature tells them apart.
	clientCfg.RootCAs = []*x509.Certificate{other.ca.Cert}
	r := handshake(t, clientCfg, serverCfg, nil, nil)
	var invalid *CertificateInvalidError
	if !errors.As(r.clientErr, &invalid) || invalid.Reason != "has a bad signature" || r.serverErr == nil ||
		!strings.Contains(r.serverErr.Error(), "bad certificate") {
		t.Fatalf("client %v, server %v", r.clientErr, r.serverErr)
	}
	clientCfg.RootCAs = []*x509.Certificate{issue(t, stbtest.Options{Name: "Another CA", IsCA: true}, nil).Cert}
	r = handshake(t, clientCfg, serverCfg, nil, nil)
	if !errors.Is(r.clientErr, ErrUnknownAuthority) || r.serverErr == nil || !strings.Contains(r.serverErr.Error(), "unknown certificate authority") {
		t.Fatalf("client %v, server %v", r.clientErr, r.serverErr)
	}
	// InsecureSkipVerify leaves the decision to VerifyPeerCertificate.
	clientCfg.InsecureSkipVerify = true
	called := false
	clientCfg.VerifyPeerCertificate = func(raw [][]byte, chains [][]*x509.Certificate) error {
		called = len(raw) == 1 && chains == nil
		return nil
	}
	r = handshake(t, clientCfg, serverCfg, nil, nil)
	if r.clientErr != nil || r.serverErr != nil || !called {
		t.Fatalf("client %v, server %v, called %v", r.clientErr, r.serverErr, called)
	}
	clientCfg.VerifyPeerCertificate = func([][]byte, [][]*x509.Certificate) error { return errors.New("pinned elsewhere") }
	r = handshake(t, clientCfg, serverCfg, nil, nil)
	if r.clientErr == nil || r.clientErr.Error() != "pinned elsewhere" {
		t.Fatalf("client %v", r.clientErr)
	}
}

func TestExpiredServer(t *testing.T) {
	p := newPKI(t, bign.Curve256)
	p.server = issue(t, stbtest.Options{Name: "server", NotBefore: time.Now().Add(-48 * time.Hour), NotAfter: time.Now().Add(-time.Hour)}, p.ca)
	clientCfg, serverCfg := p.configs(nil...)
	r := handshake(t, clientCfg, serverCfg, nil, nil)
	var invalid *CertificateInvalidError
	if !errors.As(r.clientErr, &invalid) || invalid.Reason != "has expired" {
		t.Fatalf("client %v", r.clientErr)
	}
}

func TestContextCancel(t *testing.T) {
	cc, _ := tcpPair(t)
	ctx, cancel := context.WithTimeout(context.Background(), 100*time.Millisecond)
	defer cancel()
	err := Client(cc, &Config{InsecureSkipVerify: true}).HandshakeContext(ctx)
	if !errors.Is(err, context.DeadlineExceeded) {
		t.Fatalf("handshake against a silent peer: %v", err)
	}
}
