package btls

import (
	"errors"
	"io"
	"net"
	"strings"
	"testing"

	"PorosCore/internal/stb/bign"
)

// tamperConn rewrites the records one side receives: edit gets each record with its header and returns what to
// deliver, and whether the stream ends after it.
type tamperConn struct {
	net.Conn
	edit     func(record []byte) ([]byte, bool)
	raw, out []byte
	eof      bool
}

func (c *tamperConn) Read(p []byte) (int, error) {
	for len(c.out) == 0 {
		if c.eof {
			return 0, io.EOF
		}
		if len(c.raw) >= recordHeaderLength {
			n := recordHeaderLength + (int(c.raw[3])<<8 | int(c.raw[4]))
			if len(c.raw) >= n {
				record := append([]byte(nil), c.raw[:n]...)
				c.raw = c.raw[n:]
				c.out, c.eof = c.edit(record)
				continue
			}
		}
		buf := make([]byte, 4096)
		m, err := c.Conn.Read(buf)
		c.raw = append(c.raw, buf[:m]...)
		if err != nil && m == 0 {
			return 0, err
		}
	}
	n := copy(p, c.out)
	c.out = c.out[n:]
	return n, nil
}

func tamper(edit func(record []byte) ([]byte, bool)) func(net.Conn) net.Conn {
	return func(c net.Conn) net.Conn { return &tamperConn{Conn: c, edit: edit} }
}

// isHandshakeRecord tells a plaintext record that starts a handshake message of the type.
func isHandshakeRecord(record []byte, typ uint8) bool {
	return len(record) > recordHeaderLength && recordType(record[0]) == recordTypeHandshake && record[recordHeaderLength] == typ
}

func flipLast(typ uint8) func([]byte) ([]byte, bool) {
	return func(record []byte) ([]byte, bool) {
		if isHandshakeRecord(record, typ) {
			record[len(record)-1] ^= 1
		}
		return record, false
	}
}

func TestTamperedHandshake(t *testing.T) {
	p := newPKI(t, bign.Curve256)
	t.Run("ServerKeyExchange", func(t *testing.T) {
		clientCfg, serverCfg := p.configs(TLS_DHE_BIGN_WITH_BELT_DWP_HBELT)
		r := handshake(t, clientCfg, serverCfg, tamper(flipLast(typeServerKeyExchange)), nil)
		if r.clientErr == nil || !strings.Contains(r.clientErr.Error(), "invalid signature of the server key exchange") || r.serverErr == nil {
			t.Fatalf("client %v, server %v", r.clientErr, r.serverErr)
		}
	})
	t.Run("CertificateVerify", func(t *testing.T) {
		clientCfg, serverCfg := p.configs(TLS_DHT_BIGN_WITH_BELT_DWP_HBELT)
		r := handshake(t, clientCfg, serverCfg, nil, tamper(flipLast(typeCertificateVerify)))
		if r.serverErr == nil || !strings.Contains(r.serverErr.Error(), "invalid signature by the client certificate") || r.clientErr == nil {
			t.Fatalf("client %v, server %v", r.clientErr, r.serverErr)
		}
	})
	t.Run("ServerHello", func(t *testing.T) {
		// Without ServerKeyExchange only Finished notices a changed random of the server.
		clientCfg, serverCfg := p.configs(TLS_DHT_BIGN_WITH_BELT_CTR_MAC_HBELT)
		r := handshake(t, clientCfg, serverCfg, tamper(func(record []byte) ([]byte, bool) {
			if isHandshakeRecord(record, typeServerHello) {
				record[recordHeaderLength+4+2+5] ^= 1
			}
			return record, false
		}), nil)
		if r.clientErr == nil || r.serverErr == nil {
			t.Fatalf("client %v, server %v", r.clientErr, r.serverErr)
		}
	})
	t.Run("ClientKeyExchange", func(t *testing.T) {
		clientCfg, serverCfg := p.configs(TLS_DHT_BIGN_WITH_BELT_DWP_HBELT)
		r := handshake(t, clientCfg, serverCfg, nil, tamper(flipLast(typeClientKeyExchange)))
		if r.serverErr == nil || !strings.Contains(r.serverErr.Error(), "invalid key token") || r.clientErr == nil {
			t.Fatalf("client %v, server %v", r.clientErr, r.serverErr)
		}
	})
}

func TestTamperedRecords(t *testing.T) {
	p := newPKI(t, bign.Curve256)
	for _, suite := range []uint16{TLS_DHE_BIGN_WITH_BELT_DWP_HBELT, TLS_DHE_BIGN_WITH_BELT_CTR_MAC_HBELT} {
		edits := map[string]func([]byte) ([]byte, bool){
			"first byte": func(record []byte) ([]byte, bool) {
				record[recordHeaderLength] ^= 0x80
				return record, false
			},
			"last byte": func(record []byte) ([]byte, bool) {
				record[len(record)-1] ^= 1
				return record, false
			},
			"type": func(record []byte) ([]byte, bool) {
				record[0] = byte(recordTypeHandshake)
				return record, false
			},
			"replayed": func(record []byte) ([]byte, bool) {
				return append(record, record...), false
			},
			"truncated": func(record []byte) ([]byte, bool) {
				return record[:len(record)-3], true
			},
		}
		for name, edit := range edits {
			t.Run(CipherSuiteName(suite)+"/"+name, func(t *testing.T) {
				clientCfg, serverCfg := p.configs(suite)
				seen := 0
				r := handshake(t, clientCfg, serverCfg, tamper(func(record []byte) ([]byte, bool) {
					if recordType(record[0]) != recordTypeApplicationData {
						return record, false
					}
					if seen++; seen == 2 {
						return edit(record)
					}
					return record, false
				}), nil)
				if r.clientErr != nil || r.serverErr != nil {
					t.Fatalf("handshake: client %v, server %v", r.clientErr, r.serverErr)
				}
				go func() {
					r.server.Write([]byte("first"))
					r.server.Write([]byte("second"))
				}()
				buf := make([]byte, 16)
				if n, err := r.client.Read(buf); err != nil || string(buf[:n]) != "first" {
					t.Fatalf("first record: %q, %v", buf[:n], err)
				}
				n, err := r.client.Read(buf)
				if name == "replayed" {
					if err != nil || string(buf[:n]) != "second" {
						t.Fatalf("second record: %q, %v", buf[:n], err)
					}
					n, err = r.client.Read(buf)
				}
				switch {
				case name == "truncated":
					if !errors.Is(err, io.ErrUnexpectedEOF) {
						t.Fatalf("read: %q, %v", buf[:n], err)
					}
				case err == nil || !strings.Contains(err.Error(), "bad record MAC") && !strings.Contains(err.Error(), "unexpected message"):
					t.Fatalf("read: %q, %v", buf[:n], err)
				}
				if _, again := r.client.Read(buf); again == nil {
					t.Fatal("the connection works after a forged record")
				}
			})
		}
	}
}
