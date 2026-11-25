package btls

import (
	"bytes"
	"crypto/tls"
	"net"
	"sync"
	"testing"
	"time"

	"PorosCore/internal/stb/bign"
)

// scriptedConn plays the bytes of a peer and swallows what it is sent.
type scriptedConn struct {
	r *bytes.Reader
}

func (c *scriptedConn) Read(p []byte) (int, error)         { return c.r.Read(p) }
func (c *scriptedConn) Write(p []byte) (int, error)        { return len(p), nil }
func (c *scriptedConn) Close() error                       { return nil }
func (c *scriptedConn) LocalAddr() net.Addr                { return &net.TCPAddr{} }
func (c *scriptedConn) RemoteAddr() net.Addr               { return &net.TCPAddr{} }
func (c *scriptedConn) SetDeadline(t time.Time) error      { return nil }
func (c *scriptedConn) SetReadDeadline(t time.Time) error  { return nil }
func (c *scriptedConn) SetWriteDeadline(t time.Time) error { return nil }

// recordingConn keeps what one side received.
type recordingConn struct {
	net.Conn
	mu   sync.Mutex
	read bytes.Buffer
}

func (c *recordingConn) Read(p []byte) (int, error) {
	n, err := c.Conn.Read(p)
	c.mu.Lock()
	c.read.Write(p[:n])
	c.mu.Unlock()
	return n, err
}

var fuzzPKI struct {
	once sync.Once
	p    pki
}

func fuzzSetup(t testing.TB) pki {
	fuzzPKI.once.Do(func() { fuzzPKI.p = newPKI(t, bign.Curve256) })
	return fuzzPKI.p
}

// recordSession returns what the client and the server received in a real handshake with some data.
func recordSession(t testing.TB, suite uint16) (clientGot, serverGot []byte) {
	p := fuzzSetup(t)
	clientCfg, serverCfg := p.configs(suite)
	var clientRec, serverRec *recordingConn
	r := handshake(t, clientCfg, serverCfg,
		func(c net.Conn) net.Conn { clientRec = &recordingConn{Conn: c}; return clientRec },
		func(c net.Conn) net.Conn { serverRec = &recordingConn{Conn: c}; return serverRec })
	if r.clientErr != nil || r.serverErr != nil {
		t.Fatalf("client %v, server %v", r.clientErr, r.serverErr)
	}
	r.server.Write([]byte("hello"))
	buf := make([]byte, 5)
	r.client.Read(buf)
	r.client.Close()
	r.server.Close()
	return clientRec.read.Bytes(), serverRec.read.Bytes()
}

// FuzzClient feeds a client with any answers of a server: they come from the network.
func FuzzClient(f *testing.F) {
	for _, suite := range DefaultCipherSuites() {
		got, _ := recordSession(f, suite)
		f.Add(got)
	}
	p := fuzzSetup(f)
	f.Fuzz(func(t *testing.T, data []byte) {
		clientCfg, _ := p.configs(nil...)
		c := Client(&scriptedConn{bytes.NewReader(data)}, clientCfg)
		if c.Handshake() == nil {
			c.Read(make([]byte, 64))
		}
	})
}

// FuzzServer feeds a server with any messages of a client.
func FuzzServer(f *testing.F) {
	for _, suite := range DefaultCipherSuites() {
		_, got := recordSession(f, suite)
		f.Add(got)
	}
	p := fuzzSetup(f)
	f.Fuzz(func(t *testing.T, data []byte) {
		_, serverCfg := p.configs(nil...)
		serverCfg.ClientAuth = tls.RequestClientCert
		c := Server(&scriptedConn{bytes.NewReader(data)}, serverCfg)
		if c.Handshake() == nil {
			c.Read(make([]byte, 64))
		}
	})
}
