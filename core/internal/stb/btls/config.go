package btls

import (
	"bytes"
	"crypto/rand"
	"crypto/tls"
	"crypto/x509"
	"encoding/pem"
	"errors"
	"io"
	"time"

	"PorosCore/internal/stb/stbkey"
)

// Certificate is a chain of certificates, leaf first, with the bign private key of the leaf.
type Certificate struct {
	Chain      [][]byte
	PrivateKey *stbkey.PrivateKey
	Leaf       *x509.Certificate
}

// Config configures a client or a server. A Config may be shared by connections and must not change once in use.
type Config struct {
	// Certificates holds the certificate of the server, or of the client when the server asks for one; only the
	// first one is used.
	Certificates []Certificate
	// RootCAs are the authorities a client checks the chain of the server with.
	RootCAs []*x509.Certificate
	// ClientCAs are the authorities a server checks the chain of the client with.
	ClientCAs []*x509.Certificate
	// ClientAuth is the policy of a server for certificates of clients, as in crypto/tls.
	ClientAuth tls.ClientAuthType
	// InsecureSkipVerify makes a client accept any chain of the server; VerifyPeerCertificate still runs.
	InsecureSkipVerify bool
	// VerifyPeerCertificate, if set, runs after the chain check with the certificates as they came and the
	// verified chains, if any. An error fails the handshake.
	VerifyPeerCertificate func(rawCerts [][]byte, verifiedChains [][]*x509.Certificate) error
	// CipherSuites lists the suites in the order of preference; nil means DefaultCipherSuites.
	CipherSuites []uint16
	// Rand is the source of randomness; nil means crypto/rand.
	Rand io.Reader
	// Time returns the current time for the validity of certificates; nil means time.Now.
	Time func() time.Time
}

func (c *Config) rand() io.Reader {
	if c.Rand != nil {
		return c.Rand
	}
	return rand.Reader
}

func (c *Config) now() time.Time {
	if c.Time != nil {
		return c.Time()
	}
	return time.Now()
}

func (c *Config) cipherSuites() []*cipherSuite {
	ids := c.CipherSuites
	if ids == nil {
		ids = DefaultCipherSuites()
	}
	var out []*cipherSuite
	for _, id := range ids {
		if s := cipherSuiteByID(id); s != nil {
			out = append(out, s)
		}
	}
	return out
}

func (c *Config) certificate() (*Certificate, error) {
	if len(c.Certificates) == 0 {
		return nil, nil
	}
	cert := &c.Certificates[0]
	if len(cert.Chain) == 0 || cert.PrivateKey == nil {
		return nil, errors.New("btls: the certificate has no chain or no private key")
	}
	total := 0
	for _, der := range cert.Chain {
		total += 3 + len(der)
	}
	if total > maxCertificateListLength {
		return nil, errors.New("btls: the certificate chain is too long")
	}
	return cert, nil
}

// ParseCertificates reads the certificates of a PEM text.
func ParseCertificates(data []byte) ([]*x509.Certificate, error) {
	var certs []*x509.Certificate
	for rest := data; ; {
		var block *pem.Block
		block, rest = pem.Decode(rest)
		if block == nil {
			break
		}
		if block.Type != "CERTIFICATE" {
			continue
		}
		cert, err := x509.ParseCertificate(block.Bytes)
		if err != nil {
			return nil, err
		}
		certs = append(certs, cert)
	}
	if len(certs) == 0 {
		return nil, errors.New("btls: no certificate in the PEM text")
	}
	return certs, nil
}

// X509KeyPair reads a PEM chain, leaf first, and the bign private key of the leaf in PEM or DER, with the password
// when the key is protected.
func X509KeyPair(certPEM, keyPEM, password []byte) (Certificate, error) {
	certs, err := ParseCertificates(certPEM)
	if err != nil {
		return Certificate{}, err
	}
	key, err := stbkey.ParsePrivateKey(keyPEM, password)
	if err != nil {
		return Certificate{}, err
	}
	pub, err := stbkey.ParsePublicKeyInfo(certs[0].RawSubjectPublicKeyInfo)
	if err != nil {
		return Certificate{}, err
	}
	mine, err := key.PublicKey()
	if err != nil {
		return Certificate{}, err
	}
	if pub.Curve != key.Curve || !bytes.Equal(pub.Point, mine) {
		return Certificate{}, errors.New("btls: the private key does not match the certificate")
	}
	cert := Certificate{PrivateKey: key, Leaf: certs[0]}
	for _, c := range certs {
		cert.Chain = append(cert.Chain, c.Raw)
	}
	return cert, nil
}
