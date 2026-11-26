package openvpn

import (
	"crypto/cipher"
	"crypto/tls"
	"crypto/x509"
	"hash"
	"net"
	"strings"
	"sync"
)

// TLSConn is the TLS connection of the control channel: crypto/tls, or the one of a TLSProvider.
type TLSConn interface {
	net.Conn
	Handshake() error
	// PeerCertificates are the certificates the peer sent, leaf first.
	PeerCertificates() []*x509.Certificate
	ExportKeyingMaterial(label string, context []byte, length int) ([]byte, error)
}

// TLSProvider serves the control channel with TLS of its own for the tls-cipher lists crypto/tls has no suites of.
type TLSProvider interface {
	// Serves reports whether the tls-cipher list is the provider's.
	Serves(cipherList string) bool
	// Client and Server return the constructor of connections for a role.
	Client(options TLSProviderOptions) (func(net.Conn) TLSConn, error)
	Server(options TLSProviderOptions) (func(net.Conn) TLSConn, error)
	// VerifyChain builds the chain of the peer, leaf first, up to one of the roots and checks its signatures and
	// validity. Purpose, key usage, names and fingerprints are checked by the package afterwards.
	VerifyChain(certificates []*x509.Certificate, roots []*x509.Certificate) ([]*x509.Certificate, error)
}

// TLSProviderOptions are the TLS options of a client or a server, resolved by the package.
type TLSProviderOptions struct {
	CipherList string
	Groups     string
	VersionMin uint16
	// VersionMax is zero when tls-version-max is not set.
	VersionMax uint16
	// Certificate is the PEM chain of the local certificate and Key its private key; both are empty for a client
	// without a certificate.
	Certificate []byte
	Key         []byte
	// ClientAuth is the policy of a server for the certificates of clients.
	ClientAuth tls.ClientAuthType
	// VerifyPeerCertificate checks the certificates of the peer as they came; the provider calls it during the
	// handshake, skipping its own chain check, and fails the handshake on an error. A server without client
	// certificates gets nil.
	VerifyPeerCertificate func(rawCertificates [][]byte) error
}

// DataCipherMode is how a registered cipher runs on the data channel.
type DataCipherMode uint8

const (
	DataCipherCBC DataCipherMode = iota
	DataCipherCFB
	DataCipherOFB
)

// DataCipher is a block cipher the package does not carry itself, for the data channel in TLS mode and, in CBC
// mode, for static keys.
type DataCipher struct {
	Mode     DataCipherMode
	KeySize  int
	NewBlock func(key []byte) (cipher.Block, error)
	// Chained serves a peer that runs the cipher in TLS mode as one chain per key and direction, from a zero IV,
	// and leaves the IV of each packet aside. In CFB and OFB the data channel then runs one stream per key and
	// direction. In CBC a packet goes out with the last ciphertext block of the one before as its IV, which a peer
	// that takes the IV reads the same, and a packet that comes in is read with whichever of its own IV and that
	// chain gives the packet ID expected next. The chain holds only while packets keep their order, as on TCP.
	Chained bool
}

var extensions struct {
	access       sync.RWMutex
	tlsProviders []TLSProvider
	dataCiphers  map[string]DataCipher
	digests      map[string]func() hash.Hash
}

// RegisterTLSProvider adds a provider; the first one that serves a tls-cipher list takes it.
func RegisterTLSProvider(provider TLSProvider) {
	extensions.access.Lock()
	defer extensions.access.Unlock()
	extensions.tlsProviders = append(extensions.tlsProviders, provider)
}

// RegisterDataCipher adds a data channel cipher. Names compare without regard to case, as servers push them as
// their crypto library spells them.
func RegisterDataCipher(name string, dataCipher DataCipher) {
	extensions.access.Lock()
	defer extensions.access.Unlock()
	if extensions.dataCiphers == nil {
		extensions.dataCiphers = make(map[string]DataCipher)
	}
	extensions.dataCiphers[strings.ToUpper(name)] = dataCipher
}

// RegisterDigest adds a digest for HMAC of the data channel and of tls-auth; names compare without regard to case.
func RegisterDigest(name string, newHash func() hash.Hash) {
	extensions.access.Lock()
	defer extensions.access.Unlock()
	if extensions.digests == nil {
		extensions.digests = make(map[string]func() hash.Hash)
	}
	extensions.digests[strings.ToUpper(name)] = newHash
}

func tlsProviderFor(cipherList string) TLSProvider {
	if cipherList == "" {
		return nil
	}
	extensions.access.RLock()
	defer extensions.access.RUnlock()
	for _, provider := range extensions.tlsProviders {
		if provider.Serves(cipherList) {
			return provider
		}
	}
	return nil
}

func registeredDataCipher(name string) (DataCipher, bool) {
	extensions.access.RLock()
	defer extensions.access.RUnlock()
	dataCipher, found := extensions.dataCiphers[strings.ToUpper(name)]
	return dataCipher, found
}

func registeredDigest(name string) (func() hash.Hash, bool) {
	extensions.access.RLock()
	defer extensions.access.RUnlock()
	newHash, found := extensions.digests[strings.ToUpper(name)]
	return newHash, found
}

// standardTLSConn is a connection of crypto/tls.
type standardTLSConn struct {
	*tls.Conn
}

func (c standardTLSConn) PeerCertificates() []*x509.Certificate {
	return c.ConnectionState().PeerCertificates
}

func (c standardTLSConn) ExportKeyingMaterial(label string, context []byte, length int) ([]byte, error) {
	connectionState := c.ConnectionState()
	return connectionState.ExportKeyingMaterial(label, context, length)
}
