// Package ovpn plugs the cryptography of OpenVPN|BY into sing-openvpn: BTLS for the control channel when tls-cipher
// names BTLS suites, belt for the data channel and bash and belt-hash for HMAC. Importing the package registers them.
package ovpn

import (
	"crypto/tls"
	"crypto/x509"
	"errors"
	"fmt"
	"net"
	"slices"
	"strings"
	"time"

	openvpn "github.com/sagernet/sing-openvpn"

	"PorosCore/internal/stb/bash"
	"PorosCore/internal/stb/belt"
	"PorosCore/internal/stb/btls"
)

// The belt ciphers are Chained: servers of OpenVPN|BY run belt-cbc and belt-cfb of the data channel as one chain per
// key and direction from a zero IV and take no IV of a packet. In CBC the chain suits a server that takes the IV as
// well; in CFB it suits only such servers.
func init() {
	openvpn.RegisterTLSProvider(provider{})
	for _, size := range []int{128, 192, 256} {
		key := size / 8
		openvpn.RegisterDataCipher(fmt.Sprintf("belt-cbc%d", size), openvpn.DataCipher{Mode: openvpn.DataCipherCBC, KeySize: key, NewBlock: belt.NewCipher, Chained: true})
		openvpn.RegisterDataCipher(fmt.Sprintf("belt-cfb%d", size), openvpn.DataCipher{Mode: openvpn.DataCipherCFB, KeySize: key, NewBlock: belt.NewCipher, Chained: true})
	}
	openvpn.RegisterDigest("belt-hash", belt.NewHash)
	openvpn.RegisterDigest("bash256", bash.New256)
	openvpn.RegisterDigest("bash384", bash.New384)
	openvpn.RegisterDigest("bash512", bash.New512)
}

type provider struct{}

// Serves takes a tls-cipher list that names a BTLS suite; one that mixes them with other suites is refused when
// the connection is built rather than handed to crypto/tls, which knows none of them.
func (provider) Serves(cipherList string) bool {
	for _, name := range strings.Split(cipherList, ":") {
		if _, ok := btls.CipherSuiteByName(name); ok {
			return true
		}
	}
	return false
}

func (provider) Client(options openvpn.TLSProviderOptions) (func(net.Conn) openvpn.TLSConn, error) {
	config, err := newConfig(options)
	if err != nil {
		return nil, err
	}
	return func(c net.Conn) openvpn.TLSConn { return conn{btls.Client(c, config)} }, nil
}

func (provider) Server(options openvpn.TLSProviderOptions) (func(net.Conn) openvpn.TLSConn, error) {
	config, err := newConfig(options)
	if err != nil {
		return nil, err
	}
	if len(config.Certificates) == 0 {
		return nil, errors.New("BTLS: the server needs a certificate and its key")
	}
	config.ClientAuth = options.ClientAuth
	return func(c net.Conn) openvpn.TLSConn { return conn{btls.Server(c, config)} }, nil
}

func (provider) VerifyChain(certificates []*x509.Certificate, roots []*x509.Certificate) ([]*x509.Certificate, error) {
	return btls.VerifyChain(certificates, roots, time.Now())
}

// newConfig leaves the chain to VerifyPeerCertificate, which is sing-openvpn checking it with VerifyChain and then
// purposes and names. tls-groups is not used: DHE of BTLS runs on the curve of the server certificate.
func newConfig(options openvpn.TLSProviderOptions) (*btls.Config, error) {
	if options.VersionMin > tls.VersionTLS12 || options.VersionMax != 0 && options.VersionMax < tls.VersionTLS12 {
		return nil, errors.New("BTLS: only TLS 1.2 is defined")
	}
	config := &btls.Config{InsecureSkipVerify: true}
	for _, name := range strings.Split(options.CipherList, ":") {
		id, ok := btls.CipherSuiteByName(name)
		if !ok {
			return nil, fmt.Errorf("BTLS: tls-cipher %s is not a BTLS suite, and BTLS takes no other", name)
		}
		if !slices.Contains(config.CipherSuites, id) {
			config.CipherSuites = append(config.CipherSuites, id)
		}
	}
	if len(options.Certificate) > 0 {
		cert, err := btls.X509KeyPair(options.Certificate, options.Key, nil)
		if err != nil {
			return nil, fmt.Errorf("BTLS: %w", err)
		}
		config.Certificates = []btls.Certificate{cert}
	}
	if verify := options.VerifyPeerCertificate; verify != nil {
		config.VerifyPeerCertificate = func(raw [][]byte, _ [][]*x509.Certificate) error { return verify(raw) }
	}
	return config, nil
}

type conn struct {
	*btls.Conn
}

func (c conn) PeerCertificates() []*x509.Certificate {
	return c.ConnectionState().PeerCertificates
}
